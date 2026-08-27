package ledger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func sampleSpec() Spec {
	return Spec{
		ID:        "3fa9",
		Title:     "Workload execution retry semantics",
		Status:    "draft",
		Created:   "2026-08-27",
		Evidence:  []string{},
		Decisions: []string{},
		Body:      "Body = the specification (markdown, living document).\n",
	}
}

func TestSaveSpec_ThenLoadSpec_Roundtrip(t *testing.T) {
	root := setupLedgerRoot(t)
	s := sampleSpec()

	require.NoError(t, SaveSpec(root, s))

	got, err := LoadSpec(root, "3fa9")
	require.NoError(t, err)
	require.Equal(t, s, got)
}

func TestSaveSpec_UsesIDAndSlugForFilename(t *testing.T) {
	root := setupLedgerRoot(t)
	s := sampleSpec()
	require.NoError(t, SaveSpec(root, s))

	require.FileExists(t, filepath.Join(root, ".atlas", "specs", "3fa9-workload-execution-retry-semantics.md"))
}

func TestSaveSpec_RenamesFileWhenTitleChanges(t *testing.T) {
	root := setupLedgerRoot(t)
	s := sampleSpec()
	require.NoError(t, SaveSpec(root, s))

	s.Title = "Renamed spec title"
	require.NoError(t, SaveSpec(root, s))

	require.NoFileExists(t, filepath.Join(root, ".atlas", "specs", "3fa9-workload-execution-retry-semantics.md"))
	require.FileExists(t, filepath.Join(root, ".atlas", "specs", "3fa9-renamed-spec-title.md"))
}

func TestLoadSpec_NotFoundReturnsTypedError(t *testing.T) {
	root := setupLedgerRoot(t)

	_, err := LoadSpec(root, "zzzz")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestListSpecs_ReturnsAll(t *testing.T) {
	root := setupLedgerRoot(t)
	s1 := sampleSpec()
	s2 := sampleSpec()
	s2.ID = "x1y2"
	s2.Title = "Second spec"

	require.NoError(t, SaveSpec(root, s1))
	require.NoError(t, SaveSpec(root, s2))

	specs, err := ListSpecs(root)
	require.NoError(t, err)
	require.Len(t, specs, 2)
}

func TestListSpecs_EmptyWhenDirMissing(t *testing.T) {
	root := setupLedgerRoot(t)

	specs, err := ListSpecs(root)
	require.NoError(t, err)
	require.Empty(t, specs)
}

func TestListSpecsByStatus_FiltersStatus(t *testing.T) {
	root := setupLedgerRoot(t)
	draft := sampleSpec()
	active := sampleSpec()
	active.ID = "x1y2"
	active.Title = "Active spec"
	active.Status = "active"
	superseded := sampleSpec()
	superseded.ID = "p1q2"
	superseded.Title = "Superseded spec"
	superseded.Status = "superseded"
	superseded.SupersededBy = "x1y2"

	require.NoError(t, SaveSpec(root, draft))
	require.NoError(t, SaveSpec(root, active))
	require.NoError(t, SaveSpec(root, superseded))

	drafts, err := ListSpecsByStatus(root, "draft")
	require.NoError(t, err)
	require.Len(t, drafts, 1)
	require.Equal(t, "3fa9", drafts[0].ID)

	actives, err := ListSpecsByStatus(root, "active")
	require.NoError(t, err)
	require.Len(t, actives, 1)
	require.Equal(t, "x1y2", actives[0].ID)

	supersededSpecs, err := ListSpecsByStatus(root, "superseded")
	require.NoError(t, err)
	require.Len(t, supersededSpecs, 1)
	require.Equal(t, "p1q2", supersededSpecs[0].ID)
}

func TestLoadSpec_MalformedFrontmatterIsTolerant(t *testing.T) {
	root := setupLedgerRoot(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".atlas", "specs"), 0o755))
	badPath := filepath.Join(root, ".atlas", "specs", "3fa9-broken.md")
	require.NoError(t, os.WriteFile(badPath, []byte("---\nid: 3fa9\ntitle: broken\n"), 0o644))

	require.NotPanics(t, func() {
		_, _ = LoadSpec(root, "3fa9")
	})

	_, err := LoadSpec(root, "3fa9")
	require.Error(t, err)
	var target *ErrMalformedFrontmatter
	require.ErrorAs(t, err, &target)
}

func TestSaveSpec_ThenLoadSpec_RoundtripWithDecisions(t *testing.T) {
	root := setupLedgerRoot(t)
	s := sampleSpec()
	s.Decisions = []string{"k9m2", "docs/adr/0034-enrichment-stage.md"}

	require.NoError(t, SaveSpec(root, s))

	got, err := LoadSpec(root, "3fa9")
	require.NoError(t, err)
	require.Equal(t, s, got)
}

func TestIsValidSpecStatus(t *testing.T) {
	require.True(t, IsValidSpecStatus("draft"))
	require.True(t, IsValidSpecStatus("active"))
	require.True(t, IsValidSpecStatus("superseded"))
	require.False(t, IsValidSpecStatus("bogus"))
}
