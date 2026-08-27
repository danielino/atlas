// Package contextc compiles the budgeted "ATLAS CONTEXT" brief (SPEC.md
// S5) from a derived internal/state.State: a token-budgeted plain-text
// rendering (Render), an equivalent JSON rendering (RenderJSON), and a
// workitem-centered rendering (RenderTarget) for `atlas context <id>`.
package contextc

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/danielino/atlas/internal/ledger"
	"github.com/danielino/atlas/internal/state"
	"gopkg.in/yaml.v3"
)

// EstimateTokens implements the S5.3 token estimate: rune count / 4.
func EstimateTokens(s string) int {
	return len([]rune(s)) / 4
}

// stalenessTag is the exact header suffix appended when the ledger is
// stale, for state.FreshnessN == 5 (SPEC.md S5.1/S5.2).
const stalenessTag = " [STALE: ledger older than last 5 commits]"

// pointersLine is the fixed POINTERS footer (S5.1), verbatim.
const pointersLine = "Detail: `atlas show <id>` · Full state: `atlas state` · History: `atlas log --grep <x>`"

func effectiveNow(now func() time.Time) time.Time {
	if now == nil {
		return time.Now()
	}
	return now()
}

// degradeState tracks how far the S5.3 degradation ladder has been
// walked for the current render attempt.
type degradeState struct {
	recentLines    int  // -1 = full, 0 = section removed, N>0 = first N lines
	specsReduced   bool // SPECS lines cut to "- [id] title"
	rulesTruncated bool // hooks cut to 60 chars, type suffix dropped
	readyShown     int  // -1 = show all, otherwise how many READY lines to show
}

func fullDegrade(readyLen int) degradeState {
	return degradeState{recentLines: -1, rulesTruncated: false, readyShown: readyLen}
}

// Render renders the full budgeted text context per S5.1/S5.3.
func Render(s state.State, cfg ledger.Config, now func() time.Time) string {
	t := effectiveNow(now)
	budget := cfg.Context.BudgetTokens

	dg := fullDegrade(len(s.Ready))
	text := renderText(s, t, dg)
	if budget <= 0 || EstimateTokens(text) <= budget {
		return text
	}

	// Step 1: RECENT -> first 3 lines.
	dg.recentLines = 3
	text = renderText(s, t, dg)
	if EstimateTokens(text) <= budget {
		return text
	}

	// Step 2: RECENT removed entirely.
	dg.recentLines = 0
	text = renderText(s, t, dg)
	if EstimateTokens(text) <= budget {
		return text
	}

	// Step 3: SPECS reduced to "- [id] title".
	dg.specsReduced = true
	text = renderText(s, t, dg)
	if EstimateTokens(text) <= budget {
		return text
	}

	// Step 4: RULES truncated to "[id] first 60 chars of hook".
	dg.rulesTruncated = true
	text = renderText(s, t, dg)
	if EstimateTokens(text) <= budget {
		return text
	}

	// Step 5: READY truncated, one item removed at a time. FOCUS and NOW
	// are never removed, so once READY is exhausted we accept staying
	// over budget rather than touching them.
	for shown := len(s.Ready) - 1; shown >= 0; shown-- {
		dg.readyShown = shown
		text = renderText(s, t, dg)
		if EstimateTokens(text) <= budget {
			return text
		}
	}

	return text
}

// writeSection appends sec (a "## HEADER\n...\n"-shaped section, or ""
// for an omitted one) directly after whatever came before, with no blank
// line in between: SPEC.md S5.1's example shows sections running back to
// back, never separated by a blank line.
func writeSection(b *strings.Builder, sec string) {
	b.WriteString(sec)
}

func renderText(s state.State, t time.Time, dg degradeState) string {
	var b strings.Builder

	b.WriteString(header(t, s.Stale))
	b.WriteString("\n")

	writeSection(&b, renderFocus(s.Focus))
	writeSection(&b, renderNow(s.Now))
	writeSection(&b, renderReady(s.Ready, dg.readyShown))
	writeSection(&b, renderRules(s.ActiveCards, dg.rulesTruncated))
	writeSection(&b, renderSpecs(s.Specs, dg.specsReduced))
	writeSection(&b, renderRecent(s.RecentClosed, s.RecentCommits, dg.recentLines))
	writeSection(&b, renderGround(s.Ground))

	b.WriteString("## POINTERS\n")
	b.WriteString(pointersLine)
	b.WriteString("\n")

	return b.String()
}

func header(t time.Time, stale bool) string {
	h := fmt.Sprintf("# ATLAS CONTEXT (%s)", t.Format("2006-01-02"))
	if stale {
		h += stalenessTag
	}
	return h
}

func renderFocus(focus string) string {
	trimmed := strings.TrimRight(focus, "\n")
	if trimmed == "" {
		return ""
	}
	return "## FOCUS\n" + trimmed + "\n"
}

func renderNow(items []ledger.Workitem) string {
	if len(items) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## NOW\n")
	for _, w := range items {
		b.WriteString(nowLine(w))
		b.WriteString("\n")
	}
	return b.String()
}

func nowLine(w ledger.Workitem) string {
	switch w.Status {
	case "blocked":
		return "- [" + w.ID + "] " + w.Title + " (" + blockedSuffix(w) + ")"
	default: // "doing"
		suffix := "doing"
		if w.Branch != "" {
			suffix = "doing, branch " + w.Branch
		}
		line := "- [" + w.ID + "] " + w.Title + " (" + suffix + ")"
		if len(w.Evidence) > 0 {
			line += " — evidence: " + strings.Join(w.Evidence, ", ")
		}
		return line
	}
}

func blockedSuffix(w ledger.Workitem) string {
	var onID string
	if len(w.BlockedBy) > 0 {
		onID = w.BlockedBy[0]
	}
	switch {
	case onID != "" && w.Reason != "":
		return "blocked on " + onID + ": " + w.Reason
	case onID != "":
		return "blocked on " + onID
	case w.Reason != "":
		return "blocked: " + w.Reason
	default:
		return "blocked"
	}
}

func renderReady(items []ledger.Workitem, shown int) string {
	if len(items) == 0 {
		return ""
	}
	if shown < 0 || shown > len(items) {
		shown = len(items)
	}
	var b strings.Builder
	b.WriteString("## READY\n")
	for _, w := range items[:shown] {
		b.WriteString("- [" + w.ID + "] " + w.Title + "\n")
	}
	if hidden := len(items) - shown; hidden > 0 {
		b.WriteString(fmt.Sprintf("… (+%d more: atlas state)\n", hidden))
	}
	return b.String()
}

func renderRules(cards []ledger.Card, truncated bool) string {
	if len(cards) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## RULES\n")
	for _, c := range cards {
		if truncated {
			b.WriteString("- [" + c.ID + "] " + truncateHook(c.Hook) + "\n")
		} else {
			b.WriteString("- [" + c.ID + "] " + c.Hook + " (" + c.Type + ")\n")
		}
	}
	return b.String()
}

func truncateHook(hook string) string {
	r := []rune(hook)
	if len(r) <= 60 {
		return hook
	}
	return string(r[:60])
}

// renderSpecs renders the SPECS section (S9.3): one line per draft/active
// spec, "- [id] title (status, N open tasks)". When reduced (part of the
// S5.3/S9.3 degradation ladder), lines are cut to "- [id] title".
func renderSpecs(specs []state.SpecSummary, reduced bool) string {
	if len(specs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("## SPECS\n")
	for _, sp := range specs {
		if reduced {
			b.WriteString("- [" + sp.ID + "] " + sp.Title + "\n")
		} else {
			b.WriteString(fmt.Sprintf("- [%s] %s (%s, %d open tasks)\n", sp.ID, sp.Title, sp.Status, sp.OpenTasks))
		}
	}
	return b.String()
}

func renderRecent(entries []ledger.LogEntry, commits []string, lines int) string {
	if lines == 0 {
		return ""
	}
	if len(entries) == 0 && len(commits) == 0 {
		return ""
	}

	var all []string
	for _, e := range entries {
		all = append(all, recentEntryLine(e))
	}
	if len(commits) > 0 {
		all = append(all, "- git: "+strings.Join(commits, ", "))
	}

	if lines > 0 && lines < len(all) {
		all = all[:lines]
	}
	if len(all) == 0 {
		return ""
	}

	return "## RECENT\n" + strings.Join(all, "\n") + "\n"
}

func recentEntryLine(e ledger.LogEntry) string {
	date := e.Closed
	if t, err := time.Parse(time.RFC3339, e.Closed); err == nil {
		date = t.Format("2006-01-02")
	}
	return "- [" + e.ID + "] " + e.Summary + " (" + date + ")"
}

func renderGround(g state.Ground) string {
	if g.Branch == "" {
		return ""
	}
	worktree := "clean"
	if g.Dirty {
		worktree = fmt.Sprintf("dirty(%d files)", g.DirtyCount)
	}
	line := fmt.Sprintf("branch: %s · HEAD: %s · worktree: %s", g.Branch, g.Head, worktree)
	if len(g.Elsewhere) > 0 {
		parts := make([]string, len(g.Elsewhere))
		for i, e := range g.Elsewhere {
			parts[i] = e.ID + " on " + e.Branch
		}
		line += " · elsewhere: [" + strings.Join(parts, ", ") + "]"
	}
	return "## GROUND\n" + line + "\n"
}

// --- JSON rendering (S5.4) ---

type jsonWorkitem struct {
	ID             string   `json:"id"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	Created        string   `json:"created,omitempty"`
	BlockedBy      []string `json:"blocked_by,omitempty"`
	DiscoveredFrom string   `json:"discovered_from,omitempty"`
	Branch         string   `json:"branch,omitempty"`
	Evidence       []string `json:"evidence,omitempty"`
	Summary        string   `json:"summary,omitempty"`
	Reason         string   `json:"reason,omitempty"`
}

func toJSONWorkitem(w ledger.Workitem) jsonWorkitem {
	return jsonWorkitem{
		ID:             w.ID,
		Title:          w.Title,
		Status:         w.Status,
		Created:        w.Created,
		BlockedBy:      w.BlockedBy,
		DiscoveredFrom: w.DiscoveredFrom,
		Branch:         w.Branch,
		Evidence:       w.Evidence,
		Summary:        w.Summary,
		Reason:         w.Reason,
	}
}

type jsonRule struct {
	ID   string `json:"id"`
	Hook string `json:"hook"`
	Type string `json:"type"`
}

type jsonElsewhere struct {
	ID     string `json:"id"`
	Branch string `json:"branch"`
}

type jsonGround struct {
	Branch    string          `json:"branch"`
	Head      string          `json:"head"`
	Dirty     bool            `json:"dirty"`
	Elsewhere []jsonElsewhere `json:"elsewhere"`
}

type jsonBudget struct {
	Limit     int `json:"limit"`
	Estimated int `json:"estimated"`
}

type jsonSpec struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	OpenTasks int    `json:"open_tasks"`
}

type jsonContext struct {
	Generated string            `json:"generated"`
	Stale     bool              `json:"stale"`
	Focus     string            `json:"focus"`
	Now       []jsonWorkitem    `json:"now"`
	Ready     []jsonWorkitem    `json:"ready"`
	Rules     []jsonRule        `json:"rules"`
	Specs     []jsonSpec        `json:"specs"`
	Recent    []ledger.LogEntry `json:"recent"`
	Ground    jsonGround        `json:"ground"`
	Budget    jsonBudget        `json:"budget"`
}

// RenderJSON renders the S5.4 JSON shape of the context. The "estimated"
// budget field is EstimateTokens of the equivalent text rendering
// (Render) of the same state, at the same instant.
func RenderJSON(s state.State, cfg ledger.Config, now func() time.Time) ([]byte, error) {
	t := effectiveNow(now)
	textEquivalent := Render(s, cfg, func() time.Time { return t })

	nowItems := make([]jsonWorkitem, 0, len(s.Now))
	for _, w := range s.Now {
		nowItems = append(nowItems, toJSONWorkitem(w))
	}
	readyItems := make([]jsonWorkitem, 0, len(s.Ready))
	for _, w := range s.Ready {
		readyItems = append(readyItems, toJSONWorkitem(w))
	}
	rules := make([]jsonRule, 0, len(s.ActiveCards))
	for _, c := range s.ActiveCards {
		rules = append(rules, jsonRule{ID: c.ID, Hook: c.Hook, Type: c.Type})
	}
	elsewhere := make([]jsonElsewhere, 0, len(s.Ground.Elsewhere))
	for _, e := range s.Ground.Elsewhere {
		elsewhere = append(elsewhere, jsonElsewhere{ID: e.ID, Branch: e.Branch})
	}
	specs := make([]jsonSpec, 0, len(s.Specs))
	for _, sp := range s.Specs {
		specs = append(specs, jsonSpec{ID: sp.ID, Title: sp.Title, Status: sp.Status, OpenTasks: sp.OpenTasks})
	}
	recent := s.RecentClosed
	if recent == nil {
		recent = []ledger.LogEntry{}
	}

	doc := jsonContext{
		Generated: t.Format(time.RFC3339),
		Stale:     s.Stale,
		Focus:     s.Focus,
		Now:       nowItems,
		Ready:     readyItems,
		Rules:     rules,
		Specs:     specs,
		Recent:    recent,
		Ground: jsonGround{
			Branch:    s.Ground.Branch,
			Head:      s.Ground.Head,
			Dirty:     s.Ground.Dirty,
			Elsewhere: elsewhere,
		},
		Budget: jsonBudget{
			Limit:     cfg.Context.BudgetTokens,
			Estimated: EstimateTokens(textEquivalent),
		},
	}

	return json.MarshalIndent(doc, "", "  ")
}

// --- Target mode (S5.5) ---

// RenderTarget renders FOCUS + the full workitem (frontmatter+body) + (if
// the workitem has a spec:) the full spec body + related cards + GROUND +
// POINTERS, budgeted like Render. cards is the candidate pool (typically
// all active cards); relatedCards filters it down to the ones that mention
// w or share evidence paths with it. spec is the workitem's linked spec in
// full (nil if it has none), independent of state.State.Specs so that a
// spec's full text is available here even though the general brief only
// ever carries a one-line summary of it.
func RenderTarget(s state.State, w ledger.Workitem, cards []ledger.Card, spec *ledger.Spec, cfg ledger.Config, now func() time.Time) string {
	t := effectiveNow(now)
	budget := cfg.Context.BudgetTokens
	related := relatedCards(w, cards)

	text := renderTargetText(s, w, related, spec, -1, t, false)
	if budget <= 0 || EstimateTokens(text) <= budget {
		return text
	}

	// Step 1: the spec body degrades FIRST (S9.3), truncated in shrinking
	// steps down to nothing (just the "full spec" pointer line) before any
	// other section is touched.
	if spec != nil {
		bodyLen := len([]rune(spec.Body))
		const step = 40
		max := bodyLen - step
		if max < 0 {
			max = 0
		}
		for {
			text = renderTargetText(s, w, related, spec, max, t, false)
			if EstimateTokens(text) <= budget {
				return text
			}
			if max == 0 {
				break
			}
			max -= step
			if max < 0 {
				max = 0
			}
		}
	}

	// Step 2: cards -> truncated hooks, then dropped entirely. FOCUS and
	// the target workitem itself are never removed.
	text = renderTargetText(s, w, related, spec, 0, t, true)
	if EstimateTokens(text) <= budget {
		return text
	}

	return renderTargetText(s, w, nil, spec, 0, t, false)
}

// renderTargetText renders one candidate text for RenderTarget's budget
// ladder. specMaxRunes controls the SPEC section's body truncation: -1
// means the full body, otherwise the body is cut to that many runes
// (S9.3's "spec body degrades first"). Ignored when spec is nil.
func renderTargetText(s state.State, w ledger.Workitem, cards []ledger.Card, spec *ledger.Spec, specMaxRunes int, t time.Time, rulesTruncated bool) string {
	var b strings.Builder
	b.WriteString(header(t, s.Stale))
	b.WriteString("\n")

	writeSection(&b, renderFocus(s.Focus))

	b.WriteString("## TASK\n")
	b.WriteString(renderWorkitemFull(w))

	if spec != nil {
		b.WriteString(renderSpecSection(*spec, specMaxRunes))
	}

	writeSection(&b, renderRules(cards, rulesTruncated))
	writeSection(&b, renderGround(s.Ground))

	b.WriteString("## POINTERS\n")
	b.WriteString(pointersLine)
	b.WriteString("\n")

	return b.String()
}

// renderSpecSection renders the target mode's "## SPEC [id] title" section
// (S9.3) with the spec's full body, or (maxRunes >= 0) the body truncated
// to maxRunes runes with a final "… (full spec: atlas show <id>)" line.
// Right under the header, a single "Decisions: ..." line (S9.8) lists the
// spec's linked decisions; it is never degraded, since it is one line.
func renderSpecSection(spec ledger.Spec, maxRunes int) string {
	var b strings.Builder
	b.WriteString("## SPEC [" + spec.ID + "] " + spec.Title + "\n")
	if len(spec.Decisions) > 0 {
		b.WriteString("Decisions: " + strings.Join(spec.Decisions, ", ") + "\n")
	}

	body := strings.TrimRight(spec.Body, "\n")
	runes := []rune(body)
	if maxRunes < 0 || len(runes) <= maxRunes {
		b.WriteString(body)
		b.WriteString("\n")
		return b.String()
	}

	if maxRunes > 0 {
		b.WriteString(string(runes[:maxRunes]))
		b.WriteString("\n")
	}
	b.WriteString("… (full spec: atlas show " + spec.ID + ")\n")
	return b.String()
}

func renderWorkitemFull(w ledger.Workitem) string {
	fm, err := yaml.Marshal(w)
	if err != nil {
		// yaml.Marshal on a plain struct value never fails in practice;
		// degrade to an empty frontmatter block rather than panicking.
		fm = nil
	}
	data := ledger.SerializeFrontmatter(fm, []byte(w.Body))
	out := string(data)
	if !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	return out
}

// stripLineSuffix removes an optional trailing ":<lines>" suffix from an
// evidence path such as "packages/core/pipeline/reconcile.py:120-180",
// returning just the file path.
func stripLineSuffix(evidence string) string {
	if i := strings.LastIndex(evidence, ":"); i >= 0 {
		return evidence[:i]
	}
	return evidence
}

// relatedCards returns the cards whose id appears in w's body or evidence
// list, or whose own evidence shares a path with w's evidence (S5.5),
// sorted by id.
func relatedCards(w ledger.Workitem, cards []ledger.Card) []ledger.Card {
	taskPaths := make(map[string]struct{}, len(w.Evidence))
	for _, e := range w.Evidence {
		taskPaths[stripLineSuffix(e)] = struct{}{}
	}

	var out []ledger.Card
	for _, c := range cards {
		if strings.Contains(w.Body, c.ID) {
			out = append(out, c)
			continue
		}
		mentioned := false
		for _, e := range w.Evidence {
			if e == c.ID {
				mentioned = true
				break
			}
		}
		if mentioned {
			out = append(out, c)
			continue
		}
		shared := false
		for _, ce := range c.Evidence {
			if _, ok := taskPaths[stripLineSuffix(ce)]; ok {
				shared = true
				break
			}
		}
		if shared {
			out = append(out, c)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
