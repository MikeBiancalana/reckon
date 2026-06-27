package cli

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/spf13/cobra"
)

// Query command flag variables. These are package-global so cobra can bind them;
// RunE resets them (and their Changed state) via deferred cleanup so the shared
// RootCmd singleton stays clean across repeated Execute calls (notably in tests).
var (
	queryViewFlag   string
	queryRawFlag    bool
	queryFieldsFlag string
	queryLimitFlag  int
	queryLangFlag   string
)

// queryCmd is the read-only retrieval surface: it runs a single SELECT against
// the T2 index's stable views and emits canonical-node NDJSON (default) or raw
// result rows (--raw). It opens the index database read-only and never touches
// the legacy operational database (requiresDB=false).
var queryCmd = &cobra.Command{
	Use:   "query [SQL]",
	Short: "Query the vault index (read-only SQL over stable views)",
	Long: "Run a read-only SQL SELECT against the vault index's stable views " +
		"(nodes, edges, node_props, aliases, fts). Results are emitted as canonical " +
		"node NDJSON by default, or as raw result rows with --raw. The query is " +
		"executed against a read-only connection and non-SELECT statements are rejected.\n\n" +
		"Full-text search uses the fts_search vtable, e.g.:\n" +
		"  rk query \"SELECT id FROM fts_search WHERE fts_search MATCH 'hello'\"\n" +
		"Rank results with ORDER BY bm25(fts_search). The fts view exposes the same " +
		"columns (id, body) for plain scans but does not support MATCH.",
	Annotations:  map[string]string{"requiresDB": "false"},
	SilenceUsage: true,
	Args:         cobra.ArbitraryArgs,
	RunE:         runQueryE,
}

func init() {
	f := queryCmd.Flags()
	f.StringVar(&queryViewFlag, "view", "", "Run a named saved view from <vault>/.reckon/views/<name>.sql")
	f.BoolVar(&queryRawFlag, "raw", false, "Emit raw result rows instead of canonical node envelopes")
	f.StringVar(&queryFieldsFlag, "fields", "", "Comma-separated list of fields to emit (token economy)")
	f.IntVar(&queryLimitFlag, "limit", 0, "Cap the number of emitted rows (>= 0)")
	f.StringVar(&queryLangFlag, "lang", "sql", "Query language (only \"sql\" is supported)")
}

// resetQueryFlags restores query flag variables to their defaults and clears the
// pflag Changed state. Called via defer so each invocation is self-contained.
func resetQueryFlags(cmd *cobra.Command) {
	queryViewFlag = ""
	queryRawFlag = false
	queryFieldsFlag = ""
	queryLimitFlag = 0
	queryLangFlag = "sql"
	for _, name := range []string{"view", "raw", "fields", "limit", "lang"} {
		if fl := cmd.Flags().Lookup(name); fl != nil {
			fl.Changed = false
		}
	}
}

func runQueryE(cmd *cobra.Command, args []string) error {
	defer resetQueryFlags(cmd)

	// Capture this invocation's flag state up front (before deferred reset).
	view := queryViewFlag
	raw := queryRawFlag
	lang := queryLangFlag
	fields := parseFields(queryFieldsFlag)
	limitSet := cmd.Flags().Changed("limit")
	limit := queryLimitFlag

	// --lang is a seam: only "sql" is implemented.
	if lang != "" && lang != "sql" {
		return fmt.Errorf("query: unsupported language: %s", lang)
	}

	if limitSet && limit < 0 {
		return fmt.Errorf("query: --limit must be >= 0, got %d", limit)
	}

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}
	// Query results are data, not a status line: default (Pretty) emits NDJSON.
	if mode == output.Pretty {
		mode = output.NDJSON
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("query: load config: %w", err)
	}

	sqlText, err := resolveQuerySQL(cfg, view, args)
	if err != nil {
		return err
	}

	if err := validateReadOnlySQL(sqlText); err != nil {
		return err
	}

	dbPath, err := index.DBPath(cfg)
	if err != nil {
		return fmt.Errorf("query: resolve index path: %w", err)
	}
	if _, statErr := os.Stat(dbPath); errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("query: index not found at %q — run `rk index` first", dbPath)
	}

	db, err := openReadOnlyIndex(dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	rows, err := db.Query(sqlText)
	if err != nil {
		return fmt.Errorf("query: execute: %w", err)
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return fmt.Errorf("query: columns: %w", err)
	}

	// Collect result rows (bounded by --limit) into generic maps.
	records, err := scanRows(rows, cols, limitSet, limit)
	if err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("query: iterate: %w", err)
	}

	w := output.New(cmd.OutOrStdout(), mode)

	// Canonical mode reconstructs full node envelopes when the result carries a
	// node identity ("id" column); otherwise (or with --raw) emit raw rows.
	canonical := !raw && hasColumn(cols, "id")

	var objects []any
	if canonical {
		objects, err = canonicalObjects(db, records, fields, cmd)
		if err != nil {
			return err
		}
	} else {
		objects = rawObjects(records, fields, cmd)
	}

	return emit(w, mode, objects)
}

// parseFields splits a comma-separated --fields value into trimmed, non-empty
// names. Returns nil when no fields were requested.
func parseFields(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// resolveQuerySQL determines the SQL to run from either a saved view or an inline
// positional argument. The two are mutually exclusive.
func resolveQuerySQL(cfg *config.Config, view string, args []string) (string, error) {
	inline := strings.TrimSpace(strings.Join(args, " "))
	if view != "" && inline != "" {
		return "", errors.New("query: --view and an inline SQL argument are mutually exclusive")
	}
	if view != "" {
		return loadView(cfg, view)
	}
	if inline == "" {
		return "", errors.New("query: no query provided (give SQL or --view NAME)")
	}
	return inline, nil
}

// loadView reads a saved view's SQL from <vault>/.reckon/views/<name>.sql.
func loadView(cfg *config.Config, name string) (string, error) {
	if name != filepath.Base(name) || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return "", fmt.Errorf("query: invalid view name %q", name)
	}
	path := filepath.Join(cfg.VaultDir, ".reckon", "views", name+".sql")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("query: view %q not found (looked in %s)", name, filepath.Dir(path))
	}
	if err != nil {
		return "", fmt.Errorf("query: read view %q: %w", name, err)
	}
	if strings.TrimSpace(string(data)) == "" {
		return "", fmt.Errorf("query: view %q is empty", name)
	}
	return string(data), nil
}

// openReadOnlyIndex opens the index database read-only. The file: URI form is
// required: modernc.org/sqlite strips the query string from non-file: DSNs and
// would otherwise open read-write.
func openReadOnlyIndex(dbPath string) (*sql.DB, error) {
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		return nil, fmt.Errorf("query: resolve db path: %w", err)
	}
	u := url.URL{Scheme: "file", Path: abs}
	u.RawQuery = "mode=ro"
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("query: open index (read-only): %w", err)
	}
	return db, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Statement validation (Layer 2 — defense-in-depth over the read-only connection)
// ─────────────────────────────────────────────────────────────────────────────

var (
	denyRe    = regexp.MustCompile(`\b(INSERT|UPDATE|DELETE|DROP|ALTER|CREATE|ATTACH|DETACH|REINDEX|VACUUM|PRAGMA|TRIGGER|GRANT|RENAME)\b`)
	privateRe = regexp.MustCompile(`\b_[A-Za-z]\w*`)
	wordRe    = regexp.MustCompile(`[A-Za-z]+`)
)

// validateReadOnlySQL rejects anything that is not a single read-only SELECT/CTE.
// It is defense-in-depth: the read-only connection is the real safety net, but
// these checks produce friendly, actionable errors.
func validateReadOnlySQL(raw string) error {
	scrub := scrubSQL(raw)
	trimmed := strings.TrimSpace(scrub)
	if trimmed == "" {
		return errors.New("query: empty query")
	}

	// Reject multiple statements (one optional trailing ';' is allowed).
	noTrail := strings.TrimRight(trimmed, " \t\r\n")
	noTrail = strings.TrimSuffix(noTrail, ";")
	if strings.Contains(noTrail, ";") {
		return errors.New("query: only a single SELECT statement is allowed")
	}

	upper := strings.ToUpper(noTrail)

	first := wordRe.FindString(upper)
	if first != "SELECT" && first != "WITH" {
		return fmt.Errorf("query: only SELECT statements are allowed; got %q", first)
	}

	if kw := denyRe.FindString(upper); kw != "" {
		return fmt.Errorf("query: write/DDL statements are not allowed (found %q)", kw)
	}

	if tbl := privateRe.FindString(noTrail); tbl != "" {
		return fmt.Errorf("query: access to private table %q is not allowed; query the public views (nodes, edges, node_props, aliases, fts, fts_search)", tbl)
	}

	// Full-text search is sanctioned via the public fts_search vtable, so MATCH is
	// permitted: SQLite enforces that MATCH targets a real fts5 table (the fts view
	// cannot forward it), and the read-only connection + private-table guard above
	// keep MATCH from reaching anything writable or private.
	return nil
}

// scrubSQL strips SQL comments and blanks the contents of string/identifier
// literals so keyword scanning never matches inside them.
func scrubSQL(s string) string {
	var b strings.Builder
	runes := []rune(s)
	n := len(runes)
	for i := 0; i < n; {
		c := runes[i]
		switch {
		case c == '-' && i+1 < n && runes[i+1] == '-':
			for i < n && runes[i] != '\n' {
				i++
			}
		case c == '/' && i+1 < n && runes[i+1] == '*':
			i += 2
			for i < n && !(runes[i] == '*' && i+1 < n && runes[i+1] == '/') {
				i++
			}
			i += 2
			b.WriteByte(' ')
		case c == '\'' || c == '"' || c == '`':
			quote := c
			b.WriteByte(' ')
			i++
			for i < n {
				if runes[i] == quote {
					if i+1 < n && runes[i+1] == quote { // doubled-quote escape
						i += 2
						continue
					}
					break
				}
				i++
			}
			i++ // skip closing quote
			b.WriteByte(' ')
		default:
			b.WriteRune(c)
			i++
		}
	}
	return b.String()
}

// ─────────────────────────────────────────────────────────────────────────────
// Row scanning + object construction
// ─────────────────────────────────────────────────────────────────────────────

// scanRows reads result rows into ordered column→value maps. []byte values are
// converted to string so text is not base64-encoded in JSON. Honours --limit by
// stopping after limit rows (limit 0 yields no rows).
func scanRows(rows *sql.Rows, cols []string, limitSet bool, limit int) ([]map[string]any, error) {
	var out []map[string]any
	for rows.Next() {
		if limitSet && len(out) >= limit {
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, fmt.Errorf("query: scan row: %w", err)
		}
		m := make(map[string]any, len(cols))
		for i, col := range cols {
			m[col] = normalizeValue(vals[i])
		}
		out = append(out, m)
	}
	// limit 0 explicitly means zero rows even though the loop above never appends.
	if limitSet && limit == 0 {
		return nil, nil
	}
	return out, nil
}

// normalizeValue converts driver []byte values to string and leaves other scalar
// types (int64, float64, nil, bool) as-is.
func normalizeValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

func hasColumn(cols []string, name string) bool {
	for _, c := range cols {
		if strings.EqualFold(c, name) {
			return true
		}
	}
	return false
}

// canonicalObjects reconstructs a full node envelope for each result row's id,
// emitting one JSON object per node with an injected top-level "id" identity.
func canonicalObjects(db *sql.DB, records []map[string]any, fields []string, cmd *cobra.Command) ([]any, error) {
	warned := newFieldWarner(cmd, fields)
	out := make([]any, 0, len(records))
	for _, rec := range records {
		id := lookupCI(rec, "id")
		idStr := fmt.Sprintf("%v", id)
		obj, err := reconstructEnvelopeObject(db, idStr)
		if err != nil {
			return nil, err
		}
		obj["id"] = mustRawJSON(idStr)
		filtered := applyFieldFilter(obj, fields, warned)
		out = append(out, filtered)
	}
	return out, nil
}

// reconstructEnvelopeObject loads a node by id from the stable views and returns
// its envelope as a JSON object map. A missing node yields a minimal object.
func reconstructEnvelopeObject(db *sql.DB, id string) (map[string]json.RawMessage, error) {
	var ulid, typ, tm, author, body, loc sql.NullString
	row := db.QueryRow("SELECT ulid, type, time, author, body, loc FROM nodes WHERE id = ?", id)
	err := row.Scan(&ulid, &typ, &tm, &author, &body, &loc)
	if errors.Is(err, sql.ErrNoRows) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query: load node %q: %w", id, err)
	}

	env := node.Envelope{
		V:      node.EnvelopeVersion,
		ULID:   ulid.String,
		Type:   typ.String,
		Time:   tm.String,
		Author: author.String,
		Body:   body.String,
		Loc:    node.Loc{File: loc.String},
	}

	props, err := loadProps(db, id)
	if err != nil {
		return nil, err
	}
	env.Props = props

	aliases, err := loadAliases(db, id)
	if err != nil {
		return nil, err
	}
	env.Aliases = aliases

	data, err := env.Marshal()
	if err != nil {
		return nil, fmt.Errorf("query: marshal node %q: %w", id, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("query: decode node %q: %w", id, err)
	}
	return m, nil
}

func loadProps(db *sql.DB, id string) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM node_props WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("query: load props for %q: %w", id, err)
	}
	defer rows.Close()
	var props map[string]string
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("query: scan prop for %q: %w", id, err)
		}
		if props == nil {
			props = make(map[string]string)
		}
		props[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query: iterate props for %q: %w", id, err)
	}
	return props, nil
}

func loadAliases(db *sql.DB, id string) ([]string, error) {
	rows, err := db.Query("SELECT alias FROM aliases WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("query: load aliases for %q: %w", id, err)
	}
	defer rows.Close()
	var aliases []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			return nil, fmt.Errorf("query: scan alias for %q: %w", id, err)
		}
		aliases = append(aliases, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query: iterate aliases for %q: %w", id, err)
	}
	return aliases, nil
}

// rawObjects converts result rows directly into JSON objects (no envelope
// reconstruction), honouring --fields.
func rawObjects(records []map[string]any, fields []string, cmd *cobra.Command) []any {
	warned := newFieldWarner(cmd, fields)
	out := make([]any, 0, len(records))
	for _, rec := range records {
		obj := make(map[string]json.RawMessage, len(rec))
		for k, v := range rec {
			obj[k] = mustRawJSON(v)
		}
		out = append(out, applyFieldFilter(obj, fields, warned))
	}
	return out
}

// applyFieldFilter restricts an object's keys to the requested fields. The
// envelope version key "v" is always retained. Unknown requested fields trigger
// a one-time warning (via warned) and are omitted.
func applyFieldFilter(obj map[string]json.RawMessage, fields []string, warned *fieldWarner) map[string]json.RawMessage {
	if len(fields) == 0 {
		return obj
	}
	keep := map[string]bool{"v": true}
	for _, f := range fields {
		keep[f] = true
	}
	out := make(map[string]json.RawMessage, len(keep))
	for k, v := range obj {
		if keep[k] {
			out[k] = v
		}
	}
	warned.check(fields, obj)
	return out
}

// fieldWarner emits a single stderr warning naming any requested fields that are
// not present in the result objects. Suppressed under --quiet.
type fieldWarner struct {
	cmd  *cobra.Command
	done bool
}

func newFieldWarner(cmd *cobra.Command, fields []string) *fieldWarner {
	return &fieldWarner{cmd: cmd, done: len(fields) == 0}
}

func (w *fieldWarner) check(fields []string, obj map[string]json.RawMessage) {
	if w.done || quietFlag {
		return
	}
	var unknown []string
	for _, f := range fields {
		if f == "v" {
			continue
		}
		if _, ok := obj[f]; !ok {
			unknown = append(unknown, f)
		}
	}
	if len(unknown) > 0 {
		fmt.Fprintf(w.cmd.ErrOrStderr(), "query: warning: unknown field(s): %s\n", strings.Join(unknown, ", "))
	}
	w.done = true
}

// emit writes the objects in the selected mode. NDJSON streams one object per
// line (stopping cleanly on a broken pipe); JSON writes a single array.
func emit(w *output.Writer, mode output.Mode, objects []any) error {
	if mode == output.JSON {
		if objects == nil {
			objects = []any{}
		}
		return w.PrintAll(objects)
	}
	for _, obj := range objects {
		if err := w.Print(obj); err != nil {
			if isBrokenPipe(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

// isBrokenPipe reports whether err is a downstream-closed-pipe condition, which
// is a normal end-of-consumer signal (e.g. `rk query ... | head`). It matches
// both io.ErrClosedPipe and the OS-level EPIPE ("broken pipe") without importing
// platform-specific syscall constants.
func isBrokenPipe(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.ErrClosedPipe) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "broken pipe")
}

// lookupCI returns the value for a case-insensitive key match.
func lookupCI(m map[string]any, key string) any {
	if v, ok := m[key]; ok {
		return v
	}
	for k, v := range m {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return nil
}

// mustRawJSON marshals v to json.RawMessage, falling back to a JSON null on the
// (practically impossible) marshal error.
func mustRawJSON(v any) json.RawMessage {
	data, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return data
}
