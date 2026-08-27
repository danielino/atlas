package contextc

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/dmarcocci/atlas/internal/ledger"
	"github.com/dmarcocci/atlas/internal/state"
	"github.com/stretchr/testify/require"
)

func TestEstimateTokens(t *testing.T) {
	require.Equal(t, 0, EstimateTokens(""))
	require.Equal(t, 1, EstimateTokens("abcd"))
	require.Equal(t, 2, EstimateTokens("abcdefgh"))
	require.Equal(t, 1, EstimateTokens("abcdefg")) // 7 runes / 4 = 1
}

func TestRender_FullFixture_Golden(t *testing.T) {
	got := Render(fullState(), defaultCfg(), fixedNow())
	compareGolden(t, "full.golden", got)
}

func TestRender_StaleFixture_Golden(t *testing.T) {
	got := Render(staleState(), defaultCfg(), fixedNow())
	compareGolden(t, "stale.golden", got)
}

func TestRender_NoGitFixture_Golden(t *testing.T) {
	got := Render(noGitState(), defaultCfg(), fixedNow())
	compareGolden(t, "no-git.golden", got)
}

func TestRender_EmptyState_OnlyHeaderAndPointers(t *testing.T) {
	got := Render(state.State{}, defaultCfg(), fixedNow())
	require.Equal(t, "# ATLAS CONTEXT (2026-08-27)\n"+
		"## POINTERS\n"+pointersLine+"\n", got)
}

func TestRender_SectionOrder(t *testing.T) {
	got := Render(fullState(), defaultCfg(), fixedNow())
	order := []string{"## FOCUS", "## NOW", "## READY", "## RULES", "## RECENT", "## GROUND", "## POINTERS"}
	last := -1
	for _, marker := range order {
		idx := strings.Index(got, marker)
		require.True(t, idx > last, "expected %q after previous section", marker)
		last = idx
	}
}

func TestRender_NoBlankLinesBetweenSections(t *testing.T) {
	got := Render(fullState(), defaultCfg(), fixedNow())
	require.False(t, strings.Contains(got, "\n\n"), "sections must not be separated by a blank line")
}

// --- S5.3 budget degradation ---

func TestRender_UnderBudget_NoDegradation(t *testing.T) {
	s := overBudgetState()
	cfg := defaultCfg()
	full := renderText(s, fixedNow()(), fullDegrade(len(s.Ready)))
	cfg.Context.BudgetTokens = EstimateTokens(full) + 1000

	got := Render(s, cfg, fixedNow())
	require.Equal(t, full, got)
	require.Contains(t, got, "- git:")
	readySection := got[strings.Index(got, "## READY"):strings.Index(got, "## RULES")]
	require.Equal(t, 5, strings.Count(readySection, "\n- ["))
}

func TestRender_DegradesRecentToThreeLines(t *testing.T) {
	s := overBudgetState()
	full := renderText(s, fixedNow()(), fullDegrade(len(s.Ready)))
	dg3 := fullDegrade(len(s.Ready))
	dg3.recentLines = 3
	step1 := renderText(s, fixedNow()(), dg3)
	require.Less(t, EstimateTokens(step1), EstimateTokens(full))

	cfg := defaultCfg()
	cfg.Context.BudgetTokens = EstimateTokens(step1)

	got := Render(s, cfg, fixedNow())
	require.Equal(t, step1, got)

	// RECENT kept but trimmed: 3 lines total, so the git-commits line
	// (which would be 6th) is gone.
	recentSection := got[strings.Index(got, "## RECENT"):strings.Index(got, "## GROUND")]
	require.Equal(t, 4, strings.Count(recentSection, "\n")) // header + 3 lines
	require.NotContains(t, recentSection, "- git:")
}

func TestRender_DegradesRecentToRemoved(t *testing.T) {
	s := overBudgetState()
	dgRemoved := fullDegrade(len(s.Ready))
	dgRemoved.recentLines = 0
	stepRemoved := renderText(s, fixedNow()(), dgRemoved)
	require.NotContains(t, stepRemoved, "## RECENT")

	cfg := defaultCfg()
	cfg.Context.BudgetTokens = EstimateTokens(stepRemoved)

	got := Render(s, cfg, fixedNow())
	require.Equal(t, stepRemoved, got)
	require.NotContains(t, got, "## RECENT")
	require.Contains(t, got, "## RULES")
}

func TestRender_DegradesRulesToTruncatedHooks(t *testing.T) {
	s := overBudgetState()
	dgTrunc := fullDegrade(len(s.Ready))
	dgTrunc.recentLines = 0
	dgTrunc.rulesTruncated = true
	stepTrunc := renderText(s, fixedNow()(), dgTrunc)
	require.NotContains(t, stepTrunc, "(decision)")

	cfg := defaultCfg()
	cfg.Context.BudgetTokens = EstimateTokens(stepTrunc)

	got := Render(s, cfg, fixedNow())
	require.Equal(t, stepTrunc, got)
	require.Contains(t, got, "## RULES")
	require.NotContains(t, got, "(decision)")
	compareGolden(t, "budget-rules-truncated.golden", got)
}

func TestRender_DegradesReadyTruncation(t *testing.T) {
	s := overBudgetState()
	dg := degradeState{recentLines: 0, rulesTruncated: true, readyShown: 2}
	stepReady := renderText(s, fixedNow()(), dg)
	require.Contains(t, stepReady, "… (+3 altri: atlas state)")

	cfg := defaultCfg()
	cfg.Context.BudgetTokens = EstimateTokens(stepReady)

	got := Render(s, cfg, fixedNow())
	require.LessOrEqual(t, EstimateTokens(got), cfg.Context.BudgetTokens)
	require.Contains(t, got, "atlas state")
	require.Contains(t, got, "## FOCUS")
	require.Contains(t, got, "## NOW")
	compareGolden(t, "budget-ready-truncated.golden", got)
}

func TestRender_ExtremeLowBudget_NeverRemovesFocusOrNow(t *testing.T) {
	s := overBudgetState()
	cfg := defaultCfg()
	cfg.Context.BudgetTokens = 1 // impossible to satisfy

	got := Render(s, cfg, fixedNow())
	require.Contains(t, got, "## FOCUS")
	require.Contains(t, got, "## NOW")
	require.NotContains(t, got, "## RECENT")
	require.NotContains(t, got, "(decision)")
}

func TestRender_ZeroBudget_NoDegradation(t *testing.T) {
	s := fullState()
	cfg := defaultCfg()
	cfg.Context.BudgetTokens = 0

	full := renderText(s, fixedNow()(), fullDegrade(len(s.Ready)))
	got := Render(s, cfg, fixedNow())
	require.Equal(t, full, got)
}

// --- RenderJSON (S5.4) ---

func TestRenderJSON_Shape(t *testing.T) {
	data, err := RenderJSON(fullState(), defaultCfg(), fixedNow())
	require.NoError(t, err)

	var doc map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &doc))

	require.Equal(t, "2026-08-27T09:00:00Z", doc["generated"])
	require.Equal(t, false, doc["stale"])
	require.Contains(t, doc["focus"], "Ship F3")

	now := doc["now"].([]interface{})
	require.Len(t, now, 2)
	firstNow := now[0].(map[string]interface{})
	require.Equal(t, "a1b2", firstNow["id"])
	require.Equal(t, "doing", firstNow["status"])
	require.Equal(t, "feature/context", firstNow["branch"])

	ready := doc["ready"].([]interface{})
	require.Len(t, ready, 1)

	rules := doc["rules"].([]interface{})
	require.Len(t, rules, 1)
	rule := rules[0].(map[string]interface{})
	require.Equal(t, "k9m2", rule["id"])
	require.Equal(t, "decision", rule["type"])
	require.Contains(t, rule["hook"], "O_EXCL")

	recent := doc["recent"].([]interface{})
	require.Len(t, recent, 1)

	ground := doc["ground"].(map[string]interface{})
	require.Equal(t, "feature/context", ground["branch"])
	require.Equal(t, "9f8e7d6", ground["head"])
	require.Equal(t, true, ground["dirty"])
	elsewhere := ground["elsewhere"].([]interface{})
	require.Len(t, elsewhere, 1)
	elsewhereEntry := elsewhere[0].(map[string]interface{})
	require.Equal(t, "d4e5", elsewhereEntry["id"])
	require.Equal(t, "feature/other", elsewhereEntry["branch"])

	budget := doc["budget"].(map[string]interface{})
	require.Equal(t, float64(1500), budget["limit"])
	require.Greater(t, budget["estimated"], float64(0))
}

func TestRenderJSON_EmptyState_NoNullArrays(t *testing.T) {
	data, err := RenderJSON(state.State{}, defaultCfg(), fixedNow())
	require.NoError(t, err)
	require.NotContains(t, string(data), "null")
}

func TestRenderJSON_Golden(t *testing.T) {
	data, err := RenderJSON(fullState(), defaultCfg(), fixedNow())
	require.NoError(t, err)
	compareGolden(t, "full.json.golden", string(data)+"\n")
}

// --- RenderTarget (S5.5) ---

func targetWorkitem() ledger.Workitem {
	return ledger.Workitem{
		ID:       "a1b2",
		Title:    "Fix container reconcile retry",
		Status:   "doing",
		Created:  "2026-08-27",
		Branch:   "feature/retry",
		Evidence: []string{"packages/core/pipeline/reconcile.py:120-180"},
		Summary:  "",
		Body:     "Investigate retry backoff and reconcile loop.\nRelated decision: see k9m2 for claim strategy.\n",
	}
}

func targetCardPool() []ledger.Card {
	return []ledger.Card{
		{
			ID:       "k9m2",
			Type:     "decision",
			Title:    "Use O_EXCL for claims",
			Status:   "active",
			Hook:     "Claim = file O_EXCL in $GIT_COMMON_DIR, mai mutex",
			Evidence: nil,
		},
		{
			ID:       "z9z9",
			Type:     "knowledge",
			Title:    "Unrelated card",
			Status:   "active",
			Hook:     "Totally unrelated hook",
			Evidence: []string{"some/other/path.go"},
		},
	}
}

func TestRelatedCards_MentionInBody(t *testing.T) {
	got := relatedCards(targetWorkitem(), targetCardPool())
	require.Len(t, got, 1)
	require.Equal(t, "k9m2", got[0].ID)
}

func TestRelatedCards_SharedEvidencePath(t *testing.T) {
	w := ledger.Workitem{Evidence: []string{"some/other/path.go:10-20"}}
	got := relatedCards(w, targetCardPool())
	require.Len(t, got, 1)
	require.Equal(t, "z9z9", got[0].ID)
}

func TestRelatedCards_NoMatch(t *testing.T) {
	w := ledger.Workitem{Body: "nothing relevant here", Evidence: []string{"unrelated/path.go"}}
	got := relatedCards(w, targetCardPool())
	require.Empty(t, got)
}

func TestRenderTarget_Golden(t *testing.T) {
	s := state.State{
		Focus:  "Ship F3.",
		Ground: state.Ground{Branch: "feature/retry", Head: "9f8e7d6", Dirty: false},
	}
	got := RenderTarget(s, targetWorkitem(), targetCardPool(), defaultCfg(), fixedNow())
	compareGolden(t, "target.golden", got)
}

func TestRenderTarget_IncludesFullBody(t *testing.T) {
	s := state.State{Focus: "Ship F3."}
	got := RenderTarget(s, targetWorkitem(), targetCardPool(), defaultCfg(), fixedNow())
	require.Contains(t, got, "## TASK")
	require.Contains(t, got, "Investigate retry backoff and reconcile loop.")
	require.Contains(t, got, "id: a1b2")
	require.Contains(t, got, "## RULES")
	require.Contains(t, got, "k9m2")
	require.NotContains(t, got, "z9z9")
}

func TestRenderTarget_DegradesRulesUnderBudget(t *testing.T) {
	s := state.State{Focus: "Ship F3."}
	full := RenderTarget(s, targetWorkitem(), targetCardPool(), defaultCfg(), fixedNow())

	cfg := defaultCfg()
	cfg.Context.BudgetTokens = EstimateTokens(full) - 1

	got := RenderTarget(s, targetWorkitem(), targetCardPool(), cfg, fixedNow())
	require.Contains(t, got, "## TASK")
	require.LessOrEqual(t, EstimateTokens(got), EstimateTokens(full))
}

func TestRenderTarget_ExtremeLowBudget_KeepsFocusAndTask(t *testing.T) {
	s := state.State{Focus: "Ship F3."}
	cfg := defaultCfg()
	cfg.Context.BudgetTokens = 1

	got := RenderTarget(s, targetWorkitem(), targetCardPool(), cfg, fixedNow())
	require.Contains(t, got, "## FOCUS")
	require.Contains(t, got, "## TASK")
	require.NotContains(t, got, "## RULES")
}

func TestNowLine_BlockedVariants(t *testing.T) {
	cases := []struct {
		name string
		w    ledger.Workitem
		want string
	}{
		{"no id no reason", ledger.Workitem{ID: "a", Title: "t", Status: "blocked"}, "- [a] t (blocked)"},
		{"id only", ledger.Workitem{ID: "a", Title: "t", Status: "blocked", BlockedBy: []string{"e5f6"}}, "- [a] t (blocked on e5f6)"},
		{"reason only", ledger.Workitem{ID: "a", Title: "t", Status: "blocked", Reason: "waiting"}, "- [a] t (blocked: waiting)"},
		{"id and reason", ledger.Workitem{ID: "a", Title: "t", Status: "blocked", BlockedBy: []string{"e5f6"}, Reason: "waiting"}, "- [a] t (blocked on e5f6: waiting)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, nowLine(tc.w))
		})
	}
}

func TestRenderReady_ShownClampedToLength(t *testing.T) {
	items := []ledger.Workitem{{ID: "a", Title: "t1", Status: "todo"}}
	got := renderReady(items, 99)
	require.Equal(t, "## READY\n- [a] t1\n", got)
}

func TestRender_NilNow_DefaultsToTimeNow(t *testing.T) {
	got := Render(state.State{Focus: "x"}, defaultCfg(), nil)
	require.Contains(t, got, "# ATLAS CONTEXT (")
	require.Contains(t, got, "## FOCUS")
}

func TestRenderJSON_NilNow_DefaultsToTimeNow(t *testing.T) {
	data, err := RenderJSON(state.State{}, defaultCfg(), nil)
	require.NoError(t, err)
	require.Contains(t, string(data), `"generated"`)
}

func TestRenderTarget_NilNow_DefaultsToTimeNow(t *testing.T) {
	got := RenderTarget(state.State{}, targetWorkitem(), nil, defaultCfg(), nil)
	require.Contains(t, got, "## TASK")
}

func TestRelatedCards_EvidenceIDMatch(t *testing.T) {
	w := ledger.Workitem{Evidence: []string{"k9m2"}}
	got := relatedCards(w, targetCardPool())
	require.Len(t, got, 1)
	require.Equal(t, "k9m2", got[0].ID)
}
