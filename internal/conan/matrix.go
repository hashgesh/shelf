package conan

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hashgesh/shelf/internal/i18n"
)

// CellState is the availability of a package for one profile.
type CellState int

// Cell states.
const (
	CellMissing   CellState = iota // no binary matches the profile
	CellAvailable                  // at least one binary matches
	CellUnknown                    // nothing matches, but some binaries have unknown settings
)

func (s CellState) String() string {
	switch s {
	case CellAvailable:
		return "available"
	case CellUnknown:
		return "unknown"
	}
	return "missing"
}

// MarshalJSON implements json.Marshaler.
func (s CellState) MarshalJSON() ([]byte, error) { return json.Marshal(s.String()) }

// UnmarshalJSON implements json.Unmarshaler.
func (s *CellState) UnmarshalJSON(b []byte) error {
	var v string
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch v {
	case "available":
		*s = CellAvailable
	case "unknown":
		*s = CellUnknown
	case "missing":
		*s = CellMissing
	default:
		return fmt.Errorf(i18n.T("unknown cell state: %q"), v)
	}
	return nil
}

// MatrixCell lists the binaries of a row that satisfy a profile.
type MatrixCell struct {
	State    CellState `json:"state"`
	Binaries []*Binary `json:"-"`
}

// MatrixRow is one package version.
type MatrixRow struct {
	Package string       `json:"package"`
	Ref     string       `json:"ref"`
	Version *Version     `json:"-"`
	Cells   []MatrixCell `json:"-"`
}

// Matrix is packages × profiles.
type Matrix struct {
	Profiles []Profile
	Rows     []MatrixRow
}

// MatrixOptions controls BuildMatrix.
type MatrixOptions struct {
	// Filter selects profile columns (see Filter.MatchProfile).
	Filter Filter
	// AllVersions adds a row per version instead of only the newest one.
	AllVersions bool
	// Packages limits rows to these package names when non-nil.
	Packages []*Package
}

// MatchProfile reports whether a profile passes the filter, comparing the
// filter fields against the profile's settings the same way Match does for
// binaries. A profile without the filtered setting does not pass.
func (f Filter) MatchProfile(p Profile) bool {
	s := p.Settings
	if f.OS != "" && !hasPrefixFold(s[SettingOS], f.OS) {
		return false
	}
	if f.Arch != "" && !hasPrefixFold(s[SettingArch], f.Arch) {
		return false
	}
	if f.Compiler != "" && !hasPrefixFold(s[SettingCompiler]+s[SettingCompilerVersion], f.Compiler) {
		return false
	}
	if f.BuildType != "" && !hasPrefixFold(s[SettingBuildType], f.BuildType) {
		return false
	}
	if f.Profile != "" && !strings.Contains(strings.ToLower(p.Name), strings.ToLower(f.Profile)) {
		return false
	}
	return true
}

// BuildMatrix computes availability of each package's newest recipe
// revision for every profile that passes opts.Filter. Profiles must already
// be assigned to binaries (MatchProfiles).
func BuildMatrix(cat *Catalog, profiles []Profile, opts MatrixOptions) *Matrix {
	m := &Matrix{}
	for _, p := range profiles {
		if opts.Filter.MatchProfile(p) {
			m.Profiles = append(m.Profiles, p)
		}
	}
	pkgs := opts.Packages
	if pkgs == nil {
		pkgs = cat.Packages
	}
	for _, p := range pkgs {
		for i, v := range p.Versions {
			if i > 0 && !opts.AllVersions {
				break
			}
			row := MatrixRow{Package: p.Name, Ref: v.Ref(p.Name), Version: v}
			var bins []*Binary
			if r := v.Latest(); r != nil {
				bins = r.Binaries
			}
			unknown := false
			for _, b := range bins {
				unknown = unknown || b.MetadataSource == SourceNone
			}
			for _, prof := range m.Profiles {
				cell := MatrixCell{State: CellMissing}
				for _, b := range bins {
					if contains(b.Profiles, prof.Name) {
						cell.Binaries = append(cell.Binaries, b)
					}
				}
				switch {
				case len(cell.Binaries) > 0:
					cell.State = CellAvailable
				case unknown:
					cell.State = CellUnknown
				}
				row.Cells = append(row.Cells, cell)
			}
			m.Rows = append(m.Rows, row)
		}
	}
	return m
}

// Missing counts cells without a matching binary (unknown cells excluded).
func (m *Matrix) Missing() int {
	n := 0
	for _, r := range m.Rows {
		for _, c := range r.Cells {
			if c.State == CellMissing {
				n++
			}
		}
	}
	return n
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
