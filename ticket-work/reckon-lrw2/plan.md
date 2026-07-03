# Implementation Plan: reckon-lrw2 — Separate new vault default from legacy `~/.reckon`

## Summary of approach

**Chosen option: A** — relocate *only* the new `Config.VaultDir` default; leave the legacy `DataDir()` and all its derivatives (`JournalDir`/`TasksDir`/`NotesDir`/`DatabasePath`/`LogDir`) completely untouched.

**Chosen new default path: `$HOME/reckon`** (i.e. `filepath.Join(home, AppName)`, no leading dot).

**Why Option A.** The new `Config` surface has exactly two production callers today (`rk index` at `internal/cli/index.go:28`, `rk query` at `internal/cli/query.go:107`), both `requiresDB=false` experimental commands, and nothing has yet written real vault content — T4 (`rk log`, reckon-uv09) will be the first, which is precisely why this is its blocker. The legacy `DataDir()` surface, by contrast, is exercised by 10+ shipped v0 commands via `initServiceE()` (`internal/cli/root.go:151`) and 15+ call sites, and real user data plausibly already lives at `~/.reckon/reckon.db`, `~/.reckon/journal/*.md`, etc. Options B and C move that shipped/in-use legacy default, which would orphan existing on-disk data (nothing physically moves files) — a data-migration side effect that the acceptance criteria explicitly place out of scope (that's T9/reckon-s6oh). Option A changes only the pre-launch surface, giving the smallest diff and zero regression risk to shipped commands. Option D is rejected because a same-path-plus-guard still "collides by default" (fails the literal "Done when" wording) and any filesystem probe in the guard risks the purity requirement. This matches the analysis's own lean toward A.

**Why `$HOME/reckon`.** `Config.VaultDir` is documented in code as the "git-synced content root" — a user-visible document directory the user browses and edits directly, analogous to an Obsidian vault (the design doc even lists `.obsidian/` in its ignore globs, `docs/design/composable-redesign.md:930`). That framing argues against hiding it as a dotfile and against XDG *data-dir* conventions (which are for app-private state). `$HOME/reckon` is discoverable, natural, symmetric with the legacy `"."+AppName` (new = `AppName`, legacy = `"."+AppName`), and provably non-colliding: `$HOME/reckon` is neither equal to nor nested in `$HOME/.reckon`, and the default cache `$HOME/.cache/reckon` remains outside it (EC-7 guard still passes). Alternatives considered and rejected:
- `$HOME/vault` — generic, doesn't identify the app; confusing if the user runs other vault-based tools.
- `$HOME/Documents/reckon` — assumes a localized `~/Documents` exists; not portable to headless/minimal/non-English setups.
- `$HOME/.local/share/reckon` (XDG_DATA_HOME) — hidden, app-private convention; contradicts the user-visible/browsable framing.
- `$HOME/.reckon-vault` — still hidden (dotfile) and visually adjacent to legacy `.reckon` (confusion/fat-finger risk).

I deliberately do *not* introduce any `XDG_DATA_HOME` lookup for VaultDir — the vault is content, not app data, so a plain `$HOME/reckon` keeps resolution simple and predictable. Env precedence is unchanged: explicit arg → `RECKON_VAULT` → `$HOME/reckon`.

## Files to modify

### 1. `internal/config/config.go` (the only logic change)
- **Line 131** — the sole functional edit: `vaultDir = filepath.Join(home, ".reckon")` → `vaultDir = filepath.Join(home, AppName)` (resolves to `$HOME/reckon`). Reuses the existing `AppName = "reckon"` const (line 11); no new constant strictly required, though a `// $HOME/reckon` inline comment aids readability.
- **Lines 105–107** — update the `Load()` doc comment: the line `// VaultDir: RECKON_VAULT env else $HOME/.reckon.` must become `$HOME/reckon`. This is the doc-comment half of AC-1(a)'s "documented, not a TODO" requirement.
- **Lines 118–120** — `LoadWithOverrides` doc comment: optionally add one clause naming the new default (`… then $HOME/reckon`) so the concrete default is stated on the real logic function, per AC-1(a) ("documented e.g. in LoadWithOverrides's doc comment").
- **Do NOT touch** `DataDir()` line 32 (`filepath.Join(home, "."+AppName)`), and do NOT touch the EC-7 guard (lines 150–157) — it re-validates against whatever the new default is, automatically. Do NOT add any `os.MkdirAll` to the VaultDir branch (purity, AC-1(c)).

### 2. `internal/config/config_test.go`
- **UPDATE `TestLoad_Defaults` (lines 13–38):** line 30 `wantVault := filepath.Join(tmp, ".reckon")` → `filepath.Join(tmp, "reckon")`. Also update the descriptive comment on lines 10–12 that asserts "must resolve VaultDir to `$HOME/.reckon`" → `$HOME/reckon`. The `CacheDir` assertion (lines 34–37) is unchanged.
- **No change** to `TestLoad_VaultEnvOverride` (43–61), `TestNewConfig_TempVaultHermetic` (66–98), `TestCacheNotInsideVault` (102–111), `TestCacheInsideVault_Rejected` (116–124) — all are override-based or hermetic and never assert the literal default path.
- **New tests to add** — see Test Scenarios below.

### 3. `internal/cli/root.go`
- **Line 114** — `--vault` help text: `"Override vault directory (default: $RECKON_VAULT or ~/.reckon)"` → `"Override vault directory (default: $RECKON_VAULT or ~/reckon)"`.
- **Explicitly do NOT change line 110** — the `--log-file` help mentions `~/.reckon/logs/reckon.log`; that is the *legacy* `LogDir()` (app-operational logs, unrelated to the vault). Leaving it is correct under Option A; flagging so the implementer does not over-correct it.

### 4. Docs (verification, likely no edit)
- A grep confirms no *current* spec doc states the vault-root default as `~/.reckon`: `docs/design/composable-redesign.md:928` only pins the *cache* location (`~/.cache/reckon/<vault-id>/index.db`, unchanged). `foundation-review-2026-06-24.md` and `code-walkthrough-foundation.md` are historical/append-only (AC §2 says do not edit). Implementer should run a final `grep -rn "\.reckon" docs/` sanity pass; expect no current-spec change needed. No user-facing README currently states the vault default.

## Design decisions (with alternatives)

- **A over B/C:** blast radius and data-safety. A touches one branch + one test assertion + one help string; B/C additionally move the shipped legacy default, which strands real on-disk data unless a migration runs — explicitly out of scope (AC §2 "Not a data migration", §5). Only A leaves every shipped v0 command and its tests untouched.
- **A over D:** D leaves both defaults at `~/.reckon` and bolts on a collision guard. This fails AC-1(b) ("no longer collide *by default*") and, if the guard `Stat`s the filesystem, erodes AC-1(c) purity. A removes the collision at the source.
- **`$HOME/reckon` over XDG/dotfile candidates:** the vault is user-visible content, not app-private state (see Summary). Keeping it a plain, visible home-level directory matches the Obsidian-vault framing and is trivially non-nesting vs both legacy `~/.reckon` and default cache `~/.cache/reckon`.
- **No new env var, no precedence change:** AC §5 forbids introducing env vars or changing precedence. `RECKON_VAULT`/`RECKON_CACHE`/`RECKON_DATA_DIR` stay independent (they already are).
- **Purity preserved by omission, not by new code:** the fix is a literal string change in an already-pure branch. The precedent to follow is `internal/index/index.go` — `DBPath()` (pure resolve) vs `OpenWithParser()` (mkdir at point of use, lines 76–78). Whichever future code writes into the vault (T4) creates the dir; resolution never does.

## Test scenarios (mapped to acceptance-criteria.md §4)

Every new test must set `t.Setenv("HOME", tmp)` and clear `RECKON_VAULT`, `RECKON_CACHE`, `RECKON_DATA_DIR`, and `XDG_CACHE_HOME` to avoid ambient-env leakage, mirroring the existing tests. Written failing-first by the test-writer.

**Updated (existing):**
- `TestLoad_Defaults` — change `wantVault` to `$HOME/reckon` (AC §4 "Load() default vault dir no longer equals the legacy dir"). Also implicitly covers "cache-inside-vault guard still enforced against relocated default" since `Load()` returning no error with cache `$HOME/.cache/reckon` proves non-nesting.
- `TestLoad_VaultEnvOverride` — unchanged; already covers "RECKON_VAULT override still works."
- `TestNewConfig_TempVaultHermetic` — unchanged; already covers NewConfig purity.

**New (added):**
- `TestLoad_DefaultVault_NotLegacyDir` (AC "Load() default … no longer equals legacy"): assert `Load().VaultDir != filepath.Join(tmp, ".reckon")` and `== filepath.Join(tmp, "reckon")`.
- `TestDefaults_LegacyVaultNonCollision` (AC "legacy DataDir() and new Config.VaultDir resolve to different paths"): compute legacy path as `filepath.Join(tmp, ".reckon")` and new via `Load()`; assert not equal AND non-nesting in *both* directions via `filepath.Rel` (result must start with `..`), mirroring the EC-7 check. (Note: prefer computing the legacy literal rather than calling `DataDir()` here, so this test stays free of the legacy mkdir side effect — see Risks.)
- `TestDataDir_LegacyDefault_Unchanged` (AC "DataDir() default with no env override" + "derivatives still nest under `~/.reckon`"): assert `DataDir()` returns `$HOME/.reckon` and that `JournalDir/TasksDir/NotesDir/DatabasePath/LogDir` each nest under it (`journal/`, `tasks/`, `notes/`, `reckon.db`, `logs/`). Guards Option A's promise that legacy is untouched. This test may create dirs on disk (legacy behavior) — acceptable in a temp HOME.
- `TestLoad_DefaultVault_Pure` (AC "Load()/NewConfig()/LoadWithOverrides() create no directories" + "purity holds when parent doesn't exist"): with a fresh temp HOME and no overrides, call `Load()`, assert no error, non-empty `VaultDir`, and `os.Stat(cfg.VaultDir)` returns not-exist afterward, plus no new entries created directly under `$HOME`.
- `TestLoad_VaultEnvOverride_LegacyPath` (AC "RECKON_VAULT pointed at legacy dir still honored"): set `RECKON_VAULT=$HOME/.reckon`; assert `Load().VaultDir == $HOME/.reckon` (escape hatch not blocked).
- `TestEnvIndependence_DataDirVsVault` (AC "RECKON_DATA_DIR override does not affect Config.VaultDir, and vice versa"): set `RECKON_DATA_DIR=/tmp/legacy-override`, leave `RECKON_VAULT` unset; assert `DataDir() == /tmp/legacy-override` and `Load().VaultDir == $HOME/reckon`.

## Known risks / ambiguities

- **Pre-launch vault relocation (theoretical):** an experimenter who ran `rk index`/`rk query` against the old `~/.reckon` default will now get an empty `$HOME/reckon` unless they set `RECKON_VAULT`. AC §3 marks this acceptable/theoretical since nothing has shipped writing vault content. No migration in scope.
- **Test hygiene trap:** the non-collision test must NOT call `DataDir()` if it also asserts "nothing was created," because `DataDir()` mkdirs `$HOME/.reckon` as a documented side effect. Keep the purity test (`TestLoad_DefaultVault_Pure`) and the legacy-side test (`TestDataDir_LegacyDefault_Unchanged`) separate; in the non-collision test, compute the legacy path as a literal.
- **Residual `notes/` subdir name overlap:** both legacy (`~/.reckon/notes`) and the planned vault layout (`<vault>/notes`, per `.gitattributes`) use `notes/`. Harmless once roots differ under Option A, but never nest the vault inside `~/.reckon`. Not actionable here; noted for T4/T9.
- **Purity regression guard:** the implementer must not "helpfully" add `os.MkdirAll` to the relocated VaultDir branch. `TestLoad_DefaultVault_Pure` is the guard; ensure it lands before implementation (TDD).
- **Explicit-collision guarding is out of scope:** whether to warn when `RECKON_VAULT` explicitly equals the legacy path is a deferred judgment call (AC §3, §5) — do not add it here.

### Critical Files for Implementation
- internal/config/config.go
- internal/config/config_test.go
- internal/cli/root.go
- internal/index/index.go (pure-resolve vs mkdir-at-open precedent to mirror)
