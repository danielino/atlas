package ledger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func setupLedgerRoot(t *testing.T) string {
	t.Helper()
	tmp := t.TempDir()
	require.NoError(t, EnsureDirs(tmp))
	return tmp
}

func TestAppendLog_WritesJSONLine(t *testing.T) {
	root := setupLedgerRoot(t)

	entry := LogEntry{
		ID:      "a1b2",
		Kind:    "task",
		Title:   "Fix container reconcile retry",
		Summary: "fixed the retry loop",
		Closed:  "2026-08-27T10:00:00Z",
		Commit:  "abc1234",
		Branch:  "feature/retry",
	}
	require.NoError(t, AppendLog(root, entry))

	data, err := os.ReadFile(filepath.Join(root, ".atlas", "log.jsonl"))
	require.NoError(t, err)
	require.Contains(t, string(data), `"id":"a1b2"`)
	require.Contains(t, string(data), `"kind":"task"`)
}

func TestAppendLog_AppendsMultipleEntries(t *testing.T) {
	root := setupLedgerRoot(t)

	require.NoError(t, AppendLog(root, LogEntry{ID: "a1b2", Kind: "task", Title: "one", Summary: "s1", Closed: "2026-08-27T10:00:00Z"}))
	require.NoError(t, AppendLog(root, LogEntry{ID: "c3d4", Kind: "card", Title: "two", SupersededBy: "e5f6", Closed: "2026-08-27T10:05:00Z"}))

	entries, err := ReadLog(root)
	require.NoError(t, err)
	require.Len(t, entries, 2)
	require.Equal(t, "a1b2", entries[0].ID)
	require.Equal(t, "c3d4", entries[1].ID)
	require.Equal(t, "e5f6", entries[1].SupersededBy)
}

func TestReadLog_MissingFileReturnsEmpty(t *testing.T) {
	root := setupLedgerRoot(t)

	entries, err := ReadLog(root)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestReadLog_IgnoresTrailingBlankLines(t *testing.T) {
	root := setupLedgerRoot(t)
	logPath := filepath.Join(root, ".atlas", "log.jsonl")
	content := `{"id":"a1b2","kind":"task","title":"one","closed":"2026-08-27T10:00:00Z"}
{"id":"c3d4","kind":"task","title":"two","closed":"2026-08-27T10:05:00Z"}

`
	require.NoError(t, os.WriteFile(logPath, []byte(content), 0o644))

	entries, err := ReadLog(root)
	require.NoError(t, err)
	require.Len(t, entries, 2)
}

func TestFilterLog_MatchesTitleOrSummary(t *testing.T) {
	entries := []LogEntry{
		{ID: "a1b2", Title: "Fix retry loop", Summary: "adjusted backoff"},
		{ID: "c3d4", Title: "Add feature X", Summary: "implemented X"},
	}

	got := FilterLog(entries, "retry")
	require.Len(t, got, 1)
	require.Equal(t, "a1b2", got[0].ID)

	got = FilterLog(entries, "implemented")
	require.Len(t, got, 1)
	require.Equal(t, "c3d4", got[0].ID)

	got = FilterLog(entries, "")
	require.Len(t, got, 2)
}

func TestClosedIDs_ReturnsSetFromLog(t *testing.T) {
	root := setupLedgerRoot(t)

	require.NoError(t, AppendLog(root, LogEntry{ID: "a1b2", Kind: "task", Title: "one", Closed: "2026-08-27T10:00:00Z"}))
	require.NoError(t, AppendLog(root, LogEntry{ID: "c3d4", Kind: "card", Title: "two", SupersededBy: "e5f6", Closed: "2026-08-27T10:05:00Z"}))

	closed, err := ClosedIDs(root)
	require.NoError(t, err)
	_, ok1 := closed["a1b2"]
	_, ok2 := closed["c3d4"]
	require.True(t, ok1)
	require.True(t, ok2)
	require.Len(t, closed, 2)
}
