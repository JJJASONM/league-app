# Communications

## Overview

**Owner:** `communications`
**Status:** `draft`
**Current version:** `0.2`
**Last reviewed:** `2026-09-07`

Communications is a presentation-only domain: it generates copy/paste
message text for league admins to send to teams and players by whatever
channel they already use (text, email client, group chat). It owns no
schema, no new backend routes, and no delivery mechanism -- it reads data
other domains already expose (Weekly Summary's week recap, Financial Phase
1's dues, Player Overview's aggregate) and formats it as plain text.

This is explicitly not a notifications or messaging system. There is no
automated sending, no message history, no delivery tracking, and no
per-user communication preferences. See "What Phase 1 defers" below.

## League Communication Screen Phase 1 (implemented 2026-09-06)

### Why this phase exists

The league admin already has Weekly Summary, Financial, Player Overview,
Users, score approval/processing, and substitute workflows -- everything
needed to know what to tell a team or player, but no way to turn that data
into a message without manually re-typing it. This phase closes that gap
with a small, admin-only screen that generates ready-to-copy text.

### What Phase 1 added

- New `web/domains/communications/` frontend domain
  (`communication-api-service.js`, `communication-message-generators.js`,
  `communication-page-component.js`, `communications-domain.js`) and a new
  top-level "Communication" nav entry (`#nav-item-communications`, `d-none`
  by default).
- **No new backend routes or schema.** Every message type reads an
  existing endpoint the admin's key already has access to:
  - `GET /api/seasons/{id}/weeks` and `GET /api/seasons/{id}/weeks/{week}/recap`
    (Weekly Summary Phase 1) for the Weekly Team Summary message.
  - `GET /api/seasons/{id}/finances/dues` (Financial Phase 1) for the Team
    Dues Reminder message.
  - `GET /api/players/{id}/overview` (Player Overview) for the Player
    Summary message.
  `communication-api-service.js` duplicates these as thin wrappers rather
  than importing another domain's `*-api-service.js` module directly,
  matching this codebase's existing convention of each frontend domain
  owning its own API calls even when the underlying route is shared.
- `communication-message-generators.js` holds three pure functions
  (`buildWeeklyTeamSummaryMessage`, `buildTeamDuesReminderMessage`,
  `buildPlayerSummaryMessage`) that take already-fetched data and return
  plain text. They do not compute status, paid/unpaid, or approval
  decisions themselves -- those all come from fields the backend already
  computed (`has_result`, `approved_at`, `processed_at`, `week_closed`,
  `paid`); the generators only choose display wording for values the
  server already decided, the same division of responsibility Weekly
  Summary's and Financial's own badge rendering already use. The
  Unscored/Scored/Approved/Processed/Closed label mapping is a deliberate,
  documented duplicate of `weekly-summary-page-component.js`'s private
  `#matchStatus` method (that method cannot be imported across
  components), not an independent reimplementation of the status logic.

### Message types (V1 scope)

| Type | Recipient scope | Data source |
|------|------------------|-------------|
| Weekly Team Summary | one team, one season/week | week recap, filtered to that team's match(es) |
| Team Dues Reminder | one team, one season | season dues, filtered to that team's players |
| Player Summary | one player | that player's own Player Overview |

All three are generated for the option currently selected in the screen's
Season/Week/Team/Player selectors -- there is no bulk "generate for every
team" action in V1.

### Recipients and scope

- Team messages (Weekly Team Summary, Team Dues Reminder) are generated
  once per team, covering every player on that team implicitly through the
  underlying data (the recap's match involves the whole team; the dues
  list is filtered to the team's rostered players) -- there is no
  per-player checkbox list, matching the V1 scope of "support all players
  on a selected team."
- Player messages (Player Summary) are generated for one selected player.
- Season/team/player selectors are all populated from data the app shell
  already scoped to the currently active league (`state.allSeasons`,
  `state.allTeams`, `state.allPlayers`) -- a selected team, player, or
  season can never cross into a different league's data, unlike the
  cross-league risk Player Account Access Phase 1 had to correct for its
  locked player load. No new cross-league guard was needed here for that
  reason.
- No real email addresses are used or required. The generated text
  addresses people by name only; `models.Player` has no email field wired
  into this screen. If email data is added to the players domain in the
  future, this is a V2 consideration, not implemented now.

### Copy behavior

A "Copy" button uses the Clipboard API
(`navigator.clipboard.writeText`) when available, and falls back to
selecting the generated text (so the admin can press Ctrl+C) when it is
not. The Clipboard API requires a secure context (HTTPS or `localhost`);
staging (`http://league-staging.local`) is plain HTTP today, so the
fallback path is a real, expected path on staging, not a rare edge case.
The message text also always renders in a plain `<textarea readonly>`,
so manual selection works regardless of Copy button behavior.

### What Phase 1 defers

All explicitly out of scope per PM decision, not oversights:

- Automated email sending, SMTP setup, or any delivery mechanism.
- SMS/mobile push notifications.
- A templates database or template editing/management.
- Message history or delivery tracking (sent/opened/etc.).
- An audit/history framework for generated or copied messages.
- User communication preferences (opt-in/opt-out, preferred channel).
- Real email address collection or validation.
- Bulk "generate for every team" or "generate for every player" actions.
- A dedicated backend aggregate endpoint -- V1 confirmed the frontend can
  filter existing per-season responses (week recap, dues list) by
  `team_id` without brittle cross-domain stitching, so no new endpoint
  was needed.

### Verification

Frontend-only change -- no backend code was added or modified. Verified:

- `node --check` on all four new files
  (`communication-api-service.js`, `communication-message-generators.js`,
  `communication-page-component.js`, `communications-domain.js`) and on
  `web/app.js` (nav gating + `loadSection` case added).
- `go test ./... -count=1` and `go build ./...` rerun for full regression
  safety, per PM's instruction for a cross-domain screen even with no
  backend changes -- both pass, zero regressions (no Go code touched).
- The three message-generator functions were exercised directly in a
  standalone Node script (no DOM dependency, since they are pure
  functions) against realistic fixture-shaped data mirroring the real
  Fixture Scoresheet League/season/players used in prior staging passes:
  confirmed correct team-match filtering (a team not in the selected
  match is correctly excluded), correct dues filtering by `team_id`
  (a player on a different team is correctly excluded from a team's
  reminder), correct unpaid-name collection, and correct player schedule/
  stats/dues formatting from a Player Overview-shaped response.
- Actual browser rendering of the new screen (selectors, message
  generation on selection change, the Copy button's clipboard/fallback
  behavior) remains **NOT VERIFIED (no browser)** in this developer's
  tool session.

### Correction (2026-09-07, same week): stale week selector on type/season change

**PM finding, caught before commit:** the Message Type and Season
`change` handlers called `#loadWeeksIfNeeded()` (async) without awaiting
it before calling `#generate()`. This let Weekly Team Summary generate
against the previous season's week value right after a season change,
and meant switching to Weekly Team Summary from a different message type
never reloaded the Week selector for the currently selected season
before generating at all.

**Fix:** the `change` listener is now `async`; both the type-change and
season-change branches `await #loadWeeksIfNeeded()` before
`await #generate()`. `#loadWeeksIfNeeded()` also clears the Week selector
to a "Loading weeks..." placeholder (empty value) synchronously before
its own fetch, so a `#generate()` call that slipped in mid-fetch would
see an empty week value -- which `#generate()` already treats as
"nothing to generate yet" -- rather than a stale one. `refresh()`'s own
sequencing was already correct (it chains `#loadWeeksIfNeeded().then(()
=> #generate())`) and needed no change.

Frontend-only correction. `node --check` on
`communication-page-component.js`, `communication-message-generators.js`
(unchanged, rechecked for regression safety), and `web/app.js` all pass.
No Go code changed, so `go test`/`go build` were not rerun specifically
for this correction (the phase's initial full regression run already
covers this file). Actual browser confirmation of the corrected sequencing
remains **NOT VERIFIED (no browser)**.

## Decision History

### 2026-09-06 - League Communication Screen Phase 1: copy/paste messaging

**Status:** `accepted`

Added a new, admin-only Communication screen that generates copy/paste
message text for teams and players from existing Weekly Summary,
Financial, and Player Overview data. No new backend routes, schema, or
authentication model -- the screen is gated by the same
`hasFinanceAdminRole` role set (league_admin/admin/system_admin) the
Financial and Player Overview nav entries already use. No automated
sending, notifications, template storage, message history, or audit trail
were added; these are explicitly deferred to a later phase if ever
pursued. See "League Communication Screen Phase 1" above for full detail.

### 2026-09-07 - Correction: await week reload on type/season change

**Status:** `accepted`

PM review caught the Message Type and Season change handlers calling the
async `#loadWeeksIfNeeded()` without awaiting it before generating,
letting Weekly Team Summary generate against a stale week value after a
season change or a type switch. Fixed by making the `change` listener
`async` and awaiting the week reload before generation on both paths, plus
clearing the Week selector to a loading placeholder before the fetch. See
"Correction (2026-09-07, same week)" above for full detail.
