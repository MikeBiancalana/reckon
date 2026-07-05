package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/MikeBiancalana/reckon/internal/config"
	"github.com/MikeBiancalana/reckon/internal/node"
	"github.com/MikeBiancalana/reckon/internal/output"
	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────────────────────────────────────
// Flag variables (package-global so cobra can bind them; RunE resets them,
// and their pflag Changed state, via defer resetAddFlags — mirrors todo.go's
// resetTodoFlags).
// ─────────────────────────────────────────────────────────────────────────────

var (
	addAuthorFlag string
	addAtFlag     string
)

// addCmd is the graduated `rk add` capture command (v1-T4, reckon-uv09):
// appends a timestamped, authored entry to today's (or --date's) log day
// file under log/<date>.md. Does not touch the legacy DB-journal `rk log`
// (owned by a different ticket, reckon-s6oh/T9) and does not reconcile the
// index itself (capture -> explicit `rk index` -> query is the intended
// flow, matching `rk query`'s own no-auto-reconcile contract).
var addCmd = &cobra.Command{
	Use:          "add <text...>",
	Short:        "Capture a timestamped log entry into the vault",
	Long:         "Append a timestamped, authored entry to today's (or --date's) log day file under log/<date>.md.",
	Annotations:  map[string]string{"requiresDB": "false"},
	SilenceUsage: true,
	Args:         cobra.MinimumNArgs(1),
	RunE:         runAddE,
}

func init() {
	f := addCmd.Flags()
	f.StringVar(&addAuthorFlag, "author", "", "Author to record (default: $RECKON_AUTHOR, $USER, or \"local\")")
	f.StringVar(&addAtFlag, "at", "", "Entry time HH:MM, 24-hour (default: current UTC time)")
}

// resetAddFlags restores add flag variables to their defaults and clears the
// pflag Changed state on whichever of these flags are registered on cmd.
func resetAddFlags(cmd *cobra.Command) {
	addAuthorFlag = ""
	addAtFlag = ""
	for _, name := range []string{"author", "at"} {
		if fl := cmd.Flags().Lookup(name); fl != nil {
			fl.Changed = false
		}
	}
}

// embeddedHeaderRe matches a body or author value that would introduce a
// literal "## " line (EC-9, defensive; also E1, reckon-uv09 review: applied
// to the resolved author too, since a --author/$RECKON_AUTHOR/$USER value
// containing a newline sits on the same rendered header line and can inject
// a spurious "## " header just as effectively as a body value can). Args are
// space-joined so this mostly guards an embedded-newline / programmatic
// caller rather than normal shell usage.
var embeddedHeaderRe = regexp.MustCompile(`(?m)^## `)

// logAddResult is the structured summary of one `rk add` run.
type logAddResult struct {
	Path string `json:"path"` // vault-relative: "log/<date>.md"
	ID   string `json:"id"`   // the new entry's ULID
	Day  string `json:"day"`  // the day file's date, e.g. "2026-07-05"
	Time string `json:"time"` // the entry's reconstructed time, e.g. "2026-07-05T09:15:00Z"
}

func (r logAddResult) Pretty() string {
	return fmt.Sprintf("add: logged to %s (id %s, time %s)", r.Path, r.ID, r.Time)
}

func runAddE(cmd *cobra.Command, args []string) error {
	defer resetAddFlags(cmd)

	author := resolveAuthor(addAuthorFlag)
	if embeddedHeaderRe.MatchString(author) {
		return fmt.Errorf(`add: author must not contain a line starting with "## " (would be mis-split as a new entry)`)
	}
	body := strings.TrimSpace(strings.Join(args, " "))
	if body == "" {
		return fmt.Errorf("add: empty body text")
	}
	if embeddedHeaderRe.MatchString(body) {
		return fmt.Errorf(`add: body must not contain a line starting with "## " (would be mis-split as a new entry)`)
	}

	mode, err := output.ModeFromFlags(jsonFlag, ndjsonFlag)
	if err != nil {
		return err
	}

	day, err := effectiveLogDate()
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}

	hhmm, err := resolveAtTime(addAtFlag)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}

	cfg, err := config.LoadWithOverrides(vaultFlag, "")
	if err != nil {
		return fmt.Errorf("add: load config: %w", err)
	}

	logDir := filepath.Join(cfg.VaultDir, "log")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return fmt.Errorf("add: create log dir: %w", err)
	}

	res, err := appendLogEntry(logDir, day, hhmm, author, body)
	if err != nil {
		return err
	}

	if !(mode == output.Pretty && quietFlag) {
		if err := output.New(cmd.OutOrStdout(), mode).Print(res); err != nil {
			return err
		}
	}
	return nil
}

// effectiveLogDate returns the date of the log day file to write: the
// validated --date flag when the user explicitly set it (delegating to
// getEffectiveDate for its format validation), else the current UTC
// calendar date -- deliberately NOT getEffectiveDate()'s own local-clock
// default.
//
// This is the C1 fix (reckon-uv09 review): appendLogEntry composes the
// entry's `time` field as `day + "T" + hhmm + ":00Z"`, and resolveAtTime's
// default hhmm is already UTC wall-clock. If day came from
// getEffectiveDate()'s LOCAL default, the two halves of that composed
// RFC3339 value would come from two different clocks -- e.g. a Sydney user
// at local 2026-07-05 08:30 would get day="2026-07-05" (local) +
// hhmm="22:30" (UTC, since it's already 2026-07-04 22:30 UTC), producing
// the impossible instant "2026-07-05T22:30:00Z" a full day off from the
// true UTC instant "2026-07-04T22:30:00Z". Defaulting the day to UTC here
// keeps both halves on one clock.
//
// getEffectiveDate() itself is intentionally left untouched: today.go/
// week.go and the legacy journal readers rely on its local-clock semantics,
// and it still owns --date's format validation (used here on the
// explicit-flag path only).
func effectiveLogDate() (string, error) {
	if dateFlag != "" {
		return getEffectiveDate()
	}
	return time.Now().UTC().Format("2006-01-02"), nil
}

// resolveAtTime validates and returns the HH:MM string for the new entry:
// --at if given (validated 24-hour HH:MM), else the current UTC wall-clock
// time (plan.md Decision 4: --at backfills the header time, never the
// ULID's own mint instant).
func resolveAtTime(at string) (string, error) {
	if at == "" {
		return time.Now().UTC().Format("15:04"), nil
	}
	t, err := time.Parse("15:04", at)
	if err != nil {
		return "", fmt.Errorf("invalid --at value %q (want HH:MM, 24-hour): %w", at, err)
	}
	return t.Format("15:04"), nil
}

// appendLogEntry creates log/<day>.md via the NewNode -> Render -> Parse ->
// writeFileAtomic recipe (plan.md D1/D9, mirroring addDurableTodo) if
// absent, or appends a node.RenderLogEntry block strictly at EOF if present.
// The ULID mints at real wall-clock node.Mint() regardless of --at.
func appendLogEntry(logDir, day, hhmm, author, body string) (logAddResult, error) {
	path := filepath.Join(logDir, day+".md")
	id := node.Mint()
	entryTime := day + "T" + hhmm + ":00Z"
	block := node.RenderLogEntry(hhmm, author, id, body)
	relPath := "log/" + day + ".md"

	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		n := node.NewNode("log-day", "", "# "+day+"\n\n"+block)
		n.Aliases = []string{day}

		rendered := n.Render()
		parsed, perr := node.Parse(rendered)
		if perr != nil {
			return logAddResult{}, fmt.Errorf("add: parse rendered day file: %w", perr)
		}
		if err := writeFileAtomic(path, parsed.Serialize()); err != nil {
			return logAddResult{}, fmt.Errorf("add: write: %w", err)
		}
		return logAddResult{Path: relPath, ID: id, Day: day, Time: entryTime}, nil
	}
	if err != nil {
		return logAddResult{}, fmt.Errorf("add: read %s: %w", path, err)
	}

	if bytes.Contains(raw, []byte("\r\n")) {
		return logAddResult{}, fmt.Errorf("add: CRLF line endings are not supported (reckon-vj55): %s", path)
	}

	suffix := "\n" + block
	appended := make([]byte, 0, len(raw)+len(suffix))
	appended = append(appended, raw...)
	appended = append(appended, suffix...)

	if _, perr := node.Parse(appended); perr != nil {
		return logAddResult{}, fmt.Errorf("add: parse appended day file: %w", perr)
	}

	if err := writeFileAtomic(path, appended); err != nil {
		return logAddResult{}, fmt.Errorf("add: write: %w", err)
	}
	return logAddResult{Path: relPath, ID: id, Day: day, Time: entryTime}, nil
}
