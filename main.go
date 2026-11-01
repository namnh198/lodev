package main

import (
	"os"

	"lodev/cmd"
	"lodev/internal/ui"
)

func main() {
	var on bool
	os.Args, on = cmd.StripDebugArg(os.Args)
	if on {
		ui.SetDebug(true)
	}
	if err := cmd.Execute(); err != nil {
		ui.Error("%v", err)
		os.Exit(1)
	}
}
