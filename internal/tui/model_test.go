package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/hashgesh/shelf/internal/artifactory"
	"github.com/hashgesh/shelf/internal/conan"
)

func fixtureResult(t *testing.T) *conan.LoadResult {
	t.Helper()
	fx, err := artifactory.LoadFixture("../../testdata/aql.json")
	if err != nil {
		t.Fatal(err)
	}
	res, err := conan.Load(context.Background(), fx, conan.LoadOptions{
		Repos: []string{"conan-local"}, ProfilesDir: "../../testdata/profiles",
	})
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func testStyles(ascii bool) styles {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.Ascii)
	return newStyles(r, ascii)
}

func newTestModel(t *testing.T, w, h int, ascii bool) model {
	t.Helper()
	res := fixtureResult(t)
	m := newModel(func(context.Context) (*conan.LoadResult, error) { return res, nil }, "conan-local", testStyles(ascii))
	m = update(t, m, tea.WindowSizeMsg{Width: w, Height: h})
	return update(t, m, loadedMsg{res: res})
}

func update(t *testing.T, m model, msg tea.Msg) model {
	t.Helper()
	next, _ := m.Update(msg)
	return next.(model)
}

func press(t *testing.T, m model, keys ...string) model {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "tab":
			msg = tea.KeyMsg{Type: tea.KeyTab}
		case "shift+tab":
			msg = tea.KeyMsg{Type: tea.KeyShiftTab}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "up":
			msg = tea.KeyMsg{Type: tea.KeyUp}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "left":
			msg = tea.KeyMsg{Type: tea.KeyLeft}
		case "backspace":
			msg = tea.KeyMsg{Type: tea.KeyBackspace}
		case "f5":
			msg = tea.KeyMsg{Type: tea.KeyF5}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		m = update(t, m, msg)
	}
	return m
}

func TestLoadingAndError(t *testing.T) {
	m := newModel(nil, "conan-local", testStyles(false))
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 30})
	if v := m.View(); !strings.Contains(v, "veriler alınıyor") || !strings.Contains(v, "shelf · conan-local") {
		t.Errorf("loading view:\n%s", v)
	}

	m = update(t, m, loadedMsg{err: errors.New("HTTP 401")})
	if v := m.View(); !strings.Contains(v, "Hata:") || !strings.Contains(v, "HTTP 401") || !strings.Contains(v, "R ile tekrar dene") {
		t.Errorf("error view:\n%s", v)
	}

	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("R")})
	if !next.(model).loading || cmd == nil {
		t.Error("R should start a reload")
	}
}

func TestWideLayout(t *testing.T) {
	m := newTestModel(t, 140, 30, false)
	v := m.View()
	if !strings.Contains(v, "shelf · conan-local · 4 paket") {
		t.Errorf("top bar missing:\n%s", v)
	}
	sideBySide := false
	for _, l := range strings.Split(v, "\n") {
		if strings.Contains(l, "Paketler") && strings.Contains(l, "Sürümler") {
			sideBySide = true
		}
	}
	if !sideBySide {
		t.Errorf("panels should be side by side at 140 columns:\n%s", v)
	}
	// First package is fmt; its newest version is 11.0.2 with one Linux binary.
	for _, want := range []string{"fmt", "11.0.2@mycompany/stable", "OS", "Profil", "linux-gcc12-x86_64-release"} {
		if !strings.Contains(v, want) {
			t.Errorf("missing %q", want)
		}
	}
}

func TestNarrowLayoutStacks(t *testing.T) {
	m := newTestModel(t, 90, 40, false)
	for _, l := range strings.Split(m.View(), "\n") {
		if strings.Contains(l, "Paketler") && strings.Contains(l, "Sürümler") {
			t.Fatalf("panels should stack below %d columns", narrowWidth)
		}
	}
}

func TestViewFitsTerminal(t *testing.T) {
	for _, ascii := range []bool{false, true} {
		for _, sz := range [][2]int{{60, 20}, {80, 24}, {99, 30}, {100, 30}, {160, 50}, {40, 12}} {
			t.Run(fmt.Sprintf("%dx%d ascii=%v", sz[0], sz[1], ascii), func(t *testing.T) {
				m := newTestModel(t, sz[0], sz[1], ascii)
				m = press(t, m, "G", "tab", "tab")
				for _, v := range []string{m.View(), press(t, m, "?").View()} {
					lines := strings.Split(v, "\n")
					if len(lines) > sz[1] {
						t.Errorf("height %d > %d", len(lines), sz[1])
					}
					for i, l := range lines {
						if w := lipgloss.Width(l); w > sz[0] {
							t.Errorf("line %d width %d > %d: %q", i, w, sz[0], l)
						}
					}
				}
			})
		}
	}
}

func TestASCIIMode(t *testing.T) {
	v := newTestModel(t, 140, 30, true).View()
	for _, r := range []string{"·", "╭", "┃", "›", "●", "↑"} {
		if strings.Contains(v, r) {
			t.Errorf("ASCII view contains %q", r)
		}
	}
}

func TestNavigation(t *testing.T) {
	m := newTestModel(t, 140, 30, false)
	if p := m.currentPackage(); p.Name != "fmt" {
		t.Fatalf("initial package = %s", p.Name)
	}
	m = press(t, m, "j", "j", "j")
	if p := m.currentPackage(); p.Name != "zlib" {
		t.Fatalf("after jjj package = %s", p.Name)
	}
	m = press(t, m, "j") // stays at the end
	if m.currentPackage().Name != "zlib" {
		t.Error("cursor moved past the end")
	}

	m = press(t, m, "tab", "down")
	if v := m.currentVersion(); v.Version != "1.2.13" {
		t.Errorf("version = %s", v.Version)
	}
	if n := len(m.currentBinaries()); n != 2 {
		t.Errorf("zlib/1.2.13 binaries = %d", n)
	}

	m = press(t, m, "tab", "j", "j")
	if m.focus != focusBinaries || m.binaries.pos != 1 {
		t.Errorf("focus=%d binaries.pos=%d", m.focus, m.binaries.pos)
	}

	// Changing the package resets version and binary selection.
	m = press(t, m, "tab", "k")
	if m.focus != focusPackages || m.currentPackage().Name != "openssl" || m.versions.pos != 0 || m.binaries.pos != 0 {
		t.Errorf("after package change: focus=%d pkg=%s ver=%d bin=%d", m.focus, m.currentPackage().Name, m.versions.pos, m.binaries.pos)
	}
	m = press(t, m, "shift+tab")
	if m.focus != focusBinaries {
		t.Errorf("shift+tab focus = %d", m.focus)
	}
	m = press(t, m, "left", "left", "left")
	if m.focus != focusPackages {
		t.Errorf("left focus = %d", m.focus)
	}
}

func TestRefreshKeepsSelection(t *testing.T) {
	m := newTestModel(t, 140, 30, false)
	m = press(t, m, "G", "tab", "j")
	if m.currentVersion().Version != "1.2.13" {
		t.Fatal("setup")
	}
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF5})
	m = next.(model)
	if !m.loading || cmd == nil {
		t.Fatal("F5 should reload")
	}
	if !strings.Contains(m.View(), "yenileniyor") {
		t.Error("refresh indicator missing")
	}
	m = update(t, m, loadedMsg{res: fixtureResult(t)})
	if m.currentPackage().Name != "zlib" || m.currentVersion().Version != "1.2.13" {
		t.Errorf("selection lost: %s %s", m.currentPackage().Name, m.currentVersion().Version)
	}

	// A failed refresh keeps the old data and reports in the status bar.
	m = press(t, m, "R")
	m = update(t, m, loadedMsg{err: errors.New("timeout")})
	if v := m.View(); !strings.Contains(v, "yenileme başarısız: timeout") || !strings.Contains(v, "zlib") {
		t.Errorf("failed refresh view:\n%s", v)
	}
}

func TestHelpAndQuit(t *testing.T) {
	m := newTestModel(t, 120, 30, false)
	m = press(t, m, "?")
	if !strings.Contains(m.View(), "Kısayollar") {
		t.Error("help not shown")
	}
	m = press(t, m, "j") // ignored while help is open
	if m.packages.pos != 0 {
		t.Error("navigation while help open")
	}
	m = press(t, m, "esc")
	if m.showHelp {
		t.Error("esc should close help")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil || cmd() != tea.Quit() {
		t.Error("q should quit")
	}
}

func TestScrollKeepsOffset(t *testing.T) {
	// Many packages in a short terminal force scrolling.
	var items []artifactory.Item
	for i := 0; i < 40; i++ {
		items = append(items, artifactory.Item{
			Repo: "r", Name: conan.PackageFile,
			Path: fmt.Sprintf("_/pkg%02d/1.0/_/rrev/package/id/prev", i),
		})
	}
	cat, _ := conan.BuildCatalog(items, nil)
	res := &conan.LoadResult{Catalog: cat}
	m := newModel(nil, "r", testStyles(false))
	m = update(t, m, tea.WindowSizeMsg{Width: 140, Height: 20})
	m = update(t, m, loadedMsg{res: res})

	rows := m.dims().pkgRows
	for i := 0; i < rows+5; i++ {
		m = press(t, m, "j")
	}
	if m.packages.off == 0 {
		t.Fatal("list did not scroll")
	}
	off := m.packages.off
	m = press(t, m, "k")
	if m.packages.off != off {
		t.Errorf("moving up inside the window changed offset %d -> %d", off, m.packages.off)
	}
	if !strings.Contains(m.View(), m.currentPackage().Name) {
		t.Error("selected package not visible")
	}
}
