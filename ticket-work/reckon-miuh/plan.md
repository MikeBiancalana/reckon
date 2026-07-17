# Implementation Plan: reckon-miuh — `--quiet` doesn't suppress init log line

## Summary

Under `rk --quiet`, the `time=… level=INFO msg="reckon initialized"` banner (and any future INFO/DEBUG record) still reaches stderr because `quietFlag` is never wired into the logging path. Fix shape **(b)**: when `--quiet` is set and no explicit `--log-level` was given, `buildLoggerConfig` raises the effective log level to `WARN`. This is level-based, so it generalizes to every `logger.Info` call site through the one existing chokepoint rather than special-casing the banner. The change is confined to a single function; `internal/logger` and the 9 existing stdout `--quiet` call sites are untouched.

## Approach: fix shape (b), not (a) or hybrid

| Option | Verdict |
|---|---|
| (a) `if !quietFlag` guard around `root.go:108` | Rejected. Satisfies the literal repro but not AC1's "any INFO record, any subcommand" framing; re-introduces the exact `docs/REVIEW_PATTERNS.md:204-227` anti-pattern on the logger; would need repeating at every future INFO site. |
| (b) raise effective level to WARN in `buildLoggerConfig` | **Chosen.** Both more general AND smaller (one function, zero per-call-site guards). Reuses the only mechanism that already silences INFO — `slog.HandlerOptions{Level}` (`logger.go:205-207`). |
| hybrid (both) | Rejected. Redundant parallel suppression paths; AC "Out of scope" bullet 5 explicitly forbids per-site guards on top of the level mechanism. |

## Files to modify

| File | Target | Change |
|---|---|---|
| `internal/cli/root.go` | `buildLoggerConfig` (lines 35-60), specifically the `if cfg.Level == ""` block at 43-53 | Add a `quietFlag` branch: when no explicit `--log-level` (`cfg.Level == ""`) **and** `quietFlag` is true, set `cfg.Level = "WARN"` before consulting `LOG_LEVEL`/`RECKON_DEBUG`. Reads the `quietFlag` package global directly — same style as the function already reading `logLevelFlag`. No signature change (keeps `isTUIMode bool`). |
| `internal/logger/logger.go` | — | **No change.** Stated deliberately: the logger package must not learn CLI-flag semantics, and `buildLoggerConfig` is already the sole point that assembles the effective level from flags/env. `initializeWithConfig` (127-217) already maps `"WARN"` → `slog.LevelWarn` and threads it into the handler; nothing to touch. |
| `tests/acceptance/acceptance_test.go` | new tests behind `//go:build acceptance` | Add subprocess tests (see Test Scenarios). This is the only file with a harness that observes the logger's real `os.Stderr`. |

Concrete production edit (the whole functional change):

```go
if cfg.Level == "" {
    // No explicit --log-level. --quiet raises the floor to WARN, suppressing
    // INFO (incl. the "reckon initialized" banner) and below. --quiet beats
    // LOG_LEVEL/RECKON_DEBUG: flag > env, matching --log-level's own precedence.
    switch {
    case quietFlag:
        cfg.Level = "WARN"
    default:
        cfg.Level = os.Getenv("LOG_LEVEL")
        if cfg.Level == "" {
            debugVal := os.Getenv("RECKON_DEBUG")
            if debugVal == "1" || debugVal == "true" {
                cfg.Level = "DEBUG"
            } else {
                cfg.Level = "INFO"
            }
        }
    }
}
```

## Design decisions

**Explicit `--log-level` always wins (AC2).** Detection stays `cfg.Level == ""` (i.e. `logLevelFlag == ""`), unchanged from today. Because `--log-level DEBUG|INFO|WARN` makes `logLevelFlag` non-empty, the whole quiet/env block is skipped and the explicit level flows straight through — so `--quiet --log-level INFO` correctly shows INFO (edge-case table). No `cmd` plumbing needed.
- Alternative considered: `cmd.Flags().Changed("log-level")`. Only differs for the degenerate `--log-level ""` (empty string), which isn't a valid level anyway. Rejected to avoid threading `cmd` into `buildLoggerConfig`; [INFERRED] the `== ""` check is acceptable and consistent with current behavior.

**OPEN item 1 — `--quiet` vs `LOG_LEVEL`/`RECKON_DEBUG` (RESOLVED: `--quiet` wins).** When `--quiet` is set with no explicit `--log-level`, env vars are not consulted; level is WARN. Rationale: flags beat env is already the established order (`--log-level` flag beats `LOG_LEVEL` in the current code), and `--quiet` is a per-invocation flag — more specific than an ambient env var. Placing the `quietFlag` branch above the env reads in the `switch` encodes this. So `LOG_LEVEL=DEBUG rk --quiet …` → WARN. Matches the AC's [INFERRED] recommendation.
- Alternative: env beats quiet (a `DEBUG` env as a "debug override"). Rejected as inconsistent with the existing flag>env precedence.

**OPEN item 2 — `--quiet` + `--log-file` (RESOLVED: single global level; file sink also drops INFO).** The logger has one global `logLevel` feeding one handler (`logger.go:31,204-215`); `--quiet` raises that one level, so a `--log-file` sink loses INFO too — identical to how `--log-level WARN --log-file x` already behaves. Per-sink levels (quiet console, verbose file) would require splitting the single-handler architecture and are explicitly out of scope (AC "Out of scope" bullet 4). Documented as a known tradeoff, not silently decided.

**Additive, not a replacement (AC3).** `--quiet` keeps both jobs: existing stdout `mode == output.Pretty && quietFlag` suppression (9 sites, untouched) plus the new stderr-floor raise. Nothing in the 9 stdout sites is edited.

**No help-text change.** The flag's existing "Suppress non-essential output" (root.go:84) now accurately describes the behavior; no doc edit is required. `internal/cli/AGENTS.md` / `docs/REVIEW_PATTERNS.md` updates are AC-designated follow-ups, out of scope here.

## Reachability correction (affects test design)

`internal/cli` imports only config, index, logger, node, output, textmigrate — **not** `journal` or `service`. The 23 non-init `logger.Info` sites all live in `internal/journal` and `internal/service`, which no wired CLI command reaches (`rk today` was migrated off journal — today.go's header comment says so; its real imports carry no journal). **Therefore `root.go:108` is the only INFO record any current `rk` subcommand emits.** Consequence: the AC's "`rk today` touching `SaveJournal`" scenario cannot be exercised — writing it would be a false-green (no INFO emitted with or without the fix). The correct observable is that stderr is *fully empty* under `--quiet` on a clean success path, which simultaneously proves the banner and the whole INFO class are gone. The level-based mechanism is what guarantees the "any subcommand" AC structurally; that is the design guard against a future regression to a string-based guard.

## Test scenarios

All stderr-observing tests MUST use the `tests/acceptance` subprocess harness via `v.exec(...)` or `v.runErr(...)` (both return stderr; `v.run` at acceptance_test.go:66-73 discards stderr on success and cannot see the asserted line). New tests go behind `//go:build acceptance`. No new in-process `internal/cli` test is added, so the `resetCLIFlags` `logLevelFlag`/`logFileFlag` gap (query_test.go:100-109) stays moot.

| # | Name (suggested) | Given / When / Then | Harness |
|---|---|---|---|
| 1 | `TestQuiet_SuppressesInitLog` (core) | Given `--quiet`; when `rk --quiet todo add "x"`; then stderr does not contain `reckon initialized` **and stderr is empty**. | subprocess |
| 2 | `TestBaseline_EmitsInitLog` | Given no flags; when `rk todo add "x"`; then stderr contains `msg="reckon initialized"` (documents unchanged default). | subprocess |
| 3 | `TestQuiet_ExplicitLogLevelWins` | Given `--quiet --log-level DEBUG`; when `rk … todo add "x"`; then stderr contains `reckon initialized` (AC2). | subprocess |
| 4 | `TestLogLevelWarn_SuppressesInitLog` | Given `--log-level WARN` (no `--quiet`); when run; then stderr lacks the init line (ticket's own repro / regression baseline). | subprocess |
| 5 | `TestQuiet_RedundantWithWarn` | Given `--quiet --log-level WARN`; when run; then stderr lacks the init line. | subprocess |
| 6 | `TestQuiet_JSONStdoutIntact` | Given `--quiet --json` (and repeat `--ndjson`); when `rk todo add "x"`; then stdout is valid parseable JSON/NDJSON and stderr has no INFO records. | subprocess |
| 7 | `TestQuiet_EnvLogLevelLoses` | Given `LOG_LEVEL=DEBUG` env + `--quiet`, no explicit `--log-level`; when run; then stderr lacks the init line (encodes OPEN-item-1 resolution: quiet beats env). Add `RECKON_DEBUG=1` variant. | subprocess |
| 8 | (assert, not new test) stdout `--quiet` suppression unchanged | AC3 is already covered by existing `todo_test.go` T-7.2 (todo_test.go:1407-1423), `add_test.go`, `adopt_test.go`, `query_test.go` — these must pass unmodified. | existing in-process |

Help-path edge (`rk --quiet --help`, bare `rk --quiet`): `PersistentPreRunE`/`initLoggerE` don't run on cobra's help path, so no log line today and none after the fix — optional low-value regression assertion, can be a subprocess test asserting empty stderr if desired.

The "second, non-init INFO suppressed" AC scenario is intentionally **not** written as a distinct test (see Reachability correction); test #1's empty-stderr assertion covers the observable, and the single-global-level design covers the generalization.

## Known risks / ambiguities

- **`LOG_LEVEL=ERROR rk --quiet` yields WARN, which is *louder* than `ERROR` alone.** The implementation hard-sets WARN and skips env rather than taking `max(WARN, envLevel)`, so the word "floor" is imprecise for this one degenerate combination. Accepted per the OPEN-item-1 resolution (env not consulted when quiet is set); no test covers it. Surfaced explicitly, not buried.
- **In-process shared logger state.** `initOnce`/global `logLevel` (logger.go:36) make in-process level assertions brittle; this is why all new tests are subprocess-based (fresh process each). Do not add in-process stderr tests.
- **AC drift on `rk today`.** The AC references a journal INFO site reachable from `rk today` that is no longer wired. The plan supersedes that scenario rather than encoding a false-green; flag for reviewer awareness.
- **Empty-stderr assertion assumes a clean success path emits no WARN/ERROR.** True for `todo add` on a fresh vault; if a chosen command legitimately warns, test #1 should target `todo add` specifically (as written) rather than a warn-prone command.

## Implementation steps

1. Edit `buildLoggerConfig` (root.go:43-53) to add the `quietFlag` → `WARN` branch as shown.
2. Add subprocess tests #1-#7 to `tests/acceptance/acceptance_test.go` using `v.exec`/`runErr`.
3. Run `go test -tags acceptance ./tests/acceptance/` and the existing `go test ./internal/cli/` (AC3 regression).

### Critical Files for Implementation
- internal/cli/root.go
- internal/logger/logger.go
- tests/acceptance/acceptance_test.go
- internal/cli/query_test.go
- internal/cli/todo_test.go
