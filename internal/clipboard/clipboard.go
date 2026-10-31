// Package clipboard copies text to the system clipboard without cgo:
// the Win32 API on Windows, xclip/xsel/wl-copy elsewhere, and the OSC 52
// terminal escape sequence as a last resort.
package clipboard

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Method names reported by Copy.
const (
	MethodNative = "Windows panosu"
	MethodOSC52  = "OSC52"
)

// ErrUnavailable means no clipboard mechanism worked.
var ErrUnavailable = errors.New("pano kullanılamıyor")

// Copier copies text and reports which mechanism succeeded.
type Copier struct {
	// Terminal receives the OSC 52 sequence; nil disables OSC 52.
	Terminal io.Writer
	// LookPath and Run are replaceable for tests.
	LookPath func(string) (string, error)
	Run      func(name string, args []string, stdin string) error
	// native is the platform clipboard (Windows only).
	native func(string) error
}

// New returns a Copier for the current platform that falls back to OSC 52
// on terminal.
func New(terminal io.Writer) *Copier {
	return &Copier{Terminal: terminal, LookPath: exec.LookPath, Run: runWithStdin, native: nativeCopy}
}

// tools are tried in order on non-Windows systems.
var tools = []struct {
	name string
	args []string
	env  string // required environment variable
}{
	{"wl-copy", nil, "WAYLAND_DISPLAY"},
	{"xclip", []string{"-selection", "clipboard"}, "DISPLAY"},
	{"xsel", []string{"--clipboard", "--input"}, "DISPLAY"},
}

// Copy places text on the clipboard. It returns the method used.
func (c *Copier) Copy(text string) (string, error) {
	var errs []string
	if c.native != nil {
		err := c.native(text)
		if err == nil {
			return MethodNative, nil
		}
		if !errors.Is(err, errors.ErrUnsupported) {
			errs = append(errs, err.Error())
		}
	}
	for _, t := range tools {
		if os.Getenv(t.env) == "" {
			continue
		}
		path, err := c.LookPath(t.name)
		if err != nil {
			continue
		}
		if err := c.Run(path, t.args, text); err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", t.name, err))
			continue
		}
		return t.name, nil
	}
	if c.Terminal != nil {
		if _, err := io.WriteString(c.Terminal, OSC52(text)); err == nil {
			return MethodOSC52, nil
		}
	}
	if len(errs) > 0 {
		return "", fmt.Errorf("%w: %s", ErrUnavailable, strings.Join(errs, "; "))
	}
	return "", ErrUnavailable
}

// OSC52 returns the escape sequence that asks the terminal to set the
// clipboard. Terminals without OSC 52 support ignore it silently.
func OSC52(text string) string {
	return "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a"
}

func runWithStdin(name string, args []string, stdin string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	if out, err := cmd.CombinedOutput(); err != nil {
		if msg := strings.TrimSpace(string(out)); msg != "" {
			return fmt.Errorf("%w: %s", err, msg)
		}
		return err
	}
	return nil
}
