package contextc

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "update golden files")

func goldenPath(name string) string {
	return filepath.Join("testdata", name)
}

// compareGolden compares got against the content of testdata/<name>. With
// -update it (re)writes the file from got instead of comparing, so goldens
// can be regenerated deliberately after a reviewed behavior change.
func compareGolden(t *testing.T, name string, got string) {
	t.Helper()
	path := goldenPath(name)

	if *updateGolden {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}

	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden file %s missing; run tests with -update to create it", path)
	require.Equal(t, string(want), got, "golden mismatch for %s (run with -update to inspect/regenerate)", path)
}
