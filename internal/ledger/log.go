package ledger

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// LogEntry represents one closed-item record appended to .atlas/log.jsonl.
type LogEntry struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"` // "task" | "card"
	Title        string `json:"title"`
	Summary      string `json:"summary,omitempty"`
	Closed       string `json:"closed"`
	Commit       string `json:"commit,omitempty"`
	Branch       string `json:"branch,omitempty"`
	SupersededBy string `json:"superseded_by,omitempty"`
}

func logPath(root string) string {
	return filepath.Join(root, ".atlas", "log.jsonl")
}

// AppendLog appends entry as one JSON line to .atlas/log.jsonl, creating the
// file if it does not yet exist.
func AppendLog(root string, entry LogEntry) error {
	f, err := os.OpenFile(logPath(root), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	_, err = f.Write(data)
	return err
}

// ReadLog reads all entries from .atlas/log.jsonl. A missing file is treated
// as an empty log, not an error. Blank lines are ignored.
func ReadLog(root string) ([]LogEntry, error) {
	f, err := os.Open(logPath(root))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []LogEntry
	scanner := bufio.NewScanner(f)
	// log lines can be long-ish (evidence lists etc.); grow buffer as needed.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry LogEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			return entries, err
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return entries, err
	}

	return entries, nil
}

// FilterLog returns the subset of entries whose title or summary contains
// pattern (case-insensitive substring match). An empty pattern matches all
// entries.
func FilterLog(entries []LogEntry, pattern string) []LogEntry {
	if pattern == "" {
		return entries
	}
	needle := strings.ToLower(pattern)

	var out []LogEntry
	for _, e := range entries {
		if strings.Contains(strings.ToLower(e.Title), needle) ||
			strings.Contains(strings.ToLower(e.Summary), needle) {
			out = append(out, e)
		}
	}
	return out
}

// ClosedIDs returns the set of ids present in .atlas/log.jsonl (closed
// workitems and superseded cards). Used both for ID-collision checks and for
// ready-detection (a task's blocked_by referencing a closed id is satisfied).
func ClosedIDs(root string) (map[string]struct{}, error) {
	entries, err := ReadLog(root)
	if err != nil {
		return nil, err
	}

	ids := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		ids[e.ID] = struct{}{}
	}
	return ids, nil
}
