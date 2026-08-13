package main

import (
	"os"
	"path/filepath"

	"helm/internal/cli"
)

func main() {
	inv := cli.ResolveInvocation(filepath.Base(os.Args[0]))
	os.Exit(cli.Run(inv, os.Args[1:]))
}
