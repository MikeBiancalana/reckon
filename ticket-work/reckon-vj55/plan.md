# Implementation Plan: reckon-vj55 — parser-scope (frontmatter/markdown subset + real-Obsidian tests)

## Summary of approach

All four named gaps live entirely in `internal/node` (the DERIVED-view logic: `parseFrontmatter`, `deriveView`, `parseRefValue`, `extractBody`). The write/round-trip invariant (`Serialize()` returns `Raw` verbatim) is preserved by construction because none of these fixes touch `n.Raw` — they change how the typed projection is computed from it. Consequently **the entire code change is contained to `internal/node`** (node.go, render.go, a new AGENTS.md, tests). No `internal/index`/reconcile change, no schema change, and `node.Parse`'s `(*Node, error)` signature is unchanged.

### The four decisions (stated and justified)

**(b) Block-scalar YAML lists → PARSE IT (narrow fix, not yaml.v3).**
Reject swapping `fmScalarRe` for `gopkg.in/yaml.v3`. Blast radius is the deciding factor: the whole frontmatter scanner is span-tracked — `fieldSpans[key]` holds exact byte offsets into `Raw` that `SetField` (node.go:143-159) splices surgically, and `deriveView` treats each value as the literal string between colon and EOL. yaml.v3 gives no byte spans and applies YAML type coercion/quoting semantics (`id: 123` → int, dates, etc.), which would break `SetField`, the Props pipeline, and the round-trip contract wholesale. Instead, extend `parseFrontmatter` to detect a bare `key:` (empty value) followed by indented `- item` continuation lines, collect the items, and **synthesize the equivalent flow string** (`[item1, item2]`) into `n.frontmatter[key]`. This funnels block form through the *already-supported, already-tested* flow-form path: `aliases` via `parseAliases` (node.go:264-279 already handles `[a,b]`), plain props land in `Props` as the same `[a, b]` string flow-form produces today, and ref-list props feed the multi-target parser from (e). Block-list keys get **no** `fieldSpan` (SetField cleanly refuses — editing a multi-line value is already out of the byte-preservation keystone's scope). Detection is conditional on at least one `- ` continuation, so the "empty block list" case (`aliases:` immediately followed by another key or `---`) keeps today's behavior (empty scalar → `Aliases == nil`).

**(c) CRLF → PARSE IT, via CRLF-aware scanning inside `parseFrontmatter` (NOT global normalization).**
Reject normalizing a working copy. Even though the ticket permits it ("normalize a working copy used only for parsing"), a normalized buffer breaks the span↔`Raw` alignment that everything downstream depends on: `fieldSpans` and `bodySpan` are offsets into `Raw`, and `n.Body = raw[bodyStart:]` (node.go:125) and `SetField`'s splice both index `Raw` directly. Offsets computed against a `\r`-stripped copy would mis-splice CRLF files. CRLF-aware scanning is *both* more surgical *and* keeps every span a valid `Raw` offset, so `SetField`/`Serialize` stay correct for free. Fix all three LF-only spots consistently:
- **Opening gate (node.go:165):** accept `---\n` (4-byte header) *or* `---\r\n` (5-byte header); set `pos`/`rest` past whichever matched.
- **Closing gate (node.go:169,175):** `bytes.Index(rest, "\n---")` already finds the delimiter in `\r\n---` (the `\n---` is a substring), so `closeAbs` is fine; but `afterClose` (node.go:175) points at `\r` for CRLF, so the `!= '\n'` check must also accept `\r\n` and set `bodyStart` to skip both bytes.
- **Per-line value extraction (node.go:181-197):** after locating the `\n`, if the byte before it is `\r`, treat the line content (and therefore the value span) as ending *before* the `\r`. This strips the stray `\r` from captured scalar values while keeping the span a correct `Raw` offset (the `\r` stays physically in `Raw`, untouched).

`extractBody` needs **no** CRLF change — it splits on `\n`, `fenceRe` runs on `strings.TrimSpace(line)` (strips `\r`), `blockAnchorRe`'s `\s*$` already matches a trailing `\r`, and `wikilinkRe` is `\r`-agnostic (this is why "CRLF-only-in-body already works today"). Because each delimiter check is independently EOL-lenient, mixed-EOL files (Syncthing/multi-tool edits) also parse rather than falling into the empty-fields hole — lenient handling is chosen over rejecting mixed EOL (rejecting would be more surprising and re-introduce silent field loss).

**(d) Inline-code-inert → FIX (no escape hatch).**
Add per-line inline-code masking in `extractBody` (node.go:241-260), run *before both* `wikilinkRe` (node.go:253) and `blockAnchorRe` (node.go:256) so a `^blockid` inside a code span is inert too (consistent with fenced behavior). Reference `internal/parser/links.go`'s `removeCodeBlocks` for shape, but adapt to the per-line model and improve on it: `links.go` uses `` `[^`]+` `` which mishandles the double-backtick-with-nested-tick case in the AC. Implement `maskInlineCode(line string) string`: scan for a run of N backticks; find the next run of exactly N backticks; replace that whole span (inclusive) with equal-length spaces (length-preserving mask keeps `blockAnchorRe`'s end-anchor and `\s` boundaries intact and avoids shifting anything). An unterminated backtick run (no matching closer before EOL) is left literal — so a stray `` ` `` doesn't blind a real `[[link]]` later on the line (AC edge case). Length-preserving masking means a real link immediately adjacent to a code span (`` `code` and [[real]] ``) is untouched. Full CommonMark backtick edge cases beyond variable-length-run matching, and indented (4-space) code blocks, are explicitly out of scope (documented in AGENTS.md).

**(e) Multi-target ref props → PARSE IT (multiple Links), + Render round-trip fix.**
The current anchored `refValRe = ^\[\[(.+?)\]\]$` (node.go:102) doesn't "drop" `depends: [[A]], [[B]]` — the anchoring forces the lazy group to swallow the middle, fabricating one garbage `Link{To: "A]], [[B"}` in the `edges` table. Replace `parseRefValue` with `parseRefValues(raw) ([]ref, ok)`:
1. Strip optional outer quotes (as today).
2. Extract all `wikilinkRe` matches (reuse the existing `\[\[([^\]]+)\]\]`).
3. **Guard against over-eager linkification:** remove the matched tokens from the value; treat it as a clean ref list only if the remainder consists solely of separators `{ '[', ']', ',', ' ', '\t', '"' }`. This makes `[[A]], [[B]]`, `[[A]],[[B]]`, and a flow/synthesized `["[[A]]", "[[B]]"]` (the (b) block-form-of-refs path) all parse to N refs, while a non-clean value like `[[A]], not-a-link` returns `ok=false` and falls through to `Props` — **no garbage edge is ever fabricated**, which is the mandatory floor regardless of branch. `splitRef` (node.go:304-313) is reused per token for `#frag`/`|label`.
4. `deriveView` (node.go:222-235) loops `parseRefValues` results and appends one `Link{Rel: k}` per target.

Single-target `depends: "[[X]]"` still yields exactly one link (one token, empty remainder) — regression-safe, `TestCanonicalView` stays green.

**Render round-trip (render.go):** parsing multi-target does **not** affect `TestRoundTripIdentity` (that gate uses `Serialize` → `Raw`, unaffected). But `Render` (render.go:76-82) emits one line per link, so a node holding two same-rel links would render duplicate keys that don't reparse (last-wins). Close this by grouping `typed` links by `Rel` in `Render` and emitting a rel with >1 link as one comma-joined line (`rel: "[[A]]", "[[B]]"`, quoted per-token — matches what `parseRefValues` accepts). Single-link rels keep the exact current `rel: "[[to]]"` output, so `TestCreateRoundTrip`/`render_test.go` stay byte-identical. This closes the AC-flagged latent gap; see Risks for the Obsidian-YAML-validity caveat. (Deferring the Render change is defensible — no current caller constructs multi-target links before rendering — but it's small and closes the loop, so it should be done now.)

**(a) Doc location → create `internal/node/AGENTS.md`.**
Every sibling package uses AGENTS.md (`internal/{cli,index,journal,storage,tui}/AGENTS.md`); `internal/node` is a gap. Create it (not README, not package-comment-only), mirroring `internal/index/AGENTS.md`'s structure (Overview / key files / conventions+pitfalls / testing), enumerating the *shipped* subset after this ticket lands.

---

## Files to create / modify

**`internal/node/node.go` (modify) — the core fix.**
- `parseFrontmatter` (164-209): CRLF-aware opening/closing/line-extraction (decision c); block-list continuation detection + flow synthesis + skip fieldSpan for list keys (decision b).
- New `maskInlineCode` helper + call site in `extractBody` (241-260) before `wikilinkRe`/`blockAnchorRe` (decision d).
- Replace `parseRefValue`→`parseRefValues` (283-294) and update the `deriveView` ref branch (222-235) to fan out to multiple Links; drop/replace the anchored `refValRe` (102) — the guard replaces the need for anchoring (decision e).
- Tighten the `fmScalarRe` doc comment (94-97) and `parseFrontmatter` doc (161-163) to reference AGENTS.md for the authoritative subset.

**`internal/node/render.go` (modify) — multi-target round-trip.**
- Group `typed` links by `Rel` (76-82); emit >1-link rels as one comma-joined quoted line; single-link output unchanged.

**`internal/node/AGENTS.md` (create) — deliverable (a).**
Enumerate: frontmatter delimiters (`---`, LF **and** CRLF, mixed-EOL lenient); scalar `key: value` per line; flow lists `[a, b]` **and** block lists (`key:\n  - a`) — with the note that list items with embedded commas/brackets and nested maps / `|`/`>` block scalars / flow maps / multi-doc are **out of scope**; aliases (scalar or list); ref-valued props single- **and** multi-target (`[[A]], [[B]]`); wikilink forms (`[[t]]`, `[[t|label]]`, `[[t#Heading]]`, `[[t#^block]]`, `![[embed]]`); wikilinks inert in fenced **and** inline code (indented code blocks NOT inert — documented non-goal); block-anchor `^id`; the byte-preservation/span-splice invariant and why SetField is scalar-keys-only. State each named gap's disposition explicitly (all four "handled", not "warned").

**`internal/node/node_test.go` (modify) — new fixtures + targeted tests (below).**

**`internal/node/render_test.go` (modify) — one multi-target render round-trip test.**

No changes to `internal/index/*`, `internal/parser/*`, or `schema.go`. `Links` already flows 1-row-per-link into `edges` via `insertNode`, so multi-target Just Works downstream.

---

## Test scenarios (traceable to AC §4)

Extend `roundtripCorpus` (node_test.go:84-89) with four new entries so `TestRoundTripIdentity` + `FuzzRoundTripIdentity` prove **Raw-safety holds for the new shapes** (Serialize==input, byte-for-byte — the strongest evidence the derived-view fixes don't touch Raw): `blockAliases`, `crlfNote` (Go string with literal `\r\n`), `inlineCodeNote`, `multiTargetDepends`. Add a CRLF and a block-scalar seed to `FuzzRoundTripIdentity` (299-307).

Targeted tests (mirroring the `TestStructuredViewIgnoresFences`/`TestCanonicalView`/`TestAliasParsing` style):
- **`TestBlockScalarAliases`** (AC b): Obsidian-shaped block `aliases:` + flow `tags` in one block → `Aliases == {project-x, proj-x}`, `Props["tags"]` unaffected (sibling-key regression); single-item block list; empty block list → `nil` (regression).
- **`TestCRLFFrontmatter`** (AC c): whole-file CRLF → `ULID`/`Type`/`Aliases` populate, no trailing `\r` in values, body `[[link]]` edge present, `SetField` still splices correctly; plus CRLF-only-in-body (LF frontmatter) still parses and preserves `\r\n` bytes in `Body` (regression).
- **`TestInlineCodeInert`** (AC d): `` `[[target]]` `` → no link; double-backtick nested `` ``x `y` z`` `` inert; adjacent real `[[real-target]]` still linkified; fenced `noteObsidian` regression stays green.
- **`TestMultiTargetRefProp`** (AC e): two and three targets → N same-rel links; `[[A]],[[B]]` / `[[A]], [[B]]` / `[[A]] ,  [[B]]` all equal; per-token `#frag`/`|label`; single-target regression; mixed `[[A]], not-a-link` → **no garbage `Link`** (falls to Props).
- **`TestMultiTargetRenderRoundTrip`** (render.go): node with two same-rel links → `Render` → `Parse` reproduces both links (guards the round-trip fix); confirm single-link output still `rel: "[[to]]"`.

Regression gate (must stay green, per AC §2): the existing `roundtripCorpus`, `TestStructuredViewIgnoresFences`, `TestCanonicalView`, `TestAliasParsing`, `TestSpanLocalEditIsSurgical`, and all of `render_test.go`.

Fixtures should resemble real Obsidian Properties-panel output (block-style `aliases`/`tags`, CRLF from a Windows/Syncthing note) per the ticket's "same vault edited by Obsidian" justification.

---

## Known risks / ambiguities

- **Round-trip for anything: unsupported shapes must still not corrupt on read — yes.** Because all four gaps are "handled," no new warning channel is added; but the governing principle stands and should be stated in AGENTS.md: `Serialize` stays byte-identical for *every* input (Raw-preservation), and the (e) fix removes the one place that fabricated bad derived data. Genuinely-unsupported YAML (nested maps, `|`/`>` block scalars) remains *silently skipped* (status quo) — not corrupted, just invisible. Adding a runtime warning for those would require threading a warnings slice out of `node.Parse` (new plumbing); this should be **deferred** (out of the named four gaps, expands blast radius) and instead documented as an explicit boundary in AGENTS.md. Flag for reviewer sign-off.
- **Render multi-target form vs Obsidian YAML validity.** Emitting `rel: "[[A]]", "[[B]]"` round-trips through *our* parser cleanly but is not strictly valid YAML flow syntax, so Obsidian's own YAML reader may not interpret it as a list. This only matters on the create/import path (parsing existing files never calls Render), and no current caller constructs multi-target links pre-render, so it's latent. Verify the emitted form against a live Obsidian vault, or keep single-target as the only create-path shape and treat multi-target as parse-only (documented) — decide during review.
- **Flow-synthesis fragility (b).** Re-joining block-list items into `[a, b]` breaks for items containing commas/brackets. Out of scope (general YAML) and documented; the common Obsidian cases (aliases, tags, wikilinks, simple scalars) are covered.
- **Obsidian ground-truth (AC §4 caveat).** Test fixtures are based on documented Obsidian Properties-panel conventions, not a captured export; confirm block-vs-flow default against a current Obsidian version during implementation.
- **Indented (4-space) code blocks and full CommonMark backtick rules** are explicit non-goals for (d) — must be named as such in AGENTS.md so the "markdown subset" boundary is unambiguous.

## Critical files for implementation

- `internal/node/node.go`
- `internal/node/render.go`
- `internal/node/node_test.go`
- `internal/node/render_test.go`
- `internal/node/AGENTS.md` (new)
