# Implementation Summary: reckon-fnqs.7

## Status: READY FOR PUSH, WITH ONE CAVEAT — see "Not verified" below

## Review Verdict: APPROVE WITH CHANGES (all requested + follow-up changes applied)

## Changed Files:
internal/cli/add.go
internal/cli/add_wizard.go
internal/cli/add_wizard_test.go
internal/cli/body_entry.go
internal/cli/note_create_wizard.go
internal/cli/note_create_wizard_test.go
internal/cli/note_v1.go
internal/cli/todo.go
internal/cli/todo_add_wizard.go
internal/cli/todo_add_wizard_test.go
internal/cli/todo_browse.go
internal/tui/components/multi_note_picker.go
internal/tui/components/multi_note_picker_test.go
internal/tui/components/text_prompt.go
internal/tui/components/text_prompt_test.go
ticket-work/reckon-fnqs.7/ (plan.md, acceptance-criteria.md, codebase-analysis.md, preflight-report.md, review.md, state.json, pattern-frequency.txt)

## Commits:
53a8cf5 fix: Remove dead TextPrompt.SetValue, restore stage-before-slug validation order
c17912f chore: Sync beads jsonl (claim status) and add review.md for reckon-fnqs.7
3a4b324 docs: Add implementation summary for reckon-fnqs.7
9411dbe chore: Add pattern-frequency scan for reckon-fnqs.7
d268139 fix: Address code review findings for reckon-fnqs.7
8928f88 chore: Add preflight report for reckon-fnqs.7
5ed3e98 refactor: Simplify implementation for reckon-fnqs.7
3afcecd docs: Clean stale/provenance comments in reckon-fnqs.7 test files
ab4b709 Consult --date in addWantsTUI's dispatch predicate
7e62f0c Wire wizard/prompt dispatch into todo add, note create, and add
6c64521 Implement TextPrompt and MultiNotePicker Update/View
ebfcc50 chore: Add pipeline state tracking for reckon-fnqs.7
247d03e test: Write failing tests for reckon-fnqs.7
69b476c docs: Add plan and analysis for reckon-fnqs.7

## Test Results:
All 26 feature tests pass by name; full module `go test ./...` exits 0
across every package (18 packages, no failures). All 22 pre-existing
regression-anchor tests stay green.

## Preflight: Status: PASS
## Review: **Verdict: APPROVE WITH CHANGES**

## Pattern Frequency:
unwrapped error: 0
missing defer: 0
closure capture: 0
nil check: 0
missing validation: 0

## Not verified: live wizard rendering on a real TTY

Every check in this pipeline is either a scripted-keystroke unit test
(`tea.WithInput`/`tea.WithOutput`) or an in-process `RootCmd.Execute()` —
nothing has driven the actual `View()` rendering, real-terminal key
handling, or the multi-step feel of any of the three wizards on a real
terminal. The wizard *composition* (does the right sequence of steps run)
and the *file-convergence* (does it write the same bytes as the flag path)
are both well-covered — that's not in question. What's unverified is purely
the interactive rendering/UX layer.

This matters specifically for this ticket: the reason full-screen `rk tui`
got deprioritized (reckon-nzk3) was a rendering bug that only showed up on
first real use, despite passing all its own tests the same way this ticket's
tests pass. Recommend eyeballing at least one wizard (e.g. `rk todo add`
bare, on a real terminal) before pushing, given that history.

## Scope note for Mike
Two components didn't exist yet and were built as part of this ticket
(scoped in during planning, not discovered mid-implementation):
`TextPrompt` (single-line prompt, reused for subject/title/quick-capture)
and `MultiNotePicker` (multi-select note picker, for note links). Both are
small, mirror the existing DatePicker/TaskPicker/NotePicker shape, and stay
on the mini-TUI-components track — not the deprioritized full-screen `rk
tui` layout (reckon-nzk3).

## Deferred (non-blocking, from code review)
- `rk note create` wizard echoes an untrimmed `Title` in `--json`/pretty
  output (the written *file* is unaffected — round-trips to the same
  bytes). Optional: move the trim into `normalizeNoteCreateParams`.
- `MultiNotePicker` selection order is nondeterministic (map iteration) --
  no correctness impact since the only convergence test carries no links,
  but consider sorting for stable body output.
- Esc-back to an earlier wizard step re-mounts a blank component rather
  than re-priming from the prior entry — the Wizard framework supports
  re-priming (factories can read `prior`), these three drivers just don't
  use it. `TextPrompt.SetValue` (the dead code this would have used) was
  removed rather than wired, since Esc-back-loses-input is an accepted v1
  UX cost per code review.
