package conan

import (
	"fmt"
	"strings"

	"github.com/hashgesh/shelf/internal/i18n"
)

// Filter narrows binaries. Empty fields match everything. Values are matched
// case-insensitively as prefixes, so "win" selects Windows, "rel" Release and
// "gcc" or "gcc12" a compiler (name followed by version).
type Filter struct {
	OS        string
	Arch      string
	Compiler  string
	BuildType string
	Profile   string // substring of any matched profile name
}

// IsZero reports whether the filter is empty.
func (f Filter) IsZero() bool { return f == Filter{} }

func hasPrefixFold(s, prefix string) bool {
	return len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix)
}

// Match reports whether b passes the filter.
func (f Filter) Match(b *Binary) bool {
	if f.OS != "" && !hasPrefixFold(b.OS, f.OS) {
		return false
	}
	if f.Arch != "" && !hasPrefixFold(b.Arch, f.Arch) {
		return false
	}
	if f.Compiler != "" && !hasPrefixFold(b.Compiler+b.CompilerVersion, f.Compiler) {
		return false
	}
	if f.BuildType != "" && !hasPrefixFold(b.BuildType, f.BuildType) {
		return false
	}
	if f.Profile != "" {
		want := strings.ToLower(f.Profile)
		found := false
		for _, p := range b.Profiles {
			if strings.Contains(strings.ToLower(p), want) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// Apply returns the binaries that pass the filter.
func (f Filter) Apply(bs []*Binary) []*Binary {
	if f.IsZero() {
		return bs
	}
	out := make([]*Binary, 0, len(bs))
	for _, b := range bs {
		if f.Match(b) {
			out = append(out, b)
		}
	}
	return out
}

// Describe renders the active filter fields, e.g. "os=win build=rel".
func (f Filter) Describe() string {
	var parts []string
	for _, kv := range [][2]string{{"os", f.OS}, {"arch", f.Arch}, {"compiler", f.Compiler}, {"build", f.BuildType}, {"profile", f.Profile}} {
		if kv[1] != "" {
			parts = append(parts, kv[0]+"="+kv[1])
		}
	}
	return strings.Join(parts, " ")
}

// RefSpec is a possibly partial reference typed by the user:
// name, name/version or name/version@user/channel.
type RefSpec struct {
	Name, Version, User, Channel string
	HasUserChannel               bool
}

// ParseRefSpec parses a user-typed reference.
func ParseRefSpec(s string) (RefSpec, error) {
	var r RefSpec
	s = strings.TrimSpace(s)
	main, uc, hasUC := strings.Cut(s, "@")
	name, version, _ := strings.Cut(main, "/")
	r.Name, r.Version = name, version
	if hasUC {
		user, channel, ok := strings.Cut(uc, "/")
		if !ok {
			return r, fmt.Errorf(i18n.T("invalid reference: %q (expected: name/version@user/channel)"), s)
		}
		r.User, r.Channel, r.HasUserChannel = unEmpty(user), unEmpty(channel), true
	}
	if r.Name == "" || strings.Contains(r.Version, "/") {
		return r, fmt.Errorf(i18n.T("invalid reference: %q (expected: name[/version[@user/channel]])"), s)
	}
	return r, nil
}

// Resolve finds the version a spec refers to. Without a version the newest
// one is returned. It errors when the spec is ambiguous or unknown.
func (c *Catalog) Resolve(spec RefSpec) (*Package, *Version, error) {
	p := c.Find(spec.Name)
	if p == nil {
		return nil, nil, fmt.Errorf(i18n.T("package not found: %s"), spec.Name)
	}
	var cands []*Version
	for _, v := range p.Versions {
		if spec.Version != "" && v.Version != spec.Version {
			continue
		}
		if spec.HasUserChannel && (v.User != spec.User || v.Channel != spec.Channel) {
			continue
		}
		cands = append(cands, v)
	}
	switch {
	case len(cands) == 0:
		return p, nil, fmt.Errorf(i18n.T("version not found: %s"), FormatRef(spec.Name, spec.Version, spec.User, spec.Channel))
	case spec.Version == "":
		return p, cands[0], nil // versions are sorted newest first
	case len(cands) > 1:
		var refs []string
		for _, v := range cands {
			refs = append(refs, v.Ref(p.Name))
		}
		return p, nil, fmt.Errorf(i18n.T("multiple matches, specify user/channel: %s"), strings.Join(refs, ", "))
	}
	return p, cands[0], nil
}
