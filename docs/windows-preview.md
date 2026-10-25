# Windows x64 preview — 0.1.0-alpha.3

This preview needs Windows desktop feedback before Windows 10/11 can be called
supported. Windows Server CI and cross-compilation do not establish desktop,
real Steam, default-browser, download reputation, or SmartScreen behavior.
Linux users should keep using the published alpha.2 Linux installer.

## Install and connect

1. From the project's [release page](https://github.com/effigy-analytics/steam-sage-companion/releases),
   download `steam-sage-companion-0.1.0-alpha.3-windows-amd64-setup.exe` and compare
   its SHA-256 with `SHA256SUMS`. Only use published assets from the verified release.
2. Open Setup and follow its instructions. No terminal or administrator access is
   needed. This preview is unsigned; Windows may display an unknown-publisher or
   reputation warning. We cannot promise warning-free downloads or execution.
   Review the source and release identity before deciding whether to run it;
   do not disable Windows protections. See [Microsoft's SmartScreen explanation](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/smartscreen-reputation).
3. Return to Sage. Choose **Connect this browser**, allow this site's local
   connection request if shown, then select your Steam account and approve
   **Allow collections** in the Companion page. Keep the original Sage tab open
   even if Windows opens approval in a different browser.
4. Sage reports ready only after it successfully reads the selected account's
   collections. Choose **Find me a game**. Browser-only Sage remains available.

Setup installs only for the current Windows user under LocalAppData/Programs/
Steam Sage Companion. The Start menu contains **Steam Sage Companion** and
**Companion Settings**; Windows Apps settings contains an uninstall entry.
Settings and pairing credentials live under the user's roaming AppData/
steam-sage-companion with a protected owner-only ACL. Normal installation never
requires JSON file selection. Local controls support alternate Steam folders
and account choice without changing Steam files or reading Steam credentials.

Closing Setup or the browser leaves Companion running in the background.
**Companion Settings** works when Companion is stopped, too. Start at login is
optional and initially off. Changing it in local controls changes only this
user's dedicated startup registration. No service, updater or tray is installed.

## Upgrade, stop and uninstall

Run the newer published Setup to upgrade. It preserves selected account/root,
pairings, browser choices and the start-at-login setting. It waits for an owned
helper to exit before replacing its executable. Changed or unowned files and
registrations are preserved with an explanation; do not delete recovery evidence
blindly. Ordinary-error rollback is supported; power-loss atomicity is not promised.

Use local controls to stop the helper. Restart from either Start-menu shortcut.
Uninstall through Windows Apps settings. Uninstall removes matching owned
package/shortcut/startup entries and stops only the verified installed helper.
It preserves Steam files, local settings and browser data. Revoke a paired site
in Companion before uninstalling if you want to remove that site's local grant.
Windows has no CLI purge workflow; browser data is managed in Sage/site settings.

The portable ZIP contains the same GUI executable and documentation, for manual
preview use. It does not create an Apps entry or shortcuts. Prefer Setup for the
normal installation and update workflow. `install`/`uninstall` CLI commands on
Windows direct users to Setup/Windows Apps settings; they are not a second installer.

## Friend desktop checklist

Record Windows edition/build, browser/version and the published release checksum.
Use a standard Windows account. Report pass/fail for each item without attaching
Steam files, settings, tokens, private control URLs or collection contents.

- Download and open Setup; record publisher/reputation prompts and installer UI.
- Close Setup. Open Sage and confirm Steam discovery/account choice in local controls.
- Connect, allow browser local access if prompted, approve in the default browser,
  return to the original Sage tab, and get a recommendation after a successful read.
- Close/reopen Sage; stop Companion and open **Companion Settings** while stopped.
- Opt into start at login, sign out/in, verify it starts; turn it off and repeat.
- Run Setup again; verify pairing, chosen account, startup and manual feedback remain.
- Uninstall through Windows Apps settings; verify helper/shortcuts/startup are removed
  and Steam plus browser choices remain intact.

Report issues with synthetic examples or redacted screenshots. Windows 10 and
Windows 11 results remain outstanding until actual friends complete this checklist.

## Maintainer release gate

Build from a clean explicit public Git ref with Go 1.26.8, Python 3 and PowerShell 7:
`python scripts/build_release.py --version 0.1.0-alpha.3 --platform windows --output C:\candidate-one`.
Repeat into a different empty directory and run
`python scripts/verify_release.py C:\candidate-one C:\candidate-two --version 0.1.0-alpha.3 --source-ref FULL_PUBLIC_SHA`.
The builder checks out the explicit source commit into a temporary tracked
worktree before compiling, verifies its commit/tree identity, disables ambient Go
workspaces, and removes only that temporary worktree afterward. Ignored local
files cannot enter Go's embedded assets. It verifies pinned NSIS 3.13 before
execution and exports only Setup,
portable ZIP, allowlisted public source ZIP and SHA256SUMS. QA adapters and build
scratch files never enter the distributable directory. CI records its actual
checkout (possibly a PR merge); publication requires an explicit merged public ref.

Controller publishes after review and all native/CI checks. Then verify anonymous
Setup/ZIP/source/checksum downloads against the locally reviewed hashes before
merging/deploying Sage's new Windows link. No signing credentials are assumed.

CI first observes a real OS-selected browser process under the disposable user's
SID after public launch, without inspecting command lines or asserting visibility.
For deterministic local-approval capture it then registers a temporary per-user
command for the already-selected ProgID, verifies the effective command, and
confirms UserChoice/Hash are unchanged. That command is fixture instrumentation,
not a public artifact or a default-browser desktop validation claim.
