package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/MikeBiancalana/reckon/internal/models"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	noteTagsFlag   []string
	noteSortFlag   string
	noteFormatFlag string
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

		j, err := journalService.GetByDate(effectiveDate)
		if err != nil {
			return fmt.Errorf("failed to get journal for %s: %w", effectiveDate, err)
		}

		if err := journalService.AppendLog(j, noteText); err != nil {
			return fmt.Errorf("failed to add note: %w", err)
		}

		if !quietFlag {
			fmt.Printf("✓ Added note: %s\n", noteText)
		}
		return nil
	},
}

var noteListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all zettelkasten notes",
	Long: `List all zettelkasten notes with title, slug, created date, and tags.

Supports filtering by tags, sorting options, and multiple output formats.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if notesService == nil {
			return fmt.Errorf("notes service not initialized")
		}

		// Get all notes from the database
		notes, err := notesService.GetAllNotes()
		if err != nil {
			return fmt.Errorf("failed to get notes: %w", err)
		}

		// Filter by tags if specified (AND logic - note must have all tags)
		if len(noteTagsFlag) > 0 {
			filtered := make([]*models.Note, 0, len(notes))
			for _, note := range notes {
				hasAllTags := true
				for _, filterTag := range noteTagsFlag {
					foundTag := false
					for _, noteTag := range note.Tags {
						if strings.EqualFold(noteTag, filterTag) {
							foundTag = true
							break
						}
					}
					if !foundTag {
						hasAllTags = false
						break
					}
				}
				if hasAllTags {
					filtered = append(filtered, note)
				}
			}
			notes = filtered
		}

		// Sort notes according to --sort flag
		switch noteSortFlag {
		case "created":
			sort.Slice(notes, func(i, j int) bool {
				return notes[i].CreatedAt.After(notes[j].CreatedAt)
			})
		case "updated":
			sort.Slice(notes, func(i, j int) bool {
				return notes[i].UpdatedAt.After(notes[j].UpdatedAt)
			})
		case "title":
			sort.Slice(notes, func(i, j int) bool {
				return notes[i].Title < notes[j].Title
			})
		case "slug":
			sort.Slice(notes, func(i, j int) bool {
				return notes[i].Slug < notes[j].Slug
			})
		default:
			if noteSortFlag != "" {
				return fmt.Errorf("invalid sort option: %s (supported: created, updated, title, slug)", noteSortFlag)
			}
		}

		// Handle output formats
		if noteFormatFlag != "" {
			format, err := parseFormat(noteFormatFlag)
			if err != nil {
				return err
			}
			switch format {
			case FormatJSON:
				return formatNotesJSON(notes)
			case FormatCSV:
				return formatNotesCSV(notes)
			default:
				return fmt.Errorf("unsupported format: %s (supported: json, csv)", noteFormatFlag)
			}
		}

		// Handle empty list
		if len(notes) == 0 {
			if !quietFlag {
				fmt.Println("No notes found")
			}
			return nil
		}

		// Default table output
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil {
			width = 80 // fallback
		}

		// Calculate available width for title column
		// Format: TITLE | SLUG | CREATED | TAGS
		// padding accounts for tabwriter spacing (3 spaces between columns * 3 gaps = 9)
		// plus some buffer for column borders and terminal margins
		const slugWidth = 30
		const createdWidth = 12
		const tagsWidth = 20
		const padding = 15

		availableTitleWidth := width - (slugWidth + createdWidth + tagsWidth + padding)
		if availableTitleWidth < 20 {
			availableTitleWidth = 20
		}

		tw := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', tabwriter.TabIndent)
		fmt.Fprintln(tw, "TITLE\tSLUG\tCREATED\tTAGS")
		for _, n := range notes {
			title := n.Title
			if len(title) > availableTitleWidth {
				title = title[:availableTitleWidth-3] + "..."
			}

			slug := n.Slug
			if len(slug) > slugWidth {
				slug = slug[:slugWidth-3] + "..."
			}

			tags := "-"
			if len(n.Tags) > 0 {
				tags = strings.Join(n.Tags, ", ")
				if len(tags) > tagsWidth {
					tags = tags[:tagsWidth-3] + "..."
				}
			}

			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
				title,
				slug,
				n.CreatedAt.Format("2006-01-02"),
				tags,
			)
		}
		tw.Flush()

		return nil
	},
}

func init() {
	noteCmd.AddCommand(noteNewCmd)
	noteCmd.AddCommand(noteListCmd)

	// Add flags for list command
	noteListCmd.Flags().StringSliceVar(&noteTagsFlag, "tags", []string{}, "Filter by tags (comma-separated, note must have all tags)")
	noteListCmd.Flags().StringVar(&noteSortFlag, "sort", "created", "Sort by: created, updated, title, slug")
	noteListCmd.Flags().StringVar(&noteFormatFlag, "format", "", "Output format: table (default), json, csv")
}

func GetNoteCommand() *cobra.Command {
	return noteCmd
}
