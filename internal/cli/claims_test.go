package cli

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"

	"github.com/dmarcocci/atlas/internal/testutil"
)

// commitAll commits everything currently on disk so that a worktree can be
// created against it (git worktree add requires the branch to exist / be
// reachable from a commit).
func commitAll(t *testing.T, dir, msg string) {
	t.Helper()
	gitOut(t, dir, "add", "-A")
	// commit may legitimately have nothing to do; ignore that failure only.
	cmd := exec.Command("git", "-C", dir, "commit", "-m", msg)
	_, _ = cmd.CombinedOutput()
}

func TestTaskStart_ClaimedRefusalIncludesReadyList(t *testing.T) {
	dir := initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Claimed task"})
	claimedID := strings.TrimSpace(stdout)
	stdout, _, _ = ExecuteCapture([]string{"task", "add", "Another ready task"})
	readyID := strings.TrimSpace(stdout)

	if _, stderr, code := ExecuteCapture([]string{"task", "start", claimedID}); code != 0 {
		t.Fatalf("first start failed (%d): %s", code, stderr)
	}
	commitAll(t, dir, "chore: wip")

	// Start it again from a second worktree on a different branch: the
	// claim manager lives outside the repo (common dir), so this must be
	// refused even though it's a different working tree.
	wt := testutil.SetupWorktree(t, dir, "feature/other")
	chdir(t, wt)

	out, _, code := ExecuteCapture([]string{"task", "start", claimedID, "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2, got %d: %s", code, out)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("not JSON: %v: %s", err, out)
	}
	if payload["error"] != "claimed" {
		t.Fatalf("expected error=claimed, got %v", payload)
	}
	if payload["task"] != claimedID {
		t.Errorf("expected task=%s, got %v", claimedID, payload["task"])
	}
	if payload["by"] != "main" {
		t.Errorf("expected by=main, got %v", payload["by"])
	}
	ready, ok := payload["ready"].([]any)
	if !ok {
		t.Fatalf("expected ready to be a list, got %v (%T)", payload["ready"], payload["ready"])
	}
	found := false
	for _, r := range ready {
		if r == readyID {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ready list to include %s, got %v", readyID, ready)
	}
}

func TestTaskStart_Steal(t *testing.T) {
	dir := initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Stealable task"})
	id := strings.TrimSpace(stdout)

	if _, _, code := ExecuteCapture([]string{"task", "start", id}); code != 0 {
		t.Fatal("first start failed")
	}
	commitAll(t, dir, "chore: wip")

	wt := testutil.SetupWorktree(t, dir, "feature/steal")
	chdir(t, wt)

	// Without --steal: refused.
	if _, _, code := ExecuteCapture([]string{"task", "start", id}); code != 2 {
		t.Fatalf("expected exit 2 without --steal, got %d", code)
	}

	// With --steal: succeeds, and warns on stderr.
	_, stderr, code := ExecuteCapture([]string{"task", "start", id, "--steal"})
	if code != 0 {
		t.Fatalf("steal failed (%d): %s", code, stderr)
	}
	if !strings.Contains(strings.ToLower(stderr), "steal") {
		t.Errorf("expected a steal warning on stderr, got: %q", stderr)
	}
}

func TestContext_ShowsElsewhereFromOtherWorktree(t *testing.T) {
	dir := initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Elsewhere task"})
	id := strings.TrimSpace(stdout)
	if _, _, code := ExecuteCapture([]string{"task", "start", id}); code != 0 {
		t.Fatal("start failed")
	}
	commitAll(t, dir, "chore: wip")

	wt := testutil.SetupWorktree(t, dir, "feature/elsewhere")
	chdir(t, wt)

	stdout, stderr, code := ExecuteCapture([]string{"context"})
	if code != 0 {
		t.Fatalf("context failed (%d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "elsewhere") || !strings.Contains(stdout, "main") {
		t.Errorf("expected elsewhere claim referencing main branch, got:\n%s", stdout)
	}
}
