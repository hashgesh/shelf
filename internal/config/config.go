// Package config loads shelf settings from a YAML file, environment variables
// and command-line flags, in increasing order of precedence.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Environment variable names.
const (
	EnvConfig    = "SHELF_CONFIG"
	EnvURL       = "SHELF_URL"
	EnvUser      = "SHELF_USER"
	EnvToken     = "SHELF_TOKEN"
	EnvPassword  = "SHELF_PASSWORD"
	EnvRepos     = "SHELF_REPOS"
	EnvConanHome = "CONAN_HOME"
)

// Supported values for Config.Auth.
const (
	AuthAuto   = "auto"   // password → basic, token → bearer
	AuthBearer = "bearer" // Authorization: Bearer <token>
	AuthBasic  = "basic"  // Authorization: Basic user:(password|token)
)

// Supported values for Config.MetadataSource.
const (
	MetadataAuto       = "auto"       // properties first, conaninfo.txt when missing
	MetadataProperties = "properties" // Artifactory item properties only
	MetadataConaninfo  = "conaninfo"  // always download conaninfo.txt
)

// Defaults.
const (
	DefaultRepo    = "conan-local"
	DefaultTimeout = 30 * time.Second
)

// ErrInvalid is wrapped by every configuration error so callers can map it
// to the "configuration error" exit code.
var ErrInvalid = errors.New("geçersiz yapılandırma")

// Source tells where an effective setting came from.
type Source string

// Possible setting sources.
const (
	SourceDefault Source = "varsayılan"
	SourceFile    Source = "dosya"
	SourceEnv     Source = "ortam"
	SourceFlag    Source = "bayrak"
)

// Setting keys, shared by the YAML file, Sources and `config show`.
const (
	KeyURL            = "url"
	KeyUser           = "user"
	KeyToken          = "token"
	KeyPassword       = "password"
	KeyRepos          = "repos"
	KeyProfilesDir    = "profiles_dir"
	KeyInsecureTLS    = "insecure_tls"
	KeyTimeout        = "timeout"
	KeyAuth           = "auth"
	KeyMetadataSource = "metadata_source"
)

// Keys lists all setting keys in display order.
var Keys = []string{
	KeyURL, KeyUser, KeyToken, KeyPassword, KeyRepos, KeyProfilesDir,
	KeyInsecureTLS, KeyTimeout, KeyAuth, KeyMetadataSource,
}

// Config is the effective configuration.
type Config struct {
	URL            string
	User           string
	Token          Secret
	Password       Secret
	Repos          []string
	ProfilesDir    string
	InsecureTLS    bool
	Timeout        time.Duration
	Auth           string
	MetadataSource string

	// Path is the config file that was (or would be) read.
	Path string
	// FileFound reports whether Path existed when loading.
	FileFound bool
	// Sources maps each key to the layer that set its value.
	Sources map[string]Source
}

// Overrides carries command-line flag values. A nil field means "not set".
type Overrides struct {
	URL         *string
	User        *string
	Token       *string
	Password    *string
	Repos       []string
	ProfilesDir *string
	InsecureTLS *bool
	Timeout     *time.Duration
}

// Options controls Load.
type Options struct {
	// Path is an explicit config file (e.g. from --config). When set, the
	// file must exist.
	Path string
	// LookupEnv defaults to os.LookupEnv.
	LookupEnv func(string) (string, bool)
	// Flags holds command-line overrides.
	Flags Overrides
	// AllowMissing tolerates a missing explicit Path (used by `config init`).
	AllowMissing bool
}

// DefaultPath returns the platform config file location:
// %APPDATA%\shelf\config.yaml on Windows, ~/.config/shelf/config.yaml
// (or $XDG_CONFIG_HOME/shelf/config.yaml) elsewhere.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("%w: yapılandırma klasörü bulunamadı: %w", ErrInvalid, err)
	}
	return filepath.Join(dir, "shelf", "config.yaml"), nil
}

// DefaultProfilesDir returns $CONAN_HOME/profiles or ~/.conan2/profiles.
func DefaultProfilesDir(lookupEnv func(string) (string, bool)) string {
	if home, ok := lookupEnv(EnvConanHome); ok && home != "" {
		return filepath.Join(home, "profiles")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".conan2", "profiles")
	}
	return filepath.Join(home, ".conan2", "profiles")
}

// Load builds the effective configuration: defaults < file < env < flags.
// It does not validate; call Validate before talking to a server.
func Load(opts Options) (*Config, error) {
	lookup := opts.LookupEnv
	if lookup == nil {
		lookup = os.LookupEnv
	}

	c := &Config{
		Repos:          []string{DefaultRepo},
		ProfilesDir:    DefaultProfilesDir(lookup),
		Timeout:        DefaultTimeout,
		Auth:           AuthAuto,
		MetadataSource: MetadataAuto,
		Sources:        map[string]Source{},
	}
	for _, k := range Keys {
		c.Sources[k] = SourceDefault
	}

	explicit := opts.Path != ""
	path := opts.Path
	if !explicit {
		if p, ok := lookup(EnvConfig); ok && p != "" {
			path, explicit = p, true
		}
	}
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}
	c.Path = path

	fc, err := readFile(path)
	switch {
	case err == nil:
		c.FileFound = true
		if err := c.applyFile(fc); err != nil {
			return nil, err
		}
	case errors.Is(err, os.ErrNotExist) && (!explicit || opts.AllowMissing):
		// No config file is fine; env and flags may supply everything.
	default:
		return nil, err
	}

	c.applyEnv(lookup)
	c.applyFlags(opts.Flags)
	c.URL = strings.TrimRight(c.URL, "/")
	return c, nil
}

func (c *Config) set(key string, src Source) { c.Sources[key] = src }

func (c *Config) applyFile(fc *fileConfig) error {
	if fc.URL != "" {
		c.URL = fc.URL
		c.set(KeyURL, SourceFile)
	}
	if fc.User != "" {
		c.User = fc.User
		c.set(KeyUser, SourceFile)
	}
	if fc.Token != "" {
		c.Token = Secret(fc.Token)
		c.set(KeyToken, SourceFile)
	}
	if fc.Password != "" {
		c.Password = Secret(fc.Password)
		c.set(KeyPassword, SourceFile)
	}
	if len(fc.Repos) > 0 {
		c.Repos = cleanList(fc.Repos)
		c.set(KeyRepos, SourceFile)
	}
	if fc.ProfilesDir != "" {
		c.ProfilesDir = expandHome(fc.ProfilesDir)
		c.set(KeyProfilesDir, SourceFile)
	}
	if fc.InsecureTLS != nil {
		c.InsecureTLS = *fc.InsecureTLS
		c.set(KeyInsecureTLS, SourceFile)
	}
	if fc.Timeout != "" {
		d, err := ParseTimeout(fc.Timeout)
		if err != nil {
			return fmt.Errorf("%s: %w", c.Path, err)
		}
		c.Timeout = d
		c.set(KeyTimeout, SourceFile)
	}
	if fc.Auth != "" {
		c.Auth = strings.ToLower(fc.Auth)
		c.set(KeyAuth, SourceFile)
	}
	if fc.MetadataSource != "" {
		c.MetadataSource = strings.ToLower(fc.MetadataSource)
		c.set(KeyMetadataSource, SourceFile)
	}
	return nil
}

func (c *Config) applyEnv(lookup func(string) (string, bool)) {
	if v, ok := lookup(EnvURL); ok && v != "" {
		c.URL = v
		c.set(KeyURL, SourceEnv)
	}
	if v, ok := lookup(EnvUser); ok && v != "" {
		c.User = v
		c.set(KeyUser, SourceEnv)
	}
	if v, ok := lookup(EnvToken); ok && v != "" {
		c.Token = Secret(v)
		c.set(KeyToken, SourceEnv)
	}
	if v, ok := lookup(EnvPassword); ok && v != "" {
		c.Password = Secret(v)
		c.set(KeyPassword, SourceEnv)
	}
	if v, ok := lookup(EnvRepos); ok && v != "" {
		if repos := cleanList(strings.Split(v, ",")); len(repos) > 0 {
			c.Repos = repos
			c.set(KeyRepos, SourceEnv)
		}
	}
}

func (c *Config) applyFlags(f Overrides) {
	if f.URL != nil {
		c.URL = *f.URL
		c.set(KeyURL, SourceFlag)
	}
	if f.User != nil {
		c.User = *f.User
		c.set(KeyUser, SourceFlag)
	}
	if f.Token != nil {
		c.Token = Secret(*f.Token)
		c.set(KeyToken, SourceFlag)
	}
	if f.Password != nil {
		c.Password = Secret(*f.Password)
		c.set(KeyPassword, SourceFlag)
	}
	if repos := cleanList(f.Repos); len(repos) > 0 {
		c.Repos = repos
		c.set(KeyRepos, SourceFlag)
	}
	if f.ProfilesDir != nil {
		c.ProfilesDir = expandHome(*f.ProfilesDir)
		c.set(KeyProfilesDir, SourceFlag)
	}
	if f.InsecureTLS != nil {
		c.InsecureTLS = *f.InsecureTLS
		c.set(KeyInsecureTLS, SourceFlag)
	}
	if f.Timeout != nil {
		c.Timeout = *f.Timeout
		c.set(KeyTimeout, SourceFlag)
	}
}

// Validate checks that the configuration is usable for talking to a server.
func (c *Config) Validate() error {
	var problems []string
	if c.URL == "" {
		problems = append(problems, fmt.Sprintf("url ayarlı değil (%s, --url veya config dosyası)", EnvURL))
	} else if u, err := url.Parse(c.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		problems = append(problems, fmt.Sprintf("url geçersiz: %q (http:// veya https:// ile başlamalı)", c.URL))
	}
	if len(c.Repos) == 0 {
		problems = append(problems, "en az bir repo gerekli")
	}
	if c.Timeout <= 0 {
		problems = append(problems, "timeout sıfırdan büyük olmalı")
	}
	switch c.Auth {
	case AuthAuto:
	case AuthBearer:
		if c.Token == "" {
			problems = append(problems, "auth=bearer için token gerekli")
		}
	case AuthBasic:
		if c.User == "" || (c.Token == "" && c.Password == "") {
			problems = append(problems, "auth=basic için user ve password (veya token) gerekli")
		}
	default:
		problems = append(problems, fmt.Sprintf("auth geçersiz: %q (auto, bearer, basic)", c.Auth))
	}
	switch c.MetadataSource {
	case MetadataAuto, MetadataProperties, MetadataConaninfo:
	default:
		problems = append(problems, fmt.Sprintf("metadata_source geçersiz: %q (auto, properties, conaninfo)", c.MetadataSource))
	}
	if c.Password != "" && c.User == "" {
		problems = append(problems, "password için user gerekli")
	}
	if len(problems) > 0 {
		return fmt.Errorf("%w: %s", ErrInvalid, strings.Join(problems, "; "))
	}
	return nil
}

// ParseTimeout accepts a Go duration ("30s", "2m") or a plain number of seconds.
func ParseTimeout(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if n, err := strconv.Atoi(s); err == nil {
		if n <= 0 {
			return 0, fmt.Errorf("%w: timeout sıfırdan büyük olmalı: %q", ErrInvalid, s)
		}
		return time.Duration(n) * time.Second, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%w: timeout geçersiz: %q (örnek: 30s, 2m)", ErrInvalid, s)
	}
	return d, nil
}

func cleanList(in []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[1:])
		}
	}
	return p
}
