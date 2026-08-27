// Package cli implements the ATLAS command surface (PLAN.md S2): cobra
// wiring for every command including `doctor`, the exit-code convention,
// JSON error shapes for semantic refusals, root discovery, and the
// plan-mutation policy check.
package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/danielino/atlas/internal/gitx"
	"github.com/danielino/atlas/internal/ledger"
)

// ExitError carries the process exit code decided by a command's RunE.
// Human/JSON output has already been written to the command's out/err
// writers by the time this is returned; Execute/main only need the code.
type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("cli: exit %d", e.Code)
}

// NewRootCmd assembles the `atlas` root command and every subcommand.
// A fresh tree is built on every call so repeated invocations (as in
// tests) never see stale flag state from a previous run.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "atlas",
		Short:         "ATLAS — a lossy, file-based project ledger for coding agents",
		SilenceErrors: true,
		SilenceUsage:  true,
	}

	root.AddCommand(newInitCmd())
	root.AddCommand(newSeedCmd())
	root.AddCommand(newContextCmd())
	root.AddCommand(newStateCmd())
	root.AddCommand(newTaskCmd())
	root.AddCommand(newCardCmd())
	root.AddCommand(newSpecCmd())
	root.AddCommand(newShowCmd())
	root.AddCommand(newLogCmd())
	root.AddCommand(newDoctorCmd())

	return root
}

// Execute runs atlas with args, sending command output to stdout/stderr,
// and returns the process exit code per PLAN.md S2 (0 ok, 1 I/O or parse
// error, 2 semantic refusal). It never panics and never calls os.Exit
// itself, so it is safe to call from tests.
func Execute(args []string, stdout, stderr io.Writer) int {
	return execute(args, nil, stdout, stderr)
}

// ExecuteWithStdin is Execute with an explicit stdin reader, for commands
// that support `--body -` (read the value from stdin).
func ExecuteWithStdin(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	return execute(args, stdin, stdout, stderr)
}

func execute(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	root := NewRootCmd()
	if stdin != nil {
		root.SetIn(stdin)
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.SetArgs(args)

	err := root.Execute()
	if err == nil {
		return 0
	}

	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}

	// A cobra-level error (unknown flag, wrong arg count, etc.) never went
	// through our fail() helpers: report it plainly and treat it as a
	// parse error.
	fmt.Fprintln(stderr, "atlas: "+err.Error())
	return 1
}

// ExecuteCapture is a test convenience wrapper around Execute that
// captures stdout/stderr into strings.
func ExecuteCapture(args []string) (stdout, stderr string, code int) {
	var outBuf, errBuf bytes.Buffer
	code = Execute(args, &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), code
}

// ExecuteCaptureStdin is ExecuteCapture with a stdin string, for testing
// `--body -` handling.
func ExecuteCaptureStdin(args []string, stdin string) (stdout, stderr string, code int) {
	var outBuf, errBuf bytes.Buffer
	code = ExecuteWithStdin(args, strings.NewReader(stdin), &outBuf, &errBuf)
	return outBuf.String(), errBuf.String(), code
}

// jsonFlag reads the --json flag common to every read command and every
// command that can semantically refuse.
func jsonFlag(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

func addJSONFlag(cmd *cobra.Command) {
	cmd.Flags().Bool("json", false, "output machine-readable JSON")
}

// fail writes either the JSON payload (to stdout, if useJSON) or the
// human message (to stderr), then returns an *ExitError with the given
// code. Every semantic refusal in this package goes through this so
// output routing is decided in exactly one place.
func fail(cmd *cobra.Command, code int, useJSON bool, humanMsg string, payload map[string]any) error {
	if useJSON {
		data, err := json.Marshal(payload)
		if err != nil {
			fmt.Fprintln(cmd.ErrOrStderr(), "atlas: internal error marshalling JSON error: "+err.Error())
			return &ExitError{Code: 1}
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
	} else {
		fmt.Fprintln(cmd.ErrOrStderr(), humanMsg)
	}
	return &ExitError{Code: code}
}

// failIO reports an unexpected I/O/parse-level error (exit 1).
func failIO(cmd *cobra.Command, useJSON bool, err error) error {
	if useJSON {
		return fail(cmd, 1, true, "", map[string]any{"error": "io", "message": err.Error()})
	}
	return fail(cmd, 1, false, "atlas: "+err.Error(), nil)
}

// requireRoot locates the ATLAS project root by walking up from the
// current working directory (ledger.FindRoot). Every command except
// `init` calls this first. A missing ledger is reported as exit 1: there
// is nothing to refuse against semantically, it's a precondition failure.
func requireRoot(cmd *cobra.Command, useJSON bool) (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", failIO(cmd, useJSON, err)
	}
	root, err := ledger.FindRoot(wd)
	if err != nil {
		if errors.Is(err, ledger.ErrNoLedger) {
			return "", fail(cmd, 1, useJSON,
				"atlas: not an ATLAS project (no .atlas directory found; run `atlas init`)",
				map[string]any{"error": "no_ledger"})
		}
		return "", failIO(cmd, useJSON, err)
	}
	return root, nil
}

// splitCSV splits a comma-separated flag value into a trimmed, non-empty
// slice. An empty input yields a nil slice.
func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// currentBranch returns the current git branch, or "" if root is not a
// git repository (degrades silently: policy/ownership checks that depend
// on it simply skip when there is no branch to compare against).
func currentBranch(root string) string {
	b, err := gitx.Branch(root)
	if err != nil {
		return ""
	}
	return b
}

// checkPolicy implements PLAN.md S2's plan-mutation policy: if the
// current branch is not among cfg.Policy.IntegrationBranches, a
// plan-mutating command (task add without --from, card add, card
// supersede) either warns on stderr and proceeds ("warn", default) or is
// refused with exit 2 ("strict"). Never applied when there is no git
// branch to evaluate (no repository): we cannot prove a violation, so we
// don't claim one.
func checkPolicy(cmd *cobra.Command, root string, cfg ledger.Config, useJSON bool) error {
	branch := currentBranch(root)
	if branch == "" {
		return nil
	}
	for _, b := range cfg.Policy.IntegrationBranches {
		if b == branch {
			return nil
		}
	}

	switch cfg.Policy.PlanMutations {
	case "strict":
		return fail(cmd, 2, useJSON,
			fmt.Sprintf("atlas: refused: plan-mutation on branch %q is not allowed under strict policy", branch),
			map[string]any{"error": "policy", "branch": branch})
	default: // "warn" and any unrecognized value default to warn-and-proceed.
		fmt.Fprintf(cmd.ErrOrStderr(), "atlas: warning: plan-mutation on non-integration branch %q (policy=warn)\n", branch)
		return nil
	}
}
