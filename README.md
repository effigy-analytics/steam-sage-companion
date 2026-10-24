# Steam Sage Companion

An optional, read-only local connection between Steam library collections and
Steam Sage. MIT licensed. Linux x64 alpha. See [downloads and release notes](https://github.com/effigy-analytics/steam-sage-companion/releases) for published builds.

Build with Go 1.26 or newer: `go build ./cmd/steam-sage-companion`.
Run `go run ./cmd/steam-sage-companion inspect` to check discovery. This prints
counts and read states, never paths, account IDs, collection names or raw data.
Run tests with `scripts/with-isolated-network.sh go test ./...` and race checks
with `scripts/with-isolated-network.sh go test -race ./...`. The Linux wrapper
uses a user/network namespace and activates its own loopback interface, so
fixed-port subprocess tests cannot contact an installed helper. These tests
supply synthetic subprocess homes and fake browser dispatchers. Without the
wrapper, fixed-port integration tests are skipped.

Download the Linux x64 alpha from [GitHub Releases](https://github.com/effigy-analytics/steam-sage-companion/releases).
Verify SHA256SUMS, extract it and run `./install.sh`.
For a source build, run the executable's `install` command, then open Steam Sage
Companion from the application launcher. No administrator access is needed.

Run the built helper with `launch` (or no arguments) to start or reuse the
background helper and open Sage's companion setup page. Closing the installer,
launcher, or browser tab leaves the helper running. Choose the launcher's
**Companion Settings** action, or run `settings` (legacy alias `open`), to open
local controls directly. `serve --background` remains the foreground service
command for optional start-at-login. `--config-dir <directory>` isolates an
experimental setup. Local
controls let you select an account, approve/revoke a site, choose a custom Steam
folder, opt into startup, and stop the helper. In Sage choose Connect companion,
then approve its exact origin locally. Read the [installation guide](docs/installation.md).

The tool reads known Steam collection locations without changing them. No Steam
password, API key or private Effigy dependency is needed. It does not upload
data. Last-good snapshots survive temporary source errors during a running
session; the browser will retain its own last successful copy after connection.

`uninstall` stops/removes owned installation/startup files and keeps settings.
`uninstall --purge` also removes owned settings. Changed or unrelated files,
Steam data and browser choices are preserved.

Build release candidates with `python3 scripts/build_release.py --version
0.1.0-alpha.1` from a clean Git commit. This creates allowlisted archives and
checksums, and never publishes them. Dependencies are public and pinned in
go.mod/go.sum; no Effigy credentials, private packages or user Steam data are
needed. The runtime is the single Go executable; Go/Python are build tools.

See [privacy](docs/privacy.md), [protocol](docs/protocol.md), and
[security boundaries](docs/security.md). Report reproducible behavior with
synthetic examples rather than attaching Steam files or credentials.

Linux x64 is the first validation target. Windows is experimental until actual
installation and browser tests are recorded; building it alone is insufficient.
