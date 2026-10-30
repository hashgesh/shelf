package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/halilibrahimonus/shelf/internal/config"
)

func (a *app) newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Yapılandırmayı oluştur veya görüntüle",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "init",
			Short: "Etkileşimli olarak config dosyası oluştur",
			Args:  cobra.NoArgs,
			RunE:  a.runConfigInit,
		},
		&cobra.Command{
			Use:   "show [bayraklar]",
			Short: "Etkin yapılandırmayı ve değerlerin kaynağını göster (gizli bilgiler maskelenir)",
			Args:  cobra.NoArgs,
			RunE:  a.runConfigShow,
		},
	)
	return cmd
}

// displayValue renders a setting for humans; secrets are never shown.
func displayValue(c *config.Config, key string) string {
	switch key {
	case config.KeyURL:
		return c.URL
	case config.KeyUser:
		return c.User
	case config.KeyToken:
		return secretState(c.Token)
	case config.KeyPassword:
		return secretState(c.Password)
	case config.KeyRepos:
		return strings.Join(c.Repos, ", ")
	case config.KeyProfilesDir:
		return c.ProfilesDir
	case config.KeyInsecureTLS:
		if c.InsecureTLS {
			return "evet"
		}
		return "hayır"
	case config.KeyTimeout:
		return c.Timeout.String()
	case config.KeyAuth:
		return c.Auth
	case config.KeyMetadataSource:
		return c.MetadataSource
	}
	return ""
}

func secretState(s config.Secret) string {
	if s.IsSet() {
		return s.String() + " (ayarlı)"
	}
	return ""
}

type configShowJSON struct {
	Path      string                   `json:"path"`
	FileFound bool                     `json:"file_found"`
	Values    configValuesJSON         `json:"values"`
	Sources   map[string]config.Source `json:"sources"`
	Valid     bool                     `json:"valid"`
	Error     string                   `json:"error,omitempty"`
}

type configValuesJSON struct {
	URL            string        `json:"url"`
	User           string        `json:"user"`
	Token          config.Secret `json:"token"`
	Password       config.Secret `json:"password"`
	Repos          []string      `json:"repos"`
	ProfilesDir    string        `json:"profiles_dir"`
	InsecureTLS    bool          `json:"insecure_tls"`
	Timeout        string        `json:"timeout"`
	Auth           string        `json:"auth"`
	MetadataSource string        `json:"metadata_source"`
}

func (a *app) runConfigShow(cmd *cobra.Command, _ []string) error {
	c, err := a.loadConfig(cmd)
	if err != nil {
		return err
	}
	verr := c.Validate()
	out := cmd.OutOrStdout()

	if a.flags.json {
		v := configShowJSON{
			Path:      c.Path,
			FileFound: c.FileFound,
			Values: configValuesJSON{
				URL: c.URL, User: c.User, Token: c.Token, Password: c.Password,
				Repos: c.Repos, ProfilesDir: c.ProfilesDir, InsecureTLS: c.InsecureTLS,
				Timeout: c.Timeout.String(), Auth: c.Auth, MetadataSource: c.MetadataSource,
			},
			Sources: c.Sources,
			Valid:   verr == nil,
		}
		if verr != nil {
			v.Error = verr.Error()
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(v)
	}

	state := "bulundu"
	if !c.FileFound {
		state = "bulunamadı"
	}
	fmt.Fprintf(out, "Config dosyası: %s (%s)\n\n", c.Path, state)
	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ANAHTAR\tDEĞER\tKAYNAK")
	for _, k := range config.Keys {
		fmt.Fprintf(tw, "%s\t%s\t%s\n", k, displayValue(c, k), c.Sources[k])
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintln(out)
	if verr != nil {
		fmt.Fprintf(out, "Durum: eksik veya hatalı: %s\n", strings.TrimPrefix(verr.Error(), config.ErrInvalid.Error()+": "))
	} else {
		fmt.Fprintln(out, "Durum: geçerli")
	}
	return nil
}

func (a *app) runConfigInit(cmd *cobra.Command, _ []string) error {
	path := a.flags.configPath
	if path == "" {
		if p, ok := a.lookupEnv(config.EnvConfig); ok && p != "" {
			path = p
		}
	}
	// Defaults come from the existing file only; values from the environment
	// or flags are deliberately not persisted.
	cur, err := config.Load(config.Options{
		Path:         path,
		LookupEnv:    func(string) (string, bool) { return "", false },
		AllowMissing: true,
	})
	if err != nil {
		return err
	}
	path = cur.Path

	p := newPrompter(cmd.InOrStdin(), cmd.OutOrStdout())
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "shelf yapılandırması")
	fmt.Fprintf(out, "Dosya: %s\n", path)
	if cur.FileFound {
		fmt.Fprintln(out, "Mevcut değerler köşeli parantez içinde gösterilir; Enter ile koruyun.")
	}
	fmt.Fprintln(out)

	next := *cur
	for {
		v, err := p.ask("Artifactory URL'si (örnek: https://artifactory.example/artifactory)", cur.URL)
		if err != nil {
			return err
		}
		if u, perr := url.Parse(v); perr == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			next.URL = strings.TrimRight(v, "/")
			break
		}
		fmt.Fprintln(out, "  Geçerli bir http:// veya https:// adresi girin.")
	}

	if next.User, err = p.ask("Kullanıcı adı (anonim erişim için boş bırakın)", cur.User); err != nil {
		return err
	}

	method := "token"
	if cur.Password.IsSet() && !cur.Token.IsSet() {
		method = "password"
	}
	for {
		v, err := p.ask("Kimlik doğrulama yöntemi: token veya password", method)
		if err != nil {
			return err
		}
		v = strings.ToLower(v)
		if v == "token" || v == "password" {
			method = v
			break
		}
		fmt.Fprintln(out, "  \"token\" veya \"password\" yazın.")
	}

	if method == "token" {
		s, err := p.askSecret("Access token", cur.Token.IsSet())
		if err != nil {
			return err
		}
		if s != "" {
			next.Token = config.Secret(s)
		}
		next.Password = ""
		next.Auth = config.AuthAuto
	} else {
		if next.User == "" {
			return fmt.Errorf("%w: şifre ile giriş için kullanıcı adı gerekli", config.ErrInvalid)
		}
		s, err := p.askSecret("Şifre", cur.Password.IsSet())
		if err != nil {
			return err
		}
		if s != "" {
			next.Password = config.Secret(s)
		}
		next.Token = ""
		next.Auth = config.AuthBasic
	}

	repos, err := p.ask("Repo(lar), virgülle ayrılmış", strings.Join(cur.Repos, ","))
	if err != nil {
		return err
	}
	next.Repos = nil
	for _, r := range strings.Split(repos, ",") {
		if r = strings.TrimSpace(r); r != "" {
			next.Repos = append(next.Repos, r)
		}
	}

	if next.ProfilesDir, err = p.ask("Conan profil klasörü", cur.ProfilesDir); err != nil {
		return err
	}
	if next.InsecureTLS, err = p.confirm("TLS sertifika doğrulaması kapatılsın mı (self-signed sertifika)?", cur.InsecureTLS); err != nil {
		return err
	}
	for {
		v, err := p.ask("İstek zaman aşımı", cur.Timeout.String())
		if err != nil {
			return err
		}
		d, perr := config.ParseTimeout(v)
		if perr == nil {
			next.Timeout = d
			break
		}
		fmt.Fprintln(out, "  Örnek: 30s, 2m veya saniye cinsinden sayı.")
	}

	if err := next.Validate(); err != nil {
		return err
	}
	ok, err := p.confirm(fmt.Sprintf("%s dosyasına kaydedilsin mi?", path), true)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out, "İptal edildi, hiçbir şey kaydedilmedi.")
		return nil
	}
	if err := config.Save(path, &next); err != nil {
		return err
	}
	fmt.Fprintf(out, "Kaydedildi: %s\n", path)
	return nil
}
