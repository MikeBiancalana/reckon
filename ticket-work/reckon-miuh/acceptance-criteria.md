# Acceptance Criteria: reckon-miuh — `--quiet` doesn't suppress init log line

## Grounding facts

- **Root cause**: `quietFlag` (`internal/cli/root.go:26,84`) is never consulted
  on the logging path. `initLoggerE` (`root.go:103-110`) unconditionally calls
  `logger.Info("reckon initialized", ...)`; `buildLoggerConfig` (`root.go:35-60`)
  only reads `logLevelFlag` / `LOG_LEVEL` / `RECKON_DEBUG`, never `quietFlag`.
  Not a timing bug — cobra parses all flags before `PersistentPreRunE` runs, so
  `quietFlag` is already correctly populated when `initLoggerE` runs; the flag
  is just never wired to the logger. `--log-level WARN` silences the line today
  only because WARN > INFO in `slog`'s level ordering
  (`internal/logger/logger.go:134-145`).
- **`--quiet`'s existing, established scope is stdout-only**: 9 call sites
  across `add.go`, `adopt.go`, `note_v1.go`, `migrate_legacy.go`, `today.go`,
  `todo.go`, `index.go`, `query.go` all gate `fmt.Print*`/`output.Writer.Print`
  calls with `if !(mode == output.Pretty && quietFlag)` or `if !quietFlag`.
  None touch the logger. This convention is documented at
  `internal/cli/AGENTS.md:129-135` and `docs/REVIEW_PATTERNS.md:204-227`
  ("Anti-Pattern: Ignoring --quiet Flag") — both describe stdout framing,
  neither mentions logging.
- **Other INFO-level logging exists beyond this one line**: 24
  `logger.Info(...)` call sites repo-wide (e.g.
  `internal/journal/repository.go:25`, `SaveJournal`), reachable from ordinary
  commands (e.g. `rk today`). A fix scoped to only the `root.go:108` call site
  leaves this same class of leak live at every other INFO call under
  `--quiet`.
- **stdout and stderr are already separate channels**: command data output
  (`--json`/`--ndjson`/pretty) goes through `output.Writer` to stdout
  (`internal/output/output.go`); the logger writes to stderr (or a file) via
  its own `slog.Handler`, never through cobra's `cmd.SetOut`/`SetErr`. The init
  log line has never corrupted `--json`/`--ndjson` parsing — the complaint is
  purely stderr noise, not data-stream contamination.
- **Test-infra gotcha**: `RootCmd.SetErr(&buf)` (used by e.g. `runTodo` in
  `todo_test.go:109-118`) does **not** capture the logger's output — the
  logger writes directly to real `os.Stderr`. An in-process test asserting
  "stderr lacks the init line" via `RootCmd.SetErr` would pass today, bug or
  no bug. Verification requires either a real subprocess (the
  `tests/acceptance` package, which already captures `cmd.Stderr` on the built
  `rk` binary — see `tests/acceptance/acceptance_test.go:80-91`) or an
  `os.Pipe`-based redirect of real `os.Stderr` around `Execute()`.
- **No TUI entrypoint exists today**: `buildLoggerConfig(isTUIMode bool)` has
  exactly one call site, `buildLoggerConfig(false)` (`root.go:104`); `tui` is a
  retired verb. TUI-specific logging behavior is not reachable and not
  testable in this codebase.

## 1. Explicit acceptance criteria

Primary resolution: **behavior fix** — suppress the line (and its class of
logging) under `--quiet`. Rationale for rejecting the docs-only alternative is
AC5.

1. Under `--quiet`, with no explicit, more-verbose `--log-level` given, no
   INFO-level (or lower) log record reaches stderr for any `rk` subcommand —
   framed as the observable, not "guard this one `logger.Info` call," so the
   fix naturally covers the other 24 INFO call sites (Grounding facts) rather
   than leaving them as an unfixed instance of the same bug.
2. An explicit `--log-level` always wins over `--quiet`. `rk --quiet
   --log-level DEBUG todo add "x"` shows DEBUG/INFO output (including the init
   line) exactly as `rk --log-level DEBUG todo add "x"` does without
   `--quiet` — `--quiet` raises the *default* floor, it does not clamp an
   explicit user request.
3. `--quiet`'s existing stdout-suppression behavior (the `mode ==
   output.Pretty && quietFlag` convention, 9 call sites) is unchanged — this
   is an additive fix layered on top of, not a replacement for, existing
   `--quiet` behavior. Existing tests (`todo_test.go` T-7.2, `add_test.go`,
   `adopt_test.go`, `query_test.go` quiet-flag assertions) must continue to
   pass unmodified.
4. `--log-level`, used alone (no `--quiet`), continues to behave exactly as
   today — `rk --log-level WARN todo add "x"` stays silent on the init line
   (baseline regression, matches the ticket's own repro).
5. **[CONSIDERED AND REJECTED] Docs-only resolution.** The ticket's fallback
   ("or the flag's docs should clarify it only affects log level, not command
   output framing") is rejected as the primary fix:
   - It leaves the reported friction live — the reporter's dogfooding stderr
     noise persists.
   - The proposed clarifying text is backwards vs. Grounding facts: `--quiet`
     doesn't "only affect log level" today — it affects *zero* logger
     behavior and *only* affects stdout framing. A docs fix would need to add
     a caveat to an already-accurate stdout-only description, not "clarify"
     an inaccurate log-level claim.
   - The flag's own help text ("Suppress non-essential output," no stream
     qualifier) already sets the broader expectation a docs tweak can't undo
     without also weakening the flag's stated purpose.
   - A behavior fix is equally cheap here (contained to `root.go`/
     `buildLoggerConfig`) and actually resolves the report.

## 2. Implicit requirements

- **Mechanism, strongly implied by AC1's framing (not separately mandated)**:
  raise the *effective log level* (e.g. to WARN) when `quietFlag` is true and
  no explicit `--log-level` was given, inside `buildLoggerConfig`/
  `initializeWithConfig` — not a scattered `if !quietFlag` guard around the
  one `root.go:108` call. Level-based suppression is what already makes
  `--log-level WARN` work (`internal/logger/logger.go:134-145`,
  `slog.HandlerOptions{Level: logLevel}`) and is the only mechanism that
  generalizes to the other 24 `logger.Info` sites without touching each one
  individually. A single-line guard would satisfy AC1's literal repro but not
  its "any subcommand" framing.
- **Precedence chain for the effective level**, in order:
  1. Explicit `--log-level` flag (detect via `cmd.Flags().Changed("log-level")`,
     not `logLevelFlag != ""` alone — confirm this distinction still holds
     once `--quiet` is folded into the same function).
  2. `--quiet` (no explicit `--log-level`) → floor at WARN.
  3. `LOG_LEVEL` / `RECKON_DEBUG` env vars, `--quiet` not set.
  4. Default `INFO`.
  Where `--quiet` sits relative to `LOG_LEVEL`/`RECKON_DEBUG` when **both**
  are present with no explicit `--log-level` flag (e.g. `LOG_LEVEL=DEBUG rk
  --quiet ...`) is **[OPEN]** — not specified by the ticket. **[INFERRED]**
  recommendation: flag beats env var (consistent with `buildLoggerConfig`'s
  existing flag-then-env fallback order for `--log-level` itself), i.e.
  `--quiet` wins over `LOG_LEVEL`/`RECKON_DEBUG` too. Flag for planner
  confirmation, not silently assumed.
- **`--quiet` + `--log-file` interaction is [OPEN]**: if the fix raises the
  effective `slog` level globally (single `logLevel` package var feeding the
  one handler, `logger.go:31,204-213`), a log file configured via
  `--log-file` also loses INFO records under `--quiet` — even though a log
  file is exactly where a user might want full diagnostics despite a quiet
  console. Building independent per-sink levels (quiet console, verbose file)
  is a materially bigger change to the logger's single-handler architecture
  and is **out of scope** for this ticket; the recommended default is the
  simpler, consistent behavior (one global level, same as `--log-level`
  already behaves) — flagged as a known tradeoff, not silently decided.
- **This is additive, not a replacement mechanism.** `--quiet` keeps doing two
  independent jobs after this fix: suppressing stdout status framing
  (existing, AC3) and now also raising the stderr/log floor (new, AC1). An
  implementation that collapses `--quiet` into "just set level=WARN" and
  drops the `mode == output.Pretty && quietFlag` stdout checks would be a
  regression, not a simplification.

## 3. Edge cases

| Case | Expected behavior |
|---|---|
| `--quiet` alone | stderr has no INFO (or lower) records for the invocation; WARN/ERROR still shown |
| `--quiet --log-level DEBUG` | explicit level wins — DEBUG and INFO shown, including the init line |
| `--quiet --log-level WARN` | redundant, same result as either alone — INFO suppressed |
| `--quiet --log-level INFO` (explicit) | explicit level wins — INFO shown despite `--quiet`; may feel unintuitive but is consistent with AC2 ("explicit always wins") |
| `--quiet` on a command with no logging beyond the init line (e.g. simple `todo add`) | stderr fully empty, not just missing one string |
| `--quiet --json` / `--quiet --ndjson` | stdout is valid, unaffected JSON/NDJSON; stderr has no INFO records — confirms the fix is orthogonal to output-mode flags |
| `rk --quiet` (no subcommand) / `rk <cmd> --quiet --help` | `PersistentPreRunE`/`initLoggerE` don't run on cobra's help path (`root.go:71-72` comment) — no log line today, must remain true after the fix |
| `LOG_LEVEL=DEBUG` env + `--quiet` flag, no explicit `--log-level` | **[OPEN]**, see Implicit Requirements precedence note |
| `RECKON_DEBUG=1` env + `--quiet` flag, no explicit `--log-level` | **[OPEN]**, same precedence question |
| `--quiet` + `--log-file <path>` | **[OPEN]**, see Implicit Requirements — recommended default is the file sink also loses INFO records (single global level) |

## 4. Test scenarios (Given/When/Then)

All stderr-observing scenarios **must** run against the real built `rk` binary
via the `tests/acceptance` package's subprocess harness (`vault.exec`/
`vault.run`, which sets `cmd.Stderr` on `exec.Command(rkBin, ...)`) — **not**
`RootCmd.SetErr` in an in-process `internal/cli` test, which cannot observe the
logger's real `os.Stderr` writes (Grounding facts) and would produce a
false-green test.

- **Baseline, no flags**: Given no `--quiet`/`--log-level`, When `rk todo add
  "x"` runs, Then stderr contains `msg="reckon initialized"` (documents
  current default is unchanged by this fix).
- **Core fix**: Given `--quiet`, When `rk todo add "x"` runs, Then stderr does
  not contain `"reckon initialized"` and stderr is empty.
- **Stdout regression (unchanged)**: Given `--quiet`, When `rk todo add "x"`
  runs, Then stdout still shows the existing suppression of the pretty
  status/result line (same assertion as `todo_test.go`'s existing T-7.2
  tests, `todo_test.go:1407-1423`) — proves AC3, the stdout mechanism is
  untouched.
- **`--log-level` alone, unaffected**: Given `--log-level WARN` (no
  `--quiet`), When `rk todo add "x"` runs, Then stderr does not contain the
  init line (matches ticket's own repro, regression baseline).
- **Explicit level beats `--quiet`**: Given `--quiet --log-level DEBUG`, When
  `rk todo add "x"` runs, Then stderr contains the init line (and any
  DEBUG-level records) — proves AC2.
- **Redundant combination**: Given `--quiet --log-level WARN`, When run, Then
  stderr does not contain the init line (consistent, non-conflicting case).
- **Non-init INFO logging also suppressed**: Given `--quiet`, When a command
  that triggers another INFO-level log site runs (e.g. `rk today` touching
  `journal.SaveJournal`, `internal/journal/repository.go:25`), Then that log
  record also does not reach stderr — proves AC1's "any subcommand"/"any INFO
  record" framing, not just the one hardcoded string.
- **Output-mode independence**: Given `--quiet --json`, When `rk todo add
  "x"` runs, Then stdout is valid, parseable JSON and stderr has no INFO
  records. Repeat for `--quiet --ndjson`.
- **Help path unaffected**: Given `--quiet --help` (or bare `rk --quiet`),
  When run, Then help text prints and stderr has no log output (regression
  check, not a new behavior).
- **[OPEN, stub pending precedence decision]**: Given `LOG_LEVEL=DEBUG` env
  var and `--quiet` flag with no explicit `--log-level`, When run, Then
  behavior per whichever precedence the planner confirms (see Implicit
  Requirements) — do not write this assertion until that's resolved, to avoid
  encoding an unreviewed default.

## 5. Out of scope

- **Broader logging framework changes** — no new sinks, formats, or rotation
  behavior; `internal/logger` keeps its current single-handler, single-level
  architecture (`logger.go:28-38`).
- **Changing the default log level** when neither `--quiet` nor `--log-level`
  is given — stays `INFO` (`logger.go:131`, `root.go:50`).
- **TUI behavior** — no TUI entrypoint is wired in this codebase today
  (Grounding facts); `buildLoggerConfig`'s `isTUIMode` parameter is
  unaffected.
- **Per-sink independent quiet levels** (quiet console, full-verbosity log
  file) — flagged as an open tradeoff in Edge Cases/Implicit Requirements, not
  to be built now; would require splitting `internal/logger`'s single global
  `logLevel`/handler into per-writer levels, a materially larger change.
- **Manually adding `if !quietFlag` guards to each of the other 24
  `logger.Info` call sites individually** — superseded by the level-based
  mechanism in AC1; doing both would be redundant, inconsistent parallel
  suppression logic.
- **`docs/REVIEW_PATTERNS.md` preflight-grep pattern extension** (the existing
  "ignoring --quiet" pattern's grep wouldn't have caught a `logger.Info`
  call). Worth a follow-up, not required by this ticket's AC.
- **`internal/cli/AGENTS.md:110-111` stale doc drift** (`log-level` default
  shown as `"INFO"` in the sample vs. actual `""` in `root.go:86`) —
  pre-existing, unrelated to this bug.
- **`resetCLIFlags()` test-helper gap** (`query_test.go:89-109` doesn't reset
  `logLevelFlag`/`logFileFlag` between in-process `RootCmd.Execute()` calls) —
  only relevant if new in-process unit tests are added that toggle those
  flags; the primary verification path for this ticket is subprocess-based
  (Test Scenarios), which doesn't hit this gap.
