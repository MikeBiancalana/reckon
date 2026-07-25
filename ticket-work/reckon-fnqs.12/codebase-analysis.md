# Codebase Analysis: reckon-fnqs.12 — `rk checklist run` interactive mini-TUI

## 1. `internal/tui/components/prompt.go` and `wizard.go`

### `Prompt[T]` interface (prompt.go:9-21)

```go
type Prompt[T any] interface {
    Init() tea.Cmd
    Update(tea.Msg) (Prompt[T], tea.Cmd)
    View() string
    Result() T
    Done() (finished, canceled bool)
}
```

- Each concrete component's own `Update` returns itself as `Prompt[T]` directly — no shared adapter type. Conformance is asserted with `var _ Prompt[T] = (*Concrete)(nil)` (see prompt_test.go:20-26).
- `Result()` is only meaningful once `Done()` reports `finished`.
- `Done()` reports at most one of `finished`/`canceled` true at a time.

### `IndexRow` (prompt.go:23-32) — the pre-existing display-row type

```go
type IndexRow struct {
    ID    string
    Title string
    Type  string
    Props map[string]string
}
```

This is the exact shape the ticket's `ChecklistItem{Text string; Checked bool}` should mirror in spirit (small, display-only, decoupled from domain model). It is **not** reusable as-is for checklist items — it has no `Checked` field and its semantics (ID/Title/Type/Props) don't fit a checklist row. A new type in components (e.g. `ChecklistItem`) is warranted, matching the ticket's own suggested shape.

### `PromptGuard` / TTY guard mechanism (prompt.go:34-39, 78-84)

```go
var PromptGuard func() error
```

- `nil` by default (unit tests need no setup).
- Checked by `RunPrompt` **before** it ever opens a `tea.Program` (prompt.go:79-84) — guard fires pre-flight, no partial UI flash.
- Wired once, at process entry, from `internal/cli` (see §3) — `internal/tui/components` never imports `internal/cli`.

### `RunPrompt[T]` signature (prompt.go:78)

```go
func RunPrompt[T any](p Prompt[T], opts ...tea.ProgramOption) (result T, ok bool, err error)
```

- Blocks until `p` reports `Done()`.
- `opts` is empty in production; tests inject `tea.WithInput`/`tea.WithOutput` to script keystrokes (prompt_test.go:74-79, 94-98) — required because a real `tea.Program` hangs/fails setting raw mode on a non-TTY.
- Internally builds a `runPromptHost[T]` (prompt.go:44-71) that drives `p`, and on `finished` captures `Result()`+quits, on `canceled` quits leaving the zero value.
- Return shape: 3-tuple `(T, bool, error)` — matches the deleted v0 `PickTask`/`PickNote` shape per fnqs.6's own acceptance criteria (ticket-work/reckon-fnqs.6/acceptance-criteria.md:32), so this is an established, deliberate convention to keep.

### `Wizard` (wizard.go)

Not needed for this ticket (single-step interaction, not a multi-step form chain), but relevant if any future flow (e.g. "create template" wizard) is added. `Wizard` itself implements `Prompt[map[string]any]` and is driven via `Wizard.Run(opts...)` → `RunPrompt[map[string]any](w, opts...)` (wizard.go:166-168). Not directly applicable to `ChecklistRunner`.

### How TaskPicker/NotePicker implement `Prompt[T]` and retarget `IndexRow`

Both `TaskPicker` (task_picker.go) and `NotePicker` (note_picker.go) are structurally identical:

| Method | TaskPicker (file:line) | Notes |
|---|---|---|
| `Init() tea.Cmd` | task_picker.go:212 | returns `nil` — priming happens via `Show()` before hand-off to `RunPrompt` |
| `Update(tea.Msg) (Prompt[string], tea.Cmd)` | task_picker.go:224-267 | `tea.KeyEsc` → `canceled=true`, quit-signaling cmd; `tea.KeyEnter` → reads `list.SelectedItem()`, sets `selectedTask`, quit-signaling cmd; else delegates to `bubbles/list.Model.Update` |
| `View() string` | task_picker.go:270-287 | delegates to `list.View()` + static help line, wrapped in a lipgloss box |
| `Result() string` | task_picker.go:216 | returns `GetSelectedTaskID()` |
| `Done() (bool, bool)` | task_picker.go:219-221 | `(tp.selectedTask != nil, tp.canceled)` |
| `Show(rows []IndexRow) tea.Cmd` | task_picker.go:163-178 | the priming/reset entry point — converts `[]IndexRow` into `[]list.Item` internally via a private `taskPickerItem{row IndexRow}` wrapper |

Both are declared to conform via `var _ Prompt[string] = (*TaskPicker)(nil)` / `(*NotePicker)(nil)` in prompt_test.go:24-25.

**The "IndexRow retargeting" pattern** (what the ticket means by this term): the picker component itself never imports/touches the richer domain type (`*models.Note`, task domain type, etc). Instead, the **CLI/verb layer** owns a small converter function that maps the real domain slice into `[]IndexRow` right before calling `Show()`. The concrete precedent is `notesToRows` in `internal/cli/tui_model.go:118-140`:

```go
// notesToRows converts listNotes' models.Note rows into the
// components.IndexRow shape NotePicker.Show takes.
func notesToRows(notes []*models.Note) []components.IndexRow {
    rows := make([]components.IndexRow, len(notes))
    for i, n := range notes {
        props := map[string]string{
            "slug": n.Slug,
            "tags": strings.Join(n.Tags, ", "),
        }
        if !n.CreatedAt.IsZero() {
            props["created"] = n.CreatedAt.Format("2006-01-02")
        }
        rows[i] = components.IndexRow{
            ID:    n.ID,
            Title: n.Title,
            Type:  "note",
            Props: props,
        }
    }
    return rows
}
```
Called at `tui_model.go:198` (`m.notes.picker.Show(notesToRows(msg.notes))`) and `tui_keyboard.go:192`. Tested directly in `tui_test.go:1144` (`TestNotesToRowsOmitsZeroCreatedAt`) — a plain unit test on the converter function, no bubbletea driving required.

**Applying this to fnqs.12**: define `components.ChecklistItem{Text string; Checked bool}` in `internal/tui/components` (new file, e.g. `checklist_runner.go`), a `ChecklistRunner` component implementing `Prompt[???]` (see §5 for open question on result type) that only ever sees `[]ChecklistItem`, and a converter in `internal/cli/checklist.go` (mirroring `notesToRows`) that maps `[]checklist.RunItem` → `[]components.ChecklistItem`. `ChecklistRunner` must never import `internal/checklist`.

**Important structural note**: unlike `IndexRow`'s current live callers, `TaskPicker`/`NotePicker` are driven **inline inside the big full-screen `tuiModel`** (`internal/cli/tui_panes.go`), not through `RunPrompt`. `RunPrompt`/`Wizard` currently have **zero live callers anywhere in `internal/cli`** (confirmed by grep — only mentioned in comments/tests). fnqs.12 will be the first real-world wiring of `RunPrompt` into a cobra `RunE`.

---

## 2. `internal/checklist` package

### Types (model.go)

```go
type RunStatus string
const (
    RunStatusActive    RunStatus = "active"
    RunStatusCompleted RunStatus = "completed"
    RunStatusAbandoned RunStatus = "abandoned"
)

type Run struct {
    ID           string
    TemplateID   string
    TemplateName string
    Status       RunStatus
    Items        []RunItem
    StartedAt    time.Time
    CompletedAt  *time.Time
}

type RunItem struct {
    ID             string
    RunID          string
    TemplateItemID string
    Text           string
    Position       int
    Checked        bool
    CheckedAt      *time.Time
}
```

`Run.Items` is `[]RunItem` (value slice, 0-indexed by `Position`, matching slice index in practice — `NewRun` assigns `Position: i` in template order).

### Service methods (service.go) relevant to `checklist run`

| Method | Signature | Behavior |
|---|---|---|
| Resume-or-fresh (no single method — verb composes two calls) | `GetActiveRun(nameOrID string) (*Run, error)` then fallback `StartRun(nameOrID string) (*Run, error)` | `GetActiveRun` (service.go:129-143) errors (`"no active run for %q"`) if none exists — it does **not** return `(nil, nil)`. `StartRun` (service.go:107-126) errors if an active run *already* exists. The existing `runChecklistStartE` (checklist.go:410-437) is the canonical resume-or-start composition: try `GetActiveRun`, on error call `StartRun`. |
| Toggle an item | `CheckItem(runID string, position int) error` (service.go:147-184) | Toggles `Checked` at 0-based `position` (`!item.Checked`, i.e. **toggle**, not set-true). Auto-completes the run (`RunStatusCompleted`) when `allChecked(items)` becomes true after the toggle. Does not return the updated run — caller must re-fetch. |
| Fetch by run ID | `GetRunStatus(runID string) (*Run, error)` (service.go:187-189) | Thin wrapper over `repo.GetRunByID`. Used after `CheckItem` to get fresh `Checked`/`Status` state (checklist.go:467-472 does exactly this — comment there explains why: re-calling `GetActiveRun` would error once the run auto-completes). |
| Get template (needed to resolve name→ID before run lookups) | `GetTemplate(nameOrID string) (*Template, error)` (service.go:43-55) | Tries name first, falls back to ID. |
| Abandon | `AbandonRun(nameOrID string) (*Run, error)` (service.go:218-237) | Marks active run abandoned; errors if none active. |
| Reset | `ResetRun(nameOrID string) (*Run, error)` (service.go:192-213) | Abandons any active run + starts fresh, unconditionally (no error if none active). |

**No single "resume-or-start" Service method exists** — the CLI verb layer composes `GetActiveRun` + `StartRun` itself (this is exactly `runChecklistStartE`'s body, checklist.go:421-436, and matches the deleted v0 helper `resolveChecklistRun` recovered via `git show 83ca4ef^:internal/cli/checklist_run_helper.go` — see §4). `ChecklistRunner`'s launcher in the new verb should replicate this same two-call composition (or factor a tiny local helper), not add a new Service method — fnqs.11 already shipped this surface and the ticket's scope is UI-layer only.

**Position/0-index note for the TUI — verified**: `CheckItem(runID, position)` takes a 0-based index and does `run.Items[position]` (service.go:157) — a slice index, not a query by `Position` field. This is safe: `GetRunItems` (repository.go:319-323) queries `SELECT ... FROM checklist_run_items WHERE run_id = ? ORDER BY position ASC`, and `GetRunByID` (repository.go:222-249) calls `GetRunItems` to populate `Run.Items`. So `Run.Items[i].Position == i` always holds for any run fetched through the Service, and the mini-TUI's cursor index maps 1:1 to `CheckItem`'s `position` argument with no translation needed.

---

## 3. `internal/cli` — where `checklist run` wiring slots in

### Current file: `internal/cli/checklist.go` (547 lines)

Cobra commands registered today (checklist.go:49-134): `create`, `list`, `start`, `check`, `status`, `reset`, `abandon`. **No `run` subcommand exists yet** — this ticket adds it.

Each `RunE` follows the same skeleton:
```go
func runChecklistXxxE(cmd *cobra.Command, args []string) error {
    defer resetChecklistFlags(cmd)
    name := args[0]
    mode, svc, db, err := setupChecklistRun()   // checklist.go:158-168
    if err != nil { return err }
    defer db.Close()
    // ... svc.Method(...) ...
    return printChecklistResult(cmd, mode, checklistRunResult{Run: run})
}
```
`setupChecklistRun()` (checklist.go:158-168) resolves output mode + opens `*checklist.Service` + `*storage.Database` — this is the exact pair the new `run` verb's non-TTY/`--no-input` fallback path (if any) or its interactive setup would also call, to reuse the same DB-open/service-construction plumbing.

**New `checklistRunCmd`** should be added alongside the existing 7, in the same `init()` block (checklist.go:117-134), with `Use: "run <template>"`, `Args: cobra.ExactArgs(1)`.

### TTY guard: how it actually works (interactive.go, 42 lines total)

```go
var isInteractive = func() bool {
    return isCharDevice(os.Stdin) && isCharDevice(os.Stdout)
}

func promptGuard() error {
    if noInputFlag || !isInteractive() {
        return fmt.Errorf("cannot show an interactive prompt: pass --no-input or run from an interactive terminal")
    }
    return nil
}

func init() {
    components.PromptGuard = promptGuard
}
```

- `noInputFlag` is a `RootCmd.PersistentFlags().BoolVar(&noInputFlag, "no-input", false, ...)` global (root.go:32, 99) — already available to every subcommand, including a new `checklist run`.
- **Critically**: the guard is invoked *inside* `RunPrompt`/`Wizard.Run`, not via `RootCmd.PersistentPreRunE` — confirmed explicitly by `TestPromptGuard_NotInvokedForNonPromptingVerb` (interactive_test.go:55-77) and the comment at interactive.go:28-31. This means **the new `checklist run` verb needs no manual TTY/`--no-input` check of its own** — it just calls `components.RunPrompt[T](runner)`, and the guard fires automatically before any `tea.Program` opens, returning the guard's error straight up through `RunPrompt`'s `err` return.

### fnqs.7's "bare-vs-flagged" convention — [OPEN] no code precedent exists

`bd show reckon-fnqs.7` shows **status OPEN** (not started) — it is a sibling ticket, not a dependency, and has shipped zero code. Its ticket text is the only source for "bare-vs-flagged":

> "When stdin is a TTY and the verb is invoked bare: `rk todo add` → wizard... With args/flags, behavior is exactly as today: non-interactive, agent-safe."

`grep` confirms **no verb anywhere in `internal/cli` currently branches on `isInteractive()` or calls `RunPrompt`/`Wizard`** — the convention is a design principle stated in two ticket bodies (fnqs.7 and fnqs.12), not an implemented pattern to copy code from. [INFERRED] For `checklist run` specifically: since `checklist start`/`checklist check`/etc. already exist as separate, permanently-non-interactive verbs (shipped by fnqs.11), "with args/flags... stays non-interactive" most plausibly means **`checklist run` itself takes no extra data flags** — bare + TTY opens the TUI; non-TTY or `--no-input` errors via the guard (exactly like fnqs.7's todo/note wizards, whose non-interactive fallback is simply "the same verb the flags already drove," not a special-cased branch inside `run` itself). There is no evidence of a flag like `--check <position>` being added to `run` to give it a non-interactive mode of its own — the non-interactive equivalents already live at `checklist start`/`checklist check`. **[OPEN]**: confirm this reading before implementing; the ticket's "with args/flags equivalent to the fnqs.11 verbs" phrasing is the one under-specified part of the scope.

### `internal/cli/AGENTS.md`

Documents `isTTY()`/bare-invocation-launches-form as a **generic pattern to follow** (AGENTS.md:229-260) — consistent with, but written before, fnqs.6's actual `PromptGuard` mechanism. Its example code (`os.Stdout.Stat()` only, no stdin check) is stale relative to `isInteractive()`'s actual dual stdin+stdout check (interactive.go:16-18) — follow the real `interactive.go` implementation, not the AGENTS.md snippet.

---

## 4. Reference `tea.Model` for list+checkbox rendering — v0's deleted `checklistRunModel`

The v0 inline TUI (`internal/cli/checklist_run_helper.go`) was deleted by commit `83ca4ef` ("Retire the v0 DB-primary verb surface (reckon-fnqs.1)", #158), but is fully recoverable via `git show 83ca4ef^:internal/cli/checklist_run_helper.go`. **This is reference material only — do not resurrect it as-is**: it directly embeds `*checklist.Service` and `*checklist.Run` in the model, exactly the coupling this ticket's design constraint forbids. Its keybinding scheme and rendering shape are still a useful design reference:

```go
type checklistRunModel struct {
    service   *checklist.Service
    run       *checklist.Run
    cursor    int
    completed bool
    abandoned bool
    canceled  bool
    err       error
}
```

- `Update`: `"ctrl+c", "q", "esc"` → `canceled=true`, quit. `"a"` → call `service.AbandonRun`, quit. `"j"/"down"` → `cursor++` (clamped to `len(Items)-1`). `"k"/"up"` → `cursor--` (clamped to 0). `" ", "enter"` → `service.CheckItem(run.ID, cursor)`, re-fetch via `GetRunStatus`, check `Status == RunStatusCompleted` → set `completed=true`, quit.
- `View`: builds `[]string` lines — template name header, one line per item (`[ ]`/`[x]` + position + text, cursor rendered as a styled `>` prefix), then either a hint line (`"j/k: move  space/enter: toggle  a: abandon  q: quit"`, dim-styled) or, if completed, `"✓ Complete!"` **followed by an explicit trailing blank line** — commented as working around bubbletea's renderer erasing the last line on exit, so the completion message survives in scrollback.
- Launcher: synchronous, no `tea.WithAltScreen()`, `tea.NewProgram(m).Run()`, type-asserts the final model back to `checklistRunModel`.

**Mapping onto fnqs.12's actual required shape**: v0 called `service.CheckItem`/`service.AbandonRun` directly *inside* the bubbletea model's `Update` — i.e., gave the TUI a live Service dependency, exactly what this ticket forbids. `ChecklistRunner` must instead hold only `[]ChecklistItem` + `cursor int`, and the **verb layer** owns the real `*checklist.Service` calls. **[OPEN, planning-phase]**: per-toggle persistence (verb-layer host intercepts a toggle message from `ChecklistRunner.Update`, calls `svc.CheckItem` synchronously, feeds the refreshed item list back in) vs. batch-at-`Result()` persistence. Recommend per-toggle — it's the only shape that gets live auto-complete detection (matching v0's `completed`-triggers-quit behavior) without duplicating `allChecked` client-side.

No other bubbletea `tea.Model` with checkbox-style list rendering exists live in the repo (grep for "checkbox"/`Checked`/`[ ]`/`[x]` across `internal/tui` found nothing besides `FORM_README.md` prose and the recovered v0 file above).

---

## 5. Pitfalls from docs/REVIEW_PATTERNS.md and internal/cli/AGENTS.md

Relevant, in order of applicability to this ticket:

1. **Unconditional newline join with optional strings** (REVIEW_PATTERNS.md:879-901, discovered reckon-qxem). `a + "\n" + optional + "\n" + b` produces a phantom blank line when `optional == ""`. Directly relevant to `ChecklistRunner.View()`: build lines as `[]string` and `strings.Join(lines, "\n")`, only appending the hint/status line conditionally — exactly the same pitfall the yk1i codebase-analysis.md flagged for this exact component (ticket-work/reckon-yk1i/codebase-analysis.md, "Note the Unconditional Newline Join pitfall").

2. **Closure capture bug** (REVIEW_PATTERNS.md:117-135, "very common — 18 occurrences", flagged prominently for `internal/tui/AGENTS.md` though that file doesn't currently exist in this checkout — [OPEN] confirm: `find` shows no `internal/tui/AGENTS.md` on disk despite REVIEW_PATTERNS.md citing it). If `ChecklistRunner`'s toggle handling returns a `tea.Cmd` closure that reads mutable model fields (e.g. `m.cursor`, `m.run`) at call time rather than capturing them by value first, a race/staleness bug results. Capture needed values into locals before returning the `func() tea.Msg { ... }` closure.

3. **Index-only selection identity after re-sort / list reload** (REVIEW_PATTERNS.md:955-978). Not directly applicable if item order is stable (`Position`-sorted, never re-sorted at runtime) — checklist items don't reorder within a run, so cursor-by-index should be safe. Worth a one-line acknowledgment in the plan, not a redesign.

4. **Nil component access** (REVIEW_PATTERNS.md:147-165). If `ChecklistRunner` is constructed before `Show()`/priming (mirroring `TaskPicker`/`NotePicker`'s `Init()` returning `nil` because `Show()` already primed state — task_picker.go:210-212), guard any nil-slice access in `View()`/`Update()` for the zero-items case (empty template — the old v0 model explicitly handled `len(m.run.Items) == 0` at checklist_run_helper.go, rendering `"(no items)"`).

5. **`internal/cli/AGENTS.md`'s package-global anti-pattern warning** (AGENTS.md:53-77) is **stale relative to checklist.go's actual pattern** — `checklist.go` already uses per-command `openChecklistService()`/`setupChecklistRun()` (checklist.go:142-168), not root.go package globals, for the checklist verb family specifically (root.go still has other legacy globals, but checklist avoided that). Follow `setupChecklistRun()`'s existing pattern for the new `run` verb, not AGENTS.md's generic global-service example.

6. **`--quiet`/output-mode conventions**: every existing checklist verb goes through `printChecklistResult(cmd, mode, res)` (checklist.go:173-178) which suppresses Pretty output under `--quiet`. Since `checklist run`'s primary "output" *is* the interactive TUI itself (not machine-parseable), [OPEN]: decide whether `run` even supports `--json`/`--ndjson`/`--quiet` at all, or whether those modes are simply incompatible with an interactive verb (most likely: `--no-input` combined with `--json`/`--ndjson` would need a defined non-interactive behavior too, per point 3 in §3 above — this is the same open question about what "flagged" mode for `run` actually does).

7. No test file for `checklist.go` skips existing verb patterns worth mirroring for the new command's tests: `internal/cli/checklist_test.go` (existing, not read in full here) almost certainly has per-verb test functions following the repo's cobra `cmd.SetArgs(...)` + `cmd.Execute()` convention documented in AGENTS.md:328-361 — the new `run` verb's tests will need the `tea.WithInput`/`tea.WithOutput` seam (per prompt_test.go's `runPromptForTest` helper, §1 above) rather than that convention, since it drives a real `tea.Program`.

---

## Summary of concrete file/line pointers for implementation

| Concern | File:Line |
|---|---|
| `Prompt[T]` interface | internal/tui/components/prompt.go:9-21 |
| `RunPrompt[T]` | internal/tui/components/prompt.go:78-99 |
| `PromptGuard` var + guard-before-Program check | internal/tui/components/prompt.go:39, 79-84 |
| `IndexRow` (shape precedent, not reusable type) | internal/tui/components/prompt.go:27-32 |
| TaskPicker Prompt[string] impl | internal/tui/components/task_picker.go:212-267 |
| NotePicker Prompt[string] impl | internal/tui/components/note_picker.go:256-311 |
| `notesToRows` converter (retargeting precedent) | internal/cli/tui_model.go:118-140 |
| `noInputFlag` registration | internal/cli/root.go:32, 99 |
| `isInteractive`/`promptGuard`/guard wiring | internal/cli/interactive.go (whole file, 42 lines) |
| Existing checklist cobra commands | internal/cli/checklist.go:49-134 |
| `setupChecklistRun`/`openChecklistService` | internal/cli/checklist.go:142-168 |
| Resume-or-start composition precedent | internal/cli/checklist.go:410-437 (`runChecklistStartE`) |
| `Service.GetActiveRun`/`StartRun`/`CheckItem`/`GetRunStatus` | internal/checklist/service.go:107-189 |
| `Run`/`RunItem` model | internal/checklist/model.go:57-105 |
| Deleted v0 reference TUI (recoverable, not reusable as-is) | `git show 83ca4ef^:internal/cli/checklist_run_helper.go` |
| Newline-join pitfall | docs/REVIEW_PATTERNS.md:879-901 |
| Closure-capture pitfall | docs/REVIEW_PATTERNS.md:117-135 |

## Open questions for the plan phase

- **[OPEN]** Exact meaning of "with args/flags equivalent to the fnqs.11 verbs, behavior stays non-interactive" for a verb (`run`) that itself has no obvious flag-driven data path today (unlike `todo add`/`note create` in fnqs.7, which have real field flags). Recommend confirming with the ticket owner whether `run` takes zero extra flags (guard-only branching) or needs new flags mirroring `check`'s `<position>` arg for a non-interactive fallback.
- **[OPEN]** Whether `ChecklistRunner.Update` should persist each toggle immediately (via a message the cli-layer host intercepts to call `svc.CheckItem`) or batch persistence at `Result()` time. Immediate-persist is recommended (matches v0 behavior, and is the only way to detect run auto-completion live without duplicating `allChecked` logic client-side).
- **[OPEN]** `internal/tui/AGENTS.md` is cited by docs/REVIEW_PATTERNS.md but does not exist in this checkout — no TUI-specific AGENTS.md guidance is directly available; internal/cli/AGENTS.md is the closest but is partly stale (see §5.5).
