# Build helper for Windows: the PowerShell counterpart of the Makefile.
#
#   .\scripts\build.ps1 all          # api + config + generate + docs
#   .\scripts\build.ps1 build        # -> bin\nagisa.exe
#   .\scripts\build.ps1 run          # start the service with ./configs
#   .\scripts\build.ps1 help         # everything else
#
# Nothing here needs GNU make. Only the Go toolchain is required, plus buf for
# the proto targets (installed by `init`).

[CmdletBinding()]
param(
    [Parameter(Position = 0)]
    [string]$Target = 'help',

    # Report formatting problems instead of rewriting the files.
    [switch]$Check
)

# Deliberately not 'Stop': PowerShell turns any stderr line of a native tool
# (progress bars, telemetry notices, "go: downloading ...") into a terminating
# error under that setting, which would abort a perfectly good build. Failures
# are detected through $LASTEXITCODE instead.
$ErrorActionPreference = 'Continue'

$repo = Split-Path -Parent $PSScriptRoot
Set-Location $repo

# The app name matches cmd/<app>.
$app = 'nagisa'

# wire v0.6.0 cannot parse a `go 1.25` module, so the injector is always run
# through the toolchain with `go run` at a version that can.
$wireVersion = 'v0.7.0'

# Tools are resolved once and invoked with the call operator, which passes the
# arguments through verbatim. Routing them through a PowerShell function would
# make switches such as `-o` collide with PowerShell's own parameters.
function Resolve-Tool {
    param([string]$Name, [string]$Hint)
    $cmd = Get-Command $Name -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $gopath = (& go env GOPATH 2>$null)
    if ($gopath) {
        foreach ($candidate in @("$gopath\bin\$Name.exe", "$gopath\bin\$Name")) {
            if (Test-Path $candidate) { return $candidate }
        }
    }
    throw "$Name was not found on PATH or under GOPATH\bin. $Hint"
}

$go = Resolve-Tool -Name 'go' -Hint 'Install the Go toolchain first: https://go.dev/dl/'

function Get-Buf { Resolve-Tool -Name 'buf' -Hint 'Install it with: .\scripts\build.ps1 init' }

function Get-Version {
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $described = (& git describe --tags --always 2>$null)
        if ($LASTEXITCODE -eq 0 -and $described) { return $described.Trim() }
    }
    return 'dev'
}

# Invoke-Step labels a step and fails the script when the tool it runs reports
# a non-zero exit code.
function Invoke-Step {
    param([string]$Label, [scriptblock]$Body)
    Write-Host "==> $Label" -ForegroundColor Cyan
    $global:LASTEXITCODE = 0
    & $Body
    if ($LASTEXITCODE -ne 0 -and $null -ne $LASTEXITCODE) {
        throw "$Label failed with exit code $LASTEXITCODE"
    }
}

function Show-Help {
    @(
        'Usage: .\scripts\build.ps1 <target> [-Check]',
        '',
        'Targets:',
        '  init              install buf and the wire injector',
        '  api               regenerate the proto stubs and openapi.yaml',
        '  config            regenerate internal/conf/conf.pb.go',
        '  generate          run go generate (ent) and go mod tidy',
        '  docs              enrich openapi.yaml and publish it under docs/',
        '  all               api + config + generate + docs',
        '  build             compile the service into bin\',
        '  run               start the service with -conf ./configs',
        '  test              run the unit tests',
        '  test-integration  run the end to end tests against a running server',
        '  fmt               gofmt the tree (-Check to only report)',
        '  vet               go vet the tree',
        '  tidy              go mod tidy',
        '  clean             remove bin\ and .cache\bin',
        '  help              this message'
    ) | ForEach-Object { Write-Host $_ }
}

switch ($Target.ToLowerInvariant()) {
    'init' {
        Invoke-Step 'install buf' { & $go install github.com/bufbuild/buf/cmd/buf@latest }
        Invoke-Step 'install wire' { & $go install "github.com/google/wire/cmd/wire@$wireVersion" }
    }
    'api' {
        $buf = Get-Buf
        Invoke-Step 'buf generate (api)' { & $buf generate --template buf.gen.yaml }
    }
    'config' {
        $buf = Get-Buf
        Invoke-Step 'buf generate (config)' { & $buf generate --template buf.gen.config.yaml }
    }
    'generate' {
        Invoke-Step 'go generate ./...' { & $go generate ./... }
        Invoke-Step 'go mod tidy' { & $go mod tidy }
    }
    'docs' {
        Invoke-Step 'enrich the OpenAPI document' { & $go run ./tools/openapi -in openapi.yaml -out docs }
    }
    'all' {
        & $PSCommandPath api
        & $PSCommandPath config
        & $PSCommandPath generate
        & $PSCommandPath docs
    }
    'build' {
        New-Item -ItemType Directory -Force -Path (Join-Path $repo 'bin') | Out-Null
        $version = Get-Version
        Invoke-Step "go build -> bin\$app.exe ($version)" {
            & $go build -ldflags "-X main.Version=$version" -o ./bin/ ./cmd/...
        }
    }
    'run' {
        & $go run ./cmd/$app -conf ./configs
    }
    'test' {
        Invoke-Step 'go test ./...' { & $go test ./... }
    }
    'test-integration' {
        Invoke-Step 'integration tests' { & $go test -tags integration ./test/integration/ -v }
    }
    'fmt' {
        if ($Check) {
            $unformatted = & gofmt -l ./api ./cmd ./internal ./test ./tools
            if ($unformatted) {
                Write-Host $unformatted
                throw 'gofmt reported unformatted files'
            }
            Write-Host 'gofmt: clean'
        }
        else {
            Invoke-Step 'gofmt -w' { & gofmt -w ./api ./cmd ./internal ./test ./tools }
        }
    }
    'vet' {
        Invoke-Step 'go vet ./...' { & $go vet ./... }
    }
    'tidy' {
        Invoke-Step 'go mod tidy' { & $go mod tidy }
    }
    'clean' {
        foreach ($path in @('bin', '.cache\bin')) {
            $full = Join-Path $repo $path
            if (Test-Path $full) {
                Remove-Item -Recurse -Force $full
                Write-Host "removed $path"
            }
        }
    }
    default { Show-Help }
}
