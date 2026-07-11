package textmigrate

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Given a fixture DB with checklist templates and runs, when the importer
// runs then one checklist-template node exists per template and one
// checklist-run node exists per run, each carrying its legacy id as an
// alias.
func TestImporter_Checklists_TemplatesAndRunsOneToOne(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)
	db := openFixtureDB(t, source)

	tpl, run := fixtureChecklist(t, db, "morning", []string{"Check email", "Review calendar"})

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)

	require.Len(t, report.ChecklistTemplates.Created, 1)
	require.Equal(t, tpl.ID, report.ChecklistTemplates.Created[0].SourceID)
	require.Len(t, report.ChecklistRuns.Created, 1)
	require.Equal(t, run.ID, report.ChecklistRuns.Created[0].SourceID)

	tplNode := mustParseVaultFile(t, dest, report.ChecklistTemplates.Created[0].Path)
	require.Equal(t, "checklist-template", tplNode.Type)
	require.Equal(t, "morning", tplNode.Props["name"])
	require.Contains(t, tplNode.Aliases, tpl.ID)
	require.Contains(t, tplNode.Body, "Check email")
	require.Contains(t, tplNode.Body, "Review calendar")

	runNode := mustParseVaultFile(t, dest, report.ChecklistRuns.Created[0].Path)
	require.Equal(t, "checklist-run", runNode.Type)
	require.Contains(t, runNode.Aliases, run.ID)

	var instanceOf string
	for _, l := range runNode.Links {
		if l.Rel == "instance-of" {
			instanceOf = l.To
		}
	}
	require.Equal(t, tpl.ID, instanceOf, "a run's instance-of link targets its template's legacy id, resolving via the template's alias")
}

// Given a checklist template with zero items, when migrated it produces a
// valid checklist-template file with no item lines rather than erroring.
func TestConvertChecklistTemplate_ZeroItems_NoErrorNoItemLines(t *testing.T) {
	source := newFixtureSource(t)
	dest, _ := newFixtureDest(t)
	db := openFixtureDB(t, source)

	tpl, _ := fixtureChecklist(t, db, "empty-checklist", nil)

	imp := &Importer{Source: source, Dest: dest}
	report, err := imp.Run()
	require.NoError(t, err)
	require.Len(t, report.ChecklistTemplates.Created, 1)
	require.Empty(t, report.ChecklistTemplates.Errored)

	tplNode := mustParseVaultFile(t, dest, report.ChecklistTemplates.Created[0].Path)
	require.Equal(t, "checklist-template", tplNode.Type)
	require.Contains(t, tplNode.Aliases, tpl.ID)
	require.NotContains(t, tplNode.Body, "- ", "a zero-item template must render with no item lines")
}
