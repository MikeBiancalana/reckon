// Package index is reckon's derived, disposable, per-device property-graph index.
//
// Markdown in the vault is the source of truth; this SQLite store is a rebuildable
// projection over it (canonical nodes from internal/node) used for fast query and
// graph traversal. It lives in the cache dir, NEVER inside the vault, and is never
// synced — a synced live SQLite file would corrupt.
//
// The query contract is a set of STABLE PUBLIC VIEWS (nodes, edges, node_props,
// aliases, fts); the physical tables behind them are private so storage can be
// refactored without breaking callers. Change detection is hash-authoritative
// (mtime is only a fast-path). Freshness is maintained by lazy reconcile-on-read
// plus an explicit full Rebuild; a schema_version bump auto-rebuilds on Open.
package index

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/node"

	_ "modernc.org/sqlite"
)

// Index is an open handle to a vault's property-graph index.
type Index struct {
	db       *sql.DB
	cfg      *config.Config
	vaultID  string
	dir      string // cache subdir holding index.db + index.lock
	lockPath string
	parser   node.Parser
}

// vaultID derives a stable per-vault identifier from the absolute vault path, so
// distinct vaults map to distinct cache subdirs.
func vaultID(vaultDir string) (string, error) {
	abs, err := filepath.Abs(vaultDir)
	if err != nil {
		return "", fmt.Errorf("index: resolve vault path: %w", err)
	}
	sum := sha256.Sum256([]byte(abs))
	return hex.EncodeToString(sum[:])[:12], nil
}

// DBPath returns the on-disk path of the index database for cfg, without opening
// it. T3 (rk query) uses this to open a read-only connection.
func DBPath(cfg *config.Config) (string, error) {
	id, err := vaultID(cfg.VaultDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(cfg.CacheDir, id, "index.db"), nil
}

// Open opens (creating if absent) the index for cfg. If the persisted schema
// version differs from SchemaVersion — or the store is new/empty — Open performs
// a full Rebuild from the vault text (the index is derived; there are no
// migrations). The default parser is node.LogParser, which is byte-identical
// to node.MarkdownParser for every file EXCEPT a "type: log-day" group file
// (log/<date>.md), which it splits into a day node plus one log-entry node
// per `## ` block (v1-T4) — so every reader (rk query, rk todo list, …) sees
// log entries with zero per-caller changes, and the DB's contents never
// depend on which command last built it.
func Open(cfg *config.Config) (*Index, error) {
	return OpenWithParser(cfg, node.LogParser{})
}

// OpenWithParser is Open with an explicit per-tool parser.
func OpenWithParser(cfg *config.Config, parser node.Parser) (*Index, error) {
	id, err := vaultID(cfg.VaultDir)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(cfg.CacheDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("index: create cache dir %q: %w", dir, err)
	}

	dbPath := filepath.Join(dir, "index.db")
	db, err := sql.Open("sqlite", dbPath+"?_journal=WAL&_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("index: open db: %w", err)
	}

	ix := &Index{
		db:       db,
		cfg:      cfg,
		vaultID:  id,
		dir:      dir,
		lockPath: filepath.Join(dir, "index.lock"),
		parser:   parser,
	}

	if err := ix.ensureSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return ix, nil
}

// ensureSchema brings the physical schema to SchemaVersion. A missing or stale
// schema triggers a full rebuild from text.
func (ix *Index) ensureSchema() error {
	have, err := ix.tableExists("_index_meta")
	if err != nil {
		return err
	}
	if have {
		v, err := ix.Meta("schema_version")
		if err != nil {
			return err
		}
		if v == fmt.Sprintf("%d", SchemaVersion) {
			return nil // current — nothing to do
		}
	}
	// New or stale: rebuild from scratch.
	_, err = ix.Rebuild()
	return err
}

func (ix *Index) tableExists(name string) (bool, error) {
	var n int
	err := ix.db.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type IN ('table','view') AND name=?", name,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("index: probe table %q: %w", name, err)
	}
	return n > 0, nil
}

// DB exposes the underlying connection. Callers should treat the public views as
// the stable contract and the physical tables as private.
func (ix *Index) DB() *sql.DB { return ix.db }

// Close releases the database handle.
func (ix *Index) Close() error {
	if err := ix.db.Close(); err != nil {
		return fmt.Errorf("index: close: %w", err)
	}
	return nil
}

// Meta reads a value from the global _index_meta key/value table.
func (ix *Index) Meta(key string) (string, error) {
	var v string
	err := ix.db.QueryRow("SELECT value FROM _index_meta WHERE key=?", key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("index: read meta %q: %w", key, err)
	}
	return v, nil
}
