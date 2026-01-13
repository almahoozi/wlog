package main

import (
	"fmt"
	"os"

	"github.com/almahoozi/wlog/internal/app"
)

var (
	commit  = "unknown"
	ref     = "unknown"
	version = "unknown"
)

func main() {
	info := app.BuildInfo{Commit: commit, Ref: ref, Version: version}
	if err := app.Run(os.Args[1:], info); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
