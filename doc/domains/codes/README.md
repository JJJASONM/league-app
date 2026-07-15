# Controlled Codes

## Overview

**Owner:** `codes`
**Status:** `draft`
**Current version:** `0.2`
**Last reviewed:** `2026-07-14`

The Codes domain provides stable values for statuses, reasons, categories, and
other controlled selections. It minimizes free text in business data.

## Behavior

- Records store stable codes, not labels.
- Labels may be edited without rewriting transaction history.
- Codes cannot be renamed after creation.
- Obsolete codes become inactive and remain readable historically.
- Codes are unique within a code set.
- The same code value may appear in different code sets.
- Optional explanatory free text is stored in a `notes` field.

Developers define which code sets exist and what they mean. A future Admin
screen will manage labels, display order, and active status.

## Current Implementation

Behavior-driving codes are developer-owned constants defined in each domain
package. No DB-backed `code_sets` or `code_values` tables exist. This is the
accepted Phase 1 implementation.

### Developer-Owned Behavior-Driving Codes

These codes drive branching logic, workflow, storage, or error handling. They
must remain stable after creation and cannot be edited at runtime.

| Code Set | Go Location | DB Column |
|---|---|---|
| Schedule types | `backend/domains/matches/codes.go` | `seasons.schedule_type` |
| Week statuses | `backend/domains/matches/codes.go` | `league_weeks.status` |
| Handicap update methods | `backend/domains/handicaps/codes.go` | `season_rules.rule_value` |
| Handicap reason codes | `backend/domains/handicaps/codes.go` | API responses (computed output) |
| Handicap review statuses | `backend/domains/handicaps/codes.go` | API responses (computed output) |
| Close-week validation codes | `backend/domains/matches/closeweek.go` | `week_close_acknowledgments.warning_code` |
| Season checklist blocker codes | `backend/domains/seasons/codes.go` | API responses; some reused in domainerr |
| Rule keys | `backend/domains/rules/public.go` (`Definition.Key`) | `season_rules.rule_key` |
| Game format codes | `web/domains/leagues/game-format-codes.js` | `leagues.game_format` |
| Domain error codes (domainerr) | Inline in service methods | Never stored; used in handler mapping and tests only |

Behavior-driving codes must remain developer-owned. Changing a stored code
value after records exist would corrupt historical reads. Changing a domainerr
code would break test assertions and handler error mapping.

### Potential Future Display/Configuration Values

These are candidates for DB-backed labels and admin editability once a real
admin workflow creates the need. They do not drive business logic; their display
labels can change without breaking stored records or live logic.

- Rule definition labels, help text, display order, and group order (currently
  in `rules.Definition` struct fields; changing them requires a code deploy)
- Schedule type display labels (e.g., "Double Round Robin" for `double_rr`)
- Game format display labels (e.g., "8-Ball" for `8ball`)
- Skipped-week reason codes (`skipped_weeks.reason` is free text today; a small
  stable code set would prevent label drift across records)
- Bye-request reason codes (`bye_requests.reason` is free text today)
- User role labels (`users.role` has no formal code set; only `admin` is used)

## Questions

### CODES-Q001 - Physical storage design

**Status:** `resolved`
**Opened:** `2026-06-08`
**Resolved:** `2026-07-14`

**Context:** Focused code sets may share one storage mechanism, but crossover,
foreign-key integrity, seed data, and domain ownership require review.

**Resolution (Phase 1):**
- Developer-owned stable code constants in each domain package remain the
  source of truth for all behavior-driving codes.
- No `code_sets` or `code_values` DB tables are implemented in Phase 1.
- No admin code-management screens are implemented in Phase 1.
- DB-backed labels, display order, and active flags are deferred until a real
  admin workflow requires runtime editing without a deploy.
- A future read-only backend codes registry (analogous to `rules.Definitions()`)
  and `/api/codes/{set}` endpoint may be useful when frontend components need
  shared labels for dropdowns, but this is not approved or required now.
- Behavior-driving codes must remain developer-owned and stable.

**Deferred:**
- `code_sets` / `code_values` DB tables and migrations
- Admin code-management screens (labels, display order, active flags)
- Role-based code permissions
- Audit/history integration for code changes
- PostgreSQL adapter work
- FK or CHECK constraint enforcement from domain columns to a codes table
- Read-only `/api/codes/{set}` registry endpoint

## Decision History

### 2026-06-08 - Store codes instead of labels

**Status:** `accepted`

Labels are presentation data. Stable codes are the values used by business
logic and persisted on records.

### 2026-07-14 - Phase 1: Developer-owned constants, no DB storage

**Status:** `accepted`

Behavior-driving codes remain developer-owned stable constants in each domain
package. No DB-backed code tables are implemented in Phase 1. The approved
pattern follows the same approach as `rules.Definitions()` in
`backend/domains/rules/`. Admin-editable labels and a read-only codes API are
deferred until a real admin workflow creates the need. Resolves CODES-Q001.
