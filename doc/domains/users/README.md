# Users

## Overview

**Owner:** `users`
**Status:** `draft`
**Current version:** `0.2`
**Last reviewed:** `2026-07-14`

Users are authenticated accounts with roles and permissions. They are separate
from players, who represent league participation and match history.

## Provisional Relationship

```text
users.player_id NULL UNIQUE -> players.id
```

This supports players without accounts and admins who are not players. Review
the design before implementation for household accounts, guardians, shared
email addresses, and account transfers.

## Future User Screens

A future users screen may show account status together with linked player and
statistics context. This belongs after `USERS-Q001` defines account invitation,
player linking, roles, and permissions. Until then, player statistics remain in
the standings/player-stats workflows rather than a users domain screen.

## Questions

### USERS-Q001 - Account invitation

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** An admin may create an account for an existing player.

**Resolution:** Define email invitation, credential setup, expiration,
resending, identity verification, and player-link confirmation.

## Phase C1 Implementation

**Status:** `implemented`
**Date:** `2026-06-30`

### What C1 added

- `users` table (`id`, `username`, `api_key_hash`, `role`, `active`, `created_at`)
- SHA-256 API key hashing — cleartext returned once at create, never stored
- `ApplyAuthStore` — purpose-built resolver, not a generic user store
- Dual-tier Apply auth: personal key (attributed) → `LEAGUE_ADMIN_TOKEN` fallback (unattributed)
- `POST /api/users` — create user, return one-time cleartext key (gated by admin token)
- `GET /api/users` — list users without hashes (gated by admin token)
- `handicap_history.applied_by_user_id` set to `users.id` on personal-key path; NULL on static-token path

### Apply auth flow

```
POST /api/seasons/{id}/handicap-apply  Authorization: Bearer <token>

  1. No header            → 401 (WWW-Authenticate)
  2. SHA-256(token) matches users.api_key_hash AND active=1
                          → allow; applied_by_user_id = users.id
  3. token == LEAGUE_ADMIN_TOKEN
                          → allow; applied_by_user_id = NULL; logs deprecation
  4. Neither              → 403
```

### What C1 defers

- No player↔user link (deferred to USERS-Q001 below)
- No session cookies, JWTs, or browser login flow
- No user deactivation endpoint (set `active=0` in DB directly)
- No FK enforcement between `handicap_history.applied_by_user_id` and `users.id`

## Decision History

### 2026-06-08 - Separate users and players

**Status:** `accepted`

Authentication and league participation have different lifecycles and must not
share one table.
