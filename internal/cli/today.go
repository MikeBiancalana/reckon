// `rk today` — split-actuator agenda.
//
// A live org-agenda over the node/index substrate: it surfaces the union of
// overdue + scheduled-today + today-pinned native todos (state
// open/in-progress) plus stubbed external `work-ticket` rows, and exposes
// deterministic, scriptable actuation verbs (`rk today act <ref> <key>
// [arg]`, `rk today open <ref>`) that write straight through to the task
// files and reindex.
//
// This replaces the legacy DB-journal-backed `rk today`: the
// `internal/journal` package itself is untouched (still used by
// week/schedule/task/tui), only this verb's use of it is retired.
package cli

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────────────────────────────────────
// Flag variables
// ─────────────────────────────────────────────────────────────────────────────

var todayNoLogFlag bool

// resetTodayFlags restores today flag variables to their defaults and clears
// the pflag Changed state on whichever of these flags are registered on cmd.
// Mirrors todo.go's resetTodoFlags / query.go's resetQueryFlags.
func resetTodayFlags(cmd *cobra.Command) {
	todayNoLogFlag = false
	for _, name := range []string{"no-log"} {
		if fl := cmd.Flags().Lookup(name); fl != nil {
			fl.Changed = false
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Commands
// ─────────────────────────────────────────────────────────────────────────────

var todayCmd = &cobra.Command{
	Use:          "today",
	Short:        "Show today's agenda (overdue + scheduled-today + today-pinned)",
	Long:         "Live org-agenda over the vault index: overdue, scheduled-today, and today-pinned native todos, plus stubbed external work-ticket rows.",
	Annotations:  map[string]string{"requiresDB": "false"},
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE:         runTodayE,
}

var todayActCmd = &cobra.Command{
	Use:   "act <ref> <key> [arg]",
	Short: "Actuate a today agenda item (t/pin, d/defer, D/deadline, p/priority, x/done, i/start, c/cancel)",
	Long: "Split-actuator dispatch for one agenda row: native (type todo) rows are " +
		"mutated span-locally; external (type work-ticket) rows are rejected " +
		"read-only (use `rk today open`).",
	Annotations:  map[string]string{"requiresDB": "false"},
	SilenceUsage: true,
	Args:         cobra.RangeArgs(2, 3),
	RunE:         runTodayActE,
}

var todayOpenCmd = &cobra.Command{
	Use:          "open <ref>",
	Short:        "Print the source-url of an external (work-ticket) agenda row",
	Annotations:  map[string]string{"requiresDB": "false"},
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runTodayOpenE,
}

func init() {
	todayActCmd.Flags().BoolVar(&todayNoLogFlag, "no-log", false, "Suppress the did-entry log write when completing (x/done)")
	todayCmd.AddCommand(todayActCmd, todayOpenCmd)
}

// ─────────────────────────────────────────────────────────────────────────────
// Result types (JSON shapes pinned by internal/cli/today_test.go's "Pinned
// contract" header comment; do not change field names/tags without updating
// that pin).
// ─────────────────────────────────────────────────────────────────────────────

// agendaItem is one row of `rk today`'s agenda output (native or
// external/work-ticket).
type agendaItem struct {
	ID        string `json:"id"`
	Type      string `json:"type"` // "todo" | "work-ticket"
	Path      string `json:"path,omitempty"`
	State     string `json:"state,omitempty"`
	Scheduled string `json:"scheduled,omitempty"`
	Deadline  string `json:"deadline,omitempty"`
	Pinned    string `json:"pinned,omitempty"`
	Priority  string `json:"priority,omitempty"`
	Source    string `json:"source,omitempty"`     // external only, e.g. "jira"
	SourceURL string `json:"source_url,omitempty"` // external only
	ReadOnly  bool   `json:"read_only,omitempty"`  // true for external/work-ticket rows
	Body      string `json:"body,omitempty"`
	Title     string `json:"title,omitempty"` // durable only: derived first non-empty body line
}

// agendaResult wraps `rk today`'s items so --json emits a single object
// ({"items": []} on empty), mirroring todoListResult.
type agendaResult struct {
	Items []agendaItem `json:"items"`
}

func (r agendaResult) Pretty() string {
	if len(r.Items) == 0 {
		return "today: nothing due"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "today: %d item(s)", len(r.Items))
	for _, it := range r.Items {
		marker := ""
		if it.ReadOnly {
			marker = " [read-only]"
		}
		fmt.Fprintf(&b, "\n  %s [%s]%s %s", it.ID, it.State, marker, it.Title)
	}
	return b.String()
}

// todayActResult is the structured summary of one `rk today act` run; the
// recurrence fields mirror todoDoneResult verbatim (completeDurableTodoNode
// is doneDurableTodo/doneRecurringTodo's shared body).
type todayActResult struct {
	Ref          string `json:"ref"`
	Key          string `json:"key"`
	ID           string `json:"id,omitempty"`
	Path         string `json:"path,omitempty"`
	State        string `json:"state,omitempty"`
	Scheduled    string `json:"scheduled,omitempty"`
	Deadline     string `json:"deadline,omitempty"`
	Pinned       string `json:"pinned,omitempty"`
	Priority     string `json:"priority,omitempty"`
	Skipped      bool   `json:"skipped"` // true = idempotent no-op (x on an already-done todo); mirrors todoDoneResult's shape
	Recurred     bool   `json:"recurred,omitempty"`
	Repeat       string `json:"repeat,omitempty"`
	DidEntryID   string `json:"did_entry_id,omitempty"`
	DidEntryPath string `json:"did_entry_path,omitempty"`
}

func (r todayActResult) Pretty() string {
	if r.Skipped {
		return fmt.Sprintf("today: %s already done (skipped)", r.Ref)
	}
	if r.Recurred {
		return fmt.Sprintf("today: %s advanced to %s (repeat %s)", r.Ref, r.Scheduled, r.Repeat)
	}
	return fmt.Sprintf("today: %s %s -> %s", r.Ref, r.Key, r.State)
}

// todayOpenResult is `rk today open <ref>`'s structured summary.
type todayOpenResult struct {
	Ref       string `json:"ref"`
	SourceURL string `json:"source_url"`
}

func (r todayOpenResult) Pretty() string { return r.SourceURL }

// ─────────────────────────────────────────────────────────────────────────────
// list (bare `rk today`)
// ─────────────────────────────────────────────────────────────────────────────

func runTodayE(cmd *cobra.Command, args []string) error {
	defer resetTodayFlags(cmd)

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("today: load config: %w", err)
	}

	ix, err := index.Open(cfg)
	if err != nil {
		return fmt.Errorf("today: open index: %w", err)
	}
	defer ix.Close()

	if _, err := ix.Reconcile(); err != nil {
		return fmt.Errorf("today: reconcile index: %w", err)
	}

	todayStr := todoNow().Format("2006-01-02")
	items, warnings, err := buildAgenda(ix.DB(), todayStr)
	if err != nil {
		return err
	}

	if !quietFlag {
		for _, w := range warnings {
			fmt.Fprintln(cmd.ErrOrStderr(), w)
		}
	}

	res := agendaResult{Items: items}
	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return err
		}
	}
	return nil
}

// buildAgenda loads every candidate native/external row (type IN
// ('todo','work-ticket')) and applies the agenda predicate:
// (scheduled <= today) OR (deadline <= today) OR (pinned == today), ANDed
// with state IN (open, in-progress). A malformed date on any one field is
// skipped (with a returned warning) rather than aborting the row or the
// load: the row still surfaces if a sibling field matches.
func buildAgenda(db *sql.DB, todayStr string) ([]agendaItem, []string, error) {
	todayDate, err := parseSchedDate(todayStr)
	if err != nil {
		return nil, nil, fmt.Errorf("today: internal: parse today %q: %w", todayStr, err)
	}

	rows, err := db.Query("SELECT id, type, body, loc, title FROM nodes WHERE type IN ('todo','work-ticket')")
	if err != nil {
		return nil, nil, fmt.Errorf("today: query candidate nodes: %w", err)
	}
	type candidate struct{ id, typ, body, loc, title string }
	var candidates []candidate
	for rows.Next() {
		var c candidate
		if err := rows.Scan(&c.id, &c.typ, &c.body, &c.loc, &c.title); err != nil {
			rows.Close()
			return nil, nil, fmt.Errorf("today: scan candidate node: %w", err)
		}
		candidates = append(candidates, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, nil, fmt.Errorf("today: iterate candidate nodes: %w", err)
	}
	rows.Close()

	items := []agendaItem{}
	var warnings []string
	for _, c := range candidates {
		props, err := loadTodoProps(db, c.id)
		if err != nil {
			return nil, nil, err
		}
		state := props["state"]
		// The state ∈ {open,in-progress} filter is ANDed in for NATIVE rows
		// only; external (work-ticket) rows surface purely on the date
		// predicate below, regardless of whatever foreign state vocabulary a
		// future feeder emits.
		if c.typ == "todo" && state != "open" && state != "in-progress" {
			continue
		}

		matched := false
		if v := props["scheduled"]; v != "" {
			if d, perr := parseSchedDate(v); perr != nil {
				warnings = append(warnings, fmt.Sprintf("today: %s: skipping malformed scheduled date %q: %v", c.id, v, perr))
			} else if !d.After(todayDate) {
				matched = true
			}
		}
		if v := props["deadline"]; v != "" {
			if d, perr := parseSchedDate(v); perr != nil {
				warnings = append(warnings, fmt.Sprintf("today: %s: skipping malformed deadline date %q: %v", c.id, v, perr))
			} else if !d.After(todayDate) {
				matched = true
			}
		}
		if v := props["pinned"]; v != "" {
			if d, perr := parseSchedDate(v); perr != nil {
				warnings = append(warnings, fmt.Sprintf("today: %s: skipping malformed pinned date %q: %v", c.id, v, perr))
			} else if d.Equal(todayDate) {
				matched = true
			}
		}
		if !matched {
			continue
		}

		items = append(items, agendaItem{
			ID:        c.id,
			Type:      c.typ,
			Path:      c.loc,
			State:     state,
			Scheduled: props["scheduled"],
			Deadline:  props["deadline"],
			Pinned:    props["pinned"],
			Priority:  props["priority"],
			Source:    props["source"],
			SourceURL: props["source-url"],
			ReadOnly:  c.typ == "work-ticket",
			Body:      strings.TrimSpace(c.body),
			Title:     c.title,
		})
	}
	return items, warnings, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// act (split actuator)
// ─────────────────────────────────────────────────────────────────────────────

func runTodayActE(cmd *cobra.Command, args []string) error {
	defer resetTodayFlags(cmd)

	ref := args[0]
	key := args[1]
	var arg string
	if len(args) > 2 {
		arg = args[2]
	}
	noLog := todayNoLogFlag

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("today act: load config: %w", err)
	}

	ix, err := index.Open(cfg)
	if err != nil {
		return fmt.Errorf("today act: open index: %w", err)
	}
	defer ix.Close()

	if _, err := ix.Reconcile(); err != nil {
		return fmt.Errorf("today act: reconcile index: %w", err)
	}

	// Classify the ref via the index (cheap, covers external rows that may
	// live anywhere in the vault) BEFORE any file mutation is attempted: the
	// split-actuator guard lives here, in one place.
	_, typ, _, found, err := lookupNodeByRefOrAlias(ix.DB(), ref)
	if err != nil {
		return err
	}
	if found && typ == "work-ticket" {
		return fmt.Errorf("today act: %q is read-only (external work ticket); use `rk today open` instead", ref)
	}

	res, err := dispatchTodayAct(cfg.VaultDir, ref, key, arg, noLog)
	if err != nil {
		return err
	}

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return err
		}
	}

	// Eager write-through reconcile: a subsequent read-only `rk query`
	// (which never reconciles itself) should see the mutation. The file
	// write is authoritative and the index is non-fatal/self-healing, so
	// this step must never roll back or fail the already-successful action
	// above: the file write has already happened and `res` has already been
	// printed. A hard failure here would also create a double-advance-on-
	// retry hazard for a recurring completion (the cursor advance is not
	// idempotent, unlike the plain state flip), so a failure is surfaced as
	// a warning only.
	if _, err := ix.Reconcile(); err != nil && !quietFlag {
		fmt.Fprintf(cmd.ErrOrStderr(), "today act: warning: reconcile index after write: %v\n", err)
	}
	return nil
}

// normalizeActKey maps a single-letter key or its readable alias to the
// canonical single-letter form.
func normalizeActKey(key string) (string, error) {
	switch key {
	case "t", "pin":
		return "t", nil
	case "d", "defer":
		return "d", nil
	case "D", "deadline":
		return "D", nil
	case "p", "priority":
		return "p", nil
	case "x", "done":
		return "x", nil
	case "i", "start":
		return "i", nil
	case "c", "cancel":
		return "c", nil
	default:
		return "", fmt.Errorf("today act: unknown key %q (want t|pin, d|defer, D|deadline, p|priority, x|done, i|start, c|cancel)", key)
	}
}

// dispatchTodayAct resolves ref to a native durable todo file (via the same
// todos/ walk rk todo done uses) and applies the actuation named by key.
func dispatchTodayAct(vaultDir, ref, key, arg string, noLog bool) (todayActResult, error) {
	normKey, err := normalizeActKey(key)
	if err != nil {
		return todayActResult{}, err
	}

	n, foundPath, err := loadNativeTodoForEdit(vaultDir, ref)
	if err != nil {
		return todayActResult{}, err
	}

	switch normKey {
	case "t":
		return actPin(vaultDir, n, foundPath, ref)
	case "d":
		return actDefer(vaultDir, n, foundPath, ref, arg)
	case "D":
		return actDeadline(vaultDir, n, foundPath, ref, arg)
	case "p":
		return actPriority(vaultDir, n, foundPath, ref, arg)
	case "x":
		return actDone(vaultDir, n, foundPath, ref, !noLog)
	case "i":
		return actStart(vaultDir, n, foundPath, ref)
	case "c":
		return actCancel(vaultDir, n, foundPath, ref)
	}
	return todayActResult{}, fmt.Errorf("today act: unknown key %q", key)
}

// loadNativeTodoForEdit resolves ref to a durable todo file (ULID fast-path,
// else a walk over todos/*.md matching ULID or alias) -- identical
// resolution to doneDurableTodo (todo.go), reused verbatim so `rk today act`
// and `rk todo done` never disagree on which file a ref names.
func loadNativeTodoForEdit(vaultDir, ref string) (*node.Node, string, error) {
	todosDir := filepath.Join(vaultDir, "todos")

	fastPath := filepath.Join(todosDir, ref+".md")
	n, foundPath, err := loadDurableTodoAt(fastPath)
	if err != nil {
		return nil, "", err
	}
	if n != nil && n.Type != "todo" {
		n, foundPath = nil, ""
	}
	if n == nil {
		n, foundPath, err = findDurableTodoByRefOrAlias(todosDir, ref)
		if err != nil {
			return nil, "", err
		}
	}
	if n == nil {
		return nil, "", fmt.Errorf("today act: no todo found matching %q (not found)", ref)
	}
	return n, foundPath, nil
}

// setOrInsertField applies the HasField trichotomy (SetField if the scalar
// already exists, else InsertField) so every actuation key works whether or
// not the target frontmatter key was already present.
func setOrInsertField(n *node.Node, key, value string) error {
	if n.HasField(key) {
		return n.SetField(key, value)
	}
	return n.InsertField(key, value)
}

// actPin implements the "t"/"pin" key: pinned <- today's date.
func actPin(vaultDir string, n *node.Node, foundPath, ref string) (todayActResult, error) {
	today := todoNow().Format("2006-01-02")
	if err := setOrInsertField(n, "pinned", today); err != nil {
		return todayActResult{}, fmt.Errorf("today act: set pinned: %w", err)
	}
	if err := writeFileAtomic(foundPath, n.Serialize()); err != nil {
		return todayActResult{}, fmt.Errorf("today act: write: %w", err)
	}
	return todayActResult{
		Ref: ref, Key: "t", ID: n.ULID, Path: relTodoPath(vaultDir, foundPath),
		State: n.Props["state"], Pinned: today,
	}, nil
}

// resolveDeferDate resolves a "d"/"defer" argument: the "tomorrow"/
// "next-week" shorthands, or a literal YYYY-MM-DD date.
func resolveDeferDate(arg string, now time.Time) (string, error) {
	switch arg {
	case "tomorrow":
		return now.AddDate(0, 0, 1).Format("2006-01-02"), nil
	case "next-week":
		return now.AddDate(0, 0, 7).Format("2006-01-02"), nil
	default:
		d, err := parseSchedDate(arg)
		if err != nil {
			return "", fmt.Errorf("today act: defer: %w", err)
		}
		return d.Format("2006-01-02"), nil
	}
}

// actDefer implements the "d"/"defer" key: scheduled <- resolved date.
func actDefer(vaultDir string, n *node.Node, foundPath, ref, arg string) (todayActResult, error) {
	if strings.TrimSpace(arg) == "" {
		return todayActResult{}, fmt.Errorf("today act: d/defer requires an argument (tomorrow|next-week|YYYY-MM-DD)")
	}
	date, err := resolveDeferDate(arg, todoNow())
	if err != nil {
		return todayActResult{}, err
	}
	if err := setOrInsertField(n, "scheduled", date); err != nil {
		return todayActResult{}, fmt.Errorf("today act: set scheduled: %w", err)
	}
	if err := writeFileAtomic(foundPath, n.Serialize()); err != nil {
		return todayActResult{}, fmt.Errorf("today act: write: %w", err)
	}
	return todayActResult{
		Ref: ref, Key: "d", ID: n.ULID, Path: relTodoPath(vaultDir, foundPath),
		State: n.Props["state"], Scheduled: date,
	}, nil
}

// actDeadline implements the "D"/"deadline" key: deadline <- literal date.
func actDeadline(vaultDir string, n *node.Node, foundPath, ref, arg string) (todayActResult, error) {
	if strings.TrimSpace(arg) == "" {
		return todayActResult{}, fmt.Errorf("today act: D/deadline requires a YYYY-MM-DD argument")
	}
	d, err := parseSchedDate(arg)
	if err != nil {
		return todayActResult{}, fmt.Errorf("today act: deadline: %w", err)
	}
	date := d.Format("2006-01-02")
	if err := setOrInsertField(n, "deadline", date); err != nil {
		return todayActResult{}, fmt.Errorf("today act: set deadline: %w", err)
	}
	if err := writeFileAtomic(foundPath, n.Serialize()); err != nil {
		return todayActResult{}, fmt.Errorf("today act: write: %w", err)
	}
	return todayActResult{
		Ref: ref, Key: "D", ID: n.ULID, Path: relTodoPath(vaultDir, foundPath),
		State: n.Props["state"], Deadline: date,
	}, nil
}

// actPriority implements the "p"/"priority" key: a validated A/B/C letter.
// Validation happens BEFORE any field mutation or write, so a rejected
// value leaves the file byte-identical.
func actPriority(vaultDir string, n *node.Node, foundPath, ref, arg string) (todayActResult, error) {
	if arg != "A" && arg != "B" && arg != "C" {
		return todayActResult{}, fmt.Errorf("today act: priority: invalid value %q (want A, B, or C)", arg)
	}
	if err := setOrInsertField(n, "priority", arg); err != nil {
		return todayActResult{}, fmt.Errorf("today act: set priority: %w", err)
	}
	if err := writeFileAtomic(foundPath, n.Serialize()); err != nil {
		return todayActResult{}, fmt.Errorf("today act: write: %w", err)
	}
	return todayActResult{
		Ref: ref, Key: "p", ID: n.ULID, Path: relTodoPath(vaultDir, foundPath),
		State: n.Props["state"], Priority: arg,
	}, nil
}

// actDone implements the "x"/"done" key: delegates to
// completeDurableTodoNode (todo.go), the body shared with `rk todo done`,
// with logDid = !--no-log (default on). The same logDid value is passed for
// both the plain-completion branch AND the recurrence branch (recurLogDid),
// so `--no-log` suppresses the did entry uniformly whether the target row
// is a plain todo or a recurring one — unlike `rk todo done`, whose
// recurrence branch always logs.
func actDone(vaultDir string, n *node.Node, foundPath, ref string, logDid bool) (todayActResult, error) {
	res, err := completeDurableTodoNode(vaultDir, n, foundPath, ref, logDid, logDid)
	if err != nil {
		return todayActResult{}, err
	}
	return todayActResult{
		Ref: ref, Key: "x", ID: res.ID, Path: res.Path, State: res.State,
		Scheduled: res.Scheduled, Skipped: res.Skipped, Recurred: res.Recurred, Repeat: res.Repeat,
		DidEntryID: res.DidEntryID, DidEntryPath: res.DidEntryPath,
	}, nil
}

// actStart implements the "i"/"start" key (no did-entry: only x logs).
func actStart(vaultDir string, n *node.Node, foundPath, ref string) (todayActResult, error) {
	if err := setOrInsertField(n, "state", "in-progress"); err != nil {
		return todayActResult{}, fmt.Errorf("today act: set state: %w", err)
	}
	if err := writeFileAtomic(foundPath, n.Serialize()); err != nil {
		return todayActResult{}, fmt.Errorf("today act: write: %w", err)
	}
	return todayActResult{
		Ref: ref, Key: "i", ID: n.ULID, Path: relTodoPath(vaultDir, foundPath),
		State: "in-progress",
	}, nil
}

// actCancel implements the "c"/"cancel" key (no did-entry: only x logs).
func actCancel(vaultDir string, n *node.Node, foundPath, ref string) (todayActResult, error) {
	if err := setOrInsertField(n, "state", "cancelled"); err != nil {
		return todayActResult{}, fmt.Errorf("today act: set state: %w", err)
	}
	if err := writeFileAtomic(foundPath, n.Serialize()); err != nil {
		return todayActResult{}, fmt.Errorf("today act: write: %w", err)
	}
	return todayActResult{
		Ref: ref, Key: "c", ID: n.ULID, Path: relTodoPath(vaultDir, foundPath),
		State: "cancelled",
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// open (external jump)
// ─────────────────────────────────────────────────────────────────────────────

func runTodayOpenE(cmd *cobra.Command, args []string) error {
	defer resetTodayFlags(cmd)
	ref := args[0]

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("today open: load config: %w", err)
	}

	ix, err := index.Open(cfg)
	if err != nil {
		return fmt.Errorf("today open: open index: %w", err)
	}
	defer ix.Close()

	if _, err := ix.Reconcile(); err != nil {
		return fmt.Errorf("today open: reconcile index: %w", err)
	}

	id, typ, _, found, err := lookupNodeByRefOrAlias(ix.DB(), ref)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("today open: no item found matching %q (not found)", ref)
	}
	if typ != "work-ticket" {
		return fmt.Errorf("today open: %q is a native row, not external (open is for external work-ticket rows only)", ref)
	}

	url, err := loadSourceURL(ix.DB(), id)
	if err != nil {
		return err
	}
	if url == "" {
		return fmt.Errorf("today open: %q has no source-url set", ref)
	}

	res := todayOpenResult{Ref: ref, SourceURL: url}
	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return err
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Index-backed ref resolution (external rows may live anywhere in the vault,
// so -- unlike native resolution, which walks todos/*.md directly -- these
// go through the index's nodes/aliases views; read-only, classification
// only, never used to obtain a *node.Node for editing).
// ─────────────────────────────────────────────────────────────────────────────

// lookupNodeByRefOrAlias resolves ref (a ULID or an alias) to its node id,
// type, and vault-relative loc via the index. found is false (zero values,
// nil error) when ref matches nothing.
func lookupNodeByRefOrAlias(db *sql.DB, ref string) (id, typ, loc string, found bool, err error) {
	row := db.QueryRow("SELECT id, type, loc FROM nodes WHERE id = ?", ref)
	if err = row.Scan(&id, &typ, &loc); err == nil {
		return id, typ, loc, true, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", "", "", false, fmt.Errorf("today: lookup %q: %w", ref, err)
	}

	row = db.QueryRow("SELECT n.id, n.type, n.loc FROM nodes n JOIN aliases a ON a.id = n.id WHERE a.alias = ?", ref)
	if err = row.Scan(&id, &typ, &loc); err == nil {
		return id, typ, loc, true, nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", "", false, nil
	}
	return "", "", "", false, fmt.Errorf("today: lookup alias %q: %w", ref, err)
}

// loadSourceURL reads the source-url scalar prop for id. Returns "" (no
// error) if the prop is absent.
func loadSourceURL(db *sql.DB, id string) (string, error) {
	var url string
	err := db.QueryRow("SELECT value FROM node_props WHERE id = ? AND key = 'source-url'", id).Scan(&url)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("today open: load source-url for %q: %w", id, err)
	}
	return url, nil
}
