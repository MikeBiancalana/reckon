# Code Review: reckon-vj55 — parser-scope (frontmatter/markdown subset + real-Obsidian tests)

Verdict: APPROVE WITH CHANGES

Reviewer: code-reviewer (Opus 4.8)
Scope reviewed: `internal/node/node.go`, `internal/node/render.go`, `internal/node/node_test.go`, `internal/node/render_test.go`, `internal/node/AGENTS.md`, against the true ticket changeset (`origin/main...HEAD`, i.e. merge-base `ef2f46c` → HEAD `92e0907`).

## Summary

This is a genuinely solid piece of byte-offset-sensitive parser surgery. All four named gaps (block-scalar YAML lists, CRLF frontmatter, inline-code-inert, multi-target ref props) are handled correctly, the byte-preservation invariant is preserved by construction (verified by trace + 543k-exec fuzz, 0 failures), and the tests pin specific real-Obsidian-shaped behavior rather than synthetic minimal repros. I traced every offset-sensitive path the parent flagged and empirically probed the guard/mask edge cases; the core claims in the plan hold up. The single blocking item is not a code defect but an **integration** one: this branch was cut before PR #146 (`rk adopt`) landed on `origin/main`, and the two now **conflict in `node.go`** and must be reconciled before merge. Two minor doc nits round it out.

---

## The one required change (blocker): unmerged conflict with PR #146

The instruction said to review `git diff origin/main`, but that two-dot diff is **contaminated** and misleading: it shows ~1900 spurious deletions (`internal/cli/adopt.go`, `internal/node/insert.go`, `internal/index/reconcile.go`, `ticket-work/reckon-9bfx/*`, and the exported method `HasField`). None of those are this ticket's work — they belong to PR #146 (`reckon-9bfx`, "rk adopt", commit `1ca74e7`), which landed on `origin/main` **after** this branch diverged.

The true ticket changeset (`origin/main...HEAD`) is clean and exactly matches the plan:
`internal/node/{node.go, render.go, node_test.go, render_test.go, AGENTS.md}` + `ticket-work/reckon-vj55/*`. Nothing else. So the alarming deletions are a stale-base artifact, **not** scope creep — good.

However, a trial merge (`git merge-tree`) shows a real content conflict:

```
CONFLICT (content): Merge conflict in internal/node/node.go
```

Cause: #146 inserted `HasField` (and `InsertField`) immediately before `parseFrontmatter`, and this ticket rewrote the `parseFrontmatter` doc comment (adding the CRLF/EOL-handling paragraph) in the same region. The conflict is **benign in nature** — adjacent edits, no logic overlap — and the correct resolution is "keep both": retain #146's `HasField`/`InsertField` methods **and** this ticket's updated `parseFrontmatter` doc + CRLF logic. But it must be resolved by hand; a careless "accept ours" would silently drop #146's public API.

Required before landing:
1. Rebase (or merge) this branch onto current `origin/main`, resolving the `node.go` conflict so both #146's additions and this ticket's changes survive.
2. Re-run `go test ./...` and `FuzzRoundTripIdentity` after the rebase (the fuzz corpus/behavior can shift once `HasField`/`InsertField` coexist with the new scanning loop; nothing suggests a real interaction, but re-run to be safe).

I confirmed #146's `insert.go` does **not** reference any symbol this ticket changed (`parseRefValue`/`refValRe`/`parseFrontmatter` internals), so there is no semantic coupling — the conflict is purely textual.

---

## Findings across the 7 review dimensions

### 1. Correctness — PASS (with 3 low-severity edge notes)

I traced each of the parent's four offset-sensitive concerns end to end.

**CRLF byte-offset correctness (the load-bearing claim): verified correct.** For a whole-file-CRLF note I traced the opening gate (`headerLen` 5 vs 4), the closing gate (`bytes.Index(rest, "\n---")` finds the `\n` inside `\r\n---`; `closeNLLen` correctly distinguishes `\n` (1) from `\r\n` (2) and rejects anything else), and per-line value extraction. The key invariant holds: `effEnd` only shrinks the **substring handed to the regex** by one when the last byte is `\r`; `valStart`/`valEnd` are still `pos + m[6]/m[7]` where `m` indexes into a substring that starts at `pos` in `Raw` and contains no interior `\r`, so the recorded `Span` indexes `Raw` exactly (the `\r` stays physically in `Raw`, excluded from the value span). `SetField` on a CRLF-parsed node therefore splices correctly — `TestCRLFFrontmatter/setfield_after_crlf_parse_stays_surgical` proves it, and I re-ran it green. No off-by-one, no offset computed against a `\r`-stripped view. This is exactly the subtle bug class the parent worried about, and it is genuinely avoided.

Incidental note: in the main scan loop, `raw[closeAbs-1]` is always the `\n` from the `\n---` match, so `nl` is always `>= 0`; the `if nl < 0 { break }` (node.go:254) and the `&& nl >= 0` clause (node.go:233) are effectively dead/defensive. Harmless, arguably worth a one-line comment, not worth changing.

**Multi-target "no garbage Link" guard: verified correct, mandatory floor holds.** `parseRefValues` checks `len(matches) == 0` **before** the separator remainder logic, so the parent's specific worry — "zero wikilink matches but remainder is all separators" (e.g. `,,, `) — correctly returns `ok=false` (I confirmed empirically). `[[]]` (empty target) is rejected by `wikilinkRe`'s `[^\]]+`, also `ok=false`. The ticket's headline bug (`[[A]], [[B]]` → fabricated `Link{To:"A]], [[B"}`) is fixed, and the mixed case `[[A]], not-a-link` falls through to `Props` with no link — pinned by `TestMultiTargetRefProp/mixed_valid_invalid_no_garbage_link`. The `"` in `refListSeparators` is load-bearing and correct: it's what lets Render's `depends: "[[A]]", "[[B]]"` output round-trip (traced below).

Two exotic-input notes (both **pre-existing behavior, not regressions, not blocking**): a malformed `[[[A]]]` parses to `Link{To:"[A"}` and `[[a[b]]` to `Link{To:"a[b"}` (brackets leak into the target because `[`/`]` are in the separator set and `wikilinkRe`'s inner class permits `[`). The old anchored `refValRe` would have produced the same single weird target, so this is no worse than before, and such inputs are not valid Obsidian syntax. Index resolution simply won't match them. Worth a mention only.

**maskInlineCode: verified correct per CommonMark's backtick-run rule.** A run of N backticks is closed only by a run of **exactly** N (the inner loop skips differently-sized runs via `k = m`), so the double-backtick-with-nested-single-backtick case is masked as one span — confirmed by trace and by `TestInlineCodeInert/double_backtick_span_with_nested_backtick_is_inert`. Unterminated runs set `i = j` and leave the rest of the line scannable, so a stray backtick doesn't blind a later real `[[link]]` (`unterminated_backtick_does_not_blind_rest_of_line`). Length-preserving space masking keeps `blockAnchorRe`'s end-anchor positions valid, and masking is applied to `masked` before **both** `wikilinkRe` and `blockAnchorRe` — consistent, as required.

One real edge note (low severity, **new** behavior): because masking replaces backticks with spaces, a `^blockid` glued directly to a closing backtick with no intervening space — `` `code`^bid`` at line end — now fabricates a `Fragment{ID:"bid"}` (I confirmed empirically), because the masked space satisfies `blockAnchorRe`'s leading `\s`. In the raw text the char before `^` was a backtick, not whitespace, so strictly this is a spurious fragment. Real-world likelihood is near zero, and a spurious node-local fragment is low-impact, but it's a behavior the mask introduces. Optional: mask to a non-whitespace sentinel, or document it. Not blocking.

### 2. Architecture — PASS

The fixes are correctly contained to the derived-view layer (`parseFrontmatter`/`scanBlockList`/`deriveView`/`maskInlineCode`/`parseRefValues`) with no touch to `Raw`, no schema change, no `index`/`reconcile` change, and an unchanged `Parse` signature — exactly as the plan argued. The decision to reject `yaml.v3` and hand-synthesize block lists into the already-proven flow path (`"[a, b]"`) is well justified by the span-tracking constraint; block-list keys correctly get no `fieldSpan`, so `SetField` refuses them by construction rather than by a special case. `deriveView`'s fan-out to N `Link`s is clean. Helpers are cohesive and single-purpose.

### 3. Testing — PASS (strong)

The 5 new tests assert specific outcomes, not just "no error": exact link counts/targets and per-token `#frag`/`|label` (`TestMultiTargetRefProp`), populated typed fields + no trailing `\r` + surgical `SetField` (`TestCRLFFrontmatter`), inert-plus-adjacent-real-link (`TestInlineCodeInert`), aliases-plus-undisturbed-sibling and empty-list-stays-nil regression (`TestBlockScalarAliases`), and a true Render→Parse round trip preserving two same-rel links (`TestMultiTargetRenderRoundTrip`). Fixtures match real Obsidian Properties-panel shapes (block `aliases:` with `  - item`, whole-file CRLF, single-backtick inline code) per the ticket's "same vault edited by Obsidian" justification. The 4 new `roundtripCorpus` entries plus CRLF/block-scalar fuzz seeds extend the byte-safety gate to the new shapes. Existing regression tests (`TestRoundTripIdentity`, `TestCanonicalView`, `TestAliasParsing`, `TestSpanLocalEditIsSurgical`, `TestStructuredViewIgnoresFences`, all of `render_test.go`) are **untouched** — the only `node_test.go` deletions are the `roundtripCorpus` map lines that got reformatted when the 4 entries were added; `render_test.go` is purely additive. Confirmed.

Optional coverage gaps (nice-to-have, not required): CRLF combined with a block list; an inline-code span containing a `^blockid`; a mixed-EOL file (frontmatter LF / body CRLF or vice versa, which the plan claims is tolerated); and Render idempotency for the multi-target form.

### 4. Maintainability — PASS

Comments are precise and explain *why* (the `effEnd`/`Raw`-offset comment at node.go:220-223, the "no fieldSpan" rationale, the CommonMark run-length note). AGENTS.md is accurate against actual behavior and fills a real gap (only sibling package without one). Two doc nits below.

### 5. Error handling — PASS

Consistent with the package's design: parser functions under-derive rather than error on unsupported shapes (never corrupt `Raw`), conflict markers are still refused, `SetField` still errors on span-less keys. No new error paths needed.

### 6. Performance — PASS

`maskInlineCode` allocates one line-length copy per body line and has a theoretical O(n²) worst case on a pathological all-backticks line; both are negligible for markdown. No ReDoS surface — all regexes are static, with no nested unbounded quantifiers over overlapping classes.

### 7. Security — PASS

Operates purely on in-memory `[]byte`; no file I/O, no eval/exec, no injection surface, no dynamically compiled patterns. Nothing to flag.

---

## Deep-dive verification of the parent's 6 specific concerns

1. **CRLF byte-offset correctness** — Verified in the diff, not just claimed. Spans index untouched `Raw`; `SetField` splice on a CRLF file is surgical (traced + test green). No off-by-one.
2. **"No garbage Link" guard** — Cannot slip a corrupted target through for the cases that matter: zero-match-all-separator → `false`; `[[]]` → `false`; mixed valid/invalid → `false`→Props. Only exotic malformed `[[[A]]]`-style inputs leak brackets into a target, unchanged from prior behavior.
3. **maskInlineCode** — Exact-run-length matching is correct per CommonMark; unterminated run left literal without blinding later content; applied before both regexes. One spurious-fragment edge on backtick-glued `^anchor` (new, low severity).
4. **Block-scalar synthesis** — Cannot corrupt `Raw` (round-trip/fuzz proven); the documented limitation (item with embedded comma/bracket mis-splits) is real but is a *mis-derivation*, not corruption — see doc nit below.
5. **Render round-trip** — `depends: "[[A]]", "[[B]]"` re-parses to exactly {A,B} via `parseRefValues` (traced through the outer-quote strip + separator remainder); single-link output is byte-identical to the pre-change `rel: "[[to]]"` form (`single_link_rel_unchanged` pins it).
6. **Regression safety** — Existing corpus and tests unmodified; additions only. All green locally; fuzz 543k execs / 0 failures.

---

## Minor / optional items

- **(doc, should-fix)** `render.go:28` still refers to the old function name `parseRefValue` ("the parser strips the quotes (parseRefValue)"). It's now `parseRefValues`. Update the reference.
- **(doc, should-fix)** The plan specified documenting that **block-list items containing a literal comma or bracket** mis-split (e.g. `  - a, b` synthesizes `[a, b]` → two aliases). AGENTS.md documents the general out-of-scope YAML set and the flow-list behavior but doesn't call out this specific block-item limitation. Add one sentence so the "markdown subset" boundary is unambiguous, as the plan intended.
- **(optional)** Multi-target links are not de-duplicated (`depends: [[A]], [[A]]` → two identical `Link`s → two `edges` rows). Harmless today; note if dedup is desired downstream.
- **(optional)** Consider masking inline code to a non-whitespace sentinel to avoid the backtick-glued-`^anchor` spurious-fragment edge, or document it as a known non-goal alongside the indented-code-block exclusion.

## Positive observations

- The CRLF fix's central insight — leniency checked independently at each of the three EOL-sensitive points, never normalizing the buffer — is the right call and is what keeps every `Span` a valid `Raw` offset. The `effEnd` comment makes the invariant explicit for the next reader.
- The `parseRefValues` "remainder must be only separators, and zero matches is not a list" guard is a clean, defensible formulation of the mandatory floor, and the choice to include `"` in the separator set to make Render's output round-trip is a nice closing-of-the-loop.
- Tests are honest: they assert the corrupted-target *absence* directly (`strings.Contains(l.To, "]]")`), not just link counts, which is exactly how you pin "no garbage fabricated."
- AGENTS.md is thorough, accurate, and states each gap's disposition explicitly — a real improvement to the package's onboarding surface.

## Bottom line

The implementation is correct, well-tested, and approvable. Resolve the `node.go` merge conflict against current `origin/main` (preserving #146's `HasField`/`InsertField` **and** this ticket's CRLF/doc changes), re-run tests + fuzz, and fix the two doc references. No code-logic changes are required.
