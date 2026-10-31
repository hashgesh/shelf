<#
.SYNOPSIS
    Windows build script for shelf (equivalent of the Makefile).
.EXAMPLE
    .\build.ps1 build
    .\build.ps1 release
#>
param(
    [ValidateSet('build', 'test', 'lint', 'release', 'clean', 'all')]
    [string]$Target = 'build'
)

$ErrorActionPreference = 'Stop'

$Binary = 'shelf'
$Pkg    = './cmd/shelf'
$Dist   = 'dist'
$Stage  = 'build/release'
$Targets = @(
    @{ Os = 'linux';   Arch = 'amd64' },
    @{ Os = 'linux';   Arch = 'arm64' },
    @{ Os = 'windows'; Arch = 'amd64' }
)

$Version = (git describe --tags --always --dirty 2>$null)
if (-not $Version) { $Version = 'dev' }
$Commit = (git rev-parse --short HEAD 2>$null)
if (-not $Commit) { $Commit = 'none' }
$Date    = (Get-Date).ToUniversalTime().ToString('yyyy-MM-ddTHH:mm:ssZ')
$LdFlags = "-s -w -X main.version=$Version -X main.commit=$Commit -X main.date=$Date"

$env:CGO_ENABLED = '0'

function Invoke-Checked([scriptblock]$Block) {
    & $Block
    if ($LASTEXITCODE -ne 0) { throw "command failed with exit code $LASTEXITCODE" }
}

function Build-Target([string]$Os, [string]$Arch, [string]$Out) {
    $env:GOOS = $Os
    $env:GOARCH = $Arch
    try {
        Invoke-Checked { go build -trimpath -ldflags $LdFlags -o $Out $Pkg }
    } finally {
        Remove-Item Env:GOOS, Env:GOARCH -ErrorAction SilentlyContinue
    }
}

function Clean {
    Remove-Item -Recurse -Force bin, build, $Dist -ErrorAction SilentlyContinue
}

switch ($Target) {
    'build' {
        Invoke-Checked { go build -trimpath -ldflags $LdFlags -o "bin/$Binary.exe" $Pkg }
    }
    'test' {
        Invoke-Checked { go test ./... }
    }
    'lint' {
        Invoke-Checked { go vet ./... }
        Invoke-Checked { golangci-lint run ./... }
    }
    'release' {
        # Same layout as the Makefile; tools/pack writes the archives so the
        # Linux binary keeps mode 0755 even when packed on Windows.
        Clean
        foreach ($t in $Targets) {
            $ext = if ($t.Os -eq 'windows') { '.exe' } else { '' }
            Build-Target $t.Os $t.Arch "$Stage/$($t.Os)_$($t.Arch)/$Binary$ext"
        }
        Invoke-Checked { go run ./tools/pack -name $Binary -version $Version -in $Stage -out $Dist }
    }
    'clean' { Clean }
    'all' {
        & $PSCommandPath lint
        & $PSCommandPath test
        & $PSCommandPath build
    }
}
