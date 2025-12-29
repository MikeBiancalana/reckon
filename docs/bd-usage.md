# Beads (bd) Usage for Agents

## Overview
Beads is an AI-native issue tracking system that lives in your codebase. Use `bd` commands to track work and maintain issue state during coding sessions.

## Essential Commands

### Creating Issues
```bash
bd create "Implement user authentication"
bd create "Fix login form validation" --priority P1 --type bug
```

### Status Management
```bash
# Start working on an issue
bd update <issue-id> --status in_progress

# Mark as completed
bd update <issue-id> --status done

# Close without completing (cancelled/discarded)
bd close <issue-id>
```

### Viewing Issues
```bash
# List all issues
bd list

# Show specific issue details
bd show <issue-id>

# List issues by status
bd list --status in_progress
bd list --status open

# Show only pinned (assigned) issues
bd list --pinned

# Show work ready to start (no blockers)
bd ready

# Show blocked issues (waiting on dependencies)
bd blocked

# Search issues by content
bd search "authentication"

# Get project overview and statistics
bd status

# Show issues not updated recently (potential stale work)
bd stale --days 30
```

### Work Assignment (Pinning)
```bash
# Assign work to yourself and start working
bd pin <issue-id> --for me --start

# Assign work to another agent
bd pin <issue-id> --for <agent-name>

# Just mark as pinned (assigned) without starting
bd pin <issue-id>

# View what's assigned to you (your "hook")
bd hook

# View another agent's assignments
bd hook --agent <agent-name>
```

### Workflow Management
```bash
# Defer work for later (postpone without closing)
bd defer <issue-id>

# Bring back deferred work
bd undefer <issue-id>

# Link related issues (loose "see also" relationship)
bd relate <issue-id1> <issue-id2>
```

## Workflow for Agents

### Starting Work on an Issue
1. **Check project status**: `bd status` for overall project health
2. **Check current assignments**: `bd hook` to see what's pinned to you
3. **Find ready work**: `bd ready` to see issues with no blockers
4. **Pick an issue**: Choose based on priority (P0 = critical, P4 = nice-to-have)
5. **Assign work to yourself**: `bd pin <issue-id> --for me --start` (pins and sets status to in_progress)
6. **Work on the code** following standard development practices
7. **Mark complete**: `bd update <issue-id> --status done`

### During Development
- **Add comments**: `bd comment <issue-id> "Working on the database schema"`
- **Update progress**: Use comments to track sub-tasks or blockers
- **Reference in commits**: Include issue ID in commit messages (e.g., "reckon-123: Implement auth")

### Session Completion (MANDATORY)
**CRITICAL: Always sync before ending work**
```bash
bd sync
git push
```

## Best Practices

### Issue Creation
- **Descriptive titles**: Clear, actionable descriptions
- **Appropriate priority**: P0 for blocking issues, P1 for important bugs/features
- **Correct type**: bug, task, feature, or epic for multi-issue work

### Status Updates
- **in_progress**: When actively working on the issue
- **done**: When implementation is complete and tested
- **Never leave issues in_progress** across sessions without good reason

### Work Assignment (Pinning)
- **Pin work**: Use `bd pin` to assign issues to specific agents (including yourself)
- **Check assignments**: Use `bd hook` to see what work is assigned to you or others
- **Combined action**: Use `bd pin --for me --start` to assign and start work in one command
- **Coordination**: Pinning helps coordinate work across multiple agents

### Workflow Intelligence
- **Ready work**: Use `bd ready` to find issues that can actually be started (no blockers)
- **Blocked issues**: Use `bd blocked` to identify work waiting on dependencies
- **Stale work**: Use `bd stale` to find potentially abandoned issues
- **Search**: Use `bd search` to find issues by content or keywords
- **Status overview**: Use `bd status` for project health at a glance
- **Defer/Undefer**: Use `bd defer` for work that's not currently priority, `bd undefer` to bring it back
- **Relationships**: Use `bd relate` to link related issues for better context

### System Maintenance & Hygiene
- **Health check**: Use `bd doctor` to diagnose system issues and get auto-fix recommendations
- **Auto-fix issues**: Use `bd doctor --fix` to automatically resolve detected problems
- **Unpin completed work**: Use `bd unpin` to remove assignment from closed issues
- **Cleanup old issues**: Use `bd cleanup --older-than 30 --force` to remove closed issues older than 30 days
- **Compact storage**: Use `bd compact --prune` to remove expired tombstones and reduce file size
- **Performance check**: Use `bd doctor --perf` for performance diagnostics

### Preventing Hygiene Issues
- **Unpin when closing**: Always unpin issues when marking them as done to prevent accumulation of pinned closed tasks
- **Regular cleanup**: Run `bd cleanup` periodically to remove old closed issues
- **Health monitoring**: Run `bd doctor` regularly to catch configuration issues early
- **Stale issue review**: Use `bd stale` to identify work that may need attention or closure

### Comments & Tracking
- **Add progress comments**: Track what you've done and what's next
- **Note blockers**: If stuck, comment about dependencies or issues
- **Reference related work**: Link to commits, PRs, or other issues

### Sync Requirements
- **Always run `bd sync`** before ending a session
- **Push changes** to remote repository
- **Verify sync success** - check for errors in output

## Common Patterns

### Bug Fixes
```bash
bd create "Fix null pointer in login handler" --type bug --priority P1
bd pin <id> --for me --start  # Assign and start work
# Fix the bug
bd update <id> --status done
bd sync
```

### Feature Implementation
```bash
bd create "Add dark mode toggle" --type feature --priority P2
bd pin <id> --for me --start  # Assign and start work
# Implement feature across multiple files
bd comment <id> "Completed UI component, working on state management"
bd update <id> --status done
bd sync
```

### Multi-step Tasks
```bash
bd create "Refactor authentication module" --type task --priority P2
bd pin <id> --for me --start  # Assign and start work
bd comment <id> "Step 1: Extract interface"
bd comment <id> "Step 2: Update implementations"
bd update <id> --status done
bd sync
```

### Dependency Management
```bash
bd create "Implement user API" --type task --priority P2
bd create "Build frontend client" --type task --priority P2
bd relate <api-id> <frontend-id>  # Link related issues
# If frontend blocks on API completion, use dependency commands
bd blocked  # Check for blocked work
bd ready    # See what's available to work on
```

### System Maintenance
```bash
# Regular health check
bd doctor

# Auto-fix detected issues
bd doctor --fix

# Clean up pinned closed issues
bd unpin <closed-issue-id>

# Remove old closed issues (after 30 days)
bd cleanup --older-than 30 --force

# Reduce storage size by pruning tombstones
bd compact --prune
```

## Integration with Git
- Issues are stored in `.beads/issues.jsonl`
- `bd sync` commits changes to the `beads-sync` branch
- Issue IDs can be referenced in commit messages
- Changes sync automatically with git operations

## Troubleshooting
- **Can't find bd command**: Install from https://github.com/steveyegge/beads
- **Sync fails**: Check git status, resolve conflicts if needed
- **Database issues**: Run `bd doctor` to check health
- **Lost work**: Issues are in git, check `.beads/issues.jsonl` history</content>
<parameter name="filePath">docs/bd-usage.md