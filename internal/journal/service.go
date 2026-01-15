package journal

import (
	"fmt"
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
}

// NewService creates a new journal service
func NewService(repo *Repository, fileStore *storage.FileStore) *Service {
	return &Service{
		repo:      repo,
		fileStore: fileStore,
	}
}

// GetToday returns today's journal, creating it if it doesn't exist
func (s *Service) GetToday() (*Journal, error) {
	today := time.Now().Format("2006-01-02")
	return s.GetByDate(today)
}

// GetByDate returns a journal for the given date, creating it if it doesn't exist
func (s *Service) GetByDate(date string) (*Journal, error) {
	// Try to read from filesystem first
	content, fileInfo, err := s.fileStore.ReadJournalFile(date)
	if err != nil {
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
				return nil, fmt.Errorf("failed to auto-carry intentions: %w", err)
			}
		}

		// Save the new journal
		if err := s.save(j); err != nil {
			return nil, fmt.Errorf("failed to save new journal: %w", err)
		}
	} else {
		// Parse the journal content
		j, err = s.parseJournal(content, fileInfo.Path, fileInfo.LastModified)
		if err != nil {
			return nil, fmt.Errorf("failed to parse journal: %w", err)
		}

		// Load schedule items from database (database is source of truth for schedule items)
		scheduleItems, err := s.repo.GetScheduleItems(date)
		if err != nil {
			return nil, fmt.Errorf("failed to load schedule items from database: %w", err)
		}
		j.ScheduleItems = scheduleItems
	}

	return j, nil
}

// AppendLog appends a log entry to the journal
func (s *Service) AppendLog(j *Journal, content string) error {
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

	return s.save(j)
}

// AddIntention adds a new intention to the journal
func (s *Service) AddIntention(j *Journal, text string) error {
	position := len(j.Intentions)
	intention := NewIntention(text, position)
	j.Intentions = append(j.Intentions, *intention)

	return s.save(j)
}

// ToggleIntention toggles an intention between open and done
func (s *Service) ToggleIntention(j *Journal, intentionID string) error {
	for i := range j.Intentions {
		if j.Intentions[i].ID == intentionID {
			if j.Intentions[i].Status == IntentionDone {
				j.Intentions[i].Status = IntentionOpen
			} else {
				j.Intentions[i].Status = IntentionDone
			}
			return s.save(j)
		}
	}

	return fmt.Errorf("intention not found: %s", intentionID)
}

// AddWin adds a new win to the journal
func (s *Service) AddWin(j *Journal, text string) error {
	position := len(j.Wins)
	win := NewWin(text, position)
	j.Wins = append(j.Wins, *win)

	return s.save(j)
}

// DeleteIntention removes an intention by ID and re-indexes positions
func (s *Service) DeleteIntention(j *Journal, intentionID string) error {
	found := false
	for i, intention := range j.Intentions {
		if intention.ID == intentionID {
			j.Intentions = append(j.Intentions[:i], j.Intentions[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("intention not found: %s", intentionID)
	}

	// Re-index positions
	for i := range j.Intentions {
		j.Intentions[i].Position = i
	}

	return s.save(j)
}

// DeleteWin removes a win by ID and re-indexes positions
func (s *Service) DeleteWin(j *Journal, winID string) error {
	found := false
	for i, win := range j.Wins {
		if win.ID == winID {
			j.Wins = append(j.Wins[:i], j.Wins[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("win not found: %s", winID)
	}

	// Re-index positions
	for i := range j.Wins {
		j.Wins[i].Position = i
	}

	return s.save(j)
}

// DeleteLogEntry removes a log entry by ID and re-indexes positions
func (s *Service) DeleteLogEntry(j *Journal, logEntryID string) error {
	found := false
	for i, entry := range j.LogEntries {
		if entry.ID == logEntryID {
			j.LogEntries = append(j.LogEntries[:i], j.LogEntries[i+1:]...)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("log entry not found: %s", logEntryID)
	}

	// Re-index positions
	for i := range j.LogEntries {
		j.LogEntries[i].Position = i
	}

	return s.save(j)
}

// AddLogNote adds a note to a log entry
func (s *Service) AddLogNote(j *Journal, logEntryID string, text string) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("note text cannot be empty")
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
		return fmt.Errorf("log entry not found: %s", logEntryID)
	}

	// Create new note
	position := len(targetEntry.Notes)
	note := NewLogNote(text, position)
	targetEntry.Notes = append(targetEntry.Notes, *note)

	return s.save(j)
}

// UpdateLogNote updates the text of a note in a log entry
func (s *Service) UpdateLogNote(j *Journal, logEntryID string, noteID string, newText string) error {
	newText = strings.TrimSpace(newText)
	if newText == "" {
		return fmt.Errorf("note text cannot be empty")
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
		return fmt.Errorf("log entry not found: %s", logEntryID)
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
		return fmt.Errorf("note not found: %s", noteID)
	}

	return s.save(j)
}

// DeleteLogNote removes a note from a log entry
func (s *Service) DeleteLogNote(j *Journal, logEntryID string, noteID string) error {
	// Find the log entry
	var targetEntry *LogEntry
	for i := range j.LogEntries {
		if j.LogEntries[i].ID == logEntryID {
			targetEntry = &j.LogEntries[i]
			break
		}
	}

	if targetEntry == nil {
		return fmt.Errorf("log entry not found: %s", logEntryID)
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
		return fmt.Errorf("note not found: %s", noteID)
	}

	// Re-index positions
	for i := range targetEntry.Notes {
		targetEntry.Notes[i].Position = i
	}

	return s.save(j)
}

// UpdateIntention updates the text of an intention by ID
func (s *Service) UpdateIntention(j *Journal, intentionID string, newText string) error {
	for i := range j.Intentions {
		if j.Intentions[i].ID == intentionID {
			j.Intentions[i].Text = newText
			return s.save(j)
		}
	}
	return fmt.Errorf("intention not found: %s", intentionID)
}

// UpdateWin updates the text of a win by ID
func (s *Service) UpdateWin(j *Journal, winID string, newText string) error {
	for i := range j.Wins {
		if j.Wins[i].ID == winID {
			j.Wins[i].Text = newText
			return s.save(j)
		}
	}
	return fmt.Errorf("win not found: %s", winID)
}

// UpdateLogEntry updates the content of a log entry by ID
func (s *Service) UpdateLogEntry(j *Journal, logEntryID string, newContent string) error {
	for i := range j.LogEntries {
		if j.LogEntries[i].ID == logEntryID {
			j.LogEntries[i].Content = newContent
			return s.save(j)
		}
	}
	return fmt.Errorf("log entry not found: %s", logEntryID)
}

// AddScheduleItem adds a new schedule item to the journal
func (s *Service) AddScheduleItem(j *Journal, timeStr string, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("schedule item content cannot be empty")
	}

	position := len(j.ScheduleItems)

	// Parse time if provided (HH:MM format)
	var timestamp time.Time
	if timeStr != "" {
		parsedTime, err := s.parseScheduleTime(j.Date, timeStr)
		if err != nil {
			return fmt.Errorf("failed to parse time %s: %w", timeStr, err)
		}
		timestamp = parsedTime
	}

	item := NewScheduleItem(timestamp, content, position)
	j.ScheduleItems = append(j.ScheduleItems, *item)

	return s.save(j)
}

// reindexSchedulePositions updates position fields to match slice indices
func reindexSchedulePositions(items []ScheduleItem) {
	for i := range items {
		items[i].Position = i
	}
}

// DeleteScheduleItem removes a schedule item by ID and re-indexes positions
func (s *Service) DeleteScheduleItem(j *Journal, itemID string) error {
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
		return fmt.Errorf("schedule item not found: %s", itemID)
	}

	// Re-index positions
	reindexSchedulePositions(j.ScheduleItems)

	return s.save(j)
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
	// Serialize to markdown
	content := WriteJournal(j)

	// Write to filesystem
	if err := s.fileStore.WriteJournalFile(j.Date, content); err != nil {
		return fmt.Errorf("failed to write journal file: %w", err)
	}

	// Update journal metadata
	filePath, _ := s.fileStore.GetJournalPath(j.Date)
	j.FilePath = filePath
	j.LastModified = time.Now()

	// Update database index
	if err := s.repo.SaveJournal(j); err != nil {
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
	// Parse today's date and get yesterday
	today, err := time.Parse("2006-01-02", j.Date)
	if err != nil {
		return fmt.Errorf("invalid date format: %w", err)
	}

	yesterday := today.AddDate(0, 0, -1).Format("2006-01-02")

	// Get yesterday's open intentions from database
	openIntentions, err := s.repo.GetOpenIntentions(yesterday)
	if err != nil {
		return fmt.Errorf("failed to get open intentions: %w", err)
	}

	// Carry them over
	for _, intention := range openIntentions {
		carried := NewCarriedIntention(intention.Text, yesterday, len(j.Intentions))
		j.Intentions = append(j.Intentions, *carried)
	}

	return nil
}

// Rebuild recreates the database index from all markdown files
func (s *Service) Rebuild() error {
	// Clear all data from database
	if err := s.repo.ClearAllData(); err != nil {
		return fmt.Errorf("failed to clear database: %w", err)
	}

	// Get all journal dates
	dates, err := s.fileStore.ListJournalDates()
	if err != nil {
		return fmt.Errorf("failed to list journals: %w", err)
	}

	// Reindex each journal
	for _, date := range dates {
		content, fileInfo, err := s.fileStore.ReadJournalFile(date)
		if err != nil {
			return fmt.Errorf("failed to read journal %s: %w", date, err)
		}

		if fileInfo.Exists {
			j, err := s.parseJournal(content, fileInfo.Path, fileInfo.LastModified)
			if err != nil {
				return fmt.Errorf("failed to parse journal %s: %w", date, err)
			}

			if err := s.repo.SaveJournal(j); err != nil {
				return fmt.Errorf("failed to save journal %s: %w", date, err)
			}
		}
	}

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
