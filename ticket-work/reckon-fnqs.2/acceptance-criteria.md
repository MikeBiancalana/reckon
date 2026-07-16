# Acceptance Criteria — reckon-fnqs.2 (Move the one-shot legacy migrator off `rk import`)

Grounded in direct reads of `internal/cli/import.go`, `internal/cli/import_test.go`,
`internal/cli/root.go`, `internal/cli/root_help_test.go`, `internal/cli/note.go` +
`internal/cli/note_v1.go` (the codebase's only existing parent+child cobra command pair),
`internal/textmigrate/migrate.go` (the `Importer` type), and the vendored cobra source
(`github.com/spf13/cobra@v1.10.2/args.go`) for the exact unknown-command error format. This is a
pure verb-move: no new behavior, no textmigrate changes.

## 0. Ground truth (verified, not inferred)

- `internal/cli/migrate.go` (the pre-pivot xid→slug migrator) is **already gone** — deleted by
  reckon-fnqs.1 (#158). `grep -i migrate internal/cli/root.go` → zero hits. `migrate` is free.
- Today's registration: `import.go:19-30` defines `importCmd` (`Use: "import"`, `SilenceUsage:
  true`, `Args: cobra.NoArgs`, `RunE: runImportE`); `root.go:98` does
  `RootCmd.AddCommand(importCmd)`. No other file references `importCmd`/`runImportE`/
  `resetImportFlags` (`grep -rln` confirms `import.go` + `root.go` only).
- Flags (`import.go:38-43`, all `init()`-registered, **zero shorthands**):
  `--dry-run` (bool, default false), `--verify` (bool, default false), `--source` (string,
  default ""). `resetImportFlags` (`import.go:48-57`) zeroes all three vars and clears pflag
  `Changed` on `"dry-run"`, `"verify"`, `"source"`.
- `runImportE` (`import.go:169-232`) logic, in order: mutual-exclusivity check → error
  `"import: --dry-run and --verify are mutually exclusive"` (line 173); `output.ModeFromFlags`;
  `config.LoadWithOverrides` (error-wrap `"import: load config: %w"`, line 183); `--source`
  default via `config.DataDir()` (error-wrap `"import: resolve source dir: %w"`, line 190);
  build `textmigrate.Importer{Source, Dest: cfg.VaultDir, DryRun}`; branch on `--verify`
  (`imp.Verify()`, errors `"import: verify: %w"` line 199 and `"import: verify found %d
  unexpected warning(s) and %d unresolved alias(es)"` line 208) vs. default (`imp.Run()`, errors
  `"import: %w"` line 216 and `"import: %d record(s) failed to migrate"` line 229). Output
  suppressed only when `mode == output.Pretty && quietFlag` (lines 202, 220).
- Result types/JSON tags (`import.go:59-167`, e.g. `importResult`, `importVerifyResult`,
  `Pretty()` methods) are pure data shapes with no dependency on the command's registered name —
  moving the command does not require touching them.
- `Pretty()` output text is literally prefixed `"import: %d created..."` (line 88) and
  `"import verify: %s"` (line 151) — these strings are unconditional today, not derived from
  `cmd.Name()`.
- `internal/textmigrate/migrate.go:30` (`type Importer struct{...}`) and its `Run()`/`Verify()`
  methods are a **separate package** from the retired `internal/cli/migrate.go`; nothing here
  touches `internal/textmigrate/*`.
- `internal/cli/note.go` + `note_v1.go` is the only existing 2-level cobra pattern in this repo:
  parent (`noteCmd`, no `RunE`, no flags) in one file; children attached via a second file's
  `init(){ noteCmd.AddCommand(...) }`. Model the new `migrate`/`migrate legacy` split on this,
  not on a flat rename.
- cobra's unknown-subcommand error (`cobra@v1.10.2/args.go:44`) is exactly
  `fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())` →
  `unknown command "import" for "rk"` / `unknown command "bogus" for "rk migrate"`.
- `cmd/main.go` does `os.Exit(1)` on *any* non-nil error from `cli.Execute()` — the `ExitCode*`
  constants in `root.go:18-22` are dead code (`grep` for their use finds only the declaration).
  "Exit codes preserved" is automatic: every error path here already yielded exit 1 and still
  will; no path needs to map to 2 or 3.
- `dispatch.go`'s `maybeDispatch` (checked for verb-specific special-casing — none found; it's a
  generic `rk-<verb>` PATH lookup) only runs through `cli.Execute()`, not `RootCmd.Execute()`.
  All import_test.go-style tests call `RootCmd.Execute()` directly, bypassing it — irrelevant to
  the tests below, but means a real end-user `rk import` could theoretically hit an external
  `rk-import` binary before cobra's unknown-command path. No such binary exists in this repo or a
  clean PATH; not gated by any AC.
- `tests/acceptance/README.md:44` and `docs/design/km-architecture-proposal.md:625,641` both
  reference `rk import` — the former as a stale planning note (T9), the latter as the **future**
  write-glue seam this ticket's "leaving `import` unclaimed" is explicitly protecting. Neither is
  source code; neither is gated by any AC here.

## 1. Explicit acceptance criteria

- **AC-1 (`rk import` gone).** `RootCmd.Commands()` names do not contain `"import"`.
  `RootCmd.Execute()` with args `["import", ...]` returns a non-nil error containing `unknown
  command "import" for "rk"`.
- **AC-2 (two-level `migrate legacy` structure exists).** `RootCmd.Commands()` names contain
  `"migrate"`; the `migrate` command's own `.Commands()` names equal exactly `{"legacy"}`. The
  parent has no `RunE` (mirrors `noteCmd`).
- **AC-3 (same work, real run).** Given a fixture legacy source with one gen-1 task file, `rk
  migrate legacy --vault <v> --source <s>` succeeds and creates exactly one file under
  `<v>/todos/` — byte-for-byte the same migration codepath as today's `rk import` (same
  `textmigrate.Importer{Source, Dest, DryRun}` construction, same `imp.Run()` call).
- **AC-4 (flags identical).** `--dry-run`, `--verify`, `--source` are registered on the `legacy`
  child with identical types, defaults, and **no shorthands** (matches AC-0's flag audit). Passing
  both `--dry-run` and `--verify` still returns the mutual-exclusivity error, unchanged text:
  `"import: --dry-run and --verify are mutually exclusive"` — see AC-5 on the "identical output"
  scope decision.
- **AC-5 (output identical — literal text, not just shape).** `Pretty()`/error-string content is
  preserved **verbatim**, including the literal `"import:"` / `"import verify:"` prefixes (lines
  88, 151, 173, 183, 190, 199, 208, 216, 229). Note `Pretty()`'s per-record lines (`import.go:
  104-105`, e.g. `"\n  created %s %s -> %s (id %s)"`) embed a freshly-minted ULID — non-
  deterministic across runs — so any assertion on this text must match/contain the summary line
  only (`"import: 1 created, 0 skipped, 0 errored"`), never a whole-string `==` against the full
  multi-line output (see T8). **[OPEN, resolved here]**: the ticket's "identical ... output" is
  read as governing *runtime output* (what the done-when tests actually check),
  not command metadata — `Use` necessarily changes from `"import"` to `"legacy"` since it is now a
  subcommand, and that is expected, not a violation. Do not "clean up" the output prefixes to say
  `"migrate:"` — that would be a text change disallowed by "no new behavior," and no AC/test
  requires it. If a future ticket wants `"migrate legacy:"` output text, that's separate scope.
- **AC-6 (idempotency + `--verify` unchanged).** Re-running against the same source is still a
  no-op on the vault side (skip-not-duplicate) and `--verify` still reports OK/FAILED via the
  same `imp.Verify()` call and error-count logic. This behavior is guaranteed structurally — the
  CLI-layer code calling into `textmigrate.Importer` is untouched — not by a new test; the actual
  skip-logic assertions live in `internal/textmigrate/migrate_test.go` / `verify_test.go`, which
  this ticket does not touch (see §5 "Out of scope").
- **AC-7 (`import_test.go` retargeted, not rewritten).** All 4 existing scenarios
  (`TestImportCmd_RealRun_CreatesTodoFile`, `TestImportCmd_DryRun_SucceedsWritesNoFiles`,
  `TestImportCmd_Verify_SucceedsAfterRealRun`, `TestImportCmd_SourceFlagOverridesDefault`) pass
  unchanged except their `RootCmd.SetArgs` invocation: `["import", "--vault", v, ...]` →
  `["migrate", "legacy", "--vault", v, ...]` (the `runImport` helper at `import_test.go:22-30`).
  Same fixture helper (`writeLegacyFixtureTask`), same assertions.
- **AC-8 (`root_help_test.go` flipped).** `TestRootCommandSurface` (lines 110, 117-120):
  `"import"` moves from the `survivors` slice to the `dying` slice; `"migrate"` moves from
  `dying` to `survivors`. `TestRootHelp_ListsSubcommands` (line 42): the verb list's `"import"`
  is replaced with `"migrate"` (this test only asserts substring presence of survivors — it does
  not need a negative assertion, that's `TestRootCommandSurface`'s job).

## 2. Implicit requirements

- **File layout mirrors `note`/`note_v1.go`, not a flat rename.** New parent `migrateCmd` (no
  flags, no `RunE`, `Use: "migrate"`) in its own declaration; the moved importer becomes
  `migrateLegacyCmd` (`Use: "legacy"`, same `Short`/`Long`/`SilenceUsage`/`Args`/`RunE` body as
  today's `importCmd`) registered via `migrateCmd.AddCommand(migrateLegacyCmd)`. **[INFERRED]**
  filenames `migrate.go` (parent) + `migrate_legacy.go` (child) by analogy with `note.go` +
  `note_v1.go`; not gated by any AC — a single-file implementation is equally acceptable as long
  as AC-2's structural check passes.
- **`root.go:98` swap.** `RootCmd.AddCommand(importCmd)` → `RootCmd.AddCommand(migrateCmd)`
  (only the parent needs registering on `RootCmd`; `legacy` attaches to `migrateCmd`, same as
  `note`'s children never touch `RootCmd.AddCommand` directly).
- **`Short`/`Long` text carries over verbatim onto the child.** Neither `import.go:21`'s `Short`
  ("Migrate legacy SQLite-primary data into vault-native canonical nodes") nor `Long` (`import.go:
  22-26`) contains the word "import" — both can move to `migrateLegacyCmd` unedited. Only `Use`
  changes (`"import"` → `"legacy"`).
- **New parent's `Short`/`Long` text is unscripted by the ticket.** **[INFERRED]** something like
  `Short: "Migrate/import data from other systems"` with `Long` reserving room for future
  non-legacy migrate subcommands is reasonable; exact wording is not gated by any AC.
- **Flag var/reset-function names may be renamed or left alone — cosmetic, not gated.** Whether
  `importDryRunFlag` etc. become `migrateLegacyDryRunFlag` etc. (and `resetImportFlags` becomes
  `resetMigrateLegacyFlags`) is an internal Go-identifier choice with zero user-facing effect;
  either way the `defer reset...Flags(cmd)` call at the top of the RunE body must remain (prevents
  flag-value leakage across `RootCmd.Execute()` calls in the same test binary, per the existing
  pattern already documented at `import.go:45-47`).
- **`SilenceUsage: true` and `Args: cobra.NoArgs` carry over unchanged** onto the `legacy` child
  (`import.go:27-28`) — dropping either would change error-output shape on bad invocations, which
  "identical output" (AC-5) forbids.
- **`internal/textmigrate` package: zero edits.** The only touch point is the existing call
  sites `imp.Verify()` (`import.go:197`) / `imp.Run()` (`import.go:214`) moving file/location
  along with the rest of the RunE body — the `Importer` struct and its methods
  (`internal/textmigrate/migrate.go`, `verify.go`) are untouched.
- **Result types/helpers (`importResult`, `toImportResult`, etc.) move with the RunE body** — no
  functional change; renaming their identifiers (e.g. `importResult` → `migrateLegacyResult`) is
  optional cosmetic cleanup, not gated by any AC (JSON tags like `"tasks"`, `"created"` etc. are
  unaffected either way since they don't reference the verb name).

## 3. Edge cases

| # | Scenario | Expected |
|---|---|---|
| EC-1 | `rk migrate` with no subcommand | prints help/usage, exit 0 — no `RunE` on the parent, same as `rk note` alone today |
| EC-2 | `rk migrate bogus-subcommand` | `RootCmd.Execute()` returns error `unknown command "bogus-subcommand" for "rk migrate"` (verified cobra format, §0); exit 1 via `cmd/main.go` |
| EC-3 | `rk import ...` (old verb, any args) | error `unknown command "import" for "rk"`; exit 1 — AC-1's core check |
| EC-4 | `rk migrate legacy --dry-run --verify` (both long flags) | unchanged mutual-exclusivity error, `"import: --dry-run and --verify are mutually exclusive"` (AC-5 governs the literal text) |
| EC-5 | `rk migrate legacy --dry-run --source <s>` combined (not mutually exclusive) | dry-run still reads from `--source`, still writes nothing — untested combination today and still untested after the move; not newly required, just confirmed non-conflicting |
| EC-6 | `--source` omitted | falls back to `config.DataDir()`, unchanged (`import.go:186-192`) |
| EC-7 | `--quiet`/`--json`/`--ndjson` combined with `migrate legacy` | output-suppression logic (`mode == output.Pretty && quietFlag`) and JSON/NDJSON rendering unaffected by the command-tree move |

## 4. Test scenarios (given/when/then, traceable to AC)

| # | Given | When | Then | AC |
|---|---|---|---|---|
| T1 | fresh `RootCmd` | read `RootCmd.Commands()` names | `"import"` absent, `"migrate"` present | AC-1, AC-2 |
| T2 | fresh `RootCmd` | read `migrateCmd.Commands()` names | equals exactly `{"legacy"}` (new test, mirrors `TestNoteSubcommandSurface`). Unlike `RootCmd` itself, an exact-set check is safe here: cobra's lazy help/completion registration (the reason `root_help_test.go:94-103` avoids exact-set on `RootCmd`) attaches only to the command `Execute()` is called on, i.e. `RootCmd`, never to a non-root child like `migrateCmd` | AC-2 |
| T3 | fresh `RootCmd` | `RootCmd.Execute()` with args `["import"]` | error contains `unknown command "import" for "rk"` | AC-1 |
| T4 | fixture source dir + one gen-1 task file (`writeLegacyFixtureTask`) | `rk migrate legacy --vault <v> --source <s>` | succeeds; `<v>/todos/` has exactly 1 entry (retarget of `TestImportCmd_RealRun_CreatesTodoFile`) | AC-3, AC-7 |
| T5 | same fixture | `rk migrate legacy --vault <v> --source <s> --dry-run` | succeeds; `<v>/todos/` does not exist (retarget of `TestImportCmd_DryRun_SucceedsWritesNoFiles`) | AC-4, AC-7 |
| T6 | completed real `migrate legacy` run against the fixture | `rk migrate legacy --vault <v> --source <s> --verify` | succeeds (retarget of `TestImportCmd_Verify_SucceedsAfterRealRun`) | AC-6, AC-7 |
| T7 | `RECKON_DATA_DIR` set to an empty temp dir; `--source` points at the real fixture | `rk migrate legacy --vault <v> --source <s>` | `<v>/todos/` has exactly 1 entry, proving `--source` (not the env-derived default) was read (retarget of `TestImportCmd_SourceFlagOverridesDefault`) | AC-4, AC-7 |
| T8 | same fixture as T4 | `rk migrate legacy --vault <v> --source <s>`, capture stdout | stdout **contains** the summary line `"import: 1 created, 0 skipped, 0 errored"` (`strings.Contains`, NOT `==` — the following per-record line embeds a freshly-minted ULID and is non-deterministic, see AC-5) — **[INFERRED] new scenario**; today's `import_test.go` asserts no output text, so "identical output" (AC-5) is otherwise unguarded | AC-5 |
| T9 | fresh `RootCmd` | iterate `RootCmd.Commands()` names | `"migrate"` ∈ survivors, `"import"` ∈ dying (edit `TestRootCommandSurface`, `root_help_test.go:110,117-120`) | AC-8 |
| T10 | fresh `RootCmd` | `rk --help` | output contains `"migrate"` (edit `TestRootHelp_ListsSubcommands`'s verb list, `root_help_test.go:42`, replacing `"import"`) | AC-8 |
| T11 | fresh `RootCmd` | `RootCmd.Execute()` with args `["migrate"]` | exit 0, help/usage printed, no error (EC-1) | AC-2 |
| T12 | fresh `RootCmd` | `RootCmd.Execute()` with args `["migrate", "bogus"]` | error contains `unknown command "bogus" for "rk migrate"` (EC-2) | AC-2 |
| T13 | fixture source | `rk migrate legacy --source <s> --dry-run --verify` | unchanged mutual-exclusivity error text (EC-4) | AC-4, AC-5 |

## 5. Out of scope

- Building the write-glue `rk import` seam (`--into feeds/jira`, NDJSON envelope consumer) — the
  ticket's Problem section names this as the *reason* `import` must be freed, not a task here.
- Any edit to `internal/textmigrate/*.go` (`migrate.go`, `verify.go`, `tasks.go`, `notes.go`,
  `checklists.go`, `journal.go`, `writer.go`) or their `_test.go` siblings — the entry call
  (`imp.Run()`/`imp.Verify()`) moves file, its target does not change.
- Adding a new CLI-level idempotency test (double-run-and-assert-skip-count) — idempotency is
  guaranteed by not touching the RunE↔Importer call, and is already covered at the
  `internal/textmigrate` unit level; AC-6 does not require a new CLI-level test.
- Renaming the literal `"import:"` / `"import verify:"` output-text prefixes to say `"migrate"`
  or `"migrate legacy:"` — explicitly disallowed by "no new behavior" / AC-5's resolution of the
  "identical output" requirement. A future cosmetic ticket may revisit this.
- Updating `tests/acceptance/README.md:44` or `docs/design/km-architecture-proposal.md` — both
  are planning prose, not source or tests; neither is gated by any Done-when criterion.
- Any change to `cmd/main.go`'s exit-code handling or the unused `ExitCode*` constants
  (`root.go:18-22`) — confirmed dead code, irrelevant to this ticket's exit-code requirement,
  which is already trivially satisfied (every error path yields the same `os.Exit(1)` before and
  after the move).
- Renaming Go-internal identifiers (flag vars, `reset*Flags`, result-type names) — optional,
  ungated cosmetic choice (§2).
