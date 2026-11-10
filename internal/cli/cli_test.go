package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashgesh/shelf/internal/config"
)

type result struct {
	code           int
	stdout, stderr string
}

func runCLI(t *testing.T, env map[string]string, stdin string, args ...string) result {
	t.Helper()
	var out, errOut bytes.Buffer
	a := &app{
		stdin:  strings.NewReader(stdin),
		stdout: &out,
		stderr: &errOut,
		lookupEnv: func(k string) (string, bool) {
			v, ok := env[k]
			return v, ok
		},
	}
	code := a.run(BuildInfo{Version: "1.2.3", Commit: "abc", Date: "today"}, args)
	return result{code, out.String(), errOut.String()}
}

func TestVersion(t *testing.T) {
	r := runCLI(t, nil, "", "--version")
	if r.code != ExitOK || !strings.Contains(r.stdout, "shelf 1.2.3 (commit abc") {
		t.Fatalf("got %+v", r)
	}
}

func TestHelpIsTurkish(t *testing.T) {
	r := runCLI(t, nil, "", "config", "--help")
	if r.code != ExitOK || !strings.Contains(r.stdout, "Kullanım:") || !strings.Contains(r.stdout, "Komutlar:") {
		t.Fatalf("got %+v", r)
	}
}

func TestConfigShowMasksSecretsAndReportsSources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("url: https://file.example\ntoken: file-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	env := map[string]string{config.EnvUser: "envuser", config.EnvToken: "env-secret"}

	r := runCLI(t, env, "", "--config", path, "--repo", "a", "--repo", "b", "config", "show")
	if r.code != ExitOK {
		t.Fatalf("code=%d stderr=%s", r.code, r.stderr)
	}
	for _, leak := range []string{"file-secret", "env-secret"} {
		if strings.Contains(r.stdout, leak) {
			t.Fatalf("secret %q leaked:\n%s", leak, r.stdout)
		}
	}
	for _, want := range []string{"https://file.example", "dosya", "envuser", "ortam", "a, b", "bayrak", "********", "Durum: geçerli"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("output missing %q:\n%s", want, r.stdout)
		}
	}

	r = runCLI(t, env, "", "--config", path, "--json", "config", "show")
	if r.code != ExitOK || strings.Contains(r.stdout, "secret") {
		t.Fatalf("code=%d out=%s", r.code, r.stdout)
	}
	var v configShowJSON
	if err := json.Unmarshal([]byte(r.stdout), &v); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, r.stdout)
	}
	if !v.Valid || v.Values.URL != "https://file.example" || v.Sources[config.KeyToken] != config.SourceEnv {
		t.Errorf("unexpected json: %+v", v)
	}
}

func TestConfigShowInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("auth: bearer\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	r := runCLI(t, nil, "", "--config", path, "config", "show")
	if r.code != ExitOK || !strings.Contains(r.stdout, "url ayarlı değil") || !strings.Contains(r.stdout, "token gerekli") {
		t.Fatalf("got %+v", r)
	}
}

func TestConfigErrorsExitWith2(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	r := runCLI(t, nil, "", "--config", missing, "config", "show")
	if r.code != ExitConfig || !strings.Contains(r.stderr, "bulunamadı") {
		t.Fatalf("got %+v", r)
	}
}

func TestConfigInit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "shelf", "config.yaml")
	answers := strings.Join([]string{
		"not-a-url",                        // rejected, asked again
		"https://art.example/artifactory/", // URL
		"builder",                          // user
		"",                                 // auth method → token (default)
		"s3cr3t",                           // token
		"conan-local, conan-3rd",           // repos
		"/profiles",                        // profiles_dir
		"",                                 // insecure → no
		"1m",                               // timeout
		"e",                                // save
	}, "\n") + "\n"

	r := runCLI(t, map[string]string{config.EnvToken: "env-token-must-not-be-saved"}, answers, "--config", path, "config", "init")
	if r.code != ExitOK {
		t.Fatalf("code=%d stderr=%s\nstdout=%s", r.code, r.stderr, r.stdout)
	}
	if strings.Contains(r.stdout, "s3cr3t") {
		t.Fatal("token echoed to output")
	}
	if !strings.Contains(r.stdout, "Geçerli bir http") {
		t.Error("invalid URL was not rejected")
	}

	c, err := config.Load(config.Options{Path: path, LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		t.Fatal(err)
	}
	if c.URL != "https://art.example/artifactory" || c.User != "builder" || c.Token.Reveal() != "s3cr3t" ||
		strings.Join(c.Repos, ",") != "conan-local,conan-3rd" || c.ProfilesDir != "/profiles" ||
		c.InsecureTLS || c.Timeout.String() != "1m0s" {
		t.Errorf("saved config mismatch: %+v", c)
	}

	// Re-running with empty answers keeps every existing value, including the token.
	r = runCLI(t, nil, strings.Repeat("\n", 10), "--config", path, "config", "init")
	if r.code != ExitOK {
		t.Fatalf("second run: code=%d stderr=%s", r.code, r.stderr)
	}
	c2, err := config.Load(config.Options{Path: path, LookupEnv: func(string) (string, bool) { return "", false }})
	if err != nil {
		t.Fatal(err)
	}
	if c2.Token.Reveal() != "s3cr3t" || c2.URL != c.URL || strings.Join(c2.Repos, ",") != "conan-local,conan-3rd" {
		t.Errorf("values not preserved: %+v", c2)
	}
}

func TestConfigInitAbortOnEOF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	r := runCLI(t, nil, "https://x.example\n", "--config", path, "config", "init")
	if r.code == ExitOK {
		t.Fatalf("expected failure, got %+v", r)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("config written despite aborted input")
	}
}

func fakeArtifactory(t *testing.T, status int, body string) (*httptest.Server, *[]string) {
	t.Helper()
	var queries []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/artifactory/api/system/ping" {
			_, _ = io.WriteString(w, "OK")
			return
		}
		b, _ := io.ReadAll(r.Body)
		queries = append(queries, string(b))
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	t.Cleanup(srv.Close)
	return srv, &queries
}

func TestDebugAQL(t *testing.T) {
	srv, queries := fakeArtifactory(t, http.StatusOK, `{"results":[{"repo":"conan-local","path":"p","name":"conan_package.tgz"}],"range":{"total":1}}`)
	env := map[string]string{config.EnvURL: srv.URL, config.EnvToken: "secret-tok"}

	out := filepath.Join(t.TempDir(), "aql.json")
	r := runCLI(t, env, "", "--config", writeEmptyConfig(t), "debug", "aql", "--limit", "2", "--out", out)
	if r.code != ExitOK {
		t.Fatalf("code=%d stderr=%s", r.code, r.stderr)
	}
	if len(*queries) != 1 || !strings.Contains((*queries)[0], `"name":"conan_package.tgz"`) || !strings.HasSuffix((*queries)[0], ".limit(2)") {
		t.Errorf("queries = %v", *queries)
	}
	data, err := os.ReadFile(out)
	if err != nil || !strings.Contains(string(data), "\n  \"results\"") {
		t.Errorf("output not pretty JSON: %q, %v", data, err)
	}
	if strings.Contains(r.stderr+r.stdout+string(data), "secret-tok") {
		t.Error("token leaked")
	}
}

func TestDebugAuthErrorExitsWith2(t *testing.T) {
	srv, _ := fakeArtifactory(t, http.StatusUnauthorized, `{"errors":[{"status":401,"message":"Bad credentials"}]}`)
	env := map[string]string{config.EnvURL: srv.URL, config.EnvToken: "secret-tok"}
	r := runCLI(t, env, "", "--config", writeEmptyConfig(t), "debug", "aql")
	if r.code != ExitConfig || !strings.Contains(r.stderr, "401") || strings.Contains(r.stderr, "secret-tok") {
		t.Fatalf("got %+v", r)
	}
}

func TestDebugPing(t *testing.T) {
	srv, _ := fakeArtifactory(t, http.StatusOK, `{"results":[]}`)
	env := map[string]string{config.EnvURL: srv.URL, config.EnvRepos: "conan-local,other"}
	r := runCLI(t, env, "", "--config", writeEmptyConfig(t), "debug", "ping")
	if r.code != ExitOK || !strings.Contains(r.stdout, "Repo other: erişilebilir ama boş") {
		t.Fatalf("got %+v", r)
	}
}

func TestDebugRequiresURL(t *testing.T) {
	r := runCLI(t, nil, "", "--config", writeEmptyConfig(t), "debug", "ping")
	if r.code != ExitConfig || !strings.Contains(r.stderr, "url ayarlı değil") {
		t.Fatalf("got %+v", r)
	}
}

func writeEmptyConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(p, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}
