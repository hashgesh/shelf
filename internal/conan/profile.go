package conan

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Profile is a Conan profile reduced to its [settings].
type Profile struct {
	Name     string            `json:"name"` // path relative to the profiles dir, "/"-separated
	Settings map[string]string `json:"settings"`
}

var includeRe = regexp.MustCompile(`^include\((.+)\)$`)

// LoadProfiles reads every profile under dir. Hidden files and folders are
// skipped. Problems with individual files are returned as warnings; a missing
// dir yields no profiles and no error.
func LoadProfiles(dir string) ([]Profile, []Warning, error) {
	var profiles []Profile
	var warnings []Warning
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == dir && errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			warnings = append(warnings, Warning{Path: p, Message: err.Error()})
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") && p != dir {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		rel, _ := filepath.Rel(dir, p)
		prof, warns, perr := loadProfile(dir, p, map[string]bool{})
		warnings = append(warnings, warns...)
		if perr != nil {
			warnings = append(warnings, Warning{Path: p, Message: perr.Error()})
			return nil
		}
		prof.Name = filepath.ToSlash(rel)
		profiles = append(profiles, prof)
		return nil
	})
	if err != nil {
		return nil, warnings, err
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Name < profiles[j].Name })
	return profiles, warnings, nil
}

// ParseProfile parses profile text without resolving includes.
func ParseProfile(name string, data []byte) (Profile, []string) {
	prof := Profile{Name: name, Settings: map[string]string{}}
	var includes []string
	for section, kvs := range parseINI(data) {
		for _, kv := range kvs {
			switch {
			case section == "" && includeRe.MatchString(kv.key):
				includes = append(includes, strings.TrimSpace(includeRe.FindStringSubmatch(kv.key)[1]))
			case section == "settings" && kv.value != "" && !strings.Contains(kv.key, ":"):
				// Package-scoped settings ("zlib/*:build_type=Debug") don't
				// describe the profile as a whole, so they are ignored.
				prof.Settings[kv.key] = kv.value
			}
		}
	}
	return prof, includes // in file order: later includes override earlier ones
}

// loadProfile reads file, resolving include(...) directives depth-first.
// Settings in the including profile override included ones.
func loadProfile(root, file string, visiting map[string]bool) (Profile, []Warning, error) {
	abs, _ := filepath.Abs(file)
	if visiting[abs] {
		return Profile{}, nil, fmt.Errorf("döngüsel include: %s", file)
	}
	visiting[abs] = true
	defer delete(visiting, abs)

	data, err := os.ReadFile(file)
	if err != nil {
		return Profile{}, nil, err
	}
	var warnings []Warning
	if strings.Contains(string(data), "{{") || strings.Contains(string(data), "{%") {
		warnings = append(warnings, Warning{Path: file, Message: "Jinja şablonu desteklenmiyor; şablon satırları yok sayıldı"})
	}
	own, includes := ParseProfile("", stripTemplateLines(data))

	merged := map[string]string{}
	for _, inc := range includes {
		incPath := inc
		if !filepath.IsAbs(incPath) {
			// Conan resolves includes relative to the including file first,
			// then to the profiles folder.
			incPath = filepath.Join(filepath.Dir(file), inc)
			if _, err := os.Stat(incPath); err != nil {
				incPath = filepath.Join(root, inc)
			}
		}
		p, warns, err := loadProfile(root, incPath, visiting)
		warnings = append(warnings, warns...)
		if err != nil {
			warnings = append(warnings, Warning{Path: file, Message: fmt.Sprintf("include(%s): %v", inc, err)})
			continue
		}
		for k, v := range p.Settings {
			merged[k] = v
		}
	}
	for k, v := range own.Settings {
		merged[k] = v
	}
	return Profile{Settings: merged}, warnings, nil
}

func stripTemplateLines(data []byte) []byte {
	lines := strings.Split(string(data), "\n")
	out := lines[:0]
	for _, l := range lines {
		if strings.Contains(l, "{{") || strings.Contains(l, "{%") || strings.Contains(l, "{#") {
			continue
		}
		out = append(out, l)
	}
	return []byte(strings.Join(out, "\n"))
}

// Matches reports whether b satisfies every setting of p. Setting values are
// compared case-insensitively. A setting the binary does not define at all is
// treated as compatible (Conan drops settings a recipe does not use from the
// package ID, e.g. compiler.cppstd for C libraries); a binary with no known
// settings matches nothing unless it is settings-free by design.
func (p Profile) Matches(b *Binary) bool {
	if len(p.Settings) == 0 {
		return false
	}
	if !b.HasSettings() {
		// Header-only packages have no settings and suit every profile; an
		// unknown binary (metadata missing) suits none.
		return b.MetadataSource != SourceNone
	}
	for k, want := range p.Settings {
		got, ok := b.Settings[k]
		if !ok {
			continue
		}
		if !strings.EqualFold(got, want) {
			return false
		}
	}
	return true
}

// specificity counts the profile settings the binary actually defines.
func (p Profile) specificity(b *Binary) int {
	n := 0
	for k := range p.Settings {
		if _, ok := b.Settings[k]; ok {
			n++
		}
	}
	return n
}

// MatchProfiles labels every binary in cat with the profiles it satisfies.
// Binary.Profile is the most specific match (most settings in common), with
// ties broken by name.
func MatchProfiles(cat *Catalog, profiles []Profile) {
	for _, b := range cat.Binaries() {
		AssignProfiles(b, profiles)
	}
}

// AssignProfiles sets b.Profiles and b.Profile from profiles.
func AssignProfiles(b *Binary, profiles []Profile) {
	b.Profiles, b.Profile = nil, ""
	best := -1
	for _, p := range profiles {
		if !p.Matches(b) {
			continue
		}
		b.Profiles = append(b.Profiles, p.Name)
		if s := p.specificity(b); s > best {
			best, b.Profile = s, p.Name
		}
	}
}
