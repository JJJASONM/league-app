# Validation

## Overview

**Package:** `backend/validation`
**Status:** `active`
**Current version:** `0.1`
**Last reviewed:** `2026-06-20`

The `backend/validation` package provides shared types for structured validation
results. It is not domain-specific and may be used by any backend domain validator.

## Package API

```go
var result validation.Result

result.AddError("STABLE_CODE", "field_name", "Human-readable message")
result.AddWarning("STABLE_CODE", "", "Non-blocking note")

result.HasErrors()   // true if any error-level messages exist
result.HasWarnings() // true if any warning-level messages exist; does not affect IsValid
result.IsValid()     // equivalent to !HasErrors()
result.Errors()      // []Message filtered to errors
result.Warnings()    // []Message filtered to warnings
```

### Message fields

| Field | Type | JSON | Notes |
|-------|------|------|-------|
| `Code` | `string` | `"code"` | Stable, machine-readable. Never change after release. |
| `Field` | `string` | `"field"` (omitempty) | Maps to a UI input name; omit for non-field messages. |
| `Message` | `string` | `"message"` | Human-readable display text. May be updated. |
| `Level` | `Level` | `"level"` | `"error"` or `"warning"` |
| `MatchID` | `*int64` | `"match_id"` (omitempty) | Match scope for messages emitted by `ValidateWeek`; nil for messages from `ValidateRounds` used directly (e.g. `saveRounds`). |

`Result` is JSON-serialisable (`{"messages": [...]}`). The `jsonValidation` helper
in `handlers/api.go` encodes a `Result` as an HTTP 422 response body.

### HTTP response convention

- **Errors present:** HTTP 422 with `Result` JSON body
- **Warnings present, no errors:** operation may be gated by acknowledgment (see Close Week Phase 2A)
- **No messages:** operation proceeds normally

### MatchID stamping

`MatchID` is nil on messages emitted by domain validators that are not
match-scoped (e.g. `ValidateRounds` called directly from `saveRounds`).

`ValidateWeek` iterates per-match and stamps `MatchID` on every message it
appends, including messages forwarded from `ValidateRounds`. Callers can
use `(match_id, code, field)` as a stable compound key to identify and
acknowledge specific warnings at Close Week time.

## Domain validators

**Matches -- scoresheet validator** (`backend/domains/matches.ValidateRounds`) is the first
domain validator. It validates 8-ball round submissions before any DB write. See
`doc/domains/matches/README.md` -- Backend Scoresheet Validation.

**Matches -- week validator** (`backend/domains/matches.ValidateWeek`) runs Close Week
validation across all matches in a week. It stamps `MatchID` on every message.
See `doc/domains/matches/README.md` -- Close Week.

Additional domain validators will be added one at a time as needed.

## Design notes

- `Code` values are stable API contracts. Add new codes; never rename existing ones.
- `Field` enables frontend input highlighting without parsing message text.
- `MatchID` is an additive field (`omitempty`); callers that do not set it serialize
  identically to before it was added.
