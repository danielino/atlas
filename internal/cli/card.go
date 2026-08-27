package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/danielino/atlas/internal/ledger"
)

func newCardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "card",
		Short: "Manage decision/knowledge cards",
	}
	cmd.AddCommand(newCardAddCmd())
	cmd.AddCommand(newCardSupersedeCmd())
	return cmd
}

func newCardAddCmd() *cobra.Command {
	var cardType, hook, body, evidence string
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a new decision or knowledge card",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)
			title := args[0]

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			if !ledger.IsValidCardType(cardType) {
				return fail(cmd, 2, useJSON,
					fmt.Sprintf("atlas: invalid --type %q: must be \"decision\" or \"knowledge\"", cardType),
					map[string]any{"error": "invalid_type", "type": cardType})
			}

			cfg, err := ledger.LoadConfig(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			// card add is a plan mutation regardless of any --from-like
			// escape: policy always applies.
			if err := checkPolicy(cmd, root, cfg, useJSON); err != nil {
				return err
			}

			effectiveHook := hook
			if effectiveHook == "" {
				effectiveHook = title
			}

			id, err := ledger.GenerateID(root, ledger.RandReader)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			c := ledger.Card{
				ID:       id,
				Type:     cardType,
				Title:    title,
				Status:   "active",
				Hook:     effectiveHook,
				Created:  time.Now().UTC().Format("2006-01-02"),
				Evidence: splitCSV(evidence),
				Body:     body,
			}

			if err := ledger.SaveCard(root, c); err != nil {
				return failIO(cmd, useJSON, err)
			}

			printCreated(cmd, useJSON, id)
			return nil
		},
	}
	cmd.Flags().StringVar(&cardType, "type", "", `card type: "decision" or "knowledge" (required)`)
	cmd.Flags().StringVar(&hook, "hook", "", "one-line index hook (defaults to the title)")
	cmd.Flags().StringVar(&body, "body", "", "card body")
	cmd.Flags().StringVar(&evidence, "evidence", "", "comma-separated evidence paths")
	addJSONFlag(cmd)
	return cmd
}

func newCardSupersedeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "supersede <old-id> <new-id>",
		Short: "Mark an old card superseded by a new one",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)
			oldID, newID := args[0], args[1]

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			cfg, err := ledger.LoadConfig(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			// Validate both ids before applying policy, so a doomed
			// invocation never emits a spurious policy warning first
			// (consistent with card add's type-check-before-policy order).
			old, err := ledger.LoadCard(root, oldID)
			if err != nil {
				if errors.Is(err, ledger.ErrNotFound) {
					return fail(cmd, 2, useJSON,
						fmt.Sprintf("atlas: no such card: %s", oldID),
						map[string]any{"error": "not_found", "id": oldID})
				}
				return failIO(cmd, useJSON, err)
			}

			if _, err := ledger.LoadCard(root, newID); err != nil {
				if errors.Is(err, ledger.ErrNotFound) {
					return fail(cmd, 2, useJSON,
						fmt.Sprintf("atlas: no such card: %s", newID),
						map[string]any{"error": "not_found", "id": newID})
				}
				return failIO(cmd, useJSON, err)
			}

			if err := checkPolicy(cmd, root, cfg, useJSON); err != nil {
				return err
			}

			old.Status = "superseded"
			old.SupersededBy = newID
			if err := ledger.SaveCard(root, old); err != nil {
				return failIO(cmd, useJSON, err)
			}

			entry := ledger.LogEntry{
				ID:           old.ID,
				Kind:         "card",
				Title:        old.Title,
				Closed:       time.Now().UTC().Format(time.RFC3339),
				SupersededBy: newID,
			}
			if err := ledger.AppendLog(root, entry); err != nil {
				return failIO(cmd, useJSON, err)
			}

			printCreated(cmd, useJSON, oldID)
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}
