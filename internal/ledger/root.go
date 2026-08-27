package ledger

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNoLedger is returned when no .atlas directory can be found by walking
// up from the starting directory to the filesystem root.
var ErrNoLedger = errors.New("ledger: no .atlas directory found")

// FindRoot walks up from start looking for a directory containing an
// ".atlas" subdirectory. It returns the directory that contains ".atlas"
// (not the .atlas directory itself). If none is found by the time it
// reaches the filesystem root, it returns ErrNoLedger.
func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}

	for {
		info, statErr := os.Stat(filepath.Join(dir, ".atlas"))
		if statErr == nil && info.IsDir() {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoLedger
		}
		dir = parent
	}
}

// EnsureDirs creates the standard .atlas subdirectories (work/, cards/) under
// root if they do not already exist.
func EnsureDirs(root string) error {
	atlasDir := filepath.Join(root, ".atlas")
	if err := os.MkdirAll(filepath.Join(atlasDir, "work"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(atlasDir, "cards"), 0o755); err != nil {
		return err
	}
	return nil
}
