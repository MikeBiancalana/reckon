package cli

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/output"
)

// todoEditResult is the structured summary of one `rk todo edit` run. The set
// fields carry the values written; Cleared lists the fields now absent, in
// the fixed field order.
type todoEditResult struct {
	Kind      string   `json:"kind"`
	Ref       string   `json:"ref"`
	ID        string   `json:"id,omitempty"`
	Path      string   `json:"path"`
	State     string   `json:"state,omitempty"`
	Deadline  string   `json:"deadline,omitempty"`
	Scheduled string   `json:"scheduled,omitempty"`
	Priority  string   `json:"priority,omitempty"`
	Depends   string   `json:"depends,omitempty"`
	Cleared   []string `json:"cleared,omitempty"`
}

func (r todoEditResult) Pretty() string {
	var parts []string
	for _, kv := range [][2]string{
		{"deadline", r.Deadline}, {"scheduled", r.Scheduled},
		{"priority", r.Priority}, {"depends", r.Depends},
	} {
		if kv[1] != "" {
			parts = append(parts, kv[0]+" "+kv[1])
		}
	}
	if len(r.Cleared) > 0 {
		parts = append(parts, "cleared "+strings.Join(r.Cleared, ", "))
	}
	return fmt.Sprintf("todo: %s updated (%s)", r.Ref, strings.Join(parts, ", "))
}

// todoEditField is one editable field: its flag name, the frontmatter keys a
// clear removes, and the set/clear request taken from the flags.
type todoEditField struct {
	name  string
	keys  []string
	set   bool
	value string
	clear bool
}

func runTodoEditE(cmd *cobra.Command, args []string) error {
	defer resetTodoFlags(cmd)

	ref := args[0]
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("todo edit: empty ref (give a todo ULID or alias)")
	}
	flags := cmd.Flags()
	fields := []*todoEditField{
		{name: "deadline", keys: []string{"deadline"}, set: flags.Changed("deadline"), value: todoDeadlineFlag, clear: todoNoDeadlineFlag},
		{name: "scheduled", keys: []string{"scheduled"}, set: flags.Changed("scheduled"), value: todoScheduledFlag, clear: todoNoScheduledFlag},
		{name: "priority", keys: []string{"priority"}, set: flags.Changed("priority"), value: todoPriorityFlag, clear: todoNoPriorityFlag},
		{name: "depends", keys: []string{"depends-on", "depends"}, set: flags.Changed("depends"), value: todoDependsFlag, clear: todoNoDependsFlag},
	}

	anyRequested := false
	for _, f := range fields {
		anyRequested = anyRequested || f.set || f.clear
	}
	if !anyRequested {
		return fmt.Errorf("todo edit: nothing to change (give --deadline/--scheduled/--depends/--priority or a --no-<field> flag)")
	}
	if err := validateTodoEditFlags(fields); err != nil {
		return err
	}

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}
	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("todo edit: load config: %w", err)
	}

	n, foundPath, err := resolveDurableTodoRef("todo edit", cfg.VaultDir, ref)
	if err != nil {
		return err
	}
	if n == nil {
		return fmt.Errorf("todo edit: no todo found matching %q (not found)", ref)
	}
	original := n.Serialize()

	var ix *index.Index
	defer func() {
		if ix != nil {
			ix.Close()
		}
	}()
	for _, f := range fields {
		switch {
		case f.name == "depends" && f.set:
			ix, err = index.Open(cfg)
			if err != nil {
				return fmt.Errorf("todo edit: --depends: open index: %w", err)
			}
			if _, err := ix.Reconcile(); err != nil {
				return fmt.Errorf("todo edit: --depends: reconcile index: %w", err)
			}
			if err := checkTodoDepends(ix.DB(), relTodoPath(cfg.VaultDir, foundPath), f.value); err != nil {
				return fmt.Errorf("todo edit: --depends: %w", err)
			}
		case f.name == "scheduled" && f.clear && strings.TrimSpace(n.Props["repeat"]) != "":
			return fmt.Errorf("todo edit: --no-scheduled: todo has repeat; a recurring todo needs its scheduled cursor")
		}
	}

	res := todoEditResult{Kind: "durable", Ref: ref, ID: n.ULID, Path: relTodoPath(cfg.VaultDir, foundPath)}
	for _, f := range fields {
		if !f.clear {
			continue
		}
		for _, k := range f.keys {
			if err := n.RemoveField(k); err != nil {
				return fmt.Errorf("todo edit: --no-%s: %w", f.name, err)
			}
		}
		res.Cleared = append(res.Cleared, f.name)
	}
	for _, f := range fields {
		if !f.set {
			continue
		}
		value := f.value
		if f.name == "depends" {
			value = strconv.Quote("[[" + f.value + "]]")
		}
		if err := n.UpsertField(f.keys[0], value); err != nil {
			return fmt.Errorf("todo edit: --%s: %w", f.name, err)
		}
		switch f.name {
		case "deadline":
			res.Deadline = f.value
		case "scheduled":
			res.Scheduled = f.value
		case "priority":
			res.Priority = f.value
		case "depends":
			res.Depends = f.value
		}
	}
	res.State = n.Props["state"]

	if !bytes.Equal(original, n.Serialize()) {
		if err := writeFileAtomic(foundPath, n.Serialize()); err != nil {
			return fmt.Errorf("todo edit: write: %w", err)
		}
	}

	// The file write is authoritative and already done, so index trouble is a
	// warning, never a failure. Reconcile runs before printing so a print
	// failure cannot leave the index stale.
	if err := reconcileAfterEdit(ix, cfg); err != nil && !quietFlag {
		fmt.Fprintf(cmd.ErrOrStderr(), "todo edit: warning: reconcile index after write: %v\n", err)
	}

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return fmt.Errorf("todo edit: print result (file already written): %w", err)
		}
	}
	return nil
}

// validateTodoEditFlags runs every check that needs no I/O and normalizes the
// date values in place (shorthands resolved, dates re-formatted).
func validateTodoEditFlags(fields []*todoEditField) error {
	for _, f := range fields {
		if f.set && f.clear {
			return fmt.Errorf("todo edit: --%s and --no-%s conflict", f.name, f.name)
		}
		if !f.set {
			continue
		}
		if strings.TrimSpace(f.value) == "" {
			return fmt.Errorf("todo edit: --%s: empty value (use --no-%s to clear)", f.name, f.name)
		}
		switch f.name {
		case "deadline":
			d, err := parseSchedDate(f.value)
			if err != nil {
				return fmt.Errorf("todo edit: --deadline: %w", err)
			}
			f.value = d.Format("2006-01-02")
		case "scheduled":
			date, err := resolveSchedDateArg(f.value, todoNow())
			if err != nil {
				return fmt.Errorf("todo edit: --scheduled: %w", err)
			}
			f.value = date
		case "priority":
			if err := validatePriority(f.value); err != nil {
				return fmt.Errorf("todo edit: --priority: %w", err)
			}
		case "depends":
			if strings.ContainsAny(f.value, "[]\"|#\\") || strings.IndexFunc(f.value, func(r rune) bool { return !unicode.IsPrint(r) }) >= 0 {
				return fmt.Errorf("todo edit: --depends: invalid character in %q (give a bare ULID or alias, not a wikilink)", f.value)
			}
		}
	}
	return nil
}

// checkTodoDepends verifies target resolves to an existing node other than the
// todo at selfLoc and that the target does not already depend on it directly.
func checkTodoDepends(db *sql.DB, selfLoc, target string) error {
	id, _, loc, found, err := lookupNodeByRefOrAlias(db, target)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no node found matching %q (not found)", target)
	}
	if loc == selfLoc {
		return fmt.Errorf("%q is this todo itself; a todo cannot depend on itself", target)
	}
	var one int
	err = db.QueryRow(
		"SELECT 1 FROM edges WHERE src = ? AND rel IN ('depends-on','depends') AND dst_key = (SELECT id FROM nodes WHERE loc = ?) LIMIT 1",
		id, selfLoc).Scan(&one)
	if err == nil {
		return fmt.Errorf("%q already depends on this todo; this would create a dependency cycle", target)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("check dependency cycle for %q: %w", target, err)
	}
	return nil
}

// reconcileAfterEdit refreshes the index so a following read-only query sees
// the edit. It reuses ix when the command already opened one.
func reconcileAfterEdit(ix *index.Index, cfg *config.Config) error {
	if ix == nil {
		opened, err := index.Open(cfg)
		if err != nil {
			return fmt.Errorf("open index: %w", err)
		}
		defer opened.Close()
		ix = opened
	}
	if _, err := ix.Reconcile(); err != nil {
		return err
	}
	return nil
}
