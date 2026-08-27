package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/danielino/atlas/internal/ledger"
	"github.com/danielino/atlas/internal/state"
)

func newSpecCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Manage living canonical specs",
	}
	cmd.AddCommand(newSpecAddCmd())
	cmd.AddCommand(newSpecActivateCmd())
	cmd.AddCommand(newSpecUpdateCmd())
	cmd.AddCommand(newSpecSupersedeCmd())
	cmd.AddCommand(newSpecListCmd())
	return cmd
}

// specScaffold is the default body for `spec add` when --body is omitted
// (SPEC.md S10.2): a spec-as-living-document template, distinct from
// aiops-ai-spec's per-feature template. Never applied when --body is
// given explicitly, and never re-synced afterward — the body is free text
// from that point on.
const specScaffold = `## Goal
<what this capability must achieve and for whom>

## Constraints
<hard limits, invariants, and the decisions this spec follows>

## Interfaces
<contracts, commands, data shapes exposed or consumed>

## Open questions
<unresolved points — resolve these before activating the spec>
`

// isDecisionID distinguishes a bare ATLAS id from a repo-relative ADR
// path in a spec's decisions list (S9.8): ids never contain a path
// separator or a file extension, while every ADR path in practice does
// (e.g. "docs/adr/0034-enrichment-stage.md").
func isDecisionID(entry string) bool {
	return !strings.ContainsAny(entry, "/.")
}

// specValidationError carries a semantic (exit 2) refusal produced by
// validateDecisions, kept separate from genuine I/O errors so callers can
// route each through fail()/failIO() respectively.
type specValidationError struct {
	message string
	payload map[string]any
}

// validateDecisions checks every decisions entry per S9.8: an id-shaped
// entry must resolve to an existing decision card (not any other card
// type); a path-shaped entry must exist on disk relative to root.
// checkSuperseded additionally refuses a decision card that is itself
// superseded — only enforced at `spec activate`, never at add/update (a
// draft may point at what is, for now, still a fine decision).
func validateDecisions(root string, entries []string, checkSuperseded bool) (*specValidationError, error) {
	for _, entry := range entries {
		if isDecisionID(entry) {
			card, err := ledger.LoadCard(root, entry)
			if err != nil {
				if errors.Is(err, ledger.ErrNotFound) {
					return &specValidationError{
						message: fmt.Sprintf("atlas: decision not found: %s", entry),
						payload: map[string]any{"error": "decision_not_found", "id": entry},
					}, nil
				}
				return nil, err
			}
			if card.Type != "decision" {
				return &specValidationError{
					message: fmt.Sprintf("atlas: %s is not a decision card", entry),
					payload: map[string]any{"error": "decision_not_found", "id": entry},
				}, nil
			}
			if checkSuperseded && card.Status == "superseded" {
				return &specValidationError{
					message: fmt.Sprintf("atlas: decision %s is superseded by %s", entry, card.SupersededBy),
					payload: map[string]any{"error": "decision_superseded", "id": entry, "superseded_by": card.SupersededBy},
				}, nil
			}
			continue
		}

		if _, err := os.Stat(filepath.Join(root, entry)); err != nil {
			if os.IsNotExist(err) {
				return &specValidationError{
					message: fmt.Sprintf("atlas: decision path not found: %s", entry),
					payload: map[string]any{"error": "decision_path_not_found", "path": entry},
				}, nil
			}
			return nil, err
		}
	}
	return nil, nil
}

func newSpecAddCmd() *cobra.Command {
	var body, evidence, decisions string
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a new draft spec",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)
			title := args[0]

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			decisionList := splitCSV(decisions)
			if verr, ioErr := validateDecisions(root, decisionList, false); ioErr != nil {
				return failIO(cmd, useJSON, ioErr)
			} else if verr != nil {
				return fail(cmd, 2, useJSON, verr.message, verr.payload)
			}

			cfg, err := ledger.LoadConfig(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}
			if err := checkPolicy(cmd, root, cfg, useJSON); err != nil {
				return err
			}

			resolvedBody := body
			switch {
			case body == "-":
				data, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return failIO(cmd, useJSON, err)
				}
				resolvedBody = string(data)
			case !cmd.Flags().Changed("body"):
				resolvedBody = specScaffold
			}

			id, err := ledger.GenerateID(root, ledger.RandReader)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			s := ledger.Spec{
				ID:        id,
				Title:     title,
				Status:    "draft",
				Created:   time.Now().UTC().Format("2006-01-02"),
				Evidence:  splitCSV(evidence),
				Decisions: decisionList,
				Body:      resolvedBody,
			}

			if err := ledger.SaveSpec(root, s); err != nil {
				return failIO(cmd, useJSON, err)
			}

			printCreated(cmd, useJSON, id)
			return nil
		},
	}
	cmd.Flags().StringVar(&body, "body", "", `spec body; use "-" to read from stdin; omit for a default Goal/Constraints/Interfaces/Open questions scaffold`)
	cmd.Flags().StringVar(&evidence, "evidence", "", "comma-separated evidence paths")
	cmd.Flags().StringVar(&decisions, "decision", "", "comma-separated decision-card ids or ADR paths this spec follows")
	addJSONFlag(cmd)
	return cmd
}

func newSpecActivateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activate <id>",
		Short: "Activate a draft spec",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)
			id := args[0]

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			s, err := ledger.LoadSpec(root, id)
			if err != nil {
				if errors.Is(err, ledger.ErrNotFound) {
					return fail(cmd, 2, useJSON,
						fmt.Sprintf("atlas: no such spec: %s", id),
						map[string]any{"error": "not_found", "id": id})
				}
				return failIO(cmd, useJSON, err)
			}

			if s.Status == "superseded" {
				return fail(cmd, 2, useJSON,
					fmt.Sprintf("atlas: spec %s is superseded and cannot be activated", id),
					map[string]any{"error": "superseded", "id": id, "superseded_by": s.SupersededBy})
			}

			if s.Status == "active" {
				// Idempotent: already active, nothing to do.
				printCreated(cmd, useJSON, id)
				return nil
			}

			if len(s.Decisions) == 0 {
				return fail(cmd, 2, useJSON,
					fmt.Sprintf("atlas: spec %s has no linked decision; activation requires at least one (`spec add/update --decision ...`)", id),
					map[string]any{"error": "spec_without_decision", "id": id})
			}

			if verr, ioErr := validateDecisions(root, s.Decisions, true); ioErr != nil {
				return failIO(cmd, useJSON, ioErr)
			} else if verr != nil {
				return fail(cmd, 2, useJSON, verr.message, verr.payload)
			}

			cfg, err := ledger.LoadConfig(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}
			if err := checkPolicy(cmd, root, cfg, useJSON); err != nil {
				return err
			}

			s.Status = "active"
			if err := ledger.SaveSpec(root, s); err != nil {
				return failIO(cmd, useJSON, err)
			}

			printCreated(cmd, useJSON, id)
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func newSpecUpdateCmd() *cobra.Command {
	var title, body, evidence, decisions string
	var decisionsSet bool
	cmd := &cobra.Command{
		Use:   "update <id>",
		Short: "Update a spec in place",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)
			id := args[0]

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			s, err := ledger.LoadSpec(root, id)
			if err != nil {
				if errors.Is(err, ledger.ErrNotFound) {
					return fail(cmd, 2, useJSON,
						fmt.Sprintf("atlas: no such spec: %s", id),
						map[string]any{"error": "not_found", "id": id})
				}
				return failIO(cmd, useJSON, err)
			}

			if s.Status == "superseded" {
				return fail(cmd, 2, useJSON,
					fmt.Sprintf("atlas: spec %s is superseded and cannot be updated", id),
					map[string]any{"error": "superseded", "id": id, "superseded_by": s.SupersededBy})
			}

			decisionsSet = cmd.Flags().Changed("decision")
			var newDecisions []string
			if decisionsSet {
				newDecisions = splitCSV(decisions)
				if s.Status == "active" && len(newDecisions) == 0 {
					return fail(cmd, 2, useJSON,
						fmt.Sprintf("atlas: spec %s is active and must keep at least one linked decision", id),
						map[string]any{"error": "spec_without_decision", "id": id})
				}
				if verr, ioErr := validateDecisions(root, newDecisions, false); ioErr != nil {
					return failIO(cmd, useJSON, ioErr)
				} else if verr != nil {
					return fail(cmd, 2, useJSON, verr.message, verr.payload)
				}
			}

			cfg, err := ledger.LoadConfig(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}
			if err := checkPolicy(cmd, root, cfg, useJSON); err != nil {
				return err
			}

			if cmd.Flags().Changed("title") {
				s.Title = title
			}
			if cmd.Flags().Changed("body") {
				resolvedBody := body
				if body == "-" {
					data, err := io.ReadAll(cmd.InOrStdin())
					if err != nil {
						return failIO(cmd, useJSON, err)
					}
					resolvedBody = string(data)
				}
				s.Body = resolvedBody
			}
			if cmd.Flags().Changed("evidence") {
				s.Evidence = splitCSV(evidence)
			}
			if decisionsSet {
				s.Decisions = newDecisions
			}

			if err := ledger.SaveSpec(root, s); err != nil {
				return failIO(cmd, useJSON, err)
			}

			printCreated(cmd, useJSON, id)
			return nil
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "new title")
	cmd.Flags().StringVar(&body, "body", "", `new body; use "-" to read from stdin`)
	cmd.Flags().StringVar(&evidence, "evidence", "", "comma-separated evidence paths (replaces the list)")
	cmd.Flags().StringVar(&decisions, "decision", "", "comma-separated decision-card ids or ADR paths (replaces the list)")
	addJSONFlag(cmd)
	return cmd
}

func newSpecSupersedeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "supersede <old-id> <new-id>",
		Short: "Mark an old spec superseded by a new one",
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

			old, err := ledger.LoadSpec(root, oldID)
			if err != nil {
				if errors.Is(err, ledger.ErrNotFound) {
					return fail(cmd, 2, useJSON,
						fmt.Sprintf("atlas: no such spec: %s", oldID),
						map[string]any{"error": "not_found", "id": oldID})
				}
				return failIO(cmd, useJSON, err)
			}

			if _, err := ledger.LoadSpec(root, newID); err != nil {
				if errors.Is(err, ledger.ErrNotFound) {
					return fail(cmd, 2, useJSON,
						fmt.Sprintf("atlas: no such spec: %s", newID),
						map[string]any{"error": "not_found", "id": newID})
				}
				return failIO(cmd, useJSON, err)
			}

			if err := checkPolicy(cmd, root, cfg, useJSON); err != nil {
				return err
			}

			old.Status = "superseded"
			old.SupersededBy = newID
			if err := ledger.SaveSpec(root, old); err != nil {
				return failIO(cmd, useJSON, err)
			}

			entry := ledger.LogEntry{
				ID:           old.ID,
				Kind:         "spec",
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

func newSpecListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List draft and active specs, with open-task counts",
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

			st, err := state.Build(root, cfg, state.Options{})
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			if useJSON {
				data, err := renderSpecListJSON(st.Specs)
				if err != nil {
					return failIO(cmd, true, err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}

			fmt.Fprint(cmd.OutOrStdout(), renderSpecListText(st.Specs))
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func renderSpecListText(specs []state.SpecSummary) string {
	if len(specs) == 0 {
		return "no specs\n"
	}
	var b strings.Builder
	for _, s := range specs {
		line := fmt.Sprintf("- [%s] %s (%s, %d open tasks)", s.ID, s.Title, s.Status, s.OpenTasks)
		if len(s.Decisions) > 0 {
			line += " — decisions: " + strings.Join(s.Decisions, ", ")
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}

func renderSpecListJSON(specs []state.SpecSummary) ([]byte, error) {
	out := make([]map[string]any, 0, len(specs))
	for _, s := range specs {
		decisions := s.Decisions
		if decisions == nil {
			decisions = []string{}
		}
		out = append(out, map[string]any{
			"id":         s.ID,
			"title":      s.Title,
			"status":     s.Status,
			"open_tasks": s.OpenTasks,
			"decisions":  decisions,
		})
	}
	return json.MarshalIndent(map[string]any{"specs": out}, "", "  ")
}
