package ledger

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Spec is the frontmatter+body model for a living canonical spec file
// under .atlas/specs/<id>-<slug>.md (PLAN.md S9.1). Unlike a Workitem's
// body, a Spec's body IS the specification: one document per
// capability/area, updated in place; the history of changes lives in git,
// never as accumulated spec-per-feature files.
type Spec struct {
	ID           string   `yaml:"id"`
	Title        string   `yaml:"title"`
	Status       string   `yaml:"status"`
	SupersededBy string   `yaml:"superseded_by"`
	Created      string   `yaml:"created"`
	Evidence     []string `yaml:"evidence"`
	// Decisions is the S9.8 traceability list: each entry is either the id
	// of an ATLAS decision card, or a repo-relative path to an existing
	// ADR file. A spec may be created as a draft with none, but `spec
	// activate` requires at least one.
	Decisions []string `yaml:"decisions"`
	Body      string   `yaml:"-"`
}

var validSpecStatuses = map[string]bool{
	"draft":      true,
	"active":     true,
	"superseded": true,
}

// IsValidSpecStatus reports whether s is "draft", "active" or "superseded".
func IsValidSpecStatus(s string) bool {
	return validSpecStatuses[s]
}

func specsDir(root string) string {
	return filepath.Join(root, ".atlas", "specs")
}

func specFilename(s Spec) string {
	return fmt.Sprintf("%s-%s.md", s.ID, Slugify(s.Title))
}

// SaveSpec serializes s to .atlas/specs/<id>-<slug>.md. If a file for
// s.ID already exists under a different name, the old file is removed.
func SaveSpec(root string, s Spec) error {
	dir := specsDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	newPath := filepath.Join(dir, specFilename(s))

	if existing, err := findFileByID(dir, s.ID); err == nil && existing != newPath {
		if err := os.Remove(existing); err != nil {
			return err
		}
	}

	fmBytes, err := yaml.Marshal(s)
	if err != nil {
		return err
	}

	data := SerializeFrontmatter(fmBytes, []byte(s.Body))
	return os.WriteFile(newPath, data, 0o644)
}

// LoadSpec reads and parses .atlas/specs/<id>-*.md for the given id.
// Returns ErrNotFound if no such file exists, or *ErrMalformedFrontmatter
// (never a panic) if the file's frontmatter cannot be parsed.
func LoadSpec(root string, id string) (Spec, error) {
	path, err := findFileByID(specsDir(root), id)
	if err != nil {
		return Spec{}, err
	}
	return loadSpecFile(path)
}

func loadSpecFile(path string) (Spec, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Spec{}, err
	}

	fm, body, err := ParseFrontmatter(data)
	if err != nil {
		return Spec{}, err
	}

	var s Spec
	if err := yaml.Unmarshal(fm, &s); err != nil {
		return Spec{}, err
	}
	s.Body = string(body)

	return s, nil
}

// ListSpecs returns every spec under .atlas/specs/, of any status.
func ListSpecs(root string) ([]Spec, error) {
	dir := specsDir(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var specs []Spec
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		s, err := loadSpecFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return specs, err
		}
		specs = append(specs, s)
	}

	return specs, nil
}

// ListSpecsByStatus returns only specs with the given status.
func ListSpecsByStatus(root string, status string) ([]Spec, error) {
	all, err := ListSpecs(root)
	if err != nil {
		return nil, err
	}

	var out []Spec
	for _, s := range all {
		if s.Status == status {
			out = append(out, s)
		}
	}
	return out, nil
}
