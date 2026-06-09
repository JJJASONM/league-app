# Matches

## Overview

**Owner:** `matches`
**Status:** `draft`
**Current version:** `0.2`
**Last reviewed:** `2026-06-08`

The Matches domain owns match participation, result entry, finalization,
reopening, corrections, and match-level workflow status.

## Score Entry And Workflow

Scores may be entered and saved before the league week closes. Entering scores
does not make their calculations official. The exact match status transition
after score entry remains open.

Official handicap adjustments, match outcomes, standings, and player
statistics are applied when the admin successfully closes the week. Results
that have not passed week close do not contribute to official totals.

The UI identifies affected matches directly:

```text
Team 1 vs Team 2 - Incomplete
Team 3 vs Team 4 - Missing player review
Team 5 vs Team 6 - Reopened
```

## Close Week Validation

The backend validates the week's score data before official calculations are
committed. Validation includes:

- Missing scores or players
- Impossible scoring combinations
- Duplicate player participation
- Incomplete player profiles
- Handicap or input inconsistencies
- Unresolved matches
- Format-specific scoring errors

Validation results have two severities:

- **Error:** blocks week close and cannot be overridden.
- **Warning:** may allow close only after explicit admin acknowledgment.

Every warning acknowledgment records the warning details, affected records,
admin identity, controlled reason code, optional `notes`, and timestamp in the
shared audit log. Transparency is the default.

## Corrections

An admin reopens the containing week and selects only the affected matches.
Unaffected finalized matches remain locked. Corrected matches are finalized and
the week is closed again.

All corrections record old values, new values, actor, reason, and timestamp in
the shared audit log.

## Questions

### MATCHES-Q001 - Status after score entry

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** Scores are entered before week close, but additional calculations
and validation still need to occur.

**Resolution:** Decide whether completed score entry creates a review status,
remains draft, or uses another controlled status.

### MATCHES-Q002 - Online score entry

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** Online entry affects drafts, permissions, competing edits,
validation, approval, and the Close Week preview.

**Resolution:** Design the online score-entry workflow before finalizing match
statuses or calculation-preview behavior.

### MATCHES-Q003 - Historical warning display

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** Warning acknowledgments are audited, but their placement on
historical match and week screens is not decided.

**Resolution:** Define what authorized users see outside the audit log.

## Decision History

### 2026-06-08 - Make week close authoritative

**Status:** `accepted`

Score entry stores pending data. Official calculations and result effects are
committed only after backend Close Week validation succeeds.

### 2026-06-08 - Require transparent warning acknowledgment

**Status:** `accepted`

Errors block close. Warnings require explicit, reasoned, audited admin
acknowledgment.
