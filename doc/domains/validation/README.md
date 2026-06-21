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

`Result` is JSON-serialisable (`{"messages": [...]}`). Handlers may encode it
directly or use a future shared helper. No handler helper exists in this phase.

### HTTP response convention (planned)

- **Errors present:** HTTP 422 with `Result` JSON body
- **Warnings only:** operation proceeds; warning behavior is modeled in the shared
  package and will be enforced by future domain validators and Close Week finalization
- **No messages:** operation proceeds normally

No domain validators compute warnings yet. The warning infrastructure is in place
for future use.

## Domain validators

No domain validators have been implemented yet. The shared package is in place;
domain validators will be added one at a time, beginning with the scoresheet
validator in the matches domain.

## Design notes

- `Code` values are stable API contracts. Add new codes; never rename existing ones.
- `Field` enables frontend input highlighting without parsing message text.
- Warning acknowledgment (audited admin override for warnings) is future work scoped
  to Close Week finalization.
