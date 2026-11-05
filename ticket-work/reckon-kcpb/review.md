# Code Review: reckon-kcpb

**Verdict:** APPROVE WITH CHANGES

Both bugs are closed: an empty ref no longer matches an id-less todo, and `rk todo done` can no longer write outside `todos/`. Tests pin both fixes. The two findings are Minor: one comment/guard mismatch to fix now, and one follow-up to file.

Evidence came from `go test`, branch and `origin/main` binaries run against throwaway vaults, and mutations applied to a scratch copy of the tree.

## Correctness

Probe results on the branch:

| Ref (`todo done`) | Result |
|---|---|
| `""`, `"  "` | empty-ref error |
| `../outside`, `../../outside`, `<vault>/outside`, `./foo`, `foo/`, `//`, `sub/foo` | not found, no write |
| `..`, `.`, `/` | joined inside `todos/`: `todos/...md`, `todos/..md`, `todos/.md` |

- **Wrong-todo completion:** the only remaining surprise is the last row, and it needs a file with one of those odd names to exist. Maintainability issue 1 covers it.
- **`today act`:** it goes through the same guard (`today act "" d` gives the empty-ref error). `today act "" bogus` returns the unknown-key error, which the AC allows.
- **Callers:**
  - `todo.go:695` passes the user's ref directly.
  - `todo_browse.go:135` and `tui_keyboard.go:124` pass index ids (a ULID, or `file:<path>` for id-less nodes). The index never yields an empty id, so neither can send `""`. They resolve the same as on main, including not-found for `file:` ids and nested ULIDs.
- **Verb prefix:** nothing depends on `todo done:` from `today act`. The TUI only displays the error, and no test matches the old prefix on that path.

### Issues

1. **[Minor]** Nested todos lose their only completion route, which the plan's risk note does not record: `rk adopt` stamps `todos/sub/*.md` and `todo list`/`today` show them, but `todo done <ULID>`, `todo done sub/bar` and `today act <ULID> x` all fail on the branch, while on main the path form worked; AC 3 requires the narrowing, so this is not an implementation bug, and the follow-up is to make the walk recursive, which still matches only ULID or alias and so does not bring path resolution back (internal/cli/todo.go:931)

## Architecture

One resolver now serves `todo done`, `todo edit` and `today act`, which removes the divergence that caused the escape. The guard in the resolver also covers the browse and TUI callers, which skip the cobra layer. `todo_edit.go:64` keeps its own guard so its error still comes before flag validation. The AC scopes that duplication out, so it is not a finding. The plan was followed with no deviations.

### Issues

None found.

## Testing

Each mutation below turned the listed tests red:

| Mutation | Tests that failed |
|---|---|
| Walk's non-empty ULID check removed | `TestFindDurableTodoByRefOrAlias_EmptyRefNeverMatches` only |
| Resolver guard removed, or weakened to `ref == ""` | `TestTodoDone_RejectsEmptyRef`, `TestTodayAct_RejectsEmptyRef`, `TestResolveDurableTodoRef_EmptyRefUsesCallerVerb` |
| `filepath.Base` check removed | `TestTodoDone_RefDoesNotEscapeTodosDir`, `TestTodayAct_RefDoesNotEscapeTodosDir`, `TestTodoEdit_RefDoesNotEscapeTodosDir`, `TestTodoDone_PathRefNotResolvedByDirectJoin` |
| `doneDurableTodo` fast path restored | `TestTodoDone_RefDoesNotEscapeTodosDir`, `TestTodoDone_PathRefNotResolvedByDirectJoin`, `TestTodoDone_InboxContainerNotAMatch` |
| `today act` verb reverted | `TestTodayAct_RejectsEmptyRef`, `TestTodayAct_CRLFRefusalUsesTodayActPrefix` |
| Type filter removed | `TestTodoDone_InboxContainerNotAMatch`, `TestTodoEdit_EphemeralInboxRefNotFound` |
| Alias match removed | `TestTodoDone_PathShapedAliasResolvesViaWalk`, `TestTodoDone_DurableViaAlias`, `TestTodoEdit_AliasRefEditsSameFileAsULID` |
| Stem join removed | `TestTodoDone_IDLessTodoByFileStemStillResolves`, plus existing CRLF and guard tests |

- **Walk fix:** only the direct-call unit test pins it, because the resolver guard masks it through the CLI. The plan intended this.
- **`TestTodoDone_NonEscapingPathShapesNotFound`:** this is a regression guard that passes on main, as the plan says. Its `dotdot` and `dot` cases create no `todos/...md` or `todos/..md`, so they cannot detect the join in Maintainability issue 1.
- **`absolute` table entry (preflight Info 1):** the overwritten value is what gets used and asserted, so the subtest does exercise an absolute ref. This is readability only and is not repeated as a finding.

### Issues

None found beyond the fixture gap, which is part of Maintainability issue 1.

## Maintainability

The new comments carry no provenance: no ticket IDs, plan labels or phase names in any added line. `reckon-vj55` in `loadDurableTodoAt` predates this diff.

### Issues

1. **[Minor]** Comments on the now-only resolver overstate the guard: `today.go:439-440` and `:455` say a path-shaped ref resolves by alias only, and `:467-468` says `..` must not reach the direct join, yet `..`, `.` and `/` all equal their own `filepath.Base`, so they reach the join and `rk todo done /` completes `todos/.md` if that file exists; this matches main and stays inside `todos/`, so either reword the comments or, preferably, skip the join when the ref contains a separator or is `.` or `..`, and add those three files as fixtures to pin it (internal/cli/today.go:469)

## Error handling

New errors carry the caller's verb and follow the existing `todo edit` wording. The not-found text is byte-identical. A ref too long for a filename gets a `read ...` error instead of not-found. That predates this branch, refuses without writing, and is not a finding.

### Issues

None found.

## Performance

Delegating to the resolver adds no I/O to `todo done`: one stat-like read, then the same glob walk as before. `today act` still opens and reconciles the index before the guard runs, which the plan recorded and which touches only the cache DB.

### Issues

None found.

## Security

Path traversal through a ref is closed for all three verbs. No ref shape tried writes outside `todos/`, including `../outside`, `sub/../../outside`, `../../outside` and absolute paths. A path-shaped alias only ever selects a file already found by the `todos/*.md` glob. Symlinks inside `todos/` are out of scope per the AC.

### Issues

None found.

## Required change

- Fix the comment/guard mismatch (Maintainability issue 1).

## Follow-ups to file

None of these block this change.

- Make the `todos/` walk recursive (Correctness issue 1).
- `findNoteByRefOrAlias` has the same empty-ULID defect at `note_v1.go:675` (plan Decision 5, not yet filed).
- `todo list` builds `path` from the id, so nested todos show `todos/<ULID>.md` and id-less ones show `todos/file:todos/idless.md.md`. This predates the branch.
- `today act x` can still emit `todo done:` errors from `completeDurableTodoNode` (the plan's [OPEN] item).

## Resolution

Maintainability issue 1 fixed: the direct join is now skipped for refs containing a separator and for `.` and `..`, which makes the comments accurate. `TestTodoDone_DotAndRootRefsSkipDirectJoin` pins it, with a fixture at each name a join would produce; it was red before the fix. Not re-reviewed: the change is that one condition, and the verdict required no further review.
