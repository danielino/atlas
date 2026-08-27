package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_FullLifecycleThenDoctorCatchesCorruption exercises the whole
// command surface end to end in one temp repo, then deliberately
// corrupts the ledger and checks that `atlas doctor` reports it (F5,
// SPEC.md S8).
func TestE2E_FullLifecycleThenDoctorCatchesCorruption(t *testing.T) {
	dir := initRepo(t)

	// init: AGENTS.md bootstrap block + .gitattributes merge=union.
	agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), bootstrapBegin) || !strings.Contains(string(agents), bootstrapEnd) {
		t.Fatalf("AGENTS.md missing bootstrap block:\n%s", agents)
	}
	ga, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(ga)) != gitAttributesLine {
		t.Fatalf(".gitattributes = %q, want %q", ga, gitAttributesLine)
	}

	// seed: non-empty brief, no LLM call.
	seedOut, seedErr, code := ExecuteCapture([]string{"seed"})
	if code != 0 {
		t.Fatalf("seed failed (%d): %s", code, seedErr)
	}
	if strings.TrimSpace(seedOut) == "" {
		t.Fatal("seed produced an empty brief")
	}

	// task add.
	stdout, stderr, code := ExecuteCapture([]string{"task", "add", "Fix the widget"})
	if code != 0 {
		t.Fatalf("task add failed (%d): %s", code, stderr)
	}
	taskID := strings.TrimSpace(stdout)

	// task start.
	if _, stderr, code := ExecuteCapture([]string{"task", "start", taskID}); code != 0 {
		t.Fatalf("task start failed (%d): %s", code, stderr)
	}

	// context: shows NOW with the started task.
	out, stderr, code := ExecuteCapture([]string{"context"})
	if code != 0 {
		t.Fatalf("context failed (%d): %s", code, stderr)
	}
	if !strings.Contains(out, "## NOW") || !strings.Contains(out, taskID) {
		t.Fatalf("expected NOW section with %s, got:\n%s", taskID, out)
	}

	// task done --summary.
	if _, stderr, code := ExecuteCapture([]string{"task", "done", taskID, "--summary", "widget fixed"}); code != 0 {
		t.Fatalf("task done failed (%d): %s", code, stderr)
	}

	// context: shows RECENT with the closed task's summary.
	out, stderr, code = ExecuteCapture([]string{"context"})
	if code != 0 {
		t.Fatalf("context failed (%d): %s", code, stderr)
	}
	if !strings.Contains(out, "## RECENT") || !strings.Contains(out, "widget fixed") {
		t.Fatalf("expected RECENT section with summary, got:\n%s", out)
	}

	// card add + card supersede.
	stdout, stderr, code = ExecuteCapture([]string{"card", "add", "--type", "decision", "Use O_EXCL", "--hook", "claims use O_EXCL"})
	if code != 0 {
		t.Fatalf("card add failed (%d): %s", code, stderr)
	}
	oldCardID := strings.TrimSpace(stdout)

	stdout, stderr, code = ExecuteCapture([]string{"card", "add", "--type", "decision", "Use hardlink publish instead", "--hook", "claims publish via hardlink"})
	if code != 0 {
		t.Fatalf("second card add failed (%d): %s", code, stderr)
	}
	newCardID := strings.TrimSpace(stdout)

	if _, stderr, code := ExecuteCapture([]string{"card", "supersede", oldCardID, newCardID}); code != 0 {
		t.Fatalf("card supersede failed (%d): %s", code, stderr)
	}

	// doctor: clean ledger, exit 0, no errors.
	out, stderr, code = ExecuteCapture([]string{"doctor"})
	if code != 0 {
		t.Fatalf("expected doctor exit 0 on a clean ledger, got %d\nstdout:\n%s\nstderr:\n%s", code, out, stderr)
	}
	if strings.Contains(out, "## ERRORS") {
		t.Fatalf("expected no errors on a clean ledger, got:\n%s", out)
	}

	// --- Deliberate corruption ---

	// 1. A workitem whose blocked_by points at a nonexistent id.
	brokenWorkPath := filepath.Join(dir, ".atlas", "work", "zzzz-broken.md")
	brokenWork := "---\n" +
		"id: zzzz\n" +
		"title: broken workitem\n" +
		"status: todo\n" +
		"created: 2026-08-27\n" +
		"blocked_by: [ffff]\n" +
		"summary: \"\"\n" +
		"---\n" +
		"body\n"
	if err := os.WriteFile(brokenWorkPath, []byte(brokenWork), 0o644); err != nil {
		t.Fatal(err)
	}

	// 2. A malformed card file (opens frontmatter, never closes it).
	brokenCardPath := filepath.Join(dir, ".atlas", "cards", "yyyy-broken.md")
	if err := os.WriteFile(brokenCardPath, []byte("---\nid: yyyy\nno closing delimiter here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, code = ExecuteCapture([]string{"doctor"})
	if code != 3 {
		t.Fatalf("expected doctor exit 3 after corruption, got %d\n%s", code, out)
	}
	if !strings.Contains(out, "ffff") {
		t.Errorf("expected doctor to report the orphan blocked_by reference to ffff, got:\n%s", out)
	}
	if !strings.Contains(out, "malformed frontmatter") {
		t.Errorf("expected doctor to report the malformed card frontmatter, got:\n%s", out)
	}

	jsonOut, _, code := ExecuteCapture([]string{"doctor", "--json"})
	if code != 3 {
		t.Fatalf("expected doctor --json exit 3 after corruption, got %d\n%s", code, jsonOut)
	}
	if !strings.Contains(jsonOut, "orphan_ref") || !strings.Contains(jsonOut, "malformed_frontmatter") {
		t.Errorf("expected both orphan_ref and malformed_frontmatter codes in json, got:\n%s", jsonOut)
	}
}

// TestE2E_SpecWorkflow exercises S9 end to end in a fresh repo: a
// decision card, a spec that follows it, a task linked to that spec, and
// `atlas context <task>` showing the spec — finishing with a clean
// `atlas doctor` (S9.7).
func TestE2E_SpecWorkflow(t *testing.T) {
	initRepo(t)

	// A spec must follow a decision (S9.8): create the decision first.
	stdout, stderr, code := ExecuteCapture([]string{"card", "add", "--type", "decision", "Bounded retry model", "--hook", "Retries are capped, never infinite"})
	if code != 0 {
		t.Fatalf("card add failed (%d): %s", code, stderr)
	}
	decisionID := strings.TrimSpace(stdout)

	// Add a spec → task add --spec → context <task> shows the spec.
	stdout, stderr, code = ExecuteCapture([]string{
		"spec", "add", "Workload execution retry semantics",
		"--body", "Retries are bounded exponential backoff, capped at 5 attempts.",
		"--decision", decisionID,
	})
	if code != 0 {
		t.Fatalf("spec add failed (%d): %s", code, stderr)
	}
	specID := strings.TrimSpace(stdout)

	if _, stderr, code := ExecuteCapture([]string{"spec", "activate", specID}); code != 0 {
		t.Fatalf("spec activate failed (%d): %s", code, stderr)
	}

	stdout, stderr, code = ExecuteCapture([]string{"task", "add", "Implement the retry loop", "--spec", specID})
	if code != 0 {
		t.Fatalf("task add --spec failed (%d): %s", code, stderr)
	}
	taskID := strings.TrimSpace(stdout)

	out, stderr, code := ExecuteCapture([]string{"context", taskID})
	if code != 0 {
		t.Fatalf("context failed (%d): %s", code, stderr)
	}
	if !strings.Contains(out, "## SPEC ["+specID+"]") {
		t.Fatalf("expected SPEC section in target context, got:\n%s", out)
	}
	if !strings.Contains(out, "bounded exponential backoff") {
		t.Fatalf("expected the spec's full body in target context, got:\n%s", out)
	}
	if !strings.Contains(out, "Decisions: "+decisionID) {
		t.Fatalf("expected the spec's Decisions line in target context, got:\n%s", out)
	}

	// The general brief also carries a SPECS line for this spec.
	generalOut, stderr, code := ExecuteCapture([]string{"context"})
	if code != 0 {
		t.Fatalf("context failed (%d): %s", code, stderr)
	}
	if !strings.Contains(generalOut, "## SPECS") || !strings.Contains(generalOut, specID) {
		t.Fatalf("expected a SPECS section mentioning %s, got:\n%s", specID, generalOut)
	}

	// doctor: clean ledger, exit 0, no errors.
	out, stderr, code = ExecuteCapture([]string{"doctor"})
	if code != 0 {
		t.Fatalf("expected doctor exit 0 on a clean ledger, got %d\nstdout:\n%s\nstderr:\n%s", code, out, stderr)
	}
	if strings.Contains(out, "## ERRORS") {
		t.Fatalf("expected no errors on a clean ledger, got:\n%s", out)
	}
}
