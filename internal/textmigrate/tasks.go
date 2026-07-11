package textmigrate

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/node"
)

// convertTask builds the todo node.Node for one legacy Task record.
// foldedNotes reports how many Task.Notes entries were folded into the
// rendered body's trailing notes section. An error is returned for a record
// the importer cannot faithfully convert (e.g. an unparseable
// scheduled/deadline date); the caller reports it as a per-record error and
// continues with the remaining records.
func convertTask(t journal.Task) (n *node.Node, foldedNotes int, err error) {
	return nil, 0, fmt.Errorf("textmigrate: convertTask not implemented")
}
