# ATLAS — Discovery and Architecture Analysis

**Role:** Principal Software Architect (discovery phase — no implementation)
**Date:** 2026-08-27
**Status:** historical discovery/rationale record. The decisions explicitly marked as promoted/resolved below are final and now live as ADRs in `docs/adr/`; everything else is analysis and rationale, not a live contract — when in doubt, `docs/SPEC.md` is the binding implementation spec.

---

## 1. Executive summary

The problem ATLAS must solve is **not** a lack of documentation, specs, or memory for agents. The ecosystem is saturated with those. The problem is that **no existing tool maintains a compact, reliable, low-context-cost representation of a project's current state**, such that a coding agent can resume work without reconstructing history.

Problem statement:

> At the start of every new session, an agent pays a "reconstruction tax": it must read historical TODOs, accumulated specs, ADRs, logs and code to infer *where we are* and *what comes next*. This cost grows with the age of the project, not with the size of the work to be done.

ATLAS's thesis: separate **current state** (small, curated, authoritative for intent) from **history** (append-only, never loaded by default), and offer a command that compiles a **minimum sufficient brief** made mostly of *pointers*, not content.

Three key conclusions from the research, stated up front:

1. **The gap really exists.** Spec Kit and OpenSpec manage *change* (feature → artifacts → archive), not the *current state of work*. Memory systems (Ruflo/claude-flow, Mem0, Zep) accumulate context instead of reducing it. Conventions (CLAUDE.md/AGENTS.md) cover *stable rules*, not dynamic state.
2. **The closest competitor is not an SDD tool but Beads** (a git-native issue tracker for agents, by Steve Yegge): it already solves the task graph, ready-work detection and closed-task compaction. ATLAS differentiates itself at the layer Beads doesn't have — project state + decisions + context compilation. **Product decision (2026-08-27): ATLAS is standalone; no wrapper around and no dependency on Beads.** See [ADR 0001](adr/0001-standalone-not-built-on-beads.md). Beads remains prior art from which design lessons are reused (hash ids, ready-detection, compaction on close).
3. **The main risk is not technical but behavioral:** any curated state goes stale if agents and humans don't update it. The architecture must make updating cheaper than not maintaining it, and must be able to *detect* staleness instead of pretending it doesn't exist.

---

## 2. Pain point analysis

### A. Developer pain (conceptually validated)

| Pain point | Real? | Worth solving in ATLAS? |
|---|---|---|
| Distinguishing current state from history | **Yes, it's the central pain.** The 2,000-line TODO.md is the canonical symptom: a log disguised as state. | **Yes — it's the heart of the product.** |
| Remembering the "why" behind decisions | Yes, but already well served by ADRs *as a format*; the failure is procedural (nobody reads/updates them). | Partially: ATLAS must *index* decisions into the context, not reinvent the ADR. |
| Stale TODOs / task drift | Yes. Stems from the absence of a lifecycle: tasks have no "closed and compacted" state. | Yes, with an explicit lifecycle and compaction. |
| Conflicting / duplicate specs | Yes in SDD systems (verified: OpenSpec issues #678, #1387). Caused by the "spec per feature" model with no canonical layer. | Indirectly: ATLAS must not accumulate spec-per-feature. |
| Outdated, misleading ADRs | Yes (ADR literature: "a 2021 ADR read literally is actively misleading"). | Yes, but with a lightweight mechanism: `superseded` status + exclusion from the default context. |
| Finding the current work | Yes. Beads calls this the "50 First Dates" problem. | Yes. |
| Duplicated documentation | Yes, but it's a general documentation-hygiene problem; ATLAS can't solve it for the whole repo. | Not as a direct goal; yes as a consequence (a single home for state). |

### B. Agent pain (causes of excessive context consumption)

Ordered by estimated impact:

1. **State reconstruction from historical logs** — reading 2,000 lines to extract 50 relevant ones. Poor signal-to-noise ratio; worsens with project age.
2. **Context rot** — verified (Chroma research, 18 frontier models): performance degrades non-uniformly and in steps as token count grows, even on trivial tasks. Every irrelevant token isn't just an economic cost: it reduces reliability.
3. **Retrieval that is too broad** — memory systems (Ruflo: AgentDB with vectors+graph, ~210 MCP tools) inject more than needed; the tool surface itself consumes context. Confirms the initial intuition: more memory ≠ better.
4. **Repeated re-reading** — the agent rediscovers the repo's structure and conventions every session. Partly mitigated already by CLAUDE.md/AGENTS.md (stable rules), not by dynamic state.
5. **Session discontinuity** — automatic compaction loses initial instructions; the emerging community practice is a manual "handoff brief" at end of session. ATLAS can standardize exactly this.
6. **Duplicated/stale information** — specs that repeat the code, TODOs that contradict the real state. Worse than absence: the agent doesn't know whom to trust.

**Architectural observation:** points 1, 5, 6 are solvable with a *small, curated state ledger*; points 2, 3 require the compiled context to be made of **pointers + minimal summaries**, never dumps; point 4 is already solved by the ecosystem and ATLAS must not duplicate it.

### C. Information lifecycle (synthesis)

| Type | Created by | Becomes stale when | Authoritative? | In default context? | Provenance needed? |
|---|---|---|---|---|---|
| Code | dev + agent | never (it's the truth) | **Yes — fact** | No (read just-in-time) | no (it's the source) |
| Git history | automatic | never (append-only) | Yes — for events | No (queried on demand) | no |
| Active tasks | dev/agent | within days | Yes — for intent | **Yes** | useful (link to spec/commit) |
| Closed tasks | state transition | right after closing | only historically | **No** (only a 1-line summary if recent) | yes (the commit that closes it) |
| Active decisions | dev (often with agent) | when superseded | Yes — for constraints | Yes, as an index | yes (rejected alternative, date) |
| Superseded decisions | state transition | — | no | No | — |
| Spec / feature intent | dev | once implemented | Yes while active | Only if linked to the current task | useful |
| Project knowledge (gotchas, map, conventions) | dev/agent | slowly | medium | Index yes, body on demand | recommended (path/commit) |
| Agent work log | agent | immediately | no | **Never** | — |

### D. Failure modes (preview — detailed in §13)

The four most dangerous: **stale state believed true**, **double truth** (state vs. code/git disagreeing), **missing write-back** (the agent works but doesn't update ATLAS), **concurrent conflicts** (two agents/branches modifying state). The architecture is designed around these four.

---

## 3. Existing ecosystem analysis

Research synthesis (primary sources verified as of August 2026; detailed citations in the session's research reports).

### GitHub Spec Kit (~132k stars, active)
- **Model:** `.specify/` (templates, constitution) + `specs/<branch>/` with spec.md, plan.md, tasks.md, research.md, data-model.md, contracts/. Slash-command workflow: constitution → specify → plan → tasks → implement.
- **What it does well:** multi-agent distribution via native slash-command files (30+ agents); the *constitution* concept (stable project constraints).
- **What it does badly (verified):** extreme ceremony — Scott Logic review: 2,577 lines of markdown for 689 lines of code, 3.5 hours reviewing documents vs. ~8 minutes with incremental prompting. No "current system spec" layer: per-feature directories accumulate and drift. `/speckit.converge` is a later patch for brownfield.
- **Lesson for ATLAS:** agent-native file-based distribution is the winning convention; the "artifacts per feature with no canonical state" model is the anti-pattern to avoid.

### OpenSpec (~66k stars, active)
- **Model:** two explicit levels — `openspec/specs/` = current truth, `openspec/changes/<name>/` = delta in progress (ADDED/MODIFIED/REMOVED), `changes/archive/` = history. The archive step merges deltas into current truth.
- **What it does well:** the only one to have formalized **current ≠ delta ≠ history** — exactly the distinction ATLAS wants, but applied only to specs.
- **What it does badly (verified):** specs are advisory and still drift (manual resync); parallel changes to the same requirement conflict (issue #1387); still a 4+ command workflow for every change.
- **Lesson for ATLAS:** reuse the two-level model, but apply it to *project state* and make it optional, not a ritual for every change.

### ADR / MADR
- **Established convention:** Nygard (Context/Decision/Status/Consequences) and MADR 4.0 (YAML frontmatter with status proposed/accepted/deprecated/superseded-by, file `NNNN-title.md` in `docs/decisions/`). Rule: never delete, mark superseded.
- **Known failure modes:** "ADR as theater" (written, never read), misleading staleness, broken supersession chains. Recent literature converges on: ADR as an *append-only log* + a separate current-state document + injection into agents' context so they actually get read.
- **Lesson for ATLAS:** don't invent a new decision format. Adopt MADR-compatible frontmatter and solve the problem MADR doesn't: getting active decisions to actually *reach* the agent's context.

### AGENTS.md / CLAUDE.md
- AGENTS.md: Linux Foundation standard (Agentic AI Foundation), 20-30+ agents, free-form markdown, nearest-file rule (with divergences: Codex merges root→cwd). Claude Code doesn't read it natively (needs import/symlink).
- CLAUDE.md: managed/user/project/local hierarchy, `@file` imports (depth 4), path-scoped rules in `.claude/rules/`, auto-memory with a MEMORY.md index (first 200 lines/25KB loaded) + on-demand topic files, "under 200 lines" guideline.
- **Lesson for ATLAS:** these files are the **hook point**, not competitors: they hold stable rules and can hold the 5 lines that tell the agent "run `atlas context` at session start". Claude Code's "small always-loaded index + on-demand detail" pattern is the product-level confirmation of ATLAS's context model.

### Ruflo (formerly claude-flow, ~70k stars)
- **What it is:** an orchestration meta-harness: AgentDB (SQLite + HNSW vectors), hierarchical swarms, ~210 MCP tools, 27 hooks, Raft/Byzantine consensus.
- **Issues (verified):** overkill even by its own docs for normal work; unvalidated self-reported benchmarks; always-on memory injection is the exact anti-goal of ATLAS.
- **Lesson for ATLAS:** confirmation by contrast: no DB, no embeddings, no daemon, no mandatory MCP.

### Generic memory (Mem0, Letta/MemGPT, Zep/Graphiti, Cognee)
None is designed for coding agents; all converge on "extract facts → vector/graph store → inject into the prompt". The useful concept from Zep/Graphiti is **bi-temporality** (when a fact was true vs. when it was learned) — too heavy to implement, but the principle "outdated facts are invalidated, not deleted" is worth keeping.

### Beads (Steve Yegge) — **the closest neighbor**
- Git-native issue tracker for agents: issues as a DAG (blocks, parent-child, related, **discovered-from**), collision-resistant hash IDs (`bd-a1b2`), **`bd ready`** (unblocked work, JSON output), AI compaction of closed tasks ("memory decay"), `bd prime` (working context), end-of-session protocol ("land the plane"). Storage: JSONL in git.
- **What it does NOT cover:** project state beyond tasks (phase, decisions, knowledge, specs), context compilation beyond the ready-task list.
- **Honest verdict:** if ATLAS were reduced to tasks + ready-detection, **it would be a reinvention of Beads and not worth building**. ATLAS's differential value is the layer above tasks: state + decisions + compiled brief. See §14 for the "adopt Beads" alternative.

### The ecosystem gap
1. No tool answers in O(1) the question: *"what is the project's current state?"* — all answer with logs, archives, or retrieval.
2. No tool compiles a **minimum brief** cross-artifact (state + tasks + decisions + pointers) designed for an LLM's attention budget.
3. No tool treats **staleness as a measurable property** (state vs. git HEAD) rather than a hope.

---

## 4. Product boundaries

### ATLAS IS
- A **project state ledger**: small, versioned in git, human-readable, queryable via CLI.
- A **context compiler**: from ledger + git it produces a minimum sufficient brief (text + JSON).
- A **handoff protocol** across sessions, different agents, and humans: whoever resumes work starts from the ledger, not from history.
- **Agent-neutral and file-based**: works with any agent capable of running a command or reading a file.
- **Incrementally adoptable**: `atlas init` on an existing repo requires no restructuring; it coexists with existing TODO.md, docs/, ADRs (and can reference them).

### ATLAS IS NOT
- **Not an SDD framework**: no mandatory spec→plan→tasks→implement workflow; specs are optional and linked to tasks, not rituals.
- **Not a memory/retrieval system**: no embeddings, vectors, knowledge graphs, automatic fact extraction.
- **Not an orchestrator**: it doesn't launch agents, doesn't swarm, doesn't coordinate.
- **Not a full issue tracker**: no assignees, sprints, elaborate priorities, boards (if you need that, use Beads or a real tracker).
- **Not documentation**: it doesn't replace docs/, README, ADRs; it indexes them.
- **Doesn't require infrastructure**: no server, DB, daemon, mandatory MCP, cloud.

---

## 5. Conceptual model

The 7 initial categories (state, knowledge, spec, decisions, tasks, history, evidence) **must not become 7 entities**. Analysis:

- **HISTORY** is not an entity: it's a *property* of any element that is no longer active (+ git itself). It isn't modeled, it's archived.
- **EVIDENCE** is not an entity: it's a *field* (link to file/lines/commit/test) on the elements that need it.
- **CURRENT STATE** is not a stored entity: it's a **view** = active elements + facts derived from git. Storing it as a standalone document would recreate the double-truth problem (see §7).
- **SPEC** doesn't deserve a first-class entity in the MVP: a feature's intent lives as the extended body of a task (or a linked file). Spec-per-feature systems are the main source of documentation drift (verified on Spec Kit/OpenSpec). Reopened in §16 if usage justifies it.
- **KNOWLEDGE** and **DECISION** are similar (both "things the agent must know") but with different lifecycles: a decision has supersession and rationale; a knowledge note is a practical fact updated in place. They can be unified into a single entity with a different `type`.

**Proposed minimal model: 2 stored entities + 2 derived views.**

1. **WORKITEM** (task) — a unit of work with a lifecycle. Fields: `id` (short collision-resistant hash, Beads-style), `title`, `status` (todo | doing | blocked | done), `blocked_by` (list of ids), `discovered_from` (id, optional), free markdown body (can hold intent/spec), `evidence` (optional), `summary` (1 line, filled in on close).
2. **CARD** (knowledge/decision) — a durable fact the agent must know. Fields: `id`, `type` (decision | knowledge), `title`, `status` (active | superseded), `superseded_by` (id, for decisions), markdown body (for decisions: MADR-compatible format), `evidence` (optional), `hook` (1 line for the index).
3. **STATE** *(view, not an authoritative file)* — a projection: current goal + active/blocked workitems + active cards (index) + git signals (branch, latest commits, uncommitted diff) + freshness assessment. The only *stored* fragment of state: a `focus` file of a few lines (current goal, phase, next thing) — the only part that isn't derivable.
4. **CONTEXT** *(view, not a file)* — the brief compiled from STATE per the model in §8.

**Challenge accepted against the initial idea:** a hand-maintained "STATE.md" document is the wrong abstraction — it's exactly the 2,000-line TODO.md reborn. State must be *almost entirely derived*; only "focus" (current intent) is declared, because intent isn't derivable from git.

---

## 6. Information lifecycle

    creation → active → superseded/done → compacted → history

- **Creation:** a command (`atlas task add`, `atlas card add`) or a hand-written file in the right format — the two paths must be equivalent (the CLI is a convenience, not a gatekeeper).
- **Active:** the element is visible in STATE and a candidate for the context. `doing` and `blocked` workitems weigh more than `todo` ones.
- **Closing/supersession:** an explicit transition (`atlas task done X`, `atlas card supersede A B`). On close, the workitem MUST receive a 1-line `summary` (provided by the agent/human who closes it — it's the moment when the context to write it already exists, at near-zero marginal cost). It's the lightweight version of Beads's "memory decay".
- **Compaction:** closed elements leave the active files and go into an append-only log (JSONL or dated markdown). By default only the summary of *recently* closed items (last N days) survives in the context, as a bridge between sessions.
- **History:** the log + git. Never loaded by default; queryable (`atlas log`, `git log`). No deletions: as with ADRs, history is invalidated, not rewritten.

Fundamental hygiene rule: **every state transition is also a future-context-reduction operation** (closing = compressing). It's the inverse of TODO.md, where every event *adds* lines.

---

## 7. State model

**Definition:** CURRENT STATE is the minimum set of assertions true *right now* that cannot be economically derived from code or from git:

1. **Intent:** what we're working on and why (focus, a few lines).
2. **Work:** active, blocked and ready workitems (derived from the ledger).
3. **Constraints:** active decisions that limit choices (index of cards).
4. **Ground:** signals from the repository — branch, latest commits, worktree state (derived from git, never stored).
5. **Reliability:** how fresh all of this is (derived: last ledger modification vs. latest commits).

**Answer to the architectural question "what is STATE":** an **on-demand materialized view** — never a hand-maintained document (drifts), never a DB (unjustified infrastructure), never a freeform summary written by an agent (unverifiable). The declared part (focus) is deliberately so small that keeping it up to date costs less than a commit message line.

**Source of truth (§1's architectural question):** layered, unambiguous:
- **Facts about the software** → code + git. ATLAS never duplicates them.
- **Intent and work state** → the ATLAS ledger.
- **Constraints and rationale** → ATLAS cards (or referenced existing ADRs).
- On conflict between ledger and git, **git wins on facts, the ledger wins on intent**, and the conflict itself is a signal ATLAS must surface (freshness check), not hide.

---

## 8. Context model

**Definition:** the MINIMUM SUFFICIENT CONTEXT is the smallest set of tokens that lets the agent (a) know what to do, (b) know what NOT to do, (c) know **where to look** for everything else.

Structural principle (aligned with the 2025-26 consensus on context engineering: "smallest set of high-signal tokens", just-in-time retrieval, progressive disclosure):

> **The compiled context contains summaries and pointers, never full content.** A modern agent is excellent at reading files just-in-time; the bottleneck is knowing *which* files and *what* state. ATLAS provides the map, not the territory.

Brief structure (target budget: **< 1,500 tokens** in the typical case, configurable hard cap):

    [FOCUS]      current goal, phase                          (~3 lines, declared)
    [NOW]        doing/blocked workitems + reason for block    (~1 line each + relevant paths)
    [READY]      unblocked workitems, in order                 (1 line each)
    [RULES]      active cards: 1-line hook + id                (the agent opens the card only if needed)
    [RECENT]     summary of recently closed items + latest commits (bridge between sessions)
    [GROUND]     branch, dirty/clean worktree, freshness        (~3 lines, derived)
    [POINTERS]   how to dig deeper: atlas show <id>, path       (~3 lines)

Two formats of the same content: **text** (for prompt injection, human-readable) and **JSON** (`--json`, for tooling). The context is **parametric**: `atlas context` = general brief; `atlas context <id>` = brief centered on a workitem (its full body + only the linked cards + its paths).

**Challenge accepted:** "context = a document generated by the CLI" is correct, but with a constraint the initial idea didn't spell out — if the compiler starts *including* knowledge instead of *pointing to it*, ATLAS degenerates into the retrieval system it doesn't want to be. The token budget is a first-class architectural constraint, not an optimization.

---

## 9. Context compilation (conceptual algorithm)

No semantics, no embeddings: **selection by explicit state and links**, plus git signals. Deterministic, explainable, testable.

    input: ledger (workitem, card, focus), git repo, [optional target id]

    1. GROUND    ← current branch, HEAD, dirty/clean, latest K commits (git, read-only)
    2. FRESHNESS ← compare ledger timestamp vs. recent commits;
                   if the ledger hasn't been touched in N commits/days → flag "state possibly stale"
    3. ACTIVE    ← workitem status ∈ {doing, blocked}
       READY     ← todo workitems with no open blocked_by (Beads-`bd ready`-style)
    4. RULES     ← active cards; if there's a target, cards linked to the target come first
    5. RECENT    ← summary of items done in the last N days (small default)
    6. TARGET    ← if an id is requested: full workitem body + evidence + linked cards
    7. BUDGET    ← rendering order FOCUS > NOW > GROUND/FRESHNESS > READY > RULES > RECENT;
                   if the budget is exceeded, lower-priority sections are truncated
                   (never FOCUS/NOW), degrading from summary to id-only
    8. output    ← text or JSON

Deliberately excluded: semantic retrieval (unnecessary with explicit links and agents capable of grep), probabilistic ranking (opaque), reading source code (the agent's job, just-in-time). Relevance selection over *files* is delegated to the paths cited in workitems — explicit evidence, not inference.

---

## 10. Repository model

The smallest reasonable layout — file-based, git-versioned, readable and writable by hand:

    .atlas/
      focus.md            # 3-10 lines: goal, phase, next thing (the only declared state)
      work/
        <id>-<slug>.md    # one active workitem per file (YAML frontmatter + markdown body)
      cards/
        <id>-<slug>.md    # one card per file (MADR-compatible frontmatter for decisions)
      log.jsonl           # append-only: compacted closed elements (id, title, summary, ts, commit)
      config.toml         # minimal options (budget, N recent days) — optional, sensible defaults

Choices and rationale:
- **One file per active element** (not a monolithic file): near-conflict-free git merges, hand-editable, readable diffs. It's the lesson from Beads (JSONL/files in git) and from MADR (one file per decision).
- **Markdown + YAML frontmatter**: the format every agent and human already reads/writes; the structured part lives in the frontmatter, the prose in the body. No binary format, no DB. (TOML/pure JSON evaluated: they lose the prose; SQLite evaluated: infrastructure and opacity unjustified at this scale — dozens of active elements, not thousands.)
- **Append-only `log.jsonl`** for history: cheap, greppable, never in context.
- **Hidden `.atlas/` directory**: signals "tool-managed" while staying inspectable; in git by default (sharing state is the point; the "unshared personal state" case is deferred to §17).
- **Incremental adoption**: `atlas init` creates only `.atlas/focus.md`; everything else is created on first use. No obligation to migrate existing TODOs/ADRs — cards can point to them.

---

## 11. CLI model

The smallest CLI coherent with the model — **~8 commands**, two of which do 90% of the work:

    atlas init                          # creates .atlas/ + installs the contract in AGENTS.md/CLAUDE.md (marked, idempotent block)
    atlas seed                          # brownfield: emits the curation brief the agent runs (§12.2)
    atlas context [id] [--json]         # THE command: compiles the brief
    atlas state                         # human view of state (readable superset of context)

    atlas task add "title" [--blocked-by id] [--from id]
    atlas task start|block|done <id> [--summary "..."]   # done requires a summary; start registers branch+claim (§12.1)
    atlas card add --type decision|knowledge "title"
    atlas card supersede <old> <new>
    atlas show <id>                     # full body of an element

    atlas doctor                        # freshness + integrity (orphan ids, cyclic blocks, stale focus)

Principles:
- **Every read command has `--json`.** Agents shouldn't parse markdown.
- **The CLI is not a gatekeeper:** hand-editing files is supported; `atlas doctor` checks consistency instead of preventing it.
- **No workflow subcommands** (no plan/approve/archive/sync): the whole lifecycle is `task done` and `card supersede`.
- Short command names matter: the agent invokes them autonomously and every token of the command is context.

---

## 12. Agent integration

A single mechanism, three levels of attachment — **no MCP required**:

1. **Bootstrap (mandatory, ~5-10 lines), installed by `atlas init` — never manual copy-paste.** The behavioral contract:
   *"At session start: `atlas context` → this is the current state. Before working on a task: `atlas task start <id>`. When done: `atlas task done <id> --summary '...'`. Non-obvious decision: `atlas card add`. Discovered work: `atlas task add --from <id>`. Before closing the session: update states and focus."*
   `atlas init` writes it as a **marked idempotent block** (`<!-- atlas:begin --> … <!-- atlas:end -->`) in the detected agent files: append to AGENTS.md if it exists; CLAUDE.md for Claude Code (or `@AGENTS.md` import); create AGENTS.md if nothing exists. Re-running `init` updates only the block, never the rest of the file (same pattern as `openspec update`). Works identically for Claude Code, Codex, Cursor, OpenCode: they all read their own instruction file and run shell commands — the real lowest common denominator of the ecosystem (verified: it's the same channel used by Spec Kit/OpenSpec/Beads).
2. **Per-agent convenience (optional):** thin slash-command/skill wrappers (`/atlas-context`, `/atlas-done`) generated by `atlas init --integration <agent>`, on the Spec Kit model. CLI wrappers only, never logic.
3. **End of session (protocol, not software):** the bootstrap includes the equivalent of Beads's "land the plane": before closing, update states and focus. This is where the write-back problem plays out (§13).

Cross-cutting principle — **the contract states the *when*, never the *how*.** All logic (atomicity, policy, budget, freshness) lives in the binary; situational instructions live in the **CLI's output, just-in-time**: a rejected `task start` responds "claimed by feature/a — alternatives ready: cd34, ef56"; `atlas context` closes with a write-back reminder. Every such token is spent only when relevant, instead of weighing on every session. Guard metric: if using ATLAS required 50 lines of static instructions, that would be a CLI design flaw, not a documentation problem.

Humans use the same commands or edit the files. There's no separate "for agents" path.

---

## 12.1 Concurrency: worktrees, branches and parallel agents

Real scenario (surfaced in review): multiple agents work in parallel on different worktrees/branches of the same repo. With the ledger versioned in git, each worktree sees its *own* copy of `.atlas/`: two distinct problems arise, to be treated separately.

1. **Visibility race** — agent B doesn't see that A (in another worktree) has claimed or closed a task until a merge happens: stale READY, double-claiming of the same work.
2. **Merge conflicts** — two branches modify the same `.atlas/` files.

Evaluation of the two proposals on the table:

- **"Externalize the controller outside the repository"** — as the primary store: no. It would lose versioning, team sharing via git, review of state changes in PRs, and the repository-local principle (§4). But there's a technical middle ground: all worktrees of a repo share the common git directory (`git rev-parse --git-common-dir`). An **ephemeral coordination layer** can live there (`.git/atlas/claims/<task-id>.json` — one file per claim): instantly shared across all worktrees on the machine, invisible to git, with no merge issues. Atomicity is filesystem-level: acquiring a claim = atomic exclusive file creation — `O_CREAT|O_EXCL` on POSIX, `CreateFile`+`CREATE_NEW` on Windows, exposed portably by every runtime (Go `O_EXCL`, Rust `create_new`, Node `'wx'`, Python `'x'`; it's the same primitive git itself uses for `index.lock` on every platform). A single atomic operation: the second attempt gets `EEXIST`/`ERROR_FILE_EXISTS`, which is directly the semantic outcome "already taken" — **no mutex, no retry logic anywhere, neither in the CLI nor in the agent**: the shared mutable resource is eliminated instead of protected. It's a **cache, not truth**: rebuildable at any time from the ledger; if it's missing, ATLAS degrades gracefully.
- **"CLI only on develop"** — too strong if applied to everything: it would kill write-back exactly when the context for it exists (the `task done --summary` happens in the feature worktree; deferring it to develop reintroduces the exact problem we want to solve). Correct instead if split **by operation class** (the same "plan mutations only from develop" pattern already adopted by tools like aiops-ai-spec).

**Proposed model — per-binding ownership, ephemeral coordination, centralized plan:**

1. **Task↔branch binding:** `atlas task start` registers the current branch in the frontmatter (`branch:`). From then on only that branch modifies that file: the CLI rejects (or warns on, per policy) edits to a workitem claimed elsewhere. With one-file-per-element, merge conflicts become structurally rare: each branch touches only "its own" files.
2. **Operation classes:**
   - *Read-only* (`context`, `state`, `show`, `doctor`): always allowed, in any worktree — no risk.
   - *Transitions of the task assigned to the branch* (`start/block/done`, plus `task add --from <id>` for work discovered during implementation): allowed in the feature worktree.
   - *Plan mutations* (new unlinked tasks, `card add/supersede`, focus edits): recommended only on the integration branch (develop/main). Configurable policy: `warn` by default (single-dev repos without gitflow exist), `strict` for teams.
3. **Ephemeral claims in `$GIT_COMMON_DIR`:** on `task start` the claim (branch, session, timestamp with TTL in the content) is created in the shared layer; `atlas context` from any worktree reads it and shows "in progress elsewhere". A `task start` on an already-claimed task **fails fast with a semantic outcome** — never waiting, never queuing: in `--json` it returns the reason and READY alternatives (`{"error":"claimed","by":"feature/a","ready":["cd34","ef56"]}`), and the agent moves to something else. The escape valve for the legitimate case ("B really has to work on that task", or A died before the TTL expired) is `--steal`: explicit, noisy, never an automatic fallback. Release = file deletion on `done`; orphaned claims expire via TTL. Solves the same-machine visibility race with no server, no daemon and no blocking locks.
4. **Declared limit — different machines:** claims don't cross machines. There, only the advisory binding in the frontmatter (versioned, hence visible after push/pull) and the git merge remain. Actually solving it would require a central server: out of scope on principle (§4).
5. **Log merging:** `log.jsonl` marked `merge=union` in `.gitattributes` — appends from different branches merge without conflict.

Impact on the rest of the document: §11 (the CLI gains policy and implicit claiming in `task start`), §13.3 (rewritten accordingly), §15 (the binding enters the MVP; the claims layer can slip to the next phase), §17 (new open question #9).

---

## 12.2 Seeding existing repositories (brownfield)

The primary adoption case isn't a greenfield project but a mature repo: hundreds of markdown files, ADRs, monolithic TODOs with years of history (real reference case: ~24k lines of markdown across 284 files, existing ADRs, a huge TODO/history). Manual triage is impractical; it needs LLM assistance. The architectural question is *where* the LLM lives.

**Principle: the binary stays LLM-free.** Calling models from inside `atlas` would mean API keys, vendor dependency, cost and non-determinism in a deterministic tool — a direct violation of agent-neutrality (§4) and "no infrastructure". The LLM for seeding already exists: **it's the user's coding agent.**

**Mechanics — `atlas seed` emits, the agent executes:**

1. `atlas seed` calls no model: it **prints the curation brief** (applying the "just-in-time instructions from the CLI" principle, §12) — instructions for the agent on what to inventory (TODOs, docs/, ADRs, recent git log) and how to triage it into the ATLAS model.
2. The agent explores the repo and writes via the normal commands (`task add`, `card add`, focus) — no special write path.
3. `atlas doctor` validates the result; the output lives on a dedicated branch/worktree.
4. **Mandatory human gate:** the seed is a proposal; the human prunes and commits it. Never auto-commit — the seed is the one moment where garbage-in poisons every future context.

**Anti-delusion rule — the seed is *lossy by design*, it extracts rather than migrates:**

- **Focus:** 5-10 lines on where the project stands *today*.
- **Workitem:** only open work still relevant, with a **default cap (~15)**: if more are needed, the triage isn't finished. Historical TODO material isn't imported: it stays where it is.
- **Card:** only still-binding decisions. Existing ADRs are **never copied**: card = 1-line hook + `evidence: docs/adr/NNNN-*.md`. ATLAS indexes them, it doesn't ingest them.
- **History files:** explicitly excluded; at most one "lessons" card (2-3 still-relevant gotchas) + a pointer.

**Link to the MVP:** Phase 0 (§15) *is* the prototype of this brief — seeding the pilot project is done conversationally with the agent, and whatever works becomes the text `atlas seed` will print in Phase 1.

---

## 13. Architecture failure modes

1. **Missing write-back (risk #1, behavioral).** The agent works and doesn't update the ledger → stale state → lost trust → abandonment (the death spiral of every project-knowledge system, from TODOs to ADRs). Mitigations: near-zero update cost (one command with a summary), explicit bootstrap instructions, and above all **visible freshness**: `atlas context` always states how old the state is relative to git — stale state is detected, never passed off as true. Deliberate non-mitigation: no automatic state inference from diffs (unreliable, and a wrong inference presented as state is worse than declared staleness).
2. **Double truth.** If the ledger duplicates facts about the code, it will diverge. Structural mitigation: the ledger holds only intent/constraints/work state; facts stay in code+git (§7).
3. **Concurrent conflicts (two agents/worktrees/branches).** Fully covered in §12.1. In summary: one-file-per-element + hash ids (merge collisions nearly impossible), task↔branch binding on `task start`, plan mutations confined to the integration branch (warn/strict policy), ephemeral claims in `$GIT_COMMON_DIR` for cross-worktree visibility on the same machine, `merge=union` on the log. Accepted residual case: agents on different machines that ignore the advisory binding → a normal git conflict, visible and resolvable.
   **Observed gap (2026-08-27, migration-toolkit-v2 pilot):** claims prevent concurrent *in-flight* double-claiming, but not merge-time resurrection — a branch cut from `develop` before a task was closed elsewhere can still bring the old (active) copy of its `work/` file back on merge, since `git merge` never consults claims and a delete-vs-modify merge doesn't always surface as a conflict. `atlas doctor` now catches the resulting contradiction (an id both active in `work/` and closed in `log.jsonl`) as a `resurrected_workitem` error (SPEC.md S2). This detects it after the fact; it doesn't prevent it — the bootstrap contract (§12) now recommends `atlas doctor` right after merging any branch that touches `.atlas/`.
4. **Context bloat (product drift).** The temptation to add sections to the brief. Mitigation: token budget as a tested constraint (regression test on output size).
5. **Stale cards/decisions.** Same fate as ADRs if there's no pressure to review them. Partial mitigation: cards pass through the context (so they get read, hence can be challenged); `atlas doctor` flags very old, never-touched cards. Accepted risk: not fully solvable by a tool.
6. **Ledger corrupted/inconsistent from manual edits.** Mitigation: tolerant parsing + `atlas doctor`; the CLI never assumes it's the only writer.
7. **Forgotten focus.** The declared focus is the one piece that can lie without git directly disproving it. Mitigation: it's tiny (re-readable in 5 seconds) and freshness covers it (last-modified is visible).

---

## 14. Alternatives considered

1. **Adopt Beads + conventions, build nothing.** Beads covers task-DAG, ready-work, compaction, git-native. Would add: a convention for decisions (MADR) and a focus.md. **Pros:** zero new software, existing community. **Cons:** it's missing the entire context-compilation layer (`bd prime` is task-centric), it's missing the card/decision model, and depending on another project's design (which is migrating storage, from JSONL to Dolt) limits ATLAS's evolution; an ATLAS built on top of Beads would effectively be a wrapper around someone else's decisions. **Verdict: REJECTED (product decision, 2026-08-27).** See [ADR 0001](adr/0001-standalone-not-built-on-beads.md). ATLAS is standalone and owns its own minimal task layer; from Beads only the design lessons are reused (collision-resistant hash ids, ready-detection, summary on close), not the software. The accepted cost is reimplementing a simple task layer — consistent with ATLAS workitems being deliberately poorer than a full issue tracker (§4).
2. **No CLI: just a file convention + a skill/prompt.** ATLAS as a "spec" (`.atlas/` layout + instructions): the agent itself reads/writes the files and compiles the brief. **Pros:** zero code, instant adoption. **Cons:** context compilation itself becomes a context cost (the agent reads everything to summarize it — the exact problem we wanted to eliminate); no deterministic/testable output; freshness checking is impractical. Rejected as the final product, but it's a great **pre-code validation experiment** (try the format on a real project before writing the CLI).
3. **MCP server / daemon with an index.** **Pros:** rich integration, live queries. **Cons:** infrastructure, a tool surface that consumes context, excludes agents without MCP, contradicts principle 10. Rejected for the MVP; a possible thin MCP wrapper *over the CLI* in the future (§16).
4. **State 100% derived from git (zero ledger).** Automatic synthesis from commits/diffs. **Pros:** never stale. **Cons:** git records *what happened*, not *what is intended* nor *why*; inferring intent is exactly the kind of unreliable automation to avoid. Rejected; survives as a component (GROUND/FRESHNESS).

---

## 15. MVP boundary

**Hypothesis to validate:** "a brief compiled from a minimal ledger materially reduces the cost of starting a session (tokens read and time to first useful action) without degrading the correctness of the agent's actions."

**Inside the MVP (2-phase validation):**
- **Phase 0 — no code (1-2 weeks on a real project):** `.atlas/` layout compiled by hand + bootstrap instructions in CLAUDE.md/AGENTS.md. The initial seeding is done conversationally with the agent on a real brownfield repo: it's the prototype of the `atlas seed` brief (§12.2). Measure: does the agent actually resume from the ledger? Does the format hold up? What's missing from the brief?
- **Phase 1 — minimal CLI:** `init` (layout + installing the bootstrap block in AGENTS.md/CLAUDE.md), `seed` (emitting the curation brief, §12.2), `context [id] [--json]`, `task add/start/done` (with branch binding and claims, §12.1), `card add/supersede`, `show`, `doctor` (freshness only). A static Go binary, no external dependencies, no per-agent integration beyond the bootstrap block.

**Outside the MVP (explicitly):** spec as an entity, per-agent templates, MCP, git hooks, import from existing TODOs/ADRs, multi-repo, web UI, any state-inference automation.

**Success criteria (measurement protocol in §17.7, real baseline acquired):** reconstruction tax (tokens from start to first productive action) reduced by >50% vs. the TODO.md baseline on the same project; **average context per request** (cache read ÷ requests) and compactions per session trending down; the typical brief stays under ~1,500 tokens; in ≥80% of sessions the agent performs write-back without a human prompt; zero cases of stale state not flagged by the freshness check. On total session cost the honest expectation is −10/25% (§17.7).

**Failure criterion (honesty):** if in Phase 0 the agent systematically ignores the ledger or write-back doesn't happen, the problem is behavioral and no CLI fixes it → stop and rethink the integration model before writing code.

---

## 16. Future evolution (without compromising the minimum)

Ordered by likelihood of actually being needed; each is additive, none changes the data model:

1. **Per-agent integrations beyond the bootstrap** (`atlas init --integration claude-code|codex|cursor`): slash-command/skill wrappers, on the channel already proven by Spec Kit. (Installing the bootstrap block in AGENTS.md/CLAUDE.md is already in the MVP; this is only about the extra conveniences.)
2. ~~**Lightweight import**~~ — absorbed by `atlas seed` (§12.2), promoted into Phase 1: assisted triage of existing material is the guided agent's job, not a parser's.
3. **First-class specs** — **PROMOTED AND DECIDED (2026-08-27):** see [ADR 0003](adr/0003-living-canonical-specs.md). The user's real workflow is spec-driven and workitem bodies aren't enough for large intents. Model chosen: **living canonical specs** in `.atlas/specs/` — one per capability/area, status draft→active→superseded, updated in place with git as the delta history; workitems link via a `spec:` field; the target context includes the task's spec. NEVER accumulated spec-per-feature (the verified Spec Kit anti-pattern). Implementation details in SPEC.md §S9.
4. **Optional git hooks** (post-commit → write-back reminder; never automatic state writing).
5. **Thin MCP wrapper** over the CLI, for environments where the shell command isn't available.
6. **History queries** (`atlas log --grep`), always on demand.
7. **Multi-project / workspace** (aggregating multiple ledgers), only on real need.

Permanent red line: no daemon, no DB server, no embeddings, no state write not explicitly requested by a human or an agent.

---

## 17. Open questions (to be decided together, not now)

1. **Directory name and location:** `.atlas/` (hidden) vs. `atlas/` (visible)? Always in git, or support for a local unversioned layer (CLAUDE.local.md-style)?
2. **Brief language/format:** plain text vs. lightweight markdown? Is the 1,500-token budget the right one? Fixed or adaptive?
3. **Workitem granularity:** should ATLAS discourage overly large tasks (epics), or is that the user's business? Is a `parent` field needed in addition to `blocked_by` and `discovered_from`, already in the MVP?
4. **Summary on close: mandatory or strongly recommended?** Making it mandatory guarantees RECENT's quality but adds friction for the "trivial task" case.
5. ~~**Relationship with Beads**~~ — **RESOLVED (2026-08-27):** see [ADR 0001](adr/0001-standalone-not-built-on-beads.md). ATLAS is standalone, no wrapper around and no dependency on Beads; only design lessons are reused (§14.1).
6. **CLI implementation stack** — **partially resolved (2026-08-27):** see [ADR 0002](adr/0002-go-implementation-git-subprocess.md). Decided constraint: a native compiled binary, no interpreted languages (the CLI is invoked dozens of times per session by agents; interpreter startup and dependency management are unacceptable). Architect's proposal: **Go** (the category's native ecosystem — gh/kubectl/terraform —, trivial cross-compilation for the macOS/Linux/Windows matrix, ~5ms startup, single static binary, low contributor barrier; Rust rejected because its advantage — performance/memory — is irrelevant for this workload). Related decision: interact with git via **subprocess of the system `git`** (as `gh` does), not an embedded library — true worktree/`--git-common-dir` semantics, and `go-git` has incomplete worktree support. **RESOLVED — Go confirmed (2026-08-27).**
7. **Validation telemetry — protocol defined (2026-08-27), to run in Phase 0.** Real baseline acquired from 3 Claude Code sessions (07/25-29) on the reference project: cost $180-409/session, of which **>95% is re-read/re-written context** (cache read 308-678M tokens ≈ $93-204/session; cache write $64-165) vs. fresh input <$1 and output 3-4%. Verified multiplicative effect: every token loaded at session start gets re-paid on every subsequent request (~30k reconstruction tokens × 1,000 requests ≈ 30M cache read), plus compactions that re-pay reconstruction (wall time 1-2 days ⇒ repeated cycles). **Protocol (from Claude Code JSONL transcripts, N pre- vs. post-seed sessions on the same repo):** (1) reconstruction tax = tokens from start to first productive action — here the −50% target from §15 applies; (2) **average context per request** = cache read ÷ number of requests (the main synthetic metric); (3) compactions per session; (4) write-back rate; (5) normalization per API hour or lines changed. **Honest expectation on total cost: −10/25%** (code reading is necessary work and remains), i.e. $30-100 on sessions like the baseline's; the −50% target applies only to the startup tax. The qualitative benefit (less context rot, fewer compactions) is expected to exceed the economic one.
   **Baseline measured from transcripts (2026-08-27, migration-toolkit-v2 repo, main session 08/03: 124 requests, 22h wall):** average context per request **157.6k tokens**; first productive action at request **#9** with **~49.5k tokens** of accumulated context (~392k cache read spent at startup); **TODO.md ≈ 12.4k tokens per read, re-read 3 times in the same session** (~37k total — the problem's signature: state refresh via re-reading history, then dragged in the window for ~115 requests ≈ 1.4M tokens of cache read); AGENTS.md ~0.6k (already the right size). Concrete goal: `atlas context` ≤1.5k replaces the state component of the ~50k startup and the 12.4k refreshes (−88% on that component). **Full scan of ~/.claude/projects (29 projects, 8 sessions with data, 5 with ≥30 requests):** medians — average context per request **127.6k tokens** (range 64.5k-286.4k), first productive action at request **#13** (range 1-27), context at that point **66.6k tokens** (range 49.5k-93.6k). The pattern is cross-project, not specific to migration-toolkit-v2; worst case: the ai-harness session at 286k/request, first action at request #27. Caveat: the baseline's July sessions are no longer on disk (transcript retention or a different machine) — the economic baseline remains the one from screenshots, the structural one is measured from 5 sessions.
8. **The name `card`:** does it hold up for the decision+knowledge union, or is it confusing? Alternatives: `note`, `fact`, `rule`.
9. **Concurrency (§12.1):** should the policy on plan mutations outside the integration branch default to `warn` or `strict`? Does the claims layer in `$GIT_COMMON_DIR` enter the MVP (Phase 1), or does it slip, leaving only the task↔branch binding in the MVP? Which branches count as "integration" (develop? main? configurable)?

---

*Sources: research conducted on 2026-08-27 from primary sources (GitHub repos, official Anthropic/OpenAI/MADR/agents.md documentation, independent reviews). Detailed citations available in the session's research reports.*
