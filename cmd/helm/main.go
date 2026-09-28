package main

import (
	"os"

	"github.com/divijg19/Helm/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
