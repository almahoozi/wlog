package main

import (
	"fmt"
	"os"

	"github.com/almahoozi/wlog/internal/tuiapp"
)

func main() {
	if err := tuiapp.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
