# Code Review: reckon-s6oh (v1-T9 — migrate DB-primary data to text-truth)

**Verdict: APPROVE WITH CHANGES**

## Summary

This is a strong, well-tested implementation that faithfully executes the plan. It builds
and vets cleanly, and the full suite passes under `-race` for both `internal/textmigrate`
and `internal/cli`. The `NewNode → Render → Parse → writeFileAtomic` recipe is used
consistently, so the round-trip keystone (AC2) holds by construction; error handling,
resource cleanup, and the `requiresDB:false` wiring all match repository conventions. Test
coverage maps almost 1:1 onto the acceptance scenarios, including the hard ones
(idempotency no-op/partial re-run, cross-type xid collision, missing note file, malformed
day, dry-run isolation).

The changes I'm requesting are narrow: (1) `Verify` computes per-type counts but never
actually compares them to the source, so its `OK` verdict is weaker than the plan's stated
contract; and (2) one env-var restore is not panic-safe. Neither produces a silently-wrong
migration in the common path, hence APPROVE WITH CHANGES rather than REQUEST CHANGES. Two
further items (the alias-collision suppression heuristic, and the checklist schema having no
consumer) are correct-as-designed but warrant an explicit owner decision.

---

## Critical Issues

None that produce incorrect migration output in the common path. The two "requested
changes" below are correctness-completeness gaps, not data-corruption bugs.

---

## Requested Changes

### R1 — `Verify` never asserts per-type counts against the source (verify.go:54, 83–88)

The plan's verify contract (plan.md §Dry-run/verify, item **(b)**) is: "per-`type` row
counts equal source counts (AC1)." The implementation computes `counts` via `countByType`
(verify.go:54) and returns them in `VerifyResult.Counts`, but the `OK` determination
(verify.go:87) is:

```go
OK: len(unexpected) == 0 && len(unresolved) == 0,
```

Counts are never compared to source counts — they are purely informational. A verify run can
therefore report `OK == true` while `Counts["todo"]` exceeds the source task count (e.g. a
pre-existing/native todo already in the vault, or a stray extra node), which contradicts the
plan's item (b).

This is *partially* mitigated: `unresolvedAliases` enumerates every source id and fails if any
doesn't resolve, so a *dropped* source record (task/note/checklist/journal-day) is caught
because its alias would be unresolved, and a *duplicated* record would surface as
`duplicate_ulid`/`alias_collision`. But the mitigation is indirect, doesn't cover count
*excess*, and doesn't cover dropped log-entries *within* a migrated day (those carry no source
alias). Scenario 11 only literally requires verify to "report" the counts, which it does — but
the plan's own stronger wording is not met.

**Fix:** either (a) have `Verify` compare `Counts[type]` against the source per-type counts
(you already gather source records in `sourceAliasCounts`; extend it to also return per-type
tallies) and fold a mismatch into `OK`; or (b) if count-equality is deliberately being enforced
only indirectly via alias-resolution, drop the "counts equal source" language and state in the
`Verify`/`VerifyResult` doc comment that count-equality for aliased types is enforced through
alias resolution and that raw `Counts` are advisory. Option (a) is closer to the plan.

### R2 — `sourceAliasCounts` restores the env override without `defer` (verify.go:148–150)

```go
restore := overrideDataDir(imp.Source)
tasks, err := journal.NewTaskService(nil, nil).GetAllTasks()
restore()
```

`runTasks` (tasks.go:79–80) correctly does `defer restore()`, so a panic inside
`GetAllTasks` still restores `RECKON_DATA_DIR`. Here the restore is not deferred, so a panic
mid-call leaves the process-global `RECKON_DATA_DIR` pointing at the migration source. Nothing
else in `sourceAliasCounts` reads that env var (the DB and journal paths are explicit
`filepath.Join(imp.Source, …)`), so deferring to end-of-function is safe.

**Fix:** `defer restore()` for consistency and panic-safety.

---

## Recommendations (prioritized)

### 1 — Flag the alias-collision suppression heuristic to the owner (verify.go:65–76) — Risk area #3

The distinction between a "cross-type collision" (surfaced) and a "disambiguated collision"
(suppressed) is implemented as:

```go
if w.Kind == "alias_collision" && sourceAliasCount[w.Alias] <= 1 {
    continue // accepted collateral
}
```

The logic is *correct for the two tested cases*:
- Two source records claiming the same id (task+note both `shared-legacy-id-123`) →
  `sourceAliasCount == 2` → **surfaced** (verify_test.go:88, scenario 19/EC-1). Good.
- A migrated note whose retained old slug collides with a *pre-existing native* vault node
  (only the one legacy record produced that string) → `sourceAliasCount == 1` → **suppressed**
  (verify_test.go:63, notes_test.go:78). This is the "disambiguated collision" case.

The subtlety worth the owner's eyes: in the suppressed case, the migrated note's *filename* is
disambiguated (`weekly-review-2.md`), but its retained old-slug *alias* (`weekly-review`) now
genuinely collides with the pre-existing node's alias. `resolveEdges` resolves that alias by
`ORDER BY node_key LIMIT 1` — deterministic but arbitrary — so an inbound `[[weekly-review]]`
may now resolve to a *different* node than before, and `Verify` still reports `OK`. That is
exactly the "resolved to one arbitrary winner" outcome scenario 19 says should "never" happen,
for the alias (not filename) dimension. The plan's "Global flat alias namespace" risk note even
says "Verify must surface these; a real collision blocks 'done.'"

This is a defensible design choice (the migration can't un-ambiguate a human name the user
reused in both systems, and failing verify on every pre-existing-slug overlap would be noisy),
but it is a real weakening of the AC3/scenario-19 guarantee. Recommend either surfacing these as
a distinct *non-failing* "advisory collision" line in the CLI output (so they're not invisible),
or getting explicit owner sign-off that suppressing them is intended. Note also that
`notes_test.go:78` is named `…BothAliasesResolve` but only asserts alias *presence* + "no
warnings"; it never asserts that both aliases actually resolve in the index — the test
under-tests its own claim.

### 2 — Guard the `RECKON_DATA_DIR` override against future concurrent use (migrate.go:184–194) — Risk area #2

`overrideDataDir` mutates a process-global env var to route `GetAllTasks` (which resolves its
directory through `config.TasksDir()` with no injectable param). This is safe *today*: the
importer is a one-shot CLI, and the tests run sequentially (none call `t.Parallel()`). It is
**not** safe if two `Importer.Run`/`Verify` calls ever overlap: `os.Setenv`/`os.Getenv` are
internally locked (so `-race` will *not* flag it, and indeed didn't), but the value can be
logically interleaved so `GetAllTasks` reads the wrong source dir. The restore-to-previous
logic also means two concurrent overrides can clobber each other's saved value.

Not a defect in the current design, but the failure mode is invisible to the race detector, so
it should be documented. Recommend a one-line caution in the `overrideDataDir` doc comment
("not safe under concurrent importer runs; the importer is single-shot by contract") so a future
change that adds `t.Parallel()` or a concurrent driver doesn't silently corrupt routing. The
cleaner long-term fix is an injectable tasks-dir on `TaskService`, but that's out of scope here.

### 3 — Dry-run creates `source/tasks` as a side effect (tasks.go:79 → config.TasksDir → MkdirAll)

Under `--dry-run`, `runTasks` still calls `GetAllTasks`, which calls `config.TasksDir()`, which
`os.MkdirAll`s `RECKON_DATA_DIR/tasks` (and `DataDir` MkdirAll's the root). This touches the
*source*, not the vault, so the dry-run tests (which only assert the vault is untouched)
correctly pass, and it's harmless in practice (the source already exists). But it's a minor
surprise against the "touches no files" framing. Worth a comment noting the source-side dir
creation is inherited from the legacy reader, or nothing at all if you consider it immaterial.

### 4 — Checklist schema: add a short in-code description of the invented shape (checklists.go) — Risk area #4

The `checklist-template`/`checklist-run` schema (types, `name`/`status`/`started`/`completed`
props, `instance-of` link, item lines as body) is internally consistent and does **not** paper
over the "no consumer yet" gap — the plan explicitly flags it to the owner, and the code emits
clean, round-tripping nodes (verified: the `instance-of: "[[xid]]"` typed edge parses back to a
`Link` and resolves via the template's alias — checklists_test.go:40–46). Good catch on the
`ListTemplates` gap: it does not populate `Items`, so the code explicitly calls
`GetTemplateItems` (checklists.go:97–104); `ListRuns` *does* populate items via `GetRunByID`, so
runs are read correctly.

The one improvement: the schema decisions currently live only in `plan.md` (ticket-work), not in
the codebase. A future `rk checklist` implementer reading `checklists.go` has no in-tree record
of *why* these fields/types were chosen. Per the repo's "no provenance in comments" rule, don't
cite the ticket — but a brief schema-describing doc comment on `convertChecklistTemplate`/
`convertChecklistRun` (the field/type contract a consumer must match) would de-risk drift.

### 5 — Trivial: string concatenation in loops (checklists.go:19–21, 38–44)

`convertChecklistTemplate`/`convertChecklistRun` build the body with `body += …` in a loop,
whereas `taskBody` (tasks.go) and the journal converters use `strings.Builder`. Item counts are
tiny so the O(n²) is irrelevant, but a `strings.Builder` here would match the rest of the
package for consistency.

---

## Positive Observations

- **Round-trip (AC2) is correct by construction and explicitly tested.** `renderAndParse`
  parses `Render()` output and `Serialize()` returns those exact bytes; `MintAt(sourceCreatedAt)`
  + `t.UTC().Format(RFC3339)` gives deterministic, timezone-correct ids/times (Risk area #5 —
  clean). roundtrip_test.go:19 exercises byte-identity across all five node kinds.
- **Idempotency (Risk area #1) is sound.** Alias-presence scan for tasks/notes/checklists and
  file-stat for journal days; the in-memory `migrated` set prevents within-run duplicate-id
  double-writes; ULIDs are globally unique per process (monotonic entropy under a mutex), so
  no dest-path collision. Because idempotency keys on the *alias*, the non-determinism of
  `MintAt(time.Now())` fallbacks (a task with no `created`, or an unparseable day date) does not
  break re-run safety. Full-rerun no-op and partial-rerun-completes are both directly tested
  (migrate_test.go:291, :331) including a byte-identity assertion on the un-touched file.
- **Atomic writes (Risk area #6) are clean.** Temp file in the same dir, `defer`-removed on any
  error via the `ok` flag, `os.Rename` for atomicity, mode preserved from an existing target.
  The `.import-*.tmp` prefix is hidden and non-`.md`, so a crash-orphaned temp is ignored by the
  alias scans — it can't be mistaken for a migrated node. (No `fsync`, matching the existing
  `adopt.go` pattern; not a regression.)
- **Journal preamble H3 choice is correct and defended by test.** `### Intentions/Wins/Schedule`
  never match `SplitEntries`' `^## ` splitter, so preamble content is preserved losslessly
  without fabricating phantom timed entries (journal.go:63–125; journal_test.go:118 asserts
  `SplitEntries()` length is unchanged). `j.Date = date` (journal.go:193) correctly trusts the
  filename over possibly-malformed frontmatter.
- **Error strategy is consistent with REVIEW_PATTERNS:** every per-record failure `%w`-wraps with
  the source id and accumulates into the report; the run always processes every record and
  returns a single summarizing error; `defer db.Close()`/`rows.Close()`/`stmt.Close()`/`ix.Close()`
  throughout; no `os.Exit`, `--quiet` honored, mutual-exclusion of `--dry-run`/`--verify` checked.

---

## Questions for Consideration

1. **R1 direction:** do you want `Verify` to hard-fail on a per-type count mismatch (stronger,
   matches plan item (b)), or is indirect enforcement via alias-resolution the intended contract
   (in which case the "counts equal source" language should be softened)?
2. **Recommendation 1:** is suppressing alias collisions against *pre-existing vault content*
   (retained old slug overlapping a native node) the intended behavior, given the plan's "a real
   collision blocks 'done'" note? If so, should those still be *shown* (as advisory, non-failing)
   so they aren't invisible to the operator?
3. **Checklist consumer:** confirm the `checklist-template`/`checklist-run` type names and prop
   keys are the ones a future `rk checklist` tool will adopt, since nothing reads them yet and
   changing them later means re-migrating.

---

## Verification performed

- `go build ./...`, `go vet ./internal/textmigrate/... ./internal/cli/...` — clean.
- `go test -race ./internal/textmigrate/...` and `go test -race -run TestImport ./internal/cli/...` — pass.
- Traced field usage against source types (`journal.Task`, `models.Note`, `checklist.Template/Run`,
  `journal.{Intention,Win,ScheduleItem,LogEntry,Journal}`) — all correct, including
  `ListTemplates` not populating `Items` (handled) and `ListRuns` populating them via `GetRunByID`.
- Confirmed `config.TasksDir()`/`DataDir()` read `RECKON_DATA_DIR` live (so the override routes
  `GetAllTasks`), and that `nodes`/`aliases` are real public views over `_nodes`/`_aliases`.
