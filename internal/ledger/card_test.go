package ledger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleCard() Card {
	return Card{
		ID:      "k9m2",
		Type:    "decision",
		Title:   "Use O_EXCL for claims",
		Status:  "active",
		Hook:    "Claim = file O_EXCL in $GIT_COMMON_DIR, never a mutex",
		Created:  "2026-08-27",
		Evidence: []string{},
		Body:     "Context/decision/consequences go here.\n",
	}
}

func TestSaveCard_ThenLoadCard_Roundtrip(t *testing.T) {
	root := setupLedgerRoot(t)
	c := sampleCard()

	require.NoError(t, SaveCard(root, c))

	got, err := LoadCard(root, "k9m2")
	require.NoError(t, err)
	require.Equal(t, c, got)
}

func TestSaveCard_UsesIDAndSlugForFilename(t *testing.T) {
	root := setupLedgerRoot(t)
	c := sampleCard()
	require.NoError(t, SaveCard(root, c))

	require.FileExists(t, filepath.Join(root, ".atlas", "cards", "k9m2-use-o-excl-for-claims.md"))
}

func TestLoadCard_NotFoundReturnsTypedError(t *testing.T) {
	root := setupLedgerRoot(t)

	_, err := LoadCard(root, "zzzz")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestListCards_ReturnsAll(t *testing.T) {
	root := setupLedgerRoot(t)
	c1 := sampleCard()
	c2 := sampleCard()
	c2.ID = "x1y2"
	c2.Title = "Second card"
	c2.Type = "knowledge"
	require.NoError(t, SaveCard(root, c1))
	require.NoError(t, SaveCard(root, c2))

	cards, err := ListCards(root)
	require.NoError(t, err)
	require.Len(t, cards, 2)
}

func TestListActiveCards_ExcludesSuperseded(t *testing.T) {
	root := setupLedgerRoot(t)
	active := sampleCard()
	superseded := sampleCard()
	superseded.ID = "x1y2"
	superseded.Title = "Old decision"
	superseded.Status = "superseded"
	superseded.SupersededBy = "k9m2"

	require.NoError(t, SaveCard(root, active))
	require.NoError(t, SaveCard(root, superseded))

	cards, err := ListActiveCards(root)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.Equal(t, "k9m2", cards[0].ID)
}

func TestListSupersededCards_ReturnsOnlySuperseded(t *testing.T) {
	root := setupLedgerRoot(t)
	active := sampleCard()
	superseded := sampleCard()
	superseded.ID = "x1y2"
	superseded.Title = "Old decision"
	superseded.Status = "superseded"
	superseded.SupersededBy = "k9m2"

	require.NoError(t, SaveCard(root, active))
	require.NoError(t, SaveCard(root, superseded))

	cards, err := ListSupersededCards(root)
	require.NoError(t, err)
	require.Len(t, cards, 1)
	require.Equal(t, "x1y2", cards[0].ID)
}

func TestLoadCard_MalformedFrontmatterIsTolerant(t *testing.T) {
	root := setupLedgerRoot(t)
	badPath := filepath.Join(root, ".atlas", "cards", "k9m2-broken.md")
	require.NoError(t, os.WriteFile(badPath, []byte("---\nid: k9m2\ntitle: broken\n"), 0o644))

	require.NotPanics(t, func() {
		_, _ = LoadCard(root, "k9m2")
	})

	_, err := LoadCard(root, "k9m2")
	require.Error(t, err)
	var target *ErrMalformedFrontmatter
	require.ErrorAs(t, err, &target)
}

func TestIsValidCardType(t *testing.T) {
	require.True(t, IsValidCardType("decision"))
	require.True(t, IsValidCardType("knowledge"))
	require.False(t, IsValidCardType("bogus"))
}

func TestIsValidCardStatus(t *testing.T) {
	require.True(t, IsValidCardStatus("active"))
	require.True(t, IsValidCardStatus("superseded"))
	require.False(t, IsValidCardStatus("bogus"))
}
