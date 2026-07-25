// Package cli — tests for `rk checklist` (create/list/start/check/status/
// reset/abandon). Every test drives RootCmd.Execute() exactly the way
// index_test.go and todo_test.go drive their own commands: SetOut/SetErr,
// SetArgs, Execute, then assert on captured stdout/stderr or JSON-decoded
// output. internal/cli/checklist.go does not exist yet, so this file will not
// compile until it is added — that is the expected starting state.
//
// JSON assertions decode into internal/checklist's own model types
// (checklist.Template, checklist.Run) wherever the scenario is checking that
// output matches those models' json tags, rather than any CLI-local result
// type — the CLI layer's result types don't exist yet either.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/checklist"
)

// ─────────────────────────────────────────────────────────────────────────────
// Harness helpers
// ─────────────────────────────────────────────────────────────────────────────

// setupChecklistEnv points the operational database at a fresh temp directory
// for the duration of one test and registers the shared resetCLIFlags cleanup
// (query_test.go) so root-level flag state (--json/--ndjson/--quiet/etc.)
// never leaks into the next test.
func setupChecklistEnv(t *testing.T) {
	t.Helper()
	t.Setenv("RECKON_DATA_DIR", t.TempDir())
	t.Cleanup(resetCLIFlags)
}

// runChecklist executes `rk checklist <args...>` through RootCmd and returns
// (stdout, stderr, error), mirroring runTodo/runQuery elsewhere in this
// package. Callers making more than one Execute call within a single test
// must call resetCLIFlags() between calls (pflag-bound package globals don't
// reset themselves between SetArgs/Execute cycles).
func runChecklist(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	RootCmd.SetOut(&outBuf)
	RootCmd.SetErr(&errBuf)
	RootCmd.SetArgs(append([]string{"checklist"}, args...))
	err = RootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// ─────────────────────────────────────────────────────────────────────────────
// create
// ─────────────────────────────────────────────────────────────────────────────

func TestChecklistCreate_Basic(t *testing.T) {
	setupChecklistEnv(t)

	out, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--item", "b", "--json")
	if err != nil {
		t.Fatalf("checklist create: %v\nstderr: %s", err, stderr)
	}

	var tpl checklist.Template
	mustDecodeJSON(t, out, &tpl)
	if tpl.Name != "foo" {
		t.Errorf("Name = %q, want foo", tpl.Name)
	}
	if len(tpl.Items) != 2 {
		t.Fatalf("Items = %+v, want 2", tpl.Items)
	}
	if tpl.Items[0].Position != 0 || tpl.Items[0].Text != "a" {
		t.Errorf("Items[0] = %+v, want position 0 text %q", tpl.Items[0], "a")
	}
	if tpl.Items[1].Position != 1 || tpl.Items[1].Text != "b" {
		t.Errorf("Items[1] = %+v, want position 1 text %q", tpl.Items[1], "b")
	}
}

func TestChecklistCreate_DuplicateName(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a"); err != nil {
		t.Fatalf("first create: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	_, stderr, err := runChecklist(t, "create", "foo", "--item", "a")
	if err == nil {
		t.Fatal("expected an error creating a duplicate template, got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, `checklist template "foo" already exists`) {
		t.Errorf("expected a duplicate-name error, got err=%v stderr=%q", err, stderr)
	}
}

func TestChecklistCreate_EmptyName(t *testing.T) {
	setupChecklistEnv(t)

	_, stderr, err := runChecklist(t, "create", "", "--item", "a")
	if err == nil {
		t.Fatal("expected an error for an empty template name, got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "template name cannot be empty") {
		t.Errorf("expected an empty-name error, got err=%v stderr=%q", err, stderr)
	}
}

func TestChecklistCreate_ItemsFile(t *testing.T) {
	setupChecklistEnv(t)

	dir := t.TempDir()
	itemsPath := filepath.Join(dir, "items.txt")
	if err := os.WriteFile(itemsPath, []byte("first\nsecond\nthird\n"), 0o644); err != nil {
		t.Fatalf("write items file: %v", err)
	}

	out, stderr, err := runChecklist(t, "create", "foo", "--items-file", itemsPath, "--json")
	if err != nil {
		t.Fatalf("checklist create --items-file: %v\nstderr: %s", err, stderr)
	}

	var tpl checklist.Template
	mustDecodeJSON(t, out, &tpl)
	if len(tpl.Items) != 3 {
		t.Fatalf("Items = %+v, want 3", tpl.Items)
	}
	for i, want := range []string{"first", "second", "third"} {
		if tpl.Items[i].Text != want || tpl.Items[i].Position != i {
			t.Errorf("Items[%d] = %+v, want text %q at position %d", i, tpl.Items[i], want, i)
		}
	}
}

func TestChecklistCreate_ItemsFileAndItemMutuallyExclusive(t *testing.T) {
	setupChecklistEnv(t)

	dir := t.TempDir()
	itemsPath := filepath.Join(dir, "items.txt")
	if err := os.WriteFile(itemsPath, []byte("first\n"), 0o644); err != nil {
		t.Fatalf("write items file: %v", err)
	}

	_, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--items-file", itemsPath)
	if err == nil {
		t.Fatal("expected a mutual-exclusion error for --item + --items-file, got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "mutually exclusive") {
		t.Errorf("expected a mutually-exclusive error, got err=%v stderr=%q", err, stderr)
	}
}

func TestChecklistCreate_NoItemsRejected(t *testing.T) {
	setupChecklistEnv(t)

	t.Run("no item flags at all", func(t *testing.T) {
		_, stderr, err := runChecklist(t, "create", "foo")
		if err == nil {
			t.Fatal("expected an error creating a template with no items, got nil")
		}
		combined := err.Error() + stderr
		if !strings.Contains(combined, "at least one item required") {
			t.Errorf("expected 'at least one item required', got err=%v stderr=%q", err, stderr)
		}
	})

	t.Run("all-blank items file", func(t *testing.T) {
		dir := t.TempDir()
		emptyPath := filepath.Join(dir, "empty.txt")
		if err := os.WriteFile(emptyPath, []byte("\n\n   \n"), 0o644); err != nil {
			t.Fatalf("write empty items file: %v", err)
		}
		_, stderr, err := runChecklist(t, "create", "bar", "--items-file", emptyPath)
		if err == nil {
			t.Fatal("expected an error creating a template from an all-blank items file, got nil")
		}
		combined := err.Error() + stderr
		if !strings.Contains(combined, "at least one item required") {
			t.Errorf("expected 'at least one item required', got err=%v stderr=%q", err, stderr)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// list
// ─────────────────────────────────────────────────────────────────────────────

func TestChecklistList_Empty(t *testing.T) {
	setupChecklistEnv(t)

	out, stderr, err := runChecklist(t, "list")
	if err != nil {
		t.Fatalf("checklist list: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(out, "no templates") {
		t.Errorf("pretty output missing 'no templates': %q", out)
	}
	resetCLIFlags()

	outJSON, stderr, err := runChecklist(t, "list", "--json")
	if err != nil {
		t.Fatalf("checklist list --json: %v\nstderr: %s", err, stderr)
	}
	if strings.TrimSpace(outJSON) != "[]" {
		t.Errorf("json output = %q, want []", outJSON)
	}
}

func TestChecklistList_Templates(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "create", "bar", "--item", "b"); err != nil {
		t.Fatalf("create bar: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runChecklist(t, "list", "--json")
	if err != nil {
		t.Fatalf("checklist list --json: %v\nstderr: %s", err, stderr)
	}
	var tpls []*checklist.Template
	mustDecodeJSON(t, out, &tpls)
	if len(tpls) != 2 {
		t.Fatalf("templates = %+v, want 2", tpls)
	}
	var names []string
	for _, tpl := range tpls {
		names = append(names, tpl.Name)
	}
	if !containsString(names, "foo") || !containsString(names, "bar") {
		t.Errorf("names = %v, want foo and bar", names)
	}
}

func TestChecklistList_RunsForTemplate(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	// A single-item run auto-completes on its first check.
	if _, stderr, err := runChecklist(t, "check", "foo", "0"); err != nil {
		t.Fatalf("check foo 0: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	// A second, still-active run for the same template.
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("second start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	outDefault, stderr, err := runChecklist(t, "list", "foo", "--json")
	if err != nil {
		t.Fatalf("checklist list foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	var defaultRuns []*checklist.Run
	mustDecodeJSON(t, outDefault, &defaultRuns)
	if len(defaultRuns) != 1 {
		t.Fatalf("default list foo runs = %+v, want 1 (active only)", defaultRuns)
	}
	if defaultRuns[0].Status != checklist.RunStatusActive {
		t.Errorf("default run status = %q, want active", defaultRuns[0].Status)
	}

	outAll, stderr, err := runChecklist(t, "list", "foo", "--all", "--json")
	if err != nil {
		t.Fatalf("checklist list foo --all: %v\nstderr: %s", err, stderr)
	}
	var allRuns []*checklist.Run
	mustDecodeJSON(t, outAll, &allRuns)
	if len(allRuns) != 2 {
		t.Fatalf("list foo --all runs = %+v, want 2", allRuns)
	}
	var sawActive, sawCompleted bool
	for _, r := range allRuns {
		if r.TemplateID != defaultRuns[0].TemplateID {
			t.Errorf("run %+v has a different template ID than foo's, scoping leaked", r)
		}
		switch r.Status {
		case checklist.RunStatusActive:
			sawActive = true
		case checklist.RunStatusCompleted:
			sawCompleted = true
		}
	}
	if !sawActive || !sawCompleted {
		t.Errorf("expected both an active and a completed run, got %+v", allRuns)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// start
// ─────────────────────────────────────────────────────────────────────────────

func TestChecklistStart_Fresh(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--item", "b"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runChecklist(t, "start", "foo", "--json")
	if err != nil {
		t.Fatalf("checklist start foo: %v\nstderr: %s", err, stderr)
	}
	var run checklist.Run
	mustDecodeJSON(t, out, &run)
	if run.Status != checklist.RunStatusActive {
		t.Errorf("Status = %q, want active", run.Status)
	}
	if len(run.Items) != 2 {
		t.Fatalf("Items = %+v, want 2", run.Items)
	}
	for _, item := range run.Items {
		if item.Checked {
			t.Errorf("fresh run item unexpectedly checked: %+v", item)
		}
	}
}

func TestChecklistStart_Resume(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--item", "b"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "check", "foo", "0"); err != nil {
		t.Fatalf("check foo 0: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runChecklist(t, "start", "foo", "--json")
	if err != nil {
		t.Fatalf("resuming checklist start foo: %v\nstderr: %s", err, stderr)
	}
	var run checklist.Run
	mustDecodeJSON(t, out, &run)
	if run.Status != checklist.RunStatusActive {
		t.Errorf("Status = %q, want active (resumed, not replaced)", run.Status)
	}
	if len(run.Items) != 2 || !run.Items[0].Checked || run.Items[1].Checked {
		t.Fatalf("Items = %+v, want item 0 checked and item 1 unchecked (the resumed run)", run.Items)
	}
}

func TestChecklistStart_PrettyDistinguishesResume(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	freshOut, stderr, err := runChecklist(t, "start", "foo")
	if err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	if strings.Contains(freshOut, "resuming") {
		t.Errorf("fresh start Pretty output unexpectedly mentions resuming: %q", freshOut)
	}
	resetCLIFlags()

	resumeOut, stderr, err := runChecklist(t, "start", "foo")
	if err != nil {
		t.Fatalf("resuming start foo: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(resumeOut, "resuming existing run") {
		t.Errorf("resumed start Pretty output = %q, want it to mention resuming an existing run", resumeOut)
	}
}

func TestChecklistStart_UnknownTemplate(t *testing.T) {
	setupChecklistEnv(t)

	_, stderr, err := runChecklist(t, "start", "foo")
	if err == nil {
		t.Fatal("expected a not-found error starting an unknown template, got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "not found") {
		t.Errorf("expected a not-found error, got err=%v stderr=%q", err, stderr)
	}
}

func TestChecklistStart_AfterCompletedRun(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "check", "foo", "0"); err != nil {
		t.Fatalf("check foo 0 (completes the run): %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runChecklist(t, "start", "foo", "--json")
	if err != nil {
		t.Fatalf("start foo after a completed run: %v\nstderr: %s", err, stderr)
	}
	var run checklist.Run
	mustDecodeJSON(t, out, &run)
	if run.Status != checklist.RunStatusActive {
		t.Errorf("Status = %q, want active (fresh run)", run.Status)
	}
	if len(run.Items) != 1 || run.Items[0].Checked {
		t.Errorf("Items = %+v, want a single unchecked item (a fresh run)", run.Items)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// check
// ─────────────────────────────────────────────────────────────────────────────

func TestChecklistCheck_MarksItem(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--item", "b", "--item", "c"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runChecklist(t, "check", "foo", "0", "--json")
	if err != nil {
		t.Fatalf("checklist check foo 0: %v\nstderr: %s", err, stderr)
	}
	var run checklist.Run
	mustDecodeJSON(t, out, &run)
	if run.Status != checklist.RunStatusActive {
		t.Errorf("Status = %q, want active (not all items checked)", run.Status)
	}
	if len(run.Items) != 3 || !run.Items[0].Checked {
		t.Fatalf("Items = %+v, want item 0 checked", run.Items)
	}
	if run.Items[1].Checked || run.Items[2].Checked {
		t.Errorf("Items = %+v, want items 1 and 2 unchecked", run.Items)
	}
}

func TestChecklistCheck_TogglesOff(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--item", "b"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "check", "foo", "0"); err != nil {
		t.Fatalf("first check foo 0: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runChecklist(t, "check", "foo", "0", "--json")
	if err != nil {
		t.Fatalf("second checklist check foo 0: %v\nstderr: %s", err, stderr)
	}
	var run checklist.Run
	mustDecodeJSON(t, out, &run)
	if len(run.Items) != 2 || run.Items[0].Checked {
		t.Fatalf("Items = %+v, want item 0 toggled back to unchecked", run.Items)
	}
}

func TestChecklistCheck_OutOfRange(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--item", "b", "--item", "c"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	_, stderr, err := runChecklist(t, "check", "foo", "5")
	if err == nil {
		t.Fatal("expected an out-of-range error for check foo 5, got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "position 5 out of range (run has 3 items)") {
		t.Errorf("expected an out-of-range error, got err=%v stderr=%q", err, stderr)
	}
}

func TestChecklistCheck_NoActiveRun(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	_, stderr, err := runChecklist(t, "check", "foo", "0")
	if err == nil {
		t.Fatal("expected a no-active-run error, got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, `no active run for "foo" (use 'start' to begin)`) {
		t.Errorf("expected a no-active-run error, got err=%v stderr=%q", err, stderr)
	}
}

func TestChecklistCheck_AutoCompletes(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--item", "b"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "check", "foo", "0"); err != nil {
		t.Fatalf("check foo 0: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runChecklist(t, "check", "foo", "1", "--json")
	if err != nil {
		t.Fatalf("check foo 1 (last item): %v\nstderr: %s", err, stderr)
	}
	var run checklist.Run
	mustDecodeJSON(t, out, &run)
	if run.Status != checklist.RunStatusCompleted {
		t.Errorf("Status = %q, want completed after checking the last item", run.Status)
	}
	for _, item := range run.Items {
		if !item.Checked {
			t.Errorf("item %+v unexpectedly unchecked on a completed run", item)
		}
	}
}

func TestChecklistCheck_BadPositionArg(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	_, stderr, err := runChecklist(t, "check", "foo", "x")
	if err == nil {
		t.Fatal("expected an integer-parse error for a non-numeric position, got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "integer") {
		t.Errorf("expected an integer-parse error, got err=%v stderr=%q", err, stderr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// status
// ─────────────────────────────────────────────────────────────────────────────

func TestChecklistStatus_Active(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--item", "b", "--item", "c"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "check", "foo", "1"); err != nil {
		t.Fatalf("check foo 1: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runChecklist(t, "status", "foo", "--json")
	if err != nil {
		t.Fatalf("checklist status foo: %v\nstderr: %s", err, stderr)
	}
	var run checklist.Run
	mustDecodeJSON(t, out, &run)
	if run.Status != checklist.RunStatusActive {
		t.Errorf("Status = %q, want active", run.Status)
	}
	if len(run.Items) != 3 {
		t.Fatalf("Items = %+v, want 3", run.Items)
	}
	checkedCount := 0
	for _, item := range run.Items {
		if item.Checked {
			checkedCount++
		}
	}
	if checkedCount != 1 {
		t.Errorf("checkedCount = %d, want 1", checkedCount)
	}
}

func TestChecklistStatus_NoRun(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	_, stderr, err := runChecklist(t, "status", "foo")
	if err == nil {
		t.Fatal("expected an error for status on a template with no run, got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "use 'start' to begin") {
		t.Errorf("expected an error steering to 'start', got err=%v stderr=%q", err, stderr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// reset
// ─────────────────────────────────────────────────────────────────────────────

func TestChecklistReset_WithActiveRun(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--item", "b"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "check", "foo", "0"); err != nil {
		t.Fatalf("check foo 0: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runChecklist(t, "reset", "foo", "--json")
	if err != nil {
		t.Fatalf("checklist reset foo: %v\nstderr: %s", err, stderr)
	}
	var newRun checklist.Run
	mustDecodeJSON(t, out, &newRun)
	if newRun.Status != checklist.RunStatusActive {
		t.Errorf("Status = %q, want active", newRun.Status)
	}
	for _, item := range newRun.Items {
		if item.Checked {
			t.Errorf("new run item unexpectedly checked: %+v", item)
		}
	}
	resetCLIFlags()

	outAll, stderr, err := runChecklist(t, "list", "foo", "--all", "--json")
	if err != nil {
		t.Fatalf("checklist list foo --all: %v\nstderr: %s", err, stderr)
	}
	var runs []*checklist.Run
	mustDecodeJSON(t, outAll, &runs)
	if len(runs) != 2 {
		t.Fatalf("runs = %+v, want 2 (old abandoned + new active)", runs)
	}
	var sawAbandoned bool
	for _, r := range runs {
		if r.ID != newRun.ID && r.Status == checklist.RunStatusAbandoned {
			sawAbandoned = true
		}
	}
	if !sawAbandoned {
		t.Errorf("expected the old run to have been abandoned, got %+v", runs)
	}
}

func TestChecklistReset_NoActiveRun(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runChecklist(t, "reset", "foo", "--json")
	if err != nil {
		t.Fatalf("checklist reset foo with no prior run: %v\nstderr: %s", err, stderr)
	}
	var run checklist.Run
	mustDecodeJSON(t, out, &run)
	if run.Status != checklist.RunStatusActive {
		t.Errorf("Status = %q, want active", run.Status)
	}
	if len(run.Items) != 1 || run.Items[0].Checked {
		t.Errorf("Items = %+v, want a single unchecked item", run.Items)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// abandon
// ─────────────────────────────────────────────────────────────────────────────

func TestChecklistAbandon_WithActiveRun(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runChecklist(t, "start", "foo"); err != nil {
		t.Fatalf("start foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runChecklist(t, "abandon", "foo", "--json")
	if err != nil {
		t.Fatalf("checklist abandon foo: %v\nstderr: %s", err, stderr)
	}
	var run checklist.Run
	mustDecodeJSON(t, out, &run)
	if run.Status != checklist.RunStatusAbandoned {
		t.Errorf("Status = %q, want abandoned", run.Status)
	}
	resetCLIFlags()

	_, stderr, err = runChecklist(t, "status", "foo")
	if err == nil {
		t.Fatal("expected status foo to error after abandon (no active run), got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "use 'start' to begin") {
		t.Errorf("expected a no-active-run error, got err=%v stderr=%q", err, stderr)
	}
}

func TestChecklistAbandon_NoActiveRun(t *testing.T) {
	setupChecklistEnv(t)

	if _, stderr, err := runChecklist(t, "create", "foo", "--item", "a"); err != nil {
		t.Fatalf("create foo: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	_, stderr, err := runChecklist(t, "abandon", "foo")
	if err == nil {
		t.Fatal("expected an error abandoning a template with no active run, got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, `no active run for "foo" (use 'start' to begin)`) {
		t.Errorf("expected a no-active-run error, got err=%v stderr=%q", err, stderr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// --json / --ndjson conventions
// ─────────────────────────────────────────────────────────────────────────────

// TestChecklistJSON_MatchesModelTags checks that --json output for both a
// single-object verb (create) and a run verb (start) is flat JSON keyed by
// internal/checklist's own model tags, not some CLI-local wrapper shape.
func TestChecklistJSON_MatchesModelTags(t *testing.T) {
	setupChecklistEnv(t)

	outCreate, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--json")
	if err != nil {
		t.Fatalf("checklist create foo: %v\nstderr: %s", err, stderr)
	}
	var tplMap map[string]any
	mustDecodeJSON(t, outCreate, &tplMap)
	for _, key := range []string{"id", "name", "items", "created_at", "updated_at"} {
		if _, ok := tplMap[key]; !ok {
			t.Errorf("create --json output missing model-tagged key %q: %v", key, tplMap)
		}
	}
	var tpl checklist.Template
	mustDecodeJSON(t, outCreate, &tpl)
	if tpl.Name != "foo" {
		t.Errorf("decoded Template.Name = %q, want foo", tpl.Name)
	}
	resetCLIFlags()

	outStart, stderr, err := runChecklist(t, "start", "foo", "--json")
	if err != nil {
		t.Fatalf("checklist start foo: %v\nstderr: %s", err, stderr)
	}
	var runMap map[string]any
	mustDecodeJSON(t, outStart, &runMap)
	for _, key := range []string{"id", "template_id", "template_name", "status", "items", "started_at"} {
		if _, ok := runMap[key]; !ok {
			t.Errorf("start --json output missing model-tagged key %q: %v", key, runMap)
		}
	}
	if _, ok := runMap["resumed"]; ok {
		t.Errorf("start --json output leaked a non-model 'resumed' key: %v", runMap)
	}
	var run checklist.Run
	mustDecodeJSON(t, outStart, &run)
	if run.TemplateName != "foo" {
		t.Errorf("decoded Run.TemplateName = %q, want foo", run.TemplateName)
	}
}

func TestChecklistJSON_NdjsonMutuallyExclusive(t *testing.T) {
	setupChecklistEnv(t)

	_, stderr, err := runChecklist(t, "create", "foo", "--item", "a", "--json", "--ndjson")
	if err == nil {
		t.Fatal("expected a mutually-exclusive error for --json + --ndjson, got nil")
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "mutually exclusive") {
		t.Errorf("expected a mutually-exclusive error, got err=%v stderr=%q", err, stderr)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// help text
// ─────────────────────────────────────────────────────────────────────────────

// TestChecklistHelp_DocumentsLimitation checks that the parent command's help
// text plainly states checklist state lives only in the local operational
// database and is not carried by rk index's rebuild of the vault-derived
// index.
func TestChecklistHelp_DocumentsLimitation(t *testing.T) {
	setupChecklistEnv(t)

	out, stderr, err := runChecklist(t, "--help")
	if err != nil {
		t.Fatalf("checklist --help: %v\nstderr: %s", err, stderr)
	}
	lower := strings.ToLower(out + stderr)
	if !strings.Contains(lower, "index") {
		t.Errorf("help text does not mention rk index's lack of effect on checklist state: %q", out)
	}
	if !strings.Contains(lower, "vault") {
		t.Errorf("help text does not mention the vault-native limitation: %q", out)
	}
}
