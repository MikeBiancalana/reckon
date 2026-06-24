package main

import (
	"os"

	"github.com/MikeBiancalana/reckon/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
