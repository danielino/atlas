package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/danielino/atlas/internal/claims"
	"github.com/danielino/atlas/internal/gitx"
	"github.com/danielino/atlas/internal/ledger"
	"github.com/danielino/atlas/internal/state"
)

func newTaskCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Manage workitems",
	}
	cmd.AddCommand(newTaskAddCmd())
	cmd.AddCommand(newTaskStartCmd())
	cmd.AddCommand(newTaskBlockCmd())
	cmd.AddCommand(newTaskDoneCmd())
	return cmd
}

func newTaskAddCmd() *cobra.Command {
	var body, blockedBy, from, evidence, spec string
	cmd := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a new todo workitem",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)
			title := args[0]

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			if spec != "" {
				linkedSpec, err := ledger.LoadSpec(root, spec)
				if err != nil {
					if errors.Is(err, ledger.ErrNotFound) {
						return fail(cmd, 2, useJSON,
							fmt.Sprintf("atlas: no such spec: %s", spec),
							map[string]any{"error": "spec_not_found", "id": spec})
					}
					return failIO(cmd, useJSON, err)
				}
				if linkedSpec.Status == "superseded" {
					return fail(cmd, 2, useJSON,
						fmt.Sprintf("atlas: spec %s is superseded by %s", spec, linkedSpec.SupersededBy),
						map[string]any{"error": "spec_superseded", "id": spec, "superseded_by": linkedSpec.SupersededBy})
				}
			}

			cfg, err := ledger.LoadConfig(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			// Policy applies only when this is NOT discovered work
			// (i.e. no --from): discovered work is always allowed.
			if from == "" {
				if err := checkPolicy(cmd, root, cfg, useJSON); err != nil {
					return err
				}
			}

			resolvedBody := body
			if body == "-" {
				data, err := io.ReadAll(cmd.InOrStdin())
				if err != nil {
					return failIO(cmd, useJSON, err)
				}
				resolvedBody = string(data)
			}

			id, err := ledger.GenerateID(root, ledger.RandReader)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			w := ledger.Workitem{
				ID:             id,
				Title:          title,
				Status:         "todo",
				Created:        time.Now().UTC().Format("2006-01-02"),
				BlockedBy:      splitCSV(blockedBy),
				DiscoveredFrom: from,
				Evidence:       splitCSV(evidence),
				Spec:           spec,
				Body:           resolvedBody,
			}

			if err := ledger.SaveWorkitem(root, w); err != nil {
				return failIO(cmd, useJSON, err)
			}

			printCreated(cmd, useJSON, id)
			return nil
		},
	}
	cmd.Flags().StringVar(&body, "body", "", `workitem body; use "-" to read from stdin`)
	cmd.Flags().StringVar(&blockedBy, "blocked-by", "", "comma-separated ids this workitem is blocked by")
	cmd.Flags().StringVar(&from, "from", "", "id of the task this work was discovered from")
	cmd.Flags().StringVar(&evidence, "evidence", "", "comma-separated evidence paths")
	cmd.Flags().StringVar(&spec, "spec", "", "id of the spec this workitem implements")
	addJSONFlag(cmd)
	return cmd
}

func newTaskStartCmd() *cobra.Command {
	var steal bool
	cmd := &cobra.Command{
		Use:   "start <id>",
		Short: "Claim a workitem and mark it doing",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)
			id := args[0]

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			cfg, err := ledger.LoadConfig(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			w, err := ledger.LoadWorkitem(root, id)
			if err != nil {
				if errors.Is(err, ledger.ErrNotFound) {
					return fail(cmd, 2, useJSON,
						fmt.Sprintf("atlas: no such workitem: %s", id),
						map[string]any{"error": "not_found", "id": id})
				}
				return failIO(cmd, useJSON, err)
			}

			branch := currentBranch(root)

			if !steal {
				// Versioned binding (S2): the workitem already records a
				// branch other than ours and is "doing" — refuse just like
				// a live claim conflict, unless the caller passed --steal.
				if branch != "" && w.Branch != "" && w.Branch != branch && w.Status == "doing" {
					return claimedRefusal(cmd, root, cfg, useJSON, id, w.Branch)
				}
			}

			commonDir, gitErr := gitx.CommonDir(root)
			if gitErr == nil {
				mgr := &claims.Manager{CommonDir: commonDir, TTLHours: cfg.Claims.TTLHours}

				if steal {
					if existing, ok := mgr.Get(id); ok {
						fmt.Fprintf(cmd.ErrOrStderr(),
							"atlas: WARNING: stealing claim on %s from branch %q (session %q)\n",
							id, existing.Branch, existing.Session)
					}
					if _, err := mgr.Steal(id, branch); err != nil {
						return failIO(cmd, useJSON, err)
					}
				} else {
					if _, err := mgr.Acquire(id, branch); err != nil {
						var claimedErr *claims.ErrClaimed
						if errors.As(err, &claimedErr) {
							return claimedRefusal(cmd, root, cfg, useJSON, id, claimedErr.Existing.Branch)
						}
						return failIO(cmd, useJSON, err)
					}
				}
			}
			// If commonDir can't be resolved (not a git repo), claims are
			// skipped entirely: there is nothing to lock against.

			w.Status = "doing"
			w.Branch = branch
			if err := ledger.SaveWorkitem(root, w); err != nil {
				return failIO(cmd, useJSON, err)
			}

			printCreated(cmd, useJSON, id)
			return nil
		},
	}
	cmd.Flags().BoolVar(&steal, "steal", false, "forcibly override any existing claim or version binding")
	addJSONFlag(cmd)
	return cmd
}

// claimedRefusal writes the {"error":"claimed","task":..,"by":..,"ready":[...]}
// shape (or its human equivalent) and returns the exit-2 error.
func claimedRefusal(cmd *cobra.Command, root string, cfg ledger.Config, useJSON bool, id, by string) error {
	ready := readyIDs(root, cfg)
	if useJSON {
		return fail(cmd, 2, true, "", map[string]any{
			"error": "claimed",
			"task":  id,
			"by":    by,
			"ready": ready,
		})
	}
	msg := fmt.Sprintf("atlas: task %s is already claimed by branch %q.", id, by)
	if len(ready) > 0 {
		msg += " Ready tasks: " + strings.Join(ready, ", ")
	}
	return fail(cmd, 2, false, msg, nil)
}

func readyIDs(root string, cfg ledger.Config) []string {
	workitems, err := ledger.ListWorkitems(root)
	if err != nil {
		return []string{}
	}
	closedIDs, err := ledger.ClosedIDs(root)
	if err != nil {
		return []string{}
	}
	readyItems := state.Ready(workitems, closedIDs)
	ids := make([]string, 0, len(readyItems))
	for _, w := range readyItems {
		ids = append(ids, w.ID)
	}
	return ids
}

func newTaskBlockCmd() *cobra.Command {
	var on, reason string
	cmd := &cobra.Command{
		Use:   "block <id>",
		Short: "Mark a workitem blocked",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)
			id := args[0]

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			w, err := ledger.LoadWorkitem(root, id)
			if err != nil {
				if errors.Is(err, ledger.ErrNotFound) {
					return fail(cmd, 2, useJSON,
						fmt.Sprintf("atlas: no such workitem: %s", id),
						map[string]any{"error": "not_found", "id": id})
				}
				return failIO(cmd, useJSON, err)
			}

			if err := requireOwnerOrUnowned(cmd, root, w, useJSON); err != nil {
				return err
			}

			if !ledger.CanTransition(w.Status, "blocked") {
				return fail(cmd, 2, useJSON,
					fmt.Sprintf("atlas: cannot move %s from %q to blocked", id, w.Status),
					map[string]any{"error": "invalid_transition", "task": id, "from": w.Status, "to": "blocked"})
			}

			w.Status = "blocked"
			if on != "" {
				w.BlockedBy = []string{on}
			}
			if reason != "" {
				w.Reason = reason
			}

			if err := ledger.SaveWorkitem(root, w); err != nil {
				return failIO(cmd, useJSON, err)
			}

			printCreated(cmd, useJSON, id)
			return nil
		},
	}
	cmd.Flags().StringVar(&on, "on", "", "id this workitem is blocked on")
	cmd.Flags().StringVar(&reason, "reason", "", "why this workitem is blocked")
	addJSONFlag(cmd)
	return cmd
}

func newTaskDoneCmd() *cobra.Command {
	var summary string
	cmd := &cobra.Command{
		Use:   "done <id>",
		Short: "Close a workitem",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)
			id := args[0]

			if strings.TrimSpace(summary) == "" {
				return fail(cmd, 2, useJSON,
					"atlas: --summary is required and must not be empty",
					map[string]any{"error": "summary", "task": id})
			}

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			cfg, err := ledger.LoadConfig(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			w, err := ledger.LoadWorkitem(root, id)
			if err != nil {
				if errors.Is(err, ledger.ErrNotFound) {
					return fail(cmd, 2, useJSON,
						fmt.Sprintf("atlas: no such workitem: %s", id),
						map[string]any{"error": "not_found", "id": id})
				}
				return failIO(cmd, useJSON, err)
			}

			if err := requireOwnerOrUnowned(cmd, root, w, useJSON); err != nil {
				return err
			}

			if !ledger.CanTransition(w.Status, "done") {
				return fail(cmd, 2, useJSON,
					fmt.Sprintf("atlas: cannot move %s from %q to done", id, w.Status),
					map[string]any{"error": "invalid_transition", "task": id, "from": w.Status, "to": "done"})
			}

			branch := currentBranch(root)
			headShort, _ := gitx.HeadShort(root)

			entry := ledger.LogEntry{
				ID:      w.ID,
				Kind:    "task",
				Title:   w.Title,
				Summary: summary,
				Closed:  time.Now().UTC().Format(time.RFC3339),
				Commit:  headShort,
				Branch:  branch,
			}
			if err := ledger.AppendLog(root, entry); err != nil {
				return failIO(cmd, useJSON, err)
			}

			if err := removeWorkitemFile(root, id); err != nil {
				return failIO(cmd, useJSON, err)
			}

			if commonDir, err := gitx.CommonDir(root); err == nil {
				mgr := &claims.Manager{CommonDir: commonDir, TTLHours: cfg.Claims.TTLHours}
				_ = mgr.Release(id)
			}

			printCreated(cmd, useJSON, id)
			return nil
		},
	}
	cmd.Flags().StringVar(&summary, "summary", "", "one-line summary of what changed (required)")
	addJSONFlag(cmd)
	return cmd
}

// requireOwnerOrUnowned enforces that block/done can only be performed by
// the branch that owns the workitem (w.Branch), or by anyone if it is
// currently unowned. Skipped entirely when there is no git branch to
// compare against.
func requireOwnerOrUnowned(cmd *cobra.Command, root string, w ledger.Workitem, useJSON bool) error {
	branch := currentBranch(root)
	if branch == "" || w.Branch == "" {
		return nil
	}
	if w.Branch != branch {
		return fail(cmd, 2, useJSON,
			fmt.Sprintf("atlas: %s is owned by branch %q, not %q", w.ID, w.Branch, branch),
			map[string]any{"error": "not_owner", "task": w.ID, "by": w.Branch})
	}
	return nil
}

// removeWorkitemFile deletes the .atlas/work/<id>-*.md file for id. This
// duplicates ledger's private filename-matching convention (id followed
// by "-") in a few lines rather than adding new exported surface to the
// F1 ledger package for a single delete call.
func removeWorkitemFile(root, id string) error {
	dir := filepath.Join(root, ".atlas", "work")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if fileID(e.Name()) == id {
			return os.Remove(filepath.Join(dir, e.Name()))
		}
	}
	return nil
}

func fileID(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	if i := strings.Index(base, "-"); i >= 0 {
		return base[:i]
	}
	return base
}

func printCreated(cmd *cobra.Command, useJSON bool, id string) {
	if useJSON {
		fmt.Fprintf(cmd.OutOrStdout(), "{\"id\":%q}\n", id)
		return
	}
	fmt.Fprintln(cmd.OutOrStdout(), id)
}
