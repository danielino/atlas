package ledger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestWriteFocus_ThenReadFocus_Roundtrip(t *testing.T) {
	root := setupLedgerRoot(t)

	content := "Today: ship the retry fix.\nNext: write tests for claims.\n"
	require.NoError(t, WriteFocus(root, content))

	got, err := ReadFocus(root)
	require.NoError(t, err)
	require.Equal(t, content, got)
}

func TestReadFocus_MissingFileReturnsEmptyString(t *testing.T) {
	root := setupLedgerRoot(t)

	got, err := ReadFocus(root)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestWriteFocus_OverwritesExistingContent(t *testing.T) {
	root := setupLedgerRoot(t)
	require.NoError(t, WriteFocus(root, "first\n"))
	require.NoError(t, WriteFocus(root, "second\n"))

	got, err := ReadFocus(root)
	require.NoError(t, err)
	require.Equal(t, "second\n", got)
}

func TestFocusPath_IsPlainMarkdownFile(t *testing.T) {
	root := setupLedgerRoot(t)
	require.NoError(t, WriteFocus(root, "x\n"))

	data, err := os.ReadFile(filepath.Join(root, ".atlas", "focus.md"))
	require.NoError(t, err)
	require.Equal(t, "x\n", string(data))
}
