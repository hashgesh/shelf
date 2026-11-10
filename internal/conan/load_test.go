package conan

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/hashgesh/shelf/internal/artifactory"
	"github.com/hashgesh/shelf/internal/config"
)

// countingSource wraps a fixture and counts downloads.
type countingSource struct {
	*artifactory.Fixture
	mu        sync.Mutex
	downloads int
}

func (c *countingSource) Download(ctx context.Context, repo, path string) ([]byte, error) {
	c.mu.Lock()
	c.downloads++
	c.mu.Unlock()
	return c.Fixture.Download(ctx, repo, path)
}

func newCountingSource(t *testing.T) *countingSource {
	fx, err := artifactory.LoadFixture(filepath.Join(fixtureDir, "aql.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &countingSource{Fixture: fx}
}

func TestLoadMetadataModes(t *testing.T) {
	tests := []struct {
		mode          string
		wantDownloads int
		wantUnknown   int
	}{
		{config.MetadataAuto, 2, 0},
		{config.MetadataProperties, 0, 2},
		{config.MetadataConaninfo, 24, 0}, // one per latest binary (25 items, one superseded prev)
	}
	for _, tt := range tests {
		t.Run(tt.mode, func(t *testing.T) {
			src := newCountingSource(t)
			res, err := Load(context.Background(), src, LoadOptions{
				Repos: []string{"conan-local"}, MetadataSource: tt.mode,
				ProfilesDir: filepath.Join(fixtureDir, "profiles"),
			})
			if err != nil {
				t.Fatal(err)
			}
			if src.downloads != tt.wantDownloads {
				t.Errorf("downloads = %d, want %d", src.downloads, tt.wantDownloads)
			}
			unknown := 0
			for _, b := range res.Catalog.Binaries() {
				if b.MetadataSource == SourceNone {
					unknown++
				}
			}
			if unknown != tt.wantUnknown {
				t.Errorf("unknown = %d, want %d", unknown, tt.wantUnknown)
			}
			if len(res.Profiles) != 5 || len(res.Warnings) != 0 {
				t.Errorf("profiles=%d warnings=%v", len(res.Profiles), res.Warnings)
			}
		})
	}
}

func TestLoadCachesConaninfo(t *testing.T) {
	cache := t.TempDir()
	opts := LoadOptions{Repos: []string{"conan-local"}, MetadataSource: config.MetadataAuto, CacheDir: cache}

	src := newCountingSource(t)
	if _, err := Load(context.Background(), src, opts); err != nil {
		t.Fatal(err)
	}
	if src.downloads != 2 {
		t.Fatalf("first load downloads = %d", src.downloads)
	}
	src2 := newCountingSource(t)
	res, err := Load(context.Background(), src2, opts)
	if err != nil {
		t.Fatal(err)
	}
	if src2.downloads != 0 {
		t.Errorf("second load downloads = %d, want 0 (cached)", src2.downloads)
	}
	for _, b := range res.Catalog.Binaries() {
		if b.MetadataSource == SourceNone {
			t.Errorf("%s still unknown after cached load", b)
		}
	}
}

type failingSource struct{ *artifactory.Fixture }

func (failingSource) Download(context.Context, string, string) ([]byte, error) {
	return nil, artifactory.ErrNotFound
}

func TestLoadWarnsWhenConaninfoMissing(t *testing.T) {
	src := newCountingSource(t)
	res, err := Load(context.Background(), failingSource{src.Fixture}, LoadOptions{Repos: []string{"conan-local"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Warnings) != 2 || !strings.Contains(res.Warnings[0].Message, "bulunamadı") {
		t.Errorf("warnings = %v", res.Warnings)
	}
}

func TestLoadCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	src := newCountingSource(t)
	if _, err := Load(ctx, src, LoadOptions{Repos: []string{"conan-local"}, MetadataSource: config.MetadataConaninfo}); err == nil {
		t.Error("expected context error")
	}
}

func TestDefaultCacheDir(t *testing.T) {
	if d := DefaultCacheDir(); d != "" && !strings.HasSuffix(d, filepath.Join("shelf", "conaninfo")) {
		t.Errorf("DefaultCacheDir = %q", d)
	}
}
