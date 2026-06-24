# Foundation Review — canonical node + index (v1 T0–T2)

> Status: **independent code review** of the foundational layers, by Godfrey (a session that did not build them).
> Date: 2026-06-24 · Scope: the pieces "if these are off, everything else is askew."

## Scope reviewed

Read in full: `internal/node/{node,envelope,parser,ulid}.go`; `internal/index/{schema,reconcile,index,lock_unix,lock_other}.go`; `internal/cli/index.go`; `internal/output/output.go`; `internal/config/config.go`; `cmd/rk/main.go`; `internal/spike/roundtrip/roundtrip.go`. Spanning commits T0 (dispatcher/config/layout), T1 (canonical node), T2 (SQLite index), and the gating spike.

## Verdict

**The foundation is sound and the load-bearing call is correct.** The keystone — `Raw` is authoritative, `Serialize()` returns it verbatim, typed fields are a derived projection, edits are span-splices (`SetField`) + reparse (`node.go:122-150`) — genuinely makes `parse(serialize(parse(f))) == f` true *by construction*, not by careful regeneration. That is the right answer to the failure the whole design exists to prevent, and it directly retires the old `writer.go` fixed-order-regen lossiness. The index is also strong: hash-authoritative mark-and-sweep, mtime fast-path with a content-identical-mtime-moved refresh for git/Syncthing (`reconcile.go:142-161`), malformed-skip-and-retry, stable public views over private tables, ULID-keyed rename stability, dangling edges resolved to NULL by a resolver pass. Well above the prior bar. Not building on sand.

The concerns below are about **completeness and the edges**, not the core decision.

## Concerns

### HIGH — structural

**H1. The round-trip guarantee covers *edit*, not *create* — and create is where the historical lossiness lived.**
`Serialize()` only byte-preserves an already-parsed file. `SetField` explicitly *cannot add a missing key* (`node.go:134-137`). `node.Mint()` is implemented but **called nowhere** (`ulid.go`; confirmed by grep). There is no shared "build a new node → render canonical markdown" primitive. Every create path (T4 log, T5 todo, T8 note) and the import/promotion path must render *new* markdown from fields — and that rendered text must itself satisfy `parse∘serialize` identity, or the node's **first edit** misbehaves. The gating spike proved *edit*-round-trip; it did **not** prove *create-then-edit* round-trip. This is exactly where old `writer.go` got lossy (fixed-order regeneration). The design knows creation is per-tool (`parser.go:27-32` notes model→text "is a promotion concern handled by a per-tool writer"), but two gaps remain: (a) no shared rendering primitive, so three tools will re-solve it three ways; (b) the round-trip fuzz gate does not extend to created/imported nodes.
→ **Before T4:** add one shared `Render(node) → text` and a gate test asserting `parse(render(n))` reproduces the canonical node. Hold the create path to the same discipline the edit path already meets.

### MED

**M2. ULID minting is unsourced → hand/Obsidian-authored files are second-class.**
The parser reads `id:` but never mints; the index keys a ULID-less file as `file:<relpath>` (`reconcile.go:230-240`). A note authored in Obsidian (no `id:`) is therefore **not rename-stable** (rename changes its key; old key swept, inbound edges dangle) and **not ULID-linkable** until a tool touches it. This may be intentional (the index must never mutate truth files), but Obsidian-authoring is a stated goal. → Confirm the policy and define where mint-on-first-touch happens.

**M3. No duplicate-ULID detection.** `_nodes.ulid` is indexed but **not UNIQUE** (`schema.go:33`). A copied or Syncthing-duplicated file = two nodes claiming one identity; `INSERT OR REPLACE` collapses `_nodes` while `_edges`/`_fts` accumulate, and resolution goes ambiguous — silently. Under multi-writer/sync this will occur. → reconcile should detect and warn on duplicate ULIDs.

**M4. Alias-uniqueness is not enforced** — though the design says the index enforces it. `_aliases` PK is `(alias, node_key)`, so one alias on two nodes is allowed; `resolveEdges` silently picks the lowest `node_key` (`reconcile.go:348-355`). For Obsidian-imported files with colliding aliases (a risk the design itself flagged), links resolve arbitrarily with no warning. → enforce + flag on reindex, or amend the design claim.

**M5. Legacy config overlaps the new vault at `~/.reckon`.** `config.go` still carries the pre-redesign surface — `DataDir`/`JournalDir`/`TasksDir`/`NotesDir`/`DatabasePath`/`LogDir` (→ `~/.reckon`, `~/.reckon/journal`, `~/.reckon/reckon.db`) — alongside the new `Config{VaultDir,CacheDir}` whose `VaultDir` *also* defaults to `~/.reckon`. The old DB-primary reckon writes `~/.reckon/reckon.db` + `~/.reckon/journal/*.md` while the new tools treat `~/.reckon` as the text vault root. Two code paths, one directory, different truth models → a collision/migration hazard tied to T9 (the long pole). The legacy funcs also `mkdir` as a side effect, contradicting the new "resolution is pure" principle. → separate or retire the legacy config surface before tools write into the vault.

**M4 vs Raw safety — the parser scope note.** All M4-class limits below keep `Raw` safe (byte-preservation holds) but make the *derived view*, and therefore the index, wrong:
- Single-line **scalar frontmatter only** (`node.go:88`, inherited from the spike): block-style YAML (`aliases:\n  - a`) is invisible to the typed view.
- **CRLF** files fail the `---\n` prefix check (`node.go:156`) → whole file treated as body, all typed fields empty.
- **Inline code** `` `[[x]]` `` is linkified (only fenced blocks are skipped, `node.go:232-251`) → spurious `references` edges.
- **Multi-target ref props** (`depends: [[A]], [[B]]`) match neither the single-ref regex nor stay useful — dropped to a plain prop string.
→ Enumerate the supported frontmatter/markdown subset and add tests for the real Obsidian shapes, since the same vault is edited by Obsidian.

### LOW

- **L1. `Open` does not reconcile-on-read.** `Open`→`ensureSchema` only rebuilds when new/stale (`index.go:104-120`); an existing current-schema index is served as-is. `Reconcile()` is implemented but **called nowhere** (only `rk index` → full `Rebuild`). The design's "lazy reconcile-on-read" requires every read entry point (T3 `rk query`, the agenda) to call `Reconcile()` first; that contract is unproven until T3 wires it. Both `Reconcile()` and `Mint()` are integration-unexercised today.
- **L2. Two NDJSON emitters.** `node.WriteNDJSON` guards against a literal newline in the encoded line (`envelope.go:78-80`); `output.Writer` NDJSON mode does not. Node emission should route through the guarded `node.WriteNDJSON` to preserve the one-physical-line invariant.
- **L3. Windows lock is a no-op** (`lock_other.go`); cross-process reconcile coalescing is unavailable there (tx + WAL still guard correctness). darwin (the dev target) uses `flock`, unaffected.
- **L4. mtime fast-path misses mtime-preserving edits** (`cp -p`, some restore tools) — inherent to the design; `rk index` full rebuild is the recovery.

## Bottom line

The hard call (byte-preserving keystone) is correct and cleanly built; the index is solid. The one thing to nail before tools land on the foundation is **H1 — the create/render path needs the same round-trip discipline and gate the edit path already has.** **M2–M5** are cheap now and ugly to retrofit once data exists (especially the `~/.reckon` config overlap and duplicate-ULID detection). Everything in LOW is wiring that the next tickets (T3/T4) will exercise — track that they do.
