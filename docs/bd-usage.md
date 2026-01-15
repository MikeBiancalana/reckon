# Beads (bd) Usage for Agents - v0.43.0

## Overview
Beads is an AI-native issue tracking system that lives in your codebase. Version 0.43.0 introduces powerful features for multi-agent coordination, including atomic work claiming, worktree support, and swarm management.

**Key Features for Agents:**
- Atomic work claiming (`--claim`) prevents duplicate work
- Git worktrees enable parallel development without conflicts
- Shared `.beads` database across worktrees for consistent state
- Swarm management for structured epic-based parallelism
- Auto-sync with git (hooks handle synchronization)

## Quick Reference

```bash
# Finding and claiming work
bd ready                          # Show unblocked issues
bd update <id> --claim            # Atomically claim (assignee + in_progress)
bd show <id>                      # View issue details

# Creating and managing issues
bd create "Title" --type task --priority 2
bd update <id> --status done      # or: bd close <id>
bd dep add <a> <b>                # A depends on B (B blocks A)

# Sync and health
bd sync                           # Sync with git remote
bd doctor                         # Check installation health
bd prime                          # Show workflow context
```

## Core Concepts

### Issue Lifecycle
```
open → in_progress → done/closed
```

### Atomic Work Claiming (New in v0.42.0)
The `--claim` flag provides work queue semantics:
```bash
bd update <id> --claim
```
- Atomically sets `assignee` to you and `status` to `in_progress`
- Fails if already claimed by another agent
- Perfect for multi-agent coordination
- Works across worktrees (shared database)

### Priority System
Use numeric priorities (0-4):
- **P0**: Critical/blocking
- **P1**: High priority
- **P2**: Medium (default)
- **P3**: Low priority
- **P4**: Backlog/nice-to-have

### Dependency Types
- **blocks**: Hard dependency (B must complete before A can start)
- **parent-child**: Epic/subtask hierarchical relationship
- **related**: Soft connection, doesn't block progress
- **discovered-from**: Auto-created when AI discovers related work

## Essential Commands

### Finding Work

**Show ready work (no blockers):**
```bash
bd ready                          # All unblocked issues
bd ready --parent <epic-id>       # Ready work scoped to epic
bd ready --json                   # JSON output for parsing
```

**List issues:**
```bash
bd list                           # All non-closed issues (limit 50)
bd list --status open             # Filter by status
bd list --status in_progress      # Your active work
bd list --priority 0              # Critical issues only
bd list --assignee alice          # Issues assigned to alice
bd list --label bug               # Filter by label
bd list --sort priority           # Sort by priority
bd list --reverse                 # Reverse sort order
```

**Search issues:**
```bash
bd search "authentication"        # Text search
bd search "login" --priority 1    # Search with filters
bd search "api" --after 2025-01-01  # Date filters
```

**Show blocked issues:**
```bash
bd blocked                        # All blocked issues
bd blocked --parent <epic-id>     # Blocked work in epic
```

### Creating Issues

**Basic creation:**
```bash
bd create "Fix login bug"
bd create "Add auth" --priority 1 --type feature
bd create "Write tests" --type task --assignee alice
bd create "Epic name" --type epic  # For multi-issue work
```

**Quick capture (output only ID):**
```bash
ID=$(bd q "Quick task")           # Captures ID for scripting
```

**Batch creation from markdown:**
```bash
bd create -f tasks.md             # Create multiple from file
```

**Creating with parent (subtask of epic):**
```bash
bd create "Subtask" --parent <epic-id>
```

### Updating Issues

**Atomic claiming (v0.42.0+) - PREFERRED:**
```bash
bd update <id> --claim            # Atomically claim work (sets assignee + status to in_progress)
# Fails if already claimed - prevents duplicate work in parallel workflows!
```

**Manual updates (use --claim instead for claiming work):**
```bash
bd update <id> --priority 0
bd update <id> --assignee bob
bd update <id> --description "New description"
bd update <id> --status in_progress  # Manual claim (prefer --claim for parallel agents)
```

**Defer for later:**
```bash
bd update <id> --defer 2025-02-01  # Hidden from bd ready until date
bd undefer <id>                    # Restore to ready state
```

### Closing Issues

**Simple close:**
```bash
bd close <id>
bd close <id> --reason "Fixed in PR #42"
bd close <id1> <id2> <id3>        # Close multiple
```

**Close and see newly unblocked work:**
```bash
bd close <id> --suggest-next      # Shows what's now ready
```

### Dependencies

**Add dependency:**
```bash
bd dep add <a> <b>                # A depends on B (B blocks A)
bd dep add task-1 task-2 task-3   # Multiple deps at once
```

**Remove dependency:**
```bash
bd dep remove <a> <b>
```

**View dependency tree:**
```bash
bd dep tree <id>                  # Show full tree
bd dep cycles                     # Detect circular deps
bd graph                          # Visualize all deps
```

### Comments

**Add comments:**
```bash
bd comments <id> --add "Progress update"
bd comments <id> --add "Blocked on API"
```

**View comments:**
```bash
bd show <id>                      # Includes comments
bd comments <id>                  # List all comments
```

## Multi-Agent Workflows

### Atomic Work Claiming

When multiple agents work on the same project, use `--claim` to prevent duplicate work:

```bash
# Agent 1
bd ready                          # See available work
bd update reckon-abc --claim      # Claim atomically
# ✓ Success - you now own this issue

# Agent 2 (simultaneously)
bd update reckon-abc --claim      # Try to claim same issue
# ✗ Error: already claimed by Agent 1
bd ready                          # Find different work
```

**How --claim works:**
1. Checks current assignee and status
2. If unclaimed (assignee=null or status=open), claims it
3. Sets assignee=you, status=in_progress atomically
4. If already claimed, fails with error
5. Works across worktrees (shared database)

### Git Worktrees for Parallel Development

**Why use worktrees?**
- Multiple agents work on different issues simultaneously
- No git checkout conflicts
- Shared `.beads` database ensures consistent issue state
- Each agent has isolated working directory
- Clean branch-per-issue workflow

**Creating a worktree:**
```bash
# From main repo directory
bd worktree create reckon-abc     # Creates ../reckon-abc/
bd worktree create bugfix --branch fix-validation
```

This creates:
- New directory at `../reckon-abc/`
- Branch `reckon-abc` checked out from `origin/main`
- `.beads/redirect` file pointing to shared database
- Ready-to-use working tree

**Working in a worktree:**
```bash
cd ../reckon-abc                  # Switch to worktree
bd ready                          # See work (shared DB)
bd update reckon-abc --claim      # Claim your issue
# ... do the work ...
git add .
git commit -m "feat: implement feature"
bd sync                           # Sync beads
git push -u origin reckon-abc     # Push your branch
gh pr create --base main          # Create PR
```

**Cleaning up:**
```bash
# After PR is merged
cd ../reckon                      # Return to main repo
bd worktree remove reckon-abc     # Safely remove worktree
```

**Worktree commands:**
```bash
bd worktree list                  # Show all worktrees
bd worktree info                  # Current worktree details
bd worktree create <name>         # Create new worktree
bd worktree remove <name>         # Remove worktree
bd where                          # Show active beads location
```

### Swarm Management for Structured Parallelism

**What is a swarm?**
A swarm is a structured epic with dependency-aware task distribution. Perfect for coordinating multiple agents on large features.

**Creating a swarm:**
```bash
# 1. Create an epic with subtasks
bd create "Feature X" --type epic
EPIC_ID=<epic-id>

bd create "Design API" --type task --parent $EPIC_ID
bd create "Implement backend" --type task --parent $EPIC_ID
bd create "Build frontend" --type task --parent $EPIC_ID
bd create "Write tests" --type task --parent $EPIC_ID

# 2. Add dependencies
bd dep add <frontend-id> <backend-id>   # Frontend depends on backend
bd dep add <tests-id> <backend-id>      # Tests depend on backend

# 3. Create the swarm
bd swarm create $EPIC_ID
```

**Working with swarms:**
```bash
# View swarm status
bd swarm status                   # All active swarms
bd swarm list                     # List swarm molecules

# Validate epic structure
bd swarm validate <epic-id>       # Check DAG validity

# Find ready work in swarm
bd ready --parent <epic-id>       # Unblocked tasks in epic

# Agents claim work
bd update <task-id> --claim       # Each agent claims a task
```

**Swarm benefits:**
- Automatic dependency tracking
- Parallel work coordination
- Progress visibility for all agents
- Structured completion tracking
- Prevents dependency violations

### Parallel Agent Workflow Example

**Scenario:** 3 agents working on "User Authentication" feature

```bash
# === Setup (Human or Lead Agent) ===
bd create "User Authentication" --type epic
AUTH_EPIC=<epic-id>

bd create "Database schema" --type task --parent $AUTH_EPIC
bd create "API endpoints" --type task --parent $AUTH_EPIC
bd create "Frontend UI" --type task --parent $AUTH_EPIC
bd create "Integration tests" --type task --parent $AUTH_EPIC

# Add dependencies
bd dep add <api-id> <schema-id>      # API needs schema
bd dep add <ui-id> <api-id>          # UI needs API
bd dep add <tests-id> <api-id>       # Tests need API

# Create swarm for coordination
bd swarm create $AUTH_EPIC

# === Agent 1: Database Schema ===
bd worktree create reckon-schema
cd ../reckon-schema
bd update reckon-schema --claim      # Claims database schema task
# ... implement schema ...
git commit && git push
bd close reckon-schema
# This automatically unblocks API task

# === Agent 2: API Endpoints (waits for schema) ===
bd ready                             # Sees API is now ready
bd worktree create reckon-api
cd ../reckon-api
bd update reckon-api --claim         # Claims API task
# ... implement API ...
git commit && git push
bd close reckon-api
# This unblocks UI and tests

# === Agent 3 & 4: Frontend and Tests (parallel) ===
# Both now ready after API completes
bd worktree create reckon-ui
cd ../reckon-ui
bd update reckon-ui --claim

# Simultaneously, Agent 4:
bd worktree create reckon-tests
cd ../reckon-tests
bd update reckon-tests --claim

# Both work in parallel, no conflicts
```

## Sync & Collaboration

### Git Integration

**Auto-sync (default):**
bd automatically syncs with git:
- Exports to `.beads/issues.jsonl` after CRUD operations (5s debounce)
- Imports from JSONL when newer than database (after git pull)
- Git hooks handle pre-commit and pre-push validation

**Manual sync:**
```bash
bd sync                           # Sync with remote
bd sync --status                  # Check sync status
bd sync --squash                  # Batch commits into one
```

**Sync workflow:**
```bash
# During work
bd create "New task"              # Auto-exports after 5s

# Before pushing
bd sync                           # Ensure everything synced
git push                          # Push code and issues

# After pulling
git pull                          # Auto-imports if JSONL newer
```

**Worktree sync:**
- All worktrees share the same `.beads` database
- Changes in one worktree visible in all others
- Sync happens in the main repo's `.beads/` directory
- Each worktree has a `.beads/redirect` file

### Session End Protocol

**MANDATORY steps before ending a session:**

```bash
# 1. Check git status
git status

# 2. Stage code changes
git add <files>

# 3. Sync beads
bd sync

# 4. Commit code
git commit -m "feat: implement feature (reckon-abc)"

# 5. Sync beads again (in case commit created new data)
bd sync

# 6. Push everything
git push

# 7. Verify push succeeded
git status                        # Must show "up to date"
```

**NEVER skip the push.** Work is not complete until pushed to remote.

## Project Health

### Status and Statistics

**Project overview:**
```bash
bd status                         # Overview + stats
bd stats                          # Alias for status
```

**Find problems:**
```bash
bd blocked                        # Blocked issues
bd stale --days 30                # Not updated in 30 days
bd orphans                        # In commits but not closed
```

### Health Checks

**Run diagnostics:**
```bash
bd doctor                         # Full health check
bd doctor --check-health          # Lightweight check (exit 0 if ok)
bd doctor --fix                   # Auto-fix issues
bd doctor --output report.txt     # Export diagnostics
```

**Common fixes:**
- Stale database detection
- Sync branch issues
- Hook installation
- Orphaned references
- Database corruption

### Database Maintenance

**Admin commands:**
```bash
bd repair                         # Repair orphaned references
bd admin compact                  # Reduce database size
bd admin compact --purge-tombstones  # Remove old deletions
```

## Advanced Features

### State Management (v0.42.0+)

**Label-based operational state:**
```bash
bd state <dimension>              # Query current state
bd set-state <dimension> <value>  # Set state (creates event + label)
```

Example:
```bash
bd state environment              # Check current env
bd set-state environment staging  # Deploy to staging
```

### Gates for Async Coordination

**Create coordination gates:**
```bash
bd gate create "Deploy approval" --type approval
bd gate create "Load test done" --type timer --duration 1h
```

**Manage gates:**
```bash
bd gate show <gate-id>
bd gate approve <gate-id>         # Approve human gates
bd gate eval                      # Evaluate timer/GitHub gates
bd gate wait <gate-id>            # Block until gate opens
```

### Preflight PR Checks (v0.42.0+)

**Check PR readiness:**
```bash
bd preflight                      # Show PR readiness checklist
```

Validates:
- All in_progress issues completed or blocked
- No orphaned references
- Database synced with JSONL
- Tests passing (if configured)

## Configuration

**View config:**
```bash
bd config list                    # Show all settings
```

**Common settings:**
```bash
bd config set sync.branch beads-sync     # Use separate sync branch
bd config set git.author "Agent <ai@example.com>"
bd config set create.require-description true
bd config set no-git-ops true            # Manual git control
```

## Tips for Efficient Agent Workflows

### Work Queue Pattern
```bash
while true; do
  TASK=$(bd ready --json | jq -r '.[0].id')
  [ -z "$TASK" ] && break
  bd update $TASK --claim || continue
  # ... do work ...
  bd close $TASK
done
```

### Parallel Batch Processing
```bash
# Create tasks
for item in task1 task2 task3; do
  bd create "$item" --type task &
done
wait

# Process in parallel (multiple agents/worktrees)
bd ready | while read id; do
  bd worktree create $id
  # Spawn agent in worktree
done
```

### Discovered Work Pattern
```bash
# During implementation, discover new task
NEW=$(bd create "Fix validation bug" --type bug --priority 1 -q)
bd dep add $CURRENT_TASK $NEW     # Current task blocked
bd update $CURRENT_TASK --status blocked
bd update $NEW --claim            # Switch to new task
```

### Epic Decomposition
```bash
# Break large work into coordinated pieces
EPIC=$(bd create "Large Feature" --type epic -q)
for subtask in design impl test docs; do
  bd create "$subtask" --parent $EPIC --type task
done
bd swarm create $EPIC
```

## Troubleshooting

### Common Issues

**Can't claim issue:**
```bash
bd show <id>                      # Check current assignee
# If claimed by you, just update status:
bd update <id> --status in_progress
```

**Worktree database not syncing:**
```bash
bd where                          # Check database location
cat .beads/redirect               # Verify redirect path
bd doctor                         # Check for issues
```

**Sync conflicts:**
```bash
bd sync                           # Will auto-merge most conflicts
# If fails, check:
git status                        # Check for conflicts
bd doctor --fix                   # Auto-fix database issues
```

**Stale database warning:**
```bash
bd sync                           # Sync with latest
# If persists:
bd doctor --fix                   # Repair database
```

### Getting Help

```bash
bd help <command>                 # Command-specific help
bd human                          # Essential commands
bd quickstart                     # Quick start guide
bd prime                          # Workflow context
bd info --whats-new               # Recent changes
```

## Migration from v0.38.0

**Removed commands:**
- `bd pin/unpin/hook` - Removed in v0.39.0 (use `gt mol` commands or `--claim`)

**Replacements:**
```bash
# Old way (v0.38.0)
bd pin <id> --for me --start

# New way (v0.43.0)
bd update <id> --claim            # Atomic claiming
```

**New features to adopt:**
- Use `--claim` for atomic work queue semantics
- Use `bd worktree` for parallel development
- Use `bd swarm` for structured epic coordination
- Use `bd preflight` for PR readiness checks
- Use `bd state/set-state` for operational state management

## Best Practices

1. **Always use `--claim` for multi-agent work** - Prevents duplicate effort
2. **Use worktrees for parallel development** - Clean isolation, shared state
3. **Create swarms for structured epics** - Coordinate dependencies
4. **Run `bd sync` before session end** - Ensure everything pushed
5. **Run `bd doctor` regularly** - Catch issues early
6. **Use descriptive titles** - Clear, actionable issue names
7. **Set appropriate priorities** - P0 for critical, P2 for normal
8. **Add dependencies as discovered** - Keep dependency graph accurate
9. **Close issues with reasons** - Audit trail for decisions
10. **Check `bd ready` first** - Always work on unblocked issues

## Reference

**Issue statuses:**
- `open` - Not started
- `in_progress` - Being worked on
- `blocked` - Waiting on dependencies
- `done` - Completed successfully
- `deferred` - Postponed for later
- `closed` - Final state

**Issue types:**
- `task` - Standard work item
- `bug` - Defect or error
- `feature` - New functionality
- `epic` - Multi-issue body of work
- `gate` - Coordination primitive

**Dependency types:**
- `blocks` - Hard blocker
- `parent` - Epic/subtask hierarchy
- `related` - Soft association
- `discovered-from` - AI discovery trail

For complete details: `bd --help` or visit https://github.com/steveyegge/beads
