package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/hashgesh/shelf/internal/artifactory"
	"github.com/hashgesh/shelf/internal/conan"
	"github.com/hashgesh/shelf/internal/config"
	"github.com/hashgesh/shelf/internal/i18n"
)

// newClient loads and validates the configuration and builds a client.
func (a *app) newClient(cmd *cobra.Command) (*config.Config, *artifactory.Client, error) {
	cfg, err := a.loadConfig(cmd)
	if err != nil {
		return nil, nil, err
	}
	if err := cfg.Validate(); err != nil {
		return nil, nil, err
	}
	c, err := artifactory.New(cfg, artifactory.WithUserAgent("shelf/"+a.version))
	if err != nil {
		return nil, nil, err
	}
	return cfg, c, nil
}

// newDebugCmd holds hidden diagnostics for verifying a real environment.
func (a *app) newDebugCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "debug",
		Short:  i18n.T("Diagnostic commands"),
		Hidden: true,
		Args:   noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(a.newDebugPingCmd(), a.newDebugAQLCmd())
	return cmd
}

func (a *app) newDebugPingCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ping",
		Short: i18n.T("Test the server connection and AQL access for each repo"),
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, c, err := a.newClient(cmd)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if err := c.Ping(cmd.Context()); err != nil {
				return err
			}
			fmt.Fprintf(out, i18n.T("Server reachable: %s\n"), cfg.URL)
			for _, repo := range cfg.Repos {
				q := artifactory.ItemsQuery{Repos: []string{repo}, Include: []string{"repo", "path", "name"}, Limit: 1, NoSort: true}
				res, err := c.AQL(cmd.Context(), q.String())
				if err != nil {
					return fmt.Errorf("repo %s: %w", repo, err)
				}
				state := i18n.T("reachable")
				if len(res.Results) == 0 {
					state = i18n.T("reachable but empty (or no read permission)")
				}
				fmt.Fprintf(out, "Repo %s: %s\n", repo, state)
			}
			return nil
		},
	}
}

func (a *app) newDebugAQLCmd() *cobra.Command {
	var (
		query   string
		name    string
		limit   int
		outPath string
	)
	cmd := &cobra.Command{
		Use:   i18n.T("aql [flags]"),
		Short: i18n.T("Write the raw AQL response (to see the real property names and path layout)"),
		Example: "  shelf debug aql --limit 3 --out aql-sample.json\n" +
			"  shelf debug aql --name conaninfo.txt\n" +
			`  shelf debug aql --query 'items.find({"repo":"conan-local"}).include("name","path").limit(10)'`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, c, err := a.newClient(cmd)
			if err != nil {
				return err
			}
			if query == "" {
				q := artifactory.ItemsQuery{Repos: cfg.Repos, Limit: limit, NoSort: true}
				if name != "" {
					q.Names = []string{name}
				}
				query = q.String()
			}
			fmt.Fprintf(cmd.ErrOrStderr(), i18n.T("Query: %s\n"), query)

			raw, err := c.RawAQL(cmd.Context(), query)
			if err != nil {
				return err
			}
			var pretty bytes.Buffer
			if json.Indent(&pretty, raw, "", "  ") != nil {
				pretty.Reset()
				pretty.Write(raw)
			}
			pretty.WriteByte('\n')

			if outPath != "" {
				if err := os.WriteFile(outPath, pretty.Bytes(), 0o600); err != nil {
					return fmt.Errorf(i18n.T("cannot write file: %w"), err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), i18n.T("Wrote: %s (%d bytes)\n"), outPath, pretty.Len())
				return nil
			}
			_, err = cmd.OutOrStdout().Write(pretty.Bytes())
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&query, "query", "", i18n.T("raw AQL query to run (other flags are ignored when set)"))
	f.StringVar(&name, "name", conan.PackageFile, i18n.T("file name to search for (empty: all)"))
	f.IntVar(&limit, "limit", 5, i18n.T("maximum number of results"))
	f.StringVar(&outPath, "out", "", i18n.T("write the output to this file instead of the screen"))
	return cmd
}
