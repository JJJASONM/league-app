# Audit

## Overview

**Owner:** `audit`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-08`

The Audit domain records administrative and historically important changes
across the application in one append-only log.

## Entry Contents

- Domain
- Affected record type and ID
- Action code
- Previous values
- New values
- Acting user
- Timestamp
- Reason code when applicable
- Optional `notes`

Admins can filter by date, user, domain, action, and affected record.

## Required Events

Audit rule changes, schedule pushbacks, No Play insertion, match corrections,
week reopen/close, season activation/close/reopen, team and roster changes,
player completion, user changes, code administration, and Close Week warning
acknowledgments.

Warning acknowledgments preserve the warning details, affected records, acting
admin, controlled reason code, optional `notes`, and timestamp.

## Decision History

### 2026-06-08 - Use one shared log

**Status:** `accepted`

One cross-domain history is easier to search and administer than unrelated
audit tables, while entries still identify their owning domain.

### 2026-06-08 - Audit warning acknowledgments

**Status:** `accepted`

Warnings may be overridden only through an explicit and transparent admin
acknowledgment. Validation errors cannot be overridden.
