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

## Why ATLAS

ATLAS is not a memory/retrieval system: no embeddings, no knowledge graph, no
automatic fact extraction — relevance is whatever you put in the ledger. Not a
spec-driven workflow: specs are optional, attached to workitems, never a gate.
Not an issue tracker: no assignees, sprints or boards; it coexists with one by
reference. Not documentation: it indexes `docs/` and ADRs (ANALYSIS.md §4).

A coding agent starts every session as a fresh context, so it rebuilds
project state by re-reading TODO files, git log, old specs and conventions —
then pays that reading again after each compaction. Across 8 measured Claude
Code sessions (5 with ≥30 requests): median first productive action at request
13, range 1–27, ~66.6k tokens already spent reconstructing state an earlier
session had established (ANALYSIS.md §17.7).

No tool answers "what is the state of this project?" in O(1). Issues and Beads
answer what work is open or ready, Spec Kit what we are building, ADRs why,
CLAUDE.md what the rules are — each one piece, none the brief (ANALYSIS.md §3).

`atlas context` returns that brief: seven fixed sections — FOCUS, NOW, READY,
RULES, RECENT, GROUND, POINTERS — one line per item, summaries and pointers,
never file dumps, so the agent reads code just-in-time. Output is capped by
`budget_tokens` (default 1500); over budget it degrades RECENT, RULES, then
READY, never FOCUS or NOW.

The main failure mode is behavioral. If nobody writes back, the ledger goes
stale. ATLAS detects staleness against git and always declares it, and
deliberately does not infer state from diffs: wrong state presented as fact is
worse than declared staleness (ANALYSIS.md §13.1) — a human or agent closes
the loop.

Checkable on your own repo — if none of these holds, you are not paying the tax:

- [ ] `TODO.md` long enough that reading it costs more than skimming it
- [ ] More open workitems than you can list from memory
- [ ] More than one agent or branch in flight, with ownership not obvious
- [ ] Standing decisions that constrain new work but live only in git history

Ecosystem analysis and measurement protocol: ANALYSIS.md §3, §4, §13.1, §17.7.

---

## Command reference

Every read command and every semantic refusal supports `--json`. All
commands except `init` locate the project root by walking up from the
current directory looking for a `.atlas/` directory.

| Command | Purpose |
|---|---|
| `atlas init` | Create `.atlas/` (`focus.md`, `config.toml`, `work/`, `cards/`), add `.atlas/log.jsonl merge=union` to `.gitattributes`, and install/update the bootstrap block in `AGENTS.md` (created if missing) and `CLAUDE.md` (only if it already exists). Idempotent. |
| `atlas seed [--json]` | Print a fixed curation brief for bootstrapping an existing (brownfield) repository into the ATLAS model. Prints to stdout; makes no LLM call itself. |
| `atlas context [id] [--json]` | Compile the budgeted context brief (FOCUS/NOW/READY/RULES/RECENT/GROUND/POINTERS). With `id`, centers the brief on that workitem plus related cards. |
| `atlas state [--json]` | Print the full, unbudgeted project state: focus, every workitem by status, active cards, ground (git + claims elsewhere). |
| `atlas task add <title> [--body -\|text] [--blocked-by id,id] [--from id] [--evidence p1,p2]` | Create a new `todo` workitem. Subject to plan-mutation policy unless `--from` is given (discovered work is always allowed). |
| `atlas task start <id> [--steal]` | Claim a workitem and mark it `doing`. Refused (exit 2) if already claimed by another branch/session, unless `--steal`. |
| `atlas task block <id> [--on id] [--reason text]` | Mark a workitem `blocked`. Only the owning branch (or nobody) may do this. |
| `atlas task done <id> --summary text` | Close a workitem: `--summary` is required and non-empty, appends to `log.jsonl`, removes the file from `work/`, releases the claim. |
| `atlas card add --type decision\|knowledge <title> [--hook text] [--body text] [--evidence p1,p2]` | Create a decision/knowledge card. `--hook` defaults to the title. Subject to plan-mutation policy. |
| `atlas card supersede <old-id> <new-id>` | Mark an old card `superseded`, pointing at the new one; appends to `log.jsonl`. Subject to plan-mutation policy. |
| `atlas show <id> [--json]` | Print a workitem or card in full (frontmatter + body). |
| `atlas log [--grep pattern] [--json]` | Query the closed-item log (`log.jsonl`); never included in context. |
| `atlas doctor [--json]` | Run integrity checks over the ledger; see below. |

## `.atlas/` file layout

```
.atlas/
  focus.md            # plain markdown, 3-10 lines, no frontmatter
  work/<id>-<slug>.md  # one file per active workitem
  cards/<id>-<slug>.md # one file per decision/knowledge card
  log.jsonl            # append-only record of closed workitems and superseded cards
  config.toml          # optional; see below for defaults
```

Ids are 4 lowercase hex characters (5 on collision after 20 retries),
generated randomly and checked against `work/`, `cards/` and `log.jsonl`.

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
---
Free markdown body (intent, notes, spec).
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

Claims (workitem locks) live outside the versioned repo, under
`<git-common-dir>/atlas/claims/<id>.json`, so they are per-checkout, never
committed, and shared correctly across git worktrees.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | OK |
| 1 | I/O or parse error (including: not an ATLAS project) |
| 2 | Semantic refusal (already claimed, policy=strict, missing/blank `--summary`, unknown id, invalid transition, not the owning branch) |
| 3 | `atlas doctor` only: at least one integrity error was found (warnings alone do not trigger this) |

Every semantic refusal supports `--json`, producing `{"error":"<code>", ...}`.

## The bootstrap block

`atlas init` appends (or updates) a block delimited by
`<!-- atlas:begin -->` / `<!-- atlas:end -->` in `AGENTS.md` (created if
missing) and in `CLAUDE.md` (only if it already exists). Re-running `init`
replaces the block in place and never touches text outside the markers. The
block tells an agent to run `atlas context` at the start of a session, use
`atlas task start`/`done` around work, record non-obvious decisions with
`atlas card add`, log discovered work with `atlas task add --from`, and never
hand-edit files under `.atlas/`.

## `config.toml` reference

All keys are optional; the file itself may be entirely absent. Values shown
are the built-in defaults:

```toml
[context]
budget_tokens = 1500     # context is degraded (RECENT, then RULES, then READY) beyond this
recent_days = 7           # window for the RECENT section's closed items

[policy]
plan_mutations = "warn"   # "warn" | "strict" — see below
integration_branches = ["main", "develop"]

[claims]
ttl_hours = 24            # a claim older than this is treated as expired
```

**Plan-mutation policy**: `task add` (without `--from`), `card add` and
`card supersede` are "plan mutations". If the current branch is not in
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
  active ledger nor in `log.jsonl`); cycles in the `blocked_by` graph;
  workitems with status `done` still present in `work/`; `log.jsonl` entries
  of kind `task` with no summary; malformed frontmatter in `work/` or
  `cards/`; duplicate ids across `work/` and `cards/`.
- **Warnings**: a stale ledger (no file under `.atlas/` touched since before
  the last few commits — see freshness below); active cards older than 90
  days.
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
