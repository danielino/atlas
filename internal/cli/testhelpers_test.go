package cli

import (
	"os"
	"testing"

	"github.com/dmarcocci/atlas/internal/testutil"
)

// chdir changes the working directory for the duration of the test and
// restores it afterward. cobra/CLI commands rely on os.Getwd(), so tests
// need to actually be "in" the temp repo, not merely have its path.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(old)
	})
}

// initRepo creates a fresh git repo, cds into it, and runs `atlas init`.
func initRepo(t *testing.T) string {
	t.Helper()
	dir := testutil.SetupRepo(t)
	chdir(t, dir)
	if _, _, code := ExecuteCapture([]string{"init"}); code != 0 {
		t.Fatalf("atlas init failed with code %d", code)
	}
	return dir
}
