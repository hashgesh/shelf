package tui

import (
	"context"
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/halilibrahimonus/shelf/internal/conan"
)

// Loader fetches the catalog. It is called on start and on every refresh.
type Loader func(ctx context.Context) (*conan.LoadResult, error)

type focus int

const (
	focusPackages focus = iota
	focusVersions
	focusBinaries
	focusCount
)

// narrowWidth is the width below which panels stack vertically.
const narrowWidth = 100

type loadedMsg struct {
	res *conan.LoadResult
	err error
}

type model struct {
	load      Loader
	repoLabel string
	st        styles
	spin      spinner.Model

	width, height int

	loading bool
	err     error
	result  *conan.LoadResult

	focus    focus
	packages cursor
	versions cursor
	binaries cursor
	showHelp bool
}

func newModel(load Loader, repoLabel string, st styles) model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	if st.ascii {
		sp.Spinner = spinner.Line
	}
	sp.Style = st.title
	return model{load: load, repoLabel: repoLabel, st: st, spin: sp, loading: true, width: 80, height: 24}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spin.Tick, m.fetch())
}

func (m model) fetch() tea.Cmd {
	load := m.load
	return func() tea.Msg {
		res, err := load(context.Background())
		return loadedMsg{res: res, err: err}
	}
}

// ---- selection helpers ----

func (m model) pkgs() []*conan.Package {
	if m.result == nil {
		return nil
	}
	return m.result.Catalog.Packages
}

func (m model) currentPackage() *conan.Package {
	ps := m.pkgs()
	if m.packages.pos < len(ps) {
		return ps[m.packages.pos]
	}
	return nil
}

func (m model) currentVersion() *conan.Version {
	p := m.currentPackage()
	if p == nil || m.versions.pos >= len(p.Versions) {
		return nil
	}
	return p.Versions[m.versions.pos]
}

func (m model) currentBinaries() []*conan.Binary {
	v := m.currentVersion()
	if v == nil || v.Latest() == nil {
		return nil
	}
	return v.Latest().Binaries
}

// syncCursors updates list lengths after data or selection changes.
func (m *model) syncCursors() {
	m.packages.setLen(len(m.pkgs()))
	n := 0
	if p := m.currentPackage(); p != nil {
		n = len(p.Versions)
	}
	m.versions.setLen(n)
	m.binaries.setLen(len(m.currentBinaries()))
}

// ---- update ----

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.scroll()
		return m, nil

	case spinner.TickMsg:
		if !m.loading {
			return m, nil
		}
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd

	case loadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.restoreSelection(msg.res)
			m.scroll()
		}
		return m, nil

	case tea.KeyMsg:
		next, cmd := m.handleKey(msg)
		nm := next.(model)
		nm.scroll()
		return nm, cmd
	}
	return m, nil
}

// restoreSelection installs a new result, keeping the selected package and
// version when they still exist (after a refresh).
func (m *model) restoreSelection(res *conan.LoadResult) {
	var pkgName, verRef string
	if p := m.currentPackage(); p != nil {
		pkgName = p.Name
		if v := m.currentVersion(); v != nil {
			verRef = v.Ref(p.Name)
		}
	}
	m.result = res
	m.packages.reset()
	m.versions.reset()
	m.binaries.reset()
	for i, p := range res.Catalog.Packages {
		if p.Name != pkgName {
			continue
		}
		m.packages.pos = i
		for j, v := range p.Versions {
			if v.Ref(p.Name) == verRef {
				m.versions.pos = j
			}
		}
	}
	m.syncCursors()
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	if m.showHelp {
		switch key {
		case "?", "esc", "q":
			m.showHelp = false
		}
		return m, nil
	}

	switch key {
	case "q":
		return m, tea.Quit
	case "?":
		m.showHelp = true
		return m, nil
	case "f5", "R":
		if !m.loading {
			m.loading = true
			return m, tea.Batch(m.spin.Tick, m.fetch())
		}
		return m, nil
	}
	if m.result == nil {
		return m, nil
	}

	switch key {
	case "tab":
		m.focus = (m.focus + 1) % focusCount
	case "shift+tab":
		m.focus = (m.focus + focusCount - 1) % focusCount
	case "up", "k":
		m.moveFocused(-1)
	case "down", "j":
		m.moveFocused(1)
	case "pgup":
		m.moveFocused(-10)
	case "pgdown":
		m.moveFocused(10)
	case "home", "g":
		m.moveFocused(-m.focusedCursor().n)
	case "end", "G":
		m.moveFocused(m.focusedCursor().n)
	case "left", "h":
		if m.focus > focusPackages {
			m.focus--
		}
	case "right", "l", "enter":
		if m.focus < focusBinaries {
			m.focus++
		}
	}
	return m, nil
}

func (m *model) focusedCursor() *cursor {
	switch m.focus {
	case focusVersions:
		return &m.versions
	case focusBinaries:
		return &m.binaries
	}
	return &m.packages
}

func (m *model) moveFocused(delta int) {
	switch m.focus {
	case focusPackages:
		before := m.packages.pos
		m.packages.move(delta)
		if m.packages.pos != before {
			m.versions.reset()
			m.binaries.reset()
		}
	case focusVersions:
		before := m.versions.pos
		m.versions.move(delta)
		if m.versions.pos != before {
			m.binaries.reset()
		}
	case focusBinaries:
		m.binaries.move(delta)
	}
	m.syncCursors()
}

// ---- view ----

func (m model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}
	top := m.topBar()
	bottom := m.bottomBar()
	bodyH := max(m.height-barHeight, 1)

	var body string
	switch {
	case m.result == nil && m.loading:
		body = m.center(m.spin.View()+" Artifactory'den veriler alınıyor...", bodyH)
	case m.result == nil && m.err != nil:
		body = m.center(m.st.errText.Render("Hata: ")+m.err.Error()+"\n\n"+
			m.st.dim.Render("R ile tekrar dene, q ile çık"), bodyH)
	case m.showHelp:
		body = m.helpView(bodyH)
	default:
		body = m.bodyView()
	}
	return lipgloss.JoinVertical(lipgloss.Left, top, body, bottom)
}

func (m model) center(text string, h int) string {
	return lipgloss.Place(m.width, max(h, 1), lipgloss.Center, lipgloss.Center, text)
}

func (m model) topBar() string {
	parts := []string{"shelf", m.repoLabel}
	if m.result != nil {
		parts = append(parts, fmt.Sprintf("%d paket", len(m.pkgs())))
	}
	left := " " + strings.Join(parts, m.st.dot())
	right := ""
	if m.loading && m.result != nil {
		right = m.spin.View() + " yenileniyor "
	}
	gap := max(m.width-lipgloss.Width(left)-lipgloss.Width(right), 0)
	return m.st.topBar.Render(left + strings.Repeat(" ", gap) + right)
}

func (m model) bottomBar() string {
	keys := [][2]string{{"↑↓/jk", "gezin"}, {"Tab", "panel"}, {"F5/R", "yenile"}, {"?", "yardım"}, {"q", "çıkış"}}
	if m.st.ascii {
		keys[0][0] = "j/k"
	}
	var parts []string
	for _, k := range keys {
		parts = append(parts, m.st.key.Render(k[0])+" "+k[1])
	}
	left := " " + strings.Join(parts, "  ")
	right := ""
	if m.result != nil && len(m.result.Warnings) > 0 {
		right = m.st.warnText.Render(fmt.Sprintf("%d uyarı ", len(m.result.Warnings)))
	}
	if m.err != nil && m.result != nil {
		right = m.st.errText.Render("yenileme başarısız: "+m.err.Error()) + " "
	}
	gap := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		// Drop the hint text rather than wrapping the bar.
		left, gap = "", max(m.width-lipgloss.Width(right), 0)
	}
	return m.st.bottomBar.Render(left + strings.Repeat(" ", gap) + right)
}

// panel draws a bordered box of outer size w×h with a title line.
func (m model) panel(title string, lines []string, w, h int, focused bool) string {
	innerW, innerH := max(w-2, 1), max(h-2, 1)
	style := m.st.panel
	marker := "  "
	if focused {
		style = m.st.focusedPanel
		marker = "* "
		if !m.st.ascii {
			marker = "● "
		}
	}
	out := []string{m.st.title.Render(m.st.fit(marker+title, innerW))}
	out = append(out, lines...)
	for len(out) < innerH {
		out = append(out, strings.Repeat(" ", innerW))
	}
	if len(out) > innerH {
		out = out[:innerH]
	}
	return style.Width(innerW).Height(innerH).Render(strings.Join(out, "\n"))
}

func (m model) listLines(items []string, c *cursor, innerW, rows int, focused bool) []string {
	start, end := c.window(rows)
	lines := make([]string, 0, rows)
	for i := start; i < end; i++ {
		prefix := "  "
		if i == c.pos {
			prefix = m.st.cursorMark()
		}
		text := m.st.fit(prefix+items[i], innerW)
		switch {
		case i == c.pos && focused:
			text = m.st.selected.Render(text)
		case i == c.pos:
			text = m.st.inactive.Render(text)
		}
		lines = append(lines, text)
	}
	if len(items) == 0 {
		lines = append(lines, m.st.dim.Render(m.st.fit("  (boş)", innerW)))
	}
	return lines
}

// dims is the panel geometry for the current terminal size.
type dims struct {
	narrow           bool
	leftW, rightW    int // wide layout widths; rightW is the full width when narrow
	pkgH, verH, tblH int // outer panel heights
	pkgRows, verRows int // visible list rows
	tblRows          int // visible table rows
}

// barHeight is the height of the top and bottom bars together.
const barHeight = 2

func (m model) dims() dims {
	h := max(m.height-barHeight, 1)
	nPkg, nVer := len(m.pkgs()), 0
	nameW := 10
	for _, p := range m.pkgs() {
		nameW = max(nameW, len(p.Name))
	}
	if p := m.currentPackage(); p != nil {
		nVer = len(p.Versions)
	}

	var d dims
	if m.width < narrowWidth {
		d.narrow = true
		d.leftW, d.rightW = m.width, m.width
		d.pkgH = max(min(nPkg+3, h/3), 4)
		d.verH = max(min(nVer+3, h/4), 4)
		d.tblH = max(h-d.pkgH-d.verH, 4)
		shrinkToFit(h, &d.tblH, &d.verH, &d.pkgH)
	} else {
		d.leftW = min(max(nameW+5, 18), m.width/4)
		d.rightW = m.width - d.leftW
		d.pkgH = h
		d.verH = max(min(nVer+3, h/3), 4)
		d.tblH = max(h-d.verH, 4)
		shrinkToFit(h, &d.tblH, &d.verH)
	}
	// Panel border (2) + title line; the table also has a header line.
	d.pkgRows, d.verRows, d.tblRows = d.pkgH-3, d.verH-3, d.tblH-4
	return d
}

// minPanelH is a border plus a title line.
const minPanelH = 3

// shrinkToFit reduces panel heights, in the given order of preference, until
// they add up to at most total.
func shrinkToFit(total int, heights ...*int) {
	sum := 0
	for _, h := range heights {
		sum += *h
	}
	for sum > total {
		shrunk := false
		for _, h := range heights {
			if *h > minPanelH && sum > total {
				*h--
				sum--
				shrunk = true
			}
		}
		if !shrunk {
			return
		}
	}
}

// scroll keeps every cursor inside its visible window. View works on a copy
// of the model, so offsets must be updated here to persist between frames.
func (m *model) scroll() {
	d := m.dims()
	m.packages.window(d.pkgRows)
	m.versions.window(d.verRows)
	m.binaries.window(d.tblRows)
}

func (m model) bodyView() string {
	d := m.dims()
	pkgNames := make([]string, 0, len(m.pkgs()))
	for _, p := range m.pkgs() {
		pkgNames = append(pkgNames, p.Name)
	}
	var verNames []string
	if p := m.currentPackage(); p != nil {
		for _, v := range p.Versions {
			label := strings.TrimPrefix(v.Ref(p.Name), p.Name+"/")
			n := 0
			if r := v.Latest(); r != nil {
				n = len(r.Binaries)
			}
			verNames = append(verNames, fmt.Sprintf("%s  (%d binary)", label, n))
		}
	}

	pkgFocus, verFocus := m.focus == focusPackages, m.focus == focusVersions
	pkgPanel := m.panel("Paketler", m.listLines(pkgNames, &m.packages, d.leftW-2, d.pkgRows, pkgFocus), d.leftW, d.pkgH, pkgFocus)
	verPanel := m.panel("Sürümler", m.listLines(verNames, &m.versions, d.rightW-2, d.verRows, verFocus), d.rightW, d.verH, verFocus)
	tblPanel := m.binaryPanel(d.rightW, d.tblH, d.tblRows)
	if d.narrow {
		return lipgloss.JoinVertical(lipgloss.Left, pkgPanel, verPanel, tblPanel)
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, pkgPanel, lipgloss.JoinVertical(lipgloss.Left, verPanel, tblPanel))
}

func (m model) binaryPanel(w, h, rows int) string {
	innerW := max(w-2, 1)
	focused := m.focus == focusBinaries
	bs := m.currentBinaries()

	title := "Binary'ler"
	if v := m.currentVersion(); v != nil && v.Latest() != nil {
		title = "Binary'ler" + m.st.dot() + v.Ref(m.currentPackage().Name) + m.st.dot() + "rrev " + shortID(v.Latest().Revision)
		if len(v.Revisions) > 1 {
			title += fmt.Sprintf(" (+%d eski revizyon)", len(v.Revisions)-1)
		}
	}

	cols, widths := layoutColumns(bs, innerW-2) // 2 cells for the cursor mark
	cell := func(i int, s string) string { return m.st.fit(s, widths[i]) }
	gap := strings.Repeat(" ", colGap)

	var header []string
	for i, c := range cols {
		header = append(header, cell(i, c.title))
	}
	lines := []string{m.st.header.Render(m.st.fit("  "+strings.Join(header, gap), innerW))}

	start, end := m.binaries.window(rows)
	for r := start; r < end; r++ {
		b := bs[r]
		var cells []string
		for i, c := range cols {
			cells = append(cells, cell(i, c.value(b)))
		}
		prefix := "  "
		if r == m.binaries.pos {
			prefix = m.st.cursorMark()
		}
		row := m.st.fit(prefix+strings.Join(cells, gap), innerW)
		switch {
		case r == m.binaries.pos && focused:
			row = m.st.selected.Render(row)
		case r == m.binaries.pos:
			row = m.st.inactive.Render(row)
		case b.Profile == "":
			row = m.st.dim.Render(row)
		}
		lines = append(lines, row)
	}
	if len(bs) == 0 {
		lines = append(lines, m.st.dim.Render(m.st.fit("  Binary yok", innerW)))
	}
	return m.panel(title, lines, w, h, focused)
}

func (m model) helpView(h int) string {
	rows := [][2]string{
		{"↑ ↓ / j k", "satır seç"},
		{"PgUp PgDn, g G", "sayfa / başa / sona git"},
		{"Tab, Shift+Tab", "panel değiştir"},
		{"← → / h l, Enter", "önceki / sonraki panel"},
		{"F5 / R", "verileri yeniden yükle"},
		{"?", "bu yardımı aç / kapat"},
		{"q, Ctrl+C", "çıkış"},
	}
	var b strings.Builder
	b.WriteString(m.st.title.Render("Kısayollar") + "\n\n")
	for _, r := range rows {
		b.WriteString(m.st.key.Render(m.st.fit(r[0], 20)) + "  " + r[1] + "\n")
	}
	b.WriteString("\n" + m.st.dim.Render("Kapatmak için ? veya Esc"))
	box := m.st.panel.Padding(1, 3).Render(b.String())
	if lipgloss.Width(box) <= m.width && lipgloss.Height(box) <= h {
		return m.center(box, h)
	}
	// Too small for the box: plain lines, clipped to the screen.
	lines := strings.Split(b.String(), "\n")
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lines[i] = lipgloss.NewStyle().MaxWidth(m.width).Render(l)
	}
	return lipgloss.PlaceVertical(h, lipgloss.Top, strings.Join(lines, "\n"))
}
