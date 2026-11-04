# Plan: reckon-fkgb, `rk todo edit`

## Summary

Add `rk todo edit <ref> [--deadline D] [--scheduled D] [--priority A|B|C] [--depends REF] [--no-deadline] [--no-scheduled] [--no-priority] [--no-depends]`. It covers durable todos only. Each value flag sets its field and each `--no-<field>` flag removes it. All flags are validated first, then applied in memory, then written in one `writeFileAtomic`.

There are two new node primitives, `(*Node).UpsertField` and `(*Node).RemoveField`. `today act` keeps its behavior and error text; only the shared resolution and validation code is extracted.

**One decision for the user** (D2 below); it does not block implementation. D1 was decided by the user: clear through flag negation.

**Corrections to the upstream docs**
- The analysis (§4, Pitfall 10) says `today act` error strings are test-pinned. A grep of `today_test.go`, `todo_test.go` and `todo_recur_test.go` finds no assertion on `today act:`, `invalid value`, `no todo found`, or the CRLF/vj55 text. Keep them unchanged because the task requires it, not because tests would fail.
- The analysis (§2) lists `composable-redesign.md` for a doc edit. It has no `rk todo` verb list (the only hit is `rk todo ls`, :801), so no edit is needed.
- `internal/cli/AGENTS.md` has no todo command list, and its verb section is gen-1, so it needs no edit either (AC-10 only applies "if a command list exists").
- New finding: `SetField` on a bare `deadline:` line with no trailing space writes `deadline:2026-08-01`. The gap group in `fmScalarRe` (`node.go:103`) is empty, so the value span starts right after the colon. Our parser accepts this, but it is not a YAML mapping for Obsidian. [INFERRED from the regex; not executed]

## Files to modify

| File | Change |
|---|---|
| `internal/node/insert.go` | Add `UpsertField(key, value string) error` (D3) and `RemoveField(key string) error` (D3b). |
| `internal/node/insert_test.go` | Tests N1-N12. |
| `internal/node/AGENTS.md` | Document both primitives; update the sentence saying only `SetAliases` reads `blockSpans`. |
| `internal/cli/todo.go` | Add `todoPriorityFlag`, the four `todoNo*Flag` bools, and their names to the `resetTodoFlags` var block and name list (:74). Add `todoEditCmd`, register flags in `init()`, add to `AddCommand` (:137). Update `todoCmd.Long` and the comment at :85. Add a `verb string` parameter to `loadDurableTodoAt` (:893) and `findDurableTodoByRefOrAlias` (:914); `doneDurableTodo` call sites pass `"todo done"`. |
| `internal/cli/todo_edit.go` (new) | `todoEditResult`, `runTodoEditE`, `validateTodoDepends`, post-write reconcile helper. Follows the `todo_add_wizard.go` / `todo_browse.go` file split. |
| `internal/cli/today.go` | Mechanical extractions only (table below), plus a one-line `rk todo edit` pointer in `todayActCmd.Long`. |
| `internal/cli/todo_edit_test.go` (new) | All CLI scenarios. Reuses `setupQueryVault`, `writeTestNode`, `writeTodoFixture`, `writeRecurringTodo`, `pinTodoNow`, `snapshotVaultFiles`, `runTodo`, `runQuery`, `parseNDJSONMaps`. |

**Extractions in `today.go`** (resulting strings stay byte-identical)

| New symbol | Purpose | Caller preservation |
|---|---|---|
| `resolveDurableTodoRef(verb, vaultDir, ref) (*node.Node, string, error)` | Body of `loadNativeTodoForEdit`; returns nil when not found. | `loadNativeTodoForEdit` calls it with `"todo done"` (today's CRLF/parse prefix under `today act`) and adds its own `today act: no todo found ...`. |
| `resolveSchedDateArg(arg, now) (string, error)` | Shorthand plus literal date, with no prefix. | `resolveDeferDate` wraps errors as `today act: defer: %w`. |
| `validatePriority(v) error` | Returns `invalid value %q (want A, B, or C)`. | `actPriority` wraps as `today act: priority: %w`. |

Not touched: `dispatchTodayAct`, `act*` flow, `normalizeActKey`, `doneDurableTodo`'s inline resolution duplicate.

**`runTodoEditE` flow**
1. `defer resetTodoFlags(cmd)`, then capture values using `cmd.Flags().Changed`.
2. If no flag was changed, return the nothing-to-change error.
3. Run pure validation (dates, priority, depends characters, empty values, set-and-clear conflicts). No I/O yet.
4. Get output mode and config.
5. Call `resolveDurableTodoRef("todo edit", ...)`; on nil return `todo edit: no todo found matching %q (not found)`.
6. Run node-dependent checks: the `--depends` validation with the index (D6), and the `--no-scheduled` recurrence guard (D7).
7. Apply `RemoveField` for clears, then `UpsertField` in the fixed order deadline, scheduled, priority, depends.
8. If `!bytes.Equal(original, n.Serialize())`, call `writeFileAtomic`.
9. Print the result unless `mode == Pretty && quietFlag`.
10. Run the post-write reconcile; any failure is a stderr warning unless `--quiet`.

**Result shape** (`todoEditResult`): `kind` (`"durable"`), `ref`, `id,omitempty`, `path`, `state,omitempty`, and `deadline`, `scheduled`, `priority`, `depends` (each `omitempty`, set fields only), and `cleared` (`omitempty`, field names in the fixed order). Pretty form: `todo: <ref> updated (deadline X, priority Y, cleared scheduled)`.

## Design decisions (with alternatives considered)

### Decided or needs the user

| ID | Decision | Reason | Alternatives |
|---|---|---|---|
| **D1 / OPEN-6: clear syntax** | **User decision: flag negation.** `--no-deadline`, `--no-scheduled`, `--no-priority`, `--no-depends` (bool flags; `--no-<name>` is the standard GNU/git spelling). A given-but-empty value (`--deadline ""`) is still rejected: `todo edit: --deadline: empty value (use --no-deadline to clear)`. Giving `--deadline X --no-deadline` together is rejected: `todo edit: --deadline and --no-deadline conflict`. | Closes the ticket's hand-edit pain, including "this todo depends on nothing". Explicit flags mean an unset `--deadline "$D"` cannot silently wipe a field. | (a) `""` clears: the unset-variable hazard above. (b) Set-only with a follow-up: leaves hand editing for removal. (c) `--clear <field>`: less conventional. |
| **D2: `depends:` vs `depends-on:`** | **Write `depends-on`.** Leave any existing `depends:` line untouched. File a follow-up to reconcile the doc with the code. | See below. | (a) Write `depends:` per the doc: `rk todo list` would lose the dependency (`loadDependsOn`, `todo.go:605`) and AC-6/AC-8 fail. (b) Implement per-type mapping in this ticket: changes index semantics, out of scope, and `note_v1_test.go:288` pins rel `depends` for notes. (c) Delete or refuse on a `depends:` line: needs a removal primitive, or blocks an otherwise valid edit. |

D2 detail: the design doc (:487, :523, :548, :948) has authors write `depends:` mapped to rel `depends-on`, but no code does that mapping. `todo add` writes `depends-on:` and `todo_test.go:324` pins the link. Writing `depends-on` honors the doc's normalized rel (invariant 3). The user may want to amend the doc or add the mapping.

### Resolved

| ID | Decision | Reason |
|---|---|---|
| OPEN-1 (done/cancelled) | Allow; `state` never changes. | Backfilling metadata is the dogfood case, and `today act` has no state guard. |
| OPEN-2 (target exists) | Require `--depends` to resolve through `lookupNodeByRefOrAlias` (`today.go:681`, any type or ULID or alias). | Typos are the bug being fixed. This is deliberately stricter than add (`TestTodoDependsOn_DanglingEdge` pins add as lenient). Dangling links stay valid in the model and hand-editing remains possible. |
| OPEN-3 (retrofit add) | No; follow-up. | Keeps the PR scoped. |
| OPEN-4 (cycles) | Reject self and direct 2-cycle only. | One SQL query; transitive detection is out of scope. |
| OPEN-5 (no flags) | Error `todo edit: nothing to change (give --deadline/--scheduled/--depends/--priority or a --no-<field> flag)`, no prompt. | Agent-first; TUI is out of scope. |
| OPEN-7 (shorthands) | `--scheduled` accepts `tomorrow`/`next-week`/date; `--deadline` is literal only. | Parity with `today act` `d`/`D`. Extends AC-4's "literal" wording as the AC's own default says. |
| OPEN-8 (ambiguous ref) | Keep first-match parity; follow-up. | Shared flaw across done/act/edit. |
| OPEN-9 (canonicalize alias) | Store as given. | Parity with add; the design's Fields table says `to` is a ULID or alias, resolved later (`### Fields`, :473). |
| OPEN-10 (multi-target) | Overwrite wholesale; state this in the flag help. | Add/remove semantics are out of scope, and the vault is git-tracked. |
| OPEN-11 (`today act` depends key) | No; add a pointer in `today act` Long. | `todo edit` solves discoverability; a key would not. |
| Analysis: `--ephemeral` | Do not register the flag; cobra rejects it as unknown. Ref `inbox` gets the generic not-found. | No fields exist to edit. Drops I-5's [INFERRED] special error text as unnecessary. |
| Analysis: tighten add dates | No. | Same as OPEN-3. |
| Analysis: existing `depends:` line | Leave it. | D2. |
| Analysis: self-dependency | Reject. | I-9 / V7. |

### D3: node primitive `UpsertField(key, value)`

Generalizes the `SetAliases` shape logic (`node.go:199`). All four flags use it, not `setOrInsertField`, so a block-list scalar cannot get a silently losing duplicate line.

| Shape of `key` | Behavior |
|---|---|
| Both `fieldSpans` and `blockSpans` (hand-authored duplicate) | Error, no mutation. Which line wins depends on order, so a splice could be silently lost. |
| Scalar only | Splice the value. If the span is empty and the previous byte is `:`, prepend a space (the bare-colon finding above). |
| Block only | Replace the whole block span with `key: value\n`. |
| Absent | Delegate to `InsertField`. |

Each path re-parses before committing to `*n`. Uses `\n` as `SetAliases` does; the CLI refuses CRLF earlier.

Alternatives rejected:
- `SetLink(rel, to)`: scalar fields would still lack block safety.
- Refactoring `SetAliases` onto this: not needed for the ticket, so leave it.
- Fixing the bare-colon bug inside `SetField`: changes `today act` bytes; file as a follow-up.

### D3b: node primitive `RemoveField(key)`

| Shape of `key` | Behavior |
|---|---|
| Absent | No-op, nil error, `Raw` unchanged. Clearing is idempotent. |
| Scalar | Remove the whole line, including its newline and any trailing comment. |
| Block | Remove the whole block span. |
| Duplicates, in any mix of shapes | Remove every occurrence. The parser keeps only one span per key, so removal repeats until the key is gone; a re-parse must show the key absent or the call errors with no mutation. |
| Removal would leave an empty frontmatter | Error, no mutation. `---\n---\n` re-parses as body (`node.go:243`). A valid todo always keeps `type`, so the CLI does not reach this. |

Each path re-parses before committing to `*n`, and bytes outside the removed lines stay identical.

`--no-depends` removes both `depends-on` and a hand-written `depends` line. Otherwise clearing would silently leave the dependency the author wrote per the design doc. `--depends X` still leaves a `depends` line alone (D2).

### D4: command shape
`rk todo edit` over a `today act` key (the ticket's alternative): `today act` is one field per call and stays undiscoverable from `rk todo --help`.

### D5: flag variables
Reuse `todoScheduledFlag`, `todoDeadlineFlag` and `todoDependsFlag`, the way `--author` is shared. Add only `todoPriorityFlag`. Do not confuse with the existing `todoEditFlag`, which is add's `--edit` bool (`todo.go:48`).

### D6: index use
- Open and reconcile **before** the write only when `--depends` is given; failure there is fatal. Plain field edits must not gain an index failure mode that `todo done` lacks.
- Post-write, a helper opens the index and reconciles; any failure, including `Open`, is a warning. Rationale matches `today.go:367-375`.
- Value stored: `strconv.Quote("[["+target+"]]")`, the same as `render.go:87`.

`validateTodoDepends` checks, in order:
1. **Characters** (pure, in step 3): reject `[ ] " | #`, newline, CR. Gives a clear message for `--depends "[[X]]"` and prevents a value that falls through `parseRefValues` into Props or truncates in `splitRef`.
2. **Existence**: `lookupNodeByRefOrAlias`.
3. **Self**: resolved `loc == relTodoPath(vaultDir, foundPath)`. Covers ULID, alias and id-less files.
4. **Direct cycle**: `SELECT 1 FROM edges WHERE src=? AND rel='depends-on' AND dst_key=(SELECT id FROM nodes WHERE loc=?)`.

### D7: recurrence
`--scheduled` moves the cursor, as `today act d` already does. `--deadline` and `--no-deadline` are independent; `doneRecurringTodo` never advances the deadline. `--no-scheduled` on a todo with `repeat` is rejected before any write, because completion needs the cursor (`todo.go:817-819`): `todo edit: --no-scheduled: todo has repeat; a recurring todo needs its scheduled cursor`.

### D8: determinism and idempotence
- Fixed apply order regardless of argv, because `InsertField` prepends. Clears run before sets; set-and-clear on one field is rejected earlier, so the order has no semantic effect. Resulting top lines: `depends-on`, `priority`, `scheduled`, `deadline`.
- Skip the write when bytes are unchanged, so there is no mtime churn. Still run the post-write reconcile; it is cheap and needs no branch.

## Test scenarios (directly from AC, one-line given/when/then)

**Node (`insert_test.go`)**

| ID | Given / when / then |
|---|---|
| N1 | scalar key / upsert / only value bytes change |
| N2 | bare `key:` / upsert / `key: value` with the space |
| N3 | block-list key / upsert / block collapses to one line, siblings byte-identical, re-parse stable |
| N4 | absent key / upsert / equals `InsertField` output |
| N5 | scalar + block duplicate of a key / upsert / error, `Raw` unchanged |
| N6 | `roundtripCorpus` / parse-serialize after this change, mirroring `TestSetAliases_BlockListCollapsesToFlow` / no regression (guard: passes today, not a red test) |
| N7 | scalar key with siblings / remove / that line gone, all other bytes identical |
| N8 | block-list key / remove / whole block gone, siblings byte-identical |
| N9 | absent key / remove / nil error, `Raw` unchanged |
| N10 | key duplicated as scalar+scalar and scalar+block / remove / every occurrence gone, re-parse shows no key |
| N11 | key is the only frontmatter key / remove / error, `Raw` unchanged |
| N12 | ref-valued `depends-on` line / remove / link gone from parsed `Links` |

**CLI (`todo_edit_test.go`)**

| ID | Given / when / then |
|---|---|
| E1 | no deadline / `edit <id> --deadline 2026-08-01 --json` / exact-bytes insert, JSON `deadline` set |
| E2 | has deadline / edit new date / replaced, key not duplicated |
| E3 | with and without `scheduled` / `--scheduled` / as E1 and E2; `tomorrow` under `pinTodoNow` resolves |
| E4 | with and without priority / `--priority B` / `priority: B` |
| E5 | no depends, target A exists / `--depends A` / exactly `depends-on: "[[A]]"`, one link, no Props entry |
| E6 | depends on A / `--depends B` / link is B only; `rk query` edges show B and not A |
| E6b | `depends-on` as multi-target, unquoted, and block list / `--depends X` / one canonical line, one link |
| E7 | alias ref / `edit <alias> --deadline` / same file edited as by ULID |
| E8 | all four flags in shuffled argv / run / one write, pinned deterministic bytes, body and other frontmatter identical |
| E9 | exotic hand-authored frontmatter (comments, extra keys, order) / edit / diff touches target lines only |
| V1 | `--deadline 2026-02-30` / run / error, file byte-identical |
| V2 | `--priority Z` / run / error, byte-identical |
| V3 | valid `--deadline` + invalid `--priority` / run / error, neither written |
| V4 | no flags / run / "nothing to change", untouched |
| V5 | unknown ref / run / `not found`, non-zero |
| V6 | ref `inbox` / run / not found, inbox unchanged |
| V7 | `--depends <own ULID>` and `<own alias>` / run / rejected, untouched |
| V8 | `--depends <nonexistent>`, and `[[X]]` form / run / rejected, untouched |
| V9 | A depends on B / edit B `--depends A` / rejected |
| V10 | CRLF file by ULID ref / run / refused with `todo edit:` + vj55 message |
| R1 | `repeat: +1w`, `scheduled: D1` / `--scheduled D2` then `todo done` / advances from D2 |
| R2 | recurring todo / `--no-scheduled` / rejected with the repeat reason, untouched |
| R3 | recurring todo / `--deadline` / allowed, `repeat` and `scheduled` untouched |
| D1 | `state: done` / `--deadline` / succeeds, state still done |
| I1 | E1 run twice / second run / exit 0, bytes and result identical |
| C1 | each of the four value flags given `""` / run / empty-value error naming `--no-<field>`, untouched |
| C2 | has deadline / `--no-deadline` / exact line removed, JSON `cleared` lists it |
| C3 | has priority, non-recurring scheduled / `--no-priority --no-scheduled` / both removed in one write |
| C4 | `depends-on` as scalar, multi-target, and block list / `--no-depends` / key gone, no links, `rk query` edges empty, `todo list` shows no dependency |
| C5 | hand-written `depends: "[[A]]"` / `--no-depends` / line removed |
| C6 | field absent / `--no-deadline` run twice / exit 0, bytes untouched, no write |
| C7 | `--deadline X --no-deadline` / run / conflict error, untouched |
| C8 | `--no-deadline --priority B` / run / one write with both effects, deterministic bytes |
| C9 | `state: done` with deadline / `--no-deadline` / succeeds, state unchanged |
| X1 | `--json` / `--ndjson` / pretty / `--quiet` / run / shapes per AC-7; quiet suppresses pretty only |
| X2 | command tree / find `edit` by name in `todoCmd.Commands()` and inspect its `Flags()` / `edit` and all eight flags present (assert on names per the `root_help_test.go:95` note, not rendered help) |
| X3 | after edit / read-only `rk query` on `node_props` / new `deadline` and `priority` visible, no manual reconcile (priority is not in `todoListItem`, so query is the check) |
| X4 | existing tests / run unchanged / `today act D`/`d`/`p` pass |
| X5 | `RECKON_CACHE` set to a path beneath a regular file / plain `--deadline` / exit 0, file written, stderr warning |
| X5b | same setup / `--depends X` / fatal, file untouched |

X5 needs no new seam, because a plain edit touches the index only after the write and `index.OpenWithParser` fails on `MkdirAll` (`index.go:79`). [INFERRED: `LoadWithOverrides` does not create the cache dir; the grep shows no such `MkdirAll`. Confirm when writing the test.]

## Known risks or ambiguities

| Risk | Handling |
|---|---|
| A `depends:` line coexists with a new `depends-on:`, leaving two dependency facts in the file; only `depends-on` is visible to `rk todo list`. | Accepted under D2. |
| Multi-target overwrite drops other targets without warning. | Stated in flag help (OPEN-10). |
| Stricter `--depends` than add blocks forward references and targets not yet synced to this device. | Accepted (OPEN-2). |
| `--depends` for a todo with a stale index. | Pre-write reconcile (D6). |
| CRLF file addressed by alias reports not-found, not vj55, because the walk skips such files (`todo.go:921`). | Existing behavior; V10 uses a ULID ref. |
| Literal node key input (`--depends file:todos/x.md`) passes the lookup and writes a link no resolver matches. | Rare; not guarded. |
| Read-modify-write race with a concurrent hand edit. | Out of scope, same as done/act. |
| `today act` still has the bare-colon `SetField` bug. | Unchanged by this ticket; follow-up. |

**Follow-ups to file**
1. Reconcile `depends:` vs `depends-on` between doc and code.
2. `todo add` validation and `--priority`.
3. Ambiguous-ref detection across done/act/edit.
4. Bare-colon `SetField` spacing in `today act`.
