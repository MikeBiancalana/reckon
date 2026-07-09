# Agent Quick Start

Read this first. Full workflow details in [AGENTS.md](AGENTS.md).

## What is Reckon?

UNIX-composable personal-knowledge tools (log / todo / note / checklist) over plain-text markdown +
git, with a derived SQLite property-graph index. One multi-call `rk` binary. **Current design:
[docs/design/composable-redesign.md](docs/design/composable-redesign.md); doc map:
[docs/design/INDEX.md](docs/design/INDEX.md).** Parts of the tree are gen-1 code awaiting the T9
truth-inversion — the design doc wins on conflict.

## Codebase Structure

```
cmd/rk/           CLI entry point (main.go)
internal/
  cli/            Command handlers (add, edit, list, etc.)
  config/         Configuration loading
  journal/        Journal file parsing and writing
  storage/        SQLite database layer
  sync/           Journal-to-database sync
  task/           Task data model and operations
  time/           Time tracking (planned feature)
  tui/            Terminal UI components
tests/            Integration tests
docs/             Documentation and plans
```

## Key Files

| Purpose | File |
|---------|------|
| Product overview | `README.md` |
| Journal format | `internal/journal/parser.go` |
| Database schema | `internal/storage/db.go` |
| TUI entry point | `internal/tui/app.go` |
| Task model | `internal/task/task.go` |
| CLI commands | `internal/cli/*.go` |

## Build & Test

```bash
go build -o rk ./cmd/rk    # Build
go test ./...              # Test all
go test -run TestName ./internal/journal/  # Test specific
go vet ./...               # Lint
go fmt ./...               # Format
```

## Finding Work

```bash
bd ready                   # Show unblocked issues
bd show <id>               # View issue details
bd update <id> --claim     # Claim issue (atomic)
```

## Essential Commands

| Task | Command |
|------|---------|
| Find work | `bd ready` |
| Claim issue | `bd update <id> --claim` |
| Create issue | `bd create "Title" --type task --priority 2` |
| Close issue | `bd close <id>` |
| Sync beads | `bd sync` |
| Build | `go build -o rk ./cmd/rk` |
| Test | `go test ./...` |

## Session End Checklist

Before saying "done":

```bash
git status                 # Check changes
git add <files>            # Stage code
bd sync                    # Sync beads
git commit -m "..."        # Commit
git push                   # Push to remote
```

Work is NOT complete until `git push` succeeds.

## More Info

- **Full workflow**: [AGENTS.md](AGENTS.md)
- **Beads reference**: [docs/bd-usage.md](docs/bd-usage.md)
- **Architecture**: [docs/design/composable-redesign.md](docs/design/composable-redesign.md) (doc map: [docs/design/INDEX.md](docs/design/INDEX.md))
