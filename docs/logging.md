# Logging System

## Overview

Reckon uses a structured logging system based on Go's standard `log/slog` package with automatic log rotation via [lumberjack](https://github.com/natefinch/lumberjack).

## Features

- **TUI Mode Detection**: Automatically redirects logs to a file when running in TUI mode to prevent visual corruption
- **Log Rotation**: Automatic log file rotation with configurable size, backup count, and age limits
- **Compression**: Old log files are automatically compressed to save disk space
- **Flexible Configuration**: Control logging via command-line flags or environment variables

## Configuration

### Command-Line Flags

```bash
# Specify custom log file location
rk --log-file /path/to/custom.log

# Set log level (DEBUG, INFO, WARN, ERROR)
rk --log-level DEBUG

# Combine flags
rk --log-file /tmp/reckon.log --log-level DEBUG
```

### Environment Variables

```bash
# Set log level
export LOG_LEVEL=DEBUG

# Set log format (text or json)
export LOG_FORMAT=json

# Legacy debug flag
export RECKON_DEBUG=1  # Equivalent to LOG_LEVEL=DEBUG
```

## Behavior by Mode

### TUI Mode (default `rk` command)

When running the TUI (executing `rk` without subcommands):
- Logs are automatically redirected to `~/.reckon/logs/reckon.log`
- This prevents log output from corrupting the visual display
- You can override the location with `--log-file`

### CLI Mode (subcommands)

When running CLI subcommands (e.g., `rk today`, `rk task add`):
- Logs go to stderr by default
- Use `--log-file` to redirect to a file
- Use `--log-level` to control verbosity

## Log Rotation

Logs are automatically rotated based on:
- **Max Size**: 10 MB per file
- **Max Backups**: 3 old log files kept
- **Max Age**: 30 days
- **Compression**: Enabled (old logs are gzipped)

These settings are configured in `internal/logger/logger.go` and can be adjusted if needed.

## Log Levels

- **DEBUG**: Detailed information for diagnosing issues
- **INFO**: General informational messages (default)
- **WARN**: Warning messages for potentially harmful situations
- **ERROR**: Error messages for serious problems

## Examples

### View TUI logs after running

```bash
# Run the TUI
rk

# After exiting, view the logs
tail -f ~/.reckon/logs/reckon.log
```

### Debug a CLI command

```bash
# Run with debug logging to a file
rk --log-file /tmp/debug.log --log-level DEBUG task add "Test task"

# View the debug output
cat /tmp/debug.log
```

### Debug TUI with custom log location

```bash
# Run TUI with debug logging to custom location
rk --log-file /tmp/tui-debug.log --log-level DEBUG

# In another terminal, watch logs in real-time
tail -f /tmp/tui-debug.log
```

## Implementation Details

### Logger Package (`internal/logger`)

The logger package provides:
- `InitializeWithConfig(cfg Config)`: Initialize logger with specific configuration
- `GetLogger()`: Get the configured slog.Logger instance
- `GetLevel()`: Get the current log level
- `GetLogFile()`: Get the current log file path
- `IsTUIMode()`: Check if logger is in TUI mode
- Convenience functions: `Debug()`, `Info()`, `Warn()`, `Error()`

### Configuration Structure

```go
type Config struct {
    Level   string // "DEBUG", "INFO", "WARN", "ERROR"
    Format  string // "text" or "json"
    File    string // Path to log file (empty = auto-determine)
    TUIMode bool   // If true, auto-redirect to default location
}
```

### Initialization Flow

1. Command-line flags are parsed
2. `initLogger()` is called via `cobra.OnInitialize()`
3. Logger is configured based on flags and environment variables
4. For TUI mode, logger is reconfigured to enable file redirection
5. Logs are written to the configured destination

## Troubleshooting

### Logs not appearing

Check if:
1. Log level is set low enough (e.g., DEBUG to see all logs)
2. Log file path is writable
3. Logs directory exists (created automatically for default location)

### TUI display corruption

If you see log output in the TUI:
- Check that you're running a recent version with logging fixes
- Verify logs are going to a file: `ls -l ~/.reckon/logs/`
- Try specifying explicit log file: `rk --log-file /tmp/reckon.log`

### Finding log files

Default locations:
- TUI mode: `~/.reckon/logs/reckon.log`
- CLI mode: stderr (no file unless specified)

Old rotated logs:
- `~/.reckon/logs/reckon.log.1.gz`
- `~/.reckon/logs/reckon.log.2.gz`
- `~/.reckon/logs/reckon.log.3.gz`
