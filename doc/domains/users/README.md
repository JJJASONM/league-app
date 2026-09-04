# Users

## Overview

**Owner:** `users`
**Status:** `draft`
**Current version:** `0.12`
**Last reviewed:** `2026-09-03`

Users are authenticated accounts with roles and permissions. They are separate
from players, who represent league participation and match history.

## Player Relationship

```text
users.player_id NULL UNIQUE -> players.id
```

Implemented in Player Account Access Phase 1 (see below) as a nullable
`INTEGER REFERENCES players(id)` column, with uniqueness enforced by a
partial unique index (`WHERE player_id IS NOT NULL`) rather than a `UNIQUE`
column constraint, since SQLite's `ALTER TABLE ADD COLUMN` cannot carry a
`UNIQUE` constraint directly. This still supports players without accounts
and admins who are not players, and still enforces one account per player.
Household accounts, guardians, shared email addresses, and account
transfers remain unreviewed and out of scope for this phase.

## Future User Screens

A future users screen may show account status together with linked player and
statistics context. This belongs after route-level auth is wired and a concrete
users/account-management workflow is defined. Until then, player statistics
remain in the standings/player-stats workflows rather than a users domain
screen.

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

**Status:** `resolved`
**Opened:** `2026-06-08`
**Resolved:** `2026-07-27`

**Resolution:** Admin-provisioned accounts; two-role model (system_admin,
league_admin); API key bridge continues; player link deferred; route auth wires
incrementally per phase onto clearance and operational routes. See USERS-Q001
Discovery section below.

**Update (2026-09-03):** The deferred player link is now implemented as a
third role, `player`, in Player Account Access Phase 1 (see above) --
still on the same API-key bridge, not the browser-session/JWT model this
resolution deferred.

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

- No player-user link (deferred until online score entry, attribution display,
  or a users screen creates the concrete need)
- No session cookies, JWTs, or browser login flow
- No user deactivation endpoint (set `active=0` in DB directly)
- No FK enforcement between `handicap_history.applied_by_user_id` and `users.id`

## Users Auth Phase 1 Implementation

**Status:** `implemented`
**Date:** `2026-07-28`

### What Phase 1 added

- `requirePersonalKeyAuth` middleware: personal-key-only Bearer auth; no
  static-token fallback; rejects with 401 + `WWW-Authenticate` on missing
  header, 403 on unresolved key
- `requireLeagueAdminRole` middleware: reads `*models.User` from
  `clearanceUserKey{}` context; allows `league_admin`, `admin` (backward-compat
  alias), `system_admin`; rejects all other roles with 403
- `clearanceAuth(resolver, h)` conditional wrapper: returns `h` unmodified when
  resolver is nil, preserving existing integration test behavior
- `clearanceUserKey{}` context key (type-safe, separate from `applyUserIDKey{}`)
- `clearanceUserFromContext` helper

### Protected routes (Phase 1)

| Route | Auth requirement |
|-------|-----------------|
| `POST /api/seasons/{id}/weeks/{week}/close` | personal key + league_admin role |
| `POST /api/seasons/{id}/weeks/{week}/reopen` | personal key + league_admin role |
| `POST /api/seasons/{id}/close` | personal key + league_admin role |
| `POST /api/seasons/{id}/reopen` | personal key + league_admin role |

### Clearance auth flow

```text
POST /api/seasons/{id}/weeks/{week}/close  Authorization: Bearer <token>

  1. No header               -> 401 (WWW-Authenticate: Bearer realm="league-admin")
  2. SHA-256(token) matches active user AND role in (league_admin, admin, system_admin)
                             -> allow
  3. SHA-256(token) matches active user AND role not in allowed set
                             -> 403
  4. Token not found in users table (including LEAGUE_ADMIN_TOKEN static token)
                             -> 403
```

### What Phase 1 defers

- No role protection on CRUD mutation routes (leagues, teams, players, seasons,
  roster, rules, schedule generation, pushback, scoresheet)
- Static token (`LEAGUE_ADMIN_TOKEN`) remains in `requireApplyAuth` fallback for
  `handicap-apply` only; not added to any new route
- No login endpoint, browser sessions, or JWTs
- No `system_admin`-gated user-management route protection (POST/GET /api/users
  remain gated by static admin token)

## Users Admin Screen Phase 1 Implementation

**Status:** `implemented`
**Date:** `2026-08-26`

### What this phase added

The first frontend surface for the users domain, plus the backend changes
needed to make it usable by a real system_admin rather than only the
shared static token:

- `POST /api/users` and `GET /api/users` now accept EITHER the static
  `LEAGUE_ADMIN_TOKEN` (kept as a bootstrap path -- something has to be
  able to create the first system_admin user before any personal key
  exists) OR a resolved personal key with `system_admin`/`admin` role, via
  the new `requireAdminTokenOrSystemAdminAuth` middleware. Previously only
  the static token worked, meaning a real system_admin could not use their
  own credentials to manage users at all -- this was the concrete blocking
  gap identified in discovery.
- New `GET /api/users/me`, gated by `requirePersonalKeyAuth` alone (any
  resolvable personal key, no role restriction, no static-token fallback).
  Returns the caller's own username/role/active -- the "who am I" the
  frontend needed and had no way to ask for before.
- `CreateApplyUser` (and the `ApplyAuthResolver.CreateApplyUser` interface)
  now take an explicit `role` parameter instead of hardcoding `role='admin'`
  on every insert. `postUser` validates the requested role against
  `{system_admin, league_admin}` -- `admin` is rejected for new creations,
  remaining valid only as a legacy alias already present on existing rows.
- New `web/domains/users/` domain: a Users screen (`users-management-page`)
  listing existing users and creating new ones with an explicit role
  choice, showing the one-time API key returned at creation (it cannot be
  retrieved again).
- The Admin Key modal now resolves the pasted key to a real identity via
  `GET /api/users/me` and shows "Signed in as `<username>` (`<role>`)" (or
  an explicit "did not resolve" message) instead of only "a key is set."
  The Users nav entry is hidden unless the resolved identity is
  `system_admin`/`admin`.

### Protected routes (this phase)

| Route | Auth requirement |
|-------|-------------------|
| `POST /api/users` | static `LEAGUE_ADMIN_TOKEN`, OR personal key + system_admin/admin role |
| `GET /api/users` | static `LEAGUE_ADMIN_TOKEN`, OR personal key + system_admin/admin role |
| `GET /api/users/me` | any resolvable personal key (no role restriction, no static-token fallback) |

### Intentionally unprotected / unchanged

- `handicap-apply`'s dual-tier `requireApplyAuth` (personal key + static
  token fallback) is unchanged -- explicitly out of scope per PM decision.
- No user deactivate/edit endpoint. `active` remains readable but not
  writable via the API; deferred per PM decision.
- No key rotation endpoint. Unchanged from prior phases.
- No player-facing login, sessions, or JWTs. Unchanged from prior phases.

### What this phase defers

- Player-facing user/profile screen (needs a player auth primitive that
  does not exist yet; explicitly out of scope).
- A dedicated League Admin screen (existing operational domain screens
  already serve league_admin; no separate screen was justified this
  phase).
- A dedicated Developer/Admin tools screen (Backup remains a single
  sidebar button; not enough surface yet to justify consolidation).
- Role constants/central registry (roles remain bare string literals
  across `handlers/api.go`; a real cleanup opportunity, not a blocker).
- Email invitations, password login, browser sessions/JWTs, mobile
  notifications -- all explicitly out of scope per PM decision.

### Verification

`go test ./...` and `go build ./...` pass, including new focused tests
covering: role validation on create (missing role, legacy `admin`
rejected), `system_admin` personal key authorizing create/list,
`league_admin` personal key rejected from create/list (403), and
`GET /api/users/me` for the no-token/static-token/valid-personal-key
cases. Manually verified end to end against a local server build:
bootstrap via static token, create a `system_admin`, use that user's own
personal key (not the static token) to create a second (`league_admin`)
user, confirm the `league_admin` user is rejected from create/list but
can still read its own identity via `/me`. `node --check` on all changed
JS files passes. Actual browser rendering of the new Users screen and
Admin Key modal identity line remain **NOT VERIFIED (no browser)** in
this developer's tool session.

## Player Account Access Phase 1 Implementation

**Status:** `implemented`
**Date:** `2026-09-03`

This is API-key V1 player access, not the final login/session model. It
reuses the same personal-API-key bridge introduced in Phase C1 for a third
role rather than introducing browser sessions, passwords, JWTs, or email
invitations.

### Why this phase exists

Make the app testable as more than an admin console. A player should be
able to use a personal key to view their own schedule, stats, and dues
status through Player Overview, without gaining admin access.

### What Phase 1 added

- `users.player_id INTEGER REFERENCES players(id)`, nullable, NULL for
  every existing `system_admin`/`league_admin` user; a partial unique index
  (`idx_users_player_id ... WHERE player_id IS NOT NULL`) enforces at most
  one user account per player. See "Player Relationship" above.
- `models.User` gained `PlayerID *int64` (`json:"player_id,omitempty"`) and
  a display-only `PlayerName string` (`json:"player_name,omitempty"`,
  populated only by `GET /api/users`'s list query via a `LEFT JOIN players`).
- New `role=player`, creatable alongside `system_admin`/`league_admin`.
  `POST /api/users` requires `player_id` when `role=player`, validates it
  references an existing player, and creates the user via a new
  `ApplyAuthResolver.CreateApplyPlayerUser` method added alongside the
  existing `CreateApplyUser` (rather than changing `CreateApplyUser`'s
  signature, which would have touched all of its pre-existing call sites).
  `system_admin`/`league_admin`/`admin` creation behavior is unchanged.
- `GET /api/users/me` now returns `player_id` for a linked user; for
  unlinked admin-role users, `player_id` remains absent from the JSON
  response (`models.User.PlayerID` is `*int64` with `json:",omitempty"`,
  so a nil pointer is omitted from the response body rather than
  serialized as `null`).
- Player Overview's access rule is now role-aware instead of using the
  existing `clearanceAuth` role allowlist, because ownership can only be
  checked once the URL's player id is parsed: the route now requires only a
  resolvable personal key (`requirePersonalKeyOnly`), and the handler itself
  (`checkPlayerOverviewAccess`) allows `system_admin`/`admin`/`league_admin`
  to view any player's overview unchanged, allows `role=player` to view only
  its own linked player's overview (403 otherwise), and rejects every other
  role. The static `LEAGUE_ADMIN_TOKEN` does not resolve a user at all here,
  so it does not authorize Player Overview.
- Users Admin screen: role select gained a `player` option with a
  conditionally-shown "Linked Player" picker (required when `role=player`);
  the users list gained a "Linked Player" column. No edit, deactivate, or
  key-rotation behavior was added for any role.
- Frontend: a "My Overview" nav entry (visible only to a resolved
  `role=player` identity) opens Player Overview directly on that player's
  own record, with the player-select dropdown hidden as a UX courtesy --
  the actual access control is the server-side check above. This locked
  load also omits `season_id` from the overview request entirely rather
  than passing the shell's currently selected `activeSeason.id` -- the
  shell's selected league/season may belong to a different league than
  the linked player's own, and the backend already falls back to that
  player's own league's active season when `season_id` is omitted (see
  "Correction" below). The existing admin "Player Overview" nav entry and
  the Players-list "View Overview" row button are unchanged (already
  gated to admin roles by an earlier Player Overview Money phase) and
  still pass the shell's `activeSeason.id` when present. A `role=player`
  identity also does not see the Users, Financial, or Backup admin
  surfaces.

### Protected routes (this phase)

| Route | Auth requirement |
|-------|-------------------|
| `GET /api/players/{id}/overview` | personal key required; `system_admin`/`admin`/`league_admin` may view any player, `role=player` may view only its own linked player (403 otherwise) |
| `POST /api/users` (role=player) | same gate as existing role creation: static `LEAGUE_ADMIN_TOKEN`, OR personal key + system_admin/admin role |

### Correction (2026-09-03, same day): "My Overview" ignored the linked player's own league

**PM finding:** `web/app.js` passed `state.activeSeason` into
`<player-overview-page>.refresh(...)` unconditionally, including for a
locked `role=player` view, and the component's `#load()` always sent
`fetchPlayerOverview(playerId, this.#activeSeason?.id)`. If the app
shell's currently selected league/season did not belong to the linked
player's own league, "My Overview" would request
`GET /api/players/{own_id}/overview?season_id={wrong_league_season}` and
the backend would correctly reject it -- making a player-facing entry
point fail depending on whatever league an admin had last selected in
that browser tab.

**Fix:** `#load(forcedPlayerId)` now computes
`seasonId = forcedPlayerId != null ? null : this.#activeSeason?.id` and
passes that to `fetchPlayerOverview` instead of always passing
`this.#activeSeason?.id`. Since `refresh()` already calls
`#load(lockedPlayerId)` for a locked view, the locked path now always
omits `season_id`, letting the backend fall back to the linked player's
own league's active season (`getPlayerOverview`'s existing, documented
behavior -- see `handlers/api_player_overview_handler.go`'s doc comment:
"season_id is optional: when omitted, the player's league's active
season is used"). `fetchPlayerOverview`
(`web/domains/players/players-api-service.js`) needed no change --
`seasonId ? ... : ''` already treats `null` as "omit the query param."
Admin loads (dropdown-driven `#load()` with no argument, including the
Players-list "View Overview" preselect path) are unchanged and still pass
`activeSeason.id` when present. No backend change was needed or made;
the ownership check added earlier this phase is unaffected.

### Intentionally unprotected / unchanged

- `role=player` keys are not accepted anywhere `clearanceAuth`'s
  `requireLeagueAdminRole` or `requireSystemAdminRole` is used (Users,
  Financial/finances, backup, CRUD mutations, week close/reopen, etc.);
  those already reject any role outside their allowlist, so `player` needed
  no explicit new denial there.
- No edit, deactivate, or key-rotation endpoint for any role, including
  `player`. Unchanged from prior phases.

### What Phase 1 defers

- Score submission, captain approval workflows, browser sessions,
  passwords, JWTs, email invitations, and mobile notifications --
  explicitly out of scope per PM decision.
- Household accounts, guardians, shared email addresses, and account
  transfers remain unreviewed (see "Player Relationship" above).
- A dedicated player-facing profile/settings screen beyond Player Overview.

### Verification

New backend tests: `TestApplyAuthStore_CreateApplyPlayerUser_ReturnsLinkedUser`,
`TestApplyAuthStore_Resolve_LinkedPlayerUser_ReturnsPlayerID`,
`TestApplyAuthStore_Resolve_AdminUser_HasNilPlayerID`,
`TestApplyAuthStore_List_ShowsLinkedPlayerName`,
`TestPostUsers_PlayerRoleWithoutPlayerID_Returns400`,
`TestPostUsers_PlayerRoleWithNonexistentPlayerID_Returns400`,
`TestPostUsers_PlayerRoleWithValidPlayerID_Returns201`,
`TestGetMe_PlayerRolePersonalKey_ReturnsPlayerID`,
`TestPlayerOverview_PlayerRole_CanAccessOwnOverview`,
`TestPlayerOverview_PlayerRole_CannotAccessOtherPlayerOverview`,
`TestPlayerOverview_PlayerRole_CannotAccessFinanceRoutes`,
`TestPlayerOverview_PlayerRole_CannotAccessUsersRoutes`. `go test ./...
-count=1` and `go build ./...` pass with zero regressions. Manually verified
end to end against a local server build: player-role creation validation
(missing/invalid/valid `player_id`), `/me` returning `player_id`, own-overview
success, other-player-overview 403, admin access unchanged, Users list
showing the linked player name, and 403 rejection of a player key from both
`GET /api/users` and `POST /api/backup`. `node --check` on all changed JS
files passes. Actual browser rendering of the "My Overview" nav entry and
the Users Admin "Linked Player" picker remain **NOT VERIFIED (no browser)**
in this developer's tool session.

The "My Overview" `season_id` correction above (2026-09-03, same day) is a
frontend-only change -- no Go code changed, so `go test ./... -count=1`
and `go build ./...` were rerun for regression safety only (both pass,
zero regressions), and `node --check` was rerun on
`web/domains/players/player-overview-page-component.js` and `web/app.js`.
Confirmed at the code level (`#load`'s `seasonId` computation and
`fetchPlayerOverview`'s existing `seasonId ? ... : ''` behavior); no local
server was rebuilt for this specific correction since the backend's
season-fallback behavior was already covered by the handler's existing
doc comment and behavior, not new code. Actual browser confirmation that
"My Overview" now succeeds regardless of the shell's selected league
remains **NOT VERIFIED (no browser)**.

## Users Auth Phase 6 Implementation

**Status:** `implemented`
**Date:** `2026-08-08`

### What Phase 6 added

Protected `POST /api/backup` with a stricter role check than Phases 1-5.
Backup is a system-level operation, not league-admin setup work, so it uses
a distinct middleware pair rather than reusing `clearanceAuth`:

- `requireSystemAdminRole` -- allows only `system_admin` and the legacy
  `admin` alias; rejects `league_admin` and `score_keeper`
- `systemAdminAuth` -- composes `requirePersonalKeyAuth` with
  `requireSystemAdminRole`; returns the handler unmodified when the
  resolver is nil, matching the nil-resolver compatibility behavior of
  `clearanceAuth` from Phases 1-5

### Protected route (Phase 6)

| Route | Auth requirement |
|-------|-----------------|
| `POST /api/backup` | personal key + system_admin role (admin alias accepted; league_admin rejected) |

### Key behavioral difference from Phases 1-5

Every route protected in Phases 1-5 allows `league_admin`, `admin`, and
`system_admin`. Backup allows only `system_admin` and `admin` --
`league_admin` receives 403. This is intentional: backup is treated as a
system-admin operation, not an operational league-admin action.

### Discovery findings (script/deploy dependency check)

Before implementing, confirmed no script or frontend code depends on
`POST /api/backup` being unauthenticated:

- `scripts/deploy/staging-common.ps1` (`Backup-StagingDatabase`) copies the
  SQLite file directly (`Copy-Item`) and never calls the HTTP API.
- No file under `web/` references `/api/backup`.
- `QUICKSTART.md` references a "Backup DB" action "in the app," but no such
  UI action exists in the current frontend. This is stale documentation,
  not a live dependency. Deferred as a documentation cleanup item, out of
  scope for this auth phase.

### What Phase 6 defers

- `QUICKSTART.md` backup UI reference cleanup (stale docs, unrelated to
  auth behavior)
- No change to `handicap-apply`, `POST/GET /api/users`, or GET read policy

With Phase 6 complete, all mutation routes identified in the incremental
route-level auth rollout are protected. `handicap-apply` retains its
dual-tier `requireApplyAuth` by design (see Phase C1 above); `POST/GET
/api/users` retained `requireAdminToken`-only auth by design at the time
of this phase. See "Users Admin Screen Phase 1 Implementation" below for
the later change that added personal-key (system_admin/admin) access
alongside the static token.

## Users Auth Phase 5 Implementation

**Status:** `implemented`
**Date:** `2026-08-08`

### What Phase 5 added

Protected 9 global CRUD mutation routes (leagues, players, teams) with the
same `clearanceAuth` middleware chain from Phases 1 through 4 (personal-key-only
Bearer auth + league_admin role). No new middleware or infrastructure required.

### Protected routes (Phase 5)

| Route | Auth requirement |
|-------|-----------------|
| `POST /api/leagues` | personal key + league_admin role |
| `PUT /api/leagues/{id}` | personal key + league_admin role |
| `DELETE /api/leagues/{id}` | personal key + league_admin role |
| `POST /api/players` | personal key + league_admin role |
| `PUT /api/players/{id}` | personal key + league_admin role |
| `DELETE /api/players/{id}` | personal key + league_admin role |
| `POST /api/teams` | personal key + league_admin role |
| `PUT /api/teams/{id}` | personal key + league_admin role |
| `DELETE /api/teams/{id}` | personal key + league_admin role |

### Intentionally unprotected (Phase 5)

- `GET /api/leagues`, `GET /api/leagues/{id}`, `GET /api/players`,
  `GET /api/players/{id}`, `GET /api/teams`, `GET /api/teams/{id}` -- GET reads
  are public
- `POST /api/backup` -- deferred to a separate system-admin phase
- `POST /api/seasons/{id}/handicap-apply` -- retains its existing dual-tier
  `requireApplyAuth` (personal key + static token fallback); no change
- `POST /api/users`, `GET /api/users` -- retain `requireAdminToken`; no change

### What Phase 5 defers

- `POST /api/backup` role protection (system-admin-only phase, not yet scoped)
- Any change to the `handicap-apply` static-token fallback (deferred to a
  focused attribution/auth cleanup phase per the 2026-08-08 architecture
  review roadmap alignment)

With Phase 5 complete, all admin mutation routes covered by the incremental
route-level auth rollout are protected except `POST /api/backup` and the
`handicap-apply` static-token bridge.

## Users Auth Phase 4 Implementation

**Status:** `implemented`
**Date:** `2026-08-07`

### What Phase 4 added

Protected 19 season-setup mutation routes with the same `clearanceAuth` middleware
chain from Phases 1 through 3 (personal-key-only Bearer auth + league_admin role).
No new middleware or infrastructure required.

### Protected routes (Phase 4)

| Route | Auth requirement |
|-------|-----------------|
| `POST /api/seasons` | personal key + league_admin role |
| `PUT /api/seasons/{id}` | personal key + league_admin role |
| `DELETE /api/seasons/{id}` | personal key + league_admin role |
| `POST /api/seasons/{id}/activate` | personal key + league_admin role |
| `POST /api/seasons/{id}/rules` | personal key + league_admin role |
| `PUT /api/seasons/{id}/rules/{rid}` | personal key + league_admin role |
| `DELETE /api/seasons/{id}/rules/{rid}` | personal key + league_admin role |
| `POST /api/seasons/{id}/skipped-weeks` | personal key + league_admin role |
| `DELETE /api/seasons/{id}/skipped-weeks/{sid}` | personal key + league_admin role |
| `POST /api/seasons/{id}/bye-requests` | personal key + league_admin role |
| `PUT /api/seasons/{id}/bye-requests/{bid}` | personal key + league_admin role |
| `DELETE /api/seasons/{id}/bye-requests/{bid}` | personal key + league_admin role |
| `POST /api/seasons/{id}/teams` | personal key + league_admin role |
| `PUT /api/seasons/{id}/teams/{tid}` | personal key + league_admin role |
| `DELETE /api/seasons/{id}/teams/{tid}` | personal key + league_admin role |
| `POST /api/seasons/{id}/teams/{tid}/roster` | personal key + league_admin role |
| `DELETE /api/seasons/{id}/teams/{tid}/roster/{pid}` | personal key + league_admin role |
| `POST /api/lineup-plans` | personal key + league_admin role |
| `DELETE /api/lineup-plans/{id}` | personal key + league_admin role |

### Intentionally unprotected (Phase 4)

All GET reads on season and lineup routes carry no auth (policy: GET reads are public).

### What Phase 4 defers

- No role protection on CRUD mutation routes for leagues, teams, and players
  (`POST/PUT/DELETE /api/leagues`, `/api/teams`, `/api/players`)
- `POST /api/backup` (system operation, deferred to a later phase)
- `POST /api/seasons/{id}/handicap-apply` retains its existing dual-tier
  `requireApplyAuth` (personal key + static token fallback); no change

## Users Auth Phase 3 Implementation

**Status:** `implemented`
**Date:** `2026-08-07`

### What Phase 3 added

Protected four match mutation routes with the same `clearanceAuth` middleware
chain from Phases 1 and 2 (personal-key-only Bearer auth + league_admin role).
No new middleware or infrastructure required.

### Protected routes (Phase 3)

| Route | Auth requirement |
|-------|-----------------|
| `PATCH /api/matches/{id}/assign` | personal key + league_admin role |
| `POST /api/matches/{id}/results` | personal key + league_admin role |
| `DELETE /api/matches/{id}/results` | personal key + league_admin role |
| `POST /api/matches/{id}/rounds` | personal key + league_admin role |

### Intentionally unprotected (Phase 3)

Read-only match routes carry no auth (policy: GET reads are public):
`GET /api/matches`, `GET /api/matches/{id}`, `GET /api/matches/{id}/rounds`,
`GET /api/standings`, `GET /api/player-stats`.

### What Phase 3 defers

- No role protection on remaining CRUD/setup mutation routes (leagues, teams,
  players, seasons, rules, skipped-weeks, bye-requests, roster, season
  activation, lineup plans, season setup, handicap apply)

## Users Auth Phase 2 Implementation

**Status:** `implemented`
**Date:** `2026-08-07`

### What Phase 2 added

Protected two schedule mutation routes with the same `clearanceAuth` middleware
chain introduced in Phase 1 (personal-key-only Bearer auth + league_admin role).
No new middleware or infrastructure required.

### Protected routes (Phase 2)

| Route | Auth requirement |
|-------|-----------------|
| `POST /api/matches/generate` | personal key + league_admin role |
| `POST /api/seasons/{id}/schedule/pushback-apply` | personal key + league_admin role |

### Intentionally unprotected (Phase 2)

`POST /api/seasons/{id}/schedule/pushback-preview` uses POST because it accepts
a request body (cutoff week and shift amount), but it performs no state mutation.
It is intentionally left unprotected so admins and tooling can preview the impact
of a pushback without an API key.

### What Phase 2 defers

- No role protection on CRUD mutation routes (leagues, teams, players, seasons,
  roster, rules, skipped-weeks, bye-requests, season activation, match assignment,
  scoresheet save)
- Static token (`LEAGUE_ADMIN_TOKEN`) continues as fallback for `handicap-apply`
  only; no static-token path added to Phase 2 routes

## USERS-Q001 Discovery

**Status:** `resolved`
**Date:** `2026-07-27`

### Current-State Inventory (as of 2026-07-27)

**Schema:**
- `users` table: `id`, `username` (UNIQUE), `api_key_hash` (SHA-256, 64-char hex, UNIQUE), `role` (DEFAULT 'admin'), `active`, `created_at`
- `player_id` intentionally omitted in C1; optional link deferred to this resolution
- `handicap_history.applied_by_user_id INTEGER` -- attribution column; no FK enforced

**Routes:**
- `POST /api/users` -- create user, return one-time cleartext key; gated by static admin token
- `GET /api/users` -- list users without hashes; gated by static admin token
- `POST /api/seasons/{id}/handicap-apply` -- gated by dual-tier `requireApplyAuth`

**Apply auth flow (C1):**

```text
No header                              -> 401 (WWW-Authenticate)
SHA-256(token) matches active user     -> allow; applied_by_user_id = users.id
token == LEAGUE_ADMIN_TOKEN            -> allow; applied_by_user_id = NULL; logs deprecation
Neither                                -> 403
```

**Unprotected routes as of 2026-07-27:** All season, match, schedule, scoresheet,
lineup, standings, and CRUD mutation routes carry no authorization.

### Proposed Account Model

**Role taxonomy - two roles for this phase:**

- `system_admin` -- manage leagues, manage users, global settings
- `league_admin` -- operational: close/reopen weeks, apply handicaps, close/reopen seasons, season setup

The current schema stores `role TEXT NOT NULL DEFAULT 'admin'`. Existing users should
be treated as `league_admin` until a migration aligns stored values with this taxonomy.
Reserve `score_keeper` for future online score entry (MATCHES-Q002); do not define it
until that workflow is designed, as its scope is tied to rostered players on a specific
match.

**Player-user link:** `users.player_id NULL UNIQUE` remains the approved target.
Defer until a concrete workflow requires it: online score entry, attribution display,
or a future users screen. Review before implementation for household accounts,
guardians, shared emails, and account transfers.

**Deferred items:**
- Browser sessions and JWTs -- until online score entry or a users screen requires it
- User deactivation endpoint -- set `active=0` in DB directly; endpoint is low priority
- Personal API key rotation (`POST /api/users/{id}/rotate-key`) -- deferred
- `applied_by_user_id` FK enforcement -- column exists; FK not enforced

### Roles and Permissions Matrix

| Route group | Current auth | Target auth |
|-------------|-------------|-------------|
| GET reads (all domains) | None | None |
| Mutation: leagues, teams, players CRUD | None | league_admin |
| Mutation: seasons, rules, skipped-weeks, bye-requests, season teams, roster | None | league_admin |
| POST /api/matches/generate | None | league_admin |
| POST /api/seasons/{id}/schedule/pushback-* | None | league_admin |
| POST /api/seasons/{id}/weeks/{week}/close | None | league_admin |
| POST /api/seasons/{id}/weeks/{week}/reopen | None | league_admin |
| POST /api/seasons/{id}/close | None | league_admin |
| POST /api/seasons/{id}/reopen | None | league_admin |
| POST /api/seasons/{id}/handicap-apply | requireApplyAuth | league_admin (no mechanism change) |
| POST /api/matches/{id}/results and /rounds | None | league_admin (or future score_keeper via MATCHES-Q002) |
| POST /api/users, GET /api/users | requireAdminToken | system_admin |
| POST /api/backup | None | system_admin |

Route auth is not wired for most routes as of this resolution. Wire incrementally
per phase as workflows are hardened. The matrix records intent, not current state.

### Invitation and API Bridge Decision

**Provisioning:** Admin-provisioned only. No email invitation workflow at this time.
An admin with `LEAGUE_ADMIN_TOKEN` creates accounts via `POST /api/users`. The
cleartext key is delivered out-of-band in the create response.

**Static token deprecation path:**
- Keep as fallback in `requireApplyAuth`; the deprecation log on each use is sufficient
- Do not add the static token path to newly protected routes; use personal keys + role check
- Remove the static token fallback only after all affected admins have personal keys
  and affected routes are confirmed working with personal keys

**Key management:** No key rotation endpoint in the near roadmap. Direct DB
intervention for now. Add `POST /api/users/{id}/rotate-key` only when operationally
urgent.

### API Access Transition Recommendation

**Next incremental steps:**

1. Wire `requireApplyAuth`-equivalent middleware onto clearance routes (close/reopen
   week, close/reopen season). No new infrastructure required; the middleware and
   user-from-context pattern already exist.
2. Add a `RequireRole(role string)` helper that reads the resolved user from context
   and checks `users.role`. Wire `RequireRole("league_admin")` after auth on each
   newly protected route.

**Not next:** Browser sessions, JWTs, a login endpoint, email invitations, or
permission scoping by league. Defer until online score entry or a users management
screen creates the concrete need.

## Decision History

### 2026-07-18 - Roles follow clearance workflows

**Status:** `accepted`

Roles and permissions should be designed after week-end and season-end clearance
are documented, because those workflows define the protected actions. Future
score submission should be tied to rostered players assigned to the match rather
than a generic scorekeeper role.

### 2026-07-27 - USERS-Q001 resolved: roles, permissions, and API access

**Status:** `accepted`

Week-end and season-end clearance are now stable. Roles, permissions, and API
access are resolved at the design level. Implementation proceeds incrementally
per route phase. See USERS-Q001 Discovery section.

### 2026-06-08 - Separate users and players

**Status:** `accepted`

Authentication and league participation have different lifecycles and must not
share one table.

### 2026-08-26 - Users Admin Screen Phase 1: first users domain screen

**Status:** `accepted`

Added the first frontend surface for this domain (list + create users with
an explicit role) and the backend change needed to make it usable by a real
system_admin (personal-key access alongside the static token on POST/GET
/api/users), plus a `GET /api/users/me` identity-resolution endpoint used by
the Admin Key modal. New users may only be created as `system_admin` or
`league_admin`; `admin` remains a legacy alias on existing rows only.
Deactivate/edit, key rotation, email invitations, and player-facing
login/profile remain deferred. Building this screen is the condition line
555-557 above named as the trigger to revisit browser sessions/JWTs -- that
revisit was explicitly declined for this phase (PM decision): personal API
keys remain the mechanism, and no login endpoint or session model was
introduced. See "Users Admin Screen Phase 1 Implementation" above for full
detail.

### 2026-09-03 - Player Account Access Phase 1: player-linked accounts

**Status:** `accepted`

Added a third role, `role=player`, linked one-to-one (enforced via a
partial unique index) to a `players` row via a new nullable `users.player_id`
column. A player-role personal key can view only its own linked player's
Player Overview (schedule, stats, dues); it is rejected everywhere the
existing league_admin/system_admin allowlists already apply (Users,
Financial, backup, CRUD mutations, clearance routes). Explicitly declared
as API-key V1 player access, not the final login/session model -- score
submission, captain approval, browser sessions, passwords, JWTs, email
invitations, and mobile notifications remain out of scope. See "Player
Account Access Phase 1 Implementation" above for full detail.

### 2026-09-03 - Correction: "My Overview" must ignore the shell's selected league

**Status:** `accepted`

PM review found "My Overview" could 403 depending on whatever
league/season the app shell happened to have selected, because the
locked load path passed the shell's `activeSeason.id` through to the
overview request just like the admin path does. Fixed by omitting
`season_id` entirely for a locked (`role=player`) load, letting the
backend's existing fallback (the linked player's own league's active
season) apply instead. Admin behavior is unchanged. See "Correction
(2026-09-03, same day)" under "Player Account Access Phase 1
Implementation" above for full detail.
