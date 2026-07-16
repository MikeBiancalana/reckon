package cli

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/spf13/cobra"
)

// mintTodoULID is the seam the durable-create path mints its ULID through
// (instead of calling node.Mint() directly), so tests can override it to
// force a deterministic ULID collision (see TestTodoAdd_NoClobberExistingFile).
var mintTodoULID = node.Mint

// todoNow is the seam doneRecurringTodo's date arithmetic reads "today"
// through (instead of calling time.Now().UTC() directly), mirroring
// mintTodoULID above, so recurrence integration tests can pin a fixed
// completion date and assert the acceptance-criteria's absolute expected
// dates (see todo_recur_test.go's pinTodoNow). Always UTC, matching
// parseSchedDate/advanceSchedule's (recur.go) shared clock.
var todoNow = func() time.Time { return time.Now().UTC() }

// ─────────────────────────────────────────────────────────────────────────────
// Flag variables (package-global so cobra can bind them; each subcommand's
// RunE resets them, and their pflag Changed state, via defer resetTodoFlags).
// ─────────────────────────────────────────────────────────────────────────────

var (
	todoEphemeralFlag     bool
	todoScheduledFlag     string
	todoDeadlineFlag      string
	todoDependsFlag       string
	todoRepeatFlag        string
	todoAuthorFlag        string
	todoListAllFlag       bool
	todoListStateFlag     string
	todoListDurableFlag   bool
	todoListEphemeralFlag bool
	todoDoneEphemeralFlag bool
)

// resetTodoFlags restores todo flag variables to their defaults and clears the
// pflag Changed state on whichever of these flags are registered on cmd
// (add/list/done each register a different subset). Mirrors query.go's
// resetQueryFlags.
func resetTodoFlags(cmd *cobra.Command) {
	todoEphemeralFlag = false
	todoScheduledFlag = ""
	todoDeadlineFlag = ""
	todoDependsFlag = ""
	todoRepeatFlag = ""
	todoAuthorFlag = ""
	todoListAllFlag = false
	todoListStateFlag = ""
	todoListDurableFlag = false
	todoListEphemeralFlag = false
	todoDoneEphemeralFlag = false
	for _, name := range []string{"ephemeral", "scheduled", "deadline", "depends", "repeat", "author", "all", "state", "durable"} {
		if fl := cmd.Flags().Lookup(name); fl != nil {
			fl.Changed = false
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Commands
// ─────────────────────────────────────────────────────────────────────────────

// todoCmd is the parent for the durable+ephemeral todo family (add/list/done).
var todoCmd = &cobra.Command{
	Use:   "todo",
	Short: "Manage todo items (durable and ephemeral)",
	Long:  "Create, list, and complete todo items. Durable todos live one-per-file under todos/<ULID>.md; ephemeral todos are checkbox lines in a shared todos/inbox.md container.",
}

var todoAddCmd = &cobra.Command{
	Use:          "add <text...>",
	Short:        "Create a new todo (durable by default, or --ephemeral)",
	SilenceUsage: true,
	Args:         cobra.MinimumNArgs(1),
	RunE:         runTodoAddE,
}

var todoListCmd = &cobra.Command{
	Use:          "list",
	Short:        "List open todos (durable and ephemeral)",
	SilenceUsage: true,
	Args:         cobra.NoArgs,
	RunE:         runTodoListE,
}

var todoDoneCmd = &cobra.Command{
	Use:          "done <ref>",
	Short:        "Mark a todo done (durable ref/alias, or --ephemeral <index>)",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runTodoDoneE,
}

func init() {
	af := todoAddCmd.Flags()
	af.BoolVar(&todoEphemeralFlag, "ephemeral", false, "Create an ephemeral inbox item instead of a durable todo")
	af.StringVar(&todoScheduledFlag, "scheduled", "", "Scheduled date (durable only)")
	af.StringVar(&todoDeadlineFlag, "deadline", "", "Deadline date (durable only)")
	af.StringVar(&todoDependsFlag, "depends", "", "ULID/alias this todo depends on (durable only)")
	af.StringVar(&todoRepeatFlag, "repeat", "", "Org-style repeater cookie (+Nd, ++Nd, .+Nd; durable only, requires --scheduled)")
	af.StringVar(&todoAuthorFlag, "author", "", "Author to record (default: $RECKON_AUTHOR, $USER, or \"local\")")

	lf := todoListCmd.Flags()
	lf.BoolVar(&todoListAllFlag, "all", false, "Include done/checked items")
	lf.StringVar(&todoListStateFlag, "state", "", "Filter durable todos by exact state")
	lf.BoolVar(&todoListDurableFlag, "durable", false, "Show only durable todos")
	lf.BoolVar(&todoListEphemeralFlag, "ephemeral", false, "Show only ephemeral todos")

	df := todoDoneCmd.Flags()
	df.BoolVar(&todoDoneEphemeralFlag, "ephemeral", false, "Target the ephemeral inbox: <ref> is a 1-based line index")
	df.StringVar(&todoAuthorFlag, "author", "", "Author to record on a recurring rule's did:: audit entry (default: $RECKON_AUTHOR, $USER, or \"local\")")

	todoCmd.AddCommand(todoAddCmd, todoListCmd, todoDoneCmd)
}

// ─────────────────────────────────────────────────────────────────────────────
// Result types (exact shapes pinned by internal/cli/todo_test.go's header
// comment; do not change field names/tags without updating that pin).
// ─────────────────────────────────────────────────────────────────────────────

// todoAddResult is the structured summary of one `rk todo add` run.
type todoAddResult struct {
	Kind  string `json:"kind"`            // "durable" | "ephemeral"
	Path  string `json:"path"`            // vault-relative: "todos/<ULID>.md" or "todos/inbox.md"
	ID    string `json:"id,omitempty"`    // durable only: the new node's ULID
	Line  int    `json:"line,omitempty"`  // ephemeral only: 1-based index of the appended item
	State string `json:"state,omitempty"` // durable only: "open" on create
}

func (r todoAddResult) Pretty() string {
	if r.Kind == "ephemeral" {
		return fmt.Sprintf("todo: added ephemeral item to %s (line %d)", r.Path, r.Line)
	}
	return fmt.Sprintf("todo: added %s (id %s, state %s)", r.Path, r.ID, r.State)
}

// todoListItem is one row of `rk todo list` output, durable or ephemeral.
type todoListItem struct {
	Kind      string `json:"kind"`                // "durable" | "ephemeral"
	ID        string `json:"id,omitempty"`        // durable only: ULID
	Path      string `json:"path,omitempty"`      // durable only: vault-relative file path
	Container string `json:"container,omitempty"` // ephemeral only: vault-relative container path
	Line      int    `json:"line,omitempty"`      // ephemeral only: stable 1-based index in file order
	State     string `json:"state,omitempty"`     // durable only: "open" | "done"
	Checked   bool   `json:"checked"`             // ephemeral only (meaningful false, so no omitempty)
	Scheduled string `json:"scheduled,omitempty"` // durable only
	Deadline  string `json:"deadline,omitempty"`  // durable only
	Depends   string `json:"depends,omitempty"`   // durable only
	Repeat    string `json:"repeat,omitempty"`    // durable only: repeater cookie, sourced from props["repeat"]
	Body      string `json:"body"`                // node body (durable) / checkbox text (ephemeral)
	Title     string `json:"title,omitempty"`     // durable only: derived first non-empty body line
}

// todoListResult wraps `rk todo list`'s items so --json emits a single object
// ({"items": []} on empty), not a bare top-level array.
type todoListResult struct {
	Items []todoListItem `json:"items"`
}

func (r todoListResult) Pretty() string {
	if len(r.Items) == 0 {
		return "todo: no items"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "todo: %d item(s)", len(r.Items))
	for _, it := range r.Items {
		if it.Kind == "ephemeral" {
			mark := " "
			if it.Checked {
				mark = "x"
			}
			fmt.Fprintf(&b, "\n  [%s] %d. %s", mark, it.Line, it.Body)
			continue
		}
		fmt.Fprintf(&b, "\n  %s [%s] %s", it.ID, it.State, it.Title)
	}
	return b.String()
}

// todoDoneResult is the structured summary of one `rk todo done` run.
//
// The v1-T6 recurrence fields (Recurred through Materialized) are all
// omitempty and populated only on the recurring-todo branch
// (doneRecurringTodo); a plain (non-recurring) or ephemeral completion never
// sets them. State stays "open" (never "done") on the recurrence path — the
// cursor advance is the completion signal, not a state flip (plan.md
// "State-transition question", FIRM).
type todoDoneResult struct {
	Kind    string `json:"kind"`            // "durable" | "ephemeral"
	Ref     string `json:"ref"`             // the ref/index the caller passed
	Path    string `json:"path,omitempty"`  // vault-relative: the file mutated
	ID      string `json:"id,omitempty"`    // durable only: resolved ULID
	State   string `json:"state,omitempty"` // durable only: "done" (or "open" on the recurrence path)
	Skipped bool   `json:"skipped"`         // true = idempotent no-op (already done/checked)

	Recurred     bool   `json:"recurred,omitempty"`     // true iff the recurrence branch ran
	Scheduled    string `json:"scheduled,omitempty"`    // newly-advanced date
	Repeat       string `json:"repeat,omitempty"`       // the rule's repeater cookie
	DidEntryID   string `json:"did_entry_id,omitempty"` // the did::-linked audit log entry's ULID
	DidEntryPath string `json:"did_entry_path,omitempty"`
	Missed       int    `json:"missed,omitempty"`       // count of fully-elapsed intervals since the old cursor
	Materialized string `json:"materialized,omitempty"` // "todos/inbox.md#<line>" iff Missed > 0
}

func (r todoDoneResult) Pretty() string {
	if r.Skipped {
		return fmt.Sprintf("todo: %s already done (skipped)", r.Ref)
	}
	if r.Recurred {
		s := fmt.Sprintf("todo: %s advanced to %s (repeat %s)", r.Ref, r.Scheduled, r.Repeat)
		if r.Missed > 0 {
			s += fmt.Sprintf("; missed %d occurrence(s), materialized %s", r.Missed, r.Materialized)
		}
		return s
	}
	return fmt.Sprintf("todo: %s marked done", r.Ref)
}

// ─────────────────────────────────────────────────────────────────────────────
// resolveAuthor (plan.md D8)
// ─────────────────────────────────────────────────────────────────────────────

// resolveAuthor determines the author string to stamp on a newly created
// node: --author flag > $RECKON_AUTHOR > $USER > "local". Always non-empty.
func resolveAuthor(flag string) string {
	if flag != "" {
		return flag
	}
	if v := os.Getenv("RECKON_AUTHOR"); v != "" {
		return v
	}
	if v := os.Getenv("USER"); v != "" {
		return v
	}
	return "local"
}

// ─────────────────────────────────────────────────────────────────────────────
// add
// ─────────────────────────────────────────────────────────────────────────────

func runTodoAddE(cmd *cobra.Command, args []string) error {
	defer resetTodoFlags(cmd)

	ephemeral := todoEphemeralFlag
	scheduled := todoScheduledFlag
	deadline := todoDeadlineFlag
	depends := todoDependsFlag
	repeat := todoRepeatFlag
	author := resolveAuthor(todoAuthorFlag)
	body := strings.TrimSpace(strings.Join(args, " "))
	if body == "" {
		return fmt.Errorf("todo add: empty body text")
	}

	if ephemeral && (scheduled != "" || deadline != "" || depends != "" || repeat != "") {
		return fmt.Errorf("todo add: --ephemeral does not support --scheduled/--deadline/--depends/--repeat (durable-only)")
	}
	if repeat != "" {
		if scheduled == "" {
			return fmt.Errorf("todo add: --repeat requires --scheduled (a repeater with no anchor date cannot advance)")
		}
		if _, err := parseRepeat(repeat); err != nil {
			return fmt.Errorf("todo add: %w", err)
		}
		if _, err := parseSchedDate(scheduled); err != nil {
			return fmt.Errorf("todo add: --scheduled: %w", err)
		}
	}

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("todo add: load config: %w", err)
	}

	todosDir := filepath.Join(cfg.VaultDir, "todos")
	if err := os.MkdirAll(todosDir, 0o755); err != nil {
		return fmt.Errorf("todo add: create todos dir: %w", err)
	}

	var res todoAddResult
	if ephemeral {
		res, err = addEphemeralTodo(todosDir, author, body)
	} else {
		res, err = addDurableTodo(todosDir, author, body, scheduled, deadline, depends, repeat)
	}
	if err != nil {
		return err
	}

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return err
		}
	}
	return nil
}

// addDurableTodo creates todos/<ULID>.md via the NewNode -> set fields ->
// Render -> Parse -> writeFileAtomic recipe (plan.md D1/D9). The ULID is
// minted via the mintTodoULID seam so tests can force a collision. repeat
// (v1-T6) is the raw repeater cookie; caller (runTodoAddE) has already
// validated it via parseRepeat and required --scheduled to be set alongside
// it.
func addDurableTodo(todosDir, author, body, scheduled, deadline, depends, repeat string) (todoAddResult, error) {
	id := mintTodoULID()
	path := filepath.Join(todosDir, id+".md")

	if _, err := os.Stat(path); err == nil {
		return todoAddResult{}, fmt.Errorf("todo add: refusing to overwrite existing file at %s", path)
	} else if !os.IsNotExist(err) {
		return todoAddResult{}, fmt.Errorf("todo add: stat %s: %w", path, err)
	}

	n := node.NewNode("todo", author, body+"\n")
	n.ULID = id
	n.Time = time.Now().UTC().Format(time.RFC3339)
	props := map[string]string{"state": "open"}
	if scheduled != "" {
		props["scheduled"] = scheduled
	}
	if deadline != "" {
		props["deadline"] = deadline
	}
	if repeat != "" {
		props["repeat"] = repeat
	}
	n.Props = props
	if depends != "" {
		n.Links = []node.Link{{Rel: "depends-on", To: depends}}
	}

	rendered := n.Render()
	parsed, err := node.Parse(rendered)
	if err != nil {
		return todoAddResult{}, fmt.Errorf("todo add: parse rendered node: %w", err)
	}

	if err := writeFileAtomic(path, parsed.Serialize()); err != nil {
		return todoAddResult{}, fmt.Errorf("todo add: write: %w", err)
	}

	return todoAddResult{
		Kind:  "durable",
		Path:  "todos/" + id + ".md",
		ID:    id,
		State: "open",
	}, nil
}

// addEphemeralTodo creates todos/inbox.md on first use, or appends a checkbox
// line at EOF on subsequent calls (plan.md D2).
//
// The container's raw bytes deliberately never end with a trailing newline:
// each append prepends "\n" to the new item rather than terminating the
// previous item with one. This keeps every previously-written line's exact
// byte span (and its position when the file is split on "\n") untouched by a
// later append — appending a trailing newewline instead would shift the
// file's final (empty) split segment when appending, which would otherwise
// corrupt a naive line-by-line byte-identity check on the *previous* content.
func addEphemeralTodo(todosDir, author, text string) (todoAddResult, error) {
	path := filepath.Join(todosDir, "inbox.md")
	item := "- [ ] " + text

	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		n := node.NewNode("todo-ephemeral", author, "# Inbox\n\n"+item)
		n.Time = time.Now().UTC().Format(time.RFC3339)
		rendered := n.Render()
		parsed, perr := node.Parse(rendered)
		if perr != nil {
			return todoAddResult{}, fmt.Errorf("todo add: parse rendered inbox: %w", perr)
		}
		if err := writeFileAtomic(path, parsed.Serialize()); err != nil {
			return todoAddResult{}, fmt.Errorf("todo add: write inbox: %w", err)
		}
		return todoAddResult{Kind: "ephemeral", Path: "todos/inbox.md", Line: 1}, nil
	}
	if err != nil {
		return todoAddResult{}, fmt.Errorf("todo add: read inbox: %w", err)
	}

	if bytes.Contains(raw, []byte("\r\n")) {
		return todoAddResult{}, fmt.Errorf("todo add: CRLF line endings are not supported (reckon-vj55)")
	}
	n, err := node.Parse(raw)
	if err != nil {
		return todoAddResult{}, fmt.Errorf("todo add: parse inbox: %w", err)
	}
	nextLine := len(splitChecklistLines(n.Body)) + 1

	suffix := "\n" + item
	appended := make([]byte, 0, len(raw)+len(suffix))
	appended = append(appended, raw...)
	appended = append(appended, suffix...)
	if err := writeFileAtomic(path, appended); err != nil {
		return todoAddResult{}, fmt.Errorf("todo add: write inbox: %w", err)
	}
	return todoAddResult{Kind: "ephemeral", Path: "todos/inbox.md", Line: nextLine}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// list
// ─────────────────────────────────────────────────────────────────────────────

func runTodoListE(cmd *cobra.Command, args []string) error {
	defer resetTodoFlags(cmd)

	all := todoListAllFlag
	stateFilter := strings.TrimSpace(todoListStateFlag)
	durableOnly := todoListDurableFlag
	ephemeralOnly := todoListEphemeralFlag

	if durableOnly && ephemeralOnly {
		return fmt.Errorf("todo list: --durable and --ephemeral are mutually exclusive")
	}

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("todo list: load config: %w", err)
	}

	ix, err := index.Open(cfg)
	if err != nil {
		return fmt.Errorf("todo list: open index: %w", err)
	}
	defer ix.Close()

	if _, err := ix.Reconcile(); err != nil {
		return fmt.Errorf("todo list: reconcile index: %w", err)
	}

	res := todoListResult{Items: []todoListItem{}}

	if !ephemeralOnly {
		durItems, err := listDurableTodos(ix.DB(), all, stateFilter)
		if err != nil {
			return err
		}
		res.Items = append(res.Items, durItems...)
	}
	if !durableOnly {
		ephItems, err := listEphemeralTodos(ix.DB(), all)
		if err != nil {
			return err
		}
		res.Items = append(res.Items, ephItems...)
	}

	return output.New(cmd.OutOrStdout(), mode).Print(res)
}

// listDurableTodos closes rows manually (not deferred) before issuing the
// per-row loadTodoProps queries below -- a defer would hold this cursor open
// across those nested queries on the same *sql.DB.
func listDurableTodos(db *sql.DB, all bool, stateFilter string) ([]todoListItem, error) {
	rows, err := db.Query("SELECT id, body, title FROM nodes WHERE type = 'todo'")
	if err != nil {
		return nil, fmt.Errorf("todo list: query durable nodes: %w", err)
	}
	type row struct{ id, body, title string }
	var candidates []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.id, &r.body, &r.title); err != nil {
			rows.Close()
			return nil, fmt.Errorf("todo list: scan durable node: %w", err)
		}
		candidates = append(candidates, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("todo list: iterate durable nodes: %w", err)
	}
	rows.Close()

	var items []todoListItem
	for _, r := range candidates {
		props, err := loadTodoProps(db, r.id)
		if err != nil {
			return nil, err
		}
		state := props["state"]
		if stateFilter != "" {
			if state != stateFilter {
				continue
			}
		} else if !all && state != "open" && state != "in-progress" {
			continue
		}
		depends, err := loadDependsOn(db, r.id)
		if err != nil {
			return nil, err
		}
		items = append(items, todoListItem{
			Kind:      "durable",
			ID:        r.id,
			Path:      "todos/" + r.id + ".md",
			State:     state,
			Scheduled: props["scheduled"],
			Deadline:  props["deadline"],
			Depends:   depends,
			Repeat:    props["repeat"],
			Body:      strings.TrimSpace(r.body),
			Title:     r.title,
		})
	}
	return items, nil
}

func loadTodoProps(db *sql.DB, id string) (map[string]string, error) {
	rows, err := db.Query("SELECT key, value FROM node_props WHERE id = ?", id)
	if err != nil {
		return nil, fmt.Errorf("todo list: load props for %q: %w", id, err)
	}
	defer rows.Close()
	props := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("todo list: scan prop for %q: %w", id, err)
		}
		props[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("todo list: iterate props for %q: %w", id, err)
	}
	return props, nil
}

func loadDependsOn(db *sql.DB, id string) (string, error) {
	var dst string
	err := db.QueryRow("SELECT dst FROM edges WHERE src = ? AND rel = 'depends-on' LIMIT 1", id).Scan(&dst)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("todo list: load depends-on for %q: %w", id, err)
	}
	return dst, nil
}

func listEphemeralTodos(db *sql.DB, all bool) ([]todoListItem, error) {
	var id, body string
	err := db.QueryRow("SELECT id, body FROM nodes WHERE type = 'todo-ephemeral' LIMIT 1").Scan(&id, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("todo list: query ephemeral container: %w", err)
	}

	var items []todoListItem
	for _, it := range splitChecklistLines(body) {
		if !all && it.checked {
			continue
		}
		items = append(items, todoListItem{
			Kind:      "ephemeral",
			Container: "todos/inbox.md",
			Line:      it.index,
			Checked:   it.checked,
			Body:      it.text,
		})
	}
	return items, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// done
// ─────────────────────────────────────────────────────────────────────────────

func runTodoDoneE(cmd *cobra.Command, args []string) error {
	defer resetTodoFlags(cmd)

	ref := args[0]
	ephemeral := todoDoneEphemeralFlag

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("todo done: load config: %w", err)
	}

	var res todoDoneResult
	if ephemeral {
		res, err = doneEphemeralTodo(cfg.VaultDir, ref)
	} else {
		res, err = doneDurableTodo(cfg.VaultDir, ref)
	}
	if err != nil {
		return err
	}

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return err
		}
	}
	return nil
}

// doneDurableTodo resolves ref to a durable todo file (ULID fast-path, else a
// walk over todos/*.md matching ULID or alias), then flips state->done via a
// span-local SetField, or reports an idempotent skip if already done.
func doneDurableTodo(vaultDir, ref string) (todoDoneResult, error) {
	todosDir := filepath.Join(vaultDir, "todos")

	fastPath := filepath.Join(todosDir, ref+".md")
	n, foundPath, err := loadDurableTodoAt(fastPath)
	if err != nil {
		return todoDoneResult{}, err
	}
	if n != nil && n.Type != "todo" {
		// e.g. ref == "inbox" resolving to the ephemeral container: not a
		// durable todo match, fall through to the type-filtered walk below.
		n, foundPath = nil, ""
	}
	if n == nil {
		n, foundPath, err = findDurableTodoByRefOrAlias(todosDir, ref)
		if err != nil {
			return todoDoneResult{}, err
		}
	}
	if n == nil {
		return todoDoneResult{}, fmt.Errorf("todo done: no todo found matching %q (not found)", ref)
	}

	return completeDurableTodoNode(vaultDir, n, foundPath, ref, false, true)
}

// completeDurableTodoNode is doneDurableTodo's shared completion body, also
// called by `rk today act x` (today.go's actDone).
//
// logDid governs the plain (non-recurring) state->done branch's did-entry
// write below. rk todo done always calls with logDid=false, preserving its
// existing (no did-entry) output byte-for-byte; `rk today act x` passes
// logDid = !--no-log (default true).
//
// recurLogDid separately governs whether the recurrence branch
// (doneRecurringTodo) writes a did-entry. rk todo done always passes true
// here (its recurrence branch has always logged unconditionally). Only
// `rk today act x --no-log` on a recurring row passes false, so --no-log's
// promised "suppress the did-entry log write when completing" holds on the
// recurring path too; the cursor-advance mechanics themselves (state stays
// open, scheduled advances) are untouched either way.
//
// A repeat: prop takes the recurrence branch instead of the plain
// state->done path below. A recurring rule's state is never "done" (it
// stays "open" so it remains visible in the default list), so the
// idempotent-skip check just below never trips for it — that check, and the
// state flip, are the non-recurring path only.
func completeDurableTodoNode(vaultDir string, n *node.Node, foundPath, ref string, logDid, recurLogDid bool) (todoDoneResult, error) {
	relPath := relTodoPath(vaultDir, foundPath)
	id := n.ULID

	if n.HasField("repeat") {
		repeat, ok := n.Props["repeat"]
		if !ok || strings.TrimSpace(repeat) == "" {
			return todoDoneResult{}, fmt.Errorf("todo done: malformed repeat: prop (must be a plain repeater cookie, not a link) (id %s)", id)
		}
		return doneRecurringTodo(vaultDir, n, foundPath, ref, repeat, recurLogDid)
	}

	if n.Props["state"] == "done" {
		return todoDoneResult{
			Kind: "durable", Ref: ref, Path: relPath, ID: id, State: "done", Skipped: true,
		}, nil
	}

	if err := n.SetField("state", "done"); err != nil {
		return todoDoneResult{}, fmt.Errorf("todo done: set state: %w", err)
	}
	if err := writeFileAtomic(foundPath, n.Serialize()); err != nil {
		return todoDoneResult{}, fmt.Errorf("todo done: write: %w", err)
	}

	res := todoDoneResult{
		Kind: "durable", Ref: ref, Path: relPath, ID: id, State: "done", Skipped: false,
	}

	if !logDid {
		return res, nil
	}

	author := resolveAuthor(todoAuthorFlag)
	if embeddedHeaderRe.MatchString(author) {
		return res, fmt.Errorf("todo done: author must not contain a line starting with \"## \"")
	}

	now := todoNow()
	dayStr := now.Format("2006-01-02")
	hhmm := now.Format("15:04")
	body := fmt.Sprintf("completed todo %s", id)

	logDir := filepath.Join(vaultDir, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return res, fmt.Errorf("todo done: create log dir: %w", err)
	}
	logRes, err := appendDidLogEntry(logDir, dayStr, hhmm, author, body, id)
	if err != nil {
		return res, fmt.Errorf("todo done: write did entry: %w", err)
	}
	res.DidEntryID = logRes.ID
	res.DidEntryPath = logRes.Path

	return res, nil
}

// doneRecurringTodo is doneDurableTodo's v1-T6 recurrence branch: n carries a
// non-empty repeat: prop. It validates the repeater cookie and the current
// scheduled: cursor (pure, pre-write — EC-2/3/4 guarantee the file is
// untouched on any validation error), computes the next date and any missed
// interval count (advanceSchedule, recur.go), advances the cursor via a
// span-local SetField("scheduled", next) — state is deliberately never
// touched — writes a did::-linked audit log entry, and, iff intervals piled
// up, materializes exactly one ephemeral catch-up instance into
// todos/inbox.md (plan.md "Data Flow — doneRecurringTodo").
//
// Ordering/partial-failure: the cursor advance is the authoritative write
// and happens first; the did-entry write and pile-up materialization happen
// after and are surfaced as errors (wrapped, returned) if they fail, but the
// already-advanced cursor is not rolled back — writes span multiple files
// and cannot be atomic as a unit (plan.md "Known Risks", accepted).
//
// logDid gates only the did-entry write (log dir creation +
// appendDidLogEntry) below; the cursor advance and any pile-up
// materialization always run regardless (`rk today act x --no-log` on a
// recurring row must still advance the cursor, it just suppresses the
// audit log-entry write). doneDurableTodo (`rk todo done`) always passes
// true here, preserving its existing unconditional-log recurrence behavior.
func doneRecurringTodo(vaultDir string, n *node.Node, foundPath, ref, repeatCookie string, logDid bool) (todoDoneResult, error) {
	relPath := relTodoPath(vaultDir, foundPath)
	id := n.ULID

	spec, err := parseRepeat(repeatCookie)
	if err != nil {
		return todoDoneResult{}, fmt.Errorf("todo done: %w", err)
	}

	schedStr, ok := n.Props["scheduled"]
	if !ok || !n.HasField("scheduled") || strings.TrimSpace(schedStr) == "" {
		return todoDoneResult{}, fmt.Errorf("todo done: cannot advance a recurrence cursor with no scheduled date (id %s)", id)
	}
	old, err := parseSchedDate(schedStr)
	if err != nil {
		return todoDoneResult{}, fmt.Errorf("todo done: %w", err)
	}

	now := todoNow()
	dayStr := now.Format("2006-01-02")
	today, err := parseSchedDate(dayStr)
	if err != nil {
		return todoDoneResult{}, fmt.Errorf("todo done: internal: reparse today %q: %w", dayStr, err)
	}

	next, missed := advanceSchedule(old, today, spec)
	nextStr := next.Format("2006-01-02")

	// Validated before the authoritative cursor write below: author feeds
	// straight into the did-entry header, and EC-13 means a recurring rule
	// has no idempotency signal, so a failure discovered only after the
	// cursor already advanced would double-advance on retry.
	author := resolveAuthor(todoAuthorFlag)
	if embeddedHeaderRe.MatchString(author) {
		return todoDoneResult{}, fmt.Errorf("todo done: author must not contain a line starting with \"## \"")
	}

	if err := n.SetField("scheduled", nextStr); err != nil {
		return todoDoneResult{}, fmt.Errorf("todo done: set scheduled: %w", err)
	}
	if err := writeFileAtomic(foundPath, n.Serialize()); err != nil {
		return todoDoneResult{}, fmt.Errorf("todo done: write: %w", err)
	}

	res := todoDoneResult{
		Kind: "durable", Ref: ref, Path: relPath, ID: id, State: "open", Skipped: false,
		Recurred: true, Scheduled: nextStr, Repeat: repeatCookie, Missed: missed,
	}

	if logDid {
		hhmm := now.Format("15:04")
		body := fmt.Sprintf("completed recurring todo %s (repeat %s); advanced scheduled %s → %s", id, repeatCookie, schedStr, nextStr)

		logDir := filepath.Join(vaultDir, "log")
		if err := os.MkdirAll(logDir, 0o755); err != nil {
			return res, fmt.Errorf("todo done: create log dir: %w", err)
		}
		logRes, err := appendDidLogEntry(logDir, dayStr, hhmm, author, body, id)
		if err != nil {
			return res, fmt.Errorf("todo done: write did entry: %w", err)
		}
		res.DidEntryID = logRes.ID
		res.DidEntryPath = logRes.Path
	}

	if missed > 0 {
		todosDir := filepath.Join(vaultDir, "todos")
		ruleRef := id
		if len(n.Aliases) > 0 {
			ruleRef = n.Aliases[0]
		}
		text := fmt.Sprintf("[[%s]] missed %d occurrence(s) (was due %s, repeat %s)", ruleRef, missed, schedStr, repeatCookie)
		addRes, err := addEphemeralTodo(todosDir, author, text)
		if err != nil {
			return res, fmt.Errorf("todo done: materialize pile-up: %w", err)
		}
		res.Materialized = fmt.Sprintf("%s#%d", addRes.Path, addRes.Line)
	}

	return res, nil
}

// loadDurableTodoAt reads and parses the file at path. A nonexistent file is
// reported as (nil, "", nil) — not an error — so callers can fall back to a
// search. CRLF files are refused (mirrors adopt.go, reckon-vj55).
func loadDurableTodoAt(path string) (*node.Node, string, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", fmt.Errorf("todo done: read %s: %w", path, err)
	}
	if bytes.Contains(raw, []byte("\r\n")) {
		return nil, "", fmt.Errorf("todo done: CRLF line endings are not supported (reckon-vj55): %s", path)
	}
	n, err := node.Parse(raw)
	if err != nil {
		return nil, "", fmt.Errorf("todo done: parse %s: %w", path, err)
	}
	return n, path, nil
}

// findDurableTodoByRefOrAlias walks todos/*.md looking for a durable todo
// (type "todo") whose ULID or alias matches ref. Unparsable/CRLF files are
// skipped rather than aborting the whole search.
func findDurableTodoByRefOrAlias(todosDir, ref string) (*node.Node, string, error) {
	matches, err := filepath.Glob(filepath.Join(todosDir, "*.md"))
	if err != nil {
		return nil, "", fmt.Errorf("todo done: glob todos dir: %w", err)
	}
	for _, path := range matches {
		raw, err := os.ReadFile(path)
		if err != nil || bytes.Contains(raw, []byte("\r\n")) {
			continue
		}
		n, err := node.Parse(raw)
		if err != nil || n.Type != "todo" {
			continue
		}
		if n.ULID == ref || containsString(n.Aliases, ref) {
			return n, path, nil
		}
	}
	return nil, "", nil
}

// doneEphemeralTodo flips the ref'th (1-based, file order) checkbox line in
// todos/inbox.md from unchecked to checked via a one-byte surgical splice.
func doneEphemeralTodo(vaultDir, ref string) (todoDoneResult, error) {
	idx, err := strconv.Atoi(ref)
	if err != nil || idx < 1 {
		return todoDoneResult{}, fmt.Errorf("todo done: --ephemeral requires a positive 1-based index, got %q", ref)
	}

	path := filepath.Join(vaultDir, "todos", "inbox.md")
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return todoDoneResult{}, fmt.Errorf("todo done: ephemeral container not found (not found): %s", path)
	}
	if err != nil {
		return todoDoneResult{}, fmt.Errorf("todo done: read %s: %w", path, err)
	}
	if bytes.Contains(raw, []byte("\r\n")) {
		return todoDoneResult{}, fmt.Errorf("todo done: CRLF line endings are not supported (reckon-vj55): %s", path)
	}

	newRaw, alreadyChecked, found := flipChecklistLine(raw, idx)
	if !found {
		return todoDoneResult{}, fmt.Errorf("todo done: index %d out of range (not found)", idx)
	}
	if alreadyChecked {
		return todoDoneResult{Kind: "ephemeral", Ref: ref, Path: "todos/inbox.md", Skipped: true}, nil
	}

	if err := writeFileAtomic(path, newRaw); err != nil {
		return todoDoneResult{}, fmt.Errorf("todo done: write %s: %w", path, err)
	}
	return todoDoneResult{Kind: "ephemeral", Ref: ref, Path: "todos/inbox.md", Skipped: false}, nil
}

// relTodoPath converts an absolute file path to a vault-relative,
// forward-slash-separated display path.
func relTodoPath(vaultDir, path string) string {
	rel, err := filepath.Rel(vaultDir, path)
	if err != nil {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(rel)
}

func containsString(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// Checkbox-line helpers (ephemeral container body <-> structured items).
// ─────────────────────────────────────────────────────────────────────────────

// checklistItemRe matches a markdown task-list line: "- [ ] text" / "- [x] text"
// (optionally "* " bullets). Capture group 1 is the mark char, group 2 the text.
var checklistItemRe = regexp.MustCompile(`^[-*] \[([ xX])\] ?(.*)$`)

// checklistMarkRe locates just the bracketed mark of each checklist line
// within full raw file bytes (multiline mode), for the surgical done-flip.
var checklistMarkRe = regexp.MustCompile(`(?m)^[-*] \[([ xX])\]`)

type checklistLineItem struct {
	index   int
	checked bool
	text    string
}

// splitChecklistLines scans body line by line, assigning each checkbox line a
// stable 1-based index in file order (plan.md D2/D5).
func splitChecklistLines(body string) []checklistLineItem {
	var out []checklistLineItem
	idx := 0
	for _, line := range strings.Split(body, "\n") {
		m := checklistItemRe.FindStringSubmatch(strings.TrimRight(line, "\r"))
		if m == nil {
			continue
		}
		idx++
		out = append(out, checklistLineItem{
			index:   idx,
			checked: m[1] == "x" || m[1] == "X",
			text:    m[2],
		})
	}
	return out
}

// flipChecklistLine flips the idx'th (1-based, file order) checkbox mark in
// raw from unchecked to checked, touching only that single byte. Returns
// found=false if idx is out of range.
func flipChecklistLine(raw []byte, idx int) (newRaw []byte, alreadyChecked bool, found bool) {
	matches := checklistMarkRe.FindAllSubmatchIndex(raw, -1)
	if idx < 1 || idx > len(matches) {
		return nil, false, false
	}
	m := matches[idx-1]
	markStart := m[2]
	mark := raw[markStart]
	if mark == 'x' || mark == 'X' {
		return raw, true, true
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	out[markStart] = 'x'
	return out, false, true
}
