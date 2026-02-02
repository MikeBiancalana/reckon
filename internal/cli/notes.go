package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/spf13/cobra"
)

var (
	editorFlag string
)

var notesCmd = &cobra.Command{
	Use:   "notes",
	Short: "Manage zettelkasten notes",
	Long:  `Manage zettelkasten notes - create, list, show, and edit notes.`,
}

var notesEditCmd = &cobra.Command{
	Use:   "edit [slug]",
	Short: "Edit a zettelkasten note",
	Long: `Edit a zettelkasten note in your $EDITOR.

If no slug is provided, a fuzzy picker will be shown to select a note.
After editing, the note's updated timestamp and wiki links will be refreshed.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if notesService == nil {
			return fmt.Errorf("notes service not initialized - try running 'rk init' or check your configuration")
		}

		var slug string

		// If slug provided as argument, use it
		if len(args) > 0 {
			slug = strings.TrimSpace(args[0])
		} else {
			// No slug provided - launch fuzzy picker
			notes, err := notesService.GetAllNotes()
			if err != nil {
				return fmt.Errorf("failed to get notes: %w", err)
			}

			if len(notes) == 0 {
				return fmt.Errorf("no notes found - create a note first using 'rk notes create'")
			}

			selectedSlug, canceled, err := PickNote(notes, "Select a note to edit")
			if err != nil {
				return fmt.Errorf("failed to pick note: %w", err)
			}

			if canceled {
				return nil
			}

			slug = selectedSlug
		}

		if slug == "" {
			return fmt.Errorf("note slug cannot be empty")
		}

		// Get the note by slug
		note, err := notesService.GetNoteBySlug(slug)
		if err != nil {
			return fmt.Errorf("failed to get note: %w", err)
		}
		if note == nil {
			return fmt.Errorf("note with slug '%s' not found", slug)
		}

		// Get the notes directory
		notesDir, err := config.NotesDir()
		if err != nil {
			return fmt.Errorf("failed to get notes directory: %w", err)
		}

		// Construct full path to note file
		filePath := filepath.Join(notesDir, note.FilePath)

		// Security: Verify the path is within notesDir to prevent path traversal
		cleanPath := filepath.Clean(filePath)
		cleanNotesDir := filepath.Clean(notesDir)
		rel, err := filepath.Rel(cleanNotesDir, cleanPath)
		if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			return fmt.Errorf("invalid note path: %s", note.FilePath)
		}

		// Verify file exists before opening editor
		if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
			return fmt.Errorf("note file does not exist: %s", note.FilePath)
		}

		// Get editor from flag, then $EDITOR env var, then default to nvim
		editor := editorFlag
		if editor == "" {
			editor = os.Getenv("EDITOR")
			if editor == "" {
				editor = "nvim"
			}
		}

		// Security: Validate editor command to prevent shell injection
		if strings.ContainsAny(editor, ";|&$()<>") {
			return fmt.Errorf("invalid editor command: contains shell metacharacters")
		}

		// Split editor command if it contains arguments
		editorParts := strings.Fields(editor)
		if len(editorParts) == 0 {
			return fmt.Errorf("empty editor command")
		}

		// Verify the base command exists
		_, err = exec.LookPath(editorParts[0])
		if err != nil {
			return fmt.Errorf("editor not found: %s", editorParts[0])
		}

		// Build command with arguments
		editorArgs := append(editorParts[1:], cleanPath)
		editorCmd := exec.Command(editorParts[0], editorArgs...)
		editorCmd.Stdin = os.Stdin
		editorCmd.Stdout = os.Stdout
		editorCmd.Stderr = os.Stderr

		if err := editorCmd.Run(); err != nil {
			return fmt.Errorf("editor failed: %w", err)
		}

		// Verify file still exists after editing
		if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
			return fmt.Errorf("note file was deleted during editing")
		}

		// Update the note's updated_at timestamp
		if err := notesService.UpdateNoteTimestamp(note); err != nil {
			return fmt.Errorf("failed to update note timestamp: %w", err)
		}

		// Re-parse wiki links
		if err := notesService.UpdateNoteLinks(note, notesDir); err != nil {
			return fmt.Errorf("failed to update note links: %w", err)
		}

		if !quietFlag {
			fmt.Printf("✓ Updated note: %s\n", slug)
		}

		return nil
	},
}

func init() {
	notesEditCmd.Flags().StringVar(&editorFlag, "editor", "", "Editor to use (overrides $EDITOR)")
	notesCmd.AddCommand(notesEditCmd)
}

func GetNotesCommand() *cobra.Command {
	return notesCmd
}
