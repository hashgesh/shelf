// Command pack turns release binaries into archives: .zip for Windows and
// .tar.gz elsewhere, each holding the binary plus extra files, and writes
// SHA256SUMS. It is used by both the Makefile and build.ps1 so archives are
// identical on every build host; in particular the Unix binary always gets
// mode 0755, which tar.exe on Windows could not preserve.
//
// Usage:
//
//	go run ./tools/pack -version v0.1.0 -in build/release -out dist -extra README.md,LICENSE
//
// -in must contain one directory per target named <os>_<arch> (for example
// linux_amd64) holding exactly one binary.
package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// entry is one file inside an archive.
type entry struct {
	name string // path inside the archive
	src  string // file on disk
	mode os.FileMode
}

func main() {
	var (
		name    = flag.String("name", "shelf", "program name used in archive file names")
		version = flag.String("version", "", "version used in archive file names (required)")
		in      = flag.String("in", "build/release", "directory with <os>_<arch> subdirectories")
		out     = flag.String("out", "dist", "output directory for archives and SHA256SUMS")
		extra   = flag.String("extra", "README.md,LICENSE", "comma-separated files added to every archive")
	)
	flag.Parse()
	if err := run(*name, *version, *in, *out, splitList(*extra)); err != nil {
		fmt.Fprintln(os.Stderr, "pack:", err)
		os.Exit(1)
	}
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func run(name, version, in, out string, extras []string) error {
	if version == "" {
		return errors.New("-version is required")
	}
	targets, err := os.ReadDir(in)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}

	var archives []string
	for _, t := range targets {
		if !t.IsDir() {
			continue
		}
		goos, _, ok := strings.Cut(t.Name(), "_")
		if !ok {
			return fmt.Errorf("%s: directory name must be <os>_<arch>", t.Name())
		}
		bin, err := singleFile(filepath.Join(in, t.Name()))
		if err != nil {
			return err
		}
		entries := []entry{{name: filepath.Base(bin), src: bin, mode: 0o755}}
		for _, x := range extras {
			entries = append(entries, entry{name: filepath.Base(x), src: x, mode: 0o644})
		}

		base := fmt.Sprintf("%s_%s_%s", name, version, t.Name())
		var archive string
		if goos == "windows" {
			archive = filepath.Join(out, base+".zip")
			err = writeZip(archive, entries)
		} else {
			archive = filepath.Join(out, base+".tar.gz")
			err = writeTarGz(archive, entries)
		}
		if err != nil {
			return fmt.Errorf("%s: %w", archive, err)
		}
		archives = append(archives, archive)
		fmt.Println(archive)
	}
	if len(archives) == 0 {
		return fmt.Errorf("no <os>_<arch> directories in %s", in)
	}
	return writeChecksums(filepath.Join(out, "SHA256SUMS"), archives)
}

// singleFile returns the only regular file in dir.
func singleFile(dir string) (string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var found []string
	for _, f := range files {
		if f.Type().IsRegular() {
			found = append(found, filepath.Join(dir, f.Name()))
		}
	}
	if len(found) != 1 {
		return "", fmt.Errorf("%s: expected exactly one binary, found %d", dir, len(found))
	}
	return found[0], nil
}

func writeTarGz(path string, entries []entry) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		st, err := os.Stat(e.src)
		if err != nil {
			return err
		}
		hdr := &tar.Header{
			Name:    e.name,
			Mode:    int64(e.mode),
			Size:    st.Size(),
			ModTime: st.ModTime().UTC().Truncate(time.Second),
			Format:  tar.FormatPAX,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if err := copyFile(tw, e.src); err != nil {
			return err
		}
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func writeZip(path string, entries []entry) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	zw := zip.NewWriter(f)
	for _, e := range entries {
		st, err := os.Stat(e.src)
		if err != nil {
			return err
		}
		hdr, err := zip.FileInfoHeader(st)
		if err != nil {
			return err
		}
		hdr.Name = e.name
		hdr.Method = zip.Deflate
		hdr.SetMode(e.mode)
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if err := copyFile(w, e.src); err != nil {
			return err
		}
	}
	return zw.Close()
}

func copyFile(w io.Writer, src string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = io.Copy(w, f)
	return err
}

// writeChecksums writes "<sha256>  <name>" lines, the format sha256sum -c reads.
func writeChecksums(path string, files []string) error {
	sort.Strings(files)
	var b strings.Builder
	for _, file := range files {
		sum, err := sha256File(file)
		if err != nil {
			return err
		}
		fmt.Fprintf(&b, "%s  %s\n", sum, filepath.Base(file))
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
