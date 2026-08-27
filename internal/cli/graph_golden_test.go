package cli

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/danielino/atlas/internal/ledger"
	"github.com/danielino/atlas/internal/state"
	"github.com/stretchr/testify/require"
)

var updateGraphGolden = flag.Bool("update-graph-golden", false, "update graph golden files")

func compareGraphGolden(t *testing.T, name string, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)

	if *updateGraphGolden {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}

	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden file %s missing; rerun with -update-graph-golden to create it", path)
	require.Equal(t, string(want), got, "golden mismatch for %s (rerun with -update-graph-golden to inspect/regenerate)", path)
}

// graphFixture returns a fixed, three-level dependency graph fixture (a
// diamond plus a cycle) so text/mermaid golden output is deterministic
// regardless of id generation.
func graphFixtureLevelsAndCycle() (levels [][]state.GraphNode, cycle []state.GraphNode) {
	items := []ledger.Workitem{
		{ID: "a1b2", Title: "root task", Status: "doing"},
		{ID: "b1b2", Title: "second task", Status: "todo", BlockedBy: []string{"a1b2"}},
		{ID: "c1c2", Title: "third task", Status: "todo", BlockedBy: []string{"a1b2"}},
		{ID: "d1d2", Title: "fourth task", Status: "blocked", BlockedBy: []string{"b1b2", "c1c2"}},
		{ID: "x1x2", Title: "cyclic x", Status: "todo", BlockedBy: []string{"y1y2"}},
		{ID: "y1y2", Title: "cyclic y", Status: "todo", BlockedBy: []string{"x1x2"}},
	}
	return state.GraphLevels(items)
}

func TestRenderGraphText_Golden(t *testing.T) {
	levels, cycle := graphFixtureLevelsAndCycle()
	got := renderGraphText(levels, cycle)
	compareGraphGolden(t, "graph.text.golden", got)
}

func TestRenderGraphMermaid_Golden(t *testing.T) {
	levels, cycle := graphFixtureLevelsAndCycle()
	got := renderGraphMermaid(levels, cycle)
	compareGraphGolden(t, "graph.mermaid.golden", got)
}
