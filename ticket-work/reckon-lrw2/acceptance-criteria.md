# Acceptance Criteria: Review M5 — resolve legacy `~/.reckon` config overlap before tools write the vault (reckon-lrw2)

Source: `docs/design/foundation-review-2026-06-24.md` finding **M5**, disposition
entry ("M5 — ticket `reckon-lrw2` … before T4"), and `code-walkthrough-foundation.md`
§4.1 review note. Blocks `reckon-uv09` (v1-T4: `rk log`). Ties to `reckon-s6oh`
(v1-T9: DB-primary → text-truth migration).

## Current-state facts (grounded in code, `internal/config/config.go`)

- Legacy funcs `DataDir`/`JournalDir`/`TasksDir`/`NotesDir`/`DatabasePath`/`LogDir`
  all bottom out in `DataDir()`, which defaults to `filepath.Join(home, ".reckon")`
  and calls `os.MkdirAll` as a side effect (also honors a `RECKON_DATA_DIR` env
  override, primarily for tests).
- The new `Config{VaultDir, CacheDir}`, resolved via `Load`/`NewConfig`/
  `LoadWithOverrides`, defaults `VaultDir` to `filepath.Join(home, ".reckon")` too
  (override: `RECKON_VAULT`) — **the same path** as the legacy `DataDir()` default.
  `LoadWithOverrides` is already documented/tested as pure (no `MkdirAll` anywhere
  in that path) — see `TestNewConfig_TempVaultHermetic` in `config_test.go`.
- Legacy funcs are **not dead code** — they're wired into shipped v0 commands:
  `internal/journal/task_service.go`, `internal/storage/filesystem.go`,
  `internal/cli/migrate.go`, `internal/cli/notes.go`, `internal/cli/root.go`
  (`initServiceE`, DB init for every DB-backed command), `internal/sync/watcher.go`,
  and the whole `internal/migrate/` package (the existing v0→v0.5 SQLite migration
  tool, distinct from the future T9 vault migration).
- `--vault` flag / `RECKON_VAULT` / `RECKON_CACHE` are already wired for the new v1
  commands (`rk query`, `rk index` call `config.LoadWithOverrides(vaultFlag, "")`).

## 1. Explicit Acceptance Criteria

Derived directly from the ticket's "Done when" clause (three sub-criteria):

1. **(a) A v1 vault default is chosen that is distinct from the legacy app-data
   default.** `Config.VaultDir`'s zero-override default must no longer resolve to
   `$HOME/.reckon` (or to whatever path the legacy `DataDir()` resolves to). A
   concrete new default path is decided and documented (e.g. in `LoadWithOverrides`'s
   doc comment, `--vault` flag help text, and any user-facing docs) as part of this
   ticket — it is not left as a TODO.
2. **(b) Legacy and new config surfaces no longer collide by default.** With no
   env-var overrides set, `DataDir()` (and therefore `JournalDir`/`TasksDir`/
   `NotesDir`/`DatabasePath`/`LogDir`) and `Config.VaultDir` (via `Load`/`NewConfig`/
   `LoadWithOverrides` with no explicit vault arg) must resolve to two different,
   non-overlapping directories (neither equal, nor one nested inside the other) for
   a given `$HOME`.
3. **(c) The new path resolution stays pure.** `Load`/`NewConfig`/`LoadWithOverrides`
   (and any new default-resolution code added to satisfy (a)/(b)) must not create
   directories, files, or otherwise touch the filesystem as a side effect of
   resolving a path. This must hold both before and after the default changes in
   (a)/(b) — i.e. the fix must not introduce a new `mkdir`-on-resolve regression
   while relocating the default.

## 2. Implicit Requirements

- **No breakage of shipped legacy callers.** `DataDir`/`JournalDir`/`TasksDir`/
  `NotesDir`/`DatabasePath`/`LogDir` are actively used by `internal/journal`,
  `internal/storage`, `internal/sync`, `internal/migrate`, and multiple `internal/cli`
  commands (`notes.go`, `migrate.go`, `root.go`'s DB init). This ticket must not
  change their signatures, their `mkdir`-on-call behavior, or their `RECKON_DATA_DIR`
  override semantics in a way that breaks any of those callers or the tests that
  exercise them (`internal/journal/*_test.go`, `internal/cli/notes*_test.go`,
  `tests/integration_test.go`). The legacy functions may keep their existing
  contract (including `mkdir`) — the ticket's "stays pure" requirement is scoped to
  the *new* `Config` path, per the "Done when" wording ("the new path stays pure"),
  not a retroactive purity requirement on the legacy funcs themselves.
- **Not a data migration.** Existing users with real data at `~/.reckon`
  (`reckon.db`, `journal/*.md`, `tasks/`, `notes/`) must not have that data moved,
  deleted, or silently reinterpreted as vault content by this ticket. Changing
  `Config.VaultDir`'s default away from `~/.reckon` means a fresh `rk index`/`rk
  query`/future `rk log` on an existing installation now points at an *empty* new
  default location rather than colliding with legacy data — that's the intended
  fix, not a migration. Actual migration of legacy data into the vault format is
  `reckon-s6oh` (T9)'s job, out of scope here (see §5).
- **Must land before `reckon-uv09` (T4, `rk log`).** This is a hard blocking
  dependency in beads (`reckon-lrw2` blocks `reckon-uv09`), not advisory — T4 is the
  first tool that writes into the vault, so the collision must be closed before T4
  work starts, not just before T4 ships.
- **Consistency of documentation/help text.** Any place that states or implies the
  vault default is `~/.reckon` must be updated to match the new default: the
  `--vault` flag help string in `internal/cli/root.go`
  (`"Override vault directory (default: $RECKON_VAULT or ~/.reckon)"`), the doc
  comment on `LoadWithOverrides`, and any design docs that assert the same default
  (`code-walkthrough-foundation.md`, `foundation-review-2026-06-24.md` are
  historical/append-only records and don't need edits, but any *current* spec doc
  stating the default should be reconciled).
- **`RECKON_DATA_DIR` (legacy) and `RECKON_VAULT`/`RECKON_CACHE` (new) remain
  distinct env vars.** They already are — this ticket should not conflate or merge
  them; an explicit override of one must not implicitly affect the other's default
  resolution.
- **Existing tests asserting the old default must be updated, not just left
  failing.** `internal/config/config_test.go`'s `TestLoad_Defaults` currently
  hard-codes `wantVault := filepath.Join(tmp, ".reckon")`. Changing the default
  requires updating this assertion (and any other test/fixture that assumes
  `VaultDir == $HOME/.reckon`) as part of this ticket, not as follow-up debt.

## 3. Edge Cases

- **Existing user with real legacy data at `~/.reckon`** (`reckon.db` +
  `journal/*.md` + `tasks/` + `notes/`) runs a new vault-based command (e.g. future
  `rk log`, or today's `rk index`/`rk query`) for the first time after this fix
  ships. Expected: `Config.VaultDir` now points at the new, different default path
  (empty/nonexistent), *not* at `~/.reckon` — so the new tooling does not read,
  index, or write over the legacy DB/journal files. It also must not error out just
  because the legacy directory exists; the two are simply independent.
- **User already has files at the *new* default path for unrelated reasons** (e.g.
  they happened to create `~/some-new-default-dir` themselves) — not this ticket's
  concern to detect/handle; standard "vault dir must exist or be creatable by the
  tool that writes to it" behavior applies, same as any other user-supplied path.
- **`RECKON_VAULT` explicitly set to the legacy `~/.reckon` path.** A user (or a
  script, or an old habit) could still explicitly override `RECKON_VAULT=~/.reckon`,
  making old and new coincide again. This must remain *possible* (env overrides are
  the escape hatch and the design explicitly supports `--vault`/`RECKON_VAULT`
  pointing anywhere) — the ticket only needs to prevent the *default* from
  colliding, not categorically forbid the user from pointing both at the same place
  if they choose to. (Whether such an explicit choice should also be actively
  guarded against, e.g. a warning if `RECKON_VAULT` resolves to the legacy
  `DataDir()`, is a judgment call for the implementer — not mandated by the "Done
  when" text, which only speaks to the *default*.)
- **`RECKON_CACHE`/`XDG_CACHE_HOME` interaction with the relocated vault default.**
  The existing "cache must not be inside vault" guard (`LoadWithOverrides`, EC-7 in
  prior config tests) must continue to hold against whatever the *new* vault default
  is — i.e. the new default vault path and the default cache path
  (`$XDG_CACHE_HOME/reckon` or `$HOME/.cache/reckon`) must not nest either.
- **Legacy `DataDir()` and new default cache dir.** Not addressed by the "Done
  when" text and not required: the legacy app-data dir and the new *cache* dir
  don't need to be kept apart by this ticket, only legacy-vs-new-**vault**.
  (Worth noting, not necessarily acting on, since `LogDir()` legacy default
  `~/.reckon/logs` and any future cache layout are unrelated truth models.)
- **Both `RECKON_DATA_DIR` and `RECKON_VAULT` set simultaneously to different
  paths.** Each override is independent and already resolved independently today
  (`DataDir()` reads `RECKON_DATA_DIR`; `LoadWithOverrides` reads `RECKON_VAULT`) —
  this ticket doesn't need to add cross-validation between them, only ensure the
  *unset* (default) case no longer collides.
- **Home directory resolution failure (`os.UserHomeDir()` errors)** — both legacy
  and new paths already propagate this as an error today; the fix must not change
  that (no new panics or silent fallbacks introduced while relocating the default).

## 4. Test Scenarios (Given/When/Then)

### Legacy function defaults (regression — must still work)

**Scenario: `DataDir()` default with no env override**
Given `RECKON_DATA_DIR` is unset and `$HOME` is a known test directory
When `config.DataDir()` is called
Then it returns `$HOME/.reckon`
And the directory is created on disk (legacy `mkdir`-as-side-effect behavior is
unchanged)

**Scenario: `JournalDir`/`TasksDir`/`NotesDir`/`DatabasePath`/`LogDir` still derive
from `DataDir()`**
Given `RECKON_DATA_DIR` is unset and `$HOME` is a known test directory
When each of `config.JournalDir()`, `config.TasksDir()`, `config.NotesDir()`,
`config.DatabasePath()`, `config.LogDir()` is called
Then each returns a path nested under `$HOME/.reckon` (`journal/`, `tasks/`,
`notes/`, `reckon.db`, `logs/` respectively), matching current behavior

### New `Config.VaultDir` default (changed behavior)

**Scenario: `Load()` default vault dir no longer equals the legacy dir**
Given `RECKON_VAULT`, `RECKON_CACHE`, and `RECKON_DATA_DIR` are all unset and `$HOME`
is a known test directory
When `config.Load()` is called
Then `cfg.VaultDir` does NOT equal `$HOME/.reckon`
And `cfg.VaultDir` equals the newly decided v1 default path

**Scenario: `RECKON_VAULT` override still works**
Given `RECKON_VAULT=/tmp/myvault` is set
When `config.Load()` is called
Then `cfg.VaultDir == "/tmp/myvault"` (unchanged from current behavior)

### Non-collision between legacy and new defaults

**Scenario: legacy `DataDir()` and new `Config.VaultDir` resolve to different paths
by default**
Given no relevant env overrides (`RECKON_DATA_DIR`, `RECKON_VAULT`, `RECKON_CACHE`
all unset) and a known test `$HOME`
When `config.DataDir()` and `config.Load()` (`.VaultDir`) are both called
Then the two resulting paths are different
And neither path is a parent/ancestor directory of the other (checked via
`filepath.Rel` not starting with `..`, mirroring the existing vault/cache
containment check)

**Scenario: legacy dir and new vault dir do not collide across repeated resolution**
Given the same test `$HOME` as above
When `config.DataDir()` is called, then `config.Load()` is called
Then calling `config.DataDir()` again still returns the original legacy path
(order of calls doesn't cause one default to leak into the other — no shared
mutable state between legacy and new resolution)

### Purity of the new path (no mkdir on resolve)

**Scenario: `Load()`/`NewConfig()`/`LoadWithOverrides()` create no directories**
Given a fresh test `$HOME` with nothing on disk under it, and no env overrides
When `config.Load()` is called
Then `cfg.VaultDir` (the new default) does not exist on disk afterward
And no new directories were created anywhere under `$HOME` as a result of the call
(regression coverage of the existing `TestNewConfig_TempVaultHermetic`-style check,
re-verified against the relocated default)

**Scenario: purity holds even when the new default's parent doesn't exist**
Given `$HOME` is a test directory with no pre-existing subdirectories
When `config.Load()` is called with no overrides
Then it returns successfully (no error) with a non-empty `VaultDir`
And nothing was written to disk — resolution succeeds without the path existing

### Env var override interaction

**Scenario: `RECKON_VAULT` pointed at the legacy dir is still honored (escape hatch,
not silently blocked)**
Given `RECKON_VAULT` is explicitly set to `$HOME/.reckon` (the legacy path)
When `config.Load()` is called
Then `cfg.VaultDir == $HOME/.reckon` (explicit override is respected, not overridden
by anti-collision logic) — the ticket prevents default collision, not user-directed
collision

**Scenario: `RECKON_DATA_DIR` override does not affect `Config.VaultDir`, and vice
versa**
Given `RECKON_DATA_DIR=/tmp/legacy-override` is set and `RECKON_VAULT` is unset
When `config.DataDir()` and `config.Load()` are both called
Then `config.DataDir()` returns `/tmp/legacy-override`
And `cfg.VaultDir` still resolves to the new default (unaffected by
`RECKON_DATA_DIR`)

**Scenario: cache-inside-vault guard still enforced against the relocated default**
Given no overrides, so `VaultDir` resolves to the new default and `CacheDir`
resolves to `$HOME/.cache/reckon` (or `$XDG_CACHE_HOME/reckon`)
When `config.Load()` is called
Then it succeeds (no EC-7 error) because the new default vault and default cache
paths still don't nest, same guarantee as before the default moved

## 5. Out of Scope

- **Implementing T9 (`reckon-s6oh`)** — the actual data migration tool that moves
  existing `reckon.db` tasks/notes/checklists and `journal/*.md` entries into
  text-truth vault nodes (xid→ULID aliasing, dry-run/verify mode, etc.). This
  ticket only needs to stop the *default-path collision* and keep resolution pure;
  it does not move, convert, or read any existing user data.
- **Deprecating or removing the legacy `DataDir`/`JournalDir`/`TasksDir`/
  `NotesDir`/`DatabasePath`/`LogDir` functions**, or the v0 commands/DB-primary
  storage model that depend on them. They keep working exactly as before (including
  their `mkdir` side effect) — only their *default no longer coincides* with the
  new vault default.
- **Prompting or warning users at runtime** if `RECKON_VAULT` is explicitly set to
  the legacy path, or building any automatic detection of "you have old data
  sitting where the vault used to default to." That kind of user-facing guidance,
  if wanted, belongs with T9 migration tooling, not this collision fix.
- **Changing the `RECKON_DATA_DIR`, `RECKON_VAULT`, or `RECKON_CACHE` env var names
  or precedence rules** beyond what's needed to relocate the default — no new env
  vars are being introduced by this ticket.
- **M2 (`reckon-9bfx`, ULID mint policy / `rk adopt`)** and **M-parser-scope**
  (`reckon-vj55`) — separate foundation-review findings with their own tickets, not
  part of this one even though they're adjacent in the same review doc.
- **Retrofitting purity (no-mkdir) onto the legacy functions.** The "Done when"
  text scopes the purity requirement to "the new path" only; legacy `DataDir()` and
  friends may continue to `MkdirAll` as they do today.
