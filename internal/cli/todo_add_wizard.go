package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	"github.com/spf13/cobra"
)

// wantsWizardTUI is the shared shape behind todoAddWantsTUI, noteCreateWantsTUI,
// and addWantsTUI: a bare invocation wants the wizard only on a real TTY,
// only with no positional args, and only when none of the verb's own
// input-affecting flags has been Changed. Each verb's wrapper supplies its
// own flag list and keeps its own doc comment -- the flag list is the part
// that carries verb-specific rationale (which flags force classic and why),
// so it stays inline at each call site rather than folded into this helper.
// --no-input is deliberately never consulted here in any wrapper -- it must
// still reach components.PromptGuard (via RunPrompt, inside the wizard
// driver) so a real TTY with --no-input errors instead of silently falling
// back to classic (mirrors todoListWantsTUI, todo_browse.go).
func wantsWizardTUI(cmd *cobra.Command, args []string, inputFlags []string) bool {
	if !isInteractive() {
		return false
	}
	if len(args) > 0 {
		return false
	}
	for _, name := range inputFlags {
		if cmd.Flags().Changed(name) {
			return false
		}
	}
	return true
}

// todoAddWantsTUI reports whether a bare `rk todo add` invocation should
// open the interactive wizard instead of the classic flag-driven path. The
// flag list mirrors resetTodoFlags's own list; --ephemeral is deliberately
// included -- the wizard always produces a durable todo, so --ephemeral
// routes classic unconditionally, same as every other listed flag.
func todoAddWantsTUI(cmd *cobra.Command, args []string) bool {
	return wantsWizardTUI(cmd, args, []string{"ephemeral", "scheduled", "deadline", "depends", "repeat", "author", "message", "edit"})
}

// runTodoAddWizard drives the todo-add wizard (subject -> body -> dates
// [Form, scheduled+deadline] -> depends [TaskPicker, "(no dependency)"
// prepended]) and, on completion, calls addDurableTodo with the converted
// values -- the same function the classic flag path calls. The depends row
// source is queried before the wizard opens (mirrors runTodoBrowse/
// buildTodoItems) so an index failure surfaces as a normal RunE error, not
// mid-flow.
func runTodoAddWizard(cmd *cobra.Command) error {
	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("todo add: load config: %w", err)
	}

	dependsRows, err := buildDependsRows(cfg)
	if err != nil {
		return err
	}

	w := components.NewWizard(
		components.Step("subject", func(prior map[string]any) components.Prompt[string] {
			tp := components.NewTextPrompt("Subject", true)
			tp.Show()
			return tp
		}),
		components.Step("body", func(prior map[string]any) components.Prompt[string] {
			te := components.NewTextEditor("Body")
			te.Show()
			return te
		}),
		components.Step("dates", func(prior map[string]any) components.Prompt[components.FormResult] {
			f := components.NewForm("Dates")
			f.AddField(components.FormField{Label: "Scheduled", Key: "scheduled", Type: components.FieldTypeDate, Required: false})
			f.AddField(components.FormField{Label: "Deadline", Key: "deadline", Type: components.FieldTypeDate, Required: false})
			f.Show()
			return f
		}),
		components.Step("depends", func(prior map[string]any) components.Prompt[string] {
			tpk := components.NewTaskPicker("Depends on")
			tpk.Show(dependsRows)
			return tpk
		}),
	)

	results, ok, err := w.Run()
	if err != nil {
		return fmt.Errorf("todo add: %w", err)
	}
	if !ok {
		return nil
	}

	body, scheduled, deadline, depends, err := wizardTodoAddArgs(results)
	if err != nil {
		return fmt.Errorf("todo add: %w", err)
	}

	todosDir := filepath.Join(cfg.VaultDir, "todos")
	if err := os.MkdirAll(todosDir, 0o755); err != nil {
		return fmt.Errorf("todo add: create todos dir: %w", err)
	}

	res, err := addDurableTodo(todosDir, resolveAuthor(""), body, scheduled, deadline, depends, "")
	if err != nil {
		return err
	}

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
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

// wizardTodoAddArgs converts a completed todo-add Wizard's shared result map
// into addDurableTodo's scalar args:
//
//	body      = joinSubjectBody(results["subject"].(string), results["body"].(string))
//	scheduled = normalizeWizardDate(results["dates"].(components.FormResult).Values["scheduled"])
//	deadline  = normalizeWizardDate(results["dates"].(components.FormResult).Values["deadline"])
//	depends   = results["depends"].(string)
//
// Comma-ok assertions are used throughout: the map is only fully populated
// once ok=true comes back from Wizard.Run, and a comma-ok miss degrades to
// the type's zero value instead of a panic.
func wizardTodoAddArgs(results map[string]any) (body, scheduled, deadline, depends string, err error) {
	subject, _ := results["subject"].(string)
	bodyStep, _ := results["body"].(string)
	body = joinSubjectBody(subject, bodyStep)

	dates, _ := results["dates"].(components.FormResult)
	scheduled, err = normalizeWizardDate(dates.Values["scheduled"])
	if err != nil {
		return "", "", "", "", err
	}
	deadline, err = normalizeWizardDate(dates.Values["deadline"])
	if err != nil {
		return "", "", "", "", err
	}

	depends, _ = results["depends"].(string)
	return body, scheduled, deadline, depends, nil
}

// normalizeWizardDate converts one Form-collected date field's raw string
// (Form stores raw strings, not time.Time) into addDurableTodo's expected
// "2006-01-02" UTC-date-only layout: blank stays blank; otherwise
// components.ParseRelativeDate(s) then result.UTC().Format("2006-01-02").
func normalizeWizardDate(s string) (string, error) {
	if s == "" {
		return "", nil
	}
	t, err := components.ParseRelativeDate(s)
	if err != nil {
		return "", err
	}
	return t.UTC().Format("2006-01-02"), nil
}

// buildDependsRows opens the index, reconciles it, lists open durable todos
// (mirrors buildTodoItems/listDurableTodos in todo_browse.go), and maps
// them to []components.IndexRow with Props "scheduled"/"deadline" -- then
// prepends a synthetic IndexRow{ID: "", Title: "(no dependency)"} row so
// TaskPicker can represent "no dependency" as an ordinary selectable row
// rather than needing a new skip keybinding.
func buildDependsRows(cfg *config.Config) ([]components.IndexRow, error) {
	ix, err := index.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("todo add: open index: %w", err)
	}
	defer ix.Close()

	if _, err := ix.Reconcile(); err != nil {
		return nil, fmt.Errorf("todo add: reconcile index: %w", err)
	}

	items, err := listDurableTodos(ix.DB(), false, "")
	if err != nil {
		return nil, err
	}

	rows := make([]components.IndexRow, 0, len(items)+1)
	rows = append(rows, components.IndexRow{ID: "", Title: "(no dependency)"})
	for _, it := range items {
		rows = append(rows, components.IndexRow{
			ID:    it.ID,
			Title: it.displayTitle(),
			Props: map[string]string{"scheduled": it.Scheduled, "deadline": it.Deadline},
		})
	}
	return rows, nil
}
