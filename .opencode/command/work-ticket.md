---
description: Work on a ready ticket from start to PR
agent: build
---

Check for tickets that are ready using `bd ready`. Pick an available ticket (atomically claim it with `bd update <id> --claim`), refine its description if needed for clarity.

Create a new worktree for the ticket under `~/repos/reckon/.worktrees` and create a corresponding feature branch, then switch to the worktree to work in isolation.

Use the Build agent to implement the feature. Commit progress incrementally with clear, concise commit messages following the repository's style. After each significant commit, have the code-reviewer agent review the code changes, addressing any issues or suggestions until satisfied.

Once complete, push the branch and create a PR using `gh pr create --base main`. Squash and merge the PR using `gh pr merge <number> --squash`. Clean up the worktree. Mark the ticket as done with `bd close <id>` and sync beads.

If no ready tickets, inform the user.
