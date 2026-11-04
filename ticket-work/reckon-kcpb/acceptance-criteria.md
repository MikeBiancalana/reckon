# reckon-kcpb acceptance criteria

Bug: `rk todo done` / `rk today act` accept an empty ref (matches an id-less todo, since `n.ULID == ref` is true on empty) and `rk todo done` accepts a path-shaped ref that escapes `todos/`.

## 1. Acceptance criteria

1. `rk todo done ""` and `rk todo done "   "` exit non-zero with an empty-ref error; no file is modified.
2. `rk today act "" <key> ...` and whitespace-only ref exit non-zero with an empty-ref error prefixed `today act:`; no file modified. Holds for every key (`t`, `d`, `D`, `p`, `x`, `i`, `c`), not only `x`.
3. `rk todo done ../outside` (and any ref containing a path separator or `..` that is not a legitimate alias) errors `todo done: no todo found matching "<ref>" (not found)`; `<vault>/outside.md` is byte-unchanged.
4. `doneDurableTodo` (`internal/cli/todo.go:712`) resolves via the shared `resolveDurableTodoRef` (`internal/cli/today.go:456`), deleting its inline fast-path. One resolver for done, edit, and today act.
5. Empty/whitespace check lives in one place shared by the three entry points, mirroring `internal/cli/todo_edit.go:64` (`strings.TrimSpace(ref) == ""`). Defence in depth: `findDurableTodoByRefOrAlias` (`todo.go:944`) must also never match an empty ref. [INFERRED: covers callers like `todo_browse.go:135` that bypass the cobra layer]
6. Existing behaviour unchanged: ULID refs, alias refs, `inbox`-type-filter fall-through, idempotent already-done skip, recurrence branch.
7. `go test ./...`, `go vet ./...` pass.

## 2. Implicit requirements

- Whitespace-only refs rejected identically to empty (matches `todo_edit.go:64`).
- Error prefix follows the verb: `today act: ...` for `rk today act`, `todo done: ...` for `rk todo done`, `todo edit: ...` unchanged. Note `loadNativeTodoForEdit` (`today.go:443`) currently passes verb `"todo done"` into the resolver, so read/parse/CRLF errors from `today act` say `todo done:`; the empty-ref guard must not inherit that (use `today act:`). Fixing the other prefix is optional [OPEN].
- Not-found wording `todo done: no todo found matching %q (not found)` is byte-identical (tests/agents may grep it).
- Empty-ref check must run before any filesystem access and before key normalisation side effects; unknown-key error precedence for `today act "" bogus` is unspecified [OPEN: either is acceptable].
- `rk todo done --ephemeral` is unaffected: it takes a numeric index and never calls the durable resolver (`todo.go:687-695`, `doneEphemeralTodo`). `--ephemeral ""` keeps its own `requires a positive 1-based index` error.
- Stale comment at `today.go:438-441` (says doneDurableTodo "still also accepts" direct path) must be updated once the paths converge.
- `doneDurableTodo` error prefix stays `"todo done"` (pass as verb).

## 3. Edge cases

| Case | Expected |
|---|---|
| id-less todo under `todos/` + ref `""` | error, file unchanged (the bug) |
| Empty alias | impossible: `parseAliases` (`internal/node/node.go:484`) drops empty entries; only the empty ULID (id-less node) can match `""` |
| Path-shaped ref that IS a legitimate alias (e.g. alias `sub/foo`) | still resolves via the alias walk (resolver skips direct join when `ref != filepath.Base(ref)` but walks) |
| `..` | no direct join to parent; not found (`todos/...md` does not exist) |
| `.` | not found; must not resolve to the directory |
| `foo/` trailing slash | `Base("foo/")=="foo"` != ref, so direct join skipped; alias walk only; not found unless alias is literally `foo/` |
| Absolute path `/<vault>/outside` | direct join skipped; not found; file unchanged |
| Nested `sub/foo` with `todos/sub/foo.md` present | not resolved by direct join (policy: alias walk only, glob is `*.md` non-recursive) |
| `../outside` with `<vault>/outside.md` valid open todo | not found, file unchanged (the bug) |
| ref `inbox` | `todos/inbox.md` is type != todo; falls through to walk, not found (preserve) |

## 4. Test scenarios (fail before fix)

Place in `internal/cli/todo_test.go` and `internal/cli/today_test.go` following existing fixtures; assert on file bytes via `os.ReadFile` before/after.

| # | Given | When | Then |
|---|---|---|---|
| T1 | vault; `todos/idless.md` = valid `type: todo`, `state: open`, NO `id:` line; bytes snapshotted | `rk todo done ""` | error contains empty ref; file byte-identical |
| T2 | same | `rk todo done "  "` | same as T1 |
| T3 | same | `rk today act "" x` | error starts `today act:` (not `todo done:`); file byte-identical |
| T4 | same | `rk today act "" t` | error starts `today act:`; file byte-identical (use `t` as the non-x key; pin would otherwise write `pinned:`) |
| T5 | same | `rk today act "   " x` | error, byte-identical |
| T6 | `<vault>/outside.md` valid `type: todo`, `state: open` with an `id:`; `todos/` exists | `rk todo done ../outside` | error `todo done: no todo found matching "../outside" (not found)`; `outside.md` byte-identical |
| T7 | same | `rk todo done ..` / `rk todo done /abs/path/outside` | not-found error; file unchanged |
| T8 | todo with alias `sub/foo` in `todos/` | `rk todo done sub/foo` | resolves via walk, state becomes done |
| T9 | todo with ULID and alias | `rk todo done <ULID>` and `<alias>` | still succeeds (regression) |
| T10 | `todos/inbox.md` ephemeral container | `rk todo done inbox` | not-found error (unchanged); `rk todo done --ephemeral 1` still flips line 1 |
| T11 | id-less todo | `rk todo edit "" --priority 1` | still errors `todo edit: empty ref` (unchanged) |

Verify T1-T7 fail on current code before implementing (T3/T4 fail on current code by matching and mutating the id-less file; T6 by writing outside.md).

## 5. Out of scope

- Note resolver (`findNoteByRefOrAlias`).
- Symlinks inside `todos/` (e.g. `todos/x.md` -> outside) .
- Changing `todo edit`'s own guard at `todo_edit.go:64` (leave as is; may share a helper only if behaviour is identical).
- Index-backed (SQLite) lookups.
