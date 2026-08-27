package cli

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestLifecycle_AddStartDoneContextShowsRecent(t *testing.T) {
	initRepo(t)

	stdout, stderr, code := ExecuteCapture([]string{"task", "add", "Fix the widget"})
	if code != 0 {
		t.Fatalf("task add failed (code %d): %s", code, stderr)
	}
	id := strings.TrimSpace(stdout)
	if len(id) < 4 {
		t.Fatalf("unexpected id output: %q", stdout)
	}

	if _, stderr, code := ExecuteCapture([]string{"task", "start", id}); code != 0 {
		t.Fatalf("task start failed (code %d): %s", code, stderr)
	}

	if _, stderr, code := ExecuteCapture([]string{"task", "done", id, "--summary", "widget fixed for real"}); code != 0 {
		t.Fatalf("task done failed (code %d): %s", code, stderr)
	}

	stdout, stderr, code = ExecuteCapture([]string{"context"})
	if code != 0 {
		t.Fatalf("context failed (code %d): %s", code, stderr)
	}
	if !strings.Contains(stdout, "## RECENT") {
		t.Errorf("expected RECENT section in context, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "widget fixed for real") {
		t.Errorf("expected summary in RECENT section, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, id) {
		t.Errorf("expected id %s in context output, got:\n%s", id, stdout)
	}

	// The workitem file must be gone from work/, and log.jsonl must record it.
	logOut, _, code := ExecuteCapture([]string{"log", "--json"})
	if code != 0 {
		t.Fatalf("log failed")
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(logOut), &entries); err != nil {
		t.Fatalf("log --json not valid JSON: %v\n%s", err, logOut)
	}
	if len(entries) != 1 || entries[0]["id"] != id {
		t.Errorf("expected exactly one log entry for %s, got %v", id, entries)
	}
}

func TestTaskDone_RequiresNonEmptySummary(t *testing.T) {
	dir := initRepo(t)
	_ = dir

	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Something"})
	id := strings.TrimSpace(stdout)

	_, stderr, code := ExecuteCapture([]string{"task", "done", id})
	if code != 2 {
		t.Fatalf("expected exit 2 for missing summary, got %d (stderr=%s)", code, stderr)
	}

	_, stderr, code = ExecuteCapture([]string{"task", "done", id, "--summary", "  "})
	if code != 2 {
		t.Fatalf("expected exit 2 for blank summary, got %d (stderr=%s)", code, stderr)
	}
}

func TestTaskDone_JSONErrorShape(t *testing.T) {
	initRepo(t)
	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Something"})
	id := strings.TrimSpace(stdout)

	out, _, code := ExecuteCapture([]string{"task", "done", id, "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2, got %d", code)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("not valid JSON: %v: %s", err, out)
	}
	if payload["error"] != "summary" {
		t.Errorf("expected error=summary, got %v", payload)
	}
}
