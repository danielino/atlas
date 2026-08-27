// Command atlas is the ATLAS CLI entrypoint. All behavior lives in
// internal/cli; this file only wires os.Args/os.Stdout/os.Stderr and
// converts the returned exit code into a process exit.
package main

import (
	"os"

	"github.com/danielino/atlas/internal/cli"
)

func main() {
	code := cli.Execute(os.Args[1:], os.Stdout, os.Stderr)
	os.Exit(code)
}
