# Beads Ticket Templates

This directory contains templates for creating well-structured issues with sufficient context for autonomous agents.

## Using Templates

### Manual Method (Current)

When creating a new issue, reference these templates to ensure you include all necessary context:

```bash
# Create issue
bd create "Issue title" --type task --priority 2

# Edit to add description (copy template content)
bd update <id> --description="$(cat .beads/templates/task.md)"
# Then edit the placeholders
```

### Future: Beads Molecules (Potential)

These templates could be converted to beads molecules for integrated workflow. This would allow:

```bash
bd create --molecule task
# Interactive prompts fill in template fields
```

## Template Structure

All templates follow this structure:

1. **Problem/Context** - What needs to be done and why?
2. **Solution** - High-level approach (if known)
3. **Implementation Details** - Files to modify, specific changes
4. **Acceptance Criteria** - Checklist of requirements
5. **Notes** - Gotchas, design decisions, context

## Available Templates

- **task.md** - Standard task/feature work
- **bug.md** - Bug reports and fixes
- **epic.md** - Large features with multiple subtasks

## Best Practices

### When Creating Issues

- ✅ **Use the template** - Don't skip sections
- ✅ **Be specific** - "Add timestamp parsing" not "Fix parser"
- ✅ **List files** - Help agents know where to look
- ✅ **Define done** - Clear acceptance criteria
- ✅ **Add context** - Why is this needed? What's the impact?

### What Makes a Good Issue

**Good example:**
```
Title: Add duration parsing to schedule items

## Problem
Schedule items like "09:00-10:00 Meeting" don't parse the duration.
Users expect the end time to be captured.

## Solution
Update parser to extract end time from HH:MM-HH:MM format and calculate duration.

## Implementation Details
- Modify: internal/journal/parser.go (parseScheduleItems function)
- Add duration field to ScheduleItem model
- Update tests in parser_test.go

## Acceptance Criteria
- [ ] Parser handles "HH:MM-HH:MM Description" format
- [ ] Duration is calculated correctly
- [ ] Tests cover edge cases (same day, cross midnight)
- [ ] Backward compatible with "HH:MM Description" format
```

**Bad example:**
```
Title: Fix schedule parsing
(No description)
```

## Filling Out Templates

### Replace Placeholders

Templates use `[PLACEHOLDER]` format. Replace ALL of these with actual content:

- `[Brief description of the problem or need]` → Actual problem statement
- `[What files need modification]` → List of specific files
- `[List observable outcomes]` → Actual checklist items

### Don't Skip Sections

If a section doesn't apply, write "N/A" with explanation:

```markdown
## Solution
N/A - This is exploratory work to investigate the issue, solution will be determined during investigation.
```

### Add Context Liberally

More context is better than less. Agents can skip what they don't need, but can't invent missing context.

## Template Maintenance

When you find yourself explaining the same thing repeatedly:
- Add it to the template
- Update this README with the pattern
- Consider if it should be in AGENTS.md instead
