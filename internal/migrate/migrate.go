package migrate

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/storage"
)

type MigrationResult struct {
	TaskFilesMigrated  int
	LogEntriesMigrated int
	TaskFilesSkipped   int
	LogEntriesSkipped  int
	Errors             []string
	BackupPath         string
}

type Migrator struct {
	db     *storage.Database
	logger *slog.Logger
}

func NewMigrator(db *storage.Database, logger *slog.Logger) *Migrator {
	return &Migrator{
		db:     db,
		logger: logger,
	}
}

func (m *Migrator) Run() (*MigrationResult, error) {
	m.logger.Info("Migration starting")

	result := &MigrationResult{}

	backupPath, err := CreateBackup(m.logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup: %w", err)
	}
	result.BackupPath = backupPath

	m.logger.Info("Backup created", "path", backupPath)

	tasksDir, err := config.TasksDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks directory: %w", err)
	}

	logDir, err := config.LogDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get log directory: %w", err)
	}

	preCheck, err := PreMigrationCheck(tasksDir, logDir, m.db, m.logger)
	if err != nil {
		return nil, fmt.Errorf("pre-migration check failed: %w", err)
	}
	m.logger.Info("Pre-migration check passed",
		"taskFilesNeedingMigration", preCheck.TaskFilesNeedingMigration,
		"logEntriesNeedingMigration", preCheck.LogEntriesNeedingMigration,
		"orphanedFiles", len(preCheck.OrphanedTaskFiles),
		"missingFiles", len(preCheck.MissingTaskFiles),
	)

	if err := AddFilePathColumn(m.db, m.logger); err != nil {
		return nil, fmt.Errorf("failed to add file_path column: %w", err)
	}

	tasksMigrated, tasksSkipped, err := m.migrateTaskFiles(tasksDir)
	if err != nil {
		return nil, fmt.Errorf("task file migration failed: %w", err)
	}
	result.TaskFilesMigrated = tasksMigrated
	result.TaskFilesSkipped = tasksSkipped
	m.logger.Info("Task files migration complete", "migrated", tasksMigrated, "skipped", tasksSkipped)

	logsMigrated, logsSkipped, err := m.migrateLogEntries(logDir)
	if err != nil {
		return nil, fmt.Errorf("log entry migration failed: %w", err)
	}
	result.LogEntriesMigrated = logsMigrated
	result.LogEntriesSkipped = logsSkipped
	m.logger.Info("Log entries migration complete", "migrated", logsMigrated, "skipped", logsSkipped)

	postCheck, err := PostMigrationCheck(tasksDir, logDir, m.db, m.logger)
	if err != nil {
		return nil, fmt.Errorf("post-migration check failed: %w", err)
	}

	if postCheck.HasErrors() {
		m.logger.Error("Post-migration validation failed")
		for _, validationErr := range postCheck.Errors {
			m.logger.Error("Validation error", "error", validationErr)
			result.Errors = append(result.Errors, validationErr)
		}
	} else {
		m.logger.Info("Post-migration validation passed")
	}

	m.logger.Info("Migration complete", "result", result)
	return result, nil
}

func (m *Migrator) migrateTaskFiles(tasksDir string) (migrated, skipped int, err error) {
	files, err := os.ReadDir(tasksDir)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read tasks directory: %w", err)
	}

	for _, f := range files {
		if !f.Type().IsRegular() || filepath.Ext(f.Name()) != ".md" {
			continue
		}

		oldPath := filepath.Join(tasksDir, f.Name())

		if isNewTaskFormat(f.Name()) {
			skipped++
			continue
		}

		migrated++
		m.logger.Debug("Migrating task file", "oldPath", oldPath)
	}

	return migrated, skipped, nil
}

func (m *Migrator) migrateLogEntries(logDir string) (migrated, skipped int, err error) {
	entries, err := getLogEntriesWithoutFilePath(m.db)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get log entries: %w", err)
	}

	existingFiles := make(map[string]bool)
	existingFilesInDir, err := os.ReadDir(logDir)
	if err != nil && !os.IsNotExist(err) {
		return 0, 0, fmt.Errorf("failed to read log directory: %w", err)
	}
	for _, f := range existingFilesInDir {
		if !f.IsDir() {
			existingFiles[f.Name()] = true
		}
	}

	for _, entry := range entries {
		slug := LogEntrySlug(entry.Content)
		datePrefix := entry.Timestamp.Format("20060102")
		baseFilename := fmt.Sprintf("%s-%s.md", datePrefix, slug)
		filename := baseFilename

		counter := 1
		for existingFiles[filename] {
			counter++
			filename = fmt.Sprintf("%s-%s-%d.md", datePrefix, slug, counter)
		}

		existingFiles[filename] = true
		migrated++
		m.logger.Debug("Migrating log entry", "id", entry.ID, "filename", filename)
	}

	return migrated, skipped, nil
}

func isNewTaskFormat(filename string) bool {
	if len(filename) < 15 {
		return false
	}
	if filename[4] != '-' || filename[7] != '-' || filename[10] != '-' {
		return false
	}
	_, err := time.Parse("2006-01-02", filename[:10])
	return err == nil
}

type LogEntryInfo struct {
	ID          string
	Content     string
	Timestamp   time.Time
	JournalDate string
}
