package ledger

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Card is the frontmatter+body model for a decision/knowledge card file
// under .atlas/cards/<id>-<slug>.md.
type Card struct {
	ID           string   `yaml:"id"`
	Type         string   `yaml:"type"`
	Title        string   `yaml:"title"`
	Status       string   `yaml:"status"`
	SupersededBy string   `yaml:"superseded_by"`
	Hook         string   `yaml:"hook"`
	Created      string   `yaml:"created"`
	Evidence     []string `yaml:"evidence"`
	Body         string   `yaml:"-"`
}

var validCardTypes = map[string]bool{
	"decision":  true,
	"knowledge": true,
}

var validCardStatuses = map[string]bool{
	"active":     true,
	"superseded": true,
}

// IsValidCardType reports whether t is "decision" or "knowledge".
func IsValidCardType(t string) bool {
	return validCardTypes[t]
}

// IsValidCardStatus reports whether s is "active" or "superseded".
func IsValidCardStatus(s string) bool {
	return validCardStatuses[s]
}

func cardsDir(root string) string {
	return filepath.Join(root, ".atlas", "cards")
}

func cardFilename(c Card) string {
	return fmt.Sprintf("%s-%s.md", c.ID, Slugify(c.Title))
}

// SaveCard serializes c to .atlas/cards/<id>-<slug>.md. If a file for c.ID
// already exists under a different name, the old file is removed.
func SaveCard(root string, c Card) error {
	dir := cardsDir(root)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	newPath := filepath.Join(dir, cardFilename(c))

	if existing, err := findFileByID(dir, c.ID); err == nil && existing != newPath {
		if err := os.Remove(existing); err != nil {
			return err
		}
	}

	fmBytes, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	data := SerializeFrontmatter(fmBytes, []byte(c.Body))
	return os.WriteFile(newPath, data, 0o644)
}

// LoadCard reads and parses .atlas/cards/<id>-*.md for the given id.
// Returns ErrNotFound if no such file exists, or *ErrMalformedFrontmatter
// (never a panic) if the file's frontmatter cannot be parsed.
func LoadCard(root string, id string) (Card, error) {
	path, err := findFileByID(cardsDir(root), id)
	if err != nil {
		return Card{}, err
	}
	return loadCardFile(path)
}

func loadCardFile(path string) (Card, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Card{}, err
	}

	fm, body, err := ParseFrontmatter(data)
	if err != nil {
		return Card{}, err
	}

	var c Card
	if err := yaml.Unmarshal(fm, &c); err != nil {
		return Card{}, err
	}
	c.Body = string(body)

	return c, nil
}

// ListCards returns every card under .atlas/cards/, active and superseded
// alike.
func ListCards(root string) ([]Card, error) {
	dir := cardsDir(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var cards []Card
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		c, err := loadCardFile(filepath.Join(dir, e.Name()))
		if err != nil {
			return cards, err
		}
		cards = append(cards, c)
	}

	return cards, nil
}

// ListActiveCards returns only cards with status "active".
func ListActiveCards(root string) ([]Card, error) {
	all, err := ListCards(root)
	if err != nil {
		return nil, err
	}

	var active []Card
	for _, c := range all {
		if c.Status == "active" {
			active = append(active, c)
		}
	}
	return active, nil
}

// ListSupersededCards returns only cards with status "superseded".
func ListSupersededCards(root string) ([]Card, error) {
	all, err := ListCards(root)
	if err != nil {
		return nil, err
	}

	var superseded []Card
	for _, c := range all {
		if c.Status == "superseded" {
			superseded = append(superseded, c)
		}
	}
	return superseded, nil
}
