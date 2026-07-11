package textmigrate

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MikeBiancalana/reckon/internal/checklist"
	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

// convertChecklistTemplate builds the checklist-template node.Node for one
// legacy Template record. A template with zero items renders with no item
// lines rather than erroring.
func convertChecklistTemplate(t *checklist.Template) (n *node.Node, err error) {
	var body string
	for _, item := range t.Items {
		body += "- " + item.Text + "\n"
	}

	n = node.NewNode("checklist-template", legacyAuthor, body)
	n.ULID = node.MintAt(t.CreatedAt)
	n.Time = t.CreatedAt.UTC().Format(time.RFC3339)
	n.Aliases = []string{t.ID}
	n.Props = map[string]string{"name": t.Name}

	return n, nil
}

// convertChecklistRun builds the checklist-run node.Node for one legacy Run
// record. templateOldXID is the run's template's legacy id, used as the
// target of the run's instance-of link so it resolves via the migrated
// template's alias.
func convertChecklistRun(r *checklist.Run, templateOldXID string) (n *node.Node, err error) {
	var body string
	for _, item := range r.Items {
		mark := " "
		if item.Checked {
			mark = "x"
		}
		body += fmt.Sprintf("- [%s] %s\n", mark, item.Text)
	}

	n = node.NewNode("checklist-run", legacyAuthor, body)
	n.ULID = node.MintAt(r.StartedAt)
	n.Time = r.StartedAt.UTC().Format(time.RFC3339)
	n.Aliases = []string{r.ID}

	props := map[string]string{
		"status":  string(r.Status),
		"started": r.StartedAt.UTC().Format(time.RFC3339),
	}
	if r.CompletedAt != nil {
		props["completed"] = r.CompletedAt.UTC().Format(time.RFC3339)
	}
	n.Props = props
	n.Links = []node.Link{{Rel: "instance-of", To: templateOldXID}}

	return n, nil
}

// runChecklists migrates every legacy checklist template and run 1:1 into
// checklists/templates/<ULID>.md and checklists/runs/<ULID>.md.
func (imp *Importer) runChecklists(report *Report) error {
	db, err := storage.NewDatabase(filepath.Join(imp.Source, "reckon.db"))
	if err != nil {
		return fmt.Errorf("open legacy db: %w", err)
	}
	defer db.Close()

	repo := checklist.NewRepository(db)

	templatesDir := filepath.Join(imp.Dest, "checklists", "templates")
	runsDir := filepath.Join(imp.Dest, "checklists", "runs")

	migratedTemplates, err := scanAliases(templatesDir)
	if err != nil {
		return fmt.Errorf("scan existing checklist templates: %w", err)
	}
	migratedRuns, err := scanAliases(runsDir)
	if err != nil {
		return fmt.Errorf("scan existing checklist runs: %w", err)
	}

	templates, err := repo.ListTemplates()
	if err != nil {
		return fmt.Errorf("read legacy checklist templates: %w", err)
	}
	for _, tpl := range templates {
		if migratedTemplates[tpl.ID] {
			report.ChecklistTemplates.addSkipped(tpl.ID, "already migrated (alias present)")
			continue
		}

		// ListTemplates does not populate Items (unlike GetTemplateByID/Name);
		// fetch them explicitly so the template's item lines aren't dropped.
		items, err := repo.GetTemplateItems(tpl.ID)
		if err != nil {
			report.ChecklistTemplates.addErrored(tpl.ID, fmt.Errorf("read template items: %w", err))
			continue
		}
		tpl.Items = items

		built, err := convertChecklistTemplate(tpl)
		if err != nil {
			report.ChecklistTemplates.addErrored(tpl.ID, err)
			continue
		}
		parsed, err := renderAndParse(built)
		if err != nil {
			report.ChecklistTemplates.addErrored(tpl.ID, fmt.Errorf("checklist template %s: %w", tpl.ID, err))
			continue
		}

		relPath := "checklists/templates/" + parsed.ULID + ".md"
		if imp.DryRun {
			report.ChecklistTemplates.addCreated(tpl.ID, relPath, parsed.ULID)
			continue
		}
		if err := os.MkdirAll(templatesDir, 0o755); err != nil {
			report.ChecklistTemplates.addErrored(tpl.ID, fmt.Errorf("create checklist templates dir: %w", err))
			continue
		}
		if err := writeFileAtomic(filepath.Join(templatesDir, parsed.ULID+".md"), parsed.Serialize()); err != nil {
			report.ChecklistTemplates.addErrored(tpl.ID, fmt.Errorf("write %s: %w", relPath, err))
			continue
		}
		report.ChecklistTemplates.addCreated(tpl.ID, relPath, parsed.ULID)
		migratedTemplates[tpl.ID] = true
	}

	runs, err := repo.ListRuns(false)
	if err != nil {
		return fmt.Errorf("read legacy checklist runs: %w", err)
	}
	for _, run := range runs {
		if migratedRuns[run.ID] {
			report.ChecklistRuns.addSkipped(run.ID, "already migrated (alias present)")
			continue
		}

		built, err := convertChecklistRun(run, run.TemplateID)
		if err != nil {
			report.ChecklistRuns.addErrored(run.ID, err)
			continue
		}
		parsed, err := renderAndParse(built)
		if err != nil {
			report.ChecklistRuns.addErrored(run.ID, fmt.Errorf("checklist run %s: %w", run.ID, err))
			continue
		}

		relPath := "checklists/runs/" + parsed.ULID + ".md"
		if imp.DryRun {
			report.ChecklistRuns.addCreated(run.ID, relPath, parsed.ULID)
			continue
		}
		if err := os.MkdirAll(runsDir, 0o755); err != nil {
			report.ChecklistRuns.addErrored(run.ID, fmt.Errorf("create checklist runs dir: %w", err))
			continue
		}
		if err := writeFileAtomic(filepath.Join(runsDir, parsed.ULID+".md"), parsed.Serialize()); err != nil {
			report.ChecklistRuns.addErrored(run.ID, fmt.Errorf("write %s: %w", relPath, err))
			continue
		}
		report.ChecklistRuns.addCreated(run.ID, relPath, parsed.ULID)
		migratedRuns[run.ID] = true
	}

	return nil
}
