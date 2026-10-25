# What the companion reads

The companion discovers Steam in expected installation locations and reads the
selected account's local collection namespace file. A custom installation folder
can be selected in local controls. It does not change Steam files, read Steam
login tokens or passwords, or ask for a Steam API key.

After explicit local approval, the chosen Sage origin can read collection IDs,
names and game memberships, the selected SteamID64, opaque install/source IDs,
and successful-read/error metadata. Raw namespace records, filesystem paths,
unrelated Steam settings and credentials are excluded from the protocol.

The companion makes no data uploads. The browser receives the snapshot locally,
keeps the last successful copy and a revocable connection credential, and sends
normalized game IDs to Sage's recommendation API when asking for a pick. The
API does not persist those preferences. The companion does not expand the public
API's access to private owned-library data or friends' private collections.

Local settings contain the selected account/root, source IDs, derived HMAC pairing credentials and the optional startup setting. They are stored in the per-user config
directory, not in the release archive. Revocation removes a site's derived credentials.
Uninstall stops the helper and removes its own launcher/startup files. Settings
remain unless Linux `uninstall --purge` is used. Windows Apps uninstall preserves
settings; revoke sites in Companion before uninstalling to remove their grants. Browser data is managed independently
by Sage or the browser's site-data controls. No cross-device sync is enabled.

Logs and the `inspect` command use counts/read states rather than account IDs,
collection contents, source paths, tokens or launch URLs. Please do not attach
your actual Steam JSON, settings file, control URL or tokens to a public issue.


Guided opening is a request to the operating system's default browser. Its private,
one-use local launch capability is removed from the approval page address as soon
as the page opens. The Sage website never receives that URL or a control token.
The approval page may use a separate browser profile; permission remains bound to
the original Sage origin and selected account. Approval and first-read readiness
are distinct. Cancel/decline receipts hold only verifier and issued-grant hashes
in memory for at most 120 seconds (64 records); they are not persisted. Closing
approval alone grants nothing and does not stop Companion. Optional startup stays
a local choice and upgrades preserve existing consent and settings.

Windows installation is per-user. Settings use a protected owner-only DACL in
roaming AppData. Windows discovery checks known Steam registry locations and
standard folders; it reads collection namespace files only. Installer metadata
contains public source/version identifiers and managed-file hashes, not pairing
credentials. CI fixtures use synthetic Steam data in disposable Windows accounts.
