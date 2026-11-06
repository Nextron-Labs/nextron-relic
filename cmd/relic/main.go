// Command relic is the Nextron Relic forensic artifact collector.
package main

import (
	"os"

	"github.com/Nextron-Labs/nextron-relic/internal/cli"
)

func main() {
	os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr))
}
