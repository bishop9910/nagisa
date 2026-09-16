# Dev shell helper: keeps every Go/buf cache inside the repository so the
# sandboxed file policy does not have to reach outside the workspace.
# Usage:  . .\scripts\env.ps1

$root = Split-Path -Parent $PSScriptRoot

$env:GOCACHE = Join-Path $root '.cache\go-build'
$env:GOMODCACHE = Join-Path $root '.cache\go-mod'
$env:GOPATH = Join-Path $root '.cache\gopath'
$env:BUF_CACHE_DIR = Join-Path $root '.cache\buf'
$env:GOTELEMETRY = 'off'
$env:GOTOOLCHAIN = 'local'

New-Item -ItemType Directory -Force -Path $env:GOCACHE, $env:GOMODCACHE, $env:GOPATH, $env:BUF_CACHE_DIR | Out-Null

# The launcher in Program Files is older than the go directive in go.mod, so
# put a matching toolchain first on PATH for buf's `go run` plugins.
$candidates = @("$env:USERPROFILE\go\pkg\mod\golang.org", (Join-Path $env:GOMODCACHE 'golang.org'))
$toolchain = $candidates |
    Where-Object { Test-Path $_ } |
    ForEach-Object { Get-ChildItem $_ -Filter 'toolchain@*' -Directory -ErrorAction SilentlyContinue } |
    Where-Object { Test-Path (Join-Path $_.FullName 'bin\go.exe') } |
    Sort-Object Name -Descending |
    Select-Object -First 1

$global:ToolchainGo = if ($toolchain) { Join-Path $toolchain.FullName 'bin\go.exe' } else { '' }
if ($global:ToolchainGo) {
    $env:PATH = "$(Split-Path $global:ToolchainGo -Parent);$env:PATH"
}

Set-Location $root
