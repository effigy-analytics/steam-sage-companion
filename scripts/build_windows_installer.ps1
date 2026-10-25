[CmdletBinding()]
param(
    [Parameter(Mandatory)][ValidatePattern('^[0-9]+\.[0-9]+\.[0-9]+(-alpha\.[0-9]+)?$')][string]$Version,
    [Parameter(Mandatory)][string]$Output,
    # Explicit QA compiler adapter. Artifacts are labeled QA and must never be distributed.
    [string]$TestRoot = ''
)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$repo = Split-Path $PSScriptRoot -Parent
if (-not [IO.Path]::IsPathFullyQualified($Output)) { throw 'Output must be absolute' }
if ($TestRoot -and -not [IO.Path]::IsPathFullyQualified($TestRoot)) { throw 'TestRoot must be absolute' }
Push-Location $repo
try {
    if (git status --porcelain) { throw 'Commit source changes before building installer artifacts' }
    $source = (git rev-parse HEAD).Trim()
    $manifest = Get-Content packaging/windows/toolchain.json -Raw | ConvertFrom-Json
    $cache = Join-Path ([IO.Path]::GetTempPath()) ('steam-sage-nsis-' + $manifest.sha256)
    New-Item -ItemType Directory -Force $cache | Out-Null
    $zip = Join-Path $cache 'nsis.zip'
    if (-not (Test-Path $zip)) { Invoke-WebRequest $manifest.url -OutFile $zip }
    if ((Get-FileHash $zip -Algorithm SHA256).Hash.ToLowerInvariant() -ne $manifest.sha256) { throw 'NSIS archive checksum mismatch' }
    $toolDir = Join-Path $cache 'verified'
    # Re-extract only the hash-verified archive before executing the compiler.
    Expand-Archive $zip -DestinationPath $toolDir -Force
    $compiler = Join-Path $toolDir ('nsis-' + $manifest.version + '/makensis.exe')
    New-Item -ItemType Directory -Force $Output | Out-Null
    $payload = Join-Path $Output 'steam-sage-companion.exe'
    $env:CGO_ENABLED = '0'; $env:GOOS = 'windows'; $env:GOARCH = 'amd64'
    $tags = @()
    $ld = "-s -w -H=windowsgui -X main.version=$Version"
    if ($TestRoot) { $tags = @('-tags', 'packageqa'); $ld += " -X 'github.com/effigy-analytics/steam-sage-companion/internal/install.qaRoot=$TestRoot'" }
    & go build -mod=readonly -trimpath -buildvcs=false @tags -ldflags $ld -o $payload ./cmd/steam-sage-companion
    if ($LASTEXITCODE) { throw 'Windows helper build failed' }
    $label = if ($TestRoot) { '-QA' } else { '' }
    $setup = Join-Path $Output "steam-sage-companion-$Version-windows-amd64$label-setup.exe"
    $defs = @("/DOUTPUT=$setup", "/DPAYLOAD=$payload", "/DVERSION=$Version", "/DSOURCE=$source", "/DREPO=$repo")
    if ($TestRoot) { $defs += "/DQA_ROOT=$TestRoot" }
    & $compiler /V3 @defs (Join-Path $repo 'packaging/windows/installer.nsi')
    if ($LASTEXITCODE) { throw 'NSIS compilation failed' }
    $metadata = [ordered]@{ version=$Version; source_ref=$source; go_version=(& go version); nsis=$manifest.version; nsis_sha256=$manifest.sha256; status='preview'; qa_adapter=[bool]$TestRoot; setup_sha256=(Get-FileHash $setup).Hash.ToLowerInvariant() }
    $metadata | ConvertTo-Json | Set-Content (Join-Path $Output 'windows-build.json') -Encoding utf8
    return $setup
} finally { Pop-Location }
