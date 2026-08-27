// Package gitx wraps the system git binary as a subprocess, exposing the
// small surface ATLAS needs (root, common dir, branch, head, dirty state,
// recent history). Every function runs "git -C dir ...": no libgit2, no
// network. Callers must degrade gracefully when a directory is not a git
// repository (ErrNotARepo) — this package never panics.
package gitx

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ErrNotARepo is returned when dir is not inside a git working tree.
var ErrNotARepo = errors.New("gitx: not a git repository")

// run executes "git -C dir <args...>" and returns trimmed stdout. On
// failure it wraps stderr content; if the failure looks like "not a git
// repository", it returns ErrNotARepo instead so callers can degrade.
func run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		stderr := ""
		if errors.As(err, &exitErr) {
			stderr = strings.TrimSpace(string(exitErr.Stderr))
		}
		if isNotARepoMessage(stderr) {
			return "", ErrNotARepo
		}
		if stderr != "" {
			return "", fmt.Errorf("gitx: git %s: %s", strings.Join(args, " "), stderr)
		}
		return "", fmt.Errorf("gitx: git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

func isNotARepoMessage(stderr string) bool {
	lower := strings.ToLower(stderr)
	return strings.Contains(lower, "not a git repository")
}

// Root returns the absolute top-level working directory for the repo
// containing dir.
func Root(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return filepath.Abs(out)
}

// CommonDir returns the absolute git common directory for the repo
// containing dir ("git rev-parse --git-common-dir"). For a linked
// worktree this is the main checkout's .git directory, shared across all
// worktrees — git may report it as a relative path, so CommonDir always
// resolves it to absolute using dir as the base.
func CommonDir(dir string) (string, error) {
	out, err := run(dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(out) {
		return out, nil
	}
	return filepath.Abs(filepath.Join(dir, out))
}

// Branch returns the current branch name (e.g. "main"). Detached HEAD
// yields whatever "git rev-parse --abbrev-ref HEAD" prints ("HEAD").
func Branch(dir string) (string, error) {
	return run(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// HeadShort returns the short hash of HEAD.
func HeadShort(dir string) (string, error) {
	return run(dir, "rev-parse", "--short", "HEAD")
}

// IsDirty reports whether the working tree has uncommitted changes
// (tracked or untracked), and how many files are affected.
func IsDirty(dir string) (bool, int, error) {
	out, err := run(dir, "status", "--porcelain")
	if err != nil {
		return false, 0, err
	}
	if out == "" {
		return false, 0, nil
	}
	lines := strings.Split(out, "\n")
	return true, len(lines), nil
}

// RecentCommits returns up to n most recent commits in one-line form
// ("<short-hash> <subject>"), most recent first.
func RecentCommits(dir string, n int) ([]string, error) {
	out, err := run(dir, "log", "--oneline", "-n", strconv.Itoa(n))
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []string{}, nil
	}
	return strings.Split(out, "\n"), nil
}

// CommitTimestamps returns the commit timestamps of up to n most recent
// commits, most recent first.
func CommitTimestamps(dir string, n int) ([]time.Time, error) {
	out, err := run(dir, "log", "-n", strconv.Itoa(n), "--date=iso-strict", "--pretty=format:%cI")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return []time.Time{}, nil
	}

	lines := strings.Split(out, "\n")
	times := make([]time.Time, 0, len(lines))
	for _, line := range lines {
		t, err := time.Parse(time.RFC3339, line)
		if err != nil {
			return nil, fmt.Errorf("gitx: parse commit timestamp %q: %w", line, err)
		}
		times = append(times, t)
	}
	return times, nil
}
