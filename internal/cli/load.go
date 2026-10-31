package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/halilibrahimonus/shelf/internal/artifactory"
	"github.com/halilibrahimonus/shelf/internal/conan"
	"github.com/halilibrahimonus/shelf/internal/config"
)

// session bundles what a data command needs after loading.
type session struct {
	cfg    *config.Config
	result *conan.LoadResult
	// repoLabel is shown in headers (repo names or "fixture").
	repoLabel string
}

// openSource returns the configured data source: a fixture file when
// --fixture is set, the Artifactory server otherwise.
func (a *app) openSource(cmd *cobra.Command) (*config.Config, conan.Source, string, error) {
	if a.flags.fixture != "" {
		cfg, err := a.loadConfig(cmd)
		if err != nil {
			return nil, nil, "", err
		}
		fx, err := artifactory.LoadFixture(a.flags.fixture)
		if err != nil {
			return nil, nil, "", err
		}
		// Use the fixture's own profiles unless the user chose a folder.
		if cfg.Sources[config.KeyProfilesDir] == config.SourceDefault {
			dir := filepath.Join(filepath.Dir(a.flags.fixture), "profiles")
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				cfg.ProfilesDir = dir
			}
		}
		return cfg, fx, "fixture: " + filepath.Base(a.flags.fixture), nil
	}
	cfg, c, err := a.newClient(cmd)
	if err != nil {
		return nil, nil, "", err
	}
	return cfg, c, "", nil
}

func (a *app) loadOptions(cfg *config.Config) conan.LoadOptions {
	cache := ""
	if a.flags.fixture == "" {
		cache = conan.DefaultCacheDir()
	}
	return conan.LoadOptions{
		Repos:          cfg.Repos,
		MetadataSource: cfg.MetadataSource,
		ProfilesDir:    cfg.ProfilesDir,
		CacheDir:       cache,
	}
}

// load fetches the catalog and prints load warnings to stderr.
func (a *app) load(cmd *cobra.Command) (*session, error) {
	cfg, src, label, err := a.openSource(cmd)
	if err != nil {
		return nil, err
	}
	res, err := conan.Load(cmd.Context(), src, a.loadOptions(cfg))
	if err != nil {
		return nil, err
	}
	printWarnings(cmd.ErrOrStderr(), a.newStyler(cmd.ErrOrStderr()), res.Warnings)
	if label == "" {
		label = strings.Join(cfg.Repos, ", ")
	}
	return &session{cfg: cfg, result: res, repoLabel: label}, nil
}
