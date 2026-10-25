# Native Windows Server CI with an explicitly separate compiler-injected adapter.
# Known folders and registry paths are NOT isolated by environment variables.
[CmdletBinding()]
param([Parameter(Mandatory)][string]$Output)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
Add-Type -Path (Join-Path $PSScriptRoot 'windows_shortcut_reader.cs')
$fixtureBase = Join-Path ([Environment]::GetFolderPath('LocalApplicationData')) 'Temp'
$root = Join-Path $fixtureBase ('Sage QA 雪 café ' + [guid]::NewGuid().ToString('N'))
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
$script:observationIndex = 0
$script:evidenceRoots = [Collections.Generic.List[string]]::new()
$script:evidenceRoots.Add($root)
function Check([string]$Name, [scriptblock]$Body) {
    try { & $Body; Write-Host "PASS $Name" } catch { $failures.Add($Name + ': ' + $_); Write-Host "FAIL $Name`: $_" } finally {
        # Retain only bounded synthetic reports before the next scenario resets.
        # Recovery binary payloads stay in fixture roots; never copy settings.
        $script:observationIndex++
        $observation = Join-Path $Output ('observations/' + $script:observationIndex)
        New-Item -ItemType Directory -Force $observation | Out-Null
        $Name | Set-Content (Join-Path $observation 'case.txt') -Encoding utf8
        foreach ($fixture in $script:evidenceRoots) {
            if (-not (Test-Path $fixture)) { continue }
            foreach ($recovery in @(Get-ChildItem $fixture -Directory -Filter '*.recovery')) {
                $destination = Join-Path $observation $recovery.Name
                New-Item -ItemType Directory -Force $destination | Out-Null
                foreach ($name in @('report.ini','before.ini','transaction.ini','candidate.ini','ledger.old')) {
                    $path = Join-Path $recovery.FullName $name
                    if ((Test-Path $path) -and (Get-Item $path).Length -le 65536) { Copy-Item $path (Join-Path $destination $name) }
                }
            }
        }
    }
}
function Assert([bool]$Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Run([string]$File, [string]$Arguments) {
    $p = Start-Process -FilePath $File -ArgumentList $Arguments -PassThru
    try {
        if (-not $p.WaitForExit(30000)) {
            [ordered]@{ executable=$File; pid=$p.Id; arguments=$Arguments; fixture_root=$root } | ConvertTo-Json | Set-Content (Join-Path $Output ('fixture-timeout-' + $p.Id + '.json')) -Encoding utf8
            # Only the exact process returned by Start-Process is ours to stop.
            # No process-name, port-owner, or unrelated helper termination.
            $p.Kill()
            $null = $p.WaitForExit(5000)
            throw 'Fixture process exceeded 30 seconds; its held child process was stopped'
        }
        $p.Refresh()
        return $p.ExitCode
    } finally { $p.Dispose() }
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
    Assert ($root.StartsWith($fixtureBase) -and $root -match '[a-f0-9]{32}$') 'Refusing nonfixture cleanup'
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
function Record-ShortcutEvidence([string]$Phase) {
    $destination = Join-Path $Output ('shortcut-observations/' + $Phase)
    New-Item -ItemType Directory -Force $destination | Out-Null
    $shell = New-Object -ComObject WScript.Shell
    $records = @()
    foreach ($name in @('Steam Sage Companion.lnk', 'Companion Settings.lnk')) {
        $path = Join-Path $root ('Start Menu/Steam Sage Companion/' + $name)
        $entry = [ordered]@{ phase=$Phase; name=$name; requested_path=$path; exists=(Test-Path -LiteralPath $path) }
        if ($entry.exists) {
            $item = Get-Item -LiteralPath $path
            $entry.full_name = $item.FullName
            $entry.bytes = $item.Length
            $entry.sha256 = Hash $item.FullName
            Assert ($item.Length -le 65536) 'Synthetic shortcut exceeds diagnostic bound'
            Copy-Item -LiteralPath $item.FullName -Destination (Join-Path $destination $name)
            $raw = $shell.CreateShortcut($path)
            $canonical = $shell.CreateShortcut($item.FullName)
            $entry.target = $raw.TargetPath; $entry.arguments = $raw.Arguments; $entry.working_directory = $raw.WorkingDirectory
            $entry.canonical_target = $canonical.TargetPath; $entry.canonical_arguments = $canonical.Arguments; $entry.canonical_working_directory = $canonical.WorkingDirectory
            $copy = $shell.CreateShortcut((Join-Path $destination $name))
            $entry.ascii_copy_target = $copy.TargetPath; $entry.ascii_copy_arguments = $copy.Arguments; $entry.ascii_copy_working_directory = $copy.WorkingDirectory
            $entry.native = [SagePackageFixtures.ShortcutReader]::Read($item.FullName)
        }
        Write-Host ($entry | ConvertTo-Json -Compress)
        $records += $entry
    }
    $records | ConvertTo-Json -Depth 5 | Set-Content (Join-Path $destination 'shortcuts.json') -Encoding utf8
    $ledger = Join-Path $install 'package.ini'
    if ((Test-Path $ledger) -and (Get-Item $ledger).Length -le 65536) { Copy-Item $ledger (Join-Path $destination 'package.ini') }
    foreach ($entry in $records) {
        Assert $entry.exists ('Missing native shortcut: ' + $entry.requested_path)
        $shortcut = $entry.native
        $command = if ($entry.name -eq 'Companion Settings.lnk') { 'settings' } else { 'launch' }
        Assert ($shortcut.LoadHResult -eq 0 -and $shortcut.ResolveHResult -eq 0 -and $shortcut.PathHResult -eq 0 -and $shortcut.ArgumentsHResult -eq 0 -and $shortcut.WorkingDirectoryHResult -eq 0) ('Native shortcut load/resolve failed during ' + $Phase + ': ' + ($shortcut | ConvertTo-Json -Compress))
        Assert ($shortcut.TargetPath -ceq (Join-Path $install 'steam-sage-companion.exe')) ('Native Unicode target changed during ' + $Phase + ': ' + $shortcut.TargetPath)
        Assert ($shortcut.Arguments -ceq ($command + ' --config-dir "' + (Join-Path $root 'Config') + '"')) ('Native shortcut arguments changed during ' + $Phase)
        Assert ($shortcut.WorkingDirectory -ceq $install) ('Native shortcut working directory changed during ' + $Phase)
    }
}
Check 'upgrade preserves pairing source startup and exact Unicode shortcut targets' {
    Fresh
    Record-ShortcutEvidence 'before-upgrade'
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
    Record-ShortcutEvidence 'after-upgrade'
    Assert ((Hash $settings) -eq $before) 'Settings bytes changed'
    Assert ((Get-ItemPropertyValue $run SteamSageCompanion) -ceq $command) 'Startup changed'

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
# Capture all fixed managed bytes and registration, including absence, before a
# failing update. These bounded fixture records never include user settings.
function Package-State {
    $state = [ordered]@{}
    foreach ($name in @('steam-sage-companion.exe','Uninstall.exe','LICENSE','THIRD_PARTY_NOTICES.txt','release.json','package.ini')) {
        $state["file:$name"] = Hash (Join-Path $install $name)
    }
    foreach ($name in @('Steam Sage Companion.lnk','Companion Settings.lnk')) {
        $state["link:$name"] = Hash (Join-Path $root ('Start Menu/Steam Sage Companion/' + $name))
    }
    foreach ($name in @('DisplayName','DisplayVersion','Publisher','UninstallString','InstallLocation')) {
        $state["registry:$name"] = Get-ItemPropertyValue (Join-Path $registry 'Uninstall') $name
    }
    $state['registry:Run'] = '<absent>'
    if (Test-Path (Join-Path $registry 'Run')) {
        $value = Get-ItemProperty (Join-Path $registry 'Run')
        if ($value.PSObject.Properties.Name -contains 'SteamSageCompanion') { $state['registry:Run'] = $value.SteamSageCompanion }
    }
    return $state
}
function Assert-PackageState($Before) {
    $after = Package-State
    foreach ($name in $Before.Keys) { Assert ($after[$name] -ceq $Before[$name]) "Prior package changed: $name" }
}
foreach ($failure in @(@{name='first';count='1'}, @{name='later';count='5'})) {
    Check ("journal write failure at " + $failure.name + " move restores every prior file and registration") {
        Fresh
        $before = Package-State
        New-Item -ItemType File (Join-Path $root ('fail-journal-' + $failure.name)) | Out-Null
        # The old implementation ignores the write error. Force its eventual
        # rollback too, so this fixture checks restoration, not only an exit code.
        New-Item -ItemType File (Join-Path $root 'fail-after-registration') | Out-Null
        Assert ((Run $upgrade '/S') -ne 0) 'Journal-write failure was ignored'
        $observed = Join-Path $root 'journal-failure-observed'
        Assert (Test-Path $observed) 'Fixture did not produce an actual journal-write error'
        Assert ([IO.File]::ReadAllText($observed) -ceq $failure.count) 'Journal failed at an unexpected write'
        Write-Host ('Observed actual journal-write error at write ' + $failure.count)
        Assert-PackageState $before
        Assert (@(Get-ChildItem $root -Directory -Filter '*.recovery').Count -gt 0) 'Missing retained recovery evidence'
    }
}
foreach ($operation in @('upgrade','uninstall')) {
    Check ("stale private recovery collision preserves current package and earlier evidence during " + $operation) {
        Fresh
        $before = Package-State
        $stale = Join-Path $root 'stale.recovery'
        Assert ((Run $payload ('package-check --operation preflight-install --stage-dir "' + $stale + '" --report "' + (Join-Path $stale 'report.ini') + '"')) -eq 0) 'Could not create private stale fixture'
        [IO.File]::WriteAllText((Join-Path $stale 'exe.old'), 'earlier recovery payload')
        [IO.File]::WriteAllText((Join-Path $stale 'ledger.old'), 'earlier recovery ledger')
        [IO.File]::WriteAllText((Join-Path $stale 'transaction.ini'), "[exe]`r`nold=yes`r`nnew=yes`r`n", [Text.Encoding]::Unicode)
        $evidence = [ordered]@{}
        foreach ($name in @('exe.old','ledger.old','transaction.ini','report.ini')) { $evidence[$name] = Hash (Join-Path $stale $name) }
        New-Item -ItemType File (Join-Path $root 'force-stage-collision') | Out-Null
        $code = if ($operation -eq 'upgrade') { Run $upgrade '/S' } else { Uninstall }
        Assert ($code -ne 0) 'Accepted an existing recovery directory'
        Assert-PackageState $before
        foreach ($name in $evidence.Keys) { Assert ((Hash (Join-Path $stale $name)) -eq $evidence[$name]) "Earlier recovery evidence changed: $name" }
    }
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
Check 'per-user package lock also honors the global cross-session namespace' {
    Fresh
    $sid = [Security.Principal.WindowsIdentity]::GetCurrent().User.Value
    $name = 'Global\SteamSageCompanion-Package-' + $sid + '-' + (Split-Path $root -Leaf)
    $created = $false
    $mutex = [Threading.Mutex]::new($false, $name, [ref]$created)
    try {
        Assert $created 'Fixture global lock unexpectedly existed'
        $before = Hash (Join-Path $install 'package.ini')
        Assert ((Run $upgrade '/S') -eq 1618) 'Setup ignored an existing global per-user package operation'
        Assert ((Uninstall) -eq 1618) 'Uninstall ignored an existing global per-user package operation'
        Assert ((Hash (Join-Path $install 'package.ini')) -eq $before) 'Contender changed the winning package ledger'
    } finally { $mutex.Dispose() }
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
Check 'redirected Start Menu on another volume supports upgrade and rollback' {
    $rootDrive = [IO.Path]::GetPathRoot($root)
    $otherDrive = @([IO.DriveInfo]::GetDrives() | Where-Object { $_.IsReady -and $_.DriveType -eq 'Fixed' -and $_.Name -ne $rootDrive })
    Assert ($otherDrive.Count -gt 0) 'This native fixture requires a second real fixed volume'
    $crossRoot = Join-Path $fixtureBase ('Sage Cross Volume ' + [guid]::NewGuid().ToString('N'))
    $crossMenu = Join-Path $otherDrive[0].Name ('Sage QA Menu 雪 ' + [guid]::NewGuid().ToString('N'))
    $script:evidenceRoots.Add($crossRoot)
    New-Item -ItemType Directory $crossRoot | Out-Null
    New-Item -ItemType Directory $crossMenu | Out-Null
    Assert ([IO.Path]::GetPathRoot($crossRoot) -ne [IO.Path]::GetPathRoot($crossMenu)) 'Fixture did not cross volumes'
    $first = & "$PSScriptRoot/build_windows_installer.ps1" -Version '0.1.0-alpha.3' -Output (Join-Path $Output 'cross-first') -TestRoot $crossRoot -TestMenuRoot $crossMenu
    $first = @($first)[-1]
    $next = & "$PSScriptRoot/build_windows_installer.ps1" -Version '0.1.0-alpha.4' -Output (Join-Path $Output 'cross-next') -TestRoot $crossRoot -TestMenuRoot $crossMenu
    $next = @($next)[-1]
    Assert ((Run $first '/S') -eq 0) 'Cross-volume fresh install failed'
    $crossInstall = Join-Path $crossRoot 'Programs/Steam Sage Companion'
    $before = Hash (Join-Path $crossInstall 'package.ini')
    New-Item -ItemType File (Join-Path $crossRoot 'fail-after-registration') | Out-Null
    Assert ((Run $next '/S') -ne 0) 'Cross-volume failure injection ignored'
    Assert ((Hash (Join-Path $crossInstall 'package.ini')) -eq $before) 'Cross-volume rollback lost prior ledger'
    Remove-Item (Join-Path $crossRoot 'fail-after-registration')
    Assert ((Run $next '/S') -eq 0) 'Cross-volume update failed'
    Assert ((Hash (Join-Path $crossInstall 'package.ini')) -ne $before) 'Cross-volume update did not commit new version'
    Assert (Test-Path (Join-Path $crossMenu 'Steam Sage Companion/Companion Settings.lnk')) 'Cross-volume shortcut lost'
}
[ordered]@{ fixture_root=$root; adapter=$true; platform='Windows Server CI, not Windows 10/11 desktop validation'; failures=@($failures) } | ConvertTo-Json | Set-Content (Join-Path $Output 'package-tests.json') -Encoding utf8
if ($failures.Count) { throw ($failures -join "`n") }
