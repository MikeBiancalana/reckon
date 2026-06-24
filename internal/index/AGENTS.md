# Index Subsystem Guide

## Overview

`internal/index` is reckon's **derived, disposable, per-device property-graph
index** (v1-T2). Markdown in the vault is the source of truth; this SQLite store
is a rebuildable projection over the canonical nodes from `internal/node`, used
for fast query (`rk query`, T3) and graph traversal.

It lives in the **cache dir**, never inside the vault, never synced (a synced live
SQLite file would corrupt). Path: `<CacheDir>/<vault_id>/index.db`, where
`vault_id = sha256(abs(VaultDir))[:12]`.

**Key files:**
- `index.go` — `Index` handle, `Open`/`OpenWithParser`, `Close`, `DB`, `Meta`, `DBPath`.
- `schema.go` — physical DDL + public views; `SchemaVersion`, `BuilderVersion`.
- `reconcile.go` — `Rebuild`, `Reconcile`, the mark-and-sweep core, resolver.
- `lock_unix.go` / `lock_other.go` — the reconcile-writer flock.

## The query contract: STABLE PUBLIC VIEWS

Consumers query the **views**, never the `_`-prefixed physical tables (those are
private so storage can be refactored without breaking callers):

| View | Columns |
|------|---------|
| `nodes` | `id, ulid, type, time, author, body, loc` |
| `edges` | `src, rel, dst, dst_key, from_frag, to_frag` |
| `node_props` | `id, key, value` |
| `aliases` | `alias, id` |
| `fts` | `id, body` |

`id` is the node's index identity: its inline **ULID** when present, else a
surrogate `file:<relpath>`. Edge `dst` is the raw parser target (a ULID or alias,
unresolved); `dst_key` is the resolved node id, or NULL = **dangling**.

> **Known wrinkle for T3:** FTS5 `MATCH` needs the virtual table, not a view; the
> `fts` view exposes columns only. T3 decides the sanctioned MATCH surface.

## Freshness model (design A#4)

- **Lazy reconcile-on-read** (`Reconcile`) — correctness backstop; catches
  add/edit/delete/rename uniformly, incl. out-of-band edits (Syncthing, git).
- **Explicit full rebuild** (`Rebuild`, `rk index`) — recovery / schema migration.
- Write-through (tools updating inline) is a later optimization, not here.

**Change detection is hash-authoritative.** mtime is only a fast-path to skip
unchanged files; the content `hash` (sha256) is the authority.

**Rename is free:** nodes are keyed by ULID, so a moved file keeps its node row
(loc updated) and its backlinks (resolver re-links). ULID-less files rename as
delete+add (rename-stability is a ULID property by design).

## Schema versioning

The index is derived → **no migrations**. `Open` compares stored
`_index_meta.schema_version` to `SchemaVersion`; a mismatch (or new/empty store)
triggers a full `Rebuild`. Bump `SchemaVersion` whenever the schema changes.

## Concurrency

WAL serialises the underlying writes; a single **reconcile-writer flock** on
`index.lock` serialises `Rebuild`/`Reconcile` across processes. Each pass runs in
one transaction.

## Conventions / pitfalls

- Wrap every error with context (`fmt.Errorf("index: …: %w", err)`); **no `os.Exit`** in the library.
- Malformed files (git conflict markers, Syncthing conflict copies) are logged and
  skipped — a reconcile never crashes on bad input.
- Ignore globs: dirs `.git/ .obsidian/ .reckon/ .stversions/`; files that are
  dotfiles, non-`.md`, or `*.sync-conflict-*`.
- Determinism: the row *set* is content-derived (walk order irrelevant); tests
  compare a sorted dump. Keep new resolver/aggregation queries order-stable.
- The indexer takes a `node.Parser` (default `MarkdownParser`, one node/file); the
  log tool (T4) plugs in a group-file parser via `OpenWithParser`.

## Testing

`index_test.go` covers: view population, deterministic rebuild, reconcile
add/edit/delete, ULID-keyed rename (loc + backlink survival), mtime fast-path,
schema-version auto-rebuild, ignore globs + conflict markers, dangling edges,
cache-inside-vault rejection. `../cli/index_test.go` covers the `rk index` command.
