---
status: accepted
date: 2026-08-27
---

# 3. First-class specs as living canonical documents

## Context

The original conceptual model (`docs/ANALYSIS.md` §5) deliberately left
"spec" out as a first-class entity: a feature's intent was meant to live in
a workitem's body. In practice, the user's real workflow turned out to be
spec-driven, and workitem bodies aren't enough for large intents. The
ecosystem's dominant failure mode here is well documented (Spec Kit,
OpenSpec): specs accumulated per-feature, with no canonical current-state
layer, drift and pile up over time (`docs/ANALYSIS.md` §3).

## Decision

Specs are first-class, living canonical documents in `.atlas/specs/`: one
per capability/area, with status `draft → active → superseded`, updated in
place — git provides the delta history, not a `changes/` staging directory.
Workitems link to a spec via a `spec:` field; the compiled context for a
target task includes that task's spec.

Specs must reference at least one decision (a card of type `decision`, or an
existing ADR path) to be activated — this keeps specs anchored to a rationale
instead of floating free. Implementation details: `docs/SPEC.md` §S9.

**Never** spec-per-feature accumulation: there is exactly one spec per
capability/area, superseded in place, not archived into a growing pile.

## Consequences

- `atlas doctor` enforces the decision-reference invariant on active specs.
- The context compiler gains a spec section for target-mode briefs
  (`docs/SPEC.md` §S9).
- This reopens and resolves what §16 (point 3) of `docs/ANALYSIS.md` had
  flagged as a possible future evolution — it is no longer future, it is in
  scope.

## Source

`docs/ANALYSIS.md` §16 (point 3, "promoted and decided").
