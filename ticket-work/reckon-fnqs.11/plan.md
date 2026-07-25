# Implementation Plan: `rk checklist` CLI verb surface (reckon-fnqs.11)

## Summary

Add a flat, non-interactive `rk checklist` command family (create/list/start/check/status/reset/abandon) as a thin CLI layer over the already-built, already-tested `internal/checklist` Service/Repository. No Service/Repository/model changes. All output flows through `internal/output` (Pretty/`--json`/`--ndjson`), matching `todo`/`index`. The DB is opened per-invocation via `config.DatabasePath()` (`~/.reckon/reckon.db`), mirroring `rk index`'s open-in-RunE/`defer Close()` shape. All ten pinned decisions from the task brief are treated as constraints. Rebuild-survival is handled as documentation only (parent `Long` help text), never destructive code.

## Files to modify

| File | Action | Reason |
|---|---|---|
| `internal/cli/checklist.go` | **create** | `checklistCmd` parent + 7 subcommands, result types, `openChecklistService()` + `closeChecklistService` helpers, `resetChecklistFlags`, `resolveStartOrResume`. Per codebase-analysis.md §8. |
| `internal/cli/checklist_test.go` | **create** | Table/scenario tests driving `RootCmd.Execute()`; `t.Setenv("RECKON_DATA_DIR", t.TempDir())` isolation (codebase-analysis.md §3, acceptance-criteria.md line 46). |
| `internal/cli/root.go` | **modify** | Add `checklistCmd` to the `RootCmd.AddCommand(...)` block (root.go:101-109). Direct-var pattern like `todoCmd`/`indexCmd`. |
| `internal/checklist/*`, `go.mod` | **none** | Domain layer complete and tested; deps (cobra, xid, sqlite, testify) already present. codebase-analysis.md §8. |

## Design decisions (only those NOT pinned by the brief)

### DB/service wiring helper
Unexported helper opened at the top of each `RunE`, returning both the service and the `*storage.Database` so the caller can `defer db.Close()`:
```
func openChecklistService() (*checklist.Service, *storage.Database, error)
```
Body: `config.DatabasePath()` → `storage.NewDatabase(path)` (database.go:188; creates schema + tables idempotently) → `checklist.NewRepository(db)` → `checklist.NewService(repo)`. No package-level global, no `initServiceE()` (decision #8; codebase-analysis.md §3). Wrap errors `fmt.Errorf("checklist <verb>: ...: %w", err)`.

### Result types — embed domain type / named slices (satisfies T-AC8a: "JSON matching model.go json tags")
Single-object verbs embed the domain pointer so `encoding/json` promotes its fields to top level (flat, model-tagged JSON) while a value-receiver `Pretty()` supplies human text:
```
type checklistTemplateResult struct{ *checklist.Template }   // create
type checklistRunResult struct {                              // start / check / status / reset / abandon
    *checklist.Run
    resumed bool `json:"-"`   // start-only discriminator; json:"-" keeps JSON pure-model
}
```
List verbs use named slice types (also model-tagged, and a named slice may carry a `Pretty()` method):
```
type checklistTemplateList []*checklist.Template   // list (bare)
type checklistRunList       []*checklist.Run       // list <template>
```
**Correctness constraint (AC-2):** `ListTemplates` returns a nil slice on empty (repository.go), and `json.Marshal(nil-slice)` → `null`, not `[]`. Every list RunE MUST force the slice non-nil (`if items == nil { items = []*checklist.X{} }`) before output so empty JSON is `[]`.

### Pretty() rendering (0-based positions per decision #2; adapted from v0 `printRunStatus`, which used 1-based — diverge to 0-based)
| Result | Pretty() |
|---|---|
| `checklistTemplateResult` | `checklist: created template "NAME" (N items)` |
| `checklistRunResult` | Header `NAME  [c/total]`; then one line per item `  [ ] P. TEXT` / `  [x] P. TEXT` where `P = item.Position` (0-based); trailer by status: active → `status: active`, completed → `  ✓ Complete!`, abandoned → `checklist: abandoned "NAME"`. |
| `checklistTemplateList` | empty → `checklist: no templates`; else `checklist: N template(s)` + one line per template `  NAME  (M items)`. |
| `checklistRunList` | empty → `checklist: no runs for "NAME"`; else one line per run `  STATUS  [c/total]  started TIME`. |

### Output-mode branching
- **Six single-object verbs:** `output.New(cmd.OutOrStdout(), mode).Print(res)` — correct in Pretty/JSON/NDJSON.
- **Two list forms:** branch on mode so NDJSON emits one object per line and Pretty still prints the empty-state message (a named slice through `Print` would put the whole array on one NDJSON line):
  - `Pretty` → `Print(namedSlice)` (its `Pretty()` handles the empty message);
  - `JSON`/`NDJSON` → `PrintAll(toAny(items))` (JSON → `[]`/array; NDJSON → per-line).

### `--quiet`
- Mutation verbs (create/start/check/reset/abandon): suppress the Pretty confirmation under `--quiet` via `if !(mode == output.Pretty && quietFlag) { ...Print... }` (todo.go:323, index.go:54).
- Query verbs (list/status): data is the requested output — always print (do not suppress under `--quiet`).

### Command / flag surface
| Command | `Use` / Args | Flags |
|---|---|---|
| `checklistCmd` (parent) | `checklist` | — (see §Docs below for `Long`) |
| create | `create <name>`, `ExactArgs(1)` | `--item` (`StringArrayVar`, repeatable, note_v1.go:125 idiom), `--items-file` (`StringVar`) |
| list | `list [template]`, `MaximumNArgs(1)` | `--all` (`BoolVar`) → `includeCompleted`; ignored (no error) when no template arg |
| start | `start <template>`, `ExactArgs(1)` | — |
| check | `check <template> <position>`, `ExactArgs(2)` | — |
| status | `status <template>`, `ExactArgs(1)` | — |
| reset | `reset <template>`, `ExactArgs(1)` | — |
| abandon | `abandon <template>`, `ExactArgs(1)` | — |

All subcommands set `SilenceUsage: true` (todo.go:91) and return wrapped errors (never `os.Exit`; codebase-analysis.md §3). `position` parsed with `strconv.Atoi`; parse failure → `fmt.Errorf("checklist check: position must be an integer, got %q", args[1])`.

### `resetChecklistFlags(cmd *cobra.Command)` (test-correctness, not cosmetic)
Flag vars are package-global and persist across `RootCmd.Execute()` calls in the test binary. Each RunE `defer resetChecklistFlags(cmd)` to zero the values AND clear pflag `Changed` state for `item`/`items-file`/`all`, mirroring `resetTodoFlags` (todo.go:58-75). Without this the suite is flaky (`--all`/`--items-file` leak between tests).

### Per-verb Service orchestration
| Verb | Calls |
|---|---|
| create | validate (below) → `CreateTemplate(name, items)` |
| list (bare) | `ListTemplates()` → force non-nil |
| list `<t>` | `GetTemplate(t)` (validate + get ID) → `ListRuns(all)` → **CLI-side filter** by `run.TemplateID == tpl.ID` (decision #6; `ListRuns` is unscoped across all templates — verified repository.go:268) → force non-nil |
| start | `GetTemplate(t)` → `GetActiveRun(t)` success ⇒ resume (`resumed=true`, exit 0) ; error ⇒ `StartRun(t)` (decision #7). No error-string matching. |
| check | `GetActiveRun(t)` → `CheckItem(run.ID, pos)` → `GetRunStatus(run.ID)` to re-fetch (NOT `GetActiveRun` again — after the final check the run auto-completes and `GetActiveRun` would error, EC-12) → render updated run (decision #9). |
| status | `GetActiveRun(t)` → render |
| reset | `ResetRun(t)` → render new run |
| abandon | `AbandonRun(t)` → render (Status already `abandoned`) |

### `create` validation order (CLI layer, before Service; decisions #3/#5, IR-5/6/7)
1. If `--item` set AND `--items-file` set → error `checklist create: --item and --items-file are mutually exclusive`.
2. If `--items-file` set → `os.ReadFile` (path relative to process cwd), split on `\n`, `TrimSpace` each line, skip blank lines, preserve file order (decision #4; add.go:223 bare-ReadFile idiom).
3. Resulting item slice empty → error `checklist create: at least one item required` (decision #5; a 0-item run can never auto-complete).
4. Else `CreateTemplate(name, items)`.

### Docs — parent `checklistCmd.Long` (decision #1, satisfies AC-10)
State plainly: checklist template/run state lives only in the local operational DB (`~/.reckon/reckon.db`); it is NOT vault-native, NOT git-synced, has no text source of truth, and is never rebuilt/derived by `rk index`; it will not be present on a fresh clone or new machine and does not travel with the vault the way todos/notes do. No destructive code, no `dropDDL` change, no colocation into `index.db` (that would be a forbidden persistence rewrite).

## Test scenarios (from acceptance-criteria.md §4; Go names)

Harness per test: `t.Setenv("RECKON_DATA_DIR", t.TempDir())`; `RootCmd.SetOut/SetErr(&buf)`; `RootCmd.SetArgs([...])`; `t.Cleanup` resets args/out/err and flag vars (index_test.go:27-37). JSON asserts decode `buf` into the model type. Group related whens as subtests.

| Go test func | Source ID(s) | Assertion |
|---|---|---|
| `TestChecklistCreate_Basic` | T-AC1a | `create foo --item a --item b` → template "foo", items at positions 0,1; exit 0 |
| `TestChecklistCreate_DuplicateName` | T-AC1b, EC-13 | error `checklist template "foo" already exists`; non-zero |
| `TestChecklistCreate_EmptyName` | T-AC1c, EC-14 | error `template name cannot be empty`; non-zero |
| `TestChecklistCreate_ItemsFile` | T-AC9a | `--items-file` 3 non-blank lines → 3 items in file order |
| `TestChecklistCreate_ItemsFileAndItemMutuallyExclusive` | IR-5/EC-7 | both flags → mutual-exclusion error |
| `TestChecklistCreate_NoItemsRejected` | T-EC8, EC-8, IR-7 | no `--item`, empty/all-blank `--items-file` → `at least one item required` |
| `TestChecklistList_Empty` | T-AC2a, EC-6 | Pretty `no templates`; `--json` → `[]`; exit 0 |
| `TestChecklistList_Templates` | T-AC2b | two templates listed; JSON array len 2 |
| `TestChecklistList_RunsForTemplate` | AC-11, IR-8, T-EC4 | `list foo --all` shows that template's runs incl. completed; scoped to foo |
| `TestChecklistStart_Fresh` | T-AC3a | new active run, 0 checked, exit 0 |
| `TestChecklistStart_Resume` | T-AC3b, EC-5 | existing active run (item 0 checked) redisplayed, no new run, no error |
| `TestChecklistStart_UnknownTemplate` | T-AC3c, EC-3 | not-found error; no run row created |
| `TestChecklistStart_AfterCompletedRun` | T-EC4, EC-4 | prior completed run does not block a fresh run |
| `TestChecklistCheck_MarksItem` | T-AC4a | position 0 checked; run stays active |
| `TestChecklistCheck_TogglesOff` | T-AC4b, EC-1 | re-check position 0 → unchecked |
| `TestChecklistCheck_OutOfRange` | T-AC4c, EC-2 | `check foo 5` (3 items) → `position 5 out of range (run has 3 items)`; non-zero |
| `TestChecklistCheck_NoActiveRun` | T-AC4d, EC-9 | `no active run for "foo" (use 'start' to begin)`; non-zero; no `CheckItem` on empty ID |
| `TestChecklistCheck_AutoCompletes` | T-AC4e, EC-12 | checking last item → run status `completed` in re-fetched render |
| `TestChecklistCheck_BadPositionArg` | (parse guard) | `check foo x` → integer-parse error; non-zero |
| `TestChecklistStatus_Active` | T-AC5a | 3 items, 1 checked, status `active` |
| `TestChecklistStatus_NoRun` | T-AC5b, EC-9 | error steering to `start`; non-zero |
| `TestChecklistReset_WithActiveRun` | T-AC6a | old run abandoned; new active run all-unchecked |
| `TestChecklistReset_NoActiveRun` | T-AC6b, EC-10 | succeeds, fresh active run, no error |
| `TestChecklistAbandon_WithActiveRun` | T-AC7a | run status `abandoned`; subsequent `status` hits EC-9 |
| `TestChecklistAbandon_NoActiveRun` | T-AC7b, EC-11 | `no active run...`; non-zero |
| `TestChecklistJSON_MatchesModelTags` | T-AC8a | any verb `--json` decodes into `checklist.Run`/`Template` (flat model tags) |
| `TestChecklistJSON_NdjsonMutuallyExclusive` | T-AC8b, EC-15 | `--json --ndjson` → mutually-exclusive error; non-zero |
| `TestChecklistHelp_DocumentsLimitation` | T-AC10, AC-10 | `checklist --help` `Long` mentions state does not survive `rk index`/is not vault-native |

## Known risks / remaining ambiguities

- **`--items-file` cwd-relative path.** No repo precedent for a bulk-file flag (codebase-analysis.md §3). Path resolves relative to process cwd (consistent with add.go:223); tests write the file into `t.TempDir()` and pass an absolute path to avoid cwd coupling.
- **`start` `resumed` discriminator is Pretty-only.** Carried as `json:"-"` so JSON stays pure-model (T-AC8a). No test asserts the fresh-vs-resume wording; if a future ticket wants it in JSON, that is a model/scope change, not this ticket.
- **Global flag/output state across `Execute()` calls.** Mitigated by `resetChecklistFlags` + `t.Cleanup` resets; without them the suite is flaky (not a production bug — the real binary is one process per invocation).
- **Stale `internal/cli/AGENTS.md` / `internal/storage/AGENTS.md`** describe the deleted `initServiceE()` global pattern (codebase-analysis.md §6). Do NOT follow them; follow `rk index`'s per-invocation open. Fixing those docs is out of scope.

### Critical Files for Implementation
- internal/cli/checklist.go (to create)
- internal/cli/checklist_test.go (to create)
- internal/cli/root.go
- internal/checklist/service.go
- internal/cli/todo.go
