# Code Review: reckon-fnqs.5 — Multi-line body entry paths

**Verdict: APPROVE**

Every AC (AC1–AC6) and edge case (E1–E19) is satisfied. No finding requires a
code change for correctness. All 30 new tests pass; the full `internal/cli`
suite, `go vet`, and `go build ./...` are clean. The suite completes in ~0.09s
with stdin tests present — empirical proof the TTY-hang invariant holds (an
unconditional `InOrStdin()` read would hang the run to timeout). Findings below
are non-blocking Recommendations.

## Summary

Clean, well-factored change. The shared `assembleBody` helper
(`internal/cli/body_entry.go`) centralizes the source-count dispatch and both
testable seams (`runEditor` func var, `cmd.InOrStdin()`), and both call sites
(`todo.go:289`, `add.go:95`) keep their pre-existing empty-body / header guards
unchanged. The count-active-sources dispatch (`body_entry.go:54-62`) is simpler
and less error-prone than a nested if-chain, and correctly makes the TTY read
reachable only on the sole-active stdin path.

## Correctness against the AC edge-case matrix

Traced each case against `body_entry.go` + call sites + the corresponding test.

| Case | Expected | Code path | Verdict |
|---|---|---|---|
| E1 empty `-m ''` subject | error, no write | `body_entry.go:69-70` (`requireSubject && TrimSpace(messages[0])==""`) | PASS |
| E2 whitespace `-m '   '` | error (trims to "") | same check, TrimSpace | PASS |
| E3 trailing empty `-m 'S' -m ''` | `"S\n"`, byte-identical to lone `-m 'S'` | join `"\n\n"` → outer `TrimSpace` absorbs trailer (`:76`); `addDurableTodo` adds one `\n` | PASS |
| E3b interior empty `-m 'S' -m '' -m 'p2'` | `"S\n\n\n\np2\n"` (run preserved) | outer TrimSpace touches edges only | PASS |
| E3c lone `-m 'Buy milk'` | `"Buy milk\n"` | subject non-empty; single message | PASS |
| E4 `--edit` empty save | empty-body error (E4 ≠ E5) | `runEditor` returns nil → ReadFile "" → `TrimSpace ""` → `("",nil)` → caller empty-body guard | PASS |
| E5 editor nonzero exit | distinct error, no fall-through to E4 | `runEditor` err → `"editor exited with an error: %w"` (`:101-103`), returned before ReadFile | PASS |
| E6 `$EDITOR` unset | error before temp/spawn | env check first (`:86-88`), before `CreateTemp`/seam | PASS |
| E7 stdin + `-m` | error | `active==2` (`:60-61`) | PASS |
| E8 stdin + `--edit` | error | `active==2` | PASS |
| E9 `-m` + `--edit` | error (intentional git divergence) | `active==2` | PASS |
| E10 5000-char subject | verbatim, no truncation | no length cap anywhere | PASS |
| E11 positional + `-m` | error | `active==2` | PASS |
| E12 `["-", "extra"]` | sentinel does NOT fire; `"- extra"` | `isStdinDash` requires `len==1` (`:29-31`); `hasPositional` true | PASS |
| E13/E14/E15 `--ephemeral` + any new path | error, no editor/stdin work | guard `todo.go:285-287` runs **before** `assembleBody` (`:289`) | PASS |
| E16 no source | empty-body error | `active==0` → `default` returns `("",nil)` (`:111-112`) → caller guard | PASS |
| E17 embedded `\n` in `-m` | preserved verbatim | per-message `TrimSpace` only strips outer ws | PASS `[INFERRED]` (no regression guard; see R3) |
| E18 `rk add` assembled `## ` line | error via `embeddedHeaderRe` | `add.go:102` runs against assembled `body`, not argv | PASS |
| E19 CRLF via edit/stdin | pass-through, trailing `\r` trimmed | whole-buffer `TrimSpace` | PASS (see R3 `[OPEN]`) |

Key subtle points the task flagged, all confirmed correct:

- **Two-check separation (D4).** The `-m` subject check on `messages[0]`
  (`body_entry.go:69`, gated by `requireSubject`) is genuinely distinct from
  the post-assembly `body == ""` guard. It has to be: `-m '' -m 'detail'`
  assembles to non-empty `"detail"` yet must error on `todo add` — a unified
  post-assembly check would wrongly accept it. For edit/stdin/positional the
  check is correctly *not* duplicated: after whole-buffer `TrimSpace`, a
  non-empty body cannot have a blank first line, so "body non-empty" ≡ "first
  line non-empty". For `rk add` (`requireSubject=false`) the `-m` check is
  correctly skipped (log entries have no subject semantics).
- **TTY-hang invariant.** `cmd.InOrStdin()` is read only inside
  `case hasStdin:` (`body_entry.go:78-83`), which is reachable only when it is
  the sole active source. Empirically confirmed by the fast suite run.
- **Ephemeral guard ordering.** `todo.go:285` blocks `-m`/`--edit`/stdin
  before `assembleBody`, so `--ephemeral --edit` never spawns an editor
  (verified by `TestTodoAdd_Ephemeral_RejectsEditFlag`'s failing stub).

## Testing

Strong. Tests drive real behavior through the CLI argv surface (not by poking
package internals), the only seam being `runEditor` via `stubRunEditor`
(`todo_test.go:2035`). Quality checks that would catch a broken impl:

- `TestTodoAdd_EditFlag_EditorNonzeroExitAborts` asserts the error does **not**
  contain `"empty body"` (`todo_test.go:2226`) — catches E5→E4 misrouting, not
  just "some error returned".
- Stdin tests inject via `RootCmd.SetIn`; an impl reading `os.Stdin` directly
  would read the empty test stdin and fail the success assertion — so they
  genuinely distinguish `InOrStdin()` from `os.Stdin`.
- `TestTodoAdd_StdinDash_MultipleArgsDoesNotTriggerSentinel` seeds stdin with a
  poison string and asserts it never reaches the body (`todo_test.go:2322`).
- Conflict/ephemeral+edit tests install a `t.Fatal` stub, proving the editor is
  never invoked on rejected combinations.
- `query_test.go:106-109` `SetIn(nil)` reset correctly prevents an injected
  reader leaking across `Execute()` calls in the same binary.

## Recommendations (non-blocking)

**R1 — Add one test that the scratch file handed to the editor is empty
(AC3 "no boilerplate/pre-seeded text").** This is the one stated AC with a
coverage blind spot. Production is correct by inspection —
`body_entry.go:91-99` does `CreateTemp` → `Close`, never `Write`, so the editor
opens an empty file — but **every** `stubRunEditor` overwrites the whole file,
so no test observes the pre-edit contents. A regression that pre-seeded a git-
style comment banner would pass the entire suite. Suggest a stub that reads
`path` first and asserts it is empty before writing canned content.

**R2 — `$EDITOR` with arguments is unsupported; note it as a caveat.**
`runEditor` uses `exec.Command(editor, path)` with no shell (`body_entry.go:20`).
Two consequences of the same design choice:
- **Security: not a concern, and the reason is stronger than "operator-
  controlled".** With no `sh -c`, there is no shell word-splitting or
  metacharacter interpretation of `$EDITOR` or `path` at all — no injection
  vector regardless of contents. `$EDITOR` is also outside this tool's threat
  model (the operator running `rk` already owns the process). Confirmed fine.
- **Ergonomics: `EDITOR="code --wait"`, `emacsclient -c`, `vim -u NONE` etc.
  fail** — `LookPath` receives the literal string `"code --wait"` and errors
  (a clear message, not a hang). Common enough real-world `$EDITOR` values that
  a one-line caveat in the `--edit` flag help or a follow-up ticket is worth
  it. Out of the pinned scope (AC out-of-scope lists `$VISUAL` fallback; arg-
  splitting is a missing-feature limitation, not a regression), so non-blocking.

**R3 — Two low-severity forward-looking notes.**
- `[INFERRED]` E17 (embedded `\n` in a `-m` value) has no regression guard.
  It works by construction (per-message `TrimSpace` preserves interior
  newlines) and the plan deliberately scoped tests to AC §4, so this is
  acceptable — flagged only for future awareness.
- `[OPEN]` A CRLF-bearing `--edit`/stdin body on `rk add` writes interior `\r`
  bytes into the day file; a *later* same-day `rk add` runs the whole-file CRLF
  guard (`add.go` day-file check) and could then reject the append. This is
  within E19's explicitly-accepted pass-through behavior and is a very edge
  case (pasted CRLF content), so no action now — noted for awareness.

## Positive observations

- `assembleBody`'s doc comment (`body_entry.go:33-48`) pins the exact
  invariants (single-source dispatch, no trailing newline, conditional stdin
  read) that make the two unchanged call sites correct — good load-bearing
  documentation, not narration.
- Resource handling is correct: `os.CreateTemp` (0600 perms, no world-readable
  body leak), `defer os.Remove` registered before every error path
  (`body_entry.go:96`), temp handle closed and error-checked before the editor
  runs (`:97-99`).
- Error wrapping is consistent with house style throughout; validation errors
  use bare `errors.New`, system errors use `fmt.Errorf(... %w ...)`.
- Flag reset plumbing is complete: both new flags added to the var-reset and
  the `Changed`-clear name lists in `resetTodoFlags`/`resetAddFlags`. The
  `StringArrayVar` internal `changed` state is harmless here because the var is
  reset to `nil` (append-from-nil == replace) and the code keys off
  `len(messages)`, never `Flags().Changed`.
