package cli

import (
	"context"
	"errors"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/hashgesh/shelf/internal/clipboard"
	"github.com/hashgesh/shelf/internal/conan"
	"github.com/hashgesh/shelf/internal/i18n"
	"github.com/hashgesh/shelf/internal/tui"
)

// runTUI opens the interactive browser. Configuration errors surface before
// the screen switches, so they keep their exit code.
func (a *app) runTUI(cmd *cobra.Command) error {
	out, ok := cmd.OutOrStdout().(*os.File)
	if !ok || !term.IsTerminal(int(out.Fd())) {
		return errors.New(i18n.T("the interactive interface needs a terminal; use 'shelf list' or 'shelf show' in scripts"))
	}
	cfg, src, label, err := a.openSource(cmd)
	if err != nil {
		return err
	}
	if label == "" {
		label = strings.Join(cfg.Repos, ", ")
	}
	opts := a.loadOptions(cfg)
	noColor := a.flags.noColor
	if v, ok := a.lookupEnv("NO_COLOR"); ok && v != "" {
		noColor = true
	}
	return tui.Run(tui.Options{
		Load: func(ctx context.Context) (*conan.LoadResult, error) {
			return conan.Load(ctx, src, opts)
		},
		Copy:      clipboard.New(out).Copy,
		RepoLabel: label,
		ASCII:     a.flags.ascii,
		NoColor:   noColor,
		Output:    out,
	})
}
