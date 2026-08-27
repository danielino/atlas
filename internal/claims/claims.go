// Package claims implements the per-workitem claim mechanism described in
// SPEC.md S1: an atomic, filesystem-based lock stored outside the versioned
// repo (under the git common directory), so that a task cannot be started
// concurrently by two branches/sessions. Acquisition uses exclusive file
// creation (O_CREATE|O_EXCL) — never a mutex — so it is safe across
// processes and across worktrees sharing the same common directory.
package claims

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// defaultTTLHours mirrors ledger.DefaultConfig().Claims.TTLHours. Kept as a
// local constant (rather than importing internal/ledger) to avoid a
// dependency cycle risk between low-level packages; the CLI wiring layer is
// expected to pass the configured value explicitly via Manager.TTLHours.
const defaultTTLHours = 24

// Claim is the on-disk representation of a claim file:
// <git-common-dir>/atlas/claims/<id>.json
type Claim struct {
	ID       string    `json:"id"`
	Branch   string    `json:"branch"`
	Session  string    `json:"session"`
	Created  time.Time `json:"created"`
	TTLHours int       `json:"ttl_hours"`
}

// Expired reports whether the claim has outlived its TTL as of now.
func (c Claim) Expired(now time.Time) bool {
	return c.Created.Add(time.Duration(c.TTLHours) * time.Hour).Before(now)
}

// ErrClaimed is returned by Acquire when the workitem is already claimed
// by an unexpired claim. It carries the existing claim so callers can
// report who owns it.
type ErrClaimed struct {
	ID       string
	Existing Claim
}

func (e *ErrClaimed) Error() string {
	return fmt.Sprintf("claims: %q already claimed by branch %q (session %q)", e.ID, e.Existing.Branch, e.Existing.Session)
}

// Manager manages claim files under CommonDir/atlas/claims/. Now and
// Session are injectable so tests are deterministic; both have sane
// zero-value-safe defaults applied lazily (see now/session helpers).
type Manager struct {
	// CommonDir is the git common directory (see gitx.CommonDir); claim
	// files live under CommonDir/atlas/claims/.
	CommonDir string

	// Session identifies the current agent/session, used to attribute
	// claims. If empty, DefaultSession() is used.
	Session string

	// TTLHours is how long a claim stays valid before it is considered
	// expired and re-acquirable. If zero, defaultTTLHours is used.
	TTLHours int

	// Now returns the current time. If nil, time.Now is used.
	Now func() time.Time
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) session() string {
	if m.Session != "" {
		return m.Session
	}
	return DefaultSession()
}

func (m *Manager) ttlHours() int {
	if m.TTLHours != 0 {
		return m.TTLHours
	}
	return defaultTTLHours
}

// DefaultSession returns ATLAS_SESSION if set, otherwise "<hostname>-<pid>".
func DefaultSession() string {
	if s := os.Getenv("ATLAS_SESSION"); s != "" {
		return s
	}
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

func (m *Manager) dir() string {
	return filepath.Join(m.CommonDir, "atlas", "claims")
}

func (m *Manager) path(id string) string {
	return filepath.Join(m.dir(), id+".json")
}

func (m *Manager) readClaim(id string) (Claim, error) {
	data, err := os.ReadFile(m.path(id))
	if err != nil {
		return Claim{}, err
	}
	var c Claim
	if err := json.Unmarshal(data, &c); err != nil {
		return Claim{}, fmt.Errorf("claims: parse %s: %w", m.path(id), err)
	}
	return c, nil
}

// create writes claim c to disk so that two concurrent creators race at
// the filesystem level and only one can win, AND no reader ever observes
// a partially-written claim file.
//
// DEVIATION from SPEC.md S0/S1 ("exclusive creation with
// os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)"): a plain
// O_CREATE|O_EXCL creates the (empty) file atomically, but writing its
// content is a separate step — a concurrent Get/List/Acquire can observe
// the file before it is fully written and fail to parse it (this was
// caught by the N=8 concurrency test: one loser occasionally got a
// generic "unreadable claim" error instead of ErrClaimed). To fix that
// while keeping exclusive-creation semantics, the full content is first
// written to a temp file in the same directory, then published
// atomically via os.Link: link fails with an "already exists" error
// (os.IsExist(err) is true, exactly as O_EXCL's would) if another
// creator already won — but only after the content is complete.
// Caveat: unlike O_EXCL, os.Link requires the target filesystem to
// support hard links (fails on e.g. FAT32/exFAT or some network shares);
// revisit if Windows/exotic-filesystem support becomes a requirement.
func (m *Manager) create(c Claim) error {
	if err := os.MkdirAll(m.dir(), 0o755); err != nil {
		return fmt.Errorf("claims: create claims dir: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("claims: marshal claim: %w", err)
	}

	tmp, err := os.CreateTemp(m.dir(), ".tmp-"+c.ID+"-*")
	if err != nil {
		return fmt.Errorf("claims: create temp file for %s: %w", c.ID, err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // best effort; the link (if any) keeps the data safe

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("claims: write claim %s: %w", c.ID, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("claims: write claim %s: %w", c.ID, err)
	}

	if err := os.Link(tmpPath, m.path(c.ID)); err != nil {
		return err
	}
	return nil
}

// Acquire attempts to claim id for branch. If an unexpired claim already
// exists it returns *ErrClaimed. If an expired claim exists it is removed
// and creation is retried once. Session and TTLHours come from the
// Manager.
func (m *Manager) Acquire(id, branch string) (Claim, error) {
	c := Claim{
		ID:       id,
		Branch:   branch,
		Session:  m.session(),
		Created:  m.now(),
		TTLHours: m.ttlHours(),
	}

	if err := m.create(c); err != nil {
		if !os.IsExist(err) {
			return Claim{}, fmt.Errorf("claims: acquire %s: %w", id, err)
		}

		existing, readErr := m.readClaim(id)
		if readErr != nil {
			return Claim{}, fmt.Errorf("claims: acquire %s: existing claim unreadable: %w", id, readErr)
		}

		if !existing.Expired(m.now()) {
			return Claim{}, &ErrClaimed{ID: id, Existing: existing}
		}

		// Expired: remove and retry once.
		if err := os.Remove(m.path(id)); err != nil && !os.IsNotExist(err) {
			return Claim{}, fmt.Errorf("claims: acquire %s: remove expired claim: %w", id, err)
		}
		if err := m.create(c); err != nil {
			if os.IsExist(err) {
				// Someone else won the race after we removed the expired
				// claim; report their claim as ErrClaimed.
				existing, readErr := m.readClaim(id)
				if readErr != nil {
					return Claim{}, fmt.Errorf("claims: acquire %s: %w", id, readErr)
				}
				return Claim{}, &ErrClaimed{ID: id, Existing: existing}
			}
			return Claim{}, fmt.Errorf("claims: acquire %s: %w", id, err)
		}
	}

	return c, nil
}

// Get returns the claim for id, if a claim file exists (expired or not).
func (m *Manager) Get(id string) (Claim, bool) {
	c, err := m.readClaim(id)
	if err != nil {
		return Claim{}, false
	}
	return c, true
}

// List returns all non-expired claims.
func (m *Manager) List() ([]Claim, error) {
	entries, err := os.ReadDir(m.dir())
	if err != nil {
		if os.IsNotExist(err) {
			return []Claim{}, nil
		}
		return nil, fmt.Errorf("claims: list: %w", err)
	}

	now := m.now()
	claims := make([]Claim, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-len(".json")]
		c, err := m.readClaim(id)
		if err != nil {
			continue // malformed/missing claim file: skip, doctor handles reporting
		}
		if !c.Expired(now) {
			claims = append(claims, c)
		}
	}
	return claims, nil
}

// Release removes the claim for id. Releasing a claim that does not exist
// is not an error (idempotent).
func (m *Manager) Release(id string) error {
	err := os.Remove(m.path(id))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("claims: release %s: %w", id, err)
	}
	return nil
}

// Steal forcibly removes any existing claim for id (expired or not) and
// acquires it for branch.
func (m *Manager) Steal(id, branch string) (Claim, error) {
	if err := os.Remove(m.path(id)); err != nil && !os.IsNotExist(err) {
		return Claim{}, fmt.Errorf("claims: steal %s: remove existing: %w", id, err)
	}

	c := Claim{
		ID:       id,
		Branch:   branch,
		Session:  m.session(),
		Created:  m.now(),
		TTLHours: m.ttlHours(),
	}
	if err := m.create(c); err != nil {
		return Claim{}, fmt.Errorf("claims: steal %s: %w", id, err)
	}
	return c, nil
}

// Cleanup removes all expired claim files and returns the ids removed.
// Doctor uses this to keep the claims directory tidy.
func (m *Manager) Cleanup() ([]string, error) {
	entries, err := os.ReadDir(m.dir())
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("claims: cleanup: %w", err)
	}

	now := m.now()
	var removed []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		id := e.Name()[:len(e.Name())-len(".json")]
		c, err := m.readClaim(id)
		if err != nil {
			continue
		}
		if c.Expired(now) {
			if err := os.Remove(m.path(id)); err != nil && !os.IsNotExist(err) {
				return removed, fmt.Errorf("claims: cleanup: remove %s: %w", id, err)
			}
			removed = append(removed, id)
		}
	}
	if removed == nil {
		removed = []string{}
	}
	return removed, nil
}
