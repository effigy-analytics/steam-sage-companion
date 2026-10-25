# Production package boundary. Run ONLY on a disposable GitHub Windows runner.
# Real known folders/HKCU are isolated by a new standard account, never env overrides.
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Output, [string]$SageRoot = '', [string]$BrowserTools = '')
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
if ($env:GITHUB_ACTIONS -ne 'true' -or $env:RUNNER_OS -ne 'Windows') { throw 'Requires disposable GitHub Windows runner' }
$repo = Split-Path $PSScriptRoot -Parent
$root = Join-Path $env:SystemDrive ('SageBoundary-' + [guid]::NewGuid().ToString('N'))
$name = 'SageQA' + [guid]::NewGuid().ToString('N').Substring(0, 12)
$password = ConvertTo-SecureString ('Aa1!' + [guid]::NewGuid().ToString('N')) -AsPlainText -Force
New-Item -ItemType Directory -Force $Output, $root | Out-Null
$user = New-LocalUser -Name $name -Password $password -AccountNeverExpires
try {
    Add-LocalGroupMember -SID 'S-1-5-32-545' -Member $user
    if (Get-LocalGroupMember -SID 'S-1-5-32-544' | Where-Object SID -eq $user.SID) { throw 'Fixture account is an administrator' }
    & icacls.exe $root /grant ('*' + $user.SID.Value + ':(OI)(CI)M') | Out-Null
    if ($LASTEXITCODE) { throw 'Fixture ACL grant failed' }
    $build = Join-Path $root 'production'
    & (Join-Path $PSScriptRoot 'build_windows_installer.ps1') -Version '0.1.0-alpha.3' -Output $build | Out-Host
    $setup = Join-Path $build 'steam-sage-companion-0.1.0-alpha.3-windows-amd64-setup.exe'
    Copy-Item (Join-Path $PSScriptRoot 'windows_boundary_user.ps1') $root
    # A synthetic default URL handler observes real rundll32/ShellExecute dispatch.
    # It is test infrastructure, never part of a package or distributed executable.
    Copy-Item (Join-Path $PSScriptRoot 'windows_url_capture.cs') $root
    $csc = Join-Path $env:WINDIR 'Microsoft.NET/Framework64/v4.0.30319/csc.exe'
    & $csc /nologo /target:winexe ("/out:$root/capture.exe") (Join-Path $root 'windows_url_capture.cs')
    if ($LASTEXITCODE) { throw 'Capture fixture build failed' }
    if ($SageRoot) {
        New-Item -ItemType Directory -Force (Join-Path $root 'sage/src/test/fixtures') | Out-Null
        Copy-Item (Join-Path $SageRoot 'dist') (Join-Path $root 'sage/dist') -Recurse
        Copy-Item (Join-Path $SageRoot 'scripts') (Join-Path $root 'sage/scripts') -Recurse
        Copy-Item (Join-Path $SageRoot 'src/test/fixtures/api') (Join-Path $root 'sage/src/test/fixtures/api') -Recurse
    }
    $parameters = @{ root=$root; setup=$setup; expected_sid=$user.SID.Value; node=(Get-Command node -ErrorAction SilentlyContinue).Source; browser_tools=$BrowserTools; sage=[bool]$SageRoot }
    $parameters | ConvertTo-Json | Set-Content (Join-Path $root 'parameters.json') -Encoding utf8
    $credential = [pscredential]::new("$env:COMPUTERNAME\$name", $password)
    $shell = (Get-Command pwsh).Source
    $arguments = '-NoProfile -NonInteractive -File "' + (Join-Path $root 'windows_boundary_user.ps1') + '" -Root "' + $root + '"'
    $child = Start-Process $shell -Credential $credential -LoadUserProfile -WorkingDirectory $root -ArgumentList $arguments -PassThru
    if (-not $child.WaitForExit(600000)) { $child.Kill(); throw 'Standard-user acceptance timed out' }
    if ($child.ExitCode -ne 0) { throw "Standard-user acceptance failed: $($child.ExitCode)" }
    if (-not (Test-Path (Join-Path $root 'evidence/result.json'))) { throw 'Missing standard-user evidence' }
} finally {
    if (Test-Path (Join-Path $root 'evidence')) { Copy-Item (Join-Path $root 'evidence/*') $Output -Recurse -Force }
    if (Test-Path (Join-Path $root 'production/windows-build.json')) { Copy-Item (Join-Path $root 'production/windows-build.json') $Output }
    # Terminate only fixture-account processes on this disposable runner, never by name.
    Get-CimInstance Win32_Process | ForEach-Object {
        $owner = Invoke-CimMethod -InputObject $_ -MethodName GetOwnerSid -ErrorAction SilentlyContinue
        if ($owner -and $owner.Sid -eq $user.SID.Value) { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
    }
    Remove-LocalUser -Name $name
    # Retain root on the disposable VM for debugging; upload only sanitized evidence.
}
