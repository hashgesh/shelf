package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/hashgesh/shelf/internal/config"
	"github.com/hashgesh/shelf/internal/i18n"
)

func (a *app) newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: i18n.T("Create or show the configuration"),
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "init",
			Short: i18n.T("Create a config file interactively"),
			Args:  noArgs,
			RunE:  a.runConfigInit,
		},
		&cobra.Command{
			Use:   i18n.T("show [flags]"),
			Short: i18n.T("Show the effective configuration and where each value comes from (secrets are masked)"),
			Args:  noArgs,
			RunE:  a.runConfigShow,
		},
	)
	return cmd
}

// displayValue renders a setting for humans; secrets are never shown.
func displayValue(c *config.Config, key string) string {
	switch key {
	case config.KeyURL:
		return c.URL
	case config.KeyUser:
		return c.User
	case config.KeyToken:
		return secretState(c.Token)
	case config.KeyPassword:
		return secretState(c.Password)
	case config.KeyRepos:
		return strings.Join(c.Repos, ", ")
	case config.KeyProfilesDir:
		return c.ProfilesDir
	case config.KeyInsecureTLS:
		if c.InsecureTLS {
			return i18n.T("yes")
		}
		return i18n.T("no")
	case config.KeyTimeout:
		return c.Timeout.String()
	case config.KeyAuth:
		return c.Auth
	case config.KeyMetadataSource:
		return c.MetadataSource
	}
	return ""
}

func secretState(s config.Secret) string {
	if s.IsSet() {
		return s.String() + i18n.T(" (set)")
	}
	return ""
}

type configShowJSON struct {
	Path      string                   `json:"path"`
	FileFound bool                     `json:"file_found"`
	Values    configValuesJSON         `json:"values"`
	Sources   map[string]config.Source `json:"sources"`
	Valid     bool                     `json:"valid"`
	Error     string                   `json:"error,omitempty"`
}

type configValuesJSON struct {
	URL            string        `json:"url"`
	User           string        `json:"user"`
	Token          config.Secret `json:"token"`
	Password       config.Secret `json:"password"`
	Repos          []string      `json:"repos"`
	ProfilesDir    string        `json:"profiles_dir"`
	InsecureTLS    bool          `json:"insecure_tls"`
	Timeout        string        `json:"timeout"`
	Auth           string        `json:"auth"`
	MetadataSource string        `json:"metadata_source"`
}

func (a *app) runConfigShow(cmd *cobra.Command, _ []string) error {
	c, err := a.loadConfig(cmd)
	if err != nil {
		return err
	}
	verr := c.Validate()
	out := cmd.OutOrStdout()

	if a.flags.json {
		v := configShowJSON{
			Path:      c.Path,
			FileFound: c.FileFound,
			Values: configValuesJSON{
				URL: c.URL, User: c.User, Token: c.Token, Password: c.Password,
				Repos: c.Repos, ProfilesDir: c.ProfilesDir, InsecureTLS: c.InsecureTLS,
				Timeout: c.Timeout.String(), Auth: c.Auth, MetadataSource: c.MetadataSource,
			},
			Sources: c.Sources,
			Valid:   verr == nil,
		}
		if verr != nil {
			v.Error = verr.Error()
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}

	state := i18n.T("found")
	if !c.FileFound {
		state = i18n.T("not found")
	}
	fmt.Fprintf(out, i18n.T("Config file: %s (%s)\n\n"), c.Path, state)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, i18n.T("KEY\tVALUE\tSOURCE"))
	for _, k := range config.Keys {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", k, displayValue(c, k), i18n.T(string(c.Sources[k])))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(out)
	if verr != nil {
		fmt.Fprintf(out, i18n.T("Status: missing or invalid: %s\n"), strings.TrimPrefix(verr.Error(), config.ErrInvalid.Error()+": "))
	} else {
		fmt.Fprintln(out, i18n.T("Status: valid"))
	}
	return nil
}

func (a *app) runConfigInit(cmd *cobra.Command, _ []string) error {
	path := a.flags.configPath
	if path == "" {
		if p, ok := a.lookupEnv(config.EnvConfig); ok && p != "" {
			path = p
		}
	}
	// Defaults come from the existing file only; values from the environment
	// or flags are deliberately not persisted.
	cur, err := config.Load(config.Options{
		Path:         path,
		LookupEnv:    func(string) (string, bool) { return "", false },
		AllowMissing: true,
	})
	if err != nil {
		return err
	}
	path = cur.Path

	p := newPrompter(cmd.InOrStdin(), cmd.OutOrStdout())
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, i18n.T("shelf configuration"))
	fmt.Fprintf(out, i18n.T("File: %s\n"), path)
	if cur.FileFound {
		fmt.Fprintln(out, i18n.T("Current values are shown in brackets; press Enter to keep them."))
	}
	fmt.Fprintln(out)

	next := *cur
	for {
		v, err := p.ask(i18n.T("Artifactory URL (e.g. https://artifactory.example/artifactory)"), cur.URL)
		if err != nil {
			return err
		}
		if u, perr := url.Parse(v); perr == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			next.URL = strings.TrimRight(v, "/")
			break
		}
		fmt.Fprintln(out, i18n.T("  Enter a valid http:// or https:// address."))
	}

	if next.User, err = p.ask(i18n.T("User name (leave empty for anonymous access)"), cur.User); err != nil {
		return err
	}

	method := "token"
	if cur.Password.IsSet() && !cur.Token.IsSet() {
		method = "password"
	}
	for {
		v, err := p.ask(i18n.T("Authentication method: token or password"), method)
		if err != nil {
			return err
		}
		v = strings.ToLower(v)
		if v == "token" || v == "password" {
			method = v
			break
		}
		fmt.Fprintln(out, i18n.T("  Type \"token\" or \"password\"."))
	}

	if method == "token" {
		s, err := p.askSecret("Access token", cur.Token.IsSet())
		if err != nil {
			return err
		}
		if s != "" {
			next.Token = config.Secret(s)
		}
		next.Password = ""
		next.Auth = config.AuthAuto
	} else {
		if next.User == "" {
			return fmt.Errorf(i18n.T("%w: password login requires a user name"), config.ErrInvalid)
		}
		s, err := p.askSecret(i18n.T("Password"), cur.Password.IsSet())
		if err != nil {
			return err
		}
		if s != "" {
			next.Password = config.Secret(s)
		}
		next.Token = ""
		next.Auth = config.AuthBasic
	}

	repos, err := p.ask(i18n.T("Repo(s), comma-separated"), strings.Join(cur.Repos, ","))
	if err != nil {
		return err
	}
	next.Repos = nil
	for _, r := range strings.Split(repos, ",") {
		if r = strings.TrimSpace(r); r != "" {
			next.Repos = append(next.Repos, r)
		}
	}

	// The default follows CONAN_HOME, so it is offered but not written to the
	// file: a saved copy would pin the folder and hide later CONAN_HOME changes.
	profilesDefault := cur.ProfilesDir
	if cur.Sources[config.KeyProfilesDir] == config.SourceDefault {
		profilesDefault = config.DefaultProfilesDir(a.lookupEnv)
	}
	if next.ProfilesDir, err = p.ask(i18n.T("Conan profiles folder"), profilesDefault); err != nil {
		return err
	}
	if next.ProfilesDir == config.DefaultProfilesDir(a.lookupEnv) {
		next.ProfilesDir = ""
	}
	if next.InsecureTLS, err = p.confirm(i18n.T("Disable TLS certificate verification (self-signed certificate)?"), cur.InsecureTLS); err != nil {
		return err
	}
	for {
		v, err := p.ask(i18n.T("Request timeout"), cur.Timeout.String())
		if err != nil {
			return err
		}
		d, perr := config.ParseTimeout(v)
		if perr == nil {
			next.Timeout = d
			break
		}
		fmt.Fprintln(out, i18n.T("  For example: 30s, 2m or a number of seconds."))
	}

	if err := next.Validate(); err != nil {
		return err
	}
	ok, err := p.confirm(fmt.Sprintf(i18n.T("Save to %s?"), path), true)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out, i18n.T("Cancelled, nothing was saved."))
		return nil
	}
	if err := config.Save(path, &next); err != nil {
		return err
	}
	fmt.Fprintf(out, i18n.T("Saved: %s\n"), path)
	return nil
}
