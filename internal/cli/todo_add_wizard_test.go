// Tests for `rk todo add`'s wizard dispatch: todoAddWantsTUI's predicate
// logic, the wizard-result-map conversion functions (wizardTodoAddArgs,
// normalizeWizardDate, buildDependsRows), and convergence between the
// classic flag-driven path and the wizard path (both must produce the same
// addDurableTodo call / on-disk file).
package cli

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────────────────────────────────────
// Shared test helpers (defined once here, reused by note_create_wizard_test.go
// and add_wizard_test.go -- same package, do not redefine).
// ─────────────────────────────────────────────────────────────────────────────

// mustSetFlag sets a cobra flag by name (both its value and pflag's Changed
// bit), fataling on an unknown flag name or an invalid value for its type.
func mustSetFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("Flags().Set(%q, %q): %v", name, value, err)
	}
}

// normalizeVolatileFrontmatter blanks the id: and time: frontmatter lines
// node.Render produces, so two independently-created files (one per
// convergence test's flag-path/wizard-path pair) can be compared for
// byte-identity modulo the fields that are inherently unique per write: a
// fresh ULID (createNote has no mint seam at all -- unlike todo add's
// mintTodoULID) and a fresh time.Now().UTC() timestamp (neither
// addDurableTodo nor createNote has a clock seam, so two invocations
// crossing a one-second boundary would otherwise differ by exactly that
// line even when every other byte matches).
func normalizeVolatileFrontmatter(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		switch {
		case strings.HasPrefix(line, "id: "):
			lines[i] = "id: <normalized>"
		case strings.HasPrefix(line, "time: "):
			lines[i] = "time: <normalized>"
		}
	}
	return strings.Join(lines, "\n")
}

// runWizardForTest drives a Wizard.Run-shaped call in a goroutine bounded by
// a generous timeout, mirroring internal/tui/components/prompt_test.go's
// runPromptForTest (unexported there, so it can't be imported -- this is the
// cli-package-local twin). Without this bound, a component whose Update
// never reaches a terminal state on a given scripted key sequence would
// hang go test's input loop indefinitely instead of failing fast.
func runWizardForTest(t *testing.T, fn func() (map[string]any, bool, error)) (map[string]any, bool, error) {
	t.Helper()

	type res struct {
		val map[string]any
		ok  bool
		err error
	}
	done := make(chan res, 1)
	go func() {
		val, ok, err := fn()
		done <- res{val, ok, err}
	}()

	select {
	case r := <-done:
		return r.val, r.ok, r.err
	case <-time.After(5 * time.Second):
		t.Fatal("wizard run did not complete within timeout")
		return nil, false, nil
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Dispatch predicate: todoAddWantsTUI's flag-set gating.
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoAddWantsTUI_TrueOnBareInteractive: bare invocation (no args, no
// flags) on a reported real TTY must route to the wizard.
func TestTodoAddWantsTUI_TrueOnBareInteractive(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() {
		isInteractive = prevInteractive
		resetTodoFlags(todoAddCmd)
	})
	isInteractive = func() bool { return true }

	if !todoAddWantsTUI(todoAddCmd, nil) {
		t.Error("todoAddWantsTUI(bare, TTY) = false, want true")
	}
}

// TestTodoAddWantsTUI_FalseWhenArgsPresent: any positional arg routes
// classic regardless of TTY.
func TestTodoAddWantsTUI_FalseWhenArgsPresent(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return true }

	if todoAddWantsTUI(todoAddCmd, []string{"buy", "milk"}) {
		t.Error("todoAddWantsTUI(args present) = true, want false")
	}
}

// TestTodoAddWantsTUI_FalseWhenNonInteractive: a reported non-TTY routes
// classic even with zero args.
func TestTodoAddWantsTUI_FalseWhenNonInteractive(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return false }

	if todoAddWantsTUI(todoAddCmd, nil) {
		t.Error("todoAddWantsTUI(non-interactive) = true, want false")
	}
}

// TestTodoAddWantsTUI_FalseWhenInputFlagChanged: each of resetTodoFlags's
// own input-affecting flag names, individually Changed, routes classic even
// on a reported TTY with zero args -- --ephemeral included, since the
// wizard always produces a durable todo.
func TestTodoAddWantsTUI_FalseWhenInputFlagChanged(t *testing.T) {
	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return true }

	cases := []struct{ name, value string }{
		{"ephemeral", "true"},
		{"scheduled", "2026-08-15"},
		{"deadline", "2026-08-15"},
		{"depends", "01ARZ3NDEKTSV4RRFFQ69G5FAV"},
		{"repeat", "+7d"},
		{"author", "ada"},
		{"message", "hello"},
		{"edit", "true"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Cleanup(func() { resetTodoFlags(todoAddCmd) })
			mustSetFlag(t, todoAddCmd, tc.name, tc.value)
			if todoAddWantsTUI(todoAddCmd, nil) {
				t.Errorf("todoAddWantsTUI = true with --%s Changed, want false", tc.name)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// CLI dispatch integration, at the RootCmd.Execute level.
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoAdd_ArgPresentStaysClassicNoANSI: an arg always routes classic,
// never opening the TUI.
func TestTodoAdd_ArgPresentStaysClassicNoANSI(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return true }

	out, stderr, err := runTodo(t, vault, "add", "buy milk")
	if err != nil {
		t.Fatalf("rk todo add buy milk: %v\nstderr: %s", err, stderr)
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("arg-present invocation must never open the TUI, got %q", out)
	}
}

// TestTodoAdd_NonInteractiveBareErrorsEmptyBody: bare + non-TTY falls into
// the classic body's existing "empty body text" error, not a wizard.
func TestTodoAdd_NonInteractiveBareErrorsEmptyBody(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return false }

	_, stderr, err := runTodo(t, vault, "add")
	if err == nil {
		t.Fatal("expected an error for bare non-interactive todo add, got nil")
	}
	if !strings.Contains(err.Error(), "empty body text") {
		t.Errorf("expected \"empty body text\", got: %v (stderr=%q)", err, stderr)
	}
}

// TestTodoAdd_InteractiveNoInputReachesPromptGuard: a reported TTY with
// --no-input must error via components.PromptGuard's message, proving the
// guard (inside RunPrompt/Wizard.Run) is reached rather than bypassed.
func TestTodoAdd_InteractiveNoInputReachesPromptGuard(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	prevInteractive := isInteractive
	prevNoInput := noInputFlag
	t.Cleanup(func() {
		isInteractive = prevInteractive
		noInputFlag = prevNoInput
	})
	isInteractive = func() bool { return true }

	_, stderr, err := runTodo(t, vault, "add", "--no-input")
	if err == nil {
		t.Fatal("expected an error opening the wizard on a reported TTY with --no-input, got nil")
	}
	combined := strings.ToLower(err.Error() + stderr)
	if !strings.Contains(combined, "--no-input") && !strings.Contains(combined, "terminal") {
		t.Errorf("expected the guard's --no-input/terminal error once dispatch is wired, got err=%v stderr=%q", err, stderr)
	}
}

// TestTodoAdd_EphemeralFlagStaysClassic: --ephemeral routes to
// addEphemeralTodo -- the wizard never produces an ephemeral todo.
func TestTodoAdd_EphemeralFlagStaysClassic(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return true }

	out, stderr, err := runTodo(t, vault, "add", "call dentist", "--ephemeral", "--json")
	if err != nil {
		t.Fatalf("rk todo add --ephemeral: %v\nstderr: %s", err, stderr)
	}
	var res todoAddResult
	mustDecodeJSON(t, out, &res)
	if res.Kind != "ephemeral" {
		t.Errorf("Kind = %q, want ephemeral (proves --ephemeral never reaches the wizard)", res.Kind)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Pure conversion: wizardTodoAddArgs, normalizeWizardDate.
// ─────────────────────────────────────────────────────────────────────────────

// TestWizardTodoAddArgs_SubjectAndBodyJoin: the synthetic result map uses
// the wizard's actual 4-key result shape, with "dates" as a FormResult
// (scheduled+deadline collapsed into one Form step, not two flat string
// keys).
func TestWizardTodoAddArgs_SubjectAndBodyJoin(t *testing.T) {
	results := map[string]any{
		"subject": "Buy milk",
		"body":    "at the store",
		"dates":   components.FormResult{Values: map[string]string{"scheduled": "", "deadline": ""}},
		"depends": "",
	}

	body, scheduled, deadline, depends, err := wizardTodoAddArgs(results)
	if err != nil {
		t.Fatalf("wizardTodoAddArgs: %v", err)
	}
	if want := "Buy milk\n\nat the store"; body != want {
		t.Errorf("body = %q, want %q", body, want)
	}
	if scheduled != "" || deadline != "" || depends != "" {
		t.Errorf("scheduled/deadline/depends = %q/%q/%q, want all empty", scheduled, deadline, depends)
	}
}

// TestWizardTodoAddArgs_EmptyBodyNoSeparator: an empty body step leaves the
// subject alone -- no dangling "\n\n" separator.
func TestWizardTodoAddArgs_EmptyBodyNoSeparator(t *testing.T) {
	results := map[string]any{
		"subject": "Buy milk",
		"body":    "",
		"dates":   components.FormResult{Values: map[string]string{"scheduled": "", "deadline": ""}},
		"depends": "",
	}

	body, _, _, _, err := wizardTodoAddArgs(results)
	if err != nil {
		t.Fatalf("wizardTodoAddArgs: %v", err)
	}
	if body != "Buy milk" {
		t.Errorf("body = %q, want %q (no separator when body step is empty)", body, "Buy milk")
	}
}

// TestJoinSubjectBody_MatchesAssembleBodyMessageJoin: joinSubjectBody
// (wizard-only, subject+body given as two separate strings) must produce
// the exact same bytes as assembleBody's -m path (subject+body given as two
// -m values) for the same logical content -- the convergence proof needed
// since no PTY end-to-end test is possible.
func TestJoinSubjectBody_MatchesAssembleBodyMessageJoin(t *testing.T) {
	got := joinSubjectBody("Buy milk", "at the store")

	want, err := assembleBody(&cobra.Command{}, nil, []string{"Buy milk", "at the store"}, false, true)
	if err != nil {
		t.Fatalf("assembleBody: %v", err)
	}
	if got != want {
		t.Errorf("joinSubjectBody(...) = %q, assembleBody(-m path) = %q, want byte-identical", got, want)
	}
}

// TestNormalizeWizardDate_BlankStaysBlank: an empty Form field value must
// normalize to "", not error and not a formatted zero-date.
func TestNormalizeWizardDate_BlankStaysBlank(t *testing.T) {
	got, err := normalizeWizardDate("")
	if err != nil {
		t.Fatalf("normalizeWizardDate(\"\"): %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

// TestNormalizeWizardDate_ValidInputFormatsAsDateOnly: a populated date
// input must format as a bare "2006-01-02" string (no time-of-day
// component), distinct from the empty-field case above.
//
// This intentionally does NOT assert a specific calendar day under a
// simulated non-UTC clock: ParseRelativeDate resolves relative dates in
// local time before normalizeWizardDate reformats via .UTC(), which can
// shift the calendar day near a local-midnight boundary in positive-UTC-offset
// zones -- an open, documented risk with more than one valid resolution, so
// pinning a specific day here would encode a stance not yet decided. The
// input itself is computed relative to "now" (not a hardcoded literal),
// mirroring internal/tui/components/date_picker_test.go's futureDate()
// convention, so this test is not a time bomb.
func TestNormalizeWizardDate_ValidInputFormatsAsDateOnly(t *testing.T) {
	input := time.Now().AddDate(0, 0, 30).Format("2006-01-02")

	got, err := normalizeWizardDate(input)
	if err != nil {
		t.Fatalf("normalizeWizardDate(%q): %v", input, err)
	}
	if got == "" {
		t.Fatalf("normalizeWizardDate(%q) = empty, want a formatted date", input)
	}
	if len(got) != len("2006-01-02") || strings.ContainsAny(got, "TZ:") {
		t.Errorf("normalizeWizardDate(%q) = %q, want a bare YYYY-MM-DD string", input, got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// buildDependsRows: "(no dependency)" row selection yields empty depends.
// ─────────────────────────────────────────────────────────────────────────────

// TestBuildDependsRows_PrependsNoDependencyRow: the row source for the
// depends TaskPicker step must prepend a synthetic
// IndexRow{ID:"", Title:"(no dependency)"} ahead of the real open durable
// todos -- selecting it is how the wizard user finishes with no dependency,
// without any new component-level skip affordance.
func TestBuildDependsRows_PrependsNoDependencyRow(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	writeTestNode(t, vault, "todos/"+id+".md", id, "todo", "Ship it.", "state: open")

	cfg, err := config.LoadWithOverrides(vault, "")
	if err != nil {
		t.Fatalf("config.LoadWithOverrides: %v", err)
	}

	rows, err := buildDependsRows(cfg)
	if err != nil {
		t.Fatalf("buildDependsRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2 (the synthetic none-row + one open durable todo)", rows)
	}
	if rows[0].ID != "" || rows[0].Title != "(no dependency)" {
		t.Errorf("rows[0] = %+v, want the prepended {ID:\"\", Title:\"(no dependency)\"} sentinel row", rows[0])
	}
	if rows[1].ID != id {
		t.Errorf("rows[1].ID = %q, want %q", rows[1].ID, id)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Convergence: flag path vs. wizard-conversion-fed addDurableTodo.
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoAddWizard_FileConvergence_FlagVsWizard: the classic flag-driven
// path and the wizard conversion path, given the same logical
// subject/body/scheduled/deadline, must produce byte-identical
// todos/<ULID>.md files modulo the id:/time: lines (which are inherently
// unique per write -- see normalizeVolatileFrontmatter's doc comment).
func TestTodoAddWizard_FileConvergence_FlagVsWizard(t *testing.T) {
	scheduled := time.Now().AddDate(0, 0, 30).Format("2006-01-02")
	deadline := time.Now().AddDate(0, 0, 45).Format("2006-01-02")

	// Flag path: real classic RunE, real file.
	flagVault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	out, stderr, err := runTodo(t, flagVault, "add",
		"-m", "Buy milk", "-m", "at the store",
		"--scheduled", scheduled, "--deadline", deadline,
		"--json")
	if err != nil {
		t.Fatalf("flag-path rk todo add: %v\nstderr: %s", err, stderr)
	}
	var flagRes todoAddResult
	mustDecodeJSON(t, out, &flagRes)
	flagRaw := mustReadFile(t, filepath.Join(flagVault, flagRes.Path))
	resetCLIFlags()

	// Wizard path: synthetic result map -> wizardTodoAddArgs -> addDurableTodo
	// directly (the same converge-point function the classic path calls).
	results := map[string]any{
		"subject": "Buy milk",
		"body":    "at the store",
		"dates":   components.FormResult{Values: map[string]string{"scheduled": scheduled, "deadline": deadline}},
		"depends": "",
	}
	body, wizScheduled, wizDeadline, wizDepends, err := wizardTodoAddArgs(results)
	if err != nil {
		t.Fatalf("wizardTodoAddArgs: %v", err)
	}

	wizVault, _ := setupQueryVault(t)
	wizTodosDir := filepath.Join(wizVault, "todos")
	if err := os.MkdirAll(wizTodosDir, 0o755); err != nil {
		t.Fatalf("mkdir wizard todos dir: %v", err)
	}
	wizRes, err := addDurableTodo(wizTodosDir, resolveAuthor(""), body, wizScheduled, wizDeadline, wizDepends, "")
	if err != nil {
		t.Fatalf("addDurableTodo (wizard path): %v", err)
	}
	wizRaw := mustReadFile(t, filepath.Join(wizVault, wizRes.Path))

	got := normalizeVolatileFrontmatter(wizRaw)
	want := normalizeVolatileFrontmatter(flagRaw)
	if got != want {
		t.Errorf("wizard-path file != flag-path file (modulo id:/time:):\nwizard: %q\nflag:   %q", got, want)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Component composition.
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoAddWizard_FullFlowResultShape composes the actual 4-step chain
// runTodoAddWizard drives (TextPrompt subject -> TextEditor body -> Form
// dates[scheduled,deadline] -> TaskPicker depends) via
// components.NewWizard/Step directly, mirroring
// internal/tui/components/wizard_test.go's own ad hoc compositions, and
// asserts the resulting map carries exactly the 4 keys the contract names --
// "dates" typed as components.FormResult, not two flat "scheduled"/
// "deadline" string keys.
func TestTodoAddWizard_FullFlowResultShape(t *testing.T) {
	// This test exercises component composition, not the TTY guard -- stub
	// isInteractive true so components.PromptGuard (wired from this
	// package's own interactive.go init()) doesn't short-circuit RunPrompt
	// before the Wizard ever mounts its first step.
	prevInteractive := isInteractive
	t.Cleanup(func() { isInteractive = prevInteractive })
	isInteractive = func() bool { return true }

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
			tpk.Show([]components.IndexRow{{ID: "", Title: "(no dependency)"}})
			return tpk
		}),
	)

	var keys bytes.Buffer
	keys.WriteString("Buy milk")
	keys.WriteByte('\r') // submit subject
	keys.WriteString("at the store")
	keys.WriteByte(0x04) // Ctrl+D submits body
	keys.WriteByte('\r') // blank scheduled+deadline Form submit
	keys.WriteByte('\r') // select the "(no dependency)" row (cursor starts at index 0)

	result, ok, err := runWizardForTest(t, func() (map[string]any, bool, error) {
		return w.Run(tea.WithInput(bytes.NewReader(keys.Bytes())), tea.WithOutput(io.Discard))
	})

	if err != nil {
		t.Fatalf("Wizard.Run: %v", err)
	}
	if !ok {
		t.Fatal("expected the wizard to complete (ok=true)")
	}
	if len(result) != 4 {
		t.Fatalf("result = %+v, want exactly 4 keys (subject, body, dates, depends)", result)
	}
	if _, isForm := result["dates"].(components.FormResult); !isForm {
		t.Errorf("result[\"dates\"] is %T, want components.FormResult (scheduled+deadline collapsed into one Form step)", result["dates"])
	}
}
