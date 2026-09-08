# Claude Code Entry Point for Beads

This file is intentionally short. Do not copy workflow, build, storage, or UI
rules here; those details drift quickly when repeated across agent entrypoints.

## Read First

- **Workflow and safety**: [AGENTS.md](AGENTS.md)
- **Detailed agent operations**: [AGENT_INSTRUCTIONS.md](AGENT_INSTRUCTIONS.md)
- **Architecture orientation**: [engdocs/CLAUDE.md](engdocs/CLAUDE.md)
- **PR maintenance policy**: [PR_MAINTAINER_GUIDELINES.md](PR_MAINTAINER_GUIDELINES.md)

## Current Ground Rules

- Run `bd prime` before doing tracked work.
- Follow `go.mod` and [AGENT_INSTRUCTIONS.md](AGENT_INSTRUCTIONS.md) for build
  and test commands; do not hard-code toolchain versions here.
- Beads uses Dolt as the issue database. Use `bd dolt push` / `bd dolt pull`
  for issue data sync; do not use export/import as a routine git workflow.
- The CLI Visual Design System lives in
  [AGENT_INSTRUCTIONS.md](AGENT_INSTRUCTIONS.md#visual-design-system).
- If this file conflicts with a linked source, trust the linked source and fix
  this file by removing the duplicate.

## Houston Merge role authority (repository opt-in)

This repository opts in to the Team-maintainer profile for one role only: a session acting as the
Houston **Merge** role. That session may rebase, commit, push to the integration branch (the branch
`origin/HEAD` resolves to), close beads and write metrics for a candidate that carries an APPROVE
verdict artefact, under the merge rules: fetch first; recompute the candidate's content id and halt on
a mismatch; run every gate on the rebased tip; a single-line commit message taken from the review; no
attribution, Co-Authored-By or session trailers of any kind; verify the landed SHA from the remote
before any tracker write; never force-push, never rewrite published history, never touch another
worktree's uncommitted work. Every other session remains Conservative. Recorded by Daniel Niasoff on
2026-09-08: "authorized - nothing should wait on me".
