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

## Users/Roles/Authentication Phase 1 Implementation (2026-09-18)

**Confirmed product decisions this phase implements:** email is the sole
password-login identifier (username remains, legacy-only, never accepted
by the login endpoint); passwords are chosen by the user through a
one-time system-admin-issued setup token, not self-registration; one
account may carry multiple scoped roles; system_admin is global;
league_admin is scoped to one or more leagues; player access comes from
`users.player_id`; a user may be both player and league_admin, choosing
Player View or Admin View after login and switching later without
signing out (presentation only -- backend authorization always evaluates
the full role/scope set); a league_admin may create a league, which
atomically grants the creator league_admin access to the new league
without auto-granting any other existing league_admin; system_admin may
assign or remove league-admin scopes; no captain role; existing players
without accounts and existing personal API keys both remain fully
valid.

**Schema (all additive except one guarded rebuild):**

- `users` gains `email` (nullable, normalized, partial-unique),
  `password_hash`, `password_updated_at`, `must_reset_password`,
  `updated_at`. The pre-existing `api_key_hash TEXT NOT NULL UNIQUE`
  column is removed via a one-time, idempotent, guarded table rebuild
  (`db.migrateUsersAndAPIKeys`) -- SQLite cannot relax a NOT NULL/UNIQUE
  constraint via ALTER TABLE, and that constraint made a password-only
  user (no API key at all) impossible to represent. The rebuild
  preserves every existing id, the AUTOINCREMENT sequence, and every
  other column exactly; a fresh install never has the old column at all
  and skips the rebuild entirely (see the guard in
  `db.migrateUsersAndAPIKeys`'s doc comment for the exact sequencing
  rationale -- a stash-then-rebuild-then-populate order specifically
  chosen so an already-existing empty `user_api_keys`/`role_assignments`
  table's `ON DELETE CASCADE` FK can never fire against real data during
  the rebuild's `DROP TABLE users` step).
- New `user_api_keys` table replaces `users.api_key_hash` as the source
  of truth for API-key credentials -- "no active key" is now just zero
  non-revoked rows, not a sentinel value. `ApplyAuthStore` (the existing
  personal-API-key resolver/creator for the Apply flow) was updated to
  read/write this table instead of the old column; its external
  behavior and every existing caller/test are unchanged.
- New `role_assignments` table: `role_code` (`system_admin` |
  `league_admin`), `league_id` (nullable). A CHECK constraint --
  enforceable here because this is a brand-new table, unlike the
  ALTER-TABLE-limited `users` rebuild above -- proves at the database
  layer, not just in application code, that `system_admin` always has
  `league_id IS NULL`, `league_admin` always has `league_id IS NOT
  NULL`, and `role_code='player'` can never be inserted at all (a player
  identity is `users.player_id`, never a role_assignments row). Two
  partial unique indexes prove duplicate assignments are impossible.
  `league_id ... ON DELETE CASCADE` removes a league's scoped
  assignments when it is deleted, using the same pool-wide
  `PRAGMA foreign_keys` enforcement fixed in
  `sqlite-foreign-key-cascade-enforcement` (see
  `doc/architecture-decisions.md`).
- New `sessions` table (session token hash, CSRF token hash, timestamps,
  revocation) and `password_setup_tokens` table (single-use,
  time-limited, revocable).
- Legacy role backfill (part of the same guarded rebuild, for real
  pre-existing data): `role IN ('system_admin','admin')` -> one global
  `system_admin` row; `role='league_admin'` -> one `league_admin` row
  PER LEAGUE THAT EXISTED AT MIGRATION TIME (there is no nullable
  "applies to all leagues" equivalent once the CHECK constraint requires
  a concrete league for that role) -- a real, disclosed behavior
  change: a league created AFTER migration is not automatically visible
  to a grandfathered admin; a system_admin must explicitly extend scope
  to it. `role='player'` gets no row (unaffected, uses `player_id`).
  The same backfill logic applies going forward when a NEW system_admin/
  admin user is created via the legacy `POST /api/users` bootstrap
  endpoint (global row only -- that endpoint has no concept of league
  scope, so a `league_admin` created there gets no scoped access until a
  system_admin grants one via the new roles endpoint).

**Password hashing:** Argon2id via `golang.org/x/crypto/argon2`
(`backend/domains/auth/password.go`) -- versioned, self-describing
encoded hashes (`$argon2id$v=19$m=...,t=...,p=...$salt$hash`), constant-
time verification, a strict parser that rejects any encoded hash with
out-of-range parameters or malformed structure (never lets a corrupted
or tampered stored value drive an expensive computation with
attacker-influenced cost), and rehash-on-login when stored parameters
fall below the current target. Parameters are never hardcoded:
`auth.CalibrateArgon2` measures real hash duration on the actual
deployment hardware at startup and selects the smallest iteration count
clearing a 300ms target. Measured on the primary development machine:
memory=65536KiB (64MiB), iterations=19, parallelism=2, measured
358.1102ms.

**Sessions and CSRF:** server-managed sessions (not JWT -- this app is a
single-process monolith with one SQLite database already handling all
durable state; a sessions table mirrors the existing API-key hashing
pattern and gives real, instant revocation for free, which JWT would
need its own blocklist to match anyway). 7-day idle timeout, refreshed
on every authenticated request, capped by a 30-day absolute lifetime
measured from session creation. CSRF: a session-bound, server-verified
double-submit cookie -- a second random value generated at login, its
hash stored on the session row, exposed to the browser as a second,
non-HttpOnly cookie the shared frontend API client (`web/lib/
api-client.js`) reads and mirrors into an `X-CSRF-Token` header on every
session-cookie-authenticated mutating request; Bearer-key requests never
attach or need it (inherently CSRF-immune -- a browser cannot attach a
custom header to a cross-site request the way it attaches an ambient
cookie). `INSECURE_LOCAL_COOKIES=1` is an explicit, startup-logged
opt-in that omits the cookies' `Secure` attribute for same-computer HTTP
testing only; it is never inferred from request headers.

**Authorization:** one centralized policy entry point,
`auth.Authorize(identity, action, scope)`
(`backend/domains/auth/authz.go`) -- system_admin is allowed
everywhere; league-scoped actions require system_admin or league_admin
for that exact league; league creation requires system_admin or being a
league_admin for at least one existing league already (resolving the
chicken-and-egg problem: a brand-new admin's first league_admin grant
must come from a system_admin against an already-existing league,
after which they can create additional leagues on their own, each
atomically self-granting scope for the new one only). This phase wires
`Authorize` into: `POST/PUT/DELETE /api/leagues` (league create/
update/delete, scope = the league itself) and `POST/PUT/DELETE
/api/seasons` (season create/update/delete, scope = the season's
league, resolved via its own `league_id` on create or a lookup on
update/delete) -- both ONLY when the new auth domain is wired
(`Dependencies.AuthMgr`/`RoleAssignmentMgr` non-nil), so every existing
test and any deployment that has not wired it keeps the exact prior
flat-`clearanceAuth` behavior. Every other existing `clearanceAuth`
route (matches/rounds/approval/processing, finances, handicap apply,
lineup plans, schedule generation/pushback, week close/reopen, season
rules/skipped-weeks/bye-requests/season-teams/roster, and direct
players/teams CRUD) is UNCHANGED in this phase -- still flat,
unscoped, exactly as before. Migrating those is the next slice of this
work, not yet scheduled.

**Endpoints added:** `POST /api/auth/login`, `POST /api/auth/logout`,
`GET /api/auth/me`, `POST /api/auth/password-setup`, and
system-admin-only account administration under `/api/auth/admin/
users/...` (provision, issue setup token, deactivate/reactivate, revoke
API keys, list/grant/revoke role assignments).

**Frontend:** a real login screen (`<login-page>`,
`web/domains/auth/`) replaces the app shell for an unauthenticated
visitor; the existing Admin Key modal remains reachable as a secondary,
no-longer-default path once signed in (or for bootstrap before any
account exists). A workspace picker appears only for an identity with
more than one available workspace and toggles nav visibility between
Admin View (Users, Backup, league-admin workflows, league selection --
gated more strictly now to system_admin/legacy-admin only for Users/
Backup, matching what the backend actually enforces, rather than the
previous "visible to anyone with any admin-ish role" approximation) and
Player View (My Overview only). The Users Admin screen gained a
"Provision Email Login" flow and a per-account row menu (issue setup
token, deactivate/reactivate, revoke API keys); scoped role
grant/revoke is not yet surfaced in this UI (use the API directly).

**Deliberately deferred, not oversights:** self-registration and
automatic player-email matching, pending player-link review, email
verification/delivery of any kind, the captain role, two-team player
score approval, and migrating the remaining legacy `clearanceAuth`
routes onto `Authorize`. League creation's atomic self-grant is a
compensating action (create league, then grant; on grant failure,
delete the just-created league), not a single database transaction --
`LeagueService`/`LeagueStore` and the role-assignment store are
separate domains by this codebase's own convention, and a real
cross-domain transaction helper does not exist yet; this is disclosed
here rather than silently assumed.

**Verified with:** `backend/domains/auth`'s unit test suite (password
hashing round-trip/tamper-rejection/calibration, email normalization,
the full `Authorize` matrix, `SessionService` against in-memory fakes);
`backend/storage/sqlite`'s auth store tests against a real database
(including a CHECK-constraint-rejection test and a league-deletion
cascade test using the same forced-non-init-connection technique
`sqlite-foreign-key-cascade-enforcement` established); a dedicated
migration test (`db/migrate_users_api_keys_test.go`) seeded from the
exact literal pre-Phase-1 schema, proving id/autoincrement preservation,
exactly-once key migration, correct role backfill, and idempotency; and
a full `handlers`-level integration test suite
(`api_auth_integration_test.go`) exercising login/logout/CSRF
enforcement/league-scoped authorization/the atomic self-grant/player-
denied-admin-routes/deactivation/legacy-API-key-compatibility against a
real HTTP server. See this phase's handoff for full command output.
Browser rendering of the new login screen and workspace switcher was
NOT VERIFIED (no browser in this developer's tool session) -- every
claim above is proven at the API/handler level.

## Users/Roles/Authentication Phase 1 -- PM correction round (2026-09-19)

PM review of the initial Phase 1 handoff found the phase operationally
incomplete: several claims above did not match actual route behavior, and
two schema/atomicity concerns needed real fixes rather than disclosure.
This section corrects those claims; the schema/password/session/CSRF/
authorization-model text above otherwise still stands.

**Corrected claim -- session auth now works everywhere, not only new
routes.** The original claim that "every other existing `clearanceAuth`
route ... is UNCHANGED in this phase -- still flat, unscoped" meant a
password-authenticated league_admin or system_admin could not use most
existing admin workflows at all (session cookies were never accepted by
`clearanceAuth`, only Bearer keys). This is fixed: every mutation route
that previously used `clearanceAuth`/`systemAdminAuth` now goes through
`guardedLeagueAdminAction`/`guardedSystemAdminAction`
(`handlers/api_auth_middleware.go`), which accepts a session cookie or a
Bearer API key identically and routes both through the same
`auth.Authorize` call -- covering players, teams, complete season setup
(activation, rules, breaks, bye-requests, season-teams, roster), schedule
generation and pushback, lineup/substitute mutations, match assignment/
score entry/approval/processing, week close/reopen, finances, handicap
apply, backup, and user administration. Player Overview
(`GET /api/players/{id}/overview`) is covered too -- see below. Bearer
keys remain exempt from CSRF (unchanged); session mutations still require
the `X-CSRF-Token` header. `clearanceAuth`/`systemAdminAuth` and their
role-string checks are no longer used by any route registration -- they
remain only as directly-unit-tested primitives and as
`guardedAction`'s fallback for Dependencies configurations that never
wire `RoleAssignmentMgr` at all (test-only; every real deployment wires
it in `main.go`), so there is exactly one authorization policy for real
traffic, not two running in parallel.

**Corrected claim -- league scope is now enforced on every league-owned
route family**, not only league/season CRUD. Each route resolves its
authoritative league server-side from its own resource --
`backend/handlers/api_scope_resolvers.go` adds resolvers for team_id,
player_id (falling back to a client-supplied `league_id` only for a
not-yet-rostered player, who has no other owner to look up), match_id (via
season), and lineup_plans id (via season) -- never trusting a
client-supplied `league_id` once an existing resource can determine its
own ownership. A League A administrator can no longer read or mutate
League B's players, teams, matches, lineups, finances, or schedule.

**Corrected claim -- Player Overview session access.** The original
Player Overview route was personal-key-only (`requirePersonalKeyOnly`), so
a player who logged in with email/password could not load the one screen
Player View offers. `GET /api/players/{id}/overview` now accepts a
session or a Bearer key via `playerOverviewAuth`
(`handlers/api_player_overview_routes.go`), enforcing: a linked player may
view only their own overview; a league_admin may view players in their
own assigned league(s) only; system_admin may view any player.

**Corrected claim -- migration ordering.** The original text already
described stash-then-rebuild-then-populate for `user_api_keys`, but the
actual code created `user_api_keys`/`role_assignments`/`sessions`/
`password_setup_tokens` in the base schema string, which ran BEFORE
`migrateUsersAndAPIKeys` on a legacy database -- i.e. before the `users`
rebuild, not after, contradicting the stated order (harmless only because
those tables were still empty at that exact moment; not a real
guarantee). `db.go` now creates all four of these tables from a single
`authChildTablesSchema` constant, executed only after `users` has its
final shape -- either because `migrateUsersAndAPIKeys` already rebuilt it
(inside the same transaction, right after the rename), or because a fresh
install creates `users` with its final shape from the start. Regression
test: `TestMigrateUsersAndAPIKeys_FromPrePhaseSchema` (updated) plus a new
`TestMigrateUsersAndAPIKeys_PreservesHistoricalSequenceAfterHighIDDeletion`
in `db/migrate_users_api_keys_test.go`.

**Corrected claim -- AUTOINCREMENT preservation.** Copying only
currently-existing rows' ids into the rebuilt `users` table does not
preserve `sqlite_sequence`'s historical high-water mark if the
highest-id row had been deleted before migration ran. `db.go`'s rebuild
now reads the old table's `sqlite_sequence` value before dropping it and
restores at least that value onto the renamed table afterward, so a
newly inserted user can never reuse an id that belonged to any historical
user, deleted or not.

**Corrected claim -- league creation atomicity.** "League creation
atomically grants the creator league_admin access" was previously a
compensating action (create, then grant, then delete on failure), not a
real transaction. `sqlite.LeagueSelfGrantStore.CreateLeagueWithLeagueAdminGrant`
(`backend/storage/sqlite/league_self_grant_store.go`) now performs both
inserts in a single `*sql.Tx`, committed or rolled back together -- a
narrowly-scoped, deliberate exception to the LeagueService/
RoleAssignmentStore domain-separation convention for this one workflow,
not a precedent for merging the two domains generally.

**Added -- usable password setup and Admin Key access from the login
screen.** The login screen (`web/domains/auth/login-page-component.js`)
now has a setup mode (setup token, new password, confirm password,
calling `POST /api/auth/password-setup`, honest validation, returns to
sign-in on success) and a "Use Admin Key instead" link that dispatches a
`use-admin-key-requested` event the shell (`web/app.js`) already handles
by opening the existing, unchanged Admin Key modal -- previously that
modal's only trigger button lived inside the hidden app shell, so a
browser with neither a session nor a stored key could never reach it.

**Verified with:** the full existing suite above, plus a new
`handlers/api_auth_scoped_families_test.go` proving representative
session-authenticated routes across every corrected route family
(players, teams, match assign, lineup save, finances read/write,
schedule generate, week close, season activate/rules) succeed for a
same-league league_admin and are rejected (403) across leagues; a new
`TestAuthIntegration_PlayerOverview_SessionAccess` proving own/other/
scoped-admin/cross-league-admin Player Overview access; a new
`TestAuthIntegration_PasswordSetup_FullFlow` exercising issue-token ->
redeem-token -> log-in-with-new-password end to end over real HTTP, plus
same-token replay rejection; and three new
`backend/storage/sqlite/league_self_grant_store_test.go` tests (success,
forced-failure rollback via an FK violation, no auto-grant to an
unrelated admin). Browser rendering of the setup mode and Admin Key link
was NOT VERIFIED (no browser in this developer's tool session); a
server-level smoke test (binary built and run, static assets and
`POST /api/auth/login` confirmed reachable via curl) was performed
instead.

## Users/Roles/Authentication Phase 1 -- PM final authorization corrections (round 2, 2026-09-19)

A second PM review, after the round above was accepted in direction,
found five remaining, more subtle correctness issues: authorizing the
primary path/body resource was not enough when a mutation could attach
or replace a RELATED resource from elsewhere in the request; the
unassigned-player fallback in `playerIDPathScope` could read a request
body that did not exist (DELETE) or meant something else (merge's
`target_id`); `guardedAction`'s fully-open fallback triggered on
`ApplyAuth == nil` alone, silently exposing a session-only auth
configuration; the Apply route's mounting was still gated on the static
token alone; and the `sqlite_sequence` restoration only handled a
rebuild that copied at least one row.

**Related-resource cross-league bypasses closed.** Authorizing a route's
primary resource does not stop a request from attaching or moving a
SECOND resource into or out of a league the actor does not administer.
Fixed per resource family, all in `handlers/api_scope_resolvers.go`
unless noted:
- **Player creation:** `createPlayerScope` now resolves the authoritative
  league from `body.team_id`'s OWN league when a team is supplied --
  `body.league_id` is never trusted for authorization once a real team
  identifies one. `createPlayer` (`handlers/api_players_handlers.go`)
  separately rejects a `body.league_id` that disagrees with that team's
  real league (409, a data-integrity check, not an authorization
  decision). A create with no `team_id` at all (a player with no
  persisted ownership whatsoever) is system_admin-only.
- **Player update:** `updatePlayerScope` resolves the player's origin
  league, and, when the body also supplies a `team_id`, the destination
  team's real league -- a same-league team change is authorized normally;
  a cross-league move (or any move starting from a currently unassigned
  player) is system_admin-only.
- **Player merge:** `mergePlayerScope` resolves BOTH the source (path
  `{id}`) and the target (`body.target_id`) players -- a league_admin may
  merge only when both resolve to the same league they administer; a
  cross-league merge, or one touching an unowned player on either side,
  is system_admin-only.
- **Match team assignment:** `MatchService.AssignMatchTeams`
  (`backend/domains/matches/match_service.go`) now validates, at the
  authoritative backend service boundary (not only the frontend), that
  every non-null `home_team_id`/`away_team_id` belongs to the match's own
  season's league -- via two new `MatchStore` methods, `SeasonLeagueID`
  and `TeamLeagueID`. A mismatch returns `domainerr.Conflict`
  (`MATCH_ASSIGN_CROSS_LEAGUE`, HTTP 409), not a silently-accepted
  cross-league assignment.
- **Lineup save:** `LineupService.SaveTeamLineup`
  (`backend/domains/matches/lineup_service.go`) now validates that
  `req.TeamID` belongs to `req.SeasonID`'s own league via two new
  `LineupStore` methods (`SeasonInfo`, `TeamLeagueID`), and, for a
  `teams_managed` season, that the team is actually registered in
  `season_teams` (`TeamParticipatesInSeason`) -- a legacy
  (non-`teams_managed`) season skips the participation check, matching
  existing season behavior. `SetSubstitute`/`ClearSubstitute` are
  deliberately UNCHANGED by this -- the approved rule that a substitute
  may come from another team or league still holds exactly as before.

**Unassigned-player route behavior made explicit.**
`resolveExistingPlayerScope` (`handlers/api_scope_resolvers.go`) now
resolves an existing player's scope from ONLY the player's own persisted
league -- it never reads a request body to manufacture ownership for an
unassigned player. This directly fixes two real bugs: `DELETE
/api/players/{id}` has no body at all (the old code's body-read on an
unassigned player produced an accidental 400 from JSON-decode EOF instead
of a real 200/403/404 decision), and `POST /api/players/{id}/merge`'s
body carries `target_id`, not `league_id` (the old code was reading the
wrong field entirely for that route). The explicit policy: system_admin
may manage an unassigned player; league_admin access requires real,
persisted league ownership -- if none exists, 403, never a manufactured
scope from client input.

**`guardedAction`'s fail-open path corrected.** The gate was
`deps.ApplyAuth == nil -> fully open`, which meant a Dependencies with
`AuthMgr`/`RoleAssignmentMgr` fully wired but `ApplyAuth` left nil (a
real, if partial, session-only configuration) fell through to
completely unauthenticated access on every guarded route. Fixed in
`handlers/api_auth_middleware.go`: the fully-open fallback now requires
ALL THREE of `ApplyAuth`, `AuthMgr`, and `RoleAssignmentMgr` to be nil --
the exact, and only, condition that identifies a deliberately minimal
test Dependencies with no auth wired at all (never true in production;
`main.go` always wires all three together). Wiring even one of them is
now treated as a real auth configuration that must fail closed. The same
two-condition fix was applied to `playerOverviewAuth`
(`handlers/api_player_overview_routes.go`) and to
`requireApplyAuthOrScopedAction`'s own nil-ApplyAuth special case (used
by the Apply route), both of which had the identical bug.

**Handicap Apply mounting decoupled from the static token.** `POST
/api/seasons/{id}/handicap-apply` now mounts whenever
`deps.HandicapApplier` is wired AND at least one authentication path is
available -- `deps.AdminToken`, `deps.ApplyAuth`, `deps.AuthMgr`, or
`deps.RoleAssignmentMgr`, any one of them -- instead of requiring
`AdminToken` specifically. A deployment relying purely on session login,
with no `LEAGUE_ADMIN_TOKEN` configured at all, no longer loses this
route. The static token remains a fully optional, unscoped fallback for
unattended automation.

**`sqlite_sequence` restoration now covers an emptied `users` table.**
The round-1 fix only handled "some users survived migration, so the
rebuild's row copy produces at least one insert, creating a
`sqlite_sequence` row to `UPDATE`." If EVERY legacy user was deleted
before migration ran, the copy produces zero rows, `users_new` never
receives an AUTOINCREMENT insert, and a plain `UPDATE` matches nothing --
silently losing the historical high-water mark. `db.go`'s rebuild now
`INSERT`s a `sqlite_sequence` row for `users` when none exists yet
(guarded by `WHERE NOT EXISTS`), then `UPDATE`s it upward when one
already does but is lower than the historical value -- covering both
cases with the same historical value either way.

**Verified with:** the full existing suite, plus: unit tests for
`MatchService.AssignMatchTeams`'s cross-league/unknown-team/same-league
cases (`backend/domains/matches/match_service_test.go`) and
`LineupService.SaveTeamLineup`'s cross-league/not-participating/
legacy-season cases (`backend/domains/matches/lineup_service_test.go`);
new HTTP-level cross-league tests for player create/update/merge and for
match-assign/lineup-save's domain-service invariants (extended
`handlers/api_auth_scoped_families_test.go`); a new
`TestAuthIntegration_UnassignedPlayerRoutes` covering system_admin
delete/merge of an unassigned player (including confirming a bodyless
DELETE does not 400), and league_admin denial of the same;
`handlers/api_auth_partial_wiring_test.go` (new) proving a session-only
Dependencies (AuthMgr/RoleAssignmentMgr wired, ApplyAuth nil) still
requires a credential (401 with none, 403 for an unresolvable Bearer
key) while an authorized session succeeds, alongside a test confirming
the fully-open fallback still holds for a Dependencies with nothing
auth-related wired at all; a new
`TestRegister_ApplyRoute_Mounted_WhenSessionAuthOnly_NoToken` proving the
Apply route mounts and enforces session auth with no `AdminToken`/
`ApplyAuth` configured; and a new
`TestMigrateUsersAndAPIKeys_PreservesHistoricalSequenceWhenAllUsersDeleted`
alongside the existing high-ID-deleted case. Browser verification was
not claimed or attempted this round either.

## Users/Roles/Authentication Phase 1 -- PM final credential-precedence and player-unassignment corrections (round 3, 2026-09-19)

**Status:** `accepted`

PM's second re-review of round 2's handoff found two remaining defects,
both requiring correction before commit.

### 1. Session-versus-Admin-Key credential precedence

**The defect:** the identity a signed-in browser tab DISPLAYED and the
identity its protected requests EXECUTED as could diverge. `GET
/api/auth/me` (what `web/app.js`'s `resolveCurrentIdentity` shows in the
shell) already preferred an active session over the legacy Admin Key, but
two other places did not follow that same precedence:
- `web/lib/api-client.js`'s `api()` attached `Authorization: Bearer
  <adminKey>` whenever a key remained in `sessionStorage`, regardless of
  whether a session was also active -- and it skipped the CSRF header
  whenever it did so.
- `handlers/api_auth_middleware.go`'s `resolveIdentity()` tried the
  Bearer header before the session cookie, so a request carrying BOTH
  (e.g. a stale key left over from an earlier manual test in the same
  browser tab) executed as the Bearer key's identity, silently
  overriding whatever the session's own role/scope actually was.

**The fix -- one explicit precedence, enforced in both places:** an
active password session ALWAYS takes precedence over a stored Admin Key.
  - `resolveIdentity` now checks the session cookie FIRST. Only when no
    session cookie is present, or the one present does not resolve
    (expired/invalid), does it fall back to the Bearer header. This is
    the authoritative point: even if a stale Bearer header is attached
    to a request, a resolvable session cookie always wins, and CSRF is
    still enforced on that request's mutation (the session-vs-Bearer
    branch inside `requireAction` is unchanged -- `sess != nil` still
    drives the CSRF check, and reordering `resolveIdentity` makes `sess`
    non-nil in exactly this case).
  - `web/lib/api-client.js`'s `api()` now checks whether a session is
    active (a non-empty `csrf_token` cookie -- this cookie's lifetime is
    tied exactly to the session's, see `setAuthCookies`/
    `clearAuthCookies` in `handlers/api_auth_handlers.go`) BEFORE
    deciding what to attach: if a session is active, it never attaches
    the Admin Key (even if one is stored) and attaches `X-CSRF-Token`
    for mutations instead; only when no session is active does it fall
    back to the Admin Key. Error messages (401/403) also now reflect
    which credential was actually in play.
  - `web/domains/auth/auth-api-service.js`'s `login()` additionally
    clears any stored Admin Key on a successful password login (belt
    and suspenders, not the only thing preventing the split -- the
    precedence rule above already does that on its own).
  - The static `LEAGUE_ADMIN_TOKEN` bootstrap/automation path
    (`requireAdminToken`, C1's original static-token gate) is untouched
    by any of this -- it is a wholly separate credential from the
    personal Admin Key and was never part of the split-identity
    scenario.

**Why this needed a backend fix, not just a frontend one:** the frontend
fix alone stops a *correctly-behaving browser* from ever attaching a
stale key once a session is active, but it cannot be the only thing
enforcing the contract -- any other caller of the API (a future screen,
a manual curl, a test) could still attach both headers. Reordering
`resolveIdentity` makes the precedence a server-side invariant that
holds regardless of what the caller sends.

**New regression coverage** (`handlers/api_auth_credential_precedence_test.go`,
all HTTP-level against a real session + a real personal Bearer key, since
the frontend half of this fix has no Go-testable surface):
- `TestCredentialPrecedence_SessionWinsOverDifferentSystemAdminBearerKey`
  -- a league_admin session plus a stale system_admin Bearer key calling
  the genuinely system_admin-only `POST /api/backup` still gets 403 (the
  session's own, lesser role governs, not the key's).
- `TestCredentialPrecedence_PlayerSessionCannotGainSystemAdminFromStaleKey`
  -- same proof for a `role=player` session specifically.
- `TestCredentialPrecedence_LeagueAdminSessionScopedByOwnAssignment_NotStaleKey`
  -- a League A-scoped league_admin session, plus a stale Bearer key
  scoped to League B, still succeeds creating a team in League A and is
  still denied creating one in League B.
- `TestCredentialPrecedence_BearerFallbackStillWorksWithNoSession` -- a
  Bearer key with no session cookie present at all still authenticates
  exactly as before (the fallback path is unaffected).
- `TestCredentialPrecedence_SessionMutationStillRequiresCSRF_EvenWithBearerAttached`
  -- a session-authenticated mutation with a Bearer header attached but
  no CSRF header still gets 403 (CSRF enforcement is not bypassed by the
  presence of a Bearer header once the session path is the one taken).

`POST /api/backup` was chosen over league creation for the system_admin-
only cases above because league creation is deliberately NOT
system_admin-only (`guardedLeagueAdminAction` + an atomic self-grant --
see the 2026-09-18 entry below); backup rejects league_admin outright,
making it the correct fixture for proving a lesser role cannot borrow a
stale key's system_admin power.

### 2. Player update must not let league_admin create an unassigned player

**The defect:** `updatePlayerScope` (round 2's fix) only validated a
destination league when `body.team_id` was non-nil, treating a nil
`team_id` as "no team change, keep authorizing against the player's
current league." But `updatePlayer` (`handlers/api_players_handlers.go`)
is a full-PUT handler that always persists `body.TeamID` exactly as
decoded -- and Go's JSON decoding leaves a `*int64` field nil both when
the caller sends `team_id:null` AND when the caller omits the field
entirely; there is no way to distinguish the two. A league_admin could
therefore: authorize a PUT against a player currently in their own
league (satisfying the origin-league check), send a body with no
team_id, and have the handler silently persist `team_id = NULL` --
unassigning the player into the system_admin-only unassigned state
(round 2's `resolveExistingPlayerScope` rule) with no way for that same
league_admin to undo it.

**The fix:** `updatePlayerScope` (`handlers/api_scope_resolvers.go`) now
treats a nil `body.TeamID` as "this request will persist team_id = NULL"
and requires system_admin for it, exactly like a cross-league move. Only
a non-nil `body.TeamID` that resolves to the player's own (origin) league
is authorized as an ordinary league_admin edit. The existing
players-page frontend (`web/domains/players/players-page-component.js`)
already always sends an explicit `team_id` (the selected team, or
`null` if the dropdown is deliberately set to "no team") on every save,
so a normal same-league edit that leaves the team dropdown alone is
unaffected -- only a deliberate unassignment (or an accidentally
malformed request) is now gated.

**New regression coverage** (`handlers/api_auth_scoped_families_test.go`):
- `player update: league_admin cannot unassign, system_admin can` (new
  subtest under `TestAuthIntegration_ScopedFamilies`) -- proves
  league_admin gets 403 for both an explicit `team_id:null` body and a
  body that omits `team_id` entirely, that the player remains assigned
  to their original team after each rejected attempt, and that
  system_admin CAN unassign the same player (verified by re-fetching the
  player and checking `team_id` is nil afterward).
- `league_admin denied update (assign) of an unassigned player` /
  `system_admin allowed update (assign) of an unassigned player` (new
  subtests under `TestAuthIntegration_UnassignedPlayerRoutes`) -- prove
  the mirror case: assigning a team to a player that starts unassigned
  is also system_admin-only, regardless of which league the destination
  team belongs to.

Verification: `go test ./... -count=1` (full suite, 0 failures), `go
build ./...`, `go vet ./...` (same 4 pre-existing warnings, files
untouched by this round), `gofmt -l` clean, `node --check` on
`web/app.js`, `web/lib/api-client.js`,
`web/domains/auth/auth-api-service.js`, and
`web/domains/auth/login-page-component.js` (all pass), `git diff
--check` clean (benign CRLF warnings only), and an ASCII scan of the
full cumulative diff across all three correction rounds found exactly
two non-ASCII characters, both pre-existing em-dashes being REMOVED (one
in `db/db.go`, one in `handlers/api.go`) as part of round 2's own edits
converting old comment text to this codebase's ASCII-dash convention --
zero non-ASCII characters were added. Browser verification was not
attempted or claimed for this round.

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

### 2026-09-18 - Users/Roles/Authentication Phase 1: real email/password login, sessions, and scoped roles

**Status:** `accepted`

Supersedes this file's prior "personal API keys remain the mechanism;
no login endpoint or session model" position (2026-08-26 entry above)
and the USERS-Q001 discovery's "not next: browser sessions, JWTs, a
login endpoint... permission scoping by league" (line ~724 above) --
that revisit, previously declined, is this phase. Added real
email+password login (Argon2id, measured parameters), server-managed
sessions with CSRF protection, a normalized `role_assignments` model
(system_admin global, league_admin per-league, both DB-CHECK-enforced,
`player` still exclusively `users.player_id`), a single centralized
`auth.Authorize` policy, league creation with an atomic self-grant, and
league/season-scoped authorization on league and season CRUD. Existing
personal API keys and every player without an account remain fully
valid throughout, including through a one-time schema migration that
moves API-key credentials into a dedicated table and backfills role
assignments from every legacy flat role. Self-registration, email
verification, the captain role, and two-team score approval remain
explicitly deferred; migrating the remaining flat-`clearanceAuth`
routes onto the new scoped policy is the next slice, not yet scheduled.
See "Users/Roles/Authentication Phase 1 Implementation" above for full
detail.

### 2026-09-19 - Users/Roles/Authentication Phase 1: PM correction round

**Status:** `accepted`

Corrects several claims in the entry above that did not match actual
route behavior at handoff time, found in PM review: (1) "league/season-
scoped authorization on league and season CRUD" was true but incomplete
-- every OTHER existing `clearanceAuth`/`systemAdminAuth` route (players,
teams, complete season setup, schedule/pushback, lineups, match
scoring/approval, week close/reopen, finances, handicap apply, backup,
user administration, Player Overview) still rejected session cookies
entirely and used a separate, unscoped role check; all of them now go
through the same `auth.Authorize` call as league/season CRUD, scoped to
each route's own resource. (2) "league creation with an atomic self-
grant" was actually a create-then-grant-then-compensating-delete
sequence, not a real transaction; `LeagueSelfGrantStore` now commits both
rows in one `*sql.Tx`. (3) The migration's child-table creation order
did not match its own documented sequencing (harmless in practice, since
those tables were empty at that point, but not a real guarantee); fixed
by creating them only after `users` has its final shape. (4) Copying
only currently-existing row ids into the rebuilt `users` table did not
preserve `sqlite_sequence`'s historical high-water mark across a
delete-then-migrate sequence; fixed by reading and restoring it
explicitly. Also added: a login-screen password-setup mode and a "Use
Admin Key instead" link (the modal previously had no trigger reachable
before sign-in). See "Users/Roles/Authentication Phase 1 -- PM
correction round (2026-09-19)" above for full detail.

### 2026-09-19 - Users/Roles/Authentication Phase 1: PM final authorization corrections (round 2)

**Status:** `accepted`

A second PM review of the round-1 correction found five remaining,
narrower issues, all now fixed: (1) authorizing a route's primary
resource was not enough when its body could attach or move a SECOND
resource (a team, another player) across leagues -- player create/
update/merge, match team assignment, and lineup save now all validate
every related resource's real, persisted league, not just the one the
scope resolver first looked at. (2) The unassigned-player scope fallback
read a request body that did not exist for `DELETE` or meant something
else for `merge`; fixed by never reading the body for an existing
player's scope at all -- an unassigned player is system_admin-only,
full stop. (3) `guardedAction` (and two of its call sites) fell open
whenever `ApplyAuth` was nil, even with `AuthMgr`/`RoleAssignmentMgr`
fully wired -- a session-only configuration was silently unauthenticated;
fixed to require all three nil before falling open. (4) The Apply route
was mounted only when the static `AdminToken` was configured; now
mounts whenever any auth path (including session-only) is available.
(5) The `sqlite_sequence` restoration from round 1 only handled a
rebuild that copied at least one row; a rebuild against an EMPTIED
`users` table now restores the historical high-water mark too. See
"Users/Roles/Authentication Phase 1 -- PM final authorization
corrections (round 2, 2026-09-19)" above for full detail.

### 2026-09-19 - Users/Roles/Authentication Phase 1: PM final credential-precedence and player-unassignment corrections (round 3)

**Status:** `accepted`

A third PM review found two remaining defects: (1) the identity a signed-
in browser tab displayed (`GET /api/auth/me`, session-preferring) and the
identity its protected requests actually executed as (`resolveIdentity`,
Bearer-checked-first; `api()`, Admin-Key-attached-whenever-present) could
diverge -- a stale/different Admin Key left in a tab's `sessionStorage`
could silently execute mutations under its own identity instead of the
signed-in session's. Fixed by making an active session take precedence
over a stored Admin Key everywhere the decision is made:
`resolveIdentity` now checks the session cookie first, `api()` now checks
for an active session (via the `csrf_token` cookie) before ever attaching
the Admin Key, and a successful password login now also clears any
stored Admin Key outright. (2) `updatePlayerScope` (round 2) treated a
nil `body.team_id` as "no team change," but `updatePlayer`'s full-PUT
semantics persist that nil as `team_id = NULL` regardless of whether the
caller meant to unassign or simply omitted the field -- letting a
league_admin authorize against their own player and then have the
handler silently unassign them into the system_admin-only unassigned
state. Fixed by requiring system_admin for any update that would persist
`team_id = NULL`, exactly like a cross-league move. See
"Users/Roles/Authentication Phase 1 -- PM final credential-precedence and
player-unassignment corrections (round 3, 2026-09-19)" above for full
detail.
