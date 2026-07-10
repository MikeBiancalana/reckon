# Code Review — reckon-ih5g (v1-T8: `rk note` create/show/rename/index + linking)

## Summary

This is a clean, well-scoped implementation that delivers all 14 ACs as thin
CLI orchestration over the existing `internal/node` + `internal/index`
machinery, exactly as the plan intended. The one invariant-sensitive change —
`blockSpans` + `SetAliases` in `internal/node/node.go` — is correct: the span
math is right (block start through the last continuation line's ending),
sibling keys are preserved, the block/scalar/absent routing via `HasField`
(which keys on `fieldSpans`, not `frontmatter`) dispatches correctly, and every
mutation re-parses so downstream spans are never stale. I verified this
end-to-end (block-list rename collapses to `aliases: [project-x, proj-x,
project-y]` with `tags: [x, y]` untouched and no duplicate key), and the full
suite (`go test ./...`) plus the round-trip fuzz gate pass. The rename
write-new-then-remove-old ordering is the safe one (prefers a transient
duplicate over data loss and returns an error on partial failure). Show/index
SQL is parameterized and uses the correct node_key/ulid/dst_key columns. The 21
tests assert real behavior (file bytes, derived aliases, resolved `dst_key`,
determinism), not exit codes — TS-8 in particular pins the block-list
corruption repro.

Findings are all Minor: robustness/consistency nits and one documented
pre-existing limitation surfaced on a new path. Nothing breaks a documented AC
or the normal-usage happy path.

## Functional Review

1. **[Minor]** `--dir` accepts path traversal, writing notes outside the vault — internal/cli/note_v1.go:257-265. `targetDir = filepath.Join(notesDir, dir)` cleans `..`, so `--dir "../../ESCAPED"` writes `<vault>/../../ESCAPED/<slug>.md` (verified: the file lands outside `notes/`). Such a note is then invisible to `slugCollision`, `findNoteByRefOrAlias`, `note show`, and `note index` (all scoped under `notes/`), and could clobber an arbitrary `*.md` at the traversed location. Low real-world impact today (user-supplied local flag, `.md`-only blast radius), but the plan itself floats deriving `--dir` from tags later, which would feed less-trusted input in. Suggested fix: after joining, reject any `dir` whose cleaned path escapes `notesDir` (e.g. `rel, err := filepath.Rel(notesDir, targetDir); if err != nil || strings.HasPrefix(rel, "..") { return error }`).

2. **[Minor]** Block-list alias containing a literal comma/bracket is materialized lossily to disk on rename — internal/cli/note_v1.go:521 (via node.go `SetAliases`). An Obsidian block item `  - Doe, Jane` (one alias) is mis-split by the parser's flow-string synthesis into `["Doe","Jane"]` at parse time (a documented, pre-existing limitation, internal/node/AGENTS.md:119-124), and `SetAliases` now writes that mis-split view back as `aliases: [Doe, Jane, ...]` — so the on-disk file, which Obsidian previously read as one alias, permanently becomes two (verified). This is not newly-introduced corruption (reckon's derived view was already wrong, so `[[Doe, Jane]]` never resolved as one alias in reckon), but rename is the point it becomes persistent on disk. Suggested fix: none required (documented); optionally quote items that contain a separator, or note the limitation in the `rk note rename` help text.

3. **[Minor]** `rk note show` resolves any node type, not just notes — internal/cli/note_v1.go:361-364. The resolution query has no `type='note'` predicate, so `rk note show <todo-alias>` happily prints a todo (verified: returned `"type":"todo"`). Harmless as a generic display, but surprising under a `note` verb. Suggested fix: add `AND type='note'` (returning a clearer "not a note" error), or accept it as an intentionally generic viewer and document that.

4. **[Minor]** Dead `--author` flag on `rename` — internal/cli/note_v1.go:127-128. `noteRenameCmd` registers `--author` bound to `noteAuthorFlag`, but `runNoteRenameE` never reads it (rename preserves the note's existing `author:`). The flag is inert and misleading. Suggested fix: remove the registration, or wire it if changing author on rename is desired.

5. **[Minor]** Bare error returns drop command context — internal/cli/note_v1.go:246, 341, 382, 386, 466, 651. `output.ModeFromFlags`/`Print` and the `loadNoteForwardLinks`/`loadNoteBacklinks` calls return `err` unwrapped while their siblings (lines 251, 374, 378) wrap with `"note <verb>: %w"`. Cosmetic inconsistency (the helpers already self-wrap), matches preflight findings 1 & 2. Suggested fix: wrap uniformly.

6. **[Minor]** `note show` alias order is non-deterministic in JSON — internal/cli/note_v1.go:376/395 via query.go:488 `loadAliases` (`SELECT alias FROM aliases WHERE id=?`, no `ORDER BY`). The `aliases` array in `noteShowResult` therefore has storage-order-dependent ordering, which undermines diffable/scriptable JSON output. Suggested fix: `ORDER BY alias` in `loadAliases` (shared helper — check no caller relies on current order) or sort in `noteShowResult`.

7. **[Minor]** `rk note index` clobbers a hand-authored `notes/index.md`, and never cleans a stale one — internal/cli/note_v1.go:724-727. A user-authored `notes/<dir>/index.md` is overwritten by the generated catalog (the reserved-slug guard blocks *creating* a note that slugifies to `index`, but not a hand-placed `index.md`); and when every note in a directory is deleted, that directory drops out of `byDir` so its now-stale `index.md` is never regenerated or removed. Consistent with "`index.md` is a generated, tool-owned artifact," but worth a one-line note in the command help. Suggested fix: document the ownership contract; optionally skip-with-warning if an existing `index.md` lacks the generated header.

8. **[Minor]** Test gap: `SetAliases` block-list-at-EOF and CRLF shapes are not directly unit-tested — internal/node/node_test.go. The `blockAliases` fixture always has a trailing sibling (`tags:`) after the block, so the span-`End == closeAbs` case (block list as the *last* frontmatter key) and a CRLF block list are exercised only by reasoning, not assertion. The code is correct for both (verified by inspection; rename guards CRLF out anyway), but a fixture with `aliases:` as the final key would lock in the EOF span math. Suggested fix: add an EOF block-list fixture to `TestSetAliases_BlockListCollapsesToFlow`.

## Design Review

**Architecture / plan adherence — sound.** The ticket is delivered as the plan
promised: four subcommands attached to the legacy `noteCmd` with per-subcommand
`requiresDB:"false"`, reusing `resolveAuthor`, `writeFileAtomic`, `loadProps`,
`loadAliases`, `output.ModeFromFlags`/`Print`, the `--quiet` suppression idiom,
and the `NewNode → Render → Parse → writeFileAtomic` create recipe. No
`os.Exit`, errors wrapped (modulo finding 5), no changes to `internal/index`.
Blast radius is exactly as bounded.

**The byte-preservation invariant is respected.** `blockSpans` is additive and
read only by `SetAliases`; every edit path (`SetField`, `InsertField`, the
block-splice) re-parses from spliced bytes so spans are recomputed, never
stale. The block→flow collapse is a deliberate, localized rewrite of a single
field on an already-mutating path (rename), not a general regenerate — the
correct call. Gating it on `TestRoundTripIdentity`/`FuzzRoundTripIdentity` plus
the targeted test (which re-checks the whole corpus inline) is the right level
of paranoia for the codebase's most invariant-sensitive function. The AGENTS.md
updates are accurate and appropriately scoped, including honestly restating the
comma/bracket limitation (finding 2).

**Collision strategy — matches the accepted tradeoff.** Filesystem-scan
rejection at create/rename time for the note-vs-note case (checking filename
*and* parsed aliases, excluding self on rename, skipping CRLF/unparsable files),
with cross-type collisions left to the reconcile warning, is exactly what the
plan justified. The `--slug` escape hatch works. The O(N) parse-every-note scan
per create/rename is acceptable at expected vault sizes and is a conscious
choice (no index dependency on the write path).

**Rename flow — safe.** Write-new-then-remove-old is the correct ordering
(transient duplicate over data loss; partial failure returns an error). The
same-slug/title-only path correctly writes in place with no remove. `id:` is
never touched. The pre-mutation collision check plus the `noteFiles` filename
check means an existing file at the target slug is never clobbered.

**Index generation — deterministic and correctly neutralized.** Plain markdown
links (not wikilinks) avoid injecting synthetic `references` edges; the
`slug=="index"` skip plus the `index.md` filename exclusion in `noteFiles`
keeps generated catalogs out of the note namespace; per-directory grouping with
slug-sorted entries and a sorted `files` result are byte-deterministic across
runs (verified, including the second-run re-index of the just-written
`index.md`). Note the per-directory-flat listing means a parent `index.md` does
not surface notes nested in subdirectories — a reasonable reading of AC-14, but
worth confirming it matches the OKF progressive-disclosure intent.

**Testing — genuinely asserts the ACs.** Tests check on-disk bytes, derived
`Aliases`, resolved `dst_key`, backlink `src`, and index determinism rather
than exit codes. TS-8 pins the block-list corruption repro with a no-duplicate-
key assertion; TS-6 proves dangling→resolved with no source edit; the
end-to-end test covers create→index→query→show. None would pass against a
no-op implementation.

Verdict: APPROVE WITH CHANGES
