package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/halilibrahimonus/shelf/internal/conan"
)

// run applies msg and then feeds the command's result back, like the
// Bubble Tea runtime would (ticks are skipped).
func run(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	next, cmd := m.Update(msg)
	m = next.(model)
	if cmd == nil {
		return m
	}
	switch out := cmd().(type) {
	case copiedMsg, statusMsg:
		next, _ = m.Update(out)
		return next.(model)
	}
	return m
}

func init() { statusTTL = time.Millisecond }

func keyRunes(s string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func selectZlib(t *testing.T, w int) model {
	m := newTestModel(t, w, 40, false)
	return press(t, m, "G") // zlib is last
}

func TestQuickFilters(t *testing.T) {
	m := selectZlib(t, 140)
	if n := len(m.currentBinaries()); n != 9 {
		t.Fatalf("binaries = %d", n)
	}
	m = press(t, m, "w")
	if n := len(m.currentBinaries()); n != 4 {
		t.Errorf("w: %d binaries", n)
	}
	m = press(t, m, "r")
	if n := len(m.currentBinaries()); n != 2 {
		t.Errorf("w+r: %d binaries", n)
	}
	v := m.View()
	for _, want := range []string{"filtre: Windows Release", "2/9 binary", "zlib (3)", "win-msvc194-x64-release"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(v, "linux-gcc12") {
		t.Error("Linux binary visible with Windows filter")
	}

	m = press(t, m, "l", "d") // switch to Linux Debug
	if m.filter.OS != "Linux" || m.filter.BuildType != "Debug" || len(m.currentBinaries()) != 2 {
		t.Errorf("filter=%+v binaries=%d", m.filter, len(m.currentBinaries()))
	}
	m = press(t, m, "d") // toggle off
	if m.filter.BuildType != "" {
		t.Error("d should toggle Debug off")
	}

	m = press(t, m, "tab", "tab", "G")
	m = press(t, m, "w") // cursor beyond the shorter list is clamped
	if m.binaries.pos >= len(m.currentBinaries()) {
		t.Errorf("cursor %d out of range %d", m.binaries.pos, len(m.currentBinaries()))
	}

	m = run(t, m, keyRunes("0"))
	if !m.filter.IsZero() || len(m.currentBinaries()) != 9 || !strings.Contains(m.View(), "Filtreler temizlendi") {
		t.Errorf("after 0: filter=%+v", m.filter)
	}
}

func TestFilterWithNoMatches(t *testing.T) {
	m := newTestModel(t, 140, 40, false)
	m = press(t, m, "j") // nlohmann_json: header-only, no OS
	m = press(t, m, "w")
	if !strings.Contains(m.View(), "Filtreye uyan binary yok") {
		t.Error("empty filter message missing")
	}
}

func TestSearch(t *testing.T) {
	m := newTestModel(t, 140, 40, false)
	m = press(t, m, "/", "s", "s")
	if !m.searching || m.search != "ss" {
		t.Fatalf("searching=%v search=%q", m.searching, m.search)
	}
	if ps := m.pkgs(); len(ps) != 1 || ps[0].Name != "openssl" {
		t.Fatalf("pkgs = %v", ps)
	}
	if v := m.View(); !strings.Contains(v, "/ss") || !strings.Contains(v, "Paketler (1/4)") {
		t.Errorf("search view:\n%s", v)
	}
	// Keys go to the input while searching.
	m = press(t, m, "q")
	if m.search != "ssq" {
		t.Errorf("q should be typed, search=%q", m.search)
	}
	m = press(t, m, "backspace", "enter")
	if m.searching || m.search != "ss" || m.currentPackage().Name != "openssl" {
		t.Errorf("after enter: searching=%v search=%q", m.searching, m.search)
	}
	if !strings.Contains(m.View(), "ara: ss") {
		t.Error("confirmed search not shown in status bar")
	}

	// A search that keeps the selected package keeps it selected.
	m = press(t, m, "/", "backspace", "backspace", "o")
	if m.currentPackage().Name != "openssl" {
		t.Errorf("selection jumped to %s", m.currentPackage().Name)
	}
	m = press(t, m, "esc")
	if m.search != "" || len(m.pkgs()) != 4 {
		t.Errorf("esc should clear search: %q", m.search)
	}

	m = press(t, m, "/", "z", "z", "z", "enter")
	if len(m.pkgs()) != 0 || m.currentPackage() != nil {
		t.Error("expected no packages")
	}
	_ = m.View() // must not panic with an empty list
}

func TestDetailWindow(t *testing.T) {
	m := selectZlib(t, 140)
	m = press(t, m, "tab", "tab", "enter")
	if m.detail == nil {
		t.Fatal("enter on a binary should open the detail window")
	}
	b := m.detail.binary
	v := m.View()
	for _, want := range []string{b.PackageID, b.PackageRevision, b.RecipeRevision, "Settings", "Options", "shared", "conan install", "-pr:h"} {
		if !strings.Contains(v, want) {
			t.Errorf("detail missing %q", want)
		}
	}
	// Scrolling is clamped to the content.
	m = press(t, m, "G")
	off := m.detail.off
	m = press(t, m, "j")
	if m.detail.off != off {
		t.Errorf("scrolled past the end: %d -> %d", off, m.detail.off)
	}
	m = press(t, m, "g")
	if m.detail.off != 0 {
		t.Error("g should scroll to top")
	}
	m = press(t, m, "esc")
	if m.detail != nil {
		t.Error("esc should close detail")
	}
}

func TestDetailFitsSmallTerminal(t *testing.T) {
	m := newTestModel(t, 50, 15, true)
	m = press(t, m, "G", "tab", "tab", "enter")
	for i, l := range strings.Split(m.View(), "\n") {
		if w := len([]rune(l)); w > 50 {
			t.Errorf("line %d too wide (%d)", i, w)
		}
	}
}

func TestCopyInstall(t *testing.T) {
	m := selectZlib(t, 140)
	var copied string
	m.copy = func(s string) (string, error) { copied = s; return "xclip", nil }
	m = press(t, m, "tab", "tab")
	m = run(t, m, keyRunes("c"))
	want := conan.InstallCommand(m.currentBinary())
	if copied != want {
		t.Fatalf("copied %q, want %q", copied, want)
	}
	if !strings.Contains(m.View(), "kopyalandı (xclip)") {
		t.Error("success status missing")
	}
	if m.detail != nil {
		t.Error("detail should stay closed on success")
	}
}

func TestCopyFailureShowsCommand(t *testing.T) {
	m := selectZlib(t, 140)
	m.copy = func(string) (string, error) { return "", errors.New("pano kullanılamıyor") }
	m = run(t, m, keyRunes("c"))
	if m.detail == nil {
		t.Fatal("failed copy should open the detail window with the command")
	}
	if v := m.View(); !strings.Contains(v, "Panoya kopyalanamadı") || !strings.Contains(v, "conan install --requires") {
		t.Errorf("failure view:\n%s", v)
	}
}

func TestCopyWithoutBinary(t *testing.T) {
	m := newTestModel(t, 140, 40, false)
	m.copy = func(string) (string, error) { t.Fatal("copy should not be called"); return "", nil }
	m = press(t, m, "/", "z", "z", "z", "enter")
	m = run(t, m, keyRunes("c"))
	if !strings.Contains(m.View(), "seçili değil") {
		t.Error("missing 'no binary' status")
	}
}

func TestStatusExpires(t *testing.T) {
	m := newTestModel(t, 140, 40, false)
	cmd := m.setStatus("x", false)
	if cmd == nil || m.status != "x" {
		t.Fatal("status not set")
	}
	m = update(t, m, clearStatusMsg{seq: m.statusSeq - 1}) // stale tick
	if m.status != "x" {
		t.Error("stale clear removed a newer status")
	}
	m = update(t, m, clearStatusMsg{seq: m.statusSeq})
	if m.status != "" {
		t.Error("status not cleared")
	}
}
