package cli

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/checklist"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	"github.com/spf13/cobra"
)

// checklistRunCmd opens an interactive mini-TUI over a template's active
// run (starting one if none exists), resuming on later invocations until it
// completes. TUI-only: non-TTY and --no-input are refused by the shared
// components.PromptGuard (interactive.go), not a bespoke check here.
var checklistRunCmd = &cobra.Command{
	Use:          "run <template>",
	Short:        "Interactively step through a template's active run",
	SilenceUsage: true,
	Args:         cobra.ExactArgs(1),
	RunE:         runChecklistRunE,
}

func runChecklistRunE(cmd *cobra.Command, args []string) error {
	defer resetChecklistFlags(cmd)

	name := args[0]

	_, svc, db, err := setupChecklistRun()
	if err != nil {
		return err
	}
	defer db.Close()

	run, _, err := resolveChecklistRun(svc, name)
	if err != nil {
		return fmt.Errorf("checklist run: %w", err)
	}

	runner := components.NewChecklistRunner(run.TemplateName)
	runner.Show(runItemsToChecklistItems(run.Items), makeToggleFunc(svc, run.ID))

	if _, _, err := components.RunPrompt[[]components.ChecklistItem](runner); err != nil {
		return err
	}
	if e := runner.Err(); e != nil {
		return fmt.Errorf("checklist run: %w", e)
	}
	// ok=false (the user quit) is not an error: print nothing.
	return nil
}

// runItemsToChecklistItems converts a run's persisted items into the
// display-only rows ChecklistRunner renders, dropping every field the TUI
// component doesn't need (ID, Position, CheckedAt, ...) and preserving
// order (position is implicit in slice index).
func runItemsToChecklistItems(items []checklist.RunItem) []components.ChecklistItem {
	out := make([]components.ChecklistItem, len(items))
	for i, item := range items {
		out[i] = components.ChecklistItem{Text: item.Text, Checked: item.Checked}
	}
	return out
}

// makeToggleFunc closes over svc and the immutable run ID (not the mutable
// *Run) so there is no staleness across the session's toggles. It replays
// runChecklistCheckE's CheckItem-then-GetRunStatus sequence, so `run` and
// `check` write through the same path.
func makeToggleFunc(svc *checklist.Service, runID string) components.ToggleFunc {
	return func(position int) ([]components.ChecklistItem, bool, error) {
		if err := svc.CheckItem(runID, position); err != nil {
			return nil, false, err
		}
		// Re-fetch by run ID, not GetActiveRun: the toggle that completes
		// the run would otherwise make GetActiveRun error not-found.
		updated, err := svc.GetRunStatus(runID)
		if err != nil {
			return nil, false, err
		}
		return runItemsToChecklistItems(updated.Items), updated.Status == checklist.RunStatusCompleted, nil
	}
}
