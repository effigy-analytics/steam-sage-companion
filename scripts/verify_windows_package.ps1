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
$upgrade = & "$PSScriptRoot/build_windows_installer.ps1" -Version '0.1.0-alpha.4' -Output (Join-Path $Output 'upgrade') -TestRoot $root
$upgrade = @($upgrade)[-1]
$payload = Join-Path $Output 'steam-sage-companion.exe'
$install = Join-Path $root 'Programs/Steam Sage Companion'
$stage = Join-Path $root 'inspection'
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
# Each scenario starts from files created by this QA Setup, never owner destinations.
$registry = 'HKCU:\Software\SteamSageCompanionQA\' + (Split-Path $root -Leaf)
function Reset-Fixture {
    Assert ($root.StartsWith([IO.Path]::GetTempPath()) -and $root -match '[a-f0-9]{32}$') 'Refusing nonfixture cleanup'
    if (Test-Path $root) { Remove-Item $root -Recurse -Force }
    if (Test-Path $registry) { Remove-Item $registry -Recurse -Force }
    New-Item -ItemType Directory $root | Out-Null
}
function Fresh {
    Reset-Fixture
    Assert ((Run $setup '/S') -eq 0) 'Fresh Setup failed'
    Copy-Item (Join-Path $install 'Uninstall.exe') (Join-Path $root 'trusted-uninstaller.exe')
}
function Uninstall {
    # Execute the verified fixture copy synchronously, preserving normal NSIS temp-copy semantics.
    return Run (Join-Path $root 'trusted-uninstaller.exe') ('/S _?=' + $install)
}
function Hash([string]$Path) { return (Get-FileHash $Path -Algorithm SHA256).Hash }
Check 'GUI subsystem and requested user execution level' {
    $bytes = [IO.File]::ReadAllBytes($payload)
    $pe = [BitConverter]::ToInt32($bytes, 0x3c)
    Assert ([BitConverter]::ToUInt16($bytes, $pe + 24 + 68) -eq 2) 'Helper has console subsystem'
    foreach ($file in @($setup, (Join-Path $install 'Uninstall.exe'))) {
        $raw = [Text.Encoding]::UTF8.GetString([IO.File]::ReadAllBytes($file))
        Assert ($raw -match 'requestedExecutionLevel level="asInvoker"') 'Setup/uninstaller requests elevation'
    }
}
Check 'fresh Setup refuses identical but unowned executable' {
    Reset-Fixture
    New-Item -ItemType Directory $install -Force | Out-Null
    Copy-Item $payload (Join-Path $install 'steam-sage-companion.exe')
    $before = Hash (Join-Path $install 'steam-sage-companion.exe')
    Assert ((Run $setup '/S') -ne 0) 'Accepted unowned destination'
    Assert ((Hash (Join-Path $install 'steam-sage-companion.exe')) -eq $before) 'Replaced unowned executable'
    Assert (-not (Test-Path (Join-Path $install 'package.ini'))) 'Claimed unowned package'
}
Check 'upgrade preserves pairing source startup and exact Unicode shortcut targets' {
    Fresh
    $configDir = Join-Path $root 'Config'
    New-Item -ItemType Directory $configDir | Out-Null
    $settings = Join-Path $configDir 'settings.json'
    # Package operations must preserve these bytes, without loading/initializing settings.
    [IO.File]::WriteAllText($settings, '{"kind":"steam-sage-companion","instance_id":"fixture","sources":{"snow":"雪"},"pairings":[{"hash":"unchanged"}],"startup":true}')
    $before = Hash $settings
    $run = Join-Path $registry 'Run'
    New-Item $run -Force | Out-Null
    $command = '"' + (Join-Path $install 'steam-sage-companion.exe') + '" serve --background --config-dir "' + $configDir + '"'
    Set-ItemProperty $run SteamSageCompanion $command
    Assert ((Run $upgrade '/S') -eq 0) 'Upgrade failed'
    Assert ((Hash $settings) -eq $before) 'Settings bytes changed'
    Assert ((Get-ItemPropertyValue $run SteamSageCompanion) -ceq $command) 'Startup changed'
    $shell = New-Object -ComObject WScript.Shell
    foreach ($case in @(@('Steam Sage Companion.lnk','launch'), @('Companion Settings.lnk','settings'))) {
        $shortcut = $shell.CreateShortcut((Join-Path $root ('Start Menu/Steam Sage Companion/' + $case[0])))
        Assert ($shortcut.TargetPath -ceq (Join-Path $install 'steam-sage-companion.exe')) 'Unicode target changed'
        Assert ($shortcut.Arguments -ceq ($case[1] + ' --config-dir "' + $configDir + '"')) 'Shortcut arguments changed'
    }
}
foreach ($damage in @('changed', 'malformed', 'oversize', 'wrong-root', 'reparse')) {
    Check "unsafe ledger $damage preserves installation" {
        Fresh
        $ledger = Join-Path $install 'package.ini'
        switch ($damage) {
            'changed' { [IO.File]::AppendAllText((Join-Path $install 'LICENSE'), 'changed') }
            'malformed' { [IO.File]::WriteAllText($ledger, 'not a ledger') }
            'oversize' { [IO.File]::WriteAllBytes($ledger, [byte[]]::new(65538)) }
            'wrong-root' { $text = [IO.File]::ReadAllText($ledger); [IO.File]::WriteAllText($ledger, $text.Replace($install, ($install + '-foreign')), [Text.Encoding]::Unicode) }
            'reparse' {
                $target = Join-Path $root 'foreign-ledger.ini'; Copy-Item $ledger $target; Remove-Item $ledger
                # Hard links are not reparse points; a real symlink is necessary.
                New-Item -ItemType SymbolicLink -Path $ledger -Target $target | Out-Null
            }
        }
        $before = Hash (Join-Path $install 'steam-sage-companion.exe')
        Assert ((Run $upgrade '/S') -ne 0) 'Accepted unsafe ledger/package'
        Assert ((Hash (Join-Path $install 'steam-sage-companion.exe')) -eq $before) 'Changed prior helper'
        Assert (Test-Path $ledger) 'Removed unsafe ownership evidence'
    }
}
Check 'malformed ledger uninstall preserves all package files' {
    Fresh
    $ledger = Join-Path $install 'package.ini'
    [IO.File]::WriteAllText($ledger, 'broken record')
    $before = Hash (Join-Path $install 'steam-sage-companion.exe')
    Assert ((Uninstall) -ne 0) 'Uninstall accepted damaged ownership'
    Assert ((Hash (Join-Path $install 'steam-sage-companion.exe')) -eq $before) 'Uninstall deleted unverified helper'
    Assert ([IO.File]::ReadAllText($ledger) -ceq 'broken record') 'Uninstall erased damaged ledger evidence'
}
Check 'changed uninstaller is preserved while trusted embedded inspector handles cleanup' {
    Fresh
    $uninstaller = Join-Path $install 'Uninstall.exe'
    [IO.File]::AppendAllText($uninstaller, 'altered')
    $before = Hash $uninstaller
    Assert ((Uninstall) -ne 0) 'Changed uninstaller falsely reported clean'
    Assert ((Hash $uninstaller) -eq $before) 'Changed uninstaller removed/replaced'
    Assert (Test-Path (Join-Path $install 'package.ini')) 'Changed uninstaller evidence removed'
}
Check 'inspector refuses output outside private stage without creating settings' {
    Reset-Fixture
    $outside = Join-Path $root 'outside.ini'
    $code = Run $payload ('package-check --operation preflight-install --stage-dir "' + $stage + '" --report "' + $outside + '"')
    Assert ($code -ne 0) 'Inspector accepted report outside stage'
    Assert (-not (Test-Path $outside)) 'Inspector wrote outside stage'
    Assert (-not (Test-Path (Join-Path $root 'Config'))) 'Inspector initialized config'
}
Check 'locked file update retains prior ledger and working files' {
    Fresh
    $ledger = Hash (Join-Path $install 'package.ini')
    $exe = Hash (Join-Path $install 'steam-sage-companion.exe')
    $lock = [IO.File]::Open((Join-Path $install 'LICENSE'), 'Open', 'Read', 'Read')
    try { Assert ((Run $upgrade '/S') -ne 0) 'Locked-file upgrade succeeded' } finally { $lock.Dispose() }
    Assert ((Hash (Join-Path $install 'package.ini')) -eq $ledger) 'Prior ledger lost'
    Assert ((Hash (Join-Path $install 'steam-sage-companion.exe')) -eq $exe) 'Prior helper lost'
}
Check 'ordinary failure after replacements restores files and registration' {
    Fresh
    $ledger = Hash (Join-Path $install 'package.ini')
    $exe = Hash (Join-Path $install 'steam-sage-companion.exe')
    $registration = Get-ItemPropertyValue (Join-Path $registry 'Uninstall') DisplayVersion
    New-Item -ItemType File (Join-Path $root 'fail-after-registration') | Out-Null
    Assert ((Run $upgrade '/S') -ne 0) 'Injected transaction failure was ignored'
    Assert ((Hash (Join-Path $install 'package.ini')) -eq $ledger) 'Prior ledger lost'
    Assert ((Hash (Join-Path $install 'steam-sage-companion.exe')) -eq $exe) 'Prior helper lost'
    Assert ((Get-ItemPropertyValue (Join-Path $registry 'Uninstall') DisplayVersion) -ceq $registration) 'Prior registration lost'
    Assert (@(Get-ChildItem $root -Directory -Filter '*.recovery').Count -gt 0) 'No retained recovery evidence'
}
Check 'uninstall preserves changed shortcuts registry Run and unknown files with cleanup evidence' {
    Fresh
    $shortcut = Join-Path $root 'Start Menu/Steam Sage Companion/Companion Settings.lnk'
    [IO.File]::AppendAllText($shortcut, 'changed')
    $before = Hash $shortcut
    Set-ItemProperty (Join-Path $registry 'Uninstall') DisplayName 'User changed name'
    $run = Join-Path $registry 'Run'; New-Item $run -Force | Out-Null
    Set-ItemProperty $run SteamSageCompanion 'foreign.exe'
    [IO.File]::WriteAllText((Join-Path $install 'unrelated.txt'), 'keep')
    Assert ((Uninstall) -ne 0) 'Leftovers falsely reported clean'
    Assert ((Hash $shortcut) -eq $before) 'Changed shortcut removed'
    Assert ((Get-ItemPropertyValue (Join-Path $registry 'Uninstall') DisplayName) -ceq 'User changed name') 'Changed registration removed'
    Assert ((Get-ItemPropertyValue $run SteamSageCompanion) -ceq 'foreign.exe') 'Foreign startup removed'
    Assert (Test-Path (Join-Path $install 'unrelated.txt')) 'Unknown file removed'
    Assert (Test-Path (Join-Path $install 'package.ini')) 'Ownership evidence removed'
    Assert (Test-Path (Join-Path $install 'Uninstall.exe')) 'Cleanup uninstaller removed'
}
Check 'normal uninstall preserves settings and browser choice and removes owned package' {
    Fresh
    $configDir = Join-Path $root 'Config'; New-Item -ItemType Directory $configDir | Out-Null
    $settings = Join-Path $configDir 'settings.json'; [IO.File]::WriteAllText($settings, 'preserved settings fixture')
    $browser = Join-Path $root 'browser-preferences'; [IO.File]::WriteAllText($browser, 'preserved browser fixture')
    Assert ((Uninstall) -eq 0) 'Clean uninstall failed'
    Assert (-not (Test-Path (Join-Path $install 'steam-sage-companion.exe'))) 'Owned executable left behind'
    Assert (-not (Test-Path (Join-Path $install 'package.ini'))) 'Clean ledger left behind'
    Assert ([IO.File]::ReadAllText($settings) -ceq 'preserved settings fixture') 'Settings removed'
    Assert ([IO.File]::ReadAllText($browser) -ceq 'preserved browser fixture') 'Browser preferences changed'
}
Check 'concurrent Setup and Uninstall reports busy without disturbing winning transaction' {
    Fresh
    New-Item -ItemType File (Join-Path $root 'hold-lock') | Out-Null
    $winner = Start-Process $upgrade -ArgumentList '/S' -PassThru
    $deadline = [DateTime]::UtcNow.AddSeconds(20)
    while (-not (Test-Path (Join-Path $root 'lock-held')) -and [DateTime]::UtcNow -lt $deadline) { Start-Sleep -Milliseconds 100 }
    Assert (Test-Path (Join-Path $root 'lock-held')) 'Winner did not acquire operation lock'
    try {
        Assert ((Uninstall) -eq 1618) 'Competing uninstaller did not report busy'
        Assert ((Run $setup '/S') -eq 1618) 'Competing Setup did not report busy'
    } finally { Remove-Item (Join-Path $root 'hold-lock') }
    Assert ($winner.WaitForExit(30000)) 'Winning Setup timed out'
    $winner.Refresh(); Assert ($winner.ExitCode -eq 0) 'Winning Setup failed'
    Assert (Test-Path (Join-Path $install 'package.ini')) 'Winning ledger missing'
}
[ordered]@{ fixture_root=$root; adapter=$true; platform='Windows Server CI, not Windows 10/11 desktop validation'; failures=@($failures) } | ConvertTo-Json | Set-Content (Join-Path $Output 'package-tests.json') -Encoding utf8
if ($failures.Count) { throw ($failures -join "`n") }
