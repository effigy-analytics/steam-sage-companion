# Steam Sage companion protocol v1

The fixed base URL is `http://127.0.0.1:43871`. Production Origin is
`https://steam-sage.effigy-analytics.com`. Source/response limit: 2 MiB;
512 collections; 256 UTF-8 bytes per collection ID/name; 10,000 unique game IDs.

GET /v1/status returns protocol, helper version and opaque install ID.
POST /v1/pair/request takes a SHA-256 challenge of the browser's random verifier;
POST /v1/pair/claim takes request_id and verifier, returning 202 until explicit
local approval, then a single-use token/SteamID/install claim. Requests expire
after 120 seconds. The claim returns a random browser pairing secret once.
Unknown/null origins and forged Hosts fail. Reconnects never transmit that secret.

For authenticated GET /v1/snapshot or DELETE /v1/pairing:

1. Let K be the lowercase hexadecimal SHA-256 of the UTF-8 pairing secret, used
   as a UTF-8 HMAC-SHA256 key. POST /v1/challenge sends pairing_id=SHA256(K), a
   fresh random 32-byte base64url client_nonce, method, path and body_sha256.
   Current operations have an empty body and its SHA-256 digest.
2. The server finds the exact origin/account pairing and returns a random
   challenge_id, server_nonce and proof. Challenges expire after 30 seconds,
   are single-use and memory-only, and have a global pending limit of 128.
3. Define context as [origin, challenge_id, client_nonce, server_nonce, method,
   path, body_sha256]. Proofs are lowercase hexadecimal HMAC-SHA256 of UTF-8
   JSON arrays without whitespace. The server proof signs
   ["steam-sage-companion:v1:server", ...context]. Verify it before continuing.
4. Authorization is `SageProof <challenge_id>.<client-proof>`, signing
   ["steam-sage-companion:v1:client", ...context]. The helper checks the current
   pairing again and consumes the challenge on success.
5. The response X-Sage-Proof signs ["steam-sage-companion:v1:response", ...context,
   "<HTTP status>", SHA256(raw response bytes)]. Verify this before parsing or
   applying collection data. CORS exposes only this proof header.

An unsigned 401 cannot establish revocation: preserve the remembered connection
and cached preferences, report an unverifiable/unavailable helper, and offer
reconnection. A process taking the stopped helper's port receives neither K nor
an authorization reusable against a restarted helper.

A snapshot envelope carries protocol, ready/stale/unavailable state, attempted_at,
a fixed error code or null, and a successful snapshot or null. A snapshot contains
SteamID64, opaque source/install IDs, semantic SHA-256 revision, read_at and a
complete list of collection records. Supported collections carry appids;
unsupported collections omit appids. Missing collections in a complete snapshot
mean deletion, while unsupported membership must retain previous knowledge.
Neither timestamp claims Steam Cloud freshness.

Synthetic wire examples are in protocol/fixtures. Sage's matching validator and
fixtures live in its own repository; fixture parity is checked before delivery.
This protocol is a local permission mechanism, not Steam OpenID authentication.

Guided setup adds optional status `capabilities` (at most 16 strings, each at most
64 UTF-8 bytes). Known values are `pairing-ui-v1`, `pairing-cancel-v1`, and
`settings-ui-v1`; missing means alpha.1 manual behavior and bounded unknown names
are ignored. Native UI capabilities are absent when no browser dispatcher is
configured. Snapshot protocol and successful claim shapes remain version 1.

Pair request and HTTP202 pending claim replies may include `approval_ui`:
`manual`, `opening`, `dispatched`, or `failed`. With native dispatch configured,
a newly accepted pair request schedules its focused approval opening automatically
when opening limits permit; otherwise manual approval remains available. Dispatched
means an OS launch request, never proof of a visible or focused browser window.
The original pairing tab and its verifier remain authoritative.

POST /v1/pair/open and POST /v1/pair/cancel take `{request_id, verifier}` and
check exact Origin plus constant-time SHA-256 verifier equality. Open retries a
live request and returns HTTP202 `{state:"opening"}`. Cancel returns HTTP200
`{cancelled:true}` and is idempotent while its receipt lives. Wrong verifier is
403; unknown/expired request is 410. Verified claims after decline/cancel return
410 with `error:"declined"` / `"cancelled"`; ordinary expiry is `"expired"`.
Private local POST /control/decline takes `{request_id}` under the existing
control credential. Nil dispatch uses HTTP202 `{state:"manual"}` for open calls.

Requests live 120 seconds with at most five pending and five new requests per
minute. Reopening does not extend expiry. Native opening attempts have a five
second per-request/per-pair cooldown and a shared five-per-minute limit, including
settings. HTTP429 includes `retry_after_ms`, bounded to 1..60000. In-memory terminal
receipts are capped at 64 for 120 seconds after claim/decline/cancel; reuse never
extends that lifetime. A claim receipt can withdraw only its own issued grant,
preserving older and newer pairings. Receipts and raw verifiers are not persisted.

Authenticated POST /v1/settings/open (empty body) schedules opening and returns
signed HTTP202 `{state:"opening"}`. GET on the same path returns signed current
`{state:<approval UI>}`. Exactly those method/path pairs join the HMAC operation
allowlist; empty-body binding, one-use proofs, and status/raw-response-byte proofs
remain mandatory. Errors after authorization are signed. Opening records are
memory-only, capped at 64 with 120-second lifetimes, bound to pairing and attempt
generation. Late callbacks cannot mutate newer attempts. Clients poll settings
for at most two seconds, then offer manual-opening guidance without asserting
window visibility. Anonymous or unsigned failures cannot clear browser bindings.

Native dispatch callbacks run outside the server mutex. Their private fragment
uses `#launch=<one-use-capability>&request_id=<focused-request-id>` for pairing;
settings use only `#launch=<one-use-capability>`. These URLs and control tokens
are never included in website API responses. Focused pages must remove the
fragment immediately, and cancelled/expired focus must never approve a different
request. A page already dispatched before cancellation may remain open.


## alpha.2 acceptance and compatibility

Guided UI extensions do not change snapshot protocol 1. Alpha.1 omits capabilities
and remains usable via manual controls. A first successful matching source read,
rather than a claim or installation acknowledgement, establishes Sage setup
readiness. Native dispatch must retain exact request focus across separate browser
contexts and stale callbacks. See [release preparation](release-alpha.2.md) for
verification and publication boundaries.
