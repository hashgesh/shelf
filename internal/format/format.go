// Package format holds small display helpers shared by the CLI and the TUI.
package format

import (
	"fmt"
	"time"
)

// Size formats bytes as B, KB, MB or GB (base 1024).
func Size(n int64) string {
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

// Time formats t in local time, or "-" when zero.
func Time(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02 15:04")
}

// Date formats t as a local date, or "-" when zero.
func Date(t time.Time) string {
	if t.IsZero() {
		return "-"
	}
	return t.Local().Format("2006-01-02")
}

// ShortID returns the first 8 characters of a hash.
func ShortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// Dash returns "-" for an empty string.
func Dash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
