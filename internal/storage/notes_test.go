package storage

import (
	"database/sql"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNotes_TableStructure(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	rows, err := db.DB().Query("PRAGMA table_info(notes)")
	if err != nil {
		t.Fatalf("failed to query table info: %v", err)
	}
	defer rows.Close()

	columns := make(map[string]string)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue interface{}

		err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk)
		if err != nil {
			t.Fatalf("failed to scan column info: %v", err)
		}
		columns[name] = colType
	}

	expectedColumns := map[string]string{
		"id":         "TEXT",
		"title":      "TEXT",
		"slug":       "TEXT",
		"file_path":  "TEXT",
		"created_at": "INTEGER",
		"updated_at": "INTEGER",
		"tags":       "TEXT",
	}

	for colName, expectedType := range expectedColumns {
		actualType, exists := columns[colName]
		if !exists {
			t.Errorf("expected column %s to exist", colName)
		} else if actualType != expectedType {
			t.Errorf("expected column %s to have type %s, got %s", colName, expectedType, actualType)
		}
	}
}

func TestNoteLinks_TableStructure(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	rows, err := db.DB().Query("PRAGMA table_info(note_links)")
	if err != nil {
		t.Fatalf("failed to query table info: %v", err)
	}
	defer rows.Close()

	columns := make(map[string]string)
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue interface{}

		err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk)
		if err != nil {
			t.Fatalf("failed to scan column info: %v", err)
		}
		columns[name] = colType
	}

	expectedColumns := map[string]string{
		"id":             "TEXT",
		"source_note_id": "TEXT",
		"target_slug":    "TEXT",
		"target_note_id": "TEXT",
		"link_type":      "TEXT",
		"created_at":     "INTEGER",
	}

	for colName, expectedType := range expectedColumns {
		actualType, exists := columns[colName]
		if !exists {
			t.Errorf("expected column %s to exist", colName)
		} else if actualType != expectedType {
			t.Errorf("expected column %s to have type %s, got %s", colName, expectedType, actualType)
		}
	}
}

func TestNotes_SlugUniqueness(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()
	_, err = db.DB().Exec(`
		INSERT INTO notes (id, title, slug, file_path, created_at, updated_at, tags)
		VALUES ('note-1', 'First Note', 'unique-slug', '/path/1', ?, ?, '')
	`, now, now)
	if err != nil {
		t.Fatalf("failed to insert first note: %v", err)
	}

	_, err = db.DB().Exec(`
		INSERT INTO notes (id, title, slug, file_path, created_at, updated_at, tags)
		VALUES ('note-2', 'Second Note', 'unique-slug', '/path/2', ?, ?, '')
	`, now, now)
	if err == nil {
		t.Fatal("expected unique constraint violation for slug, got nil error")
	}
}

func TestNoteLinks_CascadeDelete(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()
	_, err = db.DB().Exec(`
		INSERT INTO notes (id, title, slug, file_path, created_at, updated_at, tags)
		VALUES ('note-1', 'First Note', 'note-1', '/path/1', ?, ?, '')
	`, now, now)
	if err != nil {
		t.Fatalf("failed to create note: %v", err)
	}

	_, err = db.DB().Exec(`
		INSERT INTO note_links (id, source_note_id, target_slug, link_type, created_at)
		VALUES ('link-1', 'note-1', 'target-slug', 'reference', ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	var linkCount int
	err = db.DB().QueryRow("SELECT COUNT(*) FROM note_links WHERE source_note_id = ?", "note-1").Scan(&linkCount)
	if err != nil {
		t.Fatalf("failed to count links: %v", err)
	}
	if linkCount != 1 {
		t.Errorf("expected 1 link, got %d", linkCount)
	}

	_, err = db.DB().Exec("DELETE FROM notes WHERE id = ?", "note-1")
	if err != nil {
		t.Fatalf("failed to delete note: %v", err)
	}

	err = db.DB().QueryRow("SELECT COUNT(*) FROM note_links WHERE source_note_id = ?", "note-1").Scan(&linkCount)
	if err != nil {
		t.Fatalf("failed to count links after cascade delete: %v", err)
	}
	if linkCount != 0 {
		t.Errorf("expected 0 links after cascade delete, got %d", linkCount)
	}
}

func TestNoteLinks_SetNullForTarget(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()
	_, err = db.DB().Exec(`
		INSERT INTO notes (id, title, slug, file_path, created_at, updated_at, tags)
		VALUES ('note-1', 'First', 'note-1', '/p1', ?, ?, ''), ('note-2', 'Second', 'note-2', '/p2', ?, ?, '')
	`, now, now, now, now)
	if err != nil {
		t.Fatalf("failed to create notes: %v", err)
	}

	_, err = db.DB().Exec(`
		INSERT INTO note_links (id, source_note_id, target_slug, target_note_id, link_type, created_at)
		VALUES ('link-1', 'note-1', 'note-2', 'note-2', 'reference', ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to create link: %v", err)
	}

	_, err = db.DB().Exec("DELETE FROM notes WHERE id = ?", "note-2")
	if err != nil {
		t.Fatalf("failed to delete target note: %v", err)
	}

	var targetNoteID sql.NullString
	err = db.DB().QueryRow("SELECT target_note_id FROM note_links WHERE id = ?", "link-1").Scan(&targetNoteID)
	if err != nil {
		t.Fatalf("failed to get link target_note_id: %v", err)
	}
	if targetNoteID.Valid {
		t.Errorf("expected target_note_id to be NULL, got %s", targetNoteID.String)
	}
}

func TestNoteLinks_DuplicatePrevention(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()
	_, err = db.DB().Exec(`
		INSERT INTO notes (id, title, slug, file_path, created_at, updated_at, tags)
		VALUES ('note-1', 'First', 'note-1', '/p1', ?, ?, '')
	`, now, now)
	if err != nil {
		t.Fatalf("failed to create note: %v", err)
	}

	_, err = db.DB().Exec(`
		INSERT INTO note_links (id, source_note_id, target_slug, link_type, created_at)
		VALUES ('link-1', 'note-1', 'target', 'reference', ?)
	`, now)
	if err != nil {
		t.Fatalf("failed to create first link: %v", err)
	}

	_, err = db.DB().Exec(`
		INSERT INTO note_links (id, source_note_id, target_slug, link_type, created_at)
		VALUES ('link-2', 'note-1', 'target', 'reference', ?)
	`, now)
	if err == nil {
		t.Fatal("expected unique constraint violation for duplicate link, got nil error")
	}
}

func TestNotes_Indices(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	expectedIndices := []string{
		"idx_notes_slug",
		"idx_notes_created",
		"idx_notes_updated",
		"idx_note_links_source",
		"idx_note_links_target",
		"idx_note_links_target_slug",
	}

	for _, indexName := range expectedIndices {
		var name string
		err = db.DB().QueryRow(`
			SELECT name FROM sqlite_master
			WHERE type='index' AND name=?
		`, indexName).Scan(&name)
		if err != nil {
			t.Fatalf("index %s does not exist: %v", indexName, err)
		}
		if name != indexName {
			t.Errorf("expected index name %s, got %s", indexName, name)
		}
	}
}

func TestNotes_CRUD(t *testing.T) {
	db, err := NewDatabase(":memory:")
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	now := time.Now().Unix()
	_, err = db.DB().Exec(`
		INSERT INTO notes (id, title, slug, file_path, created_at, updated_at, tags)
		VALUES ('note-1', 'Test Note', 'test-note', '/path/test.md', ?, ?, 'tag1,tag2')
	`, now, now)
	if err != nil {
		t.Fatalf("failed to insert note: %v", err)
	}

	var title, slug, filePath, tags string
	err = db.DB().QueryRow(`
		SELECT title, slug, file_path, tags FROM notes WHERE id = 'note-1'
	`).Scan(&title, &slug, &filePath, &tags)
	if err != nil {
		t.Fatalf("failed to query note: %v", err)
	}

	assert.Equal(t, "Test Note", title)
	assert.Equal(t, "test-note", slug)
	assert.Equal(t, "/path/test.md", filePath)
	assert.Equal(t, "tag1,tag2", tags)

	_, err = db.DB().Exec(`
		UPDATE notes SET title = 'Updated Title', updated_at = ? WHERE id = 'note-1'
	`, now)
	if err != nil {
		t.Fatalf("failed to update note: %v", err)
	}

	err = db.DB().QueryRow("SELECT title FROM notes WHERE id = 'note-1'").Scan(&title)
	if err != nil {
		t.Fatalf("failed to query updated note: %v", err)
	}
	assert.Equal(t, "Updated Title", title)

	_, err = db.DB().Exec("DELETE FROM notes WHERE id = 'note-1'")
	if err != nil {
		t.Fatalf("failed to delete note: %v", err)
	}

	var count int
	err = db.DB().QueryRow("SELECT COUNT(*) FROM notes").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count notes: %v", err)
	}
	assert.Equal(t, 0, count)
}
