package journal

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type TaskFileFrontmatterForWrite struct {
	ID        string   `yaml:"id"`
	Title     string   `yaml:"title"`
	Created   string   `yaml:"created"`
	Status    string   `yaml:"status"`
	Tags      []string `yaml:"tags,omitempty"`
	Scheduled *string  `yaml:"scheduled,omitempty"`
	Deadline  *string  `yaml:"deadline,omitempty"`
}

// WriteTaskFile serializes a Task to markdown format with YAML frontmatter.
func WriteTaskFile(task Task) (string, error) {
	var sb strings.Builder

	created := task.CreatedAt.Format("2006-01-02")
	status := "open"
	if task.Status == TaskDone {
		status = "done"
	}

	frontmatter := TaskFileFrontmatterForWrite{
		ID:        task.ID,
		Title:     task.Text,
		Created:   created,
		Status:    status,
		Tags:      task.Tags,
		Scheduled: task.ScheduledDate,
		Deadline:  task.DeadlineDate,
	}

	yamlData, err := yaml.Marshal(frontmatter)
	if err != nil {
		return "", fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	sb.WriteString("---\n")
	sb.WriteString(string(yamlData))
	sb.WriteString("---\n\n")

	sb.WriteString("## Description\n\n")
	sb.WriteString(task.Text)
	sb.WriteString("\n\n")

	if len(task.Notes) > 0 {
		sb.WriteString("## Log\n\n")

		sortedNotes := make([]TaskNote, len(task.Notes))
		copy(sortedNotes, task.Notes)
		sort.Slice(sortedNotes, func(i, j int) bool {
			return sortedNotes[i].Position < sortedNotes[j].Position
		})

		for _, note := range sortedNotes {
			sb.WriteString(fmt.Sprintf("- %s %s\n", note.ID, note.Text))
		}
	}

	return sb.String(), nil
}
