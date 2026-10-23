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
