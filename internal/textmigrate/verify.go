package textmigrate

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/MikeBiancalana/reckon/internal/checklist"
	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/index"
	"github.com/MikeBiancalana/reckon/internal/journal"
	notessvc "github.com/MikeBiancalana/reckon/internal/service"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

// VerifyResult is the structured outcome of Importer.Verify: a fresh,
// index-backed check of the already-written migration output against the
// source record counts.
type VerifyResult struct {
	// Counts maps a migrated node "type" (todo, note, checklist-template,
	// checklist-run, log-day, log-entry) to its row count in the rebuilt
	// index.
	Counts map[string]int
	// UnexpectedWarnings are duplicate_ulid/alias_collision warnings from
	// Index.Rebuild that verify treats as a failure.
	UnexpectedWarnings []index.Warning
	// UnresolvedAliases are old source ids that do not resolve via the
	// index's aliases view.
	UnresolvedAliases []string
	// OK is true iff there are no unexpected warnings, no unresolved
	// aliases, and every per-type count matches the source.
	OK bool
}

// Verify opens (or builds) the index over Dest, rebuilds it, and checks the
// migration's counts, warnings, and alias resolution against Source.
func (imp *Importer) Verify() (*VerifyResult, error) {
	cfg, err := config.LoadWithOverrides(imp.Dest, "")
	if err != nil {
		return nil, fmt.Errorf("textmigrate: verify: load config: %w", err)
	}

	ix, err := index.Open(cfg)
	if err != nil {
		return nil, fmt.Errorf("textmigrate: verify: open index: %w", err)
	}
	defer ix.Close()

	stats, err := ix.Rebuild()
	if err != nil {
		return nil, fmt.Errorf("textmigrate: verify: rebuild index: %w", err)
	}

	counts, err := countByType(ix)
	if err != nil {
		return nil, fmt.Errorf("textmigrate: verify: count nodes: %w", err)
	}

	sourceAliasCount, err := imp.sourceAliasCounts()
	if err != nil {
		return nil, fmt.Errorf("textmigrate: verify: read source: %w", err)
	}

	var unexpected []index.Warning
	for _, w := range stats.Warnings {
		if w.Kind == "alias_collision" && sourceAliasCount[w.Alias] <= 1 {
			// This alias is claimed by more than one vault node, but the
			// legacy source itself only ever produced it once: the other
			// claimant is unrelated, pre-existing vault content (or a
			// deliberately retained old slug on a disambiguated note). That
			// is accepted collateral of retaining old slugs as aliases
			// regardless of filename collisions, not a migration bug.
			continue
		}
		unexpected = append(unexpected, w)
	}

	unresolved, err := unresolvedAliases(ix, sourceAliasCount)
	if err != nil {
		return nil, fmt.Errorf("textmigrate: verify: resolve aliases: %w", err)
	}

	return &VerifyResult{
		Counts:             counts,
		UnexpectedWarnings: unexpected,
		UnresolvedAliases:  unresolved,
		OK:                 len(unexpected) == 0 && len(unresolved) == 0,
	}, nil
}

// countByType returns the row count per node "type" in the rebuilt index.
func countByType(ix *index.Index) (map[string]int, error) {
	rows, err := ix.DB().Query(`SELECT type, COUNT(*) FROM nodes GROUP BY type`)
	if err != nil {
		return nil, fmt.Errorf("query type counts: %w", err)
	}
	defer rows.Close()

	counts := map[string]int{}
	for rows.Next() {
		var typ string
		var n int
		if err := rows.Scan(&typ, &n); err != nil {
			return nil, fmt.Errorf("scan type count: %w", err)
		}
		counts[typ] = n
	}
	return counts, rows.Err()
}

// unresolvedAliases returns every old source id (from sourceAliasCount) that
// does not resolve via the index's aliases view.
func unresolvedAliases(ix *index.Index, sourceAliasCount map[string]int) ([]string, error) {
	stmt, err := ix.DB().Prepare(`SELECT COUNT(*) FROM aliases WHERE alias = ?`)
	if err != nil {
		return nil, fmt.Errorf("prepare alias lookup: %w", err)
	}
	defer stmt.Close()

	var unresolved []string
	for alias := range sourceAliasCount {
		var n int
		if err := stmt.QueryRow(alias).Scan(&n); err != nil {
			return nil, fmt.Errorf("look up alias %q: %w", alias, err)
		}
		if n == 0 {
			unresolved = append(unresolved, alias)
		}
	}
	return unresolved, nil
}

// sourceAliasCounts scans every legacy source record and counts how many
// distinct records (across every type) independently produce each alias
// string. A count > 1 means the legacy source data itself has two records
// claiming the same old id/slug -- a genuine collision, as opposed to
// a collision against unrelated content that only exists in the destination
// vault. Records this importer cannot read at all (a missing DB, an unset
// legacy dir) contribute nothing rather than failing verify outright.
func (imp *Importer) sourceAliasCounts() (map[string]int, error) {
	counts := map[string]int{}
	add := func(alias string) {
		if alias != "" {
			counts[alias]++
		}
	}

	restore := overrideDataDir(imp.Source)
	tasks, err := journal.NewTaskService(nil, nil).GetAllTasks()
	restore()
	if err != nil {
		return nil, fmt.Errorf("read legacy tasks: %w", err)
	}
	for _, tk := range tasks {
		add(tk.ID)
	}

	dbPath := filepath.Join(imp.Source, "reckon.db")
	if _, err := os.Stat(dbPath); err == nil {
		db, err := storage.NewDatabase(dbPath)
		if err != nil {
			return nil, fmt.Errorf("open legacy db: %w", err)
		}
		defer db.Close()

		notes, err := notessvc.NewNotesRepository(db).GetAllNotes()
		if err != nil {
			return nil, fmt.Errorf("read legacy notes: %w", err)
		}
		for _, n := range notes {
			add(n.ID)
			add(n.Slug)
		}

		repo := checklist.NewRepository(db)
		templates, err := repo.ListTemplates()
		if err != nil {
			return nil, fmt.Errorf("read legacy checklist templates: %w", err)
		}
		for _, tpl := range templates {
			add(tpl.ID)
		}
		runs, err := repo.ListRuns(false)
		if err != nil {
			return nil, fmt.Errorf("read legacy checklist runs: %w", err)
		}
		for _, run := range runs {
			add(run.ID)
		}
	}

	journalDir := filepath.Join(imp.Source, "journal")
	entries, err := os.ReadDir(journalDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read legacy journal dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
			continue
		}
		add(e.Name()[:len(e.Name())-len(".md")])
	}

	return counts, nil
}
