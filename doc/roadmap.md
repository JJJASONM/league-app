# League App Roadmap

**Status:** working roadmap
**Last reviewed:** 2026-07-10

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
  - Major domains (matches, handicaps, seasons, leagues, players, teams) have
    purpose-built service/store layers. Remaining work: wire remaining CRUD
    handlers to their extracted services; ensure all domain boundaries are
    explicit before adding further logic.
  - Continue moving workflow UI out of `web/app.js` into domain-owned frontend
    modules. Remaining work: skipped-weeks, bye-requests, and remaining season
    CRUD workflows.
  - Keep backend/domain/store/adapter boundaries explicit and purpose-built for
    any new work added.

- Stabilize current official-results workflow.
  - Keep Close Week, Reopen, warning acknowledgment, and advance
    preview/result behavior correct.
  - Keep standings and player stats derived only from official closed-week
    results.
  - Keep handicap review/apply behavior aligned with official results and
    attribution.

- Keep staging and GitHub current after accepted work.
  - PM owns pushing committed work to origin.
  - Deploy staging after work that needs browser or user verification.

## Next

These are the next build targets after the current workflow foundation is
stable.

- Controlled codes foundation.
  - Resolve `CODES-Q001`.
  - In-domain Go constants are established for statuses and reasons. Physical
    DB storage design (code sets, labels, display order, active flags, admin
    management screens) remains open.
  - Move any remaining free-text comparisons onto stable constants as domain
    work continues.

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
- Backend domain extraction — matches (week close/reopen B1–B4, schedule A, match
  B, lineup C), handicaps (service/store Data Access A, apply B1–B3, personal key
  auth C1), and domain services for seasons, leagues, players, and teams.
  `handlers/api.go` is now a thin delegation layer for most routes.
- Rules domain — backend-authoritative rule definitions and value validation
  (`rules.Definitions()`, `rules.ValidateValue()`); `rules.RuleStore` interface
  used by `matches.ResolveRoundConfig` and `handicaps.Service` to read season rules
  without direct DB access.
- Backend controlled codes vocabulary — in-domain Go constants for schedule types,
  week statuses, handicap reasons, season checklist blockers, and game formats.
- Frontend domain extraction — handicaps, schedules, matches, players, leagues,
  seasons, and standings screens extracted from `web/app.js` into domain-owned
  Web Components and named API services under `web/domains/`.
- Frontend controlled codes — game_format and handicap reason constants in dedicated
  code modules (`web/domains/leagues/game-format-codes.js`,
  `web/domains/handicaps/handicap-codes.js`).
- Documentation alignment — roadmap and domain READMEs updated to reflect
  completed extraction phases and remove stale file/function references.

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
