# reckon-kcpb codebase analysis

## 1. Files and callers

Modify: `internal/cli/todo.go` (doneDurableTodo), `internal/cli/today.go` (resolveDurableTodoRef empty guard, stale comment at :438-441, verb), tests in `internal/cli/todo_test.go` (done tests) and `internal/cli/todo_edit_test.go` (today-act ref cases at ~:895).

| Symbol | Defined | Callers |
|---|---|---|
| doneDurableTodo | todo.go:712 | todo.go:695 (runTodoDoneE), todo_browse.go:135 (TUI markDone, passes `it.Ref`); tests via `rk todo done` + todo_test.go:1820 TestMakeMarkDoneFunc_DurablePersists |
| resolveDurableTodoRef | today.go:456 | today.go:443 (loadNativeTodoForEdit), todo_edit.go:95 (verb "todo edit") |
| loadNativeTodoForEdit | today.go:442 | today.go:414 (dispatchTodayAct) only; reached from runTodayActE:354 and tui_keyboard.go dispatchAgendaActuator |
| findDurableTodoByRefOrAlias | todo.go:944 | todo.go:726 (doneDurableTodo), today.go:476 (resolveDurableTodoRef) |
| loadDurableTodoAt | todo.go:923 | todo.go:716, today.go:467 |

Root cause: `findDurableTodoByRefOrAlias` matches `n.ULID == ref` (todo.go:960); an id-less todo has ULID "" so ref "" matches. Empty ref skips resolveDurableTodoRef's direct join (`filepath.Base("") == "."` != "") but still hits the walk, so `today act ""` is vulnerable too. `rk todo edit ""` is safe only because of the command-level guard (todo_edit.go:64), not because of the resolver.

## 2. Patterns to follow

- Guard: todo_edit.go:64 `strings.TrimSpace(ref) == ""` -> `"todo edit: empty ref (give a todo ULID or alias)"`, at command level before any I/O.
- Tests: TestTodoEdit_RejectsEmptyRef (todo_edit_test.go:929; id-less fixture `todos/idless.md`, refs `""` and `"  "`, helper `requireEditRefused` also asserts vault byte-identical). TestTodoEdit_RefDoesNotEscapeTodosDir (:951; asserts `no todo found matching "../outside"` and outside.md unchanged).
- Path-shaped check lives in resolveDurableTodoRef (today.go:464 `ref == filepath.Base(ref)`).

## 3. Signatures touched

- `doneDurableTodo(vaultDir, ref string) (todoDoneResult, error)`: unchanged signature; body becomes `resolveDurableTodoRef("todo done", vaultDir, ref)` + nil -> `todo done: no todo found matching %q (not found)` (keep that exact string; todo_test.go:1024 and acceptance rely on "not found"). The type!="todo" fall-through is already inside resolveDurableTodoRef.
- Empty-ref guard placement [OPEN, recommend]: inside `resolveDurableTodoRef` (covers todo done, today act, todo edit, TUI) using the caller's `verb`, e.g. `<verb>: empty ref`; today act's verb is currently the wrong `"todo done"` (today.go:443). Alternative: guard in runTodoDoneE and dispatchTodayAct only, mirroring todo_edit. If put in the resolver, todo_edit's own guard becomes redundant but harmless (its message `todo edit: empty ref` must stay; test asserts it).
- `loadNativeTodoForEdit`: could pass verb "today act" so errors say `today act:`; only if tests allow (see 6). Update its comment (:438-441), which says doneDurableTodo "still also accepts" paths; false after the fix.
- Also guard `findDurableTodoByRefOrAlias` against `ref == ""` defensively (cheap; protects any future caller). [INFERRED] optional.

## 4. REVIEW_PATTERNS.md pitfalls that apply

- Unwrapped errors: wrap/prefix every new error with the verb (`fmt.Errorf("todo done: ...")`).
- Missing edge-case tests (frequency 11): empty string, whitespace, path-shaped (`../outside`, `sub/x`), table-driven.
- Tests that don't test what they claim: assert error text AND that the id-less file / outside.md is byte-unchanged (use requireFile / vault snapshot), not just `err != nil`.
- Dead code after refactor (reckon-pjjr): delete doneDurableTodo's fastPath block and the comment on loadNativeTodoForEdit; check `filepath`/`todosDir` locals no longer unused.
- No provenance refs (ticket ids) in new comments (memory rule).

## 5. TUI browse (todo_browse.go:90,135)

- Browse sets `Ref: it.ID` (:90) and markDone calls `doneDurableTodo(vaultDir, it.Ref)` (:135).
- `it.ID` comes from `listDurableTodos` (todo.go:559) `SELECT id FROM nodes`; the `nodes` view maps `node_key AS id` (index/schema.go:85). `nodeKey` (index/reconcile.go:256-266) returns the ULID, else `"file:" + rel` (+ `#N` for duplicate surrogates).
- So an id-less durable todo's ref is `file:todos/<name>.md` (rel path form). Under current code it already fails: fastPath `todos/file:todos/<name>.md.md` does not exist (nil), walk matches `n.ULID == ref` -> "" != "file:..." and no alias -> `no todo found matching`. Under the fixed code: Base(ref) != ref so direct join skipped, walk fails identically. No regression; id-less todos are already un-completable from browse. [OPEN, out of scope] file a follow-up if desired (resolver could accept `file:<rel>` surrogate).
- ULID refs (the only working case) are unaffected: TestMakeMarkDoneFunc_DurablePersists (todo_test.go:1820) uses a ULID.

## 6. Existing tests vs. behavior change

- No test asserts a `todo done:` prefix on a today-act path: grep of `internal/cli/*_test.go` and `tests/` for `todo done:` finds nothing. today-act not-found asserts `today act: no todo found matching "missing-ref" (not found)` (todo_edit_test.go:895), which dispatchTodayAct/loadNativeTodoForEdit produces itself (today.go:446), so changing the resolver verb is safe.
- No test relies on `rk todo done` accepting a path-shaped ref (no `sub/foo`, `../`, or slash refs in done tests). `rk todo done` tests use ULIDs, alias `buy-milk` (todo_test.go:981; alias walk still works), `nonexistent-alias` (:1024), and `--ephemeral` indexes (unaffected: doneEphemeralTodo is separate and has its own Atoi validation, so `--ephemeral ""` already errors).
- Acceptance (tests/acceptance/acceptance_test.go:182,212,440,498) uses ULIDs.
- Behavior difference to expect: `rk todo done inbox` still falls to the walk and reports not-found (unchanged).

## 7. Test fixtures

- `writeTestNode` (query_test.go:69) always writes `id: <id>` first; passing `id=""` yields a literal `id: ` line (not "no id line"), and extraFM cannot remove it. Not suitable.
- Use raw bytes via `mustWriteFile` (adopt_test.go:70), as TestTodoEdit_RejectsEmptyRef does: `"---\ntype: todo\nstate: open\n---\nBody\n"` at `todos/idless.md`. `todoFixtureSrc`/`writeTodoFixture` (today_test.go:171,191) also always emit `id:`.
- Helpers: `newEditVault` (todo_edit_test.go:44), `runTodo` (todo_test.go:112), `runToday` (today_test.go:126), `snapshotVaultFiles`/`vaultSnapshotsEqual`, `requireFile`. `requireEditRefused` hardcodes `runEdit` (`todo edit`), so done/act tests need a small analogous helper (run, require error substrings, snapshot-equal).
- Suggested cases: `todo done ""`, `todo done "  "`, `today act "" x`, `todo done ../outside` (outside.md type todo, unchanged, no `state: done`), `today act ../outside x`; all with an id-less todo present.

## 8. `rk note rename ""` (report only)

Reachable and same defect. `findNoteByRefOrAlias` (note_v1.go:675) matches `n.ULID == ref || base == ref || alias`; an id-less note has ULID "". runNoteRename (note_v1.go:561) takes `ref := args[0]` and trims only the title (`newTitle`), with no empty-ref guard, then calls findNoteByRefOrAlias at :580. So `rk note rename "" "New"` would rename an arbitrary id-less note (first in walk order). Path escape is not a concern there (walks notesDir; no direct join). Out of scope; recommend a follow-up ticket.
