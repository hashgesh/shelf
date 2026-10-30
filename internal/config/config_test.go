package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func envMap(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func ptr[T any](v T) *T { return &v }

func TestLoadDefaultsWithoutFile(t *testing.T) {
	// Point the platform config dir at an empty temp dir so a real user
	// config cannot leak into the test.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("AppData", dir)
	c, err := Load(Options{LookupEnv: envMap(map[string]string{
		EnvConfig:    "",
		EnvConanHome: "/opt/conan",
	})})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.FileFound || !strings.HasPrefix(c.Path, dir) {
		t.Errorf("FileFound=%v Path=%q", c.FileFound, c.Path)
	}
	if c.Timeout != DefaultTimeout || c.Auth != AuthAuto || c.MetadataSource != MetadataAuto {
		t.Errorf("unexpected defaults: %+v", c)
	}
	if len(c.Repos) != 1 || c.Repos[0] != DefaultRepo {
		t.Errorf("Repos = %v", c.Repos)
	}
	if want := filepath.Join("/opt/conan", "profiles"); c.ProfilesDir != want {
		t.Errorf("ProfilesDir = %q, want %q", c.ProfilesDir, want)
	}
	if c.Sources[KeyURL] != SourceDefault {
		t.Errorf("url source = %q", c.Sources[KeyURL])
	}
}

func TestLoadPrecedence(t *testing.T) {
	path := writeConfig(t, `
url: https://file.example/artifactory/
user: fileuser
token: filetoken
repos: [file-a, file-b]
profiles_dir: /file/profiles
insecure_tls: true
timeout: 45
auth: Basic
metadata_source: conaninfo
`)
	env := envMap(map[string]string{
		EnvURL:   "https://env.example",
		EnvToken: "envtoken",
		EnvRepos: " env-a , ,env-b,env-a",
	})

	t.Run("file only", func(t *testing.T) {
		c, err := Load(Options{Path: path, LookupEnv: envMap(nil)})
		if err != nil {
			t.Fatal(err)
		}
		if c.URL != "https://file.example/artifactory" {
			t.Errorf("URL = %q (trailing slash should be trimmed)", c.URL)
		}
		if c.User != "fileuser" || c.Token.Reveal() != "filetoken" {
			t.Errorf("user/token = %q/%q", c.User, c.Token.Reveal())
		}
		if strings.Join(c.Repos, ",") != "file-a,file-b" {
			t.Errorf("Repos = %v", c.Repos)
		}
		if !c.InsecureTLS || c.Timeout != 45*time.Second {
			t.Errorf("insecure/timeout = %v/%v", c.InsecureTLS, c.Timeout)
		}
		if c.Auth != AuthBasic || c.MetadataSource != MetadataConaninfo {
			t.Errorf("auth/metadata = %q/%q", c.Auth, c.MetadataSource)
		}
		if !c.FileFound || c.Sources[KeyURL] != SourceFile {
			t.Errorf("FileFound=%v source=%q", c.FileFound, c.Sources[KeyURL])
		}
	})

	t.Run("env over file", func(t *testing.T) {
		c, err := Load(Options{Path: path, LookupEnv: env})
		if err != nil {
			t.Fatal(err)
		}
		if c.URL != "https://env.example" || c.Sources[KeyURL] != SourceEnv {
			t.Errorf("URL = %q (%s)", c.URL, c.Sources[KeyURL])
		}
		if c.Token.Reveal() != "envtoken" {
			t.Errorf("Token = %q", c.Token.Reveal())
		}
		if strings.Join(c.Repos, ",") != "env-a,env-b" {
			t.Errorf("Repos = %v", c.Repos)
		}
		if c.User != "fileuser" || c.Sources[KeyUser] != SourceFile {
			t.Errorf("User = %q (%s)", c.User, c.Sources[KeyUser])
		}
	})

	t.Run("flags over env", func(t *testing.T) {
		c, err := Load(Options{Path: path, LookupEnv: env, Flags: Overrides{
			URL:         ptr("https://flag.example"),
			Token:       ptr("flagtoken"),
			Repos:       []string{"flag-a"},
			InsecureTLS: ptr(false),
			Timeout:     ptr(5 * time.Second),
		}})
		if err != nil {
			t.Fatal(err)
		}
		if c.URL != "https://flag.example" || c.Sources[KeyURL] != SourceFlag {
			t.Errorf("URL = %q (%s)", c.URL, c.Sources[KeyURL])
		}
		if c.Token.Reveal() != "flagtoken" || strings.Join(c.Repos, ",") != "flag-a" {
			t.Errorf("token/repos = %q/%v", c.Token.Reveal(), c.Repos)
		}
		if c.InsecureTLS || c.Timeout != 5*time.Second {
			t.Errorf("insecure/timeout = %v/%v", c.InsecureTLS, c.Timeout)
		}
	})
}

func TestLoadConfigPathFromEnv(t *testing.T) {
	path := writeConfig(t, "url: https://from-env-path.example\n")
	c, err := Load(Options{LookupEnv: envMap(map[string]string{EnvConfig: path})})
	if err != nil {
		t.Fatal(err)
	}
	if c.URL != "https://from-env-path.example" || c.Path != path {
		t.Errorf("URL=%q Path=%q", c.URL, c.Path)
	}
}

func TestLoadExplicitMissingFile(t *testing.T) {
	_, err := Load(Options{Path: filepath.Join(t.TempDir(), "missing.yaml"), LookupEnv: envMap(nil)})
	if !errors.Is(err, ErrInvalid) || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want ErrInvalid wrapping ErrNotExist", err)
	}
}

func TestLoadRejectsBadFile(t *testing.T) {
	tests := map[string]string{
		"unknown key": "url: https://x\ntokn: typo\n",
		"bad timeout": "timeout: soon\n",
		"bad yaml":    "url: [unclosed\n",
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := Load(Options{Path: writeConfig(t, body), LookupEnv: envMap(nil)})
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestLoadEmptyFile(t *testing.T) {
	c, err := Load(Options{Path: writeConfig(t, ""), LookupEnv: envMap(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if !c.FileFound {
		t.Error("FileFound = false")
	}
}

func TestValidate(t *testing.T) {
	base := func() *Config {
		return &Config{
			URL: "https://art.example/artifactory", Repos: []string{"conan-local"},
			Timeout: time.Second, Auth: AuthAuto, MetadataSource: MetadataAuto,
		}
	}
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"ok anonymous", func(*Config) {}, ""},
		{"ok bearer", func(c *Config) { c.Auth = AuthBearer; c.Token = "t" }, ""},
		{"ok basic token", func(c *Config) { c.Auth = AuthBasic; c.User = "u"; c.Token = "t" }, ""},
		{"missing url", func(c *Config) { c.URL = "" }, "url ayarlı değil"},
		{"bad scheme", func(c *Config) { c.URL = "ftp://x" }, "url geçersiz"},
		{"no host", func(c *Config) { c.URL = "https://" }, "url geçersiz"},
		{"no repos", func(c *Config) { c.Repos = nil }, "repo"},
		{"bearer without token", func(c *Config) { c.Auth = AuthBearer }, "token gerekli"},
		{"basic without user", func(c *Config) { c.Auth = AuthBasic; c.Token = "t" }, "user ve password"},
		{"bad auth", func(c *Config) { c.Auth = "apikey" }, "auth geçersiz"},
		{"bad metadata", func(c *Config) { c.MetadataSource = "x" }, "metadata_source"},
		{"password without user", func(c *Config) { c.Password = "p" }, "password için user"},
		{"zero timeout", func(c *Config) { c.Timeout = 0 }, "timeout"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := base()
			tt.mutate(c)
			err := c.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, ErrInvalid) || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("err = %v, want ErrInvalid containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestParseTimeout(t *testing.T) {
	good := []struct {
		in   string
		want time.Duration
	}{
		{"30", 30 * time.Second},
		{"2m", 2 * time.Minute},
		{" 1500ms ", 1500 * time.Millisecond},
	}
	for _, tt := range good {
		got, err := ParseTimeout(tt.in)
		if err != nil || got != tt.want {
			t.Errorf("ParseTimeout(%q) = %v, %v; want %v", tt.in, got, err, tt.want)
		}
	}
	for _, in := range []string{"", "0", "-5", "-1s", "abc"} {
		if _, err := ParseTimeout(in); err == nil {
			t.Errorf("ParseTimeout(%q) succeeded, want error", in)
		}
	}
}

func TestSecretNeverPrinted(t *testing.T) {
	const raw = "super-secret-token"
	c := Config{Token: Secret(raw), Password: Secret(raw)}
	outputs := []string{
		fmt.Sprintf("%v", c), fmt.Sprintf("%+v", c), fmt.Sprintf("%#v", c),
		fmt.Sprintf("%s", c.Token), //nolint:staticcheck // exercising the %s verb on purpose
		fmt.Sprintf("%q", c.Token), c.Token.String(),
	}
	js, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	outputs = append(outputs, string(js))
	for _, out := range outputs {
		if strings.Contains(out, raw) {
			t.Errorf("secret leaked in %q", out)
		}
	}
	if c.Token.Reveal() != raw {
		t.Error("Reveal did not return raw value")
	}
	if Secret("").String() != "" {
		t.Error("empty secret should print empty")
	}
}

func TestSaveRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "dir", "config.yaml")
	in := &Config{
		URL: "https://art.example", User: "me", Token: "tok",
		Repos: []string{"a", "b"}, ProfilesDir: "/p", InsecureTLS: true,
		Timeout: 90 * time.Second, Auth: AuthBearer, MetadataSource: MetadataAuto,
	}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if perm := st.Mode().Perm(); perm != 0o600 {
			t.Errorf("perm = %o, want 600", perm)
		}
	}
	out, err := Load(Options{Path: path, LookupEnv: envMap(nil)})
	if err != nil {
		t.Fatal(err)
	}
	if out.URL != in.URL || out.User != in.User || out.Token != in.Token ||
		strings.Join(out.Repos, ",") != "a,b" || out.ProfilesDir != "/p" ||
		!out.InsecureTLS || out.Timeout != in.Timeout || out.Auth != AuthBearer {
		t.Errorf("round trip mismatch:\n in=%+v\nout=%+v", in, out)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "metadata_source") {
		t.Errorf("default metadata_source should be omitted:\n%s", data)
	}
}
