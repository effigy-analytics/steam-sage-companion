[CmdletBinding()]
param([Parameter(Mandatory)][string]$Root)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$evidence = Join-Path $Root 'evidence'
New-Item -ItemType Directory -Force $evidence | Out-Null
$checks = [Collections.Generic.List[string]]::new()
function Assert($Condition, [string]$Message) { if (-not $Condition) { throw $Message } }
function Passed([string]$Message) { $checks.Add($Message); Write-Host "PASS $Message" }
function Run([string]$File, [string]$Arguments) {
    $p = Start-Process -FilePath $File -ArgumentList $Arguments -PassThru
    if (-not $p.WaitForExit(30000)) { throw 'Owned fixture command timed out' }
    return $p.ExitCode
}
function Wait-Ready {
    for ($i=0; $i -lt 200; $i++) {
        try { return Invoke-RestMethod 'http://127.0.0.1:43871/v1/status' -Headers @{Origin='https://steam-sage.effigy-analytics.com'} -TimeoutSec 1 } catch { Start-Sleep -Milliseconds 50 }
    }
    throw 'Installed helper did not become ready'
}
function Dispatch-Diagnostics {
    $record = [ordered]@{ associations=@(); owned_processes=@(); capture_exists=(Test-Path $opened); capture_lines=0 }
    if ($record.capture_exists) { $record.capture_lines = @(Get-Content $opened -TotalCount 100).Count }
    foreach ($scheme in @('http', 'https')) {
        $choice = Get-ItemProperty ("HKCU:\Software\Microsoft\Windows\Shell\Associations\UrlAssociations\$scheme\UserChoice") -Name ProgId -ErrorAction SilentlyContinue
        $progID = if ($choice) { [string]$choice.ProgId } else { '' }
        $entry = [ordered]@{ scheme=$scheme; prog_id=$progID; query=[SageBoundary.ProfileProbe]::Association($scheme); classes=@() }
        foreach ($name in @($scheme, $progID)) {
            if ($name -notmatch '^[A-Za-z0-9._-]{1,128}$') { continue }
            foreach ($hive in @('HKEY_CURRENT_USER\Software\Classes', 'HKEY_CLASSES_ROOT')) {
                $key = Get-Item ("Registry::$hive\$name\shell\open\command") -ErrorAction SilentlyContinue
                if ($key) {
                    $command = [string]$key.GetValue('')
                    $delegate = [string]$key.GetValue('DelegateExecute')
                    $entry.classes += @{ hive=$hive; name=$name; command=$command.Substring(0, [Math]::Min($command.Length, 4096)); delegate_execute=$delegate.Substring(0, [Math]::Min($delegate.Length, 256)) }
                }
            }
        }
        $record.associations += $entry
    }
    foreach ($p in (Get-CimInstance Win32_Process)) {
        $owner = Invoke-CimMethod -InputObject $p -MethodName GetOwnerSid -ErrorAction SilentlyContinue
        if ($owner -and $owner.Sid -eq $identity.User.Value -and $record.owned_processes.Count -lt 64) {
            $record.owned_processes += @{ name=$p.Name; pid=$p.ProcessId; executable=$p.ExecutablePath }
        }
    }
    $record | ConvertTo-Json -Depth 7 | Set-Content (Join-Path $evidence 'dispatch-diagnostics.json') -Encoding utf8
}
function Wait-URL([string]$Prefix) {
    for ($i=0; $i -lt 200; $i++) {
        if (Test-Path $opened) {
            $latest = [string](Get-Content $opened | Select-Object -Last 1)
            if ($latest.StartsWith($Prefix)) { return $latest }
        }
        Start-Sleep -Milliseconds 50
    }
    Dispatch-Diagnostics
    throw 'Native URL handler did not receive the expected destination (private URL omitted)'
}
function OwnedPID {
    $connection = @(Get-NetTCPConnection -LocalPort 43871 -State Listen -ErrorAction SilentlyContinue)
    Assert ($connection.Count -eq 1) 'Expected exactly one loopback listener'
    $process = Get-CimInstance Win32_Process -Filter ("ProcessId=" + $connection[0].OwningProcess)
    Assert ($process.ExecutablePath -eq $bin) 'Listener is not the installed helper'
    $owner = Invoke-CimMethod -InputObject $process -MethodName GetOwnerSid
    Assert ($owner.Sid -eq $identity.User.Value) 'Listener is not owned by fixture account'
    return $process.ProcessId
}
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
try {
    $parameters = Get-Content (Join-Path $Root 'parameters.json') -Raw | ConvertFrom-Json
    Assert ($identity.User.Value -eq $parameters.expected_sid) 'Wrong fixture identity'
    $principal = [Security.Principal.WindowsPrincipal]::new($identity)
    Assert (-not $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) 'Elevated fixture token'
    $groups = @($identity.Groups | ForEach-Object Value)
    Assert ($groups -notcontains 'S-1-5-32-544') 'Token contains administrator group'
    Add-Type -Path (Join-Path $Root 'profile-probe.dll')
    $profileListKey = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\ProfileList\' + $identity.User.Value
    $profileList = Get-ItemProperty -Path $profileListKey -Name ProfileImagePath -ErrorAction SilentlyContinue
    $shellFolders = Get-Item 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Explorer\User Shell Folders' -ErrorAction SilentlyContinue
    $observations = [ordered]@{
        sid=$identity.User.Value
        token_profile=[SageBoundary.ProfileProbe]::Profile($identity.Token)
        profile_list=$(if ($profileList) { $profileList.ProfileImagePath } else { '' })
        environment=@{ USERPROFILE=$env:USERPROFILE; APPDATA=$env:APPDATA; LOCALAPPDATA=$env:LOCALAPPDATA; TEMP=$env:TEMP; USERNAME=$env:USERNAME }
        user_shell_folders=@{}
        native=@()
    }
    if ($shellFolders) {
        foreach ($name in @('AppData', 'Local AppData', 'Programs')) {
            $observations.user_shell_folders[$name] = $shellFolders.GetValue($name, $null, [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
        }
    }
    $folderIDs = @{ LocalApplicationData='f1b32785-6fba-4fcf-9d55-7b8e7f157091'; ApplicationData='3eb685db-65f9-4cf6-a03a-e3ef65729f3d'; Programs='a77f5d77-2e2b-44c3-a6a2-aba601054a51' }
    foreach ($name in $folderIDs.Keys) {
        foreach ($flags in @(0, 0x8000)) {
            $observations.native += @{ name=$name; flags=$flags; current=[SageBoundary.ProfileProbe]::Folder($folderIDs[$name], $flags, [IntPtr]::Zero); explicit_token=[SageBoundary.ProfileProbe]::Folder($folderIDs[$name], $flags, $identity.Token) }
        }
    }
    $observations | ConvertTo-Json -Depth 6 | Set-Content (Join-Path $evidence 'profile-diagnostics.json') -Encoding utf8
    Assert ($observations.token_profile.Error -eq 0 -and $env:USERPROFILE -eq $observations.token_profile.Path) 'Logon environment does not match the native token profile'
    # A newly loaded headless profile has not had Explorer initialize Programs.
    # Create real current-user known folders through the native folder API; do not
    # fabricate paths or substitute APPDATA/HOME for an actual logon boundary.
    $local = [Environment]::GetFolderPath('LocalApplicationData', 'Create')
    $roaming = [Environment]::GetFolderPath('ApplicationData', 'Create')
    $programs = [Environment]::GetFolderPath('Programs', 'Create')
    Assert ($local -and $roaming -and $programs) 'Native profile folder initialization failed'
    Assert ($env:APPDATA -eq $roaming -and $env:LOCALAPPDATA -eq $local) 'Logon AppData environment does not match native known folders'
    $menu = Join-Path $programs 'Steam Sage Companion'
    Assert ($local -and $roaming -and $menu) 'Missing real profile known folders'
    $install = Join-Path $local 'Programs/Steam Sage Companion'
    $bin = Join-Path $install 'steam-sage-companion.exe'
    $config = Join-Path $roaming 'steam-sage-companion'
    Assert (-not (Test-Path $install)) 'Fixture unexpectedly has a production install'
    Assert (-not (Get-NetTCPConnection -LocalPort 43871 -State Listen -ErrorAction SilentlyContinue)) 'Fixture port occupied before Setup'
    $opened = Join-Path $Root 'opened-private.txt'
    foreach ($scheme in @('http', 'https')) {
        $key = "HKCU:\Software\Classes\$scheme"
        New-Item -Path "$key\shell\open\command" -Force | Out-Null
        Set-ItemProperty $key -Name 'URL Protocol' -Value ''
        Set-Item -Path "$key\shell\open\command" -Value ('"' + $Root + '\capture.exe" "' + $opened + '" "%1"')
    }
    Assert ((Run (Join-Path $Root 'capture.exe') ('"' + $opened + '" https://example.invalid/synthetic-capture-smoke')) -eq 0) 'Capture fixture direct smoke failed'
    Assert ((Wait-URL 'https://example.invalid/synthetic-capture-smoke') -eq 'https://example.invalid/synthetic-capture-smoke') 'Capture fixture direct smoke did not record synthetic URL'
    Passed 'synthetic capture fixture direct smoke'
    $steam = Join-Path $Root 'Steam synthetic 漢字'
    $source = Join-Path $steam 'userdata/39734273/config/cloudstorage/cloud-storage-namespace-1.json'
    New-Item -ItemType Directory -Force (Split-Path $source -Parent) | Out-Null
    '[ ["user-collections.done", {"value":"{\"id\":\"done\",\"name\":\"Completed\",\"added\":[1001],\"removed\":[]}"}], ["user-collections.next", {"value":"{\"id\":\"next\",\"name\":\"Play These First\",\"added\":[1002],\"removed\":[]}"}] ]' | Set-Content $source -Encoding utf8NoBOM
    New-Item 'HKCU:\Software\Valve\Steam' -Force | Out-Null
    Set-ItemProperty 'HKCU:\Software\Valve\Steam' -Name SteamPath -Value $steam
    $steamSourceHash = (Get-FileHash $source).Hash
    Assert ((Run $parameters.setup '/S') -eq 0) 'Production Setup failed under standard token'
    $status = Wait-Ready
    Assert (Test-Path (Join-Path $install 'package.ini')) 'Missing production ledger'
    Assert ((Get-FileHash $bin).Hash -eq $parameters.payload_sha256) 'Installed payload differs from the reviewed production build'
    $installedMetadata = Get-Content (Join-Path $install 'release.json') -Raw | ConvertFrom-Json
    Assert ($installedMetadata.source_ref -eq $parameters.source_ref -and -not $installedMetadata.qa_adapter) 'Installed metadata does not identify the production source'
    Assert (Test-Path (Join-Path $menu 'Companion Settings.lnk')) 'Missing native Settings shortcut'
    Assert (Test-Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\SteamSageCompanion') 'Missing current-user Apps registration'
    $firstPID = OwnedPID
    Assert ((Run $bin 'launch') -eq 0) 'Warm launch failed'
    $sageURL = Wait-URL 'https://steam-sage.effigy-analytics.com/'
    Assert ($sageURL -eq 'https://steam-sage.effigy-analytics.com/companion?installed=1') 'Warm launch dispatched the wrong public Sage URL'
    Assert ((OwnedPID) -eq $firstPID) 'Warm launch replaced singleton'
    Assert (-not (Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name SteamSageCompanion -ErrorAction SilentlyContinue)) 'Startup enabled by default'
    Passed 'production Setup, standard token, real known folders/HKCU, background survival, singleton and startup off'
    Assert ((Run $bin 'stop') -eq 0) 'Verified stop failed'
    Assert ((Run $bin 'settings') -eq 0) 'Cold Settings failed'
    $null = Wait-Ready
    $null = OwnedPID
    $null = Wait-URL 'http://127.0.0.1:43871/'
    Passed 'cold Settings returns with owned background process and native ShellExecute dispatch'
    $manifest = @{ root=$Root; setup=$parameters.setup; bin=$bin; config=$config; source=$source; opened=$opened; evidence=$evidence; sid=$identity.User.Value; install=$install }
    $manifest | ConvertTo-Json | Set-Content (Join-Path $Root 'fixture.json') -Encoding utf8
    if ($parameters.sage) {
        $env:COMPANION_WINDOWS_FIXTURE = Join-Path $Root 'fixture.json'
        $env:PLAYWRIGHT_MODULE = Join-Path $parameters.browser_tools 'node_modules/playwright'
        $env:PLAYWRIGHT_BROWSERS_PATH = Join-Path $parameters.browser_tools 'browsers'
        & $parameters.node (Join-Path $Root 'sage/scripts/verify_windows_companion_flow.mjs') *> (Join-Path $evidence 'browser.log')
        if ($LASTEXITCODE) { throw 'Windows browser boundary failed; see browser.log' }
        Passed 'real Chromium HTTPS origin, explicit connection, separate approval context, read recovery, startup choice and upgrade'
    }
    $before = (Get-FileHash (Join-Path $config 'settings.json')).Hash
    $uninstaller = Join-Path $Root 'trusted-uninstaller.exe'
    Copy-Item (Join-Path $install 'Uninstall.exe') $uninstaller
    Assert ((Run $uninstaller ('/S _?=' + $install)) -eq 0) 'Production uninstall failed'
    Assert (-not (Test-Path $bin)) 'Uninstall kept owned executable'
    Assert (-not (Test-Path $menu)) 'Uninstall kept owned shortcuts'
    Assert (-not (Test-Path 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\SteamSageCompanion')) 'Uninstall kept owned Apps registration'
    Assert (-not (Get-ItemProperty 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Run' -Name SteamSageCompanion -ErrorAction SilentlyContinue)) 'Uninstall kept owned startup registration'
    Assert ((Get-FileHash $source).Hash -eq $steamSourceHash) 'Installed flow changed synthetic Steam contents'
    Assert (-not (Get-NetTCPConnection -LocalPort 43871 -State Listen -ErrorAction SilentlyContinue)) 'Uninstall left listener'
    Assert ((Get-FileHash (Join-Path $config 'settings.json')).Hash -eq $before) 'Uninstall changed settings'
    Passed 'actual production uninstaller stops owned process, removes package and preserves settings'
    @{ ok=$true; checks=$checks; os=(Get-CimInstance Win32_OperatingSystem).Caption; build=[Environment]::OSVersion.Version.ToString(); sid=$identity.User.Value; standard_token=$true; desktop_validation=$false; installer_ui='silent execution of production GUI binaries'; default_browser='synthetic per-user URL handler through native ShellExecute' } | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $evidence 'result.json') -Encoding utf8
} catch {
    @{ ok=$false; checks=$checks; error=$_.Exception.Message; location=$_.ScriptStackTrace } | ConvertTo-Json -Depth 4 | Set-Content (Join-Path $evidence 'result.json') -Encoding utf8
    exit 1
}
