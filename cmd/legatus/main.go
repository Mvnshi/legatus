// Command legatus runs coding agents in parallel on your own machine and never stops at a usage limit.
package main

import (
	"os"

	"github.com/Mvnshi/legatus/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
