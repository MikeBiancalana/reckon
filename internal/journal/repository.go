package journal

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/MikeBiancalana/reckon/internal/storage"
)

// Repository handles all database operations for journals
type Repository struct {
	db *storage.Database
}

// NewRepository creates a new repository
func NewRepository(db *storage.Database) *Repository {
	return &Repository{db: db}
}

// SaveJournal saves a complete journal to the database
// This performs a full replace operation within a transaction
func (r *Repository) SaveJournal(j *Journal) error {
	tx, err := r.db.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Delete existing journal data for this date
	if err := r.deleteJournalData(tx, j.Date); err != nil {
		return err
	}

	// Insert journal record
	_, err = tx.Exec(
		"INSERT INTO journals (date, file_path, last_modified) VALUES (?, ?, ?)",
		j.Date, j.FilePath, j.LastModified.Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to insert journal: %w", err)
	}

	// Insert intentions
	for _, intention := range j.Intentions {
		_, err = tx.Exec(
			`INSERT INTO intentions (id, journal_date, text, status, carried_from, position)
			 VALUES (?, ?, ?, ?, ?, ?)`,
			intention.ID, j.Date, intention.Text, intention.Status,
			intention.CarriedFrom, intention.Position,
		)
		if err != nil {
			return fmt.Errorf("failed to insert intention: %w", err)
		}
	}

	// Insert log entries
	for _, entry := range j.LogEntries {
		_, err = tx.Exec(
			`INSERT INTO log_entries (id, journal_date, timestamp, content, task_id, entry_type, duration_minutes, position)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			entry.ID, j.Date, entry.Timestamp.Format(time.RFC3339), entry.Content,
			entry.TaskID, entry.EntryType, entry.DurationMinutes, entry.Position,
		)
		if err != nil {
			return fmt.Errorf("failed to insert log entry: %w", err)
		}

		// Save notes for this log entry
		if err := r.SaveLogNotes(tx, entry.ID, entry.Notes); err != nil {
			return err
		}
	}

	// Insert wins
	for _, win := range j.Wins {
		_, err = tx.Exec(
			"INSERT INTO wins (id, journal_date, text, position) VALUES (?, ?, ?, ?)",
			win.ID, j.Date, win.Text, win.Position,
		)
		if err != nil {
			return fmt.Errorf("failed to insert win: %w", err)
		}
	}

	// Insert schedule items
	for _, item := range j.ScheduleItems {
		timeStr := ""
		if !item.Time.IsZero() {
			timeStr = item.Time.Format(time.RFC3339)
		}
		_, err = tx.Exec(
			`INSERT INTO schedule_items (id, journal_date, time, content, position)
			 VALUES (?, ?, ?, ?, ?)`,
			item.ID, j.Date, timeStr, item.Content, item.Position,
		)
		if err != nil {
			return fmt.Errorf("failed to insert schedule item: %w", err)
		}
	}

	return tx.Commit()
}

// GetJournalByDate retrieves a journal by date
func (r *Repository) GetJournalByDate(date string) (*Journal, error) {
	j := &Journal{
		Date:          date,
		Intentions:    make([]Intention, 0),
		Wins:          make([]Win, 0),
		LogEntries:    make([]LogEntry, 0),
		ScheduleItems: make([]ScheduleItem, 0),
	}

	// Get journal metadata
	var lastModifiedUnix int64
	err := r.db.DB().QueryRow(
		"SELECT file_path, last_modified FROM journals WHERE date = ?",
		date,
	).Scan(&j.FilePath, &lastModifiedUnix)

	if err == sql.ErrNoRows {
		return nil, nil // Journal not found
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get journal: %w", err)
	}

	j.LastModified = time.Unix(lastModifiedUnix, 0)

	// Get intentions
	rows, err := r.db.DB().Query(
		`SELECT id, text, status, carried_from, position
		 FROM intentions WHERE journal_date = ? ORDER BY position`,
		date,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get intentions: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var intention Intention
		var carriedFrom sql.NullString
		err := rows.Scan(&intention.ID, &intention.Text, &intention.Status,
			&carriedFrom, &intention.Position)
		if err != nil {
			return nil, fmt.Errorf("failed to scan intention: %w", err)
		}
		if carriedFrom.Valid {
			intention.CarriedFrom = carriedFrom.String
		}
		j.Intentions = append(j.Intentions, intention)
	}

	// Get log entries with notes using LEFT JOIN
	rows, err = r.db.DB().Query(
		`SELECT le.id, le.timestamp, le.content, le.task_id, le.entry_type, le.duration_minutes, le.position,
		        ln.id, ln.text, ln.position
		 FROM log_entries le
		 LEFT JOIN log_notes ln ON le.id = ln.log_entry_id
		 WHERE le.journal_date = ?
		 ORDER BY le.position, ln.position`,
		date,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get log entries: %w", err)
	}
	defer rows.Close()

	entriesMap := make(map[string]*LogEntry)
	entryOrder := make([]string, 0)

	for rows.Next() {
		var entryID, timestampStr, content string
		var taskID sql.NullString
		var durationMinutes sql.NullInt64
		var entryType EntryType
		var entryPosition int
		var noteID, noteText sql.NullString
		var notePosition sql.NullInt64

		err := rows.Scan(&entryID, &timestampStr, &content, &taskID,
			&entryType, &durationMinutes, &entryPosition,
			&noteID, &noteText, &notePosition)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log entry: %w", err)
		}

		// Get or create entry
		entry, exists := entriesMap[entryID]
		if !exists {
			entry = &LogEntry{
				ID:        entryID,
				Content:   content,
				EntryType: entryType,
				Position:  entryPosition,
				Notes:     make([]LogNote, 0),
			}
			entry.Timestamp, _ = time.Parse(time.RFC3339, timestampStr)
			if taskID.Valid {
				entry.TaskID = taskID.String
			}
			if durationMinutes.Valid {
				entry.DurationMinutes = int(durationMinutes.Int64)
			}
			entriesMap[entryID] = entry
			entryOrder = append(entryOrder, entryID)
		}

		// Add note if it exists
		if noteID.Valid {
			note := LogNote{
				ID:       noteID.String,
				Text:     noteText.String,
				Position: int(notePosition.Int64),
			}
			entry.Notes = append(entry.Notes, note)
		}
	}

	// Convert map to slice in the correct order
	for _, id := range entryOrder {
		j.LogEntries = append(j.LogEntries, *entriesMap[id])
	}

	// Get wins
	rows, err = r.db.DB().Query(
		"SELECT id, text, position FROM wins WHERE journal_date = ? ORDER BY position",
		date,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get wins: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var win Win
		err := rows.Scan(&win.ID, &win.Text, &win.Position)
		if err != nil {
			return nil, fmt.Errorf("failed to scan win: %w", err)
		}
		j.Wins = append(j.Wins, win)
	}

	// Get schedule items
	rows, err = r.db.DB().Query(
		`SELECT id, time, content, position
		 FROM schedule_items WHERE journal_date = ? ORDER BY position`,
		date,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule items: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item ScheduleItem
		var timeStr string

		err := rows.Scan(&item.ID, &timeStr, &item.Content, &item.Position)
		if err != nil {
			return nil, fmt.Errorf("failed to scan schedule item: %w", err)
		}

		if timeStr != "" {
			parsedTime, err := time.Parse(time.RFC3339, timeStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse schedule item time for item %s: %w", item.ID, err)
			}
			item.Time = parsedTime
		}

		j.ScheduleItems = append(j.ScheduleItems, item)
	}

	return j, nil
}

// DeleteJournal deletes a journal and all its associated data
func (r *Repository) DeleteJournal(date string) error {
	tx, err := r.db.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if err := r.deleteJournalData(tx, date); err != nil {
		return err
	}

	return tx.Commit()
}

// deleteJournalData deletes all data for a journal within a transaction
func (r *Repository) deleteJournalData(tx *sql.Tx, date string) error {
	// Delete in order due to foreign keys
	tables := []string{"intentions", "log_entries", "wins", "schedule_items", "journals"}
	for _, table := range tables {
		query := fmt.Sprintf("DELETE FROM %s WHERE ", table)
		if table == "journals" {
			query += "date = ?"
		} else {
			query += "journal_date = ?"
		}

		_, err := tx.Exec(query, date)
		if err != nil {
			return fmt.Errorf("failed to delete from %s: %w", table, err)
		}
	}
	return nil
}

// GetScheduleItems retrieves schedule items for a given date
func (r *Repository) GetScheduleItems(date string) ([]ScheduleItem, error) {
	rows, err := r.db.DB().Query(
		`SELECT id, time, content, position
		 FROM schedule_items WHERE journal_date = ? ORDER BY position`,
		date,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get schedule items: %w", err)
	}
	defer rows.Close()

	items := make([]ScheduleItem, 0)
	for rows.Next() {
		var item ScheduleItem
		var timeStr string

		err := rows.Scan(&item.ID, &timeStr, &item.Content, &item.Position)
		if err != nil {
			return nil, fmt.Errorf("failed to scan schedule item: %w", err)
		}

		if timeStr != "" {
			parsedTime, err := time.Parse(time.RFC3339, timeStr)
			if err != nil {
				return nil, fmt.Errorf("failed to parse schedule item time for item %s: %w", item.ID, err)
			}
			item.Time = parsedTime
		}

		items = append(items, item)
	}

	return items, nil
}

// GetOpenIntentions retrieves all open intentions for a given date
func (r *Repository) GetOpenIntentions(date string) ([]Intention, error) {
	rows, err := r.db.DB().Query(
		`SELECT id, text, status, carried_from, position
		 FROM intentions WHERE journal_date = ? AND status = ? ORDER BY position`,
		date, IntentionOpen,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get open intentions: %w", err)
	}
	defer rows.Close()

	intentions := make([]Intention, 0)
	for rows.Next() {
		var intention Intention
		var carriedFrom sql.NullString
		err := rows.Scan(&intention.ID, &intention.Text, &intention.Status,
			&carriedFrom, &intention.Position)
		if err != nil {
			return nil, fmt.Errorf("failed to scan intention: %w", err)
		}
		if carriedFrom.Valid {
			intention.CarriedFrom = carriedFrom.String
		}
		intentions = append(intentions, intention)
	}

	return intentions, nil
}

// SaveLogNotes saves notes for a log entry
// Deletes existing notes and inserts new ones
func (r *Repository) SaveLogNotes(tx *sql.Tx, logEntryID string, notes []LogNote) error {
	// Delete existing notes for this log entry
	_, err := tx.Exec("DELETE FROM log_notes WHERE log_entry_id = ?", logEntryID)
	if err != nil {
		return fmt.Errorf("failed to delete old log notes: %w", err)
	}

	// Insert new notes
	for _, note := range notes {
		_, err = tx.Exec(`
			INSERT INTO log_notes (id, log_entry_id, text, position)
			VALUES (?, ?, ?, ?)
		`, note.ID, logEntryID, note.Text, note.Position)
		if err != nil {
			return fmt.Errorf("failed to save log note: %w", err)
		}
	}

	return nil
}

// GetLogNotes retrieves notes for a log entry
func (r *Repository) GetLogNotes(logEntryID string) ([]LogNote, error) {
	rows, err := r.db.DB().Query(`
		SELECT id, text, position
		FROM log_notes
		WHERE log_entry_id = ?
		ORDER BY position
	`, logEntryID)
	if err != nil {
		return nil, fmt.Errorf("failed to query log notes: %w", err)
	}
	defer rows.Close()

	notes := make([]LogNote, 0)
	for rows.Next() {
		var note LogNote
		err := rows.Scan(&note.ID, &note.Text, &note.Position)
		if err != nil {
			return nil, fmt.Errorf("failed to scan log note: %w", err)
		}
		notes = append(notes, note)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating log notes: %w", err)
	}

	return notes, nil
}

// ClearAllData deletes all data from the database (for rebuild)
func (r *Repository) ClearAllData() error {
	tx, err := r.db.BeginTx()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	tables := []string{"intentions", "log_entries", "wins", "schedule_items", "journals"}
	for _, table := range tables {
		if _, err := tx.Exec(fmt.Sprintf("DELETE FROM %s", table)); err != nil {
			return fmt.Errorf("failed to clear %s: %w", table, err)
		}
	}

	return tx.Commit()
}
