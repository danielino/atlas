package contextc

import (
	"time"

	"github.com/danielino/atlas/internal/ledger"
	"github.com/danielino/atlas/internal/state"
)

// fixedNow returns the injectable clock used by every golden test:
// 2026-08-27, matching the project's "today" so goldens read naturally.
func fixedNow() func() time.Time {
	t := time.Date(2026, 8, 27, 9, 0, 0, 0, time.UTC)
	return func() time.Time { return t }
}

// fullState is the all-sections-populated fixture: one doing item (with
// branch+evidence), one blocked item (with blocked_by+reason), one ready
// item, one active decision card, one recent closure, recent git
// commits, and a dirty worktree with a claim held elsewhere.
func fullState() state.State {
	return state.State{
		Focus: "Ship F3: state derivation + budgeted context compiler, golden-tested.\n" +
			"Keep CLI/doctor out of scope until F4/F5.",
		Now: []ledger.Workitem{
			{
				ID:       "a1b2",
				Title:    "Implement budget degradation ladder",
				Status:   "doing",
				Branch:   "feature/context",
				Evidence: []string{"internal/contextc/contextc.go"},
			},
			{
				ID:        "c3d4",
				Title:     "Wire claims into Ground.Elsewhere",
				Status:    "blocked",
				BlockedBy: []string{"e5f6"},
				Reason:    "waiting on gitx CommonDir behavior under worktrees",
			},
		},
		Ready: []ledger.Workitem{
			{ID: "f6a7", Title: "Add doctor integrity checks", Status: "todo"},
		},
		ActiveCards: []ledger.Card{
			{
				ID:    "k9m2",
				Type:  "decision",
				Title: "Use O_EXCL for claims",
				Hook:  "Claim = file O_EXCL in $GIT_COMMON_DIR, mai mutex",
			},
		},
		RecentClosed: []ledger.LogEntry{
			{
				ID:      "b2c3",
				Kind:    "task",
				Title:   "Implement gitx wrapper",
				Summary: "Added Branch/HeadShort/IsDirty/RecentCommits",
				Closed:  "2026-08-25T10:00:00Z",
			},
		},
		RecentCommits: []string{
			"9f8e7d6 feat(gitx,claims): git wrapper and atomic claims",
			"1a2b3c4 feat(ledger): core data model with TDD",
		},
		Ground: state.Ground{
			Branch:     "feature/context",
			Head:       "9f8e7d6",
			Dirty:      true,
			DirtyCount: 2,
			Elsewhere: []state.ElsewhereClaim{
				{ID: "d4e5", Branch: "feature/other"},
			},
		},
		Stale: false,
	}
}

// staleState covers the header's STALE tag with every other section
// empty (so FOCUS and POINTERS are the only sections rendered).
func staleState() state.State {
	return state.State{
		Focus: "Investigate why HEAD moved without ledger updates.",
		Stale: true,
	}
}

// noGitState covers a repo-less project: Ground is zero-valued (GROUND
// section omitted) and a doing item has neither branch nor evidence.
func noGitState() state.State {
	return state.State{
		Focus: "Working outside git; ledger only.",
		Now: []ledger.Workitem{
			{ID: "a1b2", Title: "Draft ledger schema", Status: "doing"},
		},
	}
}

func defaultCfg() ledger.Config {
	return ledger.DefaultConfig()
}

// overBudgetState has enough content in every degradable section (READY,
// RULES, RECENT) to force each S5.3 degradation step in turn as the
// budget shrinks, while FOCUS/NOW stay small and constant.
func overBudgetState() state.State {
	s := state.State{
		Focus: "Drive the budget ladder down through every step.",
		Now: []ledger.Workitem{
			{ID: "a1b2", Title: "Keep NOW populated so it is never removed", Status: "doing", Branch: "feature/context"},
		},
	}

	readyTitles := []string{
		"Add doctor integrity checks for orphaned blocked_by references",
		"Wire cobra command surface for task lifecycle transitions",
		"Implement policy plan-mutation warn/strict enforcement",
		"Write end-to-end smoke test across init/seed/task/context",
		"Document config.toml defaults and override precedence",
	}
	for i, title := range readyTitles {
		s.Ready = append(s.Ready, ledger.Workitem{ID: readyID(i), Title: title, Status: "todo"})
	}

	hooks := []string{
		"Claims live under the git common directory, never inside the versioned repo tree, so worktrees share one lock",
		"Superseded cards stay on disk for history but are excluded from every context render going forward",
		"Frontmatter parsing is always tolerant: a malformed block is reported by doctor, never a crash anywhere",
	}
	cardIDs := []string{"k9m2", "p1q2", "r3s4"}
	for i, hook := range hooks {
		s.ActiveCards = append(s.ActiveCards, ledger.Card{ID: cardIDs[i], Type: "decision", Title: hook, Hook: hook})
	}

	closedTitles := []string{"Implement gitx wrapper", "Implement claims manager", "Implement ledger frontmatter codec", "Implement testutil helpers", "Implement id collision handling"}
	for i, title := range closedTitles {
		s.RecentClosed = append(s.RecentClosed, ledger.LogEntry{
			ID:      readyID(10 + i),
			Kind:    "task",
			Title:   title,
			Summary: title + ": added with full test coverage",
			Closed:  "2026-08-2" + string(rune('0'+(5-i%5))) + "T10:00:00Z",
		})
	}
	s.RecentCommits = []string{
		"9f8e7d6 feat(gitx,claims): git wrapper and atomic claims",
		"1a2b3c4 feat(ledger): core data model with TDD",
		"7c6d5e4 chore: bootstrap module and gitignore",
		"3b2a1f0 chore: initial commit",
		"0e0d0c0 chore: scaffold repository",
	}

	s.Ground = state.Ground{Branch: "feature/context", Head: "9f8e7d6", Dirty: false}
	return s
}

// specsState is fullState() plus a SPECS section: one draft and one active
// spec, exercising the general brief's new S9.3 section in an otherwise
// fully-populated fixture.
func specsState() state.State {
	s := fullState()
	s.Specs = []state.SpecSummary{
		{Spec: ledger.Spec{ID: "3fa9", Title: "Workload execution retry semantics", Status: "draft", Created: "2026-08-20"}, OpenTasks: 0},
		{Spec: ledger.Spec{ID: "77aa", Title: "Claim lifecycle and TTL handling", Status: "active", Created: "2026-08-10"}, OpenTasks: 2},
	}
	return s
}

// overBudgetSpecsState is overBudgetState() plus enough SPECS content to
// force the S9.3 SPECS-reduction degradation step on its own, independent
// of overBudgetState (which stays untouched so its existing goldens don't
// move).
func overBudgetSpecsState() state.State {
	s := overBudgetState()
	s.Specs = []state.SpecSummary{
		{Spec: ledger.Spec{ID: "3fa9", Title: "Workload execution retry semantics", Status: "draft", Created: "2026-08-20"}, OpenTasks: 0},
		{Spec: ledger.Spec{ID: "77aa", Title: "Claim lifecycle and TTL handling policy", Status: "active", Created: "2026-08-10"}, OpenTasks: 2},
		{Spec: ledger.Spec{ID: "88bb", Title: "Context budget degradation ladder ordering", Status: "active", Created: "2026-08-05"}, OpenTasks: 1},
	}
	return s
}

func readyID(i int) string {
	const hex = "0123456789abcdef"
	return string([]byte{hex[i%16], hex[(i/16)%16], hex[(i/256)%16], hex[(i/4096)%16]})
}
