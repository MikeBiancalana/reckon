# Code Review: reckon-fkgb (`rk todo edit`)

**Verdict:** REQUEST CHANGES

## Summary

The command's core holds up: validate first, one atomic write, truthful no-op handling, `--depends` checks, and byte-identical `today act` / `todo done` output. Two Major findings block merge, and both have small fixes:
- **Finding 1:** the new primitives leave invalid frontmatter behind when a key has continuation lines the parser doesn't model, and the command still reports success.
- **Finding 2:** an empty ref silently edits an arbitrary id-less todo.

The rest are Minor. D1, D2, and the plan's accepted risks are not re-opened.

## Findings

### Major

1. **[Major]** `UpsertField` / `RemoveField` orphan continuation lines and report success — `internal/node/insert.go:65-79`, `:105-112` — The parser gives these shapes only a one-line scalar span or a truncated block span, so the lines below are left behind. Each CLI row exited 0, and the `--json` result listed the field as set or cleared:

   | Fixture under the key | Command | Result |
   |---|---|---|
   | Unindented list `depends-on:\n- "[[tgt]]"\n- "[[other]]"` | `--depends tgt` | `depends-on: "[[tgt]]"\n- "[[tgt]]"\n- "[[other]]"` |
   | Same | `--no-depends` | `- "[[tgt]]"\n- "[[other]]"` remain under `type: todo`, while JSON says `cleared:["depends"]` |
   | Comment inside a block, `  - "[[tgt]]"\n  # keep\n  - "[[other]]"` | `--no-depends` | `  # keep\n  - "[[other]]"` orphaned |
   | Nested map `deadline:\n  soft: 2026-01-01` | `--deadline` or `--no-deadline` | `  soft: ...` left under the new line or the previous key |
   | Folded scalar `deadline: >\n  2026-01-01` | `--deadline 2026-03-03` | `deadline: 2026-03-03\n  2026-01-01` |
   | Blank line inside a block, `  - "[[A]]"\n\n  - "[[B]]"` | `--no-depends` | `  - "[[B]]"` stays in the file |
   | Primitive only: `tags:\nk: 1\n  - orphan` | `RemoveField("k")` | `tags` silently adopts `orphan` as a block item |

   Each result is invalid or changed YAML for Obsidian and other readers, while `rk`'s own re-parse passes, so the re-parse guard can't catch it. This breaks the promise that out-of-subset shapes are "not corrupted" (`internal/node/AGENTS.md:153`). `--no-depends` is the worst case: it reports the dependency cleared while targets stay in the file.

   **Fix:** add one helper in `node`, used by the scalar and block paths of both primitives. Skip blank or whitespace-only lines after the affected span. If the next line inside the fence starts with a space, a tab, or `-`, return an error with no mutation (`key %q has continuation lines; resolve by hand`). Add these seven shapes as `insert_test.go` cases. The existing tests have no such fixture. `SetField` and `SetAliases` splice the same way on these shapes; applying the helper there is a follow-up, not part of this PR.

2. **[Major]** An empty ref edits the first id-less todo — `internal/cli/todo_edit.go:92`, via `internal/cli/todo.go:958` — `n.ULID == ref` is true when both are empty. `rk todo edit "$UNSET" --deadline 2026-05-05 --json` exited 0 and wrote `deadline:` into `todos/idless.md`, reporting `"ref":""`. The plan rejects `--deadline ""` for this same unset-variable hazard (D1), but the ref has no such guard. `origin/main` has the same flaw: `todo done ""` marked the file done, and `today act "" p A` set its priority.

   **Fix:** in `runTodoEditE`, before resolving, reject an empty or whitespace-only ref. Add a test with an id-less fixture. Leave `todo done` and `today act` unchanged here, as the plan requires, and file the shared fix at `todo.go:958` as a follow-up alongside OPEN-8.

### Minor

3. **[Minor]** `RemoveField`'s empty-frontmatter guard misses a body containing `---` — `internal/node/insert.go:124` — `---\nk: 1\n---\nintro\n---\nbar\n` with `RemoveField("k")` returns nil and yields `---\n---\nintro\n---\nbar\n`, so `Body` shrinks to `bar\n` and `intro` becomes frontmatter. The CLI can't reach this because a todo keeps `type`, but the method's documented contract is false. **Fix:** error unless `next.Body == cur.Body`.

4. **[Minor]** `UpsertField` doesn't check that the re-parse matches the value it was given — `internal/node/insert.go:59-82` — Run against the primitive:
   - `"x\nstate: done"` adds a `state` key.
   - `"x\n---\ninjected"` moves text into the body.
   - `"x "` reads back as `x`.
   - `""` over a key followed by indented items makes the key a block list.

   The CLI can't reach these because every value is validated first (`todo_edit.go:183-218`). **Fix:** after re-parsing, error unless `frontmatter[key] == value`.

5. **[Minor]** A `--depends` alias with a backslash passes validation but writes a dangling link — `internal/cli/todo_edit.go:143`, `:212` — With alias `c:\x`, the lookup matches and the command exits 0, but `strconv.Quote` stores `"[[c:\\x]]"`. The parser doesn't unescape, so the edge has `dst` = `c:\\x` and `dst_key` = null. Other characters `strconv.Quote` escapes go through the same path. **Fix:** also reject `\` and non-printable characters at `:212`. `render.go` has the same quoting, but only this command validates the target and then reports success.

6. **[Minor]** Security: a `../` ref writes outside `todos/` — `internal/cli/today.go:459` — `rk todo edit ../outside --deadline 2026-05-05` modified `<vault>/outside.md`, because the fast path joins the ref unchecked and needs only `type: todo`. `todo done` uses the same fast-path code on `origin/main`, but `todo edit` adds a new way to write through it. **Fix:** skip the fast path when `ref != filepath.Base(ref)`, so the walk reports not-found.

7. **[Minor]** The cycle check ignores `depends:` edges while `--no-depends` treats them as dependencies — `internal/cli/todo_edit.go:235` vs `:69` — With B holding `depends: "[[A]]"`, `edit A --depends B` succeeded. **Fix:** use `rel IN ('depends-on','depends')`. This doesn't change D2, because it only affects what is read.

8. **[Minor]** Flag values leak in-process when cobra fails before `RunE` — `internal/cli/todo.go:132` (`Args`), `internal/cli/todo_edit.go:61` — `todo edit --deadline 2026-07-07` with no ref fails argument validation, so the deferred reset never runs. In a scratch test, the next `todo add` in that process wrote `deadline: 2026-07-07`, and `Changed` stayed true on the edit flag set even after `resetCLIFlags`. Production is one command per process, so only tests are exposed, and no current test triggers it. **Fix:** have `resetCLIFlags` in `query_test.go` also call `resetTodoFlags(todoEditCmd)` and `resetTodoFlags(todoAddCmd)`.

9. **[Minor]** The docs say "splices the value only", but a trailing comment on the line is dropped — `internal/node/AGENTS.md:91`, `internal/node/insert.go:65-73` — `deadline: 2026-01-01 # soft` with `--deadline 2026-02-02` loses `# soft`, because the parser's value span includes the comment. `SetField` behaves the same, and `RemoveField`'s docs do state it. **Fix:** document this for `UpsertField`. Preserving the comment isn't required.

10. **[Minor]** A print failure returns unwrapped, after the write, and skips the reconcile — `internal/cli/todo_edit.go:167-171` — A failed stdout write exits non-zero with nothing saying the file changed, and the index stays stale. **Fix:** reconcile before printing, and wrap as `todo edit: print result (file already written): %w`.

11. **[Minor]** Provenance in new test comments — `internal/node/insert_test.go:342` (`N6 guard:` is a plan ID), `internal/cli/todo_edit_test.go:1-7` ("TDD red tests", "compiles before the implementation exists") — **Fix:** reword both to describe behavior. New production comments in `todo_edit.go`, `insert.go`, `todo.go`, and `today.go` are clean. The `insert_test.go:3-11` and `:120` references predate this ticket.

12. **[Minor]** Unreachable branch in `RemoveField` — `internal/node/insert.go:107-110` — A scalar span always ends before the `\n` that precedes the closing fence (`closeAbs - 1`), so `nl < 0` can't happen. "Unterminated line" is also not a real state. **Fix:** delete the branch.

13. **[Minor]** The `Cleared` comment is inaccurate — `internal/cli/todo_edit.go:19-20` — It says "removed field names", but the list also names fields that were already absent. That's what the idempotence rule needs (identical result on re-run). **Fix:** reword to "fields now absent".

## Answers to the Requested Probes

| Area | Result |
|---|---|
| Value with `:` or `#` in a sibling or in the target | No finding. Bytes preserved and the value reads back whole. |
| `key:` in the body or inside another key's block list | No finding. Parsing is bounded by the fence, and indented lines never match a key. |
| Closing fence with no trailing newline | No finding. Set and clear are byte-correct. |
| Prefix collisions (`depends-on-note`, `deadline-note`) | No finding. Keys match exactly. |
| Comment or nested map after a key | Finding 1. A column-0 comment after a block survives. |
| Duplicate keys | No corruption. Scalar+scalar splices the winning line. Scalar+block is refused on set and fully removed on clear. Block+block leaves a scalar+block duplicate, so the next `--depends` is refused and `--no-depends` recovers it. |
| Skip-write and reconcile | No finding. `original` is unaffected by the edits. Same-value and absent-clear runs left the file unchanged. Reconcile failure warns with exit 0. An unwritable dir errors with the file untouched. |
| Partial writes | No finding. All edits happen in memory before one `writeFileAtomic`; a duplicate-key refusal left the file unchanged. |
| Flag reset | All eight flags and their `Changed` state reset after every `RunE` path. Leak only before `RunE` (finding 8). |
| `--depends` self detection | No finding. ULID, alias, id-less stem, id-less alias, and `file:` key all rejected. |
| Cycle query | No finding for `depends-on`. ULID, alias on both sides, and id-less self all rejected. A removed edge no longer blocks, and a new on-disk edge is seen. A stale edge with a preserved mtime does block, but that comes from reconcile's mtime fast path, not this query. |
| Duplicate alias | No finding. The lookup and the resolver picked the same node in the one fixture run. |
| Alias stored as given | No finding (unicode included). Replacing a link updates `edges`. Backslash: finding 5. |
| `verb` change, `today act` / `todo done` | No finding. 100 scenarios byte-identical on `origin/main` and this branch, with file-state hashes also identical. `today act` strings are pinned by `TestTodoEdit_GuardTodayActUnchanged`. |
| Recurrence | No finding. `--no-scheduled` uses `repeat` from the same parse, and is rejected for scalar, block, and `state: done` recurring todos. An empty `repeat:` is allowed, and `todo done` already rejects that shape as malformed (`todo.go:764-766`), so no working rule loses its cursor. `--scheduled` then `todo done` advances from the new date, and `--deadline` is left alone. |
| JSON | No finding beyond 1 and 13. A set to the existing value reports that value and doesn't write. `depends` echoes the ref as given. |

## Open Points from Earlier Phases

| Preflight site | Decision |
|---|---|
| `todo_edit.go:85` | Keep. It matches peers (`todo.go:361`, `:517`, `:683`), and `root.go:85` rejects the flag conflict first. |
| `todo_edit.go:169` | Change, see finding 10. |
| `todo_edit.go:225` | Keep. The callee adds `today: lookup %q` and the caller adds `todo edit: --depends:`. |

Provenance: see finding 11.

## Other Dimensions

- **Correctness:** Findings 1-5, 7.
- **Error handling:** Finding 10.
- **Maintainability:** Findings 9, 11-13.
- **Architecture:** No findings. The extractions in `today.go` are mechanical, the file split matches `todo_add_wizard.go` and `todo_browse.go`, and the code follows the plan with no material deviation.
- **Testing:** Gaps are listed in findings 1 and 2. `-shuffle=on` fails `TestAdd*` and `TestChecklist*` tests on both `origin/main` and this branch, and no `TestTodoEdit*` test failed across four full shuffled runs of this branch, so that isn't from this ticket; follow-up.
- **Performance:** No findings. `--depends` reconciles twice (pre-write check, post-write refresh), which D6 calls for.
- **Security:** SQL uses placeholders. See finding 6.

## Pre-existing, Not Blocking (follow-ups to file)

- Empty ref on `todo done` and `today act` (finding 2).
- `writeFileAtomic` replaces a symlinked todo with a regular file and leaves the link's target stale, while exiting 0. Observed through `todo edit`; the helper is unchanged by this branch.
- Plan-accepted risk confirmed: `--depends file:n2.md` passes the lookup and writes a link with `dst_key` null.

## Experiments Run

Everything ran outside the repo, in the scratchpad, with `RECKON_VAULT`, `RECKON_CACHE`, and `RECKON_DATA_DIR` set to temp dirs.

| Experiment | Coverage |
|---|---|
| CLI byte matrix | 33 frontmatter fixtures through the `rk todo edit` binary, inspected as file output |
| `--depends` and refs | Self, cycle, stale edge, legacy `depends`, alias, unicode, backslash, duplicate alias, non-todo target, `file:` key, empty ref, `../` ref, directory ref, `inbox` |
| Recurrence and validation | `repeat` variants, `todo done` advance, invalid values, conflict flags |
| Failure paths | Cache path under a regular file, read-only `todos/`, `--json --ndjson`, symlink, file mode |
| Parity | Binaries from `origin/main` and HEAD on a 100-scenario matrix: refs (missing, CRLF, conflict marker, inbox, directory, empty, `../`, ULID, alias) across all `today act` keys and `todo done`, bad args, unreadable file; diffed output and file hashes |
| Primitive probes | Temporary Go test in an exported copy of HEAD, for findings 3 and 4 |
| Flag state | Temporary Go test in the same copy, through `RootCmd`, for finding 8 and the reset check |
| Test order | `go test -shuffle=on` on both trees; `-race` for the `TodoEdit` tests on HEAD (passed) |

## Decision

**Required before merge:** findings 1 and 2.

**Should fix in the same pass (one-line or small changes):** findings 3, 4, 5, 6, 7, 10, 11, 12, 13.

**Optional:** findings 8 and 9.
