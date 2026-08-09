# Acceptance Criteria: reckon-fnqs.5 — Multi-line body entry paths for `rk todo add` / `rk add`

## Grounding facts

- **Both commands are in scope for the three entry mechanisms** (`-m`
  repeatable, `--edit`, stdin via `-`). Two independent sources say so: the
  ticket title and fnqs.3's own acceptance-criteria.md ("Multi-line body
  entry via `rk todo add`/`rk add` is fnqs.5's job",
  `ticket-work/reckon-fnqs.3/acceptance-criteria.md:7,275`). The ticket's
  Done-when clause only exercises `rk todo add` because that's the one
  surface with subject-line semantics to verify — it is a checkpoint, not a
  scope statement.
- **Subject semantics are todo-only.** `internal/index/AGENTS.md:38-43` and
  `internal/node/AGENTS.md:22-26` (post-fnqs.3) both confirm `nodes.title`
  is populated for `type: todo` only. `rk add` writes `log-entry`/`log-day`
  nodes, which have no title column and no "subject must be non-empty"
  requirement. For `rk add`, the three entry mechanisms only assemble the
  body text; validation stays exactly what it is today — non-empty overall
  body (`add.go:91-93`) and the `embeddedHeaderRe` "no `^## `" guard
  (`add.go:69,94`), both now applied to the *assembled* multi-paragraph body
  instead of the naive argv-join.
- **Current signatures (both commands identical shape):** `todoAddCmd` /
  `addCmd` are `Use: "add <text...>"`, `Args: cobra.MinimumNArgs(1)`
  (`todo.go:88-94`, `add.go:35-42`); `RunE` computes
  `body := strings.TrimSpace(strings.Join(args, " "))` then errors if empty
  (`todo.go:278-281`, `add.go:90-93`). **`MinimumNArgs(1)` must change** —
  `-m`-only / `--edit`-only / stdin-only invocations have zero positional
  args, so a fixed positional-arity Args validator rejects them before RunE
  ever runs. Replace with `cobra.ArbitraryArgs` (or equivalent) and move the
  "at least one source supplied" check into RunE, alongside the existing
  empty-body check.
- **Ephemeral is structurally single-line.** `todo.go:387`'s comment: the
  inbox container's checkbox-line-per-item model (`splitChecklistLines`,
  `todo.go:977`) assumes no embedded `\n` inside an item's text; nothing
  today guards against one. The new multi-line paths must not be reachable
  for `--ephemeral`.
- **Join semantics are stated by the ticket, not inferred:** "each `-m`
  becomes a paragraph" + "mirroring git" = git's own `-m`/`-m` behavior,
  paragraphs joined with a blank line (`"\n\n"`). Precise algorithm for the
  `-m` path: (1) each message individually `strings.TrimSpace`-trimmed; (2)
  the **subject check** (AC5) validates message `[0]` specifically, before
  joining — this is what "first is the subject" means (the first *flag*,
  not whatever survives trimming); (3) `strings.Join(messages, "\n\n")`;
  (4) `strings.TrimSpace` the whole joined result once more, to absorb a
  wholly-blank *trailing* paragraph (mirrors git's own leading/trailing
  blank-run cleanup) — this only touches the joined string's outer edges,
  never interior blank-paragraph runs from a *non-trailing* empty `-m` (see
  E3/E3b).
- **Write recipe (unchanged plumbing), and confirmed byte-verbatim.**
  `(*Node).Render()` (`render.go:120-`) does `out.WriteString(n.Body)` —
  zero normalization, no blank-line collapsing, nothing stripped. `NewNode`
  (`render.go:120`) just stores `body` as-is. `Parse`/`Serialize` are
  independently confirmed byte-preserving. So whatever string the assembly
  logic below produces is *exactly* what lands in the file, verified by
  reading the source, not by inference. Two different trailing-newline
  conventions already exist at the two call sites and are **unchanged** by
  this ticket: `addDurableTodo` explicitly appends `+"\n"` before calling
  `NewNode` (`todo.go:347`, because `Render` won't add one); `RenderLogEntry`
  (`logparser.go:182-184`) appends its own trailing `"\n"` after `body`, so
  `appendLogEntry` passes `body` bare (`add.go:123`, no `+"\n"`). The new
  assembly logic should return the fully-trimmed multi-paragraph text with
  **no trailing newline of its own** and let each existing call site's
  convention produce the file's final `\n` — do not invent a third
  convention.
- **No stdin/editor precedent exists in this codebase.** No caller uses
  `cmd.InOrStdin()`, `os.Stdin`, or spawns `$EDITOR` today. Two new seams are
  needed, both testable, mirroring the existing seam pattern (`mintTodoULID`,
  `todoNow` — `todo.go:25,33`; `isInteractive` — `interactive.go:16`):
  - stdin: read via `cmd.InOrStdin()`, not raw `os.Stdin` — `runTodo`/a new
    `runAdd` test helper can then inject via `RootCmd.SetIn(...)`
    (`todo_test.go:111-120` has no `SetIn` call today; it must gain one, or
    tests need a stdin-capable variant).
  - editor: a package-level func var (e.g. `runEditor func(path string)
    error`) that shells out to `$EDITOR <tmpfile>` in production and is
    stubbed in tests to write canned content / return a canned error instead
    of actually spawning a process.
- **`-m` is repeatable text, not comma-split:** use `StringArrayVar` (not
  `StringSliceVar`), matching the existing repeatable-flag precedent
  (`noteTagFlag`/`noteAliasFlag`, `note_v1.go:125-126`) — a message
  containing a literal comma must not be split.

## 1. Explicit acceptance criteria

1. **AC1 (`-m`, `rk todo add`).** `rk todo add -m 'S' -m 'p1' -m 'p2'`
   creates `todos/<ULID>.md` whose body is `S\n\np1\n\np2\n` (per-message
   trim, `"\n\n"` join, `addDurableTodo`'s existing `body+"\n"` convention
   supplies the one trailing newline — see Grounding facts). `rk todo list`
   renders only `S`. A single `-m` with no second one is equally valid and
   is the Done-when's simplest case: `rk todo add -m 'Buy milk'` produces
   body `"Buy milk\n"`, no second paragraph required.
2. **AC2 (`-m`, `rk add`).** `rk add -m 'S' -m 'p1'` appends a log entry
   whose body is `S\n\np1` (no title/subject semantics — the log entry
   body is just the joined text, `RenderLogEntry` handles final
   formatting as today).
3. **AC3 (`--edit`, both commands).** `--edit` opens `$EDITOR` on an empty
   scratch file (no boilerplate/comment lines, no pre-seeded text; a `#`
   line is legal body content — see Implicit Requirements). The saved
   file's content, `strings.TrimSpace`-trimmed as one whole buffer, becomes
   `body` (plus the trailing `\n` convention). For `rk todo add`, the first
   line of the trimmed buffer must be non-blank (the subject rule below).
4. **AC4 (stdin `-`, both commands).** When positional args are exactly
   `["-"]`, body is read to EOF from `cmd.InOrStdin()`, then
   `strings.TrimSpace`-trimmed as one whole buffer, same as AC3.
5. **AC5 (subject non-empty, `rk todo add` only).** Two mechanically
   different checks depending on entry path, both enforcing the same rule
   ("Subject line must be non-empty"):
   - `-m` path: message `[0]` (individually trimmed, *before* joining)
     must be non-empty. `rk todo add -m '' -m 'detail'` errors even though
     the overall assembled body is non-empty — the check is anchored to the
     first flag specifically, not "whatever ends up first after trimming."
   - `--edit`/stdin path: the first non-blank line of the whole
     `strings.TrimSpace`-trimmed buffer (skip leading blank lines, trim)
     must be non-empty — there's no discrete "first flag" to anchor to, so
     this mirrors the read-side title derivation exactly
     (`internal/index/AGENTS.md:38-43`).
   Both are *new* checks distinct from the pre-existing "body == ''" guard
   (`todo.go:279-281`).
6. **AC6 (existing positional-arg path unaffected).** `rk todo add foo bar`
   / `rk add foo bar` with none of `-m`/`--edit`/stdin-`-` used behaves
   exactly as today (space-joined argv, single-line-shaped body, no new
   validation beyond what already exists).

## 2. Implicit requirements

- **Per-message trim, not per-buffer, for `-m`:** each `-m` value is
  `strings.TrimSpace`-trimmed independently before joining, then the joined
  result gets one more whole-string `strings.TrimSpace` pass (Grounding
  facts). Net effect on a non-first empty `-m`: if it's the **trailing**
  message, it's fully absorbed — indistinguishable from not having passed
  it at all (E3). If it's an **interior** message (something non-empty
  follows), its blank-line contribution survives as an untouched interior
  blank-paragraph run — `-m 'S' -m '' -m 'p2'` yields `"S\n\n\n\np2"`, not
  an error (E3b). Only message `[0]`'s own emptiness is a hard error (AC5).
- **`--edit`/stdin trim the whole buffer once**, not line-by-line — this
  differs from `-m`'s per-message trim because there is exactly one
  "message" (freeform typed/piped text), and leading/trailing whitespace
  around the whole block is boilerplate, not content. Internal blank-line
  structure the user typed is preserved verbatim.
- **`--edit` must not strip comment lines.** Git's scratch buffer convention
  (`#`-prefixed lines stripped) does not transfer here: this ticket's
  bodies are markdown, and a body legitimately starting with `# Heading` is
  common (`# Inbox` is the ephemeral container's own convention,
  `todo.go:399`). Stripping `#` lines would corrupt real content. No
  boilerplate is written into the scratch file for the same reason — there
  is nothing to strip back out.
- **Entry paths are mutually exclusive; combining any two errors.**
  Positional text (non-`-`) + `-m`, positional text + `--edit`, stdin-`-` +
  `-m`, stdin-`-` + `--edit`, and `-m` + `--edit` are all rejected with a
  clear "choose one" error. This is a deliberate simplification, not full
  git parity — git's `git commit -m "x" --edit` augments the `-m` message
  in the editor rather than erroring; that augmentation is out of scope
  here (flagged as a possible future enhancement, not required).
- **`$EDITOR` unset → error, no fallback.** No `vi`/`nano` default and no
  `$VISUAL` fallback chain (git has one; not required here) — error
  immediately with something like `"--edit requires $EDITOR to be set"`,
  before attempting to create a temp file or spawn anything.
- **`--ephemeral` blanket-rejects all three new paths.** `--ephemeral` +
  any of (`-m` used at all, `--edit`, stdin-`-`) errors — one rule, no
  single-`-m`-if-no-newline carve-out. (A carve-out allowing exactly one
  `-m` with no embedded newline is a defensible future relaxation but adds
  a special case and a newline-guard test for marginal ergonomic value;
  not required.)
- **`rk add`'s `embeddedHeaderRe` guard applies to the assembled body.**
  Today it runs once against the argv-joined `body` (`add.go:94`); after
  this change it must run against whatever `body` the new paths produce
  (joined `-m` paragraphs, or the trimmed edit/stdin buffer) — same call
  site, same regex, just fed a differently-assembled string.
- **No truncation, no length cap** on the subject or any paragraph —
  consistent with fnqs.3's established "first line, not first N
  characters" convention (`ticket-work/reckon-fnqs.3/acceptance-criteria.md:127-132`).
- **CRLF in `--edit`/stdin input:** not specially handled — whatever bytes
  come back are trimmed and passed through the existing
  `NewNode`/`Render`/`Parse` recipe like any other body. (`rk add`'s
  *append*-to-existing-day-file path independently rejects CRLF in the
  *day file itself*, `add.go:242-244` — unrelated to this ticket, unchanged.)

## 3. Edge cases

| # | Case | Command(s) | Expected behavior |
|---|---|---|---|
| E1 | Empty subject: `-m ''` alone, or `-m '' -m 'x'` | `todo add` | Error (AC5); no file written |
| E2 | Whitespace-only subject: `-m '   '` | `todo add` | Error (trims to `""`, same as E1) |
| E3 | `-m` with empty string, **trailing**: `-m 'S' -m ''` | `todo add`, `add` | Allowed; the trailing empty paragraph is fully absorbed by the final whole-string trim — body is `"S\n"` (`todo add`) / `"S"` (`add`), byte-identical to a single `-m 'S'` |
| E3b | `-m` with empty string, **interior** (non-first, non-last): `-m 'S' -m '' -m 'p2'` | `todo add`, `add` | Allowed; not absorbed (only the joined string's outer edges are trimmed) — body is `"S\n\n\n\np2\n"` (`todo add`), an untouched interior blank-paragraph run |
| E3c | Single `-m`, no second one: `-m 'Buy milk'` | `todo add` | Valid, subject-only body `"Buy milk\n"` — answers "no body args at all beyond one `-m`": still valid, single-subject-line-only (AC1) |
| E4 | `--edit`, user saves an empty (or whitespace-only) file | both | Same as E1 — trimmed buffer is `""`, subject check fails on `todo add`; on `rk add` the pre-existing empty-body check fires instead |
| E5 | `--edit`, user aborts without saving (or editor exits non-zero) | both | Detect nonzero `$EDITOR` exit status and abort with a distinct error ("editor exited with an error") before reading the temp file; if the editor exits 0 but the file is unchanged/empty, falls through to E4's empty-buffer error — no file written either way |
| E6 | `--edit`, `$EDITOR` unset | both | Error immediately, no temp file created, no process spawned |
| E7 | stdin `-` combined with `-m` | both | Error ("choose one entry method") |
| E8 | stdin `-` combined with `--edit` | both | Error (same family as E7) |
| E9 | `-m` combined with `--edit` | both | Error (see Implicit Requirements — diverges from git on purpose) |
| E10 | Very long subject line (e.g. 5000 chars) | `todo add` | Accepted verbatim, no truncation |
| E11 | Positional text arg + `-m` together | both | Error ("choose one entry method") |
| E12 | Positional args `["-", "extra"]` (2+ args, one is `-`) | both | Stdin sentinel does **not** trigger (only fires when args are exactly `["-"]`); treated as ordinary positional text, `"- extra"` joined as today |
| E13 | `--ephemeral` + `-m` (even just one) | `todo add` | Error (blanket rejection, Implicit Requirements) |
| E14 | `--ephemeral` + `--edit` | `todo add` | Error (same rule as E13) |
| E15 | `--ephemeral` + stdin `-` | `todo add` | Error (same rule as E13) |
| E16 | No body args at all: `rk todo add` / `rk add` with zero positional args and none of `-m`/`--edit`/stdin set | both | Error — this is the post-`MinimumNArgs` runtime check (Grounding facts); same family as today's empty-body error, still a required error, not newly-permitted |
| E17 | `-m` value containing an embedded `\n` (e.g. `-m $'line1\nline2'`) | `todo add`, `add` | Allowed for durable/log paths — that message's internal newline is preserved verbatim as part of its paragraph; only relevant restriction is the `--ephemeral` blanket rejection (E13) |
| E18 | `rk add`'s assembled body contains a line starting with `## ` (across any `-m`/`--edit`/stdin path) | `add` | Error via existing `embeddedHeaderRe` guard, now applied post-assembly (Implicit Requirements) |
| E19 | CRLF content pasted into `--edit` or piped via stdin `-` | both | No special rejection at assembly time; passes through `TrimSpace` (which strips a trailing `\r`) same as any string; not equivalent to `add.go`'s day-file CRLF guard, which is unrelated and unaffected |

## 4. Test scenarios (Given/When/Then)

Named for direct translation to Go test functions (`todo_test.go` /
`add_test.go`), matching the existing `TestTodoAdd_XxxYyy` convention
(`todo_test.go:200` etc.).

- **`TestTodoAdd_MessageFlag_JoinsParagraphs`** — Given `rk todo add -m
  'Ship it.' -m 'More detail.'`, when run, then `todos/<ULID>.md`'s body is
  `"Ship it.\n\nMore detail.\n"` and `rk todo list` shows only `"Ship it."`.
- **`TestTodoAdd_MessageFlag_TrimsEachMessage`** — Given `-m '  Ship it.  '
  -m '  detail  '`, when run, then the stored body is
  `"Ship it.\n\ndetail\n"` (each message trimmed before joining).
- **`TestTodoAdd_MessageFlag_EmptySubjectErrors`** — Given `-m ''`, when
  run, then it errors and no `todos/*.md` file is created (E1).
- **`TestTodoAdd_MessageFlag_WhitespaceOnlySubjectErrors`** — Given `-m
  '   '`, when run, then it errors (E2).
- **`TestTodoAdd_MessageFlag_TrailingEmptyMessageAbsorbed`** — Given `-m
  'Ship it.' -m ''`, when run, then it succeeds with stored body
  `"Ship it.\n"` — byte-identical to a lone `-m 'Ship it.'` (E3).
- **`TestTodoAdd_MessageFlag_InteriorEmptyMessagePreservesBlankRun`** —
  Given `-m 'S' -m '' -m 'p2'`, when run, then it succeeds with stored body
  `"S\n\n\n\np2\n"` (E3b).
- **`TestTodoAdd_MessageFlag_SingleMessageValidSubjectOnly`** — Given a
  lone `-m 'Buy milk'` with no positional args and no second `-m`, when
  run, then it succeeds with stored body `"Buy milk\n"` and `rk todo list`
  shows `"Buy milk"` (E3c — directly answers "is a subject-only body still
  valid with no other body args").
- **`TestTodoAdd_EditFlag_UsesSavedContent`** — Given `--edit` with the
  editor seam stubbed to write `"Fix the leak\n\nRoot cause: ...\n"`, when
  run, then the stored body matches the trimmed buffer + trailing `\n`, and
  `rk todo list` shows only `"Fix the leak"`.
- **`TestTodoAdd_EditFlag_EmptySaveErrors`** — Given `--edit` with the
  editor seam writing `""` (or leaving the file untouched), when run, then
  it errors and no todo file is created (E4).
- **`TestTodoAdd_EditFlag_EditorNonzeroExitAborts`** — Given `--edit` with
  the editor seam returning a nonzero-exit error, when run, then it errors
  distinctly from the empty-buffer case and no todo file is created (E5).
- **`TestTodoAdd_EditFlag_EditorUnsetErrors`** — Given `--edit` with
  `$EDITOR` unset (test clears the env var), when run, then it errors
  before invoking the editor seam at all (E6).
- **`TestTodoAdd_EditFlag_NoCommentStripping`** — Given `--edit` with saved
  content `"# Real Heading\n\nBody text.\n"`, when run, then the stored
  subject is `"# Real Heading"` verbatim (not stripped as a comment).
- **`TestTodoAdd_StdinDash_ReadsFullBody`** — Given `RootCmd.SetIn` wired
  to a reader yielding `"Subject line\n\nDetail.\n"` and args `["-"]`, when
  run, then the stored body is the trimmed input + trailing `\n` and the
  subject is `"Subject line"`.
- **`TestTodoAdd_StdinDash_EmptyErrors`** — Given stdin yields `""`, when
  run with args `["-"]`, then it errors (E4-equivalent).
- **`TestTodoAdd_StdinDash_MultipleArgsDoesNotTriggerSentinel`** — Given
  args `["-", "extra"]`, when run, then stdin is never read and the text is
  treated as literal positional args (E12).
- **`TestTodoAdd_ConflictingSources_MessageAndPositional`** — Given `rk
  todo add "text" -m 'x'`, when run, then it errors (E11).
- **`TestTodoAdd_ConflictingSources_MessageAndEdit`** — Given `-m 'x'
  --edit`, when run, then it errors (E9).
- **`TestTodoAdd_ConflictingSources_StdinAndMessage`** — Given args `["-"]`
  with `-m 'x'` also set, when run, then it errors (E7).
- **`TestTodoAdd_ConflictingSources_StdinAndEdit`** — Given args `["-"]`
  with `--edit` also set, when run, then it errors (E8).
- **`TestTodoAdd_NoSourceAtAllErrors`** — Given zero positional args and
  none of `-m`/`--edit`/stdin-`-`, when run, then it errors (E16); this is
  the regression test for the `MinimumNArgs(1)` → `ArbitraryArgs` change
  not silently permitting a truly empty invocation.
- **`TestTodoAdd_Ephemeral_RejectsMessageFlag`** — Given `--ephemeral -m
  'x'`, when run, then it errors (E13).
- **`TestTodoAdd_Ephemeral_RejectsEditFlag`** — Given `--ephemeral --edit`,
  when run, then it errors (E14).
- **`TestTodoAdd_Ephemeral_RejectsStdinDash`** — Given `--ephemeral` with
  args `["-"]`, when run, then it errors (E15).
- **`TestTodoAdd_PositionalArgsUnaffected`** — Given `rk todo add foo bar`
  (no new flags), when run, then behavior is byte-identical to
  pre-ticket (`TestTodoAdd_DurableHappyPath` continues to pass unmodified) —
  AC6 regression guard.
- **`TestTodoAdd_LongSubjectNotTruncated`** — Given `-m <5000-char string>`,
  when run, then the stored subject/body is the full untruncated string
  (E10).
- **`TestAdd_MessageFlag_JoinsParagraphs`** — Given `rk add -m 'S' -m
  'p1'`, when run, then the appended log entry's body is `"S\n\np1"` (AC2;
  no subject/title assertion, `rk add` has none).
- **`TestAdd_MessageFlag_EmbeddedHeaderGuardAppliesPostAssembly`** — Given
  `-m 'S' -m '## fake header'`, when run, then it errors via the existing
  `embeddedHeaderRe` guard, now checked against the assembled multi-`-m`
  body (E18).
- **`TestAdd_EditFlag_UsesSavedContent`** — mirrors
  `TestTodoAdd_EditFlag_UsesSavedContent` for `rk add` (AC3, no subject
  assertion).
- **`TestAdd_StdinDash_ReadsFullBody`** — mirrors
  `TestTodoAdd_StdinDash_ReadsFullBody` for `rk add` (AC4).
- **`TestAdd_ConflictingSources_MessageAndEdit`** — mirrors the `todo add`
  conflict tests for `rk add` (E9 analog); one representative case is
  sufficient coverage for `add_test.go`, the full conflict matrix already
  lives in `todo_test.go`.

## 5. Out of scope

- **`rk note create` interactive wizard, TTY-guard/wizard flow** — fnqs.6 /
  fnqs.7. This ticket is the non-interactive flag/stdin surface only, per
  the ticket's own "non-interactive (agent + script) surface" framing.
  `isInteractive`/`promptGuard` (`interactive.go`) are not touched or
  consulted by any of these three paths — `--edit` and stdin-`-` work
  identically whether or not stdin/stdout are TTYs (an agent piping via a
  non-TTY stdin is exactly the target use case).
- **`rk todo show` / `rk todo edit` (reading/editing an existing todo's
  body)** — fnqs.4. This ticket only concerns *creation*-time body
  assembly.
- **git-parity features not asked for:** `-m` + `--edit` augmenting
  (Implicit Requirements), `$VISUAL` fallback chain, comment-line
  stripping in the scratch buffer, `--cleanup` modes. None required.
- **`--ephemeral`'s single-`-m`-with-no-newline carve-out** — noted as a
  possible future relaxation, not required (Implicit Requirements).
- **Changing `addDurableTodo`/`appendLogEntry`'s write recipe, frontmatter
  shape, or `writeFileAtomic` semantics.** Unchanged; only body-assembly
  upstream of `RunE`'s call into these functions changes.
- **`internal/node` package changes.** `NewNode`/`Render`/`Parse` are
  unchanged; this ticket only changes what string `RunE` passes in as
  `body`.
- **[OPEN] If the ticket owner narrows scope to `rk todo add` only** at
  planning time (contradicting the Grounding-facts resolution above), every
  `rk add`-specific AC/edge case/test (AC2, E18, the `TestAdd_*` scenarios)
  drops cleanly with no ripple into the `todo add` set — they're written as
  independent mirrors, not a shared implementation requirement.
