package main

import (
	"os"

	"github.com/savecomdev/blockchain-pow-go/cli"
)

func main() {
	defer os.Exit(0)

	// create the command line struct
	cmd := cli.CommandLine{}
	cmd.Run()
}
