# ATLAS

ATLAS is a lossy, file-based project ledger for coding agents. It keeps a
small, curated, versioned representation of a project's *current state* —
focus, open workitems, standing decisions — separate from its full history
(git log, closed items, superseded decisions), and gives agents and humans a
single command, `atlas context`, that compiles a minimal-but-sufficient brief
from that state. The goal is to remove the "reconstruction tax" a coding
agent otherwise pays at the start of every session: reading old TODOs,
accumulated specs, ADRs and commit history just to figure out where the
project stands.

ATLAS is a project-state ledger and a context compiler, not a spec-driven
workflow, a memory/retrieval system, or an issue tracker. It has no server,
no daemon, and no required LLM calls: every command is a local, deterministic
CLI operation backed by plain files under `.atlas/` and the system `git`
binary. It is agent-neutral (works with anything that can run a shell command
or read a file) and adoptable incrementally — `atlas init` on an existing
repository does not require restructuring anything.

## Install

Requires Go and the `git` CLI on `PATH`. No other runtime dependency.

```sh
go build -o atlas ./cmd/atlas
```

Put the resulting `atlas` binary anywhere on your `PATH`.

## Quick start

```sh
cd my-project            # inside a git repository
atlas init                # creates .atlas/, AGENTS.md bootstrap block, .gitattributes
atlas task add "Fix the widget"
atlas task start <id>
atlas context              # what an agent should read at the start of a session
atlas task done <id> --summary "widget fixed"
atlas doctor                # integrity check before committing
```

Working from a spec (a spec must follow a decision, never invented from nothing):

```sh
atlas card add --type decision "Adopt bounded-retry workload model" \
  --hook "Retries are capped, never infinite"
atlas spec add "Workload execution retry semantics" --decision <decision-id>
atlas spec activate <spec-id>              # requires the decision link above
atlas task add "Implement retry cap" --spec <spec-id>
atlas context <task-id>                    # includes the linked spec in full
```

## Why ATLAS

ATLAS is not a memory/retrieval system: no embeddings, no knowledge graph, no
automatic fact extraction — relevance is whatever you put in the ledger. Not a
spec-driven workflow: specs are optional, attached to workitems, never a gate.
Not an issue tracker: no assignees, sprints or boards; it coexists with one by
reference. Not documentation: it indexes `docs/` and ADRs (docs/ANALYSIS.md §4).

A coding agent starts every session as a fresh context, so it rebuilds
project state by re-reading TODO files, git log, old specs and conventions —
then pays that reading again after each compaction. Across 8 measured Claude
Code sessions (5 with ≥30 requests): median first productive action at request
13, range 1–27, ~66.6k tokens already spent reconstructing state an earlier
session had established (docs/ANALYSIS.md §17.7).

No tool answers "what is the state of this project?" in O(1). Issues and Beads
answer what work is open or ready, Spec Kit what we are building, ADRs why,
CLAUDE.md what the rules are — each one piece, none the brief (docs/ANALYSIS.md §3).

`atlas context` returns that brief: eight fixed sections — FOCUS, NOW, READY,
RULES, SPECS, RECENT, GROUND, POINTERS — one line per item, summaries and
pointers, never file dumps, so the agent reads code just-in-time. Output is
capped by `budget_tokens` (default 1500); over budget it degrades RECENT,
SPECS, RULES, then READY, never FOCUS or NOW. `atlas context <task-id>`
additionally includes that task's linked spec in full, if it has one.

The main failure mode is behavioral. If nobody writes back, the ledger goes
stale. ATLAS detects staleness against git and always declares it, and
deliberately does not infer state from diffs: wrong state presented as fact is
worse than declared staleness (docs/ANALYSIS.md §13.1) — a human or agent closes
the loop.

Checkable on your own repo — if none of these holds, you are not paying the tax:

- [ ] `TODO.md` long enough that reading it costs more than skimming it
- [ ] More open workitems than you can list from memory
- [ ] More than one agent or branch in flight, with ownership not obvious
- [ ] Standing decisions that constrain new work but live only in git history

Ecosystem analysis and measurement protocol: docs/ANALYSIS.md §3, §4, §13.1, §17.7.

---

## Command reference

Every read command and every semantic refusal supports `--json`. All
commands except `init` locate the project root by walking up from the
current directory looking for a `.atlas/` directory.

| Command | Purpose |
|---|---|
| `atlas init` | Create `.atlas/` (`focus.md`, `config.toml`, `work/`, `cards/`, `specs/`), add `.atlas/log.jsonl merge=union` to `.gitattributes`, and install/update the bootstrap block in `AGENTS.md` (created if missing) and `CLAUDE.md` (only if it already exists). Idempotent. |
| `atlas seed [--json]` | Print a fixed curation brief for bootstrapping an existing (brownfield) repository into the ATLAS model. Prints to stdout; makes no LLM call itself. |
| `atlas context [id] [--json]` | Compile the budgeted context brief (FOCUS/NOW/READY/RULES/SPECS/RECENT/GROUND/POINTERS). With `id`, centers the brief on that workitem plus related cards and (if the workitem has one) its full linked spec. |
| `atlas state [--json]` | Print the full, unbudgeted project state: focus, every workitem by status, active cards, draft/active specs with open-task counts, ground (git + claims elsewhere). |
| `atlas task add <title> [--body -\|text] [--blocked-by id,id] [--from id] [--evidence p1,p2] [--spec id]` | Create a new `todo` workitem. Subject to plan-mutation policy unless `--from` is given (discovered work is always allowed). `--spec` links it to a spec; refused (exit 2) if the spec doesn't exist or is superseded. |
| `atlas task start <id> [--steal]` | Claim a workitem and mark it `doing`. Refused (exit 2) if already claimed by another branch/session, unless `--steal`. |
| `atlas task block <id> [--on id] [--reason text]` | Mark a workitem `blocked`. Only the owning branch (or nobody) may do this. |
| `atlas task done <id> --summary text` | Close a workitem: `--summary` is required and non-empty, appends to `log.jsonl`, removes the file from `work/`, releases the claim. |
| `atlas card add --type decision\|knowledge <title> [--hook text] [--body text] [--evidence p1,p2]` | Create a decision/knowledge card. `--hook` defaults to the title. Subject to plan-mutation policy. |
| `atlas card supersede <old-id> <new-id>` | Mark an old card `superseded`, pointing at the new one; appends to `log.jsonl`. Subject to plan-mutation policy. |
| `atlas spec add <title> [--body -\|text] [--evidence p1,p2] [--decision id-or-path,...]` | Create a draft spec. Subject to plan-mutation policy. Omit `--body` to start from a default Goal/Constraints/Interfaces/Open questions scaffold instead of an empty document. |
| `atlas spec activate <id>` | Draft → active. Idempotent if already active. Requires at least one `--decision` entry (exit 2 `spec_without_decision` otherwise); refused (exit 2) if superseded, or if a linked decision card is superseded. Subject to plan-mutation policy. |
| `atlas spec update <id> [--title t] [--body -\|text] [--evidence ...] [--decision ...]` | Update a spec in place (`--decision` replaces the list). `--body -` reads from stdin. Refused (exit 2) on a superseded spec. Subject to plan-mutation policy. |
| `atlas spec supersede <old-id> <new-id>` | Mark an old spec `superseded`, pointing at the new one; appends a `kind:"spec"` event to `log.jsonl`. Subject to plan-mutation policy. |
| `atlas spec list [--json]` | List draft/active specs with their linked decisions and open-task counts. Read-only. |
| `atlas show <id> [--json]` | Print a workitem, card, or spec in full (frontmatter + body). |
| `atlas log [--grep pattern] [--json]` | Query the closed-item log (`log.jsonl`); never included in context. |
| `atlas doctor [--json]` | Run integrity checks over the ledger; see below. |
| `atlas graph [--mermaid\|--json]` | Read-only view of the `blocked_by` dependency graph over active workitems, as topological levels (default text), a mermaid `flowchart TD`, or JSON. Never included in `atlas context`. Unresolvable cycles are shown in a trailing group with a stderr warning; exit is always 0 (judging a cycle an error is `atlas doctor`'s job). |

## `.atlas/` file layout

```
.atlas/
  focus.md            # plain markdown, 3-10 lines, no frontmatter
  work/<id>-<slug>.md  # one file per active workitem
  cards/<id>-<slug>.md # one file per decision/knowledge card
  specs/<id>-<slug>.md # one file per living canonical spec
  log.jsonl            # append-only record of closed workitems, superseded cards and specs
  config.toml          # optional; see below for defaults
```

Ids are 4 lowercase hex characters (5 on collision after 20 retries),
generated randomly and checked against `work/`, `cards/`, `specs/` and
`log.jsonl`.

A workitem file:

```markdown
---
id: a1b2
title: Fix container reconcile retry
status: todo            # todo | doing | blocked | done
created: 2026-08-27
blocked_by: [c3d4]       # optional
discovered_from: e5f6    # optional
branch: feature/retry    # set by `task start`
evidence:                # optional
  - packages/core/pipeline/reconcile.py:120-180
summary: ""              # required (non-empty) at done
reason: ""                # optional, why it's blocked
spec: 3fa9                # optional, id of the spec this workitem implements
---
Free markdown body (intent, notes).
```

A card file:

```markdown
---
id: k9m2
type: decision           # decision | knowledge
title: Use O_EXCL for claims
status: active            # active | superseded
superseded_by: ""          # id, for superseded decisions
hook: "Claim = file O_EXCL in $GIT_COMMON_DIR, never a mutex"  # required, one-line index entry
created: 2026-08-27
evidence: []
---
Body (for decisions: context/decision/consequences, MADR-compatible).
```

A spec file — a living canonical document for one capability/area, updated
in place (never one file per feature; the history of changes is git history):

```markdown
---
id: 3fa9
title: Workload execution retry semantics
status: draft             # draft | active | superseded
superseded_by: ""          # id, for superseded specs
created: 2026-08-27
evidence: []
decisions: []              # decision-card ids and/or ADR paths this spec follows;
                            # required (non-empty) before `spec activate`
---
Body = the specification itself (free markdown, living document).
```

A spec must follow a decision (never invented from nothing): create the
governing `atlas card add --type decision` first, then `atlas spec add
--decision <that-id>`, so `--decision` can also name an existing ADR path
(e.g. `docs/adr/0034-enrichment-stage.md`) instead of a card id.

Claims (workitem locks) live outside the versioned repo, under
`<git-common-dir>/atlas/claims/<id>.json`, so they are per-checkout, never
committed, and shared correctly across git worktrees.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | OK |
| 1 | I/O or parse error (including: not an ATLAS project) |
| 2 | Semantic refusal (already claimed, policy=strict, missing/blank `--summary`, unknown id, invalid transition, not the owning branch, spec not found/superseded, spec without a linked decision, decision not found/superseded/path missing) |
| 3 | `atlas doctor` only: at least one integrity error was found (warnings alone do not trigger this) |

Every semantic refusal supports `--json`, producing `{"error":"<code>", ...}`.

## The bootstrap block

`atlas init` appends (or updates) a block delimited by
`<!-- atlas:begin -->` / `<!-- atlas:end -->` in `AGENTS.md` (created if
missing) and in `CLAUDE.md` (only if it already exists). Re-running `init`
replaces the block in place and never touches text outside the markers. The
block tells an agent to run `atlas context` at the start of a session, use
`atlas task start`/`done` around work, record non-obvious decisions with
`atlas card add`, log discovered work with `atlas task add --from`, link
tasks to a spec with `atlas task add --spec <id>` (so `atlas context
<task-id>` includes it), and never hand-edit files under `.atlas/`.

## `config.toml` reference

All keys are optional; the file itself may be entirely absent. Values shown
are the built-in defaults:

```toml
[context]
budget_tokens = 1500     # context is degraded (RECENT, then SPECS, then RULES, then READY) beyond this
recent_days = 7           # window for the RECENT section's closed items

[policy]
plan_mutations = "warn"   # "warn" | "strict" — see below
integration_branches = ["main", "develop"]

[claims]
ttl_hours = 24            # a claim older than this is treated as expired
```

**Plan-mutation policy**: `task add` (without `--from`), `card add`,
`card supersede`, and every `spec add`/`activate`/`update`/`supersede` are
"plan mutations". If the current branch is not in
`integration_branches`, `warn` (default) prints a warning to stderr and
proceeds; `strict` refuses with exit 2. Never applied when there is no git
branch to evaluate, to task lifecycle transitions (`start`/`block`/`done`),
to `task add --from` (discovered work is always allowed), or to any
read-only command.

## `atlas doctor`

Runs the following checks and reports each as an error or a warning
(`--json` yields `{"errors":[...],"warnings":[...],"fixed":[...]}`, each
entry `{"code","message"}`; human output groups the same information under
`## ERRORS` / `## WARNINGS` / `## FIXED`):

- **Errors** (exit 3 if any is found): orphaned `blocked_by` / `discovered_from`
  / `superseded_by` references (pointing at an id that exists neither in the
  active ledger nor in `log.jsonl`, checked for specs too); cycles in the
  `blocked_by` graph; workitems with status `done` still present in `work/`;
  `log.jsonl` entries of kind `task` with no summary; malformed frontmatter in
  `work/`, `cards/` or `specs/`; duplicate ids across `work/`, `cards/` and
  `specs/`; a workitem's `spec:` pointing at a nonexistent spec
  (`spec_not_found`); an active spec with an empty `decisions:` list
  (`spec_without_decision` — only reachable by hand-editing, since `spec
  activate` itself refuses this); a spec's `decisions:` entry naming a
  nonexistent or non-decision card (`decision_not_found`) or a nonexistent
  path (`decision_path_not_found`).
- **Warnings**: a stale ledger (no file under `.atlas/` touched since before
  the last few commits — see freshness below); active cards older than 90
  days; draft specs older than 30 days (`old_draft_spec`); a workitem's
  `spec:` pointing at a spec that has since been superseded
  (`spec_superseded`); a spec's `decisions:` entry naming a decision card
  that has since been superseded (`decision_superseded`, "spec may need
  revision").
- **Fixed** (applied automatically, then reported): expired claims, and
  claims referencing a workitem id that no longer exists — both are removed
  from the claims store.

A malformed file is reported and skipped, never a crash: every other check
still runs against the rest of the ledger.

Freshness (used both by `doctor` and shown in `context`'s GROUND section): the
ledger is considered stale if the newest modification time under `.atlas/`
predates the timestamp of the 5th most recent commit, and the working tree
has at least that many commits — i.e. code has moved on since the ledger was
last touched.
