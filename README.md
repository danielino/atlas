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

## Why ATLAS? (What It Is Not, Why It Exists, and Why It Matters)

### What ATLAS Is NOT

**It is not yet another general memory/retrieval system.** ATLAS doesn't extract arbitrary facts,
encode them as vectors, or inject a ranked subset into prompts. It doesn't learn what information
is relevant — you tell it, explicitly, by what you put in the ledger. **It has no embeddings, no
daemon, no knowledge graph, no probabilistic retrieval.** If you want those, Mem0 or Ruflo already
exist.

**It is not a spec-driven workflow engine.** ATLAS doesn't enforce spec→plan→tasks→implement
ceremonies. You can use it with that workflow or without it. Specs are optional. There's no
review board, approval gate, or entanglement with your branching strategy.

**It is not a full issue tracker.** It has no assignees, sprints, priorities, boards, or milestone
forecasting. If your team needs those, ATLAS cannot be your primary system — it can coexist with one
(referencing it via evidence links) but doesn't replace it.

**It is not a solution to staleness.** ATLAS detects staleness — it flags when project state hasn't
been updated relative to git — but it can't prevent it. An agent or human must close the loop by
actually updating the ledger. The tool can make that cheap (one command with a summary), but not
automatic. If the mental model breaks, ATLAS breaks with it.

### The Real Problem ATLAS Solves

At the start of every coding session, a coding agent pays a **reconstruction tax**:

1. Read 2,000-line TODO.md files to extract 50 lines of relevance.
2. Search through git log to find when decisions were made.
3. Reconstruct project phase from ADRs, old specs, and implicit conventions.
4. Re-read code to confirm what the README actually means.
5. Load all of that *again* on the next request, because the agent can't hold context across compaction.

This tax **grows with project age, not project size**. A 5-year-old repo with steady churn is worst.
Measured across 8 sessions in production codebases: first action taken after 13 requests (median),
with ~66.6k tokens of context already spent reconstructing state — and that state was re-read an
average of 1.4M tokens across the session due to compaction. **Every token of reconstruction is
paid twice: once at read, once at cache-reload.**

The gap: **no existing tool answers "what is the current state of this project?" in O(1) time**. GitHub
issues answer "what work is open?" (incomplete — no phase, no decisions, no intent). Beads answers
"what work is ready?" (same). Spec Kit/OpenSpec answer "what are we building?" (yes, but behind
ceremony). ADRs answer "why?" (yes, if you dig; usually buried in history). CLAUDE.md answers "what
are the rules?" (yes — but rules are not state). **Each tool solves one piece; none gives you the brief.**

### What ATLAS Actually Does (and Its Limits)

ATLAS provides **one deterministic command** — `atlas context` — that answers that one question:
*"What do I need to know to continue from where we left off?"* The answer is:

- **FOCUS** (3–5 lines): What we're building and why, right now.
- **NOW** (1–3 items): What the agent is currently working on, what's blocked and why.
- **READY** (3–8 items): What's unblocked and waiting, in order.
- **RULES** (5–10 hooks): Decisions that still constrain choices.
- **RECENT** (3–5 lines): Summary of work closed in the last 7 days — bridge between sessions.
- **GROUND** (3 lines): What branch, what state, whether the ledger is stale.
- **POINTERS** (2 lines): How to dig deeper.

All of that is **~1,500 tokens** in the common case, vs. the ~50k tokens the agent was burning to reconstruct it.
**The ledger is small enough to fit in cache**, so it can be included in every request. The brief is made of
**pointers and summaries, never full dumps** — the agent reads the code just-in-time, which it's good at.

**But here's the honest part:** ATLAS solves a *multiplier problem*, not the problem of coding itself.
If your agent is making bad decisions, ATLAS gives it better context to make them *faster and cheaper*,
but it doesn't make the decisions better. If your architecture is tangled or your team's mental model
is fractured, ATLAS will make that visibility clearer and more painful, not fix it. **It is an amplifier,
not a solution.**

And it only works if you update it. The moment the ledger becomes stale and agents stop trusting it,
you're left with a small frozen TODO that nobody reads — which is worse than a large living one. ATLAS'
main failure mode is **behavioral, not technical**: keeping the ledger fresh costs less than the tax
of not having it, but requires discipline. `atlas doctor` detects the staleness; only humans and agents
can fix it.

### When ATLAS Matters (And When It Doesn't)

The reconstruction tax is real and measured. But its cost scales unevenly:

**You don't need ATLAS if:**
- Your team is small and synchronous (you talk every day; state is implicit).
- Your project is under 6–12 months old and hasn't accumulated 1,000+ commits yet.
- You spawn agents episodically for isolated tasks, not continuously for ongoing work.
- You have a single focused work stream (one feature, one phase) and rarely context-switch.
- Your team doesn't use agents at all (ATLAS is designed for agent + human handoff).

**ATLAS starts paying for itself when:**
- Your codebase is 1–2+ years old with steady churn and historical baggage.
- Multiple agents work in parallel on different branches, and you need to know who owns what.
- You have 5+ active decisions/constraints that shape new work.
- Agents make decisions that older team members might not immediately recognize (onboarding cost).
- You measure per-session cost in dollars, not hours — the 10–25% reduction in context burn is material.

For projects in the second category, `atlas context` replaces 30–60 minutes of manual state assembly with
~1 minute and ~1,500 tokens. The tool is minimal enough — no server, no daemon, ~500 lines of CLI code,
plain files in git — that the friction of trying it is one-line `atlas init`.

**The honest take:** If you're not feeling the reconstruction tax yet, you won't feel ATLAS either. The
ones that need it know it from the moment they try `atlas context` on their real codebase and see their
reconstruction logic compressed into 1 minute and a brief that actually fits in the cache window.

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
