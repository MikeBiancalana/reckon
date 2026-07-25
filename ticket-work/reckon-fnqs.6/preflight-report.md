# Preflight Check Report: reckon-fnqs.6

**Ticket:** reckon-fnqs.6 (Composable prompt layer: Prompt interface, RunPrompt host, Wizard, TTY guard)

**Date:** 2026-07-20

---

## Automated Checks

### 1. Format (go fmt)
- **Status:** ✅ PASS
- **Output:** No formatting changes detected
- **Action:** No commit needed (formatting already correct)

### 2. Vet (go vet)
- **Status:** ✅ PASS
- **Output:** No issues found
- **Command:** `go vet ./...`

### 3. Tests (go test)
- **Status:** ✅ PASS
- **Output:** All 18 packages tested, all passing
  - github.com/MikeBiancalana/reckon/cmd/rk [no test files]
  - internal/checklist: ok
  - internal/cli: ok (2.562s)
  - internal/config: ok
  - internal/index: ok
  - internal/journal: ok
  - internal/logger: ok
  - internal/models: ok
  - internal/node: ok
  - internal/output: ok
  - internal/parser: ok
  - internal/perf: [no test files]
  - internal/service: ok
  - internal/spike/roundtrip: ok
  - internal/storage: ok
  - internal/textmigrate: ok
  - internal/time: [no test files]
  - internal/tui: ok
  - internal/tui/components: ok

### 4. Coverage
- **Status:** ✅ INFORMATIONAL
- **Highlights:**
  - internal/models: 100.0%
  - internal/parser: 95.5%
  - internal/node: 94.4%
  - internal/logger: 81.0%
  - internal/checklist: 81.2%
  - internal/tui/components: 75.7%
  - internal/cli: 74.1%
  - internal/config: 75.8%
  - internal/index: 74.5%
  - internal/service: 76.2%
  - internal/output: 76.5%

---

## Manual Checks

### 1. Error Handling

**Files Reviewed:**
- internal/tui/components/prompt.go
- internal/tui/components/wizard.go
- internal/tui/components/form.go
- internal/tui/components/text_editor.go
- internal/tui/components/date_picker.go
- internal/tui/components/task_picker.go
- internal/tui/components/note_picker.go
- internal/cli/interactive.go
- internal/cli/root.go
- internal/cli/tui_model.go
- internal/cli/tui_keyboard.go

**Pattern Check:** ✅ PASS
- All error assignments are checked
  - prompt.go:88 - `tea.NewProgram().Run()` error checked and wrapped
  - form.go:287-291 - `ParseRelativeDate()` error checked and stored
  - form.go:297-301 - Custom validator error checked and stored
  - form.go:341-344 - Date parse error checked
  - date_picker.go:144-148 - Parse error checked and user-friendly message set
  - date_picker.go:182-187 - Parse error checked in preview update
  - interactive.go:34 - TTY guard error wrapped with context
  - root.go:117 - Logger init error wrapped with context
  - root.go:127 - Date parse error wrapped with context

**Pattern Violations:** None detected
- All errors are handled immediately after assignment
- Errors are wrapped with `fmt.Errorf(...%w...)` for context where appropriate
- No bare `err :=` without following check found

### 2. Resource Cleanup

**Pattern Check:** ✅ PASS
- No file operations found that require cleanup in the modified files
- No database transaction patterns found in the reviewed scope
- No resource allocation without cleanup

### 3. CLI-Specific Patterns

**--quiet Flag Handling:**
- ✅ Flag defined in root.go:26 (`quietFlag bool`)
- ✅ Flag registered in root.go:93 (BoolVarP with description)
- ✅ Flag used in logger config buildLoggerConfig() at root.go:49-50
- ✅ Respects quiet flag by raising log level to ERROR when --quiet is set

**Error Return Pattern:**
- ✅ root.go:117 - Returns error from initLoggerE, does not call os.Exit
- ✅ root.go:138-150 - Execute() returns error, lets caller (main) handle exit
- ✅ No os.Exit calls found in library code

**Input Validation:**
- ✅ root.go:124-131 - getEffectiveDate() validates date format before returning
- ✅ root.go:82-86 - PersistentPreRunE validates flag exclusivity

### 4. TUI-Specific Patterns

**Variable Capture in Closures:**
- ✅ PASS - Proper pattern followed throughout
- Comment in tui_model.go:259-262 documents the pattern: "each closure captures only *sql.DB/*index.Index and any plain values it needs, never a pointer into pane state"

**Verified closures:**
- tui_model.go:265-275 (loadAgendaCmd): captures `db` (extracted via m.ix.DB()) and `today` (string) - correct
- tui_model.go:277-291 (loadTodosCmd): captures `db` - correct
- tui_model.go:293-302 (loadLogCmd): captures `db` - correct
- tui_model.go:304-313 (loadNotesListCmd): captures `db` - correct
- tui_model.go:315-324 (loadNotesLinksCmd): captures `db` - correct
- tui_model.go:328-344 (selectNoteCmd): captures `db` - correct
- tui_keyboard.go:120-129 (actuateCmd): captures `vaultDir`, `ix`, parameters - correct
- tui_keyboard.go:358-372 (addTodoCmd): captures `vaultDir`, `ix`, `author`, `body` (all parameters) - correct
- tui_keyboard.go:376-392 (addLogCmd): captures `vaultDir`, `ix`, `author`, `body` - correct
- tui_keyboard.go:399-419 (createNoteCmd): captures `vaultDir`, `ix`, `author`, `title` - correct

**Model Component Updates:**
- ✅ tui_keyboard.go:291-292 - Proper pattern: `p, cmd := m.datePicker.Update(msg); m.datePicker = p.(*components.DatePicker)`
- ✅ tui_keyboard.go:315-316 - Proper pattern: `m.textEntry, cmd = m.textEntry.Update(msg)`

**Nil Checks:**
- ✅ wizard.go:106-108 - Nil check before Update: `if w.active == nil { return w, nil }`
- ✅ wizard.go:123-127 - Nil check after factory call, handles nil factory result
- ✅ tui_model.go:194-195 - Checks nil on m.notes.notes before operations

### 5. Test Coverage

**Component Tests Present:**
- ✅ prompt_test.go - RunPrompt and Prompt interface tests
- ✅ wizard_test.go - Wizard flow and step transitions
- ✅ form_test.go - Form validation and field handling
- ✅ form_example_test.go - Example/integration test
- ✅ text_editor_test.go - TextEditor component
- ✅ text_editor_example_test.go - Example test
- ✅ date_picker_test.go - DatePicker component
- ✅ date_picker_example_test.go - Example test
- ✅ task_picker_test.go - TaskPicker component
- ✅ task_picker_example_test.go - Example test
- ✅ note_picker_test.go - NotePicker component

**CLI Tests Present:**
- ✅ interactive_test.go - TTY detection tests
- ✅ tui_test.go - TUI integration tests

### 6. Code Quality

**TODO/FIXME Comments:**
- ✅ PASS - No TODO/FIXME found without context in reviewed files

**Commented-Out Code:**
- ✅ PASS - No commented-out code blocks detected

**Print Statements in Library Code:**
- ✅ PASS - No fmt.Print/Println/Printf calls in library code
- Note: Library code correctly uses logger package for output

**Hardcoded Paths:**
- ✅ PASS - No hardcoded absolute paths detected
- Note: tui_keyboard.go uses filepath.Join() properly (lines 363, 381, 404)

**Magic Numbers:**
- ✅ PASS - Reviewed numeric literals are appropriate
  - CharLimit values (100, 500) are reasonable field limits
  - Color codes (39, 196, 240, 245, 252, etc.) are lipgloss color references
  - Width/height calculations (10, 15, 20, 40, 80) are UI layout constants

**Unused Imports:**
- ✅ PASS - go vet verified; no unused imports

---

## Issues Found

### Critical (Must Fix)
**None detected**

### Warnings (Should Fix)
**None detected**

### Info (Nice to Have)
**None detected**

---

## Summary

**Status:** ✅ **PASS**

All automated checks pass without errors or warnings. Manual pattern review confirms:
- Error handling is comprehensive and properly wrapped
- No resource leaks or unclosed files
- CLI follows --quiet flag and error return conventions
- TUI closures follow correct capture pattern (no pane state capture)
- All components have tests
- Code quality standards are maintained

**Next Steps:**
- Ready to proceed to code review phase
- No blocking issues identified

