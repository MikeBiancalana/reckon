# Plan: reckon-pjfa

## Summary

`rk note rename "" New` renamed the first note with no `id:` line, because an empty ref compared equal to its empty ULID. An unset shell variable could therefore move an arbitrary file. `rk note show ""` had the same defect and printed that note.

Both verbs now refuse an empty or whitespace ref with `<verb>: empty ref (give a note ULID, slug, or alias)`, and the rename walk matches nothing on an empty ref. ULID, slug and alias refs behave as before.

`note show` is not named in the ticket. It is included because it is the same defect in the same file, and the fix is one shared guard.

## Files to modify

| File | Change | Reason |
|---|---|---|
| `internal/cli/note_v1.go` | New `checkNoteRef(verb, ref)` helper; first check in `runNoteRenameE` and `runNoteShowE`. | One message, ahead of title validation and config/index work. |
| same | `findNoteByRefOrAlias`: return no match on an empty ref. | Covers both the empty ULID and the base of `notes/.md`, so the walk is safe for future callers. Review showed a ULID-only check left the `.md` route open. |
| `internal/cli/note_v1_test.go` | Tests below. | Pins refusals and the walk. |

## Design decisions

| Decision | Alternative | Why not |
|---|---|---|
| Guard at verb level | Guard inside the walk | `show` does not use the walk. |
| Message names slug as well as ULID and alias | Reuse the todo wording | Notes also resolve by filename slug. |
| No `ulid != ''` in the `show` SQL | Add it as a second layer | Unreachable directly below the guard in the same function, so no test could cover it. The walk is a separate function, so its check stands on its own. |
| Trim for the emptiness test only | Trim before matching | Would change how existing refs match. |

## Test scenarios

See `acceptance-criteria.md`.

| Test | Scenarios |
|---|---|
| `TestNoteRename_RejectsEmptyRef` | 1, 2 |
| `TestNoteShow_RejectsEmptyRef` | 3 |
| `TestFindNoteByRefOrAlias_EmptyRefNeverMatches` | 4, 5 |

## Risks

A note reachable only through a whitespace-only filename can no longer be addressed by that name. Accepted; `rk note rename` could not sensibly target it anyway, and its ULID or alias still works.

## Process note

Analysis and planning ran inline, not in sub-agents: the repro and sweep had already answered them, and the diff is a few lines. Review still runs as an independent agent.
