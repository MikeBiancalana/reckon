package textmigrate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/node"
)

// convertTask builds the todo node.Node for one legacy Task record.
// foldedNotes reports how many Task.Notes entries were folded into the
// rendered body's trailing notes section. An error is returned for a record
// the importer cannot faithfully convert (e.g. an unparseable
// scheduled/deadline date); the caller reports it as a per-record error and
// continues with the remaining records.
func convertTask(t journal.Task) (n *node.Node, foldedNotes int, err error) {
	props := map[string]string{"state": "open"}
	if t.Status == journal.TaskDone {
		props["state"] = "done"
	}

	if t.ScheduledDate != nil {
		if _, perr := time.Parse("2006-01-02", *t.ScheduledDate); perr != nil {
			return nil, 0, fmt.Errorf("task %s: malformed scheduled_date %q: %w", t.ID, *t.ScheduledDate, perr)
		}
		props["scheduled"] = *t.ScheduledDate
	}
	if t.DeadlineDate != nil {
		if _, perr := time.Parse("2006-01-02", *t.DeadlineDate); perr != nil {
			return nil, 0, fmt.Errorf("task %s: malformed deadline_date %q: %w", t.ID, *t.DeadlineDate, perr)
		}
		props["deadline"] = *t.DeadlineDate
	}
	if len(t.Tags) > 0 {
		props["tags"] = "[" + strings.Join(t.Tags, ", ") + "]"
	}

	body := taskBody(t)

	n = node.NewNode("todo", legacyAuthor, body)
	n.ULID = node.MintAt(t.CreatedAt)
	n.Time = t.CreatedAt.UTC().Format(time.RFC3339)
	n.Aliases = []string{t.ID}
	n.Props = props

	return n, len(t.Notes), nil
}

// taskBody composes a todo's body from the task's text and description, plus
// a trailing "### Notes" section folding any legacy task notes -- the new
// todo schema has no separate notes field, so folding into the body (with
// the count reported by the caller) keeps them visible instead of a silent
// drop.
func taskBody(t journal.Task) string {
	var b strings.Builder
	b.WriteString(t.Text)
	if strings.TrimSpace(t.Description) != "" {
		b.WriteString("\n\n")
		b.WriteString(t.Description)
	}
	if len(t.Notes) > 0 {
		b.WriteString("\n\n### Notes\n")
		for _, note := range t.Notes {
			b.WriteString("- " + note.Text + "\n")
		}
	}
	b.WriteString("\n")
	return b.String()
}

// runTasks migrates every legacy gen-1 task (file-primary, read via the
// same journal.TaskService.GetAllTasks reader the live `rk task` command
// uses) into todos/<ULID>.md.
func (imp *Importer) runTasks(report *Report) error {
	restore := overrideDataDir(imp.Source)
	defer restore()

	tasks, err := journal.NewTaskService(nil, nil).GetAllTasks()
	if err != nil {
		return fmt.Errorf("read legacy tasks: %w", err)
	}

	todosDir := filepath.Join(imp.Dest, "todos")
	migrated, err := scanAliases(todosDir)
	if err != nil {
		return fmt.Errorf("scan existing todos: %w", err)
	}

	for _, tk := range tasks {
		if migrated[tk.ID] {
			report.Tasks.addSkipped(tk.ID, "already migrated (alias present)")
			continue
		}

		built, folded, err := convertTask(tk)
		if err != nil {
			report.Tasks.addErrored(tk.ID, err)
			continue
		}

		parsed, err := renderAndParse(built)
		if err != nil {
			report.Tasks.addErrored(tk.ID, fmt.Errorf("task %s: %w", tk.ID, err))
			continue
		}

		relPath := "todos/" + parsed.ULID + ".md"
		if imp.DryRun {
			report.Tasks.addCreated(tk.ID, relPath, parsed.ULID)
			continue
		}

		if err := os.MkdirAll(todosDir, 0o755); err != nil {
			report.Tasks.addErrored(tk.ID, fmt.Errorf("create todos dir: %w", err))
			continue
		}
		if err := writeFileAtomic(filepath.Join(todosDir, parsed.ULID+".md"), parsed.Serialize()); err != nil {
			report.Tasks.addErrored(tk.ID, fmt.Errorf("write %s: %w", relPath, err))
			continue
		}

		report.FoldedTaskNotes += folded
		report.Tasks.addCreated(tk.ID, relPath, parsed.ULID)
		migrated[tk.ID] = true
	}

	return nil
}
