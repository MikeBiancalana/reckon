# Plan: reckon-fnqs.2 — Move legacy migrator off `rk import` → `rk migrate legacy`

## Summary

Move the one-shot legacy migrator from top-level `rk import` to a `rk migrate legacy` subcommand. This is a pure verb-move: a new parent `migrate` cobra command (no `RunE`, mirroring `noteCmd`) plus a `legacy` child that carries today's importer logic, flags, result types, and output verbatim. `rk import` is fully removed (registration + var), so `rk import ...` returns cobra's `unknown command "import" for "rk"`. No behavior changes, no `internal/textmigrate` changes, and the `"import:"` / `"import verify:"` output/error prefixes are preserved literally. Work splits across the TDD pipeline: Phase 3 retargets/adds tests (test files only); Phase 4 renames the production file and rewires registration (no test-file edits).

## Files to modify

| File | Phase | Reason |
|---|---|---|
| `internal/cli/import.go` → rename `internal/cli/migrate_legacy.go` | 4 | Whole file is the importer CLI surface; becomes the `legacy` child, `Use:"import"`→`Use:"legacy"`, identifiers `import*`→`migrateLegacy*`, `init()` gains `migrateCmd.AddCommand(migrateLegacyCmd)`. |
| `internal/cli/migrate.go` (NEW, ~10 lines) | 4 | Parent `migrateCmd` var (`Use:"migrate"`, no `RunE`, no `Args`, no flags), mirroring `note.go`. |
| `internal/cli/root.go` (line 98) | 4 | `RootCmd.AddCommand(importCmd)` → `RootCmd.AddCommand(migrateCmd)`. |
| `internal/cli/import_test.go` → rename `internal/cli/migrate_legacy_test.go` | 3 | Retarget `runImport` helper `SetArgs` (line 27) `{"import",…}`→`{"migrate","legacy",…}`; add new-scenario tests; helper `writeLegacyFixtureTask` moves unchanged. |
| `internal/cli/root_help_test.go` (lines 42, 110, 117-120) | 3 | Flip `import`→`migrate` as survivor in two lists; move `import` into the `dying` slice and remove `migrate` from it. |

Out of scope (confirmed, do not touch): `internal/textmigrate/*`, `internal/cli/dispatch.go`, `cmd/main.go`, `root.go:18-22` `ExitCode*` constants, `tests/acceptance/README.md`, `docs/design/km-architecture-proposal.md`.

## Design decisions

**D1 — Rename internal `import*` identifiers to `migrateLegacy*` (chosen) vs. minimal-diff (rejected).**
Rename atomically across the whole file: `importCmd`→`migrateLegacyCmd`, `runImportE`→`runMigrateLegacyE`, `resetImportFlags`→`resetMigrateLegacyFlags`, `importDryRunFlag/importVerifyFlag/importSourceFlag`→`migrateLegacy*Flag`, result types `importResult`/`importTypeResult`/`importRecordEntry`/`importSkippedEntry`/`importErroredEntry`/`importVerifyResult`→`migrateLegacy*`, mappers `toImportResult`/`toImportTypeResult`/`toImportVerifyResult`→`toMigrateLegacy*`.
Justification: matches the only in-repo parent+child precedent (`noteCreateCmd`/`runNoteCreateE`/`resetNoteFlags`) and the acceptance-criteria §2 naming; leaving `import*` names inside a file named `migrate_legacy.go` is the "dead code / misleading name after refactoring" pattern the analysis pitfalls flag. All identifiers are unexported and none are referenced by any test or other file (grep confirms `importCmd` appears only in `import.go` + `root.go:98`), so the rename is zero-coupling and user-invisible. Shorter `legacy*` prefix considered; `migrateLegacy*` chosen for full parent-prefix consistency with `note`. `AGENTS.md` is silent on identifier prefixes, so this stays a convention-match rather than a hard rule.

**D2 — Keep `"import:"` / `"import verify:"` output and error prefixes verbatim (chosen) vs. rename to `"migrate legacy:"` (rejected).**
The `Pretty()` summary prefix (line 88), verify prefix (line 151), and all error wraps (lines 173, 183, 190, 199, 208, 216, 229) stay literally `"import…"`. Justification: acceptance-criteria AC-5 mandates verbatim preservation and T8 (authored in Phase 3) asserts `strings.Contains(stdout, "import: 1 created, 0 skipped, 0 errored")` — renaming the prefix makes T8 red with no way for the pipeline to reconcile, since the test surface pins the old string. The codebase-analysis §Pitfalls suggested renaming to `"migrate legacy:"`; that is consciously rejected because the acceptance-criteria (which defines the tests) overrides it and the "no net-new behavior" constraint forbids the text change. A future cosmetic ticket may revisit.

**D3 — Register parent directly (`RootCmd.AddCommand(migrateCmd)`), no `GetMigrateCommand()` getter.** Matches the majority of top-level commands (`todoCmd`, `addCmd`, `queryCmd`, `indexCmd`, `adoptCmd` at `root.go:92-98`); `note`'s getter is incidental, not a pattern to propagate.

**D4 — Parent carries no `Args`/`RunE`/`SilenceUsage`; child keeps `SilenceUsage:true` + `Args:cobra.NoArgs`.** Mirrors `noteCmd` exactly: bare `rk migrate` prints help/exit 0; `rk migrate bogus` yields cobra's unknown-command error. Copying `NoArgs` onto the parent would wrongly error on bare `rk migrate`.

**D5 — `init()` ordering is safe.** `migrate_legacy.go`'s `init()` adds `legacy` to `migrateCmd`; `root.go`'s `init()` adds `migrateCmd` to `RootCmd`. Both target vars are fully initialized before any `init()` runs (Go guarantees), and the shared pointer makes the final tree correct regardless of cross-file `init()` order — identical to how `note_v1.go` + `root.go` already cooperate.

## Test scenarios

Traceable to acceptance-criteria §4. "Test surface" = Phase 3; all live in test files only. Cross-phase constraint: every new test must locate the command by name via `RootCmd.Commands()` / iterating children (like `TestNoteSubcommandSurface`), never by referencing the `migrateCmd`/`migrateLegacyCmd` production vars — otherwise the test package fails to compile in the red state before Phase 4 exists.

| ID | Phase | Test function | Given / When / Then |
|---|---|---|---|
| T1 | 3 (via T9 edit) | `TestRootCommandSurface` | Given fresh RootCmd, when reading `Commands()` names, then `import` absent + `migrate` present — realized by the T9 edit, not a standalone test. |
| T2 | 3 (NEW) | `TestMigrateSubcommandSurface` | Given RootCmd, when reading the `migrate` child's `Commands()` names, then equals exactly `{"legacy"}`; parent has no `RunE`. Mirrors `TestNoteSubcommandSurface`. |
| T3 | 3 (NEW) | `TestImportVerbRemoved` | Given RootCmd, when `Execute(["import"])`, then error contains `unknown command "import" for "rk"`. |
| T4 | 3 (retarget) | `TestImportCmd_RealRun_CreatesTodoFile` | Given fixture task, when `migrate legacy --vault --source`, then `todos/` has exactly 1 entry. SetArgs-only change. |
| T5 | 3 (retarget) | `TestImportCmd_DryRun_SucceedsWritesNoFiles` | Given fixture, when `… --dry-run`, then `todos/` does not exist. SetArgs-only change. |
| T6 | 3 (retarget) | `TestImportCmd_Verify_SucceedsAfterRealRun` | Given completed real run, when `… --verify`, then succeeds. SetArgs-only change. |
| T7 | 3 (retarget) | `TestImportCmd_SourceFlagOverridesDefault` | Given empty `RECKON_DATA_DIR` + fixture at `--source`, when `migrate legacy …`, then `todos/` has 1 entry (proves `--source` read). SetArgs-only change. |
| T8 | 3 (NEW, separate func) | `TestMigrateLegacyCmd_OutputSummary` | Given fixture, when run and capture stdout, then `strings.Contains(out, "import: 1 created, 0 skipped, 0 errored")` — never `==` (per-record line embeds a non-deterministic ULID). Must be its own function; bolting onto T4 would violate AC-7's "SetArgs-only" pin. |
| T9 | 3 (edit) | `TestRootCommandSurface` | Move `import` survivor→dying; `migrate` dying→survivor (lines 110, 117-120). |
| T10 | 3 (edit) | `TestRootHelp_ListsSubcommands` | Replace `"import"` with `"migrate"` in the substring verb list (line 42). |
| T11 | 3 (NEW) | `TestMigrateBareInvocation` | When `Execute(["migrate"])`, then nil error, help printed, exit 0 (EC-1). |
| T12 | 3 (NEW) | `TestMigrateUnknownSubcommand` | When `Execute(["migrate","bogus"])`, then error contains `unknown command "bogus" for "rk migrate"` (EC-2). |
| T13 | 3 (NEW) | `TestMigrateLegacy_MutualExclusivity` | When `migrate legacy --dry-run --verify`, then error text `import: --dry-run and --verify are mutually exclusive` (EC-4, verbatim per D2). |

Phase 3 red state: production `import.go` is unchanged, so the package still compiles (tests reference only stable symbols `RootCmd`, `setupQueryVault`, `resetCLIFlags`, `writeLegacyFixtureTask`); the new/retargeted tests fail at runtime until Phase 4. Retargeted tests keep both `t.Cleanup(resetCLIFlags)` and the command-local `defer reset…Flags` (both required together, unchanged by the rename). Optional: rename the four `TestImportCmd_*` functions to `TestMigrateLegacyCmd_*` for coherence — ungated; if renamed it still happens in Phase 3.

## Known risks or ambiguities

- [RESOLVED] Two source docs conflict on D2: codebase-analysis §Pitfalls recommends `"migrate legacy:"`; acceptance-criteria AC-5 + T8 pin verbatim `"import:"`. Acceptance-criteria wins (it defines the Phase-3 tests). Do not rename the prefixes.
- [RISK] Test-var coupling in red state: if a Phase-3 test references `migrateCmd`/`migrateLegacyCmd` directly, the package won't compile before Phase 4. Enforce name-lookup via `RootCmd.Commands()` for all new tests.
- [RISK] AC-7 scope creep: the four retargeted tests must change only their `SetArgs` line; the output-summary assertion (T8) belongs in a new function.
- [KNOWN GAP, intentional] `tests/acceptance/README.md:44` and `docs/design/km-architecture-proposal.md:625,641` still reference `rk import` and are deliberately left untouched (planning prose, not gated; the design doc reserves `import` for the future generic-ingest seam). A downstream reviewer may flag these as an incomplete move — they are out of scope by AC §5.
- [LOW] `internal/cli/AGENTS.md` is stale (documents retired v0 verbs) and silent on identifier naming; it only confirms the generic Cobra pattern. D1's `migrateLegacy*` prefix is a convention-match to `note`, not an AGENTS.md mandate.
- [NON-ISSUE, verified] No external `rk-import`/`rk-migrate` dispatch binary or fixture exists; `dispatch.go` is verb-agnostic, so `rk migrate legacy` resolves via cobra with no dispatch change. Exit codes are automatically preserved (`cmd/main.go` maps any non-nil error to exit 1 identically before/after).
