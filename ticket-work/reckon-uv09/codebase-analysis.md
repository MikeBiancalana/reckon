# Codebase analysis — reckon-uv09 (v1-T4: `rk log` capture end-to-end)

Read-only research for planning. Worktree: `/home/chadd/repos/reckon/.worktrees/reckon-uv09`.

## 0. TL;DR for the planner

Everything T4 needs from T0/T1/T2/T3 is merged and present. The two things that
do **not** yet exist anywhere in the codebase or design docs, and that planning
must decide, are:

1. **How a per-entry ULID (or lack thereof) is represented in plain text**,
   since a `## ` entry inside a day-group file has no frontmatter block of its
   own to hold an `id:` field. No existing code or doc fixture shows this.
2. **What `rk log` actually is as a CLI surface**, given `internal/cli/log.go`
   already defines a *different*, legacy, fully-working `log` command
   (`add`/`note`/`delete` against the old SQLite-backed `internal/journal`
   package, files under `~/.reckon/journal/`, nothing to do with the vault or
   `internal/node`/`internal/index`). T4's "rk log" must resolve this name
   collision one way or another.

Both are elaborated in §2 and §9 below. Everything else — parser plumbing,
index plumbing, query plumbing, write-back primitives, provenance stamping,
test harness — has a direct, working precedent to clone.

---

## 1. Canonical node package (T1) — `internal/node`

Package doc: `internal/node/node.go:1-29`. Byte-preserving: `Raw` is
authoritative, `Serialize()` returns it verbatim, typed fields are a read-only
derived projection (`deriveView`/`extractBody`), and the *only* write path is a
span-local splice (`SetField`, `InsertField`) that re-parses after splicing.

**`Node` struct** (`internal/node/node.go:73-91`):
```go
type Node struct {
    Raw []byte

    ULID      string
    Type      string
    Time      string
    Author    string
    Body      string
    Aliases   []string
    Props     map[string]string
    Fragments []Fragment
    Links     []Link
    Loc       Loc

    frontmatter map[string]string
    fmOrder     []string
    fieldSpans  map[string]Span
    bodySpan    Span
}
```

**`Envelope` struct** (`internal/node/envelope.go:19-33`, NDJSON interchange
form — one node per line, no embedded newlines):
```go
type Envelope struct {
    V         int               `json:"v"`
    ULID      string            `json:"ulid,omitempty"`
    Type      string            `json:"type"`
    Time      string            `json:"time"`
    Author    string            `json:"author"`
    Body      string            `json:"body"`
    Aliases   []string          `json:"aliases,omitempty"`
    Props     map[string]string `json:"props,omitempty"`
    Fragments []Fragment        `json:"fragments,omitempty"`
    Links     []Link            `json:"links,omitempty"`
    Loc       Loc               `json:"loc"`
    Hash      string            `json:"hash,omitempty"`
    Mtime     string            `json:"mtime,omitempty"`
}
```

**ULID: inline, not surrogate, when present.** `id:` is a reserved frontmatter
key (`reservedKeys`, `node.go:44-46`) promoted to `Node.ULID`. `node.Mint()` /
`node.MintAt(t)` (`internal/node/ulid.go:19-35`) mint monotonic ULIDs (26-char
Crockford base32, oklog/ulid/v2). `NewNode(typ, author, body string) *Node`
(`internal/node/render.go:120-128`) mints one automatically. Nodes with **no**
inline ULID key on the index's identity surrogate `"file:<relpath>"` (see §5,
`nodeKey` in `internal/index/reconcile.go:257-267`) — surrogate identity is a
property of the *indexer*, not of `internal/node` itself; `Node.ULID` is simply
`""` for such a node.

**Create-path recipe** (used by `internal/cli/adopt.go` and `internal/cli/todo.go`,
the pattern any new create-path in T4 should clone):
`NewNode(type, author, body)` → set `.Time`/`.Props`/`.Links`/`.ULID` →
`n.Render()` → `node.Parse(rendered)` → write `parsed.Serialize()` via
`writeFileAtomic`. The re-`Parse` after `Render` is mandatory before any
subsequent `SetField` edit (`render.go` doc contract, `internal/node/AGENTS.md`
"Render (create path)" section).

**Group-file primitives already exist** (`internal/node/node.go:519-569`),
productionized from the gating spike:
```go
type Entry struct {
    Header string
    Span   Span
}
func (n *Node) SplitEntries() []Entry
func (n *Node) ReplaceEntryBody(e Entry, newBlock string) ([]byte, error)
```
`SplitEntries` finds every `^## .*$` (ATX H2) header in the body and returns
the byte span from that header to just before the next header (or EOF) —
content before the first header (frontmatter + preamble) is not an entry.
`ReplaceEntryBody` splices a replacement block over one entry's span, leaving
siblings byte-identical (proven by `TestGroupFileEditOneEntry`,
`internal/node/node_test.go:281-309`). **These are purely byte-splicing
primitives on a single `*Node`** — they do not themselves produce additional
`*Node` objects, mint ULIDs, or plug into a `Parser`. `internal/node/parser.go:12-15`
says explicitly: *"Group-file sub-node splitting with per-entry ULIDs is a
per-tool concern (the log tool, T4); see (*Node).SplitEntries for the building
block."* — i.e. this ticket is the one that has to write that code; it exists
nowhere yet.

**`Parser` interface** (`internal/node/parser.go:7-10`):
```go
type Parser interface {
    Parse(raw []byte, loc Loc) ([]*Node, error)
    Serialize(n *Node) ([]byte, error)
}
```
`MarkdownParser{}` (`parser.go:16-34`) is the only implementation today: one
node per file, `Serialize` returns `n.Serialize()` verbatim. A log-day group
parser is a **second implementation of this interface**, T4's actual deliverable
in `internal/node`-adjacent terms.

---

## 2. Parser architecture / what a log-day group parser needs

No group parser exists yet anywhere (confirmed — see `internal/index/index_test.go:80-84`,
a `multiNodeParser` test double built explicitly *"so a test can put two nodes
... without needing a real multi-entry (group-file) parser implementation"*).
The only real fixture is the round-trip test data:

**Reference fixture** (`internal/node/node_test.go:50-61`, identical in
`internal/spike/roundtrip/roundtrip_test.go:55+`):
```markdown
---
id: 01J9Z3K7Q2W8XR4M6N0V5BYHEF
type: log-day
aliases: 2026-06-22
---
# 2026-06-22

## 08:38 progress · mike
Built the round-trip spike. Linked [[reckon-redesign]].

## 10:17 win · mike
Hardened the parser.
```

Design intent (`docs/design/composable-redesign.md:552-555`):
> Group file — a log day `log/2026-06-19.md` parses to **N+1 nodes**: one
> `log-day` node (alias `2026-06-19`) plus one `log-entry` node per timestamped
> entry, each with its own ULID. The day carries `contains` edges to its entries.

So the required shape is:
- Day node: `type: log-day`, `aliases: [<YYYY-MM-DD>]`, frontmatter `id:` (own
  ULID), body = everything after the frontmatter (title + all entries) — this
  part is just `node.Parse`/`MarkdownParser` as-is.
- One `log-entry` node per `## ` header block (`SplitEntries()` gives the byte
  spans), each needing `Type`, `Time`, `Author`, `Body` populated by parsing
  the header line (`## HH:MM <kind> · <author>` in the fixture — no existing
  regex extracts this structured shape; must be written new) plus a `contains`
  edge from the day to each entry.

**Open question A (biggest gap): per-entry ULID representation.** The design
prose says each entry gets "its own ULID," but no fixture, spec, or code shows
*where in the raw bytes* that ULID lives — entries have no frontmatter block.
Candidates, none implemented or decided anywhere in the repo:
  - Reuse the existing block-anchor mechanism: a trailing `` \s^ID`` on the
    header or last body line (`blockAnchorRe`, `node.go:109`, already parsed
    into `Fragment{ID: ...}` for *arbitrary* anchors) repurposed as the entry's
    identity. Would need new semantics in the group parser (not in core
    `node.go`) to promote that fragment to `Node.ULID` instead of a generic
    `Fragment`.
  - No literal ULID at all — key entries on the indexer's surrogate
    `"file:<relpath>#N"` (`nodeKey`, `reconcile.go:257-267` already supports
    this for multi-node files) and treat "each with its own ULID" as aspirational
    design prose, not a hard ticket requirement (the ticket's own "Done when"
    list only requires the entry be indexed/queryable and carry provenance —
    it does not require an inline literal ULID token in the text).
  - Some other embedded marker (HTML comment, zero-width token) — unprecedented
    in this codebase, would need new parsing rules and would touch the
    byte-preservation fuzz corpus.

This must be settled in planning; nothing upstream (T0/T1) resolved it.

**Open question B: entry `Type` vs. `kind` (progress/win/log/etc).** The
fixture's header text (`08:38 progress · mike`, `10:17 win · mike`) suggests a
"kind" concept survives from the pre-redesign spec's `--kind` flag (see
`docs/reckon-spec_2026-06-15.md:100-118` — a **superseded** DB-first design,
not what this codebase implements, but the alias verbs `rk log`/`rk progress`/
`rk win` it describes may be the intended UX). The current composable design
doc never elaborates whether "kind" becomes `Node.Type` (e.g. `type: progress`)
or a `Props["kind"]` on a uniformly `type: log-entry` node. Planning must pick
one; nothing forces either.

**How the parser plugs into indexing:** `internal/index.OpenWithParser(cfg,
parser node.Parser)` (`internal/index/index.go:70-100`) already exists as the
seam — `Open(cfg)` is just `OpenWithParser(cfg, node.MarkdownParser{})`
(`index.go:65-67`). `internal/index/AGENTS.md:84` states this explicitly: *"the
log tool (T4) plugs in a group-file parser via `OpenWithParser`."* **However**,
every current CLI caller (`internal/cli/index.go:33` for `rk index`,
`internal/cli/todo.go:401` for `rk todo list`) calls the plain `index.Open(cfg)`
— hardcoded `MarkdownParser{}`. For log-day files to actually get split into
N+1 nodes during a normal `rk index`/`rk query`/`rk todo list` pass (not just
inside the log tool's own writer), *something* needs to route the whole-vault
index build through a parser that handles **both** ordinary one-node-per-file
docs and `type: log-day` group files. Concretely this likely means either:
  - a small composite/dispatching `node.Parser` (e.g. try `MarkdownParser`
    first, then re-split if `Type == "log-day"`) that `rk index`/`rk query`'s
    `index.Open` call sites switch to, or
  - `index.Open` itself gains vault-wide default dispatch logic.
This is a cross-cutting change beyond `internal/cli/log.go` alone — flag for
planning's "files to modify" list.

---

## 3. `merge=union` (T0) — mechanism, already shipped

**Not a custom merge algorithm in Go — a stock git attribute.**
`.gitattributes:8`:
```
log/*.md merge=union
```
with comment (`.gitattributes:1-7`): *"Append-only daily log files use a union
merge driver so concurrent appends from multiple devices combine instead of
conflicting. Scoped to `log/*.md` ONLY ... (v1-T0 watch-item #3, log-only)."*
`merge=union` is git's **built-in** union merge driver (no custom script/driver
definition anywhere in the repo) — on a 3-way merge of `log/*.md`, git takes
the union of changed *lines* from both sides instead of emitting conflict
markers. This is why the log file must be line-appendable (each entry a
distinct `## ` block) rather than needing full-file rewrite: two concurrent
appends to the same day file merge cleanly at the git level with zero
reckon-side conflict-resolution code.

**"Lossless concurrent appends" mechanically = three layered guarantees, only
one of which (git) is implemented:**
1. **git-level:** `merge=union` — implemented (`.gitattributes`), needs no Go code.
2. **File-format-level:** each append must be its own `## ` block, never a
   full-file regen — this is exactly "span-local write-back" (§4) and *is*
   T4's job to implement (append = insert a new block at EOF, byte-preserving
   everything above it).
3. **Post-merge cleanup ("sort-on-reconcile" + "ULID-dedup"):** design docs
   (`docs/design/composable-redesign-assessment.md:23`,
   `docs/design/composable-redesign-rebuttal.md:67-69`) call for entries to be
   re-sorted by time and de-duplicated by ULID after a union merge may have
   interleaved them out of chronological order. **No such sort/dedup code
   exists anywhere in the repo today** (not in `internal/index/reconcile.go`,
   not in `internal/node`). The ticket's own "Done when" list does not
   mention sort/dedup explicitly — flag as an open question whether T4 must
   implement it or whether it is legitimately deferred (the design docs call
   it "cheap insurance," not blocking, and reckon-uv09's dependency list does
   not include a dedicated "sort-on-reconcile" ticket).

---

## 4. Span-local write-back — what exists, what T4 needs to add

**"Shared `Render(node)`" (review H1) is `internal/node/render.go`.**
`(*Node) Render() []byte` (`render.go:32-115`) is the create-path inverse of
`deriveView`+`extractBody`, used by every current create-path tool
(`adopt.go` doesn't need it — it only inserts a field into an existing file;
`todo.go`'s `addDurableTodo`/`addEphemeralTodo` do: `NewNode(...)` → set fields
→ `.Render()` → `node.Parse(rendered)` → `writeFileAtomic(parsed.Serialize())`).

**What "full-file regen" (the anti-pattern) looks like, for contrast:**
`internal/journal/writer.go`'s `WriteJournal(journal *Journal, filePath string)
error` (documented in `internal/journal/AGENTS.md:180-216`) rebuilds the entire
day file's text from the in-memory `Journal` struct on every mutation — this is
literally what `docs/design/composable-redesign.md:583-587` calls out by name
as "current `journal/service.go`'s `WriteJournal` does exactly the wrong thing"
and what the whole node/span design exists to avoid. **T4 must not use this
path or anything shaped like it for `log/*.md` files.**

**What span-local write-back for an *append* concretely means (no direct
precedent yet, closest analogues):**
- `internal/cli/todo.go`'s `addEphemeralTodo` (`todo.go:334-373`) is the
  closest existing "append to an existing file, byte-preserving everything
  before" pattern: read raw bytes, `node.Parse` to get the current body length
  and confirm CRLF/structure, then `append(raw, "\n"+newItem...)` and
  `writeFileAtomic`. No `SetField`/frontmatter edit involved — purely an EOF
  byte-append. T4's "append a new `## ` entry block at EOF" is the same shape,
  one level up (whole entry block instead of one checkbox line).
- `(*Node).ReplaceEntryBody` (`node.go:558-569`) is the primitive for editing
  an *existing* entry in place (not needed for a pure append, but relevant if
  T4 ever needs "amend last entry" or similar).
- **First-append-of-the-day (file doesn't exist yet)** has no precedent
  in `internal/cli` beyond `todo.go`'s "create container on first use"
  branch (`addEphemeralTodo`'s `os.IsNotExist(err)` branch, `todo.go:339-351`):
  build via `NewNode("log-day", ...)` → `Render()` → `Parse` → `writeFileAtomic`.

**Atomic write helper** (shared, package-private, already used by both
`adopt.go` and `todo.go` — T4 should reuse verbatim, not duplicate):
```go
// internal/cli/adopt.go:250
func writeFileAtomic(path string, data []byte) error
```
Writes to a same-directory temp file then `os.Rename`s over the target (atomic
on the same filesystem); on any error the temp file is removed and the
original is untouched.

---

## 5. `internal/index` (T2) — Reconcile/Rebuild, indexing path, fast path

**Public API** (`internal/index/index.go`):
```go
func Open(cfg *config.Config) (*Index, error)                        // MarkdownParser{}, hardcoded
func OpenWithParser(cfg *config.Config, parser node.Parser) (*Index, error)
func (ix *Index) Close() error
func (ix *Index) DB() *sql.DB
func (ix *Index) Meta(key string) (string, error)
func DBPath(cfg *config.Config) (string, error)                      // for read-only opens (T3)
```
**Reconcile/Rebuild** (`internal/index/reconcile.go`):
```go
func (ix *Index) Rebuild() (Stats, error)     // drop+recreate schema, full walk
func (ix *Index) Reconcile() (Stats, error)   // lazy, hash-authoritative, incremental
```
Both funnel through `reconcileTx` — a full `filepath.WalkDir` of the vault on
every call, with an mtime fast-path to skip unchanged files and a content-hash
fallback for files whose mtime moved but content didn't. **There is no
per-file/per-append fast path that indexes just one new/changed file** — the
"fast path" that exists is *skip unchanged files during a walk*, not *index
only this one path without walking*. `rk todo add` → `rk todo list` proves the
capture→index→query loop today by calling `ix.Reconcile()` on every `list`
(`todo.go:407-409`) — same mechanism T4 should reuse: write the file, then
either call `ix.Reconcile()` from the log tool itself, or rely on the next
`rk query`/`rk todo list`-style caller to reconcile (T3's `rk query` does
**not** auto-reconcile — see §6 — so if `rk log add` doesn't reconcile itself,
the AC "entry is indexed and retrievable via rk query" requires either T4's
add path or a manual `rk index` in between).

**Indexing one file → N node rows** (`indexFile`, `reconcile.go:227-253`):
```go
func (ix *Index) indexFile(tx *sql.Tx, rel string, raw []byte, hash string, mtime int64) ([]string, error)
```
calls `ix.parser.Parse(raw, node.Loc{File: rel})` — this is **exactly** the
seam a log-day group parser plugs into; already generic over N nodes per file
(`nodeKey` is computed per node with a `seen` map disambiguating duplicate
surrogate keys within one file, `reconcile.go:257-267`). No change to
`indexFile`/`reconcileTx` should be needed — only to *which* `node.Parser` is
passed to `OpenWithParser` for vault-wide indexing (§2's open question).

**Views (T2's stable contract, `internal/index/schema.go:81-90`):**
`nodes(id, ulid, type, time, author, body, loc)`,
`edges(src, rel, dst, dst_key, from_frag, to_frag)`,
`node_props(id, key, value)`, `aliases(alias, id)`, `fts(id, body)`,
`fts_search` (fts5 vtable, MATCH-capable). `id` = inline ULID else
`"file:<relpath>"` surrogate. A `contains` edge from day→entry (per the design
doc) would need `Link{Rel: "contains", To: <entry identity>}` on the day node —
requires the entry to have *some* resolvable identity (ULID or alias) for the
edge to resolve (`resolveEdges`, `reconcile.go:375-387`), tying back to Open
Question A.

---

## 6. `internal/cli/query.go` (T3) — read surface, validating the AC

`rk query [SQL]` (`internal/cli/query.go:37-52`, `Annotations{"requiresDB":"false"}`):
opens the index **read-only** (`openReadOnlyIndex`, `query.go:227-239`, a
`file:...?mode=ro` DSN) directly at `index.DBPath(cfg)` — **it does not call
`Reconcile()` or `Rebuild()`**, and errors out if the index file doesn't exist
yet (`query.go:125-127`: `"index not found ... run rk index first"`). This
means:
- **`rk query` never triggers indexing.** For the ticket's "capture→index→query
  proven end-to-end" AC, something between capture and query must reconcile —
  either T4's own `add` path calling `ix.Reconcile()` (mirroring `todo.go`'s
  `list`), or the test/AC driving an explicit `rk index` between `rk log add`
  and `rk query`. This is a concrete design decision T4's plan should state
  explicitly (todo.go's precedent leans toward "the porcelain command that
  reads should reconcile," but `rk log add` is a *write*, not `list`; there is
  no read-after-write porcelain command specified yet unless T4 also adds
  something like `rk log list`/`rk today` reads through it).
- Query emits canonical node NDJSON by default (`canonicalObjects`,
  `query.go:399-462`, reconstructs a full `node.Envelope` per result row from
  `nodes`/`node_props`/`aliases`) or raw rows with `--raw`.
- Validating the AC concretely: after `rk log add "text"`+reconcile,
  `rk query --vault <v> "SELECT id, type, time, author, body FROM nodes WHERE type='log-entry'"`
  (or whatever `Type` T4 settles on, cf. Open Question B) should return the new
  entry's row with `author` populated (provenance) and body containing the
  text; `SELECT * FROM edges WHERE rel='contains'` should show the day→entry
  edge if that's implemented.

---

## 7. `internal/cli` dispatch pattern + existing stub/impl status

**External dispatch** (`internal/cli/dispatch.go`): `rk <verb>` first tries
`RootCmd.Find`; if it resolves to a real builtin cobra command, that wins.
Otherwise it looks for an `rk-<verb>` executable on `PATH`
(`findExternal`, `dispatch.go:22-41`) and `syscall.Exec`s it
(`execExternal`, `dispatch.go:129-146`), replacing the current process. This
only matters for T4 if the plan is to ship `rk log` as an *external* `rk-log`
binary rather than a builtin — nothing in the ticket or design docs suggests
that; `rk todo`/`rk query`/`rk index`/`rk adopt` are all builtins registered in
`root.go`, and T4 should almost certainly follow that precedent (a builtin,
not a PATH extension).

**Current registered commands** (`internal/cli/root.go:116-135`):
```go
RootCmd.AddCommand(GetLogCommand())   // v0 legacy — internal/journal-backed, see below
RootCmd.AddCommand(GetNoteCommand())
RootCmd.AddCommand(GetNotesCommand())
RootCmd.AddCommand(todayCmd)
RootCmd.AddCommand(weekCmd)
RootCmd.AddCommand(rebuildCmd)
RootCmd.AddCommand(GetReviewCommand())
RootCmd.AddCommand(GetScheduleCommand())
RootCmd.AddCommand(GetTaskCommand())
RootCmd.AddCommand(GetWinCommand())
RootCmd.AddCommand(GetChecklistCommand())

RootCmd.AddCommand(tuiCmd)
RootCmd.AddCommand(addCmd)    // v1 stub, stubs.go — returns errNotImplemented
RootCmd.AddCommand(todoCmd)   // v1, real — graduated in reckon-qiua (PR #148)
RootCmd.AddCommand(queryCmd)  // v1, real — T3
RootCmd.AddCommand(indexCmd)  // v1, real — T2
RootCmd.AddCommand(adoptCmd)  // v1, real
```

**`GetLogCommand()` (`internal/cli/log.go:212-214`) is a live, fully-working
`log` command — but it is the *legacy v0* journal system, not this ticket's
target.** It registers `log add [message]`, `log note <id> <text>`,
`log delete <id>` (`log.go:94-210`), all calling `journalService.*`
(`internal/journal.Service`, operating on `~/.reckon/journal/YYYY-MM-DD.md` via
the old goldmark-based parser/writer — full-file regen, exactly the pattern
`docs/design/composable-redesign.md` calls out as the anti-pattern). It has
zero relationship to `internal/node`/`internal/index`/the vault
(`cfg.VaultDir`) or to `log/*.md`/`.gitattributes`'s `merge=union` scope.

**`addCmd` (`internal/cli/stubs.go:40-48`) is a v1 stub**, `Short: "Add a new
node to the vault"`, currently just `return fmt.Errorf("add: %w",
errNotImplemented)`. Its doc comment ("Add a new node (fact, task, event, …)")
matches the *old*, superseded spec's generic `rk add --kind log/progress/win/...`
capture primitive (`docs/reckon-spec_2026-06-15.md:96-118`) more closely than
anything in the current composable design doc.

**Concrete open question for planning (§0 item 2):** the ticket title says
"log tool + capture (rk log)" and its description literally says "rk log/add."
There is a real name collision to resolve:
- Does T4 **replace** `internal/cli/log.go`'s `logCmd`/`logAddCmd` (deleting
  the legacy journal-backed `add`, keeping or removing `note`/`delete`)  the
  same way `reckon-qiua` deleted the `todoCmd` *stub* from `stubs.go` — except
  here it's not a stub, it's working legacy code depended on by `rk today`/
  `rk week`/the TUI (`journalService` is also used by `today.go`, `week.go`,
  `tui`)?
- Does T4 instead graduate the **`addCmd` stub** into the real generic capture
  primitive (`rk add <text>` with `--kind`/`--at`/`--author`, per the old
  spec's alias-verb design) and leave legacy `rk log` alone for now, with a new
  `rk log` verb introduced separately (or `rk log` becoming a thin alias for
  `rk add --kind log` later, out of this ticket's scope)?
- Or does T4 add a *new* subcommand distinguishable by context (e.g. some
  vault-vs-legacy flag) — messy, likely wrong.

Nothing upstream decides this; it is a real scope call the plan.md must make
explicitly, the way `reckon-qiua`'s plan.md D1-D10 made explicit calls for
`rk todo`.

---

## 8. Config / paths / provenance conventions

**`internal/config/config.go`:**
```go
type Config struct {
    VaultDir string // git-synced content root
    CacheDir string // per-device index cache; must NOT be inside VaultDir
}
func Load() (*Config, error)
func LoadWithOverrides(vaultDir, cacheDir string) (*Config, error)
```
`VaultDir` defaults to `$RECKON_VAULT` else `$HOME/reckon`; `CacheDir` defaults
to `$RECKON_CACHE` else `$XDG_CACHE_HOME/reckon` else `$HOME/.cache/reckon`.
Every v1 tool (`todo.go`, `query.go`, `adopt.go`, `index.go`) calls
`config.LoadWithOverrides(vaultFlag, "")` at the top of its `RunE`. T4's log
file path is `filepath.Join(cfg.VaultDir, "log", <date>+".md")` — matches
`.gitattributes`'s `log/*.md` scoping and the design doc's
`log/2026-06-19.md` example. Note this is a **different tree** from the legacy
`config.JournalDir()` (`~/.reckon/journal/`, `config.go:42-54`) used by
`internal/journal` — confirms §7's "two separate systems" finding at the
config level too.

**Author/provenance stamping precedent** (`internal/cli/todo.go:209-222`,
reuse directly, it's a plain top-level func in the same package):
```go
// resolveAuthor determines the author string to stamp on a newly created
// node: --author flag > $RECKON_AUTHOR > $USER > "local". Always non-empty.
func resolveAuthor(flag string) string
```
Used as `author := resolveAuthor(todoAuthorFlag)`, then `NewNode(typ, author,
body)`/`n.Author = author`. T4 should call this exact function (already
package-visible in `internal/cli`) rather than reimplementing — "author
resolution" is identical for todo and log. `Node.Time` provenance precedent:
`n.Time = time.Now().UTC().Format(time.RFC3339)` (`todo.go:293`) — T4's "AT=
backfill" flag (see below) is the one new piece of time-handling logic needed.

**"AT= backfill for time"** — this phrase is inherited verbatim from the
**superseded** pre-composable spec, `docs/reckon-spec_2026-06-15.md:107,131-132`:
```
--at       HH:MM          # backfill ts to today HH:MM (or full RFC3339 for other days)
```
> `AT=HH:MM` semantics preserved from `work_system.sh`
> (`reference_work_system_at_override`): `ts` = backfilled, `created_at` = real
> insert time. Both retained — the projection orders by `ts`, the audit trail
> keeps `created_at`.

That old spec's schema had two separate columns (`ts` vs `created_at`); the
current `node.Node` has only one `Time` field and no "created_at" equivalent.
So T4 needs to decide: a `--at HH:MM`/`--at <RFC3339>` flag that overrides
`Node.Time` outright (losing the "audit trail" of real insert time), or a flag
that sets `Time` *and* stashes real insert time in a `Props["created_at"]` (or
similar) to preserve the old semantics. No existing code addresses this; it's
net-new for T4.

---

## 9. Docs / AGENTS.md files consulted

- `internal/node/AGENTS.md` — byte-preservation invariant, supported
  frontmatter/markdown subset, round-trip test corpus conventions
  (`TestRoundTripIdentity`/`FuzzRoundTripIdentity` must still pass after any
  change to parsing logic — the round-trip gate is non-negotiable per this doc
  and per `docs/design/spike-roundtrip-verdict.md`).
- `internal/index/AGENTS.md` — confirms `OpenWithParser` is *the* seam for
  T4 ("the log tool (T4) plugs in a group-file parser via `OpenWithParser`"),
  freshness model, stable views, ignore globs.
- `internal/cli/AGENTS.md` — general cobra conventions (mostly describes the
  *legacy* v0 command style — package-global service vars, `--quiet` handling,
  verb-alignment ticket reckon-89hp — largely superseded by the v1 pattern seen
  in `todo.go`/`query.go`/`adopt.go`, which this ticket should follow instead).
- `internal/journal/AGENTS.md` — full documentation of the legacy v0 journal
  file format/parser/writer this ticket's `rk log` must NOT resemble or share
  a file tree with (confirms the full-file-regen anti-pattern by name).
- `docs/design/composable-redesign.md` — the governing design doc; §552-555
  (group-file N+1 nodes design), §557-574 (parser contract + round-trip
  gating), §1330-1352 (canonical node spec history entry, "day→contains→entries"
  judgment default).
- `docs/design/composable-redesign-assessment.md` /
  `-rebuttal.md` — origin of `merge=union` + sort-on-reconcile + ULID-dedup
  concurrency answer (§3 above); also origin of the `author`/provenance field
  requirement.
- `docs/design/spike-roundtrip-verdict.md` — the PASSED gating spike this
  package was productionized from; lists `log-day` in its round-trip corpus.
- `docs/reckon-spec_2026-06-15.md` / `docs/reckon-redesign_2026-06-15.md` —
  **superseded** DB-first predecessor design (journal is OUTPUT, DB is truth —
  opposite of the shipped architecture). Only useful here as the origin of the
  `rk add --kind`/alias-verb UX phrasing and the literal `AT=HH:MM` flag
  semantics quoted in the ticket description; do not treat its schema
  (`entries` table, `ts`/`created_at` columns) as current.
- No `docs/REVIEW_PATTERNS.md` entries specific to node/index/parser pitfalls
  were found beyond generic Go/CLI patterns (error wrapping, `--quiet`,
  `os.Exit` avoidance) — nothing log/parser-specific has been logged there yet.

---

## 10. Existing tests — conventions to reuse

**`internal/cli/query_test.go`** (harness helpers, package `cli`, reuse
directly — same package, no import needed):
```go
func setupQueryVault(t *testing.T) (vault, cache string)          // query_test.go:54
func writeTestNode(t *testing.T, vault, filename, id, typ, body string, extraFM ...string) // :69
func resetCLIFlags()                                               // :92
func buildIndex(t *testing.T, vault string)                        // :104 (runs `rk index`)
func runQuery(t *testing.T, vault string, args ...string) (stdout, stderr string, err error) // :120
func countNDJSONLines(s string) int
```
**`internal/cli/adopt_test.go`**: `mustWriteFile`, `mustReadFile`,
`mustMkdirAll`, `isValidULID`, `crockfordAlphabet` — generic file/ULID test
helpers.

**`internal/cli/todo_test.go`** is the closest sibling ticket (reckon-qiua,
immediately prior, same shape: new create-path CLI family built on
`internal/node`+`internal/index`). Its header comment explicitly documents the
TDD-red convention this repo uses: write the test file first, referencing
result-struct shapes that don't compile yet, so the whole `cli` package fails
to *build* until the real `log.go`/whatever-it's-named exists — "not tests run
and fail, but the package does not build." A `runLog`/`runTodo`-style helper
(`RootCmd.SetArgs([]string{"log", "add", "--vault", vault, ...})` +
`RootCmd.Execute()`) is the expected pattern for T4's own test file, plus
`t.Cleanup(resetCLIFlags)`.

**`internal/index/index_test.go`**: `multiNodeParser` (test double,
`index_test.go:80-100`) proves the index already handles N nodes/file
generically; T4's real group parser is a superset of what that double faked.
Also: `TestReconcileDuplicateULIDWithinFile` (`index_test.go:580`) is relevant
precedent if entries end up with real inline ULIDs and something goes wrong
duplicating one within a single day file.

**`ticket-work/reckon-qiua/plan.md`** (immediately-preceding, merged sibling
ticket — PR #148) is the best template for T4's own plan.md structure (D1..Dn
numbered decisions, "Files to modify," "Test scenarios" grouped by AC, "Known
risks/ambiguities remaining"). Its own D7 explicitly flags "the single-global-
parser limitation remains a real, shared blocker for the still-open T4 log
tool" — confirming from the todo-ticket's own planning phase that this exact
gap (§2 above, `index.Open` vault-wide parser dispatch) was seen coming and
deliberately left for this ticket.

---

## 11. Summary of concrete open questions for planning

1. **Per-entry ULID/identity encoding in plain text** — no precedent exists;
   pick a mechanism (block-anchor reuse, surrogate-key-only, or something new)
   before writing the parser.
2. **`rk log` naming/collision with the legacy `internal/cli/log.go`
   (`journalService`-backed `add`/`note`/`delete`)** — decide replace vs.
   coexist vs. graduate `addCmd` instead; this determines which files get
   edited/deleted and whether `today.go`/`week.go`/`tui` (which also use
   `journalService`) are affected.
3. **Entry `Type` vs. `kind` prop** — is every entry `type: log-entry` with a
   `kind` prop (progress/win/log/...), or does `kind` become the literal
   `Type`? Affects both the parser and any `rk query`/`rk today` filtering.
4. **Vault-wide parser dispatch for `rk index`/`rk query`/any future `rk log
   list`** — `index.Open`'s hardcoded `MarkdownParser{}` needs to become (or
   be wrapped by) something that also splits `type: log-day` files; identify
   exactly which CLI call sites (`index.go`, and whichever new log-reading
   command T4 adds) need to switch to `OpenWithParser`.
5. **Does `rk log add`/`rk add` reconcile the index itself** (mirroring
   `todo.go`'s `list`), or does the "capture→index→query" AC rely on an
   explicit `rk index` step between capture and query? `rk query` (T3) itself
   never reconciles.
6. **Sort-on-reconcile + ULID-dedup** (concurrency insurance named in the T0
   design discussion) — in scope for this ticket's "Done when" list, or
   legitimately deferred? No code implements it today.
7. **`--at`/backfill semantics** — single-field override of `Node.Time`, or
   preserve the old spec's two-clock (`ts` vs `created_at`) model via a new
   prop? `Node` has no `created_at`-equivalent field today.
8. **`contains` edge from day→entries** — only resolvable once entry identity
   (open question 1) is settled; needs `Link{Rel:"contains", To: <id>}` on the
   day node, which requires re-parsing/updating the day node's `Links` on
   every append (or leaving it index-derived rather than stored — note
   `internal/node`'s own doc says nodes "store forward/authored facts only...
   all reverse/aggregate is index-derived, never stored," which argues
   `contains` might be better computed at query time rather than literally
   written into the day node's frontmatter on every append).
