package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/halilibrahimonus/shelf/internal/conan"
)

const fixture = "../../testdata/aql.json"

func runFixture(t *testing.T, args ...string) result {
	t.Helper()
	full := append([]string{"--config", writeEmptyConfig(t), "--fixture", fixture}, args...)
	return runCLI(t, nil, "", full...)
}

func TestListFixture(t *testing.T) {
	r := runFixture(t, "list")
	if r.code != ExitOK {
		t.Fatalf("code=%d stderr=%s", r.code, r.stderr)
	}
	for _, want := range []string{"PAKET", "zlib", "1.2.13", "10.2.1@mycompany/stable", "4 paket"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "\x1b[") {
		t.Error("color codes written to a non-terminal")
	}
	if strings.Contains(r.stderr, "uyarı") {
		t.Errorf("unexpected warnings: %s", r.stderr)
	}
}

func TestListFilterAndJSON(t *testing.T) {
	r := runFixture(t, "list", "ZL", "--json")
	if r.code != ExitOK {
		t.Fatalf("code=%d stderr=%s", r.code, r.stderr)
	}
	var pkgs []listPackageJSON
	if err := json.Unmarshal([]byte(r.stdout), &pkgs); err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0].Name != "zlib" || len(pkgs[0].Versions) != 2 {
		t.Fatalf("pkgs = %+v", pkgs)
	}
	if v := pkgs[0].Versions[0]; v.Version != "1.3.1" || v.Revisions != 2 || v.Binaries != 9 {
		t.Errorf("zlib/1.3.1 = %+v", v)
	}

	r = runFixture(t, "list", "nothing-matches")
	if r.code != ExitNoResult || !strings.Contains(r.stderr, "Paket bulunamadı") {
		t.Errorf("got %+v", r)
	}
	r = runFixture(t, "list", "nothing-matches", "--json")
	if r.code != ExitNoResult || strings.TrimSpace(r.stdout) != "[]" {
		t.Errorf("got %+v", r)
	}
}

func TestShowFixture(t *testing.T) {
	r := runFixture(t, "show", "zlib")
	if r.code != ExitOK {
		t.Fatalf("code=%d stderr=%s", r.code, r.stderr)
	}
	for _, want := range []string{"zlib/1.3.1", "toplam 2", "Diğer sürümler: 1.2.13", "OS", "PROFİL", "win-msvc194-x64-debug", "9 binary"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in:\n%s", want, r.stdout)
		}
	}
}

func TestShowFilters(t *testing.T) {
	tests := []struct {
		args []string
		want int
	}{
		{[]string{"show", "zlib/1.3.1", "--os", "win"}, 4},
		{[]string{"show", "zlib/1.3.1", "--os", "linux", "--arch", "armv8"}, 1},
		{[]string{"show", "zlib/1.3.1", "--compiler", "msvc194", "--build", "debug"}, 2},
		{[]string{"show", "zlib/1.3.1", "--profile", "x86_64-release"}, 2},
		{[]string{"show", "zlib/1.3.1", "--all-revisions"}, 11},
		{[]string{"show", "fmt/10.2.1@mycompany/stable"}, 5},
		{[]string{"show", "fmt/10.2.1@_/_"}, -1},
		{[]string{"show", "openssl", "--build", "debug"}, 1}, // filled from conaninfo.txt
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			r := runFixture(t, append(tt.args, "--json")...)
			if tt.want < 0 {
				if r.code != ExitNoResult {
					t.Fatalf("got %+v", r)
				}
				return
			}
			if r.code != ExitOK {
				t.Fatalf("code=%d stderr=%s", r.code, r.stderr)
			}
			var v showJSON
			if err := json.Unmarshal([]byte(r.stdout), &v); err != nil {
				t.Fatal(err)
			}
			if len(v.Binaries) != tt.want {
				t.Errorf("binaries = %d, want %d", len(v.Binaries), tt.want)
			}
		})
	}
}

func TestShowConaninfoFallback(t *testing.T) {
	r := runFixture(t, "show", "openssl", "--build", "debug", "--json")
	var v showJSON
	if err := json.Unmarshal([]byte(r.stdout), &v); err != nil {
		t.Fatalf("%v\n%s", err, r.stdout)
	}
	b := v.Binaries[0]
	if b.MetadataSource != conan.SourceConaninfo || b.Profile != "linux-gcc12-x86_64-debug" {
		t.Errorf("binary = %+v", b)
	}
}

func TestShowNoResults(t *testing.T) {
	r := runFixture(t, "show", "zlib", "--os", "macos")
	if r.code != ExitNoResult || !strings.Contains(r.stderr, "Eşleşen binary yok") {
		t.Errorf("got %+v", r)
	}
	r = runFixture(t, "show", "unknownpkg")
	if r.code != ExitNoResult || !strings.Contains(r.stderr, "paket bulunamadı") {
		t.Errorf("got %+v", r)
	}
	r = runFixture(t, "show", "zlib", "--os", "macos", "--json")
	if r.code != ExitNoResult || !strings.Contains(r.stdout, `"binaries": []`) {
		t.Errorf("got %+v", r)
	}
}

func TestShowArgErrors(t *testing.T) {
	r := runFixture(t, "show")
	if r.code == ExitOK || !strings.Contains(r.stderr, "argüman") {
		t.Errorf("got %+v", r)
	}
	r = runFixture(t, "show", "zlib/1.0@onlyuser")
	if r.code == ExitOK || !strings.Contains(r.stderr, "referans geçersiz") {
		t.Errorf("got %+v", r)
	}
}

func TestASCIIAndNoColorFlagsAccepted(t *testing.T) {
	r := runFixture(t, "--ascii", "--no-color", "list")
	if r.code != ExitOK {
		t.Fatalf("got %+v", r)
	}
}
