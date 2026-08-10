# Code Review: reckon-fnqs.7 — Interactive create flows over the v1 verbs

**Verdict: APPROVE WITH CHANGES**

No correctness, security, or convergence blocker. The core acceptance criterion
(byte-identical files between the flag path and the wizard path) holds, verified
against the actual write functions rather than the tests alone. The requested
changes are one robustness cleanup (`buildNoteLinkRows` nested-cursor pattern)
plus provenance-label cleanup in three test files; the rest is optional polish.

## Summary

The wizard/prompt flows are wired onto `rk todo add`, `rk note create`, and
`rk add` cleanly and, importantly, converge on the same `addDurableTodo` /
`createNote` / `appendLogEntry` write functions the classic path uses. Dispatch
gating is agent-safe. No closure-capture bugs. The design mirrors the fnqs.6 /
6k0l patterns faithfully. Findings below are all non-blocking.

## Convergence claim — traced, holds

| Verb | Mechanism | Divergence risk |
|---|---|---|
| `rk todo add` | `joinSubjectBody` trims subject+body, joins with `\n\n`; matches `assembleBody`'s `-m` path (`TrimSpace(Join(trimmed,"\n\n"))`); `addDurableTodo` Render→Parse→Serialize round-trips both | None for clean inputs; scheduled/deadline stored verbatim by flag path vs `ParseRelativeDate`-normalized by wizard — documented R2, accepted |
| `rk note create` | Both paths funnel through the extracted `normalizeNoteCreateParams` seam → `createNote` | See Finding 4 (echoed Title only; file unaffected) |
| `rk add` | `wizardAddBody` = `TrimSpace(capture)`, matches positional `assembleBody` branch; same `resolveAuthor("")` / `effectiveLogDate` / `resolveAtTime("")` / `appendLogEntry` tail | None (timestamp caveat is inherent, per T20) |

The three convergence tests pass (`go test -run Convergence` clean). Title-trim
was my initial concern for note-create; it is **nullified** by the
Render→Parse→Serialize round-trip inside `createNote` — `fmScalarRe`
(`node.go:103`, group 3 `(.*?)` excludes leading/trailing ws) strips frontmatter
whitespace on parse, so the written file is byte-identical regardless.

## Dispatch predicates — agent-safe, no gaps

- `isInteractive()` (interactive.go:16) requires **both** stdin and stdout to be
  char devices, so any piped/redirected/scripted context routes to the classic
  body. No TUI can launch in a non-interactive context.
- Per-verb flag lists exactly match the reset-flag lists — verified:
  - todo add `{ephemeral,scheduled,deadline,depends,repeat,author,message,edit}` == `resetTodoFlags`' add-relevant set (todo.go:118-125).
  - note create `{slug,type,author,stage,description,dir,tag,alias,body}` == `resetNoteFlags` (note_v1.go, all 9 registered flags).
  - add `{author,at,message,edit,date}` == `resetAddFlags` + `date` (the global persistent flag `effectiveLogDate` reads; correctly added to the reset list too, add.go:65, so `Changed` can't leak across invocations).
- `--no-input` deliberately not consulted in predicates; it reaches
  `components.PromptGuard` inside `RunPrompt`/`Wizard.Run` (prompt.go:79,
  wizard.go:167 → RunPrompt), so a real TTY + `--no-input` errors rather than
  hanging or silently falling back. Correct.

## Critical Issues

None.

## Recommendations

### 1. `buildNoteLinkRows` nests a query inside an open cursor — deviates from the in-repo safe pattern [Medium-low]

`note_create_wizard.go:140-153` calls `loadProps(db, id)` (query.go:463, which
issues its own `db.Query`) **inside** the `for rows.Next()` loop over the still-open
`SELECT id, loc FROM nodes` cursor. The established analogous code — the note-index
builder in the same package (`note_v1.go:777-808`) — deliberately drains the outer
cursor into a slice, calls `rows.Close()` (`:791`), and only then loops calling
`loadProps` per row.

- **Safe today** [INFERRED from driver+config+passing test]: `modernc.org/sqlite`
  (index.go:26,85) with the default unbounded connection pool (no
  `SetMaxOpenConns` anywhere) + WAL means the nested query gets its own pooled
  connection. `TestBuildNoteLinkRows_...` exercises the path with a real note and
  passes.
- **Fragile**: a future `SetMaxOpenConns(1)` (a common SQLite hardening to avoid
  `database is locked`) would deadlock this loop, and it contradicts the plan's
  own claim that `buildNoteLinkRows` mirrors the note-index query shape.
- **Fix** (mechanical): collect `(id, loc)` pairs, `rows.Close()`, then `loadProps`
  — matching `note_v1.go:777-808`.

### 2. Esc-back re-prime is unwired; `TextPrompt.SetValue` is dead code [Low, UX]

None of the three wizard factories reads its `prior map[string]any` argument
(`grep 'prior\['` in `*_wizard.go` → no hits), and `TextPrompt.SetValue`
(text_prompt.go:97) has **zero callers** anywhere in the tree.

- Consequence: Esc-back to an earlier step re-mounts a fresh, empty component and
  discards the user's prior entry from the UI (the value survives in the results
  map only until the retype overwrites it). The `Wizard` framework *does* support
  re-priming — `TestWizard_EscMidFlowStepsBackKeepsResult` (wizard_test.go:95)
  proves it, using a factory that reads `prior` — but these drivers don't use it.
- `SetValue`'s own doc comment ("a Wizard step factory re-priming from a prior
  result map entry on Esc-back") describes behavior that isn't wired, so it's
  actively misleading.
- **Fix**: either wire re-priming (factory reads `prior["subject"]` etc. and calls
  `SetValue`) or delete `SetValue` and correct/drop the comment. [OPEN — acceptable
  to defer for v1 if Esc-back-loses-input is a known accepted UX cost.]

### 3. Provenance labels slipped into test comments/messages [Low, cleanup]

Source `.go` (non-test) files are clean. These remain in tests and should be
stripped per the no-provenance-in-comments convention (the task flagged this as an
explicit prior cleanup that must not regress):

| File:line | Reference |
|---|---|
| text_prompt_test.go:12 | "fnqs.6-era components" |
| text_prompt_test.go:39, :66 | "(gap G2)" |
| multi_note_picker_test.go:49 | "(T15)" |
| multi_note_picker_test.go:78 | "(gap G5)" |
| note_create_wizard_test.go:112 | error string "the new AC §3.1 guard's ..." |

### 4. Wizard note-create echoes an untrimmed `Title` in output [Very low, cosmetic]

`wizardNoteParams` (note_create_wizard.go:98) and `normalizeNoteCreateParams` do
not trim `Title`; the flag path trims before building params (note_v1.go:243).
`TextPrompt` submits the raw (untrimmed) value by design. With a whitespace-padded
title the *file* is still byte-identical (see round-trip note above), but the echoed
`noteCreateResult.Title` in `--json`/pretty output would retain the padding.
Optional: move the title trim into `normalizeNoteCreateParams` so both paths' echoed
results match (idempotent for the flag path). [OPEN]

### 5. `MultiNotePicker` selection order is nondeterministic; no per-row indicator [Very low]

`SelectedSlugs()`/`Result()` (multi_note_picker.go:84-92, 101) iterate a `map`, so
link-token order in the composed body varies run-to-run. The list shows no per-row
selection mark — only the sorted "Selected: …" footer (`View()` sorts, `Result()`
does not). No correctness impact; convergence test carries no links. Consider
sorting `SelectedSlugs()` for stable body output.

## Positive Observations

- **Convergence seam is genuinely shared, not duplicated.** The
  `normalizeNoteCreateParams` extraction (note_v1.go:296) is a clean pure refactor;
  both paths call the identical `createNote`/`addDurableTodo`/`appendLogEntry`.
- **Args loosening handled correctly.** `cobra.ArbitraryArgs` + the new
  `len(args)==0` guard (note_v1.go) makes the bare case reachable for the wizard
  without breaking `note create ""` (falls through to the existing slug-validation
  error, as documented in-code).
- **No closure-capture bugs.** `TextPrompt.Update` captures `value` locally before
  the `tea.Cmd` closure; `MultiNotePicker.Update` captures `slugs`; wizard factories
  capture pre-computed `dependsRows`/`linkRows` by value.
- **Security clean.** Static SQL in `buildNoteLinkRows`; `--dir` traversal guarded
  in `createNote` (note_v1.go:354-363); no new injection surface; `embeddedHeaderRe`
  guards carried into the add wizard.
- **R4 no-empty-step-hang holds.** `buildDependsRows` always prepends the
  "(no dependency)" row; both pickers respond to Enter regardless of item count;
  components prime synchronously in `Show()`, `Init()` is a no-op.
- **Error wrapping and resource cleanup** are consistent (`%w` with verb-prefixed
  context; `defer ix.Close()` / `defer rows.Close()` throughout).
- **Synthetic none-row mechanism verified end-to-end**: `TaskPicker.Result()` →
  `GetSelectedTaskID()` returns the row `ID`, which is `""` for the synthetic row;
  ULIDs are always non-empty, so `""` unambiguously means "no dependency".

## Questions for Consideration

1. Is untrimmed `Title` in `--json` output acceptable, or should the trim move into
   the shared `normalizeNoteCreateParams` seam (Finding 4)?
2. Is Esc-back-loses-input an accepted v1 UX cost, or should the factories re-prime
   from `prior` (Finding 2)? The framework already supports it.
