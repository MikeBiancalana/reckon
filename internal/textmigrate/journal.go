package textmigrate

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/journal"
	"github.com/MikeBiancalana/reckon/internal/node"
)

// taskRefRe matches the legacy inline task-reference marker [task:<xid>];
// [meeting:]/[break] markers have no migration target and are left verbatim.
var taskRefRe = regexp.MustCompile(`\[task:([^\]]+)\]`)

// convertJournalDay builds the log-day node.Node for one legacy Journal
// (already parsed from a legacy day file). preamble reports how many
// Intentions/Wins/Schedule entries were preserved in the day's preamble
// block (they have no direct log-entry analog). rewrittenRefs counts inline
// [task:<xid>] markers rewritten to [[<xid>]] wikilinks within Log entry
// content, so the index derives a reference edge to the migrated todo.
func convertJournalDay(j *journal.Journal) (n *node.Node, preamble JournalPreambleCounts, rewrittenRefs int, err error) {
	preamble = JournalPreambleCounts{
		Intentions: len(j.Intentions),
		Wins:       len(j.Wins),
		Schedule:   len(j.ScheduleItems),
	}

	var body strings.Builder
	body.WriteString("# " + j.Date + "\n\n")
	writeIntentionsPreamble(&body, j.Intentions)
	writeWinsPreamble(&body, j.Wins)
	writeSchedulePreamble(&body, j.ScheduleItems)

	entries := append([]journal.LogEntry(nil), j.LogEntries...)
	sort.Slice(entries, func(i, k int) bool { return entries[i].Position < entries[k].Position })
	for _, e := range entries {
		content, count := rewriteTaskRefs(e.Content)
		rewrittenRefs += count
		hhmm := e.Timestamp.Format("15:04")
		id := node.MintAt(e.Timestamp)
		body.WriteString(node.RenderLogEntry(hhmm, legacyAuthor, id, content))
	}

	mintAt, perr := time.Parse("2006-01-02", j.Date)
	if perr != nil {
		mintAt = time.Now().UTC()
	}

	n = node.NewNode("log-day", legacyAuthor, body.String())
	n.ULID = node.MintAt(mintAt)
	n.Time = mintAt.UTC().Format(time.RFC3339)
	n.Aliases = []string{j.Date}

	return n, preamble, rewrittenRefs, nil
}

// writeIntentionsPreamble renders j.Intentions as an "### Intentions" H3
// block (never "## ", so LogParser's "^## " entry-splitter does not mis-split
// it into a phantom log entry). No header is written when there are no
// intentions.
func writeIntentionsPreamble(body *strings.Builder, intentions []journal.Intention) {
	if len(intentions) == 0 {
		return
	}
	sorted := append([]journal.Intention(nil), intentions...)
	sort.Slice(sorted, func(i, k int) bool { return sorted[i].Position < sorted[k].Position })

	body.WriteString("### Intentions\n")
	for _, it := range sorted {
		mark := "[ ]"
		switch it.Status {
		case journal.IntentionDone:
			mark = "[x]"
		case journal.IntentionCarried:
			mark = "[>]"
		}
		text := it.Text
		if it.CarriedFrom != "" {
			text += " (carried from " + it.CarriedFrom + ")"
		}
		body.WriteString("- " + mark + " " + text + "\n")
	}
	body.WriteString("\n")
}

// writeWinsPreamble renders j.Wins as an "### Wins" H3 block.
func writeWinsPreamble(body *strings.Builder, wins []journal.Win) {
	if len(wins) == 0 {
		return
	}
	sorted := append([]journal.Win(nil), wins...)
	sort.Slice(sorted, func(i, k int) bool { return sorted[i].Position < sorted[k].Position })

	body.WriteString("### Wins\n")
	for _, w := range sorted {
		body.WriteString("- " + w.Text + "\n")
	}
	body.WriteString("\n")
}

// writeSchedulePreamble renders j.ScheduleItems as an "### Schedule" H3
// block.
func writeSchedulePreamble(body *strings.Builder, items []journal.ScheduleItem) {
	if len(items) == 0 {
		return
	}
	sorted := append([]journal.ScheduleItem(nil), items...)
	sort.Slice(sorted, func(i, k int) bool { return sorted[i].Position < sorted[k].Position })

	body.WriteString("### Schedule\n")
	for _, it := range sorted {
		if !it.Time.IsZero() {
			body.WriteString("- " + it.Time.Format("15:04") + " " + it.Content + "\n")
		} else {
			body.WriteString("- " + it.Content + "\n")
		}
	}
	body.WriteString("\n")
}

// rewriteTaskRefs rewrites every inline [task:<xid>] marker in content to a
// [[<xid>]] wikilink so the index derives a reference edge to the migrated
// todo via its xid alias, returning the rewritten content and how many
// markers were rewritten.
func rewriteTaskRefs(content string) (string, int) {
	count := 0
	rewritten := taskRefRe.ReplaceAllStringFunc(content, func(m string) string {
		count++
		sub := taskRefRe.FindStringSubmatch(m)
		return "[[" + sub[1] + "]]"
	})
	return rewritten, count
}

// runJournal migrates every legacy journal day file into log/<date>.md.
// Idempotency is per-day-file: an existing log/<date>.md is skipped
// outright, since a day file is written atomically as one unit and is
// therefore either fully present or fully absent, never half-migrated.
func (imp *Importer) runJournal(report *Report) error {
	journalDir := filepath.Join(imp.Source, "journal")
	entries, err := os.ReadDir(journalDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read legacy journal dir: %w", err)
	}

	logDir := filepath.Join(imp.Dest, "log")

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		date := strings.TrimSuffix(e.Name(), ".md")

		destPath := filepath.Join(logDir, date+".md")
		if _, err := os.Stat(destPath); err == nil {
			report.JournalDays.addSkipped(date, "already migrated (day file present)")
			continue
		} else if !os.IsNotExist(err) {
			report.JournalDays.addErrored(date, fmt.Errorf("stat %s: %w", destPath, err))
			continue
		}

		legacyPath := filepath.Join(journalDir, e.Name())
		raw, err := os.ReadFile(legacyPath)
		if err != nil {
			report.JournalDays.addErrored(date, fmt.Errorf("read %s: %w", legacyPath, err))
			continue
		}
		if bytes.Contains(raw, []byte("\r\n")) {
			report.JournalDays.addErrored(date, fmt.Errorf("CRLF line endings are not supported"))
			continue
		}

		mtime := time.Time{}
		if info, err := e.Info(); err == nil {
			mtime = info.ModTime()
		}

		j, err := journal.ParseJournal(string(raw), legacyPath, mtime)
		if err != nil {
			report.JournalDays.addErrored(date, fmt.Errorf("parse %s: %w", legacyPath, err))
			continue
		}
		j.Date = date // filename is the authoritative date, not the (possibly malformed/absent) frontmatter

		built, preamble, rewritten, err := convertJournalDay(j)
		if err != nil {
			report.JournalDays.addErrored(date, err)
			continue
		}

		parsed, err := renderAndParse(built)
		if err != nil {
			report.JournalDays.addErrored(date, fmt.Errorf("journal day %s: %w", date, err))
			continue
		}

		relPath := "log/" + date + ".md"
		if imp.DryRun {
			report.JournalDays.addCreated(date, relPath, parsed.ULID)
			report.JournalPreamble.Intentions += preamble.Intentions
			report.JournalPreamble.Wins += preamble.Wins
			report.JournalPreamble.Schedule += preamble.Schedule
			report.RewrittenTaskRefs += rewritten
			continue
		}

		if err := os.MkdirAll(logDir, 0o755); err != nil {
			report.JournalDays.addErrored(date, fmt.Errorf("create log dir: %w", err))
			continue
		}
		if err := writeFileAtomic(destPath, parsed.Serialize()); err != nil {
			report.JournalDays.addErrored(date, fmt.Errorf("write %s: %w", relPath, err))
			continue
		}

		report.JournalDays.addCreated(date, relPath, parsed.ULID)
		report.JournalPreamble.Intentions += preamble.Intentions
		report.JournalPreamble.Wins += preamble.Wins
		report.JournalPreamble.Schedule += preamble.Schedule
		report.RewrittenTaskRefs += rewritten
	}

	return nil
}
