package task

import (
	"fmt"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Frontmatter represents the YAML frontmatter of a task file
type Frontmatter struct {
	ID      string   `yaml:"id"`
	Title   string   `yaml:"title"`
	Created string   `yaml:"created"`
	Status  string   `yaml:"status"`
	Tags    []string `yaml:"tags"`
}

// ParseTask parses a task markdown file
func ParseTask(content string, filePath string) (*Task, error) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return nil, fmt.Errorf("invalid task file: too few lines")
	}

	// Parse frontmatter
	if lines[0] != "---" {
		return nil, fmt.Errorf("invalid task file: missing frontmatter")
	}

	// Find end of frontmatter
	fmEnd := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			fmEnd = i
			break
		}
	}
	if fmEnd == -1 {
		return nil, fmt.Errorf("invalid task file: frontmatter not closed")
	}

	// Parse YAML frontmatter
	fmContent := strings.Join(lines[1:fmEnd], "\n")
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(fmContent), &fm); err != nil {
		return nil, fmt.Errorf("failed to parse frontmatter: %w", err)
	}

	// Create task
	task := &Task{
		ID:         fm.ID,
		Title:      fm.Title,
		Status:     Status(fm.Status),
		Created:    fm.Created,
		Tags:       fm.Tags,
		LogEntries: make([]LogEntry, 0),
		FilePath:   filePath,
	}

	// Parse body
	bodyStart := fmEnd + 1
	if bodyStart >= len(lines) {
		return task, nil
	}

	// Parse sections
	currentSection := ""
	descriptionLines := make([]string, 0)
	currentDate := ""

	for i := bodyStart; i < len(lines); i++ {
		line := lines[i]

		// Check for section headers
		if strings.HasPrefix(line, "## ") {
			currentSection = strings.TrimPrefix(line, "## ")
			continue
		}

		// Check for date headers in Log section
		if strings.HasPrefix(line, "### ") && currentSection == "Log" {
			currentDate = strings.TrimPrefix(line, "### ")
			continue
		}

		// Process content based on section
		switch currentSection {
		case "Description":
			if line != "" {
				descriptionLines = append(descriptionLines, line)
			}
		case "Log":
			if currentDate != "" && strings.HasPrefix(line, "- ") {
				entry := parseLogEntry(line, currentDate)
				if entry != nil {
					task.LogEntries = append(task.LogEntries, *entry)
				}
			}
		}
	}

	task.Description = strings.Join(descriptionLines, "\n")

	return task, nil
}

// parseLogEntry parses a single log entry line
func parseLogEntry(line string, date string) *LogEntry {
	// Format: - HH:MM Content
	line = strings.TrimPrefix(line, "- ")
	parts := strings.SplitN(line, " ", 2)
	if len(parts) < 2 {
		return nil
	}

	timeStr := parts[0]
	content := parts[1]

	// Parse timestamp
	timestamp, err := time.Parse("2006-01-02 15:04", date+" "+timeStr)
	if err != nil {
		return nil
	}

	return NewLogEntry(timestamp, content)
}

// WriteTask serializes a task to markdown format
func WriteTask(task *Task) string {
	var sb strings.Builder

	// Write frontmatter
	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", task.ID))
	sb.WriteString(fmt.Sprintf("title: %s\n", task.Title))
	sb.WriteString(fmt.Sprintf("created: %s\n", task.Created))
	sb.WriteString(fmt.Sprintf("status: %s\n", task.Status))

	if len(task.Tags) > 0 {
		sb.WriteString("tags: [")
		for i, tag := range task.Tags {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(tag)
		}
		sb.WriteString("]\n")
	}

	sb.WriteString("---\n\n")

	// Write description
	sb.WriteString("## Description\n")
	if task.Description != "" {
		sb.WriteString(task.Description)
		sb.WriteString("\n")
	}
	sb.WriteString("\n")

	// Write log entries grouped by date
	sb.WriteString("## Log\n\n")

	// Group entries by date
	entryMap := make(map[string][]LogEntry)
	dates := make([]string, 0)

	for _, entry := range task.LogEntries {
		if _, exists := entryMap[entry.Date]; !exists {
			dates = append(dates, entry.Date)
			entryMap[entry.Date] = make([]LogEntry, 0)
		}
		entryMap[entry.Date] = append(entryMap[entry.Date], entry)
	}

	// Write entries by date
	for _, date := range dates {
		sb.WriteString(fmt.Sprintf("### %s\n", date))
		for _, entry := range entryMap[date] {
			timeStr := entry.Timestamp.Format("15:04")
			sb.WriteString(fmt.Sprintf("- %s %s\n", timeStr, entry.Content))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}
