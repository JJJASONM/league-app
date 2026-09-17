# League App Roadmap

**Status:** working roadmap
**Last reviewed:** 2026-09-16

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
-> define week-end and season-end clearance
-> add broader audit/history capabilities later
-> define roles, permissions, and API access
-> explore simple browser match-entry prototypes
-> consider larger user/mobile expansion only after core workflows are stable
```

## Now

The whole-app admin-screen and product-test-readiness chunk is complete
(see Completed / Largely Completed below). **No actionable Now item
remains.** Known Gap #9 (the last actionable finding, "Generate Schedule"
undo) was closed 2026-09-11 as a documented decision, not a code change --
see `doc/testing/product-smoke-test-checklist.md`'s Known Gap #9 and the
Completed / Largely Completed entry below. Product test readiness moves
to a closed/monitoring posture: nothing left to schedule, only a short
list of low-severity items to watch in case a real workflow (not just
the smoke checklist) hits one of them.

- Product test-readiness monitoring (no action required unless a real
  workflow hits one of these):
  - Cosmetic/script-level: Known Gap #14 (dashboard bootstrap console
    error on initial load; content still populates correctly), Known
    Gap #5 (staging health check polls `/api/leagues` instead of the
    dedicated `/healthz`), Known Gap #3 (no seed/fixture data
    demonstrates the Dashboard readiness gate's "disabled" state).
  - Tracked-deferred: Known Gap #4 (Player safe-merge has no admin UI --
    see "Player record maintenance" in Then), Known Gap #16 (no Sub
    control for a lineup slot resolved only from already-scored round
    results), Known Gap #17 (Weekly Summary's `player_stats` array is
    still not rendered in that screen), Known Gap #18 (Player Overview's
    schedule section won't show a substitute's one-off match for a
    different team).
  - Full detail, evidence, and staging verification history for every
    item above: `doc/testing/product-smoke-test-checklist.md`.

### Standing operating rules (not active work)

- Keep `main` stable and representative of accepted milestones. PM owns
  pushing committed work to origin, merging accepted branches, and
  deploying staging after work that needs browser or user verification.
- Preserve the official-results invariants: Close Week, Reopen, warning
  acknowledgment, and advance-preview behavior stay correct; standings
  and player stats are derived only from official closed-week results;
  handicap review/apply stays aligned with official results and
  attribution.
- Keep new work inside backend/domain/store/adapter boundaries rather
  than adding workflow logic to shared shell/handler files.
- Keep roadmap and domain documentation aligned with accepted decisions;
  keep resolved questions out of the active Open Questions list.

## Next

These are the next build targets after the current workflow foundation is
stable.

- Continue backend/domain extraction where workflows are already active.
  - Reduce monolithic handler/shell ownership further.
  - Keep new work inside domain boundaries rather than adding more temporary
    logic to shared files.
  - Both the route-registration split (Phases A-D) and the handler-logic
    split (Phases A-L) are complete; see Completed / Largely Completed
    below. `handlers/api.go` now holds only `Register()`, shared helpers,
    auth middleware, the backup route closure, and the rule-definitions
    handler -- any further reduction here is a new, not-yet-scoped effort.

- Keep roadmap and domain documentation aligned with accepted decisions.
  - Promote useful TODO inbox items into the relevant roadmap or domain README.
  - Keep resolved questions out of the active Open Questions list.

## Then

These items broaden the workflow foundation after the current admin flows are
stable.

- Broader operational polish.
  - Tighten schedule usability.
  - Improve admin review flows around seasons, matches, and lineups.
  - Address deferred workflow gaps that are already known but not
    architecture-critical.

- Roles, permissions, and API access implementation.
  - `USERS-Q001` resolved 2026-07-27. Discovery complete; see
    `doc/domains/users/README.md`.
  - Phase 1 wired 2026-07-28: clearance routes (`close/reopen week`,
    `close/reopen season`) gated by personal-key-only auth + league_admin role.
    `role="admin"` accepted as backward-compatible alias. Static token does not
    grant access to clearance or schedule mutation routes.
  - Phase 2 wired 2026-08-07: schedule mutation routes (`generate`,
    `pushback-apply`) gated by the same personal-key-only auth + league_admin
    role. `pushback-preview` is intentionally unprotected (read-only semantics
    despite POST method).
  - Phase 3 wired 2026-08-07: match mutation routes (`assign`, `submit results`,
    `clear results`, `save rounds`) gated by the same auth. GET match reads
    remain unprotected.
  - Phase 4 wired 2026-08-07: season setup mutation routes (19 routes: season
    CRUD, activate, rules, skipped-weeks, bye-requests, season teams, roster,
    lineup plans) gated by the same auth. GET reads remain unprotected.
  - Phase 5 wired 2026-08-08: global league, player, and team CRUD mutation
    routes (9 routes) gated by the same auth. GET reads remain unprotected.
  - Phase 6 wired 2026-08-08: `POST /api/backup` gated by personal-key-only
    auth with a stricter role check (`system_admin`, `admin`; `league_admin`
    rejected). Distinct from the league_admin-allowing check used in Phases
    1-5 because backup is a system-level operation, not league-admin setup
    work.
  - No unprotected admin mutation routes remain from this rollout.
    `handicap-apply` retains its dual-tier `requireApplyAuth` (personal key +
    static token fallback) by design; no change planned until a focused
    attribution/auth cleanup phase.
  - For future online score entry, prefer rostered players assigned to the
    match over a generic scorekeeper role (deferred to MATCHES-Q002).
  - Users Admin Screen Phase 1 implemented 2026-08-26: `POST/GET
    /api/users` now accept either the static `LEAGUE_ADMIN_TOKEN` or a
    resolved personal key with system_admin/admin role (previously the
    static token was the only way in, even for a real system_admin); new
    `GET /api/users/me` resolves the caller's own identity; a first Users
    screen (list + create, role choice restricted to system_admin/
    league_admin) and an Admin Key modal identity indicator were added to
    the frontend. See `doc/domains/users/README.md`'s "Users Admin Screen
    Phase 1 Implementation" section for full detail.
  - Browser sessions and JWTs remain deferred. Building a users management
    screen was the condition previously named as the trigger to revisit
    this -- that revisit was explicitly declined for this phase; personal
    API keys remain the mechanism.

- Player record maintenance.
  - Build a merge UI (preview, confirm) on top of the safe-merge backend after
    product test readiness work confirms this repair path is worth bringing
    into the tested admin surface.
  - Defer INCOMPLETE profile status and close-week blocking until match-night
    quick-add or admin review creates a concrete need.
  - Duplicate detection for player quick-add and the safe-merge backend
    shipped as Phase A; see Completed / Largely Completed below.

- Season setup polish.
  - **Default lineup setup explored and largely addressed 2026-09-16** --
    see Completed / Largely Completed below. Discovery found the default
    lineup editor already existed; a real week-filter bug was fixed and
    a non-blocking setup checklist warning was added. Close Week does
    not depend on future lineups.

- Architecture review follow-up.
  - Defer `models/models.go` decomposition until a touched workflow needs
    clearer API/read-model boundaries.
  - Leave the frontend architecture alone except for opportunistic component
    splits. Do not add an npm build, framework, or global state library just to
    modernize.

## Later

These should wait until the backend workflow boundaries are clearer and the
admin workflows are stable.

- Shared audit/history capability.
  - Implement a broader append-only audit/history system across domains.
  - Record actor, timestamp, domain, affected record, action code, before/after
    values, reason code, and optional notes.
  - Use it across week close, reopen, handicap apply, roster changes, schedule
    changes, and season close.

- Users screen and account management.
  - Roles and permissions are resolved at the design level (USERS-Q001
    resolved 2026-07-27). Route auth implementation is incremental; see Then.
  - Account linking (users.player_id) deferred until online score entry,
    attribution display, or a users screen creates the concrete need.
  - No email invitation workflow selected; admin-provisioned accounts only.
  - A users screen waits for route-level auth to be wired and a concrete
    account-management workflow to be defined.

- Online score entry workflow.
  - Resolve `MATCHES-Q002`.
  - Define competing edits, draft saves, permissions, review, and submission.
  - Research whether individual matchups can be processed before the full night
    is finished.
  - Current direction: only rostered players assigned to a match can submit that
    match's scores, with admin override.
  - Includes the deferred captain/player-side (Player Portal) approval
    for Weekly Score Processing (see Completed / Largely Completed) --
    every approval/processing action today is still admin-attested
    through the Weekly Summary UI until this lands.

- Simple browser-based match-entry prototype.
  - Prototype a lightweight browser match-entry screen.
  - Use it to learn whether phone-friendly/browser-based score entry is
    practical.
  - Keep this as workflow validation, not platform expansion.

- Mobile or broader client expansion.
  - Consider only after core admin workflows, backend boundaries, and API
    contracts are stable.
  - Treat any future Flutter/Dart mobile app as an API client, not a direct
    database client.
  - Plan for stable versioned API contracts, backend-authoritative rules,
    secure token storage, offline draft/conflict handling, and API contract
    tests before mobile implementation.
  - Do not treat this as an active roadmap driver yet.

- Database portability and current-schema documentation.
  - Update `doc/erd.mermaid` to match the current schema after the active
    documentation alignment pass.
  - Continue researching the longer-term production database direction while
    keeping SQLite supported for local/dev/test.
  - Reserve PostgreSQL adapter work until an explicit data-access phase calls
    for it.
  - Longer-term startup/data-access cleanup: move away from process-global
    `db.DB` toward an owned DB handle when a focused persistence phase calls
    for it.

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

- Product test readiness (2026-08-20 to 2026-09-10). Established the
  staging test path (`scripts/deploy/seed-staging.ps1`'s `-SeedFixtures`
  switch), the 24-section browser smoke-test checklist, and the practice
  of a staging verification pass after every shipped phase. The full
  2026-08-23 staging run, plus every per-phase staging verification since,
  are recorded in `doc/testing/product-smoke-test-checklist.md`. Every
  blocker and critical/medium finding from that run is fixed and
  verified (`browser-admin-auth-bridge`, `staging-seed-fixtures-option`,
  `api-client-bodyless-post-fix`, `player-stats-roster-join-fix`,
  `handicap-preview-parity`, `player-stats-winpct-roster-scope-fix`).
  Two of the remaining low-severity findings are also fixed as of
  2026-09-11 (`season-teams-error-and-rules-echo-fixes`, see below): the
  season-team name-collision raw 500 (#10) and the rules-update response
  echo (#11). The last actionable finding, Known Gap #9 ("Generate
  Schedule" undo), was closed the same day as a documented decision
  rather than a code change -- see the discovery entry below. **Product
  test readiness has no actionable item remaining**; what's left is a
  short list of low-severity findings tracked in Now's monitoring
  posture, not scheduled work. Player merge UI and default-lineup setup
  remain deferred (see Then).

- Whole-app admin screens Phase 1 (2026-08-26 to 2026-09-10). Built the
  operational admin surface across the app: Users Admin Screen Phase 1,
  Player Overview Phases 1-3, Weekly Summary Phase 1, Financial Phase 1,
  Substitute Workflow Phase 1, Player Account Access Phase 1
  (`role=player`), League Communication Screen Phase 1, and the League
  Admin hub (Phase 1) -- each with its own detailed entry below and its
  own domain README. Real player login/sessions, automated email/SMS
  notifications, message history/audit trails, and payment
  editing/voiding are all explicitly deferred across every one of these
  phases. Any further admin screen is a new, separately-scoped item, not
  a reopening of this lane.

- Incremental route-level auth for admin mutations (Phases 1-6,
  2026-07-28 to 2026-08-08). All clearance, schedule-mutation,
  match-mutation, season-setup-mutation, global league/player/team CRUD,
  and `POST /api/backup` routes are gated by personal-key + role auth
  (`league_admin`/`admin`/`system_admin`, with backup restricted to
  `system_admin`/`admin`). No unprotected admin mutation route remains
  from this rollout; `handicap-apply` keeps its dual-tier
  `requireApplyAuth` (personal key + static token fallback) by design,
  with no change planned until a focused attribution/auth cleanup phase.
  Full per-phase detail remains in the Then section's "Roles,
  permissions, and API access implementation."

- Season-teams error and rules-echo fixes (2026-09-11). Fixed two
  low-severity correctness/clarity bugs from the smoke checklist (Known
  Gaps #10 and #11) that made season setup harder to test and explain.
  Backend-only, no schema/auth change:
  - #10: `POST /api/seasons/{id}/teams` with `name` returned a raw 500
    with a leaked SQLite constraint message when a same-named standalone
    team already existed in the league (`teams(league_id, name)` is
    `UNIQUE`). `SeasonService.AddTeam` now recognizes the constraint
    error (the same `strings.Contains(err.Error(), "UNIQUE")` pattern
    `lineup_service.go` already uses for its own UNIQUE-constraint
    mapping) and returns `domainerr.Conflict` (`SEASON_TEAM_NAME_TAKEN`),
    so the handler's existing `mapSeasonErr` now correctly returns a
    friendly 409 naming the conflicting team.
  - #11: `PUT /api/seasons/{id}/rules/{rid}` stored the update correctly
    but echoed `season_id:0, rule_key:""` in its response body, since the
    PUT body only carries `rule_label`/`rule_value` (the client never
    sends `season_id`/`rule_key`) and the handler echoed the
    caller-constructed request struct as-is. `RuleService.Update` now
    returns the updated `models.SeasonRule` -- it already fetched the
    existing row to validate the new value against the rule's real key,
    but previously discarded it -- so the handler returns the real,
    accurate row. Stored-row behavior, validation, and error mapping are
    unchanged.
  Verified with `go test ./... -count=1` and `go build ./...` (2 new
  `SeasonService`/`RuleService` unit tests plus 2 new end-to-end handler
  tests, one per fix, all passing; zero regressions). `RuleManager.Update`
  gained a return value (`(models.SeasonRule, error)` instead of `error`),
  a small, contained interface change touching only the one production
  call site and two test-only no-op stubs. See
  `doc/testing/product-smoke-test-checklist.md`'s Known Gaps #10/#11 for
  full before/after detail.

- Generate Schedule undo -- discovery and decision (2026-09-11). Known
  Gap #9 asked whether `Generate Schedule` needs an undo path (no
  `DELETE` short of removing the whole season/league). Discovery
  (`schedule-generate-undo-discovery`) found that `POST
  /api/matches/generate` already *is* the undo path:
  `ScheduleService.GenerateSchedule` -> `SaveGeneratedSchedule` deletes
  and replaces every unplayed (`completed=0`) match on every call,
  unconditionally, while completed/approved/processed matches are never
  touched (excluded from the delete by `completed=0` in the SQL, and
  further protected by the season-closed / has-closed-weeks /
  active-with-completed-matches guards `GenerateSchedule` already
  enforces). The frontend already confirms this in plain language before
  calling it (`web/domains/seasons/seasons-domain.js`). **Decision:** no
  new `DELETE /api/matches/{id}` route or broad undo feature -- the
  existing recovery path is sufficient: regenerate the schedule (with
  corrected inputs) any time before scores/round results are entered for
  the matches being replaced -- not merely before they reach completed/
  approved/processed state, since regeneration deletes every unplayed
  (`completed=0`) match unconditionally and would cascade-delete a
  partial/incomplete round_results row along with it -- or use a
  disposable season/league for staging scenarios needing repeated
  schedule churn against real seeded data (the working convention every
  staging pass has already followed since 2026-08-23, including this
  session's own). Deleting the whole season/league remains available as
  the full-wipe option. No real workflow has hit a
  need beyond this in the month since the gap was first noted -- only
  the smoke test's own test-data setup did, and that was already solved.
  A narrow single-match delete (removing one erroneous/duplicate match
  without regenerating the whole season) was considered and is a
  reasonable future candidate if a real workflow needs it, but is
  explicitly not part of the current product-readiness lane -- not
  implemented, no branch opened for it. This closes Known Gap #9 and
  leaves Now with no actionable product-test-readiness item; see
  `doc/testing/product-smoke-test-checklist.md`'s Known Gap #9 for full
  detail.

- Season setup polish: default lineup week-filter fix + setup checklist
  warning (2026-09-16). Discovery for the roadmap's "Season setup
  polish -- default lineup setup" item found that a full Default Lineup
  editor already existed on the Lineups screen (`week_number=0`, used
  consistently by Dashboard, Match Entry, and the League Admin hub) --
  no new screen was built. Discovery also found a real, live correctness
  bug: `ListLineupPlansRequest`'s `WeekNumber=0` "no filter" convention
  collided with `week_number=0`'s real meaning (the Default Lineup),
  silently mixing week-specific rows into "default" reads once a team
  had both -- confirmed against real staging data from an earlier
  discovery pass (a `week_number=0` request returned 60 rows for a
  5-week season, i.e. every week combined, not a genuine ~12-row default
  set). **Fixed:** `backend/storage/sqlite/lineup_store.go`'s
  `ListLineupPlans` now always filters on `WeekNumber` (no schema
  change, no API-shape change, no frontend caller needed to change,
  since every real caller already passed an explicit week number).
  **Added:** a non-blocking `TEAM_NO_DEFAULT_LINEUP` warning in `GET
  /api/seasons/{id}/checklist`'s existing `warnings` array for any
  season team with fewer than 3 default-lineup rows -- composed at the
  handler layer (`handlers/api_season_teams_handlers.go`) to keep
  `SeasonService` free of a lineups-domain dependency, using the
  existing `Warnings []ChecklistItem` shape so the Seasons screen's
  existing generic warning renderer displays it with no frontend change.
  Warning only -- never a blocker; `can_activate` is unaffected; Close
  Week, activation, and schedule generation still do not depend on
  future lineups. Verified with `go test ./... -count=1` and `go build
  ./...` (1 new store test proving the exact bug is fixed, 2 existing
  store tests corrected since they unintentionally relied on the removed
  "no filter" behavior, 1 new handler test proving the warning appears/
  disappears/never blocks/never affects `can_activate`; all pre-existing
  checklist tests pass unchanged). Explicitly out of scope, per
  discovery and PM decision: no new default-lineup screen, no
  link/button from the warning to Lineups (skipped as unnecessary UI
  polish), no "exactly 3" enforcement on the write side, no guard on
  substitute endpoints against week-0 rows, no consolidation of the
  three independent frontend fallback implementations, no auth/schema
  change. See `doc/domains/matches/README.md`'s "Default Lineup
  Week-Filter Fix + Setup Checklist Warning" for full detail.

- Season-end clearance (Phases 1-3, shipped 2026-07-26).
  - Close preview endpoint and close commit endpoint.
  - Final standings snapshot (`final_standings_snapshot` JSON, versioned, preserved on reopen).
  - `closed_at` lifecycle marker; Closed badge and Close Season button in Seasons UI.
  - Closed-season edit locks: `SEASON_CLOSED` (409) guards on all mutation endpoints
    (scoresheet, week workflow, schedule, match assignment, season setup, handicap apply).
  - Explicit admin reopen: `POST /api/seasons/{id}/reopen` clears `closed_at`, returns
    season to Historical state (active=0, activated_at preserved); `SEASON_NOT_CLOSED`
    (409) when season is not closed; Reopen Season button in management panel.
  - Paid/unpaid player status remains outside the app.

- Handler route split (Phases A-D, 2026-08-08 to 2026-08-09).
  - Route registration for every sizeable shared route family was extracted
    out of `handlers/api.go` into focused `handlers/api_*_routes.go` files,
    while keeping `handlers.Register` as the sole public entry point and
    preserving every URL, auth wrapper, nil-manager guard, and handler call.
  - Phase A: global league, player, and team CRUD route registration.
  - Phase B: season setup (CRUD, activation, rules, skipped-weeks,
    bye-requests, season teams/roster) and season close/close-preview/reopen
    route registration.
  - Phase C: schedule generation, schedule pushback preview/apply, lineup
    plan, and week-workflow route registration.
  - Phase D: match read/assignment, match results, rounds, standings, and
    player-stats route registration.
  - Remaining intentionally inline route registration in `handlers/api.go`:
    small special-case routes with distinct auth wiring (rule definitions,
    handicap recommendations/apply, user management, backup) -- these were
    never split into `api_*_routes.go` files since each has unique
    conditional/auth wiring not worth extracting on its own. This closure
    covered route registration only; handler logic ownership was a
    separate, subsequent effort -- see "Handler logic ownership extraction
    (Phases A-L)" below.

- Handler logic ownership extraction (Phases A-L, 2026-08-12 to 2026-08-18).
  - Handler function bodies were extracted out of `handlers/api.go` into
    focused `handlers/api_*_handlers.go` files, one domain/context per
    file, completing the split that "Handler route split (Phases A-D)"
    above started for route registration. This is pure code motion and
    handler-file ownership cleanup, not a service/domain extraction --
    route behavior, auth policy, JSON shapes, and domain-service
    boundaries are all unchanged. Business logic already lived in domain
    services before this series began; only the thin HTTP delegator
    functions moved.
  - Phase A: league/player/team CRUD handlers.
  - Phase B: schedule/pushback handlers.
  - Phase C: lineup handlers.
  - Phase D: match core handlers.
  - Phase E: match results/rounds/standings/player-stats handlers.
  - Phase F: week workflow handlers.
  - Phase G: season core handlers.
  - Phase H: season rules handlers.
  - Phase I: skipped-weeks and bye-request handlers.
  - Phase J: season teams/roster/available-player/previous-season/
    checklist handlers.
  - Phase K: handicap recommendation/apply handlers.
  - Phase L: users handlers.
  - Remaining in `handlers/api.go`: `Register()`; the shared request/
    response helpers (`jsonOK`, `jsonError`, `jsonValidation`, `pathID`,
    `qparam`, `qparamInt`, `decode`); the auth middleware
    (`requireAdminToken`, `requireApplyAuth`, `requirePersonalKeyAuth`,
    `requireLeagueAdminRole`, `requireSystemAdminRole`, `clearanceAuth`,
    `systemAdminAuth`, and their context helpers); the backup route
    closure; the rule-definitions handler; and two pre-existing orphaned
    section headers (`Leagues`, `Matches`) left over from Phases A and D
    of the route-registration split, intentionally not cleaned up since
    they predate this series.

- Workflow ownership clarification -- score-save and season close
  (2026-08-10 to 2026-08-12).
  - Score-save eligibility: `RosterEligible` remains a handler-level
    cross-domain pre-TX guard; `SaveRounds` stays in `matches`;
    `RosterEligible` stays in `seasons`. No workflow layer introduced.
    Current roster-eligible-vs-week-closed precedence is pinned by
    `TestSaveRounds_WeekClosedAndRosterShort_RosterEligibleWinsPrecedence`.
    See `doc/domains/matches/README.md` "RosterEligible ownership
    decision."
  - Season close: policy stays in `seasons.computeClose`, shared by both
    `ClosePreview` and `CloseSeason` so preview and commit cannot drift.
    The handler gathers weeks (`WeekManager.ListWeeks`) and standings
    (`RoundManager.GetStandings`) only -- it makes no close-policy
    decisions. No workflow layer introduced. Final-standings-snapshot
    persistence and reopen preservation are covered end-to-end by
    `TestCloseSeason_SnapshotPersistedAndPreservedOnReopen`. See
    `doc/domains/seasons/README.md` "Season close ownership decision."
  - Both decisions conclude the two starter cases named in this roadmap
    item; no workflow/application layer was added in either case.

- Small operations hardening pass (Phases B-C, 2026-08-12).
  - Phase B: `db.Backup` now runs `PRAGMA wal_checkpoint(TRUNCATE)` before
    copying `league.db`, so API-triggered backups are WAL-safe. Staging
    deploy backups (`scripts/deploy/staging-common.ps1`) copy any
    `league.db-wal`/`league.db-shm` sidecar files alongside the timestamped
    backup when present, and `Restore-StagingDatabase` restores matching
    sidecars, so both backup paths are safe to restore from even if the app
    was not shut down cleanly.
  - Phase C: `GET /healthz` (unauthenticated, registered in
    `handlers/api_health_routes.go`) returns 200 with `{"status":"ok"}`
    when the database connection is reachable, or 503 when it is not.
    Basic request logging with request IDs (`request_logging.go`) wraps
    the HTTP handler in `main.go`: every request gets an `X-Request-Id`
    (read from the incoming header when present, otherwise generated),
    echoed back on the response and included in a server log line
    alongside method, path, status, and duration. No request bodies,
    query strings, or auth headers are logged.
  - No new router framework, logging dependency, or metrics/tracing
    library was added. No graceful shutdown or IIS deployment changes were
    made -- deferred, not needed for this slice.

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
  Handler files now form the thin HTTP delegation layer for most routes;
  `handlers/api.go` retains `Register()`, shared helpers/auth, backup, and
  rule definitions.
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
- Schedule preview policy and enforcement. Close Week blocked for draft seasons
  (`WEEK_CLOSE_SEASON_DRAFT`, 409); regeneration blocked for active seasons
  once any match is completed (`SCHEDULE_ACTIVE_HAS_COMPLETED`, 409). Draft
  season UX clarified in schedule page and season management panel. Resolves
  `SCHEDULES-Q001`.
- Next-week preparation workflow clarified. Close Week does not mutate next-week
  data; advance-preview and advance-result report readiness only (match count,
  assigned count, lineup status). Operational admin workflow documented in
  doc/domains/matches/README.md. Blocking close on missing next-week lineup is
  explicitly deferred.
- Controlled-code storage decision. `CODES-Q001` resolved: behavior-driving
  codes remain developer-owned constants; DB-backed code tables and admin
  code-management screens are deferred until an admin workflow requires them.
- Player quick-add Phase 1. Players page now has a simplified quick-add modal
  using the existing player create endpoint. `PLAYERS-Q001` resolved for Phase
  1: minimum fields are at least one name plus diff rating, with optional team.
  Duplicate detection, INCOMPLETE profile status, and match-entry quick-add are
  deferred.
- Player quick-add duplicate warning (Phase A, 2026-08-19). Quick-add on the
  Players page now warns, before creating a player, when the typed name
  normalizes (trimmed, whitespace-collapsed, case-folded) to the same full
  name as an existing player in the active league. The admin can cancel or
  add the player anyway; no match means quick-add proceeds unchanged.
  Client-side only (`web/domains/players/players-page-component.js`) -- no
  backend, schema, or API change. Safe player-record merge was a separate,
  subsequent effort -- see "Player safe merge backend" below. INCOMPLETE
  profile status and match-entry quick-add remain deferred; see "Player
  record maintenance" above.
- Player safe merge backend (Phase A, 2026-08-19). Admins can merge a
  duplicate player into a surviving one:
  `POST /api/players/{source_id}/merge` with `{"target_id": <id>}`, gated by
  the same personal-key admin mutation auth as player CRUD. All nine
  supported player-ID references across seven tables (match_results,
  handicap_history, round_results home/away, lineup_plans player/sub_for,
  season_teams captain, season_rosters, teams captain) are repointed from
  source to target in one transaction, then the source player is deleted;
  any failure rolls back everything. Handicap snapshot columns and
  handicap_history values are preserved untouched -- only foreign keys move.
  Refused with nothing changed: 400 for same source/target player, 404 for a
  missing player, 409 for an unsafe collision (season-roster, round-results
  participation in the same match/round in any role, self-opponent, or
  lineup-plan) -- the round-results check covers any combination of home/away
  across rows, not just both-as-home; self-opponent, the full round-results
  participation check, and the lineup-plan check were all discovered or
  broadened during implementation. Backend and store only; no merge UI yet,
  see "Player record maintenance" above.
- Schedule pushback workflow (Phases M/N/O). Read-only preview endpoint, atomic
  apply endpoint, and Schedule page admin UI. Unplayed matches at or after the
  cutoff shift week number and date atomically; completed matches are preserved;
  closed weeks at or after the cutoff block the operation. skipped_weeks and
  bye_requests are not mutated. Audit write deferred until the broader audit
  system exists.
- Schedule page navigation and accordion polish. Restored the openMatchEntry
  bridge from Schedule to Match Entry (missing function caused a ReferenceError
  on every Score Entry click). Close Week modal match-error links now dismiss
  the modal before navigating. Week cards are collapsible: closed weeks
  auto-collapse on first season load; open weeks default expanded. Collapse
  state persists across same-season refreshes and resets when the season
  selector changes.
- Week-end recap (Phases A through D2). Read-only recap endpoint
  (`GET /api/seasons/{id}/weeks/{week}/recap`) assembles match results,
  missing-match count, player-stat deltas, applied handicap changes, warning
  acknowledgments, and next-week readiness in one response. Schedule page
  recap panel shows all sections; a Review & Apply deep-link opens the
  Handicap tab pre-filtered to the recap week so applied rows are linked back.
  Close Week remains the week-clearance boundary; no separate cleared state.
  Missing matches are excluded from standings until resolved. Deferred: team-
  level record and stat summaries, recommendation-change detail in recap,
  print/export, persisted recap snapshots.
- Browser admin auth bridge (2026-08-20). Every admin mutation route
  requires a personal-key Bearer token, but the shared frontend `api()`
  client (`web/lib/api-client.js`) never attached one -- discovered during
  the Product Test Readiness pass (`doc/testing/product-smoke-test-checklist.md`),
  where it was the top blocker to browser-based smoke testing. Added
  `web/lib/admin-key-store.js`, holding a personal key in `sessionStorage`
  (never `localStorage`) for the current browser tab; a new "Admin Key"
  button/modal in the shell sidebar to paste or clear it; and `api()` now
  attaches `Authorization: Bearer <key>` to every request when one is set,
  covering every domain screen that uses the shared client with no
  per-screen changes. 401/403 responses now surface as specific, actionable
  toasts instead of a generic error. The static `LEAGUE_ADMIN_TOKEN` still
  never appears in browser code -- it is only ever used server-side or via
  curl to bootstrap a personal-key user. Handicap Review & Apply keeps its
  own separate, already-working manual token field unchanged -- not
  migrated, since doing so was not clearly simpler and risked its existing
  retry/clear-on-403 behavior for no scope benefit. No backend auth policy
  changed.
- Staging seed fixtures option (2026-08-23). `scripts/deploy/seed-staging.ps1`
  gained an opt-in `-SeedFixtures` switch; default behavior (base seed only)
  is unchanged. With the switch, it runs
  `--seed-scoresheet-fixtures --fixture-weeks all` against the same staging
  executable and data directory immediately after the base seed succeeds
  (not `go run .`, so fixture data always comes from the exact binary
  already deployed to staging, never a possibly-different local checkout),
  and verifies the fixture league appears via the API before reporting
  success. A fixture-seed failure rolls back the same way a base-seed
  failure already did. This was the last item blocking a full Product Test
  Readiness pass on staging; see `doc/testing/product-smoke-test-checklist.md`.
  Discovered separately, out of scope for this branch: the
  `.codex/skills/deploy-staging/scripts/` mirror of these staging scripts
  has drifted out of sync independent of this work and does not have this
  switch either.
- Staging product smoke pass (2026-08-23). Ran
  `doc/testing/product-smoke-test-checklist.md` against real
  `http://league-staging.local` for the first time (no browser automation
  available, so every result is API-verified via curl or explicitly marked
  NOT VERIFIED for pure rendering checks). Confirmed the core workflow
  chain works end to end against real staging data: league/team/player
  CRUD, quick-add's create call, all four player-merge outcomes, season
  create/setup/rosters/rules, skipped weeks, schedule generation, pushback
  preview/apply, lineup plans, match entry/score save, close/reopen week,
  standings, handicap-recommendation eligibility gating, week recap, and
  season close/reopen. Found one new critical, staging-only blocker
  (bodyless `POST` -- see "API client bodyless POST fix" below) plus four
  smaller product findings, all documented with full evidence in the
  checklist doc: `GET /api/player-stats` drops players not assigned via the
  legacy `players.team_id`; Week Recap's handicap preview and the
  dedicated Handicap Recommendations endpoint disagree on eligibility for
  the same season; no way to cleanly undo a generated schedule; and two
  low-severity rough edges (a leaked-SQL 500 on a season-team name
  collision, a response-echo gap on season-rule updates). Used a disposable
  sandbox league/season for schedule/lineup/match-entry/close-week flows
  rather than real seeded data, and confirmed staging was fully restored to
  its pre-run baseline afterward. Did not fix any of the bugs found (out of
  scope for that branch).
- API client bodyless POST fix (2026-08-23, implemented and verified on
  staging). `web/lib/api-client.js`'s `api()` now sends a real `'{}'` body
  for `POST`/`PUT`/`PATCH` calls when the caller passes none, instead of
  omitting the body entirely -- `GET`/`DELETE` unchanged, since the smoke
  pass confirmed bodyless `DELETE` was never affected. Fixes the critical
  finding from the staging smoke pass: IIS in front of staging rejects a
  bodyless `POST` with 411 before it reaches the Go app, breaking Backup
  DB, Season Activate/Close/Reopen, and Reopen Week regardless of the
  Admin Key. Verified locally first (sandboxed Node context with a `fetch`
  spy against the real shipped source), then verified on real staging
  after deploy: with the same request shape the fixed frontend now sends
  (Admin Key + explicit `'{}'` body), Backup DB returned 200 with a real
  backup file, and Season Activate/Close/Reopen and Reopen Week against a
  deliberately nonexistent season returned 404/500 Go-app responses
  instead of IIS 411 -- confirming the request now reaches the app. Does
  not, on its own, re-verify the broader browser click-flow for these
  buttons, only that the IIS-level rejection is gone.
- Player stats season-roster join fix (2026-08-23). Fixes the staging
  smoke-pass finding that `GET /api/player-stats` silently dropped players
  assigned to a season only via `season_rosters`/`lineup_plans` (the
  documented target model) rather than the legacy `players.team_id`.
  `GetPlayerStats`'s season-scoped query
  (`backend/storage/sqlite/round_store.go`) now resolves each player's team
  by preferring their `season_rosters` entry for the requested season, and
  only falls back to `players.team_id` when they have no roster row for
  that season -- so a stale or missing `players.team_id` no longer excludes
  or misattributes a player. The league-scoped variant of `GetPlayerStats`
  was left unchanged at this point; `season_rosters` has no league-only
  concept to fall back to directly, so that branch needed its own fix.
  **Update 2026-09-01:** fixed by `player-stats-winpct-roster-scope-fix`
  -- see Completed / Largely Completed below. Verified with two new SQLite
  store tests at the time of this original fix: one reproducing the
  exact reported shape (a `players.team_id IS NULL` player present only in
  `season_rosters`, now correctly included with the right team name and
  stats) and one confirming the season roster wins when it disagrees with
  a player's current `players.team_id` (e.g. a mid-season team change).
  Full `go test ./...` passes, including the pre-existing `GetPlayerStats`
  test that predates this fix. Result shape (`models.PlayerStat`)
  unchanged. Not yet re-verified against the actual staging environment --
  that would need a deploy.
- Handicap preview parity (2026-08-24). Fixes the staging smoke-pass
  finding that Week Recap's embedded handicap preview and the dedicated
  Handicap Recommendations endpoint disagreed on eligibility for the same
  season/players: the recap preview showed concrete recommended changes
  while the Recommendations endpoint showed the same players as
  `below_threshold`. Root cause was deeper than a missing threshold check
  -- `HandicapPreview`'s `game_diff_average` case computed from
  `match_results` match-averaged diffs with no eligibility gate at all,
  a completely different algorithm from `Recommendations`'s rack-windowed
  implied-handicap engine, so the two paths could disagree on both
  eligibility and the recommended value itself.
  `backend/domains/handicaps/service.go`'s `HandicapPreview` now calls
  `Service.Recommendations` directly for `game_diff_average` and reshapes
  the result into the existing `models.AdvancePreviewHandicap`/
  `PlayerHandicapRec` response shape, so Week Recap, Advance Preview, and
  the Handicap Recommendations tab all share one computation and one
  15-rack eligibility gate. The old `applyGameDiffCap` function is deleted
  along with the now-unused match-averaged code path.
  `manual_review`/`kicker_average_preview` handling is unchanged. One
  user-visible contract change: `PlayerHandicapRec.MatchesPlayed`
  (`matches_played`) is renamed to `IncludedRacks` (`included_racks`),
  since the shared engine counts individual eligible racks rather than
  whole matches -- updated in the JSON response, the Week Recap/Advance
  Preview table header and cell in
  `web/domains/schedules/schedule-page-component.js`, and all handler
  tests referencing the field. Verified with new backend tests proving
  parity for a below-threshold player and an eligible one against
  identical stub data, plus updated `handlers/api_handicap_test.go`
  integration tests (several needed rewriting since they previously seeded
  `match_results` rows and relied on the retired algorithm's lack of a
  threshold or season-roster requirement). Full `go test ./...` and
  `go build ./...` pass. Not yet re-verified against the actual staging
  environment -- that would need a deploy.
- Weekly Score Processing Phase 1A: match-level approval/processing
  backend foundation (2026-08-25). Resolves MATCHES-Q001. Adds two new,
  independent, admin-attested states on `matches` --
  `approved_at`/`approved_by_user_id`/`approval_note` and
  `processed_at`/`processed_by_user_id` -- sitting underneath the
  unchanged Close Week, plus four endpoints
  (`POST /api/matches/{id}/approve|process|unapprove|unprocess`) gated by
  the same `clearanceAuth` chain as other match mutations. Approving
  requires the match be scored; processing requires it be approved;
  `SaveRounds`/`SubmitResults`/`ClearResults` now reject with 409 once a
  match is approved or processed, with unapprove/unprocess as the explicit
  admin correction path (processed matches must be unprocessed before they
  can be unapproved). Processing does not write `handicap_history` and does
  not itself change any handicap -- Handicap Apply remains the only writer
  of that table. The one cross-cutting change: handicap recommendation
  eligibility (`EligibleRacks`/`ClosedWeekCount` in
  `backend/storage/sqlite/handicap_store.go`) now admits a match once it is
  processed, even before its week closes, while an explicit
  `OR week_closed = 1` preserves every existing closed-week match's
  eligibility unchanged -- `Recommendations`, `HandicapPreview`, and Apply
  all picked this up for free since they already recompute live from
  `EligibleRacks`. Backend-only: no frontend buttons, no Close Week
  behavior change (Phase 1B), no real captain/player login approval.
  Verified with new service, SQLite store, and handler integration tests
  (including an end-to-end proof that a processed-but-open-week match
  appears in `GET /handicap-recommendations`); full `go test ./...` and
  `go build ./...` pass. Not yet re-verified against staging.
- Weekly Score Processing Phase 1B: Close Week auto-processes approved
  matches (2026-08-25). Close Week's transaction now also processes every
  match that is approved but not yet individually processed
  (`approved_at IS NOT NULL AND processed_at IS NULL AND completed=1`),
  atomically with the existing `week_closed=1`/`league_weeks` writes.
  `CloseWeekRequest` gained `ProcessedByUserID`; `CloseWeekResult` gained
  `ProcessedCount`, surfaced as `processed_count` in the close response.
  A scored-but-never-approved match is skipped by the auto-process step
  (not silently processed) but does not block the close -- **Close Week's
  own validation is unchanged in this phase.** A `WEEK_MATCH_NOT_APPROVED`
  hard-block was implemented first per the original Phase 1A discovery
  recommendation, then deliberately reverted after it broke ~25 existing
  Close Week/Reopen/standings tests whose purpose is unrelated to
  approval, and because it wasn't clearly required by the actual Phase 1B
  request (which asks that unapproved matches not be silently processed,
  not that they block closing). A skipped match's handicap eligibility is
  unaffected -- it qualifies through Phase 1A's existing
  `week_closed = 1` compatibility path instead. Reopen is completely
  unchanged and preserves approval/processing state; the Phase 1A
  correction path (unprocess -> unapprove -> edit -> re-approve ->
  re-process) works identically after a reopen that followed an
  auto-process. Verified with 3 new SQLite store tests (auto-processes
  approved, skips unapproved, does not reprocess already-processed), 1
  service-level pass-through test, and 4 handler integration tests,
  plus confirmation every pre-existing Close Week/Reopen/standings test
  still passes unchanged. Full `go test ./...` and `go build ./...` pass.
  **Staging-verified 2026-08-26** -- see the Phase 1C entry below for the
  same staging-verification note pattern; Phase 1B's own pass confirmed
  auto-processing, the skip-unapproved case, `processed_count` accuracy,
  both eligibility paths isolated via a reopen, and reopen/correction
  behavior, all against real fixture data with full restoration afterward.
- Weekly Score Processing Phase 1C: frontend approval/processing UI
  (2026-08-26). Match Entry's scoresheet toolbar now shows an Approved/
  Processed status badge and admin action buttons (Approve/Process/
  Unprocess/Unapprove), each shown only when valid for the current state,
  mirroring the backend's own guards. Save/Clear are hidden once a match
  is approved or processed, replaced by an inline hint naming the exact
  correction path. Schedule shows the same status badge per match row,
  a pre-close "N approved matches will auto-process" note in the Review &
  Close modal (computed client-side from the season's already-fetched
  match list -- no new endpoint), and a new "Auto-processed" row in the
  post-close success panel using the existing `processed_count` field from
  Phase 1B. Four new API wrappers
  (`approveMatch`/`processMatch`/`unprocessMatch`/`unapproveMatch`) added
  to `match-entry-api-service.js` only, not duplicated into the schedules
  domain, since Schedule only reads fields already present on data it
  fetches for other reasons. No backend, schema, route, or auth changes;
  no real captain/player-side approval. Verified with `node --check` on
  all three changed files, `go build ./...` (unaffected, but confirmed
  clean), and a full approve -> process -> blocked-edit -> unprocess ->
  unapprove cycle exercised via curl against local dev data through a
  fresh local server build (found and cleared an unrelated stale
  `league_app.exe` process from a much earlier session that was holding
  port 8080 and silently serving pre-Phase-1C code). Actual browser
  rendering and click behavior remain **NOT VERIFIED (no browser)**.
  **Corrected 2026-08-26** after PM review: the toolbar had gated all four
  action buttons (and Save/Clear) on `seasonClosed` only, but the backend
  also rejects them when the match's own week is closed. `models.Match`
  now serializes `week_closed` (existing column, not previously exposed --
  API-shape addition, not a business-rule change) via three new SQLite
  store tests, and the toolbar now gates on `seasonClosed || weekClosed`
  with a distinct "reopen the week first" hint. Also fixed inaccurate
  "Backend-only" wording in `doc/domains/matches/README.md`'s Phase 1C
  section to "UI-only." See that doc's "Phase 1C correction" subsection
  for full detail.
- Users Admin Screen Phase 1 (2026-08-26). Discovery found the backend
  auth rollout (Phases 1-6 above) was materially further along than the
  frontend, which had zero user-facing surface for the users domain --
  no Users/Login/Profile screen, no nav entry, just a session-scoped
  "paste an API key" bridge with no visibility into who that key
  belonged to. Also found a concrete blocking gap: `POST/GET /api/users`
  accepted only the static `LEAGUE_ADMIN_TOKEN`, so a real system_admin
  could not use their own personal key to manage users at all. This
  phase: added `requireAdminTokenOrSystemAdminAuth` middleware so those
  two routes now accept either the static token (kept as a bootstrap
  path -- something has to create the first system_admin) or a resolved
  system_admin/admin personal key; added `GET /api/users/me` (any
  resolvable personal key, no role restriction) for "who am I";
  `CreateApplyUser` now takes an explicit role instead of hardcoding
  `role='admin'`, and `POST /api/users` validates new roles against
  `{system_admin, league_admin}` only (`admin` stays valid on existing
  rows as a legacy alias, but is not offered for new creation). New
  `web/domains/users/` frontend domain: a Users screen listing users and
  creating new ones with a role choice, showing the one-time API key at
  creation. The Admin Key modal now resolves the pasted key to a real
  identity via `/me` and shows "Signed in as `<username>` (`<role>`)";
  the Users nav entry is hidden unless the resolved identity is
  system_admin/admin. Explicitly deferred, per PM decision: player-facing
  profile/login, a separate League Admin screen (existing domain screens
  already serve league_admin), a separate Developer/Admin tools screen
  (Backup stays a single button), browser sessions/JWTs, password login,
  email invitations, key rotation, deactivate/edit, mobile notifications,
  and the `handicap-apply` auth-fallback cleanup. Verified with
  `go test ./...` and `go build ./...` (new focused tests for role
  validation, personal-key access, and `/me`), `node --check` on all
  changed JS files, and a full manual curl walkthrough against a local
  server build: bootstrap via static token, create a system_admin, use
  that user's own personal key (not the static token) to create a second
  (league_admin) user, confirm league_admin is rejected from create/list
  but can still read its own identity via `/me`. Actual browser rendering
  of the new Users screen and Admin Key modal identity line remain **NOT
  VERIFIED (no browser)**. See `doc/domains/users/README.md`'s "Users
  Admin Screen Phase 1 Implementation" section for full detail.
- Player Overview screen Phase 1 (2026-08-27). Discovery confirmed
  money/dues tracking does not exist anywhere in the codebase (schema,
  code, or docs), and that no endpoint could answer "this player's team
  for a season," "this player's matches," or "this player's stats"
  directly -- `GET /api/matches` has no team_id/player_id filter at all,
  and the season-roster-to-team lookup existed only internally. This
  phase: added `GET /api/players/{id}/overview?season_id={id}`
  (unprotected read; `season_id` optional, defaults to the player's
  league's active season) assembled by handler-level composition per PM
  decision -- no new players-domain service layer. Team resolution
  prefers `season_rosters` via a new one-line
  `SeasonManager.GetPlayerRosterTeam` passthrough (exposing an existing
  store lookup previously used only internally by roster-add
  validation), falling back to the player's direct `team_id`; when
  neither resolves, `team` is `null` and schedule/stats are empty rather
  than erroring. Schedule is derived by fetching the season's full match
  list and filtering on the resolved team_id (accepted at current data
  volumes, no new filter added to `ListMatches`). Stats reuse the
  existing season-scoped `GetPlayerStats` query, matched by player_id.
  Money is an explicit `{"tracked": false, "message": "..."}` placeholder
  -- no schema invented. New `<player-overview-page>` frontend screen
  (player-select dropdown, team/season header, schedule table, stats,
  current handicap, money placeholder) plus a "View Overview" button on
  each Players list row that deep-links there via a new
  `openPlayerOverview()` shell bridge, mirroring the existing
  `openMatchEntry`/`openHandicapForWeek` pattern. Explicitly deferred,
  per PM decision: real player login/self-service portal, payment
  entry/history, payout calculations, communication/notifications,
  mobile-specific layout, handicap history/trend, and multi-season
  views. Two incidental gaps were flagged during discovery as
  deliberately not bundled into this phase: `GetPlayerStats`'s `WinPct`
  field appeared never computed, and the league-scoped variant of that
  same query still dropped season-roster-only players. **Update
  2026-09-01:** the league-scoped gap was real and is now fixed (see
  `player-stats-winpct-roster-scope-fix` in Completed / Largely
  Completed below); the `WinPct` gap turned out to be a discovery-time
  misdiagnosis -- `RoundStore.GetPlayerStats`'s raw SQL never selects a
  `win_pct` column, but `RoundService.GetPlayerStats` (the method both
  this endpoint and `GET /api/player-stats` actually call, since
  handlers are wired to the service, not the store) has computed
  `WinPct = games_won/(games_won+games_lost)` as a post-processing step
  since Matches Phase B3 (2026-07-01) -- well before this discovery. So
  `win_pct` was already correct in every real response; only a
  from-scratch look at the store's SQL suggested otherwise. See that
  entry for the new end-to-end regression test that closes this out
  for good. Verified with `go test ./...` and
  `go build ./...` (five new focused tests: explicit season_id, omitted
  season_id defaulting to active season, missing player 404, a
  not-rostered player falling back to their direct team, and a player
  with no resolvable team at all), `node --check` on all six changed/new
  JS files, and a manual end-to-end walkthrough against a local server
  build with seeded demo data. Actual browser rendering of the new
  screen and the "View Overview" button remain **NOT VERIFIED (no
  browser)**. See `doc/domains/players/README.md`'s "Player Overview
  Phase 1 Implementation" section for full detail.
- Weekly Summary screen Phase 1 (2026-08-27). Discovery found that Week
  Recap (already shipped) already provided nearly everything this
  screen needed -- per-week match/player/handicap-change data, and the
  same next-week-readiness block used by Close Week's own preview --
  except per-match approval/processing status, and that it was embedded
  as an accordion panel inside the Schedule page rather than a
  standalone screen. This phase: added `ApprovedAt`, `ProcessedAt`, and
  `WeekClosed` to `models.RecapMatchRow` (API-shape-only, mirroring the
  same three fields already on `models.Match`) and updated
  `GetWeekRecapData`'s query/scan accordingly -- no new endpoint, no new
  auth, Week Recap was already an unprotected read. New
  `web/domains/weekly-summary/` frontend domain with its own nav entry:
  a season+week selector, a per-match status ladder (Unscored / Scored
  / Approved / Processed / Closed), an incomplete-week-safe note (shown
  whenever matches are still unscored, making clear the handicap/stats
  sections reflect partial data), a "Process Approved Scores" button
  that loops the existing per-match `POST /api/matches/{id}/process`
  endpoint client-side (no new bulk-processing backend endpoint, per PM
  decision), a next-week readiness card reusing the existing
  `next_week`/`next_week_number` fields verbatim, and a handicap section
  showing both the week's recorded changes and the season-wide
  recommendations preview already embedded in Week Recap. Close Week
  itself is untouched -- this screen only shows an Open/Closed badge and
  an "Open in Schedule" button reusing the existing `season-nav-request`
  event. Explicitly deferred, per PM decision: substitute
  creation/lineup workflows (`lineup_plans.is_sub`/`sub_for_id` remain
  read-only in the write path, a real, unrelated gap found during
  discovery), a real atomic bulk-process backend endpoint, and any
  payment/financial schema. Verified with `go test ./... -count=1` and
  `go build ./...` (two new focused store tests covering a fresh
  unscored match's default fields and a mixed-state week with an
  approved-only match alongside an approved+processed+closed one),
  `node --check` on all four changed/new JS files, and a full manual
  walkthrough against a local server build with a real generated
  schedule: approved and processed a real match via curl, confirming
  the recap's per-match fields, `missing_count`, next-week readiness,
  and the season-wide handicap message all update and render exactly as
  the frontend expects at each step. Actual browser rendering of the
  new screen remains **NOT VERIFIED (no browser)**. See
  `doc/domains/matches/README.md`'s "Weekly Summary Phase 1" section
  for full detail.
- Financial screen Phase 1 (2026-08-27). Discovery re-confirmed no
  financial schema, code, or endpoint existed anywhere in this codebase
  -- `models.PlayerOverviewMoney` was a static placeholder, not an
  implementation. This phase: added a new `finances` domain
  (`backend/domains/finances/{service.go,store.go}`, SQLite impl
  `backend/storage/sqlite/finances_store.go`) owning two new
  append-only history tables, `dues_payments` and `payouts` -- no
  update/delete path, mirroring `handicap_history`'s shape and insert
  pattern. "Paid" for a player/season means at least one
  `dues_payments` row exists; there is no partial-payment/balance math.
  A configurable dues amount reuses the existing `season_rules`
  freeform-key mechanism (`dues_amount`) rather than a new column, per
  PM decision. `handlers/api_finances_handlers.go` composes the full
  dues/payouts views by concatenating `SeasonManager.ListRoster` across
  every season team (no new season-wide roster method, matching Player
  Overview's own minimal-diff choice) and joining against
  `FinanceManager`'s payment/payout history and
  `RoundManager.GetStandings` for reference. Unlike every other domain, **all four
  routes (reads and writes) require `clearanceAuth`**
  (league_admin/admin/system_admin) -- per explicit PM decision, money
  data is not made public just because other domain reads are. Payout
  amounts are always admin-entered; standings are shown for reference
  only and never used to compute one automatically. New
  `web/domains/finances/` frontend domain with its own nav entry,
  hidden unless the resolved Admin Key identity qualifies (extending
  `updateIdentityUI()`'s existing gating pattern from the Users screen
  with a `canManageFinances` check matching `clearanceAuth`'s allowed
  roles exactly). Explicitly deferred, per PM decision: Player Overview
  money integration (a small, isolated Phase 2), real player-facing
  login/money view, payment editing/voiding (both tables are
  append-only by design), penalties, automated payout formulas,
  email/notifications, broader audit history, and any payment
  processor/accounting integration. Verified with `go test ./...
  -count=1` and `go build ./...` (12 new FinanceService tests, 8 new
  FinanceStore tests, 11 new handler tests covering auth enforcement on
  all four routes and both success paths), `node --check` on all four
  changed/new JS files, and a full manual walkthrough against a local
  server build with a real seeded season/roster: confirmed 401/403
  without a valid league_admin key, recorded a real dues payment and
  payout end to end, and confirmed a real bug found during this pass (a
  Go nil-slice defaulting to JSON `null` instead of `[]` for
  players/teams with no payment/payout history yet) was fixed before
  handoff. Actual browser rendering of the new screen remains **NOT
  VERIFIED (no browser)**. See `doc/domains/finances/README.md` for
  full detail.
- Player Overview screen Phase 2 (2026-08-29). Replaced the Phase 1
  money-not-tracked placeholder with real per-player season dues status
  now that Financial Phase 1's `finances` domain exists. Added
  `FinanceStore.ListDuesPaymentsByPlayer(ctx, seasonID, playerID)` (plus
  the matching `FinanceService`/`FinanceManager` passthrough) rather than
  duplicating SQL in the Player Overview handler, per PM's implementation
  guidance -- a straightforward narrowing of `ListDuesPayments`' query to
  one player. `getPlayerOverview` gained `financeMgr`/`ruleMgr`
  parameters and a new `playerOverviewMoney` composition helper: paid/
  total_paid/payment history from `ListDuesPaymentsByPlayer`, plus the
  `dues_amount` season_rules key (via `RuleManager.List`, matching
  Financial Phase 1's own convention) for display only. `financeMgr` may
  be `nil` (the shared `testServer()` test helper doesn't wire one); when
  `nil`, money falls back to the original placeholder rather than
  erroring, so none of the six pre-existing Phase 1 tests needed to
  change. Money composition uses the player's own ID directly, so an
  unrostered player still gets a real dues status. Payout display was
  considered and left out -- team-level data judged not trivial enough to
  bundle onto a player-level screen per PM's "unless trivial" guidance;
  payment entry also remains Financial-screen-only. New frontend: the
  money section renders a real Dues card (paid/unpaid badge, total paid,
  last payment date, configured dues amount) instead of the static
  warning banner, falling back to the original banner only when
  `money.tracked` is `false`. At initial ship, this route remained an
  unprotected GET even though it surfaced the same per-player money
  data Financial Phase 1 deliberately put behind `clearanceAuth` --
  flagged as open question `PLAYERS-Q002` rather than resolved
  unilaterally. **Auth-corrected 2026-08-30, resolving PLAYERS-Q002:**
  PM decided the whole route, not just the `money` field, must require
  `clearanceAuth` (league_admin/admin/system_admin) -- simpler and
  clearer than field-level auth, and this screen is admin-facing until
  real player login exists. `registerPlayerOverviewRoute` now wraps the
  handler in `clearanceAuth(applyAuth, ...)`; the nav entry is hidden
  unless the resolved identity qualifies. Under the shared `testServer()`
  test helper (no `ApplyAuth` wired), `clearanceAuth` is a passthrough
  and the route stays open, matching every other clearanceAuth-protected
  route's behavior under that same setup -- so all six pre-existing
  Phase 1 tests kept passing unchanged. **Same-day follow-up:** the
  Players list's "View Overview" row button had initially been left
  rendering unconditionally -- safe (a non-admin got the existing
  401/403 toast, not a broken page) but not matching the admin-facing
  decision. PM asked for it hidden too. Fixed by extracting the role
  check into a shared `hasFinanceAdminRole(identity)` function in
  `web/app.js`, reused by both nav entries and passed into
  `<players-page>.refresh()` as a new `canViewPlayerOverview` argument
  -- no auth logic added inside the component itself. Verified with
  `go test ./... -count=1` and `go build ./...` (1 new FinanceService
  delegation test, 3 new FinanceStore tests for the new read method, 4
  new money-behavior handler tests, 7 new auth-enforcement handler tests
  covering no-header/invalid-token/static-token/score_keeper rejections
  and league_admin/admin/system_admin success, all six pre-existing
  Phase 1 tests unchanged), `node --check` on all changed JS files
  (`web/app.js`, `players-page-component.js`,
  `player-overview-page-component.js`), and a full manual walkthrough
  against a local server build: created a league/season/team/player,
  confirmed the initial overview showed `tracked:true paid:false`,
  recorded a real dues payment through the Financial API, confirmed the
  overview updated to `paid:true` with correct `total_paid` and
  history, then set a `dues_amount` season rule and confirmed it
  appeared in the response. Actual browser rendering of the new Dues
  card and the corrected nav/row-button gating remain **NOT VERIFIED
  (no browser)**. See `doc/domains/players/README.md`'s "Player
  Overview Phase 2 Implementation" section for full detail.
- Player Stats accuracy fix: WinPct and league roster scope (2026-09-01,
  `player-stats-winpct-roster-scope-fix`). Closes out Known Gaps rows
  #12 and #13 in `doc/testing/product-smoke-test-checklist.md`.
  - **`WinPct` ("always 0") turned out to be a misdiagnosis, not a live
    bug.** `RoundStore.GetPlayerStats`'s raw SQL
    (`backend/storage/sqlite/round_store.go`) genuinely never selects a
    `win_pct` column -- but `RoundService.GetPlayerStats`
    (`backend/domains/matches/round_service.go`), the method both `GET
    /api/player-stats` and `GET /api/players/{id}/overview` actually
    call (handlers are wired to the service, not the store directly),
    has computed `WinPct = games_won/(games_won+games_lost)` as a
    post-processing step since Matches Phase B3 (2026-07-01) -- more
    than a month before the 2026-08-27 discovery that flagged this as
    an open gap. Confirmed via a new full-HTTP-round-trip regression
    test, `TestPlayerStats_WinPctComputedEndToEnd`
    (`handlers/api_weeks_test.go`), which closes a real testing gap
    (until now nothing exercised `win_pct` past the isolated
    `RoundService`-level unit test) even though no code change was
    needed to fix a live bug. No standings formula touched.
  - **League-scoped `GetPlayerStats` roster/lineup gap was real, now
    fixed.** The `req.LeagueID != 0` branch previously required
    `JOIN teams t ON t.id = p.team_id AND t.league_id = ?` -- an INNER
    JOIN on the legacy column that silently dropped any player
    assigned to a team only via `season_rosters` or `lineup_plans`
    (NULL or stale `players.team_id`). Fixed with a `league_players`
    CTE that computes eligible player IDs as the `UNION` (deduplicating
    automatically) of three sources -- direct `players.team_id` in the
    league (existing, preserved), any `season_rosters` row for a season
    in that league, or any `lineup_plans` row for a season in that
    league (covers a substitute never added to `season_rosters`) --
    joined against `players` before `match_results` is ever touched, so
    a player matching multiple sources still produces exactly one
    aggregated row rather than inflated sums. Team display name uses
    the same three-source precedence, each resolved as an independent
    scalar subquery (direct team first, then the most recent -- highest
    `season_id`, then `id` -- `season_rosters` team, then the most
    recent `lineup_plans` team) rather than a join, for the same
    no-row-multiplication reason. The season-scoped branch and the
    standings computation were not touched. Result shape
    (`models.PlayerStat`) unchanged.
  - Did not share one SQL/helper path between the season-scoped and
    league-scoped branches: season scope resolves against one known
    `season_id`, while league scope has no single season to resolve
    against and must aggregate across every season in the league, so
    the two queries stayed genuinely different shapes rather than
    forcing an artificial shared abstraction.
  - Verified with `go test ./... -count=1` (4 new SQLite store tests
    for the league-scoped fix: direct-team player still appears,
    season_rosters-only player appears, lineup_plans-only substitute
    appears, and no duplicate row for a player eligible via both
    team_id and season_rosters; the two pre-existing season-scoped
    roster tests -- `TestRoundStore_GetPlayerStats_RosterOnlyPlayer_NullTeamID`
    and `TestRoundStore_GetPlayerStats_SeasonRosterTeamOverridesStaleTeamID`
    -- continue to pass unchanged; plus 1 new end-to-end handler test
    for `WinPct`) and `go build ./...`. No JS changed -- backend/SQL
    only. Manual verification was done via the automated SQLite store
    tests (which exercise the exact same query the running server
    uses) rather than a live curl walkthrough, since reproducing a
    league-scoped roster-only player through the full API requires a
    multi-step season/roster bootstrap the store tests already cover
    more directly and deterministically.
- Substitute Workflow Phase 1 (2026-09-02). Discovery confirmed
  `lineup_plans.is_sub`/`sub_for_id` already existed in the schema and
  were readable, but every write path was hardcoded (`SaveTeamLineup`
  always inserted `is_sub=0`, never set `sub_for_id`) and no test in the
  repo exercised substitute creation at all. This phase: added two new
  `clearanceAuth`-gated endpoints, `POST`/`DELETE
  /api/lineup-plans/{id}/substitute`, that replace an existing lineup
  slot's player in place (one row per slot, matching what Match Entry's
  auto-fill already expects) rather than adding a parallel row.
  `LineupService` gained a second constructor dependency, a narrow
  `MatchLockChecker` interface (`IsSeasonClosedForMatch`, `IsWeekClosed`,
  `GetMatchApprovalState`) that `RoundStore` already satisfies
  structurally, so no second store had to be built -- callers pass the
  same `RoundStore` instance already constructed for `RoundService`.
  Rejected with 409 when the team's match for that season/week is
  season-closed, week-closed, approved, or processed -- the same lock
  set score edits respect, since a substitute swap changes who is
  credited for a match just as much as a score edit would; allowed when
  no match has been scheduled yet for that team/week. Match Entry
  (`web/domains/matches/match-entry-page-component.js`) now resolves
  players (round-result reload, lineup-plan auto-fill, and the manual
  "Confirm Tonight's Lineup" picker) against the full player list
  instead of the team-filtered roster arrays, since a substitute's
  player_id may not carry the team's team_id -- the old team-filtered
  lookups would have silently broken auto-fill for a substituted slot.
  The picker's dropdowns now offer every league player, grouped "This
  Team" / "Other Players (Substitute)". The scoresheet's roster table
  gained a small "Sub" button per slot (shown only when that slot has a
  known `lineup_plans` row and scores are still editable) opening a
  shared modal, plus a "Sub for X" badge with an "Undo" link once
  substituted. Weekly Summary's `GetWeekPlayerStats` gained
  `is_sub`/`sub_for_name` fields via one additional `LEFT JOIN` on the
  same season/team/week/player key it already groups by -- data only, no
  new UI, since Weekly Summary doesn't render its `player_stats` array
  in any screen yet (adding that display would be new UI construction,
  not "showing status when data is already available"). Player
  Overview's stats section was verified (not changed) to already count
  a substitute's results correctly, since the underlying season-scoped
  `GetPlayerStats` query matches `match_results` by player_id alone with
  no team filter; its schedule section still won't show a substitute's
  one-off match for a different team, an accepted, documented limitation
  rather than the larger player-history redesign fixing it would
  require. Verified with `go test ./... -count=1` and `go build ./...`
  (12 new `LineupService` unit tests covering validation and all four
  lock checks, 8 new SQLite `LineupStore` tests, 13 new handler tests
  covering auth/lock/success paths end to end, 1 new Weekly Summary
  store test, and 1 new Player Overview stats regression test),
  `node --check` on both changed/new JS files, and a full manual
  walkthrough against a local server build: saved a real lineup, called
  the substitute endpoint, confirmed the change via a follow-up GET,
  then cleared it and confirmed the exact original row was restored.
  Actual browser rendering of the new Sub/Undo controls and the
  widened roster picker remain **NOT VERIFIED (no browser)**. See
  `doc/domains/matches/README.md`'s "Substitute Workflow Phase 1"
  section for full detail.
- Player Account Access Phase 1 (2026-09-03). Why this branch exists:
  make the app testable as more than an admin console -- a player should
  be able to use a personal key to view their own schedule, stats, and
  dues status through Player Overview, without gaining admin access.
  Added a nullable `users.player_id INTEGER REFERENCES players(id)`
  (additive migration), with a partial unique index
  (`WHERE player_id IS NOT NULL`) enforcing one account per player, since
  SQLite's `ALTER TABLE ADD COLUMN` cannot carry a `UNIQUE` constraint
  directly. This implements the one-to-one link `doc/domains/users/
  README.md` had already documented as the intended "Provisional
  Relationship" since before this phase. A new `role=player`, creatable
  alongside `system_admin`/`league_admin`, requires a valid `player_id`
  at creation; implemented as a new `ApplyAuthResolver.
  CreateApplyPlayerUser` method rather than changing `CreateApplyUser`'s
  signature, so none of `CreateApplyUser`'s 37 existing call sites across
  6 files needed to change. `GET /api/users/me` now returns `player_id`
  for a linked user. Player Overview's access rule moved from a flat
  `clearanceAuth` role allowlist to an ownership-aware handler check
  (`checkPlayerOverviewAccess`), because a `role=player` key's access
  depends on the URL path's player id, which is only available once the
  handler parses it: admin roles still view any player's overview
  unchanged; `role=player` may view only its own linked player's overview
  (403 otherwise); the static `LEAGUE_ADMIN_TOKEN` still does not
  authorize this route, as before. Users Admin screen gained a `player`
  role option with a required "Linked Player" picker and a "Linked
  Player" list column; no edit/deactivate/key-rotation was added for any
  role. Frontend added a "My Overview" nav entry (visible only to a
  resolved `role=player` identity) that opens the existing
  `<player-overview-page>` component directly on that player's own
  record via a new `lockedPlayerId` parameter (hides the player-select
  dropdown as a UX courtesy; the real access control is server-side); a
  `role=player` identity does not see Users, Financial, or Backup.
  Explicitly out of scope, per PM decision: score submission, captain
  approval, browser sessions, passwords, JWTs, email invitations, and
  mobile notifications -- this is API-key V1 player access, not the
  final login/session model. Verified with `go test ./... -count=1` and
  `go build ./...` (4 new `ApplyAuthStore` tests for the new create/
  resolve/list behavior, 3 new handler tests for player-role user
  creation validation, 1 new `/me` test, and 4 new Player Overview
  auth tests covering own-overview success, other-player 403, and
  finance/users-route rejection), `node --check` on all four changed JS
  files, and a full manual walkthrough against a local server build:
  player-role creation validation, `/me` returning `player_id`,
  own-overview success, other-player-overview 403, admin access
  unchanged, the Users list showing the linked player name, and 403
  rejection of a player key from both `GET /api/users` and
  `POST /api/backup`. Actual browser rendering of the "My Overview" nav
  entry and the Users Admin "Linked Player" picker remain **NOT
  VERIFIED (no browser)**. **PM review correction, same day:** "My
  Overview" passed the app shell's currently selected `activeSeason.id`
  through to the overview request even on the locked (`role=player`)
  path, so it could 403 whenever that season belonged to a different
  league than the linked player's own. Fixed by having
  `player-overview-page-component.js`'s `#load(forcedPlayerId)` omit
  `season_id` entirely on the locked path (`seasonId = forcedPlayerId !=
  null ? null : this.#activeSeason?.id`), letting the backend's existing
  own-league-active-season fallback apply instead; admin loads are
  unchanged. Frontend-only change -- `go test ./... -count=1` and `go
  build ./...` rerun for regression safety (pass, zero regressions),
  `node --check` rerun on the changed component and `web/app.js`. See
  `doc/domains/users/README.md`'s "Player Account Access Phase 1
  Implementation" section and `doc/domains/players/README.md`'s
  "Player Overview Phase 3" section for full detail.
- League Communication Screen Phase 1 (2026-09-06). Why this branch
  exists: the league admin already has Weekly Summary, Financial, Player
  Overview, Users, score approval/processing, and substitute workflows --
  everything needed to know what to tell a team or player, but no way to
  turn that data into a message without manually re-typing it. Added a
  new `web/domains/communications/` frontend domain and a "Communication"
  nav entry, gated by the exact same `hasFinanceAdminRole` role set
  (league_admin/admin/system_admin) the Financial and Player Overview nav
  entries already use -- reused, not reinvented, per PM's explicit
  instruction. **No new backend routes or schema were added.** Three
  message types, each a pure text-building function in
  `communication-message-generators.js` operating on already-fetched
  data: Weekly Team Summary (from `GET .../weeks/{week}/recap`, filtered
  to the selected team's match(es), including the
  Unscored/Scored/Approved/Processed/Closed status label and a status-
  appropriate reminder line); Team Dues Reminder (from `GET
  .../finances/dues`, filtered to the selected team's rostered players,
  listing paid/unpaid and collecting unpaid names); Player Summary (from
  `GET /players/{id}/overview` directly, no filtering needed). The
  generators only choose display wording for values the backend already
  computed (`has_result`/`approved_at`/`processed_at`/`week_closed`/
  `paid`) -- they do not recompute any business decision, matching the
  same division of responsibility Weekly Summary's and Financial's own
  badge rendering already use. A frontend aggregate/filter was judged
  sufficient rather than adding a new backend endpoint, since filtering
  an existing per-season response by `team_id` is a simple, non-brittle
  operation, not the kind of cross-domain stitching that would justify
  new backend surface. Season/team/player selectors are populated from
  data the shell already scopes to the active league, so (unlike Player
  Account Access Phase 1's cross-league bug) a selected team/player/
  season here can never mismatch across leagues. A Copy button uses the
  Clipboard API when available and falls back to selecting the message
  text (Ctrl+C) otherwise, since the Clipboard API requires a secure
  context and staging is plain HTTP today -- the fallback is a real path
  on staging, not a rare edge case. Explicitly out of scope, per PM
  decision: automated email sending, SMTP, SMS/mobile push, a templates
  database, message history, delivery tracking, an audit/history
  framework, and communication preferences -- this is copy/paste only.
  Verified with `node --check` on all four new files and `web/app.js`;
  `go test ./... -count=1` and `go build ./...` rerun for full regression
  safety per PM's instruction for a cross-domain screen (both pass, zero
  regressions, since no Go code was touched); the three message
  generators (pure functions, no DOM dependency) were exercised directly
  in a standalone Node script against realistic fixture-shaped data,
  confirming correct team/dues filtering and message formatting for all
  three types. Actual browser rendering of the new screen remains **NOT
  VERIFIED (no browser)**. See `doc/domains/communications/README.md`
  for full detail.
- League Admin Screen Phase 1 (2026-09-10). Why this branch exists: the
  app now has several useful admin screens, but the league-admin workflow
  is spread across many sidebar entries -- there was no single place to
  see "what needs attention now, and where do I go next." Added a new
  `web/domains/admin/` frontend domain and a "League Admin" nav entry
  (placed under Dashboard), gated by the exact same `hasFinanceAdminRole`
  role set the Financial, Communication, and Player Overview nav entries
  already use -- reused, not reinvented. **No new backend routes or
  schema.** The hub is read-only: it makes a small fixed set of existing
  GETs on load (`GET .../weeks`, `finances/dues`, and for the focus week:
  its `recap`, its `lineup-plans`, and the default `week_number=0`
  lineup-plans), each feeding exactly one card, and every card degrades to
  a plain jump link when its data is unavailable -- it never fabricates a
  count. Cards: a Season/Week strip; Weekly Score Processing (season `N/M
  scored` + `X/Y weeks closed`, plus the focus week's
  Missing/Scored/Approved/Processed/Closed ladder counts -- the ladder key
  is a deliberate duplicate of Weekly Summary's private `#matchStatus`,
  mapping the server's own status fields to labels, not recomputing
  anything); Lineups & Substitutes (`ready X/Y teams` for the focus week,
  `substitutes in use: N` -- readiness resolved the same way Match Entry
  does: week-specific plans if a team has >=3, else the default lineup,
  with the first 3 rows required to resolve to real players in the full
  league player list, since a substitute's player_id may not be on the
  team roster; corrected from a raw ">=3 rows" check after PM review);
  Money (`unpaid dues: N/M players`); Communication (static reminder of
  the three message types);
  Players & Users (link-only, Users button only for system_admin/admin);
  and a flat "Jump to" button row. "Focus week" = the lowest week with
  matches whose status is not closed, else the highest such week, else
  none. Cross-screen navigation uses a new one-line `admin-nav-request`
  shell handler (a deliberate rename of the identical
  `dashboard-nav-request` handler, kept distinct so the hub's intent
  stays self-documenting); no other shell bridge method was needed. This
  is **not** the deferred "Admin code-management screens" roadmap item --
  no controlled-code/label/display-order/active-flag editing. Explicitly
  out of scope, per PM: any write action, automated notifications, an
  audit/history framework, developer/system tools consolidation (Backup
  stays separate and is not linked), new roles/permissions/sessions, and
  payment editing. Verified with `node --check` on all four new/changed
  JS files; `go test ./... -count=1` and `go build ./...` rerun for full
  regression safety per the cross-domain-screen convention (both pass,
  zero regressions, no Go code touched); the focus-week selection and
  every card's count computation were simulated in a standalone script
  against real staging season 6 data, matching the underlying endpoint
  responses (focus week 1, ladder `2 missing / rest 0`, season `8/10
  scored` / `0/5 weeks closed`, lineups `4/4 teams` / `0 subs`, dues
  `10/12 unpaid`). PM review before commit caught the Lineups &
  Substitutes card treating "3+ lineup_plans rows" as ready -- corrected
  to Match Entry's week-specific/default resolution with first-3 rows
  required to resolve to real players in the full league player list;
  `refresh()` gained an `allPlayers` param and `#load()` now also fetches
  the `week_number=0` default lineup; re-simulated against staging with
  the same result. Actual browser rendering of the hub remains **NOT
  VERIFIED (no browser)**. See `doc/domains/admin/README.md` for full
  detail.

## Open Questions To Resolve

| ID | Area | Question |
| --- | --- | --- |
| `RULES-Q001` | Rules | How are emergency or mid-season rule amendments handled? |
| `MATCHES-Q002` | Matches | How will online score entry, permissions, drafts, individual matchup processing, and review work? |

## Resolved Questions

| ID | Area | Resolution |
| --- | --- | --- |
| `USERS-Q001` | Users | Resolved 2026-07-27 - Admin-provisioned accounts; two-role model (system_admin, league_admin); personal API keys continue; player link deferred; route auth wires incrementally per phase. **Update 2026-09-03:** the deferred player link is now implemented as a third role, `role=player`, in Player Account Access Phase 1 -- still on the same API-key bridge, not browser sessions/JWTs. |
| `MATCHES-Q001` | Matches | Resolved 2026-08-25 - Two new admin-attested match-level states (`approved`, `processed`) added underneath Close Week, not a single review status; processed matches count toward handicap eligibility before week close; real captain/player login approval deferred. See Weekly Score Processing Phase 1A. |
| `PLAYERS-Q001` | Players | Resolved 2026-07-14 - Phase 1 quick-add uses at least one name, diff rating default 0, and optional team; duplicate detection and INCOMPLETE status deferred. |
| `PLAYERS-Q002` | Players / Finances | Resolved 2026-08-30 - Player Overview is protected with `clearanceAuth` while it exposes dues/payment status. Player-facing access to a player's own money/stat/schedule view remains deferred until real player login/permissions exist. **Update 2026-09-03:** player-facing access to one's own Player Overview is now implemented via `role=player` (Player Account Access Phase 1) -- still API-key V1, not full login/permissions. |
| `CODES-Q001` | Codes | Resolved 2026-07-14 - behavior-driving codes remain developer-owned constants; DB-backed code tables deferred. |
| `SCHEDULES-Q001` | Schedules | Resolved 2026-07-13 - preview policy and enforcement complete. |

## Parking Lot

Use `doc/todo.md` for private, out-of-band notes that should not interrupt
the current conversation. Promote items from that parking lot into this roadmap
or a domain README only when they become real planned work.
