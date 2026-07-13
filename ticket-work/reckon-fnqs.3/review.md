# Code Review: reckon-fnqs.3 — derived `title` column

**Verdict:** APPROVE WITH CHANGES

Scope reviewed: full `git diff origin/main` (7 commits). Production source: `internal/index/{title.go,schema.go,reconcile.go}`, `internal/cli/{todo.go,today.go}`; docs and tests. Fresh `go test -count=1` on the index + cli title/query surfaces passes.

## Summary

Small, well-scoped, faithful to the approved plan. The derivation is correct, the schema bump rebuilds existing indexes for real, the INSERT wiring lines up, and there is no scope creep versus the plan's Files-to-modify table. One actionable change: a production code comment (`agendaItem.Title // durable only`) is factually wrong and mirrored into a pinned-contract test header. Everything else is either correct or a plan-accepted, documented tradeoff.

The four risk-list items the task asked to scrutinize were verified against implementation logic (not just "tests pass") — see Correctness #1–#4.

---

## Correctness

**Minor**

1. `deriveTitle` (title.go:8–14) is correct on every edge class in title_test.go, verified by reading the logic, not the green bar: leading blank lines are skipped (iterates all `\n`-split lines, returns first non-empty); CRLF is handled because `strings.TrimSpace` strips the trailing `\r` (`"Ship it.\r"` → `"Ship it."`); whitespace-only body → all lines trim to `""` → returns `""`; no blank-line separator required. No defect.

2. SchemaVersion 2→3 (schema.go:12) rebuilds stale indexes rather than silently keeping title-less rows. Confirmed the full path: `Open`→`ensureSchema` (index.go:108–125) compares stored `schema_version` to the constant and calls `Rebuild()` on mismatch; `Rebuild` drops/recreates from `schemaDDL` (now carrying the `title` column) and re-indexes every file through `indexFile`→`insertNode`, which computes `deriveTitle`. Existing v2 vaults genuinely repopulate title. No stale-row hazard.

3. `_nodes` INSERT alignment (reconcile.go:270–274) is exact: 10 named columns (`node_key,ulid,type,time,author,body,title,loc_file,hash,mtime`), 10 `?` placeholders, 10 args in matching order (`title` slots between `n.Body` and `rel`). Because columns are named explicitly, physical table order (schema.go:30–41) is independent and also consistent. No off-by-one.

4. The `nodes` view column addition is append-last (schema.go:85; D6), and all production consumers use named columns (`note_v1.go:392,706`, `today.go:232,685,693`, `todo.go:489,577`, `query.go:422`) — no positional/`SELECT *` reader in production breaks. `rk query "SELECT id, title FROM nodes"` (canonical path, query.go:422) still drops `title`; this is D2's documented, accepted gap, consistent with canonical mode already discarding non-envelope columns — not a new wart.

## Architecture

**Minor**

1. Boundary respected as designed: `internal/node` untouched, derivation co-located with its sole consumer in `internal/index` (D3), CLI structs thread the pre-computed column through rather than re-deriving. No layering violation.

2. `rk query` intentionally not extended (D2) — the view gaining the column satisfies the DoD for the raw/NDJSON path with zero `query.go` change. Correctly scoped.

## Testing

**Minor**

1. Coverage is layered and honest: `TestDeriveTitle` (unit table), `TestReconcilePopulatesTitle` (integration — proves the derivation is wired into the write path, not merely correct in isolation), CLI pretty/JSON on both surfaces, `TestQuery_SelectTitle_NoStringSurgery` / `_Raw_IdAndTitle`, plus `TestTodoList_MultiLineBody_RoundTripByteIdentical` as a byte-identity guard. `dumpAll` (index_test.go:180) was extended so `TestRebuildDeterministic` snapshots the new column for free. Matches the plan's test matrix.

2. No gap worth blocking on. Observation only: there is no test asserting a work-ticket agenda row's `title` value (only todos are exercised), so the mislabeled-comment behavior in Maintainability #1 is untested — but it is correct behavior, so this is not a coverage defect.

## Maintainability

**Minor (actionable — the one requested change)**

1. `agendaItem.Title` is commented `// durable only: derived first non-empty body line` (today.go:109), but `buildAgenda` sets `Title: c.title` unconditionally (today.go:307), so **external `work-ticket` rows also carry a title** in `--json` and now render title-only in `Pretty()`. The agenda split is native/external, not durable/ephemeral — there is no "ephemeral agenda row" — so "durable only" is simply wrong for this struct. The same wrong text is mirrored into the pinned-contract header (today_test.go:58). Given this pipeline already corrected a comment in be66282, fix both: drop "durable only" (e.g. `// derived first non-empty body line`). Behavior is fine; only the comment is inaccurate.

**Minor (observation, no action)**

2. Comment hygiene on production source is clean: the new comments (title.go, schema.go v3 note, the two struct fields, reconcile.go) carry no ticket/plan/phase refs, and the `plan.md D*` / `reckon-vj55` hits in todo.go are all pre-existing lines this diff never touched — the be66282 fix holds. The heavy provenance (`reckon-fnqs.3`, `Phase 4`, `plan.md D3/D4`, AC numbers) lives only in new *test* comments, which matches pervasive existing convention (query_test.go alone has 45 such refs). Consistent with house style; no cleanup warranted here.

3. `todoListItem.Pretty()` durable branch renders `it.Title`, ephemeral branch keeps `it.Body` (todo.go:194,196); `agendaResult.Pretty()` renders `it.Title` uniformly. The work-ticket consequence — a multi-line external body now shows first-line-only in pretty — is the intended, correct result of D1's uniform column (single-line stubs unchanged; empty stubs blank either way), not a regression.

## Error Handling

No findings. The new code adds no error path: `deriveTitle` is total (never errors), and the INSERT's `Exec` error is already wrapped (`reconcile.go:275`). Row iteration in the extended `buildAgenda`/`listDurableTodos` scans keeps the existing explicit `rows.Close()` on every error and success path (defer is deliberately avoided due to nested per-row prop queries on the same `*sql.DB`).

## Performance

**Minor (optional, non-blocking)**

1. `deriveTitle` uses `strings.Split(body, "\n")`, which allocates a slice of every line even though only the first non-empty one is needed. A `strings.IndexByte`/`strings.Cut` loop would avoid materializing the whole split. Negligible in practice — bodies are small and this runs at index/reconcile time, never on the query hot path — so not worth changing unless touched for another reason.

## Security

No findings. All SQL is static with `?` placeholders or literal type filters (no interpolation of user data into query text); `deriveTitle` is pure string handling with no injection, path, or resource surface.

---

## Answers to the specific scrutiny points

- **`omitempty` on `Title` (D5):** the JSON `title` key is dropped exactly when a **durable** row has an all-whitespace body. The add-guard (todo.go:273) prevents this for CLI-created todos, but a hand-authored or synced file can reach it. Note the asymmetry: `todoListItem.Body` has **no** `omitempty` (its `body` key is always present), while `agendaItem.Body` **does** — so a degenerate durable todo emits `body:""` with no `title` key, whereas an empty work-ticket drops both together. This is the D5-accepted tradeoff; a consumer only sees a shape change in the degenerate empty-body case. Acceptable as documented.
- **Missing/extra threading:** the diff matches the plan's Files-to-modify table exactly — nothing on the table is absent, and nothing off the table was changed. `listEphemeralTodos`, `query.go`, and `internal/node/*` are correctly left untouched.

## Positive observations

- Derivation is genuinely correct on the tricky inputs (blank-lead, CRLF, whitespace-only), and the integration test proves wiring rather than trusting the unit test.
- Schema-version rebuild is real, not assumed — verified end-to-end through `ensureSchema`→`Rebuild`→`insertNode`.
- Disciplined scoping: node layer untouched, `rk query` deliberately unextended, no speculative generalization.
- Documentation is placed with a single authoritative home per fact (convention in composable-redesign.md, column in index/AGENTS.md, a pointer in node/AGENTS.md).
