# ATLAS — Implementation Specification

**Architectural reference:** ANALYSIS.md (binding — in case of doubt, ANALYSIS.md decides)
**Language:** Go (static binary) · **Git:** subprocess of the system `git`
**Method:** rigorous TDD (tests before code, red → green → refactor) · **Minimum coverage: 70%** (total, `go tool cover -func`)
**Branching:** everything on `main`, conventional commits per phase, NO Claude attribution in commits.
**Scope:** beyond the MVP — all Phase 1 features from ANALYSIS.md §15 plus full doctor, log query, policy, claims.

---

## S0. Project structure

```
go.mod                        # module github.com/danielino/atlas
cmd/atlas/main.go             # entrypoint, exit code handling
internal/ledger/              # id, frontmatter codec, workitem, card, focus, log.jsonl, config
internal/gitx/                # git subprocess wrapper
internal/claims/              # claim-per-file, O_EXCL, TTL, steal
internal/state/               # derived state: ready, freshness
internal/contextc/            # brief compiler (text + JSON, budget)
internal/doctor/              # integrity checks
internal/cli/                 # cobra commands, wiring, policy, bootstrap, seed
```

Allowed dependencies: `spf13/cobra`, `gopkg.in/yaml.v3`, `github.com/BurntSushi/toml`, `github.com/stretchr/testify`. Nothing else.

Portability: `path/filepath` everywhere; exclusive creation with `os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)` (portable: POSIX O_EXCL / Windows CREATE_NEW).

## S1. On-disk data model (`.atlas/`)

```
.atlas/
  focus.md            # plain markdown, 3-10 lines, no frontmatter
  work/<id>-<slug>.md # one active workitem per file
  cards/<id>-<slug>.md
  log.jsonl           # append-only, closed items
  config.toml         # optional, sensible defaults if absent
```

**ID:** 4 lowercase hex characters, randomly generated, checked against existing files (work/, cards/ and log.jsonl) for collisions; on collision regenerate (max 20 attempts, then 5 characters).
**Slug:** from the title, lowercase, `[a-z0-9-]`, max 40 chars.

**Workitem** (YAML frontmatter + markdown body):
```markdown
---
id: a1b2
title: Fix container reconcile retry
status: todo            # todo | doing | blocked | done
created: 2026-08-27
blocked_by: [c3d4]      # optional
discovered_from: e5f6   # optional
branch: feature/retry   # set by `task start`
evidence:               # optional, list of paths (with optional :lines)
  - packages/core/pipeline/reconcile.py:120-180
summary: ""             # required at done, 1 line
reason: ""              # optional, reason for the block
---
Free markdown body (intent, notes, spec).
```

**Card:**
```markdown
---
id: k9m2
type: decision          # decision | knowledge
title: Use O_EXCL for claims
status: active          # active | superseded
superseded_by: ""       # id, for superseded decisions
hook: "Claim = O_EXCL file in $GIT_COMMON_DIR, never mutex"  # 1 line for the index, REQUIRED
created: 2026-08-27
evidence: []
---
Body (for decisions: context/decision/consequences, MADR-compatible).
```

**log.jsonl** (one line per closed item):
```json
{"id":"a1b2","kind":"task","title":"...","summary":"...","closed":"2026-08-27T10:00:00Z","commit":"<HEAD short>","branch":"feature/retry"}
```
For superseded cards: `"kind":"card","superseded_by":"x1y2"`.

**config.toml** (all optional, these are the defaults):
```toml
[context]
budget_tokens = 1500
recent_days = 7

[policy]
plan_mutations = "warn"        # "warn" | "strict"
integration_branches = ["main", "develop"]

[claims]
ttl_hours = 24
```

**Claims** (outside the versioned repo): `<git-common-dir>/atlas/claims/<id>.json`
```json
{"id":"a1b2","branch":"feature/retry","session":"<ATLAS_SESSION or hostname-pid>","created":"2026-08-27T10:00:00Z","ttl_hours":24}
```
Acquisition = atomic exclusive creation. Expired claim (created+ttl < now) = treated as nonexistent (overwritable with remove+retry create). Release = delete at `done`. `--steal` = delete + create with a warning on stderr.

## S2. Command semantics

Global conventions: exit 0 = ok · exit 1 = I/O/parse error · exit 2 = semantic refusal (claimed, strict policy, missing summary, nonexistent id) · exit 3 = only `doctor` with issues. Every read command and every refusal support `--json`. JSON errors: `{"error":"<code>", ...useful fields...}`. All commands locate the repo root (dir with `.atlas/`, walking up) except `init`.

- `atlas init` — creates `.atlas/focus.md` (commented template) and `config.toml` with defaults; adds `.atlas/log.jsonl merge=union` to `.gitattributes`; installs the bootstrap block (S3) in AGENTS.md (append or creation) and, if CLAUDE.md exists, there too. Idempotent: blocks delimited by `<!-- atlas:begin -->/<!-- atlas:end -->` replaced in place on re-run. Never touches content outside the markers.
- `atlas seed` — prints the curation brief to stdout (S4). No LLM call. `--json` = `{"brief":"..."}`.
- `atlas context [id] [--json]` — compiles the brief (S5). With `id`: centered brief (full body of the workitem + cards linked via evidence/id mention + its paths). Always includes freshness and active claims from other branches ("in progress elsewhere").
- `atlas state [--json]` — full readable view: focus, all workitems by status, active cards with hook, git ground, freshness. No budget.
- `atlas task add "title" [--body -|"text"] [--blocked-by id,id] [--from id] [--evidence p1,p2]` — creates a `todo` workitem. Subject to plan-mutation policy ONLY if without `--from` (discovered work is always allowed).
- `atlas task start <id> [--steal]` — checks claim (S1); creates claim; sets `status: doing`, `branch: <current>`. If already claimed by another active branch/session: exit 2 with `{"error":"claimed","task":"..","by":"<branch>","ready":[...]}`. If the workitem has `branch` of another branch and status doing (versioned binding, different machines): same refusal unless `--steal`.
- `atlas task block <id> [--on id] [--reason "..."]` — `status: blocked`, updates blocked_by/reason. Allowed only from the branch that owns it (or if not yet owned).
- `atlas task done <id> --summary "..."` — summary REQUIRED, non-empty (exit 2 otherwise); appends to log.jsonl (with HEAD short and branch), removes the file from work/, releases the claim. Allowed only from the owning branch (or without an owner).
- `atlas card add --type decision|knowledge "title" [--hook "..."] [--body ...] [--evidence ...]` — hook required (if absent uses the title). Subject to plan-mutation policy.
- `atlas card supersede <old> <new>` — old→`status: superseded`, `superseded_by: new`; appends event to log.jsonl; the superseded file remains in cards/ but excluded from context. Plan-mutation policy.
- `atlas show <id> [--json]` — prints the full file (JSON: structured frontmatter + body).
- `atlas log [--grep pattern] [--json]` — queries log.jsonl (never in the context).
- `atlas doctor [--json]` — checks: orphaned blocked_by/discovered_from/superseded_by; cycles in blocked_by; done without summary in the log; **a workitem active in work/ whose id already has a `task done` entry in log.jsonl — "resurrected workitem" (ERROR): a merge from a stale branch brought back a pre-close copy of the file; claims never protect against this, since `git merge` never consults them**; focus not modified for >N recent commits (freshness, S5.2); expired claims or claims referring to nonexistent workitems (removes them with a note); active cards older than 90 days never touched (warning); malformed frontmatter (tolerant parsing: reports, does not crash). Exit 3 if there are issues.

**Plan-mutation policy:** if the current branch is not in `integration_branches`: `warn` (default) → message on stderr, proceeds; `strict` → exit 2 `{"error":"policy","branch":"..."}`. Never applied to: tasks with `--from`, start/block/done transitions, read-only commands.

## S3. Bootstrap block (installed by init)

```markdown
<!-- atlas:begin -->
## ATLAS
- At session start run `atlas context`: its output is the current project state.
- Before working on a task: `atlas task start <id>` (if refused: pick a task from the ready list).
- When you finish a task: `atlas task done <id> --summary "one line on what changed"`.
- Made a non-obvious decision? `atlas card add --type decision "title" --hook "one-line summary"`.
- Discovered new work? `atlas task add "title" --from <current-task-id>`.
- Before ending the session: update task states and, if the goal changed, `.atlas/focus.md`.
- After merging any branch that touches `.atlas/`: run `atlas doctor` — a merge can bring back a stale, already-closed workitem (claims don't protect against this, only doctor catches it).
- Use `--json` on read commands. Never edit files under `.atlas/` by hand: use the CLI.
<!-- atlas:end -->
```

## S4. Seed brief (constant text printed by `atlas seed`)

Content (in English, for agents): instructions for inventorying TODO/docs/ADR/recent git log and triaging into the ATLAS model with the lossy rules from ANALYSIS.md §12.2: focus 5-10 lines about today; max ~15 workitems ONLY open and relevant ones; cards only for decisions still binding, ADRs referenced via evidence NEVER copied; history excluded (at most 1 "lessons" card + pointer); everything via CLI commands; work on a dedicated branch; close with `atlas doctor`; the human reviews and commits. Include the command examples.

## S5. Context compiler

**S5.1 Text format** (sections in this order, omitted if empty):
```
# ATLAS CONTEXT (<date>) [STALE: ledger older than last N commits]   ← tag only if stale
## FOCUS
<focus.md verbatim>
## NOW
- [a1b2] title (doing, branch feature/x) — evidence: p1, p2
- [c3d4] title (blocked on e5f6: reason)
## READY
- [f6a7] title
## RULES
- [k9m2] hook (decision)
## RECENT
- [b2c3] summary (2026-08-25)
- git: <last 5 commits oneline>
## GROUND
branch: feature/x · HEAD: abc1234 · worktree: dirty(3 files) · elsewhere: [a1b2 on feature/y]
## POINTERS
Detail: `atlas show <id>` · Full state: `atlas state` · History: `atlas log --grep <x>`
```
**S5.2 Freshness:** stale if the most recent mtime among the `.atlas/` files is earlier than the timestamp of the Nth most recent commit (N=5) AND the working tree has later commits. Exposed in GROUND and as a tag in the header.
**S5.3 Budget:** token estimate = len(runes)/4. If over `budget_tokens`, degrade in reverse priority order (priority: FOCUS > NOW > GROUND > READY > RULES > RECENT > POINTERS): first RECENT reduced to 3 lines then removed, then RULES reduced to `[id] first 60 chars`, then READY truncated with `… (+K more: atlas state)`. FOCUS and NOW never removed.
**S5.4 JSON:** `{"generated":ts,"stale":bool,"focus":"...","now":[{workitem}...],"ready":[...],"rules":[{"id","hook","type"}],"recent":[...],"ground":{"branch","head","dirty","elsewhere":[...]},"budget":{"limit":1500,"estimated":<n>}}`.
**S5.5 `context <id>`:** FOCUS + the full workitem (frontmatter+body) + cards whose id appears in the task's body/evidence or whose evidence intersects the task's paths + GROUND + POINTERS. Same budget.

## S6. gitx (subprocess)

Functions: `Root(dir)`, `CommonDir(dir)` (`git rev-parse --git-common-dir`, absolute path), `Branch(dir)`, `HeadShort(dir)`, `IsDirty(dir)` (+file count), `RecentCommits(dir,n)` (oneline), `CommitTimestamps(dir,n)`. Each function: runs `git -C dir ...`, error wrapping with stderr. If not in a git repo: git features degrade (context without GROUND git, freshness not computable) — NEVER crash.

## S7. Test strategy (TDD, binding)

- Mandatory order for every unit: write the tests (red) → implement → green → refactor. Phase commits must contain tests + implementation.
- Shared helper `internal/testutil`: `SetupRepo(t)` → t.TempDir + `git init -b main` + local user config + initial commit; `SetupWorktree(t, repo, branch)`. Git tests use ONLY temporary repos; no network; deterministic (injectable clock where needed: `Now func() time.Time` in claims/freshness constructors).
- CLI integration: tests that invoke the cobra commands via `Execute` with args and capture stdout/stderr/exit (no need to build the binary in tests).
- Golden tests for context rendering (text and JSON).
- Mandatory cases: ID collision; malformed frontmatter (tolerance); concurrent claim (2 goroutines doing O_EXCL on the same id → exactly one wins); expired claim reacquirable; steal; done without summary; policy warn vs strict; budget degradation (fixture over budget); idempotent init (double run → a single block); merge=union in .gitattributes; ready with blocked_by closed in the log; blocked_by cycles (doctor); repo without git.
- Coverage: `go test ./... -coverprofile=cover.out && go tool cover -func=cover.out | tail -1` ≥ 70% total. `go vet ./...` clean.

## S8. Execution phases (one per agent, sequential)

1. **F1 — scaffold + ledger:** verify toolchain (go, git; install go via brew if absent), `git init` (main), go.mod, .gitignore (atlas binary, cover.out), testutil, complete internal/ledger (S1) in TDD. Commit `feat(ledger): core data model with TDD`.
2. **F2 — gitx + claims:** S6 and claims (S1) in TDD, incl. worktree and concurrency tests. Commit `feat(gitx,claims): git wrapper and atomic claims`.
3. **F3 — state + contextc:** ready/freshness (S5.2) and compiler (S5) in TDD with golden tests. Commit `feat(context): state derivation and budgeted context compiler`.
4. **F4 — complete CLI:** all commands (S2), bootstrap (S3), seed (S4), policy, in TDD with integration tests. Commit `feat(cli): full command surface`.
5. **F5 — doctor + hardening:** complete doctor, filling coverage gaps up to ≥70%, `go vet`, README.md (installation, commands, file format), end-to-end smoke test in a temporary repo (init→seed→task add/start/done→context→doctor). Commit `feat(doctor): integrity checks; docs and coverage hardening`.
6. **F6 — spec management:** complete S9 in TDD (spec entity, commands, context/state/doctor integration, updated bootstrap and seed, README). Commit `feat(spec): living canonical specs linked to workitems`.

Rules for every phase: do not modify files from previous phases unless necessary; suite ALWAYS green at the end of the phase (`go test ./...`); report phase coverage.

## S9. Spec management (phase F6 — extends S1/S2/S3/S5)

Rationale: the user's workflow is spec-driven; workitem bodies are not enough for large intents (ANALYSIS §16.3, decided 2026-08-27). Model: **living canonical specs** — one per capability/area, updated in place; the history of deltas is git. NEVER accumulate per-feature specs. All user-facing strings in ENGLISH.

**S9.1 Data.** `.atlas/specs/<id>-<slug>.md`, same hex ID space (collision check extended to specs/):
```markdown
---
id: 3fa9
title: Workload execution retry semantics
status: draft        # draft | active | superseded
superseded_by: ""
created: 2026-08-27
evidence: []
---
Body = the specification (free markdown, living document).
```
Workitem: new optional field `spec: <id>`. `task add --spec <id>` validates that the spec exists and is not superseded: exit 2 `{"error":"spec_not_found"}` / `{"error":"spec_superseded","superseded_by":"..."}`.

**S9.2 Commands** (S2 conventions; plan-mutation policy on add/activate/update/supersede, never on list/show):
- `atlas spec add "title" [--body -|text] [--evidence p1,p2]` → creates draft, prints id.
- `atlas spec activate <id>` → draft→active (exit 2 if superseded or already active? no: idempotent on active, exit 2 only on superseded).
- `atlas spec update <id> [--title t] [--body -|text] [--evidence ...]` → updates in place; refused on superseded (exit 2). `--body -` reads stdin.
- `atlas spec supersede <old> <new>` → old→superseded + superseded_by; event in log.jsonl with `kind:"spec"`.
- `atlas spec list [--json]` → id, title, status, number of open workitems linked.
- `atlas show <id>` extended to specs/ (searches in work/, cards/, specs/).

**S9.3 Context.**
- General brief: new `## SPECS` section between RULES and RECENT, one line per draft/active spec: `- [id] title (status, N open tasks)`. Updated budget priority: FOCUS > NOW > GROUND > READY > RULES > SPECS > RECENT > POINTERS; SPECS degradation (after RECENT, before RULES): lines reduced to `- [id] title`.
- `atlas context <task-id>`: if the task has `spec:`, after the task body a `## SPEC [id] title` section with the full body of the spec. Over budget, the spec body degrades first: truncated with a final line `… (full spec: atlas show <id>)`.
- `atlas state`: full specs section.

**S9.4 Doctor.** New checks: a workitem's `spec:` → nonexistent spec (ERROR) or superseded (WARNING); orphaned `superseded_by` among specs (ERROR); draft specs older than 30 days (WARNING); malformed frontmatter in specs/ (ERROR); duplicate ids extended to specs/.

**S9.5 Bootstrap (S3).** Add ONE line to the block (idempotent init propagates it to repos on re-run):
`- Working from a spec? Link tasks with \`atlas task add --spec <id>\`; \`atlas context <task-id>\` will include it.`

**S9.6 Seed (S4).** Add a SPECS section to the brief: create a spec only for capability with a genuinely living intent (cap ~5 at seed); reference existing spec documents via evidence, never copy them.

**S9.7 Mandatory tests** (S7 applies in full): roundtrip save/load spec; draft→active→superseded lifecycle; `task add --spec` validation (nonexistent/superseded); updated goldens for the SPECS section in the general brief; target-mode goldens with spec included and with budget degradation of the body; doctor for each of the new checks; policy warn/strict on spec commands; cross-directory id collision (work/cards/specs); `spec update --body -` from stdin; updated README.

**S9.8 Spec ↔ decisions (ADR) — traceability constraint (added 2026-08-27).**
A spec must follow a decision. Model:
- Spec frontmatter: new field `decisions: []` — each entry is either the id of an ATLAS card of type decision, or a path in the repo to an existing ADR (e.g. `docs/adr/0034-enrichment-stage.md`).
- `spec add --decision <id-or-path>` (CSV/repeatable) and `spec update --decision ...` (replaces the list).
- **`spec activate` requires at least one decision**: exit 2 `{"error":"spec_without_decision"}` otherwise. Drafts can be created without one (sketch first, anchor to activate).
- Validation at add/update/activate: card id → must exist and be `type: decision` (`{"error":"decision_not_found","id":"..."}`); at activate a superseded decision → exit 2 `{"error":"decision_superseded","id":"...","superseded_by":"..."}`; path → must exist on disk (`{"error":"decision_path_not_found","path":"..."}`).
- Doctor: active spec with empty decisions (ERROR — invariant broken by manual edit); reference to nonexistent card (ERROR); reference to superseded decision (WARNING: "spec may need revision"); nonexistent path (ERROR).
- Target mode context: under the `## SPEC [id] title` header, a line `Decisions: k9m2, docs/adr/0034-enrichment-stage.md` (not degradable: it's 1 line).
- `spec list`: shows the linked decisions.
- Updated seed brief and README: create the decision card first, then the spec that references it.

## S10. Graph + spec scaffold (phase F7 — useful legacy from aiops-ai-spec, decided 2026-08-27)

The only two porting items approved by the aiops-ai-spec-manager analysis. User-facing strings in ENGLISH.

**S10.1 `atlas graph [--mermaid] [--json]`** — read-only, NEVER included in the context brief (opt-in command for humans). Renders the `blocked_by` DAG of ACTIVE workitems (todo/doing/blocked; done ones no longer exist in work/).
- Default (text): topological levels — level 0 = no active blockers, level N = all blockers at levels <N. Format:
```
# ATLAS GRAPH
Level 0 (unblocked, parallelizable):
- [a1b2] title (doing)
Level 1:
- [c3d4] title (todo, blocked by a1b2)
```
- Blockers referring to closed/nonexistent ids do not block (same semantics as READY).
- Cycles: the nodes involved end up in a final `Cycle (unresolvable):` group with a warning on stderr pointing to `atlas doctor`; exit 0 (graph shows, doctor judges).
- `--mermaid`: `flowchart TD`, nodes `id["id: title (status)"]`, edges `blocker --> blocked`.
- `--json`: `{"levels":[[{"id","title","status","blocked_by":[...]}]],"cycles":[...]}`.
- No DOT/Graphviz.

**S10.2 Default scaffold for `spec add`** — when `--body` is omitted, the body of the created spec is this template (rewritten for spec-as-living-document, NOT the per-feature template from aiops):
```markdown
## Goal
<what this capability must achieve and for whom>

## Constraints
<hard limits, invariants, and the decisions this spec follows>

## Interfaces
<contracts, commands, data shapes exposed or consumed>

## Open questions
<unresolved points — resolve these before activating the spec>
```
With an explicit `--body` the template is not used. No automatic body sync: it remains free editable text.

**S10.3 Mandatory tests (S7 applies):** multi-level topological levels; closed blockers do not block; cycle → Cycle group + stderr warning + exit 0; golden for text and mermaid; `--json` shape; scaffold present without `--body` and absent with `--body`; activating a spec with only the scaffold remains subject to S9.8 (decisions required). Updated README (graph in the command table, scaffold mentioned in spec add).
