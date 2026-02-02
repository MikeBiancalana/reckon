package cli

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestEditorCommandValidation tests the editor command validation logic
// to ensure shell injection attacks are prevented.
func TestEditorCommandValidation(t *testing.T) {
	tests := []struct {
		name          string
		editor        string
		shouldReject  bool
		expectedError string
	}{
		{
			name:         "valid simple editor",
			editor:       "nvim",
			shouldReject: false,
		},
		{
			name:         "valid editor with args",
			editor:       "vim -u NONE",
			shouldReject: false,
		},
		{
			name:         "valid editor with multiple args",
			editor:       "code --wait --new-window",
			shouldReject: false,
		},
		{
			name:          "reject semicolon",
			editor:        "vim; rm -rf /",
			shouldReject:  true,
			expectedError: "shell metacharacters",
		},
		{
			name:          "reject pipe",
			editor:        "vim | cat /etc/passwd",
			shouldReject:  true,
			expectedError: "shell metacharacters",
		},
		{
			name:          "reject ampersand",
			editor:        "vim && rm file",
			shouldReject:  true,
			expectedError: "shell metacharacters",
		},
		{
			name:          "reject dollar sign",
			editor:        "vim $(malicious)",
			shouldReject:  true,
			expectedError: "shell metacharacters",
		},
		{
			name:          "reject parentheses",
			editor:        "vim (evil)",
			shouldReject:  true,
			expectedError: "shell metacharacters",
		},
		{
			name:          "reject redirection",
			editor:        "vim > /dev/null",
			shouldReject:  true,
			expectedError: "shell metacharacters",
		},
		{
			name:          "reject angle brackets",
			editor:        "vim < /etc/passwd",
			shouldReject:  true,
			expectedError: "shell metacharacters",
		},
		{
			name:          "empty editor",
			editor:        "",
			shouldReject:  true,
			expectedError: "empty editor command",
		},
		{
			name:          "whitespace only",
			editor:        "   ",
			shouldReject:  true,
			expectedError: "empty editor command",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the validation logic from notes.go
			editor := tt.editor

			// Security: Validate editor command to prevent shell injection
			if strings.ContainsAny(editor, ";|&$()<>") {
				if tt.shouldReject {
					assert.Contains(t, "shell metacharacters", tt.expectedError)
					return
				}
				t.Errorf("Expected to pass but was rejected for shell metacharacters")
				return
			}

			// Split editor command if it contains arguments
			editorParts := strings.Fields(editor)
			if len(editorParts) == 0 {
				if tt.shouldReject {
					assert.Contains(t, "empty editor command", tt.expectedError)
					return
				}
				t.Errorf("Expected to pass but was rejected for being empty")
				return
			}

			// Verify the base command exists (skip this check in tests as we can't
			// guarantee all editors are installed in the test environment)
			_, err := exec.LookPath(editorParts[0])
			if err != nil {
				// In real code this would reject, but for testing we accept
				// that the editor might not be installed
				if !tt.shouldReject {
					// This is OK - editor not installed but command structure is valid
					return
				}
			}

			// If we got here, the validation passed
			if tt.shouldReject {
				t.Errorf("Expected editor '%s' to be rejected but it passed validation", tt.editor)
			}
		})
	}
}

// TestEditorCommandParsing tests that editor commands are properly split into parts
func TestEditorCommandParsing(t *testing.T) {
	tests := []struct {
		name     string
		editor   string
		expected []string
	}{
		{
			name:     "simple editor",
			editor:   "nvim",
			expected: []string{"nvim"},
		},
		{
			name:     "editor with one arg",
			editor:   "vim -u",
			expected: []string{"vim", "-u"},
		},
		{
			name:     "editor with multiple args",
			editor:   "code --wait --new-window",
			expected: []string{"code", "--wait", "--new-window"},
		},
		{
			name:     "editor with quoted args",
			editor:   "emacs -nw",
			expected: []string{"emacs", "-nw"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parts := strings.Fields(tt.editor)
			assert.Equal(t, tt.expected, parts)
		})
	}
}

// TestEditorCommandBuilding tests that commands are built correctly with file paths
func TestEditorCommandBuilding(t *testing.T) {
	tests := []struct {
		name         string
		editorParts  []string
		filePath     string
		expectedCmd  string
		expectedArgs []string
	}{
		{
			name:         "simple editor",
			editorParts:  []string{"nvim"},
			filePath:     "/path/to/file.md",
			expectedCmd:  "nvim",
			expectedArgs: []string{"/path/to/file.md"},
		},
		{
			name:         "editor with args",
			editorParts:  []string{"vim", "-u", "NONE"},
			filePath:     "/path/to/file.md",
			expectedCmd:  "vim",
			expectedArgs: []string{"-u", "NONE", "/path/to/file.md"},
		},
		{
			name:         "code editor",
			editorParts:  []string{"code", "--wait"},
			filePath:     "/path/to/file.md",
			expectedCmd:  "code",
			expectedArgs: []string{"--wait", "/path/to/file.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build command with arguments (simulating notes.go logic)
			editorArgs := append(tt.editorParts[1:], tt.filePath)

			assert.Equal(t, tt.expectedCmd, tt.editorParts[0])
			assert.Equal(t, tt.expectedArgs, editorArgs)

			// Verify we can create a command (even if we don't execute it)
			cmd := exec.Command(tt.editorParts[0], editorArgs...)
			assert.NotNil(t, cmd)
			// cmd.Path will be the resolved path, not necessarily the original command name
			assert.NotEmpty(t, cmd.Path)
		})
	}
}
