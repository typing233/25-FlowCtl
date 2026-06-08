package main

import (
	"os"

	"github.com/flowctl/flowctl/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
