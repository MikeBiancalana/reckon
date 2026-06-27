package index

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/node"
)

// Stats summarises a reconcile/rebuild pass.
type Stats struct {
	Scanned  int // markdown files seen on disk
	Reparsed int // files (re)parsed this pass (new or content-changed)
	Deleted  int // files removed from the index (gone from disk)
}

// Rebuild performs a full, deterministic rebuild from vault text: it drops and
// recreates the physical schema, then indexes every file. The resulting row set
// is content-derived, so walk order does not affect it.
func (ix *Index) Rebuild() error {
	unlock, err := ix.lock()
	if err != nil {
		return err
	}
	defer unlock()

	tx, err := ix.db.Begin()
	if err != nil {
		return fmt.Errorf("index: begin rebuild tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(dropDDL); err != nil {
		return fmt.Errorf("index: drop schema: %w", err)
	}
	if _, err := tx.Exec(schemaDDL); err != nil {
		return fmt.Errorf("index: create schema: %w", err)
	}
	if err := ix.initMeta(tx); err != nil {
		return err
	}
	if _, err := ix.reconcileTx(tx); err != nil {
		return err
	}
	now := nowStamp()
	if err := setMeta(tx, "last_full_rebuild_at", now); err != nil {
		return err
	}
	if err := setMeta(tx, "last_reconcile_at", now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("index: commit rebuild: %w", err)
	}
	return nil
}

// Reconcile performs a lazy, hash-authoritative reconcile-on-read: it picks up
// adds, edits, deletes and renames since the last pass without a full rebuild.
// mtime is a fast-path to skip unchanged files; the content hash is the authority.
func (ix *Index) Reconcile() (Stats, error) {
	unlock, err := ix.lock()
	if err != nil {
		return Stats{}, err
	}
	defer unlock()

	tx, err := ix.db.Begin()
	if err != nil {
		return Stats{}, fmt.Errorf("index: begin reconcile tx: %w", err)
	}
	defer tx.Rollback()

	st, err := ix.reconcileTx(tx)
	if err != nil {
		return Stats{}, err
	}
	if err := setMeta(tx, "last_reconcile_at", nowStamp()); err != nil {
		return Stats{}, err
	}
	if err := tx.Commit(); err != nil {
		return Stats{}, fmt.Errorf("index: commit reconcile: %w", err)
	}
	return st, nil
}

type fileMeta struct {
	hash  string
	mtime int64
	ulids []string
}

// reconcileTx is the mark-and-sweep core shared by Rebuild (empty start) and
// Reconcile. It is order-independent: a file only ever touches the rows for the
// node keys it itself produces, and a final sweep drops every key not present.
func (ix *Index) reconcileTx(tx *sql.Tx) (Stats, error) {
	var st Stats
	stored, err := loadFileMeta(tx)
	if err != nil {
		return st, err
	}

	present := map[string]bool{}   // node keys that exist after this pass
	diskPaths := map[string]bool{} // relpaths seen on disk

	walkErr := filepath.WalkDir(ix.cfg.VaultDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != ix.cfg.VaultDir && shouldSkipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !indexable(d.Name()) {
			return nil
		}
		rel, err := filepath.Rel(ix.cfg.VaultDir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		diskPaths[rel] = true
		st.Scanned++

		info, err := d.Info()
		if err != nil {
			return err
		}
		mtime := info.ModTime().UnixNano()

		// Fast-path: mtime unchanged -> trust stored nodes.
		if prev, ok := stored[rel]; ok && prev.mtime == mtime {
			markPresent(present, prev.ulids)
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		h := hashBytes(raw)

		// Content identical though mtime moved (git/Syncthing): refresh mtime only.
		if prev, ok := stored[rel]; ok && prev.hash == h {
			if err := touchMtime(tx, rel, mtime); err != nil {
				return err
			}
			markPresent(present, prev.ulids)
			return nil
		}

		// New or changed: (re)parse and upsert this file's nodes.
		keys, perr := ix.indexFile(tx, rel, raw, h, mtime)
		if perr != nil {
			// Malformed (conflict markers, etc.): log + skip, never crash the
			// reconcile. Drop any stored meta so the file is retried next pass and
			// its old nodes get swept (its keys are not added to present).
			logger.Warn("index: skipping unparsable file", "path", rel, "err", perr)
			if err := deleteFileMeta(tx, rel); err != nil {
				return err
			}
			return nil
		}
		st.Reparsed++
		markPresent(present, keys)
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return st, fmt.Errorf("index: walk vault: %w", walkErr)
	}

	if err := sweepKeys(tx, present); err != nil {
		return st, err
	}
	deleted, err := sweepFileMeta(tx, stored, diskPaths)
	if err != nil {
		return st, err
	}
	st.Deleted = deleted

	if err := resolveEdges(tx); err != nil {
		return st, err
	}
	return st, nil
}

// indexFile parses one file and upserts its nodes (and their edges/props/aliases/
// fts rows), returning the node keys it produced.
func (ix *Index) indexFile(tx *sql.Tx, rel string, raw []byte, hash string, mtime int64) ([]string, error) {
	nodes, err := ix.parser.Parse(raw, node.Loc{File: rel})
	if err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(nodes))
	seen := map[string]int{}
	for _, n := range nodes {
		keys = append(keys, nodeKey(n, rel, seen))
	}

	// Clear any stale owned rows for these exact keys, then insert fresh. We never
	// purge by a file's *old* keys (that would be order-sensitive for moves); the
	// global sweep removes keys that vanished.
	if err := deleteOwned(tx, keys); err != nil {
		return nil, err
	}
	for i, n := range nodes {
		if err := insertNode(tx, keys[i], n, rel, hash, mtime); err != nil {
			return nil, err
		}
	}
	if err := upsertFileMeta(tx, rel, hash, mtime, keys); err != nil {
		return nil, err
	}
	return keys, nil
}

// nodeKey is the node's stable index identity: its inline ULID when present, else
// a path-derived surrogate (rename-stability is a ULID property by design).
func nodeKey(n *node.Node, rel string, seen map[string]int) string {
	if n.ULID != "" {
		return n.ULID
	}
	base := "file:" + rel
	seen[base]++
	if seen[base] == 1 {
		return base
	}
	return fmt.Sprintf("%s#%d", base, seen[base]-1)
}

func insertNode(tx *sql.Tx, key string, n *node.Node, rel, hash string, mtime int64) error {
	if _, err := tx.Exec(
		`INSERT OR REPLACE INTO _nodes(node_key,ulid,type,time,author,body,loc_file,hash,mtime)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		key, n.ULID, n.Type, n.Time, n.Author, n.Body, rel, hash, mtime); err != nil {
		return fmt.Errorf("index: insert node %q: %w", key, err)
	}
	for k, v := range n.Props {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO _props(node_key,key,value) VALUES(?,?,?)`, key, k, v); err != nil {
			return fmt.Errorf("index: insert prop %q/%q: %w", key, k, err)
		}
	}
	for _, a := range n.Aliases {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO _aliases(alias,node_key) VALUES(?,?)`, a, key); err != nil {
			return fmt.Errorf("index: insert alias %q: %w", a, err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO fts_search(id,body) VALUES(?,?)`, key, n.Body); err != nil {
		return fmt.Errorf("index: insert fts %q: %w", key, err)
	}
	for _, l := range n.Links {
		if _, err := tx.Exec(
			`INSERT INTO _edges(src_key,rel,dst,from_frag,to_frag) VALUES(?,?,?,?,?)`,
			key, l.Rel, l.To, l.FromFrag, l.ToFrag); err != nil {
			return fmt.Errorf("index: insert edge %q->%q: %w", key, l.To, err)
		}
	}
	return nil
}

// deleteOwned removes the node-owned rows (node, edges-from, props, aliases, fts)
// for the given keys. Backlink edges (whose dst points at a key) are left for the
// resolver to re-evaluate.
func deleteOwned(tx *sql.Tx, keys []string) error {
	for _, k := range keys {
		for _, q := range []string{
			`DELETE FROM _edges WHERE src_key=?`,
			`DELETE FROM _props WHERE node_key=?`,
			`DELETE FROM _aliases WHERE node_key=?`,
			`DELETE FROM fts_search WHERE id=?`,
			`DELETE FROM _nodes WHERE node_key=?`,
		} {
			if _, err := tx.Exec(q, k); err != nil {
				return fmt.Errorf("index: delete owned rows for %q: %w", k, err)
			}
		}
	}
	return nil
}

// sweepKeys deletes every node-owned row whose key is not in present. Edges whose
// source survived but whose target was swept are kept (resolver nulls dst_key).
func sweepKeys(tx *sql.Tx, present map[string]bool) error {
	if _, err := tx.Exec(`DROP TABLE IF EXISTS temp._present`); err != nil {
		return fmt.Errorf("index: reset present temp: %w", err)
	}
	if _, err := tx.Exec(`CREATE TEMP TABLE _present(k TEXT PRIMARY KEY)`); err != nil {
		return fmt.Errorf("index: create present temp: %w", err)
	}
	stmt, err := tx.Prepare(`INSERT OR IGNORE INTO _present(k) VALUES(?)`)
	if err != nil {
		return fmt.Errorf("index: prepare present insert: %w", err)
	}
	for k := range present {
		if _, err := stmt.Exec(k); err != nil {
			stmt.Close()
			return fmt.Errorf("index: fill present temp: %w", err)
		}
	}
	stmt.Close()

	for _, q := range []string{
		`DELETE FROM _edges   WHERE src_key  NOT IN (SELECT k FROM _present)`,
		`DELETE FROM _props   WHERE node_key NOT IN (SELECT k FROM _present)`,
		`DELETE FROM _aliases WHERE node_key NOT IN (SELECT k FROM _present)`,
		`DELETE FROM fts_search WHERE id   NOT IN (SELECT k FROM _present)`,
		`DELETE FROM _nodes   WHERE node_key NOT IN (SELECT k FROM _present)`,
	} {
		if _, err := tx.Exec(q); err != nil {
			return fmt.Errorf("index: sweep keys: %w", err)
		}
	}
	if _, err := tx.Exec(`DROP TABLE temp._present`); err != nil {
		return fmt.Errorf("index: drop present temp: %w", err)
	}
	return nil
}

// sweepFileMeta deletes _file_meta rows for stored paths no longer on disk and
// returns the number removed.
func sweepFileMeta(tx *sql.Tx, stored map[string]fileMeta, diskPaths map[string]bool) (int, error) {
	deleted := 0
	for path := range stored {
		if diskPaths[path] {
			continue
		}
		if err := deleteFileMeta(tx, path); err != nil {
			return deleted, err
		}
		deleted++
	}
	return deleted, nil
}

// resolveEdges recomputes dst_key for every edge: a target resolves to a node by
// ULID first, then by alias; an unresolvable target stays NULL (dangling).
func resolveEdges(tx *sql.Tx) error {
	// ORDER BY keeps an ambiguous alias (same alias on >1 node) resolving
	// deterministically rather than picking an arbitrary row.
	_, err := tx.Exec(`
		UPDATE _edges SET dst_key = COALESCE(
			(SELECT n.node_key FROM _nodes n WHERE n.ulid = _edges.dst AND n.ulid <> ''),
			(SELECT a.node_key FROM _aliases a WHERE a.alias = _edges.dst ORDER BY a.node_key LIMIT 1)
		)`)
	if err != nil {
		return fmt.Errorf("index: resolve edges: %w", err)
	}
	return nil
}

// --- meta + file_meta helpers ------------------------------------------------

func (ix *Index) initMeta(tx *sql.Tx) error {
	for k, v := range map[string]string{
		"schema_version":  fmt.Sprintf("%d", SchemaVersion),
		"builder_version": BuilderVersion,
		"vault_id":        ix.vaultID,
	} {
		if err := setMeta(tx, k, v); err != nil {
			return err
		}
	}
	return nil
}

func setMeta(tx *sql.Tx, key, value string) error {
	if _, err := tx.Exec(`INSERT OR REPLACE INTO _index_meta(key,value) VALUES(?,?)`, key, value); err != nil {
		return fmt.Errorf("index: set meta %q: %w", key, err)
	}
	return nil
}

func loadFileMeta(tx *sql.Tx) (map[string]fileMeta, error) {
	rows, err := tx.Query(`SELECT path, hash, mtime, ulids FROM _file_meta`)
	if err != nil {
		return nil, fmt.Errorf("index: load file meta: %w", err)
	}
	defer rows.Close()

	out := map[string]fileMeta{}
	for rows.Next() {
		var path, hash, ulidsJSON string
		var mtime int64
		if err := rows.Scan(&path, &hash, &mtime, &ulidsJSON); err != nil {
			return nil, fmt.Errorf("index: scan file meta: %w", err)
		}
		var ulids []string
		if err := json.Unmarshal([]byte(ulidsJSON), &ulids); err != nil {
			return nil, fmt.Errorf("index: decode ulid-set for %q: %w", path, err)
		}
		out[path] = fileMeta{hash: hash, mtime: mtime, ulids: ulids}
	}
	return out, rows.Err()
}

func upsertFileMeta(tx *sql.Tx, path, hash string, mtime int64, keys []string) error {
	data, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("index: encode ulid-set for %q: %w", path, err)
	}
	if _, err := tx.Exec(
		`INSERT OR REPLACE INTO _file_meta(path,hash,mtime,ulids) VALUES(?,?,?,?)`,
		path, hash, mtime, string(data)); err != nil {
		return fmt.Errorf("index: upsert file meta %q: %w", path, err)
	}
	return nil
}

func touchMtime(tx *sql.Tx, path string, mtime int64) error {
	if _, err := tx.Exec(`UPDATE _file_meta SET mtime=? WHERE path=?`, mtime, path); err != nil {
		return fmt.Errorf("index: touch mtime %q: %w", path, err)
	}
	return nil
}

func deleteFileMeta(tx *sql.Tx, path string) error {
	if _, err := tx.Exec(`DELETE FROM _file_meta WHERE path=?`, path); err != nil {
		return fmt.Errorf("index: delete file meta %q: %w", path, err)
	}
	return nil
}

// --- small helpers -----------------------------------------------------------

func markPresent(present map[string]bool, keys []string) {
	for _, k := range keys {
		present[k] = true
	}
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func nowStamp() string { return time.Now().UTC().Format(time.RFC3339Nano) }

// skipDirs are directories never descended into during a walk.
var skipDirs = map[string]bool{
	".git": true, ".obsidian": true, ".reckon": true, ".stversions": true,
}

func shouldSkipDir(name string) bool {
	return skipDirs[name] || strings.HasPrefix(name, ".sync-conflict-")
}

// indexable reports whether a filename should be indexed: a markdown file that is
// not a dotfile and not a Syncthing conflict copy.
func indexable(name string) bool {
	if !strings.HasSuffix(name, ".md") {
		return false
	}
	if strings.HasPrefix(name, ".") {
		return false
	}
	if strings.Contains(name, ".sync-conflict-") {
		return false
	}
	return true
}
