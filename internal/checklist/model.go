package checklist

import (
	"time"

	"github.com/rs/xid"
)

// Template is a named, reusable set of checklist items.
type Template struct {
	ID        string         `json:"id"`
	Name      string         `json:"name"`
	Items     []TemplateItem `json:"items"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// NewTemplate creates a new checklist template.
func NewTemplate(name string) *Template {
	now := time.Now()
	return &Template{
		ID:        xid.New().String(),
		Name:      name,
		Items:     []TemplateItem{},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// TemplateItem is a single step within a checklist template.
type TemplateItem struct {
	ID         string `json:"id"`
	TemplateID string `json:"template_id"`
	Text       string `json:"text"`
	Position   int    `json:"position"`
}

// NewTemplateItem creates a new template item.
func NewTemplateItem(templateID, text string, position int) *TemplateItem {
	return &TemplateItem{
		ID:         xid.New().String(),
		TemplateID: templateID,
		Text:       text,
		Position:   position,
	}
}

// RunStatus represents the lifecycle state of a checklist run.
type RunStatus string

const (
	RunStatusActive    RunStatus = "active"
	RunStatusCompleted RunStatus = "completed"
	RunStatusAbandoned RunStatus = "abandoned"
)

// Run is a single execution instance of a template.
type Run struct {
	ID           string     `json:"id"`
	TemplateID   string     `json:"template_id"`
	TemplateName string     `json:"template_name"`
	Status       RunStatus  `json:"status"`
	Items        []RunItem  `json:"items"`
	StartedAt    time.Time  `json:"started_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// NewRun creates a new run from a template, copying its items.
func NewRun(t *Template) *Run {
	now := time.Now()
	items := make([]RunItem, len(t.Items))
	for i, item := range t.Items {
		items[i] = RunItem{
			ID:             xid.New().String(),
			TemplateItemID: item.ID,
			Text:           item.Text,
			Position:       item.Position,
			Checked:        false,
		}
	}
	run := &Run{
		ID:           xid.New().String(),
		TemplateID:   t.ID,
		TemplateName: t.Name,
		Status:       RunStatusActive,
		Items:        items,
		StartedAt:    now,
	}
	// RunItem.RunID set after Run.ID is assigned
	for i := range run.Items {
		run.Items[i].RunID = run.ID
	}
	return run
}

// RunItem is the state of a single item within a run.
type RunItem struct {
	ID             string     `json:"id"`
	RunID          string     `json:"run_id"`
	TemplateItemID string     `json:"template_item_id"`
	Text           string     `json:"text"`
	Position       int        `json:"position"`
	Checked        bool       `json:"checked"`
	CheckedAt      *time.Time `json:"checked_at,omitempty"`
}
