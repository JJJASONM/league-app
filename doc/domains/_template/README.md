# [Domain Name]

## Overview

**Owner:** `[domain]`
**Status:** `draft | experimental | confirmed`
**Current version:** `0.1`
**Last reviewed:** `YYYY-MM-DD`

Describe what this domain owns and why it exists. State what is explicitly
outside its responsibility.

State whether the document describes the current implementation, target design,
or both. Clearly label conceptual tables and workflows that do not exist yet.

## Public Interface

Document the supported entry points used by other domains.

### Frontend

```text
web/domains/[domain]/index.js
```

List exported components, functions, events, and expected inputs.

### Backend

```text
backend/domains/[domain]/public.go
```

List exported types and operations. Note that the backend owns authoritative
business rules and validation.

## Terminology

| Term | Meaning |
| --- | --- |
| `[term]` | `[plain-language definition]` |

## Inputs

| Input | Source | Required | Description |
| --- | --- | --- | --- |
| `[input]` | `[user/API/database]` | `yes/no` | `[meaning and constraints]` |

## Outputs

| Output | Consumer | Description |
| --- | --- | --- |
| `[output]` | `[domain/screen/API]` | `[meaning]` |

## Business Rules

### [Rule Name]

**Rule ID:** `[DOMAIN]-R001`
**Status:** `draft | experimental | confirmed`
**Version:** `0.1`

**What:** Describe the behavior in plain language.

**Why:** Explain the business reason for the behavior.

**Inputs:** List the values used by the rule.

**Output:** Describe the result, including relevant errors.

**Authority:** Identify the backend operation that enforces the rule.

**Notes:** Record source material, legacy behavior, or pending confirmation.

## Examples

### [Example Name]

**Given:** Describe the starting state.

**When:** Describe the action or calculation.

**Then:** Describe the expected result.

## Assumptions

- `[Assumption and why it is currently reasonable]`

## Current Implementation Gap

Describe how the code and schema implemented today differ from the approved
target. Link the migration work or question IDs needed to close the gap.

## Questions

### [DOMAIN]-Q001 - [Question]

**Status:** `open | answered | superseded`
**Opened:** `YYYY-MM-DD`
**Resolved:** `YYYY-MM-DD or pending`
**Related commit:** `[commit hash or pending]`

**Context:** Explain why this question matters.

**Resolution:** Keep this section even while open. Once answered, document the
decision and its reasoning rather than deleting the question.

## Decision History

### YYYY-MM-DD - [Decision Title]

**Status:** `proposed | accepted | superseded`

Describe the decision, why it was made, and any important consequences. Link
related question IDs and commits when available.

## Test Coverage

List stable boundaries and confirmed behavior covered by automated tests.
Identify draft or experimental behavior intentionally left flexible.

## Change Checklist

- [ ] Business-rule status is accurate.
- [ ] Code explanation blocks are current.
- [ ] Confirmed behavior has focused tests.
- [ ] Public interfaces are documented.
- [ ] Questions and decisions are updated.
- [ ] Related commits reference applicable question IDs.
