# Acceptance Criteria: Review M-parser-scope — enumerate supported frontmatter/markdown subset + real-Obsidian tests (reckon-vj55)

Source: `docs/design/foundation-review-2026-06-24.md` finding **"M4 vs Raw safety —
the parser scope note"** (the un-numbered M-class finding right after M4, lines
35-40) and disposition entry `M-parser-scope — ticket reckon-vj55` (line 70-72);
`docs/design/code-walkthrough-foundation.md` §1.2 review note (lines 171-175).
Blocks `reckon-uv09` (v1-T4: `rk log`) in beads — a hard dependency, not
advisory, even though the review disposition and the ticket text both frame the
deadline as "before T8 (linking), ideally before T4." `bd show` confirms
`reckon-vj55` is in `BLOCKS` for `reckon-uv09`, so in practice it gates T4 too.

## Current-state facts (grounded in code + empirical verification)

All four gaps were reproduced against the current tree (`internal/node/node.go`)
with throwaway test cases before writing this doc; exact behavior below is
observed, not inferred from the ticket's summary prose (which turns out to be
slightly imprecise on gap 4 — see below).

- **The bug lives in `internal/node`, not `internal/parser`.** The ticket's own
  framing text says "internal/parser (wikilink `[[...]]` extraction feeding the
  index's `references` edges)," but that's the **legacy v0** package
  (`internal/parser/links.go`, `ExtractWikiLinks`), used only by
  `internal/cli/notes.go` / `internal/service/notes_service.go`. It already
  strips both fenced *and* inline code before matching links
  (`removeCodeBlocks`, `links.go:143-153`) and already has a README
  (`internal/parser/README.md`) documenting its supported syntax. The **v1
  index** (`internal/index`, used by `rk index`/`rk query`, what T4/T8 build on)
  parses exclusively through `node.MarkdownParser` → `node.Parse` →
  `extractBody` (`internal/node/node.go:241-260`), which has its own,
  independent wikilink regex application and does **not** skip inline code —
  only fenced blocks. **This ticket's fix belongs in `internal/node`.** Any doc
  enumeration (deliverable (a)) belongs there too — `internal/node` currently
  has no package-level README (unlike `internal/parser`); the closest thing to
  "docs" today is the package doc comment atop `node.go` (lines 1-28, esp. the
  "FORMAT COUPLING" paragraph).
- **(b) Block-scalar YAML lists are silently invisible, not merely "not
  parsed."** `aliases:\n  - a\n  - b` parses as `aliases: ""` (the `aliases:`
  line itself matches `fmScalarRe` with an empty value capture; the following
  `  - a` / `  - b` lines don't match `fmScalarRe` at all — no leading key — so
  they're skipped in the `parseFrontmatter` loop, never entering `frontmatter`/
  `fmOrder`). `deriveView` then calls `parseAliases("")`, which returns `nil`.
  Result: `n.Aliases == nil`, no error, no warning — indistinguishable from "no
  aliases specified." Confirmed empirically. Note the **flow-style** form
  (`aliases: [a, b]`) already works today (`parseAliases`, `node.go:264-279`,
  and `TestAliasParsing` in `node_test.go:181-197`) — the gap is specifically
  the **block** form, not "lists" in general.
- **(c) CRLF fails at the very first byte check, not gradually.** `parseFrontmatter`
  requires `bytes.HasPrefix(raw, []byte("---\n"))` (`node.go:165`). A CRLF file's
  first four bytes are `-`,`-`,`-`,`\r` — the prefix check fails outright, so
  `parseFrontmatter` returns `bodyStart=0` and the **entire file, including the
  frontmatter delimiters, becomes `Body`**. `ULID`/`Type`/`Author`/`Aliases`/
  `Props` are all zero values. No error is returned — `Parse` succeeds. This is
  confirmed empirically. **Important precision**: CRLF *inside the body only*
  (frontmatter delimited with clean LF, but a CRLF line further down in the
  body) parses fine today — `ULID`/`Type` populate correctly and the CRLF bytes
  are preserved verbatim in `Body`. The bug is specific to CRLF on/around the
  `---` delimiter lines, not "any CRLF byte in the file."
- **(d) Inline code `[[x]]` is linkified — confirmed, matches the ticket's
  description exactly.** `extractBody` (`node.go:241-260`) toggles `inFence` on
  a fence-marker line (`fenceRe`, triple backtick/tilde) but has no notion of
  single-backtick inline spans; it runs `wikilinkRe` against the raw line text
  unconditionally. `` See `[[not-a-link]]` here. `` produces a spurious
  `Link{Rel: "references", To: "not-a-link"}`. **This is the one deliverable
  the ticket text gives no "or unsupported" escape hatch for** — it must
  actually be fixed, not flagged.
- **(e) Multi-target ref props are corrupted, not merely "dropped."** The
  ticket's summary says `depends: [[A]], [[B]]` is "dropped to a plain prop
  string" — empirically that's **not what happens**. `refValRe` is
  `^\[\[(.+?)\]\]$`, anchored to the whole trimmed value. For the string
  `[[A]], [[B]]`, the *only* way `\]\]$` can match is against the string's
  final two characters (which are the `]]` closing `[[B]]`), forcing the lazy
  capture group to swallow everything in between. The regex **does match**,
  producing a single, mangled `Link{Rel: "depends", To: "A]], [[B"}` — a
  garbage target string containing literal `]`, `,`, `[` characters, silently
  written into the index as an edge target that will never resolve to
  anything. This is **worse** than a silent drop: it fabricates a bad edge.
  Verified for both `[[A]], [[B]]` (with space) and `[[A]],[[B]]` (no space) —
  same corruption pattern, different garbage string. Planning should treat this
  as higher severity than the ticket text implies.
- **Mixed flat + block-style keys in the same frontmatter block do not
  corrupt sibling keys.** `aliases:\n  - a\n  - b\ntags: zettel` still parses
  `tags` correctly (`Props["tags"] == "zettel"`, `Type` unaffected) — the
  block-list lines are just skipped, not treated as garbage that de-syncs the
  rest of the loop. Useful for scoping (e): whatever fix is chosen doesn't need
  to defend against corrupting unrelated keys, that part already works.
- **Existing "loud" precedents in this codebase** (relevant to the "explicitly
  +loudly unsupported" escape hatch in (b)/(c)/(e)):
  1. **Per-file structured log**: `logger.Warn("index: skipping unparsable
     file", "path", rel, "err", perr)` (`internal/index/reconcile.go:189`),
     used when `node.Parse` returns a hard error (e.g. conflict markers) —
     the file is skipped and its old index rows are swept.
  2. **Cross-file structured warnings surfaced through `rk index`**:
     `index.Stats.Warnings []Warning` (`reconcile.go:20-40`), populated by
     `collectWarnings` for `duplicate_ulid` / `alias_collision` (the
     `reckon-5b44` precedent, landed just before this ticket per git log:
     `b1198f2 index: Detect duplicate-ULID and alias collisions in reconcile
     (#142)`). Surfaced via `internal/cli/index.go`'s `indexResult.Warnings`
     field, both in JSON output and a human-readable "warnings (N):" block in
     `Pretty()` (`index.go:73-95`). This is the most directly analogous
     precedent to "explicitly+loudly unsupported": it is exactly a case of
     "detected a data-quality problem the parser could silently swallow,
     chose to surface it via a typed `Warning.Kind` instead."
  - **Neither mechanism currently fires for any of the four gaps.** All four
    are 100% silent today — not even routed through the existing
    "skip-and-log" path, because `node.Parse` returns no error in any of the
    four cases (block-scalar, CRLF, inline-code, multi-target all "succeed"
    with wrong/silently-degraded output). Whichever loud mechanism is chosen,
    it is new wiring, not a matter of connecting an existing signal.

## 1. Explicit Acceptance Criteria

Derived directly from the ticket's "Done when" clause. The ticket bundles
**five distinct deliverables** plus one cross-cutting test requirement — none
of them optional, but three of them (b/c/e) have an explicit escape hatch that
still requires an explicit choice (see "Required Decisions" callouts below).

1. **(a) The supported frontmatter/markdown subset is enumerated in node
   package docs.** A concrete, written enumeration exists of what
   `internal/node`'s parser actually supports: frontmatter shape (single-line
   scalars; flow-style `[a, b]` lists; block-style lists — pending decision
   (b)), line-ending support (LF; CRLF — pending decision (c)), wikilink forms
   (`[[target]]`, `[[target|label]]`, `[[target#Heading]]`,
   `[[target#^block]]`, `![[embed]]`), where wikilinks are inert (fenced code —
   already true; inline code — after fixing (d); indented code blocks — needs
   a decision, see Edge Cases), and ref-valued frontmatter props (single-target
   `key: "[[X]]"`; multi-target — pending decision (e)). **Location is not
   specified by the ticket text and is ambiguous** — see "Required Decisions."
2. **(b) Block-scalar frontmatter (YAML block-style lists) is handled, or
   explicitly+loudly unsupported.** `aliases:\n  - a\n  - b` (and any other
   reserved or prop key using this form) must either (i) parse to the same
   typed result as the equivalent flow form (`aliases: [a, b]`) would, or (ii)
   be detected and surfaced through a loud, non-silent channel (see
   "Required Decisions" — this is NOT satisfied by leaving today's silent
   `nil`/empty-string behavior in place; "silently mis-parsing is the one
   option explicitly ruled out" per the task brief).
3. **(c) CRLF files are handled, or explicitly+loudly unsupported.** A file
   whose frontmatter delimiters use `\r\n` must either (i) parse identically to
   the LF equivalent (typed fields populate, body is correct), or (ii) be
   detected and surfaced loudly — not silently degrade to "whole file is body,
   every typed field empty," which is today's behavior.
4. **(d) Inline code renders `[[x]]` inert — must be fixed, no escape hatch.**
   `` `[[x]]` `` inside a single-backtick inline code span must **not** produce
   a `references` (or any) edge. The ticket text gives every other gap an "or
   unsupported" out; this one does not — per the task brief, "it must be
   fixed." Silently-linkifying inline code remains the status quo only until
   this ticket lands; there is no acceptable "detect and warn" alternative for
   this one.
5. **(e) Multi-target ref props are handled, or explicitly+loudly
   unsupported.** `depends: [[A]], [[B]]` (comma-separated wikilinks on one
   scalar line) must either (i) parse into multiple `Link` entries (one per
   target, same `Rel`), or (ii) be detected and surfaced loudly. **Critically,
   "unsupported" must mean "detected and rejected/warned," not today's actual
   behavior** — today it silently fabricates one corrupted `Link` with a
   garbage `To` value (see Current-state facts). A prop parsers currently
   silently miss must not continue to produce a bad edge; at minimum, "loudly
   unsupported" here requires suppressing the garbage-edge fabrication in
   favor of either a clean drop-with-warning or a hard parse signal.
6. **Tests cover the real Obsidian shapes for all of the above**, "since the
   same vault is edited by Obsidian" (the ticket's own justification) — meaning
   test fixtures should resemble what Obsidian's frontmatter/Properties editor
   and markdown editor actually emit, not only hand-minimized synthetic
   reproductions. See §4.

### Required Decisions (planning must resolve these, not this doc)

For **each** of (b), (c), (e) — and, subordinately, for the doc-location
question in (a) — the ticket text explicitly offers two branches ("handled ...
or explicitly+loudly unsupported"). This doc deliberately does **not** pick a
branch; that's a planning-phase call informed by how hard each is to actually
fix correctly (codebase-analysis.md's job). What this doc asserts is that a
choice must be made and recorded, for each of the three, independently:

- **(b) block-scalar lists**: parse it (extend `parseFrontmatter`/`parseAliases`
  to walk indented `- item` continuation lines under a bare `key:` line) vs.
  detect-and-warn (leave block-form values invisible but surface a
  `Warning{Kind: "block_scalar_frontmatter"}`-shaped signal, or equivalent, so
  it's not silent). Complexity note for planning: parsing requires tracking
  indentation and multi-line spans, a bigger change to `parseFrontmatter`'s
  current single-line-per-field model than (c) or (e).
- **(c) CRLF**: parse it (normalize/detect `\r\n` around the frontmatter
  delimiter, likely the cheapest fix — e.g. accept `---\r\n` as an alternate
  prefix and handle `\r`-stripping when computing scalar values, or normalize
  the whole `raw` slice's line endings at the top of `ParseAt` before frontmatter
  detection — normalization changes `Raw` semantics and needs to be reconciled
  with the byte-preservation invariant, see Implicit Requirements) vs.
  detect-and-warn (keep today's whole-file-becomes-body outcome but at least
  surface it instead of silence).
- **(e) multi-target refs**: parse it (split comma-separated `[[...]]` tokens
  on a single scalar value into multiple `Link`s sharing one `Rel` — note this
  has a **downstream wrinkle**: `Render()` (`render.go:76-82`) currently emits
  one frontmatter line per `Link`, so two `Link`s with the same `Rel` would
  render as two duplicate-keyed `key: "[[...]]"` lines, which **do not
  round-trip** through `parseFrontmatter` (last-line-wins on a duplicate key,
  per `node.go:193-197`'s `n.frontmatter[key] = ...` overwrite). Whether
  `Render` also needs updating to stay consistent is a planning call — flagged
  here, not resolved, since this ticket's user-facing brief scopes it as
  read/parse-only) vs. detect-and-warn (leave multi-target props un-parsed but
  stop the current garbage-edge fabrication — at minimum this branch requires
  fixing the corruption even if full multi-target parsing is deferred).
- **(a) doc location**: new `internal/node/README.md` (mirroring
  `internal/parser/README.md`'s existing pattern) vs. expanding the package doc
  comment at the top of `node.go` (lines 1-28) vs. a `docs/design/` entry.
  Not specified by the ticket text.

## 2. Implicit Requirements

- **Raw stays byte-safe; this is a derived-view fix, not a write-path
  ticket.** The ticket's own framing ("Raw stays byte-safe but the DERIVED
  view... is wrong") and the task brief both confirm this is scoped to
  `Parse`/`deriveView`/`extractBody` (the read side). Unlike `reckon-9bfx`
  (ULID mint policy / `rk adopt`, a write ticket), nothing here should change
  what gets written to a vault file. The one place this brushes against a
  write path is `Render()` (create-path serialization, see the (e) Required
  Decision above) — if a fix chooses to represent multi-target refs as
  multiple same-`Rel` `Link`s, `Render`'s current implementation cannot
  losslessly recreate that shape on the next parse, which is a latent
  round-trip risk even though this ticket doesn't have to close it.
- **No regression of currently-correctly-parsed shapes.** Must continue to
  pass unchanged: single-line scalar frontmatter, flow-style list aliases
  (`aliases: [a, b]`), LF line endings, single-target quoted ref props
  (`depends: "[[X]]"`), fenced-code-block inertness, the existing adversarial
  corpus in `node_test.go` (`noteObsidian`, `todoItem`, `logDay` — carried
  verbatim from the gating spike per the file's own header comment) and
  `render_test.go`'s create-round-trip gate (`reckon-s0ix`). Any fix that
  changes `parseFrontmatter`'s line-scanning loop, `extractBody`'s per-line
  processing, or `parseRefValue`/`refValRe` needs to re-run these as a
  regression gate, not just add new passing tests alongside possibly-broken
  old ones.
- **"Loudly unsupported" implies a surfaced signal a user/operator actually
  sees — not a silent no-op, and not merely a code comment.** This codebase
  has two live conventions to choose from (see Current-state facts): (1)
  `logger.Warn` per-file (fits a per-file parse anomaly, like CRLF or
  block-scalar, that affects one node's typed view), or (2) a new
  `index.Warning.Kind` entry aggregated into `Stats.Warnings` and surfaced
  through `rk index`'s JSON/pretty output (fits a cross-file or
  data-quality-class issue, matching the `reckon-5b44` precedent for
  `duplicate_ulid`/`alias_collision`). **Which convention (or both) applies to
  which of (b)/(c)/(e) is a planning decision** — flagged, not resolved, here.
  Whichever is chosen, "loud" must reach `rk index`'s user-visible output or
  the process log — not just an internal error value nobody reads.
- **Must land before `reckon-uv09` (T4) in practice**, despite the ticket
  text's softer "ideally before T4" / disposition's "before T8" framing — `bd
  show reckon-vj55` shows a hard `BLOCKS → reckon-uv09` edge, and `bd show
  reckon-uv09`'s `DEPENDS ON` list currently shows `reckon-vj55` as the only
  ◐ (in-progress, not yet ✓) dependency, with every other T4 prerequisite
  already closed. This ticket is the last thing standing between "ready" and
  T4 starting.
- **Node-package-doc enumeration (a) must reflect the *shipped* behavior after
  (b)/(c)/(d)/(e) land, not the pre-fix state or an aspirational future
  state.** If a gap is resolved via "detect and warn" rather than "parse
  correctly," the docs must say so explicitly (e.g. "block-scalar YAML lists
  are detected and rejected with a warning; use flow-style `[a, b]`
  instead") rather than implying full support.
- **Scope stays inside `internal/node`** (see Current-state facts on
  `internal/parser` vs `internal/node`) — `internal/parser/links.go`'s
  `ExtractWikiLinks` is a different, already-more-correct implementation
  serving the legacy v0 notes path; this ticket doesn't need to touch it, and
  doing so wouldn't fix the actual bug (the v1 index doesn't call it).

## 3. Edge Cases

### Block-scalar YAML (b)

- **Single-item list**: `aliases:\n  - only-one`. Should behave identically to
  `aliases: [only-one]` if (b) chooses "parse it."
- **Multi-item list**: `aliases:\n  - a\n  - b\n  - c`.
- **Empty list**: `aliases:` with no following `- item` lines (immediately
  followed by another key or the closing `---`). Confirmed empirically this
  already yields `Aliases == nil` today (same as an absent key) — arguably
  already "correct" by accident, worth a regression test either way so a fix
  to (b) doesn't accidentally start erroring on this legitimate empty case.
- **Nested structure** (a map value, or a list of maps) — e.g.
  `custom:\n  key: val`. **Should be OUT of scope** — the ticket's "Done when"
  names "block-scalar frontmatter (lists)" specifically, not general nested
  YAML. Flag for planning to confirm and, if so, document as an explicit
  non-goal in (a)'s enumeration (see §5).
- **Mixed flat + block keys in the same frontmatter block**: confirmed
  empirically that today's parser already tolerates this without corrupting
  sibling flat keys (block-list lines are inert no-ops in the scan loop, not
  garbage that desyncs parsing) — any (b) fix should preserve this, and a
  regression test should assert `tags:` (or another sibling flat key) parses
  correctly both before and after a block-style `aliases:` list in the same
  block.
- **Block list on a non-`aliases` key** (e.g. a hypothetical `tags:\n  - a`
  prop, not a reserved key) — should route the same way `aliases` does,
  through whatever general block-scalar handling (b) adds, not a
  hardcoded-to-`aliases` special case, since Props keys are open-ended.

### CRLF (c)

- **Pure CRLF file** (frontmatter delimiters and body both `\r\n`) — the
  primary documented failure; confirmed empirically total silent failure
  today.
- **Mixed LF/CRLF within one file** — genuinely ambiguous per the task brief:
  should this be normalized (treat `\r\n` and `\n` interchangeably line by
  line) or rejected as malformed (closer to the existing conflict-marker
  refusal in `ParseAt`, `node.go:110-114`)? Not decided by the ticket text;
  flag for planning. Note Obsidian itself is unlikely to *produce* a
  mixed-EOL file from a single save, but a file could accumulate mixed EOLs
  from edits across tools (e.g. an agent using `\n` appends into a
  Windows-authored `\r\n` file, or vice versa) — a real scenario for a
  multi-device Syncthing vault, not a purely synthetic one.
- **CRLF only in frontmatter, LF in body** (and the reverse — LF frontmatter,
  CRLF in body). Confirmed empirically that **CRLF-only-in-body already works
  today** (frontmatter parses correctly, body preserves the `\r\n` bytes
  verbatim) — this is not part of the bug and must not be broken by a (c) fix.
  CRLF-only-in-frontmatter-with-LF-body is a plausible partial-conversion
  scenario (e.g. a tool that line-ended-normalizes the body but not
  frontmatter, or vice versa) worth an explicit test either way.
- **The closing `---` delimiter's line ending**, independent of the opening
  one — `parseFrontmatter` locates the close via `bytes.Index(rest,
  []byte("\n---"))` (`node.go:169`), which itself assumes an LF before the
  closing fence; a file that's CRLF-consistent throughout needs the closing
  detection fixed too, not just the opening `HasPrefix` check, if (c) chooses
  "parse it."

### Inline code (d)

- **`[[x]]` in a single backtick span**: `` `[[x]]` `` — the primary
  documented, confirmed-reproduced case.
- **`[[x]]` in a fenced code block** (triple backtick) — already correctly
  inert today (`extractBody`'s `inFence` toggle); must not regress.
- **`[[x]]` in an indented code block** (4-space/tab indent, no fence
  markers) — **not currently handled at all** (fenced-only detection); the
  ticket text doesn't explicitly name indented code blocks, only "inline
  code," so whether this is in scope is ambiguous — flag for planning.
  Markdown spec treats indented blocks as code, and Obsidian renders them as
  such, so a strict reading of "markdown subset" might include it, but the
  review finding's wording ("Inline code `[[x]]` is linkified... only fenced
  blocks are skipped") frames the gap narrowly around inline spans and fences,
  not indentation.
- **`[[x]]` immediately adjacent to backtick span boundaries** — e.g.
  `` `code`[[link]] `` (link right after a code span closes) or
  `[[link]]`` `code` `` (link right before one opens) — the link here is
  *not* inside the code span and should remain linkified; a naive
  strip-between-backticks implementation must get the boundary right (don't
  over-strip and accidentally swallow an adjacent real link).
- **Nested/escaped backticks** — e.g. double-backtick spans used to include a
  literal backtick (`` ``code with ` inside`` ``) — markdown allows this; a
  correct inline-code detector needs to handle variable-length backtick
  fences for spans, not just single backticks, or it will misparse the span
  boundary. Worth at least one test case; full CommonMark backtick-fence
  matching may be more than this ticket needs — flag as a possible scope
  boundary (see §5, general markdown/YAML spec compliance is out of scope).
- **Unterminated / unbalanced single backtick on a line** (e.g. a stray `` ` ``
  with no matching close before end of line) — should not crash or hang;
  Obsidian itself treats an unmatched backtick as a literal character, and any
  `[[x]]` on that line remains a real link (nothing to make inert, since there's
  no valid span).

### Multi-target refs (e)

- **Two targets**: `depends: [[A]], [[B]]` — the ticket's own example.
- **Three or more targets**: `depends: [[A]], [[B]], [[C]]`.
- **Mixed valid/malformed targets in one list**: e.g. `depends: [[A]], not-a-link,
  [[B]]` or `depends: [[A]], [[]]` (empty target) — behavior undefined by the
  ticket text; planning should decide whether to parse the valid entries and
  warn on the malformed one(s), or reject the whole line as malformed.
- **Whitespace variations**: `[[A]],[[B]]` (no space after comma, confirmed
  empirically to hit the same corruption path as the spaced form, just a
  different garbage string) vs. `[[A]], [[B]]` (single space) vs. extra
  whitespace (`[[A]] ,  [[B]]`) — a correct splitter should tolerate all of
  these consistently since Obsidian's own list-typed-property UI is not
  guaranteed to produce a single canonical spacing convention (and hand-edits
  won't either).
- **Single target still using bare (non-comma) form**: `depends: "[[X]]"` —
  must continue to work exactly as today (regression, not a new case).
- **Fragment/label suffixes on multi-target entries**: `depends: [[A#^blk]],
  [[B|Label]]` — each target in a multi-target list may itself carry a
  fragment or label suffix, same as single-target refs already support via
  `splitRef`; a correct multi-target parser should reuse `splitRef` per-token,
  not just split on commas and stop.

## 4. Test Scenarios (Given/When/Then)

All fixtures below are meant to resemble real Obsidian output, per the
ticket's own justification ("since the same vault is edited by Obsidian"), not
minimal synthetic strings alone. **Caveat for planning**: this doc's author
does not have a live Obsidian instance to capture ground-truth output, so the
shapes below are based on the review finding's own examples plus generally
understood Obsidian conventions (its Properties panel, introduced Obsidian
1.4/2023, is known to write multi-item list properties — like `aliases` and
`tags` — in **block style** by default when edited through the UI, while
directly hand-edited or agent-written frontmatter more often uses flow style).
**Planning/implementation should verify this against an actual current-version
Obsidian vault export (or Obsidian's documented YAML behavior) rather than
trust this assumption blindly** — flagged explicitly per the task brief.

### (a) Documented subset

**Scenario: doc enumerates the shipped shapes**
Given the node package's docs (location per the Required Decision above)
When a reader consults them for "does reckon support X frontmatter/markdown
shape"
Then every one of: single-line scalars, flow-style lists, block-style lists
(with whichever disposition (b) lands on stated explicitly), LF and CRLF (with
(c)'s disposition stated), wikilink forms, inline-code inertness, and
multi-target ref props (with (e)'s disposition stated) is named with its
support status — not merely implied by test coverage

This is a documentation-presence check, not something asserted by a Go test;
it should be verified by manual review against the AC list in §1 during
implementation review, not by an automated test.

### (b) Block-scalar frontmatter

**Scenario: Obsidian-shaped multi-item block aliases list**
Given a note file with frontmatter
```
---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHED
type: note
aliases:
  - project-x
  - proj-x
tags: [zettel, active]
---
# Project X
```
(block-style `aliases`, flow-style `tags` in the same block — the mixed shape
a real Obsidian Properties-panel edit could plausibly produce depending on
which property was edited last)
When `node.Parse` runs
Then `n.Aliases` contains `project-x` and `proj-x` (if (b) = "parse it") OR a
loud signal is emitted identifying `aliases` as an unsupported block-scalar
value and `n.Aliases` is documented as empty in that case (if (b) =
"unsupported")
And `n.Props["tags"]` is unaffected either way (regression: flow-style
sibling key must keep working)

**Scenario: single-item block list**
Given `aliases:\n  - solo-alias` (block form, one item)
When parsed
Then behaves per (b)'s chosen disposition, consistently with the multi-item
case (not a special-cased single-item path)

**Scenario: empty block list stays a no-op**
Given `aliases:` with no following `- item` lines
When parsed
Then `n.Aliases` is empty/nil, same as today (regression — this case already
"works" and should not start erroring)

### (c) CRLF

**Scenario: whole-file CRLF, as a Windows-authored or Windows-synced Obsidian
note might arrive via Syncthing**
Given
```
---\r\n
id: 01J9Z3K7Q2W8XR4M6N0V5BYHED\r\n
type: note\r\n
aliases: [x]\r\n
---\r\n
# Title\r\n
\r\n
Body text with a [[link]].\r\n
```
(entire file CRLF-terminated)
When `node.Parse` runs
Then `n.ULID`, `n.Type`, `n.Aliases` all populate correctly (if (c) = "parse
it") OR a loud signal is emitted identifying the file as CRLF-frontmatter and
unsupported, distinct from today's silent empty-fields outcome (if (c) =
"unsupported")
And the body's `[[link]]` still produces a `references` edge either way
(the link-extraction logic itself is line-oriented and mostly EOL-agnostic
once frontmatter is correctly delimited)

**Scenario: CRLF-only-in-body regression**
Given LF-delimited frontmatter but a body containing a `\r\n` line (e.g. a
paragraph pasted from a Windows source into an otherwise-LF file)
When parsed
Then `n.ULID`/`n.Type` still populate correctly and `n.Body` preserves the
`\r\n` byte verbatim (regression — already works today, per empirical
verification, must not break)

### (d) Inline-code-inert

**Scenario: inline code span containing a wikilink-shaped string, as would
appear when documenting reckon's own syntax inside an Obsidian note**
Given a body:
```
Use the `[[target]]` syntax to link notes. See also ``code with `nested` tick``.
```
When `node.Parse` runs
Then no `Link` with `To == "target"` (or any link derived from text inside
either code span) appears in `n.Links`

**Scenario: fenced code block regression**
Given the existing `noteObsidian` fixture's fenced block (`node_test.go`,
containing `` fmt.Println("[[alsonotalink]]") ``)
When parsed
Then no link for `alsonotalink` appears (already passing today — must stay
green)

**Scenario: real link immediately adjacent to a code span is still linkified**
Given a body line: `` See `formatted code` and [[real-target]] right after. ``
When parsed
Then a `Link` with `To == "real-target"` IS present (the inline-code fix must
not over-strip and eat adjacent real links)

### (e) Multi-target ref props

**Scenario: Obsidian-shaped multi-value dependency list on one line**
Given frontmatter
```
---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHED
type: todo
depends: [[01J9Z2QH8M]], [[01J9Z2QH9N]]
---
```
When `node.Parse` runs
Then `n.Links` contains two `Link{Rel: "depends"}` entries, `To ==
"01J9Z2QH8M"` and `To == "01J9Z2QH9N"` respectively (if (e) = "parse it") OR a
loud signal is emitted and **no garbage-target `Link` is fabricated** (if (e)
= "unsupported" — this is the mandatory floor regardless of which branch is
chosen, since today's actual behavior of silently minting one corrupted edge
is strictly worse than either alternative)

**Scenario: single-target regression**
Given `depends: "[[01J9Z2QH8M]]"` (today's supported single-target quoted
form, from the existing `todoItem` fixture)
When parsed
Then exactly one `Link{Rel: "depends", To: "01J9Z2QH8M"}` is produced,
unchanged from today (regression per `TestCanonicalView`, `node_test.go`)

**Scenario: whitespace-insensitive splitting**
Given two otherwise-identical fixtures differing only in
`depends: [[A]],[[B]]` vs `depends: [[A]], [[B]]` vs `depends: [[A]] ,  [[B]]`
When each is parsed
Then all three produce the same set of `Link`s (if (e) = "parse it") — the
splitter must not be sensitive to incidental spacing a human or Obsidian's UI
might introduce

## 5. Out of Scope

- **General YAML spec compliance** — nested maps, anchors/aliases (`&`/`*` YAML
  references, not to be confused with reckon's `aliases:` frontmatter key),
  multi-document files (`---` separated multiple docs), flow-style maps
  (`{a: 1, b: 2}`), multi-line block scalars (`|` / `>` folded/literal
  strings) on non-list keys. The ticket's "Done when" text names
  "block-scalar frontmatter (lists)" specifically — not YAML compliance in
  general. A full YAML parser is explicitly not what "handled" means here.
- **Any WRITE-path changes.** This is a parse/read-correctness ticket
  (contrast `reckon-9bfx`, a write ticket for ULID mint policy). `SetField`'s
  span-splice behavior and `Render`'s create-path serialization are not
  required to change — though see the (e) Required Decision's note that
  `Render` has a latent round-trip gap *if* multi-target parsing is added,
  which is flagged but not mandated to fix here.
- **Changing the index schema** (`internal/index/schema.go`) — this ticket
  changes what `node.Parse` derives from a file's bytes, not how the index
  stores nodes/edges/warnings. If a new `Warning.Kind` is added as part of a
  "loudly unsupported" choice, that's additive to the existing `Warning`
  struct/mechanism (already schema-agnostic — warnings are recomputed, not
  persisted as their own table), not a schema migration.
- **Any UI/TUI rendering of parse-warnings.** Surfacing is via the existing
  `rk index` JSON/pretty output or process log (see Implicit Requirements),
  not new TUI work — the recently-landed checklist-run TUI (`reckon-yk1i`,
  #144) and its patterns are unrelated to this ticket's surface. Revisit only
  if research during implementation shows the existing `rk index` output
  channel is genuinely insufficient (not expected).
- **Performance optimization of the parser.** No performance requirement is
  named by the ticket; regex-based line scanning remains acceptable unless
  correctness requires otherwise.
- **`internal/parser` package changes.** As established in Current-state
  facts, the actual bug lives in `internal/node`; `internal/parser/links.go`
  (legacy v0, already handles inline+fenced code correctly) does not need
  touching for this ticket, even though the task's own framing text
  initially pointed there.
- **M2 (`reckon-9bfx`) and other foundation-review findings** (M3/M4,
  `reckon-5b44`, already closed; M5, `reckon-lrw2`) — separate tickets from
  the same review, not part of this one, though M3/M4's `Warning` mechanism is
  referenced above as prior art for the "loudly unsupported" decision.
