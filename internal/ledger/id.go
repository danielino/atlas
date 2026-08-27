package ledger

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const (
	idLength          = 4
	idLengthFallback  = 5
	maxCollisionTries = 20
)

// GenerateID creates a new random lowercase-hex id, checked for collisions
// against every id already in use under root (work/, cards/, log.jsonl). It
// retries up to maxCollisionTries times at idLength (4) hex chars; if all of
// those collide, it falls back to idLengthFallback (5) hex chars.
//
// entropy is the source of randomness (crypto/rand.Reader in production,
// injectable in tests for deterministic collision scenarios).
func GenerateID(root string, entropy io.Reader) (string, error) {
	existing, err := ExistingIDs(root)
	if err != nil {
		return "", err
	}

	for i := 0; i < maxCollisionTries; i++ {
		id, err := randomHexID(entropy, idLength)
		if err != nil {
			return "", err
		}
		if _, taken := existing[id]; !taken {
			return id, nil
		}
	}

	// Fallback: 5 hex chars.
	for {
		id, err := randomHexID(entropy, idLengthFallback)
		if err != nil {
			return "", err
		}
		if _, taken := existing[id]; !taken {
			return id, nil
		}
	}
}

// randomHexID reads enough random bytes to produce n hex characters and
// returns the first n characters of the hex encoding.
func randomHexID(entropy io.Reader, n int) (string, error) {
	byteLen := (n + 1) / 2
	buf := make([]byte, byteLen)
	if _, err := io.ReadFull(entropy, buf); err != nil {
		return "", fmt.Errorf("ledger: reading entropy: %w", err)
	}
	return hex.EncodeToString(buf)[:n], nil
}

// RandReader is the default entropy source for production use.
var RandReader io.Reader = rand.Reader

// ExistingIDs returns the set of ids already in use: workitem files in
// work/, card files in cards/, and entries in log.jsonl.
func ExistingIDs(root string) (map[string]struct{}, error) {
	ids := make(map[string]struct{})

	for _, dir := range []string{"work", "cards"} {
		entries, err := os.ReadDir(filepath.Join(root, ".atlas", dir))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if id := idFromFilename(e.Name()); id != "" {
				ids[id] = struct{}{}
			}
		}
	}

	closed, err := ClosedIDs(root)
	if err != nil {
		return nil, err
	}
	for id := range closed {
		ids[id] = struct{}{}
	}

	return ids, nil
}

// idFromFilename extracts the id portion of a "<id>-<slug>.md" filename: the
// substring up to (not including) the first hyphen.
func idFromFilename(name string) string {
	base := strings.TrimSuffix(name, filepath.Ext(name))
	i := strings.Index(base, "-")
	if i < 0 {
		return base
	}
	return base[:i]
}
