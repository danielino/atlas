---
status: accepted
date: 2026-08-27
---

# 1. ATLAS is standalone; not built on or wrapping Beads

## Context

Beads (Steve Yegge) is the closest prior art: a git-native issue tracker for
agents that already solves the task DAG, ready-work detection (`bd ready`)
and closed-task compaction ("memory decay"). One alternative on the table was
to adopt Beads plus a couple of conventions (a MADR-compatible decision
format, a `focus.md`) instead of building new software.

## Decision

ATLAS is a standalone tool. It does not wrap Beads and does not depend on it.
It owns its own minimal task layer (`workitem`), deliberately poorer than a
full issue tracker.

From Beads, ATLAS reuses only design lessons: collision-resistant hash ids,
ready-work detection, and a 1-line summary written at close time. It does not
reuse Beads's software or storage format.

## Alternatives considered

- **Adopt Beads + conventions, build nothing new.** Rejected: Beads has no
  context-compilation layer (`bd prime` is task-centric) and no
  decision/knowledge model. Building ATLAS on top of Beads would make it a
  wrapper around another project's design choices — including its ongoing
  storage migration (JSONL → Dolt), which would constrain ATLAS's evolution
  for no benefit.

## Consequences

- ATLAS accepts the cost of reimplementing a simple task layer that Beads
  already provides, in exchange for full control over the data model needed
  for state + decisions + context compilation — the layer Beads doesn't
  cover and that is ATLAS's actual differentiator.
- ATLAS workitems stay deliberately minimal (no assignees, sprints, or
  elaborate priorities); a team that needs a full issue tracker should use
  Beads or a real tracker alongside ATLAS, not instead of it.

## Source

`docs/ANALYSIS.md` §1 (point 2), §14 (alternative 1), §17 (open question 5).
