# reckon-vj55 — codebase analysis

Ticket: "Review M-parser-scope: enumerate supported frontmatter/markdown subset +
real-Obsidian tests" (from foundation-review-2026-06-24, disposition item
"M-parser-scope"). Blocks `reckon-uv09` (T4 log tool); should land before T8
(linking).

Scope confirmed: the DERIVED view built by `internal/node` (consumed by
`internal/index` for the SQLite projection) diverges from real Obsidian-authored
files in four ways. All four were reproduced below with either direct source
reading or a standalone Go scratch program exercising the actual regexes used in
`internal/node/node.go`.

The relevant parse path for the index is exclusively **`internal/node`**
(`node.Parse` → `node.MarkdownParser.Parse` → `internal/index/reconcile.go`'s
`indexFile`/`insertNode`). There is a second, architecturally unrelated wikilink
implementation in `internal/parser/links.go`, used only by the pre-redesign
`internal/service/notes_service.go` (old `models.Note`/`NotesRepository` stack).
It is NOT in the index's parse path — see finding 3 below for why this matters
(it already does the thing `internal/node` doesn't).

---

## 1. Block-style YAML frontmatter is invisible

**File:** `internal/node/node.go`

Frontmatter parsing is **hand-rolled, line-by-line, single-line-scalar-only** —
not `gopkg.in/yaml.v3` (which IS already a project dependency — `go.mod:19`,
used elsewhere for e.g. `internal/journal/task_parser.go` — just not here).

The doc comment on the regex says this is intentional, not an oversight
(node.go:94-97):

```go
// A scalar frontmatter line: key, the gap after the colon, then the value.
// Captures so we can compute the value's exact byte span. Single-line scalar
// values only (no block scalars, no nested maps) — matches the proven spike.
fmScalarRe = regexp.MustCompile(`^([A-Za-z0-9_-]+):([ \t]*)(.*?)([ \t]*)$`)
```

`parseFrontmatter` (node.go:164-209) applies `fmScalarRe` **per line** inside the
`---`/`---` block. Each line must match `^key: value$` from the start of the
line. A block-sequence continuation line like `  - a` starts with whitespace,
which is not in the key character class `[A-Za-z0-9_-]`, so the regex simply
does not match — and the `if m := fmScalarRe.FindStringSubmatchIndex(line); m != nil`
guard (node.go:189) means **no branch runs for that line at all**: no error, no
key registered, nothing appended to `n.frontmatter`/`n.fmOrder`. The loop just
advances `pos` to the next line (node.go:199-202).

Verified directly (scratch program against the literal `fmScalarRe`):

```
line="aliases:"  match=[aliases: aliases   ]      // key="aliases", value=""
line="  - a"     match=[]                          // no match at all — silently skipped
line="  - b"     match=[]                          // same
line="type: note" match=[type: note type   note ]  // subsequent lines parse fine
```

So for:
```yaml
---
id: 123
aliases:
  - a
  - b
type: note
---
```
`n.frontmatter["aliases"]` ends up as `""` (from the bare `aliases:` line), and
the two list items are dropped with **zero signal** — not an error, not a
warning, not even a truncated value. `deriveView` (node.go:215-236) then calls
`parseAliases(n.frontmatter["aliases"])` (node.go:264-279); since the value is
empty after `TrimSpace`, `parseAliases` returns `nil` (node.go:266-268), so
`n.Aliases` is `nil` — the alias list is entirely invisible. The same happens
for any non-reserved key with a block list value (e.g. `tags:\n  - a\n  - b`) —
it lands in `Props` as an empty string, again silently.

Note `parseAliases` DOES already handle the Obsidian **flow-sequence** form
(`aliases: [a, b]`, node.go:269-277) — only the far more common Obsidian-UI-
generated **block-sequence** form is unhandled. Obsidian's built-in "Properties"
editor writes multi-value list properties (aliases, tags, and any user list
property) in block-scalar form by default, which is exactly the shape this
ticket is about.

---

## 2. CRLF files fail the `---\n` prefix check

**File:** `internal/node/node.go:164-166`

```go
func parseFrontmatter(n *Node, raw []byte) (bodyStart int) {
	if !bytes.HasPrefix(raw, []byte("---\n")) {
		return 0
	}
```

This is the only "does this file have frontmatter at all" gate. Confirmed with
a scratch program on `raw := []byte("---\r\nid: 123\r\n---\r\nbody\r\n")`:

```
HasPrefix(raw, "---\n") = false
first 5 bytes: "---\r\n"
```

`raw[3]` is `\r`, not `\n`, so `HasPrefix` is false and `parseFrontmatter`
returns `0` immediately — **the entire file, frontmatter block included, is
treated as body**. Downstream in `ParseAt` (node.go:123-127), `bodyStart = 0`
means `n.Body` is the whole raw file (frontmatter text and all), and
`deriveView` (node.go:216-220) reads every typed field from the now-empty
`n.frontmatter` map: `ULID`, `Type`, `Time`, `Author` all become `""`,
`Aliases` becomes `nil`, and there are no `Props`/ref-valued `Links` from
frontmatter at all. This is exactly the "all typed fields go empty" behavior
the ticket describes — confirmed, not hypothetical.

Downstream effect in the index: `internal/index/reconcile.go`'s `nodeKey`
(reconcile.go:257-267) falls back to a path-derived surrogate key
(`n.ULID == ""`), so the file gets indexed under a synthetic identity rather
than its real ULID — silently breaking ULID-based rename-stability and any
`id:`-based lookups/edges for every CRLF file. There is no error surfaced
anywhere in this path; `os.ReadFile` in `reconcile.go:167` reads raw bytes
verbatim, with no CRLF normalization anywhere upstream of `node.Parse`.

Also worth noting for the eventual fix: this is not a single-line bug. Even if
the opening `HasPrefix` check were made CRLF-tolerant, the closing-delimiter
scan (`bytes.Index(rest, []byte("\n---"))`, node.go:169) and the
`afterClose < len(raw) && raw[afterClose] != '\n'` check (node.go:175) are
*also* LF-only — a CRLF closing line (`\r\n---\r\n`) would fail the `!= '\n'`
check the same way. And per-field line values are extracted via
`bytes.IndexByte(raw[pos:closeAbs], '\n')` (node.go:181) which splits only on
bare `\n`, so a captured line for a CRLF file still contains a trailing `\r` —
`fmScalarRe`'s trailing `[ \t]*` trim group does not strip `\r` (not in that
character class), so even a hypothetically-fixed boundary check would leave
every scalar value with a stray trailing `\r` byte (e.g. `id` would parse as
`"123\r"` instead of `"123"`). A correct fix needs to handle line-ending
normalization (or CRLF-aware scanning) consistently through the whole function,
not just the opening guard.

`internal/node/insert.go` (mentioned in the ticket as a sibling `HasPrefix`
site from ticket reckon-9bfx) **does not exist on this branch** — confirmed via
`ls internal/node/`. That ticket's PR is not merged into this worktree's base;
`node.go`'s `parseFrontmatter` is the only site to fix here.

---

## 3. Inline code `[[x]]` is linkified into spurious reference edges

**File:** `internal/node/node.go`, function `extractBody` (node.go:241-260) —
this is what actually populates `references` edges consumed by the index (via
`Node.Links`, `Envelope.Links`, `internal/index/reconcile.go`'s `insertNode`).

```go
func extractBody(n *Node, raw []byte, bodyStart int) {
	body := raw[bodyStart:]
	inFence := false
	for _, lineB := range bytes.Split(body, []byte("\n")) {
		line := string(lineB)
		if fenceRe.MatchString(strings.TrimSpace(line)) {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		for _, lm := range wikilinkRe.FindAllStringSubmatch(line, -1) {
			n.Links = append(n.Links, parseBodyLink(lm[1]))
		}
		...
```

`fenceRe = regexp.MustCompile("^(```|~~~)")` (node.go:101) toggles a per-line
"inside a fenced code block" flag and correctly makes *fenced* blocks inert —
this is tested (`TestStructuredViewIgnoresFences` in `node_test.go:108-144`,
using the `noteObsidian` corpus entry's ` ```go ... [[notalink]] ... ``` ` fence).

But `wikilinkRe = regexp.MustCompile(`\[\[([^\]]+)\]\]`)` (node.go:99) is run
against the **raw line string** with no awareness of inline code spans
(single backtick pairs) at all. A line like:

```
Use `[[x]]` for linking, not `[[y]]`.
```

is not inside a fence (`inFence` is false), so both `` `[[x]]` `` and
`` `[[y]]` `` get matched by `wikilinkRe` and turned into `references` edges
via `parseBodyLink` (node.go:296-300) — `x` and `y` become real (spurious)
link targets in the index, exactly as the ticket describes. There is no
`removeCodeBlocks`-style pass in `internal/node` at all.

**Contrast:** `internal/parser/links.go` (the OLD, architecturally separate
wikilink extractor used only by `internal/service/notes_service.go`, not by
the index) already solves this correctly:

```go
// codeBlockPattern matches fenced code blocks with optional language specifier.
var codeBlockPattern = regexp.MustCompile("(?s)```[^`]*```")
// inlineCodePattern matches inline code spans.
var inlineCodePattern = regexp.MustCompile("`[^`]+`")

func removeCodeBlocks(content string) string {
	content = codeBlockPattern.ReplaceAllString(content, "")
	content = inlineCodePattern.ReplaceAllString(content, "")
	return content
}
```

(`internal/parser/links.go:24-27, 143-153`), called from `ExtractWikiLinks`
before matching `wikiLinkPattern`. This is a good reference for the *shape* of
a fix (strip/mask inline code before scanning) but it's a regex-based
whole-content pass, not line-oriented like `node.go`'s fence-tracking loop, so
it can't be copy-pasted verbatim — `extractBody` would need equivalent
line-local inline-code masking (e.g. strip balanced `` `...` `` spans per line,
being careful the mask/strip doesn't shift byte offsets that other consumers
of `line` rely on — `blockAnchorRe` is also matched against the same `line`
at node.go:256, so an inline-code-masking pass would need to run before both
matchers or otherwise not blind block-anchor detection at end-of-line).

---

## 4. Multi-target ref props are dropped (mangled, not cleanly dropped)

**File:** `internal/node/node.go`, `deriveView` (node.go:215-236) and
`parseRefValue` (node.go:283-294).

```go
refValRe = regexp.MustCompile(`^\[\[(.+?)\]\]$`)
...
func parseRefValue(raw string) (to, frag string, ok bool) {
	s := strings.TrimSpace(raw)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	m := refValRe.FindStringSubmatch(s)
	if m == nil {
		return "", "", false
	}
	to, frag = splitRef(m[1])
	return to, frag, true
}
```

The existing test corpus (`node_test.go`, `todoItem`) only covers the
**single-target quoted** form: `depends: "[[01J9Z2QH8M]]"`.

For `depends: [[A]], [[B]]`, `parseRefValue` is called with
`raw = "[[A]], [[B]]"` (the whole value captured by `fmScalarRe` for that
line). `refValRe` is anchored `^...$`, i.e. it must match the *entire* value
string. Verified with a scratch program:

```
val="[[A]], [[B]]"
match=[[[A]], [[B]] A]], [[B]
captured group 1 = "A]], [[B"
```

Because `(.+?)` is non-greedy but the pattern is fully anchored (`^` and `$`
over the whole string, no multiline flag), Go's RE2 engine is forced to
consume up through the *last* `]]` in the string to satisfy `$` — the
"lazy" quantifier has no effect here since the match must span start to end.
The result is **not** "drops the second target" as the ticket description's
plain-English gloss suggests — it's worse: the two brackets get **merged into
one garbled link target**, `To = "A]], [[B"` (after `splitRef` finds no `|`/`#`
in it, so the mangled string passes through unchanged). This single malformed
`Link{Rel: "depends", To: "A]], [[B"}` is what ends up in `n.Links` and thus in
the index's `edges` table — a nonsense `dst` that will never resolve to
either real target, and will silently show up as a dangling edge with a
corrupted-looking value if anyone inspects it.

There is no code path that recognizes "this frontmatter value contains more
than one bracketed target" and splits it into two `Link`s. The whole
`deriveView` per-key model (node.go:222-235) assumes one prop key → one
`Props` value OR one `Link`, never a fan-out to multiple links from a single
key.

Design-doc context (`docs/design/composable-redesign.md:946-949`): the spec as
currently written only documents the **single**-target form
(`` depends: "[[ULID]]" `` → one `depends-on` edge); multi-target ref props are
not an explicitly specified feature at all. That supports the ticket's "handled
or explicitly+loudly unsupported" framing — the fix could legitimately be
"detect this shape and refuse/warn" rather than "parse both targets," but
*silently mangling into one bogus link* is the one option that's clearly
wrong and needs to change either way.

---

## 5. Existing "node package docs"

**There is no `internal/node/AGENTS.md`.** Every sibling package that has an
`AGENTS.md` does (`internal/cli/AGENTS.md`, `internal/index/AGENTS.md`,
`internal/journal/AGENTS.md`, `internal/storage/AGENTS.md`,
`internal/tui/AGENTS.md`) — `internal/node` and `internal/parser` are the two
exceptions in the package tree that matters here. `internal/parser/README.md`
exists (it documents the *old* `ExtractWikiLinks`/`NormalizeSlug` API, not
`internal/node`'s wikilink handling — see finding 3; it's accurate for its own
narrow scope but irrelevant to the index path).

What currently documents the supported subset for `internal/node` is only the
package doc comment at the top of `node.go:1-28`, which is architecture/intent
prose (byte-preservation, span-splice edits, link routing spec invariant 3,
FORMAT COUPLING note) — useful and accurate as far as it goes, but it does
**not enumerate the supported frontmatter/markdown subset** the ticket wants
(e.g. "frontmatter must be `---\n`-delimited with LF-only line endings;
scalar `key: value` per line only, no block scalars/nested maps; aliases as
either a bare scalar or a flow-sequence `[a, b]`; ref-valued props must be a
single bracketed target, optionally double-quoted; wikilinks recognized in
body text only, fenced code blocks inert, inline code NOT currently inert
[bug]; ..."). `parseFrontmatter`'s doc comment (node.go:161-163) and the
`fmScalarRe` doc comment (node.go:94-97) each state one piece of the subset
inline, but there's no single enumerated reference a reader (or an Obsidian
user editing the vault) can check against.

`docs/design/composable-redesign.md` (search hits at lines 92-98, 362-372,
448-458, 583-587, 944-985) has the *design decisions* for link syntax/alias
namespace/round-trip gating at a higher level (this is where "Obsidian's
wikilink surface wholesale" was decided, `~944`), but it's a design-log
document, not the day-to-day subset reference the ticket wants living next to
the code. `docs/reckon-spec_2026-06-15.md` mentions frontmatter only in
passing (migration notes, journal projection). Neither is "node package docs"
in the sense the ticket means (something in `internal/node/`).

**Conclusion:** this ticket needs to *create* `internal/node/AGENTS.md` (or
substantially expand the `node.go` package doc comment) enumerating the
supported subset — there's no existing infrastructure to merely "fill in";
following the `internal/index/AGENTS.md` structure (Overview / key files /
concrete conventions-and-pitfalls / testing section) is a reasonable template
since it's the most relevant sibling doc and already cross-references
`internal/node`.

---

## 6. Existing tests and fixtures

**No `testdata/` directories anywhere in the repo** (`find . -type d -iname
testdata` returns nothing) — all existing parser tests use **inline Go string
literals** as fixtures, not golden files.

**Best template: `internal/node/node_test.go`** (this is almost certainly the
file new tests should extend). Structure:
- A **shared adversarial corpus** at the top (node_test.go:9-89):
  `noteObsidian`, `todoItem`, `logDay`, `weirdEdges`, `conflicted`, collected
  into `roundtripCorpus = map[string]string{...}` (node_test.go:84-89). Comment
  explicitly says this corpus is "carried verbatim from the gating spike
  ... the contract: any change to the parser must keep them green."
- Table-driven round-trip test over that map (`TestRoundTripIdentity`,
  node_test.go:92-104).
- Targeted structural tests reading specific corpus entries
  (`TestStructuredViewIgnoresFences`, `TestCanonicalView`,
  `TestAliasParsing`, node_test.go:108-199) — good style precedent for adding
  e.g. `TestBlockScalarAliases`, `TestCRLFFrontmatter`,
  `TestInlineCodeInert`, `TestMultiTargetRefProp`.
- A **fuzz test** seeded from the same corpus plus a few hand-picked edge
  inputs (`FuzzRoundTripIdentity`, node_test.go:299-329) — worth seeding with
  a CRLF and a block-scalar-list case too once the parser is fixed, so the
  fuzz gate covers the new supported shapes.

The corpus currently has **zero** coverage of: CRLF line endings, block-scalar
(list) frontmatter values, inline-code-wrapped wikilinks, or multi-target ref
props (`todoItem`'s `depends` is single-target quoted only). All four gaps
named in the ticket are confirmed absent from the test corpus, not just from
the implementation — this ticket needs genuinely new fixtures, not just
un-skipping something.

`internal/index/index_test.go` has a `multiNodeParser` test double
(index_test.go:84-100) that could be used for an index-level integration test
if the fix is meant to be verified end-to-end (edges/props actually landing
correctly in the SQLite views), but the unit-level fix and its tests belong in
`internal/node`.

---

## 7. docs/REVIEW_PATTERNS.md — relevant pitfalls

Skimmed the full 1025-line file. Nothing frontmatter/YAML/wikilink/regex-
parsing-specific has been logged yet (no prior review cycle hit this class of
bug). The generally-applicable patterns worth keeping in mind while
implementing this fix:

- **"Unwrapped Errors" anti-pattern** (REVIEW_PATTERNS.md:19-40) — very common
  finding (5/5 frequency); any new error path in the frontmatter fix should
  wrap with context (`fmt.Errorf("...: %w", err)`), consistent with
  `internal/index/AGENTS.md`'s existing convention ("Wrap every error with
  context ... no `os.Exit` in the library").
  applying: if the fix chooses "explicitly+loudly unsupported" for
  multi-target ref props (see finding 4) rather than parsing them, whatever
  signal is emitted (error from `Parse`, or a `Warning` in the reconcile pass
  the way `internal/index/reconcile.go`'s `Warning`/`Stats.Warnings` already
  works for duplicate-ULID/alias-collision — see `Warning` type,
  reconcile.go:30-39) should follow that established non-fatal-warning
  pattern rather than inventing a new one, if consistency with M3/M4
  (`reckon-5b44`, already landed) is desired.
- **time.Parse / local-vs-UTC pitfall** (REVIEW_PATTERNS.md:929-951,
  1003-1025) — not directly applicable (no date parsing involved here) but
  it's the file's one example of "silent wrong behavior" being called out as
  high-impact despite low frequency (🔴, 1 occurrence) — same category of risk
  as all four bugs in this ticket (silent wrong DERIVED data, no crash, no
  visible signal), worth citing as precedent for why "loudly unsupported" is
  the right bar rather than "best-effort quiet fallback."
- No entries yet exist under a "Parsing" category — if the eventual PR fixes
  these bugs, this file is the place a reviewer would expect a new pattern
  entry (e.g. "❌ Anti-Pattern: anchored regex silently merges/mangles
  multi-value input" for finding 4, or "hand-rolled line scanners must
  explicitly define line-ending behavior" for finding 2) to be added,
  per the file's own stated purpose ("Frequency tracking ... 2-3x = add to
  guides").

---

## Summary table

| # | Bug | Location | Mechanism | Silent or errors? |
|---|-----|----------|-----------|---|
| 1 | Block YAML list invisible | `node.go:97` `fmScalarRe`, `node.go:189` guard | Regex requires line to start with bare key char class; indented `- item` lines never match, silently skipped | Silent — value becomes `""`, list items vanish |
| 2 | CRLF fails `---` detection | `node.go:165` `bytes.HasPrefix(raw, []byte("---\n"))`, also `node.go:169,175` closing check, `node.go:181` line split | `\r` before `\n` breaks all three LF-only checks; also every field's trailing `\r` isn't trimmed by `fmScalarRe`'s `[ \t]*` group | Silent — whole file becomes body-only, all typed fields empty/zero |
| 3 | Inline code `[[x]]` linkified | `node.go:99` `wikilinkRe`, `node.go:253` loop in `extractBody` | Only fenced blocks tracked (`inFence`/`fenceRe`); no inline-backtick masking before the wikilink scan | Silent — spurious `references` edges land in the index |
| 4 | Multi-target ref prop | `node.go:102` `refValRe`, `node.go:283-294` `parseRefValue` | Anchored `^...$` regex over the whole value forces the non-greedy group to swallow the middle, merging two bracket pairs into one garbled target | Silent — not "dropped," actually mangled into one bogus `Link` |

All four are silent-corruption bugs (no error returned, no warning emitted) —
consistent with the ticket's framing that the raw bytes stay safe
(`Serialize()` round-trips fine) but the *derived* view — and therefore
everything downstream in `internal/index` — is wrong without any signal.
