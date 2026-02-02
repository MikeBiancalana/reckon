package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/spf13/cobra"
)

var noteCmd = &cobra.Command{
	Use:   "note",
	Short: "Manage standalone notes",
	Long:  `Manage standalone notes - create, list, and delete notes.`,
}

var noteNewCmd = &cobra.Command{
	Use:   "new [text]",
	Short: "Create a new standalone note",
	Long: `Creates a new standalone note.
The note will be added to the journal as a log entry.`,
	Args: cobra.ArbitraryArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		var noteText string

		if len(args) == 0 {
			return fmt.Errorf("note text cannot be empty")
		}
		noteText = strings.TrimSpace(strings.Join(args, " "))

		if noteText == "" {
			return fmt.Errorf("note text cannot be empty")
		}

		effectiveDate, err := getEffectiveDate()
		if err != nil {
			return err
		}

		j, err := service.GetByDate(effectiveDate)
		if err != nil {
			return fmt.Errorf("failed to get journal for %s: %w", effectiveDate, err)
		}

		if err := service.AppendLog(j, noteText); err != nil {
			return fmt.Errorf("failed to add note: %w", err)
		}

		if !quietFlag {
			fmt.Printf("✓ Added note: %s\n", noteText)
		}
		return nil
	},
}

var noteShowCmd = &cobra.Command{
	Use:   "show [slug]",
	Short: "Show a note with its content, metadata, and links",
	Long:  `Display a note's content, metadata (title, tags, dates), outgoing links, and backlinks.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		slug := strings.TrimSpace(args[0])
		if slug == "" {
			return fmt.Errorf("note slug cannot be empty")
		}

		if notesService == nil {
			return fmt.Errorf("notes service not initialized")
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

		// Read the note content from file
		filePath := filepath.Join(notesDir, note.FilePath)

		// Security: Verify the path is within notesDir to prevent path traversal
		cleanPath := filepath.Clean(filePath)
		cleanNotesDir := filepath.Clean(notesDir)
		if !strings.HasPrefix(cleanPath, cleanNotesDir+string(filepath.Separator)) {
			return fmt.Errorf("invalid note path: %s", note.FilePath)
		}

		contentBytes, err := os.ReadFile(cleanPath)
		if err != nil {
			return fmt.Errorf("failed to read note file: %w", err)
		}

		// Strip frontmatter from content
		content := stripFrontmatter(string(contentBytes))

		// Get outgoing links
		outgoingLinks, err := notesService.GetLinksBySourceNote(note.ID)
		if err != nil {
			return fmt.Errorf("failed to get outgoing links: %w", err)
		}

		// Get backlinks
		backlinks, err := notesService.GetBacklinks(note.ID)
		if err != nil {
			return fmt.Errorf("failed to get backlinks: %w", err)
		}

		// Display the note
		displayNote(note, content, outgoingLinks, backlinks)

		return nil
	},
}

// stripFrontmatter removes YAML frontmatter from the beginning of content.
// Frontmatter is delimited by "---" lines at the start of the file.
func stripFrontmatter(content string) string {
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return content
	}

	// Check if the first line is a frontmatter delimiter
	if strings.TrimSpace(lines[0]) != "---" {
		return content
	}

	// Find the closing delimiter
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			// Return everything after the closing delimiter
			return strings.Join(lines[i+1:], "\n")
		}
	}

	// If no closing delimiter found, return the original content
	return content
}

// displayNote outputs the note with its metadata, content, and links.
func displayNote(note *models.Note, content string, outgoingLinks, backlinks []models.NoteLink) {
	// Display metadata
	fmt.Printf("Title: %s\n", note.Title)
	fmt.Printf("Slug: %s\n", note.Slug)
	fmt.Printf("Created: %s\n", note.CreatedAt.Format("2006-01-02 15:04:05"))
	fmt.Printf("Updated: %s\n", note.UpdatedAt.Format("2006-01-02 15:04:05"))

	if len(note.Tags) > 0 {
		fmt.Printf("Tags: %s\n", strings.Join(note.Tags, ", "))
	}

	fmt.Println("\n---")

	// Display content
	fmt.Print(strings.TrimSpace(content))
	fmt.Println("\n\n---")

	// Display outgoing links
	if len(outgoingLinks) > 0 {
		fmt.Printf("Links (%d):\n", len(outgoingLinks))
		for _, link := range outgoingLinks {
			displayLink(link)
		}
	} else {
		fmt.Println("Links: none")
	}

	fmt.Println()

	// Display backlinks
	if len(backlinks) > 0 {
		fmt.Printf("Backlinks (%d):\n", len(backlinks))
		for _, backlink := range backlinks {
			displayBacklink(backlink)
		}
	} else {
		fmt.Println("Backlinks: none")
	}
}

// displayLink shows an outgoing link with its target information.
func displayLink(link models.NoteLink) {
	if link.TargetNoteID != "" {
		// Link is resolved - try to get the target note's title
		targetNote, err := notesService.GetNoteByID(link.TargetNoteID)
		if err == nil && targetNote != nil {
			fmt.Printf("- [[%s]] %s\n", link.TargetSlug, targetNote.Title)
		} else {
			fmt.Printf("- [[%s]]\n", link.TargetSlug)
		}
	} else {
		// Orphaned link
		fmt.Printf("- [[%s]] (not found)\n", link.TargetSlug)
	}
}

// displayBacklink shows a backlink with its source information.
func displayBacklink(backlink models.NoteLink) {
	// Try to get the source note's title
	sourceNote, err := notesService.GetNoteByID(backlink.SourceNoteID)
	if err == nil && sourceNote != nil {
		fmt.Printf("- [[%s]] From: %s\n", sourceNote.Slug, sourceNote.Title)
	} else {
		fmt.Printf("- From: %s\n", backlink.SourceNoteID)
	}
}

func init() {
	noteCmd.AddCommand(noteNewCmd)
	noteCmd.AddCommand(noteShowCmd)
}

func GetNoteCommand() *cobra.Command {
	return noteCmd
}
