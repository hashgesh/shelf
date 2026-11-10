package artifactory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hashgesh/shelf/internal/i18n"
)

// Fixture serves a saved AQL response instead of a live server. Files for
// Download are read from <dir of aql.json>/files/<repo>/<path>.
type Fixture struct {
	items    []Item
	filesDir string
}

// LoadFixture reads an AQL response JSON file (e.g. the output of
// `shelf debug aql --out`).
func LoadFixture(path string) (*Fixture, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf(i18n.T("cannot read fixture: %w"), err)
	}
	var res Result
	if err := json.Unmarshal(data, &res); err != nil {
		return nil, fmt.Errorf(i18n.T("cannot decode fixture: %s: %w"), path, err)
	}
	return &Fixture{items: res.Results, filesDir: filepath.Join(filepath.Dir(path), "files")}, nil
}

// SearchItems filters the fixture by repo and name; paging is ignored.
func (f *Fixture) SearchItems(_ context.Context, q ItemsQuery) ([]Item, error) {
	match := func(v string, set []string) bool {
		if len(set) == 0 {
			return true
		}
		for _, s := range set {
			if s == v {
				return true
			}
		}
		return false
	}
	var out []Item
	for _, it := range f.items {
		if match(it.Repo, q.Repos) && match(it.Name, q.Names) {
			out = append(out, it)
		}
	}
	return out, nil
}

// Download reads a file from the fixture's files directory.
func (f *Fixture) Download(_ context.Context, repo, path string) ([]byte, error) {
	rel := filepath.FromSlash(repo + "/" + strings.TrimLeft(path, "/"))
	if !filepath.IsLocal(rel) {
		return nil, fmt.Errorf(i18n.T("invalid fixture path: %s"), path)
	}
	data, err := os.ReadFile(filepath.Join(f.filesDir, rel))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%s/%s: %w", repo, path, ErrNotFound)
	}
	return data, err
}
