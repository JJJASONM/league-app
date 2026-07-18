# Users

## Overview

**Owner:** `users`
**Status:** `draft`
**Current version:** `0.3`
**Last reviewed:** `2026-07-18`

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
player linking, roles, permissions, and API access. Until then, player
statistics remain in the standings/player-stats workflows rather than a users
domain screen.

Payment status is currently outside the app. A future users/accounts experience
may show a login reminder or account-status notice when a player has not paid,
but payment tracking is not part of the current auth bridge.

## Roles, Permissions, And API Access

Roles and permissions should be designed after week-end clearance and
season-end clearance are clearer. Those workflows define the protected actions.

Current direction:

- Most administrative workflow actions belong to league or system admins:
  closing weeks, reopening weeks, applying handicaps, generating schedules,
  pushing back schedules, closing seasons, reopening seasons, and managing users.
- Future online score entry should not use a generic scorekeeper role by
  default. Only rostered players assigned to a match should be able to submit or
  edit that match's scores, with admin override.
- API keys remain a bridge for admin/system-style actions. Long-term human use
  should move toward browser login and route-level authorization.
- A route-level authorization matrix is needed before building the users screen
  or online score entry.

## Questions

### USERS-Q001 - Account invitation, roles, and API access

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** An admin may create an account for an existing player. The same
design pass must define roles, permissions, account linking, and API access.

**Resolution:** Define email invitation, credential setup, expiration,
resending, identity verification, player-link confirmation, route-level
authorization, and the long-term replacement or evolution of the current API-key
bridge.

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

### 2026-07-18 - Roles follow clearance workflows

**Status:** `accepted`

Roles and permissions should be designed after week-end and season-end clearance
are documented, because those workflows define the protected actions. Future
score submission should be tied to rostered players assigned to the match rather
than a generic scorekeeper role.

### 2026-06-08 - Separate users and players

**Status:** `accepted`

Authentication and league participation have different lifecycles and must not
share one table.
