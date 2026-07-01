# League App API Guide for Integrators

This document explains how third parties can call the League App HTTP API.
Examples use `curl` and JavaScript `fetch`.

Status: draft  
Audience: external tools, scripts, automation, mobile/web clients

## Base URLs

Common environments:

- Local development: `http://localhost:8080`
- Staging IIS site: `http://league-staging.local`
- Staging app directly: `http://localhost:9000`

All API routes are under `/api`.

Example:

```text
GET http://localhost:8080/api/leagues
```

## Conventions

### JSON

Send and accept JSON unless a route clearly behaves like a plain `DELETE`.

Typical headers:

```http
Content-Type: application/json
Accept: application/json
```

### Route style

The API is resource-oriented and uses standard HTTP verbs:

- `GET` to read
- `POST` to create or perform an action
- `PUT` to replace or update
- `PATCH` for partial action-style changes
- `DELETE` to remove

### IDs

Most resource IDs are integer path parameters.

Examples:

- `/api/leagues/1`
- `/api/seasons/2`
- `/api/matches/15`

### Dates

Dates are typically sent as `YYYY-MM-DD`.

Example:

```json
{ "start_date": "2026-07-01" }
```

## Authentication

Most current routes are public inside the trusted app environment.

One important exception exists today:

- `POST /api/seasons/{id}/handicap-apply`

That route requires a bearer token.

Example:

```http
Authorization: Bearer YOUR_ADMIN_TOKEN
```

If the header is missing, the server returns:

- `401 Unauthorized`
- `WWW-Authenticate: Bearer realm="league-admin"`

If the token is wrong, the server returns:

- `403 Forbidden`

## Error Format

Most errors return this shape:

```json
{ "error": "human-readable message" }
```

Examples:

```json
{ "error": "invalid id" }
```

```json
{ "error": "forbidden" }
```

Some validation-heavy routes may return structured error payloads instead of a
single `error` string. For example, scoresheet or close-week validation can
return message arrays.

## Common Status Codes

- `200 OK` read or successful action
- `201 Created` new record created
- `400 Bad Request` malformed JSON, missing fields, invalid input
- `401 Unauthorized` auth required
- `403 Forbidden` wrong or insufficient auth
- `404 Not Found` route or resource not found
- `409 Conflict` stale state, duplicate request, business conflict
- `422 Unprocessable Entity` valid request shape, but business rules rejected it
- `500 Internal Server Error` unexpected server-side problem

## API Families

Major route groups:

- Leagues
- Players
- Teams
- Seasons
- Season rules
- Skipped weeks
- Bye requests
- Season teams and rosters
- Matches
- Match rounds and results
- Lineup plans
- Rules definitions
- Close-week workflow
- Standings and player stats
- Handicap review and apply
- Backup

## Quick Start Examples

### 1. List leagues

```bash
curl -s http://localhost:8080/api/leagues
```

Typical response:

```json
[
  {
    "id": 1,
    "name": "Demo Pool League",
    "game_format": "8ball"
  }
]
```

### 2. Create a league

```bash
curl -s -X POST http://localhost:8080/api/leagues \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Northside 8-Ball",
    "game_format": "8ball",
    "day_of_week": "Tuesday"
  }'
```

### 3. List seasons for a league

```bash
curl -s "http://localhost:8080/api/seasons?league_id=1"
```

### 4. Create a season

```bash
curl -s -X POST http://localhost:8080/api/seasons \
  -H "Content-Type: application/json" \
  -d '{
    "league_id": 1,
    "name": "Fall 2026",
    "start_date": "2026-09-01",
    "schedule_type": "single_rr",
    "num_weeks": 10
  }'
```

### 5. List teams for a season

```bash
curl -s http://localhost:8080/api/seasons/2/teams
```

### 6. List matches for a season

```bash
curl -s "http://localhost:8080/api/matches?season_id=2"
```

### 7. Get one match

```bash
curl -s http://localhost:8080/api/matches/15
```

### 8. Get standings

```bash
curl -s "http://localhost:8080/api/standings?season_id=2"
```

### 9. Get player stats

```bash
curl -s "http://localhost:8080/api/player-stats?season_id=2"
```

## Match and Scoresheet Examples

### Get saved round data for a match

```bash
curl -s http://localhost:8080/api/matches/15/rounds
```

### Save round data for a match

This route stores scoresheet round results.

```bash
curl -s -X POST http://localhost:8080/api/matches/15/rounds \
  -H "Content-Type: application/json" \
  -d '{
    "rounds": [
      {
        "round_number": 1,
        "home_player_id": 11,
        "away_player_id": 27,
        "game1_home": 10,
        "game1_away": 7,
        "game2_home": 10,
        "game2_away": 5,
        "game3_home": 10,
        "game3_away": 6
      }
    ]
  }'
```

Important note:
- the backend remains authoritative for validation
- a `200` means the save was accepted
- a `422` means the round data failed business validation

### Submit final match results

```bash
curl -s -X POST http://localhost:8080/api/matches/15/results \
  -H "Content-Type: application/json" \
  -d '{}'
```

### Clear final match results

```bash
curl -s -X DELETE http://localhost:8080/api/matches/15/results
```

## Close Week Workflow Examples

### List week status for a season

```bash
curl -s http://localhost:8080/api/seasons/2/weeks
```

### Validate a week before closing

```bash
curl -s http://localhost:8080/api/seasons/2/weeks/1/validate
```

### Get advance preview for a week

```bash
curl -s http://localhost:8080/api/seasons/2/weeks/1/advance-preview
```

### Close a week

```bash
curl -s -X POST http://localhost:8080/api/seasons/2/weeks/1/close \
  -H "Content-Type: application/json" \
  -d '{}'
```

### Reopen a week

```bash
curl -s -X POST http://localhost:8080/api/seasons/2/weeks/1/reopen
```

### Get prior close acknowledgments for a week

```bash
curl -s http://localhost:8080/api/seasons/2/weeks/1/acknowledgments
```

## Handicap Review Examples

### Get handicap recommendations

```bash
curl -s http://localhost:8080/api/seasons/2/handicap-recommendations
```

Possible response statuses inside the JSON body include:

- `no_auto_apply`
- `unsupported`
- `no_data`
- `preview`

Typical response shape:

```json
{
  "season_id": 2,
  "method": "game_diff_average",
  "status": "preview",
  "message": "2 players have recommended handicap changes (not yet applied).",
  "weeks_closed": 3,
  "recommendations": [
    {
      "player_id": 11,
      "player_name": "Avery Slate",
      "team_name": "Rack Attackers",
      "assigned_hc": 1.5,
      "recommended_hc": 2.25,
      "change_amount": 0.75,
      "reason": "",
      "rec_token": "opaque-hash-value"
    }
  ]
}
```

## Handicap Apply Example

This is the only currently authenticated route.

### Apply one or more handicap changes

```bash
curl -s -X POST http://localhost:8080/api/seasons/2/handicap-apply \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_ADMIN_TOKEN" \
  -d '{
    "apply_request_id": "550e8400-e29b-41d4-a716-446655440000",
    "entries": [
      {
        "player_id": 11,
        "expected_assigned_hc": 1.50,
        "expected_recommended_hc": 2.25,
        "rec_token": "opaque-hash-from-review-endpoint"
      }
    ]
  }'
```

Notes:

- `apply_request_id` must be a UUID v4
- `rec_token` must come from the latest handicap review response
- the backend recomputes live state before writing
- stale data can return `409 Conflict`
- business-rule rejections can return `422 Unprocessable Entity`

### Example conflict response

```json
{
  "error": "apply conflicts must be resolved before retrying",
  "conflicts": [
    {
      "player_id": 11,
      "code": "recommended_hc_changed",
      "message": "recommended handicap changed from 2.25 to 2.50; refresh and retry"
    }
  ]
}
```

### Example rejection response

```json
{
  "error": "one or more players are not eligible for apply",
  "rejections": [
    {
      "player_id": 11,
      "code": "below_threshold",
      "message": "player has not met the minimum rack threshold"
    }
  ]
}
```

## JavaScript Example

### Basic GET request

```js
const res = await fetch("http://localhost:8080/api/leagues", {
  headers: { Accept: "application/json" }
});

if (!res.ok) {
  throw new Error(`HTTP ${res.status}`);
}

const leagues = await res.json();
console.log(leagues);
```

### Authenticated POST request

```js
const token = "YOUR_ADMIN_TOKEN";

const res = await fetch("http://localhost:8080/api/seasons/2/handicap-apply", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
    "Authorization": `Bearer ${token}`,
    "Accept": "application/json"
  },
  body: JSON.stringify({
    apply_request_id: "550e8400-e29b-41d4-a716-446655440000",
    entries: [
      {
        player_id: 11,
        expected_assigned_hc: 1.5,
        expected_recommended_hc: 2.25,
        rec_token: "opaque-hash-from-review-endpoint"
      }
    ]
  })
});

const body = await res.json();

if (!res.ok) {
  console.error("API error", res.status, body);
} else {
  console.log("Applied", body);
}
```

## Current Route Inventory

The current route surface is defined in:

- [handlers/api.go](C:/Users/admin/source/league_app/handlers/api.go)

Current route groups include:

- `GET/POST /api/leagues`
- `GET/PUT/DELETE /api/leagues/{id}`
- `GET/POST /api/players`
- `GET/PUT/DELETE /api/players/{id}`
- `GET/POST /api/teams`
- `GET/PUT/DELETE /api/teams/{id}`
- `GET/POST /api/seasons`
- `GET/PUT/DELETE /api/seasons/{id}`
- `POST /api/seasons/{id}/activate`
- `GET/POST /api/seasons/{id}/rules`
- `PUT/DELETE /api/seasons/{id}/rules/{rid}`
- `GET/POST /api/seasons/{id}/skipped-weeks`
- `DELETE /api/seasons/{id}/skipped-weeks/{sid}`
- `GET/POST /api/seasons/{id}/bye-requests`
- `PUT/DELETE /api/seasons/{id}/bye-requests/{bid}`
- `GET /api/seasons/{id}/teams`
- `POST /api/seasons/{id}/teams`
- `GET /api/seasons/{id}/previous`
- `PUT/DELETE /api/seasons/{id}/teams/{tid}`
- `GET/POST /api/seasons/{id}/teams/{tid}/roster`
- `DELETE /api/seasons/{id}/teams/{tid}/roster/{pid}`
- `GET /api/seasons/{id}/players/available`
- `GET /api/seasons/{id}/checklist`
- `GET /api/matches`
- `POST /api/matches/generate`
- `GET /api/matches/{id}`
- `PATCH /api/matches/{id}/assign`
- `POST/DELETE /api/matches/{id}/results`
- `GET/POST /api/matches/{id}/rounds`
- `GET/POST /api/lineup-plans`
- `DELETE /api/lineup-plans/{id}`
- `GET /api/rules/definitions`
- `GET /api/seasons/{id}/weeks`
- `GET /api/seasons/{id}/weeks/{week}/validate`
- `POST /api/seasons/{id}/weeks/{week}/close`
- `POST /api/seasons/{id}/weeks/{week}/reopen`
- `GET /api/seasons/{id}/weeks/{week}/acknowledgments`
- `GET /api/seasons/{id}/weeks/{week}/advance-preview`
- `GET /api/seasons/{id}/handicap-recommendations`
- `POST /api/seasons/{id}/handicap-apply`
- `GET /api/standings`
- `GET /api/player-stats`
- `POST /api/backup`

## Integration Notes

- There is no public API versioning scheme yet, so clients should treat the API
  as evolving.
- Prefer consuming stable IDs and structured codes rather than display labels.
- For mutation workflows, always read fresh state first when possible.
- For handicap apply, always use the latest recommendation payload and token.
- For close-week and other administrative flows, expect validation responses and
  do not assume every non-200 failure uses the same JSON shape.

## Suggested Next Improvements

If this guide will be shared outside the core team, the next useful follow-ups
would be:

- add example success payloads for more routes
- document request bodies for every create/update endpoint
- publish an OpenAPI document
- add versioning guidance once the external API contract stabilizes
