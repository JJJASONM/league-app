# Schedules

## Overview

**Owner:** `schedules`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-08`

The Schedules domain generates, previews, adjusts, and shifts season schedules.
It applies scheduling rules but does not define their meaning.

## No Play Weeks

Planned holidays and later emergencies use the same concept. Store a controlled
reason code and optional `notes`. The UI may display labels such as Holiday,
Weather, or Location Closure, but database records store stable codes.

## Pushback

A pushback inserts one or more complete No Play league weeks at a selected
cutoff. It moves all unplayed weeks at or after the cutoff together.

The operation:

- Never changes completed match dates
- Honors existing No Play dates
- Extends the season end date
- Previews every affected match before applying
- Applies atomically
- Creates an audit entry

## Questions

### SCHEDULES-Q001 - Preview editing controls

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** The admin must be able to review a generated schedule before
activation.

**Resolution:** Define which manual team, date, table, and regeneration actions
are allowed during preview.

## Decision History

### 2026-06-08 - Shift entire league weeks

**Status:** `accepted`

Pushback means every unplayed scheduled week moves together rather than moving
individual matches independently.
