package cli

import (
	"strings"
	"testing"
)

func TestCardAdd_HookDefaultsToTitle(t *testing.T) {
	initRepo(t)

	stdout, stderr, code := ExecuteCapture([]string{"card", "add", "--type", "decision", "Use foo everywhere"})
	if code != 0 {
		t.Fatalf("card add failed (%d): %s", code, stderr)
	}
	id := strings.TrimSpace(stdout)

	out, _, code := ExecuteCapture([]string{"show", id, "--json"})
	if code != 0 {
		t.Fatalf("show failed")
	}
	if !strings.Contains(out, "Use foo everywhere") {
		t.Errorf("expected hook to default to title, got: %s", out)
	}
}

func TestCardAdd_InvalidType(t *testing.T) {
	initRepo(t)
	_, _, code := ExecuteCapture([]string{"card", "add", "--type", "bogus", "Title"})
	if code != 2 {
		t.Fatalf("expected exit 2 for invalid type, got %d", code)
	}
}

func TestCardSupersede(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"card", "add", "--type", "decision", "Old decision", "--hook", "old hook"})
	oldID := strings.TrimSpace(stdout)
	stdout, _, _ = ExecuteCapture([]string{"card", "add", "--type", "decision", "New decision", "--hook", "new hook"})
	newID := strings.TrimSpace(stdout)

	_, stderr, code := ExecuteCapture([]string{"card", "supersede", oldID, newID})
	if code != 0 {
		t.Fatalf("supersede failed (%d): %s", code, stderr)
	}

	out, _, _ := ExecuteCapture([]string{"show", oldID, "--json"})
	if !strings.Contains(out, `"status": "superseded"`) {
		t.Errorf("expected old card to be superseded, got: %s", out)
	}
	if !strings.Contains(out, newID) {
		t.Errorf("expected superseded_by to reference %s, got: %s", newID, out)
	}

	logOut, _, _ := ExecuteCapture([]string{"log", "--json"})
	if !strings.Contains(logOut, oldID) || !strings.Contains(logOut, `"kind": "card"`) {
		t.Errorf("expected supersede event in log, got: %s", logOut)
	}
}

func TestLog_Grep(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Alpha task"})
	alphaID := strings.TrimSpace(stdout)
	stdout, _, _ = ExecuteCapture([]string{"task", "add", "Beta task"})
	betaID := strings.TrimSpace(stdout)

	ExecuteCapture([]string{"task", "start", alphaID})
	ExecuteCapture([]string{"task", "done", alphaID, "--summary", "finished alpha work"})
	ExecuteCapture([]string{"task", "start", betaID})
	ExecuteCapture([]string{"task", "done", betaID, "--summary", "finished beta work"})

	out, _, code := ExecuteCapture([]string{"log", "--grep", "alpha"})
	if code != 0 {
		t.Fatalf("log --grep failed")
	}
	if !strings.Contains(out, alphaID) {
		t.Errorf("expected alpha entry, got: %s", out)
	}
	if strings.Contains(out, betaID) {
		t.Errorf("did not expect beta entry, got: %s", out)
	}
}
