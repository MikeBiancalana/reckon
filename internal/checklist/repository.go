package checklist

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/MikeBiancalana/reckon/internal/storage"
)

// Repository handles database operations for checklists.
type Repository struct {
	db *storage.Database
}

// NewRepository creates a new checklist repository.
func NewRepository(db *storage.Database) *Repository {
	return &Repository{db: db}
}

// SaveTemplate inserts or replaces a checklist template and its items in a transaction.
func (r *Repository) SaveTemplate(t *Template) error {
	tx, err := r.db.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO checklist_templates (id, name, created_at, updated_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET name=excluded.name, updated_at=excluded.updated_at`,
		t.ID, t.Name, t.CreatedAt.Unix(), t.UpdatedAt.Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to save template: %w", err)
	}

	if err := r.replaceTemplateItems(tx, t.ID, t.Items); err != nil {
		return err
	}

	return tx.Commit()
}

// replaceTemplateItems deletes and re-inserts all items for a template within a transaction.
func (r *Repository) replaceTemplateItems(tx *sql.Tx, templateID string, items []TemplateItem) error {
	if _, err := tx.Exec("DELETE FROM checklist_template_items WHERE template_id = ?", templateID); err != nil {
		return fmt.Errorf("failed to delete template items: %w", err)
	}
	for _, item := range items {
		_, err := tx.Exec(
			`INSERT INTO checklist_template_items (id, template_id, text, position) VALUES (?, ?, ?, ?)`,
			item.ID, item.TemplateID, item.Text, item.Position,
		)
		if err != nil {
			return fmt.Errorf("failed to insert template item: %w", err)
		}
	}
	return nil
}

// GetTemplateByID retrieves a template by ID, including its items.
func (r *Repository) GetTemplateByID(id string) (*Template, error) {
	var t Template
	var createdUnix, updatedUnix int64
	err := r.db.DB().QueryRow(
		`SELECT id, name, created_at, updated_at FROM checklist_templates WHERE id = ?`, id,
	).Scan(&t.ID, &t.Name, &createdUnix, &updatedUnix)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("checklist template %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	t.CreatedAt = time.Unix(createdUnix, 0)
	t.UpdatedAt = time.Unix(updatedUnix, 0)

	items, err := r.GetTemplateItems(t.ID)
	if err != nil {
		return nil, err
	}
	t.Items = items
	return &t, nil
}

// GetTemplateByName retrieves a template by name, including its items.
func (r *Repository) GetTemplateByName(name string) (*Template, error) {
	var t Template
	var createdUnix, updatedUnix int64
	err := r.db.DB().QueryRow(
		`SELECT id, name, created_at, updated_at FROM checklist_templates WHERE name = ?`, name,
	).Scan(&t.ID, &t.Name, &createdUnix, &updatedUnix)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("checklist template %q not found", name)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get template: %w", err)
	}
	t.CreatedAt = time.Unix(createdUnix, 0)
	t.UpdatedAt = time.Unix(updatedUnix, 0)

	items, err := r.GetTemplateItems(t.ID)
	if err != nil {
		return nil, err
	}
	t.Items = items
	return &t, nil
}

// GetTemplateItems returns all items for a template ordered by position.
func (r *Repository) GetTemplateItems(templateID string) ([]TemplateItem, error) {
	rows, err := r.db.DB().Query(
		`SELECT id, template_id, text, position FROM checklist_template_items
		 WHERE template_id = ? ORDER BY position ASC`, templateID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query template items: %w", err)
	}
	defer rows.Close()

	var items []TemplateItem
	for rows.Next() {
		var item TemplateItem
		if err := rows.Scan(&item.ID, &item.TemplateID, &item.Text, &item.Position); err != nil {
			return nil, fmt.Errorf("failed to scan template item: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate template items: %w", err)
	}
	return items, nil
}

// ListTemplates returns all templates ordered by name.
func (r *Repository) ListTemplates() ([]*Template, error) {
	rows, err := r.db.DB().Query(
		`SELECT id, name, created_at, updated_at FROM checklist_templates ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to list templates: %w", err)
	}
	defer rows.Close()

	var templates []*Template
	for rows.Next() {
		var t Template
		var createdUnix, updatedUnix int64
		if err := rows.Scan(&t.ID, &t.Name, &createdUnix, &updatedUnix); err != nil {
			return nil, fmt.Errorf("failed to scan template: %w", err)
		}
		t.CreatedAt = time.Unix(createdUnix, 0)
		t.UpdatedAt = time.Unix(updatedUnix, 0)
		templates = append(templates, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate templates: %w", err)
	}
	return templates, nil
}

// DeleteTemplate deletes a template by ID. Cascade handles items and runs.
func (r *Repository) DeleteTemplate(id string) error {
	result, err := r.db.DB().Exec(`DELETE FROM checklist_templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete template: %w", err)
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("checklist template %q not found", id)
	}
	return nil
}

// SaveRun inserts a new run and its items in a transaction.
func (r *Repository) SaveRun(run *Run) error {
	tx, err := r.db.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	var completedAt sql.NullInt64
	if run.CompletedAt != nil {
		completedAt = sql.NullInt64{Int64: run.CompletedAt.Unix(), Valid: true}
	}

	_, err = tx.Exec(
		`INSERT INTO checklist_runs (id, template_id, template_name, status, started_at, completed_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		run.ID, run.TemplateID, run.TemplateName, string(run.Status),
		run.StartedAt.Unix(), completedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save run: %w", err)
	}

	for _, item := range run.Items {
		var checkedAt sql.NullInt64
		if item.CheckedAt != nil {
			checkedAt = sql.NullInt64{Int64: item.CheckedAt.Unix(), Valid: true}
		}
		checked := 0
		if item.Checked {
			checked = 1
		}
		_, err = tx.Exec(
			`INSERT INTO checklist_run_items (id, run_id, template_item_id, text, position, checked, checked_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			item.ID, item.RunID, item.TemplateItemID, item.Text, item.Position, checked, checkedAt,
		)
		if err != nil {
			return fmt.Errorf("failed to save run item: %w", err)
		}
	}

	return tx.Commit()
}

// GetRunByID retrieves a run with its items.
func (r *Repository) GetRunByID(id string) (*Run, error) {
	var run Run
	var startedUnix int64
	var completedUnix sql.NullInt64
	err := r.db.DB().QueryRow(
		`SELECT id, template_id, template_name, status, started_at, completed_at
		 FROM checklist_runs WHERE id = ?`, id,
	).Scan(&run.ID, &run.TemplateID, &run.TemplateName, (*string)(&run.Status),
		&startedUnix, &completedUnix)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("checklist run %q not found", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get run: %w", err)
	}
	run.StartedAt = time.Unix(startedUnix, 0)
	if completedUnix.Valid {
		t := time.Unix(completedUnix.Int64, 0)
		run.CompletedAt = &t
	}

	items, err := r.GetRunItems(run.ID)
	if err != nil {
		return nil, err
	}
	run.Items = items
	return &run, nil
}

// GetActiveRunByTemplate returns the active run for a given template, or nil if none.
func (r *Repository) GetActiveRunByTemplate(templateID string) (*Run, error) {
	var id string
	err := r.db.DB().QueryRow(
		`SELECT id FROM checklist_runs WHERE template_id = ? AND status = 'active' LIMIT 1`,
		templateID,
	).Scan(&id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query active run: %w", err)
	}
	return r.GetRunByID(id)
}

// ListRuns returns all runs. If onlyActive is true, only returns active runs.
func (r *Repository) ListRuns(onlyActive bool) ([]*Run, error) {
	query := `SELECT id FROM checklist_runs ORDER BY started_at DESC`
	if onlyActive {
		query = `SELECT id FROM checklist_runs WHERE status = 'active' ORDER BY started_at DESC`
	}

	rows, err := r.db.DB().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to list runs: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("failed to scan run id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate runs: %w", err)
	}

	runs := make([]*Run, 0, len(ids))
	for _, id := range ids {
		run, err := r.GetRunByID(id)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, nil
}

// UpdateRunStatus updates the status and optionally completed_at for a run.
func (r *Repository) UpdateRunStatus(id string, status RunStatus, completedAt *time.Time) error {
	var completedUnix sql.NullInt64
	if completedAt != nil {
		completedUnix = sql.NullInt64{Int64: completedAt.Unix(), Valid: true}
	}
	_, err := r.db.DB().Exec(
		`UPDATE checklist_runs SET status = ?, completed_at = ? WHERE id = ?`,
		string(status), completedUnix, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update run status: %w", err)
	}
	return nil
}

// GetRunItems returns all items for a run ordered by position.
func (r *Repository) GetRunItems(runID string) ([]RunItem, error) {
	rows, err := r.db.DB().Query(
		`SELECT id, run_id, template_item_id, text, position, checked, checked_at
		 FROM checklist_run_items WHERE run_id = ? ORDER BY position ASC`, runID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query run items: %w", err)
	}
	defer rows.Close()

	var items []RunItem
	for rows.Next() {
		var item RunItem
		var checkedInt int
		var checkedAtUnix sql.NullInt64
		if err := rows.Scan(&item.ID, &item.RunID, &item.TemplateItemID,
			&item.Text, &item.Position, &checkedInt, &checkedAtUnix); err != nil {
			return nil, fmt.Errorf("failed to scan run item: %w", err)
		}
		item.Checked = checkedInt == 1
		if checkedAtUnix.Valid {
			t := time.Unix(checkedAtUnix.Int64, 0)
			item.CheckedAt = &t
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate run items: %w", err)
	}
	return items, nil
}

// UpdateRunItem updates the checked state and checked_at for a run item.
func (r *Repository) UpdateRunItem(itemID string, checked bool, checkedAt *time.Time) error {
	checkedInt := 0
	if checked {
		checkedInt = 1
	}
	var checkedAtUnix sql.NullInt64
	if checkedAt != nil {
		checkedAtUnix = sql.NullInt64{Int64: checkedAt.Unix(), Valid: true}
	}
	_, err := r.db.DB().Exec(
		`UPDATE checklist_run_items SET checked = ?, checked_at = ? WHERE id = ?`,
		checkedInt, checkedAtUnix, itemID,
	)
	if err != nil {
		return fmt.Errorf("failed to update run item: %w", err)
	}
	return nil
}
