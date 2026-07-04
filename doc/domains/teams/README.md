# Teams

## Overview

**Owner:** `teams`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-06-15`

Teams belong to a league. Their participation and player roster are selected
for each season rather than permanently stored on the team or player.

## Season Participation

Team participation is explicit and per-season. Before a draft season can
activate, its participating teams must be registered in `season_teams`.

Two tables implement explicit participation:

- **`season_teams`** — one row per (season, team). Stores a season-specific
  name snapshot (`season_name`) and the team's captain for that season
  (`captain_id`, must reference a player on the season roster).
- **`season_rosters`** — one row per (season, player). UNIQUE on
  `(season_id, player_id)` enforces one team per player per season.

The setup checklist (`GET /api/seasons/{id}/checklist`) verifies:
- At least 2 teams in season_teams
- Each team has ≥ 1 player on season roster
- Each team has a captain assigned and that captain is on the roster
- A schedule has been generated (not stale)

Seasons with `teams_managed=0` (the `DEFAULT` for rows created before Phase
One) are automatically exempt from checklist enforcement (legacy bypass).
New seasons created via the API receive `teams_managed=1` and are subject to
all checklist rules, including `TEAMS_TOO_FEW` when the season has no teams.

### Adding Teams to a Season

`POST /api/seasons/{id}/teams` accepts two mutually exclusive paths:

- `name` — creates a brand-new team in the league and registers it. This is
  the only path allowed for a managed season's first team addition.
- `from_team_id` + `from_season_id` — registers an existing team and copies
  its roster from the specified prior season. For managed seasons both fields
  are required; `from_season_id` must equal the immediately previous season
  (as returned by `PreviousSeason`). If the previous season was also managed,
  the team must appear in that season's `season_teams`.

**Legacy seasons** (`teams_managed=0`): `from_team_id` without `from_season_id`
is still accepted and falls back to copying `players WHERE team_id=?`.

Modifying teams marks `schedule_stale = 1` on the season when unplayed
matches already exist.

## Player Assignment

A player may have one home team per season. Match-level substitute
participation does not change the player's home team.

When assigning an existing player to a team via `PUT /api/players/{id}`, the
request body must include all fields the handler reads (`first_name`,
`last_name`, `phone`, `email`, `handicap`, `admin_hold`, `team_id`). Omitting
name fields causes them to be blanked — the API performs a full replacement,
not a patch. The frontend `confirmAssign` function sends the full player record
to prevent accidental data loss.

## Team Number

**Status:** `draft`

The `team_number` column on the `teams` table is a short display identifier
(e.g. `01`, `42`). It is included in the `GET /api/seasons/{id}/teams` response
and shown as a badge in the Teams screen.

### Phase 1 behavior (current)

`team_number` is **display-only**. Neither `POST /api/teams` nor `PUT /api/teams/{id}`
accepts or writes this field. It can only be set via direct database access.

### Phase 2 constraint

**Do not add a normal editable team-number field to `createTeam` or `updateTeam`.**
The approved future identifier is **auto-generated** in `YYYY-#####` form (a
four-digit year prefix followed by a zero-padded sequential number within that
year). Display formatting may hide the year portion and strip leading zeroes
for readability (e.g. `2026-00007` displayed as `#7`).

Implementation of auto-generation and migration of any existing manually-set
values remain deferred. Historical-import handling (bulk-assigning legacy codes
to teams from prior paper records) is also deferred pending a controlled-codes
design decision (see `CODES-Q001` in `doc/architecture-decisions.md`).

## Frontend Components

The Teams screen lives under `web/domains/teams/`. All eight files are imported
by `web/domains/teams/index.js`, which is loaded as a single `<script type="module">`.

### `<teams-page>`

**File:** `web/domains/teams/teams-page.js`
**Status:** `draft`

Coordinator component. Renders a header row (title + season selector), a viewing
banner for non-active seasons, draft-season management controls, and a two-panel
layout (list left, detail right).

**Public API:**
`refresh(leagueId, activeSeasonId)` - called by `loadTeams()` in `app.js` when the
user navigates to Teams or when the active league/season changes. Executes immediately
in this order: (1) `list.reset()`, (2) `detail.clear()`, (3) hide banner, (4) hide
draft controls, (5) clear captain editor, (6) clear roster editor, (7) `selector.load()`.
This guarantees no previous-league data remains visible during loading or if the selector
fetch fails.

**Internal state:** `#isDraft` (boolean) is derived from `activated_at` on each
`season-changed` event and reset in `refresh()`. `#selectedTeam` (SeasonTeam | null)
is set when `team-selected` fires and cleared on season change or team-level mutation.
These are used by roster mutation guards to verify the user is still on the same draft
team before refreshing the UI.

**Viewing banner:** Displayed when the selected season is not the active one.
Historical seasons show a warning-yellow banner; draft seasons show a blue-info banner.
The banner is hidden when the active season is selected or when `season` is null.

**Draft mode coordination:** When `season-changed` fires with a draft season
(`activated_at` absent), `<teams-page>` shows `<draft-team-actions>` and calls
`list.setEditable(true)` to enable per-card remove buttons. For active and historical
seasons both are hidden/disabled. `#updateDraftMode(null)` is called in `refresh()` to
ensure controls are hidden immediately on league change.

When a team is selected in a draft season, `<teams-page>` shows
`<draft-team-name-editor>`, `<draft-captain-editor>`, and `<draft-roster-editor>`. It
calls `nameEl.load(seasonId, teamId, team)`, `captainEl.load(seasonId, teamId, team)`
(passing the full SeasonTeam object so each component knows the current values), and
`rosterEl.load(seasonId, teamId)`. All three editors are cleared whenever the season
changes, the league changes, or a team-level mutation fires (since team-level mutations
deselect the current team).

**Mutation handling:**
- `team-remove-requested` event -- `<teams-page>` shows a `confirm()` dialog explaining
  the roster will also be removed, then calls `DELETE /api/seasons/{id}/teams/{tid}`.
  `#submitting` guard prevents concurrent team-level mutations. After the DELETE completes,
  the UI is refreshed only if the selected season still matches the originating `seasonId`
  and is still an unlocked draft (`!sel.activated_at`). The toast fires regardless.
- `draft-team-mutated` event -- fired by `<draft-team-actions>` after a successful add or
  copy, carrying the originating `seasonId` (captured before the await). `<teams-page>`
  calls `toast(message)` unconditionally, then runs `#afterMutation(seasonId)` only if
  the selected season still matches and is still an unlocked draft.
- `#afterMutation(seasonId)` -- refreshes the team list, clears the detail panel,
  reloads `<draft-team-actions>`, and hides/clears `<draft-roster-editor>` (no team
  is selected after a team-level mutation).
- `roster-remove-requested` event -- fired by `<season-team-detail>` when a remove
  button is clicked in editable mode. `<teams-page>` shows a `confirm()` dialog
  explaining the removal does not delete the player, then calls
  `DELETE /api/seasons/{id}/teams/{tid}/roster/{pid}`. `#rosterSubmitting` guard
  prevents concurrent roster removes. After the DELETE, the UI is refreshed only if
  the selected season and team still match the originating IDs and the season is still
  an unlocked draft.
- `draft-roster-mutated` event -- fired by `<draft-roster-editor>` after a successful
  add, carrying `{ seasonId, teamId, message }`. `<teams-page>` calls `toast(message)`
  unconditionally, then runs `#afterRosterMutation(seasonId, teamId)` only if the
  selected season and team still match.
- `#afterRosterMutation(seasonId, teamId)` -- calls `detail.showTeam(seasonId, teamId,
  #selectedTeam, true)` to re-fetch the roster with remove buttons, calls
  `draftCaptainEditor.load(seasonId, teamId, #selectedTeam)` to refresh captain choices
  (new players may now be eligible), calls `draftRosterEditor.load(seasonId, teamId)` to
  refresh available players, and calls `list.refreshCounts(seasonId, name, teamId)` to
  update the roster count badge without clearing the selection.
- `draft-captain-mutated` event -- fired by `<draft-captain-editor>` after a successful
  PUT, carrying `{ seasonId, teamId, updatedTeam, message }` where `updatedTeam` is the
  full SeasonTeam returned by the server. `<teams-page>` calls `toast(message)`
  unconditionally, then runs `#afterCaptainMutation(seasonId, teamId, updatedTeam)` only
  if the selected season and team still match.
- `#afterCaptainMutation(seasonId, teamId, updatedTeam)` -- sets `#selectedTeam =
  updatedTeam`, calls `detail.showTeam(seasonId, teamId, updatedTeam, true)` to reflect
  the new captain name, calls `draftCaptainEditor.load(seasonId, teamId, updatedTeam)` to
  re-render the select with the new captain pre-selected and Save disabled, and calls
  `list.refreshCounts(seasonId, name, teamId)` to update the captain name in the team
  card. Does not touch the roster editor.

**Hidden draft control invalidation:** `#updateDraftMode` calls `draftEl.clear()` when
hiding the component (non-draft season selected). `clear()` increments `#loadSeq` to
cancel any in-flight fetch and wipes `innerHTML` so stale controls cannot flash as
current data if the user returns to the draft before the next `load()` resolves.

### `<draft-team-actions>`

**File:** `web/domains/teams/draft-team-actions.js`
**Status:** `draft`

Rendered above the two-panel layout when a draft season is selected. Hidden for
active and historical seasons. Provides two independent forms:

**Copy Team from Previous Season**
- Fetches `GET /api/seasons/{id}/previous` and `GET /api/seasons/{id}/teams` in parallel.
- Filters the previous season's teams to exclude those already in the draft.
- Shows a select dropdown of available teams and a Copy button.
- `POST /api/seasons/{id}/teams` with `{ from_team_id, from_season_id }` — copies
  the team and its roster from the previous season.
- Explains: "Copies the team and roster from [previous season]. The roster can be
  edited after adding."
- When all previous teams are already participating, shows an informational message
  instead of the dropdown.
- When no previous season exists, explains that none is available.

**Create New Team**
- `POST /api/seasons/{id}/teams` with `{ name }` — creates a new team with an empty
  roster. Does not expose `team_number` (auto-generated, deferred).
- Explains: "Creates a new team with an empty roster. Players can be added later."

**Stale-response protection:** `#loadSeq` counter prevents a stale `/previous` or
`/teams` response from overwriting the current season's controls.

**Duplicate and concurrent submission prevention:** A single component-level
`#submitting` flag guards both Copy and Create. When either is in progress, the
other cannot start. Each submit button is also individually disabled during its
request. On failure the button re-enables; on success `draft-team-mutated` fires
and the coordinator calls `load(seasonId)` which re-renders the whole panel.

**Navigation-safe mutation origin:** The originating `seasonId` (and `from_season_id`
for copy) are captured as local constants before every `await`. Neither the POST URL
nor the `draft-team-mutated` event detail ever reads `this.#seasonId` after an await,
so navigation that calls `load()` with a different season ID cannot corrupt the
in-flight request or mislabel the event.

**Stale mutation-error suppression:** `#ctx` is a context token incremented by every
`load()` and `clear()` call. Before each mutation's first `await`, the current token
is captured as `const ctx = this.#ctx`. The catch block writes inline errors and
re-enables the submit button only when `this.#ctx === ctx`. If the user navigated to
a different draft season while a request was in-flight, `load()` will have incremented
`#ctx`, making the captured token stale. The check fails, so draft A's error can never
appear inside draft B's controls. `#submitting` remains true until the original request
settles regardless, preventing a second mutation from starting on the stale context.

**No backend validation duplication:** Required-field checks (empty team name) are
enforced client-side only for UX feedback. All other rules are enforced by the
backend and surfaced via inline error display.

**Data sources:**
- `GET /api/seasons/{id}/previous` -- previous season and its teams
- `GET /api/seasons/{id}/teams` -- current draft-season teams (for filtering)
- `POST /api/seasons/{id}/teams` -- add team (copy or create)

### `<draft-roster-editor>`

**File:** `web/domains/teams/draft-roster-editor.js`
**Status:** `draft`

Rendered below `<season-team-detail>` in the right column when a team is selected in a
draft season. Hidden for active and historical seasons and when no team is selected.
Provides an "Add Player to Roster" control.

**Add Player**
- Fetches `GET /api/seasons/{id}/players/available` — all active players not already on
  any roster in this season, ordered by last name then first name.
- Shows a select dropdown with each player's number, full name, handicap, and current
  permanent team name (when set).
- `POST /api/seasons/{id}/teams/{tid}/roster` with `{ player_id }` — adds the player.
- When all active players are already rostered, shows a "no eligible players" message
  instead of the dropdown.

**Remove Player**
Remove buttons appear in the roster table inside `<season-team-detail>` (editable mode).
They emit `roster-remove-requested`; `<teams-page>` handles the confirm dialog and the
`DELETE /api/seasons/{id}/teams/{tid}/roster/{pid}`.

**Stale-response protection:** `#loadSeq` guards the `/players/available` fetch.

**Duplicate submission prevention:** `#submitting` flag prevents a second add while a
POST is in-flight. The Add button is disabled for the duration of the request and
re-enabled on failure (same-context only; see stale mutation-error suppression below).

**Navigation-safe mutation origin:** `seasonId`, `teamId`, and `ctx` are captured as
local constants before every `await`. The POST URL and `draft-roster-mutated` event
detail never read `this.#seasonId` or `this.#teamId` after an await.

**Stale mutation-error suppression:** `#ctx` is a context token incremented by every
`load()` and `clear()` call. The catch block re-enables the button and shows an inline
error only when `this.#ctx === ctx` (same pattern as `<draft-team-actions>`).

**No backend validation duplication:** The "select a player" validation is client-side
only. All eligibility rules are enforced by the backend.

**Data sources:**
- `GET /api/seasons/{id}/players/available` -- unrostered active players for this season
- `POST /api/seasons/{id}/teams/{tid}/roster` -- add player to season roster

### `<draft-team-name-editor>`

**File:** `web/domains/teams/draft-team-name-editor.js`
**Status:** `draft`

Rendered below `<season-team-detail>` and above `<draft-captain-editor>` in the right
column when a team is selected in a draft season. Hidden for active and historical
seasons and when no team is selected. Provides a Season Team Name editor.

**Public API:**
- `load(seasonId, teamId, team)` - render the name input pre-filled with `team.season_name`.
  `team` is the full SeasonTeam object; its `captain_id` is forwarded unchanged in the PUT
  body so the endpoint never inadvertently clears the captain assignment.
- `clear()` - increment `#ctx` and wipe rendered content.

**Emits (bubbling):**
- `draft-name-mutated` - `detail { seasonId, teamId, updatedTeam, message }`.
  Fired on successful PUT. `updatedTeam` is the full SeasonTeam returned by the API so the
  coordinator can update `#selectedTeam` and re-render dependents without an extra GET.

**Edit Season Name**
- Input is pre-filled with the current `team.season_name`.
- When `team_name` (permanent name) differs from `season_name`, shows a read-only
  "Permanent name: …" hint below the input so admins know what the team is called in the
  league record.
- Save Name button starts disabled; enables only when the trimmed input value is non-empty
  and differs from the current `season_name` (unchanged guard).
- Pressing Enter in the name input also triggers save.
- Client-side validation: empty name is blocked before the request is sent.
- Backend validation: the handler trims `season_name` and rejects blank/whitespace-only
  values with HTTP 400 `"season_name is required"`. This is the authoritative guard;
  the client-side check is a UX convenience only.
- On Save: `PUT /api/seasons/{id}/teams/{tid}` with `{ season_name, captain_id }` where
  `captain_id` is taken from the current team object so captain assignment is preserved.
- On success: emits `draft-name-mutated` with the server response and the message
  `"Team name updated to \"<name>\""`.
- The coordinator re-calls `load(seasonId, teamId, updatedTeam)` after success, resetting
  the input to the saved value and disabling Save.

**No fetch on load:** Unlike `<draft-captain-editor>`, this component renders synchronously
from the `team` object already held by the coordinator — no extra API call is required.

**Stale mutation-error suppression:** `#ctx` is incremented by every `load()` and `clear()`
call. The catch block re-enables the Save button and shows an inline error only when
`this.#ctx === ctx` (same pattern as other draft editors).

**Duplicate submission prevention:** `#submitting` flag prevents a second PUT while one is
in-flight. Save button is disabled for the duration and re-enabled on failure
(same-context only).

**Navigation-safe mutation origin:** `seasonId`, `teamId`, `captainId`, and `ctx` are
captured as local constants before the mutation's first `await`. The PUT URL and event
detail never read `this.#seasonId` or `this.#teamId` after an await.

**Coordinator wiring:**
- `draft-name-mutated` event -- fired by `<draft-team-name-editor>` after a successful PUT.
  `<teams-page>` calls `toast(message)` unconditionally, then runs
  `#afterNameMutation(seasonId, teamId, updatedTeam)` only if the selected season and team
  still match.
- `#afterNameMutation(seasonId, teamId, updatedTeam)` -- sets `#selectedTeam = updatedTeam`,
  calls `detail.showTeam(...)` to update the heading, calls `draftNameEditor.load(...)` to
  reset the input with the saved value, calls `draftCaptainEditor.load(...)` (forwards the
  updated team so its PUT body stays consistent), and calls `list.refreshCounts(...)` to
  update the team card which displays `season_name`.
- `#afterCaptainMutation` also calls `draftNameEditor.load(...)` to keep the forwarded
  `captain_id` in the name editor synchronized with the just-saved captain assignment.

**Data sources:**
- `PUT /api/seasons/{id}/teams/{tid}` -- update season name and captain; returns updated SeasonTeam

### `<draft-captain-editor>`

**File:** `web/domains/teams/draft-captain-editor.js`
**Status:** `draft`

Rendered below `<season-team-detail>` and above `<draft-roster-editor>` in the right
column when a team is selected in a draft season. Hidden for active and historical seasons
and when no team is selected. Provides a Captain Assignment control.

**Public API:**
- `load(seasonId, teamId, team)` - fetch the team's season roster to populate the captain
  select dropdown, then render controls. `team` is the full SeasonTeam object; its
  `captain_id` determines the initial select value and its `season_name` is forwarded in
  the PUT body (the endpoint performs a full replacement, so omitting it would blank the name).
- `clear()` - cancel any in-flight fetch and wipe rendered content.

**Emits (bubbling):**
- `draft-captain-mutated` - `detail { seasonId, teamId, updatedTeam, message }`.
  Fired on successful PUT. `updatedTeam` is the full SeasonTeam returned by the API so the
  coordinator can update `#selectedTeam` and re-render dependents without an extra GET.

**Assign / Clear Captain**
- Fetches `GET /api/seasons/{id}/teams/{tid}/roster` to determine eligible captains.
- When the roster is empty: shows an informational message ("Add at least one player...").
- When the roster is populated: shows a select dropdown with a "No captain" option (`value=""`)
  and one option per rostered player (number + name). The current captain is pre-selected.
- Save Captain button starts disabled; enables only when the select value differs from the
  current captain (unchanged guard). Disables again when changed back.
- On Save: `PUT /api/seasons/{id}/teams/{tid}` with `{ season_name, captain_id }` where
  `captain_id` is the selected player ID or `null` ("No captain").
- On success: emits `draft-captain-mutated` with the server response and a user-readable
  message (`"<name> assigned as captain"` or `"Captain cleared"`).
- The coordinator re-calls `load(seasonId, teamId, updatedTeam)` after success, causing the
  select to re-render with the new captain pre-selected and the Save button disabled.

**Stale-response protection:** `#loadSeq` guards the roster fetch.

**Duplicate submission prevention:** `#submitting` flag prevents a second PUT while one is
in-flight. Save button is disabled for the duration and re-enabled on failure (same-context
only; see stale mutation-error suppression below).

**Navigation-safe mutation origin:** `seasonId`, `teamId`, `seasonName`, and `ctx` are
captured as local constants before the mutation's first `await`. The PUT URL and event
detail never read `this.#seasonId` or `this.#teamId` after an await.

**Stale mutation-error suppression:** `#ctx` is a context token incremented by every
`load()` and `clear()` call. The catch block re-enables the button and shows an inline
error only when `this.#ctx === ctx` (same pattern as `<draft-team-actions>`).

**Data sources:**
- `GET /api/seasons/{id}/teams/{tid}/roster` -- rostered players (captain candidates)
- `PUT /api/seasons/{id}/teams/{tid}` -- assign or clear captain; returns updated SeasonTeam

### `<season-selector>`

**File:** `web/domains/teams/season-selector.js`
**Status:** `draft`

League-scoped season picker. Fetches all seasons for a league and provides
three navigation controls:

- **Active Season button** -- quick-select the league's current active season.
  Shown as filled primary when active season is selected; outline otherwise.
  Replaced by a disabled "No active season" button when no active season exists.
- **Previous Season button** -- shortcut to the season immediately before the
  active one. Shown only when `GET /api/seasons/{activeSeasonId}/previous`
  returns a non-null season. The authoritative backend definition of "previous"
  (closest valid end date) is always used; no client-side fallback.
- **Season select dropdown** -- lists all league seasons grouped by status
  (Active, Historical, Draft). Historical seasons are sorted closest end date
  first; nulls sort last. The active season is always the default selection.

**Public API:**
`load(leagueId, activeSeasonId)` - fetch seasons for the given league; reset
selection to active season; emit initial `season-changed`. Pass `activeSeasonId=null`
when no active season exists. Call again when the league changes.

**Emits:** `season-changed` (bubbling) with `detail { season: Season | null }`.
Fired once after each `load()` completes and again on every user selection.

**Stale-response protection:** `#loadSeq` is incremented at the start of `#fetch()`.
A response from a previously selected league cannot overwrite the current league's
state. No-league responses emit immediately without fetching.

**Status derivation (client-side):**
- `s.active === true` --> Active
- `s.activated_at` present and `s.active === false` --> Historical
- `s.activated_at` absent and `s.active === false` --> Draft

**No active season:** The selector still loads and shows historical/draft seasons
in the dropdown. The default selection is null; `season-changed` fires with
`{ season: null }`, which causes `<season-team-list>` to show its "No active season"
state. Selecting a historical or draft season displays its read-only teams and roster.

**Data sources:**
- `GET /api/seasons?league_id={id}` -- full season list with status fields
- `GET /api/seasons/{activeSeasonId}/previous` -- previous season (shortcut only)

### `<season-team-list>`

**File:** `web/domains/teams/season-team-list.js`
**Status:** `draft`

Fetches and renders all teams registered in the given season as selectable cards.

**Public API:**
- `reset()` - immediately show the neutral "Select a season to view teams." state;
  increments `#loadSeq` to cancel any in-flight request; clears all stored state.
  Called by `<teams-page>` at the start of `refresh()` before the selector loads,
  so no previous-league teams appear during loading or on selector error.
- `refresh(seasonId, seasonName)` - load teams for the given season; clears selection
  and resets stored team data. Pass `null` seasonId to show the no-active-season state.
- `refreshCounts(seasonId, seasonName, preserveTeamId)` - re-fetch team data without
  showing a loading spinner; restores the visual selection for `preserveTeamId` if the
  team is still present. Called by `<teams-page>` after a roster player add or remove so
  the roster count badge in the team card updates without losing the current selection.
- `setEditable(editable)` - toggle per-card remove buttons for draft seasons without
  reloading team data. Re-renders the card list in place and restores the current
  selection visual state. No-op when `editable` matches the current value.

**Selection and removal:**
- Cards have `tabindex="0"` and `role="button"`.
- Click or Enter/Space on the card body activates selection.
- Selected card receives `.team-card--selected` and `aria-pressed="true"`.
- Dispatches bubbling `team-selected` CustomEvent with `detail { seasonId, teamId, team }`.
- The `team` object is the `SeasonTeam` entry already fetched by the list; the detail uses
  it directly to avoid a redundant API call.
- In editable mode each card shows a remove button (`.team-remove-btn`). Clicking it
  dispatches bubbling `team-remove-requested` CustomEvent with
  `detail { seasonId, teamId, teamName }` and does NOT select the card.
  Enter/Space on a focused button fires the click natively; the `#onKeydown` guard
  (`e.target.tagName === 'BUTTON'`) prevents accidental card selection.

**States rendered:**
- No active season - icon and explanation.
- Loading - spinner.
- API error - inline danger alert.
- Empty season - "No teams registered" message.
- Populated - stacked `col-12` cards with team number badge, season name, captain, roster count.

**Data source:** `GET /api/seasons/{id}/teams`

### `<season-team-detail>`

**File:** `web/domains/teams/season-team-detail.js`
**Status:** `draft`

Displays season-specific team information and the full season roster.

**Public API:**
- `showTeam(seasonId, teamId, team, editable)` - render the detail for the given team.
  `team` is the `SeasonTeam` object passed from the coordinator; only the roster is
  fetched independently. When `editable` is `true` (draft season), each roster row
  includes a remove button that emits `roster-remove-requested`.
- `clear()` - return to the initial "select a team" prompt.

**Emits (bubbling):**
- `roster-remove-requested` - `detail { seasonId, teamId, playerId, playerName }`.
  Fired when a remove button in the roster table is clicked in editable mode.
  `<teams-page>` handles the confirm dialog and the DELETE request.

**Detail displays:**
- Team number badge when `team_number` is set.
- Season-specific name (`season_name`).
- Permanent name (`team_name`) when it differs from the season name.
- Captain name or "No captain assigned".
- Roster count badge derived from `roster.length` (reflects the freshly-fetched roster,
  not the potentially-stale `team.roster_count` from the list).
- Roster table: player number, player name, handicap per row. Fields show `-` when
  absent. In editable mode a fourth "Remove" column appears with a per-row remove button.

**States rendered:**
- Initial - "Select a team to view details."
- Loading - spinner while roster is fetching.
- API error - inline danger alert.
- Empty roster - "No players on roster yet."
- Populated - team header card with roster table.

**Data source:** `GET /api/seasons/{id}/teams/{tid}/roster`
(response includes `player_number`, `player_name`, and `handicap`)

**Zero handicap:** `handicap` is always included in the response, even when its value
is `0`. `SeasonRosterEntry.Handicap` carries no `omitempty` tag so that a zero-rated
player is not silently dropped from the JSON. The component's `#playerRow` check
`(p.handicap != null)` correctly displays `0` as a badge because `0 != null` is `true`
in JavaScript.

**Stale-response protection:** Both `<season-team-list>` and `<season-team-detail>` use
a private integer sequence counter (`#loadSeq`) to prevent out-of-order responses from
overwriting newer state:

- At the start of every `#load()` call the counter is incremented:
  `const seq = ++this.#loadSeq;`
- Before committing any result to the DOM the guard checks:
  `if (seq !== this.#loadSeq) return;`
- In `<season-team-detail>`, the null path of `showTeam()` (reached by `clear()`) also
  increments the counter so that a roster fetch started for the previous team can never
  render after the selection is cleared.

## Backend Domain

**Package:** `backend/domains/teams`
**Status:** `Phase 1 complete`

### Phase 1 — CRUD extraction

Five team handlers extracted from `handlers/api.go` into the domain layer:

| Route | Handler | Notes |
|---|---|---|
| `GET /api/teams` | `listTeams` | Optional `?league_id=` filter |
| `POST /api/teams` | `createTeam` | Validates name and league_id required |
| `GET /api/teams/{id}` | `getTeam` | Returns team with embedded players |
| `PUT /api/teams/{id}` | `updateTeam` | Updates name and captain_id only |
| `DELETE /api/teams/{id}` | `deleteTeam` | Nulls player team assignments before delete |

**Boundary:** handler → `TeamManager` interface → `teams.TeamService` → `teams.TeamStore` → `sqlite.TeamStore`

### Cross-domain store operations (Phase 1, documented)

- `GetTeam` embeds a players sub-query directly in the SQLite store. This is a store-level cross-table read — the same pattern as `player_store.go` joining the teams table for league filtering. No Players domain interface is called.
- `DeleteTeam` executes `UPDATE players SET team_id=NULL WHERE team_id=?` before the DELETE. This is a store-level side effect — the same pattern as `season_team_store.go` which also writes to the teams table directly. No Players domain interface is called.
- `season_team_store.go` continues to INSERT and DELETE teams rows as part of the Seasons domain season-team workflow. That ownership is unchanged by this phase.

### Business rules

- `team_number` is not insertable via `POST /api/teams` and not updatable via `PUT /api/teams/{id}`. It is display-only at the team level; the `team_number` column is written only by direct DB operations or the season workflow.
- `CreateTeamInput` accepts `Name` and `LeagueID`. Validation requires both to be non-empty.
- `UpdateTeamInput` accepts `Name` and `CaptainID`. `team_number` is intentionally excluded.
- `getTeam` maps only `sql.ErrNoRows` to 404; other errors return 500 (replaces the original all-errors-to-404 anti-pattern).

### COALESCE guard

The `player_number` column on the `players` table is nullable. The embedded player query in `sqlite.TeamStore.GetTeam` uses `COALESCE(player_number,'')` to prevent NULL scan errors. This mirrors the same fix in `sqlite.PlayerStore`.

## Decision History

### 2026-06-08 - Make membership season-specific

**Status:** `accepted`

Explicit season participation supports teams sitting out and players changing
teams between seasons without rewriting history.
