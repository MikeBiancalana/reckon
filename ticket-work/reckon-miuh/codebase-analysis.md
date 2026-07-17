# reckon-miuh — codebase analysis: `--quiet` doesn't suppress init log line

## Root cause

`quietFlag` is never read on the logging path. `initLoggerE` (`internal/cli/root.go:103-110`)
unconditionally calls `logger.Info("reckon initialized", ...)` after building the logger
config; `buildLoggerConfig` (`root.go:35-60`) only consults `logLevelFlag` / `LOG_LEVEL` /
`RECKON_DEBUG`, never `quietFlag`. Default level is `"INFO"` (root.go:50, and again
independently in `internal/logger/logger.go:131`), so the line is emitted at INFO to stderr
on every invocation unless the effective level is WARN+. This is a wiring gap, not a design
choice: `quietFlag` is declared once (root.go:26) and bound once (root.go:84) and every other
reference to it (9 other files, see below) gates `fmt.Print*`/`output.New(...).Print` calls —
none of them are anywhere near the logger.

Live-verified on this checkout (built `./cmd/rk`, ran against a scratch vault):
- `rk --quiet todo add "x"` → prints the `time=... level=INFO msg="reckon initialized" ...` line to stderr.
- `rk --log-level WARN todo add "y"` → silent.
- `rk todo add "z"` (no flags) → prints the line (same as `--quiet`, confirming quiet has zero effect on it).

## 1. `--quiet` flag definition

| Item | Location |
|---|---|
| Package var | `var quietFlag bool` — `internal/cli/root.go:26` |
| Cobra binding | `RootCmd.PersistentFlags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress non-essential output")` — `root.go:84` |
| Scope | Persistent flag on `RootCmd`, inherited by all subcommands (todo, today, add, adopt, migrate, index, note, query) |

`quietFlag` is consumed in 9 call sites across `add.go`, `adopt.go`, `note_v1.go` (x4),
`migrate_legacy.go` (x2), `today.go` (x4), `todo.go` (x2), `index.go`, `query.go`. All of
them follow one of two shapes:
- `if !(mode == output.Pretty && quietFlag) { ...print result... }` — the dominant convention.
- `if !quietFlag { ...print a status/progress line... }` (today.go:202, today.go:376).

The documented rationale (`internal/cli/index.go:53-55`, comment on the same pattern):
> "In pretty mode `--quiet` suppresses the status line; structured output is the requested
> data, so `--json`/`--ndjson` always emit."

So `--quiet`'s *actual, established* scope is: suppress a command's own stdout
success/status framing in Pretty mode. It has never gated anything on stderr or in the
logger. `internal/cli/AGENTS.md:129-135` ("Standard Output (Respect --quiet)") documents
exactly this stdout convention and nothing about logging — the docs and the code agree with
each other, they just don't mention the init log line at all.

## 2. "reckon initialized" log line

| Item | Location |
|---|---|
| Call site | `logger.Info("reckon initialized", "version", "dev", "log_file", logger.GetLogFile(), "log_level", cfg.Level)` — `internal/cli/root.go:108` |
| Level | INFO (hardcoded call; suppressed only if effective level ≥ WARN) |
| Only caller of `initLoggerE` | `RootCmd.PersistentPreRunE` — `root.go:73-80` |
| No other "initialized" string | grepped the repo; this is the only startup banner log line |

Nothing upstream of `logger.Info` checks `quietFlag`. Fixing this requires either (a) an
`if !quietFlag` guard around `root.go:108`, or (b) making `buildLoggerConfig` bump the
effective level to WARN when `quietFlag` is set and `logLevelFlag`/env vars didn't already
request something more verbose (which would also quiet any other INFO-level logging emitted
during the same run — currently none exists elsewhere, but worth deciding intent). [OPEN for
planner: which of these two shapes; (a) is more surgical and matches the "quiet only affects
this one line" repro, (b) is more general and matches the flag's stated help text
"Suppress non-essential output".]

## 3. `--log-level` flag and logger wiring

| Item | Location |
|---|---|
| Package var | `var logLevelFlag string` — `root.go:28` |
| Cobra binding | `RootCmd.PersistentFlags().StringVar(&logLevelFlag, "log-level", "", "Log level: DEBUG, INFO, WARN, ERROR (default: INFO)")` — `root.go:86` |
| Consumed by | `buildLoggerConfig` (`root.go:35-60`) → `logger.Config{Level: logLevelFlag, ...}` |
| Level string → `slog.Level` | `internal/logger/logger.go:127-145` (`initializeWithConfig`), switch on `strings.ToUpper(levelStr)`: DEBUG/INFO/WARN\|WARNING/ERROR, default INFO |
| Handler construction | `logger.go:204-213` — `slog.NewTextHandler`/`slog.NewJSONHandler` with `slog.HandlerOptions{Level: logLevel}` |

Precedence for the effective level (`buildLoggerConfig`, root.go:37-53): `--log-level` flag →
`LOG_LEVEL` env var → `RECKON_DEBUG=1/true` → `"INFO"` default. `--quiet` is absent from this
chain entirely — confirms finding in section 1/root-cause.

Note: `internal/cli/AGENTS.md:110-111` documents the flag default as `"INFO"` in the
`StringVar` call itself (`"log-level", "INFO", ...`), but the actual code default in
`root.go:86` is `""` (empty), with `"INFO"` applied later inside `buildLoggerConfig`
(root.go:50) / `initializeWithConfig` (logger.go:131). Functionally identical, but AGENTS.md's
code sample is stale/simplified vs. current root.go — not a bug, just a doc drift worth a
one-line note if touching that doc.

## 4. Logger construction timing relative to flag parsing

**Not a timing bug.** Cobra fully parses all flags (persistent + local) before invoking
`PersistentPreRunE`; `RootCmd.PersistentPreRunE` (root.go:73-80) is the sole place that calls
`initLoggerE()` (root.go:79), and by the time it runs, `quietFlag`/`logLevelFlag` package vars
are already populated with the values from argv. Confirmed two ways:
- Standard cobra `Command.execute()` behavior (flags.Parse before *PreRun hooks — not
  specific to this repo, this is upstream cobra's contract).
- `internal/cli/dispatch.go`'s `maybeDispatch` (called from `Execute()`, root.go:130, *before*
  `RootCmd.Execute()`) only short-circuits for **external** `rk-<verb>` binaries: it calls
  `RootCmd.Find(args)` first and immediately returns `(false, nil)` for any builtin command
  (dispatch.go:105-108), e.g. `todo add`, falling through to normal `RootCmd.Execute()` →
  normal cobra flag parsing → `PersistentPreRunE`. So the `--quiet`/`--log-level` repro
  commands in the ticket never touch the external-dispatch path at all.

So the bug is purely "the log call ignores an already-correctly-parsed `quietFlag`", not an
ordering issue. The ticket's own phrasing ("if the init log line fires before `--quiet` is
read, that's the root cause") does not hold; document this explicitly so the fix doesn't
chase a reordering red herring.

`Execute()` (root.go:127-139) also `defer logger.Close()`s and runs `maybeDispatch` before
`RootCmd.Execute()` — irrelevant to this bug but note `logger.Close()` only flushes the
lumberjack file writer (logger.go:257-274); stderr output isn't buffered so nothing there
needs draining before a fix works correctly.

## 5. Existing tests — style references

No test currently exercises `buildLoggerConfig`, `initLoggerE`, or the "reckon initialized"
line at all (`internal/cli/root_help_test.go` is the only root.go test file; no `root_test.go`
exists). No test in the repo captures real `os.Stderr` via `os.Pipe` — the logger writes
directly to `os.Stderr` (or a file) via its own `slog.Handler`, not through cobra's
`cmd.SetOut/SetErr`, so a new test asserting the log line's presence/absence needs either (a)
pass `--log-file` to redirect into a temp file and read it back, or (b) redirect `os.Stderr`
around the `Execute()` call directly (os.Pipe + goroutine drain), since `RootCmd.SetErr` will
not capture it.

Relevant style/reuse:
- `internal/cli/query_test.go:89-109` `resetCLIFlags()` — the established pattern for
  resetting package-global cobra flag vars between `RootCmd.Execute()` calls in the same test
  binary (leakage risk is real and already documented, see dateFlag comment at query_test.go:93-99).
  **Gap**: `resetCLIFlags` resets `vaultFlag`, `jsonFlag`, `ndjsonFlag`, `quietFlag`,
  `dateFlag` but **not** `logLevelFlag` or `logFileFlag` — any new test that sets
  `--log-level`/`--log-file` via `RootCmd.SetArgs` should either reset those too or add them to
  `resetCLIFlags` (pre-existing gap, not introduced by this ticket, but a new logging test
  will be the first to hit it).
- `internal/cli/root_help_test.go` — canonical pattern for driving `RootCmd.Execute()` in a
  test: `SetOut`/`SetErr` to a `bytes.Buffer`, `SetArgs`, `t.Cleanup` to reset, assert on
  `err` and buffer contents (e.g. `TestRootHelp_ListsSubcommands`, lines 21-47). Won't capture
  the log line itself (see above) but is the right shape for asserting `Execute()` still
  succeeds and command output (JSON/pretty) is unaffected by whatever quiet-vs-logger fix
  lands.
- `internal/logger/logger_test.go` — table-driven style reference for logger-level tests
  (`TestLogLevelParsing`, line 137) and direct package-var assertions (`logger`, `logLevel`,
  `logFile`, `tuiMode` are accessible in-package, e.g. `TestInitializeWithConfig_NonTUIMode`,
  lines 65-100). If the fix lives in `buildLoggerConfig`/`logger.Config`, a same-shape test
  belongs here; if it's a pure `if !quietFlag` guard in `root.go`, the test belongs in
  `internal/cli` instead (new `root_test.go`, no such file exists yet).

## 6. docs/REVIEW_PATTERNS.md — relevant pitfall

`docs/REVIEW_PATTERNS.md:204-227`, "Anti-Pattern: Ignoring --quiet Flag" (frequency 5,
🔴🔴) is exactly this bug's shape, just never applied to logger calls before:

```go
// BAD
fmt.Printf("Processing...\n")
// GOOD
if !quietFlag { fmt.Printf("Processing...\n") }
```

Its own preflight-check line (`grep for fmt.Print* in CLI commands without quiet flag
check`) would not have caught this instance since the offending call is `logger.Info(...)`,
not `fmt.Print*` — worth extending that preflight grep pattern, or adding a new pattern entry
for "logger calls at INFO ignoring quiet", once this ticket lands (the doc's own TODO at
line 226 says "Add test pattern for --quiet flag" was never done). No other pitfall entry in
the file mentions CLI flag timing/parsing or logger setup specifically.

## Fix surface (for planner, not yet decided)

Smallest correct change is contained to `internal/cli/root.go`:
- `initLoggerE` (root.go:103-110): guard the `logger.Info("reckon initialized", ...)` call
  with `!quietFlag`, **or**
- `buildLoggerConfig` (root.go:35-60): when `quietFlag` is true and no explicit
  `--log-level`/`LOG_LEVEL` was given, set `cfg.Level = "WARN"` instead of `"INFO"`.

Both are plausible readings of the ticket's "Expected" line (which offers a fallback: "or the
flag's docs should clarify it only affects log level, not command output framing" — but that
framing is backwards vs. section 1's finding: `--quiet` already only affects *stdout* command
framing, and currently affects *nothing* on the logger side, so a docs-only fix would need to
add a caveat rather than "clarify" an existing one). No production code outside `root.go`
needs to change; `buildLoggerConfig`'s unused `isTUIMode bool` parameter (root.go:35) has
exactly one call site (`buildLoggerConfig(false)`, root.go:104) — there is no TUI entrypoint
in this codebase currently (`tui` is a retired/dead verb per
`root_help_test.go:118-119`'s `dying` list), so no second call site needs updating.
