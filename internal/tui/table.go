package tui

import (
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/hashgesh/shelf/internal/conan"
	"github.com/hashgesh/shelf/internal/format"
)

var (
	dash      = format.Dash
	shortID   = format.ShortID
	humanSize = format.Size
)

// column describes one binary table column. Columns with a higher drop
// value are removed first when the terminal is too narrow; a column with
// minWidth > 0 is shrunk to that width before anything is dropped.
type column struct {
	title    string
	maxWidth int
	minWidth int
	drop     int // 0 = never dropped
	value    func(*conan.Binary) string
}

var binaryColumns = []column{
	{title: "OS", maxWidth: 8, value: func(b *conan.Binary) string { return dash(b.OS) }},
	{title: "Arch", maxWidth: 8, value: func(b *conan.Binary) string { return dash(b.Arch) }},
	{title: "Compiler", maxWidth: 12, value: func(b *conan.Binary) string {
		return dash(strings.TrimSpace(b.Compiler + " " + b.CompilerVersion))
	}},
	{title: "Build", maxWidth: 14, value: func(b *conan.Binary) string { return dash(b.BuildType) }},
	{title: "Shared", maxWidth: 6, drop: 3, value: func(b *conan.Binary) string { return dash(b.Shared()) }},
	{title: "Boyut", maxWidth: 9, drop: 5, value: func(b *conan.Binary) string { return humanSize(b.Size) }},
	{title: "Tarih", maxWidth: 10, drop: 6, value: func(b *conan.Binary) string { return format.Date(b.Created) }},
	{title: "Yükleyen", maxWidth: 14, drop: 7, value: func(b *conan.Binary) string { return dash(b.CreatedBy) }},
	{title: "ID", maxWidth: 8, drop: 4, value: func(b *conan.Binary) string { return shortID(b.PackageID) }},
	{title: "Profil", maxWidth: 32, minWidth: 14, drop: 2, value: func(b *conan.Binary) string { return b.Label() }},
}

const colGap = 2

// layoutColumns picks columns and widths that fit into width.
func layoutColumns(bs []*conan.Binary, width int) ([]column, []int) {
	cols := append([]column(nil), binaryColumns...)
	for {
		widths := make([]int, len(cols))
		total := colGap * (len(cols) - 1)
		for i, c := range cols {
			w := runewidth.StringWidth(c.title)
			for _, b := range bs {
				w = max(w, runewidth.StringWidth(c.value(b)))
			}
			widths[i] = min(w, c.maxWidth)
			total += widths[i]
		}
		// Shrink flexible columns first.
		for i, c := range cols {
			if total > width && c.minWidth > 0 && widths[i] > c.minWidth {
				cut := min(widths[i]-c.minWidth, total-width)
				widths[i] -= cut
				total -= cut
			}
		}
		if total <= width {
			// Leftover space goes to the last column.
			widths[len(widths)-1] += width - total
			return cols, widths
		}
		drop, best := -1, 0
		for i, c := range cols {
			if c.drop > best {
				drop, best = i, c.drop
			}
		}
		if drop < 0 {
			widths[len(widths)-1] = max(widths[len(widths)-1]-(total-width), 1)
			return cols, widths
		}
		cols = append(cols[:drop], cols[drop+1:]...)
	}
}
