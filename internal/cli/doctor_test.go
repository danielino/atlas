package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctor_CleanRepo_ExitZero(t *testing.T) {
	initRepo(t)
	ExecuteCapture([]string{"task", "add", "A task"})

	out, stderr, code := ExecuteCapture([]string{"doctor"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d, stdout=%q stderr=%q", code, out, stderr)
	}
	if !strings.Contains(out, "no problems found") {
		t.Errorf("expected clean report, got: %s", out)
	}
}

func TestDoctor_OrphanBlockedBy_ExitThree(t *testing.T) {
	dir := initRepo(t)
	badPath := filepath.Join(dir, ".atlas", "work", "a1b2-broken.md")
	content := "---\nid: a1b2\ntitle: broken\nstatus: todo\ncreated: 2026-08-27\nblocked_by: [ffff]\nsummary: \"\"\n---\nbody\n"
	if err := os.WriteFile(badPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, code := ExecuteCapture([]string{"doctor"})
	if code != 3 {
		t.Fatalf("expected exit 3, got %d, out=%s", code, out)
	}
	if !strings.Contains(out, "## ERRORS") || !strings.Contains(out, "ffff") {
		t.Errorf("expected orphan ref error mentioning ffff, got: %s", out)
	}
}

func TestDoctor_JSON_Shape(t *testing.T) {
	initRepo(t)
	out, _, code := ExecuteCapture([]string{"doctor", "--json"})
	if code != 0 {
		t.Fatalf("expected exit 0, got %d", code)
	}
	for _, key := range []string{`"errors"`, `"warnings"`, `"fixed"`} {
		if !strings.Contains(out, key) {
			t.Errorf("expected json key %s, got: %s", key, out)
		}
	}
}

func TestDoctor_MalformedFrontmatter_ExitThree(t *testing.T) {
	dir := initRepo(t)
	badPath := filepath.Join(dir, ".atlas", "cards", "bbbb-broken.md")
	if err := os.WriteFile(badPath, []byte("---\nid: bbbb\nno closing delimiter\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, code := ExecuteCapture([]string{"doctor", "--json"})
	if code != 3 {
		t.Fatalf("expected exit 3, got %d, out=%s", code, out)
	}
	if !strings.Contains(out, "malformed_frontmatter") {
		t.Errorf("expected malformed_frontmatter code, got: %s", out)
	}
}

func TestDoctor_NoLedger_ExitOne(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	// no git init, no `atlas init`: no .atlas directory at all.
	_, _, code := ExecuteCapture([]string{"doctor"})
	if code != 1 {
		t.Errorf("expected exit 1 with no ledger, got %d", code)
	}
}
