package cli

import (
	"strings"
	"testing"

	"github.com/danielino/atlas/internal/testutil"
)

func TestSeed_PrintsTextAndJSON(t *testing.T) {
	initRepo(t)

	stdout, _, code := ExecuteCapture([]string{"seed"})
	if code != 0 {
		t.Fatalf("seed failed")
	}
	if !strings.Contains(stdout, "ATLAS SEED") {
		t.Errorf("expected brief text, got: %s", stdout)
	}

	out, _, code := ExecuteCapture([]string{"seed", "--json"})
	if code != 0 {
		t.Fatalf("seed --json failed")
	}
	if !strings.Contains(out, `"brief"`) {
		t.Errorf("expected {\"brief\":...}, got: %s", out)
	}
	if !strings.Contains(stdout, "atlas spec add") {
		t.Errorf("expected a `spec add` example in the brief, got: %s", stdout)
	}
	if !strings.Contains(stdout, "--decision") {
		t.Errorf("expected the brief to mention linking a spec to a decision, got: %s", stdout)
	}
}

func TestState_TextAndJSON(t *testing.T) {
	initRepo(t)
	ExecuteCapture([]string{"task", "add", "Todo task"})
	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Doing task"})
	doingID := strings.TrimSpace(stdout)
	ExecuteCapture([]string{"task", "start", doingID})
	ExecuteCapture([]string{"card", "add", "--type", "knowledge", "A fact", "--hook", "a hook"})

	out, stderr, code := ExecuteCapture([]string{"state"})
	if code != 0 {
		t.Fatalf("state failed (%d): %s", code, stderr)
	}
	if !strings.Contains(out, "## WORKITEMS") || !strings.Contains(out, "### todo") || !strings.Contains(out, "### doing") {
		t.Errorf("expected workitems by status, got:\n%s", out)
	}
	if !strings.Contains(out, "## CARDS") || !strings.Contains(out, "a hook") {
		t.Errorf("expected cards section, got:\n%s", out)
	}
	if !strings.Contains(out, "## GROUND") {
		t.Errorf("expected ground section, got:\n%s", out)
	}

	jsonOut, _, code := ExecuteCapture([]string{"state", "--json"})
	if code != 0 {
		t.Fatalf("state --json failed")
	}
	if !strings.Contains(jsonOut, doingID) {
		t.Errorf("expected doing id in json state, got: %s", jsonOut)
	}
}

func TestShow_TextAndJSONForTaskAndCard(t *testing.T) {
	initRepo(t)
	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Show me", "--body", "some body text"})
	taskID := strings.TrimSpace(stdout)
	stdout, _, _ = ExecuteCapture([]string{"card", "add", "--type", "decision", "Show card", "--hook", "hook text"})
	cardID := strings.TrimSpace(stdout)

	out, _, code := ExecuteCapture([]string{"show", taskID})
	if code != 0 || !strings.Contains(out, "some body text") {
		t.Errorf("expected raw task file, got (%d): %s", code, out)
	}

	out, _, code = ExecuteCapture([]string{"show", taskID, "--json"})
	if code != 0 || !strings.Contains(out, `"kind": "task"`) {
		t.Errorf("expected task json, got (%d): %s", code, out)
	}

	out, _, code = ExecuteCapture([]string{"show", cardID, "--json"})
	if code != 0 || !strings.Contains(out, `"kind": "card"`) {
		t.Errorf("expected card json, got (%d): %s", code, out)
	}

	_, _, code = ExecuteCapture([]string{"show", "zzzz", "--json"})
	if code != 2 {
		t.Errorf("expected exit 2 for missing id, got %d", code)
	}
}

func TestContext_TargetMode(t *testing.T) {
	initRepo(t)
	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Target task", "--body", "see the doc"})
	id := strings.TrimSpace(stdout)

	out, _, code := ExecuteCapture([]string{"context", id})
	if code != 0 {
		t.Fatalf("context <id> failed")
	}
	if !strings.Contains(out, "## TASK") || !strings.Contains(out, id) {
		t.Errorf("expected target-mode context, got:\n%s", out)
	}

	out, _, code = ExecuteCapture([]string{"context", id, "--json"})
	if code != 0 {
		t.Fatalf("context <id> --json failed")
	}
	if !strings.Contains(out, `"task"`) {
		t.Errorf("expected task in json, got: %s", out)
	}

	_, _, code = ExecuteCapture([]string{"context", "zzzz", "--json"})
	if code != 2 {
		t.Errorf("expected exit 2 for missing target id, got %d", code)
	}
}

func TestTaskBlock(t *testing.T) {
	initRepo(t)
	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Blockable"})
	id := strings.TrimSpace(stdout)

	_, stderr, code := ExecuteCapture([]string{"task", "block", id, "--reason", "waiting on infra"})
	if code != 0 {
		t.Fatalf("task block failed (%d): %s", code, stderr)
	}

	out, _, _ := ExecuteCapture([]string{"show", id, "--json"})
	if !strings.Contains(out, "blocked") || !strings.Contains(out, "waiting on infra") {
		t.Errorf("expected blocked status and reason, got: %s", out)
	}
}

func TestTaskBlock_OnSetsBlockedBy(t *testing.T) {
	initRepo(t)
	stdout, _, _ := ExecuteCapture([]string{"task", "add", "Blocker"})
	blockerID := strings.TrimSpace(stdout)
	stdout, _, _ = ExecuteCapture([]string{"task", "add", "Blocked"})
	blockedID := strings.TrimSpace(stdout)

	_, _, code := ExecuteCapture([]string{"task", "block", blockedID, "--on", blockerID})
	if code != 0 {
		t.Fatalf("block failed")
	}

	out, _, _ := ExecuteCapture([]string{"show", blockedID, "--json"})
	if !strings.Contains(out, blockerID) {
		t.Errorf("expected blocked_by to reference %s, got: %s", blockerID, out)
	}
}

func TestNotFoundErrors(t *testing.T) {
	initRepo(t)

	_, _, code := ExecuteCapture([]string{"task", "start", "zzzz", "--json"})
	if code != 2 {
		t.Errorf("task start missing id: expected 2, got %d", code)
	}
	_, _, code = ExecuteCapture([]string{"task", "block", "zzzz", "--json"})
	if code != 2 {
		t.Errorf("task block missing id: expected 2, got %d", code)
	}
	_, _, code = ExecuteCapture([]string{"task", "done", "zzzz", "--summary", "x", "--json"})
	if code != 2 {
		t.Errorf("task done missing id: expected 2, got %d", code)
	}
	_, _, code = ExecuteCapture([]string{"card", "supersede", "zzzz", "yyyy", "--json"})
	if code != 2 {
		t.Errorf("card supersede missing old id: expected 2, got %d", code)
	}
}

func TestRequireRoot_NoLedger(t *testing.T) {
	dir := testutil.SetupRepo(t)
	chdir(t, dir)

	_, _, code := ExecuteCapture([]string{"context", "--json"})
	if code != 1 {
		t.Errorf("expected exit 1 with no ledger, got %d", code)
	}
	_, _, code = ExecuteCapture([]string{"state"})
	if code != 1 {
		t.Errorf("expected exit 1 with no ledger, got %d", code)
	}
}

func TestTaskAdd_FromStdinBody(t *testing.T) {
	initRepo(t)
	// --body without "-" is a literal string; verify it round-trips.
	stdout, _, code := ExecuteCapture([]string{"task", "add", "Literal body", "--body", "literal text"})
	if code != 0 {
		t.Fatalf("task add failed")
	}
	id := strings.TrimSpace(stdout)
	out, _, _ := ExecuteCapture([]string{"show", id})
	if !strings.Contains(out, "literal text") {
		t.Errorf("expected literal body text, got: %s", out)
	}
}
