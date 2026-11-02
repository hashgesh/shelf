package tui

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

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

func TestMatrixMode(t *testing.T) {
	m := newTestModel(t, 200, 30, false)
	m = press(t, m, "m")
	if !m.matrixMode {
		t.Fatal("m should open the matrix")
	}
	v := m.View()
	for _, want := range []string{"Matrix", "linux-gcc12-armv8-release", "win-msvc194-x64-release", "zlib/1.3.1", "✓", "✗", "6 eksik"} {
		if !strings.Contains(v, want) {
			t.Errorf("matrix view missing %q", want)
		}
	}

	// Quick filters select columns; search selects rows.
	m = press(t, m, "w", "r")
	if mx := m.matrix(); len(mx.Profiles) != 1 || mx.Profiles[0].Name != "win-msvc194-x64-release" {
		t.Errorf("w+r profiles = %v", mx.Profiles)
	}
	m = press(t, m, "0")
	m = press(t, m, "/", "z", "l", "enter")
	if mx := m.matrix(); len(mx.Rows) != 1 || mx.Rows[0].Package != "zlib" {
		t.Errorf("search rows = %d", len(mx.Rows))
	}
	m = press(t, m, "esc") // esc in matrix leaves it; the search was already confirmed
	if m.matrixMode {
		t.Error("esc should leave the matrix")
	}
}

func TestMatrixEnterOpensBinaries(t *testing.T) {
	m := newTestModel(t, 200, 30, false)
	m = press(t, m, "m", "G") // zlib row
	for i := 0; i < 3; i++ {
		m = press(t, m, "right") // linux-gcc12-x86_64-release? columns are sorted by name
	}
	prof := m.matrix().Profiles[m.mxCols.pos].Name
	m = press(t, m, "enter")
	if m.matrixMode || m.currentPackage().Name != "zlib" || m.focus != focusBinaries {
		t.Fatalf("matrixMode=%v pkg=%s focus=%d", m.matrixMode, m.currentPackage().Name, m.focus)
	}
	if m.filter.Profile != prof {
		t.Errorf("profile filter = %q, want %q", m.filter.Profile, prof)
	}
	for _, b := range m.currentBinaries() {
		if !strings.Contains(strings.Join(b.Profiles, ","), prof) {
			t.Errorf("binary %s does not match %s", b, prof)
		}
	}
	if !strings.Contains(m.View(), "profil: "+prof) {
		t.Error("profile filter not shown")
	}
}

func TestMatrixColumnsScroll(t *testing.T) {
	m := newTestModel(t, 70, 20, true)
	m = press(t, m, "m")
	for i := 0; i < 4; i++ {
		m = press(t, m, "right")
	}
	v := m.View()
	if !strings.Contains(v, "win-msvc194-x64-release") || !strings.Contains(v, "sütun") {
		t.Errorf("last column not scrolled into view:\n%s", v)
	}
	for _, r := range []string{"×", "·", "✓", "✗"} {
		if strings.Contains(v, r) {
			t.Errorf("ASCII matrix contains %q", r)
		}
	}
	for i, l := range strings.Split(v, "\n") {
		if w := len([]rune(l)); w > 70 {
			t.Errorf("line %d too wide: %d", i, w)
		}
	}
}

func TestMatrixWithoutProfiles(t *testing.T) {
	m := newTestModel(t, 120, 20, false)
	m.result.Profiles = nil
	m = press(t, m, "m")
	if !strings.Contains(m.View(), "Profil yok") {
		t.Error("missing no-profiles message")
	}
}

func TestDetailMoreIndicator(t *testing.T) {
	m := newTestModel(t, 120, 20, false)
	m = press(t, m, "G", "tab", "tab", "enter")
	if !strings.Contains(m.View(), "satır daha") {
		t.Error("missing more-content indicator")
	}
	m = press(t, m, "G")
	if v := m.View(); strings.Contains(v, "satır daha") || !strings.Contains(v, "conan install --requires") {
		t.Errorf("bottom of detail:\n%s", v)
	}
}

func TestRefreshIndicatorKeepsItsStyle(t *testing.T) {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.ANSI)
	m := newModel(nil, "conan-local", newStyles(r, false))
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	m = update(t, m, loadedMsg{res: fixtureResult(t)})
	m.loading = true
	bar := m.topBar()

	// No reset between the indicator's style and its text: the old bar
	// rendered "yenileniyor" unstyled right after the spinner's reset.
	i := strings.Index(bar, "yenileniyor")
	if i < 0 {
		t.Fatalf("indicator missing: %q", bar)
	}
	before := bar[:i]
	if last := strings.LastIndex(before, "\x1b["); last >= 0 && strings.HasPrefix(before[last:], "\x1b[0m") {
		t.Errorf("indicator text follows a reset, so it is unstyled: %q", bar)
	}
	if w := lipgloss.Width(bar); w != 80 {
		t.Errorf("top bar width = %d, want 80", w)
	}

	// Narrow terminals keep the indicator and cut the left side instead.
	m = update(t, m, tea.WindowSizeMsg{Width: 30, Height: 20})
	if bar := m.topBar(); !strings.Contains(bar, "yenileniyor") || lipgloss.Width(bar) != 30 {
		t.Errorf("narrow bar = %q (width %d)", bar, lipgloss.Width(bar))
	}
}
