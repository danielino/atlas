// Package testutil provides shared helpers for tests across ATLAS packages.
package testutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// SetupRepo creates a temporary git repository in t.TempDir(), configures a
// local user identity (so commits work without any global git config) and
// disables commit signing, then makes an initial commit on branch "main".
// It returns the absolute path to the repository root.
func SetupRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()

	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	run("init", "-b", "main")
	run("config", "user.name", "Atlas Test")
	run("config", "user.email", "atlas-test@example.com")
	run("config", "commit.gpgsign", "false")

	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# test repo\n"), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	run("add", "-A")
	run("commit", "-m", "chore: initial commit")

	return dir
}

// SetupWorktree creates a git worktree for repo at a new branch, rooted in a
// fresh temp directory, and returns its absolute path. Intended for use from
// phase F2 onward (claims/gitx tests); provided here so callers don't need to
// duplicate repo bootstrapping.
func SetupWorktree(t *testing.T, repo string, branch string) string {
	t.Helper()

	wtDir := t.TempDir()
	wtPath := filepath.Join(wtDir, branch)

	cmd := exec.Command("git", "-C", repo, "worktree", "add", "-b", branch, wtPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree add failed: %v\n%s", err, out)
	}

	return wtPath
}
