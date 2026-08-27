package claims

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/dmarcocci/atlas/internal/gitx"
	"github.com/dmarcocci/atlas/internal/testutil"
	"github.com/stretchr/testify/require"
)

func newManager(t *testing.T, dir string, now time.Time) *Manager {
	t.Helper()
	commonDir, err := gitx.CommonDir(dir)
	require.NoError(t, err)

	return &Manager{
		CommonDir: commonDir,
		Session:   "test-session",
		Now:       func() time.Time { return now },
	}
}

func TestAcquire_Succeeds_And_CreatesFile(t *testing.T) {
	repo := testutil.SetupRepo(t)
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	m := newManager(t, repo, now)

	c, err := m.Acquire("a1b2", "feature/x")
	require.NoError(t, err)
	require.Equal(t, "a1b2", c.ID)
	require.Equal(t, "feature/x", c.Branch)
	require.Equal(t, "test-session", c.Session)
	require.Equal(t, now, c.Created)

	commonDir, err := gitx.CommonDir(repo)
	require.NoError(t, err)
	path := filepath.Join(commonDir, "atlas", "claims", "a1b2.json")
	require.FileExists(t, path)
}

func TestAcquire_AlreadyClaimed_ReturnsErrClaimed(t *testing.T) {
	repo := testutil.SetupRepo(t)
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	m := newManager(t, repo, now)

	_, err := m.Acquire("a1b2", "feature/x")
	require.NoError(t, err)

	_, err = m.Acquire("a1b2", "feature/y")
	require.Error(t, err)

	var claimedErr *ErrClaimed
	require.True(t, errors.As(err, &claimedErr))
	require.Equal(t, "feature/x", claimedErr.Existing.Branch)
	require.Equal(t, "test-session", claimedErr.Existing.Session)
}

func TestAcquire_ExpiredClaim_IsReacquirable(t *testing.T) {
	repo := testutil.SetupRepo(t)
	created := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	mOld := newManager(t, repo, created)

	_, err := mOld.Acquire("a1b2", "feature/x")
	require.NoError(t, err)

	// ttl_hours defaults to 24; move "now" far enough forward to expire it.
	later := created.Add(48 * time.Hour)
	mNew := newManager(t, repo, later)

	c, err := mNew.Acquire("a1b2", "feature/y")
	require.NoError(t, err)
	require.Equal(t, "feature/y", c.Branch)
	require.Equal(t, later, c.Created)
}

func TestGet(t *testing.T) {
	repo := testutil.SetupRepo(t)
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	m := newManager(t, repo, now)

	_, ok := m.Get("nope")
	require.False(t, ok)

	_, err := m.Acquire("a1b2", "feature/x")
	require.NoError(t, err)

	c, ok := m.Get("a1b2")
	require.True(t, ok)
	require.Equal(t, "feature/x", c.Branch)
}

func TestList_OnlyActiveClaims(t *testing.T) {
	repo := testutil.SetupRepo(t)
	created := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	mOld := newManager(t, repo, created)

	_, err := mOld.Acquire("expired", "feature/old")
	require.NoError(t, err)

	later := created.Add(48 * time.Hour)
	mNew := newManager(t, repo, later)

	_, err = mNew.Acquire("active", "feature/new")
	require.NoError(t, err)

	list, err := mNew.List()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "active", list[0].ID)
}

func TestRelease_Idempotent(t *testing.T) {
	repo := testutil.SetupRepo(t)
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	m := newManager(t, repo, now)

	_, err := m.Acquire("a1b2", "feature/x")
	require.NoError(t, err)

	require.NoError(t, m.Release("a1b2"))
	_, ok := m.Get("a1b2")
	require.False(t, ok)

	// releasing again, and releasing a claim that never existed, is not an error
	require.NoError(t, m.Release("a1b2"))
	require.NoError(t, m.Release("never-existed"))
}

func TestSteal_RemovesUnexpiredClaimAndAcquires(t *testing.T) {
	repo := testutil.SetupRepo(t)
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	m := newManager(t, repo, now)

	_, err := m.Acquire("a1b2", "feature/x")
	require.NoError(t, err)

	c, err := m.Steal("a1b2", "feature/y")
	require.NoError(t, err)
	require.Equal(t, "feature/y", c.Branch)

	got, ok := m.Get("a1b2")
	require.True(t, ok)
	require.Equal(t, "feature/y", got.Branch)
}

func TestCleanup_RemovesExpiredClaims(t *testing.T) {
	repo := testutil.SetupRepo(t)
	created := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	mOld := newManager(t, repo, created)

	_, err := mOld.Acquire("expired1", "feature/a")
	require.NoError(t, err)
	_, err = mOld.Acquire("expired2", "feature/b")
	require.NoError(t, err)

	later := created.Add(48 * time.Hour)
	mNew := newManager(t, repo, later)
	_, err = mNew.Acquire("active", "feature/c")
	require.NoError(t, err)

	removed, err := mNew.Cleanup()
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"expired1", "expired2"}, removed)

	list, err := mNew.List()
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "active", list[0].ID)
}

func TestClaims_WorktreeAndMainCheckout_ShareSameDirectory(t *testing.T) {
	repo := testutil.SetupRepo(t)
	wt := testutil.SetupWorktree(t, repo, "feature/wt")
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)

	mMain := newManager(t, repo, now)
	mWt := newManager(t, wt, now)

	_, err := mMain.Acquire("a1b2", "main")
	require.NoError(t, err)

	// The worktree manager, resolving CommonDir from inside the worktree,
	// must see the claim created from the main checkout.
	c, ok := mWt.Get("a1b2")
	require.True(t, ok)
	require.Equal(t, "main", c.Branch)
}

func TestAcquire_Concurrent_ExactlyOneWins(t *testing.T) {
	repo := testutil.SetupRepo(t)
	now := time.Date(2026, 8, 27, 10, 0, 0, 0, time.UTC)
	m := newManager(t, repo, now)

	const n = 8
	var wg sync.WaitGroup
	successes := make([]bool, n)
	errs := make([]error, n)

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := m.Acquire("shared-id", "feature/x")
			errs[idx] = err
			successes[idx] = err == nil
		}(i)
	}
	wg.Wait()

	successCount := 0
	claimedCount := 0
	for i := 0; i < n; i++ {
		if successes[i] {
			successCount++
			continue
		}
		var claimedErr *ErrClaimed
		if errors.As(errs[i], &claimedErr) {
			claimedCount++
		}
	}

	require.Equal(t, 1, successCount)
	require.Equal(t, n-1, claimedCount)
}

func TestDefaultSession_UsesEnvOrHostnamePID(t *testing.T) {
	session := DefaultSession()
	require.NotEmpty(t, session)
}
