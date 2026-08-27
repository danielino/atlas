package ledger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// fixedReader yields bytes from a fixed sequence of hex strings, decoded to
// raw bytes, one "read" (call) per generation attempt. It lets tests control
// exactly which id GenerateID will produce on each attempt.
type fixedReader struct {
	seqs [][]byte
	i    int
}

func newFixedReader(hexIDs ...string) *fixedReader {
	seqs := make([][]byte, len(hexIDs))
	for i, h := range hexIDs {
		seqs[i] = hexToBytes(h)
	}
	return &fixedReader{seqs: seqs}
}

func hexToBytes(h string) []byte {
	b := make([]byte, len(h)/2)
	for i := 0; i < len(b); i++ {
		hi := hexVal(h[i*2])
		lo := hexVal(h[i*2+1])
		b[i] = hi<<4 | lo
	}
	return b
}

func hexVal(c byte) byte {
	switch {
	case c >= '0' && c <= '9':
		return c - '0'
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10
	}
	return 0
}

func (r *fixedReader) Read(p []byte) (int, error) {
	seq := r.seqs[r.i]
	if r.i < len(r.seqs)-1 {
		r.i++
	}
	copy(p, seq)
	return len(seq), nil
}

func TestGenerateID_ReturnsFourHexChars(t *testing.T) {
	root := setupLedgerRoot(t)
	reader := newFixedReader("a1b2")

	id, err := GenerateID(root, reader)
	require.NoError(t, err)
	require.Equal(t, "a1b2", id)
}

func TestGenerateID_RegeneratesOnCollisionWithWorkFile(t *testing.T) {
	root := setupLedgerRoot(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atlas", "work", "a1b2-existing-task.md"), []byte("x"), 0o644))

	reader := newFixedReader("a1b2", "c3d4")

	id, err := GenerateID(root, reader)
	require.NoError(t, err)
	require.Equal(t, "c3d4", id)
}

func TestGenerateID_RegeneratesOnCollisionWithCardFile(t *testing.T) {
	root := setupLedgerRoot(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atlas", "cards", "a1b2-existing-card.md"), []byte("x"), 0o644))

	reader := newFixedReader("a1b2", "c3d4")

	id, err := GenerateID(root, reader)
	require.NoError(t, err)
	require.Equal(t, "c3d4", id)
}

func TestGenerateID_RegeneratesOnCollisionWithLogEntry(t *testing.T) {
	root := setupLedgerRoot(t)
	require.NoError(t, AppendLog(root, LogEntry{ID: "a1b2", Kind: "task", Title: "done thing", Closed: "2026-08-27T10:00:00Z"}))

	reader := newFixedReader("a1b2", "c3d4")

	id, err := GenerateID(root, reader)
	require.NoError(t, err)
	require.Equal(t, "c3d4", id)
}

func TestGenerateID_FallsBackToFiveCharsAfter20Collisions(t *testing.T) {
	root := setupLedgerRoot(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atlas", "work", "a1b2-existing-task.md"), []byte("x"), 0o644))

	// 20 collisions with "a1b2" (4 hex chars = 2 bytes), then a 5-char
	// fallback attempt (5 hex chars -> 3 bytes, last nibble ignored).
	hexes := make([]string, 20)
	for i := range hexes {
		hexes[i] = "a1b2"
	}
	hexes = append(hexes, "c3d4e0")

	reader := newFixedReader(hexes...)

	id, err := GenerateID(root, reader)
	require.NoError(t, err)
	require.Len(t, id, 5)
	require.Equal(t, "c3d4e", id)
}

func TestExistingIDs_UnionOfWorkCardsAndLog(t *testing.T) {
	root := setupLedgerRoot(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atlas", "work", "a1b2-task-one.md"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atlas", "cards", "c3d4-card-one.md"), []byte("x"), 0o644))
	require.NoError(t, AppendLog(root, LogEntry{ID: "e5f6", Kind: "task", Title: "closed", Closed: "2026-08-27T10:00:00Z"}))

	ids, err := ExistingIDs(root)
	require.NoError(t, err)
	require.Contains(t, ids, "a1b2")
	require.Contains(t, ids, "c3d4")
	require.Contains(t, ids, "e5f6")
	require.Len(t, ids, 3)
}

func TestExistingIDs_IncludesSpecFiles(t *testing.T) {
	root := setupLedgerRoot(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".atlas", "specs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atlas", "specs", "3fa9-a-spec.md"), []byte("x"), 0o644))

	ids, err := ExistingIDs(root)
	require.NoError(t, err)
	require.Contains(t, ids, "3fa9")
	require.Len(t, ids, 1)
}

func TestGenerateID_RegeneratesOnCollisionAcrossWorkCardsAndSpecs(t *testing.T) {
	root := setupLedgerRoot(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".atlas", "specs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atlas", "work", "a1b2-task.md"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atlas", "cards", "c3d4-card.md"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atlas", "specs", "e5f6-spec.md"), []byte("x"), 0o644))

	reader := newFixedReader("a1b2", "c3d4", "e5f6", "1234")

	id, err := GenerateID(root, reader)
	require.NoError(t, err)
	require.Equal(t, "1234", id)
}

func TestExistingIDs_ExtractsIDFromFirstHyphenOnly(t *testing.T) {
	root := setupLedgerRoot(t)
	// Slug itself contains a hyphen; id must be everything before the FIRST
	// hyphen, not split further.
	require.NoError(t, os.WriteFile(filepath.Join(root, ".atlas", "work", "a1b2-fix-container-reconcile-retry.md"), []byte("x"), 0o644))

	ids, err := ExistingIDs(root)
	require.NoError(t, err)
	require.Contains(t, ids, "a1b2")
	require.Len(t, ids, 1)
}
