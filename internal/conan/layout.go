// Package conan turns Artifactory items into a Conan 2 package model and
// matches binaries against Conan profiles.
package conan

import (
	"fmt"
	"regexp"
	"strings"
)

// Everything that depends on how Artifactory stores Conan 2 packages lives in
// this file. If a real server differs, change the constants below.

// File names inside a package revision folder.
const (
	PackageFile   = "conan_package.tgz"
	ConaninfoFile = "conaninfo.txt"
)

// RecipeFile is a file every recipe revision has exactly once, in its export
// folder. Its upload time dates the recipe revision.
const RecipeFile = "conanfile.py"

// EmptyField is how Artifactory stores a missing user or channel in paths.
const EmptyField = "_"

// PathLayout is the folder layout of a package revision, relative to the
// repository root. Placeholders: {user} {name} {version} {channel} {rrev}
// {pkgid} {prev}. Every other segment must match literally.
const PathLayout = "{user}/{name}/{version}/{channel}/{rrev}/package/{pkgid}/{prev}"

// RecipeLayout is the folder holding RecipeFile, with the same placeholders
// (without {pkgid} and {prev}).
const RecipeLayout = "{user}/{name}/{version}/{channel}/{rrev}/export"

// Property keys Artifactory attaches to Conan artifacts.
const (
	PropName           = "conan.package.name"
	PropVersion        = "conan.package.version"
	PropUser           = "conan.package.user"
	PropChannel        = "conan.package.channel"
	PropSettingsPrefix = "conan.settings."
	PropOptionsPrefix  = "conan.options."
)

// Setting keys (as in Conan profiles and conaninfo.txt).
const (
	SettingOS              = "os"
	SettingArch            = "arch"
	SettingCompiler        = "compiler"
	SettingCompilerVersion = "compiler.version"
	SettingBuildType       = "build_type"
)

// OptionShared is the conventional option for shared/static libraries.
const OptionShared = "shared"

// PathInfo is what the storage path tells about a binary.
type PathInfo struct {
	User, Name, Version, Channel string
	RecipeRevision               string
	PackageID                    string
	PackageRevision              string
}

// Layout parses storage paths according to a PathLayout-style template.
type Layout struct {
	template string
	re       *regexp.Regexp
	fields   []string
}

var placeholderRe = regexp.MustCompile(`^\{(user|name|version|channel|rrev|pkgid|prev)\}$`)

// NewLayout compiles a template. Each of {name} {version} {rrev} {pkgid}
// {prev} must appear exactly once; {user} and {channel} are optional.
func NewLayout(template string) (*Layout, error) {
	return newLayout(template, "name", "version", "rrev", "pkgid", "prev")
}

func newLayout(template string, required ...string) (*Layout, error) {
	segs := strings.Split(strings.Trim(template, "/"), "/")
	var pattern strings.Builder
	pattern.WriteString("^")
	seen := map[string]bool{}
	var fields []string
	for i, s := range segs {
		if i > 0 {
			pattern.WriteString("/")
		}
		if m := placeholderRe.FindStringSubmatch(s); m != nil {
			if seen[m[1]] {
				return nil, fmt.Errorf("path düzeni: {%s} birden fazla kez geçiyor", m[1])
			}
			seen[m[1]] = true
			fields = append(fields, m[1])
			pattern.WriteString("([^/]+)")
			continue
		}
		if strings.ContainsAny(s, "{}") {
			return nil, fmt.Errorf("path düzeni: bilinmeyen yer tutucu %q", s)
		}
		pattern.WriteString(regexp.QuoteMeta(s))
	}
	// The pattern also accepts a trailing file name, so both a revision folder
	// and a file path inside it parse.
	pattern.WriteString("(?:/[^/]+)?$")
	for _, req := range required {
		if !seen[req] {
			return nil, fmt.Errorf("path düzeni: {%s} eksik", req)
		}
	}
	return &Layout{template: template, re: regexp.MustCompile(pattern.String()), fields: fields}, nil
}

// MustLayout is NewLayout that panics; for package-level defaults.
func MustLayout(template string) *Layout {
	l, err := NewLayout(template)
	if err != nil {
		panic(err)
	}
	return l
}

// DefaultLayout is the layout compiled from PathLayout.
var DefaultLayout = MustLayout(PathLayout)

// recipeLayout parses the folder of a RecipeFile item.
var recipeLayout = func() *Layout {
	l, err := newLayout(RecipeLayout, "name", "version", "rrev")
	if err != nil {
		panic(err)
	}
	return l
}()

// Template returns the source template.
func (l *Layout) Template() string { return l.template }

// Parse extracts the fields from a repository-relative path (folder or file).
func (l *Layout) Parse(path string) (PathInfo, error) {
	m := l.re.FindStringSubmatch(strings.Trim(path, "/"))
	if m == nil {
		return PathInfo{}, fmt.Errorf("path beklenen düzene uymuyor (%s): %s", l.template, path)
	}
	var pi PathInfo
	for i, f := range l.fields {
		v := m[i+1]
		switch f {
		case "user":
			pi.User = unEmpty(v)
		case "name":
			pi.Name = v
		case "version":
			pi.Version = v
		case "channel":
			pi.Channel = unEmpty(v)
		case "rrev":
			pi.RecipeRevision = v
		case "pkgid":
			pi.PackageID = v
		case "prev":
			pi.PackageRevision = v
		}
	}
	return pi, nil
}

// ParsePath parses path with DefaultLayout.
func ParsePath(path string) (PathInfo, error) { return DefaultLayout.Parse(path) }

func unEmpty(v string) string {
	if v == EmptyField {
		return ""
	}
	return v
}
