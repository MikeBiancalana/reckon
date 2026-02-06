package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/logger"
	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/MikeBiancalana/reckon/internal/parser"
	"github.com/MikeBiancalana/reckon/internal/tui/components"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var (
	notesTagsFlag string
	editorFlag    string
)

// notesCmd is the root command for zettelkasten notes
var notesCmd = &cobra.Command{
	Use:   "notes",
	Short: "Manage zettelkasten notes",
	Long:  `Create and manage zettelkasten notes with wiki-style linking.`,
}

// notesCreateCmd creates a new zettelkasten note
var notesCreateCmd = &cobra.Command{
	Use:   "create [title]",
	Short: "Create a new zettelkasten note",
	Long: `Create a new zettelkasten note in the notes directory.

With no arguments, launches an interactive form.
With a title argument, opens your editor for content.

Examples:
  rk notes create                           # Launch interactive form
  rk notes create "My Note"                 # Open editor with title
  rk notes create "My Note" --tags=foo,bar  # Open editor with title and tags`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// If no args, launch mini-TUI form
		if len(args) == 0 {
			return launchNotesCreateForm()
		}

		// CLI mode: parse title and tags
		title := strings.TrimSpace(args[0])
		if title == "" {
			return fmt.Errorf("note title cannot be empty")
		}

		// Parse tags from flag
		var tags []string
		if tagsStr := strings.TrimSpace(notesTagsFlag); tagsStr != "" {
			for _, tag := range strings.Split(tagsStr, ",") {
				tag = strings.TrimSpace(tag)
				if tag != "" {
					tags = append(tags, tag)
				}
			}
		}

		// Open editor for content
		content, err := openEditorForContent()
		if err != nil {
			return fmt.Errorf("failed to get content: %w", err)
		}

		// Create the note
		return createNoteWithContent(title, tags, content)
	},
}

// notesShowCmd displays a note with its content, metadata, and links
var notesShowCmd = &cobra.Command{
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
		rel, err := filepath.Rel(cleanNotesDir, cleanPath)
		if err != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
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
		fmt.Printf("\nLinks (%d):\n", len(outgoingLinks))
		for _, link := range outgoingLinks {
			displayLink(link)
		}
	} else {
		fmt.Println("\nLinks: none")
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

// notesEditCmd edits an existing zettelkasten note
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
	notesCmd.AddCommand(notesCreateCmd)
	notesCmd.AddCommand(notesShowCmd)
	notesCmd.AddCommand(notesEditCmd)
	notesCreateCmd.Flags().StringVar(&notesTagsFlag, "tags", "", "Comma-separated tags")
	notesEditCmd.Flags().StringVar(&editorFlag, "editor", "", "Editor to use (overrides $EDITOR)")
}

// GetNotesCommand returns the notes command
func GetNotesCommand() *cobra.Command {
	return notesCmd
}

// notesCreateFormModel is the Bubble Tea model for the note creation form
type notesCreateFormModel struct {
	form     *components.Form
	editor   *components.TextEditor
	state    notesCreateFormState
	result   *components.FormResult
	content  string
	quit     bool
	canceled bool
}

type notesCreateFormState int

const (
	notesCreateFormStateForm notesCreateFormState = iota
	notesCreateFormStateEditor
	notesCreateFormStateDone
)

func (m notesCreateFormModel) Init() tea.Cmd {
	return m.form.Show()
}

func (m notesCreateFormModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if m.editor != nil {
			m.editor.SetSize(msg.Width, msg.Height)
		}
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			m.canceled = true
			m.quit = true
			return m, tea.Quit
		}

	case components.FormSubmitMsg:
		// Form submitted, move to editor
		m.result = &msg.Result
		m.state = notesCreateFormStateEditor
		return m, m.editor.Show()

	case components.FormCancelMsg:
		// Form cancelled
		m.canceled = true
		m.quit = true
		return m, tea.Quit

	case components.TextEditorSubmitMsg:
		// Editor submitted
		m.content = msg.Text
		m.quit = true
		return m, tea.Quit

	case components.TextEditorCancelMsg:
		// User cancelled text editor
		m.canceled = true
		m.quit = true
		return m, tea.Quit
	}

	// Update the current component
	var cmd tea.Cmd
	if m.state == notesCreateFormStateForm {
		m.form, cmd = m.form.Update(msg)
	} else if m.state == notesCreateFormStateEditor {
		m.editor, cmd = m.editor.Update(msg)
	}

	return m, cmd
}

func (m notesCreateFormModel) View() string {
	if m.quit {
		return ""
	}

	if m.state == notesCreateFormStateForm {
		return m.form.View()
	} else if m.state == notesCreateFormStateEditor {
		return m.editor.View()
	}

	return ""
}

// launchNotesCreateForm launches the interactive form for creating a new note
func launchNotesCreateForm() error {
	// Create form
	form := components.NewForm("Create Zettelkasten Note")

	form.AddField(components.FormField{
		Label:       "Title",
		Key:         "title",
		Type:        components.FieldTypeText,
		Required:    true,
		Placeholder: "Enter note title",
	}).AddField(components.FormField{
		Label:       "Tags",
		Key:         "tags",
		Type:        components.FieldTypeText,
		Required:    false,
		Placeholder: "tag1, tag2, tag3",
	})

	// Create editor for content
	editor := components.NewTextEditor("Enter Note Content")
	editor.SetSize(80, 15)

	// Create Bubble Tea program with form
	initialModel := notesCreateFormModel{
		form:   form,
		editor: editor,
		state:  notesCreateFormStateForm,
	}

	p := tea.NewProgram(initialModel)

	// Run the program
	finalModel, err := p.Run()
	if err != nil {
		return fmt.Errorf("failed to run form: %w", err)
	}

	m := finalModel.(notesCreateFormModel)
	if m.canceled || m.result == nil {
		// Form was cancelled
		return nil
	}

	// Process form result
	return createNoteFromForm(m.result, m.content)
}

// createNoteFromForm creates a note from the form result
func createNoteFromForm(result *components.FormResult, content string) error {
	title := strings.TrimSpace(result.Values["title"])
	if title == "" {
		return fmt.Errorf("note title is required")
	}

	// Parse tags from comma-separated string
	var tags []string
	if tagsStr := strings.TrimSpace(result.Values["tags"]); tagsStr != "" {
		for _, tag := range strings.Split(tagsStr, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				tags = append(tags, tag)
			}
		}
	}

	// Create the note
	return createNoteWithContent(title, tags, content)
}

// createNoteWithContent creates a zettelkasten note with the given metadata and content
func createNoteWithContent(title string, tags []string, content string) error {
	// Generate slug from title
	slug := parser.NormalizeSlug(title)
	if slug == "" {
		return fmt.Errorf("title '%s' produces invalid slug (contains only special characters)", title)
	}

	// Check if note with same slug already exists
	existingNote, err := notesService.GetNoteBySlug(slug)
	if err != nil {
		return fmt.Errorf("failed to check for existing note: %w", err)
	}
	if existingNote != nil {
		return fmt.Errorf("note with slug '%s' already exists", slug)
	}

	// Generate filepath: yyyy/yyyy-mm/yyyy-mm-dd-slug.md
	now := time.Now()
	year := now.Format("2006")
	yearMonth := now.Format("2006-01")
	datePrefix := now.Format("2006-01-02")
	filename := fmt.Sprintf("%s-%s.md", datePrefix, slug)
	filePath := filepath.Join(year, yearMonth, filename)

	// Create note model
	note := models.NewNote(title, slug, filePath, tags)

	// Get notes directory
	notesDir, err := config.NotesDir()
	if err != nil {
		return fmt.Errorf("failed to get notes directory: %w", err)
	}

	// Create year/month directory structure
	fullDirPath := filepath.Join(notesDir, year, yearMonth)
	if err := os.MkdirAll(fullDirPath, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create full file path
	fullFilePath := filepath.Join(notesDir, filePath)

	// Security: Validate that the resolved path stays within notes directory
	// This prevents path traversal attacks via malicious input
	fullFilePath = filepath.Clean(fullFilePath)
	notesDir = filepath.Clean(notesDir)
	if !strings.HasPrefix(fullFilePath, notesDir+string(filepath.Separator)) {
		return fmt.Errorf("invalid file path: attempted path traversal")
	}

	// Build markdown content with frontmatter
	var sb strings.Builder
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("title: %s\n", title))
	if len(tags) > 0 {
		sb.WriteString(fmt.Sprintf("tags: %s\n", strings.Join(tags, ", ")))
	}
	sb.WriteString(fmt.Sprintf("created: %s\n", note.CreatedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("updated: %s\n", note.UpdatedAt.Format(time.RFC3339)))
	sb.WriteString("---\n\n")
	sb.WriteString(content)
	sb.WriteString("\n")

	// Write file
	if err := os.WriteFile(fullFilePath, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("failed to write note file: %w", err)
	}

	// Save note to database (with relative path)
	if err := notesService.SaveNote(note); err != nil {
		// Attempt cleanup - log warning if cleanup fails
		if removeErr := os.Remove(fullFilePath); removeErr != nil {
			logger.Warn("failed to clean up note file after database error",
				"file", fullFilePath, "error", removeErr)
		}
		return fmt.Errorf("failed to save note to database: %w", err)
	}

	// Extract and save wiki links (note.FilePath is relative, notesDir is absolute)
	if err := notesService.UpdateNoteLinks(note, notesDir); err != nil {
		return fmt.Errorf("failed to update note links: %w", err)
	}

	// Resolve orphaned backlinks (links that previously pointed to this slug)
	if err := notesService.ResolveOrphanedBacklinks(note); err != nil {
		return fmt.Errorf("failed to resolve orphaned backlinks: %w", err)
	}

	// Print success message
	if !quietFlag {
		fmt.Printf("✓ Created note: %s (%s)\n", slug, filename)
		if len(tags) > 0 {
			fmt.Printf("  Tags: %s\n", strings.Join(tags, ", "))
		}
	}

	return nil
}

// openEditorForContent opens the user's editor to get note content
// This is a placeholder - editor integration is not yet implemented
func openEditorForContent() (string, error) {
	return "", fmt.Errorf("editor integration not yet implemented; use 'rk notes create' without arguments for interactive mode")
}
