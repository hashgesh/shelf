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

## Lisans

[LICENSE](LICENSE) dosyasına bakın.
