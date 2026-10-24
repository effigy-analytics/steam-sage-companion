# Linux alpha installation

This is the Linux x64 alpha. Download it from the project's
[GitHub Releases](https://github.com/effigy-analytics/steam-sage-companion/releases)
and check its SHA-256 against the release's SHA256SUMS.
Extract it, open that directory, then run `./install.sh`. Installation is per-user
and needs no administrator access. It places the executable under `~/.local/bin`
and an application launcher under the user's data directory.

Open Steam Sage Companion from the application launcher. It starts the installed
helper in the background and opens Sage's companion setup page. The installer and
launcher can then close; closing a browser page also leaves the helper running.
Sage's browser-only experience remains available, and installing does not pair a
site: choose Connect companion to approve the exact Sage site in local controls.
The browser may ask to connect to software on this computer. Only approve the
Sage site you use.

To open local controls directly, choose **Companion Settings** from the application
launcher's secondary actions, or run
`~/.local/bin/steam-sage-companion settings`. The local page shows discovered
accounts and installation folders; choose one if there are several. The same
account in native and Flatpak Steam can be selected independently. Switching
installation or account revokes previous site connections. Complete, Play Next
and Backlog names are recognized automatically; roles remain editable in Sage.
Unknown/dynamic collections are not guessed.

The local page offers optional start-at-login; it is off by default. Stopping the
helper leaves the browser's last good snapshot usable with its last-read label.
Start it again from the launcher to reconnect. No repeated JSON
selection or upload is needed. Unusual Steam installs can use the folder picker
or the local path fallback. If the fixed port is occupied by another application,
the helper reports that conflict without stopping it.

To update, extract the newer archive and run its installer. The installer stops
only the owned helper instance before replacing its executable. Settings and the
selected startup preference are retained. The installer starts the installed copy,
opens Sage, and repairs owned, enabled startup entries to its current path,
so the extracted download can be removed. Changed
or unowned files are preserved rather than overwritten.

Run `~/.local/bin/steam-sage-companion uninstall` to stop/remove the installation.
Add `--purge` to remove owned local settings, or revoke a site in local controls
before uninstalling. Steam files and browser preferences are never deleted. If
an installation file was changed, the uninstaller reports that it was kept.

`launch` (also the no-argument default) reuses an owned ready helper or starts a
detached one, then opens `https://steam-sage.effigy-analytics.com/companion?installed=1`.
The query only acknowledges installation; it is not a readiness or pairing proof.
If browser dispatch fails, open Sage in your browser. The helper checks readiness
for at most five seconds and reports a conflict without trusting or stopping an
unrelated port occupant.

`serve --background` remains the foreground service command used by optional
start-at-login. `settings` and its legacy alias `open` reuse the running helper to
open authenticated local controls. With no helper running they run the helper in
the foreground and open controls. Use `launch` for detached operation.
`--config-dir <absolute-directory>` isolates an experimental setup;
startup/launcher entries preserve that directory. `--allow-origin <exact-origin>`
explicitly enables a separate development-site pairing. Avoid enabling arbitrary
sites. There is no need to change browser security flags or install a certificate.

Windows is experimental and unverified until actual installation/browser testing
is recorded. A cross-compiled executable is not a supported Windows installer.
macOS, other architectures and Steam Deck's handheld experience are not claimed.

Validation on Linux: Chromium 151 completed production-HTTPS-origin pairing,
automatic collection changes, remembered reload, offline cached recommendations,
and helper restart against the built Sage app and synthetic API fixtures. The
real native browser permission bubble was inspected separately. The actual
installer opens the installed executable; deleting the extracted download and
launching the generated desktop startup entry succeeded. Firefox 151 has only
an HTTPS-to-loopback transport check. Edge and Safari are untested, and WebKit
could not launch without additional host libraries. These limits do not imply
support for the untested combinations or a deployed-app end-to-end result.


## Guided setup in alpha.2

The installer returns after starting the installed background copy and asking the
OS to open `https://steam-sage.effigy-analytics.com/companion?installed=1`. This
acknowledgement does not prove installation or contact the helper. Closing the
terminal or a browser page leaves Companion running. A second launch reuses the
owned helper. A conflicting port produces a diagnosis without stopping its owner.

In Sage, explicitly choose **Connect this browser**. The OS may open the approval
page in a different browser. Keep the original Sage tab open. Select the intended
Steam account and choose **Allow collections** only for that exact site request.
If the page is closed or missing, choose **Show approval page again** in Sage, or
use the launcher's **Companion Settings** action. Dispatch reports an OS request,
not proof that a window is visible. **Not now** declines the focused request;
**Cancel connection** in Sage withdraws that attempt, including a racing claim,
without revoking another connection.

A successful first collection read completes setup. Source errors offer retry and
settings recovery; empty and unsupported collections are explained honestly.
Existing alpha.1 installations keep working through manual approval. Upgrades
preserve startup choice, selected account/root, source IDs, pairings, and owned
installation metadata. Browser preferences remain in the original Sage browser;
opening approval in another browser does not transfer them. Start at login stays
off until chosen explicitly in local settings.
