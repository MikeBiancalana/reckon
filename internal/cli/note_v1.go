package cli

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/spf13/cobra"
)

// v1-T8 (reckon-ih5g): rk note create/show/rename/index, attached to the
// existing legacy noteCmd (note.go, "new"/"list") as new subcommands. See
// ticket-work/reckon-ih5g/plan.md for the full design. The legacy `rk notes`
// (plural) DB-backed surface is untouched pending T9.

// ─────────────────────────────────────────────────────────────────────────────
// Flag variables (package-global so cobra can bind them; each subcommand's
// RunE resets them, and their pflag Changed state, via defer resetNoteFlags —
// mirrors todo.go's resetTodoFlags).
// ─────────────────────────────────────────────────────────────────────────────

var (
	noteDescriptionFlag string
	noteStageFlag       string
	noteTagFlag         []string
	noteAliasFlag       []string
	noteSlugFlag        string
	noteDirFlag         string
	noteBodyFlag        string
	noteTypeFlag        string
	noteAuthorFlag      string
)

// resetNoteFlags restores note flag variables to their defaults and clears
// the pflag Changed state on whichever of these flags are registered on cmd
// (create/show/rename/index each register a different subset).
func resetNoteFlags(cmd *cobra.Command) {
	noteDescriptionFlag = ""
	noteStageFlag = ""
	noteTagFlag = nil
	noteAliasFlag = nil
	noteSlugFlag = ""
	noteDirFlag = ""
	noteBodyFlag = ""
	noteTypeFlag = ""
	noteAuthorFlag = ""
	for _, name := range []string{"description", "stage", "tag", "alias", "slug", "dir", "body", "type", "author"} {
		if fl := cmd.Flags().Lookup(name); fl != nil {
			fl.Changed = false
		}
	}
}

// validStages is the CLI-layer enum for --stage (EC-5); the node package
// itself stays permissive (stage is a plain, non-reserved Prop).
var validStages = map[string]bool{"seedling": true, "budding": true, "evergreen": true}

func validateStage(stage string) error {
	if !validStages[stage] {
		return fmt.Errorf("invalid stage %q (want one of seedling, budding, evergreen)", stage)
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Commands
// ─────────────────────────────────────────────────────────────────────────────

var noteCreateCmd = &cobra.Command{
	Use:          "create <title>",
	Short:        "Create a new note (notes/<slug>.md)",
	SilenceUsage: true,
	Args:         cobra.MinimumNArgs(1),
	RunE:         runNoteCreateE,
}

var noteShowCmd = &cobra.Command{
	Use:          "show <ref>",
	Short:        "Show a note's fields, forward links, and backlinks",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runNoteShowE,
}

var noteRenameCmd = &cobra.Command{
	Use:   "rename <ref> <new-title>",
	Short: "Rename a note, retaining its old slug as an alias redirect",
	Long: `Rename a note, retaining its old slug as an alias redirect.

The note's aliases are rewritten to one canonical flow line (aliases: [a, b]).
Known limitation: an alias containing a literal comma (e.g. an Obsidian
block-list item "- Doe, Jane") is parsed by reckon as two aliases and is
persisted that way on rename (see internal/node/AGENTS.md).`,
	SilenceUsage: true,
	Args:         cobra.ExactArgs(2),
	RunE:         runNoteRenameE,
}

var noteIndexCmd = &cobra.Command{
	Use:   "index",
	Short: "(Re)generate notes/**/index.md catalog files",
	Long: `(Re)generate notes/**/index.md catalog files.

Generated files are tool-owned: they start with a marker comment, are
overwritten on every run, and are removed when their directory no longer
contains notes. A hand-authored index.md (no marker) is never touched --
it is skipped with a warning.`,
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE:         runNoteIndexE,
}

func init() {
	cf := noteCreateCmd.Flags()
	cf.StringVar(&noteDescriptionFlag, "description", "", "One-line description (mandatory-by-convention, optional)")
	cf.StringVar(&noteStageFlag, "stage", "", "Maturity stage: seedling|budding|evergreen")
	cf.StringArrayVar(&noteTagFlag, "tag", nil, "Tag (repeatable)")
	cf.StringArrayVar(&noteAliasFlag, "alias", nil, "Extra alias, beyond the self-minted slug (repeatable)")
	cf.StringVar(&noteSlugFlag, "slug", "", "Override the self-minted slug (escape hatch for a colliding title)")
	cf.StringVar(&noteDirFlag, "dir", "", "Subdirectory under notes/ to place the note in")
	cf.StringVar(&noteBodyFlag, "body", "", "Body text (may contain [[wikilinks]])")
	cf.StringVar(&noteTypeFlag, "type", "", "Node type (default: note)")
	cf.StringVar(&noteAuthorFlag, "author", "", "Author to record (default: $RECKON_AUTHOR, $USER, or \"local\")")

	noteCmd.AddCommand(noteCreateCmd, noteShowCmd, noteRenameCmd, noteIndexCmd)
}

// ─────────────────────────────────────────────────────────────────────────────
// Result types
// ─────────────────────────────────────────────────────────────────────────────

// noteCreateResult is the structured summary of one `rk note create` run.
type noteCreateResult struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

func (r noteCreateResult) Pretty() string {
	return fmt.Sprintf("note: created %s (id %s)", r.Path, r.ID)
}

// noteForwardLink is one outgoing edge in a noteShowResult.
type noteForwardLink struct {
	Rel    string `json:"rel"`
	Dst    string `json:"dst"`
	DstKey string `json:"dst_key,omitempty"`
}

// noteBacklink is one incoming edge in a noteShowResult (index-derived only,
// never stored on the target note's own file).
type noteBacklink struct {
	Src string `json:"src"`
	Rel string `json:"rel"`
}

// noteShowResult is the structured summary of one `rk note show` run.
type noteShowResult struct {
	ID           string            `json:"id"`
	Type         string            `json:"type"`
	Title        string            `json:"title,omitempty"`
	Description  string            `json:"description,omitempty"`
	Stage        string            `json:"stage,omitempty"`
	Aliases      []string          `json:"aliases,omitempty"`
	Path         string            `json:"path"`
	ForwardLinks []noteForwardLink `json:"forward_links"`
	Backlinks    []noteBacklink    `json:"backlinks"`
}

func (r noteShowResult) Pretty() string {
	var b strings.Builder
	fmt.Fprintf(&b, "note: %s (%s)\n  path: %s", r.ID, r.Type, r.Path)
	if r.Title != "" {
		fmt.Fprintf(&b, "\n  title: %s", r.Title)
	}
	fmt.Fprintf(&b, "\n  forward_links: %d, backlinks: %d", len(r.ForwardLinks), len(r.Backlinks))
	return b.String()
}

// noteRenameResult is the structured summary of one `rk note rename` run.
type noteRenameResult struct {
	ID      string `json:"id"`
	OldPath string `json:"old_path"`
	Path    string `json:"path"`
	Title   string `json:"title"`
}

func (r noteRenameResult) Pretty() string {
	return fmt.Sprintf("note: renamed %s -> %s (id %s)", r.OldPath, r.Path, r.ID)
}

// noteIndexResult is the structured summary of one `rk note index` run.
type noteIndexResult struct {
	Files   []string `json:"files"`
	Skipped []string `json:"skipped,omitempty"`
	Removed []string `json:"removed,omitempty"`
}

func (r noteIndexResult) Pretty() string {
	s := fmt.Sprintf("note: regenerated %d index file(s)", len(r.Files))
	if len(r.Skipped) > 0 {
		s += fmt.Sprintf(", skipped %d hand-authored", len(r.Skipped))
	}
	if len(r.Removed) > 0 {
		s += fmt.Sprintf(", removed %d stale", len(r.Removed))
	}
	return s
}

// noteIndexMarker is the first line of every generated index.md. Its presence
// is the ownership contract: rk note index only overwrites or prunes files
// that carry it, and never touches a hand-authored index.md without it.
const noteIndexMarker = "<!-- generated by rk note index; do not edit -->"

// ─────────────────────────────────────────────────────────────────────────────
// create
// ─────────────────────────────────────────────────────────────────────────────

func runNoteCreateE(cmd *cobra.Command, args []string) error {
	defer resetNoteFlags(cmd)

	title := strings.TrimSpace(strings.Join(args, " "))

	stage := strings.TrimSpace(noteStageFlag)
	if stage != "" {
		if err := validateStage(stage); err != nil {
			return fmt.Errorf("note create: %w", err)
		}
	}

	slugSrc := strings.TrimSpace(noteSlugFlag)
	if slugSrc == "" {
		slugSrc = title
	}
	slug := slugify(slugSrc)
	if err := validateSlug(slug); err != nil {
		return fmt.Errorf("note create: %w", err)
	}

	typ := strings.TrimSpace(noteTypeFlag)
	if typ == "" {
		typ = "note"
	}
	author := resolveAuthor(noteAuthorFlag)
	description := strings.TrimSpace(noteDescriptionFlag)
	dir := strings.TrimSpace(noteDirFlag)
	body := noteBodyFlag
	if body != "" && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return fmt.Errorf("note create: %w", err)
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("note create: load config: %w", err)
	}

	notesDir := filepath.Join(cfg.VaultDir, "notes")
	targetDir := notesDir
	relDir := "notes"
	if dir != "" {
		if filepath.IsAbs(dir) {
			return fmt.Errorf("note create: --dir must be a relative subdirectory under notes/, got %q", dir)
		}
		targetDir = filepath.Join(notesDir, dir)
		rel, relErr := filepath.Rel(notesDir, targetDir)
		if relErr != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("note create: --dir must stay under notes/, got %q", dir)
		}
		relDir = filepath.ToSlash(filepath.Join("notes", dir))
	}
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("note create: create notes dir: %w", err)
	}

	path := filepath.Join(targetDir, slug+".md")
	if _, statErr := os.Stat(path); statErr == nil {
		return fmt.Errorf("note create: refusing to overwrite existing file at %s (duplicate)", path)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("note create: stat %s: %w", path, statErr)
	}

	collide, err := slugCollision(notesDir, slug, "")
	if err != nil {
		return fmt.Errorf("note create: scan notes dir: %w", err)
	}
	if collide {
		return fmt.Errorf("note create: slug %q already claimed by an existing note (duplicate); use --slug to override", slug)
	}

	aliases := []string{slug}
	seen := map[string]bool{slug: true}
	for _, a := range noteAliasFlag {
		a = strings.TrimSpace(a)
		if a != "" && !seen[a] {
			seen[a] = true
			aliases = append(aliases, a)
		}
	}

	n := node.NewNode(typ, author, body)
	n.Time = time.Now().UTC().Format(time.RFC3339)
	n.Aliases = aliases

	props := map[string]string{"title": title}
	if description != "" {
		props["description"] = description
	}
	if stage != "" {
		props["stage"] = stage
	}
	if len(noteTagFlag) > 0 {
		props["tags"] = "[" + strings.Join(noteTagFlag, ", ") + "]"
	}
	n.Props = props

	rendered := n.Render()
	parsed, err := node.Parse(rendered)
	if err != nil {
		return fmt.Errorf("note create: parse rendered node: %w", err)
	}

	if err := writeFileAtomic(path, parsed.Serialize()); err != nil {
		return fmt.Errorf("note create: write: %w", err)
	}

	res := noteCreateResult{
		ID:    parsed.ULID,
		Path:  relDir + "/" + slug + ".md",
		Slug:  slug,
		Title: title,
	}

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return fmt.Errorf("print result: %w", err)
		}
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────────────────
// show
// ─────────────────────────────────────────────────────────────────────────────

func runNoteShowE(cmd *cobra.Command, args []string) error {
	defer resetNoteFlags(cmd)
	ref := args[0]

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return fmt.Errorf("note show: %w", err)
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("note show: load config: %w", err)
	}

	ix, err := index.Open(cfg)
	if err != nil {
		return fmt.Errorf("note show: open index: %w", err)
	}
	defer ix.Close()

	if _, err := ix.Reconcile(); err != nil {
		return fmt.Errorf("note show: reconcile index: %w", err)
	}

	db := ix.DB()
	var id, typ, loc string
	row := db.QueryRow(
		`SELECT id, type, loc FROM nodes
		 WHERE (ulid = ? OR EXISTS (SELECT 1 FROM aliases a WHERE a.alias = ? AND a.id = nodes.id))
		   AND loc LIKE 'notes/%'
		 LIMIT 1`, ref, ref)
	if err := row.Scan(&id, &typ, &loc); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("note show: no note found matching %q (not found)", ref)
		}
		return fmt.Errorf("note show: query node: %w", err)
	}

	props, err := loadProps(db, id)
	if err != nil {
		return fmt.Errorf("note show: %w", err)
	}
	aliases, err := loadAliases(db, id)
	if err != nil {
		return fmt.Errorf("note show: %w", err)
	}
	// loadAliases (shared with query.go's envelope reconstruction, where
	// storage order is left alone) has no ORDER BY; sort here so show's JSON
	// is deterministic and diffable.
	sort.Strings(aliases)
	forwardLinks, err := loadNoteForwardLinks(db, id)
	if err != nil {
		return fmt.Errorf("note show: %w", err)
	}
	backlinks, err := loadNoteBacklinks(db, id)
	if err != nil {
		return fmt.Errorf("note show: %w", err)
	}

	res := noteShowResult{
		ID:           id,
		Type:         typ,
		Title:        props["title"],
		Description:  props["description"],
		Stage:        props["stage"],
		Aliases:      aliases,
		Path:         filepath.ToSlash(loc),
		ForwardLinks: forwardLinks,
		Backlinks:    backlinks,
	}

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return fmt.Errorf("print result: %w", err)
		}
	}
	return nil
}

func loadNoteForwardLinks(db *sql.DB, id string) ([]noteForwardLink, error) {
	rows, err := db.Query("SELECT rel, dst, dst_key FROM edges WHERE src = ?", id)
	if err != nil {
		return nil, fmt.Errorf("note show: load forward links for %q: %w", id, err)
	}
	defer rows.Close()
	links := []noteForwardLink{}
	for rows.Next() {
		var rel, dst string
		var dstKey sql.NullString
		if err := rows.Scan(&rel, &dst, &dstKey); err != nil {
			return nil, fmt.Errorf("note show: scan forward link for %q: %w", id, err)
		}
		links = append(links, noteForwardLink{Rel: rel, Dst: dst, DstKey: dstKey.String})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("note show: iterate forward links for %q: %w", id, err)
	}
	return links, nil
}

func loadNoteBacklinks(db *sql.DB, id string) ([]noteBacklink, error) {
	rows, err := db.Query("SELECT src, rel FROM edges WHERE dst_key = ?", id)
	if err != nil {
		return nil, fmt.Errorf("note show: load backlinks for %q: %w", id, err)
	}
	defer rows.Close()
	links := []noteBacklink{}
	for rows.Next() {
		var src, rel string
		if err := rows.Scan(&src, &rel); err != nil {
			return nil, fmt.Errorf("note show: scan backlink for %q: %w", id, err)
		}
		links = append(links, noteBacklink{Src: src, Rel: rel})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("note show: iterate backlinks for %q: %w", id, err)
	}
	return links, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// rename
// ─────────────────────────────────────────────────────────────────────────────

func runNoteRenameE(cmd *cobra.Command, args []string) error {
	defer resetNoteFlags(cmd)
	ref := args[0]
	newTitle := strings.TrimSpace(args[1])

	newSlug := slugify(newTitle)
	if err := validateSlug(newSlug); err != nil {
		return fmt.Errorf("note rename: %w", err)
	}

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return fmt.Errorf("note rename: %w", err)
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("note rename: load config: %w", err)
	}

	notesDir := filepath.Join(cfg.VaultDir, "notes")
	n, path, err := findNoteByRefOrAlias(notesDir, ref)
	if err != nil {
		return fmt.Errorf("note rename: scan notes dir: %w", err)
	}
	if n == nil {
		return fmt.Errorf("note rename: no note found matching %q (not found)", ref)
	}

	oldSlug := strings.TrimSuffix(filepath.Base(path), ".md")
	dir := filepath.Dir(path)

	if newSlug != oldSlug {
		collide, err := slugCollision(notesDir, newSlug, path)
		if err != nil {
			return fmt.Errorf("note rename: scan notes dir: %w", err)
		}
		if collide {
			return fmt.Errorf("note rename: slug %q already claimed by another note (collision); no changes made", newSlug)
		}
	}

	if n.HasField("title") {
		if err := n.SetField("title", newTitle); err != nil {
			return fmt.Errorf("note rename: set title: %w", err)
		}
	} else {
		if err := n.InsertField("title", newTitle); err != nil {
			return fmt.Errorf("note rename: insert title: %w", err)
		}
	}

	if newSlug != oldSlug {
		merged := make([]string, 0, len(n.Aliases)+2)
		seen := map[string]bool{}
		add := func(a string) {
			if a == "" || seen[a] {
				return
			}
			seen[a] = true
			merged = append(merged, a)
		}
		for _, a := range n.Aliases {
			add(a)
		}
		add(oldSlug)
		add(newSlug)
		if err := n.SetAliases(merged); err != nil {
			return fmt.Errorf("note rename: set aliases: %w", err)
		}
	}

	newPath := filepath.Join(dir, newSlug+".md")
	if err := writeFileAtomic(newPath, n.Serialize()); err != nil {
		return fmt.Errorf("note rename: write: %w", err)
	}
	if newPath != path {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("note rename: remove old file %s: %w", path, err)
		}
	}

	res := noteRenameResult{
		ID:      n.ULID,
		OldPath: relTodoPath(cfg.VaultDir, path),
		Path:    relTodoPath(cfg.VaultDir, newPath),
		Title:   newTitle,
	}

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return fmt.Errorf("print result: %w", err)
		}
	}
	return nil
}

// findNoteByRefOrAlias walks notesDir recursively (skipping generated
// index.md files) looking for a note whose ULID, filename slug, or alias
// matches ref. Unparsable/CRLF files are skipped rather than aborting the
// whole search (mirrors todo.go's findDurableTodoByRefOrAlias).
func findNoteByRefOrAlias(notesDir, ref string) (*node.Node, string, error) {
	files, err := noteFiles(notesDir)
	if err != nil {
		return nil, "", err
	}
	for _, path := range files {
		raw, err := os.ReadFile(path)
		if err != nil || bytes.Contains(raw, []byte("\r\n")) {
			continue
		}
		n, err := node.Parse(raw)
		if err != nil {
			continue
		}
		base := strings.TrimSuffix(filepath.Base(path), ".md")
		if n.ULID == ref || base == ref || containsString(n.Aliases, ref) {
			return n, path, nil
		}
	}
	return nil, "", nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Shared notes/ filesystem helpers (create + rename collision checks)
// ─────────────────────────────────────────────────────────────────────────────

// noteFiles returns every *.md file under notesDir (recursively), excluding
// files named "index.md" (rk note index's own generated output, never a note
// candidate). A missing notesDir is reported as an empty slice, not an error.
func noteFiles(notesDir string) ([]string, error) {
	if _, err := os.Stat(notesDir); os.IsNotExist(err) {
		return nil, nil
	}
	var out []string
	err := filepath.Walk(notesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".md") {
			return nil
		}
		if filepath.Base(path) == "index.md" {
			return nil
		}
		out = append(out, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", notesDir, err)
	}
	return out, nil
}

// slugCollision reports whether candidateSlug is already claimed by any
// existing note under notesDir (by filename or by a parsed alias), other
// than excludePath (used by rename to allow a note to keep its own current
// filename/alias without self-colliding). Unparsable/CRLF files are skipped,
// same policy as findNoteByRefOrAlias.
func slugCollision(notesDir, candidateSlug, excludePath string) (bool, error) {
	files, err := noteFiles(notesDir)
	if err != nil {
		return false, err
	}
	for _, path := range files {
		if path == excludePath {
			continue
		}
		if strings.TrimSuffix(filepath.Base(path), ".md") == candidateSlug {
			return true, nil
		}
		raw, err := os.ReadFile(path)
		if err != nil || bytes.Contains(raw, []byte("\r\n")) {
			continue
		}
		n, err := node.Parse(raw)
		if err != nil {
			continue
		}
		if containsString(n.Aliases, candidateSlug) {
			return true, nil
		}
	}
	return false, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// index
// ─────────────────────────────────────────────────────────────────────────────

func runNoteIndexE(cmd *cobra.Command, args []string) error {
	defer resetNoteFlags(cmd)

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return fmt.Errorf("note index: %w", err)
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("note index: load config: %w", err)
	}

	ix, err := index.Open(cfg)
	if err != nil {
		return fmt.Errorf("note index: open index: %w", err)
	}
	defer ix.Close()

	if _, err := ix.Reconcile(); err != nil {
		return fmt.Errorf("note index: reconcile index: %w", err)
	}

	db := ix.DB()
	rows, err := db.Query("SELECT id, loc FROM nodes WHERE type = 'note'")
	if err != nil {
		return fmt.Errorf("note index: query notes: %w", err)
	}
	type noteRow struct{ id, loc string }
	var noteRows []noteRow
	for rows.Next() {
		var r noteRow
		if err := rows.Scan(&r.id, &r.loc); err != nil {
			rows.Close()
			return fmt.Errorf("note index: scan note: %w", err)
		}
		noteRows = append(noteRows, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("note index: iterate notes: %w", err)
	}
	rows.Close()

	type entry struct {
		slug, title, description string
	}
	byDir := map[string][]entry{}
	for _, r := range noteRows {
		slug := strings.TrimSuffix(filepath.Base(r.loc), ".md")
		if slug == "index" {
			continue // generated catalog file, never an entry
		}
		props, err := loadProps(db, r.id)
		if err != nil {
			return fmt.Errorf("note index: %w", err)
		}
		title := props["title"]
		if title == "" {
			title = slug
		}
		dir := filepath.Dir(r.loc)
		byDir[dir] = append(byDir[dir], entry{slug: slug, title: title, description: props["description"]})
	}

	var written, skipped []string
	for dir, entries := range byDir {
		sort.Slice(entries, func(i, j int) bool { return entries[i].slug < entries[j].slug })
		var b strings.Builder
		b.WriteString(noteIndexMarker + "\n\n")
		b.WriteString("# Notes Index\n\n")
		for _, e := range entries {
			if e.description != "" {
				fmt.Fprintf(&b, "- [%s](%s.md) — %s\n", e.title, e.slug, e.description)
			} else {
				fmt.Fprintf(&b, "- [%s](%s.md)\n", e.title, e.slug)
			}
		}
		outPath := filepath.Join(cfg.VaultDir, dir, "index.md")
		rel, relErr := filepath.Rel(cfg.VaultDir, outPath)
		if relErr != nil {
			rel = outPath
		}
		rel = filepath.ToSlash(rel)
		if existing, readErr := os.ReadFile(outPath); readErr == nil && !bytes.HasPrefix(existing, []byte(noteIndexMarker)) {
			fmt.Fprintf(cmd.ErrOrStderr(), "warning: skipping %s: hand-authored index.md (no generated marker)\n", rel)
			skipped = append(skipped, rel)
			continue
		}
		if err := writeFileAtomic(outPath, []byte(b.String())); err != nil {
			return fmt.Errorf("note index: write %s: %w", outPath, err)
		}
		written = append(written, rel)
	}
	sort.Strings(written)
	sort.Strings(skipped)

	// Prune stale generated catalogs: a directory that no longer contains
	// any note drops out of byDir, so a marker-bearing index.md left there
	// is tool-owned garbage. Hand-authored files (no marker) are never
	// removed.
	notesDir := filepath.Join(cfg.VaultDir, "notes")
	var removed []string
	if _, statErr := os.Stat(notesDir); statErr == nil {
		walkErr := filepath.Walk(notesDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || info.Name() != "index.md" {
				return nil
			}
			relFile, relErr := filepath.Rel(cfg.VaultDir, path)
			if relErr != nil {
				return relErr
			}
			relFile = filepath.ToSlash(relFile)
			if _, live := byDir[filepath.Dir(relFile)]; live {
				return nil
			}
			raw, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if !bytes.HasPrefix(raw, []byte(noteIndexMarker)) {
				return nil
			}
			if rmErr := os.Remove(path); rmErr != nil {
				return rmErr
			}
			removed = append(removed, relFile)
			return nil
		})
		if walkErr != nil {
			return fmt.Errorf("note index: prune stale index.md: %w", walkErr)
		}
	}
	sort.Strings(removed)

	res := noteIndexResult{Files: written, Skipped: skipped, Removed: removed}
	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return fmt.Errorf("print result: %w", err)
		}
	}
	return nil
}
