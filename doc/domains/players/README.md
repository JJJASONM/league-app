# Players

## Overview

**Owner:** `players`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-08`

Players are shared system-wide identities. They are not owned by one league or
team and are separate from authenticated user accounts.

## Participation

A player may have one home team in a season and may substitute for any team in
any league or season. Match participation records the team represented and
whether the player was rostered or substituting.

The current `players.team_id` implementation cannot represent this target
behavior and will eventually be replaced by season roster relationships.

## Quick Add

Admins may quick-add a missing player during match entry. The player may play
immediately and receives an `INCOMPLETE` status. The affected week cannot close
until the Admin review process completes the profile.

## Questions

### PLAYERS-Q001 - Quick-add requirements

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** League night cannot stop for complete registration, but duplicate
players and unsupported handicap values must be avoided.

**Resolution:** Define required name fields, duplicate matching, initial
handicap entry/default, and later profile completion requirements.

## Decision History

### 2026-06-08 - Share players across leagues

**Status:** `accepted`

One player record may participate in multiple nights and formats.

### 2026-06-08 - Allow any system player to substitute

**Status:** `accepted`

Substitute eligibility is not restricted to the same league or season.
