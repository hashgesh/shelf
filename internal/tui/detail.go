package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/halilibrahimonus/shelf/internal/conan"
	"github.com/halilibrahimonus/shelf/internal/format"
)

// detail is the modal window for one binary.
type detail struct {
	binary *conan.Binary
	off    int
}

func newDetail(b *conan.Binary) *detail { return &detail{binary: b} }

func (d *detail) scroll(delta int) {
	d.off = max(d.off+delta, 0)
}

// renderFunc matches lipgloss.Style.Render.
type renderFunc func(...string) string

type detailLine struct {
	text  string
	style renderFunc
}

const detailKeyW = 18

// lines renders the content for an inner width w; long values wrap.
func (d *detail) lines(st styles, w int) []detailLine {
	b := d.binary
	var out []detailLine
	plain := renderFunc(func(s ...string) string { return strings.Join(s, "") })
	add := func(text string, style renderFunc) {
		out = append(out, detailLine{text, style})
	}
	section := func(title string) {
		add("", plain)
		add(title, st.title.Render)
	}
	kv := func(k, v string) {
		valueW := max(w-detailKeyW-2, 8)
		for i, chunk := range wrapHard(v, valueW) {
			key := ""
			if i == 0 {
				key = k
			}
			add("  "+runewidth.FillRight(key, detailKeyW)+chunk, plain)
		}
	}

	add(b.Ref()+st.dot()+b.Label(), st.title.Render)
	section("Kimlik")
	kv("Package ID", b.PackageID)
	kv("Package revision", b.PackageRevision)
	kv("Recipe revision", b.RecipeRevision)
	kv("Repo", b.Repo)
	kv("Path", b.Path)
	kv("Boyut", format.Size(b.Size))
	kv("Yüklenme", format.Time(b.Created)+st.dot()+format.Dash(b.CreatedBy))
	if b.OlderRevisions > 0 {
		kv("Eski revizyon", fmt.Sprintf("%d (en yenisi gösteriliyor)", b.OlderRevisions))
	}
	kv("Metadata", metadataLabel(b.MetadataSource))

	section("Profil")
	switch {
	case b.Profile != "":
		kv("Eşleşen", b.Profile)
		if len(b.Profiles) > 1 {
			kv("Diğer eşleşenler", strings.Join(others(b.Profiles, b.Profile), ", "))
		}
	case len(b.Profiles) > 0:
		kv("Eşleşen", fmt.Sprintf("tüm profiller (%d)", len(b.Profiles)))
	default:
		kv("Eşleşen", "yok"+st.dot()+"özet: "+b.Summary())
	}

	section("Settings")
	if len(b.Settings) == 0 {
		kv("-", "(yok)")
	}
	for _, k := range sortedKeys(b.Settings) {
		kv(k, b.Settings[k])
	}
	section("Options")
	if len(b.Options) == 0 {
		kv("-", "(yok)")
	}
	for _, k := range sortedKeys(b.Options) {
		kv(k, b.Options[k])
	}

	section("conan install")
	for _, chunk := range wrapHard(conan.InstallCommand(b), max(w-2, 8)) {
		add("  "+chunk, st.key.Render)
	}
	return out
}

func metadataLabel(src string) string {
	switch src {
	case conan.SourceProperties:
		return "Artifactory property'leri"
	case conan.SourceConaninfo:
		return "conaninfo.txt"
	}
	return "bilinmiyor (property ve conaninfo.txt yok)"
}

func others(all []string, skip string) []string {
	var out []string
	for _, s := range all {
		if s != skip {
			out = append(out, s)
		}
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// wrapHard splits s into pieces of at most w cells, preferring spaces.
func wrapHard(s string, w int) []string {
	if w <= 0 || runewidth.StringWidth(s) <= w {
		return []string{s}
	}
	var out []string
	for runewidth.StringWidth(s) > w {
		cut := runewidth.Truncate(s, w, "")
		if i := strings.LastIndexByte(cut, ' '); i > w/2 {
			cut = cut[:i]
		}
		out = append(out, cut)
		s = strings.TrimLeft(s[len(cut):], " ")
	}
	if s != "" {
		out = append(out, s)
	}
	return out
}

// detailBox returns the outer size of the detail window.
func (m model) detailBox(bodyH int) (w, h int) {
	return min(m.width, 110), bodyH
}

// clampDetail keeps the detail scroll offset within its content.
func (m *model) clampDetail() {
	if m.detail == nil {
		return
	}
	w, h := m.detailBox(max(m.height-barHeight, 1))
	n := len(m.detail.lines(m.st, w-4))
	m.detail.off = max(min(m.detail.off, n-(h-2)), 0)
}

func (m model) detailView(bodyH int) string {
	w, h := m.detailBox(bodyH)
	innerW, rows := max(w-4, 1), max(h-2, 1) // border + 1 cell padding each side
	all := m.detail.lines(m.st, innerW)
	start := min(m.detail.off, max(len(all)-rows, 0))
	end := min(start+rows, len(all))

	var lines []string
	for _, l := range all[start:end] {
		lines = append(lines, l.style(m.st.fit(l.text, innerW)))
	}
	if end < len(all) && len(lines) > 0 {
		arrow := "↓"
		if m.st.ascii {
			arrow = "v"
		}
		lines[len(lines)-1] = m.st.dim.Render(m.st.fit(fmt.Sprintf("  %s %d satır daha (j/k ile kaydır)", arrow, len(all)-end+1), innerW))
	}
	for len(lines) < rows {
		lines = append(lines, strings.Repeat(" ", innerW))
	}
	box := m.st.focusedPanel.Padding(0, 1).Width(innerW + 2).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, bodyH, lipgloss.Center, lipgloss.Top, box)
}
