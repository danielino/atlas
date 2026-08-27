package ledger

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFindRoot_FindsAtlasDirAtCwd(t *testing.T) {
	tmp := t.TempDir()
	atlasDir := filepath.Join(tmp, ".atlas")
	require.NoError(t, os.MkdirAll(atlasDir, 0o755))

	root, err := FindRoot(tmp)
	require.NoError(t, err)
	require.Equal(t, tmp, root)
}

func TestFindRoot_WalksUpFromNestedDir(t *testing.T) {
	tmp := t.TempDir()
	atlasDir := filepath.Join(tmp, ".atlas")
	require.NoError(t, os.MkdirAll(atlasDir, 0o755))

	nested := filepath.Join(tmp, "a", "b", "c")
	require.NoError(t, os.MkdirAll(nested, 0o755))

	root, err := FindRoot(nested)
	require.NoError(t, err)
	require.Equal(t, tmp, root)
}

func TestFindRoot_ReturnsTypedErrorWhenNotFound(t *testing.T) {
	tmp := t.TempDir()

	_, err := FindRoot(tmp)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoLedger))
}

func TestEnsureDirs_CreatesWorkAndCardsDirs(t *testing.T) {
	tmp := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(tmp, ".atlas"), 0o755))

	require.NoError(t, EnsureDirs(tmp))

	require.DirExists(t, filepath.Join(tmp, ".atlas", "work"))
	require.DirExists(t, filepath.Join(tmp, ".atlas", "cards"))
}
