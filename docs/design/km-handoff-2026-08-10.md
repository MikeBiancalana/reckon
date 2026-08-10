# KM/reckon workstream handoff — Luhmann, 2026-08-10

> Session-end state file. Written so a fresh session (or human) can resume with zero
> conversation context. One section per workstream; "context-only" = knowledge that
> lived nowhere else until this file.

## 1. KM-architecture proposal — COMPLETE, durable

`docs/design/km-architecture-proposal.md`, amended through §4.11. All decisions and
rationale are in the doc itself (timestamp semantics §4.2, stages+schema file §4.2.1,
`rk prime` §4.8, circles-as-repos + alias namespacing §4.9, board-as-derived-view §4.10,
external feeds + "map, never the hand" + no-ticket-grammar rulings §4.11, T8 filename
policy). Committed/pushed to reckon main. Nothing context-only. Doc map:
`docs/design/INDEX.md`.

## 2. Reckon readiness watch — all visible on GitHub/beads

- v1 T0–T9 shipped; acceptance suite in-tree (`tests/acceptance/`, tag `acceptance`),
  PR #156 (rk today scenarios) MERGED 2026-07-14. Suite has since been extended by
  others (execEnv helper etc.) — it is community property now.
- All findings filed as beads: reckon-nmcl (rk import verb collision — settle before
  any jira feed adapter), reckon-frsh, reckon-lnse, reckon-nidx, reckon-nshw, plus
  pre-existing reckon-89hp/29ln (verb cleanup — standing "biggest daily-driver hazard"
  call), reckon-cxx1 (T10 MCP), reckon-pvyl (T11 brief seam).
- Nothing mid-flight; nothing context-only.

## 3. ADR bundle (integrations_team_simple_guide_to_winning) — PR #7 OPEN

- Branch `excavation-rounds-2-3` @ 9039d42 (lint-stamped head), PR #7 open on the org
  remote, awaiting Mike's post-soak merge. No review responses pending as of this
  writing; anything new is visible on GitHub.
- Lint procedure (was context-only; the checks, so anyone can re-implement): every
  non-reserved .md has parseable frontmatter + non-empty `type`; ADRs additionally:
  valid unique 26-char Crockford ULID `id`, `author` present, `status` ∈
  accepted|superseded|abandoned; supersedes/superseded_by reciprocity (superseded ⟺
  pointer, both directions resolve); `amends:` targets resolve; all bundle-root links
  resolve; index.md ⟺ adr/ complete both directions; index.md frontmatter =
  okf_version only; log.md has no frontmatter. Last result: CLEAN — 57 ADRs, 57 ULIDs,
  211 links, 4 reciprocal pairs.
- Post-merge follow-up queue (order deliberate): (a) template comment documenting the
  `amends:` convention (amends key + body note, never a supersedes edge — parked by
  Gilbert to keep the head at the lint stamp; on his Board); (b) phase1b.md's 17
  accepted Slack rows — undrafted ADR batch; (c) unmarked CJ-19/20/21/23 need Mike's
  triage; (d) second-pull targets (AAR cluster — richest remaining vein) unmined.
- ADR 0051 (lead-to-lease) was drafted from an E-marked row interpreted as
  accept-with-edit, incorporating MB's abandonment note; Mike never objected, but it
  was flagged for his review — delete if E meant exclude.
- From 0058 on: one ADR, one PR (batch era ended with the backfill).

## 4. Board-on-reckon assessment (2026-07-20) — was context-only; captured here

Delivered to Gilbert via peers when the PA Board collapsed (glance+archive on one
page). Verdict: **invest — right fit.** Diagnosis: one markdown page serving as both
database and view was the failure, not markdown. Mapping: board item = durable todo
node; status/next/owner as props (`owner` ≠ `author`: acts vs wrote); refs as
wikilinks; history = `did::`/log stream + git, structurally off the surface; freshness
= derived per-file mtime (NO stored timestamp — consistent with §4.2 ruling), so
"changed since Friday" = `WHERE mtime > date`. Minimal extension bill (not a pillar):
(a) prop convention page — board state vocabulary (attention|watched|parked|blocked),
`owner:`, `next:` — zero code; (b) saved views: needs-mike.sql, watched.sql,
changed-since.sql — zero code; (c) `rk-board` render porcelain (PATH-extension seam,
per §4.10) — small script, output = glance table + optional marker-owned generated
page; (d) the one possible code gap: generic `rk todo set <ref> <key> <value>`
prop-edit verb (verify whether it exists; if not, thin wrapper over span-local
SetField). Migration: hand-migrate the live 15–30 items (scripted afternoon); the
Logseq glance page becomes a marker-owned render target; Logseq journal stays the
durable record; systems touch only at refs; independently adoptable — no journal
cutover required. This is the first lived consumer for §4.10 and pulls the watch/hold
convention (+eventually `rk remind due`) into existence per the build-when-lived rule.

## 5. Standing watch criteria (were context-only)

- Merge-readiness bar used for reckon PRs: full suite + vet on the branch head, plus
  acceptance suite against the built binary, plus review-findings-addressed check.
- OKF is v0.1 (watch adoption; reckon bets only the export on it — proposal §4.7).
- Godfrey's open synthesis test (§4.7): once harness-hook capture makes the log a
  firehose, re-run "would Mike read a flat day as a list?" — if no, T11 stops being
  deferred.

## Safe to end?

Yes. Everything above is now durable. No unpushed commits, no mid-flight edits, no
promises outstanding. Resume points: PR #7 merge → follow-up queue (§3); board
investment decision → §4; reckon findings → beads.
