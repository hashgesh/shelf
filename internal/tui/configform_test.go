package tui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hashgesh/shelf/internal/config"
)

func formKeys(t *testing.T, f configForm, keys ...string) configForm {
	t.Helper()
	for _, k := range keys {
		var msg tea.KeyMsg
		switch k {
		case "enter":
			msg = tea.KeyMsg{Type: tea.KeyEnter}
		case "down":
			msg = tea.KeyMsg{Type: tea.KeyDown}
		case "esc":
			msg = tea.KeyMsg{Type: tea.KeyEsc}
		case "ctrl+u":
			msg = tea.KeyMsg{Type: tea.KeyCtrlU}
		default:
			msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)}
		}
		next, _ := f.Update(msg)
		f = next.(configForm)
	}
	return f
}

func TestConfigFormEditsRepoAndSaves(t *testing.T) {
	cfg := config.Config{URL: "https://art.example", Repos: []string{"conan-local"}, Timeout: 30 * time.Second,
		Auth: config.AuthAuto, MetadataSource: config.MetadataAuto}
	f := newConfigForm(cfg, "/x/config.yaml", "/home/u/.conan2/profiles", testStyles(false))

	// Move to repos, type a new value, apply.
	f = formKeys(t, f, "down", "down", "down", "down", "enter", "ctrl+u", "conan-remote, other", "enter")
	if got := strings.Join(f.cfg.Repos, "|"); got != "conan-remote|other" {
		t.Fatalf("repos = %q", got)
	}
	if !f.dirty || f.editing {
		t.Fatalf("dirty=%v editing=%v", f.dirty, f.editing)
	}

	// Invalid timeout is reported and keeps editing.
	f = formKeys(t, f, "down", "down", "down", "enter", "ctrl+u", "soon", "enter")
	if !f.editing || !f.statusErr {
		t.Fatalf("invalid timeout accepted: editing=%v status=%q", f.editing, f.status)
	}
	f = formKeys(t, f, "esc")

	// q once warns, s saves.
	f = formKeys(t, f, "q")
	if !f.quitArmed || f.saved {
		t.Fatalf("q did not warn about unsaved changes")
	}
	f = formKeys(t, f, "s")
	if !f.saved || f.cfg.Timeout != 30*time.Second || f.cfg.URL != cfg.URL {
		t.Fatalf("save: saved=%v cfg=%+v", f.saved, f.cfg)
	}
	if !strings.Contains(f.View(), "conan-remote,other") {
		t.Errorf("view does not show the new repos:\n%s", f.View())
	}
}
