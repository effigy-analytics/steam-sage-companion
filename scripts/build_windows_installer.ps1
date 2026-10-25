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
    New-Item -ItemType Directory -Force $Output | Out-Null
    $downloads = [Collections.Generic.List[object]]::new()
    $verified = $false
    # Alternate official SourceForge transport is acceptable only for identical pinned bytes.
    $urls = @(($manifest.url + '?download=1'), 'https://psychz.dl.sourceforge.net/project/nsis/NSIS%203/3.13/nsis-3.13.zip?viasf=1', 'https://netix.dl.sourceforge.net/project/nsis/NSIS%203/3.13/nsis-3.13.zip?viasf=1')
    foreach ($url in $urls) {
        $record = [ordered]@{ url=$url; expected_sha256=$manifest.sha256 }
        try {
            # SourceForge served an HTML download landing page to native PowerShell.
            # curl's explicit download transport follows the official signed mirror URL.
            $transport = & curl.exe --fail --location --silent --show-error --retry 2 --connect-timeout 15 --max-time 90 --proto '=https' --proto-redir '=https' --user-agent 'curl/8.16.0' --output $zip --write-out '%{json}' $url
            $curlExit = $LASTEXITCODE
            $response = $transport | ConvertFrom-Json
            $record.status = [int]$response.http_code
            $record.final_url = [string]$response.url_effective
            $record.content_type = [string]$response.content_type
            $record.transport = 'curl.exe HTTPS only'
            if ($curlExit) { throw "curl failed with exit $curlExit" }
            $record.bytes = (Get-Item $zip).Length
            $record.sha256 = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLowerInvariant()
            $stream = [IO.File]::OpenRead($zip)
            try { $prefix = [byte[]]::new(8); $n = $stream.Read($prefix, 0, 8); $record.first_bytes_hex = [Convert]::ToHexString($prefix, 0, $n) } finally { $stream.Dispose() }
            $verified = $record.sha256 -eq $manifest.sha256
        } catch { $record.error = $_.Exception.Message }
        $downloads.Add($record)
        $downloads | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $Output 'nsis-download.json') -Encoding utf8
        Write-Host ($record | ConvertTo-Json -Compress)
        if ($verified) { break }
    }
    if (-not $verified) { throw 'NSIS archive checksum mismatch; see nsis-download.json (unverified compiler was not executed)' }
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
