# League App Roadmap

**Status:** working roadmap
**Last reviewed:** 2026-06-30

This roadmap shows the intended path from the current admin-focused league app
to a reliable season, match, standings, and eventually broader user-facing
system. It summarizes direction from `doc/architecture-decisions.md` and the
domain documents. Domain READMEs remain the authority for detailed rules and
open questions.

## Guiding Path

```text
Stabilize current admin workflows
-> move authoritative business logic to backend domains
-> finish week-close, standings, and handicap operational workflows
-> continue domain and data-access restructuring
-> add controlled codes and season workflow completion
-> add broader audit/history capabilities later
-> explore simple browser match-entry prototypes
-> consider larger user/mobile expansion only after core workflows are stable
```

## Now

These items should stay small enough to review and ship independently.

- Domain and data-access restructuring.
  - Continue moving logic out of `handlers/api.go` into backend domain services.
  - Continue moving workflow UI out of `web/app.js` into domain-owned frontend
    modules.
  - Keep backend/domain/store/adapter boundaries explicit and purpose-built.
  - Prioritize `matches`, `handicaps`, `schedules`, `seasons`, and `standings`
    boundaries.

- Stabilize current official-results workflow.
  - Keep Close Week, Reopen, warning acknowledgment, and advance
    preview/result behavior correct.
  - Keep standings and player stats derived only from official closed-week
    results.
  - Keep handicap review/apply behavior aligned with official results and
    attribution.

- Documentation and roadmap alignment.
  - Update roadmap and domain docs so completed Close Week and Handicap Apply
    work is reflected accurately.
  - Keep open questions current instead of letting implemented behavior live
    only in code.

- Keep staging and GitHub current after accepted work.
  - PM owns pushing committed work to origin.
  - Deploy staging after work that needs browser or user verification.

## Next

These are the next build targets after the current workflow foundation is
stable.

- Controlled codes foundation.
  - Resolve `CODES-Q001`.
  - Define physical storage for code sets, values, labels, display order, and
    active flags.
  - Move statuses, reasons, categories, and participation types onto stable
    codes instead of display labels.

- Season and schedule workflow completion.
  - Resolve `SCHEDULES-Q001`.
  - Decide which manual edits are allowed during schedule preview.
  - Preserve completed match history during schedule changes.
  - Clarify how next-week preparation should work operationally.

- Player quick-add workflow.
  - Resolve `PLAYERS-Q001`.
  - Define required quick-add fields, duplicate detection, and initial handicap.
  - Add the admin review path for incomplete player profiles.

- Continue backend/domain extraction where workflows are already active.
  - Reduce monolithic handler/shell ownership further.
  - Keep new work inside domain boundaries rather than adding more temporary
    logic to shared files.

## Then

These items broaden the workflow foundation after the current admin flows are
stable.

- Season closing.
  - Verify all matches are complete or resolved.
  - Calculate placements.
  - Store immutable final standings snapshots.
  - Support audited reopen and recalculation only after the workflow is clearly
    defined.

- Broader operational polish.
  - Tighten schedule usability.
  - Improve admin review flows around seasons, matches, and lineups.
  - Address deferred workflow gaps that are already known but not
    architecture-critical.

## Later

These should wait until the backend workflow boundaries are clearer and the
admin workflows are stable.

- Shared audit/history capability.
  - Implement a broader append-only audit/history system across domains.
  - Record actor, timestamp, domain, affected record, action code, before/after
    values, reason code, and optional notes.
  - Use it across week close, reopen, handicap apply, roster changes, schedule
    changes, and season close.

- Users, roles, and account invitations.
  - Resolve `USERS-Q001`.
  - Separate player records from authenticated user accounts.
  - Define account linking, invitations, permissions, and admin roles.
  - Treat current Handicap Apply personal-key auth as a bridge, not the final
    auth model.

- Online score entry workflow.
  - Resolve `MATCHES-Q002`.
  - Define competing edits, draft saves, permissions, review, and submission.
  - Decide how captains or scorekeepers authenticate and submit results.

- Simple browser-based match-entry prototype.
  - Prototype a lightweight browser match-entry screen.
  - Use it to learn whether phone-friendly/browser-based score entry is
    practical.
  - Keep this as workflow validation, not platform expansion.

- Mobile or broader client expansion.
  - Consider only after core admin workflows, backend boundaries, and API
    contracts are stable.
  - Do not treat this as an active roadmap driver yet.

- Historical import tooling.
  - Import teams, players, schedules, matches, and results from available
    historical data.
  - Handle legacy team numbers and generated identifiers after controlled-code
    and identifier rules are settled.

- Admin code-management screens.
  - Let admins edit labels, display order, and active flags for developer-owned
  code sets.
  - Keep machine codes stable.

## Completed / Largely Completed

These areas are no longer "next" work, though they may still receive focused
follow-up.

- Backend scoresheet validation foundation.
- Scoresheet save/review guardrails.
- Close Week workflow foundation.
- Reopen workflow.
- Warning acknowledgment flow.
- Advance preview / advance-result workflow.
- Official standings gated by closed weeks.
- Handicap review workflow.
- Handicap Apply workflow with attribution bridge.

## Open Questions To Resolve

| ID | Area | Question |
| --- | --- | --- |
| `RULES-Q001` | Rules | How are emergency or mid-season rule amendments handled? |
| `PLAYERS-Q001` | Players | What fields and handicap value are required for quick-add players? |
| `USERS-Q001` | Users | How does the invitation and account-linking workflow operate? |
| `CODES-Q001` | Codes | What physical code-table design best supports all approved code sets? |
| `SCHEDULES-Q001` | Schedules | Which manual edits are allowed during schedule preview? |
| `MATCHES-Q002` | Matches | How will online score entry, permissions, drafts, and review work? |

## Parking Lot

Use `doc/todo.md` for private, out-of-band notes that should not interrupt
the current conversation. Promote items from that parking lot into this roadmap
or a domain README only when they become real planned work.
