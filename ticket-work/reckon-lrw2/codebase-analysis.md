# Codebase Analysis: reckon-lrw2 — legacy `~/.reckon` config overlap vs new vault default

Scope: `internal/config/config.go` (both the legacy and new surfaces live in the
same file), every caller of the legacy funcs, the new `Config` resolution path,
XDG/env conventions, related design docs, and existing "pure path resolution"
patterns elsewhere in the codebase. All paths relative to the worktree root
(`/home/chadd/repos/reckon/.worktrees/reckon-lrw2`).

---

## 1. Legacy config functions — `internal/config/config.go`

All six legacy functions live in one file, all bottoming out in `DataDir()`.

| Func | Lines | Default path | mkdir? |
|---|---|---|---|
| `DataDir()` | 18–38 | `RECKON_DATA_DIR` env else `$HOME/.reckon` | yes, `os.MkdirAll` on **both** branches (18-25 for the env-override branch, 32-35 for the default branch) |
| `JournalDir()` | 42–54 | `DataDir()/journal` | yes (49) |
| `TasksDir()` | 58–70 | `DataDir()/tasks` | yes (65) |
| `NotesDir()` | 74–86 | `DataDir()/notes` | yes (81) |
| `DatabasePath()` | 89–96 | `DataDir()/reckon.db` (`DbName` const, line 12) | no direct mkdir, but calls `DataDir()` which does |
| `LogDir()` | 167–179 | `DataDir()/logs` | yes (174) |

Every one of these functions has a side effect on every call: at minimum it
transitively calls `DataDir()`, which unconditionally `os.MkdirAll`s the
resolved directory. There is no way to "just compute" a legacy path without
creating it on disk — resolution and creation are fused. `RECKON_DATA_DIR` is
an existing test-only override (used in ~62 call sites across the repo, mostly
tests, per `grep -rn RECKON_DATA_DIR`).

`AppName = "reckon"` and `DbName = "reckon.db"` constants are at lines 10–13.

## 2. New `Config` struct and `Load`/`NewConfig`/`LoadWithOverrides`

Same file, lines 98–163.

```go
type Config struct {
    VaultDir string // git-synced content root
    CacheDir string // per-device index cache; must not be inside VaultDir
}
```

- `Load()` (108–110) → `LoadWithOverrides("", "")`.
- `NewConfig(vaultDir)` (114–116) → `LoadWithOverrides(vaultDir, "")`.
- `LoadWithOverrides(vaultDir, cacheDir)` (121–163) is the real logic:
  - **VaultDir** (123–133): explicit arg, else `RECKON_VAULT` env, else
    `filepath.Join(home, ".reckon")` — **the same literal default as legacy
    `DataDir()`**.
  - **CacheDir** (136–148): explicit arg, else `RECKON_CACHE` env, else
    `$XDG_CACHE_HOME/reckon`, else `$HOME/.cache/reckon`.
  - **Guard** (150–157): if `cacheDir` is inside (or equal to) `vaultDir`,
    returns an error citing `EC-7` — this is the *only* validation
    `LoadWithOverrides` performs; it does not check the legacy `DataDir()`
    default at all.
  - Returns `&Config{VaultDir, CacheDir}` (159–162). **No filesystem writes
    anywhere in this function** — confirmed pure by inspection and by
    `TestNewConfig_TempVaultHermetic` in `config_test.go` (asserts `$HOME`'s
    mtime is untouched after `NewConfig`).

**Collision confirmed**: with no env overrides, `DataDir()` and
`LoadWithOverrides("","")` both resolve to `$HOME/.reckon` — the *exact same
directory*. The only difference is that the legacy call additionally creates
it.

The doc comment on `Config` (99) already states the aspiration: *"Resolution is
pure — no directories are created during Load/NewConfig/LoadWithOverrides."*
That's true today only because `LoadWithOverrides` itself does no I/O; nothing
enforces it stays true if a future change (e.g. relocating the default) adds a
`MkdirAll` to "help" the new default exist. There's no test that would fail if
someone added one to the VaultDir-default branch specifically (the closest is
the whole-`NewConfig` hermetic test, which does cover this).

### `internal/config/config_test.go`

- `TestLoad_Defaults` (13–38) hard-codes `wantVault := filepath.Join(tmp,
  ".reckon")` — **this assertion encodes the current colliding default** and
  will need to change if the default moves.
- `TestLoad_VaultEnvOverride`, `TestNewConfig_TempVaultHermetic`,
  `TestCacheNotInsideVault`, `TestCacheInsideVault_Rejected` all exercise
  `RECKON_VAULT`/`RECKON_CACHE`/`XDG_CACHE_HOME` and the EC-7 guard; none of
  them assert anything about the legacy `DataDir()` default, so they're
  independent of whatever legacy does.
- No existing test asserts legacy-vs-new non-collision — that check does not
  exist yet anywhere in the suite.

## 3. Callers — blast radius

### Legacy funcs (`config.DataDir/JournalDir/TasksDir/NotesDir/DatabasePath/LogDir`)

Production (non-test) call sites:

| File | Lines | Func called | Subsystem |
|---|---|---|---|
| `internal/cli/root.go` | 151 | `DatabasePath` | `initServiceE()` — runs before **every** command not annotated `requiresDB=false` (see below) |
| `internal/cli/migrate.go` | 37, 90 | `DatabasePath` | v0→v0.5 ID-format migration tool (`rk migrate run/status`) — **not** the T9 vault migration, despite the name |
| `internal/cli/migrate.go` | 101 | `TasksDir` | same |
| `internal/cli/migrate.go` | 106 | `LogDir` | same |
| `internal/cli/notes.go` | 105, 297, 578 | `NotesDir` | v0 notes commands |
| `internal/journal/task_service.go` | 68, 79, 156, 555 | `TasksDir` | v0 task service (DB + per-task `.md` files) |
| `internal/migrate/backup.go` | 14, 161 | `DataDir` | backup/restore for the v0→v0.5 migration tool |
| `internal/migrate/log_files.go` | 59, 139 | `LogDir` | same migration tool |
| `internal/migrate/migrate.go` | 48, 53 | `TasksDir`, `LogDir` | same migration tool |
| `internal/storage/filesystem.go` | 73, 83 | `JournalDir` | v0 journal file storage |
| `internal/storage/filesystem.go` | 147 | `DataDir` | same |
| `internal/sync/watcher.go` | 48, 129 | `JournalDir` | v0 fsnotify watcher on the journal dir |

Test call sites (legacy, will need updating if defaults move, mostly already
hermetic via `RECKON_DATA_DIR`): `internal/cli/note_show_test.go`,
`internal/cli/notes_integration_test.go`, `internal/cli/notes_test.go`,
`internal/journal/schedule_test.go`, `tests/integration_test.go`, plus several
more under `internal/journal/*_test.go` that set `RECKON_DATA_DIR` directly
(62 occurrences repo-wide).

**Root cause of wide blast radius**: `internal/cli/root.go`'s
`PersistentPreRunE` (83–105) calls `initServiceE()` (148–175, uses
`config.DatabasePath()` at 151) for **every** command unless the command or an
ancestor is annotated `Annotations: map[string]string{"requiresDB": "false"}`.
Today only `indexCmd` (`internal/cli/index.go:21`), `queryCmd`
(`internal/cli/query.go:48`), and the v1 stub commands `addCmd`/`todoCmd`
(`internal/cli/stubs.go:44,55`) carry that annotation. Every other
already-shipped command — `rk log`, `rk note`, `rk notes`, `rk today`, `rk
week`, `rk rebuild`, `rk review`, `rk schedule`, `rk task`, `rk win`, `rk
checklist`, `rk tui`, `rk migrate` — defaults to `requiresDB=true` and
therefore triggers the legacy `DataDir()` mkdir side effect on invocation
today, independent of this ticket.

### New `Config`/`Load`/`NewConfig`/`LoadWithOverrides`

Production call sites are narrow — only two:

| File | Line | Call |
|---|---|---|
| `internal/cli/index.go` | 28 | `config.LoadWithOverrides(vaultFlag, "")` |
| `internal/cli/query.go` | 107 | `config.LoadWithOverrides(vaultFlag, "")` |

Both are `requiresDB=false` commands (`index.go:21`, `query.go:48`) — they
skip `initServiceE()` entirely and never touch the legacy path. `--vault` is a
persistent flag (`internal/cli/root.go:114`, currently documented as
`"Override vault directory (default: $RECKON_VAULT or ~/.reckon)"` — this help
text states the current colliding default and will go stale if it's moved).

Test call sites: `internal/cli/query_extra_test.go`, `internal/cli/query_test.go`,
`internal/index/index_test.go`.

**Summary**: the legacy DB-primary model is still what every shipped,
user-facing command exercises today (10+ commands). The new vault model is
exercised only by `rk index` and `rk query`, both new/experimental. `addCmd`
and `todoCmd` are stubs that return `errNotImplemented`
(`internal/cli/stubs.go:38,55,60`) and don't touch either path yet. **T4 (`rk
log` v1 capture, reckon-uv09) will be the first tool that actually writes
vault content**, which is exactly why this ticket is a hard blocker for it.

## 4. Env var / XDG conventions already established (new `Config`)

From `LoadWithOverrides` (`internal/config/config.go:121–163`), read directly:

- `RECKON_VAULT` — explicit vault dir override, highest precedence after an
  explicit function argument.
- `RECKON_CACHE` — explicit cache dir override, same precedence tier.
- `XDG_CACHE_HOME` — standard XDG cache root; if set and `RECKON_CACHE` isn't,
  cache defaults to `$XDG_CACHE_HOME/reckon`.
- Fallback cache: `$HOME/.cache/reckon` (XDG's conventional default location
  even when `XDG_CACHE_HOME` is unset).
- **No XDG data-dir convention is used for VaultDir** — it falls straight to
  `$HOME/.reckon` with no `XDG_DATA_HOME` consideration. (Note: `VaultDir` is
  explicitly a *git-synced content root*, i.e. a user-visible document
  directory analogous to an Obsidian vault, not app-private state — so
  `XDG_DATA_HOME` semantics may or may not even be the right frame for it;
  see options below.)
- EC-7 guard: cache dir must not equal or nest inside vault dir, else
  `LoadWithOverrides` returns an error (`config.go:154-156`, referenced as
  `EC-7` in tests). This guard runs against whatever `vaultDir` and `cacheDir`
  end up being, so it will automatically re-validate against any new default
  chosen — no change needed there.

The legacy side has its own, unrelated, single env var: `RECKON_DATA_DIR`
(`config.go:20`), primarily a test-hermeticity override, not part of any XDG
convention. Legacy and new env vars are already fully independent — changing
one has no effect on the other's resolution today.

## 5. Design-doc history — which model is "current"

Two generations of design docs exist and describe **contradictory** truth
models; only the second is current:

- `docs/reckon-redesign_2026-06-15.md` / `docs/reckon-spec_2026-06-15.md`
  (2026-06-15, status "PROPOSED"): describes the **DB-primary** model —
  `~/.reckon/reckon.db` = "operational truth", `~/.reckon/archive/journal/*.md`
  = regenerated projection, `~/.reckon/pages/` = files-as-truth input. This is
  the model the legacy config functions implement.
- `docs/design/composable-redesign.md` (started 2026-06-19, "architecture
  complete") plus its `composable-redesign-assessment.md` and
  `-rebuttal.md` companions: supersedes the above. **Files-as-truth + a
  disposable per-device SQLite index, never synced, never inside the vault**
  is the locked model (`composable-redesign-assessment.md`: *"makes the
  earlier global DB-first answer unnecessary… should stop being
  re-litigated"*). This is the model `Config{VaultDir, CacheDir}` implements.
  Line 928 of `composable-redesign.md`: *"Index location — a local cache dir
  (`~/.cache/reckon/<vault-id>/index.db`)"* — consistent with the `CacheDir`
  default already in code.
- `docs/design/foundation-review-2026-06-24.md` is the review that raised
  finding **M5** verbatim (line 33) and recorded the disposition assigning it
  to this ticket (lines 68–69): *"resolve `~/.reckon` legacy/vault overlap;
  keep resolution pure"*. It does **not** prescribe what the new default
  should be — only that it must differ and stay pure.
- `docs/design/code-walkthrough-foundation.md` §4.1 (lines 630–650) restates
  the same finding, again without prescribing a new default.

**No design doc anywhere decides the concrete v1 vault default path.** I
grepped `composable-redesign.md` and all other docs for `XDG_DATA_HOME`,
`~/reckon`, `~/Documents`, `/vault` — no hits describing an intended new
default. This is a genuinely open decision for the planner, not something
already settled elsewhere.

### `.gitattributes` reveals the intended vault subdirectory layout

`/.gitattributes` (repo root) is already scoped for the new vault:
```
log/*.md merge=union
```
with a comment: *"Scoped to `log/*.md` ONLY — other markdown (`todo/`,
`notes/`, README, etc.)"* — confirming the **planned** v1 vault subdirectory
names: `log/`, `todo/`, `notes/`.

**This matters directly for collision risk**: the legacy `DataDir()` layout
uses `journal/`, `tasks/`, `notes/`, `logs/` (plural, via `JournalDir`,
`TasksDir`, `NotesDir`, `LogDir`). Compare:

| Legacy subdir (under `DataDir()`) | Planned new vault subdir |
|---|---|
| `journal/` | `log/` (different name) |
| `tasks/` | `todo/` (different name) |
| `notes/` | `notes/` — **identical name** |
| `logs/` | (no vault equivalent planned; logs are app-operational, not vault content) |

Even if the *root* default is separated, `notes/` is the one subdirectory name
both models already agree on — worth flagging explicitly since it's the
highest-residual collision risk within any shared-root scenario, and would
still be worth knowing about even after the root paths diverge, in case a
future decision ever nested one inside the other.

### `.gitignore` note

Repo-root `/.gitignore` has a `.reckon/` entry (with an explanatory absence of
comment) — this is scoped to the reckon *project repo's own working tree*, not
a statement about the user's home-directory vault; likely present because dev
tooling or a relative-path test run can incidentally create a `.reckon/` dir
inside the project checkout. Not directly load-bearing for this ticket, noted
for completeness only.

## 6. Existing "pure path resolution" pattern already in the new code

`internal/index/index.go` already demonstrates the exact separation this
ticket wants applied to config:

- `DBPath(cfg *config.Config) (string, error)` (lines 52–58) is **pure** — it
  derives a per-vault ID (`vaultID`, sha256 of the absolute vault path,
  40–47) and joins it with `cfg.CacheDir`, doing nothing but string/path math
  and one `filepath.Abs` call. No `os.MkdirAll`, no file I/O. Doc comment:
  *"returns the on-disk path of the index database for cfg, **without opening
  it**"* (51). `rk query` uses exactly this pure path (via
  `index.DBPath(cfg)` at `internal/cli/query.go:121`) to open a read-only
  connection without ever creating anything.
- `Open`/`OpenWithParser` (60–95) is the **impure** counterpart — it computes
  the same directory, then explicitly `os.MkdirAll`s it (76–78) as part of
  *opening* a real resource (a SQLite handle), not as part of *resolving* a
  path. The mkdir is co-located with the operation that actually needs the
  directory to exist (opening a DB file), not with path computation.

This "resolve is pure, a separate open/write call creates directories where
needed" split is precisely the shape already established for the index cache,
and gives a concrete precedent to mirror for `Config`/vault resolution:
compute-only functions return a path; whichever code path is about to actually
write into the vault (T4's log-append, `rk index`'s rebuild, etc.) is
responsible for creating the directory at the point of use, not resolution
time.

## 7. `docs/REVIEW_PATTERNS.md` — relevant pitfalls

Searched for `config`, `path handling`, `pure`, `mkdir`, `side effect` — no
direct hits; `REVIEW_PATTERNS.md` (1025 lines) has no existing pattern entry
specific to config/path-resolution purity or app-data-dir collisions. The
closest generically-relevant sections are:
- "Anti-Pattern: os.Exit() in Library Code" (line 175) — not directly
  applicable here (config funcs already return errors, not os.Exit).
- "Anti-Pattern: Ignored Errors" (line 50) / "Unwrapped Errors" (line 21) —
  worth keeping in mind if new default-resolution code is added, but nothing
  in the current `config.go` violates these today (`LoadWithOverrides`
  wraps every `os.UserHomeDir()` error with `%w` and a `config:` prefix,
  lines 129, 144).

No pre-existing pattern entry needs updating as a prerequisite; this ticket's
own resolution may be a good candidate to add a new pattern entry afterward
(e.g. "resolution functions must not mkdir"), but that's a follow-up
consideration, not a blocking finding.

## 8. Options for separating the legacy default from the new vault default

Presented as facts/tradeoffs only — no recommendation, per the task
instructions; this is the planner's call.

### Option A — Relocate the new `Config.VaultDir` default to a different path, leave legacy `DataDir()` untouched

E.g. default `VaultDir` to something like `$HOME/reckon` (no dot), `$HOME/vault`,
`$HOME/Documents/reckon`, `$XDG_DATA_HOME/reckon` (or `$HOME/.local/share/reckon`
per XDG convention for user-created data), or a name embedding "vault"
explicitly (e.g. `$HOME/.reckon-vault`).

- Pros: smallest change — only `LoadWithOverrides`'s VaultDir branch
  (`config.go:123-133`) and its default-path test (`config_test.go`'s
  `TestLoad_Defaults`) change. Legacy funcs, all ~15 legacy call sites, and all
  legacy tests are completely untouched — zero regression risk to shipped v0
  commands. `--vault` help text and doc comments need a one-line update.
- Cons: requires picking a *new* concrete path that has never been used or
  referenced anywhere in code/docs before (open decision, no existing
  precedent to anchor to — see §5). If the eventual choice puts VaultDir
  somewhere unconventional (not XDG-data, not a dotfile), it may need its own
  justification/documentation for future users. Also: existing v1 users who've
  already been experimentally running `rk index`/`rk query` against
  `~/.reckon` (if any) would see their vault "move" out from under them unless
  they set `RECKON_VAULT` explicitly — though per the ticket and AC doc,
  nothing has shipped writing real vault data yet, so this risk is currently
  theoretical/pre-launch.

### Option B — Relocate the legacy `DataDir()` default, leave the new `Config.VaultDir` default (`~/.reckon`) untouched

E.g. move legacy default to `$HOME/.reckon-legacy`, `$HOME/.reckon-v0`, or similar.

- Pros: the new vault model keeps the "natural"/already-documented `~/.reckon`
  default (matches the `--vault` flag help text and multiple doc references
  as-is); zero doc/UX changes needed for the v1 surface once T4 lands.
- Cons: touches the *shipped, in-active-use* legacy surface — `DataDir()` is
  called by 10+ live commands and its default has presumably been in
  production use (real user data may already exist at `~/.reckon/reckon.db` +
  `~/.reckon/journal/*.md` etc., per the AC doc's "existing user with real
  legacy data" edge case). Moving it would either (a) orphan existing
  on-disk data at the old path unless a migration/rename step runs, or (b)
  require every legacy caller's tests/fixtures that assume `~/.reckon`
  (`RECKON_DATA_DIR`-driven tests are override-based so mostly insulated, but
  any test asserting the literal default path would need updating — same
  category of risk as Option A's `TestLoad_Defaults`, just on the legacy
  side instead). Higher blast radius given `DataDir()` is exercised by every
  shipped v0 command today (§3).

### Option C — Both defaults move to new, disjoint names (retire the shared `~/.reckon` name entirely)

- Pros: cleanest long-term signal — no path in the codebase is "the old
  ambiguous default" anymore; both surfaces get names that describe what they
  actually are (e.g. legacy → `~/.reckon-data` or similar, new vault →
  `~/reckon` or `~/.local/share/reckon`).
  Symmetrical, avoids anchoring either surface to a name the other might
  someday want back.
- Cons: strictly larger diff than A or B (touches both surfaces' tests and
  docs); real user data at the current `~/.reckon` legacy location still needs
  the same handling as Option B's con (existing on-disk data doesn't
  auto-follow a code default change — nothing physically moves files, so
  users become unable to find their existing legacy data at the
  known/expected location unless `RECKON_DATA_DIR` is set, or a migration
  path is documented).

### Option D — Keep both defaults at `~/.reckon`, but make the new `Config` detect/guard against the collision explicitly rather than relocate

E.g. `LoadWithOverrides` checks (only in the default-resolution branch, not
when `RECKON_VAULT` is explicitly set — per the AC doc's edge case, explicit
user overrides pointing both at the same place should remain *possible*) for
telltale legacy artifacts (`reckon.db`, `journal/`, `tasks/` as siblings under
the resolved default) and either errors out or warns.

- Pros: no path actually moves; existing legacy data is untouched and
  automatically "found" if a later T9 migration wants to read it from the
  same place it always lived.
  Requires no coordination with the eventual T9 migration's own path
  assumptions.
- Cons: does not satisfy the ticket's literal "Done when" wording ("the new
  vault default is separated from the legacy app-data dir... legacy and new
  config surfaces no longer collide") — a same-path-plus-guard is arguably
  still "colliding by default," just with an error/warning bolted on instead
  of removed. Also directly contradicts the "resolution is pure" requirement
  if the guard does any filesystem probing beyond a cheap `os.Stat` (a `Stat`
  is arguably still "pure" in the sense of not *writing*, but it's a
  filesystem read baked into what's meant to be pure string/path computation
  — a design judgment call). This option effectively doesn't answer "decide
  the v1 vault default" as instructed by the ticket, it just defers the
  question — likely why the ticket phrasing ("the new vault default is
  separated from the legacy app-data dir") points more toward A/B/C.

### Cross-cutting considerations for whichever option is chosen

- **Env var independence must be preserved.** `RECKON_DATA_DIR` (legacy) and
  `RECKON_VAULT`/`RECKON_CACHE` (new) are already fully independent env vars
  (§4) — no option here requires touching that; whichever default changes,
  its override env var keeps working exactly as today.
  `RECKON_VAULT` pointed explicitly at whatever the legacy default is (or was)
  must remain possible — it's the documented escape hatch, not something to
  block (per the existing AC doc's "escape hatch" edge case).
- **`--vault` flag help text** (`internal/cli/root.go:114`) currently states
  the default is `~/.reckon` — must be updated to match whichever new default
  is chosen (Option A/C) or left as-is (Option B, since the new surface's
  literal default value doesn't change).
- **`TestLoad_Defaults`** (`config_test.go:13-38`) hard-codes the expected
  default and needs updating under A/C; under B it's unaffected (new default
  unchanged) but a parallel legacy default-path test, if one exists/is added,
  would need updating instead.
- **Purity requirement is scoped to the *new* Config surface only**, per the
  ticket's literal "Done when" wording and confirmed by the AC doc's "Out of
  Scope" section — none of A/B/C/D require adding or removing `mkdir` calls on
  the *legacy* side; only the *new* default's resolution logic must stay
  `MkdirAll`-free, which is already true of `LoadWithOverrides` today and
  simply needs to remain true after whatever default-path literal changes.
- **This is a default-collision fix, not a data migration** (AC doc, "Not a
  data migration" / "Out of Scope" sections) — none of the options above
  should move, copy, or delete any existing on-disk file as part of this
  ticket; T9 (`reckon-s6oh`) owns actually migrating legacy data into vault
  format. Whichever option is chosen, existing legacy data simply becomes
  "not automatically visible to new vault-based defaults" going forward,
  which is the intended effect, not a bug to work around here.
