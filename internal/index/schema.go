package index

// SchemaVersion is the physical schema version of the index. The index is a
// derived, disposable store: there are NO migrations. When this constant is
// greater than the value persisted in _index_meta, Open performs a full rebuild
// from the vault text instead.
//
// v2 (reckon-a4eh): the FTS5 store was promoted from the private `_fts` table to
// the public, MATCH-capable `fts_search` vtable.
// v3 (reckon-fnqs.3): added the derived `_nodes.title` column (first non-empty
// body line, computed in insertNode).
const SchemaVersion = 3

// BuilderVersion identifies the code that built the index (display/debounce only,
// never correctness).
const BuilderVersion = "v1-T2"

// schemaDDL creates the private physical tables and the STABLE PUBLIC VIEWS that
// form the query contract (callers read only the views; physical tables stay
// private so storage can be refactored without breaking callers). The sole
// public physical object is `fts_search`: an fts5 vtable must carry its own name
// for MATCH to resolve, so it is exposed directly (no leading underscore) as the
// sanctioned full-text surface. The `fts` view stays for plain column scans.
//
// Identity: node_key is the inline ULID when present, else a surrogate
// "file:<relpath>". ulid holds the real inline ULID (may be ”). Edges store the
// raw parser target in dst (a ULID or alias, unresolved); dst_key is filled by
// the resolver pass (NULL = dangling).
const schemaDDL = `
CREATE TABLE _nodes (
    node_key TEXT PRIMARY KEY,
    ulid     TEXT NOT NULL DEFAULT '',
    type     TEXT NOT NULL DEFAULT '',
    time     TEXT NOT NULL DEFAULT '',
    author   TEXT NOT NULL DEFAULT '',
    body     TEXT NOT NULL DEFAULT '',
    title    TEXT NOT NULL DEFAULT '',
    loc_file TEXT NOT NULL,
    hash     TEXT NOT NULL,
    mtime    INTEGER NOT NULL
);
CREATE INDEX _nodes_ulid ON _nodes(ulid);
CREATE INDEX _nodes_type ON _nodes(type);

CREATE TABLE _edges (
    src_key   TEXT NOT NULL,
    rel       TEXT NOT NULL,
    dst       TEXT NOT NULL,
    dst_key   TEXT,
    from_frag TEXT NOT NULL DEFAULT '',
    to_frag   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX _edges_src ON _edges(src_key);
CREATE INDEX _edges_dst_key ON _edges(dst_key);
CREATE INDEX _edges_dst ON _edges(dst);

CREATE TABLE _props (
    node_key TEXT NOT NULL,
    key      TEXT NOT NULL,
    value    TEXT NOT NULL,
    PRIMARY KEY (node_key, key)
);

CREATE TABLE _aliases (
    alias    TEXT NOT NULL,
    node_key TEXT NOT NULL,
    PRIMARY KEY (alias, node_key)
);

CREATE VIRTUAL TABLE fts_search USING fts5(id UNINDEXED, body);

CREATE TABLE _file_meta (
    path  TEXT PRIMARY KEY,
    hash  TEXT NOT NULL,
    mtime INTEGER NOT NULL,
    ulids TEXT NOT NULL
);

CREATE TABLE _index_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE VIEW nodes AS
    SELECT node_key AS id, ulid, type, time, author, body, loc_file AS loc, title FROM _nodes;
CREATE VIEW edges AS
    SELECT src_key AS src, rel, dst, dst_key, from_frag, to_frag FROM _edges;
CREATE VIEW node_props AS
    SELECT node_key AS id, key, value FROM _props;
CREATE VIEW aliases AS
    SELECT alias, node_key AS id FROM _aliases;
CREATE VIEW fts AS
    SELECT id, body FROM fts_search;
`

// dropDDL tears down every physical table and view. Order: views first (they
// depend on tables), then tables.
const dropDDL = `
DROP VIEW IF EXISTS nodes;
DROP VIEW IF EXISTS edges;
DROP VIEW IF EXISTS node_props;
DROP VIEW IF EXISTS aliases;
DROP VIEW IF EXISTS fts;
DROP TABLE IF EXISTS _nodes;
DROP TABLE IF EXISTS _edges;
DROP TABLE IF EXISTS _props;
DROP TABLE IF EXISTS _aliases;
DROP TABLE IF EXISTS fts_search;
DROP TABLE IF EXISTS _file_meta;
DROP TABLE IF EXISTS _index_meta;
`
