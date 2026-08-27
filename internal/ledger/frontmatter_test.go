package ledger

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseFrontmatter_WellFormed(t *testing.T) {
	data := []byte("---\nid: a1b2\ntitle: Fix retry\n---\nBody line one.\nBody line two.\n")

	fm, body, err := ParseFrontmatter(data)
	require.NoError(t, err)
	require.Equal(t, "id: a1b2\ntitle: Fix retry\n", string(fm))
	require.Equal(t, "Body line one.\nBody line two.\n", string(body))
}

func TestParseFrontmatter_NoFrontmatterAtAll(t *testing.T) {
	data := []byte("Just a plain markdown body, no frontmatter.\n")

	fm, body, err := ParseFrontmatter(data)
	require.NoError(t, err)
	require.Empty(t, fm)
	require.Equal(t, string(data), string(body))
}

func TestParseFrontmatter_EmptyBody(t *testing.T) {
	data := []byte("---\nid: a1b2\n---\n")

	fm, body, err := ParseFrontmatter(data)
	require.NoError(t, err)
	require.Equal(t, "id: a1b2\n", string(fm))
	require.Empty(t, body)
}

func TestParseFrontmatter_MissingClosingDelimiterIsTolerant(t *testing.T) {
	data := []byte("---\nid: a1b2\ntitle: broken, no closing delimiter\n")

	// Must never panic, must return a typed error, and should still surface
	// whatever it could recover instead of discarding everything.
	require.NotPanics(t, func() {
		_, _, _ = ParseFrontmatter(data)
	})

	fm, _, err := ParseFrontmatter(data)
	require.Error(t, err)
	var target *ErrMalformedFrontmatter
	require.True(t, errors.As(err, &target))
	require.Contains(t, string(fm), "id: a1b2")
}

func TestParseFrontmatter_MalformedYAMLInsideDelimitersIsTolerant(t *testing.T) {
	// Delimiters are well-formed but the YAML inside is garbage; the codec
	// itself only splits frontmatter/body, so this should parse fine at this
	// layer (the caller's yaml.Unmarshal is what would fail, separately).
	data := []byte("---\nnot: [valid: yaml: here\n---\nbody\n")

	require.NotPanics(t, func() {
		_, _, _ = ParseFrontmatter(data)
	})
}

func TestSerializeFrontmatter_Roundtrip(t *testing.T) {
	fm := []byte("id: a1b2\ntitle: Fix retry\n")
	body := []byte("Body line one.\n")

	out := SerializeFrontmatter(fm, body)

	gotFM, gotBody, err := ParseFrontmatter(out)
	require.NoError(t, err)
	require.Equal(t, string(fm), string(gotFM))
	require.Equal(t, string(body), string(gotBody))
}

func TestSerializeFrontmatter_EmptyBody(t *testing.T) {
	fm := []byte("id: a1b2\n")
	out := SerializeFrontmatter(fm, nil)

	gotFM, gotBody, err := ParseFrontmatter(out)
	require.NoError(t, err)
	require.Equal(t, string(fm), string(gotFM))
	require.Empty(t, gotBody)
}
