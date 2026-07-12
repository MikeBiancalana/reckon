# Codebase analysis: reckon-s6oh (v1-T9 migration)

## 0. Scope-defining facts (read first)

- **Two disjoint root directories.** Legacy (gen-1) data lives under
  `~/.reckon/` (`internal/config/config.go` `DataDir`/`JournalDir`/`TasksDir`/
  `NotesDir`/`DatabasePath` — no override except `RECKON_DATA_DIR`). v1 data
  lives under `cfg.VaultDir` (default `$HOME/reckon`, override `RECKON_VAULT`)
  with a separate `cfg.CacheDir` for the disposable index
  (`internal/config/config.go:98-164`, `LoadWithOverrides`). The importer must
  open **both** configs — old `config.DataDir()`-family paths as source, new
  `config.LoadWithOverrides` `Config.VaultDir` as destination — there is no
  shared root to walk once.
- **Per-type source-of-truth is not uniform** (corrects a literal reading of
  the ticket text):
  | Legacy type | Today's truth | Evidence |
  |---|---|---|
  | `journal.Journal` (day: intentions/wins/log/schedule) | **file-primary already** (old format) | `internal/journal/service.go:37` `GetByDate` calls `ParseJournal` on the `.md` file; `rebuild.go` rebuilds SQLite *from* files |
  | `journal.Task` (gen-1 global task, `rk task`) | **file-primary already** (old format) | `internal/journal/task_service.go:149` `GetAllTasks` reads `tasks/*.md` directly, ignores DB |
  | `models.Note` (zettelkasten, `rk notes`) | **SQLite-primary** | `internal/service/notes_repository.go` `SaveNote`/`GetNoteByID`; composable-redesign.md:658 |
  | `checklist.Template`/`Run` | **SQLite-primary, no file form at all** | `internal/checklist/repository.go`; no writer package exists |
  So the work is not uniformly "read SQLite, write files" — for journal/task it
  is **reparse old-format text into new canonical-node text**; for notes/
  checklists it is **extract from SQLite** (notes also has an existing `.md`
  file per note, referenced by `Note.FilePath`, but SQLite is the field
  authority per the design doc — verify title/tags/slug against the file
  before trusting either).
- **`internal/migrate` + `rk migrate` already exist and are unrelated.** They
  migrate old xid-filename task files (`{ID}.md`) to date-slug filenames and
  DB-only log entries to files — a *pre-v1* legacy migration, not this
  ticket's text-truth migration. Same collision risk for package/command
  naming: do not reuse the `migrate` package or `rk migrate` verb for the new
  work; pick a distinct name (e.g. `internal/textmigrate` + `rk import` /
  `rk migrate-v1`, TBD in planning) to avoid stepping on `internal/cli/migrate.go`,
  `internal/migrate/*.go`.
- **No v1 checklist tool exists.** `docs/design/composable-redesign.md:1210-1213`
  (Lean v1 scope) explicitly **defers** the checklist tool; `bd list --all`
  shows no `v1-T*` checklist ticket. The ticket description names checklists
  as a migration source, but there is no canonical node type/props spec, no
  writer, no `rk checklist` v1 command to migrate *into*. **[OPEN]** the
  planning phase must either (a) design a minimal `checklist-template`/
  `checklist-run` node schema as part of this ticket, or (b) explicitly
  descope checklists and flag it back to the ticket owner — silently
  skipping them would violate "migrating a COPY of real data yields matching
  counts."
- **No `--dry-run`/`--verify` flag convention exists anywhere in the repo**
  (`grep -rn "dry-run\|DryRun\|dryRun"` returns nothing). This ticket
  introduces that pattern; there's nothing to copy for the flag semantics
  themselves, only the general `output.ModeFromFlags`/`--json`/`--quiet`
  conventions (see §2).
- **A second, older `internal/parser` package exists** (per
  `internal/node/AGENTS.md:6-9`), used only by `internal/cli/notes.go` /
  `internal/service`. Don't confuse it with `internal/node/parser.go`
  (`node.Parser`, the v1 keystone interface) — different package, different
  job.

## 1. Files likely modified/created

**New package (name TBD in planning, suggest `internal/textmigrate`):**
- `internal/textmigrate/migrate.go` (or similar) — orchestration: walk legacy
  sources, build nodes, write vault files, dry-run/verify modes, stats/report
  type.
- Per-source-type converters, one file each is the established granularity in
  this codebase (mirrors `internal/migrate/{task_files,log_files,database}.go`
  and `internal/node/{node,render,logparser}.go` splitting by concern):
  `journal_convert.go` (day → log-day/log-entry nodes), `task_convert.go`
  (gen-1 task → todo node), `note_convert.go` (SQLite note → note node),
  `checklist_convert.go` (if in scope — see §0 OPEN item).
- `internal/textmigrate/*_test.go` per converter, plus an end-to-end test that
  runs the importer against a fixture legacy dataset and asserts counts +
  round-trip (mirrors `internal/spike/roundtrip/roundtrip_test.go`'s fuzz
  structure and `internal/cli/adopt_test.go`'s real-`RootCmd.Execute()` CLI
  style).

**New CLI command:**
- `internal/cli/<verb>.go` (e.g. `import.go`) — Cobra command with
  `--dry-run`, `--verify`, `--vault`/`--json`/`--ndjson`/`--quiet` (existing
  persistent flags, `internal/cli/root.go:108-114`), modeled directly on
  `internal/cli/adopt.go` (structured result type + `Pretty()`, per-item
  Adopted/Skipped/Errored-style breakdown, `Annotations: {"requiresDB":
  "false"}` since it doesn't touch the *index* DB directly — it may still open
  the *legacy* `storage.Database` as a read source).
- `internal/cli/root.go` — register the new command (`RootCmd.AddCommand(...)`
  alongside `adoptCmd`/`indexCmd`, `root.go:130-135`).

**Read-only reference points (no changes expected, but the converters call
into them):**
- `internal/node/{node,render,ulid,logparser}.go` — `node.NewNode`,
  `node.Node.Render()`, `node.Mint()`/`node.MintAt()`, `node.Parse`,
  `node.RenderLogEntry`/`RenderLogEntryWithDid` for log-entry block text.
- `internal/index` — the destination the migrated vault gets indexed into via
  a normal `rk index` / `index.Open` + `Rebuild()` after files are written;
  the migration itself should not hand-write index rows.
- `internal/config` — both `DataDir`-family (source) and `LoadWithOverrides`
  (destination).
- Legacy read paths: `internal/journal/{parser,task_parser}.go` (`ParseJournal`,
  `ParseTaskFile`), `internal/service/notes_repository.go`
  (`GetAllNotes`/`GetNoteByID`), `internal/checklist/repository.go`
  (`GetTemplateByID`, and list-all equivalents — check for a `ListTemplates`/
  `ListRuns`; not confirmed present, may need adding read-only there or
  querying `storage.Database.DB()` directly).

## 2. Existing patterns to follow

**CLI command shape** (`internal/cli/adopt.go` is the closest analog — an
idempotent, path-scoped, structured-output, atomic-write command):
- `Annotations: map[string]string{"requiresDB": "false"}`, `SilenceUsage: true`.
- A `<verb>Result` struct with `Adopted`/`Skipped`/`Errored`-shaped slices (or
  equivalent), each entry a small struct with `json` tags; a `Pretty()` method
  building a human summary line + indented per-item lines
  (`internal/cli/adopt.go:41-89`).
- `mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)` then
  `output.New(cmd.OutOrStdout(), mode).Print(res)`, gated by
  `!(mode == output.Pretty && quietFlag)` (`internal/cli/adopt.go:92-121`,
  same gating in `internal/cli/index.go:53-58`).
- Per-item errors accumulate into the result rather than aborting the whole
  run; the command returns a summarizing `fmt.Errorf` only after processing
  everything (`internal/cli/adopt.go:118-121`).
- Atomic file writes: `writeFileAtomic(path, data)` — temp file in the same
  dir, preserve mode, `os.Rename` (`internal/cli/adopt.go:250-284`); reuse
  this exact helper rather than re-deriving it.
- CRLF refusal check before parsing any legacy `.md` file text through
  `node.Parse` (`bytes.Contains(raw, []byte("\r\n"))` — reckon-vj55, appears
  in `adopt.go:205-208`, `todo.go:409-411,859-861,909-911`). Not relevant to
  *legacy* SQLite-sourced rows, but relevant if the importer also re-reads any
  existing legacy `.md` files (journal days, gen-1 tasks, note files).

**Node construction (create path, not edit path)** — this is what every
converter should do, per `internal/cli/todo.go:331-375` (`addDurableTodo`) and
`internal/cli/note_v1.go` (`runNoteCreateE`):
```go
n := node.NewNode(typ, author, body)   // mints ULID via node.Mint()
n.ULID = id                            // only if overriding the mint (n/a here — see alias note below)
n.Time = <derived from legacy timestamp, RFC3339>
n.Aliases = []string{ /* old xid + any old slug/date */ }
n.Props = map[string]string{ ... }     // per-type open bag
n.Links = []node.Link{ {Rel: "...", To: "..."} }
rendered := n.Render()
parsed, err := node.Parse(rendered)    // re-parse for canonical spans (invariant #6)
writeFileAtomic(path, parsed.Serialize())
```
Every create path in the repo renders through `node.Render` and re-parses
before writing (never writes `n.Render()`'s bytes directly) — this is
invariant #6 in composable-redesign.md:495-500 ("create renders through one
primitive... `serialize(parse(render(n))) == render(n)`"); the migration must
follow the same discipline for the "round-trip stable" acceptance criterion
to hold by construction rather than by luck.

**Alias-for-old-ID mechanism** — there is no bespoke "xid alias" primitive;
it is exactly `Node.Aliases []string` / `Node.SetAliases` (`internal/node/node.go:199-222`)
feeding the index's `_aliases` table (`internal/index/schema.go:61-65`) and
`resolveEdges`'s alias fallback (`internal/index/reconcile.go:375-387`,
resolves ULID first, then alias — ORDER BY makes ambiguous-alias resolution
deterministic but arbitrary). **Putting the old xid string into the new
node's `Aliases` slice is the entire "xid → ULID with old xid kept as alias"
mechanism** — no new index/resolver code is needed, only that the converters
populate `Aliases` correctly. Caution: the alias namespace is **global and
flat** (composable-redesign.md:963-965) and the index **flags but does not
error on** collisions (`collectWarnings`/`aliasCollisionWarnings`,
`internal/index/reconcile.go:389-480`) — with potentially thousands of
migrated xids, alias-collision warnings should be surfaced by `--verify`
mode, not silently ignored.

**`rk adopt`'s `node.InsertField` is not what this ticket needs for xid→ULID**
— `rk adopt` stamps a ULID into an *id-less* file, preserving everything
else. This migration instead **replaces** legacy non-canonical frontmatter
(`id: <xid>`, `title:`, `created:`, `status:`, etc., in an ad hoc per-type
YAML shape) with the canonical node shape — that's a full `Render`, not a
`SetField`/`InsertField` splice. Do not try to force this into the
span-surgical-edit primitives; those exist for editing an *already-canonical*
node in place.

**Dry-run/verify — no existing convention; design from scratch**, but the
"verify" half can reuse `internal/index.Index.Rebuild()`/`.Reconcile()`
(`internal/index/reconcile.go`) as the mechanical verification: write the
migrated vault (or a scratch copy), point a fresh `index.Open` at it, and
assert (a) `Stats.Warnings` is empty or explicable, (b) row counts per `type`
match the legacy source counts, (c) every legacy alias resolves
(`SELECT dst_key FROM edges WHERE dst_key IS NULL` for dangling, and a
direct `SELECT node_key FROM aliases WHERE alias = ?` lookup per migrated old
ID). `rk index`'s own summary query pattern
(`internal/cli/index.go:97-110`, counting `nodes`/`edges`/`aliases` views) is
the template for a verify-mode report.

## 3. Interfaces / types / signatures in play

| Symbol | Signature | Location |
|---|---|---|
| `node.Node` | struct: `Raw []byte; ULID,Type,Time,Author,Body string; Aliases []string; Props map[string]string; Fragments []Fragment; Links []Link; Loc Loc` | `internal/node/node.go:73-92` |
| `node.NewNode` | `func NewNode(typ, author, body string) *Node` (mints ULID) | `internal/node/render.go:120-128` |
| `(*Node) Render` | `func (n *Node) Render() []byte` | `internal/node/render.go:32` |
| `node.Parse` / `node.ParseAt` | `func Parse(raw []byte) (*Node, error)`; `func ParseAt(raw []byte, loc Loc) (*Node, error)` | `internal/node/node.go:115,118` |
| `(*Node) Serialize` | `func (n *Node) Serialize() []byte` | `internal/node/node.go:143` |
| `(*Node) SetAliases` | `func (n *Node) SetAliases(aliases []string) error` | `internal/node/node.go:199` |
| `(*Node) InsertField` | `func (n *Node) InsertField(key, value string) error` | `internal/node/insert.go:24` (existing-key-only; use `Render` for fresh nodes instead) |
| `node.Mint` / `node.MintAt` | `func Mint() string`; `func MintAt(t time.Time) string` | `internal/node/ulid.go:20,25` |
| `node.Parser` | `interface { Parse(raw []byte, loc Loc) ([]*Node, error); Serialize(n *Node) ([]byte, error) }` | `internal/node/parser.go:7-10` |
| `node.LogParser` | implements `Parser`; splits `type: log-day` group files into day+entry nodes | `internal/node/logparser.go:38` |
| `node.RenderLogEntry` / `RenderLogEntryWithDid` | `func RenderLogEntry(hhmm, author, ulid, body string) string`; `+didTarget` variant | `internal/node/logparser.go:182,192` |
| `node.Link` / `node.Fragment` | `{Rel,To,FromFrag,ToFrag string}` / `{ID,Anchor string}` | `internal/node/node.go:53-64` |
| `index.Open` / `OpenWithParser` | `func Open(cfg *config.Config) (*Index, error)`; `OpenWithParser(cfg, parser node.Parser)` | `internal/index/index.go:69,74` |
| `(*Index) Rebuild` / `Reconcile` | `func (ix *Index) Rebuild() (Stats, error)`; `Reconcile() (Stats, error)` | `internal/index/reconcile.go:45,87` |
| `index.Stats` / `index.Warning` | `{Scanned,Reparsed,Deleted int; Warnings []Warning}` / `{Kind,ULID,Alias string; NodeKeys,Files []string}` | `internal/index/reconcile.go:21-40` |
| `config.LoadWithOverrides` | `func LoadWithOverrides(vaultDir, cacheDir string) (*Config, error)` | `internal/config/config.go:122` |
| `config.DataDir`/`TasksDir`/`NotesDir`/`JournalDir`/`DatabasePath` | each `func() (string, error)`, legacy roots | `internal/config/config.go:18-96` |
| `storage.NewDatabase` | `func NewDatabase(path string) (*Database, error)` | `internal/storage/database.go:188` |
| `journal.ParseJournal` | `func ParseJournal(content, filePath string, lastModified time.Time) (*Journal, error)` | `internal/journal/parser.go:61` |
| `journal.ParseTaskFile` | `func ParseTaskFile(content string) (*Task, error)` | `internal/journal/task_parser.go:296` |
| `models.Note` / `NoteLink` | struct with `ID,Title,Slug,FilePath string; Tags []string; CreatedAt,UpdatedAt time.Time; Links []NoteLink` | `internal/models/note.go:36-45` |
| `service.NotesService.GetAllNotes` | `func (s *NotesService) GetAllNotes() ([]*models.Note, error)` | `internal/service/notes_service.go:174` |
| `checklist.Template`/`Run`/`RunItem` | structs, `internal/checklist/model.go:10-105` | |
| `checklist.Repository.ListTemplates`/`ListRuns` | `func (r *Repository) ListTemplates() ([]*Template, error)`; `func (r *Repository) ListRuns(onlyActive bool) ([]*Run, error)` (plus `GetTemplateItems`, `GetRunItems` per-parent) | `internal/checklist/repository.go:137,268` |
| `output.ModeFromFlags` / `output.New` / `(*Writer) Print` | `func ModeFromFlags(jsonFlag, ndjsonFlag bool) (Mode, error)`; `func New(w io.Writer, mode Mode) *Writer`; `func (wr *Writer) Print(v any) error` | `internal/output/output.go:32,26,52` |
| `index.Indexable` / `index.ShouldSkipDir` | `func Indexable(name string) bool`; `func ShouldSkipDir(name string) bool` | `internal/index/reconcile.go:621,625` |

**Todo node shape to emit (target for migrated gen-1 tasks)**, from
`internal/cli/todo.go:331-375`: `todos/<ULID>.md`, `type: "todo"`, `Props{state,
scheduled?, deadline?, repeat?}`, `Links{Rel:"depends-on"}` optional. Ephemeral
container shape (`todos/inbox.md`, `type: "todo-ephemeral"`) is out of scope —
gen-1 has no ephemeral-task concept to migrate from.

**Note node shape to emit**, from `internal/cli/note_v1.go` (create path,
~line 320-340): `notes/[<dir>/]<slug>.md`, `type: "note"` (or `--type`
override), `Props{title, description?, stage?, tags?}` (tags rendered as a
bracketed list string, not a `Link` — safe, `[a, b]` is not `[[a, b]]`),
`Aliases{slug, ...extra}`.

**Log-day/log-entry node shape to emit**, from `internal/node/logparser.go`:
one `log/<date>.md` file, day node `type: "log-day"`, `Aliases{date}`; each
entry is a `## HH:MM · author\nid:: <ULID>\n<body>\n` block
(`RenderLogEntry`/`RenderLogEntryWithDid`), parsed back out by `LogParser` at
index time into `type: "log-entry"` sub-nodes with `Time = date+"T"+hhmm+":00Z"`.
The day's `Props`/other frontmatter fields for intentions/wins/schedule have
**no defined v1 equivalent yet** — `rk today`/`rk todo` own scheduling and
`rk log` (T4) owns entries, but gen-1 `Intention`/`Win`/`ScheduleItem` don't
map 1:1 onto any shipped v1 type. **[OPEN]** planning must decide: intentions
→ todo nodes (durable, `scheduled` = the day)? wins → log entries tagged
`kind: win`? schedule items → todo `scheduled` props? This is unresolved by
any prior ticket and needs an explicit design call, not just code.

## 4. Pitfalls from docs/REVIEW_PATTERNS.md relevant here

- **Unwrapped/ignored errors** (`docs/REVIEW_PATTERNS.md:21-78`) — every
  per-legacy-item conversion step should `%w`-wrap and attribute to the
  specific source ID/file, mirroring `adopt.go`'s `res.addErrored(path, err)`
  pattern, not a bare `return err` that loses which of N items failed.
- **`os.Exit()` in library code** (`:175-201`) — the importer package itself
  must never call `os.Exit`; only the CLI's `RunE` returns errors for cobra to
  report.
- **Ignoring `--quiet`** (`:204-228`) — follow the exact
  `if !(mode == output.Pretty && quietFlag)` gate used throughout `internal/cli`,
  not a bare `if !quietFlag`.
- **Tests that don't test their name / missing edge cases** (`:232-295`) — for
  a migration importer the load-bearing edge cases are: empty legacy store
  (zero rows), a legacy row with a malformed/missing field, an xid that
  collides with another xid across types (both would produce the same alias —
  the flat alias namespace means a `journal.Task` xid and a `models.Note` xid
  could theoretically collide; unlikely in practice given xid's entropy, but
  worth one explicit test given the global-namespace design decision), and a
  legacy file already containing conflict markers (`node.Parse` refuses these
  — `internal/node/node.go:38-40,119-123` — the importer must treat that as a
  per-item error, not a crash).
- **Time/timezone: `time.Parse` vs `time.ParseInLocation`**
  (`docs/REVIEW_PATTERNS.md:929-955`, reckon-gcuu) — legacy `Task.CreatedAt`
  is parsed with plain `time.Parse("2006-01-02", ...)` (UTC midnight,
  `internal/journal/task_service.go:200-203`) while `journal.LogEntry.Timestamp`
  and other fields may carry different zone assumptions. When converting to
  the canonical node's `Time` (required RFC3339,
  composable-redesign.md:467), pick one consistent conversion rule and test
  it explicitly — this exact class of bug (UTC-parse vs. local-format
  mismatch) has bitten this codebase before.
- **Dead code after refactoring** (`:905-926`) — not directly triggered by
  new code, but relevant to review: once the importer ships and is run, the
  `internal/migrate` legacy-migration package/`rk migrate` command and
  possibly `internal/cli/note.go`+`notes.go` (legacy `rk note new`/`rk notes`)
  become candidates for removal in a *later* ticket; this ticket should not
  delete them (out of scope — the ticket is additive: build the importer,
  don't cut over) but should not extend them either.
