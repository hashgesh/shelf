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

	"github.com/halilibrahimonus/shelf/internal/artifactory"
	"github.com/halilibrahimonus/shelf/internal/config"
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
	fmt.Fprintf(a.stderr, "shelf: hata: %v\n", err)
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
		Short: "Artifactory'deki Conan 2 paketlerini terminalden gezin",
		Long: "shelf, JFrog Artifactory'deki Conan 2 paketlerini ve binary'lerini\n" +
			"okunur şekilde listeler. Parametresiz çalıştırıldığında etkileşimli arayüz açılır.",
		Version:           fmt.Sprintf("%s (commit %s, derleme %s)", info.Version, info.Commit, info.Date),
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
	pf.StringVar(&f.configPath, "config", "", "config dosyası (varsayılan: platforma özgü konum, $"+config.EnvConfig+")")
	pf.StringVar(&f.url, "url", "", "Artifactory URL'si ($"+config.EnvURL+")")
	pf.StringVar(&f.user, "user", "", "kullanıcı adı ($"+config.EnvUser+")")
	pf.StringVar(&f.token, "token", "", "access token; süreç listesinde görünür, $"+config.EnvToken+" tercih edin")
	pf.StringSliceVar(&f.repos, "repo", nil, "repo adı, birden fazla verilebilir ($"+config.EnvRepos+", virgülle ayrılmış)")
	pf.StringVar(&f.profilesDir, "profiles-dir", "", "Conan profil klasörü (varsayılan: ~/.conan2/profiles)")
	pf.BoolVar(&f.insecure, "insecure", false, "TLS sertifika doğrulamasını kapat")
	pf.DurationVar(&f.timeout, "timeout", 0, "istek zaman aşımı (örnek: 30s)")
	pf.BoolVar(&f.json, "json", false, "çıktıyı JSON olarak yaz")
	pf.BoolVar(&f.ascii, "ascii", false, "✓/✗ yerine +/- gibi ASCII karakterler kullan")
	pf.BoolVar(&f.noColor, "no-color", false, "renkleri kapat")
	pf.BoolP("help", "h", false, "yardımı göster")
	pf.StringVar(&f.fixture, "fixture", "", "sunucu yerine AQL yanıt dosyası kullan (geliştirme için)")
	_ = pf.MarkHidden("fixture")
	root.Flags().BoolP("version", "v", false, "sürüm bilgisini göster")

	root.SetFlagErrorFunc(func(c *cobra.Command, err error) error {
		return fmt.Errorf("%w (yardım için: %s --help)", err, c.CommandPath())
	})
	root.AddCommand(a.newListCmd(), a.newShowCmd(), a.newConfigCmd(), a.newDebugCmd())
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
