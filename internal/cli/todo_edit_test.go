// Package cli — TDD red tests for `rk todo edit <ref>`: set or clear a
// durable todo's deadline, scheduled, priority and depends-on in one atomic,
// byte-preserving write.
//
// The command is driven only through runTodo("edit", ...) and decoded into
// generic maps, so this file compiles before the implementation exists. Every
// refusal test asserts the specific error text first and then the bytes; an
// unknown-command error therefore cannot satisfy it.
//
// Behaviour pinned here:
//   - value flags: --deadline D, --scheduled D|tomorrow|next-week,
//     --priority A|B|C, --depends REF; clear flags: --no-deadline,
//     --no-scheduled, --no-priority, --no-depends. A given-but-empty value is
//     an error, and a value flag combined with its own --no-<field> conflicts.
//   - fixed apply order, so new keys land (first line first) as depends-on,
//     priority, scheduled, deadline.
//   - JSON/NDJSON result: kind, ref, id, path, state plus the set fields and a
//     "cleared" list; pretty form `todo: <ref> updated (deadline X, priority
//     Y, cleared scheduled)`.
//   - an unchanged result skips the write; a plain edit treats index failure
//     after the write as a warning, --depends treats it as fatal.
package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/MikeBiancalana/reckon/internal/node"
)

// ─────────────────────────────────────────────────────────────────────────────
// Harness
// ─────────────────────────────────────────────────────────────────────────────

// newEditVault returns a fresh vault with the cache env set and CLI flag state
// reset at cleanup.
func newEditVault(t *testing.T) string {
	t.Helper()
	vault, _ := setupQueryVault(t)
	t.Cleanup(resetCLIFlags)
	return vault
}

// runEdit runs `rk todo edit args...` and resets CLI state so it can be called
// repeatedly within one test.
func runEdit(t *testing.T, vault string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	stdout, stderr, err = runTodo(t, vault, append([]string{"edit"}, args...)...)
	resetCLIFlags()
	return stdout, stderr, err
}

// mustEdit runs an edit that must succeed and returns stdout.
func mustEdit(t *testing.T, vault string, args ...string) string {
	t.Helper()
	stdout, stderr, err := runEdit(t, vault, args...)
	if err != nil {
		t.Fatalf("rk todo edit %v: %v\nstderr: %s", args, err, stderr)
	}
	return stdout
}

// requireEditRefused runs an edit that must fail with every wantSubstr in its
// error text, then verifies the whole vault is byte-identical.
func requireEditRefused(t *testing.T, vault string, wantSubstrs []string, args ...string) {
	t.Helper()
	before := snapshotVaultFiles(t, vault)
	_, stderr, err := runEdit(t, vault, args...)
	if err == nil {
		t.Fatalf("rk todo edit %v: expected an error, got nil", args)
	}
	text := stderr + err.Error()
	for _, s := range wantSubstrs {
		if !strings.Contains(text, s) {
			t.Errorf("rk todo edit %v: error text missing %q\ngot: %q", args, s, text)
		}
	}
	if after := snapshotVaultFiles(t, vault); !vaultSnapshotsEqual(before, after) {
		t.Errorf("rk todo edit %v: vault mutated by a refused edit", args)
	}
}

func requireFile(t *testing.T, path, want string) {
	t.Helper()
	if got := mustReadFile(t, path); got != want {
		t.Fatalf("file bytes differ\n--- want ---\n%q\n--- got ---\n%q", want, got)
	}
}

func decodeMap(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("decode JSON object: %v\nstdout: %q", err, s)
	}
	return m
}

// withFirstLine returns src with a new frontmatter line inserted as the first
// line inside the fence.
func withFirstLine(src, line string) string {
	return "---\n" + line + "\n" + strings.TrimPrefix(src, "---\n")
}

// queryRows runs a read-only rk query and returns its rows.
func queryRows(t *testing.T, vault, sql string) []map[string]any {
	t.Helper()
	stdout, stderr, err := runQuery(t, vault, sql)
	resetCLIFlags()
	if err != nil {
		t.Fatalf("rk query %q: %v\nstderr: %s", sql, err, stderr)
	}
	return parseNDJSONMaps(t, stdout)
}

// newTodo writes a plain open todo and returns (id, path, src).
func newTodo(t *testing.T, vault, body string, extraFM ...string) (id, path, src string) {
	t.Helper()
	id = node.Mint()
	path, src = writeTodoFixture(t, vault, id, "open", "", body, extraFM...)
	return id, path, src
}

// ─────────────────────────────────────────────────────────────────────────────
// Set: single fields
// ─────────────────────────────────────────────────────────────────────────────

func TestTodoEdit_DeadlineInsertsExactBytes(t *testing.T) {
	vault := newEditVault(t)
	id, path, src := newTodo(t, vault, "Ship it.", "x-obsidian-extra: keep-me")

	out := mustEdit(t, vault, id, "--deadline", "2026-08-01", "--json")

	requireFile(t, path, withFirstLine(src, "deadline: 2026-08-01"))
	res := decodeMap(t, out)
	if res["deadline"] != "2026-08-01" {
		t.Errorf("JSON deadline = %v, want 2026-08-01", res["deadline"])
	}
}

func TestTodoEdit_DeadlineReplacesWithoutDuplicating(t *testing.T) {
	vault := newEditVault(t)
	id, path, src := newTodo(t, vault, "Ship it.", "deadline: 2026-07-01")

	mustEdit(t, vault, id, "--deadline", "2026-08-01")

	want := strings.Replace(src, "deadline: 2026-07-01", "deadline: 2026-08-01", 1)
	requireFile(t, path, want)
	if strings.Count(mustReadFile(t, path), "deadline:") != 1 {
		t.Error("deadline key duplicated")
	}
}

func TestTodoEdit_ScheduledInsertReplaceAndShorthand(t *testing.T) {
	t.Run("insert", func(t *testing.T) {
		vault := newEditVault(t)
		id, path, src := newTodo(t, vault, "Body")
		mustEdit(t, vault, id, "--scheduled", "2026-08-02")
		requireFile(t, path, withFirstLine(src, "scheduled: 2026-08-02"))
	})
	t.Run("replace", func(t *testing.T) {
		vault := newEditVault(t)
		id := node.Mint()
		path, src := writeTodoFixture(t, vault, id, "open", "2026-07-01", "Body")
		mustEdit(t, vault, id, "--scheduled", "2026-08-02")
		requireFile(t, path, strings.Replace(src, "scheduled: 2026-07-01", "scheduled: 2026-08-02", 1))
	})
	t.Run("tomorrow", func(t *testing.T) {
		vault := newEditVault(t)
		pinTodoNow(t, "2026-07-10")
		id, path, src := newTodo(t, vault, "Body")
		mustEdit(t, vault, id, "--scheduled", "tomorrow")
		requireFile(t, path, withFirstLine(src, "scheduled: 2026-07-11"))
	})
	t.Run("next-week", func(t *testing.T) {
		vault := newEditVault(t)
		pinTodoNow(t, "2026-07-10")
		id, path, src := newTodo(t, vault, "Body")
		mustEdit(t, vault, id, "--scheduled", "next-week")
		requireFile(t, path, withFirstLine(src, "scheduled: 2026-07-17"))
	})
}

func TestTodoEdit_DeadlineRejectsShorthand(t *testing.T) {
	vault := newEditVault(t)
	pinTodoNow(t, "2026-07-10")
	id, _, _ := newTodo(t, vault, "Body")
	requireEditRefused(t, vault, []string{"todo edit: --deadline"}, id, "--deadline", "tomorrow")
}

func TestTodoEdit_PriorityInsertAndReplace(t *testing.T) {
	t.Run("insert", func(t *testing.T) {
		vault := newEditVault(t)
		id, path, src := newTodo(t, vault, "Body")
		mustEdit(t, vault, id, "--priority", "B")
		requireFile(t, path, withFirstLine(src, "priority: B"))
	})
	t.Run("replace", func(t *testing.T) {
		vault := newEditVault(t)
		id, path, src := newTodo(t, vault, "Body", "priority: C")
		mustEdit(t, vault, id, "--priority", "A")
		requireFile(t, path, strings.Replace(src, "priority: C", "priority: A", 1))
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Set: depends
// ─────────────────────────────────────────────────────────────────────────────

func TestTodoEdit_DependsInsertsCanonicalLink(t *testing.T) {
	vault := newEditVault(t)
	aID, _, _ := newTodo(t, vault, "Target A")
	id, path, src := newTodo(t, vault, "Blocked")

	mustEdit(t, vault, id, "--depends", aID)

	requireFile(t, path, withFirstLine(src, `depends-on: "[[`+aID+`]]"`))
	n, err := node.Parse([]byte(mustReadFile(t, path)))
	if err != nil {
		t.Fatalf("parse edited file: %v", err)
	}
	if len(n.Links) != 1 || n.Links[0].Rel != "depends-on" || n.Links[0].To != aID {
		t.Errorf("Links = %+v, want single depends-on -> %s", n.Links, aID)
	}
	if _, has := n.Props["depends-on"]; has {
		t.Errorf("depends-on leaked into Props: %+v", n.Props)
	}
}

func TestTodoEdit_DependsReplacesAndIndexFollows(t *testing.T) {
	vault := newEditVault(t)
	aID, _, _ := newTodo(t, vault, "Target A")
	bID, _, _ := newTodo(t, vault, "Target B")
	id, path, src := newTodo(t, vault, "Blocked", `depends-on: "[[`+aID+`]]"`)
	buildIndex(t, vault)

	mustEdit(t, vault, id, "--depends", bID)

	requireFile(t, path, strings.Replace(src, "[["+aID+"]]", "[["+bID+"]]", 1))
	n, err := node.Parse([]byte(mustReadFile(t, path)))
	if err != nil {
		t.Fatalf("parse edited file: %v", err)
	}
	if len(n.Links) != 1 || n.Links[0].To != bID {
		t.Errorf("Links = %+v, want only -> %s", n.Links, bID)
	}

	rows := queryRows(t, vault, fmt.Sprintf(
		"SELECT dst FROM edges WHERE rel='depends-on' AND src='%s'", id))
	if len(rows) != 1 || rows[0]["dst"] != bID {
		t.Errorf("edges after edit = %v, want exactly -> %s (no manual rebuild)", rows, bID)
	}
}

func TestTodoEdit_DependsCollapsesOddShapesToOneLine(t *testing.T) {
	shapes := map[string]func(a, b string) []string{
		"multi_target": func(a, b string) []string {
			return []string{`depends-on: "[[` + a + `]]", "[[` + b + `]]"`}
		},
		"unquoted": func(a, b string) []string {
			return []string{`depends-on: [[` + a + `]]`}
		},
		"block_list": func(a, b string) []string {
			return []string{"depends-on:", `  - "[[` + a + `]]"`, `  - "[[` + b + `]]"`}
		},
	}
	for name, lines := range shapes {
		t.Run(name, func(t *testing.T) {
			vault := newEditVault(t)
			aID, _, _ := newTodo(t, vault, "A")
			bID, _, _ := newTodo(t, vault, "B")
			xID, _, _ := newTodo(t, vault, "X")
			id, path, _ := newTodo(t, vault, "Blocked", lines(aID, bID)...)

			mustEdit(t, vault, id, "--depends", xID)

			want := todoFixtureSrc(id, "open", "", "Blocked", `depends-on: "[[`+xID+`]]"`)
			requireFile(t, path, want)
			n, err := node.Parse([]byte(mustReadFile(t, path)))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(n.Links) != 1 || n.Links[0].To != xID {
				t.Errorf("Links = %+v, want single -> %s", n.Links, xID)
			}
		})
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Refs, multi-field, exotic frontmatter
// ─────────────────────────────────────────────────────────────────────────────

func TestTodoEdit_AliasRefEditsSameFileAsULID(t *testing.T) {
	vault := newEditVault(t)
	id, path, src := newTodo(t, vault, "Body", "aliases: [my-todo]")

	out := mustEdit(t, vault, "my-todo", "--deadline", "2026-08-01", "--json")

	requireFile(t, path, withFirstLine(src, "deadline: 2026-08-01"))
	res := decodeMap(t, out)
	if res["id"] != id || res["path"] != "todos/"+id+".md" {
		t.Errorf("result id/path = %v/%v, want %s/todos/%s.md", res["id"], res["path"], id, id)
	}
}

func TestTodoEdit_AllFourFlagsOneDeterministicWrite(t *testing.T) {
	orders := map[string]func(a string) []string{
		"order_1": func(a string) []string {
			return []string{"--depends", a, "--priority", "B", "--deadline", "2026-08-01", "--scheduled", "2026-08-02"}
		},
		"order_2": func(a string) []string {
			return []string{"--scheduled", "2026-08-02", "--deadline", "2026-08-01", "--depends", a, "--priority", "B"}
		},
	}
	for name, argv := range orders {
		t.Run(name, func(t *testing.T) {
			vault := newEditVault(t)
			aID, _, _ := newTodo(t, vault, "A")
			id, path, src := newTodo(t, vault, "Title line\n\nBody para.", "x-obsidian-extra: keep-me")

			mustEdit(t, vault, append([]string{id}, argv(aID)...)...)

			rest := strings.TrimPrefix(src, "---\n")
			want := "---\n" +
				`depends-on: "[[` + aID + `]]"` + "\n" +
				"priority: B\n" +
				"scheduled: 2026-08-02\n" +
				"deadline: 2026-08-01\n" + rest
			requireFile(t, path, want)
		})
	}
}

func TestTodoEdit_ExoticFrontmatterTouchesOnlyTargetLines(t *testing.T) {
	vault := newEditVault(t)
	id := node.Mint()
	src := "---\n# hand-written note\nx-obsidian-extra: keep-me\nid: " + id + "\ntags: [a, b]\ntype: todo\n\nstate: open\npriority: C\n---\nBody stays.\n"
	path := filepath.Join(vault, "todos", id+".md")
	mustWriteFile(t, path, src)

	mustEdit(t, vault, id, "--priority", "A", "--deadline", "2026-08-01")

	want := withFirstLine(strings.Replace(src, "priority: C", "priority: A", 1), "deadline: 2026-08-01")
	requireFile(t, path, want)
}

func TestTodoEdit_DoneTodoStaysDone(t *testing.T) {
	vault := newEditVault(t)
	id := node.Mint()
	path, src := writeTodoFixture(t, vault, id, "done", "", "Finished.")

	mustEdit(t, vault, id, "--deadline", "2026-08-01")

	requireFile(t, path, withFirstLine(src, "deadline: 2026-08-01"))
	n, err := node.Parse([]byte(mustReadFile(t, path)))
	if err != nil || n.Props["state"] != "done" {
		t.Errorf("state = %q (err %v), want done", n.Props["state"], err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Validation and refusals
// ─────────────────────────────────────────────────────────────────────────────

func TestTodoEdit_RejectsInvalidDate(t *testing.T) {
	vault := newEditVault(t)
	id, _, _ := newTodo(t, vault, "Body")
	requireEditRefused(t, vault, []string{"todo edit: --deadline", "2026-02-30"}, id, "--deadline", "2026-02-30")
	requireEditRefused(t, vault, []string{"todo edit: --scheduled", "2026-7-1"}, id, "--scheduled", "2026-7-1")
}

func TestTodoEdit_RejectsInvalidPriority(t *testing.T) {
	vault := newEditVault(t)
	id, _, _ := newTodo(t, vault, "Body")
	for _, bad := range []string{"Z", "a", "1"} {
		requireEditRefused(t, vault, []string{"todo edit: --priority", "(want A, B, or C)"}, id, "--priority", bad)
	}
}

func TestTodoEdit_InvalidFlagBlocksValidSiblingWrite(t *testing.T) {
	vault := newEditVault(t)
	id, _, _ := newTodo(t, vault, "Body")
	requireEditRefused(t, vault, []string{"todo edit: --priority"}, id, "--deadline", "2026-08-01", "--priority", "Z")
}

func TestTodoEdit_NoFlagsNothingToChange(t *testing.T) {
	vault := newEditVault(t)
	id, _, _ := newTodo(t, vault, "Body")
	requireEditRefused(t, vault, []string{"todo edit: nothing to change"}, id)
}

func TestTodoEdit_UnknownRefNotFound(t *testing.T) {
	vault := newEditVault(t)
	newTodo(t, vault, "Body")
	requireEditRefused(t, vault, []string{`todo edit: no todo found matching "nope" (not found)`}, "nope", "--deadline", "2026-08-01")
}

func TestTodoEdit_EphemeralInboxRefNotFound(t *testing.T) {
	vault := newEditVault(t)
	writeEphemeralContainer(t, vault, node.Mint(), checklistLine(false, "buy milk"))
	requireEditRefused(t, vault, []string{`todo edit: no todo found matching "inbox" (not found)`}, "inbox", "--deadline", "2026-08-01")
}

func TestTodoEdit_DependsRejectsSelf(t *testing.T) {
	vault := newEditVault(t)
	id, _, _ := newTodo(t, vault, "Body", "aliases: [me]")
	for _, self := range []string{id, "me"} {
		stderrWant := []string{"todo edit: --depends"}
		before := snapshotVaultFiles(t, vault)
		_, stderr, err := runEdit(t, vault, id, "--depends", self)
		if err == nil {
			t.Fatalf("--depends %s: expected error", self)
		}
		text := stderr + err.Error()
		for _, s := range stderrWant {
			if !strings.Contains(text, s) {
				t.Errorf("--depends %s: missing %q in %q", self, s, text)
			}
		}
		if !strings.Contains(strings.ToLower(text), "self") {
			t.Errorf("--depends %s: error %q should say it depends on itself", self, text)
		}
		if after := snapshotVaultFiles(t, vault); !vaultSnapshotsEqual(before, after) {
			t.Errorf("--depends %s: vault mutated", self)
		}
	}
}

func TestTodoEdit_DependsRejectsMissingTarget(t *testing.T) {
	vault := newEditVault(t)
	id, _, _ := newTodo(t, vault, "Body")
	missing := node.Mint()

	before := snapshotVaultFiles(t, vault)
	_, stderr, err := runEdit(t, vault, id, "--depends", missing)
	if err == nil {
		t.Fatal("expected an error for a nonexistent --depends target")
	}
	text := stderr + err.Error()
	if !strings.Contains(text, "todo edit: --depends") || !strings.Contains(text, missing) {
		t.Errorf("error %q should be a `todo edit: --depends` error naming %s", text, missing)
	}
	if after := snapshotVaultFiles(t, vault); !vaultSnapshotsEqual(before, after) {
		t.Error("vault mutated")
	}
}

func TestTodoEdit_DependsRejectsWikilinkForm(t *testing.T) {
	vault := newEditVault(t)
	aID, _, _ := newTodo(t, vault, "A")
	id, _, _ := newTodo(t, vault, "Body")
	requireEditRefused(t, vault, []string{"todo edit: --depends", "[[" + aID + "]]"}, id, "--depends", "[["+aID+"]]")
}

func TestTodoEdit_DependsRejectsDirectCycle(t *testing.T) {
	vault := newEditVault(t)
	bID := node.Mint()
	aID, _, _ := newTodo(t, vault, "A", `depends-on: "[[`+bID+`]]"`)
	writeTodoFixture(t, vault, bID, "open", "", "B")
	requireEditRefused(t, vault, []string{"todo edit: --depends", "cycle"}, bID, "--depends", aID)
}

func TestTodoEdit_RefusesCRLFFile(t *testing.T) {
	vault := newEditVault(t)
	id := node.Mint()
	path := filepath.Join(vault, "todos", id+".md")
	mustWriteFile(t, path, "---\r\nid: "+id+"\r\ntype: todo\r\nstate: open\r\n---\r\nBody\r\n")
	requireEditRefused(t, vault, []string{"todo edit:", "CRLF line endings are not supported"}, id, "--deadline", "2026-08-01")
}

// ─────────────────────────────────────────────────────────────────────────────
// Recurrence
// ─────────────────────────────────────────────────────────────────────────────

func TestTodoEdit_ScheduledMovesRecurringCursor(t *testing.T) {
	vault := newEditVault(t)
	pinTodoNow(t, "2026-07-05")
	id := node.Mint()
	path, src := writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	mustEdit(t, vault, id, "--scheduled", "2026-07-20")
	requireFile(t, path, strings.Replace(src, "scheduled: 2026-07-05", "scheduled: 2026-07-20", 1))

	out, stderr, err := runTodo(t, vault, "done", id, "--json")
	resetCLIFlags()
	if err != nil {
		t.Fatalf("rk todo done: %v\nstderr: %s", err, stderr)
	}
	var res todoDoneResult
	mustDecodeJSON(t, out, &res)
	if res.Scheduled != "2026-07-27" {
		t.Errorf("done advanced to %q, want 2026-07-27 (from the edited cursor)", res.Scheduled)
	}
}

func TestTodoEdit_NoScheduledRejectedOnRecurring(t *testing.T) {
	vault := newEditVault(t)
	id := node.Mint()
	writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")
	requireEditRefused(t, vault,
		[]string{"todo edit: --no-scheduled: todo has repeat; a recurring todo needs its scheduled cursor"},
		id, "--no-scheduled")
}

func TestTodoEdit_DeadlineAllowedOnRecurring(t *testing.T) {
	vault := newEditVault(t)
	id := node.Mint()
	path, src := writeRecurringTodo(t, vault, id, "2026-07-05", "+7d", "Water plants")

	mustEdit(t, vault, id, "--deadline", "2026-08-01")

	requireFile(t, path, withFirstLine(src, "deadline: 2026-08-01"))
}

// ─────────────────────────────────────────────────────────────────────────────
// Idempotence
// ─────────────────────────────────────────────────────────────────────────────

func TestTodoEdit_RepeatRunIsIdempotentAndSkipsWrite(t *testing.T) {
	vault := newEditVault(t)
	id, path, src := newTodo(t, vault, "Body")

	first := mustEdit(t, vault, id, "--deadline", "2026-08-01", "--json")
	want := withFirstLine(src, "deadline: 2026-08-01")
	requireFile(t, path, want)

	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	second := mustEdit(t, vault, id, "--deadline", "2026-08-01", "--json")
	requireFile(t, path, want)
	if !reflect.DeepEqual(decodeMap(t, first), decodeMap(t, second)) {
		t.Errorf("results differ across identical runs\n%s\n%s", first, second)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !st.ModTime().Equal(old) {
		t.Errorf("mtime changed to %v; an unchanged edit must not rewrite the file", st.ModTime())
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Clear: --no-<field>
// ─────────────────────────────────────────────────────────────────────────────

func TestTodoEdit_EmptyValueRejectedNamingClearFlag(t *testing.T) {
	vault := newEditVault(t)
	id, _, _ := newTodo(t, vault, "Body", "deadline: 2026-07-01")
	for _, f := range []string{"deadline", "scheduled", "priority", "depends"} {
		requireEditRefused(t, vault,
			[]string{"todo edit: --" + f + ": empty value", "--no-" + f},
			id, "--"+f, "")
	}
}

func TestTodoEdit_NoDeadlineRemovesLineAndReportsCleared(t *testing.T) {
	vault := newEditVault(t)
	id, path, _ := newTodo(t, vault, "Body", "deadline: 2026-07-01")

	out := mustEdit(t, vault, id, "--no-deadline", "--json")

	requireFile(t, path, todoFixtureSrc(id, "open", "", "Body"))
	res := decodeMap(t, out)
	if !reflect.DeepEqual(res["cleared"], []any{"deadline"}) {
		t.Errorf("cleared = %v, want [deadline]", res["cleared"])
	}
	if _, has := res["deadline"]; has {
		t.Errorf("deadline key present in result of a clear: %v", res)
	}
}

func TestTodoEdit_NoPriorityAndNoScheduledOneWrite(t *testing.T) {
	vault := newEditVault(t)
	id := node.Mint()
	path, _ := writeTodoFixture(t, vault, id, "open", "2026-07-20", "Body", "priority: B")

	out := mustEdit(t, vault, id, "--no-priority", "--no-scheduled", "--json")

	requireFile(t, path, todoFixtureSrc(id, "open", "", "Body"))
	res := decodeMap(t, out)
	if !reflect.DeepEqual(res["cleared"], []any{"scheduled", "priority"}) {
		t.Errorf("cleared = %v, want [scheduled priority] (fixed field order)", res["cleared"])
	}
}

func TestTodoEdit_NoDependsRemovesEveryShapeAndLinks(t *testing.T) {
	shapes := map[string]func(a, b string) []string{
		"scalar": func(a, b string) []string { return []string{`depends-on: "[[` + a + `]]"`} },
		"multi_target": func(a, b string) []string {
			return []string{`depends-on: "[[` + a + `]]", "[[` + b + `]]"`}
		},
		"block_list": func(a, b string) []string {
			return []string{"depends-on:", `  - "[[` + a + `]]"`, `  - "[[` + b + `]]"`}
		},
	}
	for name, lines := range shapes {
		t.Run(name, func(t *testing.T) {
			vault := newEditVault(t)
			aID, _, _ := newTodo(t, vault, "A")
			bID, _, _ := newTodo(t, vault, "B")
			id, path, _ := newTodo(t, vault, "Blocked", lines(aID, bID)...)
			buildIndex(t, vault)

			mustEdit(t, vault, id, "--no-depends")

			requireFile(t, path, todoFixtureSrc(id, "open", "", "Blocked"))
			n, err := node.Parse([]byte(mustReadFile(t, path)))
			if err != nil || len(n.Links) != 0 {
				t.Errorf("Links = %+v (err %v), want none", n.Links, err)
			}
			if rows := queryRows(t, vault, fmt.Sprintf(
				"SELECT dst FROM edges WHERE rel='depends-on' AND src='%s'", id)); len(rows) != 0 {
				t.Errorf("edges still present after --no-depends: %v", rows)
			}

			out, stderr, err := runTodo(t, vault, "list", "--json")
			resetCLIFlags()
			if err != nil {
				t.Fatalf("rk todo list: %v\nstderr: %s", err, stderr)
			}
			var list todoListResult
			mustDecodeJSON(t, out, &list)
			for _, it := range list.Items {
				if it.ID == id && it.Depends != "" {
					t.Errorf("todo list still shows depends %q", it.Depends)
				}
			}
		})
	}
}

func TestTodoEdit_NoDependsRemovesHandWrittenDependsLine(t *testing.T) {
	t.Run("depends_only", func(t *testing.T) {
		vault := newEditVault(t)
		aID, _, _ := newTodo(t, vault, "A")
		id, path, _ := newTodo(t, vault, "Blocked", `depends: "[[`+aID+`]]"`)
		mustEdit(t, vault, id, "--no-depends")
		requireFile(t, path, todoFixtureSrc(id, "open", "", "Blocked"))
	})
	t.Run("both_keys", func(t *testing.T) {
		vault := newEditVault(t)
		aID, _, _ := newTodo(t, vault, "A")
		id, path, _ := newTodo(t, vault, "Blocked", `depends: "[[`+aID+`]]"`, `depends-on: "[[`+aID+`]]"`)
		mustEdit(t, vault, id, "--no-depends")
		requireFile(t, path, todoFixtureSrc(id, "open", "", "Blocked"))
	})
}

func TestTodoEdit_ClearingAbsentFieldIsIdempotentNoWrite(t *testing.T) {
	vault := newEditVault(t)
	id, path, src := newTodo(t, vault, "Body")
	old := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	for i := 0; i < 2; i++ {
		mustEdit(t, vault, id, "--no-deadline")
		requireFile(t, path, src)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !st.ModTime().Equal(old) {
		t.Errorf("mtime changed to %v; clearing an absent field must not write", st.ModTime())
	}
}

func TestTodoEdit_ValueAndClearFlagConflict(t *testing.T) {
	vault := newEditVault(t)
	id, _, _ := newTodo(t, vault, "Body", "deadline: 2026-07-01")
	cases := []struct{ field, value string }{
		{"deadline", "2026-08-01"},
		{"scheduled", "2026-08-01"},
		{"priority", "B"},
		{"depends", "someref"},
	}
	for _, c := range cases {
		requireEditRefused(t, vault,
			[]string{"todo edit: --" + c.field + " and --no-" + c.field + " conflict"},
			id, "--"+c.field, c.value, "--no-"+c.field)
	}
}

func TestTodoEdit_ClearAndSetDifferentFieldsOneWrite(t *testing.T) {
	vault := newEditVault(t)
	id, path, src := newTodo(t, vault, "Body", "deadline: 2026-07-01")

	mustEdit(t, vault, id, "--no-deadline", "--priority", "B")

	want := withFirstLine(strings.Replace(src, "deadline: 2026-07-01\n", "", 1), "priority: B")
	requireFile(t, path, want)
}

func TestTodoEdit_NoDeadlineOnDoneTodoKeepsState(t *testing.T) {
	vault := newEditVault(t)
	id := node.Mint()
	path, _ := writeTodoFixture(t, vault, id, "done", "", "Finished.", "deadline: 2026-07-01")

	mustEdit(t, vault, id, "--no-deadline")

	requireFile(t, path, todoFixtureSrc(id, "done", "", "Finished."))
}

// ─────────────────────────────────────────────────────────────────────────────
// Output modes, command tree, index
// ─────────────────────────────────────────────────────────────────────────────

func TestTodoEdit_OutputShapes(t *testing.T) {
	fixture := func(t *testing.T) (vault, id string) {
		vault = newEditVault(t)
		id = node.Mint()
		writeTodoFixture(t, vault, id, "open", "2026-07-20", "Body")
		return vault, id
	}
	args := func(id string) []string {
		return []string{id, "--deadline", "2026-08-01", "--priority", "B", "--no-scheduled"}
	}

	t.Run("json", func(t *testing.T) {
		vault, id := fixture(t)
		res := decodeMap(t, mustEdit(t, vault, append(args(id), "--json")...))
		want := map[string]any{
			"kind": "durable", "ref": id, "id": id, "path": "todos/" + id + ".md",
			"state": "open", "deadline": "2026-08-01", "priority": "B",
			"cleared": []any{"scheduled"},
		}
		if !reflect.DeepEqual(res, want) {
			t.Errorf("JSON result = %v, want %v", res, want)
		}
	})
	t.Run("ndjson", func(t *testing.T) {
		vault, id := fixture(t)
		rows := parseNDJSONMaps(t, mustEdit(t, vault, append(args(id), "--ndjson")...))
		if len(rows) != 1 || rows[0]["id"] != id || rows[0]["deadline"] != "2026-08-01" {
			t.Errorf("NDJSON rows = %v, want one result object for %s", rows, id)
		}
	})
	t.Run("pretty", func(t *testing.T) {
		vault, id := fixture(t)
		out := strings.TrimSpace(mustEdit(t, vault, args(id)...))
		want := "todo: " + id + " updated (deadline 2026-08-01, priority B, cleared scheduled)"
		if out != want {
			t.Errorf("pretty = %q, want %q", out, want)
		}
	})
	t.Run("quiet_suppresses_pretty_only", func(t *testing.T) {
		vault, id := fixture(t)
		if out := mustEdit(t, vault, append(args(id), "--quiet")...); strings.TrimSpace(out) != "" {
			t.Errorf("--quiet pretty printed %q, want nothing", out)
		}
		path := filepath.Join(vault, "todos", id+".md")
		if !strings.Contains(mustReadFile(t, path), "deadline: 2026-08-01") {
			t.Error("--quiet edit did not write the file")
		}
	})
	t.Run("quiet_json_still_prints", func(t *testing.T) {
		vault, id := fixture(t)
		out := mustEdit(t, vault, append(args(id), "--quiet", "--json")...)
		if decodeMap(t, out)["id"] != id {
			t.Errorf("--quiet --json output %q lacks the result object", out)
		}
	})
}

func TestTodoEdit_RegisteredWithAllFlags(t *testing.T) {
	var edit *cobra.Command
	for _, c := range todoCmd.Commands() {
		if c.Name() == "edit" {
			edit = c
		}
	}
	if edit == nil {
		t.Fatal("`edit` is not registered under `rk todo`")
	}
	for _, f := range []string{
		"deadline", "scheduled", "priority", "depends",
		"no-deadline", "no-scheduled", "no-priority", "no-depends",
	} {
		if edit.Flags().Lookup(f) == nil {
			t.Errorf("flag --%s not registered on `rk todo edit`", f)
		}
	}
}

func TestTodoEdit_WriteThroughVisibleToQuery(t *testing.T) {
	vault := newEditVault(t)
	id, _, _ := newTodo(t, vault, "Body")
	buildIndex(t, vault)

	mustEdit(t, vault, id, "--deadline", "2026-08-01", "--priority", "B")

	rows := queryRows(t, vault, fmt.Sprintf(
		"SELECT key, value FROM node_props WHERE id='%s' AND key IN ('deadline','priority')", id))
	got := map[string]any{}
	for _, r := range rows {
		got[fmt.Sprint(r["key"])] = r["value"]
	}
	if got["deadline"] != "2026-08-01" || got["priority"] != "B" {
		t.Errorf("node_props after edit = %v, want deadline=2026-08-01 priority=B", got)
	}
}

// unusableCache points RECKON_CACHE beneath a regular file so any index open
// fails while config loading and plain file work still succeed.
func unusableCache(t *testing.T) {
	t.Helper()
	blocker := filepath.Join(t.TempDir(), "blocker")
	mustWriteFile(t, blocker, "not a directory")
	t.Setenv("RECKON_CACHE", filepath.Join(blocker, "sub"))
}

func TestTodoEdit_IndexFailureAfterWriteIsWarning(t *testing.T) {
	vault := newEditVault(t)
	id, path, src := newTodo(t, vault, "Body")
	unusableCache(t)

	_, stderr, err := runEdit(t, vault, id, "--deadline", "2026-08-01")
	if err != nil {
		t.Fatalf("plain edit must not fail on index trouble after the write: %v\nstderr: %s", err, stderr)
	}
	requireFile(t, path, withFirstLine(src, "deadline: 2026-08-01"))
	if !strings.Contains(stderr, "todo edit: warning") {
		t.Errorf("stderr = %q, want a `todo edit: warning` line", stderr)
	}

	t.Run("quiet_suppresses_warning", func(t *testing.T) {
		_, stderr, err := runEdit(t, vault, id, "--priority", "B", "--quiet")
		if err != nil {
			t.Fatalf("edit: %v", err)
		}
		if strings.Contains(stderr, "warning") {
			t.Errorf("--quiet still warned: %q", stderr)
		}
	})
}

func TestTodoEdit_DependsNeedsIndexFatalBeforeWrite(t *testing.T) {
	vault := newEditVault(t)
	aID, _, _ := newTodo(t, vault, "A")
	id, _, _ := newTodo(t, vault, "Blocked")
	unusableCache(t)

	requireEditRefused(t, vault, []string{"todo edit:", "index"}, id, "--depends", aID)
}

// ─────────────────────────────────────────────────────────────────────────────
// Guards: behaviour that must keep passing
// ─────────────────────────────────────────────────────────────────────────────

// TestTodoEdit_GuardTodayActUnchanged pins `rk today act` D/d/p results and the
// error texts of its shared resolution and validation paths, which the new
// verb must not alter.
func TestTodoEdit_GuardTodayActUnchanged(t *testing.T) {
	vault := newEditVault(t)
	pinTodoNow(t, "2026-07-10")
	id := node.Mint()
	path, src := writeTodoFixture(t, vault, id, "open", "2026-07-09", "Body")

	for _, step := range [][]string{
		{"act", id, "D", "2026-07-20"},
		{"act", id, "d", "tomorrow"},
		{"act", id, "p", "B"},
	} {
		if _, stderr, err := runToday(t, vault, step...); err != nil {
			t.Fatalf("rk today %v: %v\nstderr: %s", step, err, stderr)
		}
		resetCLIFlags()
	}
	// Each act inserts its key as the first frontmatter line, so priority
	// (last) precedes deadline.
	want := "---\npriority: B\ndeadline: 2026-07-20\n" +
		strings.TrimPrefix(strings.Replace(src, "scheduled: 2026-07-09", "scheduled: 2026-07-11", 1), "---\n")
	requireFile(t, path, want)

	errCases := []struct {
		args []string
		want string
	}{
		{[]string{"act", id, "p", "Z"}, `today act: priority: invalid value "Z" (want A, B, or C)`},
		{[]string{"act", id, "d", "2026-13-01"}, "today act: defer: malformed scheduled date"},
		{[]string{"act", "missing-ref", "d", "tomorrow"}, `today act: no todo found matching "missing-ref" (not found)`},
	}
	for _, c := range errCases {
		_, stderr, err := runToday(t, vault, c.args...)
		resetCLIFlags()
		if err == nil || !strings.Contains(stderr+err.Error(), c.want) {
			t.Errorf("rk today %v: error = %v, want text %q", c.args, err, c.want)
		}
	}

	crlfID := node.Mint()
	mustWriteFile(t, filepath.Join(vault, "todos", crlfID+".md"),
		"---\r\nid: "+crlfID+"\r\ntype: todo\r\nstate: open\r\n---\r\nBody\r\n")
	_, stderr, err := runToday(t, vault, "act", crlfID, "d", "tomorrow")
	resetCLIFlags()
	if err == nil || !strings.Contains(stderr+err.Error(), "CRLF line endings are not supported (reckon-vj55)") {
		t.Errorf("CRLF today act: error = %v, want the vj55 refusal", err)
	}
}
