# Codebase Analysis — reckon-qiua (v1-T5: `rk todo` durable + ephemeral)

Phase 1, read-only. Worktree: `/home/chadd/repos/reckon/.worktrees/reckon-qiua` (branch
`reckon-qiua`, based on `origin/main`). `go build ./...` is clean at HEAD.

## 0. Ticket restatement (grounded)

`rk todo add/list/done`. Durable todo = file-per-item node (own ULID, own file);
ephemeral todo = light property/group-file (no stable address). Props
`state`/`scheduled`/`deadline`; `depends` frontmatter prop → `depends-on` edges;
state changes via span-local edits. Done when: create/list/complete work; todo
files round-trip; depends-on edges show in the index; ephemeral-vs-durable is
queryable; tested.

Dependencies (both **closed**, so their foundations are available now):
`reckon-6giw` (v1-T1, `internal/node` canonical node package) and `reckon-pb82`
(v1-T2, `internal/index` SQLite property-graph index). `reckon-9bfx` (`rk adopt`)
is also closed and is the single best precedent in the repo for this ticket.
`reckon-uv09` (v1-T4, `rk log`) — the *other* tool that will need group-file
handling — is still **open** (not implemented). This matters: there is no
existing group-file CLI precedent to copy; `SplitEntries`/`ReplaceEntryBody`
exist in `internal/node/node.go` but are exercised only by unit tests today, not
by any CLI verb.

## 1. Files most likely touched

### New files (implementation will create)
- `internal/cli/todo.go` — the real `todoCmd` + subcommands (`add`, `list`,
  `done`), mirroring how `query.go`/`index.go` were added (see §5, "stub
  graduation").
- `internal/cli/todo_test.go` (or split `todo_add_test.go` etc.) — TDD red
  tests written before `todo.go` exists, exactly as `adopt_test.go` did for
  `rk adopt` (see §3).

### Existing files that must change
- `internal/cli/stubs.go` — the `todoCmd` **stub currently lives here** (lines
  50–59) and is already registered on `RootCmd` (`root.go:132`). It must be
  **deleted** from `stubs.go` when `todo.go` defines the real `var todoCmd`
  (duplicate `var todoCmd` in the same package will not compile otherwise).
  `errNotImplemented` (line 62) stays — `addCmd` still uses it.
- `internal/cli/root.go` — no change needed to the `RootCmd.AddCommand(todoCmd)`
  line itself (it already exists at line 132); it will just resolve to the new
  definition once the stub is removed. Double-check flag wiring is unaffected.

### Files to read as precedent, not modify
`internal/cli/adopt.go` (primary precedent), `internal/cli/query.go` +
`internal/cli/index.go` (secondary precedents), `internal/node/*.go`,
`internal/index/*.go`, `internal/output/output.go`, `internal/config/config.go`.

## 2. The precedent hierarchy (important correction to the prompt's framing)

`internal/cli/log.go` and `internal/cli/note.go` are **not** node-package
precedents — they are the **old v0 system** (`journalService`/`notesService`,
SQLite-primary, `internal/journal`/`internal/service` packages). They are useful
*only* for cobra scaffolding conventions (command tree shape, flag style,
`--quiet` handling, `getEffectiveDate()`), not for how to construct/write a
`node.Node`. Confirmed by grep: neither file imports `internal/node`.

The tool that actually uses `internal/node` + writes vault files directly is
**`internal/cli/adopt.go`** (`reckon-9bfx`, closed). It is the correct template
for `rk todo`: same package, same config resolution, same atomic-write helper,
same `node.Parse`/`node.Mint` calls, same `Annotations: {"requiresDB": "false"}`
convention, same `output.New(...).Print(res)` result pattern. **Read this file
in full before writing any todo code.**

`internal/cli/query.go` and `internal/cli/index.go` are the precedents for
*reading* the index (`config.LoadWithOverrides` → `index.DBPath`/`index.Open` →
SQL against the public views). `rk todo list` will likely need this shape (see
§6).

## 3. Test-harness precedent (for the TDD red-test phase)

`internal/cli/adopt_test.go` and `internal/cli/query_test.go` establish the
shared harness already in the `cli` package:
- `setupQueryVault(t) (vault, cache string)` — temp dirs + `t.Setenv("RECKON_CACHE", cache)`.
- `writeTestNode(t, vault, filename, id, typ, body, extraFM...)` — hand-writes a
  minimal frontmatter+body fixture file (id/type + optional extra lines).
- `resetCLIFlags()` — resets `vaultFlag`/`jsonFlag`/`ndjsonFlag`/`quietFlag` and
  clears `RootCmd`'s captured args/out/err. **Must be called between two
  `RootCmd.Execute()` calls in the same test** (shared singleton `RootCmd`).
- `buildIndex(t, vault)` — runs `rk index --vault <vault>` through `RootCmd`,
  fatals on error, then calls `resetCLIFlags()`.
- `runQuery(t, vault, args...) (stdout, stderr string, err error)` /
  `runAdopt(t, vault, args...) (stdout string, err error)` — the
  `RootCmd.SetArgs([]string{"<verb>", "--vault", vault, ...}); RootCmd.Execute()`
  pattern. A `runTodo` helper of the same shape is the natural fit.
- Tests use plain stdlib `testing` (`t.Fatalf`), not `testify`, even though
  `testify` is a go.mod dependency (used elsewhere, not in this newer code).
- `adopt_test.go`'s header comment is itself a model for how to write TDD-red
  test files: it documents that the file compiles against a not-yet-existing
  `adoptResult` type and *pins the expected struct shape* in a comment. Good
  pattern to repeat for whatever result-summary struct `rk todo add/list/done`
  return.

## 4. Exact interfaces / signatures the implementation will touch

### `internal/node` (package `node`)
```go
// Creation (render.go)
func NewNode(typ, author, body string) *Node   // mints ULID via Mint(); Time="" (caller sets it)
func (n *Node) Render() []byte                 // typed fields -> canonical markdown; NOT yet re-parsed

// Parsing / editing (node.go, insert.go)
func Parse(raw []byte) (*Node, error)
func ParseAt(raw []byte, loc Loc) (*Node, error)
func (n *Node) Serialize() []byte              // returns n.Raw verbatim
func (n *Node) SetField(key, value string) error   // existing scalar only; span-local splice + re-parse
func (n *Node) HasField(key string) bool
func (n *Node) InsertField(key, value string) error // missing scalar only; span-local splice + re-parse

// Group files (node.go, "Group files" section) — NOT currently used by any CLI verb
type Entry struct { Header string; Span Span }
func (n *Node) SplitEntries() []Entry
func (n *Node) ReplaceEntryBody(e Entry, newBlock string) ([]byte, error)

// Identity (ulid.go)
func Mint() string
func MintAt(t time.Time) string

// Types (node.go) — exported fields the CLI will set directly on a fresh NewNode
type Node struct {
    Raw []byte
    ULID, Type, Time, Author, Body string
    Aliases []string
    Props map[string]string   // state/scheduled/deadline live here
    Fragments []Fragment
    Links []Link              // depends-on edge lives here: {Rel, To, ToFrag}
    Loc Loc
    // unexported: frontmatter, fmOrder, fieldSpans, bodySpan
}
type Link struct { Rel, To, FromFrag, ToFrag string `json:"..."` }

// Parser interface (parser.go) — the per-tool keystone pair
type Parser interface {
    Parse(raw []byte, loc Loc) ([]*Node, error)
    Serialize(n *Node) ([]byte, error)
}
type MarkdownParser struct{}   // default: Parse returns exactly one *Node per file
```

**Create-path recipe** (explicitly spelled out in `render.go`'s doc comment,
directly applicable to durable-todo creation — no existing production caller
does this yet, so this ticket is the first to exercise it in anger):
```go
n := node.NewNode("todo", author, body)
n.Time = time.Now().Format(time.RFC3339)
n.Props = map[string]string{"state": "open"}           // + scheduled/deadline if given
if dependsOn != "" {
    n.Links = append(n.Links, node.Link{Rel: "depends-on", To: dependsOn})
}
raw := n.Render()
reparsed, err := node.Parse(raw)   // REQUIRED before any subsequent SetField — see render.go doc
// write reparsed.Serialize() (== raw) to todos/<n.ULID>.md via writeFileAtomic
```

### `internal/index` (package `index`)
```go
func Open(cfg *config.Config) (*Index, error)                       // hardcodes node.MarkdownParser{}
func OpenWithParser(cfg *config.Config, parser node.Parser) (*Index, error)
func DBPath(cfg *config.Config) (string, error)
func (ix *Index) Rebuild() (Stats, error)
func (ix *Index) Reconcile() (Stats, error)                          // NOT currently called by any CLI verb
func (ix *Index) DB() *sql.DB
func (ix *Index) Close() error
func Indexable(name string) bool          // exported, reused by rk adopt's dir walk
func ShouldSkipDir(name string) bool      // exported, reused by rk adopt's dir walk
```
Public read views (stable contract, `schema.go`): `nodes(id, ulid, type, time,
author, body, loc)`, `edges(src, rel, dst, dst_key, from_frag, to_frag)`,
`node_props(id, key, value)`, `aliases(alias, id)`, `fts(id, body)`,
`fts_search` (fts5 vtable, MATCH-capable).

### `internal/config`
```go
func LoadWithOverrides(vaultDir, cacheDir string) (*Config, error)
type Config struct { VaultDir, CacheDir string }
```

### `internal/output`
```go
func ModeFromFlags(jsonFlag, ndjsonFlag bool) (Mode, error)
func New(w io.Writer, mode Mode) *Writer
func (wr *Writer) Print(v any) error       // v implements Pretty() string for human mode
func (wr *Writer) PrintAll(vs []any) error
```

### CLI package-globals already in scope (`root.go`)
`vaultFlag`, `quietFlag`, `jsonFlag`, `ndjsonFlag`, `dateFlag` — all
`internal/cli` package-level vars the new `todo.go` will read directly (same
package), exactly as `adopt.go`/`query.go` do. `Annotations:
{"requiresDB":"false"}` is the convention for every v1 node-based verb (skips
the *old* SQLite service init in `initServiceE`; irrelevant to whether the verb
opens the *new* index).

### The atomic-write helper already exists and is reusable as-is
`writeFileAtomic(path string, data []byte) error` is defined in `adopt.go` but
is **unexported and package-private** — since `todo.go` will live in the same
`internal/cli` package, it can call it directly with zero duplication.

## 5. Stub graduation — exact mechanical precedent

`todoCmd` is **currently a registered stub**:
```go
// internal/cli/stubs.go:50-59
var todoCmd = &cobra.Command{
    Use: "todo", Short: "List open todo items",
    Annotations: map[string]string{"requiresDB": "false"},
    RunE: func(cmd *cobra.Command, args []string) error {
        return fmt.Errorf("todo: %w", errNotImplemented)
    },
}
```
and `root.go:132` already does `RootCmd.AddCommand(todoCmd)`.

Confirmed by git history: `queryCmd` and `indexCmd` went through **exactly this
same stub-then-graduate lifecycle**. At `89ca067` (v1-T0) all four
(`addCmd`/`todoCmd`/`queryCmd`/`indexCmd`) were stub blocks in `stubs.go`. When
T2/T3 landed, the `queryCmd`/`indexCmd` stub blocks were deleted from
`stubs.go` and real `var queryCmd = &cobra.Command{...}` / `var indexCmd =
...}` definitions were added in dedicated `query.go`/`index.go` files — `root.go`
needed **no change** since it already referenced the package-level var by name.
`rk todo` should follow the identical mechanical step: delete the `todoCmd`
block from `stubs.go`, add the real definition (with `Use: "todo"` and
`AddCommand` for `add`/`list`/`done` subcommands) in the new `todo.go`.

## 6. Concrete ephemeral-vs-durable mapping (the core design question)

### What the design doc actually says
`docs/design/composable-redesign.md`'s per-tool scope table (the authoritative
source the ticket paraphrases):

| Tool | file (storage atom) | node (addressable atom) | Notes |
|---|---|---|---|
| Todo (durable) | 1 file per todo | the todo (1:1) | deps are edges → IDs mandatory; fine git history wanted |
| Ephemeral task | 1 file per group (day / inbox) | the file/day only — **individual tasks NOT addressed** | ephemeral = no ID by design |

Governing rule: **"Addressability = durability."** A thing is a node iff you
might link to it, query it alone, or want its history — "ephemeral literally
means 'gets no stable address.'" The `Lean v1 scope` section and both the
assessment/rebuttal docs additionally describe the split as "a light todo
property/group-file, not heavy machinery" — i.e. the design deliberately leaves
room for a lightweight implementation, not a second parallel Node-splitting
subsystem.

### Answering the prompt's specific question: is `SplitEntries` a usable precedent?

**Partially, and only as a byte-splicing primitive — not as the mechanism for
making ephemeral items addressable.** `SplitEntries`/`ReplaceEntryBody` exist to
let a tool locate one `## `-headed block inside a group file and splice a
replacement over just that span, leaving siblings byte-identical (proven by
`TestGroupFileEditOneEntry` in `internal/node/node_test.go`). That is generically
useful for "find item N in the ephemeral file and toggle it" — a `rk todo done`
against an ephemeral item could use exactly this to flip `- [ ]` → `- [x]`
in-place without disturbing other lines.

**But** the way it is documented as the *log tool's* building block
(`parser.go`'s doc comment: "Group-file sub-node splitting with per-entry ULIDs
is a per-tool concern (the log tool, T4)") is precisely the case ephemeral todos
must **not** copy: T4's plan is to mint a ULID per log entry and return N+1
separate `*Node`s from its `Parser.Parse`, each becoming its own addressable row
in `_nodes`. Doing that for ephemeral todos would give every ephemeral item a
stable ULID/address — directly contradicting "ephemeral = no ID by design" and
the scope table's "individual tasks NOT addressed."

### Recommended concrete shape (grounded, not yet decided by planning — flag for Phase 2)

- **Durable todo**: `todos/<ULID>.md`, one file = one node, `type: todo`,
  frontmatter `state`/`scheduled`/`deadline` as plain scalar props, a
  ref-valued `depends-on: "[[<other ULID>]]"` prop. Created via
  `node.NewNode`/`Render` (§4 recipe); `done`/state changes via `SetField`
  exactly like `adopt.go`'s `SetField("id", id)` call. **No custom
  `node.Parser` needed** — the existing `MarkdownParser{}` (one node per file)
  already suffices, so `rk index`'s current hardcoded `index.Open(cfg)` (see
  §7 pitfall) needs no change for the durable side.
- **Ephemeral todo**: a single shared file, e.g. `todos/ephemeral.md` or a
  per-day `todos/inbox-<date>.md`. Individual items are plain body content
  (e.g. markdown task-list lines `- [ ] buy milk`), **not** split into separate
  `*Node`s by the tool's parser — the file indexes as ONE node (its `body` is
  FTS-searchable as a whole, satisfying "ephemeral is still findable" without
  making each line addressable). `SplitEntries`/regex on the body is a
  reasonable **internal editing primitive** for `rk todo done <n>` against an
  ephemeral item (locate the nth unchecked line, splice in place), but that is
  a todo-tool-local mechanism, not something that produces new `_nodes` rows.
  This also needs **no custom `node.Parser`** — plain `MarkdownParser{}`
  handles it (an id-less file keys on the `file:<relpath>` surrogate per
  `reconcile.go`'s `nodeKey`).
- **Querying the distinction**: since both shapes can go through
  `MarkdownParser{}` unmodified, "queryable ephemeral-vs-durable" reduces to a
  `type` or prop discriminator, e.g. `type: todo` (durable, has `id:`) vs.
  `type: todo-ephemeral` or a `durable: false` prop on the container node —
  either is directly expressible with what `node.go`/`schema.go` already
  provide (`SELECT * FROM nodes WHERE type = 'todo'` vs the ephemeral type/prop).
  This is a **planning-phase decision to make explicitly** (which literal
  discriminator to use); this analysis surfaces both viable options rather than
  picking one, since the ticket text ("light property/group-file") is
  compatible with either.

## 7. Known pitfalls / gaps to flag for planning-implementation

1. **`todoCmd` is a live stub, already wired into `RootCmd`.** Forgetting to
   delete it from `stubs.go` when adding the real `todoCmd` in `todo.go`
   produces a duplicate `var todoCmd` compile error. See §5.

2. **`index.Open(cfg)` (used by `rk index`, `internal/cli/index.go:33`) hardcodes
   `node.MarkdownParser{}`.** There is no per-directory/per-tool dispatch; only
   one global `node.Parser` backs a whole `rk index` run. `OpenWithParser`
   exists as an escape hatch but nothing in `internal/cli` calls it yet. As
   argued in §6, this ticket can likely avoid needing a custom parser entirely
   (both durable and ephemeral shapes fit `MarkdownParser{}`), which sidesteps
   this gap — but if the implementation *does* end up wanting group-splitting
   or edge-rel renaming via a custom `Parser`, this single-global-parser
   limitation is a real blocker shared with the (still-open) T4 log tool, and
   should be raised rather than silently worked around per-tool.

3. **`depends` → `depends-on` rel naming is not automatic.** `node.go`'s
   `deriveView` sets a ref-valued prop's edge `Rel` to the **literal frontmatter
   key** (`Link{Rel: k, ...}`), not a canonicalized name. The package doc is
   explicit that this is deliberate: "per-type rel vocab, e.g. `depends`→
   `depends-on`, is a per-tool parser's job" — i.e., generic `node.Parse` will
   never rename it for you. Two ways to get the ticket's literal
   `depends-on` edge rel: (a) simplest — name the frontmatter key itself
   `depends-on:` (a hyphen is legal in the key regex `[A-Za-z0-9_-]+`), so
   `Rel` comes out as `"depends-on"` with zero extra code; or (b) write a
   thin custom `Parser` that post-processes `Links` after calling
   `node.ParseAt`, at the cost of re-introducing pitfall #2's single-parser
   friction. Note the **existing test fixture** `todoItem` in
   `internal/node/node_test.go`/`internal/spike/roundtrip` uses a `depends:`
   key (not `depends-on:`) — that fixture is a generic ref-valued-prop
   exercise for the node package's own round-trip gate, not a pin on the real
   todo tool's frontmatter schema; it should not be read as mandating the
   literal key name `depends`.

4. **Lazy reconcile-on-read is designed but not wired up anywhere yet.**
   `Index.Reconcile()` exists and is unit-tested in `internal/index`, but grep
   across `internal/cli` shows **zero callers**. `rk query` opens the index
   read-only and, if the DB file doesn't exist, tells the user to run `rk index`
   first — it does not auto-reconcile. If `rk todo list` wants to see a
   just-written todo file without requiring a manual `rk index`, the
   implementation must explicitly call `Reconcile()` (read-write) before
   querying, or accept the same "run `rk index` first" contract `rk query`
   already has. Either is a legitimate choice — but don't assume freshness is
   automatic.

5. **No existing `author` resolution helper.** `node.NewNode(typ, author,
   body)` requires an author string, and the composable-redesign doc treats
   `author` as required-ish provenance, but nothing in `internal/cli` currently
   derives a default author (no env var, no git-config lookup, no OS-user
   fallback) outside test fixtures (`"mike"` is a hardcoded string in test
   data only). This is an open input the todo tool will need to decide
   (flag? env var? OS user via `os/user`?) — flagging for planning, not
   resolving here.

6. **CRLF files are out of scope**, per `adopt.go`'s existing pattern
   (`bytes.Contains(raw, []byte("\r\n"))` → refuse, citing `reckon-vj55`,
   "known parser-scope gap"). Any todo file read path should apply the same
   guard for consistency, and any file the todo tool *writes* should of course
   never introduce CRLF.

7. **`docs/REVIEW_PATTERNS.md`** — no todo/CLI-specific entries, but generic
   high-frequency anti-patterns to preflight against are directly applicable:
   unwrapped errors (`return err` without `fmt.Errorf(...: %w, err)` — 15
   occurrences historically), missing `--quiet` checks around
   `fmt.Print*` in RunE bodies, and `os.Exit()` inside command logic (must
   return an error instead, per `root.go`'s pattern of returning errors up to
   `Execute()`).

8. **`internal/node/node.go`'s package doc "FORMAT COUPLING" note** (lines
   21–28) states plainly that `SplitEntries`/group-file splitting is
   **markdown-ATX-header-specific** and is **not** expressed by the `Parser`
   interface — it's a `*Node` method, not pluggable per format. This is
   relevant only insofar as it confirms group-file handling (if used at all
   for the ephemeral shape) is inherently a todo-tool-local mechanism sitting
   on top of the shared primitive, not something the core node package
   generalizes further. Nothing to fix; just don't expect more structure than
   what's there.

9. **Atomicity of `writeFileAtomic`** (temp file in same dir + `os.Rename`) is
   already solved and battle-tested by `adopt.go` — reuse it verbatim rather
   than reinventing, including its mode-preservation behavior (falls back to
   `0o644` for a not-yet-existing file, which is exactly the durable-todo
   create case).

## 8. Summary of what to bring into planning (Phase 2)

- Precedent to clone structurally: `internal/cli/adopt.go` (creation/edit
  mechanics, config/output wiring, atomic write) + `internal/cli/query.go`
  (index read pattern, if `list` goes through the index) + the stub-graduation
  diff shape from `queryCmd`/`indexCmd`'s history.
- Test harness to reuse: `setupQueryVault`, `writeTestNode`, `resetCLIFlags`,
  `buildIndex`, and a new `runTodo` helper of the same shape as `runAdopt`/`runQuery`.
- Decisions Phase 2 must actually make (this analysis intentionally leaves
  these open, grounded but undecided):
  - Literal frontmatter key for the dependency edge (`depends-on:` recommended
    to avoid needing a custom `Parser`).
  - Literal discriminator for ephemeral vs. durable (`type` value vs. a
    boolean prop).
  - Exact ephemeral file path/naming convention (single `todos/ephemeral.md`?
    per-day `todos/inbox-YYYY-MM-DD.md`?).
  - Whether `rk todo list` reads via a raw directory walk (adopt-style) or via
    the index (query-style, matching the design doc's explicit statement that
    "per-tool list commands (`rk todo ls`) are thin sugar over [`rk query`]"),
    and whether it needs to call `Reconcile()` first for freshness.
  - Author-string source.
