package migrate

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/MikeBiancalana/reckon/internal/storage"
)

func AddFilePathColumn(db *storage.Database, logger *slog.Logger) error {
	logger.Info("Adding file_path column to log_entries table")

	if err := addColumnIfMissing(db, "log_entries", "file_path", "TEXT"); err != nil {
		return fmt.Errorf("failed to add file_path column to log_entries: %w", err)
	}

	logger.Info("file_path column added successfully")
	return nil
}

func addColumnIfMissing(db *storage.Database, table, column, colType string) error {
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

func getLogEntriesWithoutFilePath(db *storage.Database) ([]LogEntryMigrationInfo, error) {
	rows, err := db.DB().Query(`
		SELECT id, journal_date, timestamp, content
		FROM log_entries
		WHERE file_path IS NULL OR file_path = ''
		ORDER BY journal_date, position
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []LogEntryMigrationInfo
	for rows.Next() {
		var entry LogEntryMigrationInfo
		var timestampStr string
		if err := rows.Scan(&entry.ID, &entry.JournalDate, &timestampStr, &entry.Content); err != nil {
			return nil, err
		}
		t, _ := time.Parse(time.RFC3339, timestampStr)
		entry.Timestamp = t
		entries = append(entries, entry)
	}

	return entries, nil
}
