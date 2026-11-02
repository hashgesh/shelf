package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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
		body = m.center(m.st.loading.Render(strings.TrimSpace(m.spin.View())+" Artifactory'den veriler alınıyor..."), bodyH)
	case m.result == nil && m.err != nil:
		body = m.center(m.st.errText.Render("Hata: ")+m.err.Error()+"\n\n"+
			m.st.dim.Render("R ile tekrar dene, q ile çık"), bodyH)
	case m.showHelp:
		body = m.helpView(bodyH)
	case m.detail != nil:
		body = m.detailView(bodyH)
	case m.matrixMode:
		body = m.matrixView(bodyH)
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
	if m.matrixMode {
		parts = append(parts, "Matrix")
	}
	if m.result != nil {
		parts = append(parts, fmt.Sprintf("%d paket", len(m.pkgs())))
	}
	left := " " + strings.Join(parts, m.st.dot())
	right := ""
	if m.loading && m.result != nil {
		// Styled on its own: nesting it inside the bar's style would let the
		// inner reset sequence strip the bar style from the rest of the text.
		right = m.st.busy.Render(" " + strings.TrimSpace(m.spin.View()) + " yenileniyor ")
	}
	avail := max(m.width-lipgloss.Width(right), 0)
	left = lipgloss.NewStyle().MaxWidth(avail).Render(left)
	gap := max(avail-lipgloss.Width(left), 0)
	return m.st.topBar.Render(left+strings.Repeat(" ", gap)) + right
}

// hints returns the key hints for the current mode.
func (m model) hints() [][2]string {
	nav := "↑↓/jk"
	if m.st.ascii {
		nav = "j/k"
	}
	switch {
	case m.searching:
		return [][2]string{{"Enter", "onayla"}, {"Esc", "aramayı sil"}}
	case m.showHelp:
		return [][2]string{{"?/Esc", "kapat"}}
	case m.detail != nil:
		return [][2]string{{nav, "kaydır"}, {"c", "kopyala"}, {"Esc", "kapat"}}
	case m.result == nil:
		return [][2]string{{"R", "yeniden dene"}, {"q", "çıkış"}}
	case m.matrixMode:
		return [][2]string{
			{nav, "satır"}, {"←→", "profil"}, {"Enter", "binary'leri aç"}, {"/", "ara"},
			{"w/l", "Win/Linux"}, {"r/d", "Rel/Debug"}, {"0", "temizle"}, {"m", "geri"}, {"q", "çıkış"},
		}
	}
	return [][2]string{
		{nav, "gezin"}, {"Tab", "panel"}, {"Enter", "detay"}, {"/", "ara"},
		{"w/l", "Win/Linux"}, {"r/d", "Rel/Debug"}, {"0", "temizle"},
		{"c", "kopyala"}, {"m", "matrix"}, {"?", "yardım"}, {"q", "çıkış"},
	}
}

// activeFilters describes the filters for the status bar.
func (m model) activeFilters() string {
	var parts []string
	if m.filter.OS != "" {
		parts = append(parts, m.filter.OS)
	}
	if m.filter.BuildType != "" {
		parts = append(parts, m.filter.BuildType)
	}
	if m.filter.Profile != "" {
		parts = append(parts, "profil: "+m.filter.Profile)
	}
	if m.search != "" && !m.searching {
		parts = append(parts, "ara: "+m.search)
	}
	if len(parts) == 0 {
		return ""
	}
	return "filtre: " + strings.Join(parts, " ")
}

func (m model) bottomBar() string {
	var left string
	if m.searching {
		left = " " + m.input.View()
	} else {
		var parts []string
		for _, k := range m.hints() {
			parts = append(parts, m.st.key.Render(k[0])+" "+k[1])
		}
		left = " " + strings.Join(parts, "  ")
	}

	var right []string
	if f := m.activeFilters(); f != "" {
		right = append(right, m.st.title.Render(f))
	}
	switch {
	case m.status != "" && m.statusErr:
		right = append(right, m.st.errText.Render(m.status))
	case m.status != "":
		right = append(right, m.st.okText.Render(m.status))
	case m.err != nil && m.result != nil:
		right = append(right, m.st.errText.Render("yenileme başarısız: "+m.err.Error()))
	case m.result != nil && len(m.result.Warnings) > 0:
		right = append(right, m.st.warnText.Render(fmt.Sprintf("%d uyarı", len(m.result.Warnings))))
	}
	r := strings.Join(right, m.st.dot())
	if r != "" {
		r += " "
	}

	// When everything does not fit, the right side (state) wins over the
	// key hints, which are also in the help screen.
	if lipgloss.Width(left)+lipgloss.Width(r) > m.width {
		avail := max(m.width-lipgloss.Width(r)-1, 0)
		left = lipgloss.NewStyle().MaxWidth(avail).Render(left)
		if lipgloss.Width(r) > m.width {
			r = lipgloss.NewStyle().MaxWidth(m.width).Render(r)
			left = ""
		}
	}
	gap := max(m.width-lipgloss.Width(left)-lipgloss.Width(r), 0)
	return m.st.bottomBar.Render(left + strings.Repeat(" ", gap) + r)
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
	filtered := !m.filter.IsZero()
	pkgNames := make([]string, 0, len(m.pkgs()))
	for _, p := range m.pkgs() {
		name := p.Name
		if filtered {
			n := 0
			for _, v := range p.Versions {
				n += len(m.versionBinaries(v))
			}
			name = fmt.Sprintf("%s (%d)", p.Name, n)
		}
		pkgNames = append(pkgNames, name)
	}
	var verNames []string
	if p := m.currentPackage(); p != nil {
		for _, v := range p.Versions {
			label := strings.TrimPrefix(v.Ref(p.Name), p.Name+"/")
			total := 0
			if r := v.Latest(); r != nil {
				total = len(r.Binaries)
			}
			count := fmt.Sprintf("%d binary", total)
			if filtered {
				count = fmt.Sprintf("%d/%d binary", len(m.versionBinaries(v)), total)
			}
			verNames = append(verNames, label+"  ("+count+")")
		}
	}
	pkgTitle := "Paketler"
	if m.search != "" {
		pkgTitle += fmt.Sprintf(" (%d/%d)", len(m.pkgs()), len(m.result.Catalog.Packages))
	}

	pkgFocus, verFocus := m.focus == focusPackages, m.focus == focusVersions
	pkgPanel := m.panel(pkgTitle, m.listLines(pkgNames, &m.packages, d.leftW-2, d.pkgRows, pkgFocus), d.leftW, d.pkgH, pkgFocus)
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
		msg := "  Binary yok"
		if !m.filter.IsZero() {
			msg = "  Filtreye uyan binary yok (0 ile temizle)"
		}
		lines = append(lines, m.st.dim.Render(m.st.fit(msg, innerW)))
	}
	return m.panel(title, lines, w, h, focused)
}

func (m model) helpView(h int) string {
	rows := [][2]string{
		{"↑ ↓ / j k", "satır seç"},
		{"PgUp PgDn, g G", "sayfa / başa / sona git"},
		{"Tab, Shift+Tab, ← →", "panel değiştir"},
		{"Enter", "sonraki panel / binary detayı"},
		{"/", "paket adında anlık ara"},
		{"w / l", "sadece Windows / Linux"},
		{"r / d", "sadece Release / Debug"},
		{"0", "filtreleri ve aramayı temizle"},
		{"c", "conan install komutunu kopyala"},
		{"m", "matrix (paket × profil) aç / kapat"},
		{"F5 / R", "verileri yeniden yükle"},
		{"?", "bu yardımı aç / kapat"},
		{"q, Ctrl+C", "çıkış"},
	}
	var b strings.Builder
	b.WriteString(m.st.title.Render("Kısayollar") + "\n\n")
	for _, r := range rows {
		b.WriteString(m.st.key.Render(m.st.fit(r[0], 22)) + "  " + r[1] + "\n")
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
