# Code Review: reckon-pjfa
**Verdict:** APPROVE WITH CHANGES

Reviewed at e6a45a0. No CLI route selects a note through an empty value; the bug is fixed. The requested changes are small: the walk helper still has one empty-value match, and one acceptance clause has no test.

## Correctness

1. **[Minor]** `findNoteByRefOrAlias` still matches an empty ref through `base == ref` (internal/cli/note_v1.go:681). A note at `notes/.md` has base `""`. Adding that file to the existing walk test fails with `empty ref matched .../notes/.md, want no match`. The CLI cannot reach this, because the only caller (:586) sits behind the guard. It does contradict the plan's "safe for future callers" and the test name `..._EmptyRefNeverMatches`. Fix: start the walk with `if ref == "" { return nil, "", nil }`, which closes every arm in one place; the ULID conjunct may stay, matching todo.go:944. Add `notes/.md` to the test fixture. Applied in a scratch copy, this passes all `Note` tests in `internal/cli`.

### Empty-value routes

Probed with the real binary in throwaway vaults (`RECKON_VAULT`, `RECKON_DATA_DIR` and `RECKON_CACHE` all set).

| Route | Input | Result |
|---|---|---|
| `note rename`, `note show` | `""`, `"  "`, `"\t\n"`, NBSP (`show` only) | refused with exit 1; vault unchanged |
| `note rename "" ""`, with and without `--json` | | ref error wins over title validation |
| Filename | `notes/.md`, `notes/  .md` | refused at the verb; `.md` is not indexed (8 files gave 7 nodes) |
| Alias forms | `[,b, ,]`, block `- ` item, bare `aliases:` | no empty alias rows: `trim(alias)=''` counts 0 |
| Quoted empty alias | `""`, `''` | stored as the literal 2-character string; unset variables cannot match it |
| `id:` with no value | | no `ulid` in the node |
| `today open ""` | | not found; it queries `id` (id-less rows use `file:<path>`) and aliases, not `ulid` |
| `today act`, `todo done`, `todo edit` | `""` | refused (reckon-kcpb) |
| `note create ""` | | slug error |
| `adopt ""` | | path-outside-vault error; no change |
| `note index` | | takes no ref |

Other resolver: `resolveNoteIDBySlug` (internal/cli/tui_read.go:132) reads only `aliases`, so an empty slug finds no row.

Pre-fix baseline on an origin/main build: `note show ""` printed the id-less note and `note rename "" New` moved it, confirming the ticket.

### Plan decision: no `ulid != ''` in the show SQL

Agree. `runNoteShowE` is the only path to that query (:466) and the guard is at :438 in the same function, so the extra clause would be dead code no test could exercise. Removing the guard fails `TestNoteShow_RejectsEmptyRef`, so a regression would be caught.

## Testing

Removing the fix fails the tests:

| Mutation | Fails |
|---|---|
| Rename guard removed | `TestNoteRename_RejectsEmptyRef`, all 4 subtests |
| Show guard removed | `TestNoteShow_RejectsEmptyRef`, all 3 subtests |
| ULID conjunct removed from the walk | `TestFindNoteByRefOrAlias_EmptyRefNeverMatches` |
| Guard without `TrimSpace` | `spaces` and `tab_newline` in both verb tests |
| Rename guard after title validation | `ref_over_title` only |
| origin/main `note_v1.go` with branch tests | all three tests |

`go test ./...`, `go test -tags integration ./tests/`, `go vet ./internal/cli/` and `gofmt -l internal/cli` are clean.

2. **[Minor]** Acceptance criterion 5 says alias refs keep working, but the rename walk's alias arm has no test. Dropping `containsString(n.Aliases, ref)` from note_v1.go:681 leaves the `internal/cli` and integration suites green. Existing rename tests all use a filename ref (note_v1_test.go:402, :466, :828). Fix: give the `with-id` fixture an alias and add it to the positive-ref table in `TestFindNoteByRefOrAlias_EmptyRefNeverMatches`.

## Maintainability

3. **[Minor]** For `note show`, the new message's "slug" holds only when the slug is also an alias, and the plan's "Notes also resolve by filename slug" is true only of rename. `rk note show no-alias` on `notes/no-alias.md` (has a ULID, no alias) returns not found, while `note rename no-alias Other` succeeds. Notes from `rk note create` work because the slug is minted as an alias. An id-less note with no alias cannot be shown at all, now that the accidental `""` route is gone. No change needed in this PR; file a follow-up to make `show` resolve filename slugs as `rename` does, which also makes the message accurate.

## Resolution

| Finding | Outcome |
|---|---|
| 1 | Fixed: the walk returns no match on an empty ref, replacing the ULID check. `notes/.md` fixture added. |
| 2 | Fixed: alias case added to the walk test; removing the alias arm now fails it. |
| 3 | Filed as reckon-svdd. |
