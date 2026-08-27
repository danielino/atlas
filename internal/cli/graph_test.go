package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// graphJSONNode mirrors the S10.1 `{"id","title","status","blocked_by":[...]}`
// node shape, for structural (not just substring) assertions on `--json`.
type graphJSONNode struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Status    string   `json:"status"`
	BlockedBy []string `json:"blocked_by"`
}

type graphJSONDoc struct {
	Levels [][]graphJSONNode `json:"levels"`
	Cycles []graphJSONNode   `json:"cycles"`
}

// addTask creates a todo workitem, optionally blocked by the given ids,
// and returns its id.
func addTask(t *testing.T, title string, blockedBy ...string) string {
	t.Helper()
	args := []string{"task", "add", title}
	if len(blockedBy) > 0 {
		args = append(args, "--blocked-by", strings.Join(blockedBy, ","))
	}
	stdout, stderr, code := ExecuteCapture(args)
	if code != 0 {
		t.Fatalf("task add failed (%d): %s", code, stderr)
	}
	return strings.TrimSpace(stdout)
}

func TestGraph_NeverInContext(t *testing.T) {
	initRepo(t)
	addTask(t, "root task")

	out, _, code := ExecuteCapture([]string{"context"})
	if code != 0 {
		t.Fatalf("context failed")
	}
	if strings.Contains(out, "GRAPH") {
		t.Errorf("graph must never appear in context output, got: %s", out)
	}
}

func TestGraph_TextFormat_MultiLevel(t *testing.T) {
	initRepo(t)
	a := addTask(t, "root task")
	c := addTask(t, "second task", a)

	stdout, stderr, code := ExecuteCapture([]string{"graph"})
	if code != 0 {
		t.Fatalf("graph failed (%d): %s", code, stderr)
	}

	want := "# ATLAS GRAPH\n" +
		"Level 0 (unblocked, parallelizable):\n" +
		"- [" + a + "] root task (todo)\n" +
		"Level 1:\n" +
		"- [" + c + "] second task (todo, blocked by " + a + ")\n"
	if stdout != want {
		t.Errorf("got:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestGraph_NoActiveWorkitems(t *testing.T) {
	initRepo(t)
	stdout, _, code := ExecuteCapture([]string{"graph"})
	if code != 0 {
		t.Fatalf("graph failed")
	}
	if !strings.Contains(stdout, "# ATLAS GRAPH") {
		t.Errorf("expected header, got: %s", stdout)
	}
	if !strings.Contains(stdout, "no active workitems") {
		t.Errorf("expected empty-state message, got: %s", stdout)
	}
}

func TestGraph_BlockerClosed_DoesNotBlock(t *testing.T) {
	initRepo(t)
	a := addTask(t, "will be closed")
	c := addTask(t, "depends on a", a)

	if _, stderr, code := ExecuteCapture([]string{"task", "start", a}); code != 0 {
		t.Fatalf("task start failed: %s", stderr)
	}
	if _, stderr, code := ExecuteCapture([]string{"task", "done", a, "--summary", "done"}); code != 0 {
		t.Fatalf("task done failed: %s", stderr)
	}

	stdout, _, code := ExecuteCapture([]string{"graph"})
	if code != 0 {
		t.Fatalf("graph failed")
	}
	want := "# ATLAS GRAPH\n" +
		"Level 0 (unblocked, parallelizable):\n" +
		"- [" + c + "] depends on a (todo)\n"
	if stdout != want {
		t.Errorf("got:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestGraph_Cycle_WarnsAndExitsZero(t *testing.T) {
	initRepo(t)
	a := addTask(t, "a task")
	c := addTask(t, "c task", a)

	// Manually close the cycle: a is blocked_by c, c is blocked_by a.
	stdout, stderr, code := ExecuteCapture([]string{"task", "block", a, "--on", c})
	if code != 0 {
		t.Fatalf("task block failed (%d): %s", code, stdout+stderr)
	}

	stdout, stderr, code = ExecuteCapture([]string{"graph"})
	if code != 0 {
		t.Fatalf("expected exit 0 even with a cycle, got %d", code)
	}
	if !strings.Contains(stdout, "Cycle (unresolvable):") {
		t.Errorf("expected cycle group in output, got: %s", stdout)
	}
	if !strings.Contains(stderr, "atlas doctor") {
		t.Errorf("expected warning mentioning atlas doctor, got: %s", stderr)
	}
}

func TestGraph_Mermaid(t *testing.T) {
	initRepo(t)
	a := addTask(t, "root task")
	c := addTask(t, "second task", a)

	stdout, stderr, code := ExecuteCapture([]string{"graph", "--mermaid"})
	if code != 0 {
		t.Fatalf("graph --mermaid failed (%d): %s", code, stderr)
	}

	want := "flowchart TD\n" +
		"    " + a + "[\"" + a + ": root task (todo)\"]\n" +
		"    " + c + "[\"" + c + ": second task (todo)\"]\n" +
		"    " + a + " --> " + c + "\n"
	if stdout != want {
		t.Errorf("got:\n%s\nwant:\n%s", stdout, want)
	}
}

func TestGraph_JSON_Shape(t *testing.T) {
	initRepo(t)
	a := addTask(t, "root task")
	c := addTask(t, "second task", a)

	stdout, stderr, code := ExecuteCapture([]string{"graph", "--json"})
	if code != 0 {
		t.Fatalf("graph --json failed (%d): %s", code, stderr)
	}

	var doc graphJSONDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	if len(doc.Levels) != 2 {
		t.Fatalf("expected 2 levels, got %d: %+v", len(doc.Levels), doc.Levels)
	}
	want0 := graphJSONNode{ID: a, Title: "root task", Status: "todo", BlockedBy: []string{}}
	if len(doc.Levels[0]) != 1 || !reflect.DeepEqual(doc.Levels[0][0], want0) {
		t.Errorf("level 0 mismatch: %+v", doc.Levels[0])
	}
	if len(doc.Levels[1]) != 1 || doc.Levels[1][0].ID != c || len(doc.Levels[1][0].BlockedBy) != 1 || doc.Levels[1][0].BlockedBy[0] != a {
		t.Errorf("level 1 mismatch: %+v", doc.Levels[1])
	}
	if len(doc.Cycles) != 0 {
		t.Errorf("expected no cycles, got %+v", doc.Cycles)
	}
}

func TestGraph_JSON_Cycles(t *testing.T) {
	initRepo(t)
	a := addTask(t, "a task")
	c := addTask(t, "c task", a)
	if _, stderr, code := ExecuteCapture([]string{"task", "block", a, "--on", c}); code != 0 {
		t.Fatalf("task block failed: %s", stderr)
	}

	stdout, _, code := ExecuteCapture([]string{"graph", "--json"})
	if code != 0 {
		t.Fatalf("graph --json failed")
	}

	var doc graphJSONDoc
	if err := json.Unmarshal([]byte(stdout), &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}

	if len(doc.Levels) != 0 {
		t.Errorf("expected no levels once both nodes are on the cycle, got %+v", doc.Levels)
	}
	if len(doc.Cycles) != 2 {
		t.Fatalf("expected both nodes in cycles, got %+v", doc.Cycles)
	}
	byID := map[string]graphJSONNode{doc.Cycles[0].ID: doc.Cycles[0], doc.Cycles[1].ID: doc.Cycles[1]}
	if node, ok := byID[a]; !ok || node.BlockedBy[0] != c {
		t.Errorf("expected %s blocked by %s in cycle group, got %+v", a, c, doc.Cycles)
	}
	if node, ok := byID[c]; !ok || node.BlockedBy[0] != a {
		t.Errorf("expected %s blocked by %s in cycle group, got %+v", c, a, doc.Cycles)
	}
}
