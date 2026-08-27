package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/danielino/atlas/internal/ledger"
	"github.com/danielino/atlas/internal/state"
)

// newGraphCmd implements `atlas graph` (PLAN.md S10.1): a read-only,
// opt-in-for-humans view of the blocked_by DAG over active workitems. It
// is never part of `atlas context` output. Filtering out closed/
// nonexistent blockers ("active" blockers) is done once, in
// state.GraphLevels; every renderer below just reads GraphNode.ActiveBlockedBy.
func newGraphCmd() *cobra.Command {
	var mermaid bool
	cmd := &cobra.Command{
		Use:   "graph",
		Short: "Render the workitem dependency graph (blocked_by), read-only",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			workitems, err := ledger.ListWorkitems(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			levels, cycle := state.GraphLevels(workitems)
			if len(cycle) > 0 {
				fmt.Fprintln(cmd.ErrOrStderr(), "atlas: warning: blocked_by cycle detected among active workitems; run `atlas doctor` for details")
			}

			if useJSON {
				data, err := renderGraphJSON(levels, cycle)
				if err != nil {
					return failIO(cmd, true, err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			if mermaid {
				fmt.Fprint(cmd.OutOrStdout(), renderGraphMermaid(levels, cycle))
				return nil
			}

			fmt.Fprint(cmd.OutOrStdout(), renderGraphText(levels, cycle))
			return nil
		},
	}
	cmd.Flags().BoolVar(&mermaid, "mermaid", false, "render as a mermaid flowchart instead of text levels")
	addJSONFlag(cmd)
	return cmd
}

// nodeLine formats one node's line for the text renderer:
// "- [id] title (status)" or, when it has active blockers,
// "- [id] title (status, blocked by id1, id2)".
func nodeLine(n state.GraphNode) string {
	if len(n.ActiveBlockedBy) == 0 {
		return fmt.Sprintf("- [%s] %s (%s)", n.ID, n.Title, n.Status)
	}
	return fmt.Sprintf("- [%s] %s (%s, blocked by %s)", n.ID, n.Title, n.Status, strings.Join(n.ActiveBlockedBy, ", "))
}

func renderGraphText(levels [][]state.GraphNode, cycle []state.GraphNode) string {
	var b strings.Builder
	b.WriteString("# ATLAS GRAPH\n")

	if len(levels) == 0 && len(cycle) == 0 {
		b.WriteString("no active workitems\n")
		return b.String()
	}

	for i, level := range levels {
		if i == 0 {
			b.WriteString("Level 0 (unblocked, parallelizable):\n")
		} else {
			b.WriteString(fmt.Sprintf("Level %d:\n", i))
		}
		for _, n := range level {
			b.WriteString(nodeLine(n) + "\n")
		}
	}

	if len(cycle) > 0 {
		b.WriteString("Cycle (unresolvable):\n")
		for _, n := range cycle {
			b.WriteString(fmt.Sprintf("- [%s] %s (%s)\n", n.ID, n.Title, n.Status))
		}
	}

	return b.String()
}

// renderGraphMermaid renders the same graph as a `flowchart TD` (S10.1):
// one node declaration per workitem (levels then cycle, in that order,
// each already id-sorted) followed by one edge per active blocked_by
// relationship.
func renderGraphMermaid(levels [][]state.GraphNode, cycle []state.GraphNode) string {
	var all []state.GraphNode
	for _, level := range levels {
		all = append(all, level...)
	}
	all = append(all, cycle...)

	var b strings.Builder
	b.WriteString("flowchart TD\n")
	for _, n := range all {
		b.WriteString(fmt.Sprintf("    %s[\"%s: %s (%s)\"]\n", n.ID, n.ID, n.Title, n.Status))
	}
	for _, n := range all {
		for _, blocker := range n.ActiveBlockedBy {
			b.WriteString(fmt.Sprintf("    %s --> %s\n", blocker, n.ID))
		}
	}
	return b.String()
}

func renderGraphJSON(levels [][]state.GraphNode, cycle []state.GraphNode) ([]byte, error) {
	toNode := func(n state.GraphNode) map[string]any {
		blockers := n.ActiveBlockedBy
		if blockers == nil {
			blockers = []string{}
		}
		return map[string]any{
			"id":         n.ID,
			"title":      n.Title,
			"status":     n.Status,
			"blocked_by": blockers,
		}
	}

	jsonLevels := make([][]map[string]any, 0, len(levels))
	for _, level := range levels {
		nodes := make([]map[string]any, 0, len(level))
		for _, n := range level {
			nodes = append(nodes, toNode(n))
		}
		jsonLevels = append(jsonLevels, nodes)
	}

	jsonCycle := make([]map[string]any, 0, len(cycle))
	for _, n := range cycle {
		jsonCycle = append(jsonCycle, toNode(n))
	}

	return json.MarshalIndent(map[string]any{
		"levels": jsonLevels,
		"cycles": jsonCycle,
	}, "", "  ")
}
