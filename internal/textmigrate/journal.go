package textmigrate

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/node"
)

// convertJournalDay builds the log-day node.Node for one legacy Journal
// (already parsed from a legacy day file). preamble reports how many
// Intentions/Wins/Schedule entries were preserved in the day's preamble
// block (they have no direct log-entry analog). rewrittenRefs counts inline
// [task:<xid>] markers rewritten to [[<xid>]] wikilinks within Log entry
// content, so the index derives a reference edge to the migrated todo.
func convertJournalDay(j *journal.Journal) (n *node.Node, preamble JournalPreambleCounts, rewrittenRefs int, err error) {
	return nil, JournalPreambleCounts{}, 0, fmt.Errorf("textmigrate: convertJournalDay not implemented")
}
