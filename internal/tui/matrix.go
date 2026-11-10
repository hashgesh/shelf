package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-runewidth"

	"github.com/hashgesh/shelf/internal/conan"
	"github.com/hashgesh/shelf/internal/i18n"
)

// matrix returns the packages × profiles matrix for the current search and
// quick filters (which select profile columns).
func (m model) matrix() *conan.Matrix {
	if m.result == nil {
		return &conan.Matrix{}
	}
	return conan.BuildMatrix(m.result.Catalog, m.result.Profiles, conan.MatrixOptions{
		Filter:   conan.Filter{OS: m.filter.OS, BuildType: m.filter.BuildType},
		Packages: m.pkgs(),
	})
}

const (
	matrixRefMax = 34
	matrixColMax = 26
)

// matrixLayout returns the first column width, the profile column width and
// how many profile columns fit into innerW.
func (m model) matrixLayout(mx *conan.Matrix, innerW int) (refW, colW, visible int) {
	refW = 7
	for _, r := range mx.Rows {
		refW = max(refW, runewidth.StringWidth(r.Ref))
	}
	refW = min(refW, matrixRefMax) + 2 // cursor mark
	colW = 3
	for _, p := range mx.Profiles {
		colW = max(colW, runewidth.StringWidth(p.Name))
	}
	colW = min(colW, matrixColMax)
	if innerW-refW < colW+colGap {
		refW = max(innerW-colW-colGap, 6)
	}
	visible = max((innerW-refW)/(colW+colGap), 1)
	return refW, colW, visible
}

// scrollMatrix keeps the matrix cursor and column offset valid.
func (m *model) scrollMatrix() {
	mx := m.matrix()
	m.mxRows.setLen(len(mx.Rows))
	m.mxCols.setLen(len(mx.Profiles))
	m.mxRows.window(max(m.height-barHeight, 1) - 5)
	_, _, visible := m.matrixLayout(mx, max(m.width-2, 1))
	m.mxCols.window(visible)
}

func (m model) handleMatrixKey(key string) (model, tea.Cmd) {
	switch key {
	case "up", "k":
		m.mxRows.move(-1)
	case "down", "j":
		m.mxRows.move(1)
	case "pgup":
		m.mxRows.move(-10)
	case "pgdown":
		m.mxRows.move(10)
	case "home", "g":
		m.mxRows.move(-m.mxRows.n)
	case "end", "G":
		m.mxRows.move(m.mxRows.n)
	case "left", "shift+tab":
		m.mxCols.move(-1)
	case "right", "tab":
		m.mxCols.move(1)
	case "enter":
		return m.openMatrixCell(), nil
	case "m", "esc":
		m.matrixMode = false
	}
	return m, nil
}

// openMatrixCell switches to the browser on the selected package, filtered
// to the selected profile.
func (m model) openMatrixCell() model {
	mx := m.matrix()
	if m.mxRows.pos >= len(mx.Rows) || m.mxCols.pos >= len(mx.Profiles) {
		return m
	}
	row, prof := mx.Rows[m.mxRows.pos], mx.Profiles[m.mxCols.pos]
	m.matrixMode = false
	m.selectPackage(row.Package)
	m.versions.reset()
	m.binaries.reset()
	m.filter.Profile = prof.Name
	m.focus = focusBinaries
	m.syncCursors()
	return m
}

func (m model) cellSymbol(s conan.CellState) string {
	switch {
	case s == conan.CellAvailable && m.st.ascii:
		return "+"
	case s == conan.CellAvailable:
		return "✓"
	case s == conan.CellUnknown:
		return "?"
	case m.st.ascii:
		return "-"
	}
	return "✗"
}

func (m model) matrixView(h int) string {
	w := m.width
	innerW := max(w-2, 1)
	mx := m.matrix()
	refW, colW, visible := m.matrixLayout(mx, innerW)
	rows := max(h-5, 0) // border, title, header, legend
	gap := strings.Repeat(" ", colGap)

	c0, c1 := m.mxCols.window(visible)
	r0, r1 := m.mxRows.window(rows)

	var head strings.Builder
	head.WriteString(m.st.fit(i18n.T("  Package"), refW))
	for c := c0; c < c1; c++ {
		head.WriteString(gap + m.st.fit(mx.Profiles[c].Name, colW))
	}
	lines := []string{m.st.header.Render(m.st.fit(head.String(), innerW))}

	for r := r0; r < r1; r++ {
		row := mx.Rows[r]
		prefix := "  "
		if r == m.mxRows.pos {
			prefix = m.st.cursorMark()
		}
		var b strings.Builder
		ref := m.st.fit(prefix+row.Ref, refW)
		if r == m.mxRows.pos {
			ref = m.st.inactive.Render(ref)
		}
		b.WriteString(ref)
		for c := c0; c < c1; c++ {
			state := row.Cells[c].State
			sym := m.cellSymbol(state)
			if n := len(row.Cells[c].Binaries); n > 1 {
				sym += fmt.Sprintf(" %d", n)
			}
			cell := m.st.fit(" "+sym, colW)
			style := m.st.badText
			switch state {
			case conan.CellAvailable:
				style = m.st.okText
			case conan.CellUnknown:
				style = m.st.warnText
			}
			if r == m.mxRows.pos && c == m.mxCols.pos {
				style = m.st.selected
			}
			b.WriteString(gap + style.Render(cell))
		}
		used := runewidth.StringWidth(m.st.fit(prefix+row.Ref, refW)) + (c1-c0)*(colW+colGap)
		b.WriteString(strings.Repeat(" ", max(innerW-used, 0)))
		lines = append(lines, b.String())
	}
	if len(mx.Rows) == 0 || len(mx.Profiles) == 0 {
		msg := i18n.T("  No packages to show")
		if len(mx.Profiles) == 0 {
			msg = i18n.T("  No profiles (check the profiles_dir setting), or the filter removed every profile")
		}
		lines = append(lines, m.st.dim.Render(m.st.fit(msg, innerW)))
	}
	for len(lines) < rows+1 {
		lines = append(lines, strings.Repeat(" ", innerW))
	}
	more := ""
	if c0 > 0 || c1 < len(mx.Profiles) {
		more = fmt.Sprintf(i18n.T("%scolumns %d-%d / %d (←→)"), m.st.dot(), c0+1, c1, len(mx.Profiles))
	}
	// The column position comes before the counts so a narrow fit keeps it.
	legend := fmt.Sprintf(i18n.T("%s available  %s missing  ? unclear"),
		m.cellSymbol(conan.CellAvailable), m.cellSymbol(conan.CellMissing)) + more + m.st.dot() +
		fmt.Sprintf(i18n.T("%d packages%s%d profiles%s%d missing"), len(mx.Rows), m.st.times(), len(mx.Profiles), m.st.dot(), mx.Missing())
	lines = append(lines, m.st.dim.Render(m.st.fit("  "+legend, innerW)))
	return m.panel("Matrix", lines, w, h, true)
}
