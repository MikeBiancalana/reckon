# Implementation Plan: reckon-fnqs.7 — Interactive create flows over the v1 verbs

## Summary

Wire bare-invocation wizard/prompt create flows onto `rk todo add`, `rk note create`, `rk add`, reusing the fnqs.6 `Prompt[T]`/`Wizard`/`RunPrompt` layer and the reckon-6k0l `todo list` dispatch pattern (`internal/cli/todo_browse.go`). Each verb's `RunE` gains a top-of-function branch: a pure `…WantsTUI` predicate (mirrors `todoListWantsTUI`, `todo_browse.go:19`) routes bare-on-TTY invocations to a new wizard driver that collects values through composable components and calls the **same** write function the flag path calls (`addDurableTodo`/`createNote`/`appendLogEntry`) — no second write path. Two net-new components are required (single-line text prompt; multi-select note picker); by choosing `Form` for the two date fields and a synthetic "(none)" row for depends, **no existing shared component is modified.**

## Design decisions (points 1–5 resolved)

**1. New components — IN SCOPE (two files).** Build (a) a single-line `Prompt[string]` (`TextPrompt`, wraps `bubbles/textinput`, mirrors `DatePicker`'s struct shape minus date parsing) reused 3× for subject/title/quick-capture, with a `Required` flag; and (b) a `MultiNotePicker` (`Prompt[[]string]`, Space-toggle/Enter-confirm, mirrors `NotePicker`'s list+delegate at `note_picker.go`). Rationale: the ticket's step list presupposes these exist; small reusable `Prompt[T]` components are the still-live direction (reckon-nzk3 deprioritized only the full-screen multi-pane layout). *Alternative rejected:* reusing `Form` with one field for the text prompt — works but yields a `FormResult` where a bare string is wanted, and `rk add` must be a lone prompt, not a form.

**2. scheduled+deadline — ONE `Form` step, two `Required:false FieldTypeDate` fields.** Rationale: `Form` already supports blank-optional submit (`form.go:281-283`) and per-keystroke date preview (`form.go:84,227-229`), so it sidesteps `DatePicker`'s empty-Enter-blocks gap (§3.3) with **zero component modification**; a Form-inside-Wizard is already proven (`wizard_test.go:44-49`). AC §5 item 2 pre-blesses collapsing the two narrated "steps." *Alternative rejected:* two chained `DatePicker` steps — requires adding an opt-in skip path to a component legacy `rk tui` still drives, for no preview benefit.

**3. depends skip — synthetic "(no dependency)" row, IN the CLI-built row slice.** The wizard prepends `IndexRow{ID:"", Title:"(no dependency)"}` to the rows passed to `TaskPicker.Show()`. Selecting it sets `selectedTask` (non-nil, `ID:""`) → `Done()` reports finished (`task_picker.go:219-221`), `Result()` returns `""` (`:192-197,216`); ULIDs are always non-empty so `""` unambiguously means no link. Rationale: zero `TaskPicker` modification, and the user finishes with no-dependency by *selecting* (Enter), never through Esc — directly answering §3.3's "Esc-back returns to deadline" worry. *Alternative rejected:* a bound skip key — needs component modification and new key semantics.

**4. pinned bare+non-TTY — CONFIRM the AC recommendation.** All three predicates return `false` when `!isInteractive()`, so bare+non-TTY falls into the classic body (an ordinary error, not the `PromptGuard` wording); `--no-input` is never consulted in the predicate (stays enforced inside `RunPrompt`, matching `todo_browse.go:16-18`). `todo add`/`add` already error `"empty body text"` (`todo.go:294`, `add.go:100`). `note create` needs a **new** ordinary guard `"note create: title must not be empty"` in `runNoteCreateE` because loosening `Args` (decision below) makes 0-args-non-interactive newly reachable.

**5. note-create convergence — extract a shared `normalizeNoteCreateParams` seam.** Both `runNoteCreateE` and the wizard conversion call one helper performing the load-bearing transforms from `note_v1.go:240-259`: slug = `slugify(title)` when no slug override, `validateSlug`, `Type` default `"note"`, stage validation, and body trailing-newline (`body += "\n"`). Rationale: guarantees the wizard reuses slugify+newline rather than passing raw title/body (AC §2.8). *Do not* over-unify the subject/body join — `assembleBody`'s `-m` path is an N-message join (`body_entry.go:72-76`), semantically distinct from the wizard's 2-value formula; add a separate `joinSubjectBody(subject, body)` for the wizard only and leave `assembleBody` untouched.

## Wizard step → key → type contract (linchpin of T11–T20)

Conversion functions live in `internal/cli`, read the result map with comma-ok assertions (map is fully populated only when `ok=true`).

**`rk todo add`** — `Wizard`, 4 steps → `wizardTodoAddArgs(results) (body, scheduled, deadline, depends string, err error)`:

| key | component | `Prompt[T]` | notes |
|---|---|---|---|
| `subject` | `TextPrompt` (Required) | `string` | Enter submits; empty blocked in component |
| `body` | `TextEditor` | `string` | Ctrl+D submits; empty allowed |
| `dates` | `Form` (fields `scheduled`,`deadline`, both `FieldTypeDate Required:false`) | `FormResult` | Tab between fields, Enter submits both; blank allowed |
| `depends` | `TaskPicker` (rows = open durable todos, "(no dependency)" prepended) | `string` | selecting none-row → `""` |

Conversion: `body = joinSubjectBody(results["subject"].(string), results["body"].(string))`; `scheduled/deadline = normalizeWizardDate(results["dates"].(FormResult).Values["scheduled"|"deadline"])`; `depends = results["depends"].(string)`; `repeat = ""` (no step); `author = resolveAuthor("")`. Then `addDurableTodo(todosDir, author, body, scheduled, deadline, depends, "")`.

`normalizeWizardDate(s)`: blank → `""`; else `ParseRelativeDate(s)` then `.UTC().Format("2006-01-02")` (AC §2.7; the flag path stores `--scheduled` verbatim since `parseSchedDate` runs only in the `repeat!=""` branch, `todo.go:300-310`).

**`rk note create`** — `Wizard`, 3 steps → `wizardNoteParams(results) noteCreateParams`:

| key | component | `Prompt[T]` |
|---|---|---|
| `title` | `TextPrompt` (Required) | `string` |
| `body` | `TextEditor` | `string` |
| `links` | `MultiNotePicker` (rows = existing notes) | `[]string` (slugs) |

Conversion: append `[[slug]]` tokens (own paragraph) to body for each selected slug, build a raw `noteCreateParams{Title, Body, Author: resolveAuthor("")}`, pass through `normalizeNoteCreateParams`, then `createNote(notesDir, params)`. (Link-token exact format is UX, not byte-identity-constrained — T19 carries no links.)

**`rk add`** — single `RunPrompt[string]` (NOT a Wizard, AC §1.3.1): `TextPrompt` (Required=false) → `RunPrompt[string]`; `body = strings.TrimSpace(capture)`; then `resolveAuthor`/`effectiveLogDate`/`resolveAtTime`/`appendLogEntry` exactly as `runAddE`'s tail. Empty submit flows through to `runAddE`'s existing empty-body guard (AC §3.5).

## Files to create

| File | Reason |
|---|---|
| `internal/tui/components/text_prompt.go` | Single-line `Prompt[string]` with `Required`; subject/title/quick-capture (decision 1a). Priming synchronous in `Show()` (fnqs.6 R1). |
| `internal/tui/components/multi_note_picker.go` | `Prompt[[]string]` multi-select for links (decision 1b). Highest-risk new surface. |
| `internal/cli/todo_add_wizard.go` | `todoAddWantsTUI(cmd,args)`, `runTodoAddWizard(cmd)`, `wizardTodoAddArgs`, `normalizeWizardDate`, `buildDependsRows(cfg)`; mirrors `todo_browse.go`. |
| `internal/cli/note_create_wizard.go` | `noteCreateWantsTUI(cmd,args)`, `runNoteCreateWizard(cmd)`, `wizardNoteParams`, `buildNoteLinkRows(cfg)`. |
| `internal/cli/add_wizard.go` | `addWantsTUI(cmd,args)`, `runAddWizard(cmd)` (single-prompt driver). |
| `internal/tui/components/text_prompt_test.go`, `multi_note_picker_test.go` | Component keystroke tests (`tea.WithInput`/`WithOutput`), incl. Required/empty and none/empty-selection cases (gaps G2/G5). |
| `internal/cli/*_wizard_test.go` (3) | Dispatch-predicate + convergence tests (T1-analogs, T11-T20). |

## Files to modify

| File | Change |
|---|---|
| `internal/cli/todo.go` | Insert `if todoAddWantsTUI(cmd,args) { return runTodoAddWizard(cmd) }` after `defer resetTodoFlags` (~:277). |
| `internal/cli/note_v1.go` | `Args: cobra.MinimumNArgs(1)` → `cobra.ArbitraryArgs` (:81); insert dispatch branch + new empty-title guard in `runNoteCreateE`; extract `normalizeNoteCreateParams` from :240-259 (both `runNoteCreateE` and wizard call it). |
| `internal/cli/add.go` | Insert `if addWantsTUI(cmd,args) { return runAddWizard(cmd) }` after `defer resetAddFlags` (~:90). |
| `internal/cli/body_entry.go` | Add `joinSubjectBody(subject, body string) string` (wizard-only; `assembleBody` untouched). *May instead live in `todo_add_wizard.go`.* |

**Dispatch predicate flag sets** (any `cmd.Flags().Changed(name)` → classic; mirrors AC §1.1.3/1.2.4/1.3.3): todo add `{ephemeral,scheduled,deadline,depends,repeat,author,message,edit}`; note create `{slug,type,author,stage,description,dir,tag,alias,body}`; add `{author,at,date,message,edit}` (`date` is the global persistent flag, reachable via `cmd.Flags().Changed`). Each also returns `false` if `len(args)>0`. Cancellation (`ok=false`) → `RunE` returns `nil`, no write (AC §2.5, `todo_browse.go:56-58`).

*Optional:* `internal/cli/AGENTS.md:229-260` stale interactive sketch (out of scope per AC §5.5; fold in opportunistically only).

## Implementation steps (sequenced)

1. `TextPrompt` component + tests (unblocks all three verbs).
2. `MultiNotePicker` component + tests (unblocks note links; highest-risk — do early to surface problems).
3. Extract `normalizeNoteCreateParams` in `note_v1.go` (pure refactor, no behavior change — existing note tests must stay green).
4. `todo_add_wizard.go` (predicate + driver + `wizardTodoAddArgs` + `normalizeWizardDate` + `buildDependsRows`) using `Form` for dates and none-row for depends; insert todo.go branch.
5. `note_create_wizard.go` + Args loosening + empty-title guard; insert note_v1.go branch.
6. `add_wizard.go` + insert add.go branch.
7. Dispatch/convergence tests per verb.

Row builders (`buildDependsRows`, `buildNoteLinkRows`) mirror `buildTodoItems` (`todo_browse.go:66`): open index → `Reconcile()` → query (`listDurableTodos(ix.DB(),false,"")` for depends; notes via `SELECT id,loc FROM nodes WHERE type='note'` per `note_v1.go:742`) → map to `[]IndexRow` (depends Props `scheduled`/`deadline`; links Props `slug`) → `Close()`. Query failure propagates as a normal `RunE` error before the wizard opens (AC §3.4).

## Test scenarios (AC §4 T1–T20 mapping + gaps)

- **Dispatch (stub `isInteractive`, `RootCmd.Execute`):** T1 (todo add bare→wizard: assert via the pure `todoAddWantsTUI` predicate, not a keystroke session), T2 (arg→classic, no ANSI), T3 (non-TTY bare→`empty body text`), T4 (TTY+`--no-input`→`PromptGuard` error, proving guard inside `RunPrompt` is reached), T5 (note create non-TTY bare→new empty-title error), T6 (`--tag`→classic missing-title), T7 (`--at`→classic no ANSI), T14 (`--ephemeral`→classic `addEphemeralTodo`).
- **Component (scripted keys):** T8 (full todo wizard → map), T9 (Esc mid-flow steps back), T10 (Esc step 0 cancels), T15 (`MultiNotePicker` 2 selections → 2 `[[slug]]` tokens parsed by `wikilinkRe`).
- **Pure conversion:** T11/T12 (`wizardTodoAddArgs` subject/body formula), T16 (add trim), T17 (`normalizeWizardDate` → `2006-01-02`, fixed non-UTC `now`, §3.6).
- **Convergence (flag vs wizard):** T13 (`joinSubjectBody` vs `assembleBody` byte-identical body), T18 (todo file byte-identical modulo ULID), T19 (note file byte-identical — proves `normalizeNoteCreateParams` reuse), T20 (add — assert body/author only per its timestamp caveat).

**Gaps in the T1–T20 set to fix:**
- **G1** — no positive dispatch test for note-create-bare-TTY / add-bare-TTY (T1 analogs). Add `noteCreateWantsTUI`/`addWantsTUI` predicate unit tests.
- **G2** — `TextPrompt` Required/empty behavior is only exercised mid-wizard (T8). Add standalone tests: Required blocks empty submit; non-Required submits `""`.
- **G3** — no test that selecting the "(no dependency)" row yields empty depends. Add a component/conversion variant.
- **G4** (not new tests, but **T8 and T11's fixtures must be rewritten** for the Form-collapsed dates contract): T8's script becomes subject→body→one blank `Form` submit→depends, and the result map has **four** keys (`subject/body/dates/depends`), with `dates` a `FormResult` — not the doc's five string keys; T11's synthetic map must use `results["dates"]=FormResult{Values:{...}}`, not flat `"scheduled"`/`"deadline"` keys.
- **G5** — no test that empty `MultiNotePicker` selection (skip links) leaves body token-free.

## Known risks / ambiguities

- **R1 — `MultiNotePicker` is the largest net-new, least-covered surface** (T15 is its only scenario, itself gated). If it slips, fallback options in priority order: loop the single-select `NotePicker`, or defer links to a follow-up (links are optional and not part of the byte-identity criterion). [OPEN — confirm build vs defer with owner if timeboxed.]
- **R2 — relative-date TZ edge (§3.6):** `ParseRelativeDate` resolves `t`/`+3d` in local time, then `.UTC().Format` may land one calendar day off near local midnight. Verify with a fixed non-UTC `now` (T17); if it surfaces, normalize `normalizeWizardDate`'s resolution to UTC or document. [OPEN — implementer's call.]
- **R3 — closure-capture (`REVIEW_PATTERNS.md:117`):** `StepFactory` closures reading `prior[...]` must capture the value before any `tea.Cmd` closure.
- **R4 — fnqs.6 R1/R2:** every factory must call `Show()` and prime synchronously (`Init()` is a no-op); no empty wizard steps, or `RunPrompt` hangs at mount.
- **R5 — Form stores raw date strings:** conversion re-parses via `ParseRelativeDate`; `Form` already validated on submit so it should not error, but handle the error defensively.

### Critical Files for Implementation
- /home/chadd/repos/reckon/.worktrees/reckon-fnqs.7/internal/cli/todo_browse.go
- /home/chadd/repos/reckon/.worktrees/reckon-fnqs.7/internal/cli/note_v1.go
- /home/chadd/repos/reckon/.worktrees/reckon-fnqs.7/internal/tui/components/wizard.go
- /home/chadd/repos/reckon/.worktrees/reckon-fnqs.7/internal/tui/components/form.go
- /home/chadd/repos/reckon/.worktrees/reckon-fnqs.7/internal/tui/components/task_picker.go
