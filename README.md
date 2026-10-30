# shelf

JFrog Artifactory'deki Conan 2 paketlerini terminalden okunur şekilde gezmek için
bir TUI + CLI aracı. Windows ve Linux'ta tek binary olarak çalışır, harici
çalışma zamanı bağımlılığı yoktur.

> Durum: geliştirme aşamasında.

## Derleme

Gereksinim: Go 1.24 veya üzeri.

Linux / macOS:

```sh
make build      # bin/shelf
make test
make lint       # golangci-lint gerekir
make release    # dist/ altına windows/amd64, linux/amd64, linux/arm64
```

Windows (PowerShell):

```powershell
.\build.ps1 build     # bin\shelf.exe
.\build.ps1 test
.\build.ps1 release
```

Sürüm bilgisini görmek için:

```sh
shelf --version
```

## Yapılandırma

Ayarlar şu öncelik sırasıyla okunur: komut satırı bayrağı > ortam değişkeni > config dosyası.

```sh
shelf config init     # etkileşimli kurulum (token girişi ekranda görünmez)
shelf config show     # etkin ayarlar ve her değerin kaynağı; token/şifre maskelenir
```

Config dosyası konumu:

- Windows: `%APPDATA%\shelf\config.yaml`
- Linux: `~/.config/shelf/config.yaml`
- Başka bir dosya için: `--config <yol>` veya `SHELF_CONFIG`

Ortam değişkenleri: `SHELF_URL`, `SHELF_USER`, `SHELF_TOKEN`, `SHELF_PASSWORD`,
`SHELF_REPOS` (virgülle ayrılmış).

Örnek `config.yaml`:

```yaml
url: https://artifactory.example/artifactory
user: ci-user
token: <access token>
repos:
  - conan-local
profiles_dir: ~/.conan2/profiles
insecure_tls: false
timeout: 30s
# auth: auto | bearer | basic           (varsayılan: auto)
# metadata_source: auto | properties | conaninfo   (varsayılan: auto)
```

`auth: auto` iken şifre verilmişse Basic, yalnızca token verilmişse
`Authorization: Bearer` kullanılır. Kullanıcı adı + token ile Basic için `auth: basic` yazın.

Çıkış kodları: `0` başarı, `1` sonuç yok veya beklenmeyen hata,
`2` yapılandırma veya kimlik doğrulama hatası.

## Lisans

[LICENSE](LICENSE) dosyasına bakın.
