# League App Roadmap

**Status:** working roadmap
**Last reviewed:** 2026-08-23

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

These items should stay small enough to review and ship independently.

- Product test readiness.
  - Establish a realistic staging test path that exercises most current admin
    workflows end to end, rather than continuing immediately into deeper
    feature polish.
  - Create or refresh a known test league/season dataset that supports setup,
    scheduling, lineup planning, score entry, close/reopen, standings,
    handicap review/apply, recap, season close/reopen, backup, and health
    checks.
  - Write a browser smoke-test checklist with clear pass/fail checkpoints for
    the full admin journey.
  - Run the checklist against staging, record blockers, and promote only the
    highest-value gaps into focused follow-up branches.
  - Treat Player merge UI, default lineup setup, and other polish as follow-up
    candidates after this readiness pass identifies what most blocks testing.
  - The checklist's top blocker -- the browser could not perform any admin
    write except Handicap Apply -- is resolved; see "Browser admin auth
    bridge" in Completed / Largely Completed below.
  - The remaining staging data blocker is also resolved: `seed-staging.ps1`
    now has an opt-in `-SeedFixtures` switch to load the scoresheet fixtures
    right after the base seed, so match-entry/close-week/standings/
    handicap/recap have data to exercise on staging without generating a
    schedule by hand; see "Staging seed fixtures option" below.
  - The checklist was run against real staging 2026-08-23; see "Staging
    product smoke pass" below for the full results. It found one new
    critical, staging-only blocker (bodyless `POST` calls -- Backup,
    Season Activate/Close/Reopen, Reopen Week -- rejected with IIS 411
    regardless of the Admin Key) plus several smaller product findings.
    The critical one is fixed and verified on staging as of 2026-08-23
    (`api-client-bodyless-post-fix`) -- all five affected routes now reach
    the Go app instead of IIS 411ing. See Completed / Largely Completed
    below for both entries and `doc/testing/product-smoke-test-checklist.md`
    for full detail on every other finding, which remain open.

- Domain and data-access restructuring.
  - Major domains (matches, handicaps, seasons, leagues, players, teams) have
    purpose-built service/store layers. All CRUD handlers delegate to domain
    services; no direct DB access remains in production handler code.
  - Continue moving workflow UI out of `web/app.js` into domain-owned frontend
    modules. Seasons, skipped-weeks, bye-requests, and season CRUD are extracted.
    Remaining `web/app.js` content is shell-level event wiring.
  - Keep backend/domain/store/adapter boundaries explicit and purpose-built for
    any new work added.

- Incremental route-level auth for admin mutations is complete.
  - Phases 1-5 protect clearance, schedule mutation, match mutation, season
    setup mutation, and global league/player/team CRUD routes with
    personal-key role auth (league_admin, admin, system_admin).
  - Phase 6 protects `POST /api/backup` with a stricter system-admin-only
    check (system_admin, admin; league_admin rejected).
  - Keep `handicap-apply` static-token fallback unchanged until a focused
    attribution/auth cleanup phase.

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

- Resolve product test-readiness findings.
  - Fix only the blockers and high-friction gaps discovered by the staging
    smoke-test pass.
  - Prefer small branches that make the existing product easier to test,
    explain, and recover from.
  - Keep rare repair workflows and broader platform expansion out of the way
    unless the readiness pass proves they are blocking real testing.

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
  - Browser sessions and JWTs are deferred until online score entry or a users
    management screen creates the concrete need.

- Player record maintenance.
  - Build a merge UI (preview, confirm) on top of the safe-merge backend after
    product test readiness work confirms this repair path is worth bringing
    into the tested admin surface.
  - Defer INCOMPLETE profile status and close-week blocking until match-night
    quick-add or admin review creates a concrete need.
  - Duplicate detection for player quick-add and the safe-merge backend
    shipped as Phase A; see Completed / Largely Completed below.

- Season setup polish.
  - Explore default lineup setup during season creation or immediately after
    season creation, without making Close Week depend on future lineups.

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

## Open Questions To Resolve

| ID | Area | Question |
| --- | --- | --- |
| `RULES-Q001` | Rules | How are emergency or mid-season rule amendments handled? |
| `MATCHES-Q002` | Matches | How will online score entry, permissions, drafts, individual matchup processing, and review work? |

## Resolved Questions

| ID | Area | Resolution |
| --- | --- | --- |
| `USERS-Q001` | Users | Resolved 2026-07-27 - Admin-provisioned accounts; two-role model (system_admin, league_admin); personal API keys continue; player link deferred; route auth wires incrementally per phase. |
| `PLAYERS-Q001` | Players | Resolved 2026-07-14 - Phase 1 quick-add uses at least one name, diff rating default 0, and optional team; duplicate detection and INCOMPLETE status deferred. |
| `CODES-Q001` | Codes | Resolved 2026-07-14 - behavior-driving codes remain developer-owned constants; DB-backed code tables deferred. |
| `SCHEDULES-Q001` | Schedules | Resolved 2026-07-13 - preview policy and enforcement complete. |

## Parking Lot

Use `doc/todo.md` for private, out-of-band notes that should not interrupt
the current conversation. Promote items from that parking lot into this roadmap
or a domain README only when they become real planned work.
