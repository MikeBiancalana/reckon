// Package cli — TDD red tests for v1-T5 `rk todo add/list/done` (reckon-qiua).
//
// internal/cli/todo.go does not exist yet. Today `todoCmd` is only the stub in
// internal/cli/stubs.go (lines 50-59, returns errNotImplemented, no
// subcommands registered). Every test below references types this file's
// implementation phase (Phase 4) must define — todoAddResult, todoListResult,
// todoListItem, todoDoneResult, resolveAuthor, mintTodoULID — so the whole
// `cli` package fails to COMPILE right now. That is the expected TDD-red
// state at this stage: not "tests run and fail," but "the package does not
// build until todo.go exists." Once todo.go defines these symbols with the
// shapes pinned below, the package builds and these tests start driving real
// (pass/fail) behavior.
//
// Precedent / harness reuse (do not redefine these — they already live in
// this package): setupQueryVault, writeTestNode, resetCLIFlags, buildIndex,
// runQuery, parseNDJSONMaps, countNDJSONLines (query_test.go); mustWriteFile,
// mustReadFile, mustMkdirAll, isValidULID, crockfordAlphabet (adopt_test.go).
//
// ─────────────────────────────────────────────────────────────────────────
// Pinned contract (plan.md "Files to modify" / "Test scenarios" + AC-doc §2):
// ─────────────────────────────────────────────────────────────────────────
//
//	// todoAddResult is the structured summary of one `rk todo add` run.
//	type todoAddResult struct {
//	    Kind  string `json:"kind"`            // "durable" | "ephemeral"
//	    Path  string `json:"path"`            // vault-relative: "todos/<ULID>.md" or "todos/inbox.md"
//	    ID    string `json:"id,omitempty"`    // durable only: the new node's ULID
//	    Line  int    `json:"line,omitempty"`  // ephemeral only: 1-based index of the appended item
//	    State string `json:"state,omitempty"` // durable only: "open" on create
//	}
//
//	// todoListItem is one row of `rk todo list` output, durable or ephemeral.
//	// omitempty keeps durable-only/ephemeral-only fields from appearing on the
//	// other kind's records (plan.md D10).
//	type todoListItem struct {
//	    Kind      string `json:"kind"`                 // "durable" | "ephemeral"
//	    ID        string `json:"id,omitempty"`         // durable only: ULID
//	    Path      string `json:"path,omitempty"`       // durable only: vault-relative file path
//	    Container string `json:"container,omitempty"`  // ephemeral only: vault-relative container path
//	    Line      int    `json:"line,omitempty"`        // ephemeral only: stable 1-based index in file order
//	    State     string `json:"state,omitempty"`      // durable only: "open" | "done"
//	    Checked   bool   `json:"checked"`               // ephemeral only (meaningful false, so no omitempty)
//	    Scheduled string `json:"scheduled,omitempty"`   // durable only
//	    Deadline  string `json:"deadline,omitempty"`    // durable only
//	    Depends   string `json:"depends,omitempty"`     // durable only
//	    Body      string `json:"body"`                  // node body (durable) / checkbox text (ephemeral)
//	    Title     string `json:"title,omitempty"`       // durable only: derived first non-empty body line (reckon-fnqs.3)
//	}
//
//	// todoListResult wraps `rk todo list`'s items so --json emits a single
//	// object ({"items": []} on empty, per EC-4), not a bare top-level array.
//	type todoListResult struct {
//	    Items []todoListItem `json:"items"`
//	}
//
//	// todoDoneResult is the structured summary of one `rk todo done` run.
//	type todoDoneResult struct {
//	    Kind    string `json:"kind"`            // "durable" | "ephemeral"
//	    Ref     string `json:"ref"`             // the ref/index the caller passed
//	    Path    string `json:"path,omitempty"`  // vault-relative: the file mutated
//	    ID      string `json:"id,omitempty"`    // durable only: resolved ULID
//	    State   string `json:"state,omitempty"` // durable only: "done"
//	    Skipped bool   `json:"skipped"`         // true = idempotent no-op (already done/checked)
//	}
//
// Additional pinned implementation requirements this file's tests depend on:
//
//   - `resolveAuthor(flag string) string` (plan.md D8): precedence
//     --author flag > $RECKON_AUTHOR > $USER > "local", always non-empty.
//     Exercised directly (same package) by TestResolveAuthor_PrecedenceChain.
//
//   - `var mintTodoULID = node.Mint` (a package-level `func() string` var,
//     NOT pinned by plan.md itself but required by this test file): todo.go's
//     durable-create path must mint its ULID by calling mintTodoULID(), not
//     node.Mint() directly, so a test can override the var to force a
//     deterministic collision. Plan.md's own risk-list (#3) says forcing a
//     genuine ULID collision from the CLI black-box surface is impractical;
//     this seam is the chosen mechanism for TestTodoAdd_NoClobberExistingFile
//     (T-1.7) rather than skipping that scenario outright.
//
// Paths in results (`Path`/`Container`) are vault-relative, forward-slash
// separated ("todos/<ULID>.md", "todos/inbox.md") — mirrors adoptResult's
// convention (adopt_test.go) so output stays stable across machines/temp dirs.
package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/node"
)

// ─────────────────────────────────────────────────────────────────────────────
// Harness helpers (todo-specific; vault/cache/flag-reset plumbing, node
// fixture writers, and JSON helpers are shared with query_test.go/adopt_test.go).
// ─────────────────────────────────────────────────────────────────────────────

// runTodo executes `rk todo <args...> --vault <vault>` through RootCmd and
// returns (stdout, stderr, error), mirroring runQuery (query_test.go) and
// runAdopt (adopt_test.go). args[0] is expected to be the subcommand name
// ("add", "list", or "done"). The caller must call resetCLIFlags() before
// another Execute within the same test (t.Cleanup(resetCLIFlags) covers
// end-of-test).
func runTodo(t *testing.T, vault string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var outBuf, errBuf bytes.Buffer
	RootCmd.SetOut(&outBuf)
	RootCmd.SetErr(&errBuf)
	full := append([]string{"todo", "--vault", vault}, args...)
	RootCmd.SetArgs(full)
	err = RootCmd.Execute()
	return outBuf.String(), errBuf.String(), err
}

// mustDecodeJSON decodes s into v, fataling with the raw string on failure.
func mustDecodeJSON(t *testing.T, s string, v any) {
	t.Helper()
	if err := json.Unmarshal([]byte(s), v); err != nil {
		t.Fatalf("decode json %q: %v", s, err)
	}
}

// checklistLine renders one markdown task-list line in the exact form D2's
// ephemeral container uses ("- [ ] text" / "- [x] text").
func checklistLine(checked bool, text string) string {
	mark := " "
	if checked {
		mark = "x"
	}
	return fmt.Sprintf("- [%s] %s", mark, text)
}

// writeEphemeralContainer writes todos/inbox.md directly (bypassing `add`) so
// list/done tests can set up known checkbox-item fixtures. items should be
// pre-formatted lines from checklistLine. Returns the container's path.
func writeEphemeralContainer(t *testing.T, vault, id string, items ...string) string {
	t.Helper()
	var body strings.Builder
	body.WriteString("# Inbox\n\n")
	for _, it := range items {
		body.WriteString(it)
		body.WriteString("\n")
	}
	path := filepath.Join(vault, "todos", "inbox.md")
	mustWriteFile(t, path,
		"---\nid: "+id+"\ntype: todo-ephemeral\n---\n"+body.String())
	return path
}

// containsID reports whether items has an entry with the given durable ID.
func containsID(items []todoListItem, id string) bool {
	for _, it := range items {
		if it.ID == id {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
// resolveAuthor (plan.md D8) — direct unit test, same package.
// ─────────────────────────────────────────────────────────────────────────────

func TestResolveAuthor_PrecedenceChain(t *testing.T) {
	tests := []struct {
		name         string
		flag         string
		reckonAuthor string
		user         string
		want         string
	}{
		{"flag wins over everything", "alice", "bob", "carol", "alice"},
		{"RECKON_AUTHOR wins over USER when no flag", "", "bob", "carol", "bob"},
		{"USER used when no flag or RECKON_AUTHOR", "", "", "carol", "carol"},
		{"local fallback when nothing is set", "", "", "", "local"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("RECKON_AUTHOR", tt.reckonAuthor)
			t.Setenv("USER", tt.user)
			if got := resolveAuthor(tt.flag); got != tt.want {
				t.Errorf("resolveAuthor(%q) = %q, want %q", tt.flag, got, tt.want)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-1 (create) — T-1.1..T-1.8
// ─────────────────────────────────────────────────────────────────────────────

// T-1.1: durable happy path.
func TestTodoAdd_DurableHappyPath(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	if _, err := os.Stat(filepath.Join(vault, "todos")); !os.IsNotExist(err) {
		t.Fatalf("precondition: todos/ must not exist yet, stat err = %v", err)
	}

	out, stderr, err := runTodo(t, vault, "add", "Buy milk", "--json")
	if err != nil {
		t.Fatalf("rk todo add: %v\nstderr: %s", err, stderr)
	}

	var res todoAddResult
	mustDecodeJSON(t, out, &res)
	if res.Kind != "durable" {
		t.Errorf("Kind = %q, want durable", res.Kind)
	}
	if res.State != "open" {
		t.Errorf("State = %q, want open", res.State)
	}
	if !isValidULID(res.ID) {
		t.Errorf("ID %q is not a valid ULID", res.ID)
	}
	wantPath := "todos/" + res.ID + ".md"
	if res.Path != wantPath {
		t.Errorf("Path = %q, want %q", res.Path, wantPath)
	}

	raw := mustReadFile(t, filepath.Join(vault, wantPath))
	n, err := node.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse created file: %v", err)
	}
	if n.Type != "todo" {
		t.Errorf("Type = %q, want todo", n.Type)
	}
	if n.ULID != res.ID {
		t.Errorf("file ULID %q != reported ID %q", n.ULID, res.ID)
	}
	if n.Props["state"] != "open" {
		t.Errorf("Props[state] = %q, want open", n.Props["state"])
	}
	if strings.TrimSpace(n.Body) != "Buy milk" {
		t.Errorf("Body = %q, want %q", n.Body, "Buy milk")
	}
}

// T-1.2: durable with props (scheduled/deadline as scalar props, not links;
// no empty-string keys for omitted props).
func TestTodoAdd_DurableWithProps(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runTodo(t, vault, "add", "Ship v1",
		"--scheduled", "2026-07-10", "--deadline", "2026-07-15", "--json")
	if err != nil {
		t.Fatalf("rk todo add: %v\nstderr: %s", err, stderr)
	}
	var res todoAddResult
	mustDecodeJSON(t, out, &res)

	raw := mustReadFile(t, filepath.Join(vault, res.Path))
	n, err := node.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("parse created file: %v", err)
	}
	if n.Props["scheduled"] != "2026-07-10" {
		t.Errorf("Props[scheduled] = %q, want 2026-07-10", n.Props["scheduled"])
	}
	if n.Props["deadline"] != "2026-07-15" {
		t.Errorf("Props[deadline] = %q, want 2026-07-15", n.Props["deadline"])
	}
	if len(n.Links) != 0 {
		t.Errorf("Links = %+v, want none (scheduled/deadline are scalar props)", n.Links)
	}
	if strings.Contains(raw, "depends-on:") {
		t.Errorf("unexpected depends-on key when --depends not given: %q", raw)
	}
}

// T-1.3: durable with a live --depends target produces a Links entry, never
// a Props entry.
func TestTodoAdd_DurableWithDependsLink(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	aID := node.Mint()
	writeTestNode(t, vault, "todos/"+aID+".md", aID, "todo", "Existing todo A", "state: open")

	out, stderr, err := runTodo(t, vault, "add", "Blocked", "--depends", aID, "--json")
	if err != nil {
		t.Fatalf("rk todo add --depends: %v\nstderr: %s", err, stderr)
	}
	var res todoAddResult
	mustDecodeJSON(t, out, &res)
	if res.Kind != "durable" {
		t.Errorf("Kind = %q, want durable", res.Kind)
	}

	n, err := node.Parse([]byte(mustReadFile(t, filepath.Join(vault, res.Path))))
	if err != nil {
		t.Fatalf("parse created file: %v", err)
	}
	if len(n.Links) != 1 || n.Links[0].Rel != "depends-on" || n.Links[0].To != aID {
		t.Errorf("Links = %+v, want a single depends-on -> %s", n.Links, aID)
	}
	if _, has := n.Props["depends-on"]; has {
		t.Errorf("depends-on leaked into Props: %+v", n.Props)
	}
}

// T-1.4: ephemeral happy path — container created on first add, appended on
// second, container ULID and first line stay byte-identical across the append.
func TestTodoAdd_EphemeralHappyPath(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	containerPath := filepath.Join(vault, "todos", "inbox.md")
	if _, err := os.Stat(containerPath); !os.IsNotExist(err) {
		t.Fatalf("precondition: inbox.md must not exist yet, stat err = %v", err)
	}

	out1, stderr1, err1 := runTodo(t, vault, "add", "call dentist", "--ephemeral", "--json")
	if err1 != nil {
		t.Fatalf("first rk todo add --ephemeral: %v\nstderr: %s", err1, stderr1)
	}
	var res1 todoAddResult
	mustDecodeJSON(t, out1, &res1)
	if res1.Kind != "ephemeral" {
		t.Errorf("Kind = %q, want ephemeral", res1.Kind)
	}

	firstRaw := mustReadFile(t, containerPath)
	n1, err := node.Parse([]byte(firstRaw))
	if err != nil {
		t.Fatalf("parse container: %v", err)
	}
	if n1.Type != "todo-ephemeral" {
		t.Errorf("container Type = %q, want todo-ephemeral", n1.Type)
	}
	if !strings.Contains(n1.Body, "- [ ] call dentist") {
		t.Errorf("body missing unchecked item: %q", n1.Body)
	}
	containerID := n1.ULID
	resetCLIFlags()

	_, stderr2, err2 := runTodo(t, vault, "add", "buy milk", "--ephemeral")
	if err2 != nil {
		t.Fatalf("second rk todo add --ephemeral: %v\nstderr: %s", err2, stderr2)
	}

	secondRaw := mustReadFile(t, containerPath)
	n2, err := node.Parse([]byte(secondRaw))
	if err != nil {
		t.Fatalf("parse container after 2nd add: %v", err)
	}
	if n2.ULID != containerID {
		t.Errorf("container ULID changed across appends: %q -> %q", containerID, n2.ULID)
	}
	if !strings.Contains(n2.Body, "- [ ] call dentist") || !strings.Contains(n2.Body, "- [ ] buy milk") {
		t.Errorf("body missing one of the two items: %q", n2.Body)
	}

	beforeLines := strings.Split(firstRaw, "\n")
	afterLines := strings.Split(secondRaw, "\n")
	if len(afterLines) < len(beforeLines) {
		t.Fatalf("append shrank the file: before %d lines, after %d lines", len(beforeLines), len(afterLines))
	}
	for i, line := range beforeLines {
		if afterLines[i] != line {
			t.Fatalf("line %d changed by append (want EOF-only append)\nbefore: %q\nafter:  %q", i, line, afterLines[i])
		}
	}
}

// T-1.5: a completely fresh (nonexistent) vault path gets its directory tree
// created; no "no such file or directory" error.
func TestTodoAdd_FreshVaultCreatesDirs(t *testing.T) {
	root := t.TempDir()
	vault := filepath.Join(root, "brand-new-vault")
	cache := filepath.Join(root, "cache")
	t.Setenv("RECKON_CACHE", cache)
	t.Cleanup(resetCLIFlags)

	if _, err := os.Stat(vault); !os.IsNotExist(err) {
		t.Fatalf("precondition: vault must not exist yet, stat err = %v", err)
	}

	out, stderr, err := runTodo(t, vault, "add", "first item", "--json")
	if err != nil {
		t.Fatalf("rk todo add on a fresh vault path: %v\nstderr: %s", err, stderr)
	}
	var res todoAddResult
	mustDecodeJSON(t, out, &res)

	if _, err := os.Stat(filepath.Join(vault, "todos")); err != nil {
		t.Errorf("todos/ dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(vault, res.Path)); err != nil {
		t.Errorf("todo file not created at %q: %v", res.Path, err)
	}
}

// T-1.6: the very first write of a created durable todo is already
// round-trip-stable (create-path counterpart to T-4.1).
func TestTodoAdd_CreatePathRoundTrip(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runTodo(t, vault, "add", "Buy milk", "--json")
	if err != nil {
		t.Fatalf("rk todo add: %v\nstderr: %s", err, stderr)
	}
	var res todoAddResult
	mustDecodeJSON(t, out, &res)

	raw, err := os.ReadFile(filepath.Join(vault, res.Path))
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	n, err := node.Parse(raw)
	if err != nil {
		t.Fatalf("parse created file: %v", err)
	}
	if !bytes.Equal(n.Serialize(), raw) {
		t.Errorf("parse(raw).Serialize() != raw on first write\n--- raw ---\n%q\n--- serialize ---\n%q", raw, n.Serialize())
	}
}

// T-1.7 (EC-5): no-clobber. Forcing a genuine ULID collision from the CLI
// black-box surface is impractical (plan.md risk #3), so this test overrides
// the pinned mintTodoULID seam (see header comment) to force `add` to target
// a path a stray file already occupies, and asserts `add` refuses rather than
// silently overwriting it.
func TestTodoAdd_NoClobberExistingFile(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	forced := node.Mint()
	prevMint := mintTodoULID
	mintTodoULID = func() string { return forced }
	t.Cleanup(func() { mintTodoULID = prevMint })

	strayPath := filepath.Join(vault, "todos", forced+".md")
	const strayContent = "not a todo, just a stray file\n"
	mustWriteFile(t, strayPath, strayContent)

	if _, _, err := runTodo(t, vault, "add", "should not clobber"); err == nil {
		t.Fatal("expected an error when add's computed target path already exists, got nil")
	}

	if got := mustReadFile(t, strayPath); got != strayContent {
		t.Fatalf("stray file was overwritten\n--- want ---\n%q\n--- got ---\n%q", strayContent, got)
	}
}

// T-1.8: --ephemeral rejects durable-only date props as a usage error.
func TestTodoAdd_EphemeralRejectsDateFlags(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	cases := []struct {
		name string
		args []string
	}{
		{"scheduled", []string{"add", "x", "--ephemeral", "--scheduled", "2026-07-10"}},
		{"deadline", []string{"add", "x", "--ephemeral", "--deadline", "2026-07-15"}},
		{"depends", []string{"add", "x", "--ephemeral", "--depends", "someref"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetCLIFlags()
			if _, stderr, err := runTodo(t, vault, tc.args...); err == nil {
				t.Fatalf("expected a usage error for --ephemeral + --%s, got nil (stderr=%q)", tc.name, stderr)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-2 / AC-6 (list) — T-2.1..T-2.6
// ─────────────────────────────────────────────────────────────────────────────

// T-2.1: freshness — a fresh add is visible to the very next list, no manual
// `rk index` step.
func TestTodoList_FreshnessAfterAdd(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	if _, _, err := runTodo(t, vault, "add", "Task A"); err != nil {
		t.Fatalf("rk todo add: %v", err)
	}
	resetCLIFlags()

	out, stderr, err := runTodo(t, vault, "list", "--json")
	if err != nil {
		t.Fatalf("rk todo list: %v\nstderr: %s", err, stderr)
	}
	var res todoListResult
	mustDecodeJSON(t, out, &res)

	found := false
	for _, it := range res.Items {
		if strings.Contains(it.Body, "Task A") {
			found = true
			if it.State != "open" {
				t.Errorf("Task A state = %q, want open", it.State)
			}
		}
	}
	if !found {
		t.Errorf("Task A not present in list output: %+v", res.Items)
	}
}

// T-2.2 / T-6.2: list distinguishes durable vs ephemeral, and --durable /
// --ephemeral restrict to one kind.
func TestTodoList_KindDistinction(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	if _, _, err := runTodo(t, vault, "add", "Durable one"); err != nil {
		t.Fatalf("add durable: %v", err)
	}
	resetCLIFlags()
	if _, _, err := runTodo(t, vault, "add", "Ephemeral one", "--ephemeral"); err != nil {
		t.Fatalf("add ephemeral: %v", err)
	}
	resetCLIFlags()

	t.Run("no filter shows both", func(t *testing.T) {
		out, _, err := runTodo(t, vault, "list", "--json")
		if err != nil {
			t.Fatalf("rk todo list: %v", err)
		}
		resetCLIFlags()
		var res todoListResult
		mustDecodeJSON(t, out, &res)

		var sawDurable, sawEphemeral bool
		for _, it := range res.Items {
			switch it.Kind {
			case "durable":
				sawDurable = true
			case "ephemeral":
				sawEphemeral = true
			default:
				t.Errorf("unexpected kind %q in %+v", it.Kind, it)
			}
		}
		if !sawDurable || !sawEphemeral {
			t.Errorf("expected both kinds present, got items=%+v", res.Items)
		}
	})

	t.Run("--durable shows only durable", func(t *testing.T) {
		out, _, err := runTodo(t, vault, "list", "--durable", "--json")
		if err != nil {
			t.Fatalf("rk todo list --durable: %v", err)
		}
		resetCLIFlags()
		var res todoListResult
		mustDecodeJSON(t, out, &res)
		if len(res.Items) == 0 {
			t.Fatal("--durable returned zero items, want at least the durable todo")
		}
		for _, it := range res.Items {
			if it.Kind != "durable" {
				t.Errorf("--durable leaked a %s item: %+v", it.Kind, it)
			}
		}
	})

	t.Run("--ephemeral shows only ephemeral", func(t *testing.T) {
		out, _, err := runTodo(t, vault, "list", "--ephemeral", "--json")
		if err != nil {
			t.Fatalf("rk todo list --ephemeral: %v", err)
		}
		resetCLIFlags()
		var res todoListResult
		mustDecodeJSON(t, out, &res)
		if len(res.Items) == 0 {
			t.Fatal("--ephemeral returned zero items, want at least the ephemeral todo")
		}
		for _, it := range res.Items {
			if it.Kind != "ephemeral" {
				t.Errorf("--ephemeral leaked a %s item: %+v", it.Kind, it)
			}
		}
	})
}

// T-2.3: default hides done items; --all shows everything.
func TestTodoList_DefaultHidesDone(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	openID := node.Mint()
	doneID := node.Mint()
	writeTestNode(t, vault, "todos/"+openID+".md", openID, "todo", "Open task", "state: open")
	writeTestNode(t, vault, "todos/"+doneID+".md", doneID, "todo", "Done task", "state: done")

	out, stderr, err := runTodo(t, vault, "list", "--json")
	if err != nil {
		t.Fatalf("rk todo list: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	var res todoListResult
	mustDecodeJSON(t, out, &res)
	if containsID(res.Items, doneID) {
		t.Errorf("done item %q leaked into default list: %+v", doneID, res.Items)
	}
	if !containsID(res.Items, openID) {
		t.Errorf("open item %q missing from default list: %+v", openID, res.Items)
	}

	outAll, stderr, err := runTodo(t, vault, "list", "--all", "--json")
	if err != nil {
		t.Fatalf("rk todo list --all: %v\nstderr: %s", err, stderr)
	}
	var resAll todoListResult
	mustDecodeJSON(t, outAll, &resAll)
	if !containsID(resAll.Items, openID) || !containsID(resAll.Items, doneID) {
		t.Errorf("--all missing an item: %+v", resAll.Items)
	}
}

// T-2.4 (EC-4): empty vault -> exit 0, empty items, no error.
func TestTodoList_EmptyVault(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runTodo(t, vault, "list", "--json")
	if err != nil {
		t.Fatalf("rk todo list on an empty vault: %v\nstderr: %s", err, stderr)
	}
	var res todoListResult
	mustDecodeJSON(t, out, &res)
	if len(res.Items) != 0 {
		t.Errorf("Items = %+v, want empty", res.Items)
	}
}

// T-2.5 (EC-7): a nonexistent --vault directory is a clean empty result, not
// a crash.
func TestTodoList_MissingVaultDir(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "does-not-exist")
	cache := filepath.Join(root, "cache")
	t.Setenv("RECKON_CACHE", cache)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runTodo(t, missing, "list", "--json")
	if err != nil {
		t.Fatalf("rk todo list against a missing vault dir: %v\nstderr: %s", err, stderr)
	}
	var res todoListResult
	mustDecodeJSON(t, out, &res)
	if len(res.Items) != 0 {
		t.Errorf("Items = %+v, want empty", res.Items)
	}
}

// T-2.6: ephemeral list carries a stable 1-based index and checked flag per
// item; --all includes checked items.
func TestTodoList_EphemeralIndexAndChecked(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeEphemeralContainer(t, vault, node.Mint(),
		checklistLine(false, "first item"),
		checklistLine(true, "second item"),
		checklistLine(false, "third item"),
	)

	out, stderr, err := runTodo(t, vault, "list", "--ephemeral", "--json")
	if err != nil {
		t.Fatalf("rk todo list --ephemeral: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	var res todoListResult
	mustDecodeJSON(t, out, &res)
	if len(res.Items) != 2 { // default hides the checked "second item"
		t.Fatalf("default --ephemeral list = %+v, want 2 open items", res.Items)
	}
	for _, it := range res.Items {
		if it.Checked {
			t.Errorf("checked item leaked into default list: %+v", it)
		}
	}

	outAll, stderr, err := runTodo(t, vault, "list", "--ephemeral", "--all", "--json")
	if err != nil {
		t.Fatalf("rk todo list --ephemeral --all: %v\nstderr: %s", err, stderr)
	}
	var resAll todoListResult
	mustDecodeJSON(t, outAll, &resAll)
	if len(resAll.Items) != 3 {
		t.Fatalf("Items = %+v, want 3", resAll.Items)
	}
	byLine := map[int]todoListItem{}
	for _, it := range resAll.Items {
		byLine[it.Line] = it
	}
	if !strings.Contains(byLine[1].Body, "first item") || byLine[1].Checked {
		t.Errorf("index 1 = %+v, want unchecked 'first item'", byLine[1])
	}
	if !strings.Contains(byLine[2].Body, "second item") || !byLine[2].Checked {
		t.Errorf("index 2 = %+v, want checked 'second item'", byLine[2])
	}
	if !strings.Contains(byLine[3].Body, "third item") || byLine[3].Checked {
		t.Errorf("index 3 = %+v, want unchecked 'third item'", byLine[3])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// reckon-fnqs.3 — subject/body node convention: `rk todo list` renders only
// the derived title (not the full body) in pretty mode, while --json carries
// both title and body. writeTodoFixture is defined in today_test.go (same
// package) and used here unmodified for its byte-exact fixture-writing shape.
//
// RED-state note: TestTodoList_JSON_TitleAndBody decodes into a local
// map[string]any (not the pinned todoListItem struct) specifically so this
// file keeps compiling before todo.go gains a Title field -- referencing
// `it.Title` directly would fail to compile until Phase 4, taking the whole
// cli package down with it. Decoding generically localizes the red state to
// a runtime assertion failure (missing "title" key today), matching this
// ticket's "tests must compile but must fail" rule.
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoList_Pretty_RendersTitleOnly (AC4): a durable todo with a 5-line
// body renders only its first line ("Ship it.") in pretty `rk todo list`
// output -- not the rest of the body. RED today: todoListResult.Pretty()
// (todo.go:196) still interpolates it.Body in full.
func TestTodoList_Pretty_RendersTitleOnly(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", "", "Ship it.\n\nline2\nline3\nline4\n")

	out, stderr, err := runTodo(t, vault, "list")
	if err != nil {
		t.Fatalf("rk todo list: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(out, "Ship it.") {
		t.Errorf("pretty output missing title %q: %q", "Ship it.", out)
	}
	for _, unwanted := range []string{"line2", "line3", "line4"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("pretty output leaked body line %q, want title-only rendering: %q", unwanted, out)
		}
	}
}

// TestTodoList_Pretty_ShowsMetadata (reckon-brny): the plain-text `rk todo
// list` output includes scheduled, deadline, and depends-on inline for
// durable todos that have them, so priority decisions don't require --json.
func TestTodoList_Pretty_ShowsMetadata(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	// A dependency target that the depends-on link will reference.
	depID := node.Mint()
	writeTodoFixture(t, vault, depID, "open", "", "Dependency todo\n")

	// Todo with all three metadata fields.
	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", "2026-07-20", "Ship v1\n",
		"deadline: 2026-07-26",
		`depends-on: "[[`+depID+`]]"`,
	)

	out, stderr, err := runTodo(t, vault, "list")
	if err != nil {
		t.Fatalf("rk todo list: %v\nstderr: %s", err, stderr)
	}
	if !strings.Contains(out, "(scheduled 2026-07-20)") {
		t.Errorf("pretty output missing scheduled annotation: %q", out)
	}
	if !strings.Contains(out, "(deadline 2026-07-26)") {
		t.Errorf("pretty output missing deadline annotation: %q", out)
	}
	if !strings.Contains(out, "(blocked on "+depID+")") {
		t.Errorf("pretty output missing depends-on annotation: %q", out)
	}
}

// TestTodoList_Pretty_NoMetadataOmitsAnnotations (reckon-brny): a durable
// todo with no scheduled/deadline/depends renders cleanly with no trailing
// annotations — the original output shape is preserved.
func TestTodoList_Pretty_NoMetadataOmitsAnnotations(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", "", "Plain todo\n")

	out, stderr, err := runTodo(t, vault, "list")
	if err != nil {
		t.Fatalf("rk todo list: %v\nstderr: %s", err, stderr)
	}
	for _, unwanted := range []string{"(scheduled", "(deadline", "(blocked on"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("plain todo should not have %q annotation: %q", unwanted, out)
		}
	}
}

// TestTodoList_JSON_TitleAndBody (AC7): `rk todo list --json` carries BOTH
// the derived title and the full multi-line body on the same item -- title
// is additive, not a body replacement. RED today: no "title" key exists in
// the JSON output at all, so the decoded title is always "".
func TestTodoList_JSON_TitleAndBody(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	writeTodoFixture(t, vault, id, "open", "", "Ship it.\n\nline2\nline3\nline4\n")

	out, stderr, err := runTodo(t, vault, "list", "--json")
	if err != nil {
		t.Fatalf("rk todo list --json: %v\nstderr: %s", err, stderr)
	}

	var res struct {
		Items []map[string]any `json:"items"`
	}
	mustDecodeJSON(t, out, &res)

	var found map[string]any
	for _, it := range res.Items {
		if it["id"] == id {
			found = it
		}
	}
	if found == nil {
		t.Fatalf("item %q missing from list JSON: %v", id, res.Items)
	}
	if title, _ := found["title"].(string); title != "Ship it." {
		t.Errorf("title = %v, want %q (item=%v)", found["title"], "Ship it.", found)
	}
	body, _ := found["body"].(string)
	for _, want := range []string{"Ship it.", "line2", "line3", "line4"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q, got %q", want, body)
		}
	}
}

// TestTodoList_Ephemeral_Unaffected: ephemeral checklist rows render exactly
// as before -- no Title-specific behavior is expected/asserted for
// Kind=="ephemeral" (each item's text is already inherently single-line).
// This is a regression guard, not a red-state test: it is expected to PASS
// both before and after implementation (mirrors TestTodoRoundTrip_RenderStability's
// legitimate-pass-in-red-state precedent in this file).
func TestTodoList_Ephemeral_Unaffected(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeEphemeralContainer(t, vault, node.Mint(),
		checklistLine(false, "call dentist"),
	)

	out, stderr, err := runTodo(t, vault, "list", "--json")
	if err != nil {
		t.Fatalf("rk todo list --json: %v\nstderr: %s", err, stderr)
	}
	var res todoListResult
	mustDecodeJSON(t, out, &res)

	found := false
	for _, it := range res.Items {
		if it.Kind == "ephemeral" && strings.Contains(it.Body, "call dentist") {
			found = true
			if it.Checked {
				t.Errorf("ephemeral item unexpectedly checked: %+v", it)
			}
		}
	}
	if !found {
		t.Errorf("ephemeral item missing from list: %+v", res.Items)
	}
}

// TestTodoList_MultiLineBody_RoundTripByteIdentical (AC8b): a multi-line-body
// todo's file bytes are unchanged after the reconcile `rk todo list` triggers
// -- round-trip byte-identity is a structural guarantee of node.Parse/Serialize
// being untouched by this ticket, not new behavior. Mirrors
// TestTodoDone_DurableBytePreservation's byte-comparison style. This is a
// regression/confirmation guard expected to PASS both before and after
// implementation (AC8b: "fails only if some part of this change is wired
// through Render/SetField or otherwise touches Raw, which it should not").
func TestTodoList_MultiLineBody_RoundTripByteIdentical(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	path, src := writeTodoFixture(t, vault, id, "open", "", "Ship it.\n\nline2\nline3\nline4\n")

	if _, stderr, err := runTodo(t, vault, "list"); err != nil {
		t.Fatalf("rk todo list: %v\nstderr: %s", err, stderr)
	}

	got := mustReadFile(t, path)
	if got != src {
		t.Fatalf("todo file bytes changed by rk todo list (reconcile)\n--- want ---\n%q\n--- got ---\n%q", src, got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-3 (done) — T-3.1..T-3.6
// ─────────────────────────────────────────────────────────────────────────────

// T-3.1 (EC-8): durable done flips only the state span; every other byte
// (extra frontmatter, code fence, blank lines) is untouched.
func TestTodoDone_DurableBytePreservation(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	src := "---\n" +
		"id: " + id + "\n" +
		"type: todo\n" +
		"state: open\n" +
		"x-obsidian-extra: keep-me\n" +
		"---\n" +
		"Ship the thing.\n\n" +
		"```go\n" +
		"fmt.Println(\"hi\")\n" +
		"```\n"
	path := filepath.Join(vault, "todos", id+".md")
	mustWriteFile(t, path, src)

	out, stderr, err := runTodo(t, vault, "done", id, "--json")
	if err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}
	var res todoDoneResult
	mustDecodeJSON(t, out, &res)
	if res.Skipped {
		t.Error("Skipped = true, want a real completion")
	}

	got := mustReadFile(t, path)
	want := strings.Replace(src, "state: open", "state: done", 1)
	if got != want {
		t.Fatalf("done() disturbed bytes beyond the state field\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
}

// T-3.2: durable done resolves via an alias.
func TestTodoDone_DurableViaAlias(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	writeTestNode(t, vault, "todos/"+id+".md", id, "todo", "Buy milk",
		"state: open", "aliases: [buy-milk]")

	if _, stderr, err := runTodo(t, vault, "done", "buy-milk"); err != nil {
		t.Fatalf("rk todo done buy-milk: %v\nstderr: %s", err, stderr)
	}

	n, err := node.Parse([]byte(mustReadFile(t, filepath.Join(vault, "todos", id+".md"))))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if n.Props["state"] != "done" {
		t.Errorf("Props[state] = %q, want done", n.Props["state"])
	}
}

// T-3.3 (EC-1): done on an already-done item is an idempotent no-op — no
// rewrite, no error.
func TestTodoDone_IdempotentAlreadyDone(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	src := "---\nid: " + id + "\ntype: todo\nstate: done\n---\nAlready finished.\n"
	path := filepath.Join(vault, "todos", id+".md")
	mustWriteFile(t, path, src)

	out, stderr, err := runTodo(t, vault, "done", id, "--json")
	if err != nil {
		t.Fatalf("rk todo done on an already-done item: %v\nstderr: %s", err, stderr)
	}
	var res todoDoneResult
	mustDecodeJSON(t, out, &res)
	if !res.Skipped {
		t.Error("Skipped = false, want true (idempotent no-op)")
	}
	if got := mustReadFile(t, path); got != src {
		t.Fatalf("already-done file mutated on a no-op done\n--- want ---\n%q\n--- got ---\n%q", src, got)
	}
}

// T-3.4: a non-existent ref is a distinct, non-zero-exit error (not a skip).
func TestTodoDone_NonExistentRefErrors(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	_, stderr, err := runTodo(t, vault, "done", "nonexistent-alias")
	if err == nil {
		t.Fatal("expected an error for a nonexistent ref, got nil")
	}
	combined := strings.ToLower(stderr + err.Error())
	if !strings.Contains(combined, "not found") &&
		!strings.Contains(combined, "no such") &&
		!strings.Contains(combined, "unresolved") {
		t.Errorf("expected a 'not found'-style error, got stderr=%q err=%v", stderr, err)
	}
}

// T-3.5: ephemeral done flips exactly the targeted 1-based checkbox line,
// leaves siblings untouched, introduces no ULID, and is idempotent.
func TestTodoDone_EphemeralFlipsCheckbox(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	writeEphemeralContainer(t, vault, node.Mint(),
		checklistLine(false, "first item"),
		checklistLine(false, "second item"),
	)
	containerPath := filepath.Join(vault, "todos", "inbox.md")
	before := mustReadFile(t, containerPath)

	out, stderr, err := runTodo(t, vault, "done", "1", "--ephemeral", "--json")
	if err != nil {
		t.Fatalf("rk todo done --ephemeral 1: %v\nstderr: %s", err, stderr)
	}
	var res todoDoneResult
	mustDecodeJSON(t, out, &res)
	if res.Skipped {
		t.Error("Skipped = true on first flip, want a real completion")
	}

	after := mustReadFile(t, containerPath)
	if !strings.Contains(after, "- [x] first item") {
		t.Errorf("item 1 not flipped to checked: %q", after)
	}
	if !strings.Contains(after, "- [ ] second item") {
		t.Errorf("item 2 (untouched) missing/changed: %q", after)
	}
	beforeIDCount := strings.Count(before, "id:")
	afterIDCount := strings.Count(after, "id:")
	if afterIDCount != beforeIDCount {
		t.Errorf("id: occurrence count changed from %d to %d — a new identity was introduced", beforeIDCount, afterIDCount)
	}

	resetCLIFlags()
	out2, stderr2, err2 := runTodo(t, vault, "done", "1", "--ephemeral", "--json")
	if err2 != nil {
		t.Fatalf("second rk todo done --ephemeral 1: %v\nstderr: %s", err2, stderr2)
	}
	var res2 todoDoneResult
	mustDecodeJSON(t, out2, &res2)
	if !res2.Skipped {
		t.Error("second done on an already-checked item: Skipped = false, want true")
	}
	if got := mustReadFile(t, containerPath); got != after {
		t.Fatalf("idempotent ephemeral done rewrote bytes\n--- want ---\n%q\n--- got ---\n%q", after, got)
	}
}

// T-3.6: after done, list (Open+Reconcile) no longer shows the item by
// default, but --all shows it with state:done.
func TestTodoDone_ReflectedInList(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	writeTestNode(t, vault, "todos/"+id+".md", id, "todo", "Ship v1", "state: open")

	if _, stderr, err := runTodo(t, vault, "done", id); err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	out, stderr, err := runTodo(t, vault, "list", "--json")
	if err != nil {
		t.Fatalf("rk todo list: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	var res todoListResult
	mustDecodeJSON(t, out, &res)
	if containsID(res.Items, id) {
		t.Errorf("done item still shown by default list: %+v", res.Items)
	}

	outAll, stderr, err := runTodo(t, vault, "list", "--all", "--json")
	if err != nil {
		t.Fatalf("rk todo list --all: %v\nstderr: %s", err, stderr)
	}
	var resAll todoListResult
	mustDecodeJSON(t, outAll, &resAll)
	found := false
	for _, it := range resAll.Items {
		if it.ID == id {
			found = true
			if it.State != "done" {
				t.Errorf("state = %q, want done", it.State)
			}
		}
	}
	if !found {
		t.Errorf("done item missing from --all list: %+v", resAll.Items)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-4 (round-trip) — T-4.1..T-4.3
// ─────────────────────────────────────────────────────────────────────────────

// T-4.1: parse -> serialize identity on any tool-written durable todo, with
// and without optional props/deps.
func TestTodoRoundTrip_ParseSerializeIdentity(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"bare", []string{"add", "Bare todo"}},
		{"with props", []string{"add", "Scheduled todo", "--scheduled", "2026-07-10", "--deadline", "2026-07-15"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vault, _ := setupQueryVault(t)
			t.Cleanup(resetCLIFlags)

			out, stderr, err := runTodo(t, vault, append(append([]string{}, tc.args...), "--json")...)
			if err != nil {
				t.Fatalf("rk todo add: %v\nstderr: %s", err, stderr)
			}
			var res todoAddResult
			mustDecodeJSON(t, out, &res)

			raw, err := os.ReadFile(filepath.Join(vault, res.Path))
			if err != nil {
				t.Fatalf("read created file: %v", err)
			}
			n, err := node.Parse(raw)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !bytes.Equal(n.Serialize(), raw) {
				t.Errorf("parse(raw).Serialize() != raw\n--- raw ---\n%q\n--- serialize ---\n%q", raw, n.Serialize())
			}
		})
	}

	t.Run("with depends", func(t *testing.T) {
		vault, _ := setupQueryVault(t)
		t.Cleanup(resetCLIFlags)

		aID := node.Mint()
		writeTestNode(t, vault, "todos/"+aID+".md", aID, "todo", "Existing", "state: open")

		out, stderr, err := runTodo(t, vault, "add", "Blocked", "--depends", aID, "--json")
		if err != nil {
			t.Fatalf("rk todo add --depends: %v\nstderr: %s", err, stderr)
		}
		var res todoAddResult
		mustDecodeJSON(t, out, &res)

		raw, err := os.ReadFile(filepath.Join(vault, res.Path))
		if err != nil {
			t.Fatalf("read created file: %v", err)
		}
		n, err := node.Parse(raw)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if !bytes.Equal(n.Serialize(), raw) {
			t.Errorf("parse(raw).Serialize() != raw\n--- raw ---\n%q\n--- serialize ---\n%q", raw, n.Serialize())
		}
	})
}

// T-4.2: render round-trip stability (node package invariant #6) exercised
// via the exact construction recipe plan.md D1 commits `add` to (NewNode ->
// set Props/Links -> Render -> Parse). Like TestQueryTS11_ReadOnlyConnection
// in query_test.go, this test does not go through RootCmd/todo.go at all, so
// it can legitimately PASS even in today's pre-todo.go red state — that's
// intentional, not a bug in this file; it pins the underlying node-package
// guarantee `add`'s implementation must rely on rather than reimplement.
func TestTodoRoundTrip_RenderStability(t *testing.T) {
	n := node.NewNode("todo", "mike", "Buy milk\n")
	n.Time = "2026-07-04T12:00:00Z"
	n.Props = map[string]string{"state": "open", "scheduled": "2026-07-10"}
	n.Links = []node.Link{{Rel: "depends-on", To: "01JEXAMPLE0000000000000000"}}

	rendered := n.Render()
	parsed, err := node.Parse(rendered)
	if err != nil {
		t.Fatalf("parse(render(n)): %v", err)
	}
	if !bytes.Equal(parsed.Serialize(), rendered) {
		t.Errorf("serialize(parse(render(n))) != render(n)\n--- rendered ---\n%q\n--- serialized ---\n%q",
			rendered, parsed.Serialize())
	}
	if reRendered := parsed.Render(); !bytes.Equal(reRendered, rendered) {
		t.Errorf("render(parse(render(n))) != render(n)\n--- first ---\n%q\n--- second ---\n%q", rendered, reRendered)
	}
}

// T-4.3 (EC-8): the edit path is itself round-trip-stable — the post-`done`
// file is stable under a further parse/serialize round trip.
func TestTodoRoundTrip_EditPathStability(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	src := "---\n" +
		"id: " + id + "\n" +
		"type: todo\n" +
		"state: open\n" +
		"x-custom: keep\n" +
		"---\n" +
		"> a blockquote\n\n" +
		"```text\nfenced\n```\n"
	path := filepath.Join(vault, "todos", id+".md")
	mustWriteFile(t, path, src)

	if _, stderr, err := runTodo(t, vault, "done", id); err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}

	edited, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read edited file: %v", err)
	}
	n, err := node.Parse(edited)
	if err != nil {
		t.Fatalf("parse edited file: %v", err)
	}
	again, err := node.Parse(n.Serialize())
	if err != nil {
		t.Fatalf("re-parse serialized bytes: %v", err)
	}
	if !bytes.Equal(again.Serialize(), edited) {
		t.Errorf("parse(serialize(parse(file))) != post-edit file\n--- edited ---\n%q\n--- roundtrip ---\n%q",
			edited, again.Serialize())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-5 (depends-on edges) — T-5.1..T-5.4
// ─────────────────────────────────────────────────────────────────────────────

// T-5.1: a live --depends target produces a resolved depends-on edge after
// indexing.
func TestTodoDependsOn_EdgeIndexed(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	aID := node.Mint()
	writeTestNode(t, vault, "todos/"+aID+".md", aID, "todo", "Todo A", "state: open")

	out, stderr, err := runTodo(t, vault, "add", "Blocked", "--depends", aID, "--json")
	if err != nil {
		t.Fatalf("rk todo add --depends: %v\nstderr: %s", err, stderr)
	}
	var res todoAddResult
	mustDecodeJSON(t, out, &res)
	resetCLIFlags()

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, fmt.Sprintf(
		"SELECT dst, dst_key FROM edges WHERE rel='depends-on' AND src='%s'", res.ID))
	if err != nil {
		t.Fatalf("rk query edges: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 depends-on edge, got %d: %v", len(rows), rows)
	}
	if rows[0]["dst"] != aID {
		t.Errorf("dst = %v, want %q", rows[0]["dst"], aID)
	}
	if rows[0]["dst_key"] == nil || rows[0]["dst_key"] == "" {
		t.Errorf("dst_key = %v, want resolved (non-NULL)", rows[0]["dst_key"])
	}
}

// T-5.2: the depends-on ref-valued prop never leaks into node_props.
func TestTodoDependsOn_NotInNodeProps(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	aID := node.Mint()
	writeTestNode(t, vault, "todos/"+aID+".md", aID, "todo", "Todo A", "state: open")

	out, stderr, err := runTodo(t, vault, "add", "Blocked", "--depends", aID, "--json")
	if err != nil {
		t.Fatalf("rk todo add --depends: %v\nstderr: %s", err, stderr)
	}
	var res todoAddResult
	mustDecodeJSON(t, out, &res)
	resetCLIFlags()

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault, fmt.Sprintf(
		"SELECT * FROM node_props WHERE id='%s' AND key='depends-on'", res.ID))
	if err != nil {
		t.Fatalf("rk query node_props: %v\nstderr: %s", err, stderr)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("depends-on leaked into node_props: %q", stdout)
	}
}

// T-5.3 (EC-2): --depends on a nonexistent target is accepted; it produces a
// dangling edge (dst_key NULL), no indexing error.
func TestTodoDependsOn_DanglingEdge(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runTodo(t, vault, "add", "Blocked", "--depends", "nonexistent-ulid-or-alias", "--json")
	if err != nil {
		t.Fatalf("rk todo add --depends nonexistent: %v\nstderr: %s", err, stderr)
	}
	var res todoAddResult
	mustDecodeJSON(t, out, &res)
	resetCLIFlags()

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault,
		"SELECT dst, dst_key FROM edges WHERE rel='depends-on' AND dst='nonexistent-ulid-or-alias'")
	if err != nil {
		t.Fatalf("rk query dangling edge: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	if len(rows) != 1 {
		t.Fatalf("expected exactly 1 dangling edge, got %d: %v", len(rows), rows)
	}
	if rows[0]["dst_key"] != nil {
		t.Errorf("dst_key = %v, want NULL (dangling)", rows[0]["dst_key"])
	}
}

// T-5.4: a dangling dependency later resolves once its target is created and
// the index rebuilt.
func TestTodoDependsOn_DanglingResolvesLater(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	target := node.Mint() // a valid-looking ULID that does not exist yet
	if _, stderr, err := runTodo(t, vault, "add", "Blocked", "--depends", target); err != nil {
		t.Fatalf("rk todo add --depends: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	buildIndex(t, vault)

	stdoutBefore, _, err := runQuery(t, vault, fmt.Sprintf(
		"SELECT dst_key FROM edges WHERE rel='depends-on' AND dst='%s'", target))
	if err != nil {
		t.Fatalf("rk query before target exists: %v", err)
	}
	resetCLIFlags()
	rowsBefore := parseNDJSONMaps(t, stdoutBefore)
	if len(rowsBefore) != 1 || rowsBefore[0]["dst_key"] != nil {
		t.Fatalf("expected 1 dangling edge before target exists, got %v", rowsBefore)
	}

	writeTestNode(t, vault, "todos/"+target+".md", target, "todo", "Now it exists", "state: open")
	buildIndex(t, vault)

	stdoutAfter, _, err := runQuery(t, vault, fmt.Sprintf(
		"SELECT dst_key FROM edges WHERE rel='depends-on' AND dst='%s'", target))
	if err != nil {
		t.Fatalf("rk query after target exists: %v", err)
	}
	rowsAfter := parseNDJSONMaps(t, stdoutAfter)
	if len(rowsAfter) != 1 || rowsAfter[0]["dst_key"] == nil {
		t.Fatalf("expected the edge to resolve once its target exists, got %v", rowsAfter)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-6 (queryable distinction, index level) — T-6.1
// ─────────────────────────────────────────────────────────────────────────────

// T-6.1: durable vs ephemeral-container rows are separately countable via
// plain SQL against the public nodes view.
func TestTodoQueryable_TypeCountsAtIndexLevel(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	if _, stderr, err := runTodo(t, vault, "add", "Durable one"); err != nil {
		t.Fatalf("add durable: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()
	if _, stderr, err := runTodo(t, vault, "add", "Ephemeral one", "--ephemeral"); err != nil {
		t.Fatalf("add ephemeral: %v\nstderr: %s", err, stderr)
	}
	resetCLIFlags()

	buildIndex(t, vault)

	stdout, stderr, err := runQuery(t, vault,
		"SELECT type, count(*) AS n FROM nodes WHERE type IN ('todo','todo-ephemeral') GROUP BY type")
	if err != nil {
		t.Fatalf("rk query type counts: %v\nstderr: %s", err, stderr)
	}
	rows := parseNDJSONMaps(t, stdout)
	counts := map[string]string{}
	for _, r := range rows {
		counts[fmt.Sprintf("%v", r["type"])] = fmt.Sprintf("%v", r["n"])
	}
	if counts["todo"] != "1" {
		t.Errorf("todo count = %v, want 1 (rows=%v)", counts["todo"], rows)
	}
	if counts["todo-ephemeral"] != "1" {
		t.Errorf("todo-ephemeral count = %v, want 1 (rows=%v)", counts["todo-ephemeral"], rows)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// AC-7 (harness / plumbing) — T-7.1..T-7.3
// ─────────────────────────────────────────────────────────────────────────────

// T-7.1: runTodo drives add -> list -> done end to end through the shared
// RootCmd singleton, resetting CLI flag state between Execute calls.
func TestTodoHarness_RunTodoDrivesAddListDone(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runTodo(t, vault, "add", "Round trip via harness", "--json")
	if err != nil {
		t.Fatalf("add: %v\nstderr: %s", err, stderr)
	}
	var addRes todoAddResult
	mustDecodeJSON(t, out, &addRes)
	resetCLIFlags()

	listOut, stderr, err := runTodo(t, vault, "list", "--json")
	if err != nil {
		t.Fatalf("list: %v\nstderr: %s", err, stderr)
	}
	var listRes todoListResult
	mustDecodeJSON(t, listOut, &listRes)
	if !containsID(listRes.Items, addRes.ID) {
		t.Fatalf("added item missing from list: %+v", listRes.Items)
	}
	resetCLIFlags()

	doneOut, stderr, err := runTodo(t, vault, "done", addRes.ID, "--json")
	if err != nil {
		t.Fatalf("done: %v\nstderr: %s", err, stderr)
	}
	var doneRes todoDoneResult
	mustDecodeJSON(t, doneOut, &doneRes)
	if doneRes.Skipped {
		t.Error("Skipped = true on a fresh completion, want false")
	}
}

// T-7.2: --quiet suppresses the pretty status line on add/done without
// affecting the underlying write.
func TestTodoAdd_QuietSuppressesPrettyStatusLine(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	out, stderr, err := runTodo(t, vault, "add", "quiet please", "--quiet")
	if err != nil {
		t.Fatalf("rk todo add --quiet: %v\nstderr: %s", err, stderr)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("--quiet did not suppress the pretty status line: %q", out)
	}

	entries, err := os.ReadDir(filepath.Join(vault, "todos"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected exactly one file written despite --quiet: err=%v entries=%v", err, entries)
	}
}

func TestTodoDone_QuietSuppressesPrettyStatusLine(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	writeTestNode(t, vault, "todos/"+id+".md", id, "todo", "quiet done", "state: open")

	out, stderr, err := runTodo(t, vault, "done", id, "--quiet")
	if err != nil {
		t.Fatalf("rk todo done --quiet: %v\nstderr: %s", err, stderr)
	}
	if strings.TrimSpace(out) != "" {
		t.Errorf("--quiet did not suppress the pretty status line: %q", out)
	}

	n, err := node.Parse([]byte(mustReadFile(t, filepath.Join(vault, "todos", id+".md"))))
	if err != nil || n.Props["state"] != "done" {
		t.Fatalf("file not marked done despite --quiet: err=%v props=%v", err, n.Props)
	}
}

// T-7.3: `done` never opens/creates the index cache dir (mirrors
// TestAdoptCmd_NeverTouchesIndex in adopt_test.go).
func TestTodoDone_NeverTouchesIndex(t *testing.T) {
	vault, cache := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	writeTestNode(t, vault, "todos/"+id+".md", id, "todo", "no index yet", "state: open")

	if _, stderr, err := runTodo(t, vault, "done", id); err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}

	if _, err := os.Stat(cache); err == nil {
		t.Errorf("rk todo done created the cache dir %q; done must never touch the index", cache)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat cache dir: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// v1-T7 (reckon-liml) plan.md decision B4 — `state: in-progress` visibility.
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoList_InProgressVisibleByDefault (plan.md decision B4): once T7
// lands, an `state: in-progress` durable todo (a new state this ticket
// introduces via `rk today act <ref> i`) must remain visible in `rk todo
// list`'s default (no --all) filter, alongside `open` -- otherwise an item
// started from the agenda would vanish from `rk todo list`'s default view
// even though it is not done/cancelled (the only two terminal states).
//
// RED today (pre-T7): listDurableTodos's default hide-filter is literally
// `!all && state != "open"` (todo.go:519), which hides `in-progress` by
// default just like it hides `done` -- this test fails until that filter is
// loosened to keep both `open` and `in-progress` visible by default (only
// `done`/`cancelled` stay hidden).
func TestTodoList_InProgressVisibleByDefault(t *testing.T) {
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)

	id := node.Mint()
	writeTestNode(t, vault, "todos/"+id+".md", id, "todo", "In progress task", "state: in-progress")

	out, stderr, err := runTodo(t, vault, "list", "--json")
	if err != nil {
		t.Fatalf("rk todo list: %v\nstderr: %s", err, stderr)
	}
	var res todoListResult
	mustDecodeJSON(t, out, &res)

	if !containsID(res.Items, id) {
		t.Errorf("in-progress item %q missing from default (no --all) list: %+v", id, res.Items)
	}
}
