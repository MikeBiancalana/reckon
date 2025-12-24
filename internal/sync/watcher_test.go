package sync

import (
	"testing"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

func TestNewWatcher(t *testing.T) {
	// Create a temporary database for testing
	testDir := t.TempDir()
	dbPath := testDir + "/test.db"

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Create service
	fileStore := storage.NewFileStore()
	repo := journal.NewRepository(db)
	service := journal.NewService(repo, fileStore)

	// Create watcher
	watcher, err := NewWatcher(service)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	if watcher == nil {
		t.Fatal("watcher should not be nil")
	}

	if watcher.watcher == nil {
		t.Fatal("underlying fsnotify watcher should not be nil")
	}

	if watcher.service != service {
		t.Fatal("watcher service should match provided service")
	}

	if watcher.changes == nil {
		t.Fatal("changes channel should not be nil")
	}

	// Clean up
	watcher.Stop()
}

func TestWatcherStartStop(t *testing.T) {
	// Create a temporary database for testing
	testDir := t.TempDir()
	dbPath := testDir + "/test.db"

	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	defer db.Close()

	// Create service
	fileStore := storage.NewFileStore()
	repo := journal.NewRepository(db)
	service := journal.NewService(repo, fileStore)

	// Create watcher
	watcher, err := NewWatcher(service)
	if err != nil {
		t.Fatalf("failed to create watcher: %v", err)
	}

	// Start watcher
	if err := watcher.Start(); err != nil {
		t.Fatalf("failed to start watcher: %v", err)
	}

	// Stop watcher (should not panic)
	watcher.Stop()
}
