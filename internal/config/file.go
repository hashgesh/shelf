package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// fileConfig mirrors config.yaml. Secrets are plain strings here because this
// struct is only ever written to the config file itself.
type fileConfig struct {
	URL            string   `yaml:"url,omitempty"`
	User           string   `yaml:"user,omitempty"`
	Token          string   `yaml:"token,omitempty"`
	Password       string   `yaml:"password,omitempty"`
	Repos          []string `yaml:"repos,omitempty"`
	ProfilesDir    string   `yaml:"profiles_dir,omitempty"`
	InsecureTLS    *bool    `yaml:"insecure_tls,omitempty"`
	Timeout        string   `yaml:"timeout,omitempty"`
	Auth           string   `yaml:"auth,omitempty"`
	MetadataSource string   `yaml:"metadata_source,omitempty"`
}

func readFile(path string) (*fileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: config dosyası bulunamadı: %s: %w", ErrInvalid, path, err)
		}
		return nil, fmt.Errorf("%w: config dosyası okunamadı: %w", ErrInvalid, err)
	}
	var fc fileConfig
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&fc); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: %s: %w", ErrInvalid, path, err)
	}
	return &fc, nil
}

// Save writes the persistable fields of c to path, creating parent
// directories. The file is created with 0600 permissions since it may hold
// credentials.
func Save(path string, c *Config) error {
	fc := fileConfig{
		URL:         c.URL,
		User:        c.User,
		Token:       c.Token.Reveal(),
		Password:    c.Password.Reveal(),
		Repos:       c.Repos,
		ProfilesDir: c.ProfilesDir,
		InsecureTLS: &c.InsecureTLS,
	}
	if c.Timeout > 0 {
		fc.Timeout = c.Timeout.String()
	}
	if c.Auth != "" && c.Auth != AuthAuto {
		fc.Auth = c.Auth
	}
	if c.MetadataSource != "" && c.MetadataSource != MetadataAuto {
		fc.MetadataSource = c.MetadataSource
	}

	var buf bytes.Buffer
	buf.WriteString("# shelf yapılandırması. Bu dosya kimlik bilgisi içerebilir, paylaşmayın.\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&fc); err != nil {
		return fmt.Errorf("config kodlanamadı: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("config kodlanamadı: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("config klasörü oluşturulamadı: %w", err)
	}
	// Write to a temp file and rename so a failed write never truncates an
	// existing config.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("config yazılamadı: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }() // no-op after a successful rename
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		// Chmod is a no-op on Windows; elsewhere a failure is worth reporting.
		_ = tmp.Close()
		return fmt.Errorf("config izinleri ayarlanamadı: %w", err)
	}
	if _, err := tmp.Write(buf.Bytes()); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("config yazılamadı: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("config yazılamadı: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("config yazılamadı: %w", err)
	}
	return nil
}
