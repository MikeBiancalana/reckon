package journal

import (
	"testing"
	"time"
)

func TestNewIntention(t *testing.T) {
	text := "Test intention"
	position := 1

	intention := NewIntention(text, position)

	if intention.Text != text {
		t.Errorf("Expected text %s, got %s", text, intention.Text)
	}
	if intention.Status != IntentionOpen {
		t.Errorf("Expected status %s, got %s", IntentionOpen, intention.Status)
	}
	if intention.Position != position {
		t.Errorf("Expected position %d, got %d", position, intention.Position)
	}
	if intention.ID == "" {
		t.Error("Expected ID to be set")
	}
}

func TestNewCarriedIntention(t *testing.T) {
	text := "Carried intention"
	carriedFrom := "2023-12-01"
	position := 2

	intention := NewCarriedIntention(text, carriedFrom, position)

	if intention.Text != text {
		t.Errorf("Expected text %s, got %s", text, intention.Text)
	}
	if intention.Status != IntentionCarried {
		t.Errorf("Expected status %s, got %s", IntentionCarried, intention.Status)
	}
	if intention.CarriedFrom != carriedFrom {
		t.Errorf("Expected carriedFrom %s, got %s", carriedFrom, intention.CarriedFrom)
	}
	if intention.Position != position {
		t.Errorf("Expected position %d, got %d", position, intention.Position)
	}
}

func TestNewLogEntry(t *testing.T) {
	timestamp := time.Now()
	content := "Test log entry"
	entryType := EntryTypeLog
	position := 3

	entry := NewLogEntry(timestamp, content, entryType, position)

	if !entry.Timestamp.Equal(timestamp) {
		t.Errorf("Expected timestamp %v, got %v", timestamp, entry.Timestamp)
	}
	if entry.Content != content {
		t.Errorf("Expected content %s, got %s", content, entry.Content)
	}
	if entry.EntryType != entryType {
		t.Errorf("Expected entryType %s, got %s", entryType, entry.EntryType)
	}
	if entry.Position != position {
		t.Errorf("Expected position %d, got %d", position, entry.Position)
	}
	if entry.ID == "" {
		t.Error("Expected ID to be set")
	}
}

func TestNewWin(t *testing.T) {
	text := "Test win"
	position := 4

	win := NewWin(text, position)

	if win.Text != text {
		t.Errorf("Expected text %s, got %s", text, win.Text)
	}
	if win.Position != position {
		t.Errorf("Expected position %d, got %d", position, win.Position)
	}
	if win.ID == "" {
		t.Error("Expected ID to be set")
	}
}

func TestNewJournal(t *testing.T) {
	date := "2023-12-01"

	journal := NewJournal(date)

	if journal.Date != date {
		t.Errorf("Expected date %s, got %s", date, journal.Date)
	}
	if len(journal.Intentions) != 0 {
		t.Errorf("Expected empty intentions slice, got %d", len(journal.Intentions))
	}
	if len(journal.Wins) != 0 {
		t.Errorf("Expected empty wins slice, got %d", len(journal.Wins))
	}
	if len(journal.LogEntries) != 0 {
		t.Errorf("Expected empty log entries slice, got %d", len(journal.LogEntries))
	}
}
