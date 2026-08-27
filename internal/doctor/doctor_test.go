package doctor

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/dmarcocci/atlas/internal/claims"
	"github.com/dmarcocci/atlas/internal/gitx"
	"github.com/dmarcocci/atlas/internal/ledger"
	"github.com/dmarcocci/atlas/internal/testutil"
)

func setup(t *testing.T) string {
	t.Helper()
	repo := testutil.SetupRepo(t)
	require.NoError(t, ledger.EnsureDirs(repo))
	return repo
}

func hasIssue(issues []Issue, code string) bool {
	for _, i := range issues {
		if i.Code == code {
			return true
		}
	}
	return false
}

func TestRun_CleanRepo_NoErrorsNoWarnings(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.WriteFocus(repo, "today's focus"))
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{ID: "a1b2", Title: "t", Status: "todo", Created: "2026-08-27"}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.Empty(t, report.Errors)
	require.Empty(t, report.Warnings)
	require.False(t, report.HasErrors())
}

func TestRun_OrphanBlockedBy_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{
		ID: "a1b2", Title: "t", Status: "todo", Created: "2026-08-27",
		BlockedBy: []string{"ffff"},
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "orphan_ref"))
	require.True(t, report.HasErrors())
}

func TestRun_OrphanDiscoveredFrom_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{
		ID: "a1b2", Title: "t", Status: "todo", Created: "2026-08-27",
		DiscoveredFrom: "ffff",
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "orphan_ref"))
}

func TestRun_OrphanSupersededBy_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveCard(repo, ledger.Card{
		ID: "k9m2", Type: "decision", Title: "t", Status: "superseded",
		SupersededBy: "ffff", Hook: "h", Created: "2026-08-27",
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "orphan_ref"))
}

func TestRun_BlockedByReferencingClosedID_NotOrphan(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{
		ID: "a1b2", Title: "t", Status: "todo", Created: "2026-08-27",
		BlockedBy: []string{"c3d4"},
	}))
	require.NoError(t, ledger.AppendLog(repo, ledger.LogEntry{ID: "c3d4", Kind: "task", Summary: "done", Closed: time.Now().UTC().Format(time.RFC3339)}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.False(t, hasIssue(report.Errors, "orphan_ref"))
}

func TestRun_CycleInBlockedBy_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{ID: "a1b2", Title: "a", Status: "todo", Created: "2026-08-27", BlockedBy: []string{"c3d4"}}))
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{ID: "c3d4", Title: "c", Status: "todo", Created: "2026-08-27", BlockedBy: []string{"a1b2"}}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "cycle"))
}

func TestRun_NoCycle_ChainIsFine(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{ID: "a1b2", Title: "a", Status: "todo", Created: "2026-08-27", BlockedBy: []string{"c3d4"}}))
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{ID: "c3d4", Title: "c", Status: "todo", Created: "2026-08-27"}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.False(t, hasIssue(report.Errors, "cycle"))
}

func TestRun_DoneWorkitemStillInWork_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{ID: "a1b2", Title: "t", Status: "done", Created: "2026-08-27", Summary: "did it"}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "done_in_work"))
}

func TestRun_LogTaskMissingSummary_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.AppendLog(repo, ledger.LogEntry{ID: "a1b2", Kind: "task", Summary: "", Closed: time.Now().UTC().Format(time.RFC3339)}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "missing_summary"))
}

func TestRun_LogCardMissingSummary_NotAnError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.AppendLog(repo, ledger.LogEntry{ID: "k9m2", Kind: "card", Summary: "", Closed: time.Now().UTC().Format(time.RFC3339)}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.False(t, hasIssue(report.Errors, "missing_summary"))
}

func TestRun_DuplicateIDs_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".atlas", "work", "a1b2-one.md"),
		[]byte("---\nid: a1b2\ntitle: one\nstatus: todo\ncreated: 2026-08-27\n---\nbody\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".atlas", "cards", "a1b2-two.md"),
		[]byte("---\nid: a1b2\ntype: knowledge\ntitle: two\nstatus: active\nhook: h\ncreated: 2026-08-27\n---\nbody\n"), 0o644))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "duplicate_id"))
}

func TestRun_MalformedWorkitemFrontmatter_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".atlas", "work", "aaaa-bad.md"),
		[]byte("---\nid: aaaa\nno closing delimiter\n"), 0o644))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "malformed_frontmatter"))
	require.True(t, report.HasErrors())
}

func TestRun_MalformedCardFrontmatter_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".atlas", "cards", "bbbb-bad.md"),
		[]byte("---\nid: bbbb\nno closing delimiter\n"), 0o644))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "malformed_frontmatter"))
}

func TestRun_MalformedFile_DoesNotAbortOtherChecks(t *testing.T) {
	repo := setup(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".atlas", "work", "aaaa-bad.md"),
		[]byte("---\nid: aaaa\nno closing delimiter\n"), 0o644))
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{
		ID: "b2c3", Title: "t", Status: "todo", Created: "2026-08-27", BlockedBy: []string{"ffff"},
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "malformed_frontmatter"))
	require.True(t, hasIssue(report.Errors, "orphan_ref"))
}

func TestRun_CardOlderThan90Days_IsWarning(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveCard(repo, ledger.Card{
		ID: "k9m2", Type: "decision", Title: "t", Status: "active", Hook: "h", Created: "2026-01-01",
	}))
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	report, err := Run(repo, ledger.DefaultConfig(), Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Warnings, "old_card"))
	require.False(t, report.HasErrors())
}

func TestRun_CardYoungerThan90Days_NoWarning(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveCard(repo, ledger.Card{
		ID: "k9m2", Type: "decision", Title: "t", Status: "active", Hook: "h", Created: "2026-08-20",
	}))
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	report, err := Run(repo, ledger.DefaultConfig(), Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.False(t, hasIssue(report.Warnings, "old_card"))
}

func TestRun_SupersededOldCard_NoWarning(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveCard(repo, ledger.Card{
		ID: "k9m2", Type: "decision", Title: "t", Status: "superseded", Hook: "h", Created: "2026-01-01",
	}))
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	report, err := Run(repo, ledger.DefaultConfig(), Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.False(t, hasIssue(report.Warnings, "old_card"))
}

func TestRun_ExpiredClaim_IsRemovedAndFixed(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{ID: "a1b2", Title: "t", Status: "doing", Created: "2026-08-27"}))

	commonDir, err := gitx.CommonDir(repo)
	require.NoError(t, err)

	past := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := func() time.Time { return past.AddDate(0, 0, 2) }
	mgr := &claims.Manager{CommonDir: commonDir, TTLHours: 24, Now: func() time.Time { return past }}
	_, err = mgr.Acquire("a1b2", "feature/x")
	require.NoError(t, err)

	report, err := Run(repo, ledger.DefaultConfig(), Options{Now: now})
	require.NoError(t, err)
	require.NotEmpty(t, report.Fixed)

	liveMgr := &claims.Manager{CommonDir: commonDir, TTLHours: 24, Now: now}
	_, ok := liveMgr.Get("a1b2")
	require.False(t, ok, "expired claim file should have been removed from disk")
}

func TestRun_ClaimOnNonexistentWorkitem_IsRemovedAndFixed(t *testing.T) {
	repo := setup(t)

	commonDir, err := gitx.CommonDir(repo)
	require.NoError(t, err)
	mgr := &claims.Manager{CommonDir: commonDir, TTLHours: 24}
	_, err = mgr.Acquire("ffff", "feature/x")
	require.NoError(t, err)

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.NotEmpty(t, report.Fixed)

	_, ok := mgr.Get("ffff")
	require.False(t, ok, "dangling claim file should have been removed from disk")
}

func TestRun_ClaimOnExistingWorkitem_NotTouched(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{ID: "a1b2", Title: "t", Status: "doing", Created: "2026-08-27"}))

	commonDir, err := gitx.CommonDir(repo)
	require.NoError(t, err)
	mgr := &claims.Manager{CommonDir: commonDir, TTLHours: 24}
	_, err = mgr.Acquire("a1b2", "feature/x")
	require.NoError(t, err)

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.Empty(t, report.Fixed)

	_, ok := mgr.Get("a1b2")
	require.True(t, ok, "live claim on an existing workitem must not be removed")
}

func TestRun_NoGitRepo_ClaimsCheckSkipped(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ledger.EnsureDirs(dir))

	report, err := Run(dir, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.Empty(t, report.Errors)
}

// --- S9.4/S9.8 spec checks ---

func TestRun_WorkitemSpecNonexistent_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{
		ID: "a1b2", Title: "t", Status: "todo", Created: "2026-08-27", Spec: "ffff",
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "spec_not_found"))
}

func TestRun_WorkitemSpecSuperseded_IsWarning(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "3fa9", Title: "old spec", Status: "superseded", SupersededBy: "77aa", Created: "2026-08-27",
	}))
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "77aa", Title: "new spec", Status: "active", Created: "2026-08-27", Decisions: []string{"k9m2"},
	}))
	require.NoError(t, ledger.SaveCard(repo, ledger.Card{
		ID: "k9m2", Type: "decision", Title: "d", Status: "active", Hook: "h", Created: "2026-08-27",
	}))
	require.NoError(t, ledger.SaveWorkitem(repo, ledger.Workitem{
		ID: "a1b2", Title: "t", Status: "todo", Created: "2026-08-27", Spec: "3fa9",
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Warnings, "spec_superseded"))
	require.False(t, report.HasErrors())
}

func TestRun_SpecSupersededByOrphan_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "3fa9", Title: "old spec", Status: "superseded", SupersededBy: "ffff", Created: "2026-08-27",
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "orphan_ref"))
}

func TestRun_DraftSpecOlderThan30Days_IsWarning(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "3fa9", Title: "old draft", Status: "draft", Created: "2026-01-01",
	}))
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	report, err := Run(repo, ledger.DefaultConfig(), Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Warnings, "old_draft_spec"))
	require.False(t, report.HasErrors())
}

func TestRun_DraftSpecYoungerThan30Days_NoWarning(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "3fa9", Title: "fresh draft", Status: "draft", Created: "2026-08-20",
	}))
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	report, err := Run(repo, ledger.DefaultConfig(), Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.False(t, hasIssue(report.Warnings, "old_draft_spec"))
}

func TestRun_ActiveSpecOlderThan30Days_NoWarning(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "3fa9", Title: "old active", Status: "active", Created: "2026-01-01", Decisions: []string{"k9m2"},
	}))
	require.NoError(t, ledger.SaveCard(repo, ledger.Card{
		ID: "k9m2", Type: "decision", Title: "d", Status: "active", Hook: "h", Created: "2026-01-01",
	}))
	now := time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC)

	report, err := Run(repo, ledger.DefaultConfig(), Options{Now: func() time.Time { return now }})
	require.NoError(t, err)
	require.False(t, hasIssue(report.Warnings, "old_draft_spec"))
}

func TestRun_MalformedSpecFrontmatter_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".atlas", "specs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".atlas", "specs", "cccc-bad.md"),
		[]byte("---\nid: cccc\nno closing delimiter\n"), 0o644))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "malformed_frontmatter"))
}

func TestRun_DuplicateIDAcrossWorkCardsAndSpecs_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".atlas", "work", "a1b2-one.md"),
		[]byte("---\nid: a1b2\ntitle: one\nstatus: todo\ncreated: 2026-08-27\n---\nbody\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(repo, ".atlas", "specs"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, ".atlas", "specs", "a1b2-two.md"),
		[]byte("---\nid: a1b2\ntitle: two\nstatus: draft\ncreated: 2026-08-27\n---\nbody\n"), 0o644))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "duplicate_id"))
}

func TestRun_ActiveSpecWithEmptyDecisions_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "3fa9", Title: "active without decision", Status: "active", Created: "2026-08-27",
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "spec_without_decision"))
}

func TestRun_DraftSpecWithEmptyDecisions_NoError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "3fa9", Title: "draft without decision", Status: "draft", Created: "2026-08-27",
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.False(t, hasIssue(report.Errors, "spec_without_decision"))
}

func TestRun_SpecDecisionReferencesNonexistentCard_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "3fa9", Title: "spec", Status: "draft", Created: "2026-08-27", Decisions: []string{"ffff"},
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "decision_not_found"))
}

func TestRun_SpecDecisionReferencesSupersededCard_IsWarning(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveCard(repo, ledger.Card{
		ID: "k9m2", Type: "decision", Title: "old", Status: "superseded", SupersededBy: "x1y2", Hook: "h", Created: "2026-08-27",
	}))
	require.NoError(t, ledger.SaveCard(repo, ledger.Card{
		ID: "x1y2", Type: "decision", Title: "new", Status: "active", Hook: "h", Created: "2026-08-27",
	}))
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "3fa9", Title: "spec", Status: "active", Created: "2026-08-27", Decisions: []string{"k9m2"},
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Warnings, "decision_superseded"))
	require.False(t, report.HasErrors())
}

func TestRun_SpecDecisionReferencesNonexistentPath_IsError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "3fa9", Title: "spec", Status: "draft", Created: "2026-08-27", Decisions: []string{"docs/adr/0034-nope.md"},
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.True(t, hasIssue(report.Errors, "decision_path_not_found"))
}

func TestRun_SpecDecisionPathExists_NoError(t *testing.T) {
	repo := setup(t)
	require.NoError(t, os.MkdirAll(filepath.Join(repo, "docs", "adr"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(repo, "docs", "adr", "0034-x.md"), []byte("# ADR\n"), 0o644))
	require.NoError(t, ledger.SaveSpec(repo, ledger.Spec{
		ID: "3fa9", Title: "spec", Status: "active", Created: "2026-08-27", Decisions: []string{"docs/adr/0034-x.md"},
	}))

	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.False(t, hasIssue(report.Errors, "decision_path_not_found"))
	require.False(t, hasIssue(report.Errors, "spec_without_decision"))
}

func TestRun_JSONShape_NeverNilSlices(t *testing.T) {
	repo := setup(t)
	report, err := Run(repo, ledger.DefaultConfig(), Options{})
	require.NoError(t, err)
	require.NotNil(t, report.Errors)
	require.NotNil(t, report.Warnings)
	require.NotNil(t, report.Fixed)
}
