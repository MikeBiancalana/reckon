package config

import (
	"os"
	"path/filepath"
)

const (
	AppName = "reckon"
	DbName  = "reckon.db"
)

// DataDir returns the path to the reckon data directory (~/.reckon/)
// Creates the directory if it doesn't exist
// Can be overridden with RECKON_DATA_DIR environment variable (primarily for testing)
func DataDir() (string, error) {
	// Check for test override
	if dataDir := os.Getenv("RECKON_DATA_DIR"); dataDir != "" {
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return "", err
		}
		return dataDir, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	dataDir := filepath.Join(home, "."+AppName)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", err
	}

	return dataDir, nil
}

// JournalDir returns the path to the journal directory (~/.reckon/journal/)
// Creates the directory if it doesn't exist
func JournalDir() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}

	journalDir := filepath.Join(dataDir, "journal")
	if err := os.MkdirAll(journalDir, 0755); err != nil {
		return "", err
	}

	return journalDir, nil
}

// TasksDir returns the path to the tasks directory (~/.reckon/tasks/)
// Creates the directory if it doesn't exist
func TasksDir() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}

	tasksDir := filepath.Join(dataDir, "tasks")
	if err := os.MkdirAll(tasksDir, 0755); err != nil {
		return "", err
	}

	return tasksDir, nil
}

// NotesDir returns the path to the notes directory (~/.reckon/notes/)
// Creates the directory if it doesn't exist
func NotesDir() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}

	notesDir := filepath.Join(dataDir, "notes")
	if err := os.MkdirAll(notesDir, 0755); err != nil {
		return "", err
	}

	return notesDir, nil
}

// DatabasePath returns the path to the SQLite database (~/.reckon/reckon.db)
func DatabasePath() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(dataDir, DbName), nil
}

// LogDir returns the path to the log directory (~/.reckon/logs/)
// Creates the directory if it doesn't exist
func LogDir() (string, error) {
	dataDir, err := DataDir()
	if err != nil {
		return "", err
	}

	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return "", err
	}

	return logDir, nil
}
