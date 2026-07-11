package textmigrate

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/checklist"
	"github.com/MikeBiancalana/reckon/internal/node"
)

// convertChecklistTemplate builds the checklist-template node.Node for one
// legacy Template record. A template with zero items renders with no item
// lines rather than erroring.
func convertChecklistTemplate(t *checklist.Template) (n *node.Node, err error) {
	return nil, fmt.Errorf("textmigrate: convertChecklistTemplate not implemented")
}

// convertChecklistRun builds the checklist-run node.Node for one legacy Run
// record. templateOldXID is the run's template's legacy id, used as the
// target of the run's instance-of link so it resolves via the migrated
// template's alias.
func convertChecklistRun(r *checklist.Run, templateOldXID string) (n *node.Node, err error) {
	return nil, fmt.Errorf("textmigrate: convertChecklistRun not implemented")
}
