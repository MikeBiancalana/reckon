package checklist

import (
	"fmt"
	"time"
)

// Service handles business logic for checklists.
type Service struct {
	repo *Repository
}

// NewService creates a new checklist service.
func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

// CreateTemplate creates a new checklist template with the given name and items.
func (s *Service) CreateTemplate(name string, items []string) (*Template, error) {
	if name == "" {
		return nil, fmt.Errorf("template name cannot be empty")
	}

	// Check for duplicate name
	existing, err := s.repo.GetTemplateByName(name)
	if err == nil && existing != nil {
		return nil, fmt.Errorf("checklist template %q already exists", name)
	}

	tpl := NewTemplate(name)
	for i, text := range items {
		item := NewTemplateItem(tpl.ID, text, i)
		tpl.Items = append(tpl.Items, *item)
	}

	if err := s.repo.SaveTemplate(tpl); err != nil {
		return nil, fmt.Errorf("failed to create template: %w", err)
	}
	return tpl, nil
}

// GetTemplate retrieves a template by name or ID.
func (s *Service) GetTemplate(nameOrID string) (*Template, error) {
	// Try by name first
	tpl, err := s.repo.GetTemplateByName(nameOrID)
	if err == nil {
		return tpl, nil
	}
	// Fall back to ID
	tpl, err = s.repo.GetTemplateByID(nameOrID)
	if err != nil {
		return nil, fmt.Errorf("checklist template %q not found", nameOrID)
	}
	return tpl, nil
}

// ListTemplates returns all templates.
func (s *Service) ListTemplates() ([]*Template, error) {
	return s.repo.ListTemplates()
}

// DeleteTemplate deletes a template (and all associated runs) by name or ID.
func (s *Service) DeleteTemplate(nameOrID string) error {
	tpl, err := s.GetTemplate(nameOrID)
	if err != nil {
		return err
	}
	return s.repo.DeleteTemplate(tpl.ID)
}

// AddTemplateItem appends a new item to a template.
func (s *Service) AddTemplateItem(nameOrID, text string) error {
	tpl, err := s.GetTemplate(nameOrID)
	if err != nil {
		return err
	}

	newItem := NewTemplateItem(tpl.ID, text, len(tpl.Items))
	tpl.Items = append(tpl.Items, *newItem)
	tpl.UpdatedAt = time.Now()

	return s.repo.SaveTemplate(tpl)
}

// RemoveTemplateItem removes an item at a 0-based position and recompacts positions.
func (s *Service) RemoveTemplateItem(nameOrID string, position int) error {
	tpl, err := s.GetTemplate(nameOrID)
	if err != nil {
		return err
	}

	if position < 0 || position >= len(tpl.Items) {
		return fmt.Errorf("position %d out of range (template has %d items)", position, len(tpl.Items))
	}

	// Remove item and recompact positions
	tpl.Items = append(tpl.Items[:position], tpl.Items[position+1:]...)
	for i := range tpl.Items {
		tpl.Items[i].Position = i
	}
	tpl.UpdatedAt = time.Now()

	return s.repo.SaveTemplate(tpl)
}

// StartRun starts a new run of the given template. Returns an error if an active run already exists.
func (s *Service) StartRun(nameOrID string) (*Run, error) {
	tpl, err := s.GetTemplate(nameOrID)
	if err != nil {
		return nil, err
	}

	existing, err := s.repo.GetActiveRunByTemplate(tpl.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to check for active run: %w", err)
	}
	if existing != nil {
		return nil, fmt.Errorf("an active run already exists for %q (use 'reset' to start fresh)", tpl.Name)
	}

	run := NewRun(tpl)
	if err := s.repo.SaveRun(run); err != nil {
		return nil, fmt.Errorf("failed to start run: %w", err)
	}
	return run, nil
}

// GetActiveRun returns the active run for a template, or an error if none exists.
func (s *Service) GetActiveRun(nameOrID string) (*Run, error) {
	tpl, err := s.GetTemplate(nameOrID)
	if err != nil {
		return nil, err
	}

	run, err := s.repo.GetActiveRunByTemplate(tpl.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get active run: %w", err)
	}
	if run == nil {
		return nil, fmt.Errorf("no active run for %q (use 'start' to begin)", tpl.Name)
	}
	return run, nil
}

// CheckItem toggles the checked state of an item at 0-based position in a run.
// If all items become checked, the run is auto-completed.
func (s *Service) CheckItem(runID string, position int) error {
	run, err := s.repo.GetRunByID(runID)
	if err != nil {
		return err
	}

	if position < 0 || position >= len(run.Items) {
		return fmt.Errorf("position %d out of range (run has %d items)", position, len(run.Items))
	}

	item := run.Items[position]
	newChecked := !item.Checked

	var checkedAt *time.Time
	if newChecked {
		now := time.Now()
		checkedAt = &now
	}

	if err := s.repo.UpdateRunItem(item.ID, newChecked, checkedAt); err != nil {
		return err
	}

	// Re-fetch to check completion
	updated, err := s.repo.GetRunByID(runID)
	if err != nil {
		return err
	}

	if updated.Status == RunStatusActive && allChecked(updated.Items) {
		now := time.Now()
		if err := s.repo.UpdateRunStatus(runID, RunStatusCompleted, &now); err != nil {
			return fmt.Errorf("failed to complete run: %w", err)
		}
	}

	return nil
}

// GetRunStatus returns a run with its items loaded.
func (s *Service) GetRunStatus(runID string) (*Run, error) {
	return s.repo.GetRunByID(runID)
}

// ResetRun abandons any active run for a template and starts a new one.
func (s *Service) ResetRun(nameOrID string) (*Run, error) {
	tpl, err := s.GetTemplate(nameOrID)
	if err != nil {
		return nil, err
	}

	existing, err := s.repo.GetActiveRunByTemplate(tpl.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to check for active run: %w", err)
	}
	if existing != nil {
		if err := s.repo.UpdateRunStatus(existing.ID, RunStatusAbandoned, nil); err != nil {
			return nil, fmt.Errorf("failed to abandon run: %w", err)
		}
	}

	run := NewRun(tpl)
	if err := s.repo.SaveRun(run); err != nil {
		return nil, fmt.Errorf("failed to start new run: %w", err)
	}
	return run, nil
}

// ListRuns returns runs. If includeCompleted is false, only active runs are returned.
func (s *Service) ListRuns(includeCompleted bool) ([]*Run, error) {
	return s.repo.ListRuns(!includeCompleted)
}

// allChecked returns true if every item in the slice is checked.
func allChecked(items []RunItem) bool {
	if len(items) == 0 {
		return false
	}
	for _, item := range items {
		if !item.Checked {
			return false
		}
	}
	return true
}
