package state

import (
	"testing"

	"github.com/danielino/atlas/internal/ledger"
	"github.com/stretchr/testify/require"
)

func ids(items []GraphNode) []string {
	out := make([]string, len(items))
	for i, w := range items {
		out[i] = w.ID
	}
	return out
}

func TestGraphLevels_SingleUnblocked(t *testing.T) {
	items := []ledger.Workitem{wi("a1b2", "todo")}
	levels, cycle := GraphLevels(items)
	require.Len(t, levels, 1)
	require.Equal(t, []string{"a1b2"}, ids(levels[0]))
	require.Empty(t, cycle)
}

func TestGraphLevels_MultiLevelChain(t *testing.T) {
	items := []ledger.Workitem{
		wi("a1b2", "doing"),
		wi("c3d4", "todo", "a1b2"),
		wi("e5f6", "todo", "c3d4"),
	}
	levels, cycle := GraphLevels(items)
	require.Empty(t, cycle)
	require.Len(t, levels, 3)
	require.Equal(t, []string{"a1b2"}, ids(levels[0]))
	require.Equal(t, []string{"c3d4"}, ids(levels[1]))
	require.Equal(t, []string{"e5f6"}, ids(levels[2]))
}

func TestGraphLevels_MultipleNodesSameLevel_SortedByID(t *testing.T) {
	items := []ledger.Workitem{
		wi("e5f6", "todo"),
		wi("a1b2", "todo"),
		wi("c3d4", "todo"),
	}
	levels, _ := GraphLevels(items)
	require.Len(t, levels, 1)
	require.Equal(t, []string{"a1b2", "c3d4", "e5f6"}, ids(levels[0]))
}

func TestGraphLevels_DiamondDependency(t *testing.T) {
	// a1b2 blocks both b and c; d depends on both b and c -> level 2.
	items := []ledger.Workitem{
		wi("a1b2", "todo"),
		wi("b1b2", "todo", "a1b2"),
		wi("c1c2", "todo", "a1b2"),
		wi("d1d2", "todo", "b1b2", "c1c2"),
	}
	levels, cycle := GraphLevels(items)
	require.Empty(t, cycle)
	require.Len(t, levels, 3)
	require.Equal(t, []string{"a1b2"}, ids(levels[0]))
	require.Equal(t, []string{"b1b2", "c1c2"}, ids(levels[1]))
	require.Equal(t, []string{"d1d2"}, ids(levels[2]))
}

func TestGraphLevels_BlockerClosedOrNonexistent_DoesNotBlock(t *testing.T) {
	// c3d4 is blocked_by a1b2 (closed, not in the active set) and by
	// "zzzz" (nonexistent): neither is present among items, so neither
	// blocks — c3d4 lands in level 0, same semantics as Ready.
	items := []ledger.Workitem{
		wi("c3d4", "todo", "a1b2", "zzzz"),
	}
	levels, cycle := GraphLevels(items)
	require.Empty(t, cycle)
	require.Len(t, levels, 1)
	require.Equal(t, []string{"c3d4"}, ids(levels[0]))
}

func TestGraphLevels_SimpleCycle_GoesToCycleGroup(t *testing.T) {
	items := []ledger.Workitem{
		wi("a1b2", "todo", "c3d4"),
		wi("c3d4", "todo", "a1b2"),
	}
	levels, cycle := GraphLevels(items)
	require.Empty(t, levels)
	require.Equal(t, []string{"a1b2", "c3d4"}, ids(cycle))
}

func TestGraphLevels_CycleAndCleanNodesCoexist(t *testing.T) {
	items := []ledger.Workitem{
		wi("a1b2", "todo"),
		wi("x1x2", "todo", "y1y2"),
		wi("y1y2", "todo", "x1x2"),
	}
	levels, cycle := GraphLevels(items)
	require.Len(t, levels, 1)
	require.Equal(t, []string{"a1b2"}, ids(levels[0]))
	require.Equal(t, []string{"x1x2", "y1y2"}, ids(cycle))
}

func TestGraphLevels_Empty(t *testing.T) {
	levels, cycle := GraphLevels(nil)
	require.Empty(t, levels)
	require.Empty(t, cycle)
}

func TestGraphLevels_ActiveBlockedBy_ExposedOnNode(t *testing.T) {
	items := []ledger.Workitem{
		wi("a1b2", "doing"),
		wi("c3d4", "todo", "a1b2", "zzzz"), // zzzz: not an active workitem, filtered out
	}
	levels, _ := GraphLevels(items)
	require.Len(t, levels, 2)
	require.Empty(t, levels[0][0].ActiveBlockedBy)
	require.Equal(t, []string{"a1b2"}, levels[1][0].ActiveBlockedBy)
}

func TestGraphLevels_DownstreamOfCycle_AlsoUnplaceable(t *testing.T) {
	// x/y form a cycle; z depends on x, so z can never be placed in a
	// level either (its blocker is never resolved) and must join the
	// cycle group rather than being silently dropped.
	items := []ledger.Workitem{
		wi("x1x2", "todo", "y1y2"),
		wi("y1y2", "todo", "x1x2"),
		wi("z1z2", "todo", "x1x2"),
	}
	levels, cycle := GraphLevels(items)
	require.Empty(t, levels)
	require.Equal(t, []string{"x1x2", "y1y2", "z1z2"}, ids(cycle))
}
