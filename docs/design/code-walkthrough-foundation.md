# Code Walkthrough — Reckon Redesign Foundation (T0–T3)

> A guided tour of the completed foundation code, written for two purposes:
> **(1) learning Go** (you're an experienced dev, new to the language — Go-specific
> idioms are flagged in `> GO:` callouts) and **(2) reviewing the implementation**
> (architecture + why-it's-shaped-this-way, with `> ⚠ REVIEW NOTE:` asides for
> tradeoffs and known limits). Tour order follows the **data**, not the file list:
> a file's bytes → a `Node` → the index → a query.
>
> Companion design docs: `composable-redesign.md` (the why), `spike-roundtrip-verdict.md`
> (the keystone proof), `foundation-review-2026-06-24.md` (independent review).

## 0. Orientation

### The one-paragraph architecture

Markdown files in a vault are the **source of truth**. The `node` package turns a
file's *bytes* into a canonical `Node` (and back) **without losing a byte**. The
`index` package projects all nodes into a **disposable SQLite property-graph**
(rebuildable from text, never synced). `rk query` reads that index over a
**read-only** SQL surface. Everything an agent or human does is one of: parse,
render, index, query. That's the whole spine.

```
   vault/*.md  ──Parse──►  node.Node  ──Render/Serialize──►  vault/*.md
   (truth)                  │  ▲                              (truth)
                            │  │
                       reconcile (index pkg)
                            ▼
                   SQLite index (cache, disposable)
                            ▲
                       rk query (read-only SQL)
```

### Package map

| Package | Role | Depth here |
|---|---|---|
| `internal/node` | **keystone** — byte-preserving parse/serialize/render + NDJSON envelope + ULID | **deep** |
| `internal/index` | derived SQLite property-graph; reconcile-on-read | **deep** |
| `internal/cli/query.go` | `rk query` — read-only SQL read surface | **deep** |
| `internal/config` | pure vault/cache path resolution | light |
| `internal/output` | json/ndjson/pretty `Writer` | light |
| `internal/cli/dispatch.go` | git-style `rk-<name>` PATH dispatch | light |
| `internal/spike/roundtrip` | the gating spike `node` grew out of | light |
| `cmd/rk/main.go` | the binary entry point | light |

> GO: **Project layout.** `cmd/<binary>/main.go` is the conventional home for an
> executable's `main`; everything reusable lives under `internal/`. The Go
> toolchain special-cases `internal/`: packages under it can only be imported by
> code rooted at the same parent — a *compiler-enforced* "private to this module"
> boundary. The import path `github.com/MikeBiancalana/reckon/internal/node` comes
> from the `module` line in `go.mod`; there is no separate include path or build file.

---

## 1. The keystone — `internal/node`

This is the package to understand first; the design literally reduces to it. The
core bet (proven by the spike): **keep the original bytes authoritative; derive a
typed view alongside them; edit by splicing, never by regenerating.**

### 1.1 The `Node` struct (`node.go`)

```go
type Node struct {
	Raw []byte            // authoritative bytes — Serialize returns this

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

The split here *is* the design:

- **`Raw`** is the truth. `Serialize()` returns it verbatim, so
  `serialize(parse(f)) == f` holds **by construction** — no clever regeneration.
- The exported fields (`ULID`…`Loc`) are a **derived, read-mostly projection** for
  consumers (the index, tools, the agent).
- The **unexported** fields (`frontmatter`, `fmOrder`, `fieldSpans`, `bodySpan`)
  are parse bookkeeping — byte offsets that let an edit target exactly one field.

> GO: **Exported vs unexported = capitalization.** An identifier starting with an
> uppercase letter is exported (visible outside the package); lowercase is package-
> private. There is no `public`/`private` keyword — the case *is* the access
> modifier. So `Raw` is part of the API; `fieldSpans` is an implementation detail
> the index can't even see.

> GO: **Slices and maps.** `[]byte` is a slice (a view: pointer + length + cap over
> a backing array); `map[string]string` is a hash map. Both are reference-like —
> passing a `*Node` around shares the same `Raw` backing array rather than copying
> kilobytes of markdown. Their zero values are `nil`, and a `nil` slice/map reads
> fine (len 0); only *writing* to a `nil` map panics, which is why you'll see
> `if n.Props == nil { n.Props = map[string]string{} }` before a write.

> GO: **`Span` is `struct{ Start, End int }`** — a tiny value type. Go structs are
> value types (copied on assignment), unlike a class instance. Copying a `Span` is
> two ints; copying a `Node` would copy the slice/map *headers* (cheap) but share
> the same backing data — which is why methods that mutate use a pointer receiver
> (next).

### 1.2 Parsing — byte-preserving with spans

`Parse` → `ParseAt(raw, loc)` does four things: reject malformed files, locate the
frontmatter block (recording each field's byte span), set the body span, and derive
the typed view.

```go
func ParseAt(raw []byte, loc Loc) (*Node, error) {
	for _, m := range conflictMarkers {
		if bytes.Contains(raw, []byte(m)) {
			return nil, fmt.Errorf("refusing to parse: file contains conflict marker %q", strings.TrimSpace(m))
		}
	}
	n := &Node{Raw: raw, Loc: loc, frontmatter: map[string]string{}, fieldSpans: map[string]Span{}}
	bodyStart := parseFrontmatter(n, raw)
	n.bodySpan = Span{Start: bodyStart, End: len(raw)}
	n.Body = string(raw[bodyStart:])
	deriveView(n)
	extractBody(n, raw, bodyStart)
	return n, nil
}
```

> GO: **Multiple return values + error-as-value.** `(*Node, error)` is *the* Go
> idiom. There are no exceptions; functions return an `error` (an interface; `nil`
> means success) and the caller checks it. You'll see `if err != nil { return …,
> err }` everywhere — verbose, but the control flow is always explicit and local.

> GO: **`&Node{…}` is a composite literal taking its address** — it allocates a
> `Node` and yields a `*Node` in one step. Go decides stack vs heap via escape
> analysis; you never `malloc`/`free`. Returning a pointer to a local is safe and
> normal (it "escapes" to the heap automatically).

> GO: **`for _, m := range conflictMarkers`** — `range` yields (index, value); `_`
> discards the index. `conflictMarkers` is a package-level `[]string`. This is the
> universal Go loop.

`parseFrontmatter` walks the lines between the opening `---\n` and the closing
`---`, and for each `key: value` line records the **byte offset of the value**:

```go
if m := fmScalarRe.FindStringSubmatchIndex(line); m != nil {
	key := line[m[2]:m[3]]
	valStart := pos + m[6]
	valEnd := pos + m[7]
	...
	n.frontmatter[key] = string(raw[valStart:valEnd])
	n.fieldSpans[key] = Span{Start: valStart, End: valEnd}
}
```

`fmScalarRe` is `^([A-Za-z0-9_-]+):([ \t]*)(.*?)([ \t]*)$`. `FindStringSubmatchIndex`
returns *byte offsets* of each capture group (pairs in a flat `[]int`: `m[6]:m[7]`
is group 3, the value), which is exactly what we need to compute the absolute span
within `raw`.

> ⚠ REVIEW NOTE: **single-line scalar frontmatter only** (`node.go`, the `fmScalarRe`
> comment). Block-style YAML (`aliases:\n  - a`) is invisible to the typed view —
> `Raw` is still byte-safe, but the index would miss those aliases. Same class:
> CRLF files fail the `---\n` prefix check (everything becomes body). Both are
> tracked in `reckon-vj55`, to fix before the note tool (T8) leans on links.

### 1.3 Deriving the view — link routing

`deriveView` projects raw frontmatter into typed fields, and encodes **spec
invariant 3** (how edges are formed):

```go
for _, k := range n.fmOrder {
	if reservedKeys[k] {
		continue
	}
	val := n.frontmatter[k]
	if to, frag, ok := parseRefValue(val); ok {
		n.Links = append(n.Links, Link{Rel: k, To: to, ToFrag: frag})
		continue
	}
	if n.Props == nil {
		n.Props = map[string]string{}
	}
	n.Props[k] = val
}
```

So a **ref-valued prop** (`depends: "[[X]]"`) becomes a *typed edge* with `Rel` =
the key; a plain scalar becomes a `Prop`; reserved keys (`id`/`type`/`time`/`author`/
`aliases`) are promoted to dedicated fields. Body `[[links]]` are added separately
by `extractBody` as `Rel: "references"`, skipping fenced code blocks so a `[[x]]`
inside a ```` ``` ```` block is inert.

> GO: **`append(slice, x)` returns a (possibly new) slice** and you must assign it
> back (`n.Links = append(n.Links, …)`). `append` may reallocate when capacity runs
> out; forgetting the assignment is a classic Go bug. `reservedKeys` is a
> `map[string]bool` used as a set — `reservedKeys[k]` returns the zero value
> `false` for absent keys, so no membership check is needed.

> ⚠ REVIEW NOTE: link rel round-trips on the convention that **`"references"` means
> "came from the body."** `Render` (below) relies on it to decide what to re-emit
> in frontmatter. It's a clean rule, but it's a *naming convention doing structural
> work* — worth a comment at the `Link` type, and care if a future tool ever mints a
> typed body link.

### 1.4 The two write paths — `Serialize`, `SetField`, `Render`

There are **two** ways a node becomes text, and the distinction is the whole point.

**Edit path** — `SetField` splices into a known span and re-parses:

```go
func (n *Node) SetField(key, value string) error {
	span, ok := n.fieldSpans[key]
	if !ok {
		return fmt.Errorf("SetField: no scalar span for %q (existing scalar keys only)", key)
	}
	out := make([]byte, 0, len(n.Raw)-(span.End-span.Start)+len(value))
	out = append(out, n.Raw[:span.Start]...)
	out = append(out, value...)
	out = append(out, n.Raw[span.End:]...)

	reparsed, err := ParseAt(out, n.Loc)
	if err != nil {
		return fmt.Errorf("SetField: re-parse after splice failed: %w", err)
	}
	*n = *reparsed
	return nil
}
```

> GO: **Pointer receiver `(n *Node)`** lets the method mutate the caller's value. A
> *value* receiver would operate on a copy and the change would vanish. The
> `*n = *reparsed` line dereferences both pointers and copies the whole struct in
> place — so the caller's `Node` is replaced wholesale by the freshly-parsed one
> (refreshing all spans). `make([]byte, 0, cap)` pre-sizes the backing array (len 0,
> the given cap) to avoid re-growth during the three appends. `n.Raw[:span.Start]...`
> spreads a slice into `append`'s variadic args.

> GO: **`%w` vs `%v` in `fmt.Errorf`.** `%w` *wraps* the underlying error so callers
> can later `errors.Is`/`errors.As` to inspect the chain; `%v` just formats it as
> text. Wrapping preserves the cause; you'll see `%w` used deliberately throughout.

**Create path** — `Render` builds canonical text from the *typed fields only*
(`render.go`; this is the H1 fix). It's the inverse of `deriveView`+`extractBody`,
reading no parse internals so it works on a freshly-minted node:

```go
func (n *Node) Render() []byte {
	var fm []string
	add := func(k, v string) { fm = append(fm, k+": "+v) }
	if n.ULID != "" { add("id", n.ULID) }
	if n.Type != "" { add("type", n.Type) }
	...
	for _, l := range typed { // links where Rel != "references"
		ref := l.To
		if l.ToFrag != "" { ref += "#" + l.ToFrag }
		add(l.Rel, strconv.Quote("[["+ref+"]]"))
	}
	var out bytes.Buffer
	if len(fm) > 0 {
		out.WriteString("---\n"); out.WriteString(strings.Join(fm, "\n")); out.WriteString("\n---\n")
	}
	out.WriteString(n.Body)
	return out.Bytes()
}
```

> GO: **`add` is a closure** — a local function value that captures `fm` by
> reference, so each call appends to the same slice. `bytes.Buffer` is the idiomatic
> grow-able byte builder (cheaper than repeated string `+`). `strconv.Quote` produces
> a double-quoted, escaped string — here it makes `"[[X]]"`, which is valid YAML for
> Obsidian while the parser strips the quotes on the way back in.

The two paths together are why `node` is the keystone: **edits preserve unmodeled
bytes; creates produce text that is itself round-trip-stable.** The gate tests in
`render_test.go` assert `parse(render(n))` reproduces the node, `serialize(parse(
render(n))) == render(n)`, and render idempotence.

### 1.5 The Parser interface (`parser.go`)

```go
type Parser interface {
	Parse(raw []byte, loc Loc) ([]*Node, error)
	Serialize(n *Node) ([]byte, error)
}

type MarkdownParser struct{}
func (MarkdownParser) Parse(raw []byte, loc Loc) ([]*Node, error) { … }
func (MarkdownParser) Serialize(n *Node) ([]byte, error) { return n.Serialize(), nil }

var _ Parser = MarkdownParser{}
```

This is the "each tool ships a parse/serialize pair; the core stays generic"
contract, in Go form.

> GO: **Interfaces are satisfied structurally (implicitly).** `MarkdownParser`
> never says "implements Parser"; it just *has* the methods, so it qualifies. This
> is duck typing checked at compile time. `var _ Parser = MarkdownParser{}` is a
> common idiom: a throwaway assignment to the blank identifier `_` that makes the
> compiler **verify** `MarkdownParser` satisfies `Parser` — a zero-cost
> compile-time assertion, no runtime effect.

> GO: **An empty struct `struct{}`** has size zero — `MarkdownParser` carries no
> state, it's just a method holder. The methods use a *value* receiver
> `(MarkdownParser)` since there's nothing to mutate.

### 1.6 The envelope (`envelope.go`) — NDJSON interchange

```go
type Envelope struct {
	V       int               `json:"v"`
	ULID    string            `json:"ulid,omitempty"`
	Type    string            `json:"type"`
	...
	Aliases []string          `json:"aliases,omitempty"`
	Props   map[string]string `json:"props,omitempty"`
}
```

> GO: **Struct tags** — the backtick strings after each field — are metadata read by
> `encoding/json` via reflection. `json:"ulid,omitempty"` renames the field to
> `ulid` in JSON and drops it when empty (so an absent ULID doesn't emit a `"ulid":""`).
> `encoding/json` only sees *exported* fields, which is why the envelope mirrors the
> exported `Node` fields.

`WriteNDJSON` enforces the one-physical-line invariant:

```go
if bytes.ContainsRune(data, '\n') {
	return fmt.Errorf("WriteNDJSON: encoded envelope contains a literal newline")
}
```

This can't actually trigger (JSON escapes newlines to `\n`), but it's a cheap
guardrail asserting the property the whole NDJSON pipe relies on.

> ⚠ REVIEW NOTE: there are *two* NDJSON emitters — this guarded one and the generic
> `output.Writer` NDJSON mode (no guard). Node-emitting commands should route through
> `node.WriteNDJSON`. Tracked as L2 on T3.

### 1.7 ULID minting (`ulid.go`) — package state done right

```go
var (
	mintMu      sync.Mutex
	mintEntropy = ulid.Monotonic(rand.Reader, 0)
)

func MintAt(t time.Time) string {
	mintMu.Lock()
	defer mintMu.Unlock()
	id, err := ulid.New(ulid.Timestamp(t), mintEntropy)
	if err != nil {
		id = ulid.MustNew(ulid.Timestamp(t.Add(time.Millisecond)), mintEntropy)
	}
	return id.String()
}
```

> GO: **Package-level state guarded by a `sync.Mutex`.** `mintEntropy` is shared
> mutable state (the monotonic generator); concurrent `Mint()` calls would race on
> it. `mintMu.Lock()` + `defer mintMu.Unlock()` serialise access. **`defer`** runs
> the unlock when the function returns *no matter how* (including the error path) —
> the canonical Go pattern for "release this no matter what." The `var ( … )` block
> groups package-level declarations; both are initialised once at program start.

> GO: **`MustNew`** — the `Must…` prefix is a Go convention for "panic instead of
> returning an error," reserved for cases that can't fail in practice. Here it's a
> >2^80-mints-per-ms overflow guard that's unreachable.

---

## 2. The derived index — `internal/index`

Markdown is truth; this package is a **rebuildable projection** for fast query and
graph traversal. It lives in the cache dir, never in the vault, and is never synced
(a synced live SQLite file corrupts).

### 2.1 Opening and the auto-rebuild contract (`index.go`)

```go
func OpenWithParser(cfg *config.Config, parser node.Parser) (*Index, error) {
	id, err := vaultID(cfg.VaultDir)            // sha256(abs path)[:12]
	...
	db, err := sql.Open("sqlite", dbPath+"?_journal=WAL&_timeout=5000")
	...
	ix := &Index{db: db, cfg: cfg, vaultID: id, dir: dir,
		lockPath: filepath.Join(dir, "index.lock"), parser: parser}
	if err := ix.ensureSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return ix, nil
}
```

`ensureSchema` compares the persisted `schema_version` to the code's `SchemaVersion`;
if missing or different, it calls `Rebuild()` — a full, from-text rebuild. **There
are no migrations**: the index is derived, so a schema bump just throws it away and
regenerates.

> GO: **`import _ "modernc.org/sqlite"`** (blank import, in `index.go`). The `_`
> imports the package solely for its **side effect**: its `init()` registers a
> driver named `"sqlite"` with `database/sql`. Then `sql.Open("sqlite", …)` finds it
> by that string. `modernc.org/sqlite` is a *pure-Go* SQLite (no cgo) — important
> for cross-compilation and a clean build.

> GO: **`vaultID` uses `sha256.Sum256` returning a `[32]byte` array** (fixed size,
> a value type), then `hex.EncodeToString(sum[:])` — `sum[:]` slices the array into
> a `[]byte`. Distinct vaults → distinct cache subdirs, deterministically.

### 2.2 The query contract — private tables, public views (`schema.go`)

```sql
CREATE TABLE _nodes ( node_key TEXT PRIMARY KEY, ulid TEXT …, body TEXT …, … );
CREATE TABLE _edges ( src_key …, rel …, dst …, dst_key TEXT, … );
...
CREATE VIRTUAL TABLE fts_search USING fts5(id UNINDEXED, body);

CREATE VIEW nodes AS SELECT node_key AS id, ulid, type, … FROM _nodes;
CREATE VIEW edges AS SELECT src_key AS src, rel, dst, dst_key, … FROM _edges;
CREATE VIEW node_props AS SELECT node_key AS id, key, value FROM _props;
CREATE VIEW aliases AS SELECT alias, node_key AS id FROM _aliases;
CREATE VIEW fts AS SELECT id, body FROM fts_search;
```

This is **plumbing/porcelain applied to the database**: the `_`-prefixed physical
tables are private and may be refactored freely; the **views are the stable public
contract** that `rk query` (and agents) target. `_edges.dst` holds the *raw* link
target (a ULID or an alias); a resolver pass fills `dst_key` (NULL = dangling).

> Note the FTS shape: `fts_search` is a real **fts5 virtual table** (supports
> `MATCH`), while the `fts` view exposes only its columns for plain scans. A view
> can't forward `MATCH`, so full-text search must hit `fts_search` directly — a
> distinction `rk query` validation knows about (§3.2). This split was the
> `reckon-a4eh` follow-up.

### 2.3 Reconcile — mark-and-sweep, hash-authoritative (`reconcile.go`)

`Reconcile()` is the lazy reconcile-on-read; `Rebuild()` is the full version. Both
delegate to `reconcileTx`, which walks the vault and, per file:

```go
mtime := info.ModTime().UnixNano()

// Fast-path: mtime unchanged -> trust stored nodes.
if prev, ok := stored[rel]; ok && prev.mtime == mtime {
	markPresent(present, prev.ulids); return nil
}
raw, err := os.ReadFile(path)
h := hashBytes(raw)
// Content identical though mtime moved (git/Syncthing): refresh mtime only.
if prev, ok := stored[rel]; ok && prev.hash == h {
	touchMtime(tx, rel, mtime); markPresent(present, prev.ulids); return nil
}
// New or changed: (re)parse and upsert this file's nodes.
keys, perr := ix.indexFile(tx, rel, raw, h, mtime)
if perr != nil {
	logger.Warn("index: skipping unparsable file", "path", rel, "err", perr)
	deleteFileMeta(tx, rel); return nil   // skip, retry next pass, sweep old nodes
}
```

The shape worth absorbing:

- **mtime is a fast-path, the content hash is the authority** — exactly handling
  git/Syncthing, which rewrite mtimes without changing content.
- **Mark-and-sweep**: every file marks the node keys it produces into `present`;
  afterward `sweepKeys(present)` deletes any key no longer present. This makes the
  pass **order-independent and idempotent** — a file only ever touches its own rows.
- **Malformed files are skipped and logged, never fatal** — a single bad file (git
  conflict markers, a `.sync-conflict` copy) can't break the reconcile.

> GO: **`filepath.WalkDir` takes a callback** — `func(path, d, err) error`. Returning
> the sentinel `filepath.SkipDir` prunes a directory; returning a real error aborts
> the walk. The callback here is a **closure** capturing `tx`, `present`, `stored`,
> `st` — so it accumulates results without globals. Note it checks the incoming
> `err` first: WalkDir passes per-entry errors *into* the callback rather than
> stopping.

> GO: **The transaction + `defer` idiom.** `tx, _ := db.Begin()`, then
> `defer tx.Rollback()` immediately. On the happy path `tx.Commit()` runs and the
> later `Rollback()` becomes a harmless no-op (committing already closed the tx); on
> any early `return err`, the deferred `Rollback` undoes everything. This "defer the
> rollback, commit at the end" pattern guarantees you never leak an open transaction.

> GO: **`comma, ok` map reads** — `prev, ok := stored[rel]` returns the value and a
> bool for "was it present." Distinguishes "absent" from "present but zero," which a
> bare `stored[rel]` couldn't.

### 2.4 Cross-process locking via build tags (`lock_unix.go` / `lock_other.go`)

```go
//go:build unix
package index
func (ix *Index) lock() (func(), error) {
	f, _ := os.OpenFile(ix.lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	unix.Flock(int(f.Fd()), unix.LOCK_EX)
	return func() { unix.Flock(int(f.Fd()), unix.LOCK_UN); f.Close() }, nil
}
```
```go
//go:build !unix
package index
func (ix *Index) lock() (func(), error) { return func() {}, nil }   // no-op
```

> GO: **Build constraints (`//go:build unix`).** The first line of each file is a
> build tag; the toolchain compiles `lock_unix.go` only on unix and `lock_other.go`
> elsewhere. Both define the *same* method `lock()` — exactly one is compiled, so
> there's no conflict. This is Go's standard way to do per-platform implementations
> without `#ifdef`. `lock()` **returns a `func()`** — the release closure — so callers
> write `unlock, _ := ix.lock(); defer unlock()`. Returning a cleanup function is a
> very common Go pattern (you saw it implied by `defer` throughout).

> ⚠ REVIEW NOTE: on non-unix the lock is a no-op, so cross-process reconcile
> coalescing is unavailable there (WAL still protects the SQLite writes themselves).
> The dev target is linux/darwin, so this is accepted (L3).

---

## 3. The read surface — `internal/cli/query.go`

`rk query` is the agent+human retrieval surface: **read-only SQL over the public
views**, output as canonical node NDJSON by default.

### 3.1 Two-layer read-only safety

**Layer 1 — open the database read-only:**

```go
u := url.URL{Scheme: "file", Path: abs}
u.RawQuery = "mode=ro"
db, err := sql.Open("sqlite", u.String())
```

> ⚠ REVIEW NOTE (and a great Go-stdlib gotcha): the `file:` URI form is **mandatory**.
> `modernc.org/sqlite` *silently strips* the query string (`?mode=ro`) from a
> non-`file:` DSN and opens read-write. Building the DSN via `url.URL` guarantees the
> `file:` scheme. This is the kind of thing that's invisible until it bites — good
> that it's commented at the function.

**Layer 2 — validate the statement** (defense-in-depth, and friendlier errors):

```go
func validateReadOnlySQL(raw string) error {
	scrub := scrubSQL(raw)                 // blank out comments + string literals first
	...
	first := wordRe.FindString(upper)
	if first != "SELECT" && first != "WITH" { return … }   // only SELECT/CTE
	if kw := denyRe.FindString(upper); kw != "" { return … } // no INSERT/UPDATE/DROP/PRAGMA/…
	if tbl := privateRe.FindString(noTrail); tbl != "" { return … } // no _-private tables
	if tbl := ftsShadowRe.FindString(noTrail); tbl != "" { return … } // no fts5 shadow tables
	return nil
}
```

The ordering matters: `scrubSQL` neutralises comments and string literals **first**,
so a keyword like `DELETE` *inside a string literal* can't trip `denyRe`. The
regexes encode the contract: SELECT/CTE only, no write/DDL keywords, no `_private`
tables, no fts5 internal shadow tables — but `fts_search` itself (the sanctioned
`MATCH` surface) is deliberately allowed.

> GO: **`regexp.MustCompile` at package scope.** Compiling a regex is expensive;
> doing it once in a `var (…)` block (and `Must…`-panicking on a bad pattern at
> startup) is idiomatic. `\b` word boundaries keep `denyRe` from matching `CREATED`
> when it means `CREATE`.

> ⚠ REVIEW NOTE: defense-in-depth done right — the read-only connection is the real
> guarantee; the regex layer exists for *good error messages*, not as the primary
> defense. Regex-only SQL filtering would be a smell; here it's explicitly the
> friendly layer over an OS-level lock.

### 3.2 Output — reconstructing canonical nodes

By default the result rows (each with an `id`) are turned back into full canonical
node envelopes via per-row follow-up queries (`reconstructEnvelopeObject` →
`loadProps`, `loadAliases`), so `rk query … | rk note import` round-trips through
the same NDJSON shape. `--raw` skips that and emits literal rows; `--fields` projects
columns; `--limit` caps results.

> GO: **`map[string]any` and the type switch.** Rows come back as `map[string]any`
> (`any` is the alias for `interface{}` — the empty interface, "any type"). To use a
> value you assert its concrete type, often via a `switch v := x.(type) { case
> string: … case int64: … }`. You'll see this in `normalizeValue`/`scanRows`,
> bridging SQLite's dynamic typing to JSON.

### 3.3 Cobra command wiring — a shared-singleton footgun

```go
var queryCmd = &cobra.Command{
	...
	Annotations:  map[string]string{"requiresDB": "false"},
	SilenceUsage: true,
	RunE: runQueryE,
}

func runQueryE(cmd *cobra.Command, args []string) error {
	defer resetQueryFlags(cmd)   // ← important
	...
}
```

> GO / Cobra: commands are built once and live as package singletons. Because flag
> *values* are package vars bound to the shared command, tests (or repeated
> invocations in-process) would leak state between runs. `defer resetQueryFlags(cmd)`
> restores the flag vars **and** clears pflag's `Changed` bits after each run. The
> `Annotations` map + a `PersistentPreRunE` elsewhere gate whether a command needs
> the DB initialised — `requiresDB:false` because `rk query` opens its own read-only
> handle. `SilenceUsage:true` stops Cobra dumping the full help on a runtime error
> (you want the error, not the manual).

---

## 4. The periphery (lighter)

### 4.1 `internal/config` — pure resolution

`LoadWithOverrides(vaultDir, cacheDir)` resolves the vault and cache directories
from args → env (`RECKON_VAULT`/`RECKON_CACHE`/`XDG_CACHE_HOME`) → home defaults,
and **rejects a cache dir inside the vault** (it would get git-synced):

```go
rel, err := filepath.Rel(vaultDir, cacheDir)
if err == nil && (rel == "." || !strings.HasPrefix(rel, "..")) {
	return nil, fmt.Errorf("config: cache dir %q must not be inside vault %q (EC-7)", …)
}
```

> GO: **resolution is *pure*** — `Load*` creates no directories (a deliberate
> principle: resolving a path shouldn't have filesystem side effects).
> `filepath.Rel` returning a path that doesn't start with `..` means `cacheDir` is
> inside `vaultDir` — a neat containment check.

> ⚠ REVIEW NOTE: the file still carries the *legacy* `DataDir()/JournalDir()/…`
> funcs (pre-redesign, `~/.reckon`, and they `mkdir` as a side effect) alongside the
> new pure `Config`. Both default to `~/.reckon` → an overlap/migration hazard.
> Tracked as M5 (`reckon-lrw2`), to resolve before the log tool writes to the vault.

### 4.2 `internal/output` — one `Writer`, three modes

`Writer.Print(v any)` switches on `Mode`: JSON/NDJSON marshal `v`; Pretty prefers a
`Pretty() string` method, then `fmt.Stringer`, then `%v`.

> GO: **Interface-based polymorphism without inheritance.** `prettyPrinter` is a
> one-method interface; the type switch `case prettyPrinter:` asks "does this value
> have a `Pretty()` method?" at runtime. `fmt.Stringer` (the stdlib `String()`
> interface) is the fallback. Small, composable interfaces > class hierarchies —
> very Go.

### 4.3 `internal/cli/dispatch.go` — git-style extension point

`maybeDispatch` implements `rk foo` → exec `rk-foo` on PATH if no builtin matches —
the `rk-<name>` extension seam from the design.

```go
if err := syscall.Exec(path, argv, os.Environ()); err != nil {
	return fmt.Errorf("exec %q: %w", path, err)
}
```

> GO: **`syscall.Exec` *replaces* the current process** (it's `execve`, not
> fork+exec) — on success it never returns; the child *becomes* `rk`. Contrast with
> `os/exec.Command` which spawns a child and waits. Process replacement is exactly
> right for a transparent dispatcher.

> GO: **Sentinel errors + `errors.Is`.** `errExtNotFound = errors.New(…)` is a
> package-level sentinel; callers use `errors.Is(lerr, errExtNotFound)` to branch on
> "not found" vs a real failure (e.g. found-but-not-executable). This is why
> wrapping with `%w` earlier matters — `errors.Is` walks the wrapped chain.

> GO: deliberately **not** using `exec.LookPath` (commented) because it hides the
> found-but-not-executable case — a nice example of choosing the lower-level call
> for a clearer error.

### 4.4 `cmd/rk/main.go` — the whole entry point

```go
func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
```

> GO: `main` takes no args and returns nothing; `os.Exit(1)` sets the exit code.
> All real logic lives in `internal/cli` so it's testable (you can call `Execute`
> from a test; you can't easily test `main`). Note: `os.Exit` skips deferred funcs,
> which is why the library code avoids `os.Exit` and returns errors up to here.

### 4.5 `internal/spike/roundtrip` — where it began

This is the original gating spike (`reckon-k9ff`) that *proved* byte-preserving
write-back before any of `node` existed. It's kept as the historical proof; `node`
is its productionised form. Worth reading right after §1 to see the idea in its
smallest form.

---

## 5. Cross-cutting Go idioms (recap)

The patterns you'll see repeated, now that you've met them in context:

- **Errors are values.** `(T, error)` returns; `if err != nil { return …, fmt.Errorf("…: %w", err) }`. Wrap with `%w`, inspect with `errors.Is`/`As`. No exceptions.
- **`defer` for cleanup.** Unlock mutexes, roll back transactions, release flocks, reset flags — scheduled at acquire time, runs on every return path.
- **Small interfaces, implicit satisfaction.** `Parser`, `prettyPrinter`, `fmt.Stringer`. Types satisfy them by having the methods; `var _ I = T{}` asserts it at compile time.
- **Exported = Capitalized.** The API surface is literally the capitalized identifiers; everything else is private to the package.
- **Zero values are useful.** `nil` slices/maps read as empty; a `sync.Mutex`'s zero value is an unlocked mutex; no constructors needed for those.
- **Value vs pointer receivers.** Pointer to mutate or to avoid copying (`*Node`); value for small immutable helpers (`MarkdownParser`).
- **Composite literals + escape analysis.** `&Node{…}` allocates and returns a pointer; the compiler decides stack vs heap. No manual memory management.
- **`range`, `comma-ok`, the blank `_`.** The everyday loop, the safe map/channel read, the "I'm intentionally ignoring this" marker.
- **Build tags for platform code.** `//go:build unix` selects files at compile time.
- **Table-driven tests.** (See any `_test.go`: a slice of cases looped with `t.Run(name, …)` subtests — the standard Go test shape.)

## 6. Suggested reading order

1. `internal/spike/roundtrip/roundtrip.go` — the idea, smallest form.
2. `internal/node/node.go` → `render.go` → `envelope.go` → `parser.go` → `ulid.go` — the keystone.
3. `internal/index/schema.go` → `index.go` → `reconcile.go` → `lock_*.go` — the projection.
4. `internal/cli/query.go` — the read surface.
5. `internal/config`, `internal/output`, `internal/cli/dispatch.go`, `cmd/rk/main.go` — the frame.

For *why* any of it is shaped this way, the design record is `composable-redesign.md`
(start at "Status & resuming"); the per-package contracts live in each package's
`AGENTS.md`.
