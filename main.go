package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/almahoozi/wlog/internal/app"
	"github.com/almahoozi/wlog/internal/tuiapp"
)

var (
	commit  = "unknown"
	ref     = "unknown"
	version = "unknown"
)

func main() {
	args := os.Args[1:]
	info := app.BuildInfo{Commit: commit, Ref: ref, Version: version}

	if len(args) == 0 {
		runTUI()
		return
	}

	switch args[0] {
	case "config":
		runConfigTUI()
	case "help", "-h", "--help":
		printTUIHelp()
	default:
		if err := app.Run(args, info); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

func runTUI() {
	if err := tuiapp.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runConfigTUI() {
	if err := tuiapp.RunConfigEditor(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func printTUIHelp() {
	fmt.Println(strings.TrimSpace(`wlog - a simple work log

Usage:
  wlog                 Launch the TUI
  wlog config          Configure wlog via the TUI
  wlog version         Show build metadata
  wlog ls              Print the log storage directory path
  wlog ls config       Print the config file path
  wlog cat [interval]  Print the list view for today or a plain-english period
  wlog help            Show this help message

Tip: Press h in the TUI to toggle on-screen hints.`))
}
