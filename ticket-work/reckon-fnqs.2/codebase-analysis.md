# reckon-fnqs.2 — Codebase analysis: move `rk import` to `rk migrate legacy`

## 1. Files touched

| File | Verdict | Detail |
|---|---|---|
| `internal/cli/import.go` (284 lines) | **RENAME + EDIT** → e.g. `internal/cli/migrate_legacy.go` | Whole file is the importer's CLI surface: command var, flags, result types, `RunE`, mapping helpers. See §3 for exact identifiers. |
| `internal/cli/import_test.go` (133 lines) | **RENAME + EDIT** → e.g. `internal/cli/migrate_legacy_test.go` | All 4 tests dispatch through `RootCmd.Execute()` with literal `"import"` in `SetArgs`; retarget to `{"migrate", "legacy", ...}`. Fixture helper `writeLegacyFixtureTask` is unaffected. |
| `internal/cli/root.go` | **EDIT** (2 lines) | `root.go:98` registers `RootCmd.AddCommand(importCmd)` — replace with a `migrateCmd` registration. No new import-path added to the file's `import` block; `internal/textmigrate` moves with the command file, not into root.go. |
| `internal/cli/root_help_test.go` (162 lines) | **EDIT** (3 spots) | `TestRootHelp_ListsSubcommands` (line 42) and `TestRootCommandSurface` (line 110) both list `"import"` as a survivor — replace with `"migrate"`. `TestRootCommandSurface`'s `dying` slice (lines 117–120) currently lists `"migrate"` — remove it (now a survivor) and add `"import"` (now dead) so the test locks in both directions. |
| new file, e.g. `internal/cli/migrate.go` | **NEW** | Parent `migrate` cobra command, ~13 lines, mirrors `note.go` exactly (see §2). |
| `internal/textmigrate/*` | **NO CHANGE** | `Importer`, `Run()`, `Verify()` etc. are called by CLI glue only; nothing here references cobra or command names. |

No other file in the repo references `importCmd`, `runImportE`, or any unexported identifier from `import.go` (`ticket-work/reckon-fnqs.1/codebase-analysis.md`'s cross-reference sweep already confirmed zero external references into this file, and a fresh grep here confirms it — all "import" hits outside `internal/cli/import*.go` are either the Go `import` keyword, unrelated prose in `docs/design/km-architecture-proposal.md`, or the acceptance-suite gap note, all covered in §4).

## 2. Parent-command-with-subcommand pattern to follow

The repo already has exactly one resource with subcommands: `note`. Its split is the template:

- **Parent command** — `internal/cli/note.go:5-13`: bare `cobra.Command{Use: "note", Short, Long}`, no `RunE`, no flags. Exported via `func GetNoteCommand() *cobra.Command { return noteCmd }` (note.go:11-13).
- **Children attached in a separate file's own `init()`** — `internal/cli/note_v1.go:121-134`: flags are registered on each child (`noteCreateCmd.Flags()...`), then the *last* line of that `init()` is `noteCmd.AddCommand(noteCreateCmd, noteShowCmd, noteRenameCmd, noteIndexCmd)` (note_v1.go:133).
- **Root registration** — `internal/cli/root.go:91`: `RootCmd.AddCommand(GetNoteCommand())`.

For `migrate`/`legacy` (only one child, today):
- `migrateCmd` parent → new small file (e.g. `migrate.go`), same shape as `note.go`. **[INFERRED]** a `GetMigrateCommand()` getter is *not* required to match repo convention — every other top-level command (`todoCmd`, `addCmd`, `queryCmd`, `indexCmd`, `adoptCmd`) is registered directly by var name at `root.go:92-98`; `note`'s getter indirection looks incidental to `note`, not a pattern other resources follow. Simplest correct choice: `RootCmd.AddCommand(migrateCmd)` directly, matching the majority.
- `legacyCmd` child → the renamed `import.go` file's command var, attached via `migrateCmd.AddCommand(legacyCmd)` in that file's existing `init()` (which already registers the three flags — just add this one line after the `f.StringVar(...)` calls, same slot `note_v1.go:133` occupies).

## 3. Identifiers touched inside the renamed file

Current name → must exist as, at minimum, a command reachable as `rk migrate legacy` with unchanged behavior. Renaming everything below is **recommended** (matches `noteCreateCmd`/`runNoteCreateE`/`resetNoteFlags` resource+action naming) but the ticket only hard-requires the `Use` string and root wiring to change:

| Current (import.go) | Recommended new name | Line |
|---|---|---|
| `var importCmd` | `var legacyCmd`, `Use: "legacy"` (was `Use: "import"`) | 19-30 |
| `importDryRunFlag`, `importVerifyFlag`, `importSourceFlag` | `legacyDryRunFlag`, `legacyVerifyFlag`, `legacySourceFlag` (or `migrateLegacy*`) | 32-36 |
| `func init()` (registers 3 flags) | unchanged body; **add** `migrateCmd.AddCommand(legacyCmd)` as new last line | 38-43 |
| `func resetImportFlags(cmd *cobra.Command)` | `resetLegacyFlags` | 45-57 |
| `importResult`, `importTypeResult`, `importRecordEntry`, `importSkippedEntry`, `importErroredEntry`, `importVerifyResult` | rename prefix `import`→`legacy` (JSON tags are lowercase field names, e.g. `"tasks"`, unaffected either way — output JSON shape does not change) | 61-140 |
| `func runImportE` | `runLegacyE` | 169 (assign to `legacyCmd.RunE`) |
| `func toImportResult`, `toImportTypeResult`, `toImportVerifyResult` | `toLegacyResult`, `toLegacyTypeResult`, `toLegacyVerifyResult` | 236, 248, 267 |

**Untouched (no rename needed):** the `textmigrate.Importer` type and its `Run()`/`Verify()` methods (`internal/textmigrate/migrate.go:30,112`) — the CLI glue calls `imp.Run()` / `imp.Verify()` regardless of what the CLI-side wrapper types are called (import.go:194-214).

## 4. Every "import" string that is command-surface-wired

| Location | String | Action |
|---|---|---|
| `import.go:20` | `Use: "import"` | → `Use: "legacy"` |
| `root.go:98` | `RootCmd.AddCommand(importCmd)` | → `RootCmd.AddCommand(migrateCmd)` |
| `import_test.go:27` | `SetArgs(append([]string{"import", "--vault", vault}, args...))` | → `{"migrate", "legacy", "--vault", vault}` |
| `root_help_test.go:42` | survivors list contains `"import"` | → `"migrate"` |
| `root_help_test.go:110` | survivors list contains `"import"` | → `"migrate"` |
| `root_help_test.go:117-120` | dying list contains `"migrate"` | remove `"migrate"`, add `"import"` |
| `tests/acceptance/README.md:44` | prose: `` `rk import` legacy migration (T9) against a real gen-1 fixture. `` | **[OPEN]** cosmetic-only doc line (a "known gap", not an actual test) — update to `rk migrate legacy` for accuracy; not build/test-breaking either way. |
| `docs/design/km-architecture-proposal.md:625,641` | future-seam design prose using `rk import --into feeds/jira` as the *next* (unbuilt) generic ingest verb | **No change** — this is exactly the future use the ticket reserves `import` for; confirms the ticket's premise, do not touch. |

`internal/cli/dispatch.go`'s external-binary dispatch (`maybeDispatch`, `firstNonFlag`) is verb-name-agnostic — it calls `RootCmd.Find(args)` generically (dispatch.go:105) and only falls through to external `rk-<verb>` binaries when cobra finds no match. Once `migrateCmd`/`legacyCmd` are registered, `rk migrate legacy` resolves via cobra automatically; **no dispatch.go change needed**. Verified no `rk-import` or `rk-migrate` external-binary fixture exists in the test tree.

## 5. Pitfalls

- **Two-level `Args` on parent vs. child.** `migrateCmd` (parent) should carry no `Args` restriction (mirrors `noteCmd`, `RootCmd` — bare invocation prints help). Only `legacyCmd` keeps `Args: cobra.NoArgs` (import.go:28) — do not accidentally copy `NoArgs` onto the parent, or `rk migrate` alone would error instead of printing help/subcommand list.
- **Flag-reset ordering (existing bug class in this codebase, not new).** `resetLegacyFlags` (was `resetImportFlags`) is called via `defer` inside `runLegacyE` itself (import.go:170), *not* via the shared `resetCLIFlags()` in `query_test.go:100-108`. That shared helper only resets `vaultFlag`/`jsonFlag`/`ndjsonFlag`/`quietFlag`/`dateFlag` — it does **not** know about per-command flags. Confirm the retargeted test file keeps calling the command-local reset (still correct after rename) and that `t.Cleanup(resetCLIFlags)` in the retargeted tests continues to also exist (both are needed together, same as today).
- **"Identical output" ambiguity in the ticket's Done-when.** No test asserts the literal string `"import:"` anywhere (`import_test.go` only asserts structural results — file counts, err/nil — never output text; confirmed by grep). The `Pretty()` methods hard-code the prefix `"import: %d created..."` (import.go:88) and every wrapped error uses an `"import: ..."` prefix (import.go:173,183,190,199,208,216,229). **[OPEN — decide before implementing]**: rename these string literals to `"migrate legacy: ..."` for coherent UX under the new verb (recommended — nothing currently pins the old string), or leave them as `"import: ..."` to minimize diff. Either choice keeps all existing tests green; pick one and apply consistently across all ~8 occurrences rather than mixing.
- **Dead code after rename** (docs/REVIEW_PATTERNS.md "Dead Code After Refactoring", the one directly-applicable pattern in that file for this ticket): if only some `import*` identifiers are renamed and others left behind (e.g. renaming the command var but forgetting `toImportResult`/`toImportVerifyResult`), the file will still compile (they're just unexported names) but the naming becomes inconsistent/misleading. Do the rename as one atomic pass across the whole file rather than piecemeal.
- **cobra `Long` text on `import.go:22-26`** currently reads "Read gen-1 task files... write vault-native canonical node files..." with no reference to command name — safe to reuse verbatim under `legacyCmd`, no self-referential `rk import` string embedded in it (verified, no `Example` field is set on the original command either — nothing else to update there).
- **`internal/textmigrate` package name stays `textmigrate`** — the ticket only moves the CLI verb, not the package; do not rename the package or its import path in the retargeted file's import block (`import.go:10` → carries over unchanged: `"github.com/MikeBiancalana/reckon/internal/textmigrate"`).
