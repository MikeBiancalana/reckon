# Rebuttal & Resolution — response to `composable-redesign-assessment.md`

> Status: **owner-reviewed resolution** of the 5-lens assessment.
> Date: 2026-06-22 · Parties: Mike (owner) + primary composable-design session.
> Purpose: record which assessment findings are accepted, downgraded, or held —
> and the resulting amendments. **Godfrey: this is the input for updating
> `composable-redesign.md`;** the design doc has already been amended to match
> (see "Doc changes applied" at the end). Treat any conflict as: this rebuttal +
> the amended design doc win over the original assessment framing.

The assessment is strong, code-grounded, and largely correct. The owner left the
2026-06-15 work-coupled design **specifically because it was too work-specific**;
the same author wrote this assessment, so a few findings re-import work context
as if it were universal. The resolution sorts findings into three buckets:
**(A) accepted as universal**, **(B) work-specific — extension-resident, core
stays put**, **(C) owner's call — now settled**.

---

## A. Accepted as universal (amendments made)

### A1. The `parse`/`serialize` round-trip is the keystone — reclassified to gating
**Accepted in full.** Under files-as-truth, a parser bug does not merely misread —
`serialize` **silently rewrites the authoritative file**. That is the one failure
the whole design exists to prevent. It is wrong to file this under "Group B /
downhill."

- **Reclassified** to a **gating Group-A spike** (must pass before any tool writes
  a truth file).
- **Mechanism: span-local write-back** — byte-preserve unmodeled prose; a tool
  rewrites **only the spans it owns** (e.g. a single `scheduled:` field), never
  regenerates the whole file. Current `journal/service.go` does the opposite
  (`WriteJournal` regenerates the full file) and must change.
- **Gate: a `parse(serialize(parse(f))) == f` fuzz test**, seeded with **real**
  Obsidian/Logseq/agent input (blockquotes, code fences, `[[t|label]]`, `#^block`,
  `![[ ]]`, `#tag`, extra frontmatter, git conflict markers, `.sync-conflict`
  copies). Must pass on real input before truth writes.
- **Connection the assessment under-stated:** span-local write-back is *also* what
  makes hand-editing safe — the tool and the human can both edit the same file
  without clobbering. This is precisely what gives org-mode its beloved
  hand-tweakable feel. So #1 is not just a safety gate; it is the feature that
  reproduces org's dwimmy-ness outside Emacs.

### A2. The "~60% there / refactor not rewrite" estimate was wrong — corrected
**Accepted; independently verified by the owner's session.** Code-traced:
- `internal/service/notes_repository.go:34` and `internal/checklist/repository.go:32`
  `INSERT … ON CONFLICT DO UPDATE` into SQLite **as authority**.
- `internal/cli/rebuild.go` rebuilds **only** the journal from files
  (`journalService.Rebuild()`).
- `internal/journal/service.go:551 save()` → `WriteJournal(j)` → full-file
  `os.WriteFile` (`internal/storage/filesystem.go:64`).

So for tasks/notes/checklists, **markdown is an export artifact, not truth — the
inverse of the redesign's central pillar.** The original "~60%" came from surface
signals (package layout, `xid`, `note_links`, a SQLite index) without tracing
*truth direction*. **Corrected estimate: honest reuse ≈ 20–35%; the
differentiated core (unified node+edge graph, resolver, promotion/NDJSON,
query-views, recurrence) ≈ 0% present.** The **truth-inversion is the real long
pole** and must be scheduled as such, not waved at parenthetically.

### A3. Concurrency — accepted as real, **downgraded** from "lit fuse"
The mechanism is real (git merges sequential log appends within its 3-line context
window; an agent has no defined conflict path). **But the owner corrects the
magnitude:** the "~15 concurrent writers" was a count of *sessions alive*, not
*sessions writing the log* — in practice 1–2 actually append to the log. So this
is **not gating**.
- **Fix (kept as cheap insurance, not a blocker):** a `merge=union` driver in
  `.gitattributes` for `log/*.md` + **sort-on-reconcile** + **ULID-dedup** =
  lossless concurrent appends, clean text, no markers. Adopted.
- Only **index** concurrency was ever addressed (A#4: WAL + single-writer lock);
  that guards the disposable index. The log fix above covers the truth side
  cheaply. No single-writer commit gate needed at current scale.

### A4. `author` / provenance field — accepted, added now
A single cheap field, harmless for solo use, and required the moment work (or any
multi-writer/agent scenario) rides the same core. **Added to the canonical node
now** as a forward, authored fact. Labeled as provenance-for-multi-writer, not a
general necessity — but the affordance pays off later, so it goes in early.

---

## B. Work-specific — extension-resident; the core must NOT be recaptured

These findings are **true for the work environment** but must not silently redefine
the general tool. The owner escaped exactly this coupling once already.

### B1. Read-time synthesis is NOT core — reserve the seam only
The assessment's evidence ("you read a synthesized brief, not the raw log") is
about a **multi-agent firehose**, not personal use. The flow the owner *loved* in
Logseq (daily-log → branch-to-note) was never synthesis-dependent.
- **Resolution:** **reserve an `rk-brief` view-tool seam** over the index (cheap
  insurance; skipping it now stays reversible) — but synthesis is **not** core.
- For work, `rk-brief` is where the firehose gets tamed: an **extension**, not the
  substrate. The flagship "daily review" view stays a concatenation/query for the
  general tool; an `rk-brief` extension can synthesize when a real firehose exists.
- **This is the single biggest recapture risk** and is explicitly held.

### B2. Work task model (anchored auto-reconcile against Jira/GitHub) — extension
"Driving a ticket *done from the agenda* is the reconcile engine the owner says he
doesn't want." Correct.
- **`rk today` actuates only native nodes.** Externally-fed work tickets are
  **read-only with an "open in Jira/GH" jump**, fed by an `rk-jira` / `rk-gh`
  extension that writes read-only view-nodes into the store.
- The remote-staleness reconcile, the adapter interface, and provenance-of-15-
  personas all live in those extensions. **Core freshness/reconcile stays
  local-file-only.** This is the `rk-<name>` seam doing exactly its job.

### B3. Small core amendments the work case needs (noted, not built)
- **Remote staleness** (Jira changed, not file mtime) — an extension concern; the
  core reconcile model is deliberately local-file-only.
- **Jira keys as aliases** (`SNP-35714`) re-dangling on rebuild — handled by the
  extension owning its own alias minting/resolution; does not change the flat
  global alias namespace for the general tool.
- **Provenance** — covered by A4 (now in core).

---

## C. Owner's call — settled

### C1. Journal write-path losslessness — `merge=union` + sort + ULID-dedup
Chosen (keeps file-per-day, clean text, no markers). Single-writer commit gate
rejected as unnecessary at real (1–2 writer) scale. See A3.

### C2. `rk today` actuator — **split**
Native nodes actuated in-list (`x`/`i`/`d`); external work tickets read-only +
jump. Confirmed acceptable — done-from-agenda never reaches a real work ticket.
See B2.

### C3. Synthesis — **reserved seam, not dead, not core.** See B1.

### C4. Recurrence — keep the feature; **store the cursor, don't derive it**
The owner wants virtual-occurrence recurrence. The debate was only *where the
cycle's cursor lives*. The original proposal derived it from the **log's `did`
entries** (log as load-bearing state). **Changed.**

- **Decision:** store the cursor as the **rule node's own `scheduled:` prop**
  (Option 2), advanced on completion exactly like org-mode:
  `+Nd` catch-up · `++Nd` skip-to-future · `.+Nd` from-completion-date.
  The `did` log entry is still written, but as **audit, not source of truth**.
- **Why (the org devil's-advocate cuts this way):** org-mode is beloved precisely
  because the recurrence cursor lives in a clean, structured, hand-editable slot
  (`SCHEDULED:`), and org **never** recomputes the next date by scanning its
  `:LOGBOOK:`. "Be like org" therefore means *stored cursor*, not *log-derived
  cursor*. The original proposal was the one move org never makes.
- **The real principle is locus of state**, not "plain text is fragile":
  state in a labeled structured slot is hand-fixable and deterministic; state
  smeared across prose and derived is neither (which log line is the cursor?
  editing prose silently moves it).
- **Robustness fallout:** the cursor lives in *text* (frontmatter on the rule) and
  survives index-blow-away trivially — *more* consistent with "text is truth,
  index disposable," not less. Log → cursor remains possible as a recovery
  rebuild, but is never on the hot path.
- **Not duplicated truth:** log `did` = "a completion happened at T" (immutable
  event); rule `scheduled` = "series' current position" (current state) — the
  event-vs-state split (ledger vs balance).
- **Pile-up still works:** the cursor tracks the leading edge; overdue/stacked
  cycles materialize into instance nodes (the existing materialize-on-need path),
  each with its own state.
- **Net:** identical virtual-occurrence UX, org's exact algorithm, ~no new schema
  (reuses `scheduled` + `repeat`), and the log-integrity fragility is gone.

### C5. Lean v1 vs full mechanics
Endorsed in spirit. Build the minimum, grow on use. The one correction to the
assessment: **recurrence is IN v1** (the owner wants it) — but via the C4 stored
cursor, not the fragile log-derived cursor. Other deferrals (the `--ref/--pop/--cp`
disposition zoo → start with a single `rk promote`; hooks beyond `on-reminder-due`;
checklist tool; multi-level fragments; work feeders) accepted.

---

## Lean v1 scope (resolved)

**IN:**
- Substrate: plain-text markdown + git; Syncthing for the mobile leg.
- **Keystone (gating):** `parse`/`serialize` with **span-local write-back** + the
  round-trip fuzz gate on real input (A1). Nothing writes truth until this passes.
- Index: SQLite property-graph (nodes/edges/fts/aliases), lazy-reconcile-on-read +
  explicit `rk index`. Disposable, never synced.
- Identity: ULID inline + aliases. Block `#^frag` only when a real link target
  appears.
- **`author`/provenance on the canonical node** (A4).
- Tools: **log + todo + note**, with the ephemeral/durable distinction as a light
  todo property/group-file (not heavy machinery).
- Read glue: **`rk query`** (SQL over stable views, read-only).
- **`rk today`** agenda, **split actuator** (native actuated, external read-only).
- **Recurrence** via stored `scheduled` cursor (C4).
- Concurrency: `merge=union` + sort + ULID-dedup on `log/*.md` (A3).

**DEFERRED until a lived consumer appears:**
- Read-time synthesis — **reserve the `rk-brief` seam**, build when the
  flat-log-read test fails for the firehose case (work).
- Work feeders `rk-jira` / `rk-gh` (anchored reconcile, remote staleness,
  multi-persona provenance) — extensions, after the core proves out.
- The `--ref/--pop/--cp` disposition zoo → ship a single transactional
  `rk promote` first.
- Hooks beyond `on-reminder-due`; checklist tool; multi-level fragments.

---

## Doc changes applied to `composable-redesign.md`
1. Start-here pillar table: "Build call" corrected (~60% → ~20–35%; truth-inversion
   = long pole; refactor *and* significant new build).
2. Keystone reclassified: parse/serialize is **gating Group-A**, with span-local
   write-back + round-trip fuzz gate. "Downhill from the keystone" language
   removed where it implied the round-trip is trivial.
3. Canonical node: **`author` field added** (forward/authored provenance).
4. Recurrence (A#3): advance model changed from **log-derived done-cursor** to
   **stored `scheduled` cursor (org-style)**, log demoted to audit.
5. Concurrency trip-wire recorded (`merge=union`+sort+dedup), framed as cheap
   insurance, not gating.
6. Synthesis recorded as a **reserved `rk-brief` extension seam**, explicitly not
   core; `rk today` noted as a **split actuator**.
7. "Relationship to current reckon" truth-inversion row de-parenthesized as the
   long pole.
8. A **Lean v1 scope** section added.
9. Decision-log entries appended (2026-06-22).
