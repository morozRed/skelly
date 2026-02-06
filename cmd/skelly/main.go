package main

import (
	"os"

	"github.com/morozRed/skelly/internal/cli"
)

var version = "0.1.0-dev"

func main() {
	if err := cli.NewRootCommand(version).Execute(); err != nil {
		os.Exit(1)
	}
}
