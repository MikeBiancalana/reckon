# Code Review: reckon-lrw2 — Separate new vault default from legacy `~/.reckon`

## Summary

**APPROVE.** This is a minimal, surgical, correctly-scoped implementation of Option A
from the approved plan. It relocates *only* the new `Config.VaultDir` default from
`$HOME/.reckon` to `$HOME/reckon` (reusing the `AppName` const), leaves every legacy
function and shipped caller untouched, keeps path resolution pure, updates the one
existing test that hard-coded the old default, adds six well-targeted hermetic tests,
and reconciles the two doc comments plus the `--vault` help text. All three "Done when"
sub-criteria (distinct default / no collision / stays pure) are satisfied and verified
by tests. `go build ./...` and `go test ./...` both pass clean.

Verdict: APPROVE

---

## Functional Review

### Requirements Met
- [✅] AC-1(a) — a distinct v1 vault default is chosen and *documented* (not a TODO):
  `$HOME/reckon` via `filepath.Join(home, AppName)` (config.go:132), with the default
  stated in `Load()`'s doc comment (config.go:106), `LoadWithOverrides`'s doc comment
  (config.go:119-120), and the `--vault` flag help (root.go:114).
- [✅] AC-1(b) — legacy and new no longer collide by default. Legacy `DataDir()` →
  `$HOME/.reckon` (`"."+AppName`, config.go:32); new `VaultDir` → `$HOME/reckon`
  (`AppName`, config.go:132). Not equal, and non-nesting in **both** directions
  (siblings under `$HOME`); the default cache `$HOME/.cache/reckon` also does not nest
  in `$HOME/reckon`, so the EC-7 guard still passes (proven by `TestLoad_Defaults`
  succeeding).
- [✅] AC-1(c) — resolution stays pure. `LoadWithOverrides` contains no `os.MkdirAll`
  on any branch; the VaultDir-default branch does only `os.Getenv` + `os.UserHomeDir`
  + `filepath.Join` (all read-only), and the guard uses `filepath.Rel` (pure). No
  `mkdir`-on-resolve was introduced while relocating the default.
- [✅] Edge cases handled: explicit `RECKON_VAULT=$HOME/.reckon` escape hatch still
  honored; `RECKON_DATA_DIR` and `RECKON_VAULT` remain independent; cache-inside-vault
  guard re-validates automatically against the new default.
- [✅] Error handling unchanged and comprehensive: the two `os.UserHomeDir()` failure
  paths still wrap with `%w` (config.go:130, 145); no new panics or silent fallbacks.

### Crux Verification (the six points this ticket turns on)

1. **Non-collision correctness** — VERIFIED. `.reckon` ≠ `reckon`; `filepath.Rel`
   between the two yields `../reckon` and `../.reckon` respectively (both start with
   `..`), so neither is nested in the other. `filepath.Join(home, AppName)` with
   `AppName="reckon"` is unambiguously `$HOME/reckon`. Because both defaults derive
   from the same `AppName` const (`"."+AppName` vs `AppName`), they stay distinct even
   if `AppName` is ever renamed.
2. **Purity of the whole Load/NewConfig/LoadWithOverrides chain** — VERIFIED by
   inspection: zero filesystem writes on the VaultDir-default branch (or anywhere in
   `LoadWithOverrides`). `Config`'s struct doc (config.go:99) still accurately claims
   purity.
3. **Test quality / hermeticity** — VERIFIED. Every new test uses `t.Setenv` for
   `HOME` + all four relevant env vars and a fresh `t.TempDir()` HOME; no reliance on
   real `$HOME` for assertions (the one real-`$HOME` read, in the pre-existing
   `TestNewConfig_TempVaultHermetic`, only asserts it was *not* touched). The plan's
   specific trap is avoided: `TestDefaults_LegacyVaultNonCollision` computes the legacy
   path as a literal (config_test.go:177) rather than calling `DataDir()`, so no
   `mkdir` side effect leaks into the non-collision check.
4. **Scope discipline** — VERIFIED. The six legacy functions
   (`DataDir`/`JournalDir`/`TasksDir`/`NotesDir`/`DatabasePath`/`LogDir`) and all their
   shipped callers are byte-for-byte untouched; the only production-code edit is the
   single string on config.go:132 plus two doc comments and one help string.
5. **Docs/help consistency** — VERIFIED. `--vault` help updated to `~/reckon`
   (root.go:114); the `--log-file` help (root.go:110) correctly *left* referencing
   `~/.reckon/logs/reckon.log`, which is the legacy `LogDir()` (app-operational logs),
   not the vault.
6. **No stale current-spec doc** — VERIFIED. A repo-wide grep confirms no *current*
   spec doc asserts the vault-root default as `~/.reckon`. `composable-redesign.md:928`
   pins only the *cache* location; its line 931 lists `.reckon/` as an ignore glob
   (an app-config dir *inside* the vault, analogous to `.obsidian/`), not the vault
   root. The remaining `~/.reckon` hits (`docs/logging.md`, `reckon-spec_2026-06-15.md`,
   `reckon-redesign_2026-06-15.md`, `migration-tui-redesign.md`,
   `2026-01-17-review-and-assessment.md`) are either legacy app-data/logs references or
   superseded (proposed) DB-primary-generation docs — correctly out of scope.

### Issues
None blocking. See minor optional suggestions under Test Review and Code Quality.

---

## Design Review

### Architecture
- [✅] Fits existing architecture — mirrors the codebase's "pure resolve vs. mkdir at
  point of use" split (the `internal/index` `DBPath()` vs `OpenWithParser()` precedent
  cited in the plan). The vault directory will be created by the first writer (T4),
  not by resolution.
- [✅] Follows subsystem patterns — reuses the `AppName` const rather than a new string
  literal, keeping the two defaults derived from one source of truth.
- [✅] Appropriate abstraction level — a one-line default change; no new env var, no
  precedence change, no new abstraction (correctly resists YAGNI temptation to add a
  collision-warning guard, which AC §3/§5 place out of scope).
- [✅] Simplest reasonable solution — smallest possible diff that closes the collision.

### Suggestions
None required. `$HOME/reckon` is a defensible, discoverable choice consistent with the
Obsidian-vault framing and provably non-nesting against both legacy `~/.reckon` and the
default cache.

---

## Test Review

### Coverage
- [✅] Happy path tested — `TestLoad_Defaults` (updated), `TestLoad_DefaultVault_NotLegacyDir`.
- [✅] Non-collision tested — `TestDefaults_LegacyVaultNonCollision` (both-direction
  nesting via `filepath.Rel`, mirroring the EC-7 technique).
- [✅] Legacy regression tested — `TestDataDir_LegacyDefault_Unchanged` (table-driven
  over all five derivatives; may create dirs on disk, acceptable in a temp HOME).
- [✅] Purity tested — `TestLoad_DefaultVault_Pure` (fresh HOME, asserts `VaultDir`
  absent after resolve).
- [✅] Env-interaction / edge cases tested — `TestLoad_VaultEnvOverride_LegacyPath`
  (escape hatch), `TestEnvIndependence_DataDirVsVault` (independence).
- [✅] Tests would catch regressions — a stray `mkdir` in the vault-default branch would
  fail `TestLoad_DefaultVault_Pure`; a reversion to `.reckon` would fail three tests.

### Test Quality
- Clear, descriptive names tied to AC clauses; each test carries a comment mapping it
  to the acceptance criterion it covers.
- Independent and hermetic via `t.Setenv` + `t.TempDir` (no `t.Parallel` mixing that
  would conflict with `t.Setenv`, which is correct).
- TDD honored: the failing tests landed in a separate commit (`07d869b`) before the fix
  (`17ecdcf`).

### Missing Tests (optional, non-blocking)
1. `TestLoad_DefaultVault_Pure` only `os.Stat`s `VaultDir`; the AC scenario also says
   "no new directories were created *anywhere* under `$HOME`." A stronger assertion
   would diff `os.ReadDir(tmp)` before/after. The purity is already well-covered in
   aggregate (no `mkdir` exists in the code path, and `TestNewConfig_TempVaultHermetic`
   checks HOME is untouched), so this is a nice-to-have, not a gap.

---

## Code Quality

### Readability
- The inline `// $HOME/reckon` comment on config.go:132 makes the resolved value
  obvious at the call site.
- Doc comments on `Load` and `LoadWithOverrides` now state the concrete default
  consistently.

### Maintainability
- No duplication introduced; `relInside` in the test file intentionally mirrors the
  production EC-7 helper and is documented as such.
- Deriving both defaults from `AppName` avoids a future drift hazard.

### Anti-Patterns Detected
None.

---

## Security Review
- [✅] No input validation surface changed; env-var handling unchanged.
- [✅] No SQL / no injection surface.
- [✅] No new path-traversal risk — `filepath.Join(home, AppName)` joins a constant to
  the OS-provided home dir. If anything, moving the default *out* of the shared
  `~/.reckon` reduces the risk of new tooling reading/overwriting legacy DB/journal data.
- [✅] No resource/DoS surface.

---

## Performance Considerations
- [✅] No inefficiencies — a single string change on a cold config-resolution path.
- [✅] No resources to clean up; no DB queries; trivially scalable.

---

## Comparison to Plan

**Followed plan:** Yes — exactly.

**Deviations:** None material. The plan floated an *optional* extra clause on
`LoadWithOverrides`'s doc comment; the implementation added it (config.go:119-120),
which is the stronger choice.

**Improvements over plan:** Test set is slightly broader than the minimum
(`TestDataDir_LegacyDefault_Unchanged` exercises all five derivatives table-driven),
strengthening the Option-A "legacy untouched" guarantee.

**Incidental change:** `internal/spike/roundtrip/roundtrip.go` carries a whitespace-only
`gofmt` realignment (var block), committed separately as "go fmt for reckon-lrw2". It is
unrelated to the ticket but mechanically required by `go fmt ./...` and harmless.

---

## Patterns for Future
- **Good pattern reinforced:** derive sibling defaults from a shared const
  (`AppName` vs `"."+AppName`) so they cannot silently converge later.
- **Good pattern reinforced:** pure path resolution + `mkdir` at point-of-use (matches
  `internal/index` `DBPath()`/`OpenWithParser()`), with a dedicated purity test guarding
  against a well-meaning `mkdir`-on-resolve regression.

---

## Decision

**Verdict: ✅ APPROVE**

**Required changes:** None.

**Suggested improvements (optional):**
1. Optionally strengthen `TestLoad_DefaultVault_Pure` to assert nothing new appears under
   `$HOME` (not just that `VaultDir` is absent).

**Notes for downstream (reckon-uv09 / T4):** T4 is the first writer into the vault and
must `os.MkdirAll(VaultDir)` at point of use (do not push creation back into
`LoadWithOverrides`, or it will break `TestLoad_DefaultVault_Pure`). Also note the
in-vault app-config convention `<vault>/.reckon/` (used by saved views in
`internal/cli/query.go` and skipped by the reconcile ignore-glob in
`internal/index/reconcile.go`): under the new default this becomes `$HOME/reckon/.reckon/`,
which is distinct from legacy `$HOME/.reckon` — no collision, just worth being aware of.

## Sign-off

Reviewed by: Opus Code Reviewer
Date: 2026-07-03
