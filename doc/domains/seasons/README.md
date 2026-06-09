# Seasons

## Overview

**Owner:** `seasons`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-08`

The Seasons domain owns setup, activation, league-week workflow, closing,
reopening, and final standings snapshots.

## Setup Workflow

```text
Draft
-> configure rules
-> select/create teams
-> assign players to home teams
-> set start date
-> configure schedule requests and No Play weeks
-> generate and preview schedule
-> approve
-> activate immediately
```

The admin owns setup. All active league teams are initially selected, and may
be removed before activation.

## Active Season

One season per league may be active. Different leagues, nights, and formats may
have active seasons simultaneously.

Activation locks rules and team membership. Controlled schedule changes remain
available.

## Week Review

Scores may be entered before an admin closes the league week. Close Week runs
backend validation, presents errors and warnings, and commits official
calculations only after errors are resolved and every warning is explicitly
acknowledged.

A closed week may be reopened multiple times, but only selected affected
matches become editable. Closing again reruns validation and creates another
audited review event.

## Closing

A season cannot close while matches remain unresolved. Each match must be
completed or receive a controlled resolution. Closing calculates placements,
requires admin approval, and stores an immutable final standings snapshot.

Corrections to a closed season require audited reopening, recalculation, review,
and closing again.

## Decision History

### 2026-06-08 - Separate activation and closing

**Status:** `accepted`

Activating a new season never silently closes the previous season.

### 2026-06-08 - Make season participation explicit

**Status:** `accepted`

Season teams will be recorded directly rather than inferred from matches.

## Deferred Enhancements

### SEASONS-TODO-001 — Manual team selection in Edit Season

**Status:** `deferred` — not MVP scope

The "Use Teams From Prior Season" field in the Edit Season / New Season form currently
pre-selects a prior season's teams at schedule-generation time. A future enhancement
would add a dedicated "Select Teams" step where the operator explicitly picks which
teams participate in the season before schedule generation runs.

This requires:
- A `season_teams` join table (approved design direction, see architecture-decisions.md)
- A team-selection UI step in the season setup workflow
- Validation that at least N teams are selected before generation is allowed

Do not implement the manual selection workflow until the `season_teams` table and
explicit season-participation design are finalized.
