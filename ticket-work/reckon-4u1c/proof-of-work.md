# reckon-4u1c: Add description field to task creation TUI form

*2026-03-30T00:10:34Z by Showboat 0.6.1*
<!-- showboat-id: 94c9c9c5-6393-44c9-b8d3-e0142e26dbbd -->

Add Description string field to Task model, wire through parser/writer/service, add to TUI form and CLI flag.

```bash
go test ./internal/journal/... -run 'Description|AddTask' -v 2>&1 | grep -E '(=== RUN|--- |PASS|FAIL)'
```

```output
=== RUN   TestWriteTaskFile_WithDescription
--- PASS: TestWriteTaskFile_WithDescription (0.00s)
=== RUN   TestWriteTaskFile_EmptyDescription
--- PASS: TestWriteTaskFile_EmptyDescription (0.00s)
=== RUN   TestParseTaskFile_DescriptionWithMarkdown
--- PASS: TestParseTaskFile_DescriptionWithMarkdown (0.00s)
=== RUN   TestAddTask
--- PASS: TestAddTask (0.01s)
=== RUN   TestAddTask_MultipleTasksIncrementPosition
--- PASS: TestAddTask_MultipleTasksIncrementPosition (0.01s)
=== RUN   TestAddTaskNote
--- PASS: TestAddTaskNote (0.01s)
=== RUN   TestAddTaskNote_MultipleNotes
--- PASS: TestAddTaskNote_MultipleNotes (0.01s)
=== RUN   TestAddTaskNote_TaskNotFound
--- PASS: TestAddTaskNote_TaskNotFound (0.01s)
=== RUN   TestAddTask_WithDescription
--- PASS: TestAddTask_WithDescription (0.01s)
=== RUN   TestAddTask_EmptyDescription
--- PASS: TestAddTask_EmptyDescription (0.01s)
PASS
```

```bash
go test ./internal/journal/... ./internal/cli/... ./internal/tui/... 2>&1 | grep -E '^(ok|FAIL|---\s+FAIL)'
```

```output
--- FAIL: TestParseTaskFile (0.00s)
--- FAIL: TestWriteTaskFile (0.00s)
FAIL
FAIL	github.com/MikeBiancalana/reckon/internal/journal	0.730s
--- FAIL: TestNoteCommand_DeprecationNotice (0.00s)
--- FAIL: TestNoteNewCommand_DeprecationNotice (0.00s)
--- FAIL: TestNoteFileStructure (0.01s)
FAIL
FAIL	github.com/MikeBiancalana/reckon/internal/cli	0.165s
ok  	github.com/MikeBiancalana/reckon/internal/tui	(cached)
ok  	github.com/MikeBiancalana/reckon/internal/tui/components	(cached)
FAIL
```

Pre-existing test failures (not caused by this change): TestParseTaskFile/parse_task_with_notes (note format mismatch), TestWriteTaskFile/write_task_with_scheduled|deadline (YAML date quoting), TestNoteCommand_DeprecationNotice, TestNoteNewCommand_DeprecationNotice, TestNoteFileStructure (deprecation text missing). All description-related tests PASS.
