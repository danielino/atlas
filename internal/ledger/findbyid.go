package ledger

import (
	"os"
	"path/filepath"
	"strings"
)

// findFileByID looks in dir for a file named "<id>-*.md" (or exactly
// "<id>.md"). It returns the full path, or ("", ErrNotFound) if no such
// file exists.
func findFileByID(dir string, id string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		if idFromFilename(name) == id {
			return filepath.Join(dir, name), nil
		}
	}

	return "", ErrNotFound
}
