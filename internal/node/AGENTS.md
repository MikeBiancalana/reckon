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
- `logparser.go` — `LogParser` (v1-T4), the group-file parser for the log
  tool (`rk add`, `internal/cli/add.go`); see "Group files: LogParser and
  the `id::` marker" below.

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

## Group files: LogParser and the `id::` marker (v1-T4)

A "log-day" file (frontmatter `type: log-day`) is a *group file*: one day's
preamble/frontmatter followed by N `## HH:MM [kind] · author` entry blocks,
each optionally carrying an inline `id:: <ULID>` line as its first body line
(the log tool's, `rk add`'s, per-entry-identity marker — composable-
redesign.md's locked choice, distinct from the day node's own frontmatter
`id:` — note the double colon). `LogParser` (`logparser.go`) is the `Parser`
implementation that understands this shape:

- `LogParser.Parse` runs `ParseAt` once; if the resulting node's `Type` is
  not `"log-day"`, it returns `[]*Node{day}` unchanged — byte-identical to
  `MarkdownParser`'s behavior for every other node type (notes, todos,
  anything). Dispatch is on parsed frontmatter `type`, not on path, so it is
  location-independent.
- For a `log-day` node it additionally calls `(*Node).SplitEntries()` and
  builds one `log-entry` *Node* per block (`buildLogEntry`): `ULID` from the
  entry's `id:: <ULID>` line (dropped from `Body`; entries with no `id::`
  line get `ULID == ""`, surrogate-keyed at the index level); `Time`
  reconstructed as `<dayDate>T<HH:MM>:00Z` (dayDate from the day node's
  first alias, else derived from the `log/<date>.md` `Loc.File`) — left
  `""` when the header doesn't carry a parseable `HH:MM` (see EC-9 below);
  `Author` from the header's `· author` suffix; `Body` trimmed of
  surrounding whitespace; `Props["kind"]` set only when the header carries
  an optional kind word (`## HH:MM kind · author`).
- The day node also gets one synthetic `Link{Rel: "contains", To: <ULID>}`
  per ID-bearing entry, appended in-memory only — never written into `Raw`,
  consistent with this package's "forward facts only; aggregates are
  index-derived" doctrine. `LogParser.Serialize` always returns
  `n.Serialize()` (i.e. `Raw` verbatim); entry sub-nodes are never
  serialized back to their own files.
- `id:: <ULID>` is provably inert to the *core* parser: `parseFrontmatter`
  only scans the `---…---` block, `extractBody` only reacts to
  `[[wikilinks]]` and trailing `^anchors`, so an `id::` line is preserved
  verbatim in `Raw` and survives `TestRoundTripIdentity`/
  `FuzzRoundTripIdentity` (`logDayWithIDs` in `roundtripCorpus`) exactly
  like any other body text.
- `index.Open`'s default parser is `LogParser`, not `MarkdownParser` — see
  `internal/index/AGENTS.md`. Because `LogParser` is byte-identical to
  `MarkdownParser` for every non-`log-day` file, this is safe for every
  existing reader; the whole vault index is log-aware regardless of which
  command last built it.
- The writer (`internal/cli/add.go`, `rk add`) and this parser share one
  format definition, `RenderLogEntry(hhmm, author, ulid, body string) string`,
  so they can never drift apart on the entry byte-format.

**Known limitation (EC-9):** `SplitEntries` uses a naive `(?m)^## .*$`
header match with **no fence-awareness** — unlike this package's
wikilink/inline-code masking (see above), it does not toggle off inside a
fenced code block. A hand-authored or synced day file containing a fenced
`## `-prefixed line, e.g.:

    ```
    ## foo
    ```

inside its body would be mis-split: the fenced `## foo` line is
indistinguishable from a real entry header and starts a spurious extra
`log-entry` node. `rk add`'s own write path defensively rejects any
*outgoing* body/author that would introduce a `^## ` line (`add.go`'s
`embeddedHeaderRe` guard), but that only protects text `rk add` itself
writes — it does not fix `SplitEntries` for arbitrary pre-existing content.
Fixing this properly means teaching `SplitEntries` fence-toggling (a
`node.go` change, gated by the round-trip fuzz corpus) and is not done here;
avoid `## `-prefixed lines inside fenced code blocks in hand-authored
log-day files until it is.

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

`logparser_test.go` covers `LogParser`/`RenderLogEntry`: N+1 split count,
distinct non-empty entry ULIDs from `id::` lines, time reconstruction (from
day alias and from `Loc.File`), kind-word tolerance, a non-timestamp `## `
heading yielding an empty `Time` rather than a malformed one, hand-authored
entries with no `id::` line still splitting, `contains` link synthesis,
`MarkdownParser`-parity for non-`log-day` files, and `logDayWithIDs`
(`roundtripCorpus`) round-tripping byte-for-byte through `LogParser.Serialize`.
`../cli/add_test.go` covers the `rk add` writer end-to-end.
