# Code Review — reckon-fnqs.2 (`rk import` → `rk migrate legacy`)

Scope: branch diff vs `origin/main`, `internal/cli/` only. Verified against vendored cobra
`v1.10.2` source, a full `go build ./...`, and a fresh `go test ./internal/cli/... -count=1`
(13/13 relevant tests PASS). gofmt clean, `go vet ./internal/cli/` exit 0.

Bottom line: the move is complete and correct. No Critical or Major defects. Findings below are
Minor/cosmetic or informational.

## Correctness

- Verified the cobra parent/child wiring end-to-end against source:
  - `rk migrate legacy`: `Find` descends to the child; the child keeps `RunE`/`SilenceUsage`/
    `Args: NoArgs`; migration codepath (`textmigrate.Importer{...}` → `Run()`/`Verify()`)
    is byte-identical to the old `runImportE`. (migrate_legacy.go:194-234)
  - Bare `rk migrate`: `NoArgs([])` passes → `Run` calls `cmd.Help()` → nil error, exit 0 (EC-1).
  - `rk migrate bogus`: parent is Runnable (has `Run`), so `execute()` reaches
    `ValidateArgs` → `NoArgs(["bogus"])` → `unknown command "bogus" for "rk migrate"` (EC-2).
    Confirmed in cobra `command.go:954` (`if !c.Runnable() { return flag.ErrHelp }` precedes
    `ValidateArgs`) and `args.go:42` (`NoArgs`).
  - `rk import`: not registered → `RootCmd.Find` returns root with residual arg → `legacyArgs`
    (args.go:34) → `unknown command "import" for "rk"` (EC-3).
- `init()` ordering is safe: `migrate_legacy.go`'s `init()` does `migrateCmd.AddCommand(...)` and
  `root.go`'s `init()` does `RootCmd.AddCommand(migrateCmd)`; both target vars are fully
  initialized before any `init()` runs and share the same pointer (D5 holds).
- No net-new behavior; result types/mappers/`Pretty()` moved verbatim aside from identifier
  renames.

No issues.

## Architecture

1. **[Minor]** The `migrate` parent is *Runnable* (`Run` calling `cmd.Help()` + `Args: NoArgs`),
   which deliberately diverges from the `noteCmd` "no `Run`/`RunE`" pattern the AC cites as the
   model. The divergence is correct and necessary (a non-runnable parented command does **not**
   error on an unknown subcommand — `legacyArgs` returns nil when `HasParent()`, so it would print
   help instead), and it is documented inline. Consequence: the repo now has two divergent
   parent-command behaviors — `rk note bogus` prints help/exit 0 (no error) while `rk migrate
   bogus` errors. Not a defect (migrate's behavior is the AC-required one); consider a follow-up to
   align `note` or note the intended difference in `internal/cli/AGENTS.md`. (migrate.go:5-16)

2. **[Minor, doc-only]** plan.md D4 states the parent "carries no `Args`/`RunE`" and claims that
   "Mirrors `noteCmd` exactly: ... `rk migrate bogus` yields cobra's unknown-command error." That
   rationale is factually wrong about cobra (a no-`Args`/no-`RunE` parented command would print
   help, not error). The **implementation is correct** and supersedes the plan (see preflight
   report and the migrate.go inline comment); flagging only so the stale plan prose isn't taken as
   ground truth. (ticket-work/reckon-fnqs.2/plan.md:30)

## Testing

- Tests pin real behaviors, not trivially-passing shells: T8 asserts the verbatim
  `"import: 1 created, 0 skipped, 0 errored"` summary via `strings.Contains` (correctly avoiding
  the ULID-bearing per-record line); T12 pins the exact `migrate`-scoped unknown-command string;
  T3 pins `rk import` removal; T2 pins the `{legacy}` child set and `RunE == nil`. All located by
  name via `RootCmd.Commands()`, so they never reference production vars (red-state safe).
- The four retargeted scenarios changed only their `SetArgs` line (`import` → `migrate legacy`),
  honoring AC-7.

3. **[Minor]** `TestImportVerbRemoved`'s comment "for as long as `import` remains registered,
   executing it for real must not touch the caller's actual home-directory vault" is stale —
   `import` is no longer registered, so the `RECKON_*` env isolation is now belt-and-suspenders,
   not load-bearing. Harmless but misleading. (migrate_legacy_test.go:194-201)

4. **[Minor]** `TestMigrateBareInvocation` (T11/EC-1) asserts only a nil error; it does not assert
   help text reached stdout, so it under-specifies "prints help/usage." A `strings.Contains(out,
   "Usage:")` (or a subcommand-list check) would fully pin EC-1. Low priority — nil-error +
   `TestMigrateSubcommandSurface` together cover the intent. (migrate_legacy_test.go:216-231)

5. **[Minor, cosmetic]** The helper is still named `runImport` though it now drives `migrate
   legacy`; the doc comment was updated but the identifier wasn't. Ungated per §2, but renaming to
   `runMigrateLegacy` would remove the last `import` residue in the test file.
   (migrate_legacy_test.go:23)

## Maintainability

- Identifier rename (`import*` → `migrateLegacy*`) is complete and consistent across the file
  (command var, flags, reset helper, result types, mappers); grep confirms zero remaining
  non-test references to any `import*` production symbol. JSON tags and `Short`/`Long` text moved
  unedited (neither contains "import"), matching D1/§2.
- Only residue is the cosmetic `runImport` helper name and the stale test comment above.

## Error handling

- D2 honored: `"import:"` / `"import verify:"` output and error prefixes preserved verbatim
  (migrate_legacy.go:98,155,172,186,193,202,211,219,232). All error-wrapping sites use `%w`; the
  three non-`%w` `fmt.Errorf` calls are terminal messages, not wraps — correct.
- Parent has no `SilenceUsage`, so `rk migrate bogus` prints usage alongside the error. This
  matches `RootCmd`'s own behavior for `rk bogus`; the `legacy` child retains `SilenceUsage: true`.
  Consistent, not a defect.

## Performance

- N/A — pure verb-move; no algorithmic or allocation change. Slice pre-sizing in the mappers is
  unchanged from the original.

## Security

6. **[Minor / informational — intended]** Because `rk import` is no longer a cobra builtin, the
   production `cli.Execute()` path now routes `rk import ...` through `maybeDispatch`
   (root.go:130 → dispatch.go:103), which performs a `$PATH` lookup for an external `rk-import`
   binary *before* falling through to cobra's unknown-command error. Pre-change, `import` was a
   builtin and never reached the external-lookup branch. This is the documented, intended
   consequence of freeing the `import` verb (AC §0; a future ingest seam), and no `rk-import`
   binary exists in-repo or on a clean PATH. `TestImportVerbRemoved` calls `RootCmd.Execute()`
   directly and so bypasses `maybeDispatch` — correct for the AC, but note it does not exercise
   this production dispatch branch. Flagged for awareness only; no change required.

## Cross-cutting confirmations

- `rk import` fully removed from the non-test surface: no `importCmd`/`runImportE`/`resetImportFlags`
  references anywhere in `internal/`/`cmd/`; `root.go:98` now registers `migrateCmd`.
- The only remaining `rk import` source strings outside tests are the two known out-of-scope docs
  (`tests/acceptance/README.md:44`, `docs/design/km-architecture-proposal.md:625,641`) — correctly
  left untouched per AC §5.
- `internal/cli/AGENTS.md` contains no stale `import`/`migrate` command reference (only a Go
  `import "os"` example).

**Verdict:** APPROVE
