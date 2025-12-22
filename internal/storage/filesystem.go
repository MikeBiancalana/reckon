package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
)

// FileStore handles file system operations for journals
type FileStore struct{}

// NewFileStore creates a new file store
func NewFileStore() *FileStore {
	return &FileStore{}
}

// FileInfo holds file metadata
type FileInfo struct {
	Path         string
	LastModified time.Time
	Exists       bool
}

// ReadJournalFile reads a journal file and returns its content and metadata
func (fs *FileStore) ReadJournalFile(date string) (content string, info FileInfo, err error) {
	filePath, err := fs.GetJournalPath(date)
	if err != nil {
		return "", FileInfo{}, err
	}

	// Check if file exists
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		return "", FileInfo{Path: filePath, Exists: false}, nil
	}
	if err != nil {
		return "", FileInfo{}, fmt.Errorf("failed to stat file: %w", err)
	}

	// Read file content
	contentBytes, err := os.ReadFile(filePath)
	if err != nil {
		return "", FileInfo{}, fmt.Errorf("failed to read file: %w", err)
	}

	return string(contentBytes), FileInfo{
		Path:         filePath,
		LastModified: fileInfo.ModTime(),
		Exists:       true,
	}, nil
}

// WriteJournalFile writes content to a journal file
func (fs *FileStore) WriteJournalFile(date string, content string) error {
	filePath, err := fs.GetJournalPath(date)
	if err != nil {
		return err
	}

	// Write to file
	if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// GetJournalPath returns the file path for a journal date
func (fs *FileStore) GetJournalPath(date string) (string, error) {
	journalDir, err := config.JournalDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(journalDir, date+".md"), nil
}

// ListJournalDates returns all journal dates (sorted)
func (fs *FileStore) ListJournalDates() ([]string, error) {
	journalDir, err := config.JournalDir()
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(journalDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read journal directory: %w", err)
	}

	dates := make([]string, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) == ".md" {
			// Extract date from filename (YYYY-MM-DD.md)
			date := name[:len(name)-3]
			dates = append(dates, date)
		}
	}

	return dates, nil
}

// JournalExists checks if a journal file exists for the given date
func (fs *FileStore) JournalExists(date string) (bool, error) {
	filePath, err := fs.GetJournalPath(date)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(filePath)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// DeleteJournal deletes a journal file
func (fs *FileStore) DeleteJournal(date string) error {
	filePath, err := fs.GetJournalPath(date)
	if err != nil {
		return err
	}

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete journal: %w", err)
	}

	return nil
}
