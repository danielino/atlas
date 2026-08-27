package ledger

import "strings"

// maxSlugLength is the maximum length of a slug derived from a title.
const maxSlugLength = 40

// Slugify converts a title into a lowercase slug matching [a-z0-9-]*, with
// runs of non-matching characters collapsed to a single dash, no leading or
// trailing dashes, and truncated to at most 40 characters (without leaving a
// trailing dash after truncation).
func Slugify(title string) string {
	lower := strings.ToLower(title)

	var b strings.Builder
	lastWasDash := true // treat start as if preceded by a dash, to avoid leading dash
	for _, r := range lower {
		isAllowed := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAllowed {
			b.WriteRune(r)
			lastWasDash = false
			continue
		}
		if !lastWasDash {
			b.WriteByte('-')
			lastWasDash = true
		}
	}

	slug := strings.TrimRight(b.String(), "-")

	if len(slug) > maxSlugLength {
		slug = slug[:maxSlugLength]
		slug = strings.TrimRight(slug, "-")
	}

	return slug
}
