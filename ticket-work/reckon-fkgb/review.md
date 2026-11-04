# Code Review: reckon-fkgb (`rk todo edit`), feedback loop 2

**Verdict:** APPROVE WITH CHANGES

Reviewed HEAD `89d5ea9` against `origin/main`. Earlier reviews are at git revs `2361eb9` (first) and `1ff8aa2` (second).

Findings 1-5 of the second review are fixed. The remaining findings are Minor and can go to follow-ups, so nothing blocks merge and no further review loop is needed. One item is due when the PR is opened: state the `today act` change in its description (prior 3).

## Prior Findings (second review)

| # | Status | Evidence |
|---|---|---|
| 1 | Fixed | All 22 refusal runs (second review's table, first review's shapes, stray `...`) exit 1 with bytes unchanged. |
| 2 | Fixed | `deadline: 2026-01-01\n  # note\nk: v`, and the same comment last before the fence, succeed with correct bytes on set and clear. |
| 3 | Fixed in code; PR text pending | The `today.go:438-441` comment now matches behavior. `sub/x` and `../outside`: `today act` and `todo edit` report not found, `todo done` writes the file. |
| 4 | Fixed | `a\xffb`, `\`, `"`, tab, newline, NBSP and U+2028 refused, bytes unchanged. Unquoted aliases `日本語`, `a'b`, `a:b`, `proj/foo`, `a^b`, `x y` and `naïve2` still write and resolve to a `dst_key`. |
| 5 | Fixed | `todo_edit_test.go:916` now names the behavior. |
| 6 | Accepted follow-up | Not re-tested; code path unchanged. |
| First review's 8 | Accepted follow-up | Test-only flag leak, unchanged. |

## New Findings

1. **[Minor]** The sibling heuristic accepts a column-0 continuation that looks like a key — `internal/node/insert.go:132-155` — This is the fix author's [OPEN]. It is wider than stated: it also covers multi-line flow collections and continuation lines with `foo:bar`, a trailing `:`, or a comment containing `: `. The edit exits 0 and reports success:

   | Fixture under the key | Command | Left in the file | PyYAML on the output |
   |---|---|---|---|
   | `deadline: "2026-01-01\nk: v"` | `--no-deadline` or `--deadline` | `k: v"` | Valid, new key `k` |
   | `priority: 'A\nnote: B'` | `--no-priority` or `--priority B` | `note: B'` | Valid, new key `note` |
   | `deadline: {soft: 2026-01-01,\nhard: 2026-02-02}` | `--no-deadline` | `hard: 2026-02-02}` | Valid, new key `hard` |
   | `deadline: "x\nfoo:bar"` | `--no-deadline` | `foo:bar"` | Invalid |
   | `depends-on: [\n"[[a: b]]",\n"[[other]]"\n]` | `--no-depends` or `--depends tgt` | `"[[a: b]]",\n"[[other]]"\n]` | Invalid |
   | `depends-on: [\n"[[tgt]]", # why: blocked\n"[[other]]"\n]` | `--no-depends` | `"[[tgt]]", # why: blocked\n...` | Invalid |

   **Seriousness: silent, but not realistic enough to block.**
   - **Silent:** yes. The exit code is 0, and a YAML reader sees either a stray key or a broken file.
   - **Rarity:** it needs a hand-written multi-line value on one of five keys that hold dates, a priority letter or wikilinks, with the continuation at column 0 and key-shaped. The indented form people actually write is refused (`depends-on: [\n  "[[tgt]]",\n  ...`), and no `rk` writer produces any of these shapes [INFERRED; Obsidian not run].
   - **Impact:** the requested field is correct wherever the output still parses, so the reported value is true. `rk` already misreads these files before any edit: the index holds `deadline` = `"2026-01-01` and `k` = `v"`. The vault is git-tracked, so a bad edit can be reverted.

   **Cheap detection exists.** Add an `unterminatedValue` check on the key's own scalar span: a value starting with `"`, `'`, `[` or `{` that never closes is refused, using the existing error. Prototyped in an exported copy at about 25 lines plus one call in each primitive:
   - 14 of the 16 accepted runs above and in the matrix are now refused with bytes unchanged. The other 2 are the anchor row below.
   - All 64 legitimate-shape runs keep their results, and `internal/node` and `internal/cli` tests pass.
   - One further legitimate shape is now refused, safely: `depends-on: [it's, b]`.
   - Known gap, run on the prototype: `priority: 'it''s\nnote: B'` still passes, because the scan stops at the doubled quote.

   **Fix:** follow-up, P3. Not worth adding to this PR at this stage.

2. **[Minor]** Docs understate the gap — `internal/node/insert.go:129-131`, `internal/node/AGENTS.md:106-110`, `internal/node/insert.go:59` — The helper comment names only quoted scalars, though flow collections share the gap. `AGENTS.md` says both primitives refuse lines that still belong to the key, with no exception listed, so a reader would trust it for the rows above. `insert.go:59` also breaks the comment's wrapping. **Fix:** name both shapes in the comment and the guide, and rewrap. If finding 1's check lands, narrow the wording to what remains.

## Regression Checks

| Question | Result |
|---|---|
| Legitimate shapes refused? | None in the 64 runs: sibling after a blank or comment, last key before the fence, unindented or block list on a sibling, nested sibling, `Due Date:`, `my.key:`, `"my key":`, `"quoted":`, `état:`, `日本語:`, `<<:`, URL value, multi-line sibling values, bare `key:`, quoted value, trailing comment, flow, block and both dependency keys. Set and clear for each. |
| Accepted edits valid and truthful? | 62 of 64 pass PyYAML with file value = JSON value, `cleared` listed, no new, lost or changed sibling. The other 2 are `k:\tv`, which PyYAML rejects on the input too, and whose line is unchanged. |
| Unsafe shapes accepted? | Finding 1, plus the anchor row below. Everything without a key-shaped continuation is refused. |
| Anchors | `deadline: &d 2026-01-01\nk: *d` with set or clear leaves `k: *d` undefined, so the output is invalid YAML. Theoretical on these fields, no cheap check; no action. |
| `-foo: 1`, `my.key:\tv` as next line | Refused, bytes unchanged (`insert.go:143`, `:153-155`). `-foo: 1` is valid YAML, so this refusal is unnecessary, but too rare and too harmless for a finding. PyYAML rejects the `my.key:\tv` input. |
| No-space sibling `due:2026-08-01` | Accepted, sibling line unchanged (CLI run and `TestUpsertRemoveField_AcceptsSiblingShapes/no_space_after_colon`). PyYAML rejects it on the input too. |
| Atomicity | `--no-depends` where `depends-on` clears and `depends` refuses, and `--no-priority --deadline` where the set refuses: exit 1, bytes unchanged. |
| Rest of the delta | `strconv.Quote` comparison behaves as intended (prior 4). New tests are sound. No other production change. |
| Quality gates | `gofmt -l internal` empty, `go vet ./internal/...` clean, `go test -count=1 ./...` and `-tags integration ./tests/` pass, worktree clean afterwards. |

## Consolidated Follow-ups

| # | Item | Origin |
|---|---|---|
| 1 | Unterminated quoted or flow value on the edited key: new finding 1's check, with the wording fix in new finding 2. | This review |
| 2 | Empty ref on `todo done` and `today act` edits the first id-less todo. | First review, finding 2 |
| 3 | `todo done` has no path guard: `../outside` still writes outside `todos/` (`todo.go:715`, re-verified). Fix with item 2; routing through `resolveDurableTodoRef` closes both. | Second review, finding 3 |
| 4 | `SetField` and `SetAliases` splice the same way on unmodeled continuation shapes. | First review, finding 1 |
| 5 | `writeFileAtomic` replaces a symlinked todo with a regular file and leaves the link's target stale, while exiting 0. | First review |
| 6 | `--depends file:n2.md` passes the lookup and writes a link with `dst_key` null (plan-accepted risk). | First review |
| 7 | `-shuffle=on` fails `TestAdd*` and `TestChecklist*` on both `origin/main` and this branch (not re-run this loop). | First review |
| 8 | Flag values leak in-process when cobra fails before `RunE`; test-only. Fix in `resetCLIFlags`. | First review, finding 8 |
| 9 | Keys the parser doesn't match (`"deadline":`, `deadline :`) look absent: a clear reports `cleared` and leaves the line, and a set inserts a second key. | Second review, finding 6 |
| 10 | A quoted alias keeps its quotes in the index (`aliases: ["a b"]` stores `"a b"`, `node.go:484`), so `--depends "a b"` reports not found. It refuses rather than writing wrong data, and the ULID works. Pre-existing; `node.go` is unchanged here. | This review |

The plan's own follow-ups (`plan.md:218-222`) still stand.

## Experiments Run

All ran outside the repo with `RECKON_VAULT`, `RECKON_CACHE` and `RECKON_DATA_DIR` in fresh temp dirs, using a binary built from HEAD to a temp path.

| Experiment | Coverage |
|---|---|
| Continuation matrix | 104 CLI runs: 22 must-refuse, 64 legitimate, 18 [OPEN]-family (16 accepted, 2 refused). Each checked exit code, bytes, PyYAML on input and output, file value against the JSON result, and new, lost or changed sibling keys. |
| Extra shapes | `-foo: 1`, `my.key:\tv`, `due:2026-08-01`, `priority: 'it''s\nnote: B'` on HEAD and the prototype |
| `--depends` characters | 10 refused byte strings; 7 unquoted aliases with the resulting edge; quoted alias forms |
| Refs | `sub/x` and `../outside` on `today act`, `todo done`, `todo edit` |
| Atomicity | Two multi-field refusals |
| Parser view | `rk query` on the multi-line quoted fixture before any edit |
| Fix prototype | `unterminatedValue` in an exported copy: `internal/node` and `internal/cli` tests, full matrix diffed against HEAD, 5 extra quoted and flow shapes |
| Gates | `gofmt`, `go vet ./internal/...`, `go test ./...`, integration tests |

Not run: Obsidian. Not re-run: `-shuffle=on`, and the `today act` / `todo done` parity matrix (the delta only changes a comment in `today.go`).

## Decision

**Required before merge:** none.

**At PR creation:** state in the description that `today act` no longer resolves path-shaped refs as direct paths (prior 3).

**Optional, one comment edit:** new finding 2.

**Follow-up:** the list above.
