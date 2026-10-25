# Native Windows Server CI with an explicitly separate compiler-injected adapter.
# Known folders and registry paths are NOT isolated by environment variables.
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Output)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$root = Join-Path ([IO.Path]::GetTempPath()) ('Sage QA 雪 café ' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory $root | Out-Null
$setup = & "$PSScriptRoot/build_windows_installer.ps1" -Version '0.1.0-alpha.3' -Output $Output -TestRoot $root
$setup = @($setup)[-1]
$payload = Join-Path $Output 'steam-sage-companion.exe'
$install = Join-Path $root 'Programs/Steam Sage Companion'
$stage = Join-Path $root 'inspection'
New-Item -ItemType Directory $stage | Out-Null
$report = Join-Path $stage 'report.ini'
$failures = [Collections.Generic.List[string]]::new()
function Check([string]$Name, [scriptblock]$Body) {
    try { & $Body; Write-Host "PASS $Name" } catch { $failures.Add($Name + ': ' + $_); Write-Host "FAIL $Name`: $_" }
}
function Assert([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Run([string]$File, [string]$Arguments) {
    $p = Start-Process -FilePath $File -ArgumentList $Arguments -PassThru
    if (-not $p.WaitForExit(30000)) { throw 'Fixture process exceeded 30 seconds' }
    $p.Refresh()
    return $p.ExitCode
}
Check 'inspector writes bounded Unicode report without product or settings mutations' {
    $code = Run $payload ('package-check --operation preflight-install --stage-dir "' + $stage + '" --report "' + $report + '"')
    Assert ($code -eq 0) "package-check returned $code"
    $bytes = [IO.File]::ReadAllBytes($report)
    Assert ($bytes.Length -le 16384 -and $bytes[0] -eq 255 -and $bytes[1] -eq 254) 'Report must be bounded UTF-16LE with BOM'
    Assert (-not (Test-Path $install)) 'Inspector created product files'
    Assert (-not (Test-Path (Join-Path $root 'Config'))) 'Inspector initialized settings'
}
Check 'real Setup installs fixed files and Unicode shortcuts' {
    $code = Run $setup '/S'
    Assert ($code -eq 0) "Setup returned $code"
    Assert (Test-Path (Join-Path $install 'package.ini')) 'Setup did not commit package ledger'
    Assert (Test-Path (Join-Path $install 'Uninstall.exe')) 'Setup did not create uninstaller'
    Assert (Test-Path (Join-Path $root 'Start Menu/Steam Sage Companion/Companion Settings.lnk')) 'Missing settings shortcut'
}
[ordered]@{ fixture_root=$root; adapter=$true; platform='Windows Server CI, not Windows 10/11 desktop validation'; failures=@($failures) } | ConvertTo-Json | Set-Content (Join-Path $Output 'package-tests.json') -Encoding utf8
if ($failures.Count) { throw ($failures -join "`n") }
