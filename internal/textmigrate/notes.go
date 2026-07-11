package textmigrate

import (
	"fmt"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/node"
)

// convertNote builds the note node.Node for one legacy DB note row plus its
// already-read legacy body content. existingSlugs is consulted for
// filename-collision disambiguation; the returned slug is the new
// (possibly disambiguated) filename slug. The note's old slug is always
// retained as an alias regardless of disambiguation.
func convertNote(dbNote *models.Note, body string, existingSlugs map[string]bool) (n *node.Node, newSlug string, err error) {
	return nil, "", fmt.Errorf("textmigrate: convertNote not implemented")
}
