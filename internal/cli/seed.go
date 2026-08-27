package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
)

// seedBrief is the constant curation brief printed by `atlas seed`
// (PLAN.md S4). atlas never calls an LLM itself: this text is read by
// the human's coding agent, which does the actual triage using ordinary
// `atlas task add` / `atlas card add` / focus-file edits.
const seedBrief = `ATLAS SEED — curate this repository into the ATLAS ledger

You are about to bootstrap .atlas/ from the existing project material. This
is a one-time (or occasional) curation pass performed by YOU, the coding
agent — atlas itself never calls an LLM and makes no network calls.

1. INVENTORY. Before writing anything, read broadly:
   - Any TODO.md, a TODO/ directory, or inline TODO/FIXME/HACK comments.
   - docs/ and any ADR/decision directory (docs/adr, decisions/, etc.).
   - Recent history: run "git log --oneline -50" to see what has actually
     been happening lately, not just what is documented.

2. TRIAGE — this is LOSSY BY DESIGN. You are extracting a present-tense
   snapshot, not migrating history. Nothing described here is deleted from
   its original location; it simply isn't copied into the ledger.

   FOCUS (.atlas/focus.md): 5-10 lines on where the project stands TODAY.
   Overwrite the placeholder template atlas init left there.

   WORKITEMS: only work that is OPEN and still RELEVANT right now. Cap
   yourself at roughly 15 workitems total. If you think you need more, the
   triage isn't finished yet — group related items or drop the stale ones.
   Closed or historical TODOs are NOT imported; they stay wherever they
   already live (TODO.md, closed issues, git history).
     Examples:
       atlas task add "Fix flaky retry in reconcile loop" \
         --evidence packages/core/pipeline/reconcile.py:120-180
       atlas task add "Migrate config loader to v2 schema" \
         --blocked-by a1b2

   CARDS: only decisions that are STILL BINDING today. If a written ADR
   already exists for a decision, do NOT copy its text into the card body
   — the card is a one-line hook plus a pointer via --evidence, never a
   duplicate of the ADR's content.
     Example:
       atlas card add --type decision "Use O_EXCL for claims, never a mutex" \
         --hook "Claim = file O_EXCL in git-common-dir" \
         --evidence docs/adr/0007-claims.md

   HISTORY is excluded from the ledger, with exactly one allowed exception:
   at most one "lessons learned" knowledge card, pointing at where the real
   history lives (CHANGELOG.md, closed ADRs, an issue tracker) — never a
   separate card per historical decision.
     Example:
       atlas card add --type knowledge "Past incidents and lessons" \
         --hook "See CHANGELOG.md and docs/adr/ for closed decisions" \
         --evidence CHANGELOG.md

   SPECS: only for capabilities/areas with a genuinely LIVING intent —
   something still being shaped, not a capability that is simply done. Cap
   yourself at roughly 5 specs at seed time; most projects need zero or one.
   If a written spec/design doc already exists, reference it via --evidence
   — never copy its text into the spec body. A spec MUST follow a decision
   (S9.8): create the governing decision card FIRST, then the spec that
   references it via --decision.
     Example:
       atlas card add --type decision "Adopt bounded-retry workload model" \
         --hook "Retries are capped, never infinite" \
         --evidence docs/adr/0034-enrichment-stage.md
       atlas spec add "Workload execution retry semantics" \
         --decision <the-decision-id-just-created> \
         --evidence docs/design/retry.md
       atlas spec activate <the-new-spec-id>

3. WORK ON A DEDICATED BRANCH. Do not curate directly on main/develop:
     git checkout -b atlas/seed

4. USE THE CLI ONLY. Every write goes through "atlas task add", "atlas
   card add", "atlas spec add", or an edit to .atlas/focus.md — never
   hand-craft files under .atlas/work, .atlas/cards or .atlas/specs directly.

5. FINISH WITH A HEALTH CHECK:
     atlas doctor
   Resolve anything it flags (orphan references, malformed frontmatter,
   cycles, etc.) before handing this off.

6. HUMAN REVIEW IS MANDATORY. This is a proposal, not a commit. Do not
   run "git commit" yourself — hand the branch back to a human to prune,
   correct, and commit. Bad seed data poisons every future "atlas context"
   call, so when in doubt, curate less rather than more.
`

func newSeedCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "seed",
		Short: "Print the curation brief for bootstrapping an existing repository",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonFlag(cmd) {
				data, err := json.Marshal(map[string]string{"brief": seedBrief})
				if err != nil {
					return failIO(cmd, true, err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
				return nil
			}
			fmt.Fprint(cmd.OutOrStdout(), seedBrief)
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}
