package ledger

import (
	"os"
	"path/filepath"
)

func focusPath(root string) string {
	return filepath.Join(root, ".atlas", "focus.md")
}

// ReadFocus reads the plain-markdown content of .atlas/focus.md. A missing
// file returns an empty string, not an error.
func ReadFocus(root string) (string, error) {
	data, err := os.ReadFile(focusPath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return string(data), nil
}

// WriteFocus overwrites .atlas/focus.md with content, verbatim (no
// frontmatter).
func WriteFocus(root string, content string) error {
	return os.WriteFile(focusPath(root), []byte(content), 0o644)
}
