# Node Subsystem Guide

## Overview

`internal/node` is the canonical-node keystone: one byte-preserving
representation of a markdown file, derived into typed fields on `Parse` and
serialized back out verbatim (`Serialize() == Raw`) unless a caller performs a
surgical `SetField` splice. Everything the v1 index (`internal/index`) and
tools built on it (`rk index`, `rk query`, T4 `rk log`) see comes from this
package's parser — **not** the legacy v0 `internal/parser` package, which
serves only the older `internal/cli/notes.go` / `internal/service` path.

**Key files:**
- `node.go` — `Node`, `Parse`/`ParseAt`, `parseFrontmatter`, `deriveView`,
  `extractBody`, `SetField`, group-file `SplitEntries`/`ReplaceEntryBody`.
- `render.go` — `Render`/`NewNode`, the create-path inverse of `deriveView`.
- `parser.go` — `Parser` interface + `MarkdownParser` (per-tool adapter).

## The byte-preservation invariant

`Raw` is authoritative. `Serialize()` returns it unmodified; every typed field
(`ULID`, `Type`, `Time`, `Author`, `Body`, `Aliases`, `Props`, `Links`,
`Fragments`) is a *read-only derived projection* computed by `deriveView` and
`extractBody` from `Raw`. `SetField` is the one write path: it splices a new
value into a previously-recorded byte `Span` and re-parses the spliced bytes —
it never regenerates a file from the typed model. This is why `parseFrontmatter`
tracks an exact `Span` per scalar key (`fieldSpans`) instead of just a string
value: the span is what makes an edit surgical (only that field's bytes
change) instead of a lossy round-trip through a generic serializer.

`SetField` only works on a previously-recorded scalar span. Any value with no
single contiguous byte span in `Raw` — a synthesized block-list value (see
below) — has no entry in `fieldSpans`, so `SetField` on that key correctly
returns an error rather than attempting an edit with nowhere to splice.

## Supported frontmatter/markdown subset (shipped, this ticket: reckon-vj55)

**Frontmatter delimiters:** a leading `---` line and a matching `---` line,
each terminated by `\n` **or** `\r\n`, independently detected at the open,
close, and per-line-value-extraction points — so a whole-file-CRLF note (a
Windows/Syncthing-authored file arriving via a synced vault) parses exactly
like its LF equivalent, and a mixed-EOL file (frontmatter one style, body
another, or vice versa) is tolerated leniently rather than rejected. No
`\r` byte is ever stripped from `Raw` — only the *string values* handed to
typed fields have a trailing `\r` trimmed; the recorded `Span` still points at
the exact bytes in `Raw` (the `\r` stays there, physically).

**Scalar values:** one `key: value` per line (`fmScalarRe`). This is the only
form with a byte span, hence the only form `SetField` can edit.

**Flow-style lists:** `key: [a, b, c]` (comma-split, whitespace-trimmed).
Already worked before this ticket for `aliases` (`parseAliases`) and now for
ref-valued props too (`parseRefValues`, see below).

**Block-style lists:** an Obsidian Properties-panel-shaped
```
key:
  - a
  - b
```
(a bare `key:` with an empty value, followed by one or more indented `- item`
lines) is detected (`scanBlockList`) and synthesized into the equivalent flow
string (`"[a, b]"`) *before* it reaches any downstream logic — so it takes the
identical code path as a hand-written flow list (`parseAliases` for `aliases`,
`parseRefValues` for ref-valued props, plain `Props` for everything else). A
block key gets **no** `fieldSpan` (see invariant above). An empty block list
(`key:` immediately followed by another key or the closing `---`, no `- `
lines) is a no-op — same as an absent key, not an error. A sibling flat/flow
key elsewhere in the same frontmatter block is never disturbed.

A block-list item containing a literal comma or bracket (e.g. `  - a, b`) is
a known, documented limitation of the flow-string synthesis: it re-joins as
`"[a, b, item2]"`, silently mis-splitting into extra items rather than
preserving `a, b` as one item. This does not corrupt `Raw` — the derived view
is merely wrong for that shape — and is not fixed here; avoid literal commas
or brackets inside block-list item text.

Out of scope, silently un-derived (not corrupted — just invisible, same as
today's status quo for anything not on this list): nested maps
(`key:\n  sub: val`), YAML flow maps (`{a: 1}`), `|`/`>` block scalars,
YAML anchors/aliases (`&`/`*`), multi-document files. A general YAML parser is
explicitly not what this package implements — swapping in `yaml.v3` was
considered and rejected because it gives no byte spans and applies YAML type
coercion, which would break `SetField`, `Props`, and the round-trip contract.

**Wikilink forms** (body and ref-valued frontmatter props):
`[[target]]`, `[[target|label]]`, `[[target#Heading]]`, `[[target#^block]]`,
`![[embed]]` (the leading `!` is just ordinary text preceding the match).
`splitRef` strips the optional `|label` then `#fragment`/`#^block` suffix.

**Wikilinks are inert inside:**
- fenced code blocks (` ``` `/`~~~`, toggled per line by `fenceRe`) —
  supported before this ticket.
- inline code spans (single- or double-backtick, `` ` ``/`` `` ``), including a
  double-backtick span containing a nested single backtick
  (`` ``a `b` c`` ``) — `maskInlineCode` replaces matched-run-length backtick
  spans with equal-length spaces per body line, before `wikilinkRe`/
  `blockAnchorRe` run, so adjacent non-code content on the same line (e.g. a
  real link right after a code span closes) is untouched. An unterminated
  backtick run (no matching same-length closer before end of line) is left as
  literal text — it does not blind a real link later on the line.

Explicitly **not** inert: indented (4-space/tab) code blocks — full CommonMark
code-block detection is out of scope; only fenced and inline-backtick forms
are recognized.

**Ref-valued frontmatter props** (a non-reserved key whose value is a clean
wikilink reference, or list of references) become typed `Link`s with
`Rel = <the key>` instead of landing in `Props`:
- Single target: `depends: "[[X]]"` (optionally double-quoted) → one `Link`.
- Multi-target: `depends: [[A]], [[B]]` (any comma/space spacing, e.g.
  `[[A]],[[B]]` / `[[A]], [[B]]` / `[[A]] ,  [[B]]` all agree) → one `Link` per
  target, same `Rel`, in source order. Each target may carry its own
  `#fragment`/`|label` suffix (`splitRef` applied per token).
- **The floor, regardless of shape:** `parseRefValues` only returns `ok=true`
  if, after removing every matched `[[...]]` token, the remainder is composed
  solely of separator characters (`[`, `]`, `,`, space, tab, `"`). A value like
  `depends: [[A]], not-a-link` is *not* a clean ref list — it returns
  `ok=false` and falls straight through to `Props` as a plain string. No
  garbage/partial `Link` is ever fabricated from an unclean value.

**Body links** (`rel = "references"`) are derived line-by-line from the body
text (post fence/inline-code masking); block anchors (`^id` at end of a
non-code line) become `Fragment`s.

## Render (create path)

`Render` is the inverse of `deriveView`+`extractBody` for a node built from
typed fields only (no authoritative `Raw` yet — a create or import). Typed
links are grouped by `Rel` (already sorted by `Rel, To`, so same-`Rel` links
are contiguous): a `Rel` with exactly one `Link` renders as today's single
`rel: "[[to#frag]]"` line; a `Rel` with more than one `Link` renders as **one**
comma-joined, per-token-quoted line (`rel: "[[a]]", "[[b]]"`) so it survives a
`Render` → `Parse` round trip as two links via `parseRefValues`, instead of
silently collapsing to one via duplicate frontmatter keys (`parseFrontmatter`
overwrites `n.frontmatter[key]` on each occurrence — last line wins).

**Known caveat:** the multi-target comma-joined form round-trips cleanly
through *this* package's parser but is not strictly valid YAML flow syntax, so
an external YAML-strict reader (e.g. Obsidian's own frontmatter parser) may
not interpret it as a list. This only matters on the create/import path
(parsing an existing vault file never calls `Render`); no current caller
constructs multi-target links before rendering, so it's latent. Worth
verifying against a live Obsidian vault before a caller starts doing so.

## Conventions / pitfalls

- Every parse fix in this package changes how the typed view is *derived* from
  `Raw` — never `Raw` itself. Any change to `parseFrontmatter`'s scanning loop,
  `extractBody`'s per-line processing, or the ref-value parser must be checked
  against `TestRoundTripIdentity` + `FuzzRoundTripIdentity` (byte-identical
  `Serialize` for every input `Parse` accepts) before anything else.
- Git-conflict-marker files are refused outright (`ParseAt` returns an error) —
  never silently "fixed."
- `fieldSpans` are the edit surface; a key with no span (block-list-derived,
  or simply absent) is read-only via `SetField` by construction, not by a
  special-cased error check.

## Testing

`node_test.go`'s `roundtripCorpus` is the adversarial fixture set (real-shaped
Obsidian/Logseq/agent input, including block-style `aliases`, whole-file CRLF,
inline-code, and multi-target-ref fixtures) driving both
`TestRoundTripIdentity` and `FuzzRoundTripIdentity`. Targeted tests:
`TestBlockScalarAliases`, `TestCRLFFrontmatter`, `TestInlineCodeInert`,
`TestMultiTargetRefProp` (node_test.go) and `TestMultiTargetRenderRoundTrip`
(render_test.go).
