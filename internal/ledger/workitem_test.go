package ledger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleWorkitem() Workitem {
	return Workitem{
		ID:      "a1b2",
		Title:   "Fix container reconcile retry",
		Status:  "todo",
		Created: "2026-08-27",
		Evidence: []string{
			"packages/core/pipeline/reconcile.py:120-180",
		},
		Body: "Investigate the retry backoff and fix the off-by-one.\n",
	}
}

func TestSaveWorkitem_ThenLoadWorkitem_Roundtrip(t *testing.T) {
	root := setupLedgerRoot(t)
	w := sampleWorkitem()

	require.NoError(t, SaveWorkitem(root, w))

	got, err := LoadWorkitem(root, "a1b2")
	require.NoError(t, err)
	require.Equal(t, w, got)
}

func TestSaveWorkitem_UsesIDAndSlugForFilename(t *testing.T) {
	root := setupLedgerRoot(t)
	w := sampleWorkitem()
	require.NoError(t, SaveWorkitem(root, w))

	require.FileExists(t, filepath.Join(root, ".atlas", "work", "a1b2-fix-container-reconcile-retry.md"))
}

func TestSaveWorkitem_RenamesFileWhenTitleChanges(t *testing.T) {
	root := setupLedgerRoot(t)
	w := sampleWorkitem()
	require.NoError(t, SaveWorkitem(root, w))

	w.Title = "A completely different title"
	require.NoError(t, SaveWorkitem(root, w))

	require.NoFileExists(t, filepath.Join(root, ".atlas", "work", "a1b2-fix-container-reconcile-retry.md"))
	require.FileExists(t, filepath.Join(root, ".atlas", "work", "a1b2-a-completely-different-title.md"))

	got, err := LoadWorkitem(root, "a1b2")
	require.NoError(t, err)
	require.Equal(t, "A completely different title", got.Title)
}

func TestLoadWorkitem_NotFoundReturnsTypedError(t *testing.T) {
	root := setupLedgerRoot(t)

	_, err := LoadWorkitem(root, "zzzz")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestListWorkitems_ReturnsAllInWorkDir(t *testing.T) {
	root := setupLedgerRoot(t)
	w1 := sampleWorkitem()
	w2 := sampleWorkitem()
	w2.ID = "c3d4"
	w2.Title = "Second task"
	require.NoError(t, SaveWorkitem(root, w1))
	require.NoError(t, SaveWorkitem(root, w2))

	items, err := ListWorkitems(root)
	require.NoError(t, err)
	require.Len(t, items, 2)

	ids := []string{items[0].ID, items[1].ID}
	require.ElementsMatch(t, []string{"a1b2", "c3d4"}, ids)
}

func TestListWorkitems_EmptyWhenNoWorkitems(t *testing.T) {
	root := setupLedgerRoot(t)

	items, err := ListWorkitems(root)
	require.NoError(t, err)
	require.Empty(t, items)
}

func TestLoadWorkitem_MalformedFrontmatterIsTolerant(t *testing.T) {
	root := setupLedgerRoot(t)
	badPath := filepath.Join(root, ".atlas", "work", "a1b2-broken.md")
	require.NoError(t, os.WriteFile(badPath, []byte("---\nid: a1b2\ntitle: broken\n"), 0o644))

	require.NotPanics(t, func() {
		_, _ = LoadWorkitem(root, "a1b2")
	})

	_, err := LoadWorkitem(root, "a1b2")
	require.Error(t, err)
	var target *ErrMalformedFrontmatter
	require.ErrorAs(t, err, &target)
}

func TestIsValidStatus(t *testing.T) {
	require.True(t, IsValidStatus("todo"))
	require.True(t, IsValidStatus("doing"))
	require.True(t, IsValidStatus("blocked"))
	require.True(t, IsValidStatus("done"))
	require.False(t, IsValidStatus("bogus"))
	require.False(t, IsValidStatus(""))
}

func TestCanTransition_ValidMoves(t *testing.T) {
	require.True(t, CanTransition("todo", "doing"))
	require.True(t, CanTransition("todo", "blocked"))
	require.True(t, CanTransition("doing", "blocked"))
	require.True(t, CanTransition("doing", "done"))
	require.True(t, CanTransition("blocked", "doing"))
	require.True(t, CanTransition("blocked", "todo"))
}

func TestCanTransition_InvalidMoves(t *testing.T) {
	require.False(t, CanTransition("done", "todo"))
	require.False(t, CanTransition("done", "doing"))
	require.False(t, CanTransition("todo", "bogus"))
	require.False(t, CanTransition("bogus", "todo"))
}
