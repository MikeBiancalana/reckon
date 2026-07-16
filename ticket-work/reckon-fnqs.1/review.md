# Code Review — reckon-fnqs.1 (Retire the v0 DB-primary verb surface)

Verdict: APPROVE

Clean, disciplined pure-deletion. Every gating AC verifies green, the two new spec
tests honor D3 exactly, the scope boundary held (survivor libraries and
`internal/tui/components` untouched), and there is no compiler-invisible over-deletion.
One optional Minor cleanup, non-blocking.

## Mechanical verification (all pass)

| Check | Result |
|---|---|
| `go build ./...` | exit 0 |
| `go vet ./...` | exit 0 |
| `go test ./...` | all packages `ok` |
| `go test -tags acceptance ./tests/acceptance/` | `ok` (drives real binary through trimmed `PersistentPreRunE`) |
| `grep requiresDB` (non-test) | 0 — AC-4 |
| `grep DatabasePath()` (non-test, non-`tests/`) | only the func def `internal/config/config.go:89`, zero callers — AC-3/EC-6 |
| `t.Skip(` in `internal/cli/*.go` | exactly 2 (`add_test.go`, `gitattributes_test.go`) — AC-8, deletion not skipping |
| deleted v0 files / `internal/{db,sync,migrate}` / `internal/tui` root | all gone — AC-9, T9 |
| `internal/{journal,service,checklist,storage}` + `internal/tui/components` | 0-line diff vs origin/main — D1/AC-9 held |
| `go mod tidy` re-run | no further changes; `fsnotify` + `golang.org/x/term` dropped as direct requires |

## Dimension-by-dimension

- **Over-deletion (the high-risk axis):** none found. Every deleted-symbol reference
  resolves inside another deleted file or one of the two edit files. The `note` parent
  survives and still hosts the v1 children (`note_v1.go:133`
  `noteCmd.AddCommand(noteCreateCmd, noteShowCmd, noteRenameCmd, noteIndexCmd)` intact).
  `getEffectiveDate`/`dateFlag` kept (add.go dependency). `buildLoggerConfig`/`initLoggerE`
  kept. `rk import` path (`import.go` + textmigrate) untouched — fnqs.2 dependency protected.
- **Under-deletion / residue:** the `requiresDB` machinery is fully gone — no `Annotations`
  key, no ancestor walk, no `initServiceE`. `PersistentPreRunE` (root.go:73-80) correctly
  reduces to the json/ndjson exclusivity check + `initLoggerE()`. Service globals, imports
  (`checklist`/`journal`/`service`/`storage`/`config`), and constructor funcs all removed
  with zero live referrers (only hit is a stale *comment* in `today_test.go:10`, the
  EC-7-accepted historical narration — harmless). See M1 below for the sole live residue.
- **The two spec tests (D3):** correct and non-flaky.
  - `TestRootCommandSurface` asserts over `RootCmd.Commands()` `.Name()` set, two-directional
    (8 survivors present, all 12 dying verbs absent incl. `migrate`/`summary`), no `Execute()`
    call, no exact-count snapshot. Immune to the substring traps (`log`→`--log-file`,
    `task`→root Long, `notes`→note Short).
  - `TestNoteSubcommandSurface` mirrors it over `noteCmd.Commands()`: `{create,show,rename,index}`
    present, `{new,list}` absent.
  - `TestRootHelp_ListsSubcommands` (the one permitted edit) now asserts presence of the 8
    survivors only — `log` dropped, no dead verb asserted present. Presence-only substring is
    acceptable here since absence is fully covered by `TestRootCommandSurface`; the docstring
    correctly labels it "superseded ... for absence checks."
- **note.go / note_v1.go:** `noteCmd.Short`/`Long` rewritten to describe `create/show/rename/index`
  (no longer the stale "create, list, and delete"). Imports collapse to `cobra` alone. v1
  registration intact.
- **Error handling / security / performance:** deletion-only; the change *removes* the
  unconditional legacy-DB open, shrinking runtime surface. No new paths, no concerns.

## Findings

### [Minor] root.go:104 — `buildLoggerConfig(isTUIMode bool)` param is now always `false`

`initLoggerE` (root.go:104) is the only remaining caller and passes `false`; the sole path
that ever passed `true` was `tuiCmd` (deleted `stubs.go`). So the parameter and the
`TUIMode: isTUIMode` line (root.go:40) are effectively dead — this is the
`docs/REVIEW_PATTERNS.md:905` "Dead Code After Refactoring" shape.

Failure it causes: none functional. Cleanup only.

Fix (optional): drop the parameter and set `TUIMode: false` inline, or leave it. Defensible
to keep as-is: `logger.Config.TUIMode` remains a legitimate API and the planned TUI rebuild
(components/ is deliberately retained for it) would re-introduce a `true` caller — so this
reads as forward-looking rather than strictly dead. Not gated by any AC; author's call.
Deferring it to the TUI-rebuild ticket is fine.

## Accepted / not reported (per assignment)
`tests/integration` pre-existing failure; `config.DatabasePath()` def retained; doc staleness
(`internal/cli/AGENTS.md`, `docs/agents/planner.md:13`); `deadcode` false positives awaiting
fnqs.2; `today_test.go` EC-7 stale comment. None are blockers.
