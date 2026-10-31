package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func TestPack(t *testing.T) {
	dir := t.TempDir()
	in, out := filepath.Join(dir, "in"), filepath.Join(dir, "out")
	// Deliberately non-executable on disk: pack must still mark binaries 0755.
	writeFile(t, filepath.Join(in, "linux_amd64", "shelf"), "linux-bin", 0o644)
	writeFile(t, filepath.Join(in, "windows_amd64", "shelf.exe"), "windows-bin", 0o644)
	readme, license := filepath.Join(dir, "README.md"), filepath.Join(dir, "LICENSE")
	writeFile(t, readme, "readme", 0o644)
	writeFile(t, license, "license", 0o644)

	if err := run("shelf", "v1.2.3", in, out, []string{readme, license}); err != nil {
		t.Fatal(err)
	}

	tgz := filepath.Join(out, "shelf_v1.2.3_linux_amd64.tar.gz")
	files := readTarGz(t, tgz)
	if got := keys(files); got != "LICENSE,README.md,shelf" {
		t.Errorf("tar entries = %s", got)
	}
	if files["shelf"].mode != 0o755 || files["shelf"].body != "linux-bin" {
		t.Errorf("tar binary = %+v", files["shelf"])
	}
	if files["README.md"].mode != 0o644 {
		t.Errorf("README mode = %o", files["README.md"].mode)
	}

	zf := filepath.Join(out, "shelf_v1.2.3_windows_amd64.zip")
	files = readZip(t, zf)
	if got := keys(files); got != "LICENSE,README.md,shelf.exe" {
		t.Errorf("zip entries = %s", got)
	}
	if files["shelf.exe"].body != "windows-bin" {
		t.Errorf("zip binary = %+v", files["shelf.exe"])
	}

	sums, err := os.ReadFile(filepath.Join(out, "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(sums)), "\n")
	if len(lines) != 2 {
		t.Fatalf("SHA256SUMS:\n%s", sums)
	}
	for _, l := range lines {
		sum, name, ok := strings.Cut(l, "  ")
		if !ok {
			t.Fatalf("bad line %q", l)
		}
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(data)
		if hex.EncodeToString(h[:]) != sum {
			t.Errorf("checksum mismatch for %s", name)
		}
	}
}

func TestPackErrors(t *testing.T) {
	dir := t.TempDir()
	if err := run("shelf", "", dir, dir, nil); err == nil {
		t.Error("missing version accepted")
	}
	if err := run("shelf", "v1", dir, filepath.Join(dir, "out"), nil); err == nil {
		t.Error("empty input accepted")
	}
	writeFile(t, filepath.Join(dir, "linux_amd64", "a"), "a", 0o755)
	writeFile(t, filepath.Join(dir, "linux_amd64", "b"), "b", 0o755)
	if err := run("shelf", "v1", dir, filepath.Join(dir, "out"), nil); err == nil || !strings.Contains(err.Error(), "exactly one") {
		t.Errorf("two binaries: err = %v", err)
	}
}

type packed struct {
	mode os.FileMode
	body string
}

func keys(m map[string]packed) string {
	var k []string
	for name := range m {
		k = append(k, name)
	}
	sort.Strings(k)
	return strings.Join(k, ",")
}

func readTarGz(t *testing.T, path string) map[string]packed {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string]packed{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		out[hdr.Name] = packed{mode: os.FileMode(hdr.Mode).Perm(), body: string(b)}
	}
}

func readZip(t *testing.T, path string) map[string]packed {
	t.Helper()
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = zr.Close() }()
	out := map[string]packed{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		out[f.Name] = packed{mode: f.Mode().Perm(), body: string(b)}
	}
	return out
}
