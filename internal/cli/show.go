package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/danielino/atlas/internal/ledger"
)

func newShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <id>",
		Short: "Print a workitem or card in full",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)
			id := args[0]

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			path, kind, err := findLedgerFile(root, id)
			if err != nil {
				return fail(cmd, 2, useJSON,
					fmt.Sprintf("atlas: no such workitem, card or spec: %s", id),
					map[string]any{"error": "not_found", "id": id})
			}

			if !useJSON {
				data, err := os.ReadFile(path)
				if err != nil {
					return failIO(cmd, useJSON, err)
				}
				fmt.Fprint(cmd.OutOrStdout(), string(data))
				return nil
			}

			var doc map[string]any
			switch kind {
			case "task":
				w, err := ledger.LoadWorkitem(root, id)
				if err != nil {
					return failIO(cmd, useJSON, err)
				}
				doc = map[string]any{
					"kind":            "task",
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
				}
			case "card":
				c, err := ledger.LoadCard(root, id)
				if err != nil {
					return failIO(cmd, useJSON, err)
				}
				doc = map[string]any{
					"kind":          "card",
					"id":            c.ID,
					"type":          c.Type,
					"title":         c.Title,
					"status":        c.Status,
					"superseded_by": c.SupersededBy,
					"hook":          c.Hook,
					"created":       c.Created,
					"evidence":      c.Evidence,
					"body":          c.Body,
				}
			case "spec":
				s, err := ledger.LoadSpec(root, id)
				if err != nil {
					return failIO(cmd, useJSON, err)
				}
				doc = map[string]any{
					"kind":          "spec",
					"id":            s.ID,
					"title":         s.Title,
					"status":        s.Status,
					"superseded_by": s.SupersededBy,
					"created":       s.Created,
					"evidence":      s.Evidence,
					"decisions":     s.Decisions,
					"body":          s.Body,
				}
			}

			data, err := json.MarshalIndent(doc, "", "  ")
			if err != nil {
				return failIO(cmd, true, err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

// findLedgerFile locates the file for id under .atlas/work, .atlas/cards
// or .atlas/specs (searched in that order) and reports which kind it is.
func findLedgerFile(root, id string) (path string, kind string, err error) {
	for _, sub := range []struct {
		dir  string
		kind string
	}{
		{"work", "task"},
		{"cards", "card"},
		{"specs", "spec"},
	} {
		dir := filepath.Join(root, ".atlas", sub.dir)
		entries, readErr := os.ReadDir(dir)
		if readErr != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			if fileID(name) == id {
				return filepath.Join(dir, name), sub.kind, nil
			}
		}
	}
	return "", "", ledger.ErrNotFound
}
