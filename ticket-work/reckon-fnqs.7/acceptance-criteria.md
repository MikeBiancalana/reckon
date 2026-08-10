# Acceptance Criteria: reckon-fnqs.7 — Interactive create flows over the v1 verbs

Builds on `ticket-work/reckon-fnqs.7/codebase-analysis.md` (facts not restated here). Depends on fnqs.5 (merged) and fnqs.6 (merged).

## 1. Explicit acceptance criteria

### 1.1 `rk todo add`

1. Bare invocation (`rk todo add`, no args, no flags) on a real TTY opens a `Wizard` with steps: subject (single-line) -> body (multi-line, `TextEditor`) -> scheduled (`DatePicker`) -> deadline (`DatePicker`) -> depends (`TaskPicker` over open durable todos).
2. On wizard completion, the CLI calls `addDurableTodo(todosDir, author, body, scheduled, deadline, depends, repeat)` (`todo.go:351`) with `body` built via the fnqs.5 subject/body convergence formula (§3.2 below) — the same function the flag-driven path calls, not a parallel write.
3. Any arg or any input-affecting flag present (`--scheduled/--deadline/--depends/--repeat/--author/-m/--edit/--ephemeral`, mirroring `resetTodoFlags`'s flag list, `todo.go:74`) routes to the existing classic `runTodoAddE` body unchanged — no TUI code is reached, matching the ticket's "with args, no TUI code is reached."
4. `--ephemeral` never triggers the wizard (§4.2 pins this: wizard always produces a durable todo; ephemeral stays flag-only).
5. `repeat` has no wizard step (matches item 1's step list, which omits it) — only reachable via `--repeat`, which forces classic path per AC1.3.

### 1.2 `rk note create`

1. `noteCreateCmd.Args` loosens from `cobra.MinimumNArgs(1)` (`note_v1.go:81`) to `cobra.ArbitraryArgs`, unblocking bare invocation — required before any wizard code is reachable.
2. Bare invocation on a real TTY opens a `Wizard`: title (single-line) -> body (multi-line, `TextEditor`) -> links (one-or-more existing notes, appended as `[[slug]]` tokens into body text — see §3.3 gap).
3. On completion, the CLI calls `createNote(notesDir, noteCreateParams{...})` (`note_v1.go:271`) — same converge point as the flag path, with `Title`/`Body` set from wizard results *after* the same pre-assembly `runNoteCreateE` performs (slugify, body-newline normalization — see §2.8, this is a transformation, not a default) and all other fields (`Type`, `Author`, `Stage`, `Description`, `Dir`, `Tags`, `Aliases`) defaulted exactly as `runNoteCreateE` defaults them today when their flags are unset (`note_v1.go:242-253`).
4. Any arg or any note-create flag present (`--slug/--type/--author/--stage/--description/--dir/--tag/--alias/--body`) routes to classic `runNoteCreateE` unchanged.
5. Bare + non-TTY (stdin/stdout not both char devices) with the loosened `Args` must not silently proceed into `createNote` with an empty title — needs an explicit "empty title"-style guard now that 0-args-non-interactive is a newly reachable state (see §4.1, pinned decision).

### 1.3 `rk add`

1. Bare invocation on a real TTY opens a single-step quick-capture prompt (`text_entry_bar`-shaped, single-line, `Prompt[string]`) — not a multi-step `Wizard` (only one field: the captured line).
2. On submit, the CLI calls `appendLogEntry(logDir, day, hhmm, author, body)` (`add.go` — see codebase-analysis §4) with `body = strings.TrimSpace(capturedLine)`, matching `requireSubject=false`'s convergence (no subject/body split for this verb).
3. Any arg or any flag present (`--author/--at/--date/-m/--edit`, whatever `add.go` defines) routes to classic `runAddE` unchanged.

## 2. Implicit requirements

1. **Converge, don't duplicate.** All three wizard paths must call the exact same write/persistence functions the flag paths call — `addDurableTodo`/`addEphemeralTodo`, `createNote`, `appendLogEntry` — with wizard-collected values mapped into the same parameter shapes (`todoAddResult` inputs, `noteCreateParams`, `appendLogEntry`'s args). No second `NewNode`/`Render`/`writeFileAtomic` call site.
2. **Subject/body convergence formula (fnqs.5) must be reused verbatim**, not reimplemented: `finalBody := strings.TrimSpace(subject); if b := strings.TrimSpace(body); b != "" { finalBody += "\n\n" + b }` (codebase-analysis §5). Extract this into a shared helper if it doesn't already exist as one — both `assembleBody`'s `-m` path and the wizard path should ideally call the same trim/join logic to prevent drift.
3. **Dispatch predicate precedes RunE side effects**, mirroring `todoListWantsTUI` (`todo_browse.go:19`): a pure `todoAddWantsTUI(cmd) bool` / `noteCreateWantsTUI(cmd, args) bool` / `addWantsTUI(cmd) bool`, each `false` if `!isInteractive()` or any relevant flag `Changed`. Each verb's `RunE` branches at the top exactly like `runTodoListE` does (`todo.go:452`).
4. **Guard stays inside `RunPrompt`/`Wizard.Run`, not the predicate.** The predicate does not consult `--no-input`; `PromptGuard`/`promptGuard()` inside `RunPrompt` remains the sole enforcement point, so `--no-input` + real TTY still produces the standard error rather than silently falling back (same precedent `todoListWantsTUI`'s comment documents, `todo_browse.go:16-18`).
5. **Cancellation is silent success, not an error.** Mirroring `runTodoBrowse` (`todo_browse.go:47-53`): `Wizard.Run` returning `ok=false` (user canceled at step 0) means the verb's `RunE` returns `nil` with no output and no file written — not an error, not a partial write.
6. **New single-line `Prompt[string]` component is required** for subject (todo add), title (note create), and the quick-capture bar (add) — codebase-analysis §3 confirms no such component exists today. One component, reused three times, is implied by "same shape" language in the gap table — building three bespoke components would be over-scope.
7. **`DatePicker`'s `time.Time` result must be reformatted back to `addDurableTodo`'s string params, exactly matching `parseSchedDate`'s expected format.** `DatePicker.Result()` is `time.Time` (codebase-analysis §3), but `addDurableTodo(...scheduled, deadline string...)` (`todo.go:351`) takes strings, and `parseSchedDate` (`recur.go:97`) parses them as UTC date-only via `time.ParseInLocation("2006-01-02", s, time.UTC)`. The wizard-to-params conversion must produce `result.UTC().Format("2006-01-02")`, not `result.Format(time.RFC3339)` or any other layout — a mismatch here is a silent data-shape bug, not a compile error. Flag as risk: `ParseRelativeDate` (`date_parser.go:18`) resolves relative-date components (`"t"`, `"+3d"`, weekday names) in `now.Location()` (local), not UTC — a date entered near a local-midnight boundary could format one calendar day off after `.UTC()` conversion. Worth a dedicated edge case, not just a formatting nit (see §3.6).
8. **`rk note create`'s converge point is `createNote`, but only after the same pre-assembly `runNoteCreateE` performs today** — this is a correction to AC 1.2.3's looser "defaulted exactly as" wording. `runNoteCreateE` doesn't just default unset fields; it *transforms* the wizard-provided ones before calling `createNote`: `slug := slugify(slugSrc)` where `slugSrc` falls back to `title` (`note_v1.go:242-244`), and `body += "\n"` if `body != "" && !strings.HasSuffix(body, "\n")` (`note_v1.go:253-255`). `createNote`'s own doc comment states it expects an already-slugified `Slug` (codebase-analysis §4) — passing the wizard's raw title through without slugifying it produces an invalid/empty `Slug`, and skipping the trailing-newline normalization produces a body that differs from the flag path's by exactly one byte. **The wizard path must call this same pre-assembly logic**, ideally by extracting it into a shared helper both `runNoteCreateE` and the wizard's conversion function call — not by re-deriving slug/newline rules independently.

## 3. Edge cases

### 3.1 Non-TTY stdin with bare invocation — pinned decision

**Bare + non-TTY routes to the classic (non-wizard) code path without ever reaching `PromptGuard`** — it does *not* produce the `--no-input`-flavored error. This is not "silent success": the classic path still runs its own validation and, for all three verbs, still ends in an ordinary error, since bare invocation supplies no content. The point being pinned is only *which* error/path fires, not that nothing happens.

Rationale: `todoListWantsTUI` already establishes this precedent for `list` (`!isInteractive()` -> predicate returns `false` -> classic path). `PromptGuard`'s "`pass --no-input` or run from an interactive terminal" message is reserved for a *genuine, deliberate* interactive attempt that gets denied (TTY present + `--no-input` flag, or a bug that reaches `RunPrompt` despite `!isInteractive()`) — it would be misleading on an ordinary non-interactive invocation (e.g. `rk todo add < /dev/null` in a script), which has never been required to say anything about interactivity.
Consequence for each verb:
- `rk todo add` / `rk add` bare + non-TTY: falls into the existing classic body, which already errors `"todo add: empty body text"` / equivalent for `add` (`todo.go:293`) — no new error text needed, this is unchanged current behavior.
- `rk note create` bare + non-TTY: falls into classic `runNoteCreateE` with `args=[]`, a *newly reachable* state now that `Args` is loosened (AC 1.2.1) — needs a new ordinary (non-interactivity-flavored) guard, e.g. `"note create: title must not be empty"`, analogous to todo add's empty-body guard, not the `PromptGuard` wording.

### 3.2 TTY present but args/flags also given

Classic path, no TUI code reached — explicit ticket requirement (§1.1.3, §1.2.4, §1.3.3). This is the same "any arg/flag -> classic" rule `todoListWantsTUI` already applies for `list`'s output-shaping flags, generalized here to each create verb's full input-flag set (codebase-analysis §8, first `[OPEN]` item — resolved here in favor of the broader check).

### 3.3 Optional fields with no "skip" affordance — real gap, AC-relevant risk

`DatePicker` (scheduled/deadline) and `TaskPicker` (depends) have no way to submit "no value" today (codebase-analysis §3): Enter on empty `DatePicker` input blocks with a validation error instead of advancing; `TaskPicker`'s Esc is consumed by `Wizard` as step-back, not "skip with no dependency," and depends is the *last* step so an Esc-back there returns to `deadline` rather than finishing the flow.

This blocks AC 1.1.1 as literally specified (scheduled/deadline/depends are optional in `addDurableTodo`, and must remain optional through the wizard, since nothing in the ticket says the wizard may *require* them). Two resolution paths, both requiring implementation work beyond "wire existing components":
- (a) add a skip affordance to `DatePicker`/`TaskPicker` (e.g., a bound key or an empty-Enter-submits-zero-value mode), or
- (b) collapse scheduled+deadline into one `Form` step with two `Required: false` `FieldTypeDate` fields (codebase-analysis §3 flags `Form` as already supporting blank-optional submit) and give `TaskPicker` an explicit "no dependency" list entry / bound key.

**This is not a minor detail — flag it to the planner as blocking scope, not a follow-up.** Recommend the planner pick (a) or (b) before implementation starts; do not let the implementer discover this mid-task.

### 3.4 Invalid input in the depends picker

`TaskPicker`'s row source must be built from actually-open durable todos (codebase-analysis §8: mirror `buildTodoItems`/`listDurableTodos`, `todo.go:511`), so only existing IDs are selectable — invalid/nonexistent depends values are structurally impossible through the wizard (unlike the flag path, where `--depends` is unvalidated, `todo.go:375-377`). No new validation logic needed inside the picker; correctness comes from the row source being correct. If the row-building query itself fails (e.g. index unreadable), that error should propagate as a normal `RunE` error, aborting before the wizard even opens (matches `runTodoBrowse`'s `buildTodoItems` error handling, `todo_browse.go:43-46`).

### 3.5 Empty title / empty subject inside the wizard

The new single-line `Prompt[string]` component (§2.6) needs its own empty-input handling — likely mirroring `assembleBody`'s "subject (first -m) must not be empty" rule for todo add's subject step and note create's title step, enforced either in the component (block submit) or in the post-wizard conversion function. `rk add`'s quick-capture line has no such requirement in the flag path (`requireSubject=false`), so it should allow empty submission through to the same empty-body guard `runAddE` already has, not block at the component level.

### 3.6 Relative-date timezone edge case (from §2.7)

`ParseRelativeDate` resolves `"t"`/`"tm"`/`"+3d"`/weekday-name inputs in `now.Location()` (local time), but `parseSchedDate` (the flag path's parser) treats the stored string as UTC date-only. A wizard user near a local-midnight boundary (e.g. 11:58pm local, several hours off from UTC) entering `"t"` (today) could have the `.UTC()`-converted result land on a different calendar day than typing the literal date string would via `--scheduled`. Treat as: implementer should verify with a test using a fixed non-UTC `now` near a day boundary; if a mismatch surfaces, either normalize `DatePicker`'s relative-date resolution to UTC internally, or accept and document the discrepancy — planner's call, not pre-decided here.

## 4. Test scenarios (given/when/then)

Two-layer pattern per codebase-analysis §7: CLI dispatch tests stub `isInteractive`; component tests script keystrokes via `tea.WithInput`/`tea.WithOutput`; a pure "wizard result map -> verb params" conversion function is tested directly for convergence proof. At least one scenario per AC above.

| # | Layer | Given | When | Then |
|---|---|---|---|---|
| T1 | CLI dispatch | `isInteractive` stubbed `true`, no args/flags | `rk todo add` executed | dispatch predicate routes to wizard path (assert via a seam — e.g. stub `Wizard.Run`/the RunE branch — not a real keystroke session) |
| T2 | CLI dispatch | `isInteractive` stubbed `true`, arg `"buy milk"` given | `rk todo add buy milk` executed | output has no ANSI escapes (`strings.Contains(out, "\x1b[")` false); classic result printed, matching today's `TestTodoAdd*` output |
| T3 | CLI dispatch | `isInteractive` stubbed `false`, no args | `rk todo add` executed | classic path runs, error `"todo add: empty body text"` (unchanged from current behavior) |
| T4 | CLI dispatch | `isInteractive` stubbed `true`, `--no-input` passed, no args | `rk todo add --no-input` executed | error text matches `promptGuard`'s existing `"...pass --no-input or run from an interactive terminal"` message |
| T5 | CLI dispatch | `noteCreateCmd.Args` loosened, `isInteractive` stubbed `false`, no args | `rk note create` executed | new "empty title" error (not the `PromptGuard` wording), proving AC 3.1's note-create branch |
| T6 | CLI dispatch | `isInteractive` stubbed `true`, `--tag foo` passed with no positional title | `rk note create --tag foo` executed | classic path (arg/flag present bypasses wizard); errors on missing title exactly as pre-ticket `MinimumNArgs(1)` would have (behavior-preserving) |
| T7 | CLI dispatch | `isInteractive` stubbed `true`, `--at 14:30` passed | `rk add --at 14:30 hello` executed | classic path, no ANSI in output |
| T8 | Component | scripted keys: subject text + Enter, body text + Ctrl+D, empty Enter at scheduled, empty Enter (or skip key, per §3.3's resolution) at deadline, one selection at depends | `Wizard.Run` driven via `tea.WithInput` | resulting `map[string]any` has all five keys; scheduled/deadline are `""`/zero-value; depends is the selected ID |
| T9 | Component | scripted keys: subject text + Enter, then Esc at body step | `Wizard.Run` driven | steps back to subject (re-mounted), not full cancel — proves `Wizard`'s step>0 Esc semantics apply here, not a full-flow abort |
| T10 | Component | scripted key: Esc at the very first step (subject) | `Wizard.Run` driven | `ok=false`, `canceled=true`, zero-value result map |
| T11 | Pure conversion fn | synthetic `map[string]any{"subject": "Buy milk", "body": "at the store", "scheduled": "", "deadline": "", "depends": ""}` | wizard-result-to-`addDurableTodo`-args conversion function called | args equal `body="Buy milk\n\nat the store"`, scheduled/deadline/depends `""` — matches §2.2's convergence formula exactly |
| T12 | Pure conversion fn | synthetic map with `body: ""` (subject only) | conversion function called | `body == "Buy milk"` with no `\n\n` separator (empty body branch of the formula not appending) |
| T13 | Pure conversion fn | flag path: `rk todo add -m "Buy milk" -m "at the store"` run through `assembleBody` directly | compared against T11's conversion function output for the same logical subject/body | both produce byte-identical `body` strings — the convergence proof the "identical file" AC requires, since no PTY end-to-end test is possible |
| T14 | CLI dispatch | `isInteractive` stubbed `true`, `--ephemeral` passed, no other args | `rk todo add --ephemeral` executed | classic path, `addEphemeralTodo` called — proves AC 1.1.4 (ephemeral never reaches wizard) |
| T15 | Component | scripted keys select 2 notes in sequence at the links step | `Wizard.Run` driven (once §3.3/gap-table's links loop design lands) | resulting body contains two `[[slug]]` tokens, both parsed correctly by `node.Parse`'s `wikilinkRe` |
| T16 | Pure conversion fn | synthetic map for `add`: `{"capture": "  quick note  "}` | conversion function called | `appendLogEntry` args' `body == "quick note"` (trimmed, matches `requireSubject=false` convergence) |
| T17 | Pure conversion fn | synthetic map with `scheduled`/`deadline` set to populated `time.Time` values (e.g. `2026-08-15`, non-UTC `now` fixture) | conversion function called | output strings equal `"2026-08-15"` (matches `parseSchedDate`'s `"2006-01-02"` UTC layout exactly) — proves §2.7's formatting obligation, not just the empty-field case T11/T12 cover |
| T18 | Convergence (flag vs. wizard, todo add) | same logical subject/body/scheduled/deadline/depends, once via `--scheduled 2026-08-15 -m "Buy milk" -m "at the store"` through the real classic path, once via the wizard conversion function feeding `addDurableTodo` directly | both invoked against separate temp vaults | resulting `todos/<id>.md` files are byte-identical except for the ULID filename/`id` field — the direct proof of the ticket's "identical file" criterion for todo add, extending T13 to a populated (non-empty) case |
| T19 | Convergence (flag vs. wizard, note create) | same logical title/body, once via `rk note create "My Title" --body "text"` through classic `runNoteCreateE`, once via the wizard conversion function feeding `createNote` after the shared slugify/newline pre-assembly (§2.8) | both invoked against separate temp vaults | resulting `notes/<slug>.md` files are byte-identical — proves the wizard path reuses `slugify(title)` and the trailing-newline normalization, not just calls `createNote` with raw values |
| T20 | Convergence (flag vs. wizard, add) | same logical capture text, once via `rk add "quick note"` through classic `runAddE`, once via the wizard conversion function feeding `appendLogEntry` directly | both invoked against separate temp vaults on the same day | resulting `log/<date>.md` entries are identical apart from timestamp-dependent fields already variable between any two runs (e.g. `hhmm` if the two invocations don't land in the same minute — pin `hhmm` via a fixed-clock test seam if one exists, otherwise assert on body/author fields only) |

## 5. Out of scope / scope questions for the planner

1. **[OPEN — scope decision needed]** This ticket is framed as "wire *existing* wizard components onto three verbs," but the gap table (codebase-analysis §3) shows two component types genuinely don't exist: a single-line text `Prompt[string]` (needed for subject/title/quick-capture — §2.6 above) and a multi-select note picker (needed for "links" — §3.3/gap table, `NotePicker` is single-select only). Building these is net-new component work, not wiring. Recommend the planner explicitly scope these in (most likely reading, since the ticket's step list requires them to exist) rather than have the implementer discover and freelance-design them mid-task.
2. **[OPEN — carried from codebase-analysis §3, not resolved here]** Whether `scheduled`/`deadline` are two chained `DatePicker` `Wizard` steps (literal reading of the ticket) or one combined `Form` step with two optional date fields (sidesteps the skip-gap in §3.3 more cheaply) is a design choice for the planner, not this document.
3. **Confirmed in scope**: `noteCreateCmd.Args` loosening from `cobra.MinimumNArgs(1)` to `cobra.ArbitraryArgs` (AC 1.2.1) — required to make bare invocation reachable at all; not a judgment call.
4. **Out of scope**: any change to the flag-driven paths' existing validation, output, or file formats. `--depends`'s current lack of existence-checking on the flag path (`todo.go:375-377`) stays as-is; the picker only offers valid rows as a side effect of its own design, not a newly enforced constraint on `--depends` itself (codebase-analysis §8).
5. **Out of scope**: `internal/cli/AGENTS.md`'s stale interactive-prompt guidance (`AGENTS.md:229-260`) — codebase-analysis §6 flags it as predating fnqs.6 and inconsistent with the real dispatch machinery; updating it is a documentation cleanup, not part of this ticket's functional scope, though the planner may want to fold it in opportunistically since fnqs.7 is the first ticket to exercise `Wizard` from a real CLI call site.
6. **Out of scope**: fixing `PromptGuard`'s error-message wording (codebase-analysis §6, R3) beyond what §3.1 above already requires (a *new, separate* error for note-create's newly-reachable 0-args-non-interactive state) — the existing `--no-input` message for the TTY+`--no-input` case is unchanged.
