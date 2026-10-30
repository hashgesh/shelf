package clipboard

import (
	"bytes"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func fakeCopier(available map[string]bool, failing map[string]bool, term *bytes.Buffer) (*Copier, *[]string) {
	var ran []string
	c := &Copier{
		LookPath: func(name string) (string, error) {
			if available[name] {
				return "/usr/bin/" + name, nil
			}
			return "", errors.New("not found")
		},
		Run: func(name string, _ []string, stdin string) error {
			ran = append(ran, name+":"+stdin)
			if failing[strings.TrimPrefix(name, "/usr/bin/")] {
				return errors.New("Error: Can't open display")
			}
			return nil
		},
	}
	if term != nil {
		c.Terminal = term
	}
	return c, &ran
}

func TestCopyPrefersTools(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	t.Setenv("WAYLAND_DISPLAY", "")
	c, ran := fakeCopier(map[string]bool{"xclip": true, "xsel": true}, nil, nil)
	method, err := c.Copy("conan install x")
	if err != nil || method != "xclip" {
		t.Fatalf("method=%q err=%v", method, err)
	}
	if len(*ran) != 1 || (*ran)[0] != "/usr/bin/xclip:conan install x" {
		t.Errorf("ran = %v", *ran)
	}
}

func TestCopyFallsBackToXselThenOSC52(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	t.Setenv("WAYLAND_DISPLAY", "")
	c, _ := fakeCopier(map[string]bool{"xclip": true, "xsel": true}, map[string]bool{"xclip": true}, nil)
	if method, err := c.Copy("x"); err != nil || method != "xsel" {
		t.Fatalf("method=%q err=%v", method, err)
	}

	var term bytes.Buffer
	c, _ = fakeCopier(map[string]bool{"xclip": true}, map[string]bool{"xclip": true}, &term)
	method, err := c.Copy("hello")
	if err != nil || method != MethodOSC52 {
		t.Fatalf("method=%q err=%v", method, err)
	}
	want := "\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte("hello")) + "\a"
	if term.String() != want {
		t.Errorf("OSC52 = %q", term.String())
	}
}

func TestCopyNeedsDisplayForX11Tools(t *testing.T) {
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	var term bytes.Buffer
	c, ran := fakeCopier(map[string]bool{"xclip": true}, nil, &term)
	if method, _ := c.Copy("x"); method != MethodOSC52 || len(*ran) != 0 {
		t.Errorf("method=%q ran=%v", method, *ran)
	}
}

func TestCopyUnavailable(t *testing.T) {
	t.Setenv("DISPLAY", ":0")
	c, _ := fakeCopier(map[string]bool{"xclip": true}, map[string]bool{"xclip": true}, nil)
	_, err := c.Copy("x")
	if !errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "display") {
		t.Fatalf("err = %v", err)
	}
}

func TestNativeFirst(t *testing.T) {
	c, ran := fakeCopier(map[string]bool{"xclip": true}, nil, nil)
	var got string
	c.native = func(s string) error { got = s; return nil }
	if method, err := c.Copy("abc"); err != nil || method != MethodNative || got != "abc" || len(*ran) != 0 {
		t.Fatalf("method=%q err=%v got=%q ran=%v", method, err, got, *ran)
	}
}
