package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/danielino/atlas/internal/testutil"
)

func TestInit_CreatesExpectedLayout(t *testing.T) {
	dir := testutil.SetupRepo(t)
	chdir(t, dir)

	_, stderr, code := ExecuteCapture([]string{"init"})
	if code != 0 {
		t.Fatalf("init failed (code %d): %s", code, stderr)
	}

	for _, p := range []string{
		filepath.Join(dir, ".atlas", "focus.md"),
		filepath.Join(dir, ".atlas", "config.toml"),
		filepath.Join(dir, ".atlas", "work"),
		filepath.Join(dir, ".atlas", "cards"),
		filepath.Join(dir, ".atlas", "specs"),
		filepath.Join(dir, ".gitattributes"),
		filepath.Join(dir, "AGENTS.md"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}

	// CLAUDE.md must NOT be created since it didn't exist before.
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Errorf("CLAUDE.md should not have been created")
	}

	agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), bootstrapBegin) || !strings.Contains(string(agents), bootstrapEnd) {
		t.Errorf("AGENTS.md missing bootstrap markers:\n%s", agents)
	}

	ga, err := os.ReadFile(filepath.Join(dir, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(ga)) != gitAttributesLine {
		t.Errorf(".gitattributes = %q, want exactly %q", string(ga), gitAttributesLine)
	}
}

func TestInit_BootstrapBlockMentionsSpecs(t *testing.T) {
	dir := testutil.SetupRepo(t)
	chdir(t, dir)

	if _, stderr, code := ExecuteCapture([]string{"init"}); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	agents, err := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(agents), "atlas task add --spec <id>") {
		t.Errorf("expected bootstrap block to mention `atlas task add --spec <id>`, got:\n%s", agents)
	}
}

func TestInit_PatchesExistingCLAUDEmd(t *testing.T) {
	dir := testutil.SetupRepo(t)
	claudePath := filepath.Join(dir, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte("# My rules\nDo not do X.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	if _, stderr, code := ExecuteCapture([]string{"init"}); code != 0 {
		t.Fatalf("init failed: %s", stderr)
	}

	data, err := os.ReadFile(claudePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "# My rules") || !strings.Contains(content, "Do not do X.") {
		t.Errorf("pre-existing CLAUDE.md content was lost:\n%s", content)
	}
	if !strings.Contains(content, bootstrapBegin) {
		t.Errorf("CLAUDE.md missing bootstrap block:\n%s", content)
	}
}

func TestInit_Idempotent(t *testing.T) {
	dir := testutil.SetupRepo(t)
	chdir(t, dir)

	if _, _, code := ExecuteCapture([]string{"init"}); code != 0 {
		t.Fatal("first init failed")
	}
	agentsBefore, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	gaBefore, _ := os.ReadFile(filepath.Join(dir, ".gitattributes"))

	if _, _, code := ExecuteCapture([]string{"init"}); code != 0 {
		t.Fatal("second init failed")
	}
	agentsAfter, _ := os.ReadFile(filepath.Join(dir, "AGENTS.md"))
	gaAfter, _ := os.ReadFile(filepath.Join(dir, ".gitattributes"))

	if string(agentsBefore) != string(agentsAfter) {
		t.Errorf("AGENTS.md changed on second init:\nbefore:\n%s\nafter:\n%s", agentsBefore, agentsAfter)
	}
	if strings.Count(string(agentsAfter), bootstrapBegin) != 1 {
		t.Errorf("expected exactly one bootstrap block, got content:\n%s", agentsAfter)
	}
	if string(gaBefore) != string(gaAfter) {
		t.Errorf(".gitattributes changed on second init")
	}
	if strings.Count(string(gaAfter), gitAttributesLine) != 1 {
		t.Errorf("expected exactly one merge=union line, got:\n%s", gaAfter)
	}
}

func TestInit_NeverOverwritesFocusOrConfig(t *testing.T) {
	dir := testutil.SetupRepo(t)
	chdir(t, dir)

	if _, _, code := ExecuteCapture([]string{"init"}); code != 0 {
		t.Fatal("init failed")
	}
	focusPath := filepath.Join(dir, ".atlas", "focus.md")
	if err := os.WriteFile(focusPath, []byte("Custom focus content.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, code := ExecuteCapture([]string{"init"}); code != 0 {
		t.Fatal("second init failed")
	}

	data, err := os.ReadFile(focusPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "Custom focus content.\n" {
		t.Errorf("focus.md was overwritten: %q", data)
	}
}

func TestInit_TextOutsideMarkersUntouched(t *testing.T) {
	dir := testutil.SetupRepo(t)
	agentsPath := filepath.Join(dir, "AGENTS.md")
	before := "# Pre-existing agent notes\nSomething important here.\n"
	if err := os.WriteFile(agentsPath, []byte(before), 0o644); err != nil {
		t.Fatal(err)
	}
	chdir(t, dir)

	if _, _, code := ExecuteCapture([]string{"init"}); code != 0 {
		t.Fatal("init failed")
	}
	if _, _, code := ExecuteCapture([]string{"init"}); code != 0 {
		t.Fatal("second init failed")
	}

	data, err := os.ReadFile(agentsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), before) {
		t.Errorf("pre-existing AGENTS.md text was not preserved verbatim:\n%s", data)
	}
}
