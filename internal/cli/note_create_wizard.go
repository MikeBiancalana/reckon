package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	"github.com/spf13/cobra"
)

// noteCreateWantsTUI reports whether a bare `rk note create` invocation
// should open the interactive wizard instead of the classic flag-driven
// path. See wantsWizardTUI (todo_add_wizard.go) for the shared shape and
// the --no-input rationale; the flags below are note-create's own input
// flags (--slug/--type/--author/--stage/--description/--dir/--tag/--alias/
// --body).
func noteCreateWantsTUI(cmd *cobra.Command, args []string) bool {
	return wantsWizardTUI(cmd, args, []string{"slug", "type", "author", "stage", "description", "dir", "tag", "alias", "body"})
}

// runNoteCreateWizard drives the note-create wizard (title -> body ->
// links [MultiNotePicker over existing notes]) and, on completion, converts
// the result through wizardNoteParams -> normalizeNoteCreateParams -> the
// same createNote the classic flag path calls. The links row source is
// queried before the wizard opens (mirrors runTodoAddWizard/buildDependsRows)
// so an index failure surfaces as a normal RunE error, not mid-flow.
func runNoteCreateWizard(cmd *cobra.Command) error {
	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("note create: load config: %w", err)
	}

	linkRows, err := buildNoteLinkRows(cfg)
	if err != nil {
		return err
	}

	w := components.NewWizard(
		components.Step("title", func(prior map[string]any) components.Prompt[string] {
			tp := components.NewTextPrompt("Title", true)
			tp.Show()
			return tp
		}),
		components.Step("body", func(prior map[string]any) components.Prompt[string] {
			te := components.NewTextEditor("Body")
			te.Show()
			return te
		}),
		components.Step("links", func(prior map[string]any) components.Prompt[[]string] {
			mp := components.NewMultiNotePicker("Link notes")
			mp.Show(linkRows)
			return mp
		}),
	)

	results, ok, err := w.Run()
	if err != nil {
		return fmt.Errorf("note create: %w", err)
	}
	if !ok {
		return nil
	}

	params, err := normalizeNoteCreateParams(wizardNoteParams(results))
	if err != nil {
		return err
	}

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return fmt.Errorf("note create: %w", err)
	}

	notesDir := filepath.Join(cfg.VaultDir, "notes")
	res, err := createNote(notesDir, params)
	if err != nil {
		return err
	}

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return fmt.Errorf("print result: %w", err)
		}
	}
	return nil
}

// wizardNoteParams converts a completed note-create Wizard's shared result
// map into a raw noteCreateParams (Title/Body/Author only -- Slug/Type/
// Stage/etc are still unset; normalizeNoteCreateParams fills those in):
// Body is results["body"].(string) with one "[[slug]]" paragraph appended
// per results["links"].([]string) entry; Title is results["title"].(string).
func wizardNoteParams(results map[string]any) noteCreateParams {
	title, _ := results["title"].(string)
	body, _ := results["body"].(string)
	links, _ := results["links"].([]string)

	for _, slug := range links {
		if body != "" {
			body += "\n\n"
		}
		body += "[[" + slug + "]]"
	}

	return noteCreateParams{
		Title:  title,
		Body:   body,
		Author: resolveAuthor(""),
	}
}

// buildNoteLinkRows opens the index, reconciles it, lists existing notes
// (mirrors buildTodoItems's shape; query is "SELECT id, loc FROM nodes WHERE
// type='note'", matching note_v1.go's own note-listing query), and maps
// them to []components.IndexRow with Props["slug"] set (mirrors NotePicker's
// own row convention).
func buildNoteLinkRows(cfg *config.Config) ([]components.IndexRow, error) {
	ix, err := index.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("note create: open index: %w", err)
	}
	defer ix.Close()

	if _, err := ix.Reconcile(); err != nil {
		return nil, fmt.Errorf("note create: reconcile index: %w", err)
	}

	db := ix.DB()
	rows, err := db.Query("SELECT id, loc FROM nodes WHERE type = 'note'")
	if err != nil {
		return nil, fmt.Errorf("note create: query notes: %w", err)
	}
	type noteRow struct{ id, loc string }
	var noteRows []noteRow
	for rows.Next() {
		var r noteRow
		if err := rows.Scan(&r.id, &r.loc); err != nil {
			rows.Close()
			return nil, fmt.Errorf("note create: scan note: %w", err)
		}
		noteRows = append(noteRows, r)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("note create: iterate notes: %w", err)
	}
	rows.Close()

	var out []components.IndexRow
	for _, r := range noteRows {
		slug := strings.TrimSuffix(filepath.Base(r.loc), ".md")
		if slug == "index" {
			continue
		}
		props, err := loadProps(db, r.id)
		if err != nil {
			return nil, fmt.Errorf("note create: %w", err)
		}
		title := props["title"]
		if title == "" {
			title = slug
		}
		out = append(out, components.IndexRow{
			ID:    r.id,
			Title: title,
			Type:  "note",
			Props: map[string]string{"slug": slug},
		})
	}
	return out, nil
}
