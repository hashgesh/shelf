package tui

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/hashgesh/shelf/internal/config"
	"github.com/hashgesh/shelf/internal/i18n"
)

// ConfigFormOptions configures EditConfig.
type ConfigFormOptions struct {
	Config          *config.Config // starting values, left untouched
	Path            string         // file shown in the title
	ProfilesDefault string         // shown while profiles_dir is empty
	ASCII           bool
	NoColor         bool
	Input           io.Reader // defaults to os.Stdin
	Output          io.Writer // defaults to os.Stdout
}

// EditConfig shows a settings form and returns the edited copy. ok is false
// when the user left without saving.
func EditConfig(opts ConfigFormOptions) (cfg *config.Config, ok bool, err error) {
	out := opts.Output
	if out == nil {
		out = os.Stdout
	}
	r := lipgloss.NewRenderer(out)
	if opts.NoColor {
		r.SetColorProfile(termenv.Ascii)
	}
	m := newConfigForm(*opts.Config, opts.Path, opts.ProfilesDefault, newStyles(r, opts.ASCII))

	progOpts := []tea.ProgramOption{tea.WithAltScreen(), tea.WithOutput(out)}
	if opts.Input != nil {
		progOpts = append(progOpts, tea.WithInput(opts.Input))
	}
	final, err := tea.NewProgram(m, progOpts...).Run()
	if err != nil {
		return nil, false, err
	}
	f := final.(configForm)
	if !f.saved {
		return nil, false, nil
	}
	return &f.cfg, true, nil
}

type fieldKind int

const (
	fieldText fieldKind = iota
	fieldSecret
	fieldToggle
	fieldChoice
)

type formField struct {
	key     string
	kind    fieldKind
	hint    string
	choices []string
}

type configForm struct {
	cfg             config.Config
	path            string
	profilesDefault string
	st              styles
	fields          []formField

	cursor    int
	editing   bool
	input     textinput.Model
	dirty     bool
	saved     bool
	quitArmed bool // q pressed once with unsaved changes
	status    string
	statusErr bool
}

func newConfigForm(cfg config.Config, path, profilesDefault string, st styles) configForm {
	in := textinput.New()
	in.Prompt = ""
	in.CharLimit = 500
	return configForm{
		cfg: cfg, path: path, profilesDefault: profilesDefault, st: st, input: in,
		fields: []formField{
			{key: config.KeyURL, hint: i18n.T("e.g. https://artifactory.example/artifactory")},
			{key: config.KeyUser, hint: i18n.T("leave empty for anonymous access")},
			{key: config.KeyToken, kind: fieldSecret, hint: i18n.T("input is hidden; an empty value clears it")},
			{key: config.KeyPassword, kind: fieldSecret, hint: i18n.T("input is hidden; an empty value clears it")},
			{key: config.KeyRepos, hint: i18n.T("comma-separated")},
			{key: config.KeyProfilesDir, hint: i18n.T("leave empty to follow CONAN_HOME")},
			{key: config.KeyInsecureTLS, kind: fieldToggle, hint: i18n.T("for self-signed certificates")},
			{key: config.KeyTimeout, hint: i18n.T("for example: 30s, 2m or a number of seconds")},
			{key: config.KeyAuth, kind: fieldChoice, choices: []string{config.AuthAuto, config.AuthBearer, config.AuthBasic}},
			{key: config.KeyMetadataSource, kind: fieldChoice, choices: []string{config.MetadataAuto, config.MetadataProperties, config.MetadataConaninfo}},
		},
	}
}

func (f configForm) Init() tea.Cmd { return nil }

// text returns the editable form of a field's value.
func (f configForm) text(key string) string {
	switch key {
	case config.KeyURL:
		return f.cfg.URL
	case config.KeyUser:
		return f.cfg.User
	case config.KeyRepos:
		return strings.Join(f.cfg.Repos, ",")
	case config.KeyProfilesDir:
		return f.cfg.ProfilesDir
	case config.KeyTimeout:
		return f.cfg.Timeout.String()
	case config.KeyAuth:
		return f.cfg.Auth
	case config.KeyMetadataSource:
		return f.cfg.MetadataSource
	}
	return ""
}

// display returns what the row shows when not being edited.
func (f configForm) display(fd formField) string {
	switch fd.key {
	case config.KeyToken, config.KeyPassword:
		s := f.cfg.Token
		if fd.key == config.KeyPassword {
			s = f.cfg.Password
		}
		if s.IsSet() {
			return i18n.T("(set)")
		}
		return f.st.dim.Render(i18n.T("(not set)"))
	case config.KeyInsecureTLS:
		if f.cfg.InsecureTLS {
			return i18n.T("yes")
		}
		return i18n.T("no")
	case config.KeyProfilesDir:
		if f.cfg.ProfilesDir == "" {
			return f.st.dim.Render(f.profilesDefault)
		}
	}
	return f.text(fd.key)
}

// set stores a typed value, returning a validation error to show.
func (f *configForm) set(key, v string) error {
	v = strings.TrimSpace(v)
	switch key {
	case config.KeyURL:
		f.cfg.URL = strings.TrimRight(v, "/")
	case config.KeyUser:
		f.cfg.User = v
	case config.KeyToken:
		f.cfg.Token = config.Secret(v)
	case config.KeyPassword:
		f.cfg.Password = config.Secret(v)
	case config.KeyRepos:
		f.cfg.Repos = nil
		for _, r := range strings.Split(v, ",") {
			if r = strings.TrimSpace(r); r != "" {
				f.cfg.Repos = append(f.cfg.Repos, r)
			}
		}
	case config.KeyProfilesDir:
		if v == f.profilesDefault {
			v = ""
		}
		f.cfg.ProfilesDir = v
	case config.KeyTimeout:
		d, err := config.ParseTimeout(v)
		if err != nil {
			return err
		}
		f.cfg.Timeout = d
	}
	f.dirty = true
	return nil
}

func (f configForm) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return f, nil
	}
	if key.String() == "ctrl+c" {
		return f, tea.Quit
	}
	if f.editing {
		return f.handleEditKey(key)
	}
	f.status, f.statusErr = "", false
	fd := f.fields[f.cursor]
	switch key.String() {
	case "up", "k":
		f.cursor = max(f.cursor-1, 0)
	case "down", "j":
		f.cursor = min(f.cursor+1, len(f.fields)-1)
	case "enter", " ", "left", "right":
		switch fd.kind {
		case fieldToggle:
			f.cfg.InsecureTLS = !f.cfg.InsecureTLS
			f.dirty = true
		case fieldChoice:
			f.cycle(fd, key.String() == "left")
		default:
			if key.String() == " " || key.String() == "left" || key.String() == "right" {
				return f, nil
			}
			f.editing = true
			f.input.EchoMode = textinput.EchoNormal
			f.input.SetValue(f.text(fd.key))
			if fd.kind == fieldSecret {
				f.input.EchoMode = textinput.EchoPassword
				f.input.SetValue("")
			}
			f.input.CursorEnd()
			return f, f.input.Focus()
		}
	case "s":
		if err := f.cfg.Validate(); err != nil {
			f.status, f.statusErr = err.Error(), true
			return f, nil
		}
		f.saved = true
		return f, tea.Quit
	case "q", "esc":
		if f.dirty && !f.quitArmed {
			f.quitArmed = true
			f.status, f.statusErr = i18n.T("unsaved changes: press q again to discard them"), true
			return f, nil
		}
		return f, tea.Quit
	}
	return f, nil
}

func (f *configForm) cycle(fd formField, back bool) {
	cur := f.text(fd.key)
	i := 0
	for n, c := range fd.choices {
		if c == cur {
			i = n
		}
	}
	if back {
		i = (i + len(fd.choices) - 1) % len(fd.choices)
	} else {
		i = (i + 1) % len(fd.choices)
	}
	if fd.key == config.KeyAuth {
		f.cfg.Auth = fd.choices[i]
	} else {
		f.cfg.MetadataSource = fd.choices[i]
	}
	f.dirty = true
}

func (f configForm) handleEditKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch key.String() {
	case "esc":
		f.editing = false
		f.input.Blur()
		return f, nil
	case "enter":
		if err := f.set(f.fields[f.cursor].key, f.input.Value()); err != nil {
			f.status, f.statusErr = err.Error(), true
			return f, nil
		}
		f.status, f.statusErr = "", false
		f.editing = false
		f.input.Blur()
		return f, nil
	}
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(key)
	return f, cmd
}

func (f configForm) View() string {
	var b strings.Builder
	b.WriteString(" " + f.st.title.Render(i18n.T("shelf configuration")) + "  " + f.st.dim.Render(f.path) + "\n\n")
	for i, fd := range f.fields {
		label := f.st.fit(fd.key, 16)
		mark := "  "
		val := f.display(fd)
		if i == f.cursor {
			mark = f.st.cursorMark()
			label = f.st.selected.Render(label)
			if f.editing {
				val = f.input.View()
			}
		} else {
			label = f.st.key.Render(label)
		}
		fmt.Fprintf(&b, " %s%s  %s\n", mark, label, val)
	}
	b.WriteString("\n")
	if hint := f.fields[f.cursor].hint; hint != "" {
		b.WriteString(" " + f.st.dim.Render(hint) + "\n")
	}
	hints := [][2]string{{"↑↓", i18n.T("move")}, {"enter", i18n.T("edit")}, {"s", i18n.T("save")}, {"q", i18n.T("quit")}}
	if f.editing {
		hints = [][2]string{{"enter", i18n.T("apply")}, {"esc", i18n.T("cancel")}}
	}
	if f.st.ascii {
		hints[0][0] = strings.Replace(hints[0][0], "↑↓", "up/down", 1)
	}
	var parts []string
	for _, h := range hints {
		parts = append(parts, f.st.key.Render(h[0])+" "+h[1])
	}
	b.WriteString(" " + strings.Join(parts, "  ") + "\n")
	switch {
	case f.statusErr:
		b.WriteString(" " + f.st.errText.Render(f.status) + "\n")
	case f.dirty:
		b.WriteString(" " + f.st.warnText.Render(i18n.T("unsaved changes")) + "\n")
	}
	return b.String()
}
