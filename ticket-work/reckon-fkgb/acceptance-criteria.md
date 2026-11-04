# reckon-fkgb — `rk todo edit` acceptance criteria

## 0. Verified state of the code (ticket claims vs. today)

| Claim | Status | Pointer |
|---|---|---|
| `rk todo` has only add/list/done | TRUE | `internal/cli/todo.go:137` (`todoCmd.AddCommand(todoAddCmd, todoListCmd, todoDoneCmd)`) |
| `rk today act <ref> D <date>` is the only deadline mutator | TRUE | `internal/cli/today.go:528` `actDeadline` |
| `today act` keys cover no depends/scheduled | PARTIALLY FALSE | `d`/`defer` sets `scheduled` (`today.go:507`, accepts `tomorrow`/`next-week`/date). Only **depends-on** is truly uncovered |
| `priority` has no add-time path | TRUE (not in ticket) | `todo add` has no `--priority`; only `today act p` (`today.go:552`, A/B/C only) |
| depends cannot be changed post-creation | TRUE | `depends-on` is a frontmatter **ref-valued link** written via `node.Render` as `depends-on: "[[ID]]"` (`todo.go:392`, `node/render.go`), not a Props scalar. `SetField`/`InsertField` are scalar-only; **no remove-field and no link-edit primitive exists in `internal/node`** |

Existing semantics to reuse (not re-decide):
- Ref resolution: ULID fast-path `todos/<ref>.md`, else walk `todos/*.md` matching ULID or alias, `type: todo` only (`today.go:441 loadNativeTodoForEdit`, `todo.go:914 findDurableTodoByRefOrAlias`). First match wins; **ambiguity is not detected** today.
- Date validation: `parseSchedDate` = strict `YYYY-MM-DD` UTC (`recur.go:97`). `todo add` only validates `--scheduled` when `--repeat` is set; `--deadline` and `--depends` are **unvalidated on add** (`todo.go:318-327`, `addDurableTodo`).
- Writes: `setOrInsertField` (SetField-or-InsertField) then `writeFileAtomic(foundPath, n.Serialize())` (`today.go:467`); byte-preserving by construction.
- Index: `today act` Reconciles before and after (`today.go:314-370`); `todo add/done` do not touch the index.
- Output: `todayActResult` JSON with `ref,key,id,path,state,<changed field>` (`today.go:~95-150`).
- Recurrence: cursor is the `scheduled:` prop; `repeat:` requires `scheduled` (add enforces, `todo.go:318`; `doneRecurringTodo` errors if `scheduled` blank, `todo.go:808`). Design: `docs/design/composable-redesign.md` "Recurrence (A#3b/c)".

## 1. Explicit acceptance criteria

1. `rk todo edit <ref>` exists, appears in `rk todo --help` (and the `todo` Long description), with `Use: "edit <ref>"`, `cobra.ExactArgs(1)`, `SilenceUsage: true` like siblings.
2. Flags: `--deadline <date>`, `--scheduled <date>`, `--depends <ref>`, `--priority <A|B|C>`. Each is optional; any combination in one invocation applies atomically (one file write, all-or-nothing).
3. `<ref>` resolves identically to `rk todo done <ref>` (ULID or alias, durable `type: todo` only).
4. Date flags accept literal `YYYY-MM-DD` and are validated with `parseSchedDate` before any mutation; `--priority` is validated A/B/C before any mutation. Rejected input leaves the file byte-identical.
5. Only the named fields change; every other byte of the file is preserved (`node.Serialize()` span-local edit; gated round-trip invariant).
6. `--depends` replaces/creates the `depends-on` link in the same canonical form `todo add` renders (`depends-on: "[[<ref>]]"`), and the result re-parses to exactly one `Links` entry with `Rel=depends-on`, not a Props entry (mirror `TestTodoAdd_DurableWithDependsLink`, `todo_test.go:303`).
7. `--json`/`--ndjson` emit one result object (fields: `kind`,`ref`,`id`,`path`,`state`, plus the edited fields `deadline`,`scheduled`,`depends`,`priority`, all `omitempty`) via `output.New(...).Print`; `--quiet` suppresses Pretty output only (same gating as `todo.go:672`).
8. After a successful edit, `rk query`/`rk todo list`/`rk today` reflect the change without a manual rebuild (see I-7).
9. `rk today act` keeps working unchanged (no regression); its `D`/`d`/`p` actuators remain.
10. Tests (table in §4) added in `internal/cli/todo_test.go` style; `go test ./...`, `go vet ./...` pass; `rk todo edit` documented in the todo Long text and, if a command list exists, `internal/cli/AGENTS.md` (which already prescribes `edit` as the standard verb, l.31).

## 2. Implicit requirements

| # | Requirement | Decision |
|---|---|---|
| I-1 | Agent-first: structured result, stable field names, errors on stderr non-zero exit | as AC-7; error prefix `todo edit: ...` |
| I-2 | Discoverability | AC-1; optionally add a one-line pointer in `today act` help to `rk todo edit` |
| I-3 | Validation parity with `todo add` | `edit` must be at least as strict as `add`. Add is lax (no deadline validation, no depends check); edit uses strict dates. **Do not** retrofit `add` in this ticket, but see [OPEN-3] |
| I-4 | Ref resolution parity with `todo done` | AC-3; reuse `loadNativeTodoForEdit` (rename-error-prefix issue: its errors say `today act:`/`todo done:`; wrap or parametrize so errors say `todo edit:`) |
| I-5 | Ephemeral todos | Not editable: no frontmatter, no fields. `edit` targets durable only. `--ephemeral` flag is **not** added; an inbox ref (`inbox`) is not `type: todo` and yields "not found". Error text should say ephemeral items have no editable fields [INFERRED] |
| I-6 | Recurrence | Editing `--scheduled` on a recurring todo moves the cursor (valid, user-intended). Clearing `scheduled` on a todo that has `repeat:` must be **rejected** (would make `done` fail at `todo.go:~820`). `--deadline` on a recurring todo allowed. No `--repeat` flag (out of scope) |
| I-7 | Index freshness | Follow `today act` pattern: open index, `Reconcile()` after write, failure after the write is a stderr warning, never an error (`today.go:354-370`). Rationale in that comment applies |
| I-8 | Done/cancelled todos | `state: done` todos are editable for deadline/priority/depends [OPEN-1] default allow; no state change ever made by `edit` |
| I-9 | depends integrity | Reject self-dependency (resolved ULID == target ULID, either by ULID or alias). Reject unknown target [OPEN-2]. Cycle detection: reject direct cycle A->B->A at minimum [OPEN-4] |
| I-10 | Atomicity | Compute all new bytes in memory, then single `writeFileAtomic`; a mid-way validation failure never writes |
| I-11 | CRLF / unparsable file | Inherit refusal from `loadDurableTodoAt` (`todo.go:893`, reckon-vj55) |
| I-12 | Per-field write order matters | Apply via sequential `setOrInsertField` on the in-memory node (each re-parses); validate everything first |

## 3. Edge cases

| Case | Required behavior |
|---|---|
| No flags: `rk todo edit <ref>` | Error `todo edit: nothing to change (give --deadline/--scheduled/--depends/--priority)`, exit non-zero, file untouched [OPEN-5: interactive prompt when TTY — default **no**; TUI is out of scope] |
| Clear a field | Needs an explicit syntax [OPEN-6]. Default: empty string `--deadline ""` clears. Clearing requires **removing the key line**, which `node` cannot do today: needs a new `node.RemoveField` (byte-preserving line splice) — in scope as a prerequisite if clear is accepted |
| Invalid date (`2026-13-01`, `tomorrow` on deadline, `2026-7-1`) | Error from `parseSchedDate`, file untouched. `--scheduled` additionally accepts `tomorrow`/`next-week` iff we reuse `resolveDeferDate` [OPEN-7, default: accept for `--scheduled` only, matching `today act d`; deadline literal only, matching `D`] |
| Invalid priority (`a`, `D`, `1`) | Error `invalid value ... (want A, B, or C)`; case-sensitive like `actPriority` |
| Idempotent re-edit (same value) | Succeeds, exit 0, file byte-identical, result identical; no mtime churn required but acceptable [INFERRED] |
| Unknown ref | `todo edit: no todo found matching "<ref>" (not found)`, exit non-zero |
| Ambiguous ref (alias shared by two files / alias equals another's ULID) | Today: first glob match wins silently. Default: keep parity (do not fix here); file follow-up ticket for ambiguity detection across done/edit/act [OPEN-8] |
| Field absent vs present-but-blank | Use `HasField` trichotomy (`setOrInsertField`); blank `deadline:` line gets `SetField` |
| `--depends` target is an alias | Store as given vs. canonicalize to ULID [OPEN-9]; default: store exactly what `todo add --depends` would store (as given) for parity |
| `--depends` replaces existing | Single-target replace. Pre-existing multi-target `depends-on: "[[A]], [[B]]"` (hand-authored, `node.go:519` supports lists) is overwritten wholesale; document, or refuse with message [OPEN-10, default overwrite+document]. Add-semantic (`--depends-add/--depends-rm`) out of scope |
| `depends-on` key authored as Props / odd form | Edit must not leave two `depends-on` lines |
| Ref is a `work-ticket` or non-todo node | Not found (type filter), consistent with done/act |
| Body-only file with no frontmatter | Not a todo (no `type`), not found |
| Concurrent hand edit between read and write | Out of scope (same race exists in done/act) |

## 4. Test scenarios (given / when / then)

| ID | Scenario |
|---|---|
| E1 | todo w/o deadline / `edit <id> --deadline 2026-08-01 --json` / file gains `deadline: 2026-08-01`, JSON `deadline` set, all other bytes identical |
| E2 | todo with deadline / edit new date / value replaced, key not duplicated |
| E3 | `--scheduled` same as E1/E2 for `scheduled` |
| E4 | `--priority B` on todo with/without priority / `priority: B` set |
| E5 | todo with no depends / `--depends <A-ULID>` / exactly one `depends-on: "[[A]]"`; `n.Links` has one depends-on, Props has none |
| E6 | todo depending on A / `--depends B` / link now points to B only; `rk query` edges show B not A after edit (index freshness) |
| E7 | alias ref / `edit <alias> --deadline ...` / same file edited as by ULID |
| E8 | all four flags at once / single write; all four fields present; unrelated frontmatter and body byte-identical |
| E9 | hand-authored file with CRLF-free exotic frontmatter (comments, extra keys, order) / edit / diff touches only the target line(s) |
| V1 | `--deadline 2026-02-30` / error, file byte-identical (sha before == after) |
| V2 | `--priority Z` / error, file byte-identical |
| V3 | `--deadline` valid + `--priority` invalid / error, **neither** field written (atomicity) |
| V4 | no flags / error "nothing to change", file untouched |
| V5 | unknown ref / error not-found, exit non-zero |
| V6 | ref `inbox` (ephemeral container) / not found / inbox.md untouched |
| V7 | `--depends <own ULID>` and `<own alias>` / rejected, file untouched |
| V8 | `--depends <nonexistent>` / rejected per [OPEN-2] default; file untouched |
| V9 | A depends on B, edit B `--depends A` / rejected (direct cycle) |
| V10 | CRLF file / refused with vj55 message |
| R1 | recurring todo (`repeat: +1w`, `scheduled: D1`) / `--scheduled D2` / cursor moves; subsequent `todo done` advances from D2 |
| R2 | recurring todo / clear `scheduled` / rejected |
| R3 | recurring todo / `--deadline` / allowed, repeat+scheduled untouched |
| D1 | `state: done` todo / `--deadline` / per [OPEN-1] default succeeds, `state` still done |
| I1 | idempotent: run E1 twice / second run exit 0, bytes unchanged |
| C1 | `--deadline ""` (if [OPEN-6] accepted) / `deadline:` line removed, neighbors intact; clearing absent field is a no-op success |
| X1 | `--json` vs `--ndjson` vs pretty vs `--quiet` / shapes per AC-7 |
| X2 | `rk todo --help` lists `edit`; `rk todo edit --help` lists four flags |
| X3 | after edit, read-only `rk query "SELECT ... deadline"` sees new value with no manual reconcile |
| X4 | regression: existing `today act D/d/p` tests still pass |
| X5 | index Reconcile failure after write / warning on stderr, exit 0 (mirror today act) |

## 5. Out of scope

- Editing body/title/aliases/state/author/`pinned`, `--repeat` (add/clear/change), `start`/`cancel` transitions (stay in `today act`).
- Ephemeral inbox items (no fields).
- Interactive/TUI edit wizard; `--edit` ($EDITOR) mode.
- Multi-target depends management (`--depends-add/--depends-rm`), transitive-cycle detection beyond the direct case [OPEN-4].
- Retrofitting `todo add` validation (deadline/depends) and `--priority` on add; follow-up ticket candidates.
- Fixing ref ambiguity globally; removing/deprecating `today act`.
- Org-style relative dates beyond what `today act d` already accepts; time-of-day.
- Editing external `work-ticket` nodes.

## 6. Open questions (human decisions)

| ID | Question | Recommended default |
|---|---|---|
| OPEN-1 | Editable when `state: done`/`cancelled`? | Allow (backfilling metadata is the ticket's dogfood case); never change state |
| OPEN-2 | `--depends` target must exist as a node? `add` does not check | Require it resolve to an existing node (todo or any type with ULID/alias) via file walk or index; error otherwise. Hand-edit remains the escape hatch for dangling links (design allows dangling `[[...]]`, `composable-redesign.md` A#6, so this is a UX guard, not an invariant) |
| OPEN-3 | Should `todo add` get the same validation + `--priority`? | No, separate ticket; keep this PR scoped |
| OPEN-4 | Cycle detection depth | Reject self and direct 2-cycle; skip transitive (index query could do it later) |
| OPEN-5 | No-flags behavior | Error, no prompt |
| OPEN-6 | Clear syntax | `--deadline ""` / `--scheduled ""` / `--depends ""` / `--priority ""` clear; requires new `node.RemoveField`. Fallback if too costly: ship set-only, file follow-up for clear (note: `--depends ""` and clear are the only way to fix a wrong `--depends` into none) |
| OPEN-7 | `--scheduled` accepts `tomorrow`/`next-week`? | Yes (reuse `resolveDeferDate`); `--deadline` literal only |
| OPEN-8 | Ambiguous ref | Parity (first match); follow-up ticket |
| OPEN-9 | Canonicalize alias depends to ULID? | No, store as given (parity with add) |
| OPEN-10 | Existing multi-target depends on `--depends X` | Overwrite wholesale, state in help text |
| OPEN-11 | Also extend `today act` with a depends key (ticket's alternative)? | No; ticket offers either-or, `todo edit` chosen. Mention `rk todo edit` in `today act` help |
