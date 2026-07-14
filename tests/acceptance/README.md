# Acceptance suite — daily-driver workflows over the real binary

System tests for the composable v1: `TestMain` builds the actual `rk` binary
once, each test gets a throwaway vault + cache, and every assertion is made
against **what the binary leaves on disk or emits as JSON** — never against
internal packages. If the internals are refactored wholesale and these still
pass, the daily driver still works; if a behavior a workflow depends on
regresses, these fail regardless of how green the unit tests are.

```bash
go test -tags acceptance ./tests/acceptance/
```

Not run by a bare `go test ./...` (build-tagged, same pattern as the
`integration` tag) — wire into CI as its own step.

## What's covered (maps to the v1 pillars)

| Test | Workflow / invariant |
|---|---|
| `TestCapture_DayFileAuthoredAndBackfilled` | capture front door: day group file, provenance, `--at` backfill, inline entry ULIDs (T4) |
| `TestTodo_OpenToDoneClosesInPlace` | real task lifecycle: done flips `state:` in the todo's own file; list hides done by default |
| `TestTodo_RecurrenceAdvancesCursorAndLogsDid` | org-style stored `scheduled` cursor advance; rule stays open; `did::` audit entry in the day log (T6 + complete→log) |
| `TestTodo_EphemeralInboxLifecycle` | ephemeral/durable split: group-file inbox, done-by-index |
| `TestNote_CreateConventions` | T8 conventions: slug filename, self-minted alias, title/description/stage, **no** timestamp/updated field, stage enum |
| `TestNote_DanglingLinkResolvesAndBacklinks` | dangling `[[link]]` = unresolved edge; auto-resolves on target creation with **no source edit**; backlinks index-derived |
| `TestNote_RenameRetainsRedirect` | alias-redirect contract: old slug keeps resolving after rename |
| `TestNote_IndexDeterministicAndOwnershipSafe` | generated `index.md`: marker, byte-determinism across runs, hand-authored files never touched |
| `TestQuery_CrossTypeGraphReadOnly` | one graph across log/todo/note types; non-SELECT rejected |
| `TestToday_AgendaSurfacesActionableOnly` | T7 agenda: overdue + scheduled-today in; future, unscheduled, done out |
| `TestToday_ActCompletesInPlace` | agenda as actuator: `act <ref> x` closes in place + writes the `did::` audit entry; item leaves the agenda |
| `TestDailyDriver_EndToEnd` | the composed day: capture → todo → note linking the todo by ULID → done → cross-type edge queryable → day file tells the story |

## Extending

One test per workflow, not per flag — flag-level coverage belongs in the unit
tests. A good acceptance test here reads like a use case ("rename keeps old
links working"), runs only the public binary, and asserts on file bytes or
`--json` output. Use `newVault(t)` per test; tests are parallel-safe.

Known gaps (add as the features land or stabilize):

- `rk today` external work-ticket rows (read-only + `open` jump) once a feed exists.
- `rk import` legacy migration (T9) against a real gen-1 fixture.
- Checklist template → run materialization.
- `rk adopt` on hand-authored/Obsidian files.
- Concurrent capture (parallel `rk add` writers, `merge=union` behavior).
- TUI flows (needs a pty harness; deliberately out of scope for this layer).
