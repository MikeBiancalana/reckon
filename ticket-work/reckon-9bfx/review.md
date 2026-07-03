# Code Review: reckon-9bfx — ULID mint policy + `rk adopt` for id-less files

Verdict: APPROVE WITH CHANGES

Reviewer: code-reviewer (Opus 4.8)
Date: 2026-07-03
Scope: `git diff origin/main` — `internal/node/insert.go`, `internal/cli/adopt.go`,
`internal/index/reconcile.go` (exported wrappers), `internal/cli/root.go`,
`docs/design/composable-redesign.md`, plus `internal/node/insert_test.go` and
`internal/cli/adopt_test.go`.

## Summary

This is a strong, carefully-scoped implementation. The load-bearing pieces —
the span-safe byte splice, the atomic write path, idempotency, vault
containment against `..` traversal, and the ignore-rule reuse — are all
correct, and I verified the two most consequential edge cases empirically
(byte-identical no-op on re-run; the detection trichotomy) rather than trusting
them by inspection. All three "Done when" criteria are genuinely met: the policy
is recorded and accurate in the design doc, `rk adopt` stamps via a span-safe
insert and is well tested, and the mint-on-create criterion is honestly (not
vacuously) fulfilled. I confirmed independently that there are no other
node-creation call sites in the tree to wire into.

I am not blocking. The verdict is APPROVE WITH CHANGES because two behaviors
deserve a decision before merge — a plausible real-world input (a blank `id:`
frontmatter field on an Obsidian template) currently produces a misleading
error, and the vault-containment check does not account for symlinks (low
severity for this threat model, but the preflight report's security rationale
for it is factually backwards). Neither corrupts data; both refusal/error paths
leave the file untouched.

## Verification performed

- `go vet ./internal/{node,cli,index}/...` — clean.
- `go test ./internal/{node,cli,index}/...` — all pass.
- Independent grep: `node.NewNode` and `(*Node).Render()` have **zero** callers
  outside `internal/node`; the only production `node.Mint()` caller is
  `adopt.go:217`. `add`/`todo` are `errNotImplemented` stubs; the legacy
  `journal`/`task`/`log`/`note` code does not import `internal/node` at all.
  This substantiates the plan's criterion-3 reasoning as fact, not hand-wave.
- Throwaway probe test (added, run, removed) to confirm two edge cases below.

---

## Critical Issues

None. No path corrupts a truth file: every refusal and every per-file error
returns before `writeFileAtomic` is reached, and the atomic write itself leaves
the original intact on any failure. This was the primary risk for a command
that mutates hand-authored notes, and it is handled correctly.

---

## Recommendations (prioritized)

### R1 — Blank `id:` frontmatter value produces a misleading error (should address)

`adoptOneFile` decides "already has an id" with `n.ULID != ""`
(`adopt.go:212`), but `InsertField` decides "key already present" with
`n.fieldSpans[key]` (`insert.go:25`). These two "does it have an id?" tests
disagree when the value is empty. A file containing a literal blank field:

```
---
id: 
type: note
---
body
```

parses to `n.ULID == ""` (so adopt does **not** skip it), then calls
`InsertField("id", …)`, which finds a span for `id` and returns
`InsertField: key "id" already present (use SetField)`. The file lands in the
`Errored` bucket with a message that reads as nonsense to an end user who sees
no id in their file. I confirmed this empirically:

```
ULID="" (empty means adopt would NOT skip)
InsertField on empty id -> err=InsertField: key "id" already present (use SetField)
```

This input is squarely in scope: a blank `id:` is exactly what an Obsidian
template or a half-migrated note looks like, and this command's whole purpose is
to first-class Obsidian-authored files. Options, in rough order of preference:
1. In `adoptOneFile`, when `id` exists but is empty, call `n.SetField("id", id)`
   (the field exists → SetField is the right primitive) and report it adopted.
2. Or treat a present-but-empty `id` as a `skipped` no-op with a clear reason.
3. At minimum, do not surface a raw "use SetField" internal hint to end users.

Whichever is chosen, add a test — this branch is currently untested.

### R2 — Vault containment does not resolve symlinks; correct the preflight claim (should address)

The containment check (`adopt.go:135-139`) is correct for lexical `..`
traversal — and is in fact slightly better than the `config.LoadWithOverrides`
guard it mirrors, because the separator-aware prefix (`".."+Separator`)
correctly does **not** reject a legitimately-named file like `..foo`. Good.

However, both `absVault` and the argument are only `filepath.Abs`-cleaned
(lexical), never `filepath.EvalSymlinks`-resolved. `os.Stat` (`:142`),
`os.ReadFile` (`:192`), and `writeFileAtomic`'s `os.CreateTemp`/`os.Rename`
(`:243`,`:265`) all follow symlinks. So a symlink **inside** the vault whose
target is outside it (e.g. `vault/link -> /etc`, then `vault/link/foo.md`)
passes the lexical `Rel` check yet reads/writes outside the nominal boundary.
The directory-walk path is partly protected — `filepath.WalkDir` does not
descend symlinked directories — but a symlinked **file** (`x.md -> /outside/y.md`)
in a walked dir still satisfies `index.Indexable` (name-only) and would be
followed by the read/write.

For a single-user, local notes tool operating on the user's own vault this is
low severity (the "attacker" is the user's own filesystem), so I am not raising
it to critical. But two things should change:
- The preflight report states "No symlink-based traversal risks (os.Stat used,
  not Lstat)". That reasoning is inverted — `os.Stat` *follows* symlinks; it is
  `Lstat` that would let you detect and refuse them. The claim should be
  corrected so it doesn't mask the actual posture.
- Either harden (resolve with `EvalSymlinks` before the `Rel` check, or `Lstat`
  and refuse symlinked entries), or make the trust boundary an explicit,
  documented decision. There is also a benign TOCTOU between the `Rel`/`Stat`
  check and the later independent `ReadFile`/`writeFileAtomic` re-opens by path;
  same threat model, worth a one-line acknowledgement.

### R3 — Leading `---\n` thematic break is refused (document as a user-facing caveat)

The detection trichotomy is implemented correctly — I verified the exact-fence
distinction the plan calls out:

```
bodySpan.Start=0 ULID=""    (file: "---\n# Heading\nContent.\n")
InsertField ... -> err=InsertField: unterminated frontmatter block; refusing to insert
```

A note whose very first bytes are exactly `---\n` (a Markdown thematic
break/horizontal rule) with no later `\n---` is classified as an *unterminated
frontmatter block* and refused. This is the deliberate, conservative,
non-destructive choice (prepending would nest the user's `---` into the body),
and the `TestInsertField_LeadingDashesNotFrontmatterFence` test correctly pins
that `---Not a fence` / `----` (not the exact 4-byte signature) still prepend.
So the trichotomy is right, not merely plausible.

The residual gap: a legitimate id-less note that opens with a `---` rule — a
real Obsidian-authored shape, i.e. exactly this command's target population —
cannot be auto-adopted and surfaces a per-file error. That is acceptable, but
it should be a documented caveat (in `adopt`'s long help and/or the ticket's
known-limitations), so users understand the error and know the fix (add content
or a closing fence, or stamp manually). Full handling belongs to the parser
scope (reckon-vj55), consistent with how CRLF is deferred.

### R4 — Minor: explicitly-named ignored directory is walked (low priority)

`walkAdoptDir` guards `path != dir && index.ShouldSkipDir(...)` (`:170-172`),
matching the index's own `path != VaultDir` guard (`reconcile.go:138`) exactly —
so nested `.git`/`.obsidian`/`.stversions`/`.sync-conflict-*` are correctly
skipped and the walk-root is not. The one asymmetry vs. the index: the index
only ever walks from the vault root, whereas adopt can be pointed at an
arbitrary subdirectory. Consequently `rk adopt .obsidian` would walk *into*
`.obsidian` (the walk-root is never skipped). Defensible as explicit user
intent, but worth a conscious decision — if adopting an explicitly-named ignored
dir should be refused, add the check; otherwise leave a comment noting it's
intentional.

### R5 — Optional hardening: no fsync in `writeFileAtomic` (low priority)

`writeFileAtomic` writes → closes → chmods → renames, with correct temp cleanup
on every branch (the `ok` flag + deferred `os.Remove` is right, and the temp
name `.adopt-*.tmp` is deliberately ignored by both the index and a concurrent
adopt walk). It does not `fsync` the file before rename, nor the parent
directory after. The rename-over-original is atomic w.r.t. concurrent readers,
but durability across a power loss is not guaranteed. For a notes tool this is a
reasonable omission; flagging only so it's a conscious trade-off.

---

## Positive Observations

- **Byte-preservation splice is exactly right.** In the terminated-block branch
  (`insert.go:32-36`), `bodySpan.Start > 0` is only ever set by
  `parseFrontmatter` after `HasPrefix(raw, "---\n")` succeeded, so `raw[:4]` is
  provably the opening fence; `raw[4:]` is preserved verbatim. The no-block
  branch preserves the entire original as a suffix. Tests assert byte-exact
  expected output, not just "no error."
- **`InsertField` mirrors `SetField` cleanly** — same splice-then-`ParseAt`
  re-parse discipline, same `*n = *reparsed` refresh, same encapsulation (spans
  stay inside `internal/node`). It is a genuine complement (missing key) to
  `SetField` (existing key), and correctly leaves `SetField` untouched.
- **Idempotency is genuine**, not assumed: `TestAdoptCmd_IdempotentAcrossTwoRuns`
  asserts the second run is byte-identical, and the already-id'd no-op test
  asserts `got == src`. Confirmed the detection keys on the parsed field, not a
  truthy-string trap.
- **Ignore rules are drift-free.** The exported `Indexable`/`ShouldSkipDir`
  wrappers delegate to the exact `indexable`/`shouldSkipDir` the index walk uses
  (`reconcile.go:133-143`); adopt's walk predicate structure matches it
  line-for-line. Exporting thin wrappers rather than copying the predicates was
  the right call and adds no new dependency direction.
- **Criterion 3 is met honestly.** Adopt mints via `node.Mint()` — the same
  function `node.NewNode` uses — so adopted files are format-indistinguishable
  from tool-created ones, and I verified there is no competing "author
  frontmatter from scratch" path and no other create call site to wire. The
  design-doc note (invariant #7, now citing reckon-9bfx/`InsertField`/`node.Mint`)
  is accurate to the shipped code.
- **Design-doc policy (criterion 1) is present and correct**: invariant #7
  states the index-never-mutates-truth + `file:<relpath>` keying + opt-in
  `rk adopt` policy verbatim; invariant #6 records mint-on-create via
  `node.NewNode`.
- **Output-mode handling is correct**: `--quiet` suppresses only the Pretty
  status line (`!(mode == Pretty && quietFlag)`), while `--json`/`--ndjson`
  always emit — matching the `index.go` convention. `adoptResult.Pretty()` (value
  receiver) satisfies `output.prettyPrinter` and is dispatched correctly.
- **Error handling is consistent and batch-friendly**: per-file failures are
  collected, one bad path never blocks the rest (D6), and a summarizing error is
  returned so the exit code reflects failure while the structured result is still
  printed. All errors are wrapped with context.
- **Tests are substantive, not fitted after the fact**: independent byte-exact
  expectations, an independent Crockford-alphabet ULID validator, distinct-ULID
  assertions across a mixed batch, file-unchanged assertions on *every* refusal
  branch (non-md, conflict marker, CRLF, unterminated, out-of-vault), and a
  no-cache-dir-created assertion proving the index is never touched.

## Questions for Consideration

- **R1 direction**: should a blank `id:` be *filled* (SetField) or *skipped*?
  Filling matches the "give Obsidian files a path to first-class" intent; skipping
  is the more conservative "never touch a field the user started." Please pick
  one deliberately and test it.
- **R2 trust boundary**: is a symlink inside the vault in scope to honor
  (resolve + contain) or explicitly out of scope (documented)? Either is
  defensible; the current state is neither stated nor tested.
- **Test completeness (minor)**: the plan listed an `--ndjson` output scenario;
  only `--json` and `--quiet` got dedicated tests. For a single record the
  NDJSON and JSON code paths are identical (`json.Marshal(v)` + `"\n"`), so this
  is near-redundant — noting only for traceability against the plan.
