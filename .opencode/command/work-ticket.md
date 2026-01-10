---
description: Work on a ready ticket from start to PR
agent: Build
---

Check for tickets that are ready using `bd ready`. Pick an available ticket (atomically claim it with `bd update <id> --claim`), refine its description if needed for clarity, then work on it in a feature branch following the project guidelines.

Use the Build agent to implement the feature. Commit progress incrementally with clear, concise commit messages following the repository's style. After each significant commit, have the code-reviewer agent review the code changes, addressing any issues or suggestions until satisfied.

Once complete, push the branch and create a PR using `gh pr create --base main`. Squash and merge the PR using `gh pr merge <number> --squash`. Mark the ticket as done with `bd close <id>` and sync beads.

If no ready tickets, inform the user.
