// Tests for `rk note create`'s wizard dispatch: noteCreateWantsTUI's
// predicate logic, the wizard-result-map conversion functions
// (wizardNoteParams, normalizeNoteCreateParams, buildNoteLinkRows), and
// convergence between the classic flag-driven path and the wizard path
// (both must produce the same createNote call / on-disk file).
package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/config"
)

// ─────────────────────────────────────────────────────────────────────────────
// Dispatch predicate: noteCreateWantsTUI's flag-set gating.
// ─────────────────────────────────────────────────────────────────────────────

// TestNoteCreateWantsTUI_TrueOnBareInteractive: bare invocation on a
// reported real TTY must route to the wizard.
func TestNoteCreateWantsTUI_TrueOnBareInteractive(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() {
		isInteractive = prevInteractive
		resetNoteFlags(noteCreateCmd)
	})
	isInteractive = func() bool { return true }

	if !noteCreateWantsTUI(noteCreateCmd, nil) {
		t.Error("noteCreateWantsTUI(bare, TTY) = false, want true")
	}
}

// TestNoteCreateWantsTUI_FalseWhenArgsPresent: any positional arg (the
// title) routes classic regardless of TTY.
func TestNoteCreateWantsTUI_FalseWhenArgsPresent(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return true }

	if noteCreateWantsTUI(noteCreateCmd, []string{"My Title"}) {
		t.Error("noteCreateWantsTUI(args present) = true, want false")
	}
}

// TestNoteCreateWantsTUI_FalseWhenNonInteractive: a reported non-TTY routes
// classic even with zero args.
func TestNoteCreateWantsTUI_FalseWhenNonInteractive(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return false }

	if noteCreateWantsTUI(noteCreateCmd, nil) {
		t.Error("noteCreateWantsTUI(non-interactive) = true, want false")
	}
}

// TestNoteCreateWantsTUI_FalseWhenInputFlagChanged: each note-create input
// flag, individually Changed, routes classic even on a reported TTY with
// zero args.
func TestNoteCreateWantsTUI_FalseWhenInputFlagChanged(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return true }

	cases := []struct{ name, value string }{
		{"slug", "custom-slug"},
		{"type", "reference"},
		{"author", "ada"},
		{"stage", "seedling"},
		{"description", "a description"},
		{"dir", "sub"},
		{"tag", "foo"},
		{"alias", "alt-name"},
		{"body", "hello"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { resetNoteFlags(noteCreateCmd) })
			mustSetFlag(t, noteCreateCmd, tc.name, tc.value)
			if noteCreateWantsTUI(noteCreateCmd, nil) {
				t.Errorf("noteCreateWantsTUI = true with --%s Changed, want false", tc.name)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// CLI dispatch integration, at the RootCmd.Execute level.
// ─────────────────────────────────────────────────────────────────────────────

// TestNoteCreate_BareNonInteractive_NewEmptyTitleErrorNotYetReachable: bare
// invocation on a non-TTY must hit a new, non-interactivity-flavored
// "note create: title must not be empty" guard -- distinct from cobra's own
// generic arg-count message, since Args is loosened to cobra.ArbitraryArgs
// specifically so this runtime guard (not cobra) rejects the empty title.
func TestNoteCreate_BareNonInteractive_NewEmptyTitleErrorNotYetReachable(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return false }

	_, stderr, err := runNote(t, vault, "create")
	if err == nil {
		t.Fatal("expected an error for bare note create, got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "title must not be empty") {
		t.Errorf("expected the new AC §3.1 guard's \"title must not be empty\" error once Args is loosened and the guard lands, got: %v (stderr=%q)", err, stderr)
	}
}

// TestNoteCreate_TagFlagNoPositionalTitle_Errors: --tag with no positional
// title errors, via the new empty-title runtime guard now that cobra's own
// MinimumNArgs validator no longer rejects it first. Deliberately does not
// pin the exact message, since the enforcing mechanism changed.
func TestNoteCreate_TagFlagNoPositionalTitle_Errors(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return true }

	if _, _, err := runNote(t, vault, "create", "--tag", "foo"); err == nil {
		t.Fatal("expected an error for --tag with no positional title")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Pure conversion: wizardNoteParams, normalizeNoteCreateParams.
// ─────────────────────────────────────────────────────────────────────────────

// TestWizardNoteParams_TitleBodyAndLinksTokens: the raw (pre-normalize)
// conversion appends one "[[slug]]" token per selected link to the body,
// alongside carrying Title through and resolving a non-empty Author.
func TestWizardNoteParams_TitleBodyAndLinksTokens(t *testing.T) {
	results := map[string]any{
		"title": "PAS Entity Model",
		"body":  "Some notes.",
		"links": []string{"oauth-flow-patterns", "second-note"},
	}

	got := wizardNoteParams(results)

	if got.Title != "PAS Entity Model" {
		t.Errorf("Title = %q, want %q", got.Title, "PAS Entity Model")
	}
	if !strings.Contains(got.Body, "Some notes.") {
		t.Errorf("Body = %q, want it to still contain the original body text", got.Body)
	}
	if !strings.Contains(got.Body, "[[oauth-flow-patterns]]") || !strings.Contains(got.Body, "[[second-note]]") {
		t.Errorf("Body = %q, want a [[slug]] token appended for each selected link", got.Body)
	}
	if got.Author == "" {
		t.Error("Author is empty, want resolveAuthor(\"\")'s non-empty result")
	}
}

// TestNormalizeNoteCreateParams_SlugifiesTypeDefaultsAndBodyNewline: the
// load-bearing transforms both runNoteCreateE and the wizard path share
// (slugify(title) when Slug unset, Type default "note", body
// trailing-newline normalization) must be reproduced by this shared helper.
func TestNormalizeNoteCreateParams_SlugifiesTypeDefaultsAndBodyNewline(t *testing.T) {
	raw := noteCreateParams{
		Title: "My Title",
		Body:  "hello",
	}

	got, err := normalizeNoteCreateParams(raw)
	if err != nil {
		t.Fatalf("normalizeNoteCreateParams: %v", err)
	}
	if got.Slug != "my-title" {
		t.Errorf("Slug = %q, want %q (slugify(title))", got.Slug, "my-title")
	}
	if got.Type != "note" {
		t.Errorf("Type = %q, want default %q", got.Type, "note")
	}
	if got.Body != "hello\n" {
		t.Errorf("Body = %q, want trailing-newline-normalized %q", got.Body, "hello\n")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// buildNoteLinkRows.
// ─────────────────────────────────────────────────────────────────────────────

// TestBuildNoteLinkRows_ReturnsExistingNotesWithSlugProp: the row source for
// the links MultiNotePicker step must list existing notes with
// Props["slug"] set (mirrors NotePicker's own row convention, note_v1.go's
// "SELECT id, loc FROM nodes WHERE type='note'" query).
func TestBuildNoteLinkRows_ReturnsExistingNotesWithSlugProp(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runNote(t, vault, "create", "OAuth Flow Patterns", "--json")
	if err != nil {
		t.Fatalf("seed note create: %v\nstderr: %s", err, stderr)
	}
	var seedRes noteCreateResult
	mustDecodeJSON(t, out, &seedRes)
	resetCLIFlags()

	cfg, err := config.LoadWithOverrides(vault, "")
	if err != nil {
		t.Fatalf("config.LoadWithOverrides: %v", err)
	}

	rows, err := buildNoteLinkRows(cfg)
	if err != nil {
		t.Fatalf("buildNoteLinkRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want 1", rows)
	}
	if rows[0].Props["slug"] != "oauth-flow-patterns" {
		t.Errorf("rows[0].Props[slug] = %q, want %q", rows[0].Props["slug"], "oauth-flow-patterns")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Convergence: flag path vs. wizard-conversion-fed createNote.
// ─────────────────────────────────────────────────────────────────────────────

// TestNoteCreateWizard_FileConvergence_FlagVsWizard: the classic
// flag-driven path and the wizard conversion path, given the same logical
// title/body, must produce byte-identical notes/<slug>.md files modulo the
// id:/time: lines -- proving the wizard path reuses slugify(title) and the
// trailing-newline normalization, not raw values. Carries no links.
func TestNoteCreateWizard_FileConvergence_FlagVsWizard(t *testing.T) {
	// Flag path: real classic RunE, real file.
	flagVault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	out, stderr, err := runNote(t, flagVault, "create", "My Title", "--body", "some body text", "--json")
	if err != nil {
		t.Fatalf("flag-path rk note create: %v\nstderr: %s", err, stderr)
	}
	var flagRes noteCreateResult
	mustDecodeJSON(t, out, &flagRes)
	flagRaw := mustReadFile(t, filepath.Join(flagVault, flagRes.Path))
	resetCLIFlags()

	// Wizard path: synthetic result map -> wizardNoteParams ->
	// normalizeNoteCreateParams -> createNote directly (the same
	// converge-point function the classic path calls).
	results := map[string]any{
		"title": "My Title",
		"body":  "some body text",
		"links": []string{},
	}
	raw := wizardNoteParams(results)
	params, err := normalizeNoteCreateParams(raw)
	if err != nil {
		t.Fatalf("normalizeNoteCreateParams: %v", err)
	}

	wizVault, _ := setupQueryVault(t)
	wizNotesDir := filepath.Join(wizVault, "notes")
	wizRes, err := createNote(wizNotesDir, params)
	if err != nil {
		t.Fatalf("createNote (wizard path): %v", err)
	}
	wizRaw := mustReadFile(t, filepath.Join(wizVault, wizRes.Path))

	got := normalizeVolatileFrontmatter(wizRaw)
	want := normalizeVolatileFrontmatter(flagRaw)
	if got != want {
		t.Errorf("wizard-path file != flag-path file (modulo id:/time:):\nwizard: %q\nflag:   %q", got, want)
	}
}
