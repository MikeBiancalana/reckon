# Agent Guidelines for Reckon

## Issue Tracking Quickstart

This project uses **bd (beads) v0.43.0** for issue tracking.

**Essential commands:**
```bash
# Find and claim work
bd ready                           # Show unblocked issues
bd update <id> --claim             # Atomically claim issue (sets you as assignee + in_progress)

# Create and manage issues
bd create "Title" --type task --priority 2
bd update <id> --status done       # Mark complete (or use bd close <id>)
bd close <id>                      # Close issue
bd sync                            # Sync with git (hooks auto-sync, run at session end)

# Dependencies
bd dep add <a> <b>                 # A depends on B (B blocks A)
bd blocked                         # Show blocked issues
bd show <id>                       # View issue details
```

**For full workflow:** Run `bd prime` or see [docs/bd-usage.md](docs/bd-usage.md)

## Build/Lint/Test Commands
- **Build**: `go build -o rk ./cmd/rk`
- **Test all**: `go test ./...`
- **Test integration**: `go test -tags integration ./tests/`
- **Test single**: `go test -run TestName ./path/to/package`
- **Lint**: `go vet ./...`
- **Format**: `go fmt ./...`
- **Tidy deps**: `go mod tidy`

## Code Style Guidelines
- **Formatting**: Use `go fmt` (standard Go formatting)
- **Imports**: stdlib → third-party → internal packages (blank line between groups)
- **Naming**: PascalCase for exported, camelCase for unexported
- **Packages**: lowercase, single word (journal, tui, cli, storage)
- **Errors**: Return errors, wrap with `fmt.Errorf("context: %w", err)`
- **Pointers**: Use for optional values and large structs to avoid copying
- **Types**: Strongly typed, avoid interface{}
- **Comments**: Document all exported functions/types
- **Enums**: Use iota for constants

## Branch Management

### Standard Workflow (Single Agent)

**ALWAYS use short-lived feature branches.** Each ticket/issue gets its own branch.

**Starting work on a ticket:**

1. **Ensure you're on main and up to date:**
   ```bash
   git checkout main
   git pull origin main
   ```

2. **Create a new branch from main:**
   ```bash
   git checkout -b <issue-id>  # e.g., reckon-8ji
   ```
   - Branch naming: Use the issue ID for easy tracking
   - Examples: `reckon-8ji`, `reckon-abc`, `feature/add-timer` (if no issue ID)

3. **Work in that branch** - All commits for the ticket stay in this branch

4. **When done:**
   - Push the branch: `git push -u origin <branch-name>`
   - Create a PR to merge into main
   - Squash merge to main (keeps history clean)
   - Delete the branch after merging (GitHub does this automatically)

**CRITICAL RULES:**
- ONE branch per ticket/issue
- ALWAYS branch from `origin/main` (use `git checkout -b <branch> origin/main`)
- Keep branches short-lived (hours to days, not weeks)
- Merge to main frequently via PRs
- Delete branches after merging

### Multi-Agent Workflow (Parallel Worktrees)

**When multiple agents work in parallel**, use git worktrees to avoid branch conflicts:

**Creating a worktree for an issue:**
```bash
# From main repo directory
bd worktree create <issue-id>      # Creates worktree at ../<issue-id>/
cd ../<issue-id>/                  # Switch to worktree directory
```

This creates:
- A new directory with a clean working tree
- A branch named `<issue-id>` checked out from `origin/main`
- A `.beads/redirect` file pointing to the shared database
- All worktrees share the same `.beads` database (consistent issue state)

**Working in a worktree:**
```bash
# You're now in the worktree directory
bd ready                           # Find work (shared database)
bd update <id> --claim             # Claim your issue atomically
# ... do the work ...
git add .
git commit -m "feat: implement feature"
bd sync                            # Sync beads
git push -u origin <issue-id>      # Push branch
gh pr create --base main           # Create PR
```

**Cleaning up after PR merge:**
```bash
cd ../reckon                       # Return to main repo
bd worktree remove <issue-id>      # Safely remove worktree
```

**Worktree benefits for parallel agents:**
- Each agent works in isolation (no checkout conflicts)
- Shared `.beads` database (all agents see same issue state)
- Atomic work claiming with `--claim` prevents duplicate work
- Clean branch-per-issue model maintained

**When to use worktrees:**
- Multiple agents working simultaneously on different issues
- You want to switch between issues without stashing
- Long-running work needs parallel development

**When NOT to use worktrees:**
- Single agent working on one issue at a time (use standard branches)
- Very short-lived changes (less than an hour)

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work:
   ```bash
   bd close <id> --reason="Completed in PR #XX"
   bd sync                         # Sync beads changes
   ```
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git add .
   git commit -m "..."
   bd sync                         # Ensure beads synced
   git push
   git status                      # MUST show "up to date with origin"
   ```
5. **Clean up** - If using worktrees, remove after PR merges
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
- Run `bd sync` before final push to ensure issue state is pushed

## Parallel Agent Coordination

When multiple agents work on the same project:

**Claiming work atomically:**
```bash
bd ready                           # See available work
bd update <id> --claim             # Atomically claim (fails if already claimed)
```

The `--claim` flag:
- Sets assignee to you
- Sets status to `in_progress`
- Fails if already claimed (prevents duplicate work)
- Works across worktrees (shared database)

**Creating batches of work for parallel execution:**
```bash
# Create an epic with subtasks
bd create "Feature X" --type epic
bd create "Subtask 1" --type task --parent <epic-id>
bd create "Subtask 2" --type task --parent <epic-id>
bd create "Subtask 3" --type task --parent <epic-id>

# Create a swarm for parallel coordination
bd swarm create <epic-id>

# Multiple agents can now claim and work in parallel
bd ready                           # Shows available subtasks
bd update <task-id> --claim        # Each agent claims their work
```

**Swarm benefits:**
- Structured parallel work on epics
- Dependency tracking across agents
- Progress visibility for all agents
- Automatic unblocking when dependencies complete

## Common Patterns

### Single Agent, Single Issue
```bash
git checkout -b reckon-abc origin/main
bd update reckon-abc --claim
# ... work ...
git add . && git commit -m "..."
bd close reckon-abc
bd sync && git push
gh pr create --base main
```

### Multi-Agent, Parallel Issues (Worktrees)
```bash
# Agent 1
bd worktree create reckon-xyz
cd ../reckon-xyz
bd update reckon-xyz --claim
# ... work ...

# Agent 2 (simultaneously)
bd worktree create reckon-abc
cd ../reckon-abc
bd update reckon-abc --claim
# ... work ...

# Both agents share the same .beads database via redirects
```

### Discovering New Work During Development
```bash
# While working, you discover a bug or task
bd create "Fix validation bug" --type bug --priority 1
bd dep add reckon-current reckon-new  # New issue blocks current
bd update reckon-current --status blocked
bd update reckon-new --claim          # Switch to new issue
```
