package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/hashgesh/shelf/internal/i18n"
)

// errAborted is returned when input ends before the interview finishes.
var errAborted = i18n.Error("input ended, configuration not saved")

// prompter asks line-oriented questions. When input is a terminal, secrets
// are read without echo.
type prompter struct {
	r   *bufio.Reader
	out io.Writer
	fd  int // terminal file descriptor, or -1
}

func newPrompter(in io.Reader, out io.Writer) *prompter {
	p := &prompter{r: bufio.NewReader(in), out: out, fd: -1}
	if f, ok := in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		p.fd = int(f.Fd())
	}
	return p
}

func (p *prompter) readLine() (string, error) {
	line, err := p.r.ReadString('\n')
	if err != nil && (!errors.Is(err, io.EOF) || line == "") {
		if errors.Is(err, io.EOF) {
			return "", errAborted
		}
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// ask prints label with an optional default and returns the answer or def.
func (p *prompter) ask(label, def string) (string, error) {
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", label)
	}
	v, err := p.readLine()
	if err != nil {
		return "", err
	}
	if v == "" {
		return def, nil
	}
	return v, nil
}

// askSecret reads a secret without echo. An empty answer returns "" which
// callers treat as "keep the existing value" when hasCurrent is true.
func (p *prompter) askSecret(label string, hasCurrent bool) (string, error) {
	hint := i18n.T("input is hidden")
	if hasCurrent {
		hint += i18n.T(", leave empty to keep the current value")
	}
	fmt.Fprintf(p.out, "%s (%s): ", label, hint)
	if p.fd >= 0 {
		b, err := term.ReadPassword(p.fd)
		fmt.Fprintln(p.out)
		if err != nil {
			return "", fmt.Errorf(i18n.T("cannot read hidden input: %w"), err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return p.readLine()
}

// confirm asks a yes/no question. Accepts English and Turkish answers
// (y/yes/e/evet, n/no/h/hayır) whatever the interface language.
func (p *prompter) confirm(label string, def bool) (bool, error) {
	opts := i18n.T("y/N")
	if def {
		opts = i18n.T("Y/n")
	}
	for {
		fmt.Fprintf(p.out, "%s [%s]: ", label, opts)
		v, err := p.readLine()
		if err != nil {
			return false, err
		}
		switch strings.ToLower(v) {
		case "":
			return def, nil
		case "e", "evet", "y", "yes":
			return true, nil
		case "h", "hayır", "hayir", "n", "no":
			return false, nil
		}
		fmt.Fprintln(p.out, i18n.T("  Type \"y\" or \"n\"."))
	}
}
