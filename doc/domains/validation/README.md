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

| Field | Type | Notes |
|-------|------|-------|
| `Code` | `string` | Stable, machine-readable. Never change after release. |
| `Field` | `string` | Maps to a UI input name; omit for non-field messages. |
| `Message` | `string` | Human-readable display text. May be updated. |
| `Level` | `Level` | `"error"` or `"warning"` |

`Result` is JSON-serialisable (`{"messages": [...]}`). The `jsonValidation` helper
in `handlers/api.go` encodes a `Result` as an HTTP 422 response body.

### HTTP response convention

- **Errors present:** HTTP 422 with `Result` JSON body (implemented -- see `saveRounds`)
- **Warnings only:** operation proceeds; warnings are computed but not currently
  returned to callers; available for future Close Week finalization
- **No messages:** operation proceeds normally

The matches scoresheet validator (`ValidateRounds`) computes both errors and warnings.
Warning acknowledgment (audited admin override) is future work scoped to Close Week.

## Domain validators

**Matches -- scoresheet validator** (`backend/domains/matches.ValidateRounds`) is the first
domain validator. It validates 8-ball round submissions before any DB write. See
`doc/domains/matches/README.md` -- Backend Scoresheet Validation.

Additional domain validators will be added one at a time as needed.

## Design notes

- `Code` values are stable API contracts. Add new codes; never rename existing ones.
- `Field` enables frontend input highlighting without parsing message text.
- Warning acknowledgment (audited admin override for warnings) is future work scoped
  to Close Week finalization.
