# Reckon

A terminal-based productivity system combining daily journaling, task management, and time tracking. Reckon uses plain markdown files as your source of truth, making your daily workflow portable, searchable, and version-controllable.

## Features

- **Interactive TUI**: Beautiful terminal interface for managing your day
- **Plain Text Storage**: Markdown-based journal format that's human-readable and git-friendly
- **Daily Intentions**: Track what you plan to accomplish each day
- **Time Logging**: Automatic timestamping with duration tracking for tasks, meetings, and breaks
- **Wins Tracking**: Celebrate your daily accomplishments
- **Time Analytics**: View time breakdowns by day or week
- **CLI Integration**: Quick commands for logging on the go
- **SQLite Database**: Fast querying and aggregation of your journal data

## Installation

### From Source

Requires Go 1.25.5 or later:

```bash
git clone https://github.com/MikeBiancalana/reckon.git
cd reckon
go build -o rk ./cmd/rk
```

Move the binary to your PATH:

```bash
sudo mv rk /usr/local/bin/
```

## Usage

### Interactive TUI

Launch the terminal UI by simply running:

```bash
rk
```

The TUI provides an interactive interface with a modern three-column layout:

**Left Column (40%)**: Activity log with timestamps showing what you're working on throughout the day

**Center Column (40%)**: General task list with support for collapsible notes

**Right Column (18%)**: Stacked sections for:
- **Schedule**: Upcoming items for the day
- **Intentions**: 1-3 focus tasks for today
- **Wins**: Daily accomplishments

#### Key Bindings

**Navigation:**
- `h/←`, `l/→` - Previous/next day
- `T` - Jump to today
- `tab/shift+tab` - Cycle through sections
- `j/k` - Navigate within section

**Actions:**
- `t` - Add task
- `i` - Add intention
- `w` - Add win
- `L` - Add log entry
- `space` - Toggle task completion (Tasks section)
- `enter` - Toggle intention / Expand task (Intentions/Tasks section)
- `d` - Delete item (with confirmation)
- `?` - Toggle help
- `q` - Quit

### CLI Commands

#### Quick Logging

Add a timestamped entry to today's journal:

```bash
rk log Started working on the new feature
rk log [meeting:standup] 30m - Daily standup discussion
rk log [break] 15m - Coffee break
rk log [task:beads-123] Implemented user authentication
```

#### View Today's Journal

Output today's journal content:

```bash
rk today
```

This is particularly useful for piping into other tools or LLMs for analysis.

#### Time Summaries

View your time breakdown for today:

```bash
rk summary
```

View your time breakdown for the week:

```bash
rk summary --week
```

#### Rebuild Database

Rebuild the database from your markdown files:

```bash
rk rebuild
```

## Journal Format

Reckon uses a simple markdown format with YAML frontmatter. Journals are stored as markdown files in your journal directory.

### Example Journal Entry

```markdown
---
date: 2025-12-22
---

## Intentions

- [ ] Implement user authentication
- [x] Fix bug in checkout flow
- [>] Review pull requests (carried from 2025-12-21)

## Log

- 09:00 Started day, reviewing priorities
- 09:30 [meeting:standup] 15m - Daily team sync
- 10:00 [task:beads-456] Working on authentication flow 2h
- 12:00 [break] 30m - Lunch break
- 14:30 Fixed checkout bug, testing locally
- 16:00 Code review session 1h30m

## Wins

- Completed authentication implementation
- Fixed critical checkout bug
- Helped teammate with debugging
```

### Intention Statuses

- `[ ]` - Open/pending task
- `[x]` - Completed task
- `[>]` - Carried forward to another day

### Log Entry Formats

- **Basic log**: `- HH:MM Message`
- **With duration**: `- HH:MM Message 30m` or `- HH:MM Message 1h30m`
- **Task reference**: `- HH:MM [task:id] Working on feature`
- **Meeting**: `- HH:MM [meeting:name] 45m - Meeting description`
- **Break**: `- HH:MM [break] 15m - Break description`

### Time Tracking

Durations can be specified as:
- `30m` - 30 minutes
- `2h` - 2 hours
- `1h30m` - 1 hour 30 minutes

The system automatically categorizes entries:
- Regular work logs (default)
- Meetings (when using `[meeting:name]`)
- Breaks (when using `[break]`)

## Configuration

Reckon stores its data in:
- **Database**: `~/.config/reckon/reckon.db` (SQLite)
- **Journal files**: User-configured location (markdown files)

## Development

### Build

```bash
go build -o rk ./cmd/rk
```

### Test

Run all tests:

```bash
go test ./...
```

Run specific test:

```bash
go test -run TestName ./path/to/package
```

### Code Quality

Format code:

```bash
go fmt ./...
```

Lint code:

```bash
go vet ./...
```

Tidy dependencies:

```bash
go mod tidy
```

### Code Style

- **Formatting**: Use `go fmt` (standard Go formatting)
- **Imports**: stdlib → third-party → internal packages (blank line between groups)
- **Naming**: PascalCase for exported, camelCase for unexported
- **Errors**: Return errors, wrap with `fmt.Errorf("context: %w", err)`
- **Comments**: Document all exported functions/types

## Architecture

```
reckon/
├── cmd/rk/              # Main application entry point
├── internal/
│   ├── cli/             # CLI commands (log, today, week, etc.)
│   ├── config/          # Configuration management
│   ├── journal/         # Journal models, parsing, and business logic
│   ├── storage/         # Database and filesystem operations
│   └── tui/             # Terminal UI components (Bubble Tea)
│       └── components/  # Reusable TUI components
└── docs/                # Documentation
```

### Key Technologies

- **[Bubble Tea](https://github.com/charmbracelet/bubbletea)**: Terminal UI framework
- **[Cobra](https://github.com/spf13/cobra)**: CLI framework
- **[Lipgloss](https://github.com/charmbracelet/lipgloss)**: Terminal styling
- **[SQLite](https://modernc.org/sqlite)**: Local database storage

## Why Reckon?

Reckon is designed for developers and knowledge workers who:
- Prefer working in the terminal
- Want their productivity data in plain text (git-friendly, searchable)
- Need both structured task tracking AND flexible time logging
- Like to reflect on daily wins and accomplishments
- Want to analyze their time usage patterns

Unlike heavyweight project management tools, Reckon stays out of your way while providing the structure you need to stay organized and focused.

## License

See [LICENSE](LICENSE) file for details.

## Contributing

Contributions are welcome! Please feel free to submit issues and pull requests.

---

Built with ❤️ for terminal enthusiasts who believe productivity tools should be fast, simple, and respect your data.
