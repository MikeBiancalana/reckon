# Reckon design docs — the map

> One line per doc, status-first. Agents: read **composable-redesign.md** for current direction;
> everything else is context. Updated 2026-07-09 (Luhmann).

## LIVE — current direction

- **[composable-redesign.md](composable-redesign.md)** — THE design. UNIX-composable tools over
  plain-text + git; canonical node; disposable SQLite property-graph index. Has a "Status &
  resuming (start here)" section, full decision log, and the 2026-06-22 amendments folded in.
  Build state: v1 T0–T6 shipped; T7/T8/T9 open (`bd ready`).
- **[km-architecture-proposal.md](km-architecture-proposal.md)** — 2026-07-09 research + proposal
  (Luhmann): OKF/Karpathy-brain analysis; confirms the composable direction (Option A); adds T8
  note-format conventions (OKF-conformant), maturity stages, schema file, `rk export --okf`,
  `rk lint`, harness-hook capture, `rk prime`.

## LIVE — companions to the design (read with it)

- **[composable-redesign-assessment.md](composable-redesign-assessment.md)** — Godfrey's 5-lens
  critique (2026-06-22): the three gating findings (round-trip, write-path losslessness, reuse
  estimate). All resolved via amendments.
- **[composable-redesign-rebuttal.md](composable-redesign-rebuttal.md)** — owner's resolution of
  the assessment; where the line was held (synthesis out of core, work stays extension-resident).
- **[spike-roundtrip-verdict.md](spike-roundtrip-verdict.md)** — the gating parse/serialize spike:
  **PASSED** 2026-06-23 (~396k fuzz execs, 0 failures). Unblocked v1.
- **[foundation-review-2026-06-24.md](foundation-review-2026-06-24.md)** — independent code review
  of T0–T2 foundations (Godfrey).
- **[code-walkthrough-foundation.md](code-walkthrough-foundation.md)** — guided tour of the shipped
  T0–T3 code.
- [node-representations.html](node-representations.html) — visual explainer, node inline/envelope forms.

## SUPERSEDED — kept for rationale, do not build from these

- **[../reckon-redesign_2026-06-15.md](../reckon-redesign_2026-06-15.md)** +
  **[../reckon-spec_2026-06-15.md](../reckon-spec_2026-06-15.md)** — Godfrey's DB-first,
  work-coupled design. Halted at NO BUILD (2026-06-15, see doc bottom); its "portable core"
  split seeded the composable design.

## HISTORICAL — gen-1 reckon (pre-redesign)

- [../reckon-plan_2025-12-22.md](../reckon-plan_2025-12-22.md) — original plan.
- [../2026-01-17-review-and-assessment.md](../2026-01-17-review-and-assessment.md) — gen-1 code
  assessment (B+; its criticisms fed both redesigns).
- [../migration-tui-redesign.md](../migration-tui-redesign.md) — gen-1 TUI layout migration.

## OPERATIONAL — still-valid engineering docs (code-level, not design)

- [../TESTING.md](../TESTING.md) · [../ASYNC_PATTERNS.md](../ASYNC_PATTERNS.md) ·
  [../REVIEW_PATTERNS.md](../REVIEW_PATTERNS.md) · [../exit-codes.md](../exit-codes.md) ·
  [../logging.md](../logging.md) · [../bd-usage.md](../bd-usage.md) ·
  [../agents/](../agents/) (work-ticket pipeline roles)

> Caveat: gen-1 docs and some operational docs describe subsystems the truth-inversion (v1-T9)
> will replace. When an operational doc contradicts composable-redesign.md, the design doc wins.
