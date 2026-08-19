# Players

## Overview

**Owner:** `players`
**Status:** `draft`
**Current version:** `0.2`
**Last reviewed:** `2026-07-14`

Players are shared system-wide identities. They are not owned by one league or
team and are separate from authenticated user accounts.

## Participation

A player may have one home team in a season and may substitute for any team in
any league or season. Match participation records the team represented and
whether the player was rostered or substituting.

The current `players.team_id` implementation cannot represent this target
behavior and will eventually be replaced by season roster relationships.

## Quick Add

Admins can quick-add a missing player from the Players page without entering a
full profile. The minimum required fields are at least one name (first or last)
and a diff rating (defaults to 0). Player number, phone, email, and admin hold
are optional and can be completed later using the standard Edit Player modal.

Before creating the player, quick-add warns when the typed name normalizes to
the same full name as an existing player in the active league (see Duplicate
Warning below). This is advisory only; it does not block creation.

Deferred: INCOMPLETE profile status, close-week blocking for incomplete
profiles, and match-entry quick-add integration are not yet implemented.

## Duplicate Warning (Phase A, 2026-08-19)

Quick-add compares the typed first/last name against the already-loaded,
league-scoped player list on the Players page. Both the typed name and each
existing player's name are normalized (trimmed, internal whitespace collapsed,
case-folded) before comparison; a match is exact-normalized only, not fuzzy.

When a match is found, the admin sees the existing player's name and team (or
"no team" if unrostered) and can either cancel or continue and add the new
player anyway. No match means quick-add proceeds exactly as before this phase.

This is entirely client-side (`web/domains/players/players-page-component.js`):
no backend validation, DB constraint, or API change was made. Two different
real people who share a name will still see the warning every time they are
quick-added -- that is expected, not a bug, given the warn-only design.

## Deferred Player Maintenance

The following player-record maintenance items remain parked:

- Safe merge workflow for accidental duplicate player records.
- INCOMPLETE profile status and close-week blocking for incomplete profiles.
- Match-entry quick-add integration.

Duplicate detection for quick-added players is implemented as of Phase A above
(client-side, warn-only, normalized-exact name match); it is no longer in this
deferred list.

## Questions

### PLAYERS-Q001 - Quick-add requirements

**Status:** `resolved`
**Opened:** `2026-06-08`
**Resolved:** `2026-07-14`

**Context:** League night cannot stop for complete registration, but duplicate
players and unsupported handicap values must be avoided.

**Resolution (Phase 1):**
- Quick-add lives on the Players page as a simplified modal.
- Required fields: at least one of first name or last name; diff rating (default 0).
- Optional fields: team. Player number, phone, email, and admin hold are omitted
  from quick-add and completed later via Edit Player.
- Duplicate detection: deferred. No DB unique constraint on name or player number.
- INCOMPLETE profile status and close-week blocking: deferred to a later phase.
- Match-entry quick-add integration: deferred.

## Decision History

### 2026-06-08 - Share players across leagues

**Status:** `accepted`

One player record may participate in multiple nights and formats.

### 2026-06-08 - Allow any system player to substitute

**Status:** `accepted`

Substitute eligibility is not restricted to the same league or season.

### 2026-07-14 - Quick-add Phase 1: Players page only, name + handicap minimum

**Status:** `accepted`

Phase 1 quick-add lives on the Players page as a simplified modal. The minimum
field set is at least one name and a diff rating (default 0). Player number,
contact fields, and admin hold are omitted and completed later via Edit Player.
INCOMPLETE status, close-week blocking, and match-entry integration are
deferred. Resolves PLAYERS-Q001.

### 2026-08-19 - Quick-add duplicate warning Phase A

**Status:** `accepted`

Added a client-side, warn-only, normalized-exact-name duplicate check to
Players page quick-add, comparing against the already-loaded league-scoped
player list. No backend, schema, or API change. Safe merge, INCOMPLETE profile
status, and match-entry quick-add remain deferred.
