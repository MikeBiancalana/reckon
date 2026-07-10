# Acceptance Criteria — reckon-ih5g (v1-T8: note tool `rk note` + linking)

Read-only extraction, no implementation. Ticket text (verbatim, `bd show reckon-ih5g`):

> The zettelkasten pillar. rk note create/show; file-per-item note nodes; body
> [[wikilinks]] -> reference edges; frontmatter typed-edge props -> typed
> edges; aliases/slugs (Obsidian-compatible, global flat namespace,
> index-enforced uniqueness); dangling links allowed, stored as unresolved
> edges, auto-resolve on target creation, queryable. Done when: notes create
> and round-trip; forward links + backlinks (index-derived) are queryable; a
> dangling link resolves once its target exists; rename retains the old alias
> as a redirect; tested. FILENAME POLICY (settled 2026-07-09,
> km-architecture-proposal.md §4.2): note filename = title-derived slug (the
> canonical alias), NOT the ULID — notes are the browse-heavy type; identity
> stays inline (id: wins) so rename is free and slug churn rides the
> alias-redirect machinery. Raw [[ULID]] links may dangle in Obsidian (reckon
> resolver handles them); per-note escape hatch = list the ULID in aliases:.
> Also adopt the T8 frontmatter conventions from the proposal: title +
> description (mandatory-by-convention), optional stage:
> seedling|budding|evergreen, no timestamp/updated field (git is the
> versioning backplane; OKF timestamp derived at export). Generate
> notes/**/index.md (OKF progressive disclosure) as part of reconcile or
> rk note index.

**Grounding sources** (read in full or targeted; verified against actual code,
not just design prose):
`docs/design/km-architecture-proposal.md` (§4.1, §4.2, §4.2.1, §4.3, sources
list) — the settled design, tip commit `d758ed5` "docs: settle T8
note-filename policy (slug, not ULID) in proposal + bead"; `docs/design/
composable-redesign.md` (Node ID scheme decision, Canonical node spec +
Invariants, §"Design: link syntax (A#5) & alias namespace (A#6)"); `internal/
node/{node,parser,render,insert,envelope}.go`; `internal/index/{schema,
reconcile}.go`; `internal/cli/{todo,add,root,query,note,notes}.go`;
`internal/models/note.go`, `internal/parser/links.go`, `internal/config/
config.go` (legacy DB-backed notes system, for contrast); `internal/cli/
todo_test.go` (`TestTodoDependsOn_Dangling{Edge,ResolvesLater}` — the existing
generic dangling-edge mechanism, already proven at the index layer for a
different node type); sibling ticket-work docs `ticket-work/reckon-uv09/
acceptance-criteria.md` (T4, closest precedent — same template followed here)
and `ticket-work/reckon-9bfx/*` (M2 adopt/InsertField); beads `reckon-pfpb`,
`reckon-23z`, `reckon-29ln`, `reckon-akn`, `reckon-bmvu`, `reckon-lvam`,
`reckon-oxkb`, `reckon-qyvh`, `reckon-75r`, `reckon-sr7` (adjacent P3/P4 note
tickets, for scoping §5). Two claims below were verified directly by running
throwaway test cases against `internal/node` in this worktree (not committed;
deleted after use) rather than taken on faith from doc comments — see §2.4 and
§2.7.

---

## Open questions for the planner (do not guess past these)

Collected here (each also flagged inline) because each is load-bearing enough
to change the implementation's shape, not just a detail — mirroring the T4
acceptance-criteria doc's own practice:

1. **Alias-collision enforcement is a real, unresolved discrepancy between
   design prose and shipped code.** `composable-redesign.md`'s locked A#6
   decision says colliding alias mints are "**rejected / auto-disambiguated**"
   by the index. The only alias-collision mechanism that actually exists
   today (`internal/index/reconcile.go:433-479`, shipped under #142) is a
   **passive, non-fatal `Warning`** surfaced on the *next* reconcile pass,
   with the ambiguous alias resolved to an arbitrary-but-deterministic
   `node_key` (`ORDER BY a.node_key LIMIT 1`, `reconcile.go:381`) — nothing
   rejects a write or auto-renames a slug at creation time. `rk note create`
   must decide whether it adds its **own** proactive index-query-before-write
   guard (new pattern — no existing "add" command touches the index; see §2.1)
   or accepts the reconcile-only "detect after the fact" contract as what
   "index-enforced" actually means in practice. See §2.1/EC-1.
2. **The `notes/<topic>/` subdirectory has no selection mechanism specified
   anywhere.** The proposal's own tree diagram (`km-architecture-proposal.md`
   line 300) shows `notes/<topic>/slug.md` but nothing in the proposal, the
   bead, or `composable-redesign.md` says how `<topic>` is chosen — a
   `--dir`/`--topic` flag, a derived value (first tag?), or is nesting purely
   optional/manual with a flat `notes/slug.md` being equally valid? See §2.2.
3. **`rk note index` vs. folding into `reconcile` — the proposal genuinely
   does not pick one.** Both the proposal (§4.2.1) and the bead text present
   it as an "or": *"generate `notes/**/index.md` ... as part of reconcile or
   `rk note index`"*. See §2.3 for the reasoning and a recommended default;
   this is explicitly not a hard preference, so flag rather than assume.
4. **Rename-retains-alias-as-redirect has no existing safe primitive for
   Obsidian block-list-style `aliases:`.** Verified directly (§2.4): the
   existing `InsertField`/`SetField` pair silently corrupts the aliases list
   — producing a duplicate `aliases:` key whose newly-added value is dropped
   from the derived view on reparse — when a note's `aliases:` were authored
   in Obsidian's indented-list Properties-panel style rather than the inline
   `[a, b]` flow form. This is the single most architecturally significant
   gap this ticket must resolve, on par with T4's "single global parser"
   finding.
5. **Case-insensitive alias matching is a stated invariant that is not
   implemented.** `composable-redesign.md`'s A#6 decision says alias
   resolution is "case-insensitive match"; the shipped `resolveEdges`
   (`reconcile.go:374-386`) does a byte-exact SQL `=` comparison with no
   `COLLATE NOCASE` anywhere in `schema.go`. In practice this mostly doesn't
   bite because slugs are lowercased by convention (§2.6), but a
   hand-authored mixed-case `aliases:` entry (very plausible via Obsidian)
   will not resolve to a same-text-different-case link. See §2.6/EC-8.
6. **Whether a dedicated `rk note show`/`rk note backlinks` verb is in this
   ticket's scope is genuinely ambiguous, not just "obviously deferred."** The
   ticket's own *scope paragraph* explicitly says "`rk note create/show`," but
   a separate, still-open P4 bead `reckon-23z` is titled "Add `rk note show`
   and `rk note backlinks` CLI commands." The ticket's own "Done when" *test*
   clause never names a subcommand — it only requires forward-links/backlinks
   to be **queryable**, which the existing generic `rk query` surface over the
   `nodes`/`edges`/`aliases` views already satisfies for any node type with no
   new code (exactly how T4 satisfied its own query requirement for log
   entries). Recommended reading, **not a certainty**: `rk note create` and a
   minimal `rk note show <ref>` (single-note display, since the scope
   paragraph explicitly names it) are in scope; a dedicated `rk note
   backlinks` subcommand is `reckon-23z`'s job and out of scope here. See §5.

---

## 1. Explicit acceptance criteria

Split from the ticket's "Done when" sentence, filename-policy paragraph, and
the T8 frontmatter/index.md conventions the bead text explicitly folds in
(these are the same clusters named in the ticket, numbered for independent
verification):

- **AC-1 (create succeeds).** `rk note create` writes a new file-per-item note
  under `notes/` (subdirectory placement per §2.2) whose frontmatter carries a
  freshly minted `id:` (ULID), `type:`, and the note's title/description, and
  whose filename is the title-derived slug (`.md`), not the ULID.
- **AC-2 (round-trip holds).** The created file round-trips:
  `node.Parse(raw).Serialize() == raw` immediately after creation, matching
  the same keystone guarantee every other v1 create path (`todo add`, `add`)
  already provides via the `NewNode -> Render -> Parse -> writeFileAtomic`
  recipe (`internal/node/render.go` Invariant 6).
- **AC-3 (forward links queryable).** Body `[[wikilink]]` references produce
  `rel="references"` edges (`internal/node/node.go` `parseBodyLink`), and
  ref-valued frontmatter props (e.g. `depends: [[X]]`) produce typed edges
  whose `rel` = the prop key (`deriveView`) — both already-generic mechanisms,
  exercised end-to-end for a note file and retrievable via `rk query` over the
  `edges` stable view (`src`, `rel`, `dst`, `dst_key`).
- **AC-4 (backlinks queryable, index-derived only).** A note's inbound links
  are computable purely as `SELECT * FROM edges WHERE dst_key = '<node_key>'`
  — never stored on the note itself (Invariant 1, "forward + authored only ...
  all reverse/aggregate views are index-derived and never stored").
- **AC-5 (dangling link auto-resolves on target creation).** A `[[not-yet-
  created]]` link indexes as an edge with `dst_key IS NULL` before the target
  exists, and the *same* edge row resolves (`dst_key` becomes non-null) after
  the target note is created and the index reconciled/rebuilt — no edit to
  the source note required. (The underlying mechanism, `resolveEdges`, is
  already generic and already proven for a different node type — see
  `TestTodoDependsOn_DanglingResolvesLater`, `internal/cli/todo_test.go:1149`
  — so this AC is largely a "prove it also holds for note `references`
  edges," not new machinery, modulo §2.1/§2.6's gaps.)
- **AC-6 (rename retains old alias as redirect).** Renaming a note (title
  and/or filename change) leaves the note's **old** slug resolvable: the old
  slug is added to the note's `aliases:` list (not deleted), so `[[old-slug]]`
  links elsewhere in the vault keep resolving after the rename with **zero**
  edits to any other file. The note's `id:` (ULID) never changes across the
  rename (Invariant 2, "inline `id:` wins").
- **AC-7 (tested).** All of AC-1 through AC-6 have test coverage, including at
  least one end-to-end integration test proving create -> index -> query
  (mirroring T4's own "capture->index->query proven end-to-end" precedent,
  `ticket-work/reckon-uv09/acceptance-criteria.md` AC-5).

**Filename policy (settled 2026-07-09, proposal §4.2):**

- **AC-8.** The note's on-disk filename is the title-derived slug
  (`<slug>.md`), never the ULID — the opposite of `todos/<ULID>.md`'s
  convention, a deliberate, explicit per-type divergence.
- **AC-9.** Identity is carried **only** by the inline `id:` field; the
  filename/slug is non-authoritative and free to change (Invariant 2). No
  code anywhere may treat the filename as identity for a note.
- **AC-10.** A raw `[[ULID]]` link resolves through reckon's own resolver
  regardless of filename (via `_nodes.ulid` match in `resolveEdges`,
  independent of `_aliases`) but will **not** resolve as an Obsidian
  in-editor link unless that ULID is *also* listed in the target note's
  `aliases:` — the documented per-note escape hatch for raw-ULID links that
  must work inside Obsidian itself.

**Frontmatter conventions (proposal §4.2/§4.2.1):**

- **AC-11.** `title` and `description` are mandatory **by convention** (schema
  file / lint guidance), not a hard parse-time requirement — `node.go`'s
  `reservedKeys` set (`id, type, time, author, aliases`) does not include
  `title`/`description`, so structurally they are ordinary optional `Props`
  entries; "mandatory" here means enforced by `rk lint`/agent discipline, not
  by `rk note create` refusing to write without them (confirm with planner
  whether `rk note create` should hard-require `--title` as a CLI argument
  even though the stored field itself is soft-optional at the format level).
- **AC-12.** An optional `stage` prop is accepted with exactly the three
  values `seedling|budding|evergreen`; absent is legal and means "untracked."
  No other value is a valid `stage` (see EC-6).
- **AC-13.** The vault **never** stores an `updated`/`timestamp` field on a
  note. Only `time` (semantic/creation timestamp, reserved key) is stored;
  "last meaningful change" is derived at OKF-export time from git history,
  never persisted inline. A note frontmatter block containing a literal
  `updated:`/`timestamp:` key would be a proposal violation (though the parser
  itself would tolerate it generically as an unrecognized Prop — this is a
  convention/lint check, not a structural one).

**Generated index (proposal §4.2.1):**

- **AC-14.** `notes/**/index.md` files are generated (OKF progressive-
  disclosure catalog: title + one-line description per note, one `index.md`
  per notes subdirectory the glob implies) from the property-graph index,
  deterministically and without hand-editing. **Mechanism is genuinely
  unresolved by the proposal** (§2.3 below) — the proposal offers "as part of
  reconcile, or a separate `rk note index` verb" as an explicit either/or, not
  a preference. Recommended default (not a stated preference): fold into
  `reconcile`, since every other derived vault artifact (edges, aliases,
  warnings) is already a reconcile output and a fourth distinct
  freshness/staleness contract (`rk query`'s manual-only vs. `rk todo
  list`'s auto-reconcile vs. now a third for `index.md`) is a real, avoidable
  cost — but this is the planner's call to make explicitly, not infer.

---

## 2. Implicit requirements

### 2.1 Alias/slug uniqueness enforcement — verified gap between design prose and shipped code

`composable-redesign.md`'s locked A#6 decision: *"The index **enforces
uniqueness** (which Obsidian can't): a colliding alias mint is **rejected /
auto-disambiguated**."* The bead text echoes this almost verbatim
("index-enforced uniqueness").

What is actually shipped (`internal/index/reconcile.go:433-479`,
`aliasCollisionWarnings`, from ticket #142) is **not** rejection or
auto-disambiguation — it is a **passive `Warning`** (`Kind:
"alias_collision"`) computed fresh on every reconcile pass, listing every
alias claimed by more than one surviving node. Resolution for query purposes
is `ORDER BY a.node_key LIMIT 1` (`reconcile.go:381`) — an arbitrary but
deterministic pick, not a refusal. **Nothing today stops two notes from being
created with the same slug/alias**; the collision only becomes *visible* on
the next reconcile, as a queryable warning, with one of the two nodes winning
alias resolution arbitrarily until a human/agent renames one.

`rk note create` must decide (**[OPEN, §1.1]**):
  (a) Add its own proactive check — open (or reuse an already-open) index,
      query `aliases`/`nodes` for the candidate slug, refuse if claimed. This
      is a **new pattern**: no existing "add"-style command (`todo add`,
      `add`) touches the index at all today — they are pure vault-text
      writes, and only "list"-style commands (`todo list`) open the index.
      Adding an index dependency to `note create` also inherits the index's
      own staleness question (a slug that collides with a very recent,
      not-yet-reconciled write from another process/device would be missed).
  (b) Do a pure filesystem scan of `notes/**/*.md` for a matching slug (no
      SQLite dependency, but catches only *filename* collisions, not a slug
      claimed as an *alias* on a differently-named file elsewhere in the
      vault, or an alias on a **non-note** node — the namespace is "global
      flat," not scoped to notes).
  (c) Accept the shipped reconcile-Warning as the entire "enforcement"
      surface (rely on `rk lint`/`rk query` post-hoc detection, matching the
      literal shipped behavior, at the cost of literally matching the design
      prose's stronger "rejected" language).

### 2.2 `notes/<topic>/` placement — no selection mechanism specified

Neither the proposal, the bead, nor `composable-redesign.md`'s per-tool scope
table (`Note (zettel) | 1 file per note | ...`) specifies how the `<topic>`
path segment in `notes/<topic>/slug.md` (proposal §4.1 tree diagram, line
300) is chosen. Candidates, none textually confirmed: a `--dir`/`--topic`
CLI flag; derived from the first `tags` entry; or nesting is simply optional
and a flat `notes/slug.md` is equally valid (the diagram is illustrative, not
prescriptive). This also determines what "**/index.md" in AC-14 actually
globs over — one `index.md` at `notes/` root only, or one per populated
subdirectory. **[OPEN]**

### 2.3 `rk note index` vs. reconcile — proposal explicitly declines to pick

Direct quote, proposal §4.2.1: *"`rk note index` (**or** as part of
reconcile): generate `notes/**/index.md` ... Deterministic, derived, cheap."*
The bead text repeats the same "or." This is a genuine either/or in the
source material, not an oversight to resolve by re-reading more carefully.
Recommended default given in AC-14 above (fold into reconcile, avoid a third
freshness contract) — flagged, not asserted, per the task's instruction not
to guess past an open question.

### 2.4 Rename-retains-alias-as-redirect — verified primitive gap (most significant finding)

Directly verified in this worktree (throwaway `_test.go` in `internal/node`,
not committed) against three concrete frontmatter shapes:

| Existing `aliases:` shape | Primitive that must add the old slug | Result |
|---|---|---|
| Absent entirely | `InsertField("aliases", "[old-slug]")` | **Works** — inserts a clean new `aliases: [old-slug]` line. |
| Inline flow list, `aliases: [a, b]` | `SetField("aliases", "[old-slug, a, b]")` | **Works** — `HasField` is true, a real scalar span exists, splice succeeds cleanly. |
| Obsidian block-list style (`aliases:` then indented `- a` / `- b` lines — how Obsidian's own Properties panel writes it) | Neither has a safe path | **Corrupts.** `InsertField`'s existence check only inspects `n.fieldSpans` (`insert.go:25`), not `n.frontmatter` — and block-list values are deliberately recorded in `frontmatter` **without** a `fieldSpan` (`node.go:252-254`'s own comment: *"No fieldSpan: this value has no single contiguous byte span in Raw ... SetField correctly refuses to edit it"*). So `InsertField` does **not** error (it should, by its own doc comment's intent, but the check is keyed on the wrong map) and instead prepends a **second, duplicate** `aliases: [old-slug]` line ahead of the original block. On re-parse, `parseFrontmatter`'s last-line-wins overwrite means the **derived `n.Aliases` reverts to the original items only — the newly "inserted" old-slug is silently dropped**, while `n.fieldSpans["aliases"]` is left pointing at the now-superfluous first line (so a *subsequent* `SetField("aliases", ...)` would silently patch the wrong, orphaned line forever, never touching the real block list). |

This means: **if a note's `aliases:` were ever hand-authored via Obsidian's
Properties panel (the block-list style is Obsidian's own default UI output),
today's `InsertField`/`SetField` pair cannot safely append a redirect alias to
it** — a silent data-loss bug, not a clean refusal. This is the single
highest-severity implicit requirement in this ticket: AC-6 cannot be
considered met for real-world (Obsidian-authored) vault content without
either (a) a new edit primitive that handles the block-list case, (b) a
policy of normalizing `aliases:` to flow-list form on any note the tool
touches (a rewrite the byte-preservation invariant is normally reluctant to
do, though arguably in-scope for *this specific field* on a rename, since
rename is already a mutation), or (c) an explicit, documented limitation.
**[OPEN — architecturally significant, do not improvise silently.]**

### 2.5 File-per-item note is the *simple* case relative to T4's log tool — stated for balance

Unlike log's group-file day parser (T4's biggest architectural blocker — a
day file splits into N+1 nodes and required a new per-directory parser-
dispatch mechanism, `ticket-work/reckon-uv09/acceptance-criteria.md` §2.3), a
note is one file = one node, exactly what the existing default
`node.MarkdownParser{}` already does (`internal/node/parser.go:16-25`,
already wired as `index.Open`'s default,
`internal/index/index.go` per the T4 doc's own citation). **No parser-
dispatch work is needed for notes** — this is materially simpler than the
sibling ticket, and should not be over-engineered by analogy to T4's harder
problem.

### 2.6 Case-insensitive alias matching is documented but not implemented

`composable-redesign.md`'s A#6: *"case-insensitive match"* for aliases. The
shipped `resolveEdges` (`reconcile.go:374-386`) does a plain SQL `=` against
`_aliases.alias` / `_edges.dst`, with no `COLLATE NOCASE` declared anywhere in
`schema.go`'s `_aliases`/`_edges` table DDL. Slugs generated by the existing
precedent slugifier (`internal/parser/links.go`'s `NormalizeSlug` — legacy
system, but the only in-repo precedent for "Obsidian-compatible slug from
title") are always lowercased, which papers over most of this in the common
case (tool-generated slugs never collide case-only with themselves), but a
hand-authored `aliases:` entry with any uppercase character will not match a
same-text link differing only in case. Confirm whether T8 must add
`COLLATE NOCASE` (or lowercase-normalize at write and read time) or accepts
this as a pre-existing, out-of-ticket gap. **[OPEN, low-probability but
real.]**

### 2.7 `id:` reserved-key precedent confirms Render()'s frontmatter key order will not match the proposal's illustrative example verbatim

`node.Render()` (`internal/node/render.go:22-31`) emits frontmatter in a
**fixed canonical order**: `id, type, time, author, aliases`, then `Props`
sorted alphabetically by key, then typed-edge links sorted by `(rel, to)`.
Since `title`/`description`/`tags`/`stage` are **not** in `reservedKeys`
(`node.go:44-46` — only `id, type, time, author, aliases` are reserved), they
land in `Props` and render alphabetically: `description, stage, tags, title`
— not the proposal's illustrative order (`id, type, title, description,
tags, time, author, stage, aliases`, §4.2's example block). This is not a
contradiction (the round-trip/content invariants never promised key *order*
stability against a doc example, only `parse(serialize(x)) == x`), but it is
worth stating explicitly so a test asserting exact frontmatter text against
the proposal's example verbatim would be testing the wrong thing — assert on
parsed fields, not literal key order, unless byte-for-byte order is later
decided to matter for human readability (not stated as a requirement
anywhere).

### 2.8 `tags` is not a structurally parsed list at the node-package level

The proposal's frontmatter example shows `tags: [pas, integrations]`, and
`composable-redesign.md`'s field table calls tags "queryable, not
pseudo-links" (§2.1 pain-points table pitch). But `tags` is not a reserved
key, so it is stored as an ordinary `Props` **string** value — the literal
text `"[pas, integrations]"` — not parsed into a `[]string` the way `Aliases`
is a first-class typed field. Any tag-based filtering (`rk query` or a future
`--tag` flag on `rk note list`, out of scope per §5 anyway) would need to
parse that bracket-string itself at query time; this is pre-existing generic
behavior (applies to any tool's tags, not T8-specific), not a defect to fix
in this ticket, but worth naming so nobody assumes tags are queryable as a
list column out of the box.

### 2.9 CLI conventions to match (established, not open)

Modeled on `internal/cli/todo.go`'s `addDurableTodo` and `internal/cli/
add.go`'s `runAddE` (the two closest, already-shipped v1 file-per-item /
group-file create paths):
- `Annotations: map[string]string{"requiresDB": "false"}` — notes operate on
  vault text directly; the *legacy* operational DB (`internal/storage`) is
  unrelated and must not be initialized (`root.go:93-104`'s walk-ancestors
  check).
- `output.ModeFromFlags(jsonFlag, ndjsonFlag)` + `output.New(cmd.OutOrStdout(),
  mode).Print(res)`, suppressed under `mode == output.Pretty && quietFlag` —
  the uniform pattern every v1 command follows (`output.go`, `todo.go:129-134`,
  `add.go:129-133`).
- `--quiet`/`-q` (`root.go:109`), `--vault` (`root.go:114`), `--json`/`--ndjson`
  (mutually exclusive, enforced in `PersistentPreRunE`, `root.go:85-87`) are
  global persistent flags already available with no new wiring.
- `resolveAuthor(flag string) string` (`todo.go:246-257`) should be reused
  verbatim for any `--author` on note creation, not reimplemented.
- `writeFileAtomic` (`adopt.go:250-284`, temp-file + `os.Rename`) for all
  writes; CRLF-file guard (`reckon-vj55`, checked in both `todo.go` and
  `add.go`) should extend to any note file the tool edits (e.g. on rename).
- No `os.Exit` anywhere in command `RunE` — return a wrapped error;
  `cmd/rk/main.go:11` is the sole `os.Exit(1)` call site, on `Execute()`'s
  returned error (`root.go:189-205`).
- **Naming note (lower stakes than T4's collision, but real):** the
  **legacy**, DB-backed `internal/cli/note.go` (`noteCmd`, `Use: "note"`) and
  `internal/cli/notes.go` (`notesCmd`, `Use: "notes"`) already occupy the
  `rk note`/`rk notes` command names, both wired at `root.go:118-119`
  (`GetNoteCommand()`/`GetNotesCommand()`), both backed by
  `internal/models.Note` (xid IDs, `~/.reckon/notes/` via
  `config.go:72-85`'s `NotesDir()`, a completely different store from the
  vault's `notes/` directory this ticket targets). `reckon-29ln`
  ("Consolidate note and notes CLI commands") exists precisely because of
  this naming overlap. This ticket must decide how the new vault-backed `rk
  note create` coexists with (or replaces a subcommand of) the existing
  `noteCmd`/`notesCmd` tree — the same shape of decision T4 faced and
  resolved by graduating the `addCmd` stub under a fresh name rather than
  fighting the legacy `logCmd`. **[OPEN, but lower severity than §2.1/§2.4 —
  a registration/naming decision, not a data-loss risk.]**

---

## 3. Edge cases

| # | Edge case | Notes |
|---|---|---|
| EC-1 | Two notes created with the same title (hence the same slug) | Exposes §2.1 directly: today's shipped mechanism does not reject the second write; it only becomes a queryable `alias_collision` warning on the next reconcile, with an arbitrary-but-deterministic node winning resolution. Legacy precedent (`internal/cli/notes.go:557-563`) *does* hard-reject on slug collision in the **old** DB system — a real prior-art data point for "reject" being an established pattern in this codebase, even though the new index's collision surface doesn't do that today. |
| EC-2 | Rename to a title whose slug is already claimed by another note | Compounds EC-1 with AC-6: the rename path must decide whether landing on an already-claimed slug is itself rejected (preferred, given EC-1's ambiguity) or silently creates a second alias collision. |
| EC-3 | `[[link]]` to a nonexistent note, then that note is later created | AC-5's core scenario. Already proven generically at the index layer for a different edge type/rel (`TestTodoDependsOn_DanglingResolvesLater`, `todo_test.go:1149-1184`) — the risk surface here is a note-specific parser or resolution quirk, not the resolve mechanism itself. |
| EC-4 | Note with no `description` | Legal per AC-11 (soft convention, not structurally enforced) — the parser stores nothing extra and doesn't error; `rk lint`/schema-file discipline is the only enforcement layer, and per §1.1's [OPEN] item, `rk note create` itself may or may not additionally require `--title`/`--description` as CLI args regardless of the stored field being optional. |
| EC-5 | Invalid `stage` value (e.g. `stage: mature`) | Not one of `seedling\|budding\|evergreen` (AC-12). The parser has no enum validation anywhere (`Props` is a generic open string bag) — an invalid value parses and stores fine at the node-package level; only a note-tool-level or `rk lint`-level check could catch it. Decide whether `rk note create --stage` (if such a flag exists) validates against the enum at write time. |
| EC-6 | `[[link\|display text]]` pipe syntax | Already handled generically by `splitRef` (`node.go:508-517`, strips `|label`) for body wikilinks — should just work for notes with no new code, but must be included in note-specific tests rather than assumed by analogy. |
| EC-7 | `[[ULID]]` raw links | Resolves via reckon's resolver (`_nodes.ulid` match, independent of aliases) regardless of the target's filename; will not resolve in Obsidian's own UI unless the ULID is also listed in the target's `aliases:` (AC-10's escape hatch) — this divergence (resolves in reckon, dangles in Obsidian) is expected/by-design, not a bug, and should be an explicit test rather than an assumption. |
| EC-8 | Unicode / special characters in the title, used for slugification | No slugify function exists yet for the new vault note tool (only the legacy `internal/parser/links.go` `NormalizeSlug` precedent: lowercases, keeps `unicode.IsLetter`/`IsDigit`/`-`/`_`/space, collapses/trims hyphens, truncates at 200 chars). Whether T8 reuses this exact algorithm, a variant, or something new is unstated — but whatever is chosen must be validated against Obsidian's own filename restrictions (illegal characters per OS: `\ / : * ? " < > \|` at minimum) since the slug becomes a real filename, not just an alias string as in the legacy system. Also ties to §2.6 (case-insensitive alias matching) since slugification's lowercasing is doing load-bearing work there today. |
| EC-9 | Empty title (after trimming, or title that slugifies to the empty string, e.g. a title of only punctuation/emoji) | The legacy path explicitly guards this (`internal/cli/notes.go:552-555`: *"title '%s' produces invalid slug (contains only special characters)"*). The new tool needs an equivalent guard — no precedent yet in the vault-backed code path. |
| EC-10 | A title that slugifies to `index` (e.g. title "Index") | Collides with the OKF-reserved generated `notes/**/index.md` filename (AC-14) — a genuinely non-obvious collision between ordinary user content and a reserved, tool-generated file. Not mentioned anywhere in the proposal or bead; flag as a real gap for the planner to decide (reject the slug `index` outright, or namespace generated index files differently, e.g. never named exactly `index.md` at a level that also holds a same-named note). |
| EC-11 | Vault directory / `notes/` subdirectory does not yet exist | Mirrors `todo.go`'s and `add.go`'s existing `os.MkdirAll` pattern (`todo.go:296`, `add.go:120`) — should need no new design, just consistent application. |
| EC-12 | CRLF-authored note file (Syncthing/Windows/hand-edited) | Extend the existing `reckon-vj55` CRLF-refusal guard (`todo.go`, `add.go`) to any note-file mutation path (create is unaffected since it's a fresh write; rename/alias-append on an *existing* hand-authored file is the actual risk). |
| EC-13 | Note whose `aliases:` were authored in Obsidian's block-list style, then renamed | The concrete trigger for §2.4's verified corruption bug — must be an explicit test case, not just a flow-list-only test suite (which would pass while hiding the real-world-common case). |
| EC-14 | Global-namespace collision between a note's alias and a **different node type's** alias/slug (e.g. a todo's alias) | The "global flat namespace" language (bead text, A#6) means uniqueness is vault-wide, not scoped to `notes/`. A slug generated for a note that happens to match an existing todo's alias is the same class of problem as EC-1, just cross-type — confirm the uniqueness check (however §2.1 resolves) queries across all node types, not just other notes. |

---

## 4. Test scenarios (Given/When/Then)

Command names assume `rk note create <title>` / `rk note create --title
"..."` per the closest existing convention family (`rk todo add`, `rk add`);
adjust mechanically once §2.9's naming question is resolved.

**TS-1** (AC-1/AC-2/AC-8, fresh create)
Given an empty vault with no `notes/` directory,
When `rk note create "PAS Entity Model"` is run,
Then `notes/pas-entity-model.md` (or `notes/<topic>/pas-entity-model.md` per
§2.2) is created with frontmatter containing a freshly minted `id:` (a valid
ULID) and `type: note` (or the chosen default type), the command exits 0, and
`node.Parse(raw).Serialize()` on the resulting bytes equals the file's
on-disk bytes exactly.

**TS-2** (AC-9, filename is not identity)
Given the note from TS-1,
When the file is renamed on disk directly (outside the tool) to a different
filename, without touching its `id:` field,
Then `rk index`/reconcile still resolves the node by its unchanged `id:`
(`node_key` keyed on ULID, not path) — proving filename never carries
identity.

**TS-3** (AC-3, forward links: body wikilink -> references edge)
Given note A's body contains `[[note-b]]`, and note B exists with alias/slug
`note-b`,
When the index is (re)built,
Then `rk query "SELECT rel, dst, dst_key FROM edges WHERE rel='references'
AND src=<A's node_key>"` returns exactly one row with `dst_key` equal to B's
`node_key` (resolved, not dangling).

**TS-4** (AC-3, typed edge from a ref-valued frontmatter prop)
Given note A's frontmatter contains `depends: "[[note-b]]"` (or whatever
prop key the note schema uses for a typed relationship),
When the index is (re)built,
Then `rk query "SELECT rel, dst_key FROM edges WHERE rel='depends' AND
src=<A's node_key>"` returns one row with `rel='depends'` (not
`'references'`) and a resolved `dst_key`.

**TS-5** (AC-4, backlinks are index-derived only)
Given note A links to note B,
When `rk query "SELECT src FROM edges WHERE dst_key=<B's node_key>"` is run,
Then it returns A's node_key — and note B's own on-disk file contains no
inbound-link data whatsoever (grep the raw file for A's ULID/alias/title
finds nothing), proving backlinks are computed, never stored.

**TS-6** (AC-5, dangling link auto-resolves — mirrors
`TestTodoDependsOn_DanglingResolvesLater`)
Given a note is created containing `[[not-yet-created]]` in its body, and no
note with that slug/alias exists yet,
When the index is built,
Then `rk query "SELECT dst_key FROM edges WHERE rel='references' AND
dst='not-yet-created'"` returns exactly one row with `dst_key` NULL;
When a new note with slug/alias `not-yet-created` is subsequently created and
the index rebuilt,
Then the same query now returns `dst_key` equal to the new note's node_key —
no edit to the original linking note required.

**TS-7** (AC-6, rename retains old alias — flow-list aliases, the clean case)
Given a note at `notes/old-title.md` with `aliases: [old-title]` (inline
flow-list form) and another note elsewhere linking `[[old-title]]`,
When the note is renamed to "New Title" via the tool's rename path,
Then the file becomes `notes/new-title.md`, its `aliases:` now includes both
`old-title` and `new-title` (or equivalent), its `id:` is unchanged, and the
other note's `[[old-title]]` link still resolves (to the same node_key) after
reindex.

**TS-8** (AC-6/EC-13, rename retains old alias — Obsidian block-list aliases,
the gap case)
Given a note whose `aliases:` are authored in Obsidian's indented block-list
style (`aliases:\n  - old-title`),
When the note is renamed via the tool's rename path,
Then (per §2.4's verified finding) **naive use of today's `InsertField`/
`SetField` would silently corrupt the file / drop the appended alias from the
derived view on reparse** — this scenario should be written explicitly
(even if initially expected to fail) to force the planner's decision in §2.4
before sign-off, exactly as T4's own AC doc did for its EC-9 (embedded `## `
line).

**TS-9** (AC-11/EC-4, no description present)
Given `rk note create "Some Title"` is run with no description supplied,
When the file is parsed,
Then the command succeeds (no hard parse-time requirement) and `description`
is simply absent from Props — confirming AC-11's "soft, convention-level"
reading, pending the planner's confirmation of whether the CLI itself should
still require `--description` as an argument.

**TS-10** (AC-12/EC-5, invalid stage value)
Given a note is written (by hand or via a not-yet-existing `--stage` flag)
with `stage: mature` (not one of the three legal values),
When the file is parsed,
Then the node package itself does not error (Props has no enum validation) —
this scenario documents the current permissive floor and pins whichever
additional validation (CLI-level or `rk lint`-level) the planner decides to
add, rather than assuming one exists.

**TS-11** (AC-13, no timestamp/updated field)
Given a note is created and later hand-edited (content change, not via the
tool),
When the file is re-parsed,
Then no `updated`/`timestamp` frontmatter key is present or expected — the
git commit history (or file `mtime`) is the sole source of "last changed,"
confirmed by there being no code path anywhere that writes such a key.

**TS-12** (EC-6, pipe syntax in a body link)
Given note A's body contains `[[note-b|Note B's Nicer Title]]`,
When the index is built,
Then the resulting `references` edge's `dst` is `note-b` (the pipe and label
text are stripped, per `splitRef`), resolving to note B exactly as an
unlabeled `[[note-b]]` link would.

**TS-13** (EC-7, raw ULID link resolves in reckon, dangles in Obsidian
without the escape hatch)
Given note A's body contains `[[01J9Z3K7Q2W8XR4M6N0V5BYHED]]` (note B's raw
ULID) and note B's filename is its slug (not its ULID) with no ULID entry in
its own `aliases:`,
When the index is built,
Then `rk query` shows the edge resolved (`dst_key` = B's node_key, via
`_nodes.ulid` match) — while a note that B's ULID is **not** listed in its
`aliases:` means the same link would show as a broken/red link inside
Obsidian's own UI (documented, expected divergence, not asserted via `rk
query` since it's an Obsidian-side behavior, but should be called out in the
test's comments as the reason the two systems diverge here).

**TS-14** (EC-9, empty/invalid-slug title rejected)
Given `rk note create "!!!"` (a title that slugifies to the empty string) or
`rk note create ""`,
When run,
Then the command errors with a clear, non-zero-exit message (mirroring the
legacy path's `"title '%s' produces invalid slug"` guard,
`internal/cli/notes.go:553-555`) rather than silently creating a file with an
empty or garbage filename.

**TS-15** (EC-10, title slugifies to the reserved `index`)
Given `rk note create "Index"` is run,
When the resulting slug collides with the OKF-reserved generated
`notes/index.md` (or `notes/<topic>/index.md`),
Then **[OPEN, pending planner decision]** either the create is rejected with
an explicit "reserved filename" error, or the generated index file is
namespaced differently at that directory level — this scenario cannot be
finalized without §"EC-10"'s open decision, but must exist so the collision
is a deliberate choice rather than an accidental first-repro-in-production
discovery.

**TS-16** (AC-14, generated index.md reflects current notes)
Given two notes exist, each with a `title` and `description`,
When the index-generation step runs (mechanism per §2.3 — either `rk note
index` or folded into reconcile),
Then `notes/index.md` (or the relevant subdirectory's) contains one entry per
note with its title and one-line description, deterministically regenerated
(re-running with no vault changes produces byte-identical output) — proving
"derived, cheap, deterministic" as the proposal names it, not accumulated/
hand-mergeable state.

**TS-17** (EC-1, duplicate title/slug collision)
Given a note already exists with slug `duplicate-example`,
When `rk note create "Duplicate Example"` is run again (same slugification
result),
Then **[OPEN, pending §2.1's resolution]** either the second create is
rejected outright (matching the legacy system's precedent,
`notes.go:557-563`) or it succeeds and the resulting `alias_collision`
warning is visible via the next reconcile/`rk lint` pass — this scenario's
expected outcome cannot be finalized without §2.1's decision, but must be
written explicitly either way.

---

## 5. Explicitly out of scope

Checked against the proposal, the bead text, and the adjacent P3/P4 beads
that exist specifically to carve pieces away from a general "notes" epic:

- **`rk note list`/`rk note search`, fuzzy TUI search, tag-filter search** —
  explicitly a separate bead, `reckon-pfpb` ("No way to list or search
  zettelkasten notes"), which frames itself against the **legacy** `rk notes
  create/edit/show` command family, not this ticket's new vault-backed tool,
  but the functional carve-out (list/search is a distinct concern) applies
  equally here. Not this ticket's job.
- **A dedicated `rk note backlinks` (or `rk note show`'s backlinks display)
  CLI subcommand** — likely out of scope; confirm. A separate, still-open P4
  bead `reckon-23z` is titled exactly "Add `rk note show` and `rk note
  backlinks` CLI commands," which is in real tension with this ticket's own
  scope-paragraph wording ("`rk note create/show`"). Recommended reading
  (§1.6/§1's open-questions list): a minimal `rk note show <ref>` (single-note
  display) is in scope since it's explicitly named in the ticket's own
  prose, but a dedicated `backlinks` subcommand is `reckon-23z`'s job —
  the "Done when" clause's backlinks-queryable requirement is satisfied
  generically by `rk query` over `edges`, with no dedicated verb needed.
  **Likely out of scope for the backlinks-subcommand specifically; confirm.**
- **Consolidating the legacy `rk note`/`rk notes` commands with the new
  vault-backed tool** — a separate bead, `reckon-29ln` ("Consolidate note and
  notes CLI commands"). This ticket must *coexist* with (or graduate past) the
  legacy commands (§2.9) but a full consolidation/cleanup is that bead's job,
  not this one's.
- **TUI note features** — notes browser (`reckon-oxkb`), backlinks pane
  (`reckon-bmvu`), note editing in TUI (`reckon-lvam`), wikilink highlighting
  (`reckon-qyvh`), a `TagBrowser` component (`reckon-sr7`) — all separate,
  still-open P3/P4 beads. None of this ticket's "Done when" clause requires
  TUI surface at all.
- **Note graph export for visualization** — `reckon-75r`, separate P3 bead.
  Not mentioned by this ticket.
- **Note *editing*** (beyond create + the rename path AC-6 requires) — an
  `rk note edit`/general content-mutation command is not named by the "Done
  when" clause (which only requires create/round-trip/links/rename); treat
  general editing as likely out of scope, **confirm** — the rename path
  itself is in scope only insofar as it's needed to prove AC-6.
- **T9 (migration of the legacy DB-primary data to text-truth)** —
  `reckon-s6oh`, a separate, still-open P2 ticket that this one *blocks*
  (per `bd show`'s dependency graph) but does not itself perform. This ticket
  does not migrate, delete, or read `internal/models.Note` /
  `internal/storage`'s legacy notes tables.
- **FTS ranking/ranking UI beyond the existing `fts_search`/`fts` view
  surface** (already shipped, T2/`reckon-a4eh`) — a note's `body` is indexed
  into `fts_search` for free via the generic `insertNode` path, identically
  to any other node type; no note-specific search-ranking work is implied.
- **`rk export --okf` (the actual OKF bundle exporter)** — proposal §4.2.1
  names this as a *follow-on* to T8 ("This is the interop door ..."), not part
  of T8 itself. T8 only needs the vault's on-disk shape to be OKF-conformant
  *by convention* (frontmatter fields, no stored timestamp); emitting an
  actual export bundle, running `okf-lint`/`okf-conformance` in CI, and `kiso`
  publishing are explicitly named as separate, later work (proposal §4.2.1,
  §4.3).
- **`rk-ingest`/`rk-lint-deep` agent porcelains** (Karpathy-brain
  ingest/judgment-lint operations, proposal §4.3) — explicitly framed as
  work that comes "after T8," not part of it.
- **The vault schema file itself** (proposal §4.2.1's "single most important
  file in the repo" — type taxonomy, evergreen-writing discipline,
  stage-transition rules) — the proposal says write it "alongside T8," which
  could be read as in-scope-adjacent documentation work, but it is not a code
  acceptance criterion and produces no testable behavior; **likely out of
  scope for this specific bead's Done-when clause, confirm** whether the
  planner wants it delivered as part of this ticket's PR regardless.
- **Circles/sharing (`rk circle`, alias namespacing for shared repos,
  §4.9)** — explicitly labeled "v2, design-captured only" in the proposal
  itself; zero code changes needed for v1/T8.
- **MCP porcelain** — `reckon-cxx1` (v1-T10, fast-follow), not this ticket.
- **`rk today`/agenda surfacing of notes** — `reckon-liml` (v1-T7), a
  sibling, not a dependency of this ticket in either direction per `bd show`.
- **Multi-vault / cross-vault note aggregation** — `rk note` operates on the
  single resolved `cfg.VaultDir`, like every other v1 tool (no ticket text
  anywhere suggests otherwise).
