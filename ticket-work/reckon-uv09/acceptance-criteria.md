# Acceptance Criteria — reckon-uv09 (v1-T4: log tool + capture, `rk log` end-to-end)

Read-only extraction, no implementation. Ticket text (verbatim):

> The highest-use surface: append-only daily log, replacing Logseq journal+recall.
> rk log/add appends a log-entry node to the day group file via span-local
> write-back (no full-file regen); log-day group parser yields N+1 nodes (day +
> entries); author/provenance stamped; AT= backfill for time. Relies on
> merge=union (T0) for lossless concurrent appends. Done when: rk log appends
> and the file round-trips; the day parses to day+entry nodes; the entry is
> indexed and retrievable via rk query (capture->index->query proven
> end-to-end); provenance present on every entry.

Grounding sources: `docs/design/composable-redesign.md` (per-tool scope table,
node-ID/group-file decision, node-field table incl. `author`=provenance and
`time` vs ULID-mint-time, concurrency section), `internal/node/{node,parser,
render,ulid,envelope}.go`, `internal/index/{index,reconcile,schema}.go`,
`internal/cli/{todo,adopt,query,index,log,root,stubs}.go`,
`internal/node/node_test.go` / `internal/spike/roundtrip/roundtrip_test.go`
(`logDay` fixture), `.gitattributes` + `internal/cli/gitattributes_test.go`,
and the closed sibling ticket `reckon-qiua`'s own plan (`ticket-work/
reckon-qiua/plan.md`, decision D7), which explicitly names this ticket as the
owner of a known architectural gap (see §2.3).

---

## Open questions for the planner (do not guess past these)

These are called out inline below too, but are collected here because each is
load-bearing enough to change the shape of the implementation, not just a
detail:

1. **Command-name collision.** `rk log` (with `add`/`note`/`delete`
   subcommands) already exists and is fully implemented against the *old*
   DB-backed journal (`internal/cli/log.go`, wired at `root.go:117`). A
   separate, still-`errNotImplemented` v1 stub `rk add` also already exists
   (`internal/cli/stubs.go:40-48`, wired at `root.go:131`). The ticket's own
   shorthand "rk log/add" does not resolve which of these the new
   text-vault-backed capture tool should become. See §2.1.
2. **The single-global-parser blocker in `index.Open`.** Every current index
   reader (`rk index`, `rk todo list`, `rk query`) opens the index with the
   hardcoded `node.MarkdownParser{}` default; nothing dispatches per
   directory/type to a group-file-aware parser. This is a real, previously
   *flagged-and-deferred* gap (`reckon-qiua`'s plan.md, decision D7, names T4
   as the ticket that must resolve it). See §2.3.
3. **The exact per-entry-ULID inline marker.** The design doc says a group
   file's contained nodes are "marked inline per item (`id::`-style)", but the
   only existing `logDay` fixture (used purely for round-trip testing) has no
   inline ID at all, and `node.Entry` carries no ULID field. This syntax does
   not exist yet anywhere in the code and must be designed by this ticket. See
   §2.4.
4. **`AT=` syntax.** Flag (`--at`) or inline token parsed out of the message
   text (`AT=14:30 ...`)? Both are plausible reads of the ticket text; neither
   has any precedent in the codebase. See §2.2.
5. **Same-host concurrent-append safety.** The ticket names `merge=union` (a
   *cross-device, git-merge-time* mechanism) as the stated safety net for
   "lossless concurrent appends." Nothing protects two `rk log` processes
   racing on the *same* checkout at the same time (`writeFileAtomic` is atomic
   per-write, not a read-modify-write lock). Is that same-host race
   acceptable/out of scope, or does it need an advisory lock? See §2.8/EC-3.
6. **Does `rk log add` trigger reindexing itself**, or is the "capture ->
   index -> query" chain in the ticket's own Done-when clause license for an
   explicit, separate `rk index` step (matching `rk query`'s existing
   no-auto-reconcile contract, unlike `rk todo list`, which does
   auto-reconcile)? See §2.9/EC-11.
7. **Are day->entry `contains` edges in scope?** The design doc mentions them;
   the ticket's own four-clause Done-when sentence does not. See §2.5.

---

## 1. Explicit acceptance criteria

Directly from the ticket's "Done when" sentence, split into independently
verifiable items:

- **AC-1 (append succeeds).** `rk log` (its capture verb, name TBD — §2.1)
  appends a new log-entry to the current day's group file in the vault,
  creating the day file (and its `log-day` node) on first use if it does not
  yet exist.
- **AC-2 (append is span-local).** The append is a surgical write-back: every
  byte of the day file that existed before the append (frontmatter, prior
  entries, any hand-authored content) is unchanged after it — no full-file
  regeneration, mirroring `internal/node`'s `SetField`/`ReplaceEntryBody`
  discipline applied to a *new* trailing span rather than an existing one.
- **AC-3 (round-trip holds).** The resulting file round-trips:
  `node.Parse(raw).Serialize() == raw`, both immediately after an append and
  for a day file that already contains prior entries (tool-written or
  hand-authored).
- **AC-4 (day parses to day+entry nodes).** Parsing a day file through the log
  tool's `node.Parser` implementation yields exactly **N+1** nodes for N
  entries: one `log-day` node plus one `log-entry` node per timestamped entry
  block, each entry with its own distinct identity.
- **AC-5 (indexed and retrievable via `rk query`, capture->index->query
  end-to-end).** After a capture and an index pass, `rk query`'s SQL surface
  over the T2 stable views (`nodes`/`edges`/`node_props`/`aliases`/`fts`)
  returns a row corresponding to the specific entry just captured — proving
  the full pipeline from text capture through parsing/indexing to SQL
  retrieval, not just that indexing runs without error.
- **AC-6 (provenance present on every entry).** Every `log-entry` node (not
  just the first entry of a day) carries a non-empty `author` field stamped
  at write time — per the design doc's own definition of "provenance" (see
  §2.6, `author` *is* the provenance field; there is no separate concept).

---

## 2. Implicit requirements

### 2.1 CLI verb placement — real naming collision, not a cosmetic detail

The ticket says "rk log/add," but two pre-existing, non-stub artifacts already
occupy adjacent names:

- `internal/cli/log.go:94-214` — a fully implemented `logCmd` with
  `add`/`note`/`delete` subcommands (`logAddCmd`'s `RunE` literally calls
  `journalService.AppendLog`, the **old**, DB-backed (`~/.reckon/reckon.db`)
  journal). Registered at `root.go:117` via `GetLogCommand()`. Has its own
  test suite (`log_test.go`, Bubble Tea multiline-editor tests) that is not
  about this ticket at all.
- `internal/cli/stubs.go:40-48` — a genuine, still-`errNotImplemented` v1 stub:
  `var addCmd = &cobra.Command{Use: "add", ... "Add a new node to the vault
  (v1 stub — not yet implemented)"}`, registered at `root.go:131`. This is
  structurally identical to how `todoCmd` was a stub in `stubs.go` before
  `reckon-qiua` graduated it into `internal/cli/todo.go` (deleting the stub
  block once the real command existed).

The "clean" graduation pattern (matching `qiua`) would delete a stub and add a
same-named real command — but there is no `logCmd` *stub* to graduate; the
name is taken by working, tested, unrelated legacy code. Concretely this
ticket must decide one of:

  (a) Graduate the `addCmd` stub (`rk add <text>`) instead — but that
      contradicts the ticket title's explicit "`rk log`" framing.
  (b) Replace/augment `logCmd`'s subcommands with the new node-based
      implementation — risks silently retiring the legacy DB journal as a
      side effect of this ticket, when a separate ticket (`reckon-s6oh`, T9)
      exists specifically to own that migration.
  (c) Introduce the new capture command under a different name (e.g. a new
      top-level verb, or `rk log` reserved for the new tool with the legacy
      one renamed/deprecated deliberately as part of this ticket).
  (d) Some coexistence mechanism (config/vault-shape detection) neither
      ticket text nor any code hints at today.

**This must be resolved by the planner before implementation starts** — it
determines which files get edited and whether existing `log_test.go` coverage
is expected to keep passing unmodified.

### 2.2 Verb surface (once naming is resolved)

Modeled on `internal/cli/todo.go`'s `todoAddCmd` (the closest real v1
precedent — file-per-item vs. group-file create, `NewNode`/`Render`/`Parse`/
`writeFileAtomic` recipe, `resolveAuthor` helper, `output.Writer` result
pattern, `Annotations: {"requiresDB": "false"}`):

- Positional body: `rk log add <text...>` (args joined like `todoAddCmd`'s
  `strings.Join(args, " ")`), or a bare `rk log <text...>` if the ticket's
  "rk log/add" phrasing means the subcommand is implied/optional for the
  "highest-use surface" (friction-reduction motive stated in the ticket
  itself) — **[OPEN, tie to §2.1]**.
- `--author` — reuse `resolveAuthor(flag string) string` verbatim
  (`todo.go:209-222`: `--author` flag > `$RECKON_AUTHOR` > `$USER` >
  `"local"`); it's already package-private within `internal/cli`, so this is
  a direct reuse, not a reimplementation.
- Global `--vault`, `--json`/`--ndjson` (via `output.ModeFromFlags`), `--quiet`
  — same persistent flags every v1 command already uses (`root.go:107-114`).
- **`--date`** (existing **global** persistent flag, `root.go:108`, today only
  consumed by the legacy `getEffectiveDate()`) is a strong, already-there
  candidate for "which day's *file*" an entry targets — i.e. backfilling into
  a **past day's** group file entirely (not just the time-of-day within
  today's file). This is a distinct concern from `AT=`, which the ticket
  frames purely as backfilling the entry's `time` value. **[OPEN — confirm
  `--date` picks the day-file and `AT=` backfills the time-within-that-file,
  since the ticket text only unambiguously specifies the latter.]**
- **`AT=` mechanism — [OPEN, no precedent either way]:**
  - (a) An explicit flag, e.g. `--at 14:30` / `--at 2026-07-04T14:30:00Z`,
    mirroring `todoAddCmd`'s `--scheduled`/`--deadline` flag shape.
  - (b) An inline token stripped from the message text itself, e.g.
    `rk log add "AT=14:30 did the thing"` — the literal capitalized `AT=`
    with an equals sign (not a flag) in the ticket text leans toward this
    reading.
  - Whichever syntax is chosen, the underlying mechanism it drives is already
    named by the design doc's node-field table (`composable-redesign.md`
    line 467-468): the entry's `time` field is independently settable from the
    ULID's own mint timestamp ("`time` … may differ from the ULID's mint
    time, e.g. backdated log"). So `AT=`/`--at` only overrides the rendered
    `time:` frontmatter value; the ULID still mints at the real wall-clock
    moment the command runs (preserving mint-order/uniqueness and the
    distinction between "when captured" and "when it happened").
- Output: a `logAddResult`-shaped struct (mirrors `todoAddResult`) with
  `.Pretty()`, printed via `output.New(cmd.OutOrStdout(), mode).Print(res)`,
  suppressed under `mode==Pretty && quietFlag` exactly like `todo.go`.

### 2.3 The single-global-parser blocker in `index.Open` — architecturally significant

`internal/index/index.go:60-67`:

```go
// Open ... The default parser indexes one node per file (node.MarkdownParser);
// group-file parsers (e.g. the log tool) plug in via OpenWithParser later.
func Open(cfg *config.Config) (*Index, error) {
	return OpenWithParser(cfg, node.MarkdownParser{})
}
```

Every current caller of the index uses the parser-less `Open` — `rk index
rebuild` (`internal/cli/index.go:33`), `rk todo list`
(`internal/cli/todo.go:401`), and `rk query`'s own read-only open via
`index.DBPath`. None of them can see a day file split into day+entry nodes
today; `reconcile.go`'s `indexFile` (`reconcile.go:227-253`) calls
`ix.parser.Parse(raw, loc)` exactly once per file using whatever single parser
the `Index` was opened with — there is no per-file-type dispatch mechanism at
all.

This is not a fresh discovery: `reckon-qiua`'s own plan.md (decision D7)
explicitly deferred this, verbatim: *"The single-global-parser limitation
remains a real, shared blocker for the still-open T4 log tool, but is
genuinely a non-issue here — flag it there, don't work around it here."*

T4 must resolve this one of at least three ways (**[OPEN — architecturally
significant, not a detail to improvise]**):

  (a) Build a composite/dispatching `node.Parser` (e.g. "path under `log/` ->
      split via `n.SplitEntries()`; else -> delegate to `node.MarkdownParser`")
      and make it the **new default** inside `index.Open` — a cross-cutting
      change touching every existing index reader, not something scoped to
      `internal/cli/log.go` alone.
  (b) Have `rk log`'s own commands (and, awkwardly, `rk query`/`rk index` too,
      if they are expected to see log entries) call `OpenWithParser` with an
      explicit log-aware parser — risking different tools' index opens
      disagreeing about how `log/*.md` parses depending on which command
      built/reconciled the DB last.
  (c) Some other seam (e.g. a parser registry keyed by directory, resolved
      inside `index.Open` itself) not yet designed anywhere in this codebase.

Whichever path is chosen, it is the mechanism that makes AC-4/AC-5 possible at
all — without it, a day file indexes as a single opaque `MarkdownParser` node
(current default), never splitting into day+entry rows.

### 2.4 Per-entry ULID marking — the design's stated mechanism doesn't exist in code yet

`docs/design/composable-redesign.md` ("Decision (2026-06-19) — locked"):

> *group file* (journal day, ephemeral inbox): one ULID per contained node,
> marked inline per item (`id::`-style)

But the only fixture exercising group-file splitting
(`internal/node/node_test.go:48-63`, identical in
`internal/spike/roundtrip/roundtrip_test.go`) has **no inline ID at all**:

```
## 08:38 progress · mike
Built the round-trip spike. Linked [[reckon-redesign]].

## 10:17 win · mike
Hardened the parser.
```

`node.Entry` (`node.go:529-532`) carries only `Header`/`Span` — no ULID field
— and `SplitEntries`/`entryHeaderRe` (`node.go:534-556`) do nothing but locate
`^## .*$` header lines and their byte spans; nothing derives a per-entry ULID
from that header today. **T4 is the ticket that must design and implement**
the actual inline-marker syntax (an `id:: <ULID>` line under each header, per
the design doc's stated intent, or some alternative), plus the logic that
reads/writes it. **[OPEN — no existing test or code pins the concrete
syntax.]** Whatever is chosen must:
  - parse identically whether the entry was written by `rk log add` or typed
    directly by a human/agent (the byte-preservation invariant applies to
    every file the vault holds, not only tool-written ones);
  - be weighed against the very Logseq `id::`-block noise this redesign is
    partly reacting against (worth a explicit design call, not a reflexive
    copy of Logseq's own convention).

### 2.5 `contains` edges from day to entries — implicit, not in the ticket's own Done-when

`composable-redesign.md` line 552-554: *"one `log-day` node … plus one
`log-entry` node per timestamped entry … The day carries `contains` edges to
its entries."* This means the log tool's `Parser.Parse` must synthesize a
`contains`-rel `Link` per entry on the day node — nothing in `internal/node`
auto-generates structural edges between a group file's own sub-nodes; it's
new logic this ticket must write. **[OPEN]** — the ticket's own four-clause
"Done when" sentence never mentions `contains` edges; treat as
implicit-but-important (it's the natural way "today's entries" becomes a
graph query rather than a body/time heuristic) but don't let its absence
block sign-off if the planner decides it's deferred.

### 2.6 Author/provenance mechanism

Per `composable-redesign.md` line 468 (`author` | inline + env | yes³ |
string | **provenance** — who wrote it (`mike` / agent persona / peer id)),
`author` **is** the design's literal definition of provenance — there is no
separate "provenance" struct/field anywhere in `internal/node` or elsewhere.
AC-6 is satisfied purely by a non-empty `author:` frontmatter value on every
`log-entry` node. Reuse `resolveAuthor` (`todo.go:209-222`) rather than
inventing a second author-resolution helper.

### 2.7 Span-local append primitive — new, not yet built

`ReplaceEntryBody` (`node.go:558-569`) replaces an **existing** entry's byte
span — it is an edit primitive, not an append primitive. There is no
`AppendEntry`/`InsertEntry` method on `Node` today. The natural shape mirrors
`todo.go`'s `addEphemeralTodo` EOF-append convention
(`todo.go:324-373`, especially its documented rationale at lines 327-333 for
never terminating the *previous* item with a fresh trailing newline that a
later append also "owns"): read raw bytes, refuse CRLF (mirrors the
`reckon-vj55` guard used throughout `todo.go`/`adopt.go`), append a new
`"\n## <header>\n<body>"` block strictly at EOF without touching any existing
byte, re-`Parse` the result, `writeFileAtomic`. First entry of a new day
additionally needs the `NewNode`/`Render` create path (mint a day ULID, set
`type: log-day`, alias = the ISO date, write `"# YYYY-MM-DD\n\n"` as initial
body) — directly analogous to `addDurableTodo`'s/`addEphemeralTodo`'s
`os.IsNotExist(err)` branch.

### 2.8 `merge=union` compatibility and same-host concurrency — two different guarantees, don't conflate them

`.gitattributes` already ships `log/*.md merge=union` (T0, watch-item #3;
enforced by `internal/cli/gitattributes_test.go`'s
`TestGitattributes_ScopedToLogOnly`/`TestGitattributes_CheckAttr`). This only
pays off if the append strictly follows an append-only-at-EOF discipline (same
reasoning `todo.go:327-333` documents for its ephemeral inbox) — never
rewriting a byte that belongs to a prior entry, so a line-based union merge of
two diverged day files combines cleanly instead of duplicating/reordering.

**Two genuinely different concurrency scenarios, only one of which
`merge=union` addresses:**
  - **Cross-device** (two git clones/checkouts diverge, then later merge) —
    this is what `merge=union` protects, and what the ticket's "Relies on
    merge=union (T0) for lossless concurrent appends" sentence is about.
  - **Same-host, same-checkout** (two `rk log` processes racing on one
    machine, e.g. two terminals or two agent processes) — `merge=union` does
    **nothing** here (there's only one file, no git merge involved).
    `writeFileAtomic` (`adopt.go:250-284`) only makes the *write itself*
    atomic (temp file + rename); it does **not** protect against a
    read-modify-write race between two concurrent invocations (both read the
    file before either writes; the second `os.Rename` silently clobbers the
    first's appended entry — a lost update, not a crash). The index's own
    `flock` (`internal/index/lock_unix.go`) guards only the index's own
    Reconcile/Rebuild pass — a different lock on a different file — and
    offers no protection to the vault text file itself.

**[OPEN]** — is same-host race safety in scope for this ticket, given the
ticket text names *only* `merge=union` (a cross-device mechanism) as the
stated safety net? Recommend treating it as an accepted, documented risk
(no advisory lock) unless the planner decides otherwise, since inventing a
new locking primitive for vault text files would be a bigger change than this
ticket's stated scope suggests.

### 2.9 Freshness — does `rk log add` reconcile the index itself?

`rk query` (`query.go:120-124`) requires a pre-existing index DB and errors —
*"index not found … run `rk index` first"* — if absent; it never auto-builds
or reconciles. `rk todo list` (`todo.go:401-409`), by contrast, calls
`index.Open` (auto-rebuild-if-stale) **and** an explicit `ix.Reconcile()`
before querying, specifically so a just-added todo is visible to the very
next `list` with no manual step (`qiua`'s plan.md D4 calls this a "hard
requirement" for that command).

The T4 ticket's own Done-when phrasing — *"the entry is indexed and
retrievable via rk query (**capture->index->query** proven end-to-end)"* —
names three distinct stages, reading as license for an explicit, separate
index/reconcile step between capture and query, consistent with `rk query`'s
existing no-auto-reconcile contract. **[OPEN — confirm this reading rather
than assuming `rk query` must see a just-appended entry with zero index step,
since that would require either changing `rk query`'s own freshness behavior
(arguably out of this ticket's scope) or having a write-path tool
(`rk log add`) reach into the read-index package, which no other v1 write
tool does today.]** Recommended default: capture -> explicit `rk index` (or
equivalent reconcile) -> query is the intended, tested flow; `rk log add`
itself does not need to trigger reconciliation.

---

## 3. Edge cases to handle

| # | Edge case | Notes |
|---|---|---|
| EC-1 | First-ever log entry of a brand-new day (no `log/` dir, no day file yet) | Must create the day file + `log-day` node fresh via the `NewNode`/`Render` path (§2.7); `os.MkdirAll` the `log/` subdir — `config.LoadWithOverrides` is pure and creates no directories (`config.go:118-121`'s own doc comment). |
| EC-2 | Vault directory itself does not exist (`--vault <fresh-path>`) | Mirrors `todo.go`'s TS-1.5: must create the vault root + `log/` subdir transitively, no "no such file or directory" error. |
| EC-3 | Two `rk log add` processes racing on the *same* host/checkout | Lost-update race, not protected by `merge=union` or `writeFileAtomic` alone (see §2.8). **[OPEN]** whether this needs a fix or is an accepted risk. |
| EC-4 | Two devices each append independently to the same day, later git-merged | `log/*.md merge=union` combines the two files' lines; the merged result must still parse cleanly into day+entries (no orphaned half-block, no duplicated/garbled frontmatter if union naively merges two competing frontmatter blocks — worth an explicit test, mirroring `gitattributes_test.go`'s `git check-attr`-based integration style). |
| EC-5 | Malformed/hand-edited day file: a conflict-marker file, or entries hand-typed out of chronological order | Conflict markers must be refused per `node.go:38-40`'s existing `conflictMarkers` check (never silently "fixed"). `SplitEntries` must not assume monotonic time ordering; appending must never reorder existing entries. |
| EC-6 | Whole-file-CRLF day file (Syncthing/Windows-authored) | `entryHeaderRe = regexp.MustCompile("(?m)^## .*$")` (`node.go:534`) — verify whether a `\r\n`-terminated header line is matched cleanly or leaves a trailing `\r` inside the captured header text. Recommend refusing CRLF day files outright on append, mirroring `todo.go`'s/`adopt.go`'s existing `reckon-vj55` CRLF guard, rather than risking a silent mis-split. |
| EC-7 | Empty or whitespace-only entry text | `todo.go`'s `runTodoAddE` rejects an empty body (`if body == "" { return fmt.Errorf(...) }`). **[OPEN]** whether a log — unlike a todo — should allow a bare-timestamp entry with no text, or reject it the same way. |
| EC-8 | Very long / multi-line entry body | Args-joined-by-spaces (the `todo add` convention) collapses shell-level newlines; a genuinely multi-line body (agent-authored, piped) has no existing precedent (no `--message-file`/stdin convention anywhere in `internal/cli`). Separately: `envelope.go:78-79`'s `WriteNDJSON` **errors** if an encoded envelope contains a literal newline byte — multi-line bodies must round-trip through JSON-escaped `\n` (already handled generically by `encoding/json`, just don't regress it). |
| EC-9 | Entry body containing a literal `## ` line (e.g. pasted markdown snippet) or other special characters (`---`, `[[`, unicode/RTL, emoji) | `entryHeaderRe`'s naive `^## .*$` match has **no fence/inline-code awareness** the way `extractBody`'s wikilink/tag extraction does (`maskInlineCode`, fence toggling). A body line that happens to start with `## ` would very likely be mis-detected as a **new** entry header by `SplitEntries`, silently splitting one logical entry into two. This is a real, currently-unguarded gap — flag explicitly, don't assume it's already handled by analogy to `extractBody`'s fence-awareness (that logic lives in a different function and isn't wired into `SplitEntries`). |
| EC-10 | Appending across a day boundary (session spans midnight) | Which day file does an entry land in — wall-clock "now" at write time, or something tied to `--date`/session start? Recommend: file selection always follows wall-clock now unless `--date` is explicitly passed (§2.2); `AT=`/`--at` only backfills the `time:` value *within* whichever day file is targeted, a distinct concern. **[OPEN]** — the ticket text only unambiguously covers the latter. |
| EC-11 | Index/reconcile lag: entry captured but no index pass run yet | Per §2.9, `rk query` should show nothing new until an explicit index/reconcile step runs — this is expected, current-architecture behavior, not a bug, but must be an explicit tested scenario so it isn't mistaken for one. |
| EC-12 | Transitional command-registration collision | If a new log-capture command and the legacy `logCmd` were both registered under the same `Use` during a half-finished implementation, Cobra's `AddCommand` would produce a duplicate-command error. Ties to §2.1 — must be resolved architecturally, not discovered as a build break. |

---

## 4. Test scenarios (Given/When/Then)

Command names below assume `rk log add <text>` per the closest existing
convention (`rk todo add`); adjust mechanically if §2.1 resolves the naming
question differently.

**TS-1** (AC-1/AC-3, fresh day, create path)
Given an empty vault with no `log/` directory,
When `rk log add "did the thing"` is run,
Then `log/2026-07-05.md` (today's date) is created containing a `log-day`
frontmatter block (`type: log-day`, alias `2026-07-05`) plus exactly one
`## ` entry block whose body is "did the thing"; the command exits 0; and
`node.Parse(raw).Serialize()` on the resulting bytes equals the file's on-disk
bytes exactly.

**TS-2** (AC-2, span-local append on a second entry)
Given the day file from TS-1 (one entry already present),
When `rk log add "second thing"` is run again,
Then the first entry's exact bytes (frontmatter block + its `## ` block) are
byte-identical before and after — a diff shows only newly appended bytes at
EOF — and the file now splits into 2 entries.

**TS-3** (AC-4, N+1 parse)
Given a day file containing 3 appended entries,
When the log tool's `node.Parser` implementation (or equivalently `rk index`)
processes the file,
Then it yields exactly 4 nodes — 1 `log-day` + 3 `log-entry` — each entry
carrying a distinct, non-empty ULID different from the day node's ULID and
from every other entry's.

**TS-4** (AC-5, capture->index->query end-to-end)
Given a fresh vault,
When `rk log add "buy milk"` is run, then `rk index` (rebuild) is run, then
`rk query "SELECT body, type FROM nodes WHERE type='log-entry'"` is run,
Then exactly one row is returned with `body` containing "buy milk" and
`type='log-entry'`.

**TS-5** (AC-6, provenance on every entry, not just the first)
Given `rk log add "did the thing" --author agent-x` followed by a second
`rk log add "did another thing"` with no `--author`/`$RECKON_AUTHOR`/`$USER`
set,
When both entries are parsed,
Then the first entry's `author` is `"agent-x"` and the second's falls back to
`"local"` (per `resolveAuthor`'s precedence) — both non-empty, demonstrating
provenance is stamped on every entry, not only the day's first.

**TS-6** (`AT=`/`--at` backfill — exact syntax pending §2.2)
Given `rk log add "AT=09:15 stood up"` (or the confirmed flag equivalent),
When the entry is parsed,
Then its `time:` field reflects `09:15` on the entry's day, while the entry's
ULID's own mint timestamp reflects the actual wall-clock moment the command
ran (not `09:15`) — demonstrating the design's stated `time`-vs-ULID-mint-time
divergence for backdated entries.

**TS-7** (EC-3, same-host race — outcome depends on §2.8's resolution)
Given two `rk log add` invocations launched concurrently against the same
vault checkout,
When both complete,
Then **[OPEN, pending planner decision]** either both entries are present (if
a lock is implemented) or the race is a documented, accepted risk and only one
survives — this scenario's expected result cannot be finalized without §2.8.

**TS-8** (EC-4, cross-device merge)
Given two independently diverged copies of the same day's file, each with one
distinct entry appended on a "different device" (simulated via two git
branches),
When git merges them (`log/*.md merge=union` applies),
Then the merged file still parses cleanly to 1 day node + 2 entry nodes — both
entries preserved, no conflict markers, no duplicated/garbled frontmatter.

**TS-9** (EC-6, CRLF day file refused)
Given a day file whose line endings are entirely CRLF,
When `rk log add` attempts to append to it,
Then (recommended) the command refuses with a clear "CRLF not supported
(reckon-vj55)" error, mirroring `todo.go`'s existing CRLF guard, rather than
silently mis-parsing entry headers.

**TS-10** (EC-7, empty body — behavior pending confirmation)
Given `rk log add ""` (or all-whitespace text),
When run,
Then (recommended, pending confirmation) the command errors with a non-zero
exit and a clear message, mirroring `todo add`'s empty-body rejection.

**TS-11** (EC-9, embedded `## ` line in entry body — expected to currently fail)
Given `rk log add` is called with a body containing a literal `## ` line
(e.g. a pasted markdown sub-heading),
When the resulting day file is parsed via `SplitEntries`,
Then — under today's unguarded `entryHeaderRe` — the embedded `## ` line is
very likely mis-detected as a new entry header, splitting one logical entry
into two spurious ones. This scenario should be written explicitly (even if
initially expected to fail/xfail) to force an explicit decision: guard entry
bodies against literal `## ` lines, or document the limitation the way
`internal/node/AGENTS.md` documents the block-list-comma limitation.

**TS-12** (EC-10, day-boundary routing)
Given the wall clock crosses midnight between two `rk log add` invocations in
the same terminal session, with no `--date`/`AT=` override on the second,
When the second `rk log add` runs just after midnight,
Then the entry lands in the **new** day's file (routing follows wall-clock
now, not "session start"), per the recommended default in §2.2/EC-10.

**TS-13** (EC-11, index lag is expected, not a bug)
Given `rk log add "just added"` has run but no index/reconcile pass has run
since,
When `rk query "SELECT * FROM nodes WHERE type='log-entry'"` is run,
Then the new entry is **absent** (stale index, matching `rk query`'s
documented no-auto-reconcile contract) — followed by running `rk index` and
re-querying, where the entry now appears. This pins "a manual index step is
expected" as intentional, not a defect.

**TS-14** (round-trip against a hand-authored day file)
Given a day file typed directly by a human (not via `rk log add`) but
following the same day+entries shape (frontmatter + `## ` blocks),
When `rk log add` appends a new entry to it,
Then every one of the human-authored entries' bytes is preserved untouched —
proving the append path is span-local against *any* valid day file, not only
ones the tool itself created.

**TS-15** (`--date` targets a past day's file, distinct from `AT=`)
Given an existing day file for `2026-07-01`,
When `rk log add --date 2026-07-01 "backfilled entry"` is run (assuming §2.2's
recommended split between `--date` = file selection and `AT=` = time-within-file),
Then the entry is appended to `log/2026-07-01.md`, not today's file, with its
`time:` defaulting to the actual wall-clock capture moment unless `AT=`/`--at`
is also given.

---

## 5. Explicitly out of scope

- **Recurrence** (`repeat:` org-repeaters, stored `scheduled`-cursor advance,
  a `did` log entry written as completion audit) — `reckon-ar9m` (v1-T6),
  which depends on this ticket. T4 only needs the log-entry creation primitive
  to exist and be robust; it does not need to build a `did->task` edge/link
  capability preemptively (see §2.5's note on `contains` edges being the only
  in-scope structural edge this ticket should add).
- **`rk today` / the agenda** (overdue+scheduled-today surfacing, in-list
  scheduling/do keys, completion emitting a linked `log-entry` `did->task`) —
  `reckon-liml` (v1-T7), which also depends on this ticket. T4 is not
  responsible for the "emit-on-complete" call site, only for the append
  primitive another tool can call into.
- **MCP porcelain** — `reckon-cxx1` (v1-T10, fast-follow). No MCP tool wiring
  for log capture in this ticket.
- **`rk-brief` synthesis** (AI-authored `type: brief` nodes summarizing log
  content) — `reckon-pvyl` (v1-T11, deferred seam). Not this ticket's job.
- **Promotion / branch-to-note from a log entry** (the design doc's
  `--ref <ID>` / `--pop <ID>` disposition machinery, "Logseq's loved
  capture-then-branch flow") — the note tool itself doesn't exist yet
  (`reckon-ih5g`, v1-T8, still `OPEN`). No `rk log --ref <id> | rk note
  --import` piping is in scope here.
- **Migration of the legacy DB-journal data** — `reckon-s6oh` (v1-T9). T4
  does not migrate, read, or delete `journal.Service`'s existing SQLite
  journal tables; the new `log/` text vault is a parallel, from-scratch
  store. (Coexistence *at the command-name layer* is unresolved per §2.1 —
  that's a naming/registration question, not a data-migration one.)
- **TUI display of the new log** — the existing three-column Activity Log
  pane reads the old `journal.Service`; wiring the TUI to the new text-vault
  log is not mentioned by this ticket or implied by any dependency.
- **FTS ranking/snippeting beyond the existing `fts_search`/`fts` view
  surface** (already shipped by T2/`reckon-a4eh`) — a log entry's `body` is
  already indexed into `fts_search` for free via the generic `insertNode`
  path (`reconcile.go:286`), identically to any other node type; no
  log-specific search UI/ranking work is needed.
- **Multi-vault / cross-vault log aggregation** — `rk log` operates on the
  single resolved `cfg.VaultDir`, like every other v1 tool.
- **Timezone handling beyond `AT=`/`--at` backfilling a time** — no stated
  requirement for timezone-aware storage/display beyond plain RFC3339 (which
  itself carries an offset); DST transitions, per-user timezone
  configuration, etc. are not mentioned by the ticket and should not be
  invented.
- **Editing or deleting existing log entries** (beyond create/append) — the
  ticket's scope is capture -> index -> query. An `rk log edit`/`rk log
  delete` for the *new* node-based tool (as distinct from the legacy
  `logNoteCmd`/`logDeleteCmd`, which mutate the old DB journal and are
  unrelated to this ticket) is not requested by the Done-when clause.
