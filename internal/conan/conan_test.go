package conan

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/halilibrahimonus/shelf/internal/artifactory"
)

const fixtureDir = "../../testdata"

func TestParsePath(t *testing.T) {
	tests := []struct {
		path string
		want PathInfo
	}{
		{
			"_/zlib/1.3.1/_/rrev1/package/pkg1/prev1",
			PathInfo{Name: "zlib", Version: "1.3.1", RecipeRevision: "rrev1", PackageID: "pkg1", PackageRevision: "prev1"},
		},
		{
			"mycompany/fmt/10.2.1/stable/rrev2/package/pkg2/prev2/conan_package.tgz",
			PathInfo{User: "mycompany", Name: "fmt", Version: "10.2.1", Channel: "stable", RecipeRevision: "rrev2", PackageID: "pkg2", PackageRevision: "prev2"},
		},
		{
			"/_/zlib/1.3.1/_/r/package/p/v/",
			PathInfo{Name: "zlib", Version: "1.3.1", RecipeRevision: "r", PackageID: "p", PackageRevision: "v"},
		},
	}
	for _, tt := range tests {
		got, err := ParsePath(tt.path)
		if err != nil {
			t.Errorf("ParsePath(%q): %v", tt.path, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParsePath(%q) = %+v, want %+v", tt.path, got, tt.want)
		}
	}

	for _, bad := range []string{
		"",
		"_/zlib/1.3.1/_/rrev/export/conanfile.py", // recipe export, not a package
		"_/zlib/1.3.1/_/rrev/package/pkg",         // missing prev
		"zlib/1.3.1/_/_/0/package/pkg/0/extra/conan_package.tgz",
	} {
		if _, err := ParsePath(bad); err == nil {
			t.Errorf("ParsePath(%q) succeeded, want error", bad)
		}
	}
}

func TestCustomLayout(t *testing.T) {
	// Conan-1-style ordering, to show the layout is a single switch.
	l, err := NewLayout("{name}/{version}/{user}/{channel}/{rrev}/package/{pkgid}/{prev}")
	if err != nil {
		t.Fatal(err)
	}
	got, err := l.Parse("zlib/1.3.1/_/_/r/package/p/v")
	if err != nil || got.Name != "zlib" || got.User != "" || got.PackageRevision != "v" {
		t.Fatalf("Parse = %+v, %v", got, err)
	}

	// {user}/{channel} are optional.
	l, err = NewLayout("{name}/{version}/{rrev}/pkg/{pkgid}/{prev}")
	if err != nil {
		t.Fatal(err)
	}
	if got, err := l.Parse("zlib/1.3.1/r/pkg/p/v"); err != nil || got.PackageID != "p" {
		t.Fatalf("Parse = %+v, %v", got, err)
	}

	for _, bad := range []string{
		"{name}/{version}/{rrev}/{pkgid}",               // no {prev}
		"{name}/{name}/{version}/{rrev}/{pkgid}/{prev}", // duplicate
		"{name}/{version}/{rrev}/{pkgid}/{prev}/{bogus}",
	} {
		if _, err := NewLayout(bad); err == nil {
			t.Errorf("NewLayout(%q) succeeded", bad)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.3.1", "1.2.13", 1},
		{"1.10", "1.9", 1},
		{"194", "193", 1},
		{"1.0", "1.0", 0},
		{"1.0", "1.0.1", -1},
		{"3.0.0-beta", "3.0.0", 1}, // natural order, not semver pre-release rules
		{"1.2a", "1.2b", -1},
	}
	for _, tt := range tests {
		if got := CompareVersions(tt.a, tt.b); got != tt.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestParseConaninfo(t *testing.T) {
	data := []byte("\ufeff[settings]\narch=x86_64\nbuild_type=Release\ncompiler.version=12\nos=Linux\n\n" +
		"[options]\nshared=False\n# comment\n[requires]\nzlib/1.3.Z\n")
	info := ParseConaninfo(data)
	want := map[string]string{"arch": "x86_64", "build_type": "Release", "compiler.version": "12", "os": "Linux"}
	if !reflect.DeepEqual(info.Settings, want) {
		t.Errorf("Settings = %v", info.Settings)
	}
	if !reflect.DeepEqual(info.Options, map[string]string{"shared": "False"}) {
		t.Errorf("Options = %v", info.Options)
	}
}

func loadFixture(t *testing.T) []artifactory.Item {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(fixtureDir, "aql.json"))
	if err != nil {
		t.Fatal(err)
	}
	var res artifactory.Result
	if err := json.Unmarshal(data, &res); err != nil {
		t.Fatal(err)
	}
	return res.Results
}

func TestBuildCatalogFromFixture(t *testing.T) {
	cat, warns := BuildCatalog(loadFixture(t), nil)
	if len(warns) != 0 {
		t.Fatalf("warnings: %v", warns)
	}
	var names []string
	for _, p := range cat.Packages {
		names = append(names, p.Name)
	}
	if got := strings.Join(names, ","); got != "fmt,nlohmann_json,openssl,zlib" {
		t.Fatalf("packages = %s", got)
	}

	zlib := cat.Find("ZLIB")
	if zlib == nil || len(zlib.Versions) != 2 || zlib.Versions[0].Version != "1.3.1" {
		t.Fatalf("zlib versions = %+v", zlib)
	}
	v := zlib.Versions[0]
	if len(v.Revisions) != 2 {
		t.Fatalf("zlib/1.3.1 revisions = %d", len(v.Revisions))
	}
	latest := v.Latest()
	if len(latest.Binaries) != 9 {
		t.Fatalf("latest zlib revision binaries = %d, want 9", len(latest.Binaries))
	}
	// Two package revisions for one package ID collapse into one binary.
	older := 0
	for _, b := range latest.Binaries {
		older += b.OlderRevisions
	}
	if older != 1 {
		t.Errorf("OlderRevisions total = %d, want 1", older)
	}
	// Sorted: Linux before Windows, armv8 before x86_64, Debug before Release.
	first := latest.Binaries[0]
	if first.OS != "Linux" || first.Arch != "armv8" {
		t.Errorf("first binary = %s %s", first.OS, first.Arch)
	}
	if last := latest.Binaries[len(latest.Binaries)-1]; last.OS != "Windows" || last.BuildType != "Release" || last.Shared() != "True" {
		t.Errorf("last binary = %s %s shared=%s", last.OS, last.BuildType, last.Shared())
	}
	if first.MetadataSource != SourceProperties || first.Compiler != "gcc" || first.CompilerVersion != "12" {
		t.Errorf("first binary metadata = %+v", first)
	}

	fmtPkg := cat.Find("fmt")
	if ref := fmtPkg.Versions[0].Ref("fmt"); ref != "fmt/11.0.2@mycompany/stable" {
		t.Errorf("fmt ref = %s", ref)
	}

	var missing []*Binary
	for _, b := range cat.Binaries() {
		if b.MetadataSource == SourceNone {
			missing = append(missing, b)
		}
	}
	if len(missing) != 2 {
		t.Fatalf("binaries without metadata = %d, want 2 (openssl Debug, nlohmann_json)", len(missing))
	}
	for _, b := range missing {
		if b.Summary() != "bilinmiyor" {
			t.Errorf("%s summary = %q", b, b.Summary())
		}
	}
}

func TestConaninfoFallbackFromFixture(t *testing.T) {
	cat, _ := BuildCatalog(loadFixture(t), nil)
	for _, b := range cat.Binaries() {
		if b.MetadataSource != SourceNone {
			continue
		}
		data, err := os.ReadFile(filepath.Join(fixtureDir, "files", b.Repo, b.ConaninfoPath()))
		if err != nil {
			t.Fatal(err)
		}
		b.ApplyConaninfo(ParseConaninfo(data))
	}
	ossl := cat.Find("openssl").Versions[0].Latest()
	found := false
	for _, b := range ossl.Binaries {
		if b.MetadataSource == SourceConaninfo {
			found = true
			if b.OS != "Linux" || b.BuildType != "Debug" || b.Shared() != "False" {
				t.Errorf("conaninfo binary = %+v", b)
			}
		}
	}
	if !found {
		t.Error("no binary filled from conaninfo")
	}
	hdr := cat.Find("nlohmann_json").Binaries()[0]
	if hdr.HasSettings() || hdr.Summary() != "ayarsız" {
		t.Errorf("header-only binary = %+v summary=%q", hdr, hdr.Summary())
	}
}

func TestBuildCatalogWarnsOnUnknownLayout(t *testing.T) {
	items := []artifactory.Item{
		{Repo: "r", Path: "zlib/1.0/legacy", Name: PackageFile},
		{Repo: "r", Path: "_/zlib/1.0/_/r/package/p/v", Name: PackageFile, Created: time.Now()},
		{Repo: "r", Path: "_/zlib/1.0/_/r/package/p/v", Name: ConaninfoFile}, // ignored
	}
	cat, warns := BuildCatalog(items, nil)
	if len(warns) != 1 || !strings.Contains(warns[0].Path, "zlib/1.0/legacy") {
		t.Fatalf("warnings = %v", warns)
	}
	if len(cat.Binaries()) != 1 {
		t.Fatalf("binaries = %d", len(cat.Binaries()))
	}
}

func TestSummary(t *testing.T) {
	b := &Binary{MetadataSource: SourceProperties}
	b.SetSettings(map[string]string{"os": "Linux", "arch": "x86_64", "compiler": "gcc", "compiler.version": "12", "build_type": "Release"})
	if got := b.Summary(); got != "Linux_x86_64_gcc12_Release" {
		t.Errorf("Summary = %q", got)
	}
	if b.Label() != b.Summary() {
		t.Error("Label should fall back to Summary")
	}
	b.Profile = "linux-gcc12-x86_64-release"
	if b.Label() != b.Profile {
		t.Error("Label should prefer Profile")
	}
}

func TestLoadProfilesFixture(t *testing.T) {
	profiles, warns, err := LoadProfiles(filepath.Join(fixtureDir, "profiles"))
	if err != nil || len(warns) != 0 {
		t.Fatalf("err=%v warns=%v", err, warns)
	}
	if len(profiles) != 5 || profiles[0].Name != "linux-gcc12-armv8-release" {
		t.Fatalf("profiles = %+v", profiles)
	}
	if profiles[0].Settings["arch"] != "armv8" || profiles[0].Settings["compiler.cppstd"] != "17" {
		t.Errorf("settings = %v", profiles[0].Settings)
	}
}

func TestLoadProfilesMissingDir(t *testing.T) {
	profiles, warns, err := LoadProfiles(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(profiles) != 0 || len(warns) != 0 {
		t.Fatalf("profiles=%v warns=%v err=%v", profiles, warns, err)
	}
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestLoadProfilesIncludesAndEdgeCases(t *testing.T) {
	dir := writeFiles(t, map[string]string{
		"base/gcc12": "[settings]\ncompiler=gcc\ncompiler.version=12\nbuild_type=Debug\n",
		"linux-rel": "include(base/gcc12)\n\n[settings]\nos=Linux\nbuild_type=Release\n" +
			"zlib/*:build_type=Debug\n[options]\n*:shared=True\n",
		"loop-a":      "include(loop-b)\n[settings]\nos=Linux\n",
		"loop-b":      "include(loop-a)\n[settings]\narch=armv8\n",
		"templated":   "{% set v = '12' %}\n[settings]\nos=Linux\ncompiler.version={{ v }}\n",
		".hidden":     "[settings]\nos=Hidden\n",
		".git/HEAD":   "ref: main\n",
		"sub/win-rel": "[settings]\nos=Windows\n",
	})
	profiles, warns, err := LoadProfiles(dir)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]Profile{}
	for _, p := range profiles {
		byName[p.Name] = p
	}
	if _, ok := byName[".hidden"]; ok {
		t.Error("hidden profile loaded")
	}
	if _, ok := byName["sub/win-rel"]; !ok {
		t.Errorf("nested profile missing: %v", profiles)
	}
	want := map[string]string{"compiler": "gcc", "compiler.version": "12", "build_type": "Release", "os": "Linux"}
	if got := byName["linux-rel"].Settings; !reflect.DeepEqual(got, want) {
		t.Errorf("linux-rel settings = %v, want %v", got, want)
	}
	if got := byName["templated"].Settings; !reflect.DeepEqual(got, map[string]string{"os": "Linux"}) {
		t.Errorf("templated settings = %v", got)
	}
	var sawLoop, sawJinja bool
	for _, w := range warns {
		sawLoop = sawLoop || strings.Contains(w.Message, "döngüsel")
		sawJinja = sawJinja || strings.Contains(w.Message, "Jinja")
	}
	if !sawLoop || !sawJinja {
		t.Errorf("warnings = %v", warns)
	}
}

func TestProfileMatching(t *testing.T) {
	profiles, _, err := LoadProfiles(filepath.Join(fixtureDir, "profiles"))
	if err != nil {
		t.Fatal(err)
	}
	cat, _ := BuildCatalog(loadFixture(t), nil)
	MatchProfiles(cat, profiles)

	counts := map[string]int{}
	for _, b := range cat.Find("zlib").Versions[0].Latest().Binaries {
		if b.Profile == "" {
			t.Errorf("zlib binary %s (%s) matched no profile", b.PackageID, b.Summary())
		}
		counts[b.Profile]++
		if len(b.Profiles) != 1 {
			t.Errorf("zlib binary %s matched %v", b.Summary(), b.Profiles)
		}
	}
	// Shared and static variants both satisfy a profile that does not set options.
	want := map[string]int{
		"linux-gcc12-x86_64-release": 2, "linux-gcc12-x86_64-debug": 2, "linux-gcc12-armv8-release": 1,
		"win-msvc194-x64-release": 2, "win-msvc194-x64-debug": 2,
	}
	if !reflect.DeepEqual(counts, want) {
		t.Errorf("profile counts = %v, want %v", counts, want)
	}

	// Binaries without metadata match nothing until conaninfo is applied.
	for _, b := range cat.Binaries() {
		if b.MetadataSource == SourceNone && b.Profile != "" {
			t.Errorf("%s matched %q without metadata", b, b.Profile)
		}
	}
}

func TestProfileMatchesRules(t *testing.T) {
	p := Profile{Name: "p", Settings: map[string]string{"os": "Linux", "build_type": "Release", "compiler.cppstd": "17"}}
	mk := func(src string, s map[string]string) *Binary {
		b := &Binary{MetadataSource: src}
		b.SetSettings(s)
		return b
	}
	tests := []struct {
		name string
		b    *Binary
		want bool
	}{
		{"exact", mk(SourceProperties, map[string]string{"os": "Linux", "build_type": "Release", "compiler.cppstd": "17"}), true},
		{"case-insensitive", mk(SourceProperties, map[string]string{"os": "linux", "build_type": "release"}), true},
		{"setting erased from package id", mk(SourceProperties, map[string]string{"os": "Linux", "build_type": "Release"}), true},
		{"mismatch", mk(SourceProperties, map[string]string{"os": "Linux", "build_type": "Debug"}), false},
		{"different cppstd", mk(SourceProperties, map[string]string{"os": "Linux", "build_type": "Release", "compiler.cppstd": "20"}), false},
		{"header-only", mk(SourceConaninfo, map[string]string{}), true},
		{"unknown metadata", mk(SourceNone, map[string]string{}), false},
	}
	for _, tt := range tests {
		if got := p.Matches(tt.b); got != tt.want {
			t.Errorf("%s: Matches = %v, want %v", tt.name, got, tt.want)
		}
	}
	if (Profile{Name: "empty"}).Matches(tests[0].b) {
		t.Error("empty profile should match nothing")
	}
}

func TestAssignProfilesPrefersMostSpecific(t *testing.T) {
	b := &Binary{MetadataSource: SourceProperties}
	b.SetSettings(map[string]string{"os": "Linux", "arch": "x86_64", "build_type": "Release"})
	AssignProfiles(b, []Profile{
		{Name: "a-generic", Settings: map[string]string{"os": "Linux"}},
		{Name: "b-specific", Settings: map[string]string{"os": "Linux", "arch": "x86_64", "build_type": "Release"}},
		{Name: "c-other", Settings: map[string]string{"os": "Windows"}},
	})
	if b.Profile != "b-specific" || !reflect.DeepEqual(b.Profiles, []string{"a-generic", "b-specific"}) {
		t.Errorf("Profile=%q Profiles=%v", b.Profile, b.Profiles)
	}
}
