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

	if st.Ground.Branch != "" {
		worktree := "clean"
		if st.Ground.Dirty {
			worktree = fmt.Sprintf("dirty(%d files)", st.Ground.DirtyCount)
		}
		line := fmt.Sprintf("branch: %s · HEAD: %s · worktree: %s", st.Ground.Branch, st.Ground.Head, worktree)
		if len(st.Ground.Elsewhere) > 0 {
			parts := make([]string, len(st.Ground.Elsewhere))
			for i, e := range st.Ground.Elsewhere {
				parts[i] = e.ID + " su " + e.Branch
			}
			line += " · elsewhere: [" + strings.Join(parts, ", ") + "]"
		}
		b.WriteString("## GROUND\n" + line + "\n")
	}

	return b.String()
}

func renderStateJSON(st state.State, workitems []ledger.Workitem) ([]byte, error) {
	doc := map[string]any{
		"focus":         st.Focus,
		"workitems":     workitems,
		"active_cards":  st.ActiveCards,
		"ground":        st.Ground,
		"stale":         st.Stale,
		"recent_closed": st.RecentClosed,
	}
	return json.MarshalIndent(doc, "", "  ")
}
