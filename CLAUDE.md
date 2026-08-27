# CLAUDE.md — atlas

ATLAS is a Go CLI: a git-native project-state ledger + budgeted context compiler
for AI coding agents. Read `README.md` for what the tool does; this file is for
working ON the codebase.

## Binding documents

- **`PLAN.md` is the binding implementation spec** (sections S0–S10: data model,
  command semantics, exit codes, JSON shapes, context format, test rules).
  When code and PLAN.md disagree, one of them is a bug — fix deliberately, never
  silently drift. `ANALYSIS.md` is the architecture/discovery record (rationale,
  product boundaries, rejected alternatives). Both are written in Italian by
  design; code, comments, docs and ALL user-facing CLI output are English-only.

## Layout

```
cmd/atlas/          entrypoint, exit-code mapping
internal/ledger/    data model: workitem, card, spec, focus, log.jsonl, config, ids, frontmatter
internal/gitx/      git via subprocess (never a git library)
internal/claims/    cross-worktree claims in $GIT_COMMON_DIR (atomic create, TTL, steal)
internal/state/     derived state: ready, freshness, graph levels
internal/contextc/  budgeted brief renderer (text/JSON, golden-tested)
internal/doctor/    integrity checks (exit 3 on errors)
internal/cli/       cobra commands, policy, bootstrap block, seed brief
internal/testutil/  SetupRepo/SetupWorktree (temp git repos for tests)
```

## Hard rules

1. **TDD is mandatory**: write failing tests first, then implement (red→green→
   refactor). Every change ships with tests; full suite green before commit.
2. **Coverage ≥70% total** (`go test ./... -coverprofile=cover.out &&
   go tool cover -func=cover.out | tail -1`). Currently 83.4% — don't regress.
3. Quality gate before any commit: `go test ./... -race` green, `go vet ./...`
   clean, `gofmt -l .` empty.
4. Dependencies: only cobra, yaml.v3, BurntSushi/toml, testify. Nothing else.
5. All user-facing strings in English. Errors are semantic: exit 0 ok / 1 I/O /
   2 refusal (JSON `{"error":"<code>",...}` with `--json`) / 3 doctor issues.
6. Commits: Conventional Commits, single line, on `main`, NO Claude attribution
   / Co-Authored-By. Push to `origin` (github.com/danielino/atlas).
7. After changing the CLI, reinstall the binary the user runs:
   `go build -o ~/go/bin/atlas ./cmd/atlas`.

## Conventions & gotchas

- Golden tests live in `internal/contextc/testdata/` and `internal/cli/testdata/`
  (`-update` / `-update-graph-golden` flags regenerate). Hand-check regenerated
  goldens against PLAN.md wording — goldens validate the spec, not the code.
- Tests are deterministic: inject `Now func() time.Time` (contextc, claims,
  doctor, state options); never call wall-clock in assertions; git tests use
  `testutil.SetupRepo` temp repos only, no network.
- Claims use write-temp-then-`os.Link` for atomic publication (same semantics
  as O_EXCL but readers never see a half-written file). Requires hard links —
  documented trade-off in `internal/claims/claims.go`.
- One file per entity under `.atlas/` is a design invariant (merge safety);
  `log.jsonl` is append-only with `merge=union`. Don't introduce shared mutable
  files.
- The context brief has a token budget with an exact degradation ladder
  (FOCUS > NOW > GROUND > READY > RULES > SPECS > RECENT > POINTERS; FOCUS/NOW
  never dropped). Any new context content must define its place in the ladder
  and stay out of the brief unless it earns its tokens.
- `atlas graph` and other human-facing views must NEVER be wired into
  `atlas context` output.
- Specs must reference at least one decision (card id or ADR path) to activate;
  doctor enforces the invariant. Don't weaken it.
