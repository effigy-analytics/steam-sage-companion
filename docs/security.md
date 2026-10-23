# Connection boundary

This is an optional read-only local helper, not a Steam authentication provider.
It binds only `127.0.0.1:43871`. A private fixed-port service is not an excuse to
accept requests from any website: every v1 request checks Host and exact approved
Origin. Only the production Sage origin is enabled by default. An additional
development origin requires an explicit flag and separate local approval.

Pair requests expire after 120 seconds, are limited in number/rate, and need a
local approval plus the requesting browser's random verifier. Successful claims
are single-use. Pairing secrets are random 256-bit values, bound to origin/account,
and revocable. The browser retains the initial secret; the helper persists its
SHA-256 derivation as an HMAC credential. This derivation is sensitive credential
material, not a harmless hash. OS-private settings protect it. Reconnects use
mutual nonce-bound proofs and authenticate response bytes; neither the initial
secret nor the derived credential is sent to a listener on the fixed port.
Challenges expire, are single-use and do not survive restart. Account changes revoke previous pairings.

Local controls are separate from the website API. Their launch fragment is an
expiring one-use capability, removed immediately and exchanged for a short-lived
in-memory control credential. Control mutation/exchange requires that credential
and the exact loopback Origin. Read-only control GETs also allow same-origin fetch
metadata when browsers omit Origin. No wildcard CORS reaches controls. The local
page embeds no credentials, uses restrictive CSP, and prevents framing.

Linux process activation uses an abstract Unix socket with peer UID checks on
both sides. It cannot activate an arbitrary HTTP listener. Installation never
kills an unknown process to claim the port. It manages only known per-user files,
uses owned-file hashes to preserve changed files, and has no privileged updater.

JSON/file/response sizes, collection names/counts and app IDs are bounded. Invalid
or incomplete reads retain last-good data. Dynamic membership is reported as
unresolved. There is no arbitrary-file endpoint or remote-fetch proxy. Choosing
an installation root is available only in authenticated local controls.

The website receives collection preferences, not filesystem access. The helper
relies on the local operating system and installed executable being trusted;
malicious local software is not sandboxed by CORS. The same-user account can
already read Steam files. An unrelated user occupying the stopped helper's port cannot obtain a reusable
credential or forge a snapshot. This does not isolate the tool from malicious
same-user software, a compromised Sage origin, or a hostile operating system.

Do not send credentials or private Steam files with vulnerability reports. This
candidate has received an independent source review and remains unpublished; recorded browser
tests and remaining platform limits accompany the release candidate.
