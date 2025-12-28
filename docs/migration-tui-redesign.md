# TUI Redesign Migration Guide

## What's New

- New 40-40-18 layout (Logs | Tasks | Sidebar)
- Tasks: General todo list with nested notes
- Schedule: Upcoming items in daily journals
- Always-visible text entry bar

## For Existing Users

### Automatic Changes

- New database tables created automatically
- Existing journals unchanged and fully compatible
- No data migration needed

### New Files

- Individual task files in `~/.reckon/tasks/`

### New Keyboard Shortcuts

- `t` - Add task
- `n` - Add note to selected task
- `enter` - Expand/collapse task (in Tasks section)

### Updated Sections

Daily journals now support optional `## Schedule` section:
```markdown
## Schedule

- 10:00 Morning standup
- 14:00 Client meeting
```

## Rollback

If needed, you can roll back without data loss:
- Old journal files remain valid
- New database tables are separate (won't affect old code)
- Can delete task files if desired

## Testing

All backward compatibility has been tested:
- Existing journals parse correctly
- Schedule items can be added to old journals
- Database migration works automatically
- Empty state handled properly
- Edge cases with long content work