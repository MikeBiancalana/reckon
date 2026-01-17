package journal

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/storage"
)

// Service handles journal business logic.
// Note: Service methods are not thread-safe. If concurrent access is required,
// external synchronization must be provided by the caller.
type Service struct {
	repo      *Repository
	fileStore *storage.FileStore
	logger    *slog.Logger
}

// NewService creates a new journal service
func NewService(repo *Repository, fileStore *storage.FileStore, logger *slog.Logger) *Service {
	return &Service{
		repo:      repo,
		fileStore: fileStore,
		logger:    DefaultLogger(logger),
	}
}

// GetToday returns today's journal, creating it if it doesn't exist
func (s *Service) GetToday() (*Journal, error) {
	today := time.Now().Format("2006-01-02")
	return s.GetByDate(today)
}

// GetByDate returns a journal for the given date, creating it if it doesn't exist
func (s *Service) GetByDate(date string) (*Journal, error) {
	s.logger.Info("GetByDate", "journal_date", date)

	// Try to read from filesystem first
	content, fileInfo, err := s.fileStore.ReadJournalFile(date)
	if err != nil {
		s.logger.Error("GetByDate", "error", err, "journal_date", date)
		return nil, fmt.Errorf("failed to read journal: %w", err)
	}

	var j *Journal

	// If journal doesn't exist, create a new one
	if !fileInfo.Exists {
		j = NewJournal(date)

		// Auto-carry intentions from yesterday if this is today
		today := time.Now().Format("2006-01-02")
		if date == today {
			if err := s.autoCarryIntentions(j); err != nil {
				s.logger.Error("GetByDate", "error", err, "journal_date", date, "operation", "auto_carry_intentions")
				return nil, fmt.Errorf("failed to auto-carry intentions: %w", err)
			}
		}

		// Save the new journal
		if err := s.save(j); err != nil {
			s.logger.Error("GetByDate", "error", err, "journal_date", date, "operation", "save_journal")
			return nil, fmt.Errorf("failed to save new journal: %w", err)
		}
	} else {
		// Parse the journal content
		j, err = s.parseJournal(content, fileInfo.Path, fileInfo.LastModified)
		if err != nil {
			s.logger.Error("GetByDate", "error", err, "journal_date", date, "operation", "parse_journal")
			return nil, fmt.Errorf("failed to parse journal: %w", err)
		}

		// Load schedule items from database (database is source of truth for schedule items)
		scheduleItems, err := s.repo.GetScheduleItems(date)
		if err != nil {
			s.logger.Error("GetByDate", "error", err, "journal_date", date, "operation", "load_schedule_items")
			return nil, fmt.Errorf("failed to load schedule items from database: %w", err)
		}
		j.ScheduleItems = scheduleItems
	}

	s.logger.Debug("GetByDate", "journal_date", date, "intention_count", len(j.Intentions), "log_entry_count", len(j.LogEntries), "win_count", len(j.Wins))
	return j, nil
}

// AppendLog appends a log entry to the journal
func (s *Service) AppendLog(j *Journal, content string) error {
	s.logger.Debug("AppendLog", "journal_date", j.Date, "content_length", len(content))

	timestamp := time.Now()
	position := len(j.LogEntries)

	// Determine entry type based on content
	entryType := EntryTypeLog
	if len(content) > 0 {
		if content[0] == '[' {
			if content[1:9] == "meeting:" {
				entryType = EntryTypeMeeting
			} else if content[1:7] == "break]" {
				entryType = EntryTypeBreak
			}
		}
	}

	entry := NewLogEntry(timestamp, content, entryType, position)
	j.LogEntries = append(j.LogEntries, *entry)

	if err := s.save(j); err != nil {
		s.logger.Error("AppendLog", "error", err, "journal_date", j.Date, "entry_id", entry.ID)
		return err
	}
	return nil
}

// AddIntention adds a new intention to the journal
func (s *Service) AddIntention(j *Journal, text string) error {
	s.logger.Debug("AddIntention", "journal_date", j.Date, "intention_text", text)

	position := len(j.Intentions)
	intention := NewIntention(text, position)
	j.Intentions = append(j.Intentions, *intention)

	if err := s.save(j); err != nil {
		s.logger.Error("AddIntention", "error", err, "journal_date", j.Date, "intention_id", intention.ID)
		return err
	}
	return nil
}

// ToggleIntention toggles an intention between open and done
func (s *Service) ToggleIntention(j *Journal, intentionID string) error {
	s.logger.Debug("ToggleIntention", "journal_date", j.Date, "intention_id", intentionID)

	for i := range j.Intentions {
		if j.Intentions[i].ID == intentionID {
			if j.Intentions[i].Status == IntentionDone {
				j.Intentions[i].Status = IntentionOpen
			} else {
				j.Intentions[i].Status = IntentionDone
			}
			if err := s.save(j); err != nil {
				s.logger.Error("ToggleIntention", "error", err, "journal_date", j.Date, "intention_id", intentionID)
				return err
			}
			return nil
		}
	}

	err := fmt.Errorf("intention not found: %s", intentionID)
	s.logger.Warn("ToggleIntention", "error", err, "journal_date", j.Date, "intention_id", intentionID)
	return err
}

// AddWin adds a new win to the journal
func (s *Service) AddWin(j *Journal, text string) error {
	s.logger.Debug("AddWin", "journal_date", j.Date, "win_text", text)

	position := len(j.Wins)
	win := NewWin(text, position)
	j.Wins = append(j.Wins, *win)

	if err := s.save(j); err != nil {
		s.logger.Error("AddWin", "error", err, "journal_date", j.Date, "win_id", win.ID)
		return err
	}
	return nil
}

// DeleteIntention removes an intention by ID and re-indexes positions
func (s *Service) DeleteIntention(j *Journal, intentionID string) error {
	s.logger.Debug("DeleteIntention", "journal_date", j.Date, "intention_id", intentionID)

	found := false
	for i, intention := range j.Intentions {
		if intention.ID == intentionID {
			j.Intentions = append(j.Intentions[:i], j.Intentions[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		err := fmt.Errorf("intention not found: %s", intentionID)
		s.logger.Warn("DeleteIntention", "error", err, "journal_date", j.Date, "intention_id", intentionID)
		return err
	}

	// Re-index positions
	for i := range j.Intentions {
		j.Intentions[i].Position = i
	}

	if err := s.save(j); err != nil {
		s.logger.Error("DeleteIntention", "error", err, "journal_date", j.Date, "intention_id", intentionID)
		return err
	}
	return nil
}

// DeleteWin removes a win by ID and re-indexes positions
func (s *Service) DeleteWin(j *Journal, winID string) error {
	s.logger.Debug("DeleteWin", "journal_date", j.Date, "win_id", winID)

	found := false
	for i, win := range j.Wins {
		if win.ID == winID {
			j.Wins = append(j.Wins[:i], j.Wins[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		err := fmt.Errorf("win not found: %s", winID)
		s.logger.Warn("DeleteWin", "error", err, "journal_date", j.Date, "win_id", winID)
		return err
	}

	// Re-index positions
	for i := range j.Wins {
		j.Wins[i].Position = i
	}

	if err := s.save(j); err != nil {
		s.logger.Error("DeleteWin", "error", err, "journal_date", j.Date, "win_id", winID)
		return err
	}
	return nil
}

// DeleteLogEntry removes a log entry by ID and re-indexes positions
func (s *Service) DeleteLogEntry(j *Journal, logEntryID string) error {
	s.logger.Debug("DeleteLogEntry", "journal_date", j.Date, "log_entry_id", logEntryID)

	found := false
	for i, entry := range j.LogEntries {
		if entry.ID == logEntryID {
			j.LogEntries = append(j.LogEntries[:i], j.LogEntries[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		err := fmt.Errorf("log entry not found: %s", logEntryID)
		s.logger.Warn("DeleteLogEntry", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID)
		return err
	}

	// Re-index positions
	for i := range j.LogEntries {
		j.LogEntries[i].Position = i
	}

	if err := s.save(j); err != nil {
		s.logger.Error("DeleteLogEntry", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID)
		return err
	}
	return nil
}

// AddLogNote adds a note to a log entry
func (s *Service) AddLogNote(j *Journal, logEntryID string, text string) error {
	s.logger.Debug("AddLogNote", "journal_date", j.Date, "log_entry_id", logEntryID)

	text = strings.TrimSpace(text)
	if text == "" {
		err := fmt.Errorf("note text cannot be empty")
		s.logger.Warn("AddLogNote", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID)
		return err
	}

	// Find the log entry
	var targetEntry *LogEntry
	for i := range j.LogEntries {
		if j.LogEntries[i].ID == logEntryID {
			targetEntry = &j.LogEntries[i]
			break
		}
	}

	if targetEntry == nil {
		err := fmt.Errorf("log entry not found: %s", logEntryID)
		s.logger.Warn("AddLogNote", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID)
		return err
	}

	// Create new note
	position := len(targetEntry.Notes)
	note := NewLogNote(text, position)
	targetEntry.Notes = append(targetEntry.Notes, *note)

	if err := s.save(j); err != nil {
		s.logger.Error("AddLogNote", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID, "note_id", note.ID)
		return err
	}
	return nil
}

// UpdateLogNote updates the text of a note in a log entry
func (s *Service) UpdateLogNote(j *Journal, logEntryID string, noteID string, newText string) error {
	s.logger.Debug("UpdateLogNote", "journal_date", j.Date, "log_entry_id", logEntryID, "note_id", noteID)

	newText = strings.TrimSpace(newText)
	if newText == "" {
		err := fmt.Errorf("note text cannot be empty")
		s.logger.Warn("UpdateLogNote", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID, "note_id", noteID)
		return err
	}

	// Find the log entry
	var targetEntry *LogEntry
	for i := range j.LogEntries {
		if j.LogEntries[i].ID == logEntryID {
			targetEntry = &j.LogEntries[i]
			break
		}
	}

	if targetEntry == nil {
		err := fmt.Errorf("log entry not found: %s", logEntryID)
		s.logger.Warn("UpdateLogNote", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID, "note_id", noteID)
		return err
	}

	// Find and update the note
	found := false
	for i := range targetEntry.Notes {
		if targetEntry.Notes[i].ID == noteID {
			targetEntry.Notes[i].Text = newText
			found = true
			break
		}
	}

	if !found {
		err := fmt.Errorf("note not found: %s", noteID)
		s.logger.Warn("UpdateLogNote", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID, "note_id", noteID)
		return err
	}

	if err := s.save(j); err != nil {
		s.logger.Error("UpdateLogNote", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID, "note_id", noteID)
		return err
	}
	return nil
}

// DeleteLogNote removes a note from a log entry
func (s *Service) DeleteLogNote(j *Journal, logEntryID string, noteID string) error {
	s.logger.Debug("DeleteLogNote", "journal_date", j.Date, "log_entry_id", logEntryID, "note_id", noteID)

	// Find the log entry
	var targetEntry *LogEntry
	for i := range j.LogEntries {
		if j.LogEntries[i].ID == logEntryID {
			targetEntry = &j.LogEntries[i]
			break
		}
	}

	if targetEntry == nil {
		err := fmt.Errorf("log entry not found: %s", logEntryID)
		s.logger.Warn("DeleteLogNote", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID, "note_id", noteID)
		return err
	}

	// Find and remove the note
	found := false
	for i, note := range targetEntry.Notes {
		if note.ID == noteID {
			targetEntry.Notes = append(targetEntry.Notes[:i], targetEntry.Notes[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		err := fmt.Errorf("note not found: %s", noteID)
		s.logger.Warn("DeleteLogNote", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID, "note_id", noteID)
		return err
	}

	// Re-index positions
	for i := range targetEntry.Notes {
		targetEntry.Notes[i].Position = i
	}

	if err := s.save(j); err != nil {
		s.logger.Error("DeleteLogNote", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID, "note_id", noteID)
		return err
	}
	return nil
}

// UpdateIntention updates the text of an intention by ID
func (s *Service) UpdateIntention(j *Journal, intentionID string, newText string) error {
	s.logger.Debug("UpdateIntention", "journal_date", j.Date, "intention_id", intentionID)

	for i := range j.Intentions {
		if j.Intentions[i].ID == intentionID {
			j.Intentions[i].Text = newText
			if err := s.save(j); err != nil {
				s.logger.Error("UpdateIntention", "error", err, "journal_date", j.Date, "intention_id", intentionID)
				return err
			}
			return nil
		}
	}
	err := fmt.Errorf("intention not found: %s", intentionID)
	s.logger.Warn("UpdateIntention", "error", err, "journal_date", j.Date, "intention_id", intentionID)
	return err
}

// UpdateWin updates the text of a win by ID
func (s *Service) UpdateWin(j *Journal, winID string, newText string) error {
	s.logger.Debug("UpdateWin", "journal_date", j.Date, "win_id", winID)

	for i := range j.Wins {
		if j.Wins[i].ID == winID {
			j.Wins[i].Text = newText
			if err := s.save(j); err != nil {
				s.logger.Error("UpdateWin", "error", err, "journal_date", j.Date, "win_id", winID)
				return err
			}
			return nil
		}
	}
	err := fmt.Errorf("win not found: %s", winID)
	s.logger.Warn("UpdateWin", "error", err, "journal_date", j.Date, "win_id", winID)
	return err
}

// UpdateLogEntry updates the content of a log entry by ID
func (s *Service) UpdateLogEntry(j *Journal, logEntryID string, newContent string) error {
	s.logger.Debug("UpdateLogEntry", "journal_date", j.Date, "log_entry_id", logEntryID)

	for i := range j.LogEntries {
		if j.LogEntries[i].ID == logEntryID {
			j.LogEntries[i].Content = newContent
			if err := s.save(j); err != nil {
				s.logger.Error("UpdateLogEntry", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID)
				return err
			}
			return nil
		}
	}
	err := fmt.Errorf("log entry not found: %s", logEntryID)
	s.logger.Warn("UpdateLogEntry", "error", err, "journal_date", j.Date, "log_entry_id", logEntryID)
	return err
}

// AddScheduleItem adds a new schedule item to the journal
func (s *Service) AddScheduleItem(j *Journal, timeStr string, content string) error {
	s.logger.Debug("AddScheduleItem", "journal_date", j.Date, "time_str", timeStr)

	content = strings.TrimSpace(content)
	if content == "" {
		err := fmt.Errorf("schedule item content cannot be empty")
		s.logger.Warn("AddScheduleItem", "error", err, "journal_date", j.Date)
		return err
	}

	position := len(j.ScheduleItems)

	// Parse time if provided (HH:MM format)
	var timestamp time.Time
	if timeStr != "" {
		parsedTime, err := s.parseScheduleTime(j.Date, timeStr)
		if err != nil {
			s.logger.Error("AddScheduleItem", "error", err, "journal_date", j.Date, "time_str", timeStr)
			return fmt.Errorf("failed to parse time %s: %w", timeStr, err)
		}
		timestamp = parsedTime
	}

	item := NewScheduleItem(timestamp, content, position)
	j.ScheduleItems = append(j.ScheduleItems, *item)

	if err := s.save(j); err != nil {
		s.logger.Error("AddScheduleItem", "error", err, "journal_date", j.Date, "schedule_item_id", item.ID)
		return err
	}
	return nil
}

// reindexSchedulePositions updates position fields to match slice indices
func reindexSchedulePositions(items []ScheduleItem) {
	for i := range items {
		items[i].Position = i
	}
}

// DeleteScheduleItem removes a schedule item by ID and re-indexes positions
func (s *Service) DeleteScheduleItem(j *Journal, itemID string) error {
	s.logger.Debug("DeleteScheduleItem", "journal_date", j.Date, "schedule_item_id", itemID)

	// Find and remove the item
	found := false
	for i, item := range j.ScheduleItems {
		if item.ID == itemID {
			j.ScheduleItems = append(j.ScheduleItems[:i], j.ScheduleItems[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		err := fmt.Errorf("schedule item not found: %s", itemID)
		s.logger.Warn("DeleteScheduleItem", "error", err, "journal_date", j.Date, "schedule_item_id", itemID)
		return err
	}

	// Re-index positions
	reindexSchedulePositions(j.ScheduleItems)

	if err := s.save(j); err != nil {
		s.logger.Error("DeleteScheduleItem", "error", err, "journal_date", j.Date, "schedule_item_id", itemID)
		return err
	}
	return nil
}

// parseScheduleTime converts a date string and time string to time.Time
func (s *Service) parseScheduleTime(date string, timeStr string) (time.Time, error) {
	// Handle both HH:MM and HH:MM:SS formats
	var layout string
	if strings.Count(timeStr, ":") == 1 {
		layout = "2006-01-02 15:04"
		timeStr = date + " " + timeStr
	} else {
		layout = "2006-01-02 15:04:05"
		timeStr = date + " " + timeStr
	}

	return time.Parse(layout, timeStr)
}

// save saves a journal to both filesystem and database
func (s *Service) save(j *Journal) error {
	s.logger.Debug("save", "journal_date", j.Date, "intentions", len(j.Intentions), "log_entries", len(j.LogEntries), "wins", len(j.Wins), "schedule_items", len(j.ScheduleItems))

	// Serialize to markdown
	content := WriteJournal(j)

	// Write to filesystem
	if err := s.fileStore.WriteJournalFile(j.Date, content); err != nil {
		s.logger.Error("save", "error", err, "journal_date", j.Date, "operation", "write_file")
		return fmt.Errorf("failed to write journal file: %w", err)
	}

	// Update journal metadata
	filePath, _ := s.fileStore.GetJournalPath(j.Date)
	j.FilePath = filePath
	j.LastModified = time.Now()

	// Update database index
	if err := s.repo.SaveJournal(j); err != nil {
		s.logger.Error("save", "error", err, "journal_date", j.Date, "operation", "save_to_db")
		return fmt.Errorf("failed to save journal to database: %w", err)
	}

	return nil
}

// parseJournal parses journal content
func (s *Service) parseJournal(content string, filePath string, lastModified time.Time) (*Journal, error) {
	return ParseJournal(content, filePath, lastModified)
}

// autoCarryIntentions carries over open intentions from yesterday
func (s *Service) autoCarryIntentions(j *Journal) error {
	s.logger.Debug("autoCarryIntentions", "journal_date", j.Date)

	// Parse today's date and get yesterday
	today, err := time.Parse("2006-01-02", j.Date)
	if err != nil {
		s.logger.Error("autoCarryIntentions", "error", err, "journal_date", j.Date)
		return fmt.Errorf("invalid date format: %w", err)
	}

	yesterday := today.AddDate(0, 0, -1).Format("2006-01-02")

	// Get yesterday's open intentions from database
	openIntentions, err := s.repo.GetOpenIntentions(yesterday)
	if err != nil {
		s.logger.Error("autoCarryIntentions", "error", err, "journal_date", j.Date, "yesterday", yesterday)
		return fmt.Errorf("failed to get open intentions: %w", err)
	}

	// Carry them over
	carriedCount := 0
	for _, intention := range openIntentions {
		carried := NewCarriedIntention(intention.Text, yesterday, len(j.Intentions))
		j.Intentions = append(j.Intentions, *carried)
		carriedCount++
	}

	s.logger.Debug("autoCarryIntentions", "journal_date", j.Date, "carried_count", carriedCount)
	return nil
}

// Rebuild recreates the database index from all markdown files
func (s *Service) Rebuild() error {
	s.logger.Info("Rebuild", "operation", "start")

	// Clear all data from database
	if err := s.repo.ClearAllData(); err != nil {
		s.logger.Error("Rebuild", "error", err, "operation", "clear_db")
		return fmt.Errorf("failed to clear database: %w", err)
	}

	// Get all journal dates
	dates, err := s.fileStore.ListJournalDates()
	if err != nil {
		s.logger.Error("Rebuild", "error", err, "operation", "list_journals")
		return fmt.Errorf("failed to list journals: %w", err)
	}

	// Reindex each journal
	reindexedCount := 0
	for _, date := range dates {
		content, fileInfo, err := s.fileStore.ReadJournalFile(date)
		if err != nil {
			s.logger.Error("Rebuild", "error", err, "operation", "read_journal", "journal_date", date)
			return fmt.Errorf("failed to read journal %s: %w", date, err)
		}

		if fileInfo.Exists {
			j, err := s.parseJournal(content, fileInfo.Path, fileInfo.LastModified)
			if err != nil {
				s.logger.Error("Rebuild", "error", err, "operation", "parse_journal", "journal_date", date)
				return fmt.Errorf("failed to parse journal %s: %w", date, err)
			}

			if err := s.repo.SaveJournal(j); err != nil {
				s.logger.Error("Rebuild", "error", err, "operation", "save_journal", "journal_date", date)
				return fmt.Errorf("failed to save journal %s: %w", date, err)
			}
			reindexedCount++
		}
	}

	s.logger.Info("Rebuild", "operation", "complete", "total_journals", reindexedCount)
	return nil
}

// GetJournalContent returns the journal as markdown text
func (s *Service) GetJournalContent(date string) (string, error) {
	content, fileInfo, err := s.fileStore.ReadJournalFile(date)
	if err != nil {
		return "", fmt.Errorf("failed to read journal: %w", err)
	}

	if !fileInfo.Exists {
		return "", fmt.Errorf("journal not found for date: %s", date)
	}

	return content, nil
}

// GetWeekContent returns the last 7 days of journals as markdown
func (s *Service) GetWeekContent() (string, error) {
	var content string

	for i := 6; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")

		// Read file directly instead of parsing
		filePath, err := s.fileStore.GetJournalPath(date)
		if err != nil {
			continue
		}

		fileContent, err := os.ReadFile(filePath)
		if err != nil {
			continue // Skip missing days
		}

		content += fmt.Sprintf("# %s\n\n", date)
		content += string(fileContent) + "\n\n---\n\n"
	}

	return content, nil
}

// GetWeekJournals returns the last 7 days of journals as a slice
func (s *Service) GetWeekJournals() ([]*Journal, error) {
	journals := make([]*Journal, 0, 7)

	for i := 6; i >= 0; i-- {
		date := time.Now().AddDate(0, 0, -i).Format("2006-01-02")
		j, err := s.GetByDate(date)
		if err != nil {
			continue
		}
		journals = append(journals, j)
	}

	return journals, nil
}
