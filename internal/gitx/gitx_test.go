package gitx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dmarcocci/atlas/internal/testutil"
	"github.com/stretchr/testify/require"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestRoot(t *testing.T) {
	repo := testutil.SetupRepo(t)

	root, err := Root(repo)
	require.NoError(t, err)

	// resolve symlinks (e.g. macOS /tmp -> /private/tmp) before comparing
	wantRoot, err := filepath.EvalSymlinks(repo)
	require.NoError(t, err)
	gotRoot, err := filepath.EvalSymlinks(root)
	require.NoError(t, err)
	require.Equal(t, wantRoot, gotRoot)
}

func TestRoot_NotARepo(t *testing.T) {
	dir := t.TempDir()

	_, err := Root(dir)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotARepo))
}

func TestCommonDir_IsAbsolute(t *testing.T) {
	repo := testutil.SetupRepo(t)

	cd, err := CommonDir(repo)
	require.NoError(t, err)
	require.True(t, filepath.IsAbs(cd))

	info, err := os.Stat(cd)
	require.NoError(t, err)
	require.True(t, info.IsDir())
}

func TestCommonDir_Worktree_MatchesMain(t *testing.T) {
	repo := testutil.SetupRepo(t)
	wt := testutil.SetupWorktree(t, repo, "feature/wt")

	mainCommon, err := CommonDir(repo)
	require.NoError(t, err)

	wtCommon, err := CommonDir(wt)
	require.NoError(t, err)

	mainResolved, err := filepath.EvalSymlinks(mainCommon)
	require.NoError(t, err)
	wtResolved, err := filepath.EvalSymlinks(wtCommon)
	require.NoError(t, err)

	require.Equal(t, mainResolved, wtResolved)
}

func TestCommonDir_NotARepo(t *testing.T) {
	dir := t.TempDir()

	_, err := CommonDir(dir)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotARepo))
}

func TestBranch(t *testing.T) {
	repo := testutil.SetupRepo(t)

	branch, err := Branch(repo)
	require.NoError(t, err)
	require.Equal(t, "main", branch)
}

func TestBranch_Worktree(t *testing.T) {
	repo := testutil.SetupRepo(t)
	wt := testutil.SetupWorktree(t, repo, "feature/xyz")

	branch, err := Branch(wt)
	require.NoError(t, err)
	require.Equal(t, "feature/xyz", branch)
}

func TestBranch_DetachedHEAD(t *testing.T) {
	repo := testutil.SetupRepo(t)
	runGit(t, repo, "checkout", "--detach", "HEAD")

	branch, err := Branch(repo)
	require.NoError(t, err)
	require.Equal(t, "HEAD", branch)
}

func TestBranch_NotARepo(t *testing.T) {
	dir := t.TempDir()

	_, err := Branch(dir)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotARepo))
}

func TestHeadShort(t *testing.T) {
	repo := testutil.SetupRepo(t)

	short, err := HeadShort(repo)
	require.NoError(t, err)
	require.NotEmpty(t, short)
	require.LessOrEqual(t, len(short), 12)
}

func TestHeadShort_NotARepo(t *testing.T) {
	dir := t.TempDir()

	_, err := HeadShort(dir)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotARepo))
}

func TestIsDirty_Clean(t *testing.T) {
	repo := testutil.SetupRepo(t)

	dirty, count, err := IsDirty(repo)
	require.NoError(t, err)
	require.False(t, dirty)
	require.Equal(t, 0, count)
}

func TestIsDirty_WithChanges(t *testing.T) {
	repo := testutil.SetupRepo(t)

	require.NoError(t, os.WriteFile(filepath.Join(repo, "a.txt"), []byte("a"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "b.txt"), []byte("b"), 0o644))

	dirty, count, err := IsDirty(repo)
	require.NoError(t, err)
	require.True(t, dirty)
	require.Equal(t, 2, count)
}

func TestIsDirty_NotARepo(t *testing.T) {
	dir := t.TempDir()

	_, _, err := IsDirty(dir)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotARepo))
}

func TestRecentCommits(t *testing.T) {
	repo := testutil.SetupRepo(t)

	for i := 0; i < 3; i++ {
		p := filepath.Join(repo, "file.txt")
		require.NoError(t, os.WriteFile(p, []byte(time.Now().String()), 0o644))
		runGit(t, repo, "add", "-A")
		runGit(t, repo, "commit", "-m", "chore: update")
	}

	commits, err := RecentCommits(repo, 2)
	require.NoError(t, err)
	require.Len(t, commits, 2)
	for _, c := range commits {
		require.NotEmpty(t, c)
	}
}

func TestRecentCommits_NMoreThanAvailable(t *testing.T) {
	repo := testutil.SetupRepo(t) // exactly 1 commit

	commits, err := RecentCommits(repo, 5)
	require.NoError(t, err)
	require.Len(t, commits, 1)
}

func TestRecentCommits_NotARepo(t *testing.T) {
	dir := t.TempDir()

	_, err := RecentCommits(dir, 5)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotARepo))
}

func TestCommitTimestamps(t *testing.T) {
	repo := testutil.SetupRepo(t)

	ts, err := CommitTimestamps(repo, 5)
	require.NoError(t, err)
	require.Len(t, ts, 1)
	require.WithinDuration(t, time.Now(), ts[0], 5*time.Minute)
}

func TestCommitTimestamps_NotARepo(t *testing.T) {
	dir := t.TempDir()

	_, err := CommitTimestamps(dir, 5)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNotARepo))
}
