package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/halilibrahimonus/shelf/internal/conan"
)

// styler applies ANSI styles when color is enabled and picks ASCII
// replacements for symbols when requested.
type styler struct {
	color bool
	ascii bool
}

// newStyler decides on color for w: off with --no-color, NO_COLOR, or when w
// is not a terminal. On Windows it enables VT processing; if that fails
// (legacy console) color is turned off.
func (a *app) newStyler(w io.Writer) styler {
	s := styler{ascii: a.flags.ascii}
	if a.flags.noColor {
		return s
	}
	if v, ok := a.lookupEnv("NO_COLOR"); ok && v != "" {
		return s
	}
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return s
	}
	restore, err := termenv.EnableVirtualTerminalProcessing(termenv.NewOutput(f))
	if err != nil {
		return s
	}
	_ = restore // leave VT mode on for the rest of the process
	s.color = true
	return s
}

func (s styler) wrap(code, text string) string {
	if !s.color || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

func (s styler) bold(t string) string { return s.wrap("1", t) }
func (s styler) dim(t string) string  { return s.wrap("2", t) }

// table renders aligned columns. Widths are computed on unstyled text; the
// optional style hook colors a cell after padding.
type table struct {
	headers []string
	rows    [][]string
	style   func(row, col int, padded string) string
}

func (t *table) add(cells ...string) { t.rows = append(t.rows, cells) }

func (t *table) render(w io.Writer, st styler) {
	widths := make([]int, len(t.headers))
	for i, h := range t.headers {
		widths[i] = runewidth.StringWidth(h)
	}
	for _, r := range t.rows {
		for i, c := range r {
			if i < len(widths) {
				widths[i] = max(widths[i], runewidth.StringWidth(c))
			}
		}
	}
	line := func(cells []string, styleCell func(col int, s string) string) {
		var b strings.Builder
		for i, c := range cells {
			if i >= len(widths) {
				break
			}
			padded := c
			if i < len(cells)-1 {
				padded = runewidth.FillRight(c, widths[i])
			}
			b.WriteString(styleCell(i, padded))
			if i < len(cells)-1 {
				b.WriteString("  ")
			}
		}
		fmt.Fprintln(w, strings.TrimRight(b.String(), " "))
	}
	line(t.headers, func(_ int, s string) string { return st.bold(s) })
	for ri, r := range t.rows {
		line(r, func(col int, s string) string {
			if t.style != nil {
				return t.style(ri, col, s)
			}
			return s
		})
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// humanSize formats bytes as B, KB, MB or GB (base 1024).
func humanSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit && exp < 2; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMG"[exp])
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

const maxWarnings = 5

// printWarnings writes at most maxWarnings warnings to stderr.
func printWarnings(w io.Writer, st styler, warns []conan.Warning) {
	for i, wr := range warns {
		if i == maxWarnings {
			fmt.Fprintf(w, "%s ... ve %d uyarı daha\n", st.dim("uyarı:"), len(warns)-maxWarnings)
			return
		}
		fmt.Fprintf(w, "%s %s\n", st.dim("uyarı:"), wr)
	}
}
