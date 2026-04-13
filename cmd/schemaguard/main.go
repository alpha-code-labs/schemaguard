// Command schemaguard is the SchemaGuard CLI entry point. It delegates all
// work to the internal/cli package so the binary wrapper stays a one-liner
// and the cli package can be exercised directly from tests.
package main

import (
	"os"

	"github.com/schemaguard/schemaguard/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args, os.Stdout, os.Stderr))
}
