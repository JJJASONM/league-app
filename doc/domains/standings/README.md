# Standings

## Overview

**Owner:** `standings`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-08`

The Standings domain calculates and presents team standings and player
statistics from finalized match results.

## Live Results

Standings and statistics are available throughout the season and recalculate
when finalized results change. Draft, incomplete, and reopened matches are
excluded from official totals.

The UI identifies excluded matches and their controlled status so users can
understand why results are provisional.

## Final Snapshot

Closing a season calculates placements and creates an immutable final standings
snapshot after admin review. Reopening a closed season requires recalculation,
review, and a new close event.

## Decision History

### 2026-06-08 - Keep live statistics available

**Status:** `accepted`

Week closure is an administrative checkpoint, not a standings snapshot.

### 2026-06-08 - Snapshot only at season close

**Status:** `accepted`

The final approved season standings are preserved separately from live
calculations.
