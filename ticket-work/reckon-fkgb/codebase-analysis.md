# reckon-fkgb codebase analysis: `rk todo edit` (deadline/scheduled/depends/priority)

All paths relative to the worktree. Line numbers verified against current HEAD (post reckon-fnqs.7).

## 1. Ticket claims vs current code

| Claim | Status |
|---|---|
| `rk todo` has only add/done/list | TRUE. `todoCmd.AddCommand(todoAddCmd, todoListCmd, todoDoneCmd)` internal/cli/todo.go:137. fnqs.7 only added a TUI wizard for `add` (todo_add_wizard.go, calls `addDurableTodo` at :111); nothing edits existing todos. |
| `today act <ref> D <date>` sets deadline | TRUE (today.go:528 `actDeadline`). Also `d`/defer sets `scheduled` (today.go:507), `p` sets priority A/B/C (today.go:552). Ticket missed that scheduled IS reachable via `d`. |
| No key for depends | TRUE. Key set t/d/D/p/x/i/c (today.go:384 `normalizeActKey`). |
| No way to clear a field | TRUE. `node` has no Remove/Clear field primitive (grep empty). `SetField` with "" leaves `key: ` (blank value). |
| Not discoverable from `rk todo --help` | TRUE. |

## 2. Files likely modified

| File | Change |
|---|---|
| internal/cli/todo.go | add `todoEditCmd`, flag vars, `runTodoEditE`, extend `resetTodoFlags` (:60) name list and `init()` (:116) |
| internal/cli/today.go | optional: reuse shared helpers (see 4); extend `dispatchTodayAct` only if adding a depends key |
| internal/node/*.go | only if adding a field-removal / link-edit primitive (needed for depends and clear); must keep byte-preservation gate (internal/node/AGENTS.md) |
| internal/cli/todo_test.go (helper `runTodo` :112), today_test.go (`runToday` :126) | tests; root_help_test.go:42/110 lists verbs only at top level, no change needed |
| docs/design/composable-redesign.md, internal/cli/todo.go `todoCmd.Long` (:86) | doc/help text mention of edit |

internal/cli/AGENTS.md is mostly gen-1 (task/notes verbs); relevant rules only: honor `--quiet`, errors to stderr via returned error, validate args via Cobra `Args`.

## 3. Existing patterns

**Ref resolution (shared, reuse as-is):** `loadNativeTodoForEdit(vaultDir, ref)` today.go:441: ULID fast-path `todos/<ref>.md` via `loadDurableTodoAt` (todo.go:893, refuses CRLF, nil on not-exist), rejects non-`todo` type, else `findDurableTodoByRefOrAlias` (todo.go:914; glob `todos/*.md`, match ULID or alias). Error strings are hard-coded `today act:` prefix (today.go:462) and `todo done:` inside the load helpers (todo.go:893-935); a `todo edit` caller will see wrong prefixes unless wrapped/parameterized. `doneDurableTodo` (todo.go:682) duplicates the same resolution inline.

**Mutate + write:** `n.SetField/InsertField` (span-splice, re-parse in place) then `writeFileAtomic(path, n.Serialize())` (adopt.go:249; temp file in same dir + rename, preserves mode). No file locking anywhere; read-modify-write race is accepted repo-wide. `setOrInsertField` today.go:467 = `HasField ? SetField : InsertField`. InsertField puts new key as FIRST line inside the fence (insert.go:24), not canonical order; fine.

**Validate before mutate:** `actPriority` (today.go:552) validates A/B/C before any write so rejection leaves file byte-identical. `parseSchedDate` (recur.go:97, strict `2006-01-02`, UTC; error text says "scheduled date" even for deadline, so `actDeadline` wraps it). `parseRepeat` recur.go:63. `resolveDeferDate` (today.go:491) adds `tomorrow`/`next-week`; `todoNow` (todo.go:33, UTC, test seam) is the clock.

**Output:** `output.ModeFromFlags(jsonFlag, ndjsonFlag)`; print `res` via `output.New(cmd.OutOrStdout(), mode).Print(res)` unless `mode == output.Pretty && quietFlag`; result types implement `Pretty()` (todoDoneResult todo.go:234, todayActResult today.go:134). JSON field names of existing results are test-pinned (header comments in todo_test.go / today_test.go); a new `todoEditResult` is free but should mirror `todayActResult` (ref, id, path, state, scheduled, deadline, priority, depends).

**Index:** `todo add`/`done` do NOT reconcile. `today act` opens the index, reconciles before, classifies work-ticket (rejects read-only external rows, today.go:343-356), and reconciles after as non-fatal warning (:362-374) so `rk query` sees the write. `todo edit` should decide which; today act's post-write reconcile is the nearest precedent for "mutating verb of an existing node".

**Flag reset on singleton RootCmd:** flag vars are package globals (todo.go:40-54); every RunE starts `defer resetTodoFlags(cmd)`, which zeroes the vars and sets `Changed=false` for names in the hard-coded list at todo.go:74 *only if registered on cmd*. New flags must be added to BOTH the var block and that name list, or state leaks between in-process test invocations (and `Changed` is how "flag given, value empty" is distinguishable, relevant to clear semantics). Note `--author` and `--ephemeral` flag vars are shared across add/done registrations.

**Ephemeral vs durable:** ephemeral = checkbox lines in `todos/inbox.md`, addressed by 1-based index with `--ephemeral`; `add` rejects scheduled/deadline/depends/repeat for ephemeral (todo.go:313-315); `done --ephemeral` splices one byte. `today act` is durable-only (no ephemeral path). `todo edit` should be durable-only with the same rejection text style, or reject `--ephemeral` entirely [OPEN].

## 4. Signatures to reuse / share

| Symbol | Where | Note |
|---|---|---|
| `loadNativeTodoForEdit(vaultDir, ref string) (*node.Node, string, error)` | today.go:441 | resolution; prefix issue above |
| `setOrInsertField(n *node.Node, key, value string) error` | today.go:467 | |
| `writeFileAtomic(path string, data []byte) error` | adopt.go:249 | |
| `relTodoPath(vaultDir, path string) string` | todo.go:971 | |
| `parseSchedDate(s) (time.Time, error)` | recur.go:97 | |
| `resolveDeferDate(arg string, now time.Time) (string, error)` | today.go:491 | `today act`-prefixed errors |
| `parseRepeat(s) (repeatSpec, error)` | recur.go:63 | |
| `(*Node).SetField/InsertField/HasField/SetAliases` | node.go:153/insert.go:24/node.go:177/node.go:199 | |
| `actDefer/actDeadline/actPriority` | today.go:507/528/552 | each bundles validate + set + write + result; **cannot be shared directly** because they write once per call and hard-code `today act:` errors and `Key`. A multi-flag edit needs validate-all, mutate-all, ONE atomic write. Recommended: extract pure helpers (`validateDeadline(arg) (string, error)`, `validatePriority`, set-field helpers) used by both act* and edit, keeping act* behavior/error text unchanged (today_test.go pins them) [INFERRED]. |

`today act` flow: `runTodayActE` today.go:314 -> `dispatchTodayAct` :407 -> `act*`.

## 5. How `todo add` handles each field (and depends representation)

- `--scheduled`/`--deadline`: stored raw, NOT validated for plain add (todo.go:298-310 validates `--scheduled` only when `--repeat` is set; `deadline` never). Edit should validate with `parseSchedDate`; consider whether to also tighten add [OPEN].
- `--priority`: not an `add` flag at all; only `today act p`. Priority is also absent from `todoListItem` (todo.go:162) so `todo list` never shows it.
- `--depends`: raw string, no resolution/existence/self-reference/cycle check, no `[[ ]]` wrapping by caller. `addDurableTodo` (todo.go:368) sets `n.Links=[{Rel:"depends-on",To:depends}]`; `Render` (render.go:32) emits frontmatter key **`depends-on: "[[X]]"`** (quoted; multiple links of one rel collapse to one comma-joined line). Parser derives `Link.Rel` = the frontmatter key verbatim (node.go:382-385), so the on-disk key must be `depends-on` to yield a `depends-on` edge. todo_test.go:324-328 pins Links, not Props.
- DISCREPANCY: design doc (composable-redesign.md:487, :523, :548) and node.go:13/519 say authored `depends: [[X]]` maps to `depends-on`, but no per-type mapping exists in code (grep: none under internal/ besides comments). A hand-written `depends:` key yields rel `depends`, invisible to `loadDependsOn` (todo.go:603, `rel='depends-on'`). `todo edit --depends` must write `depends-on`, and should probably also handle an existing `depends:` line [OPEN].
- Index side: `todo list` reads depends from `edges` (rel `depends-on`, LIMIT 1) -> only first target displayed; scheduled/deadline from props via `loadTodoProps` (todo.go:583).
- Clearing: no existing mechanism for any field. Options: new node primitive removing a scalar line (span of whole `key: value\n` line; fieldSpans only cover the value, so a line span must be derived) [INFERRED]; or documented empty-string semantic. If clear is supported, handle a block-list-form `depends-on:` (no fieldSpan; only `blockSpans`, node.go:199 SetAliases is the only consumer).

## 6. Pitfalls

1. **Block-list frontmatter.** `InsertField` checks `fieldSpans` only; on a key authored as an Obsidian block list (`depends-on:` + `  - "[[X]]"`) it would insert a second duplicate line, and last-wins parsing silently drops the edit (internal/node/AGENTS.md "SetAliases"). Setting `depends-on` (ref-valued, may be block-list or flow multi-target `"[[A]], [[B]]"`) needs a generalized `blockSpans` splice, not `setOrInsertField`. Same risk for any scalar a hand-editor wrote as a block list (unlikely for dates/priority).
2. **SetField on a ref-valued key fails.** A key whose value parses as `[[..]]` has fieldSpan still (scalar line) but is exposed in `Links`, not `Props`; `HasField` is by fieldSpans so it works, but `n.Props["depends-on"]` is empty - do not use Props to detect presence.
3. **Replace vs append semantics for depends** (multi-target lines exist): decide set-replaces-all; `Render`/`parseRefValues` accept comma-joined quoted form `"[[A]], [[B]]"`.
4. **Recurrence interaction.** `repeat` requires a `scheduled` cursor; `doneRecurringTodo` errors if `scheduled` missing/blank (todo.go:817-819), and advances only `scheduled` (:845), never `deadline`. So: editing `scheduled` on a repeating todo is legitimate (today act `d` already does it, no check); clearing `scheduled` (or setting blank) while `repeat` present would break completion - reject. Changing `repeat` itself is out of the ticket's scope. Deadline on a recurring todo does not advance [INFERRED from absence in doneRecurringTodo].
5. **Done/cancelled todos.** `today act` does not guard on state (editing deadline on a done todo succeeds). Decide whether edit should.
6. **Dates/timezone** (REVIEW_PATTERNS.md "Time and Timezone Handling"): use `parseSchedDate` + `todoNow()` for shorthands; do not mix `time.Now()` local with UTC-parsed dates. `d` shorthands (`tomorrow`/`next-week`) exist only in `today act`; decide whether `--scheduled` accepts them [OPEN].
7. **--quiet** honored on output (REVIEW_PATTERNS "Ignoring --quiet"); pretty output for no-op edits (value unchanged) should still write nothing or be idempotent - SetField splice of identical bytes is byte-stable.
8. **Byte-preservation gate:** every edit must go through SetField/InsertField (or a new surgical primitive covered by `TestRoundTripIdentity`/`FuzzRoundTripIdentity`-style tests in internal/node); never regenerate via `Render()` for an existing file (add-only path). CRLF files are refused by loaders.
9. **Reset flags** (section 3) and validate-all-before-first-write: a bad `--priority` combined with a good `--deadline` must leave the file byte-identical (mirror `actPriority` test at today_test.go:661-679).
10. **Error-prefix leakage:** reusing `loadNativeTodoForEdit` yields "today act: no todo found ..." for `rk todo edit`; tests pin these strings for today act, so parameterize rather than change.
11. **Depends validation gap:** neither add nor edit verifies the target exists (index resolves later; dangling edges allowed by design invariant 4). Self-dependency (`--depends <own id>`) is not guarded anywhere [OPEN]; decide whether edit validates via index or stays lenient like add.

## 7. Design options (for the planner)

- A (ticket's primary ask): `rk todo edit <ref> [--deadline D] [--scheduled D] [--priority A|B|C] [--depends REF]`, durable-only, one validate-all/one-write pass, `--json`/Pretty result, `Changed`-based flag detection, require at least one flag. Needs a depends write primitive in node (block/flow-aware) and a decided clear syntax.
- B: add a `depends`/`s` key to `today act`. Smaller, but keeps the discoverability problem and `today act` is single-field per call.
- A can still leave `today act` untouched; sharing validators avoids drift.
