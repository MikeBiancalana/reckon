# Testing Guide: Task New Form Integration

## Overview
The `rk task new` command now supports an interactive TUI form when called with no arguments.

## Test Cases

### 1. Interactive Form (Manual Test Required)
**Command:**
```bash
rk task new
```

**Expected Behavior:**
- Form should appear with 4 fields:
  - Title (required, text)
  - Tags (optional, text - comma-separated)
  - Schedule (optional, date)
  - Deadline (optional, date)
- TAB should move between fields
- SHIFT+TAB should move backwards
- Date fields should show live preview when valid
- ENTER should submit (after validation)
- ESC should cancel
- Success message should display after submission

**Test Steps:**
1. Run `rk task new` with no arguments
2. Enter "Test Task" for title
3. Enter "test, demo" for tags
4. Enter "tomorrow" for schedule
5. Enter "+3d" for deadline
6. Press ENTER to submit
7. Verify task was created with:
   - Title: "Test Task"
   - Tags: ["test", "demo"]
   - Scheduled: 2026-02-01 (tomorrow)
   - Deadline: 2026-02-03 (+3 days)

### 2. CLI Argument Mode (Backward Compatibility)
**Command:**
```bash
rk task new "CLI Task" --tags=cli,test --schedule=+2d --deadline=+5d
```

**Expected Behavior:**
- Task created immediately without form
- Output shows success message with all fields
- Task file contains correct dates

**Verification:**
```bash
# Check the task file
cat ~/.reckon/tasks/2026-01-31-cli-task.md
```

Expected frontmatter:
```yaml
---
id: <generated-id>
title: CLI Task
created: "2026-01-31"
status: open
tags:
  - cli
  - test
scheduled_date: "2026-02-02"
deadline_date: "2026-02-05"
---
```

### 3. Simple Task Creation
**Command:**
```bash
rk task new "Simple Task"
```

**Expected Behavior:**
- Task created with just a title
- No schedule or deadline set
- No tags

### 4. Form Cancellation (Manual Test Required)
**Command:**
```bash
rk task new
```

**Test Steps:**
1. Run command
2. Press ESC immediately
3. Verify no task was created
4. Verify command exits cleanly

### 5. Form Validation (Manual Test Required)
**Command:**
```bash
rk task new
```

**Test Steps:**
1. Run command
2. Leave title empty, press ENTER
3. Verify error message appears
4. Fill in title: "Valid Task"
5. Enter invalid date in schedule: "xyz"
6. Press ENTER
7. Verify error message appears
8. Fix date to "tomorrow"
9. Press ENTER
10. Verify task is created successfully

## Implementation Details

### Files Modified
- `/home/chadd/repos/reckon/reckon-s22g/internal/cli/task.go`

### New Code
1. **taskNewFormModel** - Bubble Tea model for form interaction
2. **launchTaskNewForm()** - Launches the interactive form
3. **createTaskFromForm()** - Processes form submission and creates task
4. **Updated taskNewCmd** - Added logic to detect when to launch form vs CLI mode

### Key Features
- Form launches only when: `rk task new` with NO arguments AND NO flags
- Preserves all existing CLI behavior
- Uses components.ParseRelativeDate() for natural date parsing
- Supports all date formats: t, tm, +3d, +2w, mon-sun, YYYY-MM-DD
- Live date preview in form
- Proper error handling and validation

## Known Limitations
1. Form requires terminal support (won't work in pipes/redirects)
2. Cannot test form interactively in automated tests
3. Finding newly created task relies on title matching (small race condition possibility)

## Future Improvements
1. Make AddTask return the task ID to avoid the search
2. Add form presets/templates
3. Add autocomplete for tags
4. Add recurring task support in form

## Quick CLI Test Commands

```bash
# Test 1: CLI with all flags
rk task new "Test CLI" --tags=test,demo --schedule=tomorrow --deadline=+3d

# Test 2: CLI with just title
rk task new "Simple Task"

# Test 3: CLI with schedule only
rk task new "Scheduled Task" --schedule=+2d

# Test 4: Launch form (no args)
rk task new
# Then fill in the form interactively

# Test 5: Verify help text
rk task new --help

# Clean up test tasks
rm ~/.reckon/tasks/*test-cli*.md ~/.reckon/tasks/*simple-task*.md ~/.reckon/tasks/*scheduled-task*.md
```
