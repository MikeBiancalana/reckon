# Implementation Summary: `rk note list` Command

## Overview
Implemented the `rk note list` CLI command for listing zettelkasten notes with filtering, sorting, and multiple output format options.

## Changes Made

### 1. Backend Services

#### `/internal/service/notes_repository.go`
- Added `GetAllNotes()` method to retrieve all notes from the database
- Notes are returned sorted by `created_at DESC` (newest first)
- Properly handles tag parsing from comma-separated string storage

#### `/internal/service/notes_service.go`
- Added `GetAllNotes()` service method as a passthrough to repository

### 2. CLI Implementation

#### `/internal/cli/note.go`
- Added `noteListCmd` command with full feature set
- Implemented filtering by tags using OR logic (any tag match)
- Implemented sorting by: created (default), updated, title, slug
- Added support for multiple output formats: table, json, csv
- Dynamic column width calculation for table output based on terminal size
- Graceful handling of empty note lists

#### `/internal/cli/format.go`
- Added `formatNotesJSON()` for JSON output format
- Added `formatNotesCSV()` for CSV output format with proper escaping
- CSV headers: title, slug, created, updated, tags

#### `/internal/cli/root.go`
- Added `notesService` variable initialization
- Created notes service with repository in `initService()`
- Fixed import/naming conflicts by renaming journal service variable

### 3. Tests

#### `/internal/service/notes_service_test.go`
- Added `TestGetAllNotes_Empty()` - Tests empty database scenario
- Added `TestGetAllNotes()` - Tests retrieval of multiple notes with correct ordering
- Added `TestGetAllNotes_WithTags()` - Tests tag retrieval and parsing

All new tests pass successfully.

### 4. Bug Fixes

Fixed variable naming conflicts throughout CLI files:
- Renamed `service` variable to `journalService` to avoid conflict with imported `service` package
- Used package alias `notesvc` for notes service import
- Updated references across multiple CLI files: note.go, log.go, rebuild.go, schedule.go, today.go, week.go

## Command Usage

```bash
# List all notes (default table format)
rk note list

# Filter by tags (OR logic - shows notes with ANY of the tags)
rk note list --tags=golang,testing

# Sort by different fields
rk note list --sort=title
rk note list --sort=updated
rk note list --sort=slug

# Different output formats
rk note list --format=json
rk note list --format=csv

# Combine options
rk note list --tags=golang --sort=title --format=json
```

## Output Examples

### Table Format (Default)
```
TITLE                 SLUG                  CREATED      TAGS
Zettelkasten Method   zettelkasten-method   2026-02-01   knowledge, method
My First Note         my-first-note         2026-01-31   golang, testing
```

### JSON Format
```json
[
  {
    "id": "abc123",
    "title": "Zettelkasten Method",
    "slug": "zettelkasten-method",
    "file_path": "2026/2026-02/2026-02-01-zettelkasten-method.md",
    "tags": ["knowledge", "method"],
    "created_at": "2026-02-01T10:00:00Z",
    "updated_at": "2026-02-01T10:00:00Z"
  }
]
```

### CSV Format
```csv
title,slug,created,updated,tags
Zettelkasten Method,zettelkasten-method,2026-02-01,2026-02-01,"knowledge, method"
My First Note,my-first-note,2026-01-31,2026-01-31,"golang, testing"
```

## Testing

All service tests pass:
```bash
go test ./internal/service/... -v
# PASS: TestGetAllNotes_Empty
# PASS: TestGetAllNotes
# PASS: TestGetAllNotes_WithTags
```

Build succeeds:
```bash
go build ./...
# Success
```

CLI command works correctly:
```bash
/tmp/rk note list --help
# Shows proper help with all flags
```

## Architecture Decisions

1. **Filtering in CLI Layer**: Tag filtering is implemented in the CLI layer rather than the service layer to keep the service simple and allow for future flexibility.

2. **Sorting**: Default sort is by created date (newest first) which matches the SQL query. Other sort options are applied in-memory in the CLI.

3. **Format Support**: JSON and CSV formats use standard Go libraries (encoding/json, encoding/csv) for proper escaping and standards compliance.

4. **Terminal Width**: Table output dynamically adjusts column widths based on terminal size for better readability.

5. **Empty State**: Shows friendly "No notes found" message when no notes exist, respecting the --quiet flag.

## Code Quality

- Follows existing patterns from task list command
- Uses established format functions for consistency
- Proper error handling with descriptive messages
- Comprehensive test coverage for new functionality
- Clean separation of concerns (repository, service, CLI)

## Future Enhancements (Not Implemented)

Potential improvements for future PRs:
- Add search/filter by title or content
- Support for date range filtering
- More sort options (by tag count, by link count)
- Interactive selection mode with fuzzy search
- Export to other formats (markdown table, org-mode)
