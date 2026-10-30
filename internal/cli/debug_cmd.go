package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/halilibrahimonus/shelf/internal/artifactory"
	"github.com/halilibrahimonus/shelf/internal/conan"
	"github.com/halilibrahimonus/shelf/internal/config"
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
		Short:  "Tanılama komutları",
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
		Short: "Sunucu bağlantısını ve her repo için AQL erişimini dene",
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
			fmt.Fprintf(out, "Sunucu erişilebilir: %s\n", cfg.URL)
			for _, repo := range cfg.Repos {
				q := artifactory.ItemsQuery{Repos: []string{repo}, Include: []string{"repo", "path", "name"}, Limit: 1}
				res, err := c.AQL(cmd.Context(), q.String())
				if err != nil {
					return fmt.Errorf("repo %s: %w", repo, err)
				}
				state := "erişilebilir"
				if len(res.Results) == 0 {
					state = "erişilebilir ama boş (veya okuma izni yok)"
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
		Use:   "aql [bayraklar]",
		Short: "Ham AQL yanıtını yaz (gerçek property adlarını ve path düzenini görmek için)",
		Example: "  shelf debug aql --limit 3 --out aql-ornek.json\n" +
			"  shelf debug aql --name conaninfo.txt\n" +
			`  shelf debug aql --query 'items.find({"repo":"conan-local"}).include("name","path").limit(10)'`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, c, err := a.newClient(cmd)
			if err != nil {
				return err
			}
			if query == "" {
				q := artifactory.ItemsQuery{Repos: cfg.Repos, Limit: limit}
				if name != "" {
					q.Names = []string{name}
				}
				query = q.String()
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "Sorgu: %s\n", query)

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
					return fmt.Errorf("dosya yazılamadı: %w", err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Yazıldı: %s (%d bayt)\n", outPath, pretty.Len())
				return nil
			}
			_, err = cmd.OutOrStdout().Write(pretty.Bytes())
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&query, "query", "", "çalıştırılacak ham AQL sorgusu (verilirse diğer bayraklar yok sayılır)")
	f.StringVar(&name, "name", conan.PackageFile, "aranacak dosya adı (boş: tümü)")
	f.IntVar(&limit, "limit", 5, "en fazla kaç sonuç")
	f.StringVar(&outPath, "out", "", "çıktıyı ekran yerine bu dosyaya yaz")
	return cmd
}
