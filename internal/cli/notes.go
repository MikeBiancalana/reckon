package cli

import (
	"fmt"
	"os"
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

func init() {
	notesCmd.AddCommand(notesCreateCmd)
	notesCreateCmd.Flags().StringVar(&notesTagsFlag, "tags", "", "Comma-separated tags")
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

	// Temporarily update note with full file path for link extraction
	// (UpdateNoteLinks needs to read the file)
	// Use defer to guarantee path restoration even if panic occurs
	originalPath := note.FilePath
	note.FilePath = fullFilePath
	defer func() { note.FilePath = originalPath }()

	// Extract and save wiki links
	if err := notesService.UpdateNoteLinks(note); err != nil {
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
