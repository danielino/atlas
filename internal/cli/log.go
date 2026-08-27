package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/dmarcocci/atlas/internal/ledger"
)

func newLogCmd() *cobra.Command {
	var grep string
	cmd := &cobra.Command{
		Use:   "log",
		Short: "Query the closed-item log",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			entries, err := ledger.ReadLog(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}
			filtered := ledger.FilterLog(entries, grep)

			if useJSON {
				if filtered == nil {
					filtered = []ledger.LogEntry{}
				}
				data, err := json.MarshalIndent(filtered, "", "  ")
				if err != nil {
					return failIO(cmd, true, err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			for _, e := range filtered {
				line := "- [" + e.ID + "] " + e.Title
				if e.Summary != "" {
					line += " — " + e.Summary
				}
				line += " (" + e.Closed + ")"
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&grep, "grep", "", "filter entries by case-insensitive substring on title/summary")
	addJSONFlag(cmd)
	return cmd
}
