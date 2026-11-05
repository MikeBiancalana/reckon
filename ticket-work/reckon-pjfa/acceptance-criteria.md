# Acceptance Criteria: reckon-pjfa

## Explicit

1. `rk note rename <ref> <title>` refuses an empty or whitespace ref.
2. `findNoteByRefOrAlias` matches nothing on an empty ref, neither an empty ULID nor `notes/.md`.

## Implicit

3. A refused rename leaves the vault byte-identical: no new file, no removed file.
4. The ref error wins over title validation: `rename "" ""` reports the ref, not the slug.
5. ULID, slug and alias refs keep working, including a slug ref to an id-less note.

## Added scope

6. `rk note show` refuses an empty or whitespace ref (same defect, found during repro).

## Scenarios

| # | Given | When | Then |
|---|---|---|---|
| 1 | id-less note | `rename "" New`, `rename "  " New`, `rename "\t\n" New` | `note rename: empty ref (give a note ULID, slug, or alias)`; vault unchanged |
| 2 | id-less note | `rename "" ""` | same ref error |
| 3 | id-less note | `show ""`, `show "  "`, `show "\t\n"` | `note show: empty ref (give a note ULID, slug, or alias)`; vault unchanged |
| 4 | id-less note, `notes/.md` and note with ULID | `findNoteByRefOrAlias` with `""` | no match, no error |
| 5 | same | `findNoteByRefOrAlias` with the ULID, an alias, and the id-less slug | each resolves to its own file |

## Out of scope

- Non-note verbs; `todo` was fixed in reckon-kcpb.
- Trimming refs before matching.
- Path-shaped note refs: the note walk has no direct path join, so they cannot escape `notes/`.
