package cli

import (
	"fmt"
	"strconv"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	"github.com/spf13/cobra"
)

// todoListWantsTUI reports whether a bare `rk todo list` invocation should
// open the interactive mini-TUI instead of the classic text-output path:
// only on a real TTY, and only when no output-shaping flag was passed.
// --no-input is deliberately not consulted here -- it must still reach
// components.PromptGuard (via RunPrompt, inside runTodoBrowse) so a real TTY
// with --no-input errors instead of silently falling back to classic output.
func todoListWantsTUI(cmd *cobra.Command) bool {
	if !isInteractive() {
		return false
	}
	for _, name := range []string{"all", "state", "durable", "ephemeral"} {
		if cmd.Flags().Changed(name) {
			return false
		}
	}
	if jsonFlag || ndjsonFlag {
		return false
	}
	return true
}

// runTodoBrowse opens the mini-TUI over today's open todos. It does not
// re-defer resetTodoFlags -- runTodoListE's defer already covers it.
func runTodoBrowse(cmd *cobra.Command) error {
	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("todo list: load config: %w", err)
	}

	items, err := buildTodoItems(cfg)
	if err != nil {
		return err
	}

	browser := components.NewTodoBrowser("Todos")
	browser.Show(items, makeMarkDoneFunc(cfg.VaultDir, items))

	if _, _, err := components.RunPrompt[[]components.TodoItem](browser); err != nil {
		return fmt.Errorf("todo list: %w", err)
	}
	if e := browser.Err(); e != nil {
		return fmt.Errorf("todo list: %w", e)
	}
	// ok=false (the user quit) is not an error: print nothing. Every
	// mark-done already persisted per-keypress.
	return nil
}

// buildTodoItems opens the index, reconciles it, lists today's open durable
// and ephemeral todos (matching the classic path's default filters), maps
// them to the display type, and closes the index before returning -- the
// write path (doneDurableTodo/doneEphemeralTodo) takes vaultDir directly and
// bypasses the index, so no handle is held across the interactive session.
func buildTodoItems(cfg *config.Config) ([]components.TodoItem, error) {
	ix, err := index.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("todo list: open index: %w", err)
	}
	defer ix.Close()

	if _, err := ix.Reconcile(); err != nil {
		return nil, fmt.Errorf("todo list: reconcile index: %w", err)
	}

	durItems, err := listDurableTodos(ix.DB(), false, "")
	if err != nil {
		return nil, err
	}
	ephItems, err := listEphemeralTodos(ix.DB(), false)
	if err != nil {
		return nil, err
	}

	items := make([]components.TodoItem, 0, len(durItems)+len(ephItems))
	for _, it := range durItems {
		items = append(items, components.TodoItem{
			Kind:  "durable",
			Ref:   it.ID,
			Title: it.displayTitle(),
			Done:  it.State == "done",
		})
	}
	for _, it := range ephItems {
		title := it.Body
		if title == "" {
			title = "(empty item)"
		}
		items = append(items, components.TodoItem{
			Kind:  "ephemeral",
			Ref:   strconv.Itoa(it.Line),
			Title: title,
			Done:  it.Checked,
		})
	}
	return items, nil
}

// makeMarkDoneFunc returns the dispatch closure the TUI calls on every
// mark-done keypress. It captures vaultDir and its own mutable copy of the
// session's items, kept in lockstep with the component (which adopts
// whatever this closure returns).
//
// Every non-error result is treated uniformly as "remove from session",
// including the recurring-todo branch of doneDurableTodo (state stays
// "open", scheduled advances): leaving a repeat: item visible after one
// mark-done would let a second keypress advance its cursor again within the
// same session. It reappears, correctly, on the next `rk todo list`.
//
// Refresh strategy is local mutation with no mid-session re-query:
// flipChecklistLine only ever flips a single byte in place and never
// removes a line, so every other ephemeral item's captured Ref (a 1-based
// file-order index) stays valid for the whole session; durable ULIDs are
// immutable. A recurring completion that materializes a new inbox.md
// pile-up item won't appear live -- it surfaces on the next `rk todo list`.
func makeMarkDoneFunc(vaultDir string, items []components.TodoItem) components.MarkDoneFunc {
	session := append([]components.TodoItem(nil), items...)
	return func(pos int) ([]components.TodoItem, error) {
		it := session[pos]
		var err error
		if it.Kind == "ephemeral" {
			_, err = doneEphemeralTodo(vaultDir, it.Ref)
		} else {
			_, err = doneDurableTodo(vaultDir, it.Ref)
		}
		if err != nil {
			return nil, err
		}
		session = append(session[:pos], session[pos+1:]...)
		return append([]components.TodoItem(nil), session...), nil
	}
}
