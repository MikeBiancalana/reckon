package cli

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	"github.com/spf13/cobra"
)

// noteCreateWantsTUI reports whether a bare `rk note create` invocation
// should open the interactive wizard instead of the classic flag-driven
// path: only on a real TTY, only with no positional args, and only when
// none of the note-create input flags (--slug/--type/--author/--stage/
// --description/--dir/--tag/--alias/--body) has been Changed. --no-input is
// deliberately NOT consulted here -- see todoAddWantsTUI's doc comment for
// the identical rationale.
//
// NOT YET IMPLEMENTED: returns false unconditionally.
func noteCreateWantsTUI(cmd *cobra.Command, args []string) bool {
	return false
}

// runNoteCreateWizard drives the note-create wizard (title -> body ->
// links [MultiNotePicker over existing notes]) and, on completion, appends
// "[[slug]]" tokens (one per selected link, own paragraph) to the body,
// builds a raw noteCreateParams{Title, Body, Author: resolveAuthor("")},
// normalizes it via normalizeNoteCreateParams, and calls createNote -- the
// same function the classic flag path calls.
//
// NOT YET IMPLEMENTED: not wired from note_v1.go's RunE yet, and this stub
// does not construct or run a real Wizard.
func runNoteCreateWizard(cmd *cobra.Command) error {
	return fmt.Errorf("note create: interactive wizard not implemented")
}

// wizardNoteParams converts a completed note-create Wizard's shared result
// map into a raw noteCreateParams (Title/Body/Author only -- Slug/Type/
// Stage/etc are still unset; normalizeNoteCreateParams fills those in):
// Body is results["body"].(string) with one "[[slug]]" paragraph appended
// per results["links"].([]string) entry; Title is results["title"].(string).
//
// NOT YET IMPLEMENTED: returns a zero-value noteCreateParams unconditionally.
func wizardNoteParams(results map[string]any) noteCreateParams {
	return noteCreateParams{}
}

// normalizeNoteCreateParams performs the load-bearing transforms shared by
// the flag path (runNoteCreateE, note_v1.go:240-259) and the wizard
// conversion path: slug = slugify(title) when p.Slug is unset, validateSlug,
// Type default "note", stage validation (when non-empty), and body
// trailing-newline normalization
// (body += "\n" if body != "" && !strings.HasSuffix(body, "\n")).
//
// runNoteCreateE itself must be wired to call this helper too (that edit
// belongs to note_v1.go's dispatch-wiring, not this stub) so both paths
// provably converge on one implementation rather than two
// independently-maintained copies.
//
// NOT YET IMPLEMENTED: returns the input unchanged with a nil error.
func normalizeNoteCreateParams(p noteCreateParams) (noteCreateParams, error) {
	return p, nil
}

// buildNoteLinkRows opens the index, reconciles it, lists existing notes
// (mirrors buildTodoItems's shape; query is "SELECT id, loc FROM nodes WHERE
// type='note'", note_v1.go:742), and maps them to []components.IndexRow with
// Props["slug"] set (mirrors NotePicker's own row convention).
//
// NOT YET IMPLEMENTED: returns (nil, nil) unconditionally.
func buildNoteLinkRows(cfg *config.Config) ([]components.IndexRow, error) {
	return nil, nil
}
