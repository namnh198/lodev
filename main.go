package main

import (
	"os"

	"github.com/namnh198/lodev/cmd"
	"github.com/namnh198/lodev/pkg/util"
)

func main() {
	if os.Getuid() == 0 {
		util.Failed("LODEV is not designed to be run as root. Please run it as a regular user.")
		os.Exit(-1)
	}
	cmd.Execute()
}
