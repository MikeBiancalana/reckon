# Codebase Analysis: reckon-pjfa

Subsystem: `cli`.

## Repro (throwaway vault, one id-less note)

| Command | Before the fix |
|---|---|
| `rk note rename "" New` | exit 0; moves `notes/idless-note.md` to `notes/new.md` |
| `rk note rename " " New` | exit 1; `no note found matching " " (not found)` (no match, misleading message) |
| `rk note show ""` | exit 0; prints the id-less note (same defect, read-only, not named in the ticket) |
| `rk note show " "` | exit 1; not found |

## Touch points

| Location | Role |
|---|---|
| `internal/cli/note_v1.go:435` `runNoteShowE` | Takes `args[0]` as ref; query at :463 has `ulid = ?`, and id-less nodes hold an empty `ulid`. |
| `internal/cli/note_v1.go:559` `runNoteRenameE` | Takes `args[0]` as ref; slug validation of the title runs first (:564). |
| `internal/cli/note_v1.go:675` `findNoteByRefOrAlias` | `n.ULID == ref`; only caller is rename. |

## Pattern to follow

`resolveDurableTodoRef` (`internal/cli/today.go`) and `findDurableTodoByRefOrAlias` (`internal/cli/todo.go:944`): `strings.TrimSpace(ref) == ""` guard reports `<verb>: empty ref (give a todo ULID or alias)`; the walk uses `n.ULID != "" && n.ULID == ref`. Trimming is for the emptiness test only, never for matching.

## Sweep for the same defect

`grep -E 'ULID == |ulid = \?'` over `internal/cli` and `internal/index` finds only the two note sites above; `todo.go:944` is already fixed. `parseAliases` (`internal/node/node.go:484`) drops empty flow-list entries, so an empty alias cannot be parsed.

## Test helpers

`runNote` (`note_v1_test.go:123`), `requireRefused` (`todo_test.go:2506`; asserts exact error and an unchanged vault snapshot), `mustWriteFile` (`adopt_test.go:70`), `setupQueryVault`.
