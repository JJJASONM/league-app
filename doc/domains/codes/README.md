# Controlled Codes

## Overview

**Owner:** `codes`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-08`

The Codes domain provides stable values for statuses, reasons, categories, and
other controlled selections. It minimizes free text in business data.

## Behavior

- Records store stable codes, not labels.
- Labels may be edited without rewriting transaction history.
- Codes cannot be renamed after creation.
- Obsolete codes become inactive and remain readable historically.
- Codes are unique within a code set.
- The same code, such as `OTHER`, may exist in different code sets.
- Optional explanatory free text is stored in a `notes` field.

Developers define code sets and their business meaning. A future Admin screen
will manage labels, display order, and active status.

## Questions

### CODES-Q001 - Physical storage design

**Status:** `open`
**Opened:** `2026-06-08`
**Resolved:** `pending`
**Related commit:** `pending`

**Context:** Focused code sets may share one storage mechanism, but crossover,
foreign-key integrity, seed data, and domain ownership require review.

**Resolution:** Review all required sets together before choosing one generic
table, several focused tables, or a hybrid.

## Decision History

### 2026-06-08 - Store codes instead of labels

**Status:** `accepted`

Labels are presentation data. Stable codes are the values used by business
logic and persisted on records.
