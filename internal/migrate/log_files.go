package migrate

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
)

var logSlugRegex = regexp.MustCompile(`[^a-z0-9]+`)

type LogEntryFile struct {
	ID          string
	Content     string
	Timestamp   time.Time
	JournalDate string
	FilePath    string
}

type LogEntryMigrationInfo struct {
	ID          string
	Content     string
	Timestamp   time.Time
	JournalDate string
}

func LogEntrySlug(content string) string {
	words := strings.Fields(content)
	if len(words) == 0 {
		return "untitled"
	}
	slug := strings.Join(words[:min(len(words), 4)], " ")
	slug = strings.ToLower(slug)
	slug = logSlugRegex.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if len(slug) > 50 {
		slug = slug[:50]
		slug = strings.Trim(slug, "-")
	}
	if slug == "" {
		slug = "untitled"
	}
	return slug
}

func LogEntryFilename(info LogEntryMigrationInfo) string {
	datePrefix := info.Timestamp.Format("20060102")
	slug := LogEntrySlug(info.Content)
	return fmt.Sprintf("%s-%s.md", datePrefix, slug)
}

func MigrateLogFiles(logDir string, db *sql.DB, logger *slog.Logger) (migrated, skipped int, err error) {
	logsDir, err := config.LogDir()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get log directory: %w", err)
	}

	entries, err := getLogEntriesWithoutFilePathDB(db)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get log entries: %w", err)
	}

	existingFiles := make(map[string]bool)
	existingFilesInDir, err := os.ReadDir(logsDir)
	if err != nil && !os.IsNotExist(err) {
		return 0, 0, fmt.Errorf("failed to read log directory: %w", err)
	}
	for _, f := range existingFilesInDir {
		if !f.IsDir() {
			existingFiles[f.Name()] = true
		}
	}

	for _, entry := range entries {
		info := LogEntryMigrationInfo{
			ID:          entry.ID,
			Content:     entry.Content,
			Timestamp:   entry.Timestamp,
			JournalDate: entry.JournalDate,
		}

		filename := LogEntryFilename(info)

		if existingFiles[filename] {
			base := strings.TrimSuffix(filename, ".md")
			counter := 1
			for {
				newFilename := fmt.Sprintf("%s-%d.md", base, counter)
				if !existingFiles[newFilename] {
					filename = newFilename
					break
				}
				counter++
			}
		}

		existingFiles[filename] = true

		filePath := filepath.Join(logsDir, filename)

		content := formatLogEntryContent(entry)
		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			logger.Error("Failed to write log entry file", "path", filePath, "error", err)
			continue
		}

		if err := updateLogEntryFilePath(db, entry.ID, filePath); err != nil {
			logger.Error("Failed to update log entry file path", "id", entry.ID, "error", err)
			continue
		}

		migrated++
		logger.Debug("Migrated log entry", "id", entry.ID, "filename", filename)
	}

	return migrated, skipped, nil
}

func formatLogEntryContent(entry LogEntryFile) string {
	var sb strings.Builder

	sb.WriteString("---\n")
	sb.WriteString(fmt.Sprintf("id: %s\n", entry.ID))
	sb.WriteString(fmt.Sprintf("journal_date: %s\n", entry.JournalDate))
	sb.WriteString(fmt.Sprintf("timestamp: %s\n", entry.Timestamp.Format(time.RFC3339)))
	sb.WriteString("---\n\n")
	sb.WriteString(entry.Content)

	return sb.String()
}

func ValidateLogFiles(logDir string, db *sql.DB, logger *slog.Logger) error {
	_, err := config.LogDir()
	if err != nil {
		return fmt.Errorf("failed to get log directory: %w", err)
	}

	entries, err := getLogEntriesWithoutFilePathDB(db)
	if err != nil {
		return fmt.Errorf("failed to get log entries: %w", err)
	}

	if len(entries) > 0 {
		return fmt.Errorf("found %d log entries without file_path", len(entries))
	}

	return nil
}

func CountLogEntriesWithoutFiles(db *sql.DB) (int, error) {
	return getLogEntriesWithoutFilePathCount(db)
}

func getLogEntriesWithoutFilePathDB(db *sql.DB) ([]LogEntryFile, error) {
	rows, err := db.Query(`
		SELECT id, journal_date, timestamp, content, file_path
		FROM log_entries
		WHERE file_path IS NULL OR file_path = ''
		ORDER BY journal_date, position
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LogEntryFile
	for rows.Next() {
		var entry LogEntryFile
		var timestampStr string
		if err := rows.Scan(&entry.ID, &entry.JournalDate, &timestampStr, &entry.Content, &entry.FilePath); err != nil {
			return nil, err
		}
		entry.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
		entries = append(entries, entry)
	}

	return entries, nil
}

func getLogEntriesWithoutFilePathCount(db *sql.DB) (int, error) {
	var count int
	rows, err := db.Query("SELECT COUNT(*) FROM log_entries WHERE file_path IS NULL OR file_path = ''")
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

func updateLogEntryFilePath(db *sql.DB, id, filePath string) error {
	_, err := db.Exec("UPDATE log_entries SET file_path = ? WHERE id = ?", filePath, id)
	return err
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
