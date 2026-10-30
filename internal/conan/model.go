package conan

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/halilibrahimonus/shelf/internal/artifactory"
)

// Where a binary's settings came from.
const (
	SourceProperties = "properties"
	SourceConaninfo  = "conaninfo"
	SourceNone       = "none"
)

// Catalog is the whole browsable model: Package → Version → Revision → Binary.
type Catalog struct {
	Packages []*Package `json:"packages"`
}

// Package groups every version of one recipe name.
type Package struct {
	Name     string     `json:"name"`
	Versions []*Version `json:"versions"`
}

// Version is one recipe reference: name/version[@user/channel].
type Version struct {
	Version   string      `json:"version"`
	User      string      `json:"user,omitempty"`
	Channel   string      `json:"channel,omitempty"`
	Revisions []*Revision `json:"revisions"`
}

// Revision is one recipe revision with its binaries.
type Revision struct {
	Revision string    `json:"revision"`
	Binaries []*Binary `json:"binaries"`
}

// Binary is one package ID at its latest package revision.
type Binary struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	User            string            `json:"user,omitempty"`
	Channel         string            `json:"channel,omitempty"`
	RecipeRevision  string            `json:"recipe_revision"`
	PackageID       string            `json:"package_id"`
	PackageRevision string            `json:"package_revision"`
	OS              string            `json:"os"`
	Arch            string            `json:"arch"`
	Compiler        string            `json:"compiler"`
	CompilerVersion string            `json:"compiler_version"`
	BuildType       string            `json:"build_type"`
	Settings        map[string]string `json:"settings"`
	Options         map[string]string `json:"options"`
	Size            int64             `json:"size"`
	Created         time.Time         `json:"created"`
	CreatedBy       string            `json:"created_by"`
	Repo            string            `json:"repo"`
	Path            string            `json:"path"` // package revision folder
	Profile         string            `json:"profile,omitempty"`
	Profiles        []string          `json:"profiles,omitempty"`
	MetadataSource  string            `json:"metadata_source"`
	// OlderRevisions counts package revisions superseded by this one.
	OlderRevisions int `json:"older_revisions,omitempty"`
}

// Ref returns name/version[@user/channel].
func (b *Binary) Ref() string { return FormatRef(b.Name, b.Version, b.User, b.Channel) }

// Ref returns name/version[@user/channel] for the version.
func (v *Version) Ref(name string) string { return FormatRef(name, v.Version, v.User, v.Channel) }

// FormatRef formats a Conan recipe reference.
func FormatRef(name, version, user, channel string) string {
	ref := name + "/" + version
	if user != "" || channel != "" {
		ref += "@" + orEmpty(user) + "/" + orEmpty(channel)
	}
	return ref
}

func orEmpty(s string) string {
	if s == "" {
		return EmptyField
	}
	return s
}

// ConaninfoPath is the repository path of the binary's conaninfo.txt.
func (b *Binary) ConaninfoPath() string { return path.Join(b.Path, ConaninfoFile) }

// Shared returns the "shared" option value, or "" when not present.
func (b *Binary) Shared() string { return b.Options[OptionShared] }

// HasSettings reports whether any settings are known.
func (b *Binary) HasSettings() bool { return len(b.Settings) > 0 }

// SetSettings replaces settings and refreshes the derived fields.
func (b *Binary) SetSettings(settings map[string]string) {
	b.Settings = settings
	b.OS = settings[SettingOS]
	b.Arch = settings[SettingArch]
	b.Compiler = settings[SettingCompiler]
	b.CompilerVersion = settings[SettingCompilerVersion]
	b.BuildType = settings[SettingBuildType]
}

// Summary is a short label used when no profile matches, e.g.
// "Linux_x86_64_gcc12_Release".
func (b *Binary) Summary() string {
	var parts []string
	for _, p := range []string{b.OS, b.Arch, b.Compiler + b.CompilerVersion, b.BuildType} {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		if b.MetadataSource == SourceNone {
			return "bilinmiyor"
		}
		return "ayarsız" // header-only or settings-free package
	}
	return strings.Join(parts, "_")
}

// Label is the matched profile name, or Summary when nothing matched.
func (b *Binary) Label() string {
	if b.Profile != "" {
		return b.Profile
	}
	return b.Summary()
}

// Warning describes an item that could not be used.
type Warning struct {
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (w Warning) String() string { return w.Path + ": " + w.Message }

// BuildCatalog converts conan_package.tgz items into a Catalog. Items whose
// path does not match layout are reported as warnings and skipped. Items
// without settings properties get MetadataSource=SourceNone; the caller may
// fill them from conaninfo.txt afterwards.
func BuildCatalog(items []artifactory.Item, layout *Layout) (*Catalog, []Warning) {
	if layout == nil {
		layout = DefaultLayout
	}
	var warnings []Warning
	type pkgKey struct{ repo, user, name, version, channel, rrev, pkgid string }
	latest := map[pkgKey]*Binary{}
	var order []pkgKey

	for _, it := range items {
		if it.Name != PackageFile {
			continue
		}
		full := it.Repo + "/" + it.FullPath()
		pi, err := layout.Parse(it.Path)
		if err != nil {
			warnings = append(warnings, Warning{Path: full, Message: err.Error()})
			continue
		}
		b := binaryFromItem(it, pi)
		k := pkgKey{it.Repo, b.User, b.Name, b.Version, b.Channel, b.RecipeRevision, b.PackageID}
		prev, ok := latest[k]
		switch {
		case !ok:
			latest[k] = b
			order = append(order, k)
		case b.Created.After(prev.Created):
			b.OlderRevisions = prev.OlderRevisions + 1
			latest[k] = b
		default:
			prev.OlderRevisions++
		}
	}

	cat := &Catalog{}
	pkgs := map[string]*Package{}
	type verKey struct{ name, version, user, channel string }
	vers := map[verKey]*Version{}
	type revKey struct {
		verKey
		rrev string
	}
	revs := map[revKey]*Revision{}
	revCreated := map[*Revision]time.Time{}

	for _, k := range order {
		b := latest[k]
		p := pkgs[b.Name]
		if p == nil {
			p = &Package{Name: b.Name}
			pkgs[b.Name] = p
			cat.Packages = append(cat.Packages, p)
		}
		vk := verKey{b.Name, b.Version, b.User, b.Channel}
		v := vers[vk]
		if v == nil {
			v = &Version{Version: b.Version, User: b.User, Channel: b.Channel}
			vers[vk] = v
			p.Versions = append(p.Versions, v)
		}
		rk := revKey{vk, b.RecipeRevision}
		r := revs[rk]
		if r == nil {
			r = &Revision{Revision: b.RecipeRevision}
			revs[rk] = r
			v.Revisions = append(v.Revisions, r)
		}
		r.Binaries = append(r.Binaries, b)
		if b.Created.After(revCreated[r]) {
			revCreated[r] = b.Created
		}
	}

	sort.Slice(cat.Packages, func(i, j int) bool {
		return strings.ToLower(cat.Packages[i].Name) < strings.ToLower(cat.Packages[j].Name)
	})
	for _, p := range cat.Packages {
		sort.SliceStable(p.Versions, func(i, j int) bool {
			a, b := p.Versions[i], p.Versions[j]
			if c := CompareVersions(a.Version, b.Version); c != 0 {
				return c > 0 // newest first
			}
			return a.User+"/"+a.Channel < b.User+"/"+b.Channel
		})
		for _, v := range p.Versions {
			sort.SliceStable(v.Revisions, func(i, j int) bool {
				return revCreated[v.Revisions[i]].After(revCreated[v.Revisions[j]]) // latest first
			})
			for _, r := range v.Revisions {
				SortBinaries(r.Binaries)
			}
		}
	}
	return cat, warnings
}

func binaryFromItem(it artifactory.Item, pi PathInfo) *Binary {
	b := &Binary{
		Name:            pi.Name,
		Version:         pi.Version,
		User:            pi.User,
		Channel:         pi.Channel,
		RecipeRevision:  pi.RecipeRevision,
		PackageID:       pi.PackageID,
		PackageRevision: pi.PackageRevision,
		Size:            it.Size,
		Created:         it.Created,
		CreatedBy:       it.CreatedBy,
		Repo:            it.Repo,
		Path:            it.Path,
		Options:         map[string]string{},
		MetadataSource:  SourceNone,
	}
	// The path is authoritative for identity; properties only fill gaps.
	if b.Name == "" {
		b.Name, _ = it.Prop(PropName)
	}
	if b.Version == "" {
		b.Version, _ = it.Prop(PropVersion)
	}

	settings := map[string]string{}
	for _, p := range it.Properties {
		switch {
		case strings.HasPrefix(p.Key, PropSettingsPrefix):
			settings[strings.TrimPrefix(p.Key, PropSettingsPrefix)] = p.Value
		case strings.HasPrefix(p.Key, PropOptionsPrefix):
			b.Options[strings.TrimPrefix(p.Key, PropOptionsPrefix)] = p.Value
		}
	}
	b.SetSettings(settings)
	if len(settings) > 0 || len(b.Options) > 0 {
		b.MetadataSource = SourceProperties
	}
	return b
}

// ApplyConaninfo replaces the binary's settings and options with those parsed
// from conaninfo.txt.
func (b *Binary) ApplyConaninfo(info *Conaninfo) {
	b.SetSettings(info.Settings)
	b.Options = info.Options
	b.MetadataSource = SourceConaninfo
}

// SortBinaries orders binaries by OS, arch, compiler, compiler version
// (numerically), build type and package ID.
func SortBinaries(bs []*Binary) {
	sort.SliceStable(bs, func(i, j int) bool {
		a, b := bs[i], bs[j]
		for _, pair := range [][2]string{{a.OS, b.OS}, {a.Arch, b.Arch}, {a.Compiler, b.Compiler}} {
			if x, y := strings.ToLower(pair[0]), strings.ToLower(pair[1]); x != y {
				return x < y
			}
		}
		if c := CompareVersions(a.CompilerVersion, b.CompilerVersion); c != 0 {
			return c < 0
		}
		if a.BuildType != b.BuildType {
			return a.BuildType < b.BuildType
		}
		if a.Shared() != b.Shared() {
			return a.Shared() < b.Shared()
		}
		return a.PackageID < b.PackageID
	})
}

// CompareVersions compares two version strings naturally, so "1.10" > "1.9"
// and "194" > "193". Returns -1, 0 or 1.
func CompareVersions(a, b string) int {
	ta, tb := versionTokens(a), versionTokens(b)
	for i := 0; i < len(ta) && i < len(tb); i++ {
		x, y := ta[i], tb[i]
		nx, errx := strconv.ParseUint(x, 10, 64)
		ny, erry := strconv.ParseUint(y, 10, 64)
		switch {
		case errx == nil && erry == nil:
			if nx != ny {
				if nx < ny {
					return -1
				}
				return 1
			}
		case x != y:
			if x < y {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(ta) < len(tb):
		return -1
	case len(ta) > len(tb):
		return 1
	}
	return 0
}

func versionTokens(s string) []string {
	var out []string
	var cur strings.Builder
	digit := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		if r == '.' || r == '-' || r == '_' || r == '+' {
			flush()
			continue
		}
		d := unicode.IsDigit(r)
		if cur.Len() > 0 && d != digit {
			flush()
		}
		digit = d
		cur.WriteRune(r)
	}
	flush()
	return out
}

// Find returns the package with the given name (case-insensitive).
func (c *Catalog) Find(name string) *Package {
	for _, p := range c.Packages {
		if strings.EqualFold(p.Name, name) {
			return p
		}
	}
	return nil
}

// Binaries returns every binary in the catalog.
func (c *Catalog) Binaries() []*Binary {
	var out []*Binary
	for _, p := range c.Packages {
		out = append(out, p.Binaries()...)
	}
	return out
}

// Binaries returns every binary of every version of p.
func (p *Package) Binaries() []*Binary {
	var out []*Binary
	for _, v := range p.Versions {
		for _, r := range v.Revisions {
			out = append(out, r.Binaries...)
		}
	}
	return out
}

// Latest returns the newest recipe revision, or nil.
func (v *Version) Latest() *Revision {
	if len(v.Revisions) == 0 {
		return nil
	}
	return v.Revisions[0]
}

// String implements fmt.Stringer for debugging.
func (b *Binary) String() string {
	return fmt.Sprintf("%s#%s:%s#%s", b.Ref(), b.RecipeRevision, b.PackageID, b.PackageRevision)
}
