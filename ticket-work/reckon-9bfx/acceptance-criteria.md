# Acceptance Criteria: Review M2 — ULID mint policy + `rk adopt` for id-less (Obsidian) files (reckon-9bfx)

Source: ticket `reckon-9bfx` description (from `foundation-review-2026-06-24` finding
**M2**), the review doc's disposition entry, and `docs/design/composable-redesign.md`
invariant #7. Blocks `reckon-ih5g` (v1-T8, `rk note`) and `reckon-s6oh` (v1-T9,
DB-primary → text-truth migration) — both need the mint/adopt story settled before
they can mint ULIDs for notes they create or aliases they migrate.

## Current-state facts (grounded in code)

- **Part (a) — policy — is already recorded.** The review's disposition
  (`docs/design/foundation-review-2026-06-24.md` line 63-65) says "M2 — policy
  recorded (design invariant #7)", and `docs/design/composable-redesign.md` lines
  501-506 already contain invariant #7 verbatim: *"Index never mutates truth files
  (ULID mint policy, 2026-06-24). Tools that create nodes mint a ULID; the indexer
  never writes into source files, so a hand/Obsidian-authored id-less file keys on
  `file:<relpath>` and is not rename-stable until a tool touches it. An explicit,
  opt-in `rk adopt` stamps ULIDs into id-less files (a user-initiated mutation),
  giving Obsidian-authored files a path to first-class without the index ever
  writing truth."* This ticket's obligation for (a) is therefore to **verify** this
  text is present, accurate, and consistent with the shipped index behavior — not
  to author it from scratch. (It should still be listed as an explicit AC below,
  since the ticket's "Done when" names it, and because verification can fail — e.g.
  if the invariant text drifts from what `rk adopt`/reconcile actually do.)
- **Part (c)'s target does not exist yet as CLI code.** There is currently no
  production "tool create path" in the new (v1, `internal/node`-based) architecture
  to wire mint-on-create into: `rk query` (`internal/cli/query.go`) and `rk index`
  (`internal/cli/index.go`) are the only shipped v1 commands, and both are
  read-only. `addCmd` and `todoCmd` (`internal/cli/stubs.go`) are unimplemented
  stubs (`errNotImplemented`). The actual create tools — `rk log` (T4,
  `reckon-uv09`), `rk todo` (T5, `reckon-qiua`), `rk note` (T8, `reckon-ih5g`) — are
  separate, still-open tickets; `reckon-ih5g` *depends on* this ticket, it isn't
  built by it. The legacy `rk task`/`rk notes`/`rk log` commands
  (`internal/cli/task.go`, `notes.go`, `log.go`) are pre-redesign, DB/`internal/models`-
  backed, and never touch `internal/node` at all — they are out of scope (see §5).
  The shared mint-on-create primitive itself — `node.NewNode` (mints via
  `node.Mint`) + `node.Render` — **was already built and gate-tested** by the H1
  fix (`reckon-s0ix`, closed): `internal/node/render.go`, gated by
  `internal/node/render_test.go` (`TestCreateRoundTrip`,
  `TestNewNodeMintsAndRoundTrips`). See §2 for how this ticket scopes (c) given
  that reality.
- **`node.SetField` cannot add a missing key** — its doc comment says so
  explicitly ("adding a missing key (inserting a frontmatter line) is a separate
  concern, out of scope for the byte-preservation keystone"), and
  `TestSetFieldMissingKey` (`internal/node/node_test.go:223`) asserts it errors.
  `rk adopt`'s "stamp `id:` into an id-less file" therefore needs **new** span-safe
  insert logic (in `internal/node` or the `rk adopt` command itself) — it cannot
  reuse `SetField` as-is for the no-frontmatter-block and
  frontmatter-without-`id`-key cases.
- **The index already implements the `file:<relpath>` surrogate-key policy** that
  invariant #7 describes: `nodeKey` in `internal/index/reconcile.go:255-266`
  returns the inline ULID when present, else `"file:" + rel`; the comment at
  `reconcile.go:578-582` and `internal/index/AGENTS.md` ("ULID-less files rename as
  delete+add") both confirm id-less files are not rename-stable today, matching the
  policy. `reckon-5b44` (M3/M4, merged #142) added duplicate-ULID/alias-collision
  *warnings* to reconcile but did not change this keying. **No index-package code
  change is required by this ticket** (see §5).
- **No existing atomic/span-safe "insert frontmatter key" helper exists anywhere in
  the codebase** to reuse — `rk adopt` will be the first tool in the new
  architecture that mutates a truth file at all (`rk query`/`rk index` never
  write into the vault). There's also no precedent yet for how a v1 write-command
  reports results (`--json`/`--ndjson`/pretty via `internal/output`) since none
  exists; `rk adopt` should follow the `internal/output.Writer` convention used by
  `rk index` (`indexSummary`/`indexResult` in `internal/cli/index.go`) for
  consistency, but that's an implementation choice, not a literal AC.
- The vault root and its ignore rules are defined by `internal/config.Config.VaultDir`
  and `internal/index`'s walk (`internal/index/reconcile.go:594-606`,
  `internal/index/AGENTS.md`): directories `.git/`, `.obsidian/`, `.reckon/`,
  `.stversions/` are skipped, as are dotfiles, non-`.md` files, and
  `*.sync-conflict-*` files. `rk adopt` walking a directory argument should honor
  the same ignore rules for consistency with what the index will (not) pick up.

## 1. Explicit Acceptance Criteria

Derived directly from the ticket's "Done when" clause (three bundled sub-criteria):

1. **(a) The policy is recorded in the design doc.** `docs/design/composable-redesign.md`
   states, as a numbered invariant, that (i) the index never mutates truth files,
   (ii) id-less files key on `file:<relpath>` in the index and are not
   rename-stable until a tool touches them, (iii) tools that *create* nodes mint a
   ULID via `node.NewNode`/`Render`, and (iv) `rk adopt` is the explicit, opt-in,
   user-initiated path for stamping ULIDs into pre-existing id-less
   (Obsidian-authored) files. This already exists as invariant #7 (lines 501-506)
   — this AC is satisfied by confirming that text is present, still accurate
   against current code, and not contradicted elsewhere in the doc (e.g. by the
   M2 disposition note at `foundation-review-2026-06-24.md:63-65`, which should
   also be checked for consistency, though as a historical/append-only review doc
   it doesn't need editing per the `reckon-lrw2` convention for such docs).
2. **(b) `rk adopt [path...]` exists, stamps `id:` into id-less files via a
   span-safe write, and is tested.**
   - The command accepts one or more file/directory paths as positional args.
   - For each id-less markdown file in scope, it inserts a new `id: <ULID>`
     frontmatter line, minting the ULID the same way `node.NewNode`/`node.Mint`
     do (see AC 3 below — same mint function, same 26-char Crockford-base32
     format).
   - The write is **span-safe**: it is a targeted insert (new frontmatter line
     added), not a full-file regenerate/reformat. Every byte of the file outside
     the inserted `id:` line — other frontmatter keys/values/order/comments/
     whitespace, and the entire body — is preserved exactly (see AC 2's
     "span-safe" implicit requirements below for the precise byte-preservation
     contract).
   - Command and helper logic are covered by tests (`internal/cli/adopt_test.go`
     or equivalent) exercising at least the edge cases enumerated in §3.
3. **(c) Mint-on-create is wired in the tool create paths.** Every code path that
   *creates* a new node (as opposed to editing/adopting an existing file) mints
   its ULID via `node.NewNode` (which calls `node.Mint`) and renders via
   `node.Render`, never leaving a freshly created file id-less. Given the current
   codebase (§ "Current-state facts"), this ticket's concrete obligation is:
   - Confirm `node.NewNode`/`node.Render` remain the **one** shared mint-on-create
     primitive (no second, competing "build markdown from fields" code path is
     introduced) — this was delivered by `reckon-s0ix` and gated by
     `render_test.go`; this ticket does not need to re-derive it, only avoid
     regressing it and (if it touches `internal/node` at all for AC 2's span-safe
     insert) keep it clearly distinct from — not a fork of — `Render`.
   - Since no v1 CLI create command exists in the tree yet to "wire" this into
     (see facts above), this sub-criterion is **satisfied by the primitive
     existing, being correct/tested, and being the documented contract** that
     `reckon-uv09`/`reckon-qiua`/`reckon-ih5g` (T4/T5/T8) must follow when they
     land — record that expectation explicitly (e.g. in the design doc and/or
     each downstream ticket) rather than leaving it implicit. If the implementer
     judges that literally nothing is left to "wire" beyond what H1 already did,
     that's a valid resolution of this AC, but it must be stated as a deliberate
     scoping decision, not silently dropped.

## 2. Implicit Requirements

- **The index must never be mutated by `rk adopt` as a side effect.** `rk adopt`
  writes only to the truth file(s) named/discovered on disk. It must not open,
  write to, or otherwise touch the SQLite index (`internal/index`) — no
  `index.Open`/`Rebuild`/`Reconcile` call as part of adopting. The index picks up
  the now-ULID'd file through the **normal** reconcile/rebuild path the next time
  `rk index` or a lazy `Reconcile()` runs, same as any other file edit. (This
  mirrors invariant #7's "index never mutates truth files" — the corollary here is
  "and `rk adopt`, despite mutating truth, never mutates the index directly
  either; the two stay decoupled.")
- **`rk adopt` is idempotent / safe to re-run.** Running `rk adopt` on a file that
  already has a non-empty `id:` must not mint a second ULID, must not duplicate
  the `id:` key, and must not alter the file at all (byte-identical no-op) unless
  the implementer explicitly decides adopt should also *report* already-adopted
  files (fine — but the file itself must stay untouched). Running `rk adopt`
  twice in a row on a truly id-less file must mint exactly once (the second run
  sees the id already present from the first and no-ops).
- **"Span-safe write" means byte-preservation outside the touched span**,
  consistent with the `node` package's existing keystone (`Serialize()`,
  `SetField`'s span-splice discipline in `node.go:122-150`): no reformatting of
  unrelated frontmatter, no key reordering, no whitespace/line-ending
  normalization, no clobbering of any existing key (including keys `node`
  doesn't recognize/reserve), and the body is untouched byte-for-byte. Concretely:
  `adopt(serialize(f))` should differ from `f` by exactly one inserted line (plus,
  where a frontmatter block must be created from scratch, the minimal `---`
  delimiters) — nothing else. This is stronger than merely "produces
  semantically-equivalent YAML"; it is byte-for-byte outside the inserted span,
  matching the round-trip discipline the rest of the codebase already holds
  itself to (H1/`render_test.go`, the spike's fuzz gate).
- **Line-ending preservation.** If a file uses CRLF (a known parser-scope gap,
  `foundation-review-2026-06-24.md` M4-class note: "CRLF files fail the `---\n`
  prefix check … whole file treated as body, all typed fields empty" — tracked
  separately under `reckon-vj55`), `rk adopt`'s behavior on such a file must be
  decided and tested, not silently mishandled (see §3 "malformed/unparseable
  frontmatter"). It should not introduce mixed line endings into an
  otherwise-consistent file.
- **The minted ULID must be produced by the same function `node.NewNode` uses**
  (`node.Mint`/`node.MintAt` in `internal/node/ulid.go`) — same 26-character
  Crockford-base32 ULID, same monotonic-entropy source — so an adopted file's `id:`
  is indistinguishable in format/behavior from a tool-created node's `id:`. This
  is explicit in the ticket's own framing: "so downstream tools treat adopted
  files identically to tool-created ones."
- **`rk adopt` must not require the index to exist or be current.** Since it's a
  pure filesystem operation on truth files (per the design's "index is derived and
  disposable" principle, `internal/index/AGENTS.md`), it should work even with no
  cache/index directory present yet — likely `Annotations: {"requiresDB": "false"}`
  in Cobra terms, matching `rk query`/`rk index`'s pattern in
  `internal/cli/root.go`.
- **Consistency with existing frontmatter conventions.** The inserted `id:` line
  should follow the canonical key ordering `Render` uses when a full frontmatter
  block must be freshly created (`id, type, time, author, aliases, then props...`,
  `internal/node/render.go:22-31`) to the extent that's meaningful for an
  *insert* into an already-existing, arbitrarily-ordered Obsidian frontmatter
  block — but for the common case (frontmatter exists, `id:` is simply missing),
  the natural/simplest choice (e.g. prepend `id:` as the first line inside the
  existing `---`/`---` block) should be a deliberate, documented, tested choice,
  not an accident of implementation.
- **No behavior change to `node.SetField`.** `rk adopt`'s insert logic is
  additive (new capability), and must not weaken `SetField`'s existing contract
  (still errors on a missing key) or the round-trip guarantees it and `Render`
  currently pass (`TestSetFieldMissingKey`, the H1 gate tests) — regression
  coverage should confirm those still pass.

## 3. Edge Cases

- **File with no frontmatter block at all** (body-only markdown, or empty file).
  `rk adopt` must insert a full `---\nid: <ULID>\n---\n` block ahead of the
  existing content, preserving the existing content (including its own leading
  whitespace/blank lines, if any) byte-for-byte after the inserted block.
- **File with a frontmatter block that has no `id:` key.** The common case:
  insert an `id:` line into the existing block; every other line in the block and
  the entire body stay byte-identical.
- **File that already has an `id:` key.** Per idempotency (§2): must be a
  no-op (byte-identical file, non-error exit) — not a silent skip that still
  exits 0 vs. an explicit "already adopted"/skip message is an implementer
  choice, but it must **not** overwrite, duplicate, or otherwise touch the
  existing `id:` value. (An explicit `--force`/re-mint mode is plausible future
  scope but is not required by the ticket text and should not be assumed without
  being called out.)
- **Malformed or unparseable frontmatter** — e.g. an unterminated `---` block
  (per `node.go`'s `parseFrontmatter`, an unterminated block means the *whole
  file* is treated as body with no frontmatter recognized at all), a git
  conflict-marker file (`node.Parse`/`ParseAt` explicitly refuses these,
  returning an error — `internal/node/node.go:112-117`), or a CRLF file (parsed
  as body-only per the known M4 gap). `rk adopt` must handle each without
  crashing or corrupting the file: conflict-marker files should surface the
  parser's refusal error and skip that file (non-zero exit or per-file error
  reported, existing files untouched); an unterminated-`---` file is
  indistinguishable from "no frontmatter" to the parser today, so adopt's
  behavior here should be decided and tested explicitly (most consistent
  choice: treat it the same as "no frontmatter block", i.e. insert a fresh block
  — but that changes what looks like a frontmatter attempt into a nested block,
  so this needs a conscious call, not silent inheritance of parser behavior).
- **Multiple paths passed to `rk adopt`, mix of already-id'd and id-less files.**
  Each file is handled independently: id-less files get stamped, already-id'd
  files no-op, and one file's outcome (success, no-op, or error) must not block
  or corrupt processing of the others. The command's summary output (however
  `rk adopt` reports results) should distinguish adopted vs. skipped vs. errored
  files, particularly under `--json`/`--ndjson` (existing `internal/output`
  convention).
- **Path that is a directory vs. a single file.** A directory argument should
  walk it recursively for candidate files, applying the same file-type/ignore
  rules the index uses (`.md` only; skip `.git/`, `.obsidian/`, `.reckon/`,
  `.stversions/`, dotfiles, `*.sync-conflict-*` — `internal/index/reconcile.go:594-606`)
  so `rk adopt <vaultdir>` behaves predictably and doesn't try to stamp
  frontmatter into non-markdown or index-ignored files. A single-file argument
  that doesn't end in `.md` is a distinct edge case (next bullet).
- **Non-markdown file passed explicitly.** Decide and test: reject with an error
  (preferred, to avoid corrupting an arbitrary file with an injected YAML-ish
  block), vs. silently skip. Given `rk adopt`'s destructive-if-wrong nature
  (mutates a user's file), erroring loudly on an explicit non-`.md` path is the
  safer default and should be the tested behavior unless explicitly decided
  otherwise.
- **Path not under the vault root.** Decide and test whether `rk adopt` is
  vault-scoped (resolves/validates paths against `config.Config.VaultDir`,
  rejecting paths outside it) or is a general-purpose filesystem tool usable
  anywhere (no vault-root check at all, since ULID minting itself has no
  vault dependency). Given the ticket frames this as vault-facing tooling ("For
  Obsidian-authored id-less files the user wants first-class") and the index is
  inherently vault-scoped, a vault-containment check (mirroring the
  cache-inside-vault guard pattern in `config.LoadWithOverrides`,
  `internal/config/config.go:151-157`) is the more consistent default — but this
  is a judgment call the implementer must make explicitly and cover with a test
  either way, not leave unspecified.
- **Concurrent/repeated runs (idempotency).** Covered under §2 idempotency;
  additionally, two `rk adopt` invocations racing on the *same* file
  concurrently (e.g. from two terminals/scripts) is a narrower concern — at
  minimum the second writer must not corrupt the file (e.g. via a
  non-atomic partial write); a full concurrency/locking guarantee analogous to
  the index's reconcile-writer flock is not required by the ticket text, but
  "doesn't corrupt the file under a same-process re-run" is a hard requirement,
  and "doesn't corrupt under an interrupted/crashed write" (partial write) should
  be considered — e.g. write-to-temp-then-rename rather than truncate-and-write
  the original file in place.
- **What ULID format/mint function must be used.** Explicit in the ticket:
  "must match `node.NewNode`'s minting so downstream tools treat adopted files
  identically to tool-created ones." Concretely: `rk adopt` must call
  `node.Mint()` (or `node.MintAt`) directly — not reimplement ULID generation,
  not use a different library/format, not truncate/transform the ULID string in
  any way before writing it into the `id:` line.

## 4. Test Scenarios (Given/When/Then)

### `rk adopt` command

**Scenario: stamps a missing `id:` into an existing frontmatter block**
Given a file `note.md` with `---\ntype: note\n---\nbody text\n` (no `id:` key)
When `rk adopt note.md` is run
Then the file has a new `id:` line inside the frontmatter block whose value is a
valid 26-char ULID
And every other byte of the original file (the `type: note` line, `---`
delimiters, `body text`) is unchanged
And re-parsing the file with `node.Parse` yields `ULID != ""` and
`Type == "note"` and `Body == "body text\n"` (matching the pre-adopt body)

**Scenario: inserts a whole frontmatter block into a file with none**
Given a file `plain.md` containing only `just some text\n` (no `---` block)
When `rk adopt plain.md` is run
Then the file now begins with `---\nid: <ULID>\n---\n` followed by
`just some text\n`, byte-identical to the original content after the inserted
block

**Scenario: no-op on a file that already has an `id:`**
Given a file `a.md` with `---\nid: 01J9Z3K7Q2W8XR4M6N0V5BYHED\ntype: note\n---\nbody\n`
When `rk adopt a.md` is run
Then the file's bytes are unchanged
And the original `id:` value is unchanged
And the command exits 0 (or reports "already adopted" per the implementer's
chosen skip-reporting, but does not error)

**Scenario: repeated adopt is idempotent**
Given an id-less file `b.md`
When `rk adopt b.md` is run twice in a row
Then the first run mints and stamps an `id:`
And the second run is a byte-identical no-op (same file content after run 2 as
after run 1, same ULID value — no second mint)

**Scenario: mixed batch — some files already id'd, some not**
Given `x.md` (id-less), `y.md` (already has `id:`), and `z.md` (id-less), all
passed in one invocation
When `rk adopt x.md y.md z.md` is run
Then `x.md` and `z.md` each get a freshly minted, distinct `id:`
And `y.md` is untouched
And no error from `y.md`'s already-adopted state prevents `x.md`/`z.md` from
being stamped

**Scenario: directory argument walks and adopts eligible files, skipping
ignored paths**
Given a directory tree containing `notes/a.md` (id-less), `notes/b.md` (already
id'd), `.obsidian/config.md`, `.git/x.md`, and `notes/readme.txt`
When `rk adopt notes/` (or `rk adopt .` at the vault root) is run
Then `notes/a.md` is stamped
And `notes/b.md`, `.obsidian/config.md`, `.git/x.md`, and `notes/readme.txt` are
all left untouched (ignored by the same rules the index uses)

**Scenario: explicit non-markdown file is rejected**
Given a file `data.json` with arbitrary JSON content
When `rk adopt data.json` is run
Then the command errors (non-zero exit) without modifying `data.json`

**Scenario: conflict-marker file is skipped with an error, not corrupted**
Given a file `conflict.md` containing `<<<<<<< HEAD\n...\n=======\n...\n>>>>>>> other\n`
When `rk adopt conflict.md` is run
Then the command reports an error for that file (mirroring `node.Parse`'s
refusal) and does not modify `conflict.md`

**Scenario: adopt never touches the index**
Given a fresh vault with an id-less file and no index/cache directory yet on disk
When `rk adopt <file>` is run
Then it succeeds without creating an index/cache directory or database file
And a subsequent `rk index` run (or lazy reconcile) is what picks up the file's
new ULID — `rk adopt` itself performed no index read/write

**Scenario: minted ULID matches `node.NewNode`'s format**
Given an id-less file
When `rk adopt` stamps it
Then the inserted `id:` value parses as a valid ULID via the same validation
`node.Mint`'s output would satisfy (26-char Crockford base32, time-sortable
prefix), i.e. indistinguishable from an `id:` written by `node.NewNode`

**Scenario: path outside the vault root** *(exact behavior per implementer's
documented decision — test whichever is chosen)*
Given `--vault /path/to/vault` and a target file outside that directory
When `rk adopt /elsewhere/file.md` is run
Then it either (a) errors, rejecting the out-of-vault path, or (b) proceeds
unrestricted — whichever the implementation documents — and the test asserts
that documented behavior, not an assumed one

### Mint-on-create wiring (`node.NewNode`/`Render`, regression coverage)

**Scenario: `node.NewNode` always mints a non-empty ULID**
Given `node.NewNode("todo", "mike", "buy milk")` is called
When the returned node is inspected
Then `n.ULID != ""` and is a valid 26-char ULID (existing coverage:
`TestNewNodeMintsAndRoundTrips`, `internal/node/render_test.go` — cite as
regression, not new work)

**Scenario: a node built via `NewNode` + `Render` round-trips through Parse**
Given a node built by `node.NewNode` with typed fields set, then `Render()`ed to
bytes
When those bytes are re-parsed via `node.Parse`
Then the reparsed node's typed view matches the original (existing coverage:
`TestCreateRoundTrip` — cite as regression)

**Scenario: no second/competing create-path primitive is introduced**
Given the full `internal/node`, `internal/cli`, and any code this ticket adds
When searched for markdown-from-fields construction (`grep` for hand-built
`"---\n" + ...` frontmatter assembly outside `node.Render`/the `rk adopt` insert
path)
Then the only two places a node's frontmatter is *authored from scratch* are (1)
`node.Render` (create/import/promotion) and (2) `rk adopt`'s span-safe insert
(stamping into an existing file) — no third, ad hoc renderer exists

## 5. Out of Scope

- **Bulk/automatic adoption without explicit user action.** No auto-adopt on
  `rk index`/`Reconcile`, no background daemon, no "adopt everything in the
  vault" implicit default triggered by another command. `rk adopt` is only ever
  invoked directly by the user (or a script they control) naming path(s) — this
  is the explicit point of the ticket ("user-initiated mutation, never an
  automatic index side effect").
- **Changes to `internal/index`'s own behavior.** The index already treats
  id-less files as `file:<relpath>` (confirmed in "Current-state facts" above,
  per the M3/M4 ticket `reckon-5b44` merged separately). This ticket should not
  need to modify `internal/index/reconcile.go`, `schema.go`, or any index test —
  only ensure truth-file tools (create paths, `rk adopt`) mint/stamp IDs, and let
  the index pick the result up through its existing, unmodified reconcile path.
- **Building `rk log` / `rk todo` / `rk note` themselves.** T4 (`reckon-uv09`), T5
  (`reckon-qiua`), and T8 (`reckon-ih5g`) are separate tickets that will
  implement the actual create commands and, when they do, are expected to call
  `node.NewNode`/`Render` (per AC 3 / invariant #7). This ticket does not
  implement those commands or their business logic — it ensures the mint
  primitive they must use already exists, is correct, and is documented as the
  contract.
- **Any Obsidian-plugin-side work.** No changes to, or new development of, an
  Obsidian plugin/extension. `rk adopt` is a reckon CLI command operating on
  plain files; how (or whether) a user runs it from within Obsidian is outside
  this ticket.
- **Migrating existing DB-primary data** (legacy SQLite tasks/notes/checklists,
  xid→ULID aliasing, journal markdown → log nodes). That is `reckon-s6oh` (T9),
  which this ticket *blocks* but does not implement. `rk adopt` operates on
  plain id-less markdown files already in the vault; it is not a database
  importer and has no xid/alias-migration behavior.
- **Enforcing frontmatter-subset limitations (block-scalar YAML, CRLF, inline-
  code-inert links, multi-target ref props).** Those are `reckon-vj55`
  (M-parser-scope)'s job. `rk adopt` should behave *sanely* (not corrupt data) on
  such files per the edge cases in §3, but extending the parser's supported
  subset is not this ticket's work.
- **A `--force`/re-mint mode that overwrites an existing `id:`.** Not requested
  by the ticket text; idempotency (§2) requires adopt to leave an already-id'd
  file alone. Adding a way to *replace* an existing ULID is a different, riskier
  feature (breaks inbound links) and is not in scope here.
- **Locking/concurrency guarantees beyond "don't corrupt on a normal re-run."**
  The index has a dedicated reconcile-writer flock (`internal/index/lock_unix.go`);
  `rk adopt` is not required to add an equivalent cross-process lock for
  simultaneous adopts of the same file, only to avoid corrupting the file via a
  non-atomic write (see §3's concurrent/repeated-runs note).
