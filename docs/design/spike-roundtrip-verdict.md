# Spike verdict — parse/serialize round-trip + span-local write-back

> Status: **PASS — the load-bearing bet holds.** Gating Group-A spike
> (`reckon-k9ff`, watch-item #1). Code: `internal/spike/roundtrip/`.
> Date: 2026-06-23.

## Question

The keystone of the composable redesign reduces to a per-tool `parse`/`serialize`
pair. Under files-as-truth, a `serialize` bug doesn't misread — it **silently
rewrites the authoritative file**. The design gated the whole architecture on
proving **span-local write-back**: a tool can rewrite **only the spans it owns**
while byte-preserving every other byte, such that `parse(serialize(parse(f))) == f`
on real Obsidian/Logseq/agent input.

## Verdict: PASS

A self-contained spike (`internal/spike/roundtrip/`) demonstrates it. All unit
tests pass and a 30s fuzz run (~396k execs) found **zero** round-trip violations.

| Test | Proves |
|---|---|
| `TestRoundTripIdentity` | `serialize(parse(f)) == f` byte-for-byte across the corpus (note, todo, log-day, edge cases) |
| `TestStructuredViewIgnoresFences` | the structured view is correct *and* code-fence content is inert (no `[[notalink]]`/`#nottag` leakage); labels/fragments parsed |
| `TestSpanLocalEditIsSurgical` | editing one frontmatter field changes **only** that value's bytes; siblings, ref-props, and body untouched; view updates |
| `TestGroupFileEditOneEntry` | editing one log entry leaves its siblings **byte-identical** (the "edits above/below the span" worry) |
| `TestConflictMarkersRefused` | malformed (git-conflict) files are refused gracefully, never panic |
| `FuzzRoundTripIdentity` | round-trip identity + span-edit invariants hold on arbitrary fuzzed input (~396k execs, 0 failures) |

## Why it works (the insight that makes it safe)

1. **Bytes are authoritative; the structured model is a derived view.** `Parse`
   keeps the original `Raw` bytes and builds the view (links, fields, fragments)
   *alongside* them with byte-offset spans for owned fields. **`Serialize` of an
   unedited node returns `Raw` verbatim** — so the round-trip is lossless *by
   construction*, not by careful regeneration. This is the opposite of the current
   journal writer, which regenerates from a model and drops what it doesn't model.
2. **Edits are surgical splices into spans**, never whole-file regeneration. The
   bytes outside the owned span are copied, not rebuilt.
3. **Offsets are ephemeral, recomputed on every `Parse`.** reckon re-parses on its
   lazy reconcile-on-read, so spans are always fresh against current text — the
   "does the span survive edits above/below" worry never arises, because nothing
   caches offsets across external edits. **The durable anchor is the ULID** (in
   frontmatter / a `^frag`), not a byte offset.

Corollary: this is exactly what lets a human (nvim/Obsidian) and a tool edit the
same file without clobbering — i.e. it reproduces org-mode's hand-editable feel.

## Boundaries (what the spike deliberately scoped out)

The bet is proven for the common, load-bearing cases. Not yet covered:

- **Scalar frontmatter only.** Block scalars (`|`/`>`), nested maps, and flow
  sequences edited *element-wise* are not span-edited. (They round-trip fine —
  they're just bytes — but `SetField` only targets single-line scalar values.)
- **Adding a missing key** (vs. editing an existing one) means inserting a line
  into the frontmatter block — straightforward, but a separate concern from the
  byte-preservation bet, so it's out of scope here.
- **CRLF / encoding oddities** not exercised (the vault is LF markdown).
- **Per-tool parsers** still each need their own span logic; the spike proves the
  *technique*, not every tool's mapping.

None of these threaten the bet; they are the remaining engineering surface.

## Pre-named fallback (per watch-item #1)

The spike passed, so the fallback is **not** triggered. Recorded anyway so a
future regression on a harder type reroutes the design instead of stranding it:

> **Fallback — DB-truth for the single most-structured type.** If span-local
> write-back ever proves infeasible for a specific type (most likely a heavily
> structured one), make *that type* DB-primary with a deterministic generated
> markdown projection (the 6-15 "journal projection = OUTPUT" model), while every
> other type stays files-as-truth. This is a per-type retreat, not a whole-system
> one — the architecture already tolerates mixed truth direction because each tool
> owns its `parse`/`serialize`. Trip-wire: a type whose round-trip fuzz cannot be
> made green with reasonable span logic.

## Implications

- **Keystone de-risked.** The gating bet is green; the design can proceed to the
  v1 sequence (substrate + log capture + index + `rk query`, then todo/agenda).
- **Production parser pattern is set:** preserve bytes, derive a view with spans,
  splice to edit, re-parse to refresh. Replaces the regenerate-from-model writer.
- **`merge=union` stays log-only** (watch-item #3): span splices keep frontmatter
  edits surgical, but two *concurrent* edits to the same node's frontmatter are a
  git conflict, not silent loss — acceptable, and a post-write "did my ULID land?"
  verify is the trip-wire.
