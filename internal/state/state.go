// Package state derives the project's current, in-memory view from the
// on-disk ledger (internal/ledger) plus git (internal/gitx) and claims
// (internal/claims): which workitems are ready to start, which are in
// flight, whether the ledger looks stale relative to recent commits, and
// what other branches currently hold claims. It never panics: when the
// working directory is not a git repository (or has too little history),
// the git-derived parts of the state degrade to empty/false rather than
// erroring.
package state

import (
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/dmarcocci/atlas/internal/claims"
	"github.com/dmarcocci/atlas/internal/gitx"
	"github.com/dmarcocci/atlas/internal/ledger"
)

// FreshnessN is the number of most-recent commits considered by the
// staleness check (S5.2): the ledger is stale if its newest mtime predates
// the timestamp of the FreshnessN-th most recent commit.
const FreshnessN = 5

// RecentCommitsN is the number of oneline commit summaries carried in
// State.RecentCommits for the RECENT section's "- git: ..." line.
const RecentCommitsN = 5

// ElsewhereClaim describes a claim on a workitem held by a branch other
// than the caller's current one ("in corso altrove").
type ElsewhereClaim struct {
	ID     string
	Branch string
}

// Ground is the git-derived snapshot shown in the context's GROUND
// section. A zero-value Ground (Branch == "") means no git information is
// available (not a repository).
type Ground struct {
	Branch     string
	Head       string
	Dirty      bool
	DirtyCount int
	Elsewhere  []ElsewhereClaim
}

// SpecSummary is a draft or active spec (PLAN.md S9) plus its open-task
// count, exposed in State for the context brief's SPECS section and for
// `atlas state`. Superseded specs are never included: they are excluded
// from the ledger's "current" view just like superseded cards.
type SpecSummary struct {
	ledger.Spec
	// OpenTasks is the count of workitems currently under .atlas/work/
	// whose `spec:` field names this spec. Every file under work/ is, by
	// construction, still open (`atlas task done` removes the file), so
	// this is a plain count with no status filter.
	OpenTasks int
}

// State is the full derived view of the project assembled by Build.
type State struct {
	Focus        string
	Now          []ledger.Workitem
	Ready        []ledger.Workitem
	ActiveCards  []ledger.Card
	Specs        []SpecSummary
	RecentClosed []ledger.LogEntry
	// RecentCommits holds up to gitx' default window of oneline commit
	// summaries, most recent first, for the RECENT section's "- git: ..."
	// line. Empty when root is not a git repository.
	RecentCommits []string
	Ground        Ground
	Stale         bool
}

// sortedByID returns a copy of items sorted by ID for deterministic
// output (directory listing order is not guaranteed stable across
// filesystems).
func sortedByID(items []ledger.Workitem) []ledger.Workitem {
	out := make([]ledger.Workitem, len(items))
	copy(out, items)
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Ready returns the todo workitems that are unblocked: every id in their
// blocked_by either does not refer to any currently-active workitem, or
// refers to one already recorded as closed. In other words, a blocked_by
// id only blocks when it names a workitem that is presently active (todo,
// doing, or blocked); a reference to a closed or nonexistent id never
// blocks. Results are sorted by id.
func Ready(workitems []ledger.Workitem, closedIDs map[string]struct{}) []ledger.Workitem {
	active := make(map[string]struct{}, len(workitems))
	for _, w := range workitems {
		active[w.ID] = struct{}{}
	}

	var ready []ledger.Workitem
	for _, w := range workitems {
		if w.Status != "todo" {
			continue
		}
		blocked := false
		for _, dep := range w.BlockedBy {
			if _, isClosed := closedIDs[dep]; isClosed {
				continue
			}
			if _, isActive := active[dep]; isActive {
				blocked = true
				break
			}
		}
		if !blocked {
			ready = append(ready, w)
		}
	}
	return sortedByID(ready)
}

// Now returns the workitems currently in flight: status "doing" or
// "blocked". Results are sorted by id.
func Now(workitems []ledger.Workitem) []ledger.Workitem {
	var now []ledger.Workitem
	for _, w := range workitems {
		if w.Status == "doing" || w.Status == "blocked" {
			now = append(now, w)
		}
	}
	return sortedByID(now)
}

// Elsewhere returns the claims held by branches other than currentBranch,
// sorted by workitem id. Used to surface "in corso altrove" in GROUND.
func Elsewhere(list []claims.Claim, currentBranch string) []ElsewhereClaim {
	var out []ElsewhereClaim
	for _, c := range list {
		if c.Branch != currentBranch {
			out = append(out, ElsewhereClaim{ID: c.ID, Branch: c.Branch})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Stale implements the S5.2 freshness check: the ledger is stale if the
// newest mtime among files under root/.atlas is older than the timestamp
// of the FreshnessN-th most recent commit, and there is git history deep
// enough to make that comparison meaningful. It degrades gracefully to
// (false, nil) when root is not a git repository or has fewer than
// FreshnessN commits, and never returns an error for those cases.
func Stale(root string) (bool, error) {
	times, err := gitx.CommitTimestamps(root, FreshnessN)
	if err != nil {
		if err == gitx.ErrNotARepo {
			return false, nil
		}
		return false, err
	}
	if len(times) < FreshnessN {
		return false, nil
	}
	nthCommitTime := times[FreshnessN-1]

	newest, found, err := newestMtime(filepath.Join(root, ".atlas"))
	if err != nil {
		return false, err
	}
	if !found {
		return false, nil
	}

	return newest.Before(nthCommitTime), nil
}

// newestMtime walks dir recursively and returns the most recent
// modification time among regular files. found is false if dir does not
// exist or contains no files.
func newestMtime(dir string) (newest time.Time, found bool, err error) {
	walkErr := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if !found || info.ModTime().After(newest) {
			newest = info.ModTime()
			found = true
		}
		return nil
	})
	if walkErr != nil && !os.IsNotExist(walkErr) {
		return time.Time{}, false, walkErr
	}
	return newest, found, nil
}

// Options configures Build. Now is the reference clock used for
// RecentClosed's window (config.recent_days) and defaults to time.Now.
type Options struct {
	Now func() time.Time
}

func (o Options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// Build assembles the full derived State for the project rooted at root.
// It never panics: when root is not (or is no longer) a git repository,
// Ground is left zero-valued and Stale is false, but every ledger-derived
// field is still populated normally.
func Build(root string, cfg ledger.Config, opts Options) (State, error) {
	focus, err := ledger.ReadFocus(root)
	if err != nil {
		return State{}, err
	}

	workitems, err := ledger.ListWorkitems(root)
	if err != nil {
		return State{}, err
	}

	closedIDs, err := ledger.ClosedIDs(root)
	if err != nil {
		return State{}, err
	}

	activeCards, err := ledger.ListActiveCards(root)
	if err != nil {
		return State{}, err
	}
	sort.Slice(activeCards, func(i, j int) bool { return activeCards[i].ID < activeCards[j].ID })

	specs, err := buildSpecSummaries(root, workitems)
	if err != nil {
		return State{}, err
	}

	logEntries, err := ledger.ReadLog(root)
	if err != nil {
		return State{}, err
	}
	recent := recentClosed(logEntries, cfg.Context.RecentDays, opts.now())

	stale, err := Stale(root)
	if err != nil {
		return State{}, err
	}

	ground := buildGround(root, cfg)

	var recentCommits []string
	if commits, err := gitx.RecentCommits(root, RecentCommitsN); err == nil {
		recentCommits = commits
	}

	return State{
		Focus:         focus,
		Now:           Now(workitems),
		Ready:         Ready(workitems, closedIDs),
		ActiveCards:   activeCards,
		Specs:         specs,
		RecentClosed:  recent,
		RecentCommits: recentCommits,
		Ground:        ground,
		Stale:         stale,
	}, nil
}

// buildSpecSummaries returns the draft/active specs (superseded excluded),
// sorted by id, each annotated with its open-task count derived from
// workitems.
func buildSpecSummaries(root string, workitems []ledger.Workitem) ([]SpecSummary, error) {
	all, err := ledger.ListSpecs(root)
	if err != nil {
		return nil, err
	}

	openTasks := make(map[string]int)
	for _, w := range workitems {
		if w.Spec != "" {
			openTasks[w.Spec]++
		}
	}

	var out []SpecSummary
	for _, s := range all {
		if s.Status == "superseded" {
			continue
		}
		out = append(out, SpecSummary{Spec: s, OpenTasks: openTasks[s.ID]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// recentClosed returns the log entries closed within recentDays of now,
// most-recently-closed first. Entries with an unparseable Closed timestamp
// are excluded (tolerant: never errors).
func recentClosed(entries []ledger.LogEntry, recentDays int, now time.Time) []ledger.LogEntry {
	cutoff := now.AddDate(0, 0, -recentDays)

	var out []ledger.LogEntry
	for _, e := range entries {
		closed, err := time.Parse(time.RFC3339, e.Closed)
		if err != nil {
			continue
		}
		if !closed.Before(cutoff) {
			out = append(out, e)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Closed > out[j].Closed })
	return out
}

// buildGround assembles the git-derived Ground snapshot. Any git failure
// (most commonly: not a repository) yields a zero-value Ground rather
// than an error, per package doc.
func buildGround(root string, cfg ledger.Config) Ground {
	branch, err := gitx.Branch(root)
	if err != nil {
		return Ground{}
	}
	head, err := gitx.HeadShort(root)
	if err != nil {
		return Ground{}
	}
	dirty, count, err := gitx.IsDirty(root)
	if err != nil {
		return Ground{}
	}

	ground := Ground{
		Branch:     branch,
		Head:       head,
		Dirty:      dirty,
		DirtyCount: count,
	}

	commonDir, err := gitx.CommonDir(root)
	if err != nil {
		return ground
	}
	mgr := &claims.Manager{CommonDir: commonDir, TTLHours: cfg.Claims.TTLHours}
	list, err := mgr.List()
	if err != nil {
		return ground
	}
	ground.Elsewhere = Elsewhere(list, branch)

	return ground
}
