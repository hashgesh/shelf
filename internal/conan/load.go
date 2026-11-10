package conan

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/hashgesh/shelf/internal/artifactory"
	"github.com/hashgesh/shelf/internal/config"
)

// Source is what Load needs from Artifactory; *artifactory.Client and
// *artifactory.Fixture implement it.
type Source interface {
	SearchItems(ctx context.Context, q artifactory.ItemsQuery) ([]artifactory.Item, error)
	Download(ctx context.Context, repo, path string) ([]byte, error)
}

// LoadOptions controls Load.
type LoadOptions struct {
	Repos          []string
	MetadataSource string // config.Metadata*
	ProfilesDir    string
	// CacheDir stores downloaded conaninfo.txt files. Package revisions are
	// content hashes, so cached files never go stale. Empty disables caching.
	CacheDir string
	// Concurrency bounds parallel conaninfo downloads (default 8).
	Concurrency int
	Layout      *Layout
}

// LoadResult is everything the CLI and TUI display.
type LoadResult struct {
	Catalog  *Catalog
	Profiles []Profile
	Warnings []Warning
}

// Load fetches all package and recipe items with one (paged) AQL search, builds the
// catalog, fills missing metadata from conaninfo.txt as configured and
// matches profiles.
func Load(ctx context.Context, src Source, opts LoadOptions) (*LoadResult, error) {
	// Recipe files come along only to date the recipe revisions.
	items, err := src.SearchItems(ctx, artifactory.ItemsQuery{Repos: opts.Repos, Names: []string{PackageFile, RecipeFile}})
	if err != nil {
		return nil, err
	}
	cat, warnings := BuildCatalog(items, opts.Layout)

	var targets []*Binary
	for _, b := range cat.Binaries() {
		switch opts.MetadataSource {
		case config.MetadataConaninfo:
			targets = append(targets, b)
		case config.MetadataProperties:
		default: // auto
			if b.MetadataSource == SourceNone {
				targets = append(targets, b)
			}
		}
	}
	warnings = append(warnings, fetchConaninfo(ctx, src, targets, opts)...)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	for _, v := range allRevisions(cat) {
		SortBinaries(v.Binaries)
	}

	res := &LoadResult{Catalog: cat, Warnings: warnings}
	if opts.ProfilesDir != "" {
		profiles, pw, err := LoadProfiles(opts.ProfilesDir)
		res.Warnings = append(res.Warnings, pw...)
		if err != nil {
			res.Warnings = append(res.Warnings, Warning{Path: opts.ProfilesDir, Message: err.Error()})
		}
		res.Profiles = profiles
		MatchProfiles(cat, profiles)
	}
	return res, nil
}

func allRevisions(cat *Catalog) []*Revision {
	var out []*Revision
	for _, p := range cat.Packages {
		for _, v := range p.Versions {
			out = append(out, v.Revisions...)
		}
	}
	return out
}

func fetchConaninfo(ctx context.Context, src Source, targets []*Binary, opts LoadOptions) []Warning {
	if len(targets) == 0 {
		return nil
	}
	n := opts.Concurrency
	if n <= 0 {
		n = 8
	}
	var (
		mu       sync.Mutex
		warnings []Warning
		wg       sync.WaitGroup
		sem      = make(chan struct{}, n)
	)
	for _, b := range targets {
		wg.Add(1)
		go func(b *Binary) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			data, err := readConaninfo(ctx, src, b, opts.CacheDir)
			if err != nil {
				msg := fmt.Sprintf("conaninfo.txt alınamadı: %v", err)
				if errors.Is(err, artifactory.ErrNotFound) {
					msg = "conaninfo.txt bulunamadı; ayarlar bilinmiyor"
				}
				mu.Lock()
				warnings = append(warnings, Warning{Path: b.Repo + "/" + b.ConaninfoPath(), Message: msg})
				mu.Unlock()
				return
			}
			b.ApplyConaninfo(ParseConaninfo(data))
		}(b)
	}
	wg.Wait()
	return warnings
}

func readConaninfo(ctx context.Context, src Source, b *Binary, cacheDir string) ([]byte, error) {
	var cachePath string
	if cacheDir != "" {
		rel := filepath.FromSlash(b.Repo + "/" + b.ConaninfoPath())
		if filepath.IsLocal(rel) {
			cachePath = filepath.Join(cacheDir, rel)
			if data, err := os.ReadFile(cachePath); err == nil {
				return data, nil
			}
		}
	}
	data, err := src.Download(ctx, b.Repo, b.ConaninfoPath())
	if err != nil {
		return nil, err
	}
	if cachePath != "" {
		// Best effort: a cache write failure must not fail the load.
		if os.MkdirAll(filepath.Dir(cachePath), 0o700) == nil {
			_ = os.WriteFile(cachePath, data, 0o600)
		}
	}
	return data, nil
}

// DefaultCacheDir returns the per-user cache folder for conaninfo files.
func DefaultCacheDir() string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "shelf", "conaninfo")
}
