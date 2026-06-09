# Users

## Overview

**Owner:** `users`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-08`

Users are authenticated accounts with roles and permissions. They are separate
from players, who represent league participation and match history.

## Provisional Relationship

```text
users.player_id NULL UNIQUE -> players.id
```

This supports players without accounts and admins who are not players. Review
the design before implementation for household accounts, guardians, shared
email addresses, and account transfers.

## Questions

### USERS-Q001 - Account invitation

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** An admin may create an account for an existing player.

**Resolution:** Define email invitation, credential setup, expiration,
resending, identity verification, and player-link confirmation.

## Decision History

### 2026-06-08 - Separate users and players

**Status:** `accepted`

Authentication and league participation have different lifecycles and must not
share one table.
