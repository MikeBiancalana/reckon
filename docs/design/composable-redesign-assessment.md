# Composable Redesign — Critical Assessment & Readiness Gaps

> Status: **assessment of `composable-redesign.md`.** Companion / critique, not a replacement.
> Date: 2026-06-22 · Author: Godfrey (ancillary session) · Method: solo comparative read + a 5-lens adversarial agent panel (code-grounded).

This assesses `docs/design/composable-redesign.md` against (a) the earlier work-coupled design (`reckon-redesign_2026-06-15.md` / `reckon-spec_2026-06-15.md`) and (b) the owner's actual work context, and records the readiness gaps a 5-lens adversarial panel surfaced. The owner's latest reframes are treated as the live design positions: concurrency deferred; sync moving Syncthing→git; Jira/GitHub as external view-feeding extensions (no internal reconcile engine); wall-of-text framed as an input problem.

## Verdict

**The composable design is sound as the core direction** — independently convergent (~80%) with the portable core of the 6-15 design (links-not-schema, property-graph index in SQLite, two-stage resolve, time-sortable inline ID, type-as-property, ephemeral/durable split, agent-first query), and field-validated against the owner's PKM history (org-roam without Emacs). Under the owner's reframes, files-as-truth + disposable-index + work-as-external-read-only-view is a coherent expression of intent, and it **makes the earlier global DB-first answer unnecessary.** The data model is twice-converged and should stop being re-litigated.

**But the doc's "architecture complete" label is wrong on three counts, each of which defaults to *silent data loss or corruption*** — the one failure a plain-text-truth system exists to prevent. These are gating; everything else is eyes-open judgment.

## The three non-negotiables (must settle before any tool writes a truth file)

### 1. The `parse`/`serialize` round-trip is THE keystone — and current code fails it today
Two lenses independently reduced the entire architecture to the per-tool `parse(file)->[]node` / `serialize(node)->file` pair (the doc itself: "emit/import/index/resolve all reduce to that pair"). Under files-as-truth, a parser bug is not a bad read — `serialize` **silently rewrites the authoritative file**. Current `internal/journal/parser.go` is brittle line-regex that drops unmodeled content; `writer.go` regenerates sections in fixed order. The property test `parse(serialize(parse(f))) == f`, seeded with real Obsidian/Logseq/agent files (blockquotes, code fences, `[[t|label]]`, `#^block`, `![[ ]]`, `#tag`, extra frontmatter, git conflict markers, `.sync-conflict` copies), **fails immediately on today's code.**
- **Fix:** reclassify out of "Group B / downhill" into a gating Group-A spike. Adopt **span-local write-back** — byte-preserve unmodeled prose, touch only the spans a tool owns. Pass the round-trip fuzz on real input *before* any tool writes a truth file.

### 2. Write-path concurrency can't be deferred — only *index* concurrency can
The doc's only concurrency provision (A#4: WAL + single reconcile-writer lock) guards the **disposable index** — corruption there costs nothing (rebuildable). The exposure is the **source of truth**: `internal/journal/service.go:551 save()` → `WriteJournal(j)` (full serialize) → `internal/storage/filesystem.go:64 os.WriteFile` rewrites the **entire** day file. Two agents that each load→add→serialize **clobber each other — silent last-writer-wins loss, on local disk, before git is involved.** The fuse is lit and masked only by current low write-overlap through reckon specifically.
- **Reframe tension:** #3 (defer concurrency) and #4 (move to git) fight each other — *moving to git is the act that activates the deferred problem*. Empirically (git's own merge), two appends to the day-log conflict even at EOF, because sequential log entries always land within git's 3-line context window. A non-interactive agent has no defined conflict path; `--ours/--theirs` silently discards the other entry.
- **Fix (cheap, keeps file-per-day):** a `merge=union` driver in `.gitattributes` for `log/*.md` + **sort-on-reconcile** + **ULID-dedup** = lossless concurrent appends, no markers. Alternative: a single-writer commit gate (agents pipe to one `rk log` server that owns the file + commits). Doing nothing = silent loss is the default.

### 3. "~60% there / refactor not rewrite" is off — tasks & notes are DB-PRIMARY today
Code-traced: `internal/service/notes_repository.go:34` and `internal/checklist/repository.go:32` do `INSERT … ON CONFLICT DO UPDATE` into SQLite **as authority**; `internal/cli/rebuild.go` rebuilds **only** the journal from files (`journalService.Rebuild()`). So for tasks/notes/checklists, **markdown is an export artifact, not truth — the inverse of the redesign's central pillar.** Plus: tasks live in one `tasks.md` group file (design mandates file-per-todo); no edges table (only note→note slug links); xid not ULID.
- Honest reuse ≈ 20–35% (package structure, the SQLite-index *concept*, the journal round-trip, slug logic). The differentiated core (unified node+edge graph, resolver, promotion/NDJSON, query-views, recurrence) is ≈ 0% present.
- **The truth-inversion is the real long pole**, waved at parenthetically (doc line 612). Re-baseline the estimate by feature before committing to a schedule.

## Where the panel corrected the reframes

- **#2 "wall-of-text is mostly an input problem" — empirically undercut by the owner's own behavior.** Input discipline is *already in force* (the journal routes through Lycurgus for brevity), yet the owner still reads a synthesized morning brief, not the raw log. The doc's flagship "daily review" view (lines 199–204) is a **concatenation**, not a synthesis — the exact surface he stopped reading. So read-time synthesis may be **core, not optional.** Honest test: render one real ~15-agent day as the proposed flat log-entry list and ask whether he'd read it instead of his brief. If not, synthesis is core.
- **#1 "work bolts on without touching the core" — direction fine, claim already false.** Three core mechanisms need amendment: (a) the freshness/reconcile model knows only *local* file-change, no notion of *remote* staleness (Jira changes, not file mtime); (b) flat alias-uniqueness *fights* using Jira keys (`SNP-35714`) as aliases and they re-dangle on every full rebuild that precedes a feed; (c) the canonical node has **no `author`/provenance field**, colliding with the owner's verified+attributed-provenance rule and ~15 concurrent agent writers. Each is a small-but-real core amendment.
- **`rk today` becomes a unified VIEW with a SPLIT actuator.** Native tasks are driven in-list (`x`/`i`/`d`); externally-fed work tickets are read-only with an "open in Jira" jump. Driving a ticket *done from the agenda* **is** the reconcile engine the owner says he doesn't want. This is materially smaller than "org-agenda for my whole life" — must be confirmed as acceptable, not assumed.

## Key disagreement (highest-signal panel output)
Lens 5 (Coherence) names the **parser round-trip** as the fatal bet and rules concurrency safely deferrable; Lens 1 (Concurrency) calls the write-path a **lit fuse**. Not contradictory — different layers. Resolution: **do both** — spike the parser *and* write down the concurrency trip-wire so "deferred" never becomes "forgotten." All five lenses agree: right architecture, wrong readiness label.

## The four decisions left to the owner
1. **Journal write-path losslessness** — `merge=union` + sort-on-reconcile + ULID-dedup (cheapest, keeps file-per-day) vs single-writer commit gate vs per-entry log files (doc rejects). *Default = silent loss.* Settle before reckon is the primary capture path.
2. **`rk today`: unified actuator or split actuator?** Read-only-Jira-jump (consistent with the reframes) vs round-trip-to-Jira (= the engine refused). Confirm done-from-agenda doesn't reach real work tickets.
3. **Synthesis: dead or reserved seam?** Cheap insurance = reserve an `rk-brief` view-tool seam over the index so skipping it now is reversible. (Zero synthesis code exists to "just add later.")
4. **Lean v1 vs full mechanics** — ~40% of specced mechanics is optionality with no current consumer (virtual-occurrence recurrence [also fragile — done-cursor couples cycle correctness to free-text log integrity], the `--ref/--pop/--cp` disposition zoo, pre-built hook seams). For each mechanism, name the lived pain it solves or defer it.

## Proposed lean v1 (build what's needed, expand on use)

Per the owner's stated intent — instantiate the minimum, grow as used. **IN:**
- Substrate: plain-text markdown + git. Phone via hybrid (keep Syncthing for the mobile leg; git for desktop history) until a git-mobile flow proves out — the doc's own ignore-globs already tolerate `.sync-conflict-*`.
- **Keystone**: `parse`/`serialize` with span-local write-back + the round-trip fuzz gate (non-negotiable #1).
- Index: SQLite property-graph (nodes/edges/fts/aliases), lazy-reconcile-on-read + explicit `rk index`. Disposable, never synced.
- Identity: ULID inline + human aliases. Block `#^frag` only when a real link target appears (defer otherwise).
- **Provenance**: add `author` to the canonical node now — cheap, and required by the owner's rule + multi-writer reality.
- Tools: **log + todo + note**. Keep the ephemeral/durable distinction (field-validated gap) but as a lightweight todo property/group-file, not heavy machinery.
- Read glue: **`rk query`** (SQL over stable views) — needed for agent retrieval and the agenda.
- **`rk today`** agenda (split actuator) — the org-agenda flow that fixes the Logseq pain.
- Write/move: a single transactional **`rk promote`** verb (defer the `--ref/--pop/--cp` zoo).
- Concurrency: `merge=union` + sort-on-reconcile + ULID-dedup on the log (non-negotiable #2).

**DEFERRED until a lived consumer appears:** virtual-occurrence recurrence (if any recurrence at all, start with a stored next-date — robust, not done-cursor-fragile), reminders, the hooks taxonomy beyond `on-reminder-due`, the checklist tool, multi-level fragments, the disposition zoo, work `rk-jira`/`rk-gh` feeders (build after the core is proven). Read-time synthesis: **reserve the `rk-brief` seam**, build when the flat-log-read test fails (likely, per reframe-#2 correction).

## Convergence note
The composable design and the 6-15 design agree on ~80% of the core. They diverged exactly on the work-coupling (reconcile / provenance / concurrency / synthesis) — the split the 6-15 "Decision point" section predicted, and the seam (`rk-<name>` extensions) the composable doc independently arrived at. The panel improved on the 6-15 synthesis: the cheap lossless-log answer (`merge=union` + sort + dedup) keeps files-as-truth everywhere — no DB-backed log tool needed, so the global DB-first lock is retired in favor of a lighter, file-native fix.

## Round 2 — post-rebuttal convergence (2026-06-23)

Reviewed `composable-redesign-rebuttal.md` + the amendments folded into `composable-redesign.md` (keystone reclassified to gating Group-A; `author` field added; recurrence → stored `scheduled` cursor; concurrency trip-wire recorded; synthesis as a reserved `rk-brief` seam; the new "AI as a first-class participant" pillar).

**Verdict: converged. Direction sound, core settled — no disagreement with the resolution.** The rebuttal accepts the two gating findings in full (keystone→gating; ~60%→20-35% truth-inversion as the long pole) and correctly **quarantines synthesis + Jira-reconcile as work-extensions** rather than letting them recapture the core — the discipline the owner left the 6-15 design to get. Two resolutions are net improvements over the original assessment framing:
- **Recurrence via stored `scheduled` cursor (log = audit)** dissolves the H5 log-integrity fragility *and* is more org-faithful than the log-derived cursor — better than either prior position.
- **The AI-as-first-class pillar** ("the determinism boundary is the tool/LLM boundary"; model-free core; agents-as-porcelains; MCP-as-just-another-porcelain; AI output re-enters as nodes with `author=<model>` + `derived-from`) resolves the synthesis-out-of-core tension cleanly. Read-only `rk query` = safety by construction.

The concurrency downgrade (sessions-alive ≠ sessions-writing-the-log; 1–2 real log writers) is a legitimate factual correction; the `merge=union` fix went in regardless as cheap insurance.

### Open watch-items (edges for the build, not objections)

1. **Span-local write-back is the load-bearing unproven bet.** Correctly gated. But byte-preserving unmodeled prose *while* tool and human edit the same file needs span anchoring that survives edits above/below the span — hard for markdown. Promise nothing downstream until the spike passes, and **pre-name a fallback** (e.g. DB-truth for the single most-structured type) so a *failed* spike reroutes the design instead of stranding it. Don't discover the fallback under pressure.
2. **Lean v1 is a scope list, not a sequence.** Sequence *within* v1: keystone spike → substrate + log capture + index + `rk query` (this alone replaces logseq's journal + retrieval — the highest-use surfaces) → prove the substrate → *then* todo / `rk today` / recurrence. Don't build the agenda before log+query earn trust.
3. **Keep `merge=union` scoped to `log/*.md`.** Correct for the append-only log; **unsafe for frontmatter** on file-per-item todos/notes (union-merging two edits to the same `scheduled:` key garbles it). Residual edge = concurrent frontmatter edits to the *same* node — rare, surfaces as a git conflict not silent loss (acceptable) — but name it, and keep a post-write "did my ULID land?" verify as the trip-wire.
4. **Migration is part of the long pole, not a Group-B footnote.** Moving existing DB-primary tasks/notes/checklists to text-truth is the *same workstream* as span-local write-back. Elevate it beside the keystone, or the 20–35% reuse estimate quietly grows.
