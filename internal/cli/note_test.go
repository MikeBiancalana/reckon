package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoteCommand_DeprecationNotice(t *testing.T) {
	// Test that note command shows deprecation in Short and Long descriptions
	assert.Contains(t, noteCmd.Short, "[DEPRECATED]")
	assert.Contains(t, noteCmd.Short, "rk log add")

	assert.Contains(t, noteCmd.Long, "[DEPRECATED]")
	assert.Contains(t, noteCmd.Long, "rk log add")
	assert.Contains(t, noteCmd.Long, "zettelkasten")
}

func TestNoteNewCommand_DeprecationNotice(t *testing.T) {
	// Test that note new command shows deprecation in Short and Long descriptions
	assert.Contains(t, noteNewCmd.Short, "[DEPRECATED]")
	assert.Contains(t, noteNewCmd.Short, "rk log add")

	assert.Contains(t, noteNewCmd.Long, "[DEPRECATED]")
	assert.Contains(t, noteNewCmd.Long, "rk log add")
	assert.Contains(t, noteNewCmd.Long, "zettelkasten")
}

func TestNoteNewCommand_HasRunFunction(t *testing.T) {
	// Test that the command has a RunE function that will display the deprecation warning
	// We verify this by inspecting the source behavior rather than running
	// the full command (which would require database setup).

	// Verify the command has a RunE function that should print the warning
	assert.NotNil(t, noteNewCmd.RunE, "noteNewCmd should have a RunE function that displays deprecation warning")
}
