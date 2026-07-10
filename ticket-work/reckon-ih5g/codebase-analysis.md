# Codebase analysis — reckon-ih5g (v1-T8: note tool `rk note` + linking)

Read-only research for planning. Worktree: `/home/chadd/repos/reckon/.worktrees/reckon-ih5g`
(tip `d758ed5`, the commit that settled the T8 filename policy in
`docs/design/km-architecture-proposal.md` §4.2).

## 0. TL;DR for the planner

**The generic machinery T8 needs is already fully built and tested — by T1/T2, not by this
ticket.** `internal/node` already parses body `[[wikilinks]]` into `references` edges and
ref-valued frontmatter props into typed edges (`node.go:319-342`, `:344-369`); already parses
`aliases:` frontmatter into `Node.Aliases`; already has the `NewNode → Render → Parse →
writeFileAtomic` create recipe (cloned three times already: `todo.go`'s `addDurableTodo`,
`add.go`'s `appendLogEntry`, `adopt.go`). `internal/index`'s `resolveEdges` (`reconcile.go:375-387`)
recomputes **every** edge's `dst_key` against `nodes.ulid` then `_aliases.alias` on **every**
reconcile pass — so "a dangling link resolves once its target exists" is not something T8 writes,
it is a free consequence of a mechanism T2 shipped and `TestRebuildPopulatesViews`
(`index_test.go:102-155`) already exercises (a dangling `[[ghost]]` alongside a resolved
`[[ULID]]` and a resolved `depends:` ref-prop, in one test). Alias-collision detection is also
already generic (`collectWarnings`/`aliasCollisionWarnings`, `reconcile.go:389-480`,
tested `index_test.go:614-765`). `rk query` already makes unresolved edges ad-hoc queryable
(`SELECT * FROM edges WHERE dst_key IS NULL`) with zero note-specific code.

**What T8 actually has to build is thin CLI orchestration plus one real design gap:**

1. **`internal/cli/note.go` (singular) and `internal/cli/notes.go` (plural) already exist** —
   but both are the *old, DB-primary, pre-redesign* zettelkasten (`models.Note`, `notesService`,
   `internal/service`, `internal/db`, `internal/parser`), completely disconnected from
   `internal/node`/`internal/index`. `rk notes create/show/edit` (plural) is the closest-named
   existing command and is legacy code awaiting the T9 truth-inversion, not a T8 starting point.
   T8 must add `rk note create`/`rk note show` (singular, per the ticket) onto the *existing*
   `noteCmd` cobra command, which currently only has legacy `new`/`list` subcommands
   (`internal/cli/note.go:21-25, 210-212`). This name collision is a real decision point, not
   just trivia — see §1.
2. **The index's `_aliases` table is populated *only* from a node's frontmatter `aliases:` field
   (`reconcile.go:281-285`) — never from the filename.** The settled T8 policy is "filename =
   slug = the canonical alias" (proposal §4.2), and `composable-redesign.md`'s A#6 (already
   decided 2026-06-21, before this ticket existed) says explicitly: "each tool auto-mints a
   canonical alias (note slug from title...)". Concretely this means **`rk note create` must
   write its own slug into its `aliases:` frontmatter line**, or `[[the-slug]]` links from other
   notes will never resolve through the index (they'd still resolve in Obsidian's own
   file-based lookup, but not in reckon's graph — defeating the "forward links + backlinks are
   queryable" acceptance criterion). This is the single most important non-obvious finding in
   this analysis — see §3.3.
3. **No slugify helper exists in the v1 code path.** There is one in the *legacy* v0 package,
   `internal/parser.NormalizeSlug` (`internal/parser/links.go:101-140`) — pure, dependency-free,
   reasonable — but it belongs to the package `internal/node/AGENTS.md` explicitly calls out as
   serving only the old `internal/cli/notes.go`/`internal/service` path. Reuse vs. reimplement is
   a real decision — see §5.
4. **No rename/alias-redirect code exists anywhere** (`grep -rn redirect internal/` finds
   nothing). But the node-package primitives to build it (`SetField`/`InsertField` on the
   `aliases` scalar) already exist and are exactly sufficient — see §6. The semantics are already
   fully decided in `composable-redesign.md`'s A#6, dated *before* this ticket: "Rename = retain
   old as redirect... zero file churn."

Everything else — output modes, config/vault resolution, author resolution, atomic writes,
CRLF/conflict-marker rejection, `requiresDB` wiring — has a direct, working precedent to clone
from `todo.go`/`add.go`/`adopt.go`. Below is the detail, file-and-line.

---

## 1. The naming collision: `note`/`notes` already exist, and are legacy

`internal/cli/root.go:117-119` registers both, unconditionally, as top-level commands:
```go
RootCmd.AddCommand(GetLogCommand())
RootCmd.AddCommand(GetNoteCommand())   // "rk note" -- new, list (legacy, DB-backed)
RootCmd.AddCommand(GetNotesCommand())  // "rk notes" -- create, show, edit, delete? (legacy, DB-backed)
```

- **`internal/cli/note.go`** (singular, 223 lines): `noteCmd` has subcommands `new` (appends to
  the DB-backed journal via `journalService.AppendLog`, `note.go:33-64`) and `list` (reads
  `notesService.GetAllNotes()`, `note.go:66-208`, DB-backed). `note_test.go` even has a test named
  `TestNoteNewCommand_DeprecationNotice` (asserting `RunE != nil`) suggesting this surface is
  already expected to be retired/warned-about, though no deprecation message is actually printed
  today.
- **`internal/cli/notes.go`** (plural, 18KB): `notesCmd` has `create`/`show`/`edit`, all backed by
  `notesService` (`internal/service/notes_service.go`), `internal/db`, `internal/models.Note`
  (file layout `yyyy/yyyy-mm/yyyy-mm-dd-slug.md` under `internal/models/note.go:15-24` — a totally
  different filename convention than T8's `notes/<topic>/slug.md`). `notesShowCmd` at
  `notes.go:80-100` even prints "outgoing links"/"backlinks" already — but sourced from SQLite
  tables `note_links`/`notes` (`internal/db/db.go`, `internal/storage/database.go:93-110`), not
  from `internal/index`.
- Both are flagged in `internal/cli/AGENTS.md`'s "Current Verb Inconsistency (Being Fixed)"
  section (line 21-32) as pre-existing debt (`notes: create, edit, show`) unrelated to T8.
- **root AGENTS.md is current and explicit about this**: "`internal/cli/AGENTS.md` … pre-redesign
  code awaiting the T9 truth-inversion. When old code contradicts the design doc, the design doc
  wins" (`AGENTS.md:27-29`). Treat `internal/cli/AGENTS.md`, `internal/journal/AGENTS.md` (if
  referenced from it) as **not authoritative for T8**.

**Decision T8's plan must make explicit:** add `create`/`show` as new subcommands under the
*existing* `noteCmd` (singular) — cleanly separable from legacy `new`/`list` since Cobra
subcommands don't collide — and leave `rk notes` (plural, legacy) untouched pending T9. Do not
try to unify or migrate the legacy DB-backed commands as part of this ticket; that inversion is
explicitly T9's job (`reckon-s6oh`, listed open in `AGENTS.md`/`km-architecture-proposal.md`
table §1.3).

---

## 2. `internal/node` — everything T8 needs already ships, unmodified

Package doc `internal/node/node.go:1-29`. `internal/node/AGENTS.md` is current and accurate.

**Node struct** (`node.go:73-91`):
```go
type Node struct {
    Raw []byte
    ULID, Type, Time, Author, Body string
    Aliases   []string
    Props     map[string]string
    Fragments []Fragment
    Links     []Link
    Loc       Loc
    // + private parse-internal spans
}
```
`reservedKeys` (`node.go:44-46`): `id, type, time, author, aliases` — everything else
(`title`, `description`, `tags`, `stage` — the T8 frontmatter conventions) is **not** reserved,
so it falls straight into `Props` with zero node-package changes required. `stage:
seedling|budding|evergreen` needs no enum validation in the node package (it's just a string
prop); if enum validation is wanted it belongs in the note command layer.

**Link routing already does exactly what the ticket describes**, with no changes needed:
- Body `[[wikilink]]` → `Link{Rel: "references", To: target}` (`extractBody`, `node.go:344-369`,
  via `wikilinkRe` + `parseBodyLink`).
- A ref-valued frontmatter prop (`depends: "[[X]]"` or multi-target
  `depends: [[A]], [[B]]`) → `Link{Rel: "depends", To: X}` per target, dropped from `Props`
  (`deriveView`, `node.go:319-342`, via `parseRefValues`, `node.go:470-498`). This is the
  "frontmatter typed-edge props → typed edges" requirement, already shipped.
- `aliases:` frontmatter (flow `[a, b]` or bare scalar or Obsidian block-list form) →
  `Node.Aliases` (`parseAliases`, `node.go:431-448`; block-list form via `scanBlockList`,
  `node.go:283-313`, documented in `node/AGENTS.md` "Block-style lists").

**Create path**: `node.NewNode(typ, author, body)` (`render.go:120-128`) mints a ULID, leaves
`Time`/`Aliases`/`Props`/`Links` empty for the caller to fill in; `(*Node).Render()`
(`render.go:32-115`) is the inverse serializer — canonical key order `id, type, time, author,
aliases, then Props sorted alphabetically, then typed links grouped by rel`. **Note:** this
fixed order does not match the illustrative frontmatter example in
`km-architecture-proposal.md` §4.2 (`id, type, title, description, tags, time, author, stage,
aliases`) — `Render()` would actually emit `id, type, time, author, aliases, description, stage,
tags` (Props alphabetical) for a note with `Props={title,description,tags,stage}` **and put
`title` before/after other Props alphabetically** (`description`, `stage`, `tags`, `title` — `t`
before `title`... actually alphabetically `description < stage < tags < title`). This is
cosmetic only (Render's ordering is a generic contract every type shares; nothing reads
frontmatter key order) but worth a planning note: either accept `Render()`'s existing generic
order for notes too, or decide `title`/`description` deserve promotion to reserved-field status
(a real node-package change, bigger blast radius, probably not warranted for T8).

**Parser**: `MarkdownParser` (`parser.go:16-34`) is the file-per-item parser (one node per file) —
this is what notes use; **no group-file splitting needed** (notes aren't day-files like
`log/*.md`). `index.Open`'s default parser is `node.LogParser` (`index.go:69-71`), but
`LogParser` is byte-identical to `MarkdownParser` for every file whose `type` isn't `log-day`
(`node/AGENTS.md` "Group files" section, `logparser.go`) — so notes need **zero parser wiring
changes**; they ride the existing default for free.

**Write primitives available for the rename/alias-redirect requirement** (§6): `SetField`
(`node.go:151-167`, overwrite an existing scalar span) and `InsertField` (`insert.go:24-50`, add a
previously-absent scalar key as the first frontmatter line). Both re-parse after splicing so the
view stays consistent. Neither needs a new node-package method for T8's rename case.

---

## 3. `internal/index` — dangling-link auto-resolve is generic and already shipped

Package doc + `internal/index/AGENTS.md` are current and accurate.

### 3.1 Auto-resolve already works, for any node type, no T8 code needed

`resolveEdges` (`reconcile.go:373-387`) runs unconditionally at the end of **every** `Rebuild` and
**every** `Reconcile` pass (`reconcileTx`, `reconcile.go:213-215`):
```sql
UPDATE _edges SET dst_key = COALESCE(
    (SELECT n.node_key FROM _nodes n WHERE n.ulid = _edges.dst AND n.ulid <> ''),
    (SELECT a.node_key FROM _aliases a WHERE a.alias = _edges.dst ORDER BY a.node_key LIMIT 1)
)
```
It recomputes `dst_key` for **all** edges from scratch each pass — so a `[[not-yet-created]]` link
indexed today (`dst_key IS NULL`) will resolve automatically the next time `Reconcile`/`Rebuild`
runs after the target file appears, with no per-type or per-tool special-casing.
`TestRebuildPopulatesViews` (`index_test.go:102-155`) demonstrates a dangling `[[ghost]]` body
link, a resolved `[[ULID]]` body link, and a resolved `depends: "[[ULID]]"` frontmatter-ref edge,
all in one file, in one pass. There is no test named specifically "dangling resolves after target
is created later" (a two-pass scenario), but it follows directly from `resolveEdges`'s
full-recompute semantics and `TestReconcileAddEditDelete`'s add/edit/delete coverage
(`index_test.go:214-256`) of the surrounding reconcile loop — worth a small explicit T8-level
regression test (create note A with `[[bare-slug]]`, reconcile, assert `dst_key IS NULL`; create
note with that slug, reconcile again, assert `dst_key` now resolves) even though the underlying
mechanism needs no new code.

### 3.2 Alias-collision detection is already generic

`collectWarnings`/`aliasCollisionWarnings` (`reconcile.go:389-480`) flag any alias claimed by
`>1` surviving `node_key` as a `Warning{Kind: "alias_collision", ...}`, recomputed fresh every
pass (tested `index_test.go:614-765`, including collision resolving itself on edit). This is
**post-hoc/reactive** (surfaced after the fact via `rk index`'s stats or a `rk query` view), not
**preventive** (nothing stops `rk note create` from writing a second file with a colliding slug
today). The ticket's "index-enforced uniqueness" phrase is satisfied by the reactive warning
mechanism already; if T8 wants a hard *rejection* at create-time (nicer UX, avoids ever writing
the colliding file), that's new CLI-layer code — check the index (or glob the `notes/`
directory, per `findDurableTodoByRefOrAlias`'s pattern, §4) for an existing alias/slug before
writing.

### 3.3 The gap: aliases come **only** from frontmatter, never from the filename

`insertNode` (`reconcile.go:269-297`) populates `_aliases` purely from `n.Aliases` (i.e., the
frontmatter `aliases:` field parsed by `node.go`'s `deriveView`) — there is no code anywhere
(confirmed by grep for `loc_file`/`basename`/`TrimSuffix` in `internal/index/*.go`) that derives
an alias from the node's own file path or basename. **The settled T8 filename policy is
"filename = title-derived slug = the canonical alias"** — but the index has no mechanism to treat
a file's basename as an alias by itself.

This is resolved by `composable-redesign.md`'s A#6 (decided 2026-06-21, i.e. before this ticket
was even filed), which says exactly what to do: **"each tool auto-mints a canonical alias (note
slug from title...)"** — meaning `rk note create` must write the slug into the note's own
`aliases:` frontmatter line at creation time (in addition to using it as the filename), so that
`[[the-slug]]` links written in *other* notes resolve through `resolveEdges`'s alias lookup. This
is not optional polish — without it, cross-note forward-links/backlinks via slug (the primary
Obsidian-native linking style) would silently never populate `dst_key`, failing the "forward
links + backlinks... are queryable" acceptance criterion for slug-style links (ULID-style links
would still resolve fine, since `nodes.ulid` is always populated regardless of aliases).

### 3.4 Rename = retain old alias, already fully speced

`composable-redesign.md` §"Alias namespace & dangling links (A#6)" (around line 963-974): **"Rename
= retain old as redirect. On slug/alias change, the old alias is kept in frontmatter `aliases:` →
both reckon and Obsidian still resolve `[[old]]`; zero file churn (no mass link rewrites — safe
across Syncthing/git devices)."** This is exactly the ticket's "rename retains the old alias as a
redirect" acceptance criterion, decided at the design level well before T8 existed. Concretely, a
`rk note` rename operation is: (1) rewrite the note's `title:` (Props) and mint a new slug; (2)
append the *old* slug into the `aliases:` list (via `SetField` if `aliases:` already has a scalar
span, else `InsertField` to create it fresh); (3) `os.Rename`/atomic-write the file to the new
slug path — nothing touches any *other* file (no mass link rewrite, matching "zero file churn").
No index-package change needed; the existing alias-resolution machinery (§3.1) picks up both the
new slug (auto-minted per §3.3) and the retained old one identically.

### 3.5 A#6's case-insensitivity is not actually implemented (pre-existing gap, worth flagging)

A#6 also says alias matching should be "case-insensitive" and forbid link-control chars
(`# | [ ] ^`). `schemaDDL`'s `_aliases` table (`schema.go:61-65`) has no `COLLATE NOCASE`, and
`resolveEdges`'s `a.alias = _edges.dst` (`reconcile.go:379-382`) is therefore SQLite's default
case-**sensitive** BINARY comparison; `parseAliases` (`node.go:431-448`) does no case
normalization either. This predates T8 (it's T1/T2 territory) and may be out of scope to fix
here, but since T8 is the first tool to actually mint aliases at scale (slugs are typically
already-lowercase, so it may rarely bite), it's worth a one-line note to the planner: either
lowercase slugs consistently (sidesteps the issue entirely for T8's own aliases) or flag the
gap as a known limitation, not silently rely on case-insensitivity that isn't there.

### 3.6 Views available for "forward links + backlinks... queryable"

Stable public views (`index/AGENTS.md` table, `schema.go:81-90`): `nodes(id,ulid,type,time,
author,body,loc)`, `edges(src,rel,dst,dst_key,from_frag,to_frag)`, `node_props(id,key,value)`,
`aliases(alias,id)`, `fts`/`fts_search`. Forward links for a note = `SELECT * FROM edges WHERE
src=?`; backlinks = `SELECT * FROM edges WHERE dst_key=?` (or `dst=<slug/ulid>` for
not-yet-resolved). All of this is already exposed to `rk query` (T3, shipped) with zero
note-specific work; `rk note show` (§4) would run the same style of query in-process (see
`todo.go`'s pattern, §4.3) to render "forward links" / "backlinks" sections without shelling out.

---

## 4. CLI patterns to clone (from `add.go`/`todo.go`/`adopt.go`, all v1, all current)

### 4.1 Command registration + `requiresDB` annotation

`add.go:35-43` — the annotation pattern that skips DB init (legacy `initServiceE`, `root.go:149-
176`) for a command that only touches the vault text + (optionally) the index directly:
```go
var addCmd = &cobra.Command{
    Use:          "add <text...>",
    Annotations:  map[string]string{"requiresDB": "false"},
    SilenceUsage: true,
    Args:         cobra.MinimumNArgs(1),
    RunE:         runAddE,
}
```
`root.go:83-105`'s `PersistentPreRunE` walks `cmd`+ancestors for this annotation. T8's `note
create`/`note show` should carry the same annotation (the legacy `initServiceE()` sets up
`journalService`/`notesService`/`checklistService` against the old SQLite DB — completely
irrelevant to a text-truth note command). Register the new subcommands under the existing
`noteCmd` (already added to `RootCmd` at `root.go:118`); no new top-level `RootCmd.AddCommand`
needed.

### 4.2 Create recipe: `NewNode → set fields → Render → Parse → writeFileAtomic`

Cloned identically by `todo.go`'s `addDurableTodo` (`todo.go:331-375`) and `add.go`'s
`appendLogEntry`/`writeLogEntryBlock` (`add.go:184-260`):
```go
n := node.NewNode("todo", author, body+"\n")
n.ULID = id                                   // (NewNode already mints one; todo overwrites w/ a seam)
n.Time = time.Now().UTC().Format(time.RFC3339)
n.Props = map[string]string{"state": "open", ...}
n.Links = []node.Link{{Rel: "depends-on", To: depends}}
rendered := n.Render()
parsed, err := node.Parse(rendered)     // round-trip through the parser once, immediately
writeFileAtomic(path, parsed.Serialize())
```
Both callers pre-check `os.Stat(path)` and refuse to overwrite an existing file
(`todo.go:335-339`). For notes, `path` is `notes/<topic-or-flat>/<slug>.md` instead of
`todos/<ULID>.md` — the only structural difference. `writeFileAtomic` itself
(`adopt.go:250-...`) is shared, temp-file-same-dir + rename, already battle-tested.

### 4.3 Read/show recipe: `index.Open` → `Reconcile()` → `db.Query` over the stable views

`todo.go:454-482` (`runTodoListE`) is the precedent for a command that reads *through the index*
rather than glob-scanning files:
```go
ix, err := index.Open(cfg)
defer ix.Close()
ix.Reconcile()                     // lazy reconcile-on-read before every read
rows, err := ix.DB().Query("SELECT id, body FROM nodes WHERE type = 'todo'")
```
and per-node prop/edge lookups (`loadTodoProps`, `todo.go:541-...`; `loadDependsOn`,
`todo.go:561-...`, `SELECT dst FROM edges WHERE src=? AND rel='depends-on'`). `rk note show
<slug-or-ulid>` should follow exactly this shape: open index, reconcile, resolve the ref via
`nodes.ulid=? OR EXISTS(SELECT 1 FROM aliases WHERE alias=? AND id=nodes.id)`, then two more
queries for outgoing (`edges WHERE src=?`) and incoming (`edges WHERE dst_key=?`) links.

Contrast: `findDurableTodoByRefOrAlias` (`todo.go:816-831`) is the **other** existing pattern —
`filepath.Glob` + `node.Parse` each file directly, bypassing the index entirely, used by `todo
done` for direct-write mutation (it needs the actual file path to write back to, not just index
metadata). `rk note create`'s pre-write collision check (§3.2) would likely use this
glob-scan-`notes/` style (mutation path, needs the real file), while `rk note show` (read-only)
should prefer the index-query style (cheaper, and gets backlinks for free without an extra
in-process graph walk).

### 4.4 Output: `internal/output.Writer`

`internal/output/output.go` — `Mode` (`Pretty|JSON|NDJSON`), `output.ModeFromFlags(jsonFlag,
ndjsonFlag)`, `output.New(w, mode).Print(v)`/`.PrintAll(vs)`. `Print` prefers a `Pretty() string`
method (see `add.go`'s `logAddResult.Pretty()`, `add.go:80-82`), else `fmt.Stringer`, else `%v`.
T8's create/show result structs should implement `Pretty()` the same way. Global `--json`/
`--ndjson` flags already exist on `RootCmd` (`root.go:112-113`); nothing new needed.

### 4.5 Config / vault layout

`internal/config.Config{VaultDir, CacheDir}`, resolved via `config.LoadWithOverrides(vaultFlag,
"")` (`config.go` — `RECKON_VAULT` env, else `$HOME/reckon`). **No `notes/` subdir constant
exists yet anywhere in v1 code** (only `log/`, `todos/` are hardcoded via
`filepath.Join(cfg.VaultDir, "todos")` etc. in `todo.go:302`/`add.go:119`); T8 introduces
`filepath.Join(cfg.VaultDir, "notes")` the same way, `os.MkdirAll(...,0o755)` before writing
(`todo.go:303`, `add.go:120-122`). Note: the legacy `internal/config.NotesDir()`
(`config.go` — `~/.reckon/notes/`) is unrelated pre-v1 DB-era plumbing; do not use it.

**`.gitattributes` already anticipates the `notes/` directory** and explicitly excludes it from
the union-merge driver: "Scoped to `log/*.md` ONLY — other markdown (todo/, notes/, README, etc.)
must use the default 3-way merge so real edit conflicts surface" (`.gitattributes:3-8`). This is
correct/expected for T8 (file-per-note, not a group file — normal 3-way merge is right) and needs
no change.

### 4.6 Author resolution

`todo.go:246` `resolveAuthor(flag string) string` — `--author` flag > `$RECKON_AUTHOR` > `$USER`
> `"local"`, always non-empty. Reuse directly (already exported-enough within `package cli`,
called from both `add.go` and `todo.go`).

---

## 5. Slugification: exists, but in the legacy package

`internal/parser.NormalizeSlug` (`internal/parser/links.go:101-140`): lowercases, keeps only
`[unicode letter/digit, '-', '_', space]`, converts spaces→hyphens, collapses repeated hyphens,
trims, truncates to `MaxSlugLength = 200`. It is a **pure function with no dependency on the
legacy DB/service layer** — only string manipulation — so importing `internal/parser` from new
T8 code is technically safe, but stylistically odd given `internal/node/AGENTS.md`'s explicit
framing of `internal/parser` as serving only the old `internal/cli/notes.go`/`internal/service`
path. Other slugify implementations exist too, all near-duplicates of the same regex-based
approach: `internal/journal/task_service.go:44-58` (`generateSlug`, ASCII-only
`[^a-z0-9]+` collapse), `internal/migrate/log_files.go:16-49`, `internal/migrate/task_files.go:
16-37` — all legacy/migration-only, none reachable from v1 code.

**Planner decision:** either (a) import `internal/parser.NormalizeSlug` directly (least new
code, but crosses the legacy/v1 boundary the AGENTS.md docs otherwise keep clean), or (b) write a
small fresh slugify in `internal/cli` (or promote one into `internal/node` as a shared helper,
if a second tool will want it) — a ~15-line function, trivial either way. No test coverage or
Obsidian-compatibility concern favors one over the other; this is purely a code-organization
call. Whichever is chosen, decide the **charset** deliberately against A#6's rule ("forbid
link-control chars `# | [ ] ^`" — `NormalizeSlug` already strips everything to
alnum/hyphen/underscore, which satisfies this incidentally) and decide whether slugs allow
non-ASCII letters as-is (current `NormalizeSlug` keeps Unicode letters verbatim, no
transliteration) or transliterate (e.g. "café" → "cafe") — Obsidian itself does no
transliteration, so keeping Unicode letters verbatim is probably the compatible choice.

---

## 6. Rename/alias-redirect: no code exists, but the design is fully decided and primitives suffice

Confirmed by grep: no "redirect" anywhere in `internal/`, no existing rename command for any
node type (todos/logs are never renamed in the current code — only notes will need this, being
"the one browse-heavy type", per `km-architecture-proposal.md` §4.2). The full semantics were
decided in `composable-redesign.md`'s A#6 (§3.4 above), well before T8 was scoped — this is a
sequencing/orchestration task inside `internal/cli`, not a design question and not a
node/index-package change:

1. Compute new slug from the new title.
2. `n.SetField("aliases", "["+strings.Join(append(existingAliases, oldSlug), ", ")+"]")` if
   `aliases:` already has a scalar span (`n.HasField("aliases")`, `node.go:175-178`), else
   `n.InsertField("aliases", "["+oldSlug+"]")` if this is the note's first rename.
3. `n.SetField("title", newTitle)` (or `InsertField` if not yet present — but `title` should
   always be present per T8's mandatory-by-convention rule, so `SetField` should suffice for any
   note that went through `rk note create`).
4. Serialize + `writeFileAtomic` to the **new** path; remove/leave the old path per whatever
   `rk note rename`'s contract ends up being (a `git mv`-equivalent, not a copy — old filename
   must stop existing so Obsidian doesn't see two files for one ULID).

No new `internal/node` or `internal/index` code is needed for any of this — it composes entirely
from primitives that already exist and are already exercised by other callers (`SetField` in
`todo.go`'s `doneEphemeralTodo`/`doneRecurringTodo` paths; `InsertField` in `adopt.go`).

---

## 7. Frontmatter conventions T8 must add (all land in `Props`, no node-package change)

Per `km-architecture-proposal.md` §4.2 (verbatim example, settled 2026-07-09):
```yaml
id: 01J9Z3K7Q2W8XR4M6N0V5BYHED
type: concept        # free taxonomy: concept|entity|summary|decision|runbook|reference|memory...
title: PAS Entity Model
description: How PAS entities map across Propexo/AppFolio.
tags: [pas, integrations]
time: 2026-07-09T10:40:00-04:00
author: mike
stage: seedling       # optional: seedling|budding|evergreen
aliases: [pas-entities]
```
- `title`, `description` are "mandatory-by-convention" — i.e. the note **command** should
  require/default them (validation in `internal/cli`), not the node package (they aren't
  reserved keys there, so nothing stops a hand-edited file from omitting them — that's
  deliberate, matches OKF's own soft-conformance philosophy per §1.1/§2.2 of the same doc).
- **No `updated`/`timestamp` field** — settled explicitly (§4.2 "Timestamp semantics"): git is
  the versioning backplane; a stored updated-at would duplicate truth and can lie on hand edits.
  `node.Node.Time` (reserved) is create/event time only, matching existing `add.go`/`todo.go`
  usage (`n.Time = time.Now().UTC().Format(time.RFC3339)`) — note the proposal's own example uses
  a `-04:00` local offset rather than the `...UTC()...Z` form every existing v1 writer uses; this
  is presumably just illustrative rather than a hard format requirement, but worth settling
  explicitly in the plan (recommend: follow existing precedent, UTC `Z`-suffixed RFC3339, for
  consistency with `add.go`/`todo.go` unless there's a reason notes need local-offset display).
- `stage` needs no node-package enum validation — a plain string prop; if `rk note create`
  wants to validate `stage ∈ {seedling,budding,evergreen}` (or default it to `seedling`), that's
  CLI-layer logic, mirroring how `todo.go` validates `repeat`/`scheduled`/`deadline` today
  (`parseRepeat`, `parseSchedDate`, `todo.go:280-289`).
- `notes/**/index.md` generation ("as part of reconcile or `rk note index`") is explicitly named
  in the ticket description but is **not** listed among the "Done when" bullets — treat as
  in-scope-but-lower-priority/stretch within T8, or explicitly deferred with a one-line rationale
  in the plan; nothing else in the codebase depends on it existing yet (no beads ticket for OKF
  export/lint/prime exists either — those are still proposal-only, correctly scoped to "after
  T8" per §4.3 of the proposal, not part of reckon-ih5g).

---

## 8. Known pitfalls (from `docs/REVIEW_PATTERNS.md` + `internal/node/AGENTS.md` +
   `internal/index/AGENTS.md`, filtered to what's relevant to a file-per-note writer/reader)

- **No `os.Exit` in library/command code** — return wrapped errors, let Cobra handle exit codes
  (`REVIEW_PATTERNS.md` "os.Exit() in Library Code"; root AGENTS.md repeats this).
- **Wrap every error with context** (`fmt.Errorf("note create: ...: %w", err)`) — the single
  most-flagged historical pattern (`REVIEW_PATTERNS.md`, 15 occurrences).
- **Respect `--quiet`** on success output (`if !(mode == output.Pretty && quietFlag) { ... }` —
  the exact idiom `add.go:129` and `todo.go:317` both use; copy it verbatim).
- **CRLF rejection**: every existing writer (`add.go:243-245`, `todo.go`'s done-paths) explicitly
  rejects `\r\n` line endings before appending/parsing with `reckon-vj55`-referencing error
  messages. A **create** path (writing a brand-new file) doesn't need this check (nothing to
  read first), but any note **edit/rename** path that reads-modifies-writes an existing file
  should carry the same guard.
- **Refuse to overwrite on create** — `todo.go:335-339`'s `os.Stat` pre-check pattern; apply the
  same to `rk note create` at the slug path (distinct from the alias-collision check, which is
  about a *different* file claiming the *same slug as an alias* elsewhere in the vault — both
  checks are needed).
- **Git-conflict-marker files refuse to parse** (`node.go:38-40,118-122`) — already handled
  generically by `node.Parse`; nothing extra needed, but `rk note show`/rename should propagate
  (not swallow) a parse error from a conflicted note file rather than silently skipping it (skip-
  and-log is the *index*'s reconcile-loop policy, `reconcile.go:186-194` — not necessarily right
  for a direct single-file read in `rk note show`, which should probably just error).
- **`internal/cli/AGENTS.md` is stale for T8** (§1 above) — describes the old
  `journalService`/`notesService` package-global init pattern and doesn't mention
  `output.Writer`, `requiresDB` annotations, or `internal/node`/`internal/index` at all. Do not
  pattern-match against it; pattern-match against `add.go`/`todo.go`/`adopt.go` and the root/
  node/index `AGENTS.md` files instead (all confirmed current in this analysis).
- **Timezone-date pitfall** (`REVIEW_PATTERNS.md` "time.Parse for Calendar Date Comparisons") —
  probably not very relevant to notes (no scheduling/deadline fields), but if any date-only flag
  is added (e.g. filtering `stage='seedling' ORDER BY time`), reuse `effectiveLogDate`
  (`add.go:158-163`)'s already-fixed UTC-consistent pattern rather than re-deriving one.

---

## 9. Test precedent to model T8's tests on

- `internal/node/node_test.go`'s `roundtripCorpus` + `TestRoundTripIdentity`/
  `FuzzRoundTripIdentity` — the byte-preservation gate; any new fixture shape T8 introduces
  (e.g. a note with `stage:`, multi-tag `tags: [...]`) should be added to this corpus if it
  exercises a genuinely new frontmatter shape (it likely doesn't — `tags`/`stage`/`title`/
  `description` are all plain scalars, already covered by existing scalar-prop fixtures).
- `internal/index/index_test.go`'s `TestRebuildPopulatesViews` (dangling + resolved edges, one
  pass) and `TestReconcileAddEditDelete`/`TestReconcileRenameKeyedByULID` (multi-pass lifecycle)
  are the direct templates for a T8-level test proving "dangling link resolves once its target
  exists" end-to-end through two `rk note create` calls + reconcile, even though the underlying
  index mechanism needs no new code (§3.1).
- `internal/cli/add_test.go`/`internal/cli/todo_test.go` — end-to-end CLI test shape: build a
  temp vault, run the command's `RunE` (or exported wrapper) directly, assert on file bytes +
  `output` JSON shape. Clone this harness style for `note_create_test.go`/`note_show_test.go`
  (new files — the existing `note_show_test.go`/`notes_edit_test.go`/`notes_integration_test.go`
  in the repo today test the **legacy** DB-backed `rk notes`, not T8's target).

---

## 10. Summary table: what exists vs. what T8 must build

| Capability (from ticket description) | Status | Where |
|---|---|---|
| File-per-note nodes, `NewNode`/`Render`/`Parse` create recipe | ✅ shipped (generic) | `internal/node`, cloned pattern in `todo.go`/`add.go` |
| Body `[[wikilinks]]` → `references` edges | ✅ shipped (generic) | `node.go:344-369` |
| Frontmatter typed-edge props → typed edges | ✅ shipped (generic) | `node.go:319-342`, `parseRefValues` |
| Aliases parsed from frontmatter | ✅ shipped (generic) | `node.go:431-448` |
| Dangling links stored as unresolved edges | ✅ shipped (generic) | `schema.go` (`dst_key` nullable) |
| Auto-resolve on target creation | ✅ shipped (generic, untested for the 2-pass case specifically) | `reconcile.go:373-387` |
| Unresolved edges queryable | ✅ shipped (generic, via `rk query`) | stable `edges` view |
| Alias-collision flagging | ✅ shipped (generic, reactive) | `reconcile.go:389-480` |
| Alias uniqueness *enforced at write time* | ⬜ not built | new CLI-layer pre-check |
| **Filename-as-alias wiring** (slug must be written into `aliases:`) | ⬜ **not built — real gap** | new: `rk note create` must self-mint |
| Slugification | ⚠️ exists only in legacy `internal/parser` | reuse-or-reimplement decision |
| `rk note create`/`rk note show` command surface | ⬜ not built (collides in name with legacy `note`/`notes`) | `internal/cli/note.go` (add subcommands) |
| Rename retains old alias as redirect | ⬜ not built, but fully speced (A#6) + primitives (`SetField`/`InsertField`) exist | new: `rk note rename` (or `edit --title`) |
| `notes/**/index.md` generation | ⬜ not built, named in ticket but not in "Done when" | stretch/deferrable |
| `rk export --okf`, `rk lint`, `rk prime`, schema file | out of scope | separate future beads, per proposal §4.3/§4.8 |
