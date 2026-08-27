---
status: accepted
date: 2026-08-27
---

# 2. Go binary, git via subprocess

## Context

The CLI is invoked dozens of times per session by agents. Interpreter
startup cost and dependency management for an interpreted language are
unacceptable at that call frequency, so a native compiled binary was already
a settled constraint. What remained open was which language, and how to talk
to git.

## Decision

Implement ATLAS as a single static Go binary, and interact with git by
shelling out to the system `git` binary (as `gh` does), never through an
embedded git library.

**Go**, over the alternatives:
- Native ecosystem for this category of tool (gh, kubectl, terraform).
- Trivial cross-compilation across the macOS/Linux/Windows matrix.
- ~5ms startup, single static binary, low barrier for contributors.
- Rust was considered and rejected: its main advantage (performance/memory)
  is irrelevant for this workload, at the cost of a steeper contributor
  barrier.

**git via subprocess**, over an embedded library (e.g. `go-git`):
- True worktree and `--git-common-dir` semantics are required (see the
  claims mechanism in §12.1 of `docs/ANALYSIS.md`), and `go-git`'s worktree
  support is incomplete.

## Consequences

- ATLAS requires a `git` binary on PATH at runtime; it is not a fully
  self-contained dependency-free tool in that narrow sense.
- Git-related behavior inherits whatever the installed `git` version
  supports, which is the same trade-off `gh` makes.
- Tests exercise real git repositories via subprocess (`internal/testutil`),
  not a mocked git library.

## Source

`docs/ANALYSIS.md` §17 (open question 6, "partially resolved" → resolved
here).
