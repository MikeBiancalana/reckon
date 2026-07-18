# Code Review: reckon-fnqs.8 — Persistent multi-pane `rk tui`

**Verdict: APPROVE**

> **Supersedes the prior REQUEST CHANGES review** (kept below as §5 for the record).
> Since that review the owner chose to wire real creation flows now (not ship agenda-only).
> All blocking/should-fix items are resolved: (1) agenda/todos row truncation added;
> (2) `n` keybindings wired for todos/log/notes creation; (3) box-sizing mismatch fixed via a
> shared `paneContentDims`; (4) `notes_pane.go:455` nil-guard added; plus a simplify pass
> (`reconcileDone`/`startCreateSubFlow`/`finishCreateSubFlow`) and two preflight fixes. Build/vet/
> tests green (`go build ./...`, `go vet ./internal/cli/... ./internal/tui/...`, `go test` same).

---

## 1. Required-fix verification (re-read actual code, not commit messages)

| Prior blocker/should-fix | Status | Evidence |
|---|---|---|
| Truncation missing on agenda/todos rows | **Resolved** | `renderAgendaBody`/`renderTodosBody` (`tui_model.go:415-460`) compute `innerW` via `paneContentDims(p.width,p.height)` and clip each line through `truncateRow` → `lipgloss.NewStyle().MaxWidth(width)` — the same mechanism `log_view.go`'s `LogDelegate.Render` uses. `TestAgendaPaneRenderTruncatesLongContent` (`tui_test.go`) covers the agenda renderer. |
| Box-sizing inconsistent across panes | **Resolved** | `paneContentDims` (`tui_model.go:369-382`) is now the single source of truth: `renderPaneBox` calls it, and both `logPane.SetSize` (`tui_panes.go:176-181`) and `notesPane.SetSize` (`tui_panes.go:225-232`) feed the inner widgets `paneContentDims(...)` instead of the outer target. `View` passes outer `p.width`/`p.height` into `renderPaneBox` (`tui_model.go:216-225`), which re-derives inner via the same helper — no double-subtraction. Duplication risk from the prior review is gone (the border/pad constants live in one function). |
| `notes_pane.go:455` unguarded `SourceNote` deref | **Resolved** | `getLinkAtCursor` (`notes_pane.go:452-458`) returns `nil` when `bl.NoteLink.SourceNote == nil`. The only caller, `selectCurrentLink` (`notes_pane.go:408-411`), already `return nil`s on a nil link → the nil case is a clean skip-navigation no-op, **not** a silent wrong-navigation. |
| Creation flows unreachable by keyboard | **Resolved** (owner chose to wire now) | `n` bound in all three panes; verbs called correctly; see §2–§3. |

Preflight fixes also confirmed: `note_v1.go:285` now `fmt.Errorf("note create: %w", err)`; `tui_read.go`'s `loadLogEntries`/`listNotes` use `defer rows.Close()`.

## 2. New keybinding surface — collisions

| Pane | New key | Collision check | Verdict |
|---|---|---|---|
| Todos | `n` → add-todo | Existing keys are `j/k/down/up` only (`handleTodosKey`, `tui_keyboard.go:147-156`). | No collision. |
| Log | `n` → add-log | `n` is intercepted in `handleLogKey` **before** forwarding to `LogView.Update` (`tui_keyboard.go:163-171`). The log list has **filtering disabled** (`log_view.go:127` `SetFilteringEnabled(false)`), so there is no `/`-filter typing state for `n` to be stolen from. | No collision. |
| Notes | `n` → create-note (browse only) | Guarded by `!m.notes.picker.IsFiltering()` (`tui_keyboard.go:180-183`). `IsFiltering()` (`note_picker.go:206-208`) returns `list.FilterState()==list.Filtering`; the picker has filtering **enabled** (`note_picker.go:157`), so mid-filter `n` keystrokes correctly reach the filter input. Inspect mode never reaches the `n` branch (create is browse-only). | No collision. |

## 3. Creation sub-flows call the right verb with sane args

| Flow | Verb + args (`tui_keyboard.go`) | Signature | Verdict |
|---|---|---|---|
| Add todo | `addDurableTodo(todosDir, author, body, "", "", "", "")` | `todo.go:328` (7 params) | Correct. No scheduled/deadline/depends/repeat for v1 — consistent with what `TestAddTodoFlow` exercises. |
| Add log | `appendLogEntry(logDir, day, hhmm, author, body)` with `day=time.Now().UTC().Format("2006-01-02")`, `hhmm=…"15:04"` | `add.go:183` | Correct. **Matches the CLI default exactly** — `rk add`'s `getEffectiveDate`/`getEffectiveTime` default to `time.Now().UTC()` (`add.go:161,170`). No UTC/local divergence; TUI entries file under the same day the CLI would. |
| Create note | `createNote(notesDir, noteCreateParams{Title, Slug: slugify(title), Type: "note", Author})` | `note_v1.go:319` | Correct. Slug self-minted via `slugify`+`validateSlug`, mirroring `runNoteCreateE:244-247`; `Type:"note"` matches the CLI's empty→"note" default; `Body` empty (single-line entry bar, v1). `createNote` does its own dir-mkdir/collision/overwrite checks, so duplicate titles surface a real error to `m.lastErr`. |

## 4. Simplify-pass extraction — no regression

The "collapse 3 near-identical flows into one parameterized helper" refactor is wired correctly end-to-end; the kind string and dispatch func stay matched on every path:

| Pane handler (`startCreateSubFlow`) | `handleSubFlowKey` dispatch | `*Cmd` → `reconcileDone(kind)` | `reloadCmdFor(kind)` |
|---|---|---|---|
| `subFlowAddTodo` | `m.addTodoCmd` | `"todos"` | `loadTodosCmd` |
| `subFlowAddLog` | `m.addLogCmd` | `"log"` | `loadLogCmd` |
| `subFlowNewNote` | `m.createNoteCmd` | `"notes"` | `loadNotesListCmd` |

`View`'s modal switch (`tui_model.go:211`) routes all three new kinds to `textEntry.View()`. `finishCreateSubFlow` (`tui_keyboard.go`) handles Esc (cancel, no verb), Enter (trim → cancel → dispatch iff non-empty → silent no-op on empty), else forwards to `textEntry.Update`. The three `*Cmd`s snapshot `vaultDir`/`ix`/`author` before returning the closure (no mutable model field read inside the async closure) — the closure-capture pattern from the prior review holds. `reconcileDone` reconciles then emits `mutationDoneMsg{kind}`, routed by `Update` (`tui_model.go:194`). No mis-routing, no cross-wiring.

Test coverage for the new path is real: `TestTodosPaneAddKeybinding`/`TestLogPaneAddKeybinding`/`TestNotesPaneCreateKeybinding` drive the actual `handleKey("n")` → `handleKey(Enter)` sequence and assert a real vault file appears plus the pane reloads to include it.

## 5. Other dimensions (fresh pass on new surface)

**Correctness / error handling** — Add-todo and add-log `os.MkdirAll` their target dir before the verb; create-note relies on `createNote`'s internal mkdir (cosmetic inconsistency, harmless). Errors from all three verbs surface to `m.lastErr` via `errMsg`, unmodified. `addLogCmd` reads `time.Now().UTC()` twice (day, then hhmm) — a sub-nanosecond midnight-crossing could split them, but the CLI's own path has the identical two-call shape, so it is not a new divergence.

**Security** — Note title flows through `slugify`→`validateSlug` before becoming a filename; `createNote` also runs collision/overwrite guards. No injection or path-escape surface added.

**Testing / maintainability / performance** — No new concerns; the shared helpers reduce duplication cleanly and are well-commented (why, not what).

## 6. Non-blocking observations (follow-up, not gating)

1. **Stale empty-state hint now points at the wrong key** (`log_view.go:155`): the empty log pane renders `"No log entries yet - press L to add one"`, but the key just wired is `n`, and there is **no `L` handler anywhere** (verified: global keys are Tab/CtrlC/q, no pane binds `L`, `LogView.Update` only forwards to `list.Update`). Pre-existing text, but wiring `n` as add-log is what makes it *actively* wrong — at the exact moment (empty log) the hint matters. One-line fix: change `L` → `n`. Should-fix.
2. **Double `"note create:"` prefix on the CLI path** (`note_v1.go:285`): the preflight fix wrapped `createNote`'s error with `fmt.Errorf("note create: %w", err)`, but every error `createNote` returns already self-prefixes `"note create: …"` (`note_v1.go:324-352`), so e.g. a duplicate slug via `rk note create` now surfaces as `"note create: note create: slug … already claimed"`. (The TUI's `createNoteCmd` returns the verb error un-rewrapped, so the TUI banner is unaffected.) Clean fix is either drop the self-prefix in the verb or don't re-wrap at the call site — minor, error-handling dimension.
3. **Pre-existing (not this diff): global `q`/`Tab`/`Ctrl+C` are matched in `handleKey` (`tui_keyboard.go:30-39`) *before* the focused-pane handlers**, so they steal keystrokes while the notes picker is mid-filter — the same class of bug the `n` guard fixes, but for global keys (a filter query containing `q` would quit instead of typing). Present in the prior TUI version too; the model doesn't track picker filter-state at the global layer. Worth a follow-up (e.g. gate global single-letter quit on `!picker.IsFiltering()` when notes is focused), but out of scope here and minor.
4. **Coverage gap**: no test asserts the `IsFiltering()` guard (that `n` typed mid-filter does *not* create a note). The guard is correct by inspection; a one-case test would lock it in.
5. `renderTodosBody` truncation has no direct test (only the agenda renderer does), though it is the same code path.

None of the above gate merge.

---

## APPENDIX — Prior review (REQUEST CHANGES), retained for history

The prior review gated on: (blocking) missing agenda/todos truncation and unreachable creation
keybindings; (should-fix) box-sizing mismatch and the `notes_pane.go:455` nil deref. It confirmed
as correct: the read-only guard, `logDid=true` default for `x`, the verb→reconcile→reload→
reselect-by-ID mutation flow, strict journal/service decoupling (AST test), and async closure
capture. The current diff does not touch those five confirmed-correct areas except through the
shared `reconcileDone`/creation-cmd plumbing, which was re-verified above (§4). All four flagged
items are now resolved.
