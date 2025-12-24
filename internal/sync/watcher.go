package sync

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/fsnotify/fsnotify"
)

// FileChangeEvent represents a file change notification
type FileChangeEvent struct {
	FilePath string
	Date     string // Journal date (YYYY-MM-DD)
}

// Watcher watches the journal directory for changes
type Watcher struct {
	watcher       *fsnotify.Watcher
	service       *journal.Service
	changes       chan FileChangeEvent
	done          chan struct{}
	debounceTimer *time.Timer
	pendingEvents map[string]bool // Track pending file changes
}

// NewWatcher creates a new file watcher
func NewWatcher(service *journal.Service) (*Watcher, error) {
	fsWatcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create watcher: %w", err)
	}

	return &Watcher{
		watcher:       fsWatcher,
		service:       service,
		changes:       make(chan FileChangeEvent, 10),
		done:          make(chan struct{}),
		pendingEvents: make(map[string]bool),
	}, nil
}

// Start begins watching the journal directory
func (w *Watcher) Start() error {
	journalDir, err := config.JournalDir()
	if err != nil {
		return fmt.Errorf("failed to get journal directory: %w", err)
	}

	if err := w.watcher.Add(journalDir); err != nil {
		return fmt.Errorf("failed to watch directory: %w", err)
	}

	go w.watch()
	return nil
}

// Stop stops the watcher
func (w *Watcher) Stop() {
	close(w.done)
	if w.debounceTimer != nil {
		w.debounceTimer.Stop()
	}
	w.watcher.Close()
	close(w.changes)
}

// Changes returns the channel for file change notifications
func (w *Watcher) Changes() <-chan FileChangeEvent {
	return w.changes
}

// watch is the main event loop
func (w *Watcher) watch() {
	const debounceDelay = 100 * time.Millisecond

	for {
		select {
		case <-w.done:
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			// Only process Write and Create events for .md files
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				if filepath.Ext(event.Name) == ".md" {
					// Extract date from filename (YYYY-MM-DD.md)
					filename := filepath.Base(event.Name)
					if len(filename) > 3 {
						date := filename[:len(filename)-3]
						w.pendingEvents[date] = true

						// Reset debounce timer
						if w.debounceTimer != nil {
							w.debounceTimer.Stop()
						}

						w.debounceTimer = time.AfterFunc(debounceDelay, func() {
							w.processPendingEvents()
						})
					}
				}
			}

		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			// Log error but continue watching
			fmt.Printf("watcher error: %v\n", err)
		}
	}
}

// processPendingEvents handles all pending file changes after debounce
func (w *Watcher) processPendingEvents() {
	for date := range w.pendingEvents {
		// Re-index the changed file
		if err := w.reindexJournal(date); err != nil {
			fmt.Printf("failed to reindex journal %s: %v\n", date, err)
			continue
		}

		// Notify listeners
		journalDir, _ := config.JournalDir()
		filePath := filepath.Join(journalDir, date+".md")
		w.changes <- FileChangeEvent{
			FilePath: filePath,
			Date:     date,
		}
	}

	// Clear pending events
	w.pendingEvents = make(map[string]bool)
}

// reindexJournal re-indexes a single journal file
func (w *Watcher) reindexJournal(date string) error {
	// Use the service's GetByDate method which will parse and save to DB
	_, err := w.service.GetByDate(date)
	if err != nil {
		return fmt.Errorf("failed to reindex journal: %w", err)
	}
	return nil
}
