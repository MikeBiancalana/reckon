package cli

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	"github.com/spf13/cobra"
)

// todoAddWantsTUI reports whether a bare `rk todo add` invocation should
// open the interactive wizard instead of the classic flag-driven path:
// only on a real TTY, only with no positional args, and only when none of
// the input-affecting flags (mirrors resetTodoFlags's own list, todo.go:74)
// has been Changed. --ephemeral is deliberately included in that list --
// the wizard always produces a durable todo; --ephemeral routes classic
// unconditionally, same as every other listed flag. --no-input is
// deliberately NOT consulted here -- it must still reach
// components.PromptGuard (via RunPrompt, inside runTodoAddWizard) so a real
// TTY with --no-input errors instead of silently falling back to classic
// (mirrors todoListWantsTUI, todo_browse.go:16-18).
//
// NOT YET IMPLEMENTED: returns false unconditionally, so no bare-TTY
// dispatch test can pass yet.
func todoAddWantsTUI(cmd *cobra.Command, args []string) bool {
	return false
}

// runTodoAddWizard drives the todo-add wizard (subject -> body -> dates
// [Form, scheduled+deadline] -> depends [TaskPicker, "(no dependency)"
// prepended]) and, on completion, calls addDurableTodo with the converted
// values -- the same function the classic flag path calls.
//
// NOT YET IMPLEMENTED: not wired from todo.go's RunE yet, and this stub
// does not construct or run a real Wizard.
func runTodoAddWizard(cmd *cobra.Command) error {
	return fmt.Errorf("todo add: interactive wizard not implemented")
}

// wizardTodoAddArgs converts a completed todo-add Wizard's shared result map
// into addDurableTodo's scalar args:
//
//	body      = joinSubjectBody(results["subject"].(string), results["body"].(string))
//	scheduled = normalizeWizardDate(results["dates"].(components.FormResult).Values["scheduled"])
//	deadline  = normalizeWizardDate(results["dates"].(components.FormResult).Values["deadline"])
//	depends   = results["depends"].(string)
//
// Comma-ok assertions are required: the map is only fully populated once
// ok=true comes back from Wizard.Run.
//
// NOT YET IMPLEMENTED: returns all-zero values and a nil error unconditionally.
func wizardTodoAddArgs(results map[string]any) (body, scheduled, deadline, depends string, err error) {
	return "", "", "", "", nil
}

// normalizeWizardDate converts one Form-collected date field's raw string
// (Form stores raw strings, not time.Time) into addDurableTodo's expected
// "2006-01-02" UTC-date-only layout: blank stays blank; otherwise
// components.ParseRelativeDate(s) then result.UTC().Format("2006-01-02").
//
// NOT YET IMPLEMENTED: returns ("", nil) unconditionally.
func normalizeWizardDate(s string) (string, error) {
	return "", nil
}

// buildDependsRows opens the index, reconciles it, lists open durable todos
// (mirrors buildTodoItems/listDurableTodos, todo_browse.go:66), and maps
// them to []components.IndexRow with Props "scheduled"/"deadline" -- then
// prepends a synthetic IndexRow{ID: "", Title: "(no dependency)"} row so
// TaskPicker can represent "no dependency" as an ordinary selectable row
// rather than needing a new skip keybinding.
//
// NOT YET IMPLEMENTED: returns (nil, nil) unconditionally, so the returned
// slice never carries the prepended none-row yet.
func buildDependsRows(cfg *config.Config) ([]components.IndexRow, error) {
	return nil, nil
}
