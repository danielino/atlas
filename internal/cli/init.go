package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/dmarcocci/atlas/internal/ledger"
)

const focusTemplate = `<!-- ATLAS focus: 3-10 lines on where the project stands TODAY. Edit this
     file freely; ` + "`atlas init`" + ` will never overwrite it once it exists. -->

Project just initialized with ATLAS.
Run ` + "`atlas seed`" + ` to bootstrap workitems and cards from existing docs/TODOs,
or ` + "`atlas task add \"...\"`" + ` to start tracking work directly.
`

const configTemplate = `# ATLAS configuration (.atlas/config.toml). Every key below is optional;
# the commented values are the built-in defaults.

[context]
# budget_tokens = 1500
# recent_days = 7

[policy]
# plan_mutations = "warn"        # "warn" | "strict"
# integration_branches = ["main", "develop"]

[claims]
# ttl_hours = 24
`

const gitAttributesLine = ".atlas/log.jsonl merge=union"

const bootstrapBegin = "<!-- atlas:begin -->"
const bootstrapEnd = "<!-- atlas:end -->"

const bootstrapBody = `## ATLAS
- At session start run ` + "`atlas context`" + `: its output is the current project state.
- Before working on a task: ` + "`atlas task start <id>`" + ` (if refused: pick a task from the ready list).
- When you finish a task: ` + "`atlas task done <id> --summary \"one line on what changed\"`" + `.
- Made a non-obvious decision? ` + "`atlas card add --type decision \"title\" --hook \"one-line summary\"`" + `.
- Discovered new work? ` + "`atlas task add \"title\" --from <current-task-id>`" + `.
- Before ending the session: update task states and, if the goal changed, ` + "`.atlas/focus.md`" + `.
- Use ` + "`--json`" + ` on read commands. Never edit files under ` + "`.atlas/`" + ` by hand: use the CLI.
`

func bootstrapBlock() string {
	return bootstrapBegin + "\n" + bootstrapBody + bootstrapEnd + "\n"
}

var bootstrapBlockRe = regexp.MustCompile(`(?s)<!-- atlas:begin -->.*?<!-- atlas:end -->\n?`)

// upsertBootstrapBlock replaces an existing <!-- atlas:begin -->/<!-- atlas:end -->
// block in content with the current bootstrap block, or appends the block
// (creating separation whitespace as needed) if none is present. Text
// outside the markers is never touched.
func upsertBootstrapBlock(content string) string {
	block := bootstrapBlock()
	if bootstrapBlockRe.MatchString(content) {
		return bootstrapBlockRe.ReplaceAllString(content, block)
	}
	if content == "" {
		return block
	}
	sep := "\n"
	if strings.HasSuffix(content, "\n") {
		sep = ""
	}
	return content + sep + "\n" + block
}

func upsertBootstrapFile(path string, createIfMissing bool) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
		if !createIfMissing {
			return nil
		}
		data = nil
	}
	updated := upsertBootstrapBlock(string(data))
	return os.WriteFile(path, []byte(updated), 0o644)
}

func writeIfAbsent(path string, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func ensureGitAttributes(root string) error {
	path := filepath.Join(root, ".gitattributes")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte(gitAttributesLine+"\n"), 0o644)
		}
		return err
	}

	content := string(data)
	for _, line := range strings.Split(content, "\n") {
		if strings.TrimSpace(line) == gitAttributesLine {
			return nil // already present, exactly one line, idempotent
		}
	}

	if content != "" && !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	content += gitAttributesLine + "\n"
	return os.WriteFile(path, []byte(content), 0o644)
}

func newInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize an ATLAS project in the current directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := os.Getwd()
			if err != nil {
				return failIO(cmd, false, err)
			}

			if err := ledger.EnsureDirs(root); err != nil {
				return failIO(cmd, false, err)
			}

			if err := writeIfAbsent(filepath.Join(root, ".atlas", "focus.md"), focusTemplate); err != nil {
				return failIO(cmd, false, err)
			}

			if err := writeIfAbsent(filepath.Join(root, ".atlas", "config.toml"), configTemplate); err != nil {
				return failIO(cmd, false, err)
			}

			if err := ensureGitAttributes(root); err != nil {
				return failIO(cmd, false, err)
			}

			// AGENTS.md: append the bootstrap block, or create the file.
			if err := upsertBootstrapFile(filepath.Join(root, "AGENTS.md"), true); err != nil {
				return failIO(cmd, false, err)
			}

			// CLAUDE.md: only patch it if it already exists; never create it.
			if err := upsertBootstrapFile(filepath.Join(root, "CLAUDE.md"), false); err != nil {
				return failIO(cmd, false, err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Initialized ATLAS project in "+root)
			return nil
		},
	}
}
