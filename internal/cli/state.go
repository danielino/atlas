package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dmarcocci/atlas/internal/ledger"
	"github.com/dmarcocci/atlas/internal/state"
)

func newStateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Print the full, unbudgeted project state",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)
			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			cfg, err := ledger.LoadConfig(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			workitems, err := ledger.ListWorkitems(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			st, err := state.Build(root, cfg, state.Options{})
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			if useJSON {
				data, err := renderStateJSON(st, workitems)
				if err != nil {
					return failIO(cmd, true, err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			fmt.Fprint(cmd.OutOrStdout(), renderStateText(st, workitems))
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func renderStateText(st state.State, workitems []ledger.Workitem) string {
	var b strings.Builder

	b.WriteString("# ATLAS STATE")
	if st.Stale {
		b.WriteString(" [STALE: ledger older than last 5 commits]")
	}
	b.WriteString("\n")

	if focus := strings.TrimRight(st.Focus, "\n"); focus != "" {
		b.WriteString("## FOCUS\n" + focus + "\n")
	}

	byStatus := map[string][]ledger.Workitem{}
	for _, w := range workitems {
		byStatus[w.Status] = append(byStatus[w.Status], w)
	}

	b.WriteString("## WORKITEMS\n")
	for _, status := range []string{"todo", "doing", "blocked", "done"} {
		items := byStatus[status]
		if len(items) == 0 {
			continue
		}
		b.WriteString("### " + status + "\n")
		for _, w := range items {
			b.WriteString("- [" + w.ID + "] " + w.Title)
			if w.Branch != "" {
				b.WriteString(" (branch " + w.Branch + ")")
			}
			b.WriteString("\n")
		}
	}

	if len(st.ActiveCards) > 0 {
		b.WriteString("## CARDS\n")
		for _, c := range st.ActiveCards {
			b.WriteString("- [" + c.ID + "] " + c.Hook + " (" + c.Type + ")\n")
		}
	}

	if len(st.Specs) > 0 {
		b.WriteString("## SPECS\n")
		for _, s := range st.Specs {
			line := fmt.Sprintf("- [%s] %s (%s, %d open tasks)", s.ID, s.Title, s.Status, s.OpenTasks)
			if len(s.Decisions) > 0 {
				line += " — decisions: " + strings.Join(s.Decisions, ", ")
			}
			b.WriteString(line + "\n")
		}
	}

	if st.Ground.Branch != "" {
		worktree := "clean"
		if st.Ground.Dirty {
			worktree = fmt.Sprintf("dirty(%d files)", st.Ground.DirtyCount)
		}
		line := fmt.Sprintf("branch: %s · HEAD: %s · worktree: %s", st.Ground.Branch, st.Ground.Head, worktree)
		if len(st.Ground.Elsewhere) > 0 {
			parts := make([]string, len(st.Ground.Elsewhere))
			for i, e := range st.Ground.Elsewhere {
				parts[i] = e.ID + " on " + e.Branch
			}
			line += " · elsewhere: [" + strings.Join(parts, ", ") + "]"
		}
		b.WriteString("## GROUND\n" + line + "\n")
	}

	return b.String()
}

func renderStateJSON(st state.State, workitems []ledger.Workitem) ([]byte, error) {
	items := make([]map[string]any, 0, len(workitems))
	for _, w := range workitems {
		items = append(items, map[string]any{
			"id":              w.ID,
			"title":           w.Title,
			"status":          w.Status,
			"created":         w.Created,
			"blocked_by":      w.BlockedBy,
			"discovered_from": w.DiscoveredFrom,
			"branch":          w.Branch,
			"evidence":        w.Evidence,
			"summary":         w.Summary,
			"reason":          w.Reason,
		})
	}

	cards := make([]map[string]any, 0, len(st.ActiveCards))
	for _, c := range st.ActiveCards {
		cards = append(cards, map[string]any{
			"id":            c.ID,
			"type":          c.Type,
			"title":         c.Title,
			"hook":          c.Hook,
			"created":       c.Created,
			"evidence":      c.Evidence,
			"superseded_by": c.SupersededBy,
		})
	}

	elsewhere := make([]map[string]any, 0, len(st.Ground.Elsewhere))
	for _, e := range st.Ground.Elsewhere {
		elsewhere = append(elsewhere, map[string]any{"id": e.ID, "branch": e.Branch})
	}

	specs := make([]map[string]any, 0, len(st.Specs))
	for _, s := range st.Specs {
		decisions := s.Decisions
		if decisions == nil {
			decisions = []string{}
		}
		specs = append(specs, map[string]any{
			"id":         s.ID,
			"title":      s.Title,
			"status":     s.Status,
			"open_tasks": s.OpenTasks,
			"decisions":  decisions,
		})
	}

	recent := st.RecentClosed
	if recent == nil {
		recent = []ledger.LogEntry{}
	}

	doc := map[string]any{
		"focus":        st.Focus,
		"workitems":    items,
		"active_cards": cards,
		"specs":        specs,
		"ground": map[string]any{
			"branch":      st.Ground.Branch,
			"head":        st.Ground.Head,
			"dirty":       st.Ground.Dirty,
			"dirty_count": st.Ground.DirtyCount,
			"elsewhere":   elsewhere,
		},
		"stale":         st.Stale,
		"recent_closed": recent,
	}
	return json.MarshalIndent(doc, "", "  ")
}
