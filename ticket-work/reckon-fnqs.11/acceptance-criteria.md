# reckon-fnqs.11 — Acceptance Criteria: `rk checklist` CLI verb surface

Source: `bd show reckon-fnqs.11` (full text read), plus confirmation reads of
`internal/checklist/{model,service,repository}.go`, `internal/cli/{todo,dispatch,root,index}.go`,
`internal/output/output.go`, `internal/config/config.go`, `internal/index/{index,schema}.go`.

## Domain facts pinned by code (not the ticket prose)

| Fact | Source |
|---|---|
| Position is 0-based everywhere in the domain layer | `service.go` doc comments on `RemoveTemplateItem`/`CheckItem`: "0-based position" |
| `CheckItem` **toggles** checked state, does not set it | `service.go:147-184`; `newChecked := !item.Checked` |
| `StartRun` **errors** if an active run exists (`"use 'reset' to start fresh"`) — it does not auto-resume | `service.go:107-126` |
| `GetActiveRun` errors when no active run exists (`"use 'start' to begin"`) — it is the only way to fetch "the" active run | `service.go:129-143` |
| `CreateTemplate` does not reject an empty `items` slice | `service.go:19-40` |
| `ResetRun` succeeds even with no prior active run (abandon-if-present, then always start fresh) | `service.go:192-213` |
| `AbandonRun` errors if no active run exists | `service.go:218-237` |
| `ListRuns(includeCompleted bool)` — only Service method for run history | `service.go:240-242` |
| `--json`/`--ndjson` are **persistent root flags**, mutually exclusive, resolved via `output.ModeFromFlags` → `output.Writer` | `internal/cli/root.go:29-30,84-97`, `internal/output/output.go:32-43` |
| No existing CLI caller of `storage.NewDatabase` anywhere in `internal/cli/*.go` | grep across `internal/cli` |
| Only path helper for `storage.Database` is `config.DatabasePath()` → `~/.reckon/reckon.db` | `internal/config/config.go:88-89` |
| `rk index --rebuild` (`internal/cli/index.go`) opens/rebuilds **only** `cfg.CacheDir/<vaultID>/index.db`, explicitly commented "independent of the legacy operational database" | `internal/cli/index.go:14-15`, `internal/index/index.go:52-57` |
| `index.Rebuild()`'s `dropDDL` names only `_nodes,_edges,_props,_aliases,fts_search,_file_meta,_index_meta` — no `checklist_*` tables | `internal/index/schema.go:98-111` |
| `checklist_templates/checklist_template_items/checklist_runs/checklist_run_items` are defined (`CREATE TABLE IF NOT EXISTS`) in `internal/storage/database.go`, the legacy operational DB schema — a physically different file from `index.db` | `internal/storage/database.go:116-159` |

---

## 1. Explicit acceptance criteria

| # | Verb | Criterion |
|---|---|---|
| AC-1 | `create` | `rk checklist create <name> [--item TEXT]... [--items-file PATH]` calls `Service.CreateTemplate(name, items)`. Empty name and duplicate name both propagate the Service error verbatim, non-zero exit. |
| AC-2 | `list` | `rk checklist list` (bare) calls `Service.ListTemplates()`. Empty result is not an error (Pretty: "no templates"; JSON: `[]`). |
| AC-3 | `start` | `rk checklist start <template>` starts a fresh run if none is active, or displays/resumes the existing active run without erroring if one is (see IR-1). Unknown template → clear not-found error. |
| AC-4 | `check` | `rk checklist check <template> <position>` resolves the template's active run, then calls `Service.CheckItem(run.ID, position)`. No active run, or out-of-range position, both propagate as errors. |
| AC-5 | `status` | `rk checklist status <template>` calls `Service.GetActiveRun(template)` and renders per-item checked state + progress count. No active run → clear error steering the user to `start`. |
| AC-6 | `reset` | `rk checklist reset <template>` calls `Service.ResetRun(template)` unconditionally (works with or without a prior active run). |
| AC-7 | `abandon` | `rk checklist abandon <template>` calls `Service.AbandonRun(template)`. No active run → propagate Service error. |
| AC-8 | `--json` | Every checklist subcommand supports the existing root `--json`/`--ndjson` pair via `output.ModeFromFlags` + `output.Writer`, matching `todo`/`query`/`index`'s convention — no bespoke per-verb JSON flag. |
| AC-9 | `--items-file` | `create` accepts `--items-file PATH` as an alternative/supplement to repeated `--item` for supplying item text (format: [OPEN], see IR-6). |
| AC-10 | limitation doc | `rk checklist --help` (parent command `Long`) states plainly that checklist template/run state lives in the operational DB, is not vault-native, is not re-derived from vault text, and is not versioned with the vault in git. **This part is unconditional — proceed regardless of OPEN-1.** The one specific sentence "…and does not survive `rk index --rebuild`" is contingent on OPEN-1's resolution; write that clause last, once OPEN-1 is settled, so the rest of the doc work isn't blocked. |
| AC-11 | run-history reachability | `Service.ListRuns` must be reachable through *some* verb per the ticket's "Done when" list, but the Scope bullets never name a "list runs" command. [OPEN, see IR-8] |

---

## 2. Implicit requirements

| # | Requirement | Status |
|---|---|---|
| IR-1 | **Start-vs-resume logic must live in the CLI, not the Service.** `StartRun` errors on an existing active run rather than returning it; there is no sentinel/typed error to branch on cleanly. Clean implementation using only existing public methods: `GetTemplate` (validate existence, isolates "not found") → `GetActiveRun` (if it succeeds, that's the resume path — display it, exit 0) → `StartRun` (only called when `GetActiveRun` errored, i.e. confirmed none exists). No Service changes needed, no string-matching on error text. | INFERRED (resolved) |
| IR-2 | **Position base for `check` is not specified by the ticket.** Domain layer is unambiguously 0-based (`CheckItem`/`RemoveTemplateItem` doc comments). But this exact codebase has a live precedent for 1-based user-facing indices elsewhere (`rk todo done --ephemeral <index>`: "1-based line index"). Whichever base is chosen, `status`'s per-item listing must display the same number `check` expects (no silent translation between display and input). | **OPEN** — recommend **1-based**, matching the repo's existing user-facing convention (`todo done --ephemeral`) and typical human expectation ("item 1" = first item); `status`'s renderer does the +1 display translation, `check`'s handler does the -1 before calling `Service.CheckItem`. 0-based passthrough is the simpler implementation (no translation layer, no off-by-one risk) but breaks with the repo's own precedent — flag to ticket owner if simplicity should win instead. |
| IR-3 | **Where does `rk checklist`'s `storage.Database` file live?** No existing CLI code opens `storage.NewDatabase` at all. The only path helper, `config.DatabasePath()`, points at `~/.reckon/reckon.db`, a file `rk index --rebuild` never touches (it only rebuilds `cfg.CacheDir/<vaultID>/index.db`, and that rebuild's DROP list doesn't include `checklist_*` tables even if they were co-located). **As currently architected, wiring checklist to the only established path convention makes AC-10 / the ticket's "Done when" rebuild-visibility bullet false.** | **OPEN — highest priority.** Must be resolved before AC-10 can be honestly written into `--help` text. Either (a) the ticket's constraint is aspirational/inaccurate and the rebuild-visibility "Done when" bullet should be dropped or reworded, or (b) checklist storage must be deliberately colocated with `index.db` and `dropDDL` extended to include `checklist_*` tables (a small, explicit addition, arguably still "as-is" since it doesn't touch checklist's own schema/business logic). |
| IR-4 | `check` must resolve the run via `GetActiveRun(template)`, not a raw run ID or `ListRuns`. This is what makes edge case EC-12 (mutating a completed run) structurally unreachable from the CLI — `GetActiveRun` only returns `status='active'` rows, so a completed/abandoned run can never be targeted by `check`. | INFERRED |
| IR-5 | `--item` and `--items-file` combination behavior (mutually exclusive vs. additive/concatenated) is unspecified. | **OPEN** — recommend mutually exclusive (error if both given) to avoid ordering ambiguity; simplest to implement and test. |
| IR-6 | `--items-file` format is unspecified; no precedent flag anywhere in the repo (grep clean). | **OPEN** — recommend plain text, one item per line, blank lines skipped, surrounding whitespace trimmed, file order preserved. |
| IR-7 | `CreateTemplate` itself permits zero items, but a zero-item run can never auto-complete (`allChecked` returns `false` when `len(items)==0`), producing a run that's permanently stuck `active`. | **OPEN/INFERRED** — recommend the CLI verb (not the Service) requires ≥1 item for `create`, rejecting empty `--item`/`--items-file` input before calling the Service. |
| IR-8 | Ticket's "Done when" list requires `ListRuns` reachable through a verb, but Scope's verb bullets don't name one. | **OPEN** — recommend folding into `list`: bare `rk checklist list` → templates (AC-2); `rk checklist list <template> [--all]` → that template's runs via `ListRuns(includeCompleted)`, `--all` mapping to `includeCompleted=true`. Avoids adding a verb outside the ticket's named list. |
| IR-9 | Exit codes / error surfacing: no existing checklist-specific convention exists yet; follow `todo`/`query`'s pattern (`RunE` returns the wrapped error, `SilenceUsage: true`, cobra prints to stderr, non-zero exit). | INFERRED |

---

## 3. Edge cases

| # | Case | Expected behavior |
|---|---|---|
| EC-1 | `check` an already-checked item | Toggles it back to **unchecked** (Service semantics, not idempotent-set) — must be explicitly tested, not treated as a no-op. |
| EC-2 | `check` position out of range (negative or ≥ item count) | Service error `"position %d out of range (run has %d items)"` propagated, non-zero exit. |
| EC-3 | `start` a template that doesn't exist | `GetTemplate` fails → clear not-found error, no run created. |
| EC-4 | `start` a template whose only run is already `completed` | Allowed — `GetActiveRunByTemplate` only matches `status='active'`; a completed run doesn't block a new one. Old completed run persists in history (visible via IR-8's `list --all`). |
| EC-5 | `start` a template with an already-active run | Resume path (IR-1) — no error, existing run displayed. |
| EC-6 | `list` (templates) when none exist | Empty result, not an error. |
| EC-7 | `--items-file` combined with inline `--item` | [OPEN, IR-5]. |
| EC-8 | Empty `--items-file` (0 lines / all blank) | Equivalent to 0 items; interacts with IR-7 — recommend reject with "at least one item required." |
| EC-9 | `check`/`status` on a template with no run ever started (or already abandoned/completed) | `GetActiveRun` errors `"no active run for %q (use 'start' to begin)"`; must not attempt `CheckItem` with an empty/invalid run ID. |
| EC-10 | `reset` with no active run yet | Succeeds anyway — `ResetRun` doesn't require a prior active run, just starts fresh. Contrast with `abandon`. |
| EC-11 | `abandon` with no active run | Service errors `"no active run for %q (use 'start' to begin)"`, propagated. |
| EC-12 | Checking the last unchecked item | Run auto-transitions to `completed` (`allChecked`); subsequent `status`/`check` against that template now hit EC-9 (`GetActiveRun` no longer finds it, since it's no longer `status='active'`). |
| EC-13 | `create` with duplicate name | Service error `"checklist template %q already exists"`, propagated. |
| EC-14 | `create` with empty name | Service error `"template name cannot be empty"`, propagated. |
| EC-15 | `--json` and `--ndjson` both passed | Already handled at the root/global level (`output.ModeFromFlags` → "mutually exclusive" error); no per-verb handling needed, but confirm it isn't bypassed by checklist-specific flag parsing. |

---

## 4. Test scenarios (given/when/then)

| ID | Given | When | Then |
|---|---|---|---|
| T-AC1a | no template named "foo" exists | `rk checklist create foo --item a --item b` | template "foo" created with 2 items at positions 0,1; exit 0 |
| T-AC1b | template "foo" already exists | `rk checklist create foo --item a` | error `checklist template "foo" already exists`; exit non-zero |
| T-AC1c | — | `rk checklist create "" --item a` | error `template name cannot be empty`; exit non-zero |
| T-AC2a | no templates exist | `rk checklist list` | Pretty: "no templates" message; `--json`: `[]`; exit 0 |
| T-AC2b | templates "foo","bar" exist | `rk checklist list` | both names listed; JSON array of 2 |
| T-AC3a | template "foo" exists, no run | `rk checklist start foo` | new active run created, exit 0, output shows fresh run (0 checked) |
| T-AC3b | template "foo" has an active run with item 0 checked | `rk checklist start foo` | no error; existing run returned/displayed showing item 0 checked (resume, not a new run) |
| T-AC3c | template "foo" does not exist | `rk checklist start foo` | not-found error, exit non-zero, no run row created |
| T-AC4a | active run for "foo" with 3 unchecked items | `rk checklist check foo 0` | item at position 0 becomes checked; run stays active (not all checked) |
| T-AC4b | active run for "foo", position 0 already checked | `rk checklist check foo 0` (again) | position 0 becomes **unchecked** (toggle, EC-1) |
| T-AC4c | active run for "foo" with 3 items | `rk checklist check foo 5` | error "position 5 out of range (run has 3 items)"; exit non-zero |
| T-AC4d | template "foo" exists, no active run | `rk checklist check foo 0` | error "no active run for \"foo\" (use 'start' to begin)"; exit non-zero |
| T-AC4e | active run for "foo" with 2 items, item 0 already checked | `rk checklist check foo 1` | both items now checked → run auto-transitions to completed |
| T-AC5a | active run for "foo", 1 of 3 items checked | `rk checklist status foo` | output shows 3 items, 1 checked, run status "active" |
| T-AC5b | template "foo" exists, no run started | `rk checklist status foo` | error steering to `start`; exit non-zero |
| T-AC6a | template "foo" has an active run (partially checked) | `rk checklist reset foo` | old run marked abandoned; new active run created with all items unchecked |
| T-AC6b | template "foo" exists, no active run | `rk checklist reset foo` | succeeds, creates a fresh active run (no error for missing prior run) |
| T-AC7a | template "foo" has an active run | `rk checklist abandon foo` | run status becomes "abandoned"; subsequent `status foo` hits EC-9 |
| T-AC7b | template "foo" exists, no active run | `rk checklist abandon foo` | error "no active run..."; exit non-zero |
| T-AC8a | any checklist verb | invoked with `--json` | output is valid JSON matching the record's `json:` tags from `model.go` |
| T-AC8b | any checklist verb | invoked with both `--json --ndjson` | error "mutually exclusive"; exit non-zero |
| T-AC9a | a items file with 3 non-blank lines | `rk checklist create foo --items-file items.txt` | template created with 3 items in file order |
| T-EC8 | items file with 0 non-blank lines | `rk checklist create foo --items-file empty.txt` | rejected with "at least one item required" (IR-7 resolution) or Service accepts 0-item template — **pin whichever IR-7 decision is made** |
| T-EC4 | template "foo" whose only run is `completed` | `rk checklist start foo` | new run created; old completed run still retrievable via run-history listing (IR-8) |
| T-AC11 | template "foo" has 1 completed run + 1 active run | `rk checklist list foo --all` (exact flag/shape pending IR-8 decision) | both runs listed with correct statuses; bare `rk checklist list foo` (no `--all`) shows only the active one — **stub: gated on IR-8's verb-shape decision, not forgotten** |
| T-AC10 | — | `rk checklist --help` | help text mentions checklist state does not survive `rk index --rebuild` (contingent on OPEN-1 resolution) |

---

## 5. Explicitly out of scope

- **Interactive mini-TUI** for running through a checklist — deferred to `reckon-fnqs.12` (blocked child ticket, "rk checklist run: interactive mini-TUI on the Prompt/Wizard layer").
- **Persistence rewrite** — no migration of checklist state to a vault-native (file-backed, index-derived) representation. `internal/checklist`'s `storage.Database`/`Repository`/`Service` are used exactly as they exist; this ticket adds a CLI layer only.
- **Changes to `internal/checklist/{model,service,repository}.go` business logic** — e.g. do not add a "set checked" (idempotent, non-toggle) method, do not add pagination, do not change position indexing to 1-based inside the Service. Any CLI-facing convention choice (IR-2, IR-5, IR-6, IR-7) should be implemented at the CLI/verb layer, not by changing Service signatures — *unless* resolving OPEN-1 requires the narrowly-scoped `dropDDL` addition described there.
- **Template mutation verbs** — `Service.AddTemplateItem`, `RemoveTemplateItem`, `DeleteTemplate` exist but are not named in the ticket's Scope bullets or "Done when" method list; no `rk checklist edit`/`delete` verb is required by this ticket.
