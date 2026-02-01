package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/parser"
	notessvc "github.com/MikeBiancalana/reckon/internal/service"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

func TestCreateNoteWithContent(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	os.Setenv("RECKON_DATA_DIR", tmpDir)
	defer os.Unsetenv("RECKON_DATA_DIR")

	// Initialize database and service
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	repo := notessvc.NewNotesRepository(db)
	notesService = notessvc.NewNotesService(repo)

	// Test creating a note
	title := "Test Note"
	tags := []string{"test", "demo"}
	content := "This is test content with [[another-note]] link."

	err = createNoteWithContent(title, tags, content)
	if err != nil {
		t.Fatalf("Failed to create note: %v", err)
	}

	// Verify note was created
	slug := parser.NormalizeSlug(title)
	note, err := notesService.GetNoteBySlug(slug)
	if err != nil {
		t.Fatalf("Failed to get note: %v", err)
	}
	if note == nil {
		t.Fatal("Note was not created")
	}

	// Verify note properties
	if note.Title != title {
		t.Errorf("Expected title '%s', got '%s'", title, note.Title)
	}
	if note.Slug != slug {
		t.Errorf("Expected slug '%s', got '%s'", slug, note.Slug)
	}
	if len(note.Tags) != len(tags) {
		t.Errorf("Expected %d tags, got %d", len(tags), len(note.Tags))
	}

	// Verify file was created
	notesDir, _ := config.NotesDir()
	fullPath := filepath.Join(notesDir, note.FilePath)
	if _, err := os.Stat(fullPath); os.IsNotExist(err) {
		t.Errorf("Note file was not created at %s", fullPath)
	}

	// Read file and verify content
	fileContent, err := os.ReadFile(fullPath)
	if err != nil {
		t.Fatalf("Failed to read note file: %v", err)
	}
	fileStr := string(fileContent)
	if !contains(fileStr, "title: Test Note") {
		t.Error("File missing title in frontmatter")
	}
	if !contains(fileStr, "tags: test, demo") {
		t.Error("File missing tags in frontmatter")
	}
	if !contains(fileStr, content) {
		t.Error("File missing content")
	}

	// Verify links were extracted
	links, err := notesService.GetLinksBySourceNote(note.ID)
	if err != nil {
		t.Fatalf("Failed to get links: %v", err)
	}
	if len(links) != 1 {
		t.Errorf("Expected 1 link, got %d", len(links))
	}
	if len(links) > 0 && links[0].TargetSlug != "another-note" {
		t.Errorf("Expected link to 'another-note', got '%s'", links[0].TargetSlug)
	}
}

func TestDuplicateSlugDetection(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	os.Setenv("RECKON_DATA_DIR", tmpDir)
	defer os.Unsetenv("RECKON_DATA_DIR")

	// Initialize database and service
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	repo := notessvc.NewNotesRepository(db)
	notesService = notessvc.NewNotesService(repo)

	// Create first note
	err = createNoteWithContent("Test Note", []string{}, "First note")
	if err != nil {
		t.Fatalf("Failed to create first note: %v", err)
	}

	// Try to create duplicate
	err = createNoteWithContent("Test Note", []string{}, "Duplicate note")
	if err == nil {
		t.Error("Expected error for duplicate slug, got nil")
	}
	if err != nil && !contains(err.Error(), "already exists") {
		t.Errorf("Expected 'already exists' error, got: %v", err)
	}
}

func TestSlugGeneration(t *testing.T) {
	tests := []struct {
		title    string
		expected string
	}{
		{"Simple Title", "simple-title"},
		{"Title With Numbers 123", "title-with-numbers-123"},
		{"Title-With-Hyphens", "title-with-hyphens"},
		{"Title_With_Underscores", "title_with_underscores"},
		{"Title   With   Spaces", "title-with-spaces"},
		{"UPPERCASE TITLE", "uppercase-title"},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			slug := parser.NormalizeSlug(tt.title)
			if slug != tt.expected {
				t.Errorf("For title '%s', expected slug '%s', got '%s'", tt.title, tt.expected, slug)
			}
		})
	}
}

func TestInvalidSlugValidation(t *testing.T) {
	// Setup test environment
	tmpDir := t.TempDir()
	os.Setenv("RECKON_DATA_DIR", tmpDir)
	defer os.Unsetenv("RECKON_DATA_DIR")

	// Initialize database and service
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	repo := notessvc.NewNotesRepository(db)
	notesService = notessvc.NewNotesService(repo)

	// Test titles that produce empty slugs (only special characters)
	testCases := []struct {
		title string
		desc  string
	}{
		{"!!!", "only exclamation marks"},
		{"@@@", "only at signs"},
		{"...", "only dots"},
		{"---", "only hyphens that get normalized away"},
		{"   ", "only spaces"},
	}

	for _, tc := range testCases {
		t.Run(tc.desc, func(t *testing.T) {
			err := createNoteWithContent(tc.title, []string{}, "test content")
			if err == nil {
				t.Errorf("Expected error for title '%s', got nil", tc.title)
			}
			if err != nil && !contains(err.Error(), "invalid slug") {
				t.Errorf("Expected 'invalid slug' error for title '%s', got: %v", tc.title, err)
			}
		})
	}
}

func TestEditorIntegrationError(t *testing.T) {
	// Test that openEditorForContent returns a clear error
	_, err := openEditorForContent()
	if err == nil {
		t.Error("Expected error from openEditorForContent, got nil")
	}
	if err != nil && !contains(err.Error(), "not yet implemented") {
		t.Errorf("Expected 'not yet implemented' error, got: %v", err)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
