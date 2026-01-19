package migrate

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeBiancalana/reckon/internal/storage"
)

type PreMigrationCheckResult struct {
	TaskFilesNeedingMigration  int
	TaskFilesSkipped           int
	LogEntriesNeedingMigration int
	OrphanedTaskFiles          []string
	MissingTaskFiles           []string
	OrphanedLogFiles           []string
	MissingLogFiles            []string
	DuplicateSlugs             []string
}

type PostMigrationCheckResult struct {
	Errors            []string
	TotalFiles        int
	TotalDBRecords    int
	OrphanedTaskFiles []string
	DuplicateSlugs    []string
}

func (r *PostMigrationCheckResult) HasErrors() bool {
	return len(r.Errors) > 0
}

func PreMigrationCheck(tasksDir, logDir string, db *storage.Database, logger *slog.Logger) (*PreMigrationCheckResult, error) {
	result := &PreMigrationCheckResult{}

	tasksFiles, err := os.ReadDir(tasksDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read tasks directory: %w", err)
	}

	for _, f := range tasksFiles {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".md") {
			if isNewTaskFormat(f.Name()) {
				result.TaskFilesSkipped++
			} else {
				result.TaskFilesNeedingMigration++
				result.OrphanedTaskFiles = append(result.OrphanedTaskFiles, f.Name())
			}
		}
	}

	logFiles, err := os.ReadDir(logDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read log directory: %w", err)
	}
	for _, f := range logFiles {
		if !f.IsDir() {
			result.OrphanedLogFiles = append(result.OrphanedLogFiles, f.Name())
		}
	}

	if err := addColumnIfMissingForCheck(db, "log_entries", "file_path", "TEXT"); err != nil {
		logger.Warn("Could not add file_path column for check", "error", err)
		result.LogEntriesNeedingMigration = -1
	} else {
		logEntriesCount, err := countLogEntriesWithoutFilePath(db)
		if err != nil {
			return nil, fmt.Errorf("failed to count log entries: %w", err)
		}
		result.LogEntriesNeedingMigration = logEntriesCount
	}

	logger.Info("Pre-migration check completed", "result", result)
	return result, nil
}

func addColumnIfMissingForCheck(db *storage.Database, table, column, colType string) error {
	rows, err := db.DB().Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return fmt.Errorf("failed to get table info for %s: %w", table, err)
	}
	defer rows.Close()

	columnExists := false
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dfltValue sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return fmt.Errorf("failed to scan column info: %w", err)
		}
		if name == column {
			columnExists = true
			break
		}
	}

	if !columnExists {
		_, err := db.DB().Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, colType))
		if err != nil {
			return fmt.Errorf("failed to add column %s to %s: %w", column, table, err)
		}
	}

	return nil
}

func PostMigrationCheck(tasksDir, logDir string, db *storage.Database, logger *slog.Logger) (*PostMigrationCheckResult, error) {
	result := &PostMigrationCheckResult{}

	tasksFiles, err := os.ReadDir(tasksDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read tasks directory: %w", err)
	}

	existingSlugs := make(map[string]bool)
	for _, f := range tasksFiles {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".md") {
			result.TotalFiles++
			if !isNewTaskFormat(f.Name()) {
				result.Errors = append(result.Errors, fmt.Sprintf("Task file not in new format: %s", f.Name()))
			}
			if strings.HasSuffix(f.Name(), ".md") && len(f.Name()) > 11 {
				slug := strings.TrimSuffix(f.Name(), ".md")
				if existingSlugs[slug] {
					result.DuplicateSlugs = append(result.DuplicateSlugs, f.Name())
				}
				existingSlugs[slug] = true
			}
		}
	}

	logFiles, err := os.ReadDir(logDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read log directory: %w", err)
	}
	for _, f := range logFiles {
		if !f.IsDir() {
			result.TotalFiles++
			if !strings.HasSuffix(f.Name(), ".md") {
				result.Errors = append(result.Errors, fmt.Sprintf("Log file not markdown: %s", f.Name()))
			}
		}
	}

	totalRecords, err := countAllRecords(db)
	if err != nil {
		return nil, fmt.Errorf("failed to count records: %w", err)
	}
	result.TotalDBRecords = totalRecords

	if len(result.OrphanedTaskFiles) > 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("Found %d orphaned task files", len(result.OrphanedTaskFiles)))
	}

	if len(result.DuplicateSlugs) > 0 {
		result.Errors = append(result.Errors, fmt.Sprintf("Found %d duplicate slugs", len(result.DuplicateSlugs)))
	}

	logger.Info("Post-migration check completed", "totalFiles", result.TotalFiles, "totalDBRecords", result.TotalDBRecords, "errors", len(result.Errors))
	return result, nil
}

func ValidateNoOrphanedFiles(tasksDir string, db *sql.DB, logger *slog.Logger) error {
	tasksFiles, err := os.ReadDir(tasksDir)
	if err != nil {
		return fmt.Errorf("failed to read tasks directory: %w", err)
	}

	existingPaths := make(map[string]bool)
	rows, err := db.Query("SELECT id FROM tasks")
	if err != nil {
		return fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("failed to scan task id: %w", err)
		}
		existingPaths[id] = true
	}

	var orphaned []string
	for _, f := range tasksFiles {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".md") {
			continue
		}

		baseName := strings.TrimSuffix(f.Name(), ".md")
		if !existingPaths[baseName] {
			orphaned = append(orphaned, f.Name())
		}
	}

	if len(orphaned) > 0 {
		return fmt.Errorf("found %d orphaned task files: %v", len(orphaned), orphaned)
	}

	return nil
}

func ValidateNoMissingFiles(tasksDir string, db *sql.DB, logger *slog.Logger) error {
	rows, err := db.Query("SELECT id FROM tasks")
	if err != nil {
		return fmt.Errorf("failed to query tasks: %w", err)
	}
	defer rows.Close()

	var missing []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return fmt.Errorf("failed to scan task id: %w", err)
		}

		expectedPath := filepath.Join(tasksDir, id+".md")
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			missing = append(missing, id)
		}
	}

	if len(missing) > 0 {
		return fmt.Errorf("found %d missing task files for IDs: %v", len(missing), missing)
	}

	return nil
}

func countLogEntriesWithoutFilePath(db *storage.Database) (int, error) {
	var count int
	rows, err := db.DB().Query("SELECT COUNT(*) FROM log_entries WHERE file_path IS NULL OR file_path = ''")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}

	return count, nil
}

func countAllRecords(db *storage.Database) (int, error) {
	var count int
	rows, err := db.DB().Query("SELECT COUNT(*) FROM tasks")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.Scan(&count); err != nil {
			return 0, err
		}
	}

	return count, nil
}
