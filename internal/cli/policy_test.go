package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func TestPolicy_WarnOnFeatureBranchProceedsWithStderrWarning(t *testing.T) {
	dir := initRepo(t)
	gitRun(t, dir, "checkout", "-b", "feature/x")

	stdout, stderr, code := ExecuteCapture([]string{"task", "add", "New feature work"})
	if code != 0 {
		t.Fatalf("expected exit 0 under warn policy, got %d: %s", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Errorf("expected an id on stdout, got empty")
	}
	if !strings.Contains(stderr, "warning") {
		t.Errorf("expected a policy warning on stderr, got: %q", stderr)
	}
}

func TestPolicy_StrictOnFeatureBranchRefuses(t *testing.T) {
	dir := initRepo(t)

	cfgPath := filepath.Join(dir, ".atlas", "config.toml")
	strictCfg := "[policy]\nplan_mutations = \"strict\"\nintegration_branches = [\"main\", \"develop\"]\n"
	if err := os.WriteFile(cfgPath, []byte(strictCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	gitRun(t, dir, "checkout", "-b", "feature/y")

	out, _, code := ExecuteCapture([]string{"task", "add", "Should be refused", "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2 under strict policy, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"error":"policy"`) {
		t.Errorf("expected policy error JSON, got: %s", out)
	}
	if !strings.Contains(out, `"branch":"feature/y"`) {
		t.Errorf("expected branch in JSON, got: %s", out)
	}
}

func TestPolicy_NotAppliedWithFrom(t *testing.T) {
	dir := initRepo(t)

	cfgPath := filepath.Join(dir, ".atlas", "config.toml")
	strictCfg := "[policy]\nplan_mutations = \"strict\"\n"
	if err := os.WriteFile(cfgPath, []byte(strictCfg), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Original task"})
	origID := strings.TrimSpace(stdout)

	gitRun(t, dir, "checkout", "-b", "feature/z")

	_, stderr, code := ExecuteCapture([]string{"task", "add", "Discovered work", "--from", origID})
	if code != 0 {
		t.Fatalf("expected --from to bypass strict policy, got %d: %s", code, stderr)
	}
}

func TestPolicy_AlwaysAppliesToCardAdd(t *testing.T) {
	dir := initRepo(t)

	cfgPath := filepath.Join(dir, ".atlas", "config.toml")
	strictCfg := "[policy]\nplan_mutations = \"strict\"\n"
	if err := os.WriteFile(cfgPath, []byte(strictCfg), 0o644); err != nil {
		t.Fatal(err)
	}
	gitRun(t, dir, "checkout", "-b", "feature/cards")

	_, _, code := ExecuteCapture([]string{"card", "add", "--type", "decision", "A decision", "--json"})
	if code != 2 {
		t.Fatalf("expected card add to be refused under strict policy, got %d", code)
	}
}
