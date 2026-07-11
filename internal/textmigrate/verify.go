package textmigrate

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/index"
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
	return nil, fmt.Errorf("textmigrate: Importer.Verify not implemented")
}
