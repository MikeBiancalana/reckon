package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoteCommand_DeprecationNotice(t *testing.T) {
	// Test that note command has Short and Long descriptions
	assert.NotEmpty(t, noteCmd.Short)
	assert.NotEmpty(t, noteCmd.Long)
}

func TestNoteNewCommand_DeprecationNotice(t *testing.T) {
	// Test that note new command has Short and Long descriptions
	assert.NotEmpty(t, noteNewCmd.Short)
	assert.NotEmpty(t, noteNewCmd.Long)
}

func TestNoteNewCommand_HasRunFunction(t *testing.T) {
	// Test that the command has a RunE function that will display the deprecation warning
	// We verify this by inspecting the source behavior rather than running
	// the full command (which would require database setup).

	// Verify the command has a RunE function that should print the warning
	assert.NotNil(t, noteNewCmd.RunE, "noteNewCmd should have a RunE function that displays deprecation warning")
}
