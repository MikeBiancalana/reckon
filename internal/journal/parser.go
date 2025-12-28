package journal

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	// Frontmatter
	frontmatterStartRe = regexp.MustCompile(`^---\s*$`)
	frontmatterDateRe  = regexp.MustCompile(`^date:\s*(.+)$`)

	// Sections
	sectionHeaderRe = regexp.MustCompile(`^##\s+(.+)$`)

	// Intentions
	intentionOpenRe    = regexp.MustCompile(`^-\s+\[\s+\]\s+(.+)$`)
	intentionDoneRe    = regexp.MustCompile(`^-\s+\[x\]\s+(.+)$`)
	intentionCarriedRe = regexp.MustCompile(`^-\s+\[>\]\s+(.+)$`)

	// Log entries - matches "- HH:MM ..." or "- HH:MM:SS ..."
	logEntryRe = regexp.MustCompile(`^-\s+(\d{1,2}:\d{2}(?::\d{2})?)\s+(.+)$`)

	// Task reference - [task:id]
	taskRefRe = regexp.MustCompile(`\[task:([^\]]+)\]`)

	// Meeting reference - [meeting:name]
	meetingRefRe = regexp.MustCompile(`\[meeting:([^\]]+)\]`)

	// Break reference - [break]
	breakRefRe = regexp.MustCompile(`\[break\]`)

	// Duration - Xm or XhYm
	durationRe = regexp.MustCompile(`(\d+)h?(\d+)?m?`)

	// Wins (just bullet points under Wins section)
	winRe = regexp.MustCompile(`^-\s+(.+)$`)

	// Schedule items - matches "- HH:MM Content" or "- Content" (time optional)
	scheduleItemRe = regexp.MustCompile(`^-\s+(\d{1,2}:\d{2})?\s*(.+)$`)
)

type Section string

const (
	SectionNone       Section = "none"
	SectionIntentions Section = "intentions"
	SectionWins       Section = "wins"
	SectionLog        Section = "log"
	SectionSchedule   Section = "schedule"
)

// ParseJournal parses a markdown journal file and returns a Journal object
func ParseJournal(content string, filePath string, lastModified time.Time) (*Journal, error) {
	j := &Journal{
		FilePath:      filePath,
		LastModified:  lastModified,
		Intentions:    make([]Intention, 0),
		Wins:          make([]Win, 0),
		LogEntries:    make([]LogEntry, 0),
		ScheduleItems: make([]ScheduleItem, 0),
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	currentSection := SectionNone
	inFrontmatter := false
	intentionPos := 0
	winPos := 0
	logPos := 0
	schedulePos := 0

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Handle frontmatter
		if frontmatterStartRe.MatchString(trimmed) {
			inFrontmatter = !inFrontmatter
			continue
		}

		if inFrontmatter {
			if match := frontmatterDateRe.FindStringSubmatch(trimmed); match != nil {
				j.Date = strings.TrimSpace(match[1])
			}
			continue
		}

		// Handle section headers
		if match := sectionHeaderRe.FindStringSubmatch(trimmed); match != nil {
			sectionName := strings.ToLower(strings.TrimSpace(match[1]))
			switch sectionName {
			case "intentions":
				currentSection = SectionIntentions
			case "wins":
				currentSection = SectionWins
			case "log":
				currentSection = SectionLog
			case "schedule":
				currentSection = SectionSchedule
			default:
				currentSection = SectionNone
			}
			continue
		}

		// Skip empty lines
		if trimmed == "" {
			continue
		}

		// Parse content based on current section
		switch currentSection {
		case SectionIntentions:
			if intention := parseIntention(trimmed, intentionPos); intention != nil {
				j.Intentions = append(j.Intentions, *intention)
				intentionPos++
			}

		case SectionWins:
			if match := winRe.FindStringSubmatch(trimmed); match != nil {
				win := NewWin(strings.TrimSpace(match[1]), winPos)
				j.Wins = append(j.Wins, *win)
				winPos++
			}

		case SectionLog:
			if entry := parseLogEntry(trimmed, j.Date, logPos); entry != nil {
				j.LogEntries = append(j.LogEntries, *entry)
				logPos++
			}

		case SectionSchedule:
			if item := parseScheduleItem(trimmed, schedulePos); item != nil {
				j.ScheduleItems = append(j.ScheduleItems, *item)
				schedulePos++
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning journal: %w", err)
	}

	return j, nil
}

// parseIntention parses an intention line and returns an Intention
func parseIntention(line string, position int) *Intention {
	// Check for open intention [ ]
	if match := intentionOpenRe.FindStringSubmatch(line); match != nil {
		intention := NewIntention(strings.TrimSpace(match[1]), position)
		intention.Status = IntentionOpen
		return intention
	}

	// Check for done intention [x]
	if match := intentionDoneRe.FindStringSubmatch(line); match != nil {
		intention := NewIntention(strings.TrimSpace(match[1]), position)
		intention.Status = IntentionDone
		return intention
	}

	// Check for carried intention [>]
	if match := intentionCarriedRe.FindStringSubmatch(line); match != nil {
		fullText := strings.TrimSpace(match[1])
		// Extract carried date if present in format "(carried from YYYY-MM-DD)"
		text := fullText
		carriedFrom := ""
		if idx := strings.Index(fullText, " (carried from "); idx != -1 {
			text = strings.TrimSpace(fullText[:idx])
			carriedFrom = strings.Trim(strings.TrimSpace(fullText[idx+len(" (carried from "):]), ")")
		}
		intention := NewCarriedIntention(text, carriedFrom, position)
		return intention
	}

	return nil
}

// parseLogEntry parses a log entry line and returns a LogEntry
func parseLogEntry(line string, date string, position int) *LogEntry {
	match := logEntryRe.FindStringSubmatch(line)
	if match == nil {
		return nil
	}

	timeStr := match[1]
	content := strings.TrimSpace(match[2])

	// Parse timestamp
	timestamp, err := parseTime(date, timeStr)
	if err != nil {
		return nil
	}

	entry := NewLogEntry(timestamp, content, EntryTypeLog, position)

	// Check for task reference [task:id]
	if taskMatch := taskRefRe.FindStringSubmatch(content); taskMatch != nil {
		entry.TaskID = taskMatch[1]
	}

	// Check for meeting reference [meeting:name]
	if meetingMatch := meetingRefRe.FindStringSubmatch(content); meetingMatch != nil {
		entry.EntryType = EntryTypeMeeting
	}

	// Check for break reference [break]
	if breakRefRe.MatchString(content) {
		entry.EntryType = EntryTypeBreak
	}

	// Parse duration if present (e.g., "30m" or "1h30m")
	entry.DurationMinutes = parseDuration(content)

	return entry
}

// parseTime converts a date string and time string to time.Time
func parseTime(date string, timeStr string) (time.Time, error) {
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

// parseDuration extracts duration in minutes from content (e.g., "30m" or "1h30m")
func parseDuration(content string) int {
	// Look for patterns like "30m" or "1h30m" or "2h"
	patterns := []string{
		`(\d+)h(\d+)m`, // 1h30m
		`(\d+)h`,       // 2h
		`(\d+)m`,       // 30m
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		if match := re.FindStringSubmatch(content); match != nil {
			if len(match) == 3 {
				// Format: XhYm
				hours, _ := strconv.Atoi(match[1])
				minutes, _ := strconv.Atoi(match[2])
				return hours*60 + minutes
			} else if len(match) == 2 {
				if strings.HasSuffix(match[0], "h") {
					// Format: Xh
					hours, _ := strconv.Atoi(match[1])
					return hours * 60
				} else {
					// Format: Xm
					minutes, _ := strconv.Atoi(match[1])
					return minutes
				}
			}
		}
	}

	return 0
}

// parseScheduleItem parses a schedule item line and returns a ScheduleItem
func parseScheduleItem(line string, position int) *ScheduleItem {
	match := scheduleItemRe.FindStringSubmatch(line)
	if match == nil {
		return nil
	}

	var timeStr string
	var content string

	if match[1] != "" {
		// Format: "- HH:MM Content"
		timeStr = match[1]
		content = strings.TrimSpace(match[2])
	} else {
		// Format: "- Content" (no time)
		timeStr = ""
		content = strings.TrimSpace(match[2])
	}

	// Parse time if present
	var scheduleTime time.Time
	if timeStr != "" {
		// Parse time with journal date
		parsedTime, err := parseTime("", timeStr)
		if err == nil {
			scheduleTime = parsedTime
		}
		// If time parsing fails, leave as zero time
	}

	item := NewScheduleItem(scheduleTime, content, position)
	return item
}
