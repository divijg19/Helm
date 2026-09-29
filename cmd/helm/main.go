package main

import (
	"os"

	"github.com/divijg19/Helm/v2/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
