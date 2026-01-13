package tuiapp

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/almahoozi/wlog/internal/app"
)

// Run launches the daily log TUI. It loads the config before starting and returns
// any fatal error encountered while initializing or running the program.
func Run() error {
	cfg, err := app.LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "using default questions: %v\n", err)
	}
	return RunWithConfig(cfg)
}

// RunWithConfig is like Run but uses a provided config instance.
func RunWithConfig(cfg app.Config) error {
	mdl, err := newModel(cfg)
	if err != nil {
		return err
	}
	return runProgram(mdl)
}

func runProgram(m tea.Model) error {
	program := tea.NewProgram(m, tea.WithAltScreen())
	if err := program.Start(); err != nil && err != tea.ErrProgramKilled {
		return err
	}
	return nil
}
