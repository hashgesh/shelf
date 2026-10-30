// Package tui implements the interactive terminal browser.
package tui

import (
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// Options configures Run.
type Options struct {
	Load      Loader
	Copy      CopyFunc // nil disables the c key
	RepoLabel string
	ASCII     bool
	NoColor   bool
	Input     io.Reader // defaults to os.Stdin
	Output    io.Writer // defaults to os.Stdout
}

// Run starts the TUI and blocks until the user quits.
func Run(opts Options) error {
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}
	r := lipgloss.NewRenderer(out)
	if opts.NoColor {
		r.SetColorProfile(termenv.Ascii)
	}
	m := newModel(opts.Load, opts.RepoLabel, newStyles(r, opts.ASCII))
	m.copy = opts.Copy

	progOpts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithOutput(out)}
	if opts.Input != nil {
		progOpts = append(progOpts, tea.WithInput(opts.Input))
	}
	_, err := tea.NewProgram(m, progOpts...).Run()
	return err
}
