## Summary

`rk todo done ""` and `rk today act "" <key>` mutated the first todo with no `id:` line, because an empty ref compared equal to its empty ULID. An unset shell variable could therefore change an arbitrary file. `rk todo done ../outside` also resolved through its own direct-path lookup and wrote to files outside `todos/`.

This change makes `todo done`, `todo edit` and `today act` share one resolver:

- **Empty or whitespace refs:** the resolver refuses them with `<verb>: empty ref (give a todo ULID or alias)`, and the `todos/` walk can no longer match an empty ULID.
- **Path-shaped refs:** `todo done` matches them only as aliases. Anything else reports `todo done: no todo found matching "<ref>" (not found)` and leaves the file untouched.
- **Error prefix:** resolver errors from `today act` now say `today act:` instead of `todo done:`.

ULID, alias, `inbox` fall-through, idempotent-skip, recurrence and `--ephemeral` behaviour are unchanged.

## Files to modify

| File | Change | Reason |
|---|---|---|
| `internal/cli/today.go` | `resolveDurableTodoRef` (:456): first statement is the `strings.TrimSpace(ref) == ""` guard, returning `fmt.Errorf("%s: empty ref (give a todo ULID or alias)", verb)`. | One guard for all three entry points, before the resolver's own file reads. |
| same | Resolver: move the `inbox` explanation from todo.go:721-722 to the type check at :472; update the doc comment at :453-455 (empty-ref error; path-shaped refs resolve by alias only). | Keeps that rationale once the fast path is deleted. |
| same | `loadNativeTodoForEdit` (:443): pass verb `"today act"`. Rewrite the comment at :438-441 to drop the "still also accepts" sentence. | Correct prefix; the comment becomes false. |
| `internal/cli/todo.go` | `doneDurableTodo` (:712-736): body becomes `resolveDurableTodoRef("todo done", vaultDir, ref)`, the nil check with the verbatim not-found error (:732), then the unchanged `completeDurableTodoNode` call. Delete `todosDir` and `fastPath`. | Closes the traversal. |
| same | Doc comment at :709-711: state that it resolves through the shared resolver. | It describes the deleted fast path. |
| same | `findDurableTodoByRefOrAlias` (:958): `(n.ULID != "" && n.ULID == ref) \|\| containsString(n.Aliases, ref)`. | Root cause; safe for future callers. |
| `internal/cli/todo_test.go` | New `todo done` and walk/resolver tests; new helper `requireRefused`. | See Test scenarios. |
| `internal/cli/today_test.go` | New `today act` tests. | See Test scenarios. |

`todo_edit.go` and `todo_browse.go` do not change. New comments carry no ticket IDs.

## Design decisions

| # | Decision (made; not reopened) | Alternative considered | Why not |
|---|---|---|---|
| 1 | Empty/whitespace guard lives in `resolveDurableTodoRef`, using the caller's verb and the same wording as todo_edit.go:64-65. `todo edit`'s own guard stays. | Guards in `runTodoDoneE` and `dispatchTodayAct` | Duplicates the check. `doneDurableTodo` called from todo_browse.go:135 would bypass a guard in `runTodoDoneE`. |
| 2 | The walk also requires a non-empty ULID. Aliases need no change: `parseAliases` (internal/node/node.go:484-499) can never yield an empty alias. | Resolver guard only | Leaves the root cause for any future walk caller. |
| 3 | `loadNativeTodoForEdit` passes `"today act"`. | Keep `"todo done"` and guard separately | The empty-ref error would carry the wrong prefix. |
| 4 | `doneDurableTodo` keeps its signature and delegates to the resolver, keeping its not-found wording verbatim. Inline fast path deleted. | Patch the fast path with a `filepath.Base` check | Keeps two resolvers, which is how they diverged. |
| 5 | Out of scope, to file as follow-ups: `findNoteByRefOrAlias` has the same defect for `rk note rename ""`; id-less todos cannot be completed from browse. | Fix here | Separate surfaces. |

Planning choices for this ticket:

| Choice | Alternative | Why |
|---|---|---|
| New helper `requireRefused(t, vault, run, wantErr, args...)`. `run` has the signature shared by `runTodo` and `runToday`. It resets CLI flags, requires `err.Error() == wantErr` exactly, and requires an equal vault snapshot. | Generalise `requireEditRefused` | That helper hardcodes `runEdit` and is used widely. Exact matching is needed because whitespace cases are red only on wording, and because `today act: d/defer requires an argument` also starts with `today act:`. |
| Direct-call unit tests for the resolver and the walk. | CLI tests only | Each layer masks the other through the CLI. Without these, removing the walk fix, or moving the guard to the command layer (AC 5), would stay green. |
| Fresh vault per subtest. | Shared vault | On current code an earlier subtest mutates the fixture, which would muddy the red results. |

## Test scenarios

Shared fixtures:

- **`idless`:** raw bytes via `mustWriteFile` at `todos/idless.md`: `"---\ntype: todo\nstate: open\n---\nBody\n"`. Do not use `writeTestNode`, which always emits `id:`.
- **`outside`:** `<vault>/outside.md` with `id:`, `type: todo`, `state: open`.

Empty-ref wording checked below: `<verb>: empty ref (give a todo ULID or alias)`.

### Red tests (the red-state gate names only these)

Red status comes from tracing the code; none of these have been run. [INFERRED] The gate confirms it.

| Test | File | AC | Given / when / then | Why it fails today |
|---|---|---|---|---|
| `TestTodoDone_RejectsEmptyRef` (subtests `empty`, `whitespace`) | todo_test.go | 1 | `idless` / `todo done ""`, `"  "` / exact empty-ref error with verb `todo done`; vault unchanged | `empty`: the walk matches and completes the file. `whitespace`: **wording only**; today gives `no todo found`, file untouched. |
| `TestTodayAct_RejectsEmptyRef` (7 keys × `empty`/`whitespace`, 14 subtests) | today_test.go | 2 | `idless`, `pinTodoNow` / `today act <ref> t`, `d tomorrow`, `D 2026-07-20`, `p A`, `x`, `i`, `c`, with ref `""` and `"   "` / exact empty-ref error with verb `today act`; vault unchanged | `empty`: each key mutates `idless`. `whitespace`: **wording only**. Valid args are required: without them, the `d` key's missing-argument error comes after resolve and writes nothing, and the other arg-taking keys are expected to behave the same. |
| `TestTodoDone_RefDoesNotEscapeTodosDir` (subtests `parent`, `embedded_dotdot`, `above_vault`) | todo_test.go | 3, 4 | Valid open todo at the target / `todo done ../outside`, `sub/../../outside` (both target `<vault>/outside.md`), `../../outside` (targets `outside.md` in the vault's parent) / exact not-found error; target bytes unchanged (`requireFile`, since the snapshot misses the vault's parent) | `filepath.Join` cleans each ref to the target, even with no `todos/` directory, so the file gets written. |
| `TestTodoDone_PathRefNotResolvedByDirectJoin` (subtests `nested`, `dot_slash`) | todo_test.go | 3, 4 | Valid open todo with `id:` at `todos/sub/foo.md`, `todos/foo.md` / `todo done sub/foo`, `./foo` / exact not-found error; vault unchanged | The fast path resolves and completes both. |
| `TestResolveDurableTodoRef_EmptyRefUsesCallerVerb` | todo_test.go | 5 | `idless` / direct call with verb `"todo edit"`, refs `""`, `"  "` / nil node; exact empty-ref error | `""` returns the node. `"  "` returns nil with no error. |
| `TestFindDurableTodoByRefOrAlias_EmptyRefNeverMatches` | todo_test.go | 5 | `idless` / direct call with ref `""` / nil node, no error | Returns the id-less node. |
| `TestTodayAct_CRLFRefusalUsesTodayActPrefix` | today_test.go | Decision 3 | CRLF todo at `todos/<id>.md` (as todo_edit_test.go:905-907) / `today act <id> d tomorrow` / error starts `today act: CRLF line endings`; vault unchanged | The error starts `todo done: CRLF`. |

### Regression guards (expected to pass today; exclude from the red gate)

| Test | File | AC | Given / when / then | Why it passes today |
|---|---|---|---|---|
| `TestTodoDone_NonEscapingPathShapesNotFound` (subtests `dotdot`, `dot`, `absolute`, `trailing_slash`) | todo_test.go | 3 | `outside`, `todos/foo.md` / `todo done ..`, `.`, `<vault>/outside`, `foo/` / exact not-found error; vault unchanged | Joined paths are `todos/...md`, `todos/..md`, `todos/<abs>/outside.md`, `todos/foo/.md`; none exist. |
| `TestTodoDone_PathShapedAliasResolvesViaWalk` | todo_test.go | 6, T8 | Todo with `aliases: [sub/foo]`, no `todos/sub/foo.md` / `todo done sub/foo` / state done | The fast path misses and the walk matches the alias. |
| `TestTodoDone_InboxContainerNotAMatch` | todo_test.go | 6, T10 | `writeEphemeralContainer` / `todo done inbox` / exact not-found error; vault unchanged | Type filter. |
| `TestTodoDone_EphemeralEmptyIndexKeepsOwnError` | todo_test.go | 6 | `todo done "" --ephemeral` / error `todo done: --ephemeral requires a positive 1-based index, got ""` | `doneEphemeralTodo` is separate. |
| `TestTodoDone_IDLessTodoByFileStemStillResolves` | todo_test.go | 6 | `idless` / `todo done idless` / state done | The direct join is kept for non-path refs; guards against over-tightening. |
| `TestTodayAct_RefDoesNotEscapeTodosDir` | today_test.go | 3 | `outside` / `today act ../outside x` / `today act: no todo found matching "../outside" (not found)`; vault unchanged | The resolver already skips the join. |

### Already covered by existing tests (no new test)

| AC scenario | Existing test |
|---|---|
| T9 ULID, alias | `TestTodoDone_DurableBytePreservation`, `TestTodoDone_DurableViaAlias` |
| T10 `--ephemeral 1` | `TestTodoDone_EphemeralFlipsCheckbox` |
| T11 `todo edit ""` | `TestTodoEdit_RejectsEmptyRef` (uses `--deadline`; the guard runs before flag handling, so `--priority` behaves the same) |
| AC 6 idempotent skip, recurrence | `TestTodoDone_IdempotentAlreadyDone`, `TestTodoDone_Recurring_*` |
| Browse ULID path | `TestMakeMarkDoneFunc_DurablePersists` |
| AC 7 | `go test ./...`, `go vet ./...` |

## Known risks or ambiguities

- **AC T7 is wrong about red state.** Section 4 of the AC says T1-T7 fail before the fix, but `todo done ..` and the absolute-path form already return not-found. They are listed as guards. The real traversal coverage is `TestTodoDone_RefDoesNotEscapeTodosDir`.
- **AC T2 and T5 are red on wording only.** `"  "` never matches `idless` today, so no data is at risk from whitespace. Those subtests stay red only through the exact-message assertion, which must not be weakened to `err != nil`.
- **`today act` misses part of an implicit AC.** The AC says the empty-ref check runs before any filesystem access, but the resolver guard (Decision 1) runs after `index.Open`, `Reconcile` and `lookupNodeByRefOrAlias` (today.go:336-349). Those touch the cache DB; the test harness places it outside the vault (`RECKON_CACHE`), and the vault stays unchanged, so AC 2's "no file modified" holds for vault files. Recorded rather than reopened.
- **Key error precedence.** `today act "" bogus` returns the unknown-key error, because `normalizeActKey` runs first (today.go:409). The AC allows either; no test pins it.
- **Intentional behaviour narrowing.** `rk todo done sub/foo` (nested file) and `./foo` stop resolving unless the ref is an alias, per AC 3 and the AC's edge-case table. Grep finds no existing test using this form; external scripts could.
- **Residual wrong prefix.** `today act x` on a todo that resolves can still emit `todo done:` from `completeDurableTodoNode` and `doneRecurringTodo` (e.g. todo.go:766, :778). Decision 3 fixes only resolver errors. Those bodies are shared and out of scope. [OPEN] File a follow-up if wanted.
- **Follow-ups to file.** Decision 5's two items are not filed yet; whoever closes this ticket should file them.
