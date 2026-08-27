package ledger

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config holds ATLAS project configuration, loaded from .atlas/config.toml.
// All fields are optional in the file; DefaultConfig provides the values
// used when the file is absent or a section/key is omitted.
type Config struct {
	Context ContextConfig `toml:"context"`
	Policy  PolicyConfig  `toml:"policy"`
	Claims  ClaimsConfig  `toml:"claims"`
}

type ContextConfig struct {
	BudgetTokens int `toml:"budget_tokens"`
	RecentDays   int `toml:"recent_days"`
}

type PolicyConfig struct {
	PlanMutations       string   `toml:"plan_mutations"`
	IntegrationBranches []string `toml:"integration_branches"`
}

type ClaimsConfig struct {
	TTLHours int `toml:"ttl_hours"`
}

// DefaultConfig returns the built-in defaults per S1.
func DefaultConfig() Config {
	return Config{
		Context: ContextConfig{
			BudgetTokens: 1500,
			RecentDays:   7,
		},
		Policy: PolicyConfig{
			PlanMutations:       "warn",
			IntegrationBranches: []string{"main", "develop"},
		},
		Claims: ClaimsConfig{
			TTLHours: 24,
		},
	}
}

// LoadConfig loads .atlas/config.toml under root, starting from
// DefaultConfig and overlaying whatever the file specifies. A missing file
// is not an error: the defaults are returned as-is.
func LoadConfig(root string) (Config, error) {
	cfg := DefaultConfig()

	path := filepath.Join(root, ".atlas", "config.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}

	if err := toml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
