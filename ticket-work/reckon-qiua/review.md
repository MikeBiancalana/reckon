# Code Review: `rk todo add/list/done` (reckon-qiua, v1-T5)

**Verdict: APPROVE WITH CHANGES**

Reviewer: code-reviewer (Opus 4.8)
Scope: `internal/cli/todo.go` (new, 776 LOC), `internal/cli/stubs.go` (stub removed),
`internal/cli/todo_test.go` (new, 1324 LOC). Cross-checked against `internal/cli/adopt.go`,
`internal/node/{node,render}.go`, `internal/index/{index,reconcile,schema}.go`,
`internal/cli/query.go`, plan.md (D1–D10), acceptance-criteria.md (AC-1..7 / EC-1..9),
and preflight-report.md.

Build: clean. `go vet ./internal/cli`: clean. Targeted test run
(`-run 'Todo|ResolveAuthor'`): PASS. This is a high-quality, plan-faithful
implementation. The changes below are minor hardening items, none blocking.

---

## Summary

The implementation is a close, disciplined clone of the `adopt.go` precedent and
follows the approved plan almost verbatim. All seven acceptance criteria are
exercised by tests, the byte-preservation and round-trip contracts are honored,
and every design-critical seam the prompt called out checks out correctly. The
few findings are small robustness/spec-fidelity nits, all in the "would improve
it" bucket rather than "must fix to be correct."

## Verification of the prompt's specific scrutiny points

All six load-bearing concerns verify **positively**:

1. **`mintTodoULID` seam does not weaken production randomness.** In production,
   `mintTodoULID = node.Mint` (todo.go:25). `addDurableTodo` mints via the seam
   for the path/no-clobber check, and `node.NewNode` *also* mints a real ULID
   internally (render.go:99) which is then overwritten by `n.ULID = id`
   (todo.go:292). The seam is only reassigned inside the test
   (`TestTodoAdd_NoClobberExistingFile`), restored via `t.Cleanup`. Production
   entropy is untouched. (Note: a harmless double-mint per create — one discarded.)

2. **Ephemeral checkbox parse/splice is correct and internally consistent.**
   The critical property is that `list`'s indexing and `done`'s targeting agree
   on *which* lines are checkbox items. `splitChecklistLines`
   (`^[-*] \[([ xX])\] ?(.*)$`, todo.go:726) and `flipChecklistLine`'s
   `checklistMarkRe` (`(?m)^[-*] \[([ xX])\]`, todo.go:730) accept the exact same
   set of lines (verified across indented, extra-space, and empty-text cases), so
   the 1-based file-order index `list` prints is the same index `done` consumes —
   no off-by-one, no "nth-unchecked" confusion. `flipChecklistLine` copies the
   whole buffer and flips a single byte (todo.go:772-774), so sibling lines are
   byte-identical (confirmed by `TestTodoDone_EphemeralFlipsCheckbox`). The
   deliberate no-trailing-newline invariant (todo.go:327-333) keeps the append
   from disturbing prior line spans.

3. **`depends-on` edge mechanism is exactly as specified (D6).** `--depends`
   writes `n.Links = [{Rel:"depends-on", To:target}}` (todo.go:303); `render.go`
   emits `depends-on: "[[target]]"`; `deriveView` routes a ref-valued key to
   `Links` with `Rel` = the literal key (node.go:238-240), so the generic parser
   produces the `depends-on` edge with zero custom code, and it lands in `edges`,
   never `node_props`. Dangling targets are written verbatim with no write-time
   validation, resolving on reindex via `resolveEdges`. Tests T-5.1..5.4 cover it.

4. **Round-trip stability (AC-4) is genuine span-local editing.** `done` calls
   `n.SetField("state","done")` (todo.go:610), which splices only the value span
   and re-parses (node.go:143-159). `TestTodoDone_DurableBytePreservation` asserts
   the result equals `strings.Replace(src,"state: open","state: done",1)` — i.e.
   every other byte, including an extra `x-obsidian-extra:` key and a code fence,
   is preserved. Not a rewrite.

5. **`list` freshness / read-write divergence is real and contained.** `list`
   does `index.Open(cfg)` (auto-rebuilds a missing/stale DB via
   `ensureSchema`→`Rebuild`) then an explicit `ix.Reconcile()` (todo.go:401-409)
   before querying the public views. This is a conscious divergence from
   `rk query`, which opens read-only and errors "run `rk index` first" when the
   DB is absent (query.go:125-129). The divergence is confined to `list` and
   matches the index package's documented "lazy reconcile-on-read" intent. The
   missing-vault case is tolerated by `reconcileTx`'s `os.IsNotExist` guard
   (reconcile.go:200), so `TestTodoList_MissingVaultDir` returns an empty result.

6. **`done` never touches the index.** The `done` path
   (`runTodoDoneE`→`doneDurableTodo`/`doneEphemeralTodo`) only does
   `config.LoadWithOverrides` (pure), filesystem reads, `node.Parse`, `SetField`,
   `filepath.Glob`, and `writeFileAtomic`. There is no `index.Open` anywhere on
   that path, so the cache dir is never constructed.
   `TestTodoDone_NeverTouchesIndex` backs this.

---

## Critical Issues

None.

---

## Recommendations (prioritized)

### R1 — `--ephemeral --depends <X>` silently ignores the dependency (spec deviation)
**Severity: low (silent intent loss).** `runTodoAddE` rejects `--ephemeral`
combined with `--scheduled`/`--deadline` (todo.go:241-243) but **omits
`--depends`**:

```go
if ephemeral && (scheduled != "" || deadline != "") { ... }   // depends not checked
```

Plan D2 explicitly lists `depends` among the durable-only flags that should make
`add --ephemeral` a usage error, and AC-doc §2.1 marks `depends` "not applicable"
for ephemeral. As written, `rk todo add "x" --ephemeral --depends foo` succeeds
and the dependency is dropped with no warning. `addEphemeralTodo` never receives
`depends`, so it cannot even round-trip it. Fix: add `|| depends != ""` to the
guard and widen the message. (`TestTodoAdd_EphemeralRejectsDateFlags` only covers
scheduled/deadline, so the gap is untested.)

### R2 — Durable `done` fast-path doesn't guard node type
**Severity: low (confusing error, not corruption).** `doneDurableTodo` resolves
`todos/<ref>.md` via `loadDurableTodoAt` (todo.go:586-590), which parses and
returns *any* node without checking `n.Type`. The fallback walk
`findDurableTodoByRefOrAlias` *does* filter `n.Type != "todo"` (todo.go:657). So
`rk todo done inbox` (ref `"inbox"` → `todos/inbox.md`, the ephemeral container)
takes the fast path, finds a `todo-ephemeral` node with no `state` field, and
fails at `SetField("state",...)` with `no scalar span for "state"` instead of a
clean "not found." Recommend guarding the fast path with `n.Type == "todo"`
(treat a non-todo match as a miss and fall through / report not-found), mirroring
the walk path.

### R3 — `done` is brittle on a durable todo lacking a `state:` field
**Severity: low-medium.** D5 commits to `SetField` (not `InsertField`) on the
premise that `state` always exists from create. That holds for tool-created
todos, but a hand-authored durable todo without a `state:` line — plausible given
the tool's stated Obsidian-authored-file use case (EC-8) — makes `done` fail with
the internal-sounding `SetField: no scalar span for "state"`. Consider either
(a) falling back to `InsertField("state","done")` when the field is absent
(exactly the pattern `adopt.go:222-235` uses for a blank `id`), or (b) at minimum
wrapping it in a user-facing message ("todo has no `state` field to complete").
This is a documented design choice, so it's a judgment call, not a defect.

### R4 — `list` reconstructs the durable path instead of using the indexed `loc`
**Severity: low (maintainability/robustness).** `listDurableTodos` builds
`Path: "todos/" + r.id + ".md"` (todo.go:473) from the node key rather than
reading the `loc` column already available in the `nodes` view
(schema.go:82, `loc_file AS loc`). For any durable todo whose file isn't named
exactly `<ULID>.md` or lives in a nested subdir, the reported path is wrong. The
query already selects from `nodes`; adding `loc` to the `SELECT` and using it is
strictly more correct and no more code.

### R5 — No-clobber check is Stat-based, not atomic
**Severity: informational.** `addDurableTodo` does `os.Stat` then
`writeFileAtomic` (todo.go:285-312). `writeFileAtomic`'s `os.Rename` replaces an
existing destination, so there is a TOCTOU window between the stat and the rename
— the guard is not strictly "race-free." Given 80 bits of ULID randomness and a
single-user local tool, the practical collision/race probability is nil, and the
realistic case (a pre-planted stray file) is caught. If you want the guarantee to
be atomic rather than probabilistic, an `os.OpenFile(path, O_CREATE|O_EXCL)`
sentinel before the atomic write would close it. Acceptable as-is; noting for
completeness because the prompt asked whether it is race-free (it is not, strictly).

### R6 — Minor style (both pre-flagged, both safe)
- `listDurableTodos` closes `rows` manually across paths (todo.go:441-450) while
  its sibling `loadTodoProps` uses `defer rows.Close()` (todo.go:489). Both are
  correct; the manual form is deliberate (it queries `loadTodoProps` per row
  *after* the outer rows are drained, so the outer cursor must close first — a
  `defer` would hold it open during the inner queries). Worth a one-line comment
  saying so, since it reads as an inconsistency otherwise.
- `output.ModeFromFlags` errors are returned unwrapped (todo.go:246, 392, 552),
  matching the established `adopt.go:92-94` convention. Consistent; leave as-is or
  wrap uniformly across the package in a separate pass.
- `listDurableTodos` is an N+1 query shape (a props query + a depends query per
  durable row). Fine at todo-list scale; a single `LEFT JOIN` would remove it if
  this ever grows hot.

---

## Positive Observations

- **Faithful precedent reuse.** `writeFileAtomic` is reused verbatim, the
  `Annotations{"requiresDB":"false"}`, `!(mode==Pretty && quietFlag)` output
  gating, `resetTodoFlags`/pflag-`Changed` hygiene, and CRLF-refusal guard all
  mirror `adopt.go`/`query.go` exactly — zero drift, zero duplication of the
  atomic-write logic.
- **The create recipe is correct.** `NewNode → set ULID/Time/Props/Links →
  Render → Parse → writeFileAtomic(parsed.Serialize())` (todo.go:291-312) follows
  the render.go doc contract precisely, which is why first-write round-trip
  (T-1.6, T-4.1) holds.
- **The ephemeral no-trailing-newline invariant** (todo.go:327-333) is a subtle
  but correct choice, well-documented, and the reason the append can't disturb a
  prior line's byte span.
- **Idempotency is modeled distinctly from failure** for both durable
  (`Skipped:true`, no rewrite — todo.go:604-608) and ephemeral
  (`alreadyChecked` — todo.go:691-693), and "not found" is a separate non-zero
  exit, satisfying EC-1 vs the error case cleanly.
- **Tests match their names and cover the ACs plus edge cases**, including the
  hard ones: byte-preservation with unmodeled frontmatter, dangling-edge
  resolution, freshness-without-manual-index, and the never-touch-index invariant.
  Test-first structure with a pinned result-struct contract in the header comment
  is exemplary.

---

## Questions for Consideration

1. Is `done` intended to complete hand-authored durable todos that were never
   created through `add` (and thus may lack `state:`)? The answer decides whether
   R3 is a real gap or genuinely out of scope. Given EC-8's emphasis on
   Obsidian-authored files, I'd lean toward the `InsertField` fallback.
2. Should `--ephemeral` combined with any durable-only flag (`--depends`,
   `--author` is fine) be a hard usage error uniformly? R1 fixes `--depends`;
   confirm that's the desired contract vs. silent-ignore.

---

## AC / Test Coverage Matrix

| AC | Covered by | Status |
|----|-----------|--------|
| AC-1 create (durable/ephemeral, dirs, no-clobber) | T-1.1..1.8 | ✅ |
| AC-2 list (freshness, kinds, done-hiding, empty, missing vault) | T-2.1..2.6 | ✅ |
| AC-3 done (span-local, alias, idempotent, not-found, ephemeral) | T-3.1..3.6 | ✅ |
| AC-4 round-trip (parse/serialize, render, edit-path) | T-4.1..4.3 | ✅ |
| AC-5 depends-on edges (indexed, not-props, dangling, resolves) | T-5.1..5.4 | ✅ |
| AC-6 queryable distinction (index + porcelain) | T-6.1, T-2.2 | ✅ |
| AC-7 tested (harness, quiet, never-touch-index) | T-7.1..7.3 | ✅ |

Untested spec point: `--ephemeral --depends` rejection (R1) — currently the code
doesn't reject it, so there is nothing to test until the guard is added.
