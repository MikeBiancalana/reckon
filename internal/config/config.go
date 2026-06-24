package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

// Config holds the resolved v1 vault and cache directory paths.
// Resolution is pure — no directories are created during Load/NewConfig/LoadWithOverrides.
type Config struct {
	VaultDir string // git-synced content root
	CacheDir string // per-device index cache; must not be inside VaultDir
}

// Load resolves Config from environment variables and XDG/home defaults.
// VaultDir: RECKON_VAULT env else $HOME/.reckon.
// CacheDir: RECKON_CACHE env else $XDG_CACHE_HOME/reckon else $HOME/.cache/reckon.
func Load() (*Config, error) {
	return LoadWithOverrides("", "")
}

// NewConfig sets VaultDir to the given vaultDir and resolves CacheDir from env/defaults.
// Equivalent to LoadWithOverrides(vaultDir, "").
func NewConfig(vaultDir string) (*Config, error) {
	return LoadWithOverrides(vaultDir, "")
}

// LoadWithOverrides resolves Config with optional overrides for vault and cache dirs.
// Empty strings fall back to env vars and then OS defaults. Returns an error if
// cacheDir is inside vaultDir (would cause the cache to be git-synced).
func LoadWithOverrides(vaultDir, cacheDir string) (*Config, error) {
	// Resolve vault dir
	if vaultDir == "" {
		if v := os.Getenv("RECKON_VAULT"); v != "" {
			vaultDir = v
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("config: resolve vault dir: %w", err)
			}
			vaultDir = filepath.Join(home, ".reckon")
		}
	}

	// Resolve cache dir
	if cacheDir == "" {
		if v := os.Getenv("RECKON_CACHE"); v != "" {
			cacheDir = v
		} else if xdg := os.Getenv("XDG_CACHE_HOME"); xdg != "" {
			cacheDir = filepath.Join(xdg, "reckon")
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("config: resolve cache dir: %w", err)
			}
			cacheDir = filepath.Join(home, ".cache", "reckon")
		}
	}

	// Guard: cache must not be inside vault (would get git-synced)
	rel, err := filepath.Rel(vaultDir, cacheDir)
	if err == nil {
		// rel == "." means cacheDir == vaultDir; rel not starting with ".." means inside
		if rel == "." || !strings.HasPrefix(rel, "..") {
			return nil, fmt.Errorf("config: cache dir %q must not be inside vault %q (EC-7)", cacheDir, vaultDir)
		}
	}

	return &Config{
		VaultDir: vaultDir,
		CacheDir: cacheDir,
	}, nil
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
