# Finances

## Overview

**Owner:** `finances`
**Status:** `draft`
**Current version:** `0.1`
**Last reviewed:** `2026-08-27`

The Finances domain owns per-player season dues payments and per-team
season payouts. It is a new domain -- before Financial Phase 1, no
financial schema, code, or endpoint existed anywhere in this codebase;
`models.PlayerOverviewMoney` (Player Overview Phase 1) was a static
placeholder (`"Dues and payouts are not tracked yet."`), not a real
implementation.

Finances is intentionally narrow: it owns only its own two tables
(`dues_payments`, `payouts`) and basic input validation. It has no Go-level
dependency on the seasons, players, or teams domains -- composing a full
dues/payouts view with player/team names, season rosters, and standings is
the handler's job (`handlers/api_finances_handlers.go`), following the same
handler-level composition convention already used by Player Overview and
Weekly Summary rather than introducing a new cross-domain service
dependency.

## Financial Phase 1 -- Dues and Payouts (implemented 2026-08-27)

### Goal

Give the league admin a working screen to record which players have paid
their season dues and what each team was paid out at season end, without
building a full accounting system. Per PM decision, this is deliberately
small: dues are paid on the first week with no penalties for being
unpaid, payout amounts are always admin-entered (never computed from
standings), and there is no partial-payment/balance math -- "paid" for a
player/season means at least one payment row exists.

### Data model

Two new append-only history tables, mirroring `handicap_history`'s shape
and insert pattern -- no update/delete path on either:

```sql
dues_payments (
    id, season_id, player_id, team_id,   -- team_id is a denormalized
                                          -- snapshot of the player's
                                          -- roster team at payment time
    amount, paid_at, recorded_by_user_id, note, created_at
)

payouts (
    id, season_id, team_id,
    amount, recorded_by_user_id, note, created_at
)
```

`recorded_by_user_id` is nullable with no FK constraint, matching the
existing attribution columns `handicap_history.applied_by_user_id` and
`matches.approved_by_user_id`.

A configurable dues amount uses the existing `season_rules` freeform-key
mechanism (`rule_key: "dues_amount"`) rather than a new column -- per PM
decision, no dedicated seasons field was added in Phase 1. It is
informational display only in the dues response; nothing validates a
payment against it.

### Backend

New domain: `backend/domains/finances/{service.go,store.go}`, SQLite impl
`backend/storage/sqlite/finances_store.go`, following the `teams` domain's
file-set shape exactly (service wraps a store interface, validates input
via `domainerr`, no ORM). `FinanceService` validation is per operation:

- `RecordDuesPayment` validates `season_id`, `player_id`, `amount > 0`,
  and `paid_at` non-empty. It does not take or validate `team_id` --
  that is resolved and denormalized by the handler from the player's
  season roster entry (see below) before the store call.
- `RecordPayout` validates `season_id`, `team_id`, and `amount > 0`.

Neither method validates that the player is rostered or the team is a
season team -- that check happens in the handler, which already has the
roster/season-team data loaded for composition (see below).

`FinanceStore`'s SQLite queries join `players`/`teams` directly in SQL for
display names -- a storage-layer convenience already used throughout this
package (e.g. `MatchStore`, `WeekStore`), not a Go-level dependency on
another domain package.

### Handler composition

`handlers/api_finances_handlers.go` composes the full dues/payouts views:

- **Dues**: `financeSeasonRoster` (new handler helper, not a new
  `SeasonManager` method) concatenates `SeasonManager.ListRoster` across
  every team from `SeasonManager.ListSeasonTeams` to get every rostered
  player for the season -- the same minimal-diff choice Player Overview
  made for its own team resolution, rather than adding a season-wide
  roster method. Each roster entry is matched against
  `FinanceManager.ListDuesPayments` by `player_id` to compute `paid`/
  `total_paid`/payment history. `season_rules`' `dues_amount` key (via
  the existing `RuleManager`) is included for display only.
- **Payouts**: every `SeasonManager.ListSeasonTeams` row is matched
  against `FinanceManager.ListPayouts` by `team_id`, plus
  `RoundManager.GetStandings` for the reference-only standing shown
  alongside (never used to compute a payout).
- `POST .../dues-payments` requires the player to be found in
  `financeSeasonRoster`'s result (404 `"player is not rostered for this
  season"` otherwise) and denormalizes that entry's `team_id` onto the
  stored payment row.
- `POST .../payouts` requires `team_id` to be one of
  `ListSeasonTeams`' results (404 `"team is not part of this season"`
  otherwise).
- `recorded_by_user_id` on both writes comes from the existing
  `approvingUserID(r)` helper (already used by match approval), which
  reads the resolved user from `clearanceUserFromContext` -- no new
  attribution helper was needed.

### Routes and auth

| Route | Method | Auth |
|-------|--------|------|
| `/api/seasons/{id}/finances/dues` | GET | `clearanceAuth` (league_admin/admin/system_admin) |
| `/api/seasons/{id}/finances/dues-payments` | POST | `clearanceAuth` |
| `/api/seasons/{id}/finances/payouts` | GET | `clearanceAuth` |
| `/api/seasons/{id}/finances/payouts` | POST | `clearanceAuth` |

**All four routes are protected, including both GETs.** This is a
deliberate departure from every other domain's convention (players,
teams, standings, and even the new Player Overview/Weekly Summary reads
are all open) -- per explicit PM decision, money data is not made public
just because other domain reads are. `FinanceManager` is optional in
`handlers.Dependencies` (routes registered only when non-nil, matching
`MatchManager`/`RoundManager`'s pattern) rather than required, so adding
it did not require touching the dozens of existing tests that construct
`Dependencies` without a `FinanceMgr` field.

### Response shapes

```json
// GET /api/seasons/{id}/finances/dues
{
  "season_id": 2,
  "dues_amount": "25",
  "players": [
    {
      "player_id": 3, "player_name": "Remy Cole", "player_number": "13",
      "team_id": 1, "team_name": "Rack Attackers",
      "paid": true, "total_paid": 25,
      "payments": [
        { "id": 1, "amount": 25, "paid_at": "2026-01-05T00:00:00Z",
          "recorded_by_user_id": 1, "note": "cash at first week",
          "created_at": "2026-08-27T..." }
      ]
    }
  ]
}

// GET /api/seasons/{id}/finances/payouts
{
  "season_id": 2,
  "teams": [
    {
      "team_id": 1, "team_name": "Rack Attackers",
      "total_paid": 150,
      "payouts": [ { "id": 1, "amount": 150, "note": "1st place", "...": "..." } ],
      "standing": { "team_id": 1, "wins": 0, "losses": 0, "points": 0, "...": "..." }
    }
  ]
}
```

### Frontend

New `web/domains/finances/` domain (`finances-api-service.js`,
`finances-page-component.js`, `finances-domain.js`) with its own
top-level "Financial" nav entry, hidden by default
(`#nav-item-finances`, `d-none`) and shown only when the resolved Admin
Key identity is `league_admin`/`admin`/`system_admin` -- the same
identity-gating pattern the Users screen already uses for
`#nav-item-users`, extended in `updateIdentityUI()` (`web/app.js`) with a
`canManageFinances` check matching `clearanceAuth`'s allowed role set
exactly (unlike Users' gate, `league_admin` also qualifies here).

The screen shows a season selector, a Dues section (every rostered
player, paid/unpaid badge, total paid, last payment date, a "Record
Payment" button opening a small modal), and a Payouts section (every
season team, standing shown for reference, total paid, a "Record
Payout" button). Uses the existing shared `api()` client and Admin Key
behavior unchanged -- no finances-specific auth code in the frontend.

### What Phase 1 defers

All explicitly out of scope per PM decision, not oversights:

- Player Overview money integration (replacing the static placeholder
  with a real per-player dues lookup) -- deferred to Phase 2, a small,
  isolated follow-on once this backend exists. **Update 2026-08-29:**
  implemented -- see "Player Overview Phase 2 read method" below and
  `doc/domains/players/README.md`'s "Player Overview Phase 2
  Implementation" for full detail.
- Real player-facing login or a player-only money view.
- Payment editing or voiding (both tables are append-only; correcting a
  mistake today means recording an offsetting entry, not modifying the
  original row).
- Penalties for unpaid dues.
- Automated payout formulas or standings-driven suggestions -- payout
  amounts are always admin-entered; standings are reference-only.
- Email/notifications.
- Broader audit/history beyond these two tables.
- Any payment processor or accounting system integration.
- Any change to standings behavior.

### Verification

`go test ./... -count=1` and `go build ./...` pass, including: 12 new
`FinanceService` validation/delegation tests (`backend/domains/finances/
service_test.go`), 8 new `FinanceStore` tests covering insert/list/
newest-first ordering/season-scoping for both tables
(`backend/storage/sqlite/finances_store_test.go`), and 11 new handler
tests (`handlers/api_finances_test.go`) covering all four routes'
401/403 auth enforcement, a `league_admin` key succeeding on a GET,
full dues and payout success paths (unpaid -> paid, empty -> recorded),
a not-rostered-player rejection (404), an invalid-amount rejection
(400), and a team-not-in-season rejection (404). `node --check` passes
on all four changed/new JS files. Manually verified end to end against
a local server build with a real seeded season/roster: confirmed
401/403 without a valid league_admin key, confirmed a dues payment
correctly flips a player from unpaid to paid with the denormalized
`team_id` attached, and confirmed a payout is recorded and totaled
correctly alongside a real (zero-value, no matches played) standings
reference. One real bug found and fixed during this pass: the handler's
per-player/per-team history composition returned a JSON `null` instead
of `[]` for players/teams with no payments/payouts yet (a Go nil-slice
default from an unmatched map lookup) -- fixed to always initialize a
non-nil empty slice, matching the rest of the codebase's convention.
Actual browser rendering of the new screen remains **NOT VERIFIED (no
browser)** in this developer's tool session.

## Player Overview Phase 2 read method (added 2026-08-29)

`FinanceStore` gained a fifth method, `ListDuesPaymentsByPlayer(ctx,
seasonID, playerID)`, added when Player Overview Phase 2 needed one
player's dues history rather than `ListDuesPayments`' full season list.
It is a straightforward narrowing of the same query (adds `AND
dp.player_id = ?`), kept in this domain's store/service per the
`finances` domain's own convention of owning all reads/writes to its two
tables -- Player Overview's handler calls `FinanceManager` the same way
it calls every other manager, rather than the finances package gaining
any dependency on players/seasons. See
`doc/domains/players/README.md`'s "Player Overview Phase 2
Implementation" for the consuming handler's composition detail.
**Update 2026-08-30:** this integration initially left Player Overview's
route unprotected despite surfacing the same money data this domain's
own routes keep behind `clearanceAuth`; PM resolved that gap
(`PLAYERS-Q002`) by requiring `clearanceAuth` on the whole Player
Overview route too -- see "Privacy inconsistency -- resolved 2026-08-30"
in the players doc for full detail. No change was made to any Financial
Phase 1 route, table, or auth as part of that correction.

## Decision History

### 2026-08-27 - Financial Phase 1: dues and payouts

**Status:** `accepted`

Added the first financial data model and screen: `dues_payments` and
`payouts`, both simple append-only history tables with no
partial-payment/balance math, backing a new league-admin-only Financial
screen. Payout amounts are always admin-entered; standings are shown for
reference only, never used to compute anything automatically. All four
routes (reads and writes) require `clearanceAuth`, a deliberate
departure from every other domain's open-GET convention, since money
data should not be public just because other domain reads are. Real
player login, payment editing/voiding, penalties, payout formulas, and
Player Overview integration are all explicitly deferred. See "Financial
Phase 1 -- Dues and Payouts" above for full detail.

### 2026-08-29 - Player Overview Phase 2 read method

**Status:** `accepted`

Added `FinanceStore.ListDuesPaymentsByPlayer` (season+player-scoped
read) so Player Overview Phase 2 could show real per-player dues status
without duplicating SQL outside this domain. No change to any Financial
Phase 1 route, table, or auth. At initial ship this left Player
Overview's route unprotected despite surfacing the same money data --
flagged as `PLAYERS-Q002` and resolved 2026-08-30 (see below). See
"Player Overview Phase 2 read method" above and
`doc/domains/players/README.md` for full detail.

### 2026-08-30 - Player Overview auth correction (resolves PLAYERS-Q002)

**Status:** `accepted`

Player Overview's `GET /api/players/{id}/overview` is now protected by
`clearanceAuth`, the same role gate (league_admin/admin/system_admin)
this domain's own routes use, since it exposes the same kind of
per-player money data. No change to any Financial Phase 1 route, table,
or auth -- this correction is entirely within the players/handlers
layer. See `doc/domains/players/README.md`'s "Privacy inconsistency --
resolved 2026-08-30" for full detail.
