# CLI Subsystem Guide

## Overview

The CLI is built with [Cobra](https://github.com/spf13/cobra), providing subcommands for quick journaling and task management from the terminal.

**Key files:**
- `root.go` - Root command and initialization
- `task.go` - Task management commands
- `notes.go` - Zettelkasten notes commands
- `log.go` - Journal log entry commands
- `schedule.go` - Schedule management
- `win.go` - Wins management
- `today.go` - Output today's journal
- `week.go` - Output last 7 days
- `summary.go` - Time tracking summaries
- `review.go` - Stale task review

## Command Structure

### Current Verb Inconsistency (Being Fixed)

**Current state (inconsistent):**
- `task`: new, list, show, edit, delete, done, schedule, deadline, log, note
- `notes`: create, edit, show
- `log`: add, delete, note
- `schedule`: add, list, delete
- `win`: add, list, delete

**Target state (reckon-89hp):**
All resources should use: **add, list, show, edit, delete**

### Common Command Pattern

```go
var commandCmd = &cobra.Command{
    Use:   "command [args]",
    Short: "Short description",
    Long:  "Long description with examples",
    Args:  cobra.ExactArgs(1),  // Or MinimumNArgs, etc.
    RunE:  func(cmd *cobra.Command, args []string) error {
        // 1. Parse arguments
        // 2. Call service layer
        // 3. Format output
        // 4. Handle --quiet flag
        return nil
    },
}
```

## Service Initialization

### Current Pattern (Package Globals - Anti-pattern)

```go
// root.go
var (
    service     *journal.Service      // ❌ Package-level global
    taskService *journal.TaskService  // ❌ Package-level global
    notesService *service.NotesService // ❌ Package-level global
)

func initService() error {
    // Initialize globals
    service = journal.NewService(...)
    taskService = journal.NewTaskService(...)
    notesService = service.NewNotesService(...)
    return nil
}
```

**Problems:**
- Makes testing difficult (shared state)
- Not thread-safe
- Hidden dependencies

**TODO (future refactor):** Use dependency injection instead

### Database and Config

```go
// Typical init flow in root.go
func initService() error {
    // 1. Load config
    cfg, err := config.LoadConfig()

    // 2. Initialize database
    db, err := storage.NewDatabase(cfg.DatabasePath)

    // 3. Create repositories
    repo := journal.NewRepository(db)

    // 4. Create services
    service = journal.NewService(repo, fileStore)

    return nil
}
```

## Flags

### Global Flags (Available to All Commands)

Defined in `root.go`:
```go
rootCmd.PersistentFlags().StringVar(&dateFlag, "date", "",
    "Date to operate on in YYYY-MM-DD format")
rootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false,
    "Suppress non-essential output")
rootCmd.PersistentFlags().StringVar(&logLevel, "log-level", "INFO",
    "Log level: DEBUG, INFO, WARN, ERROR")
```

### Command-Specific Flags

```go
func init() {
    taskCmd.AddCommand(taskNewCmd)

    taskNewCmd.Flags().StringSliceVar(&tagsFlag, "tags", []string{},
        "Tags for the task (comma-separated)")
    taskNewCmd.Flags().StringVar(&scheduleFlag, "schedule", "",
        "Schedule date in YYYY-MM-DD format")
}
```

## Output Formats

### Standard Output (Respect --quiet)

```go
if !quietFlag {
    fmt.Printf("✓ Created task: %s\n", task.ID)
}
```

### Machine-Readable Output

For commands like `rk today`, `rk week`:
```go
// Output to stdout for piping
fmt.Print(journalContent)  // No extra formatting
```

This allows:
```bash
rk today | llm "summarize my day"
rk week | pbcopy
```

### Formatted Tables

Use consistent column formatting:
```go
// Example from task list
fmt.Printf("%-12s %-8s %s\n", "ID", "STATUS", "TITLE")
for _, task := range tasks {
    fmt.Printf("%-12s %-8s %s\n",
        truncate(task.ID, 12),
        task.Status,
        task.Text)
}
```

## Error Handling

### Return Errors, Don't Exit

```go
// ❌ Don't call os.Exit() in commands
if err != nil {
    fmt.Fprintf(os.Stderr, "Error: %v\n", err)
    os.Exit(1)
}

// ✅ Return error, let Cobra handle it
if err != nil {
    return fmt.Errorf("failed to create task: %w", err)
}
```

Cobra will:
- Print the error
- Exit with code 1
- Allow testing to catch errors

### Wrap Errors with Context

```go
task, err := taskService.GetTaskByID(taskID)
if err != nil {
    return fmt.Errorf("failed to get task %s: %w", taskID, err)
}
```

## Date Handling

### Date Flag Parsing

```go
func getTargetDate() string {
    if dateFlag != "" {
        // Use provided date (validate format)
        _, err := time.Parse("2006-01-02", dateFlag)
        if err != nil {
            return ""  // Invalid
        }
        return dateFlag
    }
    // Default to today
    return time.Now().Format("2006-01-02")
}
```

### Date Validation

```go
const dateFormat = "2006-01-02"

func validateDate(dateStr string) error {
    _, err := time.Parse(dateFormat, dateStr)
    if err != nil {
        return fmt.Errorf("invalid date format, expected YYYY-MM-DD: %w", err)
    }
    return nil
}
```

## Interactive vs Non-Interactive

### Detecting TTY

```go
import "os"

func isTTY() bool {
    fileInfo, _ := os.Stdout.Stat()
    return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

// Usage
if isTTY() {
    // Show interactive prompts, colors, etc.
} else {
    // Plain output for piping
}
```

### Interactive Prompts

For commands with no args, launch interactive forms:
```go
RunE: func(cmd *cobra.Command, args []string) error {
    if len(args) == 0 {
        return launchInteractiveForm()  // TUI form
    }
    // CLI mode with args
    return createFromArgs(args)
},
```

## Common Tasks

### Add a New Subcommand

1. **Create command in relevant file** (e.g., `task.go`):
   ```go
   var taskAddCmd = &cobra.Command{
       Use:   "add [title]",
       Short: "Add a new task",
       Args:  cobra.MinimumNArgs(1),
       RunE: func(cmd *cobra.Command, args []string) error {
           title := strings.Join(args, " ")
           task, err := taskService.CreateTask(title)
           if err != nil {
               return fmt.Errorf("failed to create task: %w", err)
           }
           if !quietFlag {
               fmt.Printf("✓ Created task: %s\n", task.ID)
           }
           return nil
       },
   }
   ```

2. **Register in init()**:
   ```go
   func init() {
       taskCmd.AddCommand(taskAddCmd)

       // Add flags if needed
       taskAddCmd.Flags().StringSliceVar(&tagsFlag, "tags", []string{}, "Tags")
   }
   ```

3. **Add to parent command** (if new top-level):
   ```go
   // In root.go
   func init() {
       rootCmd.AddCommand(taskCmd)
   }
   ```

### Add Output Format Support

```go
var formatFlag string

func init() {
    listCmd.Flags().StringVar(&formatFlag, "format", "table",
        "Output format: table, json, csv")
}

RunE: func(cmd *cobra.Command, args []string) error {
    items := fetchItems()

    switch formatFlag {
    case "json":
        return outputJSON(items)
    case "csv":
        return outputCSV(items)
    default:
        return outputTable(items)
    }
},
```

## Testing CLI Commands

### Test Command Execution

```go
func TestTaskNewCommand(t *testing.T) {
    // Setup
    cmd := taskNewCmd
    cmd.SetArgs([]string{"Test task"})

    // Execute
    err := cmd.Execute()

    // Assert
    assert.NoError(t, err)
    // Verify side effects (check database, etc.)
}
```

### Test with Flags

```go
func TestWithFlags(t *testing.T) {
    cmd := taskNewCmd
    cmd.SetArgs([]string{
        "Test task",
        "--tags=foo,bar",
        "--schedule=2025-01-15",
    })

    err := cmd.Execute()
    assert.NoError(t, err)
}
```

### Mock Services for Testing

```go
// Create mock service
mockService := &mockTaskService{
    createFunc: func(title string) (*Task, error) {
        return &Task{ID: "test-id", Title: title}, nil
    },
}

// Inject into command (requires refactoring away from globals)
```

## Common Pitfalls

### 1. Forgetting --quiet Flag

```go
// ❌ Always prints
fmt.Printf("Task created\n")

// ✅ Respects quiet flag
if !quietFlag {
    fmt.Printf("Task created\n")
}
```

### 2. Printing to Wrong Stream

```go
// ❌ Error to stdout
fmt.Printf("Error: %v\n", err)

// ✅ Error to stderr
fmt.Fprintf(os.Stderr, "Error: %v\n", err)

// ✅ Or return error (Cobra handles stderr)
return err
```

### 3. Not Validating Args

```go
// ❌ Panic on missing args
title := args[0]  // Panic if no args

// ✅ Use Cobra validation
Args: cobra.MinimumNArgs(1),
```

### 4. Hardcoding Dates

```go
// ❌ Ignores --date flag
date := time.Now().Format("2006-01-02")

// ✅ Check date flag
date := getTargetDate()
```

## Planned Improvements

### Issue reckon-89hp: Verb Alignment

**Goal:** Standardize on `add/list/show/edit/delete` across all resources.

**Changes needed:**
- `task new` → `task add`
- `notes create` → `notes add`
- Add missing commands: `notes list`, `notes delete`, `log list`, etc.

See ticket for full details.

### Future: Dependency Injection

**Current:** Package-level globals
**Target:** Inject services via struct or context

```go
type CLI struct {
    journal *journal.Service
    task    *journal.TaskService
    notes   *service.NotesService
}

func (c *CLI) Execute() error {
    return rootCmd.Execute()
}
```

## Exit Codes

See `/docs/exit-codes.md` for comprehensive documentation.

**Common codes:**
- `0` - Success
- `1` - General error
- `2` - Invalid arguments
- `130` - Interrupted (Ctrl+C)

## Resources

- **Cobra docs:** https://github.com/spf13/cobra
- **CLI skills:** `/cli-skills` (skill file)
- **Exit codes:** `/docs/exit-codes.md`
- **Examples:** Check `*_test.go` files for usage examples
