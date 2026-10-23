# Linux alpha installation

This is a reviewed release candidate pending publication. Use a downloaded Linux
amd64 archive only after checking its SHA-256 against the release's SHA256SUMS.
Extract it, open that directory, then run `./install.sh`. Installation is per-user
and needs no administrator access. It places the executable under `~/.local/bin`
and an application launcher under the user's data directory.

Open Steam Sage Companion from the application launcher. The local browser page
shows discovered accounts and installation folders; choose one if there are several.
The same account in native and Flatpak Steam can be selected independently.
Switching installation or account revokes previous site connections. In Sage, choose
Connect companion and approve its exact site in the local page. The browser may
ask to connect to software on this computer. Only approve the Sage site you use.
Complete, Play Next and Backlog names are recognized automatically; roles remain
editable in Sage. Unknown/dynamic collections are not guessed.

The local page offers optional start-at-login; it is off by default. Stopping the
helper leaves the browser's last good snapshot usable with its last-read label.
Start it again or use Open from the launcher to reconnect. No repeated JSON
selection or upload is needed. Unusual Steam installs can use the folder picker
or the local path fallback. If the fixed port is occupied by another application,
the helper reports that conflict without stopping it.

To update, extract the newer archive and run its installer. The installer stops
only the owned helper instance before replacing its executable. Start it again
from the launcher; settings and selected startup preference are retained. The installer opens the
installed copy and repairs owned, enabled startup entries to its current path,
so the extracted download can be removed. Changed
or unowned files are preserved rather than overwritten.

Run `~/.local/bin/steam-sage-companion uninstall` to stop/remove the installation.
Add `--purge` to remove owned local settings, or revoke a site in local controls
before uninstalling. Steam files and browser preferences are never deleted. If
an installation file was changed, the uninstaller reports that it was kept.

`serve --background` runs without opening controls. `open` reuses the running
helper. `--config-dir <absolute-directory>` isolates an experimental setup;
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
