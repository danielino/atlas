package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/danielino/atlas/internal/ledger"
	"github.com/stretchr/testify/require"
)

// addDecisionCard creates a decision card and returns its id, for tests
// that need a valid `--decision` target to activate a spec (S9.8).
func addDecisionCard(t *testing.T) string {
	t.Helper()
	stdout, _, code := ExecuteCapture([]string{"card", "add", "--type", "decision", "A governing decision"})
	if code != 0 {
		t.Fatalf("card add failed")
	}
	return strings.TrimSpace(stdout)
}

func TestSpecAdd_CreatesDraft(t *testing.T) {
	initRepo(t)

	stdout, stderr, code := ExecuteCapture([]string{"spec", "add", "Workload retry semantics"})
	if code != 0 {
		t.Fatalf("spec add failed (%d): %s", code, stderr)
	}
	id := strings.TrimSpace(stdout)

	out, _, code := ExecuteCapture([]string{"show", id, "--json"})
	if code != 0 {
		t.Fatalf("show failed")
	}
	if !strings.Contains(out, `"status": "draft"`) {
		t.Errorf("expected draft status, got: %s", out)
	}
}

func TestSpecAdd_WithBodyAndEvidence(t *testing.T) {
	initRepo(t)

	stdout, _, code := ExecuteCapture([]string{"spec", "add", "A spec", "--body", "the spec text", "--evidence", "docs/a.md,docs/b.md"})
	if code != 0 {
		t.Fatalf("spec add failed")
	}
	id := strings.TrimSpace(stdout)

	out, _, _ := ExecuteCapture([]string{"show", id})
	if !strings.Contains(out, "the spec text") {
		t.Errorf("expected body in output, got: %s", out)
	}
	if !strings.Contains(out, "docs/a.md") || !strings.Contains(out, "docs/b.md") {
		t.Errorf("expected evidence in output, got: %s", out)
	}
}

func TestSpecAdd_NoBody_UsesScaffold(t *testing.T) {
	root := initRepo(t)

	stdout, _, code := ExecuteCapture([]string{"spec", "add", "A scaffolded spec"})
	if code != 0 {
		t.Fatalf("spec add failed")
	}
	id := strings.TrimSpace(stdout)

	s, err := ledger.LoadSpec(root, id)
	if err != nil {
		t.Fatalf("LoadSpec failed: %v", err)
	}
	require.Equal(t, specScaffold, s.Body, "spec body must match the S10.2 scaffold verbatim")
}

func TestSpecAdd_ExplicitBody_NoScaffold(t *testing.T) {
	initRepo(t)

	stdout, _, code := ExecuteCapture([]string{"spec", "add", "A hand-written spec", "--body", "just this text"})
	if code != 0 {
		t.Fatalf("spec add failed")
	}
	id := strings.TrimSpace(stdout)

	out, _, _ := ExecuteCapture([]string{"show", id})
	if strings.Contains(out, "## Goal") {
		t.Errorf("expected no scaffold when --body is explicit, got: %s", out)
	}
	if !strings.Contains(out, "just this text") {
		t.Errorf("expected explicit body, got: %s", out)
	}
}

func TestSpecAdd_ScaffoldOnly_CannotActivateWithoutDecisions(t *testing.T) {
	initRepo(t)

	stdout, _, code := ExecuteCapture([]string{"spec", "add", "A scaffolded spec, no decisions"})
	if code != 0 {
		t.Fatalf("spec add failed")
	}
	id := strings.TrimSpace(stdout)

	stdout, _, code = ExecuteCapture([]string{"spec", "activate", id, "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2 activating a scaffold-only spec without decisions, got %d (%s)", code, stdout)
	}
	if !strings.Contains(stdout, "spec_without_decision") {
		t.Errorf("expected spec_without_decision refusal, got: %s", stdout)
	}
}

func TestSpecAdd_BodyFromStdin(t *testing.T) {
	initRepo(t)

	stdout, _, code := ExecuteCaptureStdin([]string{"spec", "add", "Stdin spec", "--body", "-"}, "body from stdin\n")
	if code != 0 {
		t.Fatalf("spec add failed")
	}
	id := strings.TrimSpace(stdout)

	out, _, _ := ExecuteCapture([]string{"show", id})
	if !strings.Contains(out, "body from stdin") {
		t.Errorf("expected stdin body, got: %s", out)
	}
}

func TestSpecActivate_DraftToActive(t *testing.T) {
	initRepo(t)
	decisionID := addDecisionCard(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "A spec", "--decision", decisionID})
	id := strings.TrimSpace(stdout)

	_, stderr, code := ExecuteCapture([]string{"spec", "activate", id})
	if code != 0 {
		t.Fatalf("activate failed (%d): %s", code, stderr)
	}

	out, _, _ := ExecuteCapture([]string{"show", id, "--json"})
	if !strings.Contains(out, `"status": "active"`) {
		t.Errorf("expected active status, got: %s", out)
	}
}

func TestSpecActivate_IdempotentOnActive(t *testing.T) {
	initRepo(t)
	decisionID := addDecisionCard(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "A spec", "--decision", decisionID})
	id := strings.TrimSpace(stdout)
	ExecuteCapture([]string{"spec", "activate", id})

	_, stderr, code := ExecuteCapture([]string{"spec", "activate", id})
	if code != 0 {
		t.Fatalf("expected idempotent activate to succeed, got %d: %s", code, stderr)
	}
}

func TestSpecActivate_RefusedWithoutDecision(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "A spec without decision"})
	id := strings.TrimSpace(stdout)

	out, _, code := ExecuteCapture([]string{"spec", "activate", id, "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2 activating without a decision, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"error":"spec_without_decision"`) {
		t.Errorf("expected spec_without_decision error shape, got: %s", out)
	}
}

func TestSpecAdd_DecisionNotFound(t *testing.T) {
	initRepo(t)

	out, _, code := ExecuteCapture([]string{"spec", "add", "A spec", "--decision", "abcd", "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2 for nonexistent decision, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"error":"decision_not_found"`) {
		t.Errorf("expected decision_not_found error shape, got: %s", out)
	}
}

func TestSpecAdd_DecisionMustBeDecisionTypeCard(t *testing.T) {
	initRepo(t)
	stdout, _, code := ExecuteCapture([]string{"card", "add", "--type", "knowledge", "Just knowledge"})
	if code != 0 {
		t.Fatalf("card add failed")
	}
	knowledgeID := strings.TrimSpace(stdout)

	out, _, code := ExecuteCapture([]string{"spec", "add", "A spec", "--decision", knowledgeID, "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2 for a knowledge card used as decision, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"error":"decision_not_found"`) {
		t.Errorf("expected decision_not_found error shape, got: %s", out)
	}
}

func TestSpecAdd_DecisionPathNotFound(t *testing.T) {
	initRepo(t)

	out, _, code := ExecuteCapture([]string{"spec", "add", "A spec", "--decision", "docs/adr/0034-nope.md", "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2 for nonexistent decision path, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"error":"decision_path_not_found"`) {
		t.Errorf("expected decision_path_not_found error shape, got: %s", out)
	}
}

func TestSpecAdd_DecisionPathOnDisk(t *testing.T) {
	dir := initRepo(t)
	adrDir := dir + "/docs/adr"
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(adrDir+"/0034-enrichment-stage.md", []byte("# ADR\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := ExecuteCapture([]string{"spec", "add", "A spec", "--decision", "docs/adr/0034-enrichment-stage.md"})
	if code != 0 {
		t.Fatalf("spec add failed (%d): %s", code, stderr)
	}
	id := strings.TrimSpace(stdout)

	out, _, _ := ExecuteCapture([]string{"show", id})
	if !strings.Contains(out, "docs/adr/0034-enrichment-stage.md") {
		t.Errorf("expected decision path in output, got: %s", out)
	}
}

func TestSpecActivate_RefusedOnSupersededDecision(t *testing.T) {
	initRepo(t)
	oldDecision := addDecisionCard(t)
	newDecision := addDecisionCard(t)
	ExecuteCapture([]string{"card", "supersede", oldDecision, newDecision})

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "A spec", "--decision", oldDecision})
	id := strings.TrimSpace(stdout)

	out, _, code := ExecuteCapture([]string{"spec", "activate", id, "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2 activating with a superseded decision, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"error":"decision_superseded"`) {
		t.Errorf("expected decision_superseded error shape, got: %s", out)
	}
	if !strings.Contains(out, newDecision) {
		t.Errorf("expected superseded_by in error, got: %s", out)
	}
}

func TestSpecUpdate_DecisionReplacesList(t *testing.T) {
	initRepo(t)
	d1 := addDecisionCard(t)
	d2 := addDecisionCard(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "A spec", "--decision", d1})
	id := strings.TrimSpace(stdout)

	_, stderr, code := ExecuteCapture([]string{"spec", "update", id, "--decision", d2})
	if code != 0 {
		t.Fatalf("update failed (%d): %s", code, stderr)
	}

	out, _, _ := ExecuteCapture([]string{"show", id})
	if strings.Contains(out, d1) {
		t.Errorf("expected old decision replaced, got: %s", out)
	}
	if !strings.Contains(out, d2) {
		t.Errorf("expected new decision present, got: %s", out)
	}
}

func TestSpecActivate_RefusedOnSuperseded(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "Old spec"})
	oldID := strings.TrimSpace(stdout)
	stdout, _, _ = ExecuteCapture([]string{"spec", "add", "New spec"})
	newID := strings.TrimSpace(stdout)
	ExecuteCapture([]string{"spec", "supersede", oldID, newID})

	out, _, code := ExecuteCapture([]string{"spec", "activate", oldID, "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2 activating a superseded spec, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"error":"not_found"`) && !strings.Contains(out, `"error":"superseded"`) {
		t.Errorf("expected a refusal error shape, got: %s", out)
	}
}

func TestSpecUpdate_UpdatesTitleBodyEvidence(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "Original title", "--body", "original body"})
	id := strings.TrimSpace(stdout)

	_, stderr, code := ExecuteCapture([]string{"spec", "update", id, "--title", "New title", "--body", "new body", "--evidence", "docs/x.md"})
	if code != 0 {
		t.Fatalf("update failed (%d): %s", code, stderr)
	}

	out, _, _ := ExecuteCapture([]string{"show", id})
	if !strings.Contains(out, "New title") || !strings.Contains(out, "new body") || !strings.Contains(out, "docs/x.md") {
		t.Errorf("expected updated fields, got: %s", out)
	}
}

func TestSpecUpdate_BodyFromStdin(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "A spec"})
	id := strings.TrimSpace(stdout)

	_, _, code := ExecuteCaptureStdin([]string{"spec", "update", id, "--body", "-"}, "updated from stdin\n")
	if code != 0 {
		t.Fatalf("update from stdin failed")
	}

	out, _, _ := ExecuteCapture([]string{"show", id})
	if !strings.Contains(out, "updated from stdin") {
		t.Errorf("expected stdin body, got: %s", out)
	}
}

func TestSpecUpdate_RefusedOnSuperseded(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "Old spec"})
	oldID := strings.TrimSpace(stdout)
	stdout, _, _ = ExecuteCapture([]string{"spec", "add", "New spec"})
	newID := strings.TrimSpace(stdout)
	ExecuteCapture([]string{"spec", "supersede", oldID, newID})

	_, _, code := ExecuteCapture([]string{"spec", "update", oldID, "--title", "renamed"})
	if code != 2 {
		t.Fatalf("expected exit 2 updating a superseded spec, got %d", code)
	}
}

func TestSpecUpdate_RefusedEmptyingDecisionsOnActiveSpec(t *testing.T) {
	initRepo(t)
	decisionID := addDecisionCard(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "A spec", "--decision", decisionID})
	id := strings.TrimSpace(stdout)
	if _, stderr, code := ExecuteCapture([]string{"spec", "activate", id}); code != 0 {
		t.Fatalf("activate failed (%d): %s", code, stderr)
	}

	out, _, code := ExecuteCapture([]string{"spec", "update", id, "--decision", "", "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2 emptying decisions on an active spec, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"error":"spec_without_decision"`) {
		t.Errorf("expected spec_without_decision error shape, got: %s", out)
	}

	// The spec must still be active with its decision intact.
	show, _, _ := ExecuteCapture([]string{"show", id, "--json"})
	if !strings.Contains(show, `"status": "active"`) || !strings.Contains(show, decisionID) {
		t.Errorf("expected spec to remain active with its decision, got: %s", show)
	}
}

func TestSpecSupersede_MarksOldAndLogsEvent(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "Old spec"})
	oldID := strings.TrimSpace(stdout)
	stdout, _, _ = ExecuteCapture([]string{"spec", "add", "New spec"})
	newID := strings.TrimSpace(stdout)

	_, stderr, code := ExecuteCapture([]string{"spec", "supersede", oldID, newID})
	if code != 0 {
		t.Fatalf("supersede failed (%d): %s", code, stderr)
	}

	out, _, _ := ExecuteCapture([]string{"show", oldID, "--json"})
	if !strings.Contains(out, `"status": "superseded"`) || !strings.Contains(out, newID) {
		t.Errorf("expected old spec superseded by new, got: %s", out)
	}

	logOut, _, _ := ExecuteCapture([]string{"log", "--json"})
	if !strings.Contains(logOut, oldID) || !strings.Contains(logOut, `"kind": "spec"`) {
		t.Errorf("expected supersede event with kind spec in log, got: %s", logOut)
	}
}

func TestSpecSupersede_NotFound(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "A spec"})
	id := strings.TrimSpace(stdout)

	_, _, code := ExecuteCapture([]string{"spec", "supersede", "zzzz", id})
	if code != 2 {
		t.Fatalf("expected exit 2 for nonexistent old spec, got %d", code)
	}

	_, _, code = ExecuteCapture([]string{"spec", "supersede", id, "zzzz"})
	if code != 2 {
		t.Fatalf("expected exit 2 for nonexistent new spec, got %d", code)
	}
}

func TestSpecList_ReadOnly_IncludesOpenTaskCount(t *testing.T) {
	initRepo(t)
	decisionID := addDecisionCard(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "A spec", "--decision", decisionID})
	id := strings.TrimSpace(stdout)
	ExecuteCapture([]string{"spec", "activate", id})
	ExecuteCapture([]string{"task", "add", "Linked task", "--spec", id})

	out, _, code := ExecuteCapture([]string{"spec", "list", "--json"})
	if code != 0 {
		t.Fatalf("spec list failed")
	}
	if !strings.Contains(out, id) {
		t.Errorf("expected spec id in list, got: %s", out)
	}
	if !strings.Contains(out, `"open_tasks": 1`) {
		t.Errorf("expected open_tasks count of 1, got: %s", out)
	}
	if !strings.Contains(out, decisionID) {
		t.Errorf("expected linked decision id in --json list, got: %s", out)
	}

	textOut, _, code := ExecuteCapture([]string{"spec", "list"})
	if code != 0 {
		t.Fatalf("spec list (text) failed")
	}
	if !strings.Contains(textOut, id) || !strings.Contains(textOut, "1 open tasks") {
		t.Errorf("expected id and open-task count in text list, got: %s", textOut)
	}
	if !strings.Contains(textOut, decisionID) {
		t.Errorf("expected linked decision id in text list, got: %s", textOut)
	}
}

func TestTaskAdd_WithSpec_Success(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "A spec"})
	specID := strings.TrimSpace(stdout)

	stdout, stderr, code := ExecuteCapture([]string{"task", "add", "Task linked to spec", "--spec", specID})
	if code != 0 {
		t.Fatalf("task add --spec failed (%d): %s", code, stderr)
	}
	taskID := strings.TrimSpace(stdout)

	out, _, _ := ExecuteCapture([]string{"show", taskID, "--json"})
	if !strings.Contains(out, specID) {
		t.Errorf("expected spec id on workitem, got: %s", out)
	}
}

func TestTaskAdd_WithSpec_NotFound(t *testing.T) {
	initRepo(t)

	out, _, code := ExecuteCapture([]string{"task", "add", "Task", "--spec", "zzzz", "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2 for nonexistent spec, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"error":"spec_not_found"`) {
		t.Errorf("expected spec_not_found error shape, got: %s", out)
	}
}

func TestTaskAdd_WithSpec_Superseded(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "Old spec"})
	oldID := strings.TrimSpace(stdout)
	stdout, _, _ = ExecuteCapture([]string{"spec", "add", "New spec"})
	newID := strings.TrimSpace(stdout)
	ExecuteCapture([]string{"spec", "supersede", oldID, newID})

	out, _, code := ExecuteCapture([]string{"task", "add", "Task", "--spec", oldID, "--json"})
	if code != 2 {
		t.Fatalf("expected exit 2 for superseded spec, got %d: %s", code, out)
	}
	if !strings.Contains(out, `"error":"spec_superseded"`) {
		t.Errorf("expected spec_superseded error shape, got: %s", out)
	}
	if !strings.Contains(out, newID) {
		t.Errorf("expected superseded_by in error, got: %s", out)
	}
}

func TestContextTarget_ShowsLinkedSpec(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "Linked spec", "--body", "spec body text"})
	specID := strings.TrimSpace(stdout)
	ExecuteCapture([]string{"spec", "activate", specID})

	stdout, _, _ = ExecuteCapture([]string{"task", "add", "Task with spec", "--spec", specID})
	taskID := strings.TrimSpace(stdout)

	out, _, code := ExecuteCapture([]string{"context", taskID})
	if code != 0 {
		t.Fatalf("context failed")
	}
	if !strings.Contains(out, "## SPEC ["+specID+"]") {
		t.Errorf("expected SPEC section, got: %s", out)
	}
	if !strings.Contains(out, "spec body text") {
		t.Errorf("expected spec body, got: %s", out)
	}
}

func TestContext_ShowsSpecsSection(t *testing.T) {
	initRepo(t)

	ExecuteCapture([]string{"spec", "add", "A living spec"})

	out, _, code := ExecuteCapture([]string{"context"})
	if code != 0 {
		t.Fatalf("context failed")
	}
	if !strings.Contains(out, "## SPECS") {
		t.Errorf("expected SPECS section, got: %s", out)
	}
	if !strings.Contains(out, "A living spec") {
		t.Errorf("expected spec title, got: %s", out)
	}
}

func TestState_ShowsSpecsSection(t *testing.T) {
	initRepo(t)

	ExecuteCapture([]string{"spec", "add", "A spec for state"})

	out, _, code := ExecuteCapture([]string{"state"})
	if code != 0 {
		t.Fatalf("state failed")
	}
	if !strings.Contains(out, "SPECS") {
		t.Errorf("expected specs section, got: %s", out)
	}
	if !strings.Contains(out, "A spec for state") {
		t.Errorf("expected spec title, got: %s", out)
	}
}

func TestShow_ExtendsToSpecs(t *testing.T) {
	initRepo(t)

	stdout, _, _ := ExecuteCapture([]string{"spec", "add", "Show me"})
	id := strings.TrimSpace(stdout)

	out, _, code := ExecuteCapture([]string{"show", id})
	if code != 0 {
		t.Fatalf("show failed")
	}
	if !strings.Contains(out, "Show me") {
		t.Errorf("expected spec content, got: %s", out)
	}
}
