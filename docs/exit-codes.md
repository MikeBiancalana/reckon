# Exit Codes and Error Handling

This document describes the exit codes and error handling conventions used in the Reckon CLI.

## Exit Codes

Reckon uses standard UNIX exit codes for predictable behavior in scripts and pipelines.

| Code | Name | Description |
|------|------|-------------|
| 0 | Success | Command completed successfully |
| 1 | General Error | Database failure, file I/O error, or unexpected error |
| 2 | Usage Error | Invalid flags, missing arguments, or validation failures |
| 3 | Not Found | Requested resource (task, journal, etc.) was not found |

### Exit Code Details

#### Exit Code 0 - Success
Returned when a command completes without errors. Examples:
- `rk today` successfully displays today's journal
- `rk task list` successfully lists all tasks
- `rk task done 123` successfully marks a task as done

#### Exit Code 1 - General Error
Returned when an unexpected error occurs. Examples:
- Database connection fails
- File I/O errors (permissions, disk full)
- Internal errors in the application

#### Exit Code 2 - Usage Error
Returned when command-line arguments are invalid. Examples:
- Missing required arguments
- Invalid flag values
- Invalid date formats
- Validation failures (e.g., empty task title)

#### Exit Code 3 - Not Found
Returned when a requested resource does not exist. Examples:
- `rk task show 999` when task 999 doesn't exist
- `rk task delete non-existent-task` when the task isn't found
- `rk schedule delete 10` when index 10 doesn't exist

## Error Handling Patterns

Reckon follows specific patterns for error handling throughout the codebase.

### CLI Layer (Cobra Commands)

CLI commands use Cobra's `RunE` functions and return errors:

```go
func (s *Service) runTaskList(cmd *cobra.Command, args []string) error {
    tasks, err := s.journalService.ListTasks(ctx, filter)
    if err != nil {
        return fmt.Errorf("failed to list tasks: %w", err)
    }
    // ...
    return nil
}
```

Cobra automatically handles errors returned from `RunE`:
- Prints the error message to stderr
- Exits with code 1

### Service Layer

The service layer uses structured logging with `slog.Logger`:

```go
func (s *TaskService) LogTaskProgress(ctx context.Context, id string, msg string) error {
    task, err := s.getTaskByID(ctx, id)
    if err != nil {
        s.logger.Error("failed to get task", "task_id", id, "error", err)
        return fmt.Errorf("failed to get task: %w", err)
    }
    // ...
}
```

### Error Wrapping Convention

All errors should be wrapped with context using `fmt.Errorf`:

```go
return fmt.Errorf("operation description: %w", err)
```

This preserves the underlying error while adding context for debugging.

## Common Error Messages

### Task Commands

| Error | Exit Code | Cause |
|-------|-----------|-------|
| "task not found" | 3 | Specified task ID doesn't exist |
| "invalid task ID format" | 2 | Task ID is not a valid number |
| "task title cannot be empty" | 2 | Empty title provided to `rk task new` |

### Schedule Commands

| Error | Exit Code | Cause |
|-------|-----------|-------|
| "schedule item not found" | 3 | Specified schedule index doesn't exist |
| "invalid time format" | 2 | Time must be in HH:MM format |

### Win Commands

| Error | Exit Code | Cause |
|-------|-----------|-------|
| "win not found" | 3 | Specified win ID doesn't exist |

### General Commands

| Error | Exit Code | Cause |
|-------|-----------|-------|
| "failed to open database: ..." | 1 | Database file cannot be opened |
| "database migration failed: ..." | 1 | Schema migration error |
| "configuration error: ..." | 1 | Invalid configuration |

## Scripting Examples

### Checking Command Success

```bash
# Basic success/failure check
rk task done 123
if [ $? -eq 0 ]; then
    echo "Task completed"
fi

# Using set for error handling
set -e  # Exit on error (any non-zero exit)
rk task done 123
echo "Task completed"
```

### Handling Specific Errors

```bash
# Check for not-found errors
rk task show 999
if [ $? -eq 3 ]; then
    echo "Task 999 does not exist"
fi

# Check for usage errors
rk task new ""
if [ $? -eq 2 ]; then
    echo "Invalid command usage"
fi
```

### Pipelines and Exit Codes

```bash
# Get open tasks, filter for urgent, mark one as done
rk task list --status=open --tag=urgent | head -1 | xargs rk task done
```

Note: In pipelines, only the exit code of the last command is preserved. Use `set -o pipefail` to catch failures in earlier commands.

## Error Output

Errors are written to stderr. Example:

```
$ rk task show 999
Error: task not found: task 999 does not exist
```

Error messages follow the format: `Error: <context>: <detailed message>`

## See Also

- [Bubbletea TUI](./tui-guide.md) - Error handling in the TUI
- [API Reference](./api.md) - Service layer error handling
- [Configuration](./configuration.md) - Configuration-related errors
