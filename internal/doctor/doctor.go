// Package doctor implements the integrity checks run by `atlas doctor`
// (SPEC.md S2): orphan id references, blocked_by cycles, done workitems
// stranded in work/, log entries missing a required summary, focus
// staleness (S5.2), expired or dangling claims (auto-removed), stale
// active cards, malformed frontmatter, and duplicate ids. It never
// panics: a malformed file is reported as an Issue and skipped, not a
// crash, so one bad file never hides problems in the rest of the ledger.
package doctor

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/danielino/atlas/internal/claims"
	"github.com/danielino/atlas/internal/gitx"
	"github.com/danielino/atlas/internal/ledger"
	"github.com/danielino/atlas/internal/state"
)

// cardMaxAgeDays is the age (S2) beyond which an active card is flagged
// as a staleness warning.
const cardMaxAgeDays = 90

// specDraftMaxAgeDays is the age (S9.4) beyond which a draft spec is
// flagged as a staleness warning.
const specDraftMaxAgeDays = 30

// isDecisionID distinguishes a bare ATLAS id from a repo-relative ADR
// path in a spec's decisions list (S9.8): ids never contain a path
// separator or a file extension, while every ADR path in practice does.
func isDecisionID(entry string) bool {
	return !strings.ContainsAny(entry, "/.")
}

// Issue is one integrity problem or informational fix, identified by a
// stable machine-readable Code plus a human-readable Message.
type Issue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Report is the full result of a doctor run, grouped by severity per
// SPEC.md S2. Errors, Warnings and Fixed are never nil so they marshal
// to `[]`, not `null`, even when empty.
type Report struct {
	Errors   []Issue  `json:"errors"`
	Warnings []Issue  `json:"warnings"`
	Fixed    []string `json:"fixed"`
}

// HasErrors reports whether the report contains at least one error.
// Per SPEC.md S2 this is what decides `atlas doctor`'s exit code
// (3 if true; warnings alone do not affect it).
func (r Report) HasErrors() bool {
	return len(r.Errors) > 0
}

// Options configures Run. Now is the reference clock used for claim
// expiry and the card-age check; it defaults to time.Now so tests can
// inject a deterministic value.
type Options struct {
	Now func() time.Time
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// Run performs every integrity check against the ATLAS project rooted
// at root and returns the resulting Report. It returns a non-nil error
// only for genuine I/O failures (e.g. an unreadable directory); a
// malformed or inconsistent ledger is reported as Issues in the
// Report, not as an error.
func Run(root string, cfg ledger.Config, opts Options) (Report, error) {
	report := Report{Errors: []Issue{}, Warnings: []Issue{}, Fixed: []string{}}

	workitems, wIssues, err := scanWorkitems(root)
	if err != nil {
		return Report{}, err
	}
	cards, cIssues, err := scanCards(root)
	if err != nil {
		return Report{}, err
	}
	specs, sIssues, err := scanSpecs(root)
	if err != nil {
		return Report{}, err
	}
	report.Errors = append(report.Errors, wIssues...)
	report.Errors = append(report.Errors, cIssues...)
	report.Errors = append(report.Errors, sIssues...)

	logEntries, err := ledger.ReadLog(root)
	if err != nil {
		return Report{}, err
	}

	closedIDs := make(map[string]struct{}, len(logEntries))
	for _, e := range logEntries {
		closedIDs[e.ID] = struct{}{}
	}

	activeIDs := make(map[string]struct{}, len(workitems)+len(cards)+len(specs))
	for _, w := range workitems {
		activeIDs[w.ID] = struct{}{}
	}
	for _, c := range cards {
		activeIDs[c.ID] = struct{}{}
	}
	for _, s := range specs {
		activeIDs[s.ID] = struct{}{}
	}

	cardsByID := make(map[string]ledger.Card, len(cards))
	for _, c := range cards {
		cardsByID[c.ID] = c
	}

	report.Errors = append(report.Errors, duplicateIDIssues(workitems, cards, specs)...)
	report.Errors = append(report.Errors, orphanRefIssues(workitems, cards, specs, activeIDs, closedIDs)...)
	report.Errors = append(report.Errors, cycleIssues(workitems)...)
	report.Errors = append(report.Errors, doneInWorkIssues(workitems)...)
	report.Errors = append(report.Errors, resurrectedWorkitemIssues(workitems, logEntries)...)
	report.Errors = append(report.Errors, missingSummaryIssues(logEntries)...)

	specErrors, specWarnings := specRefIssues(workitems, specs)
	report.Errors = append(report.Errors, specErrors...)
	report.Warnings = append(report.Warnings, specWarnings...)

	decisionErrors, decisionWarnings := decisionIssues(root, specs, cardsByID)
	report.Errors = append(report.Errors, decisionErrors...)
	report.Warnings = append(report.Warnings, decisionWarnings...)

	stale, err := state.Stale(root)
	if err != nil {
		return Report{}, err
	}
	if stale {
		report.Warnings = append(report.Warnings, Issue{
			Code:    "stale_focus",
			Message: "ledger is stale: no .atlas file has been touched since before the last few commits",
		})
	}

	report.Warnings = append(report.Warnings, oldCardIssues(cards, opts.now())...)
	report.Warnings = append(report.Warnings, oldDraftSpecIssues(specs, opts.now())...)

	fixed, err := reconcileClaims(root, cfg, activeIDs, opts)
	if err != nil {
		return Report{}, err
	}
	report.Fixed = append(report.Fixed, fixed...)

	sortIssues(report.Errors)
	sortIssues(report.Warnings)
	sort.Strings(report.Fixed)

	return report, nil
}

func sortIssues(issues []Issue) {
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Code != issues[j].Code {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].Message < issues[j].Message
	})
}

// scanWorkitems reads every file under .atlas/work/ directly (rather
// than via ledger.ListWorkitems, which aborts at the first malformed
// file): a bad file is reported as an Issue and skipped, so every
// other file is still checked.
func scanWorkitems(root string) ([]ledger.Workitem, []Issue, error) {
	dir := filepath.Join(root, ".atlas", "work")
	names, err := sortedFileNames(dir)
	if err != nil {
		return nil, nil, err
	}

	var items []ledger.Workitem
	var issues []Issue
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, err
		}
		fm, body, err := ledger.ParseFrontmatter(data)
		if err != nil {
			issues = append(issues, Issue{Code: "malformed_frontmatter", Message: "work/" + name + ": " + err.Error()})
			continue
		}
		var w ledger.Workitem
		if err := yaml.Unmarshal(fm, &w); err != nil {
			issues = append(issues, Issue{Code: "malformed_frontmatter", Message: "work/" + name + ": " + err.Error()})
			continue
		}
		w.Body = string(body)
		items = append(items, w)
	}
	return items, issues, nil
}

// scanCards is scanWorkitems' twin for .atlas/cards/.
func scanCards(root string) ([]ledger.Card, []Issue, error) {
	dir := filepath.Join(root, ".atlas", "cards")
	names, err := sortedFileNames(dir)
	if err != nil {
		return nil, nil, err
	}

	var items []ledger.Card
	var issues []Issue
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, err
		}
		fm, body, err := ledger.ParseFrontmatter(data)
		if err != nil {
			issues = append(issues, Issue{Code: "malformed_frontmatter", Message: "cards/" + name + ": " + err.Error()})
			continue
		}
		var c ledger.Card
		if err := yaml.Unmarshal(fm, &c); err != nil {
			issues = append(issues, Issue{Code: "malformed_frontmatter", Message: "cards/" + name + ": " + err.Error()})
			continue
		}
		c.Body = string(body)
		items = append(items, c)
	}
	return items, issues, nil
}

// scanSpecs is scanWorkitems'/scanCards' twin for .atlas/specs/.
func scanSpecs(root string) ([]ledger.Spec, []Issue, error) {
	dir := filepath.Join(root, ".atlas", "specs")
	names, err := sortedFileNames(dir)
	if err != nil {
		return nil, nil, err
	}

	var items []ledger.Spec
	var issues []Issue
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, nil, err
		}
		fm, body, err := ledger.ParseFrontmatter(data)
		if err != nil {
			issues = append(issues, Issue{Code: "malformed_frontmatter", Message: "specs/" + name + ": " + err.Error()})
			continue
		}
		var s ledger.Spec
		if err := yaml.Unmarshal(fm, &s); err != nil {
			issues = append(issues, Issue{Code: "malformed_frontmatter", Message: "specs/" + name + ": " + err.Error()})
			continue
		}
		s.Body = string(body)
		items = append(items, s)
	}
	return items, issues, nil
}

func sortedFileNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
}

// duplicateIDIssues flags any id that names more than one file across
// work/, cards/ and specs/ combined.
func duplicateIDIssues(workitems []ledger.Workitem, cards []ledger.Card, specs []ledger.Spec) []Issue {
	count := make(map[string]int)
	for _, w := range workitems {
		count[w.ID]++
	}
	for _, c := range cards {
		count[c.ID]++
	}
	for _, s := range specs {
		count[s.ID]++
	}

	var issues []Issue
	for id, n := range count {
		if n > 1 {
			issues = append(issues, Issue{
				Code:    "duplicate_id",
				Message: "id " + id + " is used by more than one file across work/, cards/ and specs/",
			})
		}
	}
	return issues
}

// orphanRefIssues flags blocked_by/discovered_from/superseded_by
// references that name an id present in neither the active ledger nor
// the closed-item log.
func orphanRefIssues(workitems []ledger.Workitem, cards []ledger.Card, specs []ledger.Spec, activeIDs, closedIDs map[string]struct{}) []Issue {
	exists := func(id string) bool {
		if id == "" {
			return true
		}
		if _, ok := activeIDs[id]; ok {
			return true
		}
		_, ok := closedIDs[id]
		return ok
	}

	var issues []Issue
	for _, w := range workitems {
		for _, dep := range w.BlockedBy {
			if !exists(dep) {
				issues = append(issues, Issue{
					Code:    "orphan_ref",
					Message: "workitem " + w.ID + ": blocked_by references nonexistent id " + dep,
				})
			}
		}
		if !exists(w.DiscoveredFrom) {
			issues = append(issues, Issue{
				Code:    "orphan_ref",
				Message: "workitem " + w.ID + ": discovered_from references nonexistent id " + w.DiscoveredFrom,
			})
		}
	}
	for _, c := range cards {
		if !exists(c.SupersededBy) {
			issues = append(issues, Issue{
				Code:    "orphan_ref",
				Message: "card " + c.ID + ": superseded_by references nonexistent id " + c.SupersededBy,
			})
		}
	}
	for _, s := range specs {
		if !exists(s.SupersededBy) {
			issues = append(issues, Issue{
				Code:    "orphan_ref",
				Message: "spec " + s.ID + ": superseded_by references nonexistent id " + s.SupersededBy,
			})
		}
	}
	return issues
}

// specRefIssues implements S9.4's workitem.spec checks: a reference to a
// nonexistent spec is an ERROR (spec_not_found); a reference to a spec
// that has since been superseded is a WARNING (spec_superseded), since
// the workitem can still proceed but should be re-pointed.
func specRefIssues(workitems []ledger.Workitem, specs []ledger.Spec) (errors, warnings []Issue) {
	byID := make(map[string]ledger.Spec, len(specs))
	for _, s := range specs {
		byID[s.ID] = s
	}

	for _, w := range workitems {
		if w.Spec == "" {
			continue
		}
		s, ok := byID[w.Spec]
		if !ok {
			errors = append(errors, Issue{
				Code:    "spec_not_found",
				Message: "workitem " + w.ID + ": spec references nonexistent id " + w.Spec,
			})
			continue
		}
		if s.Status == "superseded" {
			warnings = append(warnings, Issue{
				Code:    "spec_superseded",
				Message: "workitem " + w.ID + ": spec " + w.Spec + " is superseded by " + s.SupersededBy,
			})
		}
	}
	return errors, warnings
}

// oldDraftSpecIssues flags draft specs (S9.4) older than
// specDraftMaxAgeDays: a draft is meant to be short-lived, either
// activated (anchored to a decision) or abandoned.
func oldDraftSpecIssues(specs []ledger.Spec, now time.Time) []Issue {
	cutoff := now.AddDate(0, 0, -specDraftMaxAgeDays)

	var issues []Issue
	for _, s := range specs {
		if s.Status != "draft" {
			continue
		}
		created, err := time.Parse("2006-01-02", s.Created)
		if err != nil {
			continue
		}
		if created.Before(cutoff) {
			issues = append(issues, Issue{
				Code:    "old_draft_spec",
				Message: "spec " + s.ID + " (" + s.Title + ") has been a draft since " + s.Created + ", older than " + strconv.Itoa(specDraftMaxAgeDays) + " days",
			})
		}
	}
	return issues
}

// decisionIssues implements S9.8's decision-traceability checks: an
// active spec must carry at least one decisions entry (ERROR, since this
// can only happen from a hand-edit — `spec activate` itself refuses an
// empty list); each entry must resolve, either to an existing decision
// card (ERROR if missing or not type "decision"; WARNING "spec may need
// revision" if the card is superseded) or to a path that exists on disk
// (ERROR if not).
func decisionIssues(root string, specs []ledger.Spec, cardsByID map[string]ledger.Card) (errors, warnings []Issue) {
	for _, s := range specs {
		if s.Status == "superseded" {
			continue
		}
		if s.Status == "active" && len(s.Decisions) == 0 {
			errors = append(errors, Issue{
				Code:    "spec_without_decision",
				Message: "spec " + s.ID + " (" + s.Title + ") is active but has no linked decision",
			})
		}
		for _, entry := range s.Decisions {
			if isDecisionID(entry) {
				card, ok := cardsByID[entry]
				if !ok || card.Type != "decision" {
					errors = append(errors, Issue{
						Code:    "decision_not_found",
						Message: "spec " + s.ID + ": decisions references nonexistent decision card " + entry,
					})
					continue
				}
				if card.Status == "superseded" {
					warnings = append(warnings, Issue{
						Code:    "decision_superseded",
						Message: "spec " + s.ID + ": decision " + entry + " is superseded by " + card.SupersededBy + " — spec may need revision",
					})
				}
				continue
			}
			if _, err := os.Stat(filepath.Join(root, entry)); err != nil {
				errors = append(errors, Issue{
					Code:    "decision_path_not_found",
					Message: "spec " + s.ID + ": decisions references nonexistent path " + entry,
				})
			}
		}
	}
	return errors, warnings
}

// cycleIssues detects cycles in the blocked_by graph among active
// workitems (an edge to a closed or nonexistent id can never be part
// of a cycle and is reported separately as an orphan reference).
func cycleIssues(workitems []ledger.Workitem) []Issue {
	byID := make(map[string]ledger.Workitem, len(workitems))
	ids := make([]string, 0, len(workitems))
	for _, w := range workitems {
		byID[w.ID] = w
		ids = append(ids, w.ID)
	}
	sort.Strings(ids)

	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(ids))
	var stack []string
	seen := make(map[string]bool)
	var issues []Issue

	var visit func(id string)
	visit = func(id string) {
		color[id] = gray
		stack = append(stack, id)
		for _, dep := range byID[id].BlockedBy {
			if _, ok := byID[dep]; !ok {
				continue
			}
			switch color[dep] {
			case white:
				visit(dep)
			case gray:
				cycle := cyclePath(stack, dep)
				key := cycleKey(cycle)
				if !seen[key] {
					seen[key] = true
					issues = append(issues, Issue{
						Code:    "cycle",
						Message: "cycle in blocked_by: " + strings.Join(cycle, " -> "),
					})
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
	}

	for _, id := range ids {
		if color[id] == white {
			visit(id)
		}
	}
	return issues
}

// cyclePath extracts the cycle from stack starting at the first
// occurrence of dep, closing it by repeating dep at the end.
func cyclePath(stack []string, dep string) []string {
	idx := 0
	for i, id := range stack {
		if id == dep {
			idx = i
			break
		}
	}
	cycle := append([]string{}, stack[idx:]...)
	cycle = append(cycle, dep)
	return cycle
}

// cycleKey normalizes a cycle (dropping the repeated closing id and
// sorting) so the same cycle found from different entry points is
// only reported once.
func cycleKey(cycle []string) string {
	members := append([]string{}, cycle[:len(cycle)-1]...)
	sort.Strings(members)
	return strings.Join(members, ",")
}

func doneInWorkIssues(workitems []ledger.Workitem) []Issue {
	var issues []Issue
	for _, w := range workitems {
		if w.Status == "done" {
			issues = append(issues, Issue{
				Code:    "done_in_work",
				Message: "workitem " + w.ID + " has status done but is still in work/ (should have been compacted by `atlas task done`)",
			})
		}
	}
	return issues
}

// resurrectedWorkitemIssues catches a workitem file that is active in
// work/ while its id already has a `task done` entry in log.jsonl: the
// closed-and-active states can only coexist if a merge brought back a
// stale (pre-close) copy of the file — a `git merge` never consults
// claims, so this is the one corruption mode nothing else in the
// pipeline stops (SPEC.md S9.8 has no equivalent for tasks; this is
// that check). Only "task" log entries count: a superseded card
// deliberately stays in cards/ alongside its "card" log entry (S2),
// which is not a resurrection.
func resurrectedWorkitemIssues(workitems []ledger.Workitem, entries []ledger.LogEntry) []Issue {
	closedTaskIDs := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		if e.Kind == "task" {
			closedTaskIDs[e.ID] = struct{}{}
		}
	}

	var issues []Issue
	for _, w := range workitems {
		if _, closed := closedTaskIDs[w.ID]; !closed {
			continue
		}
		issues = append(issues, Issue{
			Code: "resurrected_workitem",
			Message: "workitem " + w.ID + " (" + w.Title + ") is active in work/ but already has a " +
				"`task done` entry in log.jsonl — likely resurrected by a merge that brought back a " +
				"stale copy of its file; delete .atlas/work/" + w.ID + "-*.md, it was already closed",
		})
	}
	return issues
}

func missingSummaryIssues(entries []ledger.LogEntry) []Issue {
	var issues []Issue
	for _, e := range entries {
		if e.Kind == "task" && strings.TrimSpace(e.Summary) == "" {
			issues = append(issues, Issue{
				Code:    "missing_summary",
				Message: "log entry " + e.ID + " (task) has no summary",
			})
		}
	}
	return issues
}

func oldCardIssues(cards []ledger.Card, now time.Time) []Issue {
	cutoff := now.AddDate(0, 0, -cardMaxAgeDays)

	var issues []Issue
	for _, c := range cards {
		if c.Status != "active" {
			continue
		}
		created, err := time.Parse("2006-01-02", c.Created)
		if err != nil {
			continue // tolerant: an unparseable date isn't this check's problem to report
		}
		if created.Before(cutoff) {
			issues = append(issues, Issue{
				Code:    "old_card",
				Message: "card " + c.ID + " (" + c.Title + ") has been active since " + c.Created + ", older than " + strconv.Itoa(cardMaxAgeDays) + " days",
			})
		}
	}
	return issues
}

// reconcileClaims removes expired claims and claims referencing a
// workitem id that no longer exists, returning a human-readable note
// per removal. Skipped entirely (no error) when root is not a git
// repository: there is no claims store to reconcile.
func reconcileClaims(root string, cfg ledger.Config, activeIDs map[string]struct{}, opts Options) ([]string, error) {
	commonDir, err := gitx.CommonDir(root)
	if err != nil {
		return nil, nil
	}

	mgr := &claims.Manager{CommonDir: commonDir, TTLHours: cfg.Claims.TTLHours, Now: opts.Now}

	expired, err := mgr.Cleanup()
	if err != nil {
		return nil, err
	}

	var fixed []string
	for _, id := range expired {
		fixed = append(fixed, "removed expired claim on "+id)
	}

	remaining, err := mgr.List()
	if err != nil {
		return fixed, err
	}
	for _, c := range remaining {
		if _, ok := activeIDs[c.ID]; ok {
			continue
		}
		if err := mgr.Release(c.ID); err != nil {
			return fixed, err
		}
		fixed = append(fixed, "removed claim on "+c.ID+": workitem does not exist")
	}

	return fixed, nil
}
