# Implementation Plan: reckon-fnqs.5 — Multi-line body entry paths

## Summary of approach

Add three body-entry mechanisms (`-m`/`--message` repeatable, `--edit` $EDITOR shellout, stdin via lone `-` arg) to both `rk todo add` and `rk add`, mirroring git. All the assembly and mutual-exclusion logic is byte-identical across both commands, so it goes in one new file, `internal/cli/body_entry.go`, exposing a single `assembleBody` helper plus two testable seams (`runEditor` func var; stdin via `cmd.InOrStdin()`). Each command's `RunE` gains a call to `assembleBody` in place of its current `strings.TrimSpace(strings.Join(args, " "))`, keeps its existing empty-body guard, and (todo only) gains an ephemeral-rejection guard. `Args` relaxes from `MinimumNArgs(1)` to `ArbitraryArgs` on both commands. No changes to the node package, write recipes, frontmatter, or the read/derive-title path (fnqs.3 already satisfies "list shows only subject").

## Files to modify

| File | Change |
|---|---|
| `internal/cli/body_entry.go` **(new)** | Shared `assembleBody(...)`, `isStdinDash(...)` predicate, and the `runEditor` seam + its production impl. |
| `internal/cli/todo.go` | `todoAddCmd.Args` → `ArbitraryArgs` (`:92`); add `todoMessageFlag []string` + `todoEditFlag bool` globals (`:40-52`) and register on `todoAddCmd` (`init`, `:112-119`); add both to `resetTodoFlags` (`:58-75`, var reset + Changed-clear name list); in `runTodoAddE` (`:269-329`) replace body-build (`:278`) with `assembleBody(cmd, args, todoMessageFlag, todoEditFlag, true)`, add ephemeral-rejection guard before the call, keep the empty-body guard (`:279-281`) and existing ephemeral/durable-flag guard (`:283-285`). |
| `internal/cli/add.go` | `addCmd.Args` → `ArbitraryArgs` (`:40`); add `addMessageFlag []string` + `addEditFlag bool` globals (`:24-27`) and register in `init` (`:44-48`); add both to `resetAddFlags` (`:52-60`); in `runAddE` (`:83-134`) replace body-build (`:90`) with `assembleBody(cmd, args, addMessageFlag, addEditFlag, false)`; the existing `embeddedHeaderRe` body guard (`:94-96`) then runs against the assembled body (satisfies E18). |
| `internal/cli/query_test.go` | Add `RootCmd.SetIn(nil)` to `resetCLIFlags` (`:100-109`) — prevents an injected stdin reader leaking across `Execute()` calls. |
| `internal/cli/todo_test.go` / `internal/cli/add_test.go` | New tests (Section below); add a stdin-capable variant of `runTodo`/`runAdd`. |

**Read, unchanged** (write plumbing implementer must not touch): `addDurableTodo`'s `body+"\n"` (`todo.go:347`) and `appendLogEntry`'s bare-body pass (`add.go:123`, via `RenderLogEntry`'s own trailing `\n`). `assembleBody` returns a fully-trimmed, no-trailing-newline string precisely so each call site's existing newline convention yields exactly one `\n`.

## Design decisions

### D1 — Shared helper in a new file `internal/cli/body_entry.go` (not inline/duplicated)
New file. Justified for a 3-flag feature because the shared logic is ~100+ lines (source-count dispatch + per-message vs. whole-buffer trim rules + mutual-exclusion + two seams), the AC doc requires byte-identical assembly across both commands (drift risk if duplicated), and the `runEditor` seam needs an unambiguous home. Follows the sibling-file convention (`todo.go`/`add.go`/`note_v1.go` each own their command family; a cross-cutting concern earns its own file).

### D2 — `assembleBody` signature
```go
func assembleBody(cmd *cobra.Command, args, messages []string, edit, requireSubject bool) (string, error)
```
- Returns the fully-trimmed multi-paragraph body, **no trailing newline**, or an error.
- `requireSubject` is `true` for `rk todo add`, `false` for `rk add`. Gates only the `-m` `messages[0]`-non-empty check (D4).
- Errors returned bare/descriptive; call sites wrap `fmt.Errorf("todo add: %w", err)` / `fmt.Errorf("add: %w", err)` matching existing style.

Internal structure — count active sources, then dispatch (not a nested if-chain, not a formal enum):
```
hasStdin      = isStdinDash(args)                    // len(args)==1 && args[0]=="-"
hasPositional = len(args) > 0 && !hasStdin
hasMessages   = len(messages) > 0
hasEdit       = edit
active = count(hasStdin, hasPositional, hasMessages, hasEdit)
if active > 1 { return "", errors.New("choose one entry method (-m, --edit, stdin '-', or positional text)") }
```
Then dispatch on the single active source; `active == 0` returns `("", nil)` and falls through to the caller's empty-body guard (serves E16). `isStdinDash` is a shared predicate (also used by `runTodoAddE`'s ephemeral guard). E12 (`["-", "extra"]`) is handled for free: `hasStdin` false, `hasPositional` true → joined as literal text.

Per-path assembly:
- **positional**: `strings.TrimSpace(strings.Join(args, " "))` (unchanged AC6 behavior).
- **-m**: trim each message; if `requireSubject && strings.TrimSpace(messages[0]) == ""` → error; `strings.TrimSpace(strings.Join(trimmed, "\n\n"))`.
- **stdin**: `b, err := io.ReadAll(cmd.InOrStdin())`; `strings.TrimSpace(string(b))`.
- **edit**: env-check → temp → seam → read → trim (D3).

**Invariant: `cmd.InOrStdin()` is read only on the active stdin path.** An unconditional `io.ReadAll` on a non-stdin invocation blocks forever on a TTY waiting for EOF — the count-dispatch structure prevents this. `TestTodoAdd_PositionalArgsUnaffected` implicitly guards it (would hang otherwise).

### D3 — Editor seam: fine-grained func var, env check in the helper
```go
var runEditor = func(editor, path string) error {
    c := exec.Command(editor, path)
    c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr  // real TTY, not cmd.InOrStdin
    return c.Run()  // nonzero exit -> *exec.ExitError -> E5
}
```
Edit-path sequence inside `assembleBody`: (1) `editor := strings.TrimSpace(os.Getenv("EDITOR")); if editor == "" { return error }` — satisfies `TestTodoAdd_EditFlag_EditorUnsetErrors` ("errors before invoking the seam"); (2) `os.CreateTemp` a scratch file (naming style per `adopt.go:256`), **`defer os.Remove(tmpPath)` immediately**; (3) `runEditor(editor, tmpPath)`; (4) `os.ReadFile(tmpPath)`; (5) `strings.TrimSpace`. No boilerplate/comment written into the buffer.

Consequences: (a) edit tests must `t.Setenv("EDITOR", "stub")` to clear the env guard before the stubbed seam runs; (b) production `runEditor` (first `os/exec` use in `internal/cli`) is always stubbed in tests — the real shellout is manual-verification-only, a known/accepted limitation.

**Alternative rejected:** a coarse seam encapsulating the whole edit path. The unset-$EDITOR error would then fire inside the seam, contradicting the AC's pinned "errors before invoking the seam at all." Fine-grained seam wins on AC fidelity.

### D4 — Two subject checks, deliberately not unified
Followed the AC doc (AC5), not codebase-doc §5's "one post-assembly check suffices" claim: that's true for edit/stdin/positional (TrimSpace makes "body non-empty" ≡ "first non-blank line non-empty") but wrong for `-m` — `-m '' -m 'detail'` assembles to non-empty `"detail"` yet must error (AC5, E1). So `assembleBody` keeps a bespoke `messages[0]`-non-empty check on the `-m` path only (gated by `requireSubject`); edit/stdin/positional/no-source all route through each command's unchanged empty-body guard (also newly serves E16).

### D5 — Ephemeral rejection lives in `runTodoAddE`, before `assembleBody`
Keeps `assembleBody` command-agnostic (`rk add` has no ephemeral concept). Guard: `if ephemeral && (len(todoMessageFlag) > 0 || todoEditFlag || isStdinDash(args)) { return error }`, placed before the `assembleBody` call so `--ephemeral --edit` never spawns an editor. Blanket rejection, no single-`-m` carve-out (E13-15).

### D6 — Flags: `StringArrayVarP` with `-m` shorthand; per-command vars
`-m`/`--message` uses `StringArrayVar` (not `StringSliceVar` — no comma-splitting; matches `noteTagFlag` at `note_v1.go:125`). Shorthand `-m` verified free of collision (only `-q` is bound on root). Separate var sets per command (`todoMessageFlag`/`todoEditFlag` vs. `addMessageFlag`/`addEditFlag`) following the existing `todoAuthorFlag`/`addAuthorFlag` split. Both added to the respective `reset*Flags` var-reset and Changed-clear name lists.

### D7 — `["-"]` reaches `args` under pflag (verified, not assumed)
A single-dash token with `len==1` is treated as a positional, not a flag, by pflag — no special arg-parsing needed; `cobra.ArbitraryArgs` passes it straight through to `runE`.

## Test scenarios

Use the AC doc Section 4 list verbatim (ticket-work/reckon-fnqs.5/acceptance-criteria.md) — comprehensive; sparse `rk add` coverage is intentional (one representative conflict case suffices for `add_test.go`, full matrix lives in `todo_test.go`). No additions or removals.

- **`todo add` (`todo_test.go`):** `TestTodoAdd_MessageFlag_JoinsParagraphs`, `_TrimsEachMessage`, `_EmptySubjectErrors`, `_WhitespaceOnlySubjectErrors`, `_TrailingEmptyMessageAbsorbed`, `_InteriorEmptyMessagePreservesBlankRun`, `_SingleMessageValidSubjectOnly`; `TestTodoAdd_EditFlag_UsesSavedContent`, `_EmptySaveErrors`, `_EditorNonzeroExitAborts`, `_EditorUnsetErrors`, `_NoCommentStripping`; `TestTodoAdd_StdinDash_ReadsFullBody`, `_EmptyErrors`, `_MultipleArgsDoesNotTriggerSentinel`; `TestTodoAdd_ConflictingSources_{MessageAndPositional,MessageAndEdit,StdinAndMessage,StdinAndEdit}`; `TestTodoAdd_NoSourceAtAllErrors`; `TestTodoAdd_Ephemeral_Rejects{MessageFlag,EditFlag,StdinDash}`; `TestTodoAdd_PositionalArgsUnaffected`; `TestTodoAdd_LongSubjectNotTruncated`.
- **`rk add` (`add_test.go`):** `TestAdd_MessageFlag_JoinsParagraphs`, `TestAdd_MessageFlag_EmbeddedHeaderGuardAppliesPostAssembly`, `TestAdd_EditFlag_UsesSavedContent`, `TestAdd_StdinDash_ReadsFullBody`, `TestAdd_ConflictingSources_MessageAndEdit`.

**Harness additions (both test files):** model new create-path tests on `TestTodoAdd_DurableHappyPath` (`todo_test.go:196-231`) — decode `todoAddResult`, re-parse the written file with `node.Parse`, assert `strings.Split(n.Body, "\n")[0]` == subject and (todo) `rk todo list`'s `Title` == subject. Edit tests stub `runEditor` (`t.Cleanup` to restore original var) and `t.Setenv("EDITOR", "stub")`. Stdin tests need a variant of `runTodo`/`runAdd` that calls `RootCmd.SetIn(strings.NewReader(...))` before `Execute` (add `runTodoWithStdin`/`runAddWithStdin` siblings rather than changing existing helpers' signatures); rely on the new `resetCLIFlags` `SetIn(nil)` via `t.Cleanup` to avoid cross-test leakage.

## Known risks / ambiguities

- **[RESOLVED] `-m` + `--edit` → hard error.** Deliberate divergence from git's augment-in-editor behavior; augmentation is out of scope (E9).
- **[RESOLVED] `$EDITOR` unset → hard error, no fallback.** No `vi`/`$VISUAL` fallback; error before temp/spawn (E6).
- **[RESOLVED] Shared helper file.** New `internal/cli/body_entry.go` (D1).
- **Doc divergence on the subject check (D4):** codebase-doc §5 vs AC5 — followed AC5. The `-m` `messages[0]` bespoke check is required; do not collapse it into the post-assembly guard.
- **TTY-hang landmine:** only read `cmd.InOrStdin()` on the active stdin path (D2 invariant).
- **Production `runEditor` has no automated coverage** (D3) — always stubbed; the real shellout is manual-verify only. Standard testability pattern in this codebase (`mintTodoULID`/`todoNow`).

### Critical Files for Implementation
- `internal/cli/body_entry.go` (new — shared `assembleBody` + `isStdinDash` + `runEditor` seam)
- `internal/cli/todo.go` (`todoAddCmd` Args, flags, `runTodoAddE`, `resetTodoFlags`)
- `internal/cli/add.go` (`addCmd` Args, flags, `runAddE`, `resetAddFlags`)
- `internal/cli/query_test.go` (`resetCLIFlags` gains `RootCmd.SetIn(nil)`)
- `internal/cli/todo_test.go` / `internal/cli/add_test.go` (new scenarios and stdin-capable run helpers)
