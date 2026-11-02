package tui

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// styles holds every Lip Gloss style the views use, created from one
// renderer so --no-color applies everywhere.
type styles struct {
	ascii bool

	topBar    lipgloss.Style
	busy      lipgloss.Style // refresh indicator in the top bar
	loading   lipgloss.Style // first-load message
	bottomBar lipgloss.Style
	title     lipgloss.Style
	header    lipgloss.Style
	dim       lipgloss.Style
	selected  lipgloss.Style // cursor row in the focused panel
	inactive  lipgloss.Style // cursor row in other panels
	errText   lipgloss.Style
	warnText  lipgloss.Style
	okText    lipgloss.Style
	badText   lipgloss.Style
	key       lipgloss.Style

	panel        lipgloss.Style
	focusedPanel lipgloss.Style
}

func newStyles(r *lipgloss.Renderer, ascii bool) styles {
	// Only the 16 basic ANSI colors are used: they follow the terminal's own
	// theme, so no background detection is needed. AdaptiveColor would make
	// Lip Gloss query the terminal (OSC 11) at startup, which stalls on
	// terminals and consoles that never answer.
	accent := lipgloss.Color("12")
	muted := lipgloss.Color("8")

	border, focusBorder := lipgloss.RoundedBorder(), lipgloss.ThickBorder()
	if ascii {
		// The ASCII border has no thick variant; focus is shown by the
		// title marker instead.
		border, focusBorder = lipgloss.ASCIIBorder(), lipgloss.ASCIIBorder()
	}

	s := styles{ascii: ascii}
	s.topBar = r.NewStyle().Bold(true).Reverse(true)
	// Black on bright blue stands out from the reversed bar in light and dark
	// themes; without color it falls back to bold.
	s.busy = r.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(accent)
	s.loading = r.NewStyle().Bold(true).Foreground(accent)
	s.bottomBar = r.NewStyle()
	s.title = r.NewStyle().Bold(true).Foreground(accent)
	s.header = r.NewStyle().Bold(true).Underline(true)
	s.dim = r.NewStyle().Foreground(muted)
	s.selected = r.NewStyle().Reverse(true).Bold(true)
	s.inactive = r.NewStyle().Bold(true)
	s.errText = r.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	s.warnText = r.NewStyle().Foreground(lipgloss.Color("11"))
	s.okText = r.NewStyle().Foreground(lipgloss.Color("10"))
	s.badText = r.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	s.key = r.NewStyle().Bold(true).Foreground(accent)
	s.panel = r.NewStyle().Border(border).BorderForeground(muted)
	s.focusedPanel = r.NewStyle().Border(focusBorder).BorderForeground(accent)
	return s
}

func (s styles) ellipsis() string {
	if s.ascii {
		return "~"
	}
	return "…"
}

// fit truncates or pads plain text to exactly w terminal cells.
func (s styles) fit(text string, w int) string {
	if w <= 0 {
		return ""
	}
	if runewidth.StringWidth(text) > w {
		text = runewidth.Truncate(text, w, s.ellipsis())
	}
	return runewidth.FillRight(text, w)
}

func (s styles) dot() string {
	if s.ascii {
		return " | "
	}
	return " · "
}

func (s styles) times() string {
	if s.ascii {
		return " x "
	}
	return " × "
}

func (s styles) cursorMark() string {
	if s.ascii {
		return "> "
	}
	return "› "
}
