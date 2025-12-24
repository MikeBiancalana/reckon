# Agent Guidelines for Reckon

## Build/Lint/Test Commands
- **Build**: `go build -o rk ./cmd/rk`
- **Test all**: `go test ./...`
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

Use 'bd' for task tracking

## Branch Management

**ALWAYS use short-lived feature branches.** Each ticket/issue gets its own branch.

**Starting work on a ticket:**

1. **Ensure you're on main and up to date:**
   ```bash
   git checkout main
   git pull origin main
   ```

2. **Create a new branch from main:**
   ```bash
   git checkout -b <descriptive-branch-name>
   ```
   - Branch naming: `<type>/<short-description>` or `<issue-id>/<short-description>`
   - Examples: `feature/add-timer`, `bug/fix-parser`, `reckon-123/implement-analytics`

3. **Work in that branch** - All commits for the ticket stay in this branch

4. **When done:**
   - Push the branch: `git push -u origin <branch-name>`
   - Create a PR to merge into main
   - Delete the branch after merging (GitHub does this automatically)

**CRITICAL RULES:**
- ONE branch per ticket/issue
- ALWAYS branch from main (not from other feature branches)
- Keep branches short-lived (hours to days, not weeks)
- Name branches with short, meaningful names related to the changes in the branch
- Merge to main frequently via PRs
- Delete branches after merging

## Landing the Plane (Session Completion)

**When ending a work session**, you MUST complete ALL steps below. Work is NOT complete until `git push` succeeds.

**MANDATORY WORKFLOW:**

1. **File issues for remaining work** - Create issues for anything that needs follow-up
2. **Run quality gates** (if code changed) - Tests, linters, builds
3. **Update issue status** - Close finished work, update in-progress items
4. **PUSH TO REMOTE** - This is MANDATORY:
   ```bash
   git pull --rebase
   bd sync
   git push
   git status  # MUST show "up to date with origin"
   ```
5. **Clean up** - Clear stashes, prune remote branches
6. **Verify** - All changes committed AND pushed
7. **Hand off** - Provide context for next session

**CRITICAL RULES:**
- Work is NOT complete until `git push` succeeds
- NEVER stop before pushing - that leaves work stranded locally
- NEVER say "ready to push when you are" - YOU must push
- If push fails, resolve and retry until it succeeds
