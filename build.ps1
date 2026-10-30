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
    Remove-Item -Recurse -Force bin, $Dist -ErrorAction SilentlyContinue
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
        Clean
        Build-Target 'windows' 'amd64' "$Dist/$Binary-windows-amd64.exe"
        Build-Target 'linux'   'amd64' "$Dist/$Binary-linux-amd64"
        Build-Target 'linux'   'arm64' "$Dist/$Binary-linux-arm64"
        Get-ChildItem $Dist -File | Get-FileHash -Algorithm SHA256 |
            ForEach-Object { "{0}  {1}" -f $_.Hash.ToLower(), (Split-Path $_.Path -Leaf) } |
            Set-Content -Encoding ascii "$Dist/SHA256SUMS"
    }
    'clean' { Clean }
    'all' {
        & $PSCommandPath lint
        & $PSCommandPath test
        & $PSCommandPath build
    }
}
