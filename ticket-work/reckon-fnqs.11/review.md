# Code Review: `rk checklist` CLI verb surface (reckon-fnqs.11)

**Verdict: APPROVE WITH CHANGES**

Single required change: resolve the dead `checklistRunResult.resumed` field (details in Critical/Recommendations). Everything else is production-quality. Build, `go vet ./internal/cli/`, and `go test ./internal/cli/` all pass locally (28 checklist test functions; package coverage 74.6%).

---

## Summary

This is a clean, faithful implementation of a thin CLI layer over the already-tested `internal/checklist` Service/Repository, with no domain-layer changes. The embedded-pointer result-type pattern for flat JSON is elegant and correct, error wrapping is consistent, and the test suite is thorough. There is exactly one loose end — a discriminator field that is set but never read — which the plan itself flagged as deferred; it should be closed because the accompanying comment describes behavior the code does not actually have.

I independently verified all seven focus areas from the review brief. Findings below.

---

## Critical Issues

None that block functionality. (The `resumed` item below is the required "change" for APPROVE WITH CHANGES, but it does not affect correctness of any working path or of JSON output.)

---

## Recommendations (prioritized)

### 1. Dead `resumed` field — set but never read (required change)

`checklist.go:197,422,430,433`. `checklistRunResult.resumed` is assigned in `runChecklistStartE` (fresh vs. resume detection) and carried into the result, but `checklistRunResult.Pretty()` (lines 200-225) never reads it. Confirmed by grep: the only other references are unrelated test message strings and a test asserting `resumed` is *absent* from JSON.

Two defects, not one:
- **Dead code**: a field computed and threaded through but never consumed — the same class as REVIEW_PATTERNS.md's "Computed offset not applied" / "Dead code after refactoring" anti-patterns.
- **Misleading comment**: lines 193-194 call it a "Pretty-only discriminator," but it discriminates nothing. A resumed run renders *identically* to a fresh one (`start foo` on an in-progress run and on a brand-new run both emit the header + items + `status: active`).

Impact is genuinely minor: JSON output is unaffected (the field is correctly excluded), and a resumed run's Pretty output *does* show the previously-checked items, so an attentive user sees prior progress. But AC-3 frames `start` as "starts (or resumes)," and today the Pretty text gives the user no explicit signal which occurred — the field exists precisely to carry that signal and doesn't.

Pick one, cheaply:
- **Wire it into `Pretty()`** (the plan's stated intent): prefix the header with e.g. `resumed ` vs `started ` when `r.resumed`. ~1 line, and it fulfills AC-3's observability.
- **Delete it**: remove the field, the `json:"-"` tag, the comment, and the `resumed` bookkeeping in `runChecklistStartE` (collapse to a plain `GetActiveRun`-else-`StartRun`). Removes the dead code and the misleading comment.

Either is acceptable; leaving it as-is is not, because the comment asserts a capability the binary lacks.

### 2. `TestChecklistList_RunsForTemplate` does not actually exercise cross-template exclusion (testing)

`checklist_test.go:243-303`. The test asserts scoping (`r.TemplateID != defaultRuns[0].TemplateID` ⇒ "scoping leaked"), but only ever creates one template (`foo`). With a single template in the DB, the client-side `TemplateID` filter's *exclusion* branch is never taken, so a regression that dropped the filter would still pass this test. The filter itself (`checklist.go:372-377`) is correct by inspection, but the test claiming to guard it has no second template to leak. Recommend creating a second template with its own run and asserting it's absent from `list foo`.

### 3. `json:"-"` on unexported `resumed` is redundant (nit)

`checklist.go:197`. Unexported fields are never marshaled by `encoding/json` regardless of tag, so the tag is decorative. `go vet` does not complain (verified). If option (a) above is taken the field stays unexported and the tag can be dropped; if (b), it disappears. No action needed independently of finding 1.

---

## Focus-area verification (all pass)

- **Position base (0-based) consistency** — Consistent end to end. `CreateTemplate` assigns `NewTemplateItem(..., i)` from `i=0`; `CheckItem` bounds-checks `position < 0 || position >= len` and indexes `run.Items[position]` with the same base; `Pretty()` prints `it.Position` verbatim; the CLI `check <position>` arg is passed through unmodified. No off-by-one. Out-of-range message (`position 5 out of range (run has 3 items)`) matches the test.

- **`list <template>` client-side filtering** — Correct. `ListRuns(includeCompleted)` is unscoped (repository.go:268), and `runChecklistListE` filters `r.TemplateID == tpl.ID`. No leak by construction. (Test-coverage caveat: finding 2.)

- **JSON shape (flat, model-tagged)** — Correct. The embedded `*checklist.Template` / `*checklist.Run` promote their fields to the top level under `json.Marshal`, producing flat model-tagged JSON; `resumed` is unexported and absent. `TestChecklistJSON_MatchesModelTags` asserts both the presence of model keys and the absence of `resumed`, and passes. No double-nesting.

- **`--items-file` edge cases** — Robust. `parseChecklistItemsFile` splits on `\n`, `TrimSpace`s each line, and skips blanks: trailing newline yields a trailing `""` that is skipped (no phantom empty item); CRLF is handled because `TrimSpace` eats the `\r`; leading/trailing spaces are trimmed; an all-blank file yields zero items → `at least one item required`. Mutual exclusion with `--item` is enforced before any file read.

- **Nil-slice → `[]` (not `null`)** — Handled for both list verbs. Templates forced non-nil (`checklist.go:355-357`); filtered runs forced non-nil (`checklist.go:378-380`). Empty `list --json` emits `[]` (verified by `TestChecklistList_Empty`). The `Pretty` path uses the empty-state message via the named-slice `Pretty()` / caller, so the forcing only matters for JSON/NDJSON, which is exactly where `null` would otherwise appear.

- **DB never left unclosed on early return** — No leak on any path. In every RunE, `defer db.Close()` is registered only after `setupChecklistRun` returns a nil error (and it returns a nil `db` on error, so there's nothing to close). `create` performs all input validation *before* opening the DB, so validation failures never open a connection. `output.ModeFromFlags` runs first inside `setupChecklistRun`, so `--json --ndjson` errors out before the open. `defer resetChecklistFlags(cmd)` is registered before `defer db.Close()`; LIFO ordering runs `Close()` first — both are independent and safe.

---

## Positive observations

- **Clean thin-layer separation** — no business logic leaks into the CLI; every verb is a small orchestration over the tested service. Matches REVIEW_PATTERNS.md's "Service Layer Pattern."
- **Elegant flat-JSON via embedded pointer** — `checklistTemplateResult` / `checklistRunResult` get model-tagged JSON for free while supplying a value-receiver `Pretty()`; the type-switch in `output.Writer.Print` dispatches to `Pretty()` correctly for values.
- **Consistent, contextful error wrapping** — every error is `fmt.Errorf("checklist <verb>: ...: %w", ...)`; no bare `return err` reaches the user unwrapped.
- **`--quiet` semantics are correct and deliberate** — mutation verbs (create/start/check/reset/abandon) suppress the Pretty confirmation; query verbs (list/status) always print because their data *is* the requested output. `status` even documents this at the call site (line 494).
- **`check` re-fetches via `GetRunStatus(run.ID)` rather than `GetActiveRun`** — correctly anticipates that the final check auto-completes the run (after which `GetActiveRun` would error). This EC is handled deliberately and tested (`TestChecklistCheck_AutoCompletes`).
- **`resetChecklistFlags`** correctly zeroes both the package-global flag vars and pflag `Changed` state, preventing cross-test leakage of `--all`/`--items-file`.
- **Parent `Long` help** plainly documents the non-vault-native / not-rebuilt-by-`rk index` limitation (AC-10), tested by `TestChecklistHelp_DocumentsLimitation`.

---

## Other dimensions

- **Architecture** — Sound. Per-invocation DB open in RunE with `defer Close()`, mirroring `rk index`; no package-level service global (correctly ignoring the stale `internal/cli/AGENTS.md` `initServiceE` pattern). `checklistCmd` registered directly in `root.go` alongside peers.
- **Security** — Nothing to flag. All SQL is parameterized (domain layer, unmodified); `PRAGMA foreign_keys = ON` with `ON DELETE CASCADE` on `checklist_runs`/`checklist_run_items` is present. `--items-file` reads a user-supplied local path in a local single-user CLI — no traversal/injection concern. No secrets or PII handled.
- **Performance** — Acceptable and out of scope to change. `list <template>` loads all runs then filters in Go (decision #6, because `ListRuns` is unscoped), and the repository's `ListRuns` does an N+1 `GetRunByID` per run. Both are fine at checklist-run scale and live in the unmodified domain layer; noting only for completeness.
- **Maintainability** — Good: focused helpers (`openChecklistService`/`setupChecklistRun`/`printChecklistResult`), clear section banners, no commented-out code. The one blemish is the dead `resumed` field (finding 1).

---

## Questions for consideration

- Is explicit fresh-vs-resume wording in Pretty output actually desired for `start` (AC-3)? If yes, take option (a) of finding 1; if the team is content that a resumed run simply redisplays prior progress, take option (b) and drop the field. Either resolves the finding — the current middle state (field + comment, no behavior) is the only outcome to avoid.
