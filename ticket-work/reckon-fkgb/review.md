# Code Review: reckon-fkgb (`rk todo edit`), feedback loop 1

**Verdict:** REQUEST CHANGES

Reviewed HEAD `4e915a1` against `origin/main`. The first review is at git rev `2361eb9`.

All prior findings are fixed or accepted except that finding 1's class is only partly closed. `checkNoContinuation` still lets three continuation shapes through, and each one silently corrupts the file while the command reports success (new finding 1). The fix is one small helper change, prototyped and passing. Everything else is Minor.

## Prior Findings

| # | Status | Evidence |
|---|---|---|
| 1 | Partly fixed | All seven original shapes are refused with bytes unchanged at CLI or primitive level. Three other shapes still get through: new finding 1. |
| 2 | Fixed | `""` and `"  "` refused, id-less todo unchanged. |
| 3 | Fixed | Body-containing-`---` fixture refused, `Raw` unchanged; control fixture with a remaining key succeeds. |
| 4 | Fixed | Newline-injection and trailing-space values refused on the scalar, block and absent paths. `""` over indented items refused by the continuation check. |
| 5 | Mostly fixed | Backslash and tab refused. Invalid UTF-8 still gets through: new finding 4. |
| 6 | Fixed for `todo edit` | `../outside` reports not found, file unchanged. Side effect on `today act`: new finding 3. |
| 7 | Fixed | Legacy `depends:` cycle refused by ULID and by alias on both sides. |
| 8 | Accepted, not fixed | Test-only in-process flag leak, deliberately left. |
| 9 | Fixed | `internal/node/AGENTS.md:91-93` documents it; the binary drops `# soft` as documented. |
| 10 | Fixed | With stdout at `/dev/full`: exit 1, wrapped message, file written, index already shows the new value. |
| 11 | Fixed | Both cited comments reworded. A new reference was added elsewhere: new finding 5. |
| 12 | Fixed | Branch removed (`insert.go:158-160`). |
| 13 | Fixed | Comment reworded (`todo_edit.go:19-21`); absent-field clear still reports `cleared`. |

## New Findings

1. **[Major]** `checkNoContinuation` misses continuation lines that start at column 0 — `internal/node/insert.go:139-142` — The helper accepts any first non-blank line that doesn't start with space, tab or `-`. Each row below exited 0 and reported the field set or cleared. PyYAML loads every input and rejects every output. Obsidian was not run; [INFERRED] it fails the same way.

   | Fixture under the key | Command | Left in the file |
   |---|---|---|
   | `depends-on:\n  - "[[tgt]]"\n# keep\n  - "[[other]]"` | `--no-depends` | `# keep\n  - "[[other]]"`, with `"cleared":["depends"]` |
   | Same | `--depends tgt` | `depends-on: "[[tgt]]"\n# keep\n  - "[[other]]"` |
   | `depends-on:\n# keep\n- "[[tgt]]"` | `--no-depends` | `# keep\n- "[[tgt]]"` |
   | `deadline:\n# c\n  soft: 2026-01-01` | `--deadline` or `--no-deadline` | `# c\n  soft: 2026-01-01` |
   | `depends-on: [\n"[[tgt]]",\n"[[other]]"\n]` | `--no-depends` or `--depends tgt` | `"[[tgt]]",\n"[[other]]"\n]` |
   | `depends-on: ["[[tgt]]",\n"[[other]]"]` | `--no-depends` | `"[[other]]"]` |
   | `deadline: "2026-01-01\nsoft"` | `--deadline` or `--no-deadline` | `soft"` |
   | `priority: 'A\nB'` | `--no-priority` | `B'` |

   This also makes `AGENTS.md:107-108` and the `UpsertField` doc comment (`insert.go:57-60`) untrue, since both say a comment inside a list is refused. `--no-depends` is the worst case, as in the first review: it reports the dependency cleared while a target stays in the file.

   **Fix:** invert the default in the helper:
   - Skip a blank line or a comment line at any indent.
   - Return nil for the exact `---` line.
   - Return nil for a column-0 line that doesn't start with `-` and either matches `fmScalarRe`, contains `: `, or ends in `:`.
   - Refuse everything else.

   Add the table's shapes to `TestUpsertRemoveField_RefuseContinuationShapes`. I prototyped this in an exported copy: `internal/node` and `internal/cli` tests pass, all rows above are refused with bytes unchanged, and every legitimate shape in the regression table below still succeeds except the `...` line, which it refuses.

   All three sibling tests are needed. `fmScalarRe` alone would refuse `Due Date: 1` and `my.key: 1`. Without it, `deadline:2026-08-01` is refused, and `SetField` writes that form on a bare `deadline:` line (plan, "New finding"). A prototype without it did refuse that sibling and `k:\tv`, both of which HEAD accepts. A plain "contains `:`" test is too loose, because it would pass `"[[a:b]]",`, which the rule above refuses. [OPEN] A multi-line quoted scalar whose continuation line itself contains `: ` still passes; accept it, since only a real YAML parser could tell.

2. **[Minor]** An indented comment after a key is wrongly refused — `internal/node/insert.go:139` — `deadline: 2026-01-01\n  # note\nk: v` with `--deadline` or `--no-deadline`, and the same fixture with the comment last before the fence, fail with `has continuation lines`. PyYAML reads this as a plain sibling and comment, so the message is wrong. Bytes stay unchanged, so this is an annoyance rather than a risk. **Fix:** new finding 1's comment skip covers it (verified in the prototype).

3. **[Minor]** `today act` changed for path refs, and now disagrees with `todo done` — `internal/cli/today.go:438-441`, `:466`, `internal/cli/todo.go:715` — Finding 6's guard sits in the shared resolver, but `doneDurableTodo` keeps its own unguarded copy.

   | Ref | `today act` on `origin/main` | `today act` on HEAD | `todo done` on HEAD |
   |---|---|---|---|
   | `sub/x` (`todos/sub/x.md`) | Edits the file | Not found | Marks done |
   | `../outside` | Edits the file | Not found | Marks done |

   The comment at `today.go:438-441` still says the two "never disagree on which file a ref names", which is now false, and the plan's "`today act` keeps its behavior" no longer holds for these refs. The traversal write also stays reachable through `todo done`. **Fix:** keep the guard, correct the comment, state the `today act` change in the PR description, and add the `todo done` guard to the follow-up with its empty-ref fix (below). Routing `doneDurableTodo` through `resolveDurableTodoRef` would close both, but the plan keeps `todo done` out of this PR.

4. **[Minor]** A `--depends` alias with invalid UTF-8 still writes a dangling link — `internal/cli/todo_edit.go:217` — An invalid byte decodes to U+FFFD, which `unicode.IsPrint` accepts, but `strconv.Quote` escapes it. Alias `a\xffb` exited 0, wrote `depends-on: "[[a\xffb]]"` with a literal backslash, and the edge has `dst_key` null. Very unlikely in practice. **Fix:** replace the non-printable test with ``strconv.Quote(v) != `"`+v+`"` ``, keeping `[]|#` in `ContainsAny`. That one comparison covers backslash, double quote, non-printable characters and invalid UTF-8, and can't drift from the quoting it guards.

5. **[Minor]** New provenance in a test comment — `internal/cli/todo_edit_test.go:916` — The section header `// Review regressions` refers to this review, which is what prior finding 11 removed elsewhere. **Fix:** name the behavior, for example `// Refusals: unmodeled shapes, refs, depends targets`.

6. **[Minor]** A key the parser doesn't match looks absent — `internal/node/node.go:103`, `internal/cli/todo_edit.go:139` — `"deadline": 2026-01-01` or `deadline : 2026-01-01` with `--no-deadline` exits 0, reports `cleared`, and leaves the line. `"deadline": ...` with `--deadline` inserts a second `deadline` key. No file gets corrupted on a clear, but the JSON is false. Rare, because neither Obsidian nor `rk` writes these forms [INFERRED]. `today act D` reaches the same insert through `setOrInsertField` (read, not run). **Fix:** follow-up; not needed for this PR.

## Regression Checks

| Question | Result |
|---|---|
| Following column-0 comment, sibling after a blank line, last key before the fence, blank line before the fence, sibling with an unindented list of its own, sibling `Due Date:`, `my.key:`, `deadline:2026-08-01` or `k:\tv` | Accepted, correct bytes. |
| Following indented comment | Wrongly refused: finding 2. |
| `---x` line | Not reachable. The parser takes the first `\n---` as the fence and treats a suffixed one as no frontmatter, so the file has no todo type and reports not found. |
| `...` as a fence | Not a fence in the parser. As a stray line it passes today and is refused after the finding 1 fix; PyYAML rejects `...` followed by a key, so refusing is fine. |
| CRLF | CLI refuses earlier. At primitive level the helper handles CRLF lines and fence correctly. |
| Failure message | Clear and names flag and key: `todo edit: --no-depends: RemoveField: key "depends-on" has continuation lines; resolve by hand`. Only finding 2 mislabels. |
| Error leaves the file untouched | Yes. Clear refused alongside sets, and upsert refused after an earlier clear, both left bytes unchanged; mtime and directory listing checked on the first. |
| Slash alias through the walk | Resolves for `proj/foo`, `../esc`, `a/`, `.`, `..` on `todo edit`, and `proj/foo` on `today act`. |
| `today act` / `todo done` parity for non-path refs | 259 scenarios, output and `todos/` hashes identical. The single diff is a random `did_entry_id` ULID. |
| Read-back on CLI values | No false failure. Absent, double-quoted, unquoted, single-quoted, flow, block, bare `key:`, bare with trailing spaces, trailing spaces after the value, and duplicate scalars all succeed. `frontmatter[key]` keeps the quotes as written, so `"[[x]]"` compares equal. Date, shorthand, ULID, and alias values with space, `:`, `'`, `/`, `^`, accents and CJK pass and resolve to a `dst_key`. |
| Same-value set | Exit 0, no write. |
| Quality gates | `gofmt -l` empty, `go vet ./internal/...` clean, `go test -count=1 ./internal/node ./internal/cli` pass, no worktree changes from the tests. |

Anything else in the fix delta: no finding. Reconcile before print, `lineAfter`, the `commitUpsert` rename and the `RemoveField` loop all behaved correctly, including removing a scalar and block duplicate of one key.

## Pre-existing, Not Blocking (follow-ups to file)

- Empty ref on `todo done` and `today act` (prior finding 2). Add the `todo done` path guard here (new finding 3).
- `SetField` and `SetAliases` splice the same way on unmodeled continuation shapes (prior finding 1).
- `writeFileAtomic` replaces a symlinked todo with a regular file and leaves the link's target stale, while exiting 0.
- `--depends file:n2.md` passes the lookup and writes a link with `dst_key` null (plan-accepted risk).
- `-shuffle=on` fails `TestAdd*` and `TestChecklist*` on both `origin/main` and this branch (not re-run this loop).
- Keys the parser doesn't match (new finding 6).

## Experiments Run

All ran outside the repo with `RECKON_VAULT`, `RECKON_CACHE` and `RECKON_DATA_DIR` in fresh temp dirs, using binaries built from HEAD and `origin/main`.

| Experiment | Coverage |
|---|---|
| Prior findings | CLI reproductions of 1, 2, 5, 6, 7, 9, 10, 13; temporary Go test in an exported copy for 1 (primitive row), 3, 4; code read for 11, 12 |
| Continuation matrix | 18 legitimate-shape runs and 12 continuation-shape runs, with exit code, byte hash and resulting frontmatter |
| Atomicity | Refusal with several flags, in both clear-first and later-upsert order |
| Read-back | 13 existing-shape runs, 9 alias forms with the resulting edge, invalid UTF-8 alias, primitive probes for quoted, date, ULID and flow values, plus CRLF and duplicates |
| Refs | Slash aliases, `.`, `..`, `a/`, nested and `../` refs on `today act` (both binaries) and `todo done` (HEAD) |
| Parity | 17 refs (ULID, alias, slash alias, id-less, recurring, CRLF, conflict marker, note, inbox, directory, unreadable, missing, empty, space, `.`, `..`, directory name) across the `today act` keys, `--json` and pretty `todo done`, and bad args: 259 scenarios. Timestamps in log lines and temp paths normalized; `log/` excluded from hashes |
| YAML check | PyYAML on the input and output of every finding 1 row, the indented comment, and a `...` line |
| Fix prototype | Finding 1's rule applied in an exported copy; `internal/node` and `internal/cli` tests, the continuation matrix re-run, and no-space siblings compared across HEAD, a prototype without `fmScalarRe`, and the final rule |

Not run: Obsidian, full `go test ./...`, integration tests (preflight covered those at `e85c7d0`; later commits are docs only).

## Decision

**Required before merge:** finding 1.

**Should fix in the same pass (small changes):** findings 3, 4, 5. Finding 2 comes with finding 1.

**Follow-up:** finding 6 and the list above.
