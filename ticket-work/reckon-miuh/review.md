# Code Review: reckon-miuh — `--quiet` doesn't suppress init log line

## Verdict: APPROVE

The fix is correct, minimal, well-placed, and thoroughly tested. Build, vet, the
full acceptance suite (uncached, incl. the 7 new tests), and the existing
`internal/cli` suite are all green. Nothing functional needs to change. There is
one style-preference comment reword (a borderline provenance phrase) and a few
optional cleanups — none block merge.

Scope reviewed: `internal/cli/root.go` (`buildLoggerConfig`), `tests/acceptance/acceptance_test.go`.

---

## Summary

`buildLoggerConfig` now raises the effective log floor to `WARN` when `quietFlag`
is set and no explicit `--log-level` was given (root.go:43-61). This is the
right shape: level-based suppression at the single chokepoint, `internal/logger`
untouched, generalizing to every INFO-and-below record rather than special-casing
the banner. Precedence (explicit flag > `--quiet` > env > default) is correctly
implemented. Verified locally: `go build ./...`, `go vet -tags acceptance`,
`go test -tags acceptance -count=1 ./tests/acceptance/` (full suite, uncached),
and `go test ./internal/cli/` all pass.

**Dimension coverage:** correctness, architecture, testing, maintainability, and
error handling are addressed in the focus-question answers and observations below.
The remaining two:

- **Performance:** negligible — one added branch evaluated once at startup, no
  hot path. No concern.
- **Security:** no attack surface introduced (no input parsing, no I/O change).
  Forward-looking note only: raising the floor to WARN suppresses all INFO
  records, so if security-relevant *audit* logging is ever added at INFO level,
  `--quiet` would hide it. None exists today (the init banner is the only
  CLI-reachable INFO site). Not actionable now; worth awareness for future
  audit-logging work.

---

## Answers to the five focus questions

### 1. Precedence logic — correct; `== ""` vs `Changed()` is a documented, acceptable simplification

The `if cfg.Level == "" { switch { case quietFlag: WARN; default: env-chain } }`
structure implements the intended ladder exactly:

| Input | Path | Result |
|---|---|---|
| `--log-level X` (X non-empty) | outer `if` skipped, X flows through | X wins ✓ |
| `--quiet`, no `--log-level` | outer `if` entered, `case quietFlag` | WARN, env not consulted ✓ |
| no flag, no quiet | `default` | `LOG_LEVEL` → `RECKON_DEBUG` → `INFO` ✓ |

Edge case `--log-level ""` (literal empty flag value): cobra sets `logLevelFlag = ""`,
which is indistinguishable from unset under the `== ""` check, so `--log-level "" --quiet`
resolves to WARN rather than a hypothetical explicit-empty. This is the only
divergence from an AC-doc-suggested `cmd.Flags().Changed("log-level")` check, and
it is harmless — `""` is not a valid level. The plan (design-decisions §1) called
this out and consciously rejected threading `cmd` through the function to avoid a
signature change. [INFERRED] correct call; no real breakage.

Related, pre-existing (unchanged): a bogus `--log-level FOO` is non-empty, so it
wins the precedence and then silently maps to INFO in `initializeWithConfig`
(logger.go:143-144). Not introduced here, not in scope.

### 2. WARN vs ERROR — no unintended interaction; ERROR still surfaces

slog level ordering is Debug(-4) < Info(0) < Warn(4) < Error(8); a handler with
`Level: LevelWarn` (logger.go:205-206) admits records `>= Warn`, i.e. WARN and
ERROR pass, INFO/DEBUG drop. So `logger.Error(...)` still reaches stderr under
`--quiet`. Confirmed correct.

Additionally, the CLI's primary user-visible failure path does **not** go through
the slog logger at all — command errors return up to cobra (prints to stderr) and
the dispatch path uses `fmt.Fprintf(os.Stderr, "rk: %v\n", err)` (root.go:141).
Neither is gated by the log level, so `--quiet` cannot hide command failures
regardless. No risk to error reporting.

### 3. Test isolation — no shared-state risk

Every new test spawns the real `rk` binary via `exec.Command` (fresh OS process),
so the `initOnce` / global `logLevel` singleton in `internal/logger` lives in the
child's memory and is fresh per invocation — never shared across tests. Env is
injected per-subprocess through `cmd.Env` (the new `execEnv` helper), **not**
`os.Setenv`, so `LOG_LEVEL`/`RECKON_DEBUG` cannot leak into the parent test
process or sibling tests. Each test gets its own `newVault(t)` (two `t.TempDir()`s).
`t.Parallel()` is therefore safe here and across the pre-existing acceptance
suite. This is exactly the isolation the plan reasoned for; the in-process
`resetCLIFlags` `logLevelFlag`/`logFileFlag` gap stays moot because no in-process
logging test was added.

One documented dependency: `TestQuiet_SuppressesInitLog`'s empty-stderr assertion
assumes a clean `todo add` on a fresh vault emits zero WARN/ERROR. The baseline
test confirms only the INFO init line is emitted today, so this holds. If
`todo add` ever legitimately warns on a fresh vault this would fail — but that is
a true signal, not a flaky false positive.

### 4. `LOG_LEVEL=ERROR` + `--quiet` → WARN — real but acceptably out of scope

The concern is genuine: the implementation hard-sets WARN and skips env, so this
one combination yields output marginally *louder* than `ERROR` alone (WARN admits
warnings; ERROR would not). Assessment: **not worth blocking.**

- It requires a deliberate, unusual pairing (env set to the quietest useful level
  *and* `--quiet`).
- The chosen semantics — flag beats env, env not consulted — is internally
  consistent with how `--log-level` itself already beats `LOG_LEVEL`, and is
  explicitly documented in plan.md "Known risks" (not buried).
- Impact is non-destructive, non-security cosmetic noise, in the *opposite*
  direction of the reported bug and far smaller in magnitude.

Optional future refinement (do not block): if a literal "floor" is ever desired,
compute the effective level as the quieter of {WARN, env-requested} so an explicit
`LOG_LEVEL=ERROR` stays honored. That would mean `--quiet` consults env, trading
the current flag>env simplicity — a deliberate design change, not a bug fix.

### 5. Provenance — one borderline phrase; diff is otherwise clean

No `reckon-miuh`, `Phase`, or `plan.md` appears in either changed file. The sole
provenance-flavored comment is in the added lines:

- `tests/acceptance/acceptance_test.go:120` (added) — `// regression baseline matching the ticket's own repro.`

The task-defined provenance check (`reckon-miuh` / `Phase` / `plan.md`) passes
cleanly — **zero** matches in the diff. This phrase is caught only under the
broader `no-provenance-in-comments` memory rule's *spirit*: it cites no ticket ID,
plan.md decision label, or review-issue number, so it is not a rule-defined
violation, but "the ticket's own repro" is author-to-reviewer provenance in spirit,
the category Mike is sensitive about (memory + PR #154 cleanup). Treated as a
style-preference nit, not a blocker (see Suggested nits). The many pre-existing
`plan.md` refs elsewhere in the tree are out of scope per the memory rule ("clean
only when touching those lines").

---

## Suggested nits (non-blocking)

1. **Reword the provenance-flavored comment** at `acceptance_test.go:120` — a
   style preference, not a rule-defined violation. Drop "the ticket" and state the
   behavior instead, e.g.:
   `// --log-level WARN alone (no --quiet) already silences the init line — the`
   `// regression baseline this fix must not disturb.`
   Non-functional; aligns with the spirit of Mike's provenance rule.

## Recommendations (optional)

1. **DRY the two exec helpers.** `execEnv` (acceptance_test.go:96-110) duplicates
   `exec` almost verbatim. Have `exec` delegate: `return v.execEnv(nil, args...)`.
   Removes ~10 duplicated lines and one future edit-in-two-places hazard.
2. **Consider a `--quiet --log-level INFO` test** (edge-case table row): explicit
   INFO shown despite `--quiet`. `TestQuiet_ExplicitLogLevelWins` already covers
   "explicit more-verbose (DEBUG) wins," so this is incremental AC2 coverage, low
   value. Optional.
3. **Doc follow-up (already flagged in AC, out of scope here):** `internal/cli/AGENTS.md`
   §"Standard Output (Respect --quiet)" still frames `--quiet` as stdout-only; its
   scope is now broader. A follow-up doc/ticket is warranted — do not expand this PR.

## Positive observations

- Correct fix shape: level-based, single chokepoint, `internal/logger` untouched —
  generalizes to all INFO sites and avoids re-introducing the `docs/REVIEW_PATTERNS.md`
  per-call `if !quietFlag` anti-pattern on the logger.
- The production comment (root.go:44-46) explains the precedence *intent* (the
  "why") without provenance — exactly the comment style the memory rule wants.
- Test suite is well-targeted: correctly uses the subprocess harness (the only way
  to observe the logger's real `os.Stderr`, per grounding facts), covers baseline,
  core, explicit-wins, warn-alone, redundant, json/ndjson, and both env-var
  variants. The empty-stderr assertion (not just "banner absent") is the stronger,
  more future-proof observable.
- `execEnv` is a clean, minimal addition that passes env per-subprocess rather than
  mutating process-global state — preserving parallel-safety.

## Questions for consideration

- None blocking. The `LOG_LEVEL=ERROR + --quiet` semantics (Q4) is the only
  conscious design tradeoff; it is documented and acceptable as-is.
