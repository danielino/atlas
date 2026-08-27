package state

import (
	"sort"

	"github.com/danielino/atlas/internal/ledger"
)

// GraphNode is one workitem's position in the dependency graph (PLAN.md
// S10.1): the workitem itself plus ActiveBlockedBy, its blocked_by ids
// restricted to those naming another currently-active workitem (closed or
// nonexistent ids never block, same semantics as Ready) and sorted for
// deterministic display. Rendering code (text, mermaid, JSON) reads this
// field rather than re-filtering BlockedBy itself, so the "closed/
// nonexistent ids don't block" rule lives in exactly one place.
type GraphNode struct {
	ledger.Workitem
	ActiveBlockedBy []string
}

// GraphLevels computes the topological levels of the blocked_by DAG among
// the given (active) workitems, for `atlas graph` (PLAN.md S10.1). Level 0
// holds every workitem with no active blocker; level N holds workitems
// whose active blockers are all in levels < N. A blocked_by id that does
// not name one of the given workitems (closed or nonexistent) is not an
// active blocker and never keeps a workitem out of level 0 — the same
// semantics as Ready, applied without needing a separate closedIDs set:
// since workitems is by construction the currently-active set, anything
// not in it is, for this purpose, indistinguishable from closed.
//
// Levels are each sorted by id for deterministic output. Any workitems
// that cannot be placed because they sit on (or downstream of) a
// blocked_by cycle are returned separately in cycle (also sorted by id)
// rather than being silently dropped: `atlas graph` renders them as a
// trailing "Cycle (unresolvable)" group and warns on stderr; judging the
// cycle an error is `atlas doctor`'s job, not graph's.
func GraphLevels(workitems []ledger.Workitem) (levels [][]GraphNode, cycle []GraphNode) {
	byID := make(map[string]ledger.Workitem, len(workitems))
	for _, w := range workitems {
		byID[w.ID] = w
	}

	activeBlockers := make(map[string][]string, len(workitems))
	for _, w := range workitems {
		var deps []string
		for _, dep := range w.BlockedBy {
			if _, ok := byID[dep]; ok {
				deps = append(deps, dep)
			}
		}
		sort.Strings(deps)
		activeBlockers[w.ID] = deps
	}

	toNode := func(id string) GraphNode {
		return GraphNode{Workitem: byID[id], ActiveBlockedBy: activeBlockers[id]}
	}

	resolved := make(map[string]bool, len(workitems))
	remaining := make(map[string]bool, len(workitems))
	for _, w := range workitems {
		remaining[w.ID] = true
	}

	for len(remaining) > 0 {
		var levelIDs []string
		for id := range remaining {
			ready := true
			for _, dep := range activeBlockers[id] {
				if !resolved[dep] {
					ready = false
					break
				}
			}
			if ready {
				levelIDs = append(levelIDs, id)
			}
		}
		if len(levelIDs) == 0 {
			break // no more progress possible: whatever remains sits on a cycle
		}
		sort.Strings(levelIDs)

		levelNodes := make([]GraphNode, 0, len(levelIDs))
		for _, id := range levelIDs {
			levelNodes = append(levelNodes, toNode(id))
			resolved[id] = true
			delete(remaining, id)
		}
		levels = append(levels, levelNodes)
	}

	if len(remaining) > 0 {
		ids := make([]string, 0, len(remaining))
		for id := range remaining {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			cycle = append(cycle, toNode(id))
		}
	}

	return levels, cycle
}
