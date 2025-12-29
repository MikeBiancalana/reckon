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
```

## Workflow for Agents

### Starting Work on an Issue
1. **Check current issues**: `bd list --status open` or `bd list --status in_progress`
2. **Pick an issue**: Choose based on priority (P0 = critical, P4 = nice-to-have)
3. **Start working**: `bd update <issue-id> --status in_progress`
4. **Work on the code** following standard development practices
5. **Mark complete**: `bd update <issue-id> --status done`

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
bd update <id> --status in_progress
# Fix the bug
bd update <id> --status done
bd sync
```

### Feature Implementation
```bash
bd create "Add dark mode toggle" --type feature --priority P2
bd update <id> --status in_progress
# Implement feature across multiple files
bd comment <id> "Completed UI component, working on state management"
bd update <id> --status done
bd sync
```

### Multi-step Tasks
```bash
bd create "Refactor authentication module" --type task --priority P2
bd update <id> --status in_progress
bd comment <id> "Step 1: Extract interface"
bd comment <id> "Step 2: Update implementations"
bd update <id> --status done
bd sync
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