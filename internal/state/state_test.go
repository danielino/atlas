package state

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dmarcocci/atlas/internal/claims"
	"github.com/dmarcocci/atlas/internal/gitx"
	"github.com/dmarcocci/atlas/internal/ledger"
	"github.com/dmarcocci/atlas/internal/testutil"
	"github.com/stretchr/testify/require"
)

func wi(id, status string, blockedBy ...string) ledger.Workitem {
	return ledger.Workitem{ID: id, Title: "title " + id, Status: status, BlockedBy: blockedBy}
}

func TestReady_UnblockedTodo(t *testing.T) {
	items := []ledger.Workitem{wi("a1b2", "todo")}
	got := Ready(items, nil)
	require.Len(t, got, 1)
	require.Equal(t, "a1b2", got[0].ID)
}

func TestReady_ExcludesNonTodo(t *testing.T) {
	items := []ledger.Workitem{wi("a1b2", "doing"), wi("c3d4", "blocked"), wi("e5f6", "todo")}
	got := Ready(items, nil)
	require.Len(t, got, 1)
	require.Equal(t, "e5f6", got[0].ID)
}

func TestReady_BlockedByActiveItem_NotReady(t *testing.T) {
	items := []ledger.Workitem{
		wi("a1b2", "todo"),
		wi("c3d4", "todo", "a1b2"),
	}
	got := Ready(items, nil)
	require.Len(t, got, 1)
	require.Equal(t, "a1b2", got[0].ID)
}

func TestReady_BlockedByClosedItem_IsReady(t *testing.T) {
	items := []ledger.Workitem{
		wi("c3d4", "todo", "a1b2"),
	}
	closed := map[string]struct{}{"a1b2": {}}
	got := Ready(items, closed)
	require.Len(t, got, 1)
	require.Equal(t, "c3d4", got[0].ID)
}

func TestReady_BlockedByNonexistentItem_IsReady(t *testing.T) {
	items := []ledger.Workitem{
		wi("c3d4", "todo", "ffff"),
	}
	got := Ready(items, nil)
	require.Len(t, got, 1)
}

func TestReady_SortedByID(t *testing.T) {
	items := []ledger.Workitem{wi("f6a7", "todo"), wi("a1b2", "todo")}
	got := Ready(items, nil)
	require.Equal(t, []string{"a1b2", "f6a7"}, []string{got[0].ID, got[1].ID})
}

func TestNow_DoingAndBlocked(t *testing.T) {
	items := []ledger.Workitem{
		wi("a1b2", "doing"),
		wi("c3d4", "blocked"),
		wi("e5f6", "todo"),
	}
	got := Now(items)
	require.Len(t, got, 2)
	require.Equal(t, "a1b2", got[0].ID)
	require.Equal(t, "c3d4", got[1].ID)
}

func TestElsewhere_FiltersCurrentBranch(t *testing.T) {
	list := []claims.Claim{
		{ID: "a1b2", Branch: "feature/x"},
		{ID: "c3d4", Branch: "feature/y"},
	}
	got := Elsewhere(list, "feature/x")
	require.Len(t, got, 1)
	require.Equal(t, "c3d4", got[0].ID)
	require.Equal(t, "feature/y", got[0].Branch)
}

func TestElsewhere_Empty_WhenAllCurrentBranch(t *testing.T) {
	list := []claims.Claim{{ID: "a1b2", Branch: "feature/x"}}
	got := Elsewhere(list, "feature/x")
	require.Empty(t, got)
}

func TestStale_NoGitRepo_NotStale(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".atlas"), 0o755))
	stale, err := Stale(dir)
	require.NoError(t, err)
	require.False(t, stale)
}

func TestStale_FewerThanNCommits_NotStale(t *testing.T) {
	repo := testutil.SetupRepo(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".atlas"), 0o755))
	stale, err := Stale(repo)
	require.NoError(t, err)
	require.False(t, stale)
}

func commit(t *testing.T, repo, msg string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(repo, msg+".txt"), []byte(msg), 0o644))
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	run("add", "-A")
	run("commit", "-m", msg)
}

func TestStale_LedgerOlderThanFifthCommit_IsStale(t *testing.T) {
	repo := testutil.SetupRepo(t)
	atlasDir := filepath.Join(repo, ".atlas")
	require.NoError(t, os.MkdirAll(atlasDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(atlasDir, "focus.md"), []byte("old focus"), 0o644))

	old := time.Now().Add(-1 * time.Hour)
	require.NoError(t, os.Chtimes(filepath.Join(atlasDir, "focus.md"), old, old))

	for i := 0; i < 5; i++ {
		commit(t, repo, "commit-"+string(rune('a'+i)))
	}

	stale, err := Stale(repo)
	require.NoError(t, err)
	require.True(t, stale)
}

func TestStale_LedgerNewerThanCommits_NotStale(t *testing.T) {
	repo := testutil.SetupRepo(t)
	for i := 0; i < 5; i++ {
		commit(t, repo, "commit-"+string(rune('a'+i)))
	}

	atlasDir := filepath.Join(repo, ".atlas")
	require.NoError(t, os.MkdirAll(atlasDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(atlasDir, "focus.md"), []byte("fresh focus"), 0o644))

	stale, err := Stale(repo)
	require.NoError(t, err)
	require.False(t, stale)
}

func TestStale_EmptyAtlasDir_NotStale(t *testing.T) {
	repo := testutil.SetupRepo(t)
	for i := 0; i < 5; i++ {
		commit(t, repo, "commit-"+string(rune('a'+i)))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".atlas"), 0o755))

	stale, err := Stale(repo)
	require.NoError(t, err)
	require.False(t, stale)
}

func TestBuild_NoGitRepo_DegradesGracefully(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ledger.EnsureDirs(dir))
	require.NoError(t, ledger.WriteFocus(dir, "today's focus"))

	s, err := Build(dir, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.Equal(t, "today's focus", s.Focus)
	require.False(t, s.Stale)
	require.Equal(t, Ground{}, s.Ground)
}

func TestBuild_PopulatesFromLedger(t *testing.T) {
	repo := testutil.SetupRepo(t)
	require.NoError(t, ledger.EnsureDirs(repo))
	require.NoError(t, ledger.WriteFocus(repo, "focus text"))
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{ID: "a1b2", Title: "doing task", Status: "doing", Created: "2026-08-27"}))
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{ID: "c3d4", Title: "ready task", Status: "todo", Created: "2026-08-27"}))
	require.NoError(t, ledger.SaveCard(repo, ledger.Card{ID: "k9m2", Type: "decision", Title: "a decision", Status: "active", Hook: "hook text", Created: "2026-08-27"}))
	require.NoError(t, ledger.AppendLog(repo, ledger.LogEntry{ID: "e5f6", Kind: "task", Title: "closed task", Summary: "done it", Closed: time.Now().UTC().Format(time.RFC3339)}))

	s, err := Build(repo, ledger.DefaultConfig(), Options{Now: func() time.Time { return time.Now() }})
	require.NoError(t, err)
	require.Equal(t, "focus text", s.Focus)
	require.Len(t, s.Now, 1)
	require.Equal(t, "a1b2", s.Now[0].ID)
	require.Len(t, s.Ready, 1)
	require.Equal(t, "c3d4", s.Ready[0].ID)
	require.Len(t, s.ActiveCards, 1)
	require.Len(t, s.RecentClosed, 1)
	require.NotEmpty(t, s.Ground.Branch)
	require.NotEmpty(t, s.Ground.Head)
	require.NotEmpty(t, s.RecentCommits)
}

func TestBuild_RecentCommits_EmptyWithoutGit(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ledger.EnsureDirs(dir))
	s, err := Build(dir, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.Empty(t, s.RecentCommits)
}

func TestBuild_RecentClosed_RespectsWindow(t *testing.T) {
	repo := testutil.SetupRepo(t)
	require.NoError(t, ledger.EnsureDirs(repo))
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	require.NoError(t, ledger.AppendLog(repo, ledger.LogEntry{ID: "aaaa", Kind: "task", Closed: now.AddDate(0, 0, -1).Format(time.RFC3339)}))
	require.NoError(t, ledger.AppendLog(repo, ledger.LogEntry{ID: "bbbb", Kind: "task", Closed: now.AddDate(0, 0, -30).Format(time.RFC3339)}))

	cfg := ledger.DefaultConfig()
	cfg.Context.RecentDays = 7
	s, err := Build(repo, cfg, Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.Len(t, s.RecentClosed, 1)
	require.Equal(t, "aaaa", s.RecentClosed[0].ID)
}

func TestBuild_Elsewhere_ShowsOtherBranchClaims(t *testing.T) {
	repo := testutil.SetupRepo(t)
	require.NoError(t, ledger.EnsureDirs(repo))
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{ID: "a1b2", Title: "t", Status: "doing", Created: "2026-08-27"}))

	commonDir, err := gitx.CommonDir(repo)
	require.NoError(t, err)
	mgr := &claims.Manager{CommonDir: commonDir}
	_, err = mgr.Acquire("a1b2", "feature/other")
	require.NoError(t, err)

	s, err := Build(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.Len(t, s.Ground.Elsewhere, 1)
	require.Equal(t, "a1b2", s.Ground.Elsewhere[0].ID)
	require.Equal(t, "feature/other", s.Ground.Elsewhere[0].Branch)
}

func TestBuild_ErrorPropagation_MalformedWorkitem(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ledger.EnsureDirs(dir))
	badPath := filepath.Join(dir, ".atlas", "work", "aaaa-bad.md")
	require.NoError(t, os.WriteFile(badPath, []byte("---\nid: aaaa\nno closing delimiter\n"), 0o644))

	_, err := Build(dir, ledger.DefaultConfig(), Options{})
	require.Error(t, err)
}

func TestBuild_ErrorPropagation_MalformedLog(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ledger.EnsureDirs(dir))
	logPath := filepath.Join(dir, ".atlas", "log.jsonl")
	require.NoError(t, os.WriteFile(logPath, []byte("not valid json\n"), 0o644))

	_, err := Build(dir, ledger.DefaultConfig(), Options{})
	require.Error(t, err)
}

func TestBuild_ErrorPropagation_MalformedCard(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ledger.EnsureDirs(dir))
	badPath := filepath.Join(dir, ".atlas", "cards", "bbbb-bad.md")
	require.NoError(t, os.WriteFile(badPath, []byte("---\nid: bbbb\nno closing delimiter\n"), 0o644))

	_, err := Build(dir, ledger.DefaultConfig(), Options{})
	require.Error(t, err)
}

func TestBuild_DirtyWorktree_ReflectedInGround(t *testing.T) {
	repo := testutil.SetupRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "dirty.txt"), []byte("uncommitted"), 0o644))

	s, err := Build(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, s.Ground.Dirty)
	require.Equal(t, 1, s.Ground.DirtyCount)
}

func TestRecentClosed_SkipsUnparseableTimestamps(t *testing.T) {
	entries := []ledger.LogEntry{
		{ID: "a", Closed: "not-a-timestamp"},
		{ID: "b", Closed: "2026-08-20T00:00:00Z"},
	}
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)
	got := recentClosed(entries, 7, now)
	require.Len(t, got, 1)
	require.Equal(t, "b", got[0].ID)
}

func TestNewestMtime_MissingDir_NotFound(t *testing.T) {
	newest, found, err := newestMtime(filepath.Join(t.TempDir(), "does-not-exist"))
	require.NoError(t, err)
	require.False(t, found)
	require.True(t, newest.IsZero())
}

func TestBuild_Specs_DraftAndActiveIncluded_SupersededExcluded(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ledger.EnsureDirs(dir))
	require.NoError(t, ledger.SaveSpec(dir, ledger.Spec{ID: "3fa9", Title: "Draft spec", Status: "draft", Created: "2026-08-27"}))
	require.NoError(t, ledger.SaveSpec(dir, ledger.Spec{ID: "a1a1", Title: "Active spec", Status: "active", Created: "2026-08-27"}))
	require.NoError(t, ledger.SaveSpec(dir, ledger.Spec{ID: "b2b2", Title: "Old spec", Status: "superseded", SupersededBy: "a1a1", Created: "2026-08-01"}))

	s, err := Build(dir, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.Len(t, s.Specs, 2)
	require.Equal(t, "3fa9", s.Specs[0].ID)
	require.Equal(t, "a1a1", s.Specs[1].ID)
}

func TestBuild_Specs_OpenTaskCount(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ledger.EnsureDirs(dir))
	require.NoError(t, ledger.SaveSpec(dir, ledger.Spec{ID: "3fa9", Title: "Spec", Status: "active", Created: "2026-08-27"}))
	require.NoError(t, ledger.SaveWorkitem(dir, ledger.Workitem{ID: "aaaa", Title: "linked 1", Status: "todo", Created: "2026-08-27", Spec: "3fa9"}))
	require.NoError(t, ledger.SaveWorkitem(dir, ledger.Workitem{ID: "bbbb", Title: "linked 2", Status: "doing", Created: "2026-08-27", Spec: "3fa9"}))
	require.NoError(t, ledger.SaveWorkitem(dir, ledger.Workitem{ID: "cccc", Title: "unlinked", Status: "todo", Created: "2026-08-27"}))

	s, err := Build(dir, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.Len(t, s.Specs, 1)
	require.Equal(t, 2, s.Specs[0].OpenTasks)
}

func TestBuild_Specs_EmptyWhenNoneExist(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ledger.EnsureDirs(dir))
	s, err := Build(dir, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.Empty(t, s.Specs)
}
