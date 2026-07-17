# Implementation Summary: reckon-fnqs.2

## Status: READY FOR PUSH

## Review Verdict: APPROVE

## What changed
`rk import` (one-shot legacy migrator) moved to `rk migrate legacy`. `import` verb removed and left unclaimed for the future write-glue/ingest seam. No behavior change; flags (`--dry-run`/`--verify`/`--source`) and runtime output/error text preserved verbatim.

- `internal/cli/import.go` → `internal/cli/migrate_legacy.go`: `Use:"import"`→`"legacy"`, identifiers `import*`→`migrateLegacy*`, `init()` attaches child to `migrateCmd`. `"import:"` output prefixes kept (D2).
- `internal/cli/migrate.go` (new): parent `migrateCmd` — `Args: cobra.NoArgs` + `Run: cmd.Help()` so `rk migrate bogus` errors correctly while `RunE == nil`.
- `internal/cli/root.go`: register `migrateCmd` instead of `importCmd`.
- Tests: `import_test.go` → `migrate_legacy_test.go` (retargeted + 6 new); `root_help_test.go` surface lists flipped.

## Verification (through the real binary, not just tests)
- `rk import` → `unknown command "import" for "rk"`
- `rk migrate legacy --help` → shows `--dry-run`/`--verify`/`--source`
- `rk migrate` → help, exit 0
- `rk migrate bogus` → `unknown command "bogus" for "rk migrate"`
- `--json` output: 21 struct tags byte-identical to old `import` command
- `go test ./...` green; `go vet` clean; `gofmt` clean; coverage internal/cli 78%
- 12 target tests: red before impl, all PASS after

## Commits (origin/main..HEAD)
- docs: Add plan and analysis
- test: Write failing tests
- feat: Move legacy migrator to 'rk migrate legacy'
- refactor: Align retargeted test docs
- docs: Preflight report
- docs: Code review (APPROVE) + address stale test comment
- docs: Feedback-loop note + final state

## Out of scope (intentional, per AC)
- `tests/acceptance/README.md:44` and `docs/design/km-architecture-proposal.md` still mention `rk import` (design reserves `import` for the future ingest seam).
