# Code Review — reckon-uv09 (v1-T4: log tool + capture, end-to-end)

**Verdict: APPROVE WITH CHANGES**

The core feature is sound and faithfully implements the plan's six decisions. The
capture -> index -> query pipeline works, the tricky format claims all hold up
under verification (`id::` is genuinely inert to the core parser; type-dispatch is
byte-identical to `MarkdownParser` for every non-log-day file; `contains` edges are
derived and never leak into serialized text; the `[^\s·]+` regex deviation is
correct and slightly more robust than the plan's `\S+`). Tests are real end-to-end
exercises, not mocks.

One correctness defect should be fixed before ship: the entry `time` field is
composed from **two different clocks** (local date + UTC time), producing a
demonstrably wrong RFC3339 value for non-UTC users. Plus two minor guard/robustness
gaps and a missed documentation deliverable. None block the pipeline; all are
low-cost fixes.

---

## Verification of the 8 flagged focus areas

1. **`id::` marker inert to core parser — CONFIRMED.** `grep '::'` over
   `node.go`/`parser.go`/`render.go` returns nothing. The `id::` line lives in the
   body: `parseFrontmatter` only scans the `---…---` block; `extractBody`
   (node.go:349) reacts only to `[[wikilinks]]` and trailing `^anchors` (neither
   present in `id:: <ULID>`); `deriveView` only walks frontmatter keys; `Serialize`
   returns `Raw` verbatim. `logDayWithIDs` is added to `roundtripCorpus`
   (node_test.go), so `TestRoundTripIdentity`/`FuzzRoundTripIdentity` now actually
   exercise the claim. Holds.

2. **`LogParser` type-dispatch — CONFIRMED, no regression to other node types.**
   `Type != "log-day"` returns `[]*Node{day}` (logparser.go:36-38), byte-identical
   to `MarkdownParser.Parse` (parser.go:19-25). Dispatch keys on parsed frontmatter
   `type`, not path, so it is location-independent and identical for notes/todos/
   any file. `TestLogParser_NonLogDayReturnsSingleNodeUnchanged` proves both the
   single-node return and Serialize parity. Making it the `index.Open` default is
   safe for every existing reader.

3. **Same-host race (Decision 5) — CONFIRMED matches stated intent.** No advisory
   lock; `appendLogEntry` is an unguarded read-modify-write and `writeFileAtomic`
   (adopt.go:250) is temp-file + `os.Rename` — atomic per write, so the worst
   same-host outcome is a lost update, never a torn/corrupt file. No half-
   implemented locking exists. Matches the accepted-risk decision. (Coverage gap:
   see Testing.)

4. **CRLF + embedded-`## ` guards — REAL, not tautological.** CRLF: append path
   rejects on `bytes.Contains(raw, []byte("\r\n"))` (add.go:181); the test asserts
   both the error *and* that the file is left byte-unchanged. Embedded header:
   `embeddedHeaderRe = (?m)^## ` on the user body (add.go:72,94), tested with an
   embedded-newline case, a body-starts-with-header case, and a true-negative
   (mid-line `##` accepted). Genuine guards.

5. **`--at` / time reconstruction — DEFECT FOUND.** See Correctness finding C1
   (clock mismatch) and C2 (malformed-header empty `HH:MM`).

6. **`contains` synthesis — CONFIRMED derived, no leak.** `day.Links = append(…,
   Link{Rel:"contains", To: entry.ULID})` (logparser.go:41) mutates only the
   in-memory node; only for ULID-bearing entries; `Serialize` returns `Raw`, so it
   cannot enter the file. `TestLogParser_IDBearingFixtureRoundTrips` proves
   `Serialize(day) == source` even after the mutation. Reconcile writes these to the
   `_edges` table (reconcile.go:289-295). Correct.

7. **`[^\s·]+` vs plan's `\S+` — CORRECT, no new bug.** Excluding the `·`
   separator from the kind-word class makes the "kind present" vs "kind absent"
   header shapes unambiguous without depending on RE2's optional-group fallback.
   Verified against all fixtures (`## HH:MM kind · author`, `## HH:MM · author`):
   kind and author are extracted correctly in both. The deviation is an improvement.

8. **Test genuineness — mostly genuine, three gaps.** Tests use real vaults, real
   `node.LogParser{}.Parse`, and a real `buildIndex` + `runQuery` for the end-to-end
   case; nothing asserts a mock. No tautological tests. Gaps: TS-7, TS-8 absent;
   TS-12 skipped (see Testing).

---

## Correctness

### C1 (should fix) — entry `time` mixes local date with UTC time-of-day
`getEffectiveDate()` (root.go:186) returns **local** `time.Now().Format("2006-01-02")`,
while `resolveAtTime("")` (add.go:142) returns **UTC** `time.Now().UTC().Format("15:04")`.
`appendLogEntry` then composes `entryTime := day + "T" + hhmm + ":00Z"`
(add.go:158). The two halves come from different clocks, so the stored `time` is
neither the true UTC instant nor the local wall-clock. Demonstrated:

```
Australia/Sydney  local=2026-07-05 08:30  -> stored time=2026-07-05T22:30:00Z
                                              (true UTC instant=2026-07-04T22:30:00Z)
America/New_York  local=2026-07-05 08:30  -> stored time=2026-07-05T12:30:00Z
                                              (true UTC instant=2026-07-05T12:30:00Z)
```

Near a day boundary the **date component is wrong by a full day** (Sydney example:
`2026-07-05` stored vs `2026-07-04` actual), and the `Z`-tagged value does not
denote the moment of capture. `time` is a first-class, T2-view-queryable field, so
this ships wrong data.

Note the ACs put general timezone handling out of scope, so this is not an AC
failure — but "TZ out of scope" licenses *displaying UTC*, not composing one
timestamp from two clocks. The tests miss it because the CI/test env is effectively
UTC and each test derives its expectation from the same `getEffectiveDate()`, so
the mismatch is unobservable there.

Fix (low cost): derive the default date and default time from a **single** clock.
Simplest: in `resolveAtTime` and the date default, both use UTC — e.g. have
`appendLogEntry`'s caller default the day to `time.Now().UTC().Format("2006-01-02")`
when `--date` is not set, so date and `HH:MM` agree. (Can't just change the shared
`getEffectiveDate` — the legacy `today`/`week`/journal readers rely on its local
semantics — so add a log-local default or pass an explicit `--date`-aware UTC date
into the append.) Alternatively, commit to local for both and drop the `Z`
(store a local RFC3339 offset) — but that is a larger call.

### C2 (minor) — a non-time `## ` heading yields a malformed `time`
`buildLogEntry` (logparser.go:56-59): when `entryHeaderFieldsRe` does not match a
header, `hhmm` stays `""` and `Time` becomes `dayDate + "T" + "" + ":00Z"`, e.g.
`2026-07-05T:00Z`. `SplitEntries` treats *any* `^## ` line as an entry, so a
hand-authored / synced log-day file containing a non-timestamp `## Section` heading
splits into a `log-entry` node with a garbage `time` string that is silently
indexed. Not reachable via `rk add` (tool headers always match), but reachable for
any human/agent-authored day file. Recommend: when the fields regex does not match,
leave `Time` empty rather than emitting `<date>T:00Z`, and/or document that a
log-day file's `## ` lines must be timestamped entries (ties to the EC-9/EC-5
SplitEntries limitation the plan said it would document).

---

## Error handling / integrity

### E1 (minor) — embedded-`## ` guard covers body but not `author`
The header line is `## HH:MM · <author>`, and `author` is free text. `--author`
(or a pathological `$RECKON_AUTHOR`/`$USER`) containing a newline injects a spurious
header that `SplitEntries` reads as an extra entry. Demonstrated:

```
author = "evil\n## injected 23:59 · attacker"
-> rendered block contains 2 '## ' headers (want 1); re-parse succeeds and writes.
```

Low severity/likelihood (self-inflicted; author is normally trusted), but it is an
inconsistency with the deliberately-added body guard and lets one entry become two.
Recommend rejecting a newline in the resolved author (or masking it), mirroring the
body guard. `--at` is already safely constrained to `HH:MM` by `time.Parse`, and
`--date` is validated to `YYYY-MM-DD` (so no path traversal via the filename — good).

---

## Testing

Coverage of the formal ACs (AC-1..AC-6) is solid and genuinely end-to-end. Gaps
relative to the plan's own TS list:

- **TS-7 (same-host race)** — not implemented. Defensible (inherently flaky), but
  the accepted-risk behavior ("no corruption, last writer wins") is now unverified.
- **TS-8 (cross-device `merge=union`)** — not implemented. The ticket's Done-when
  explicitly "relies on merge=union for lossless concurrent appends"; the append-
  only-at-EOF discipline that makes union merges clean is real in the code but has
  no test here. (The `.gitattributes` attribute itself is covered by T0's
  `gitattributes_test.go`, which softens this.)
- **TS-12 (day-boundary routing)** — `t.Skip`'d with an honest rationale (no clock
  seam). Reasonable, but EC-10 is therefore unverified; note that C1 above is
  precisely a day-boundary correctness issue that a real boundary test would have
  surfaced.

No blocking test issues; consider at least a table-test for time reconstruction
across TZs once C1 is fixed.

---

## Maintainability / documentation

### M1 (should fix) — promised AGENTS.md docs did not land
plan.md ("Files to modify") commits to updating `internal/node/AGENTS.md` and
`internal/index/AGENTS.md` to document `LogParser`, the `id::` entry format, that
`index.Open`'s default is now log-aware, and the `SplitEntries` `## `-in-body
limitation (EC-9). `git diff --stat origin/main` shows neither AGENTS.md changed.
The `index.go` `Open` doc comment *was* updated well; the house docs were not. Land
the documented behavior so the `id::` format and the log-aware default are
discoverable, and so the EC-9/C2 limitation is on record.

### M2 (low) — entry `Body` is not trimmed
plan.md specifies "Body = entry lines minus header and `id::`, **trimmed**";
`buildLogEntry` returns the untrimmed remainder (retains the inter-entry blank line
/ trailing newline). Harmless (entry nodes are never serialized; only the `body`/FTS
column carries trailing whitespace, and tests use `TrimSpace`/`Contains`), but it is
a silent deviation. Trim, or update the plan text.

---

## Positive observations

- Clean stub graduation mirroring reckon-qiua; `addCmd` var name preserved so
  `root.go` needs no edit; `errNotImplemented` correctly removed with its last user.
- Type-driven single default parser (vs path-glob or per-caller `OpenWithParser`) is
  the right architectural call — the DB is identical regardless of which command
  built it.
- `RenderLogEntry` as the one shared writer/parser format definition genuinely
  prevents drift; `TestRenderLogEntry_RoundTripsThroughLogParser` locks it.
- Both write paths re-parse before writing (create: `Parse(rendered)`; append:
  `Parse(appended)`), so a would-be-unparseable file is rejected pre-write.
- Body validation (empty + embedded-`## `) runs *before* `MkdirAll`, so a rejected
  input leaves no `log/` dir — verified by `TestAddCmd_EmptyOrWhitespaceBodyRejected`.
- The `dateFlag` reset added to `resetCLIFlags` (query_test.go) with a clear
  rationale is a genuine latent-bug catch: the persistent `--date` flag would
  otherwise leak across `Execute` calls within one test binary.
- `--date`/`--at` are both input-validated; no path traversal or injection surface.

---

## Questions for the author

1. Is the C1 clock mismatch acceptable to ship under the "TZ out of scope" banner,
   or should date+time be unified to one clock now? (Recommend fixing now — it is a
   one-line default change and the `time` field is queryable.)
2. Was dropping TS-7/TS-8 and skipping TS-12 an intentional scope trim, or an
   oversight? If intentional, a one-line note in the plan's "Known risks" would
   close the loop.
