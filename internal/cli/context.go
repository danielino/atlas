package cli

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/danielino/atlas/internal/contextc"
	"github.com/danielino/atlas/internal/ledger"
	"github.com/danielino/atlas/internal/state"
)

func newContextCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "context [id]",
		Short: "Print the budgeted ATLAS context brief",
		Args:  cobra.MaximumNArgs(1),
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

			st, err := state.Build(root, cfg, state.Options{})
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			if len(args) == 1 {
				return runContextTarget(cmd, root, args[0], st, cfg, useJSON)
			}

			if useJSON {
				data, err := contextc.RenderJSON(st, cfg, nil)
				if err != nil {
					return failIO(cmd, true, err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}
			fmt.Fprint(cmd.OutOrStdout(), contextc.Render(st, cfg, nil))
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func runContextTarget(cmd *cobra.Command, root, id string, st state.State, cfg ledger.Config, useJSON bool) error {
	w, err := ledger.LoadWorkitem(root, id)
	if err != nil {
		if errors.Is(err, ledger.ErrNotFound) {
			return fail(cmd, 2, useJSON,
				fmt.Sprintf("atlas: no such workitem: %s", id),
				map[string]any{"error": "not_found", "id": id})
		}
		return failIO(cmd, useJSON, err)
	}

	cards, err := ledger.ListActiveCards(root)
	if err != nil {
		return failIO(cmd, useJSON, err)
	}

	var spec *ledger.Spec
	if w.Spec != "" {
		loaded, err := ledger.LoadSpec(root, w.Spec)
		if err == nil {
			spec = &loaded
		} else if !errors.Is(err, ledger.ErrNotFound) {
			return failIO(cmd, useJSON, err)
		}
		// A dangling spec reference is a doctor-level problem, not a
		// reason to fail `context`: just render without it.
	}

	if useJSON {
		related := relatedCardsFor(w, cards)
		doc := map[string]any{
			"focus": st.Focus,
			"task": map[string]any{
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
				"spec":            w.Spec,
				"body":            w.Body,
			},
			"rules": relatedCardsJSON(related),
			"ground": map[string]any{
				"branch":    st.Ground.Branch,
				"head":      st.Ground.Head,
				"dirty":     st.Ground.Dirty,
				"elsewhere": st.Ground.Elsewhere,
			},
		}
		if spec != nil {
			doc["spec"] = map[string]any{
				"id":     spec.ID,
				"title":  spec.Title,
				"status": spec.Status,
				"body":   spec.Body,
			}
		}
		data, err := json.MarshalIndent(doc, "", "  ")
		if err != nil {
			return failIO(cmd, true, err)
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}

	fmt.Fprint(cmd.OutOrStdout(), contextc.RenderTarget(st, w, cards, spec, cfg, nil))
	return nil
}

func relatedCardsJSON(cards []ledger.Card) []map[string]any {
	out := make([]map[string]any, 0, len(cards))
	for _, c := range cards {
		out = append(out, map[string]any{"id": c.ID, "hook": c.Hook, "type": c.Type})
	}
	return out
}
