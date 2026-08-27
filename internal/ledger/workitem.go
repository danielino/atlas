package ledger

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Workitem is the frontmatter+body model for a task file under
// .atlas/work/<id>-<slug>.md.
type Workitem struct {
	ID             string   `yaml:"id"`
	Title          string   `yaml:"title"`
	Status         string   `yaml:"status"`
	Created        string   `yaml:"created"`
	BlockedBy      []string `yaml:"blocked_by,omitempty"`
	DiscoveredFrom string   `yaml:"discovered_from,omitempty"`
	Branch         string   `yaml:"branch,omitempty"`
	Evidence       []string `yaml:"evidence,omitempty"`
	Summary        string   `yaml:"summary"`
	Reason         string   `yaml:"reason,omitempty"`
	Spec           string   `yaml:"spec,omitempty"`
	Body           string   `yaml:"-"`
}

// validStatuses are the allowed values of Workitem.Status.
var validStatuses = map[string]bool{
	"todo":    true,
	"doing":   true,
	"blocked": true,
	"done":    true,
}

// IsValidStatus reports whether status is one of todo|doing|blocked|done.
func IsValidStatus(status string) bool {
	return validStatuses[status]
}

// transitions maps each valid status to the set of statuses it may move to.
// "done" is terminal at the type level: in practice a `done` workitem's file
// is removed from work/ (S2 `task done`), so there is nothing left to
// transition further.
var transitions = map[string]map[string]bool{
	"todo":    {"doing": true, "blocked": true, "done": true},
	"doing":   {"blocked": true, "done": true, "todo": true},
	"blocked": {"doing": true, "todo": true, "done": true},
	"done":    {},
}

// CanTransition reports whether moving a workitem from `from` to `to` is a
// valid status transition. Unknown statuses on either side are always
// invalid.
func CanTransition(from, to string) bool {
	if !IsValidStatus(from) || !IsValidStatus(to) {
		return false
	}
	return transitions[from][to]
}

func workDir(root string) string {
	return filepath.Join(root, ".atlas", "work")
}

func workitemFilename(w Workitem) string {
	return fmt.Sprintf("%s-%s.md", w.ID, Slugify(w.Title))
}

// SaveWorkitem serializes w to .atlas/work/<id>-<slug>.md. If a file for
// w.ID already exists under a different name (because the title/slug
// changed), the old file is removed.
func SaveWorkitem(root string, w Workitem) error {
	dir := workDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	newPath := filepath.Join(dir, workitemFilename(w))

	if existing, err := findFileByID(dir, w.ID); err == nil && existing != newPath {
		if err := os.Remove(existing); err != nil {
			return err
		}
	}

	fmBytes, err := yaml.Marshal(w)
	if err != nil {
		return err
	}

	data := SerializeFrontmatter(fmBytes, []byte(w.Body))
	return os.WriteFile(newPath, data, 0o644)
}

// LoadWorkitem reads and parses .atlas/work/<id>-*.md for the given id.
// Returns ErrNotFound if no such file exists, or *ErrMalformedFrontmatter
// (never a panic) if the file's frontmatter cannot be parsed.
func LoadWorkitem(root string, id string) (Workitem, error) {
	path, err := findFileByID(workDir(root), id)
	if err != nil {
		return Workitem{}, err
	}
	return loadWorkitemFile(path)
}

func loadWorkitemFile(path string) (Workitem, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Workitem{}, err
	}

	fm, body, err := ParseFrontmatter(data)
	if err != nil {
		return Workitem{}, err
	}

	var w Workitem
	if err := yaml.Unmarshal(fm, &w); err != nil {
		return Workitem{}, err
	}
	w.Body = string(body)

	return w, nil
}

// ListWorkitems returns every workitem under .atlas/work/, in directory
// listing order. An empty/missing work/ directory yields an empty slice.
func ListWorkitems(root string) ([]Workitem, error) {
	dir := workDir(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var items []Workitem
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		w, err := loadWorkitemFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return items, err
		}
		items = append(items, w)
	}

	return items, nil
}
