// Package cli implements the shelf command-line interface.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/hashgesh/shelf/internal/artifactory"
	"github.com/hashgesh/shelf/internal/config"
	"github.com/hashgesh/shelf/internal/i18n"
)

// Exit codes.
const (
	ExitOK       = 0
	ExitNoResult = 1 // also used for unexpected errors
	ExitConfig   = 2 // configuration or authentication error
)

// BuildInfo is injected by main from ldflags.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// globalFlags are persistent flags shared by every command.
type globalFlags struct {
	configPath  string
	url         string
	user        string
	token       string
	repos       []string
	profilesDir string
	insecure    bool
	timeout     time.Duration

	fixture string // hidden: read an AQL response file instead of a server

	json    bool
	ascii   bool
	noColor bool
}

// app holds per-invocation state; tests construct it with fake streams.
type app struct {
	flags     globalFlags
	stdin     io.Reader
	stdout    io.Writer
	stderr    io.Writer
	lookupEnv func(string) (string, bool)
	version   string
}

// Execute runs the CLI and returns the process exit code.
func Execute(info BuildInfo) int {
	a := &app{stdin: os.Stdin, stdout: os.Stdout, stderr: os.Stderr, lookupEnv: os.LookupEnv}
	i18n.Detect(a.lookupEnv)
	return a.run(info, os.Args[1:])
}

func (a *app) run(info BuildInfo, args []string) int {
	a.version = info.Version
	root := a.newRootCmd(info)
	root.SetArgs(args)
	root.SetIn(a.stdin)
	root.SetOut(a.stdout)
	root.SetErr(a.stderr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err := root.ExecuteContext(ctx)
	if err == nil {
		return ExitOK
	}
	var nr noResultError
	if errors.As(err, &nr) {
		if nr.msg != "" {
			fmt.Fprintln(a.stderr, nr.msg)
		}
		return ExitNoResult
	}
	fmt.Fprintf(a.stderr, i18n.T("shelf: error: %v\n"), err)
	if errors.Is(err, config.ErrInvalid) || errors.Is(err, artifactory.ErrAuth) {
		return ExitConfig
	}
	return ExitNoResult
}

// noResultError signals "the query succeeded but matched nothing" (exit 1).
type noResultError struct{ msg string }

func (e noResultError) Error() string { return e.msg }

func (a *app) newRootCmd(info BuildInfo) *cobra.Command {
	root := &cobra.Command{
		Use:   "shelf",
		Short: i18n.T("Browse Conan 2 packages in Artifactory from the terminal"),
		Long: i18n.T("shelf lists the Conan 2 packages and binaries in JFrog Artifactory in a\n") +
			i18n.T("readable form. Run without arguments to open the interactive interface."),
		Version:           fmt.Sprintf(i18n.T("%s (commit %s, built %s)"), info.Version, info.Commit, info.Date),
		SilenceErrors:     true,
		SilenceUsage:      true,
		Args:              noArgs,
		CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.runTUI(cmd)
		},
	}
	root.SetVersionTemplate("shelf {{.Version}}\n")

	pf := root.PersistentFlags()
	f := &a.flags
	pf.StringVar(&f.configPath, "config", "", i18n.T("config file (default: platform-specific location, $")+config.EnvConfig+")")
	pf.StringVar(&f.url, "url", "", i18n.T("Artifactory URL ($")+config.EnvURL+")")
	pf.StringVar(&f.user, "user", "", i18n.T("user name ($")+config.EnvUser+")")
	pf.StringVar(&f.token, "token", "", i18n.T("access token; visible in the process list, prefer $")+config.EnvToken+i18n.T(" instead"))
	pf.StringSliceVar(&f.repos, "repo", nil, i18n.T("repo name, may be repeated ($")+config.EnvRepos+i18n.T(", comma-separated)"))
	pf.StringVar(&f.profilesDir, "profiles-dir", "", i18n.T("Conan profiles folder (default: ~/.conan2/profiles)"))
	pf.BoolVar(&f.insecure, "insecure", false, i18n.T("disable TLS certificate verification"))
	pf.DurationVar(&f.timeout, "timeout", 0, i18n.T("request timeout (e.g. 30s)"))
	pf.BoolVar(&f.json, "json", false, i18n.T("write the output as JSON"))
	pf.BoolVar(&f.ascii, "ascii", false, i18n.T("use ASCII characters such as +/- instead of ✓/✗"))
	pf.BoolVar(&f.noColor, "no-color", false, i18n.T("disable colors"))
	pf.BoolP("help", "h", false, i18n.T("show help"))
	pf.StringVar(&f.fixture, "fixture", "", i18n.T("read an AQL response file instead of a server (for development)"))
	_ = pf.MarkHidden("fixture")
	root.Flags().BoolP("version", "v", false, i18n.T("show version information"))

	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return fmt.Errorf(i18n.T("%w (see: %s --help)"), err, c.CommandPath())
	})
	root.AddCommand(a.newListCmd(), a.newShowCmd(), a.newMatrixCmd(), a.newConfigCmd(), a.newDebugCmd())
	root.SetHelpCommand(newHelpCmd())
	localize(root)
	return root
}

// overrides converts explicitly set flags into config overrides.
func (a *app) overrides(cmd *cobra.Command) config.Overrides {
	var o config.Overrides
	fs := cmd.Flags()
	f := &a.flags
	if fs.Changed("url") {
		o.URL = &f.url
	}
	if fs.Changed("user") {
		o.User = &f.user
	}
	if fs.Changed("token") {
		o.Token = &f.token
	}
	if fs.Changed("repo") {
		o.Repos = f.repos
	}
	if fs.Changed("profiles-dir") {
		o.ProfilesDir = &f.profilesDir
	}
	if fs.Changed("insecure") {
		o.InsecureTLS = &f.insecure
	}
	if fs.Changed("timeout") {
		o.Timeout = &f.timeout
	}
	return o
}

// loadConfig returns the effective configuration for cmd.
func (a *app) loadConfig(cmd *cobra.Command) (*config.Config, error) {
	return config.Load(config.Options{
		Path:      a.flags.configPath,
		LookupEnv: a.lookupEnv,
		Flags:     a.overrides(cmd),
	})
}
