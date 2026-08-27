package ledger

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoadConfig_DefaultsWhenFileAbsent(t *testing.T) {
	root := setupLedgerRoot(t)

	cfg, err := LoadConfig(root)
	require.NoError(t, err)

	require.Equal(t, 1500, cfg.Context.BudgetTokens)
	require.Equal(t, 7, cfg.Context.RecentDays)
	require.Equal(t, "warn", cfg.Policy.PlanMutations)
	require.Equal(t, []string{"main", "develop"}, cfg.Policy.IntegrationBranches)
	require.Equal(t, 24, cfg.Claims.TTLHours)
}

func TestLoadConfig_PartialOverridesKeepOtherDefaults(t *testing.T) {
	root := setupLedgerRoot(t)
	configPath := filepath.Join(root, ".atlas", "config.toml")
	content := "[context]\nbudget_tokens = 3000\n"
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0o644))

	cfg, err := LoadConfig(root)
	require.NoError(t, err)

	require.Equal(t, 3000, cfg.Context.BudgetTokens)
	// Untouched defaults must survive the partial override.
	require.Equal(t, 7, cfg.Context.RecentDays)
	require.Equal(t, "warn", cfg.Policy.PlanMutations)
	require.Equal(t, []string{"main", "develop"}, cfg.Policy.IntegrationBranches)
	require.Equal(t, 24, cfg.Claims.TTLHours)
}

func TestLoadConfig_FullOverride(t *testing.T) {
	root := setupLedgerRoot(t)
	configPath := filepath.Join(root, ".atlas", "config.toml")
	content := `
[context]
budget_tokens = 2000
recent_days = 14

[policy]
plan_mutations = "strict"
integration_branches = ["main"]

[claims]
ttl_hours = 48
`
	require.NoError(t, os.WriteFile(configPath, []byte(content), 0o644))

	cfg, err := LoadConfig(root)
	require.NoError(t, err)

	require.Equal(t, 2000, cfg.Context.BudgetTokens)
	require.Equal(t, 14, cfg.Context.RecentDays)
	require.Equal(t, "strict", cfg.Policy.PlanMutations)
	require.Equal(t, []string{"main"}, cfg.Policy.IntegrationBranches)
	require.Equal(t, 48, cfg.Claims.TTLHours)
}
