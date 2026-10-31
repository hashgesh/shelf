package tui

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/halilibrahimonus/shelf/internal/conan"
)

// Loader fetches the catalog. It is called on start and on every refresh.
type Loader func(ctx context.Context) (*conan.LoadResult, error)

// CopyFunc puts text on the clipboard and returns the method used.
type CopyFunc func(text string) (method string, err error)

type focus int

const (
	focusPackages focus = iota
	focusVersions
	focusBinaries
	focusCount
)

// narrowWidth is the width below which panels stack vertically.
const narrowWidth = 100

// statusTTL is how long a status message stays in the bottom bar (a
// variable so tests can shorten it).
var statusTTL = 4 * time.Second

type (
	loadedMsg struct {
		res *conan.LoadResult
		err error
	}
	copiedMsg struct {
		text   string
		method string
		err    error
	}
	statusMsg struct {
		text  string
		isErr bool
	}
	clearStatusMsg struct{ seq int }
)

type model struct {
	load      Loader
	copy      CopyFunc
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

	// Filters: quick toggles and the package name search.
	filter    conan.Filter
	search    string
	searching bool
	input     textinput.Model

	detail *detail

	matrixMode     bool
	mxRows, mxCols cursor

	status    string
	statusErr bool
	statusSeq int
}

func newModel(load Loader, repoLabel string, st styles) model {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	if st.ascii {
		sp.Spinner = spinner.Line
	}
	sp.Style = st.title

	in := textinput.New()
	in.Prompt = "/"
	in.Placeholder = "paket adı"
	in.CharLimit = 100

	return model{
		load: load, repoLabel: repoLabel, st: st, spin: sp, input: in,
		loading: true, width: 80, height: 24,
	}
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

// pkgs returns the packages matching the search text.
func (m model) pkgs() []*conan.Package {
	if m.result == nil {
		return nil
	}
	all := m.result.Catalog.Packages
	if m.search == "" {
		return all
	}
	needle := strings.ToLower(m.search)
	out := make([]*conan.Package, 0, len(all))
	for _, p := range all {
		if strings.Contains(strings.ToLower(p.Name), needle) {
			out = append(out, p)
		}
	}
	return out
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

// versionBinaries returns the latest revision's binaries of v after filters.
func (m model) versionBinaries(v *conan.Version) []*conan.Binary {
	if v == nil || v.Latest() == nil {
		return nil
	}
	return m.filter.Apply(v.Latest().Binaries)
}

func (m model) currentBinaries() []*conan.Binary { return m.versionBinaries(m.currentVersion()) }

func (m model) currentBinary() *conan.Binary {
	bs := m.currentBinaries()
	if m.binaries.pos < len(bs) {
		return bs[m.binaries.pos]
	}
	return nil
}

// syncCursors updates list lengths after data, filter or selection changes.
func (m *model) syncCursors() {
	m.packages.setLen(len(m.pkgs()))
	n := 0
	if p := m.currentPackage(); p != nil {
		n = len(p.Versions)
	}
	m.versions.setLen(n)
	m.binaries.setLen(len(m.currentBinaries()))
}

// selectPackage moves the package cursor to name if it is visible.
func (m *model) selectPackage(name string) bool {
	for i, p := range m.pkgs() {
		if p.Name == name {
			m.packages.pos = i
			return true
		}
	}
	return false
}

// ---- update ----

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height

	case spinner.TickMsg:
		if m.loading {
			m.spin, cmd = m.spin.Update(msg)
		}

	case loadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err == nil {
			m.restoreSelection(msg.res)
		}

	case copiedMsg:
		if msg.err != nil {
			// Show the command so it can be copied by hand.
			if m.detail == nil {
				if b := m.currentBinary(); b != nil {
					m.detail = newDetail(b)
				}
			}
			cmd = m.setStatus("Panoya kopyalanamadı; komutu detay penceresinden seçip kopyalayın", true)
		} else {
			cmd = m.setStatus("conan install komutu kopyalandı ("+msg.method+")", false)
		}

	case statusMsg:
		cmd = m.setStatus(msg.text, msg.isErr)

	case clearStatusMsg:
		if msg.seq == m.statusSeq {
			m.status = ""
		}

	case tea.KeyMsg:
		m, cmd = m.handleKey(msg)
	}
	m.scroll()
	m.scrollMatrix()
	m.clampDetail()
	return m, cmd
}

func (m *model) setStatus(text string, isErr bool) tea.Cmd {
	m.statusSeq++
	m.status, m.statusErr = text, isErr
	seq := m.statusSeq
	return tea.Tick(statusTTL, func(time.Time) tea.Msg { return clearStatusMsg{seq: seq} })
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
	if m.selectPackage(pkgName) {
		for j, v := range m.currentPackage().Versions {
			if v.Ref(pkgName) == verRef {
				m.versions.pos = j
			}
		}
	}
	m.syncCursors()
}

func (m model) handleKey(msg tea.KeyMsg) (model, tea.Cmd) {
	key := msg.String()
	if key == "ctrl+c" {
		return m, tea.Quit
	}
	switch {
	case m.searching:
		return m.handleSearchKey(msg)
	case m.showHelp:
		switch key {
		case "?", "esc", "q":
			m.showHelp = false
		}
		return m, nil
	case m.detail != nil:
		return m.handleDetailKey(key)
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
	if next, cmd, ok := m.handleFilterKey(key); ok {
		return next, cmd
	}
	if m.matrixMode {
		return m.handleMatrixKey(key)
	}

	switch key {
	case "tab":
		m.focus = (m.focus + 1) % focusCount
	case "shift+tab":
		m.focus = (m.focus + focusCount - 1) % focusCount
	case "left":
		if m.focus > focusPackages {
			m.focus--
		}
	case "right":
		if m.focus < focusBinaries {
			m.focus++
		}
	case "enter":
		if m.focus < focusBinaries {
			m.focus++
		} else if b := m.currentBinary(); b != nil {
			m.detail = newDetail(b)
		}
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
	case "c":
		return m, m.copyInstall()
	}
	return m, nil
}

// handleFilterKey handles the search and quick filter keys shared by the
// browser and the matrix.
func (m model) handleFilterKey(key string) (model, tea.Cmd, bool) {
	switch key {
	case "/":
		m.searching = true
		m.input.SetValue(m.search)
		m.input.CursorEnd()
		return m, m.input.Focus(), true
	case "w":
		m.toggleOS("Windows")
	case "l":
		m.toggleOS("Linux")
	case "r":
		m.toggleBuild("Release")
	case "d":
		m.toggleBuild("Debug")
	case "0":
		m.filter = conan.Filter{}
		m.setSearch("")
		m.binaries.reset()
		m.syncCursors()
		return m, m.setStatus("Filtreler temizlendi", false), true
	case "m":
		m.matrixMode = !m.matrixMode
	default:
		return m, nil, false
	}
	return m, nil, true
}

func (m *model) toggleOS(os string) {
	if m.filter.OS == os {
		m.filter.OS = ""
	} else {
		m.filter.OS = os
	}
	m.binaries.reset()
	m.syncCursors()
}

func (m *model) toggleBuild(bt string) {
	if m.filter.BuildType == bt {
		m.filter.BuildType = ""
	} else {
		m.filter.BuildType = bt
	}
	m.binaries.reset()
	m.syncCursors()
}

// setSearch changes the package search, keeping the selected package when
// it still matches.
func (m *model) setSearch(s string) {
	if s == m.search {
		return
	}
	prev := ""
	if p := m.currentPackage(); p != nil {
		prev = p.Name
	}
	m.search = s
	if !m.selectPackage(prev) {
		m.packages.reset()
		m.versions.reset()
		m.binaries.reset()
	}
	m.syncCursors()
}

func (m model) handleSearchKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.searching = false
		m.input.Blur()
		m.focus = focusPackages
		return m, nil
	case "esc":
		m.searching = false
		m.input.Blur()
		m.setSearch("")
		return m, nil
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.setSearch(m.input.Value())
	return m, cmd
}

func (m model) handleDetailKey(key string) (model, tea.Cmd) {
	switch key {
	case "esc", "enter", "q":
		m.detail = nil
	case "up", "k":
		m.detail.scroll(-1)
	case "down", "j":
		m.detail.scroll(1)
	case "pgup":
		m.detail.scroll(-10)
	case "pgdown":
		m.detail.scroll(10)
	case "home", "g":
		m.detail.off = 0
	case "end", "G":
		m.detail.scroll(1 << 20)
	case "c":
		return m, m.copyInstall()
	}
	return m, nil
}

// copyInstall copies the selected binary's conan install command.
func (m model) copyInstall() tea.Cmd {
	b := m.currentBinary()
	if m.detail != nil {
		b = m.detail.binary
	}
	if b == nil {
		return statusCmd("Kopyalanacak binary seçili değil", true)
	}
	if m.copy == nil {
		return statusCmd("Pano desteği yok", true)
	}
	text, copyFn := conan.InstallCommand(b), m.copy
	return func() tea.Msg {
		method, err := copyFn(text)
		return copiedMsg{text: text, method: method, err: err}
	}
}

// statusCmd shows a status message from code that cannot mutate the model.
func statusCmd(text string, isErr bool) tea.Cmd {
	return func() tea.Msg { return statusMsg{text: text, isErr: isErr} }
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
