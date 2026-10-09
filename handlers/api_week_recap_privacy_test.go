// Player Score Approval Phase 1D privacy correction: GET
// /api/seasons/{id}/weeks/{week}/recap stays PUBLIC for its existing,
// non-sensitive recap data, but the four new team-approval-readiness
// fields (score_revision, home_approval_status, away_approval_status,
// approval_ready) are protected approval metadata, per Phase 1B's policy
// -- they must be present ONLY for an appropriately scoped administrator
// (system_admin, or a league_admin for the league that owns the
// requested season) and absent (not a placeholder zero/false/empty
// value) for every other caller, including an eligible linked player.
// Uses the same real sqlite-backed auth stack as
// api_match_approval_phase1b_test.go (seedApprovalFixture,
// testServerWithAuth, authDo, loginCookies, seedPasswordUser).
package handlers_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"league_app/backend/domains/auth"
	"league_app/db"
)

// recapFirstMatchRow fetches the week-1 recap for f's season and returns
// the first match row as a generic map, so presence/absence of a JSON key
// can be asserted directly (a struct-typed decode would hide a key that
// never round-trips at all).
func recapFirstMatchRow(t *testing.T, f approvalFixture, cookies []*http.Cookie) map[string]any {
	t.Helper()
	resp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/seasons/%d/weeks/1/recap", f.seasonID), "", cookies, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for the public recap route, got %d", resp.StatusCode)
	}
	var recap map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&recap); err != nil {
		t.Fatalf("decode recap: %v", err)
	}
	rows, ok := recap["matches"].([]any)
	if !ok || len(rows) == 0 {
		t.Fatalf("want at least 1 match in recap, got %v", recap["matches"])
	}
	row, ok := rows[0].(map[string]any)
	if !ok {
		t.Fatalf("want match row to be a JSON object, got %T", rows[0])
	}
	return row
}

func assertApprovalFieldsAbsent(t *testing.T, row map[string]any) {
	t.Helper()
	for _, key := range []string{"score_revision", "home_approval_status", "away_approval_status", "approval_ready"} {
		if v, present := row[key]; present {
			t.Errorf("want %q absent from the recap response for this caller, got present with value %v", key, v)
		}
	}
}

func assertApprovalFieldsPresent(t *testing.T, row map[string]any) {
	t.Helper()
	for _, key := range []string{"score_revision", "home_approval_status", "away_approval_status", "approval_ready"} {
		if _, present := row[key]; !present {
			t.Errorf("want %q present in the recap response for a scoped administrator, got absent", key)
		}
	}
}

// 1. An unauthenticated caller receives the existing recap data but none
// of the four new approval fields.
func TestWeekRecapPrivacy_Unauthenticated_NoApprovalFields(t *testing.T) {
	f := seedApprovalFixture(t)
	row := recapFirstMatchRow(t, f, nil)
	assertApprovalFieldsAbsent(t, row)
}

// 2. A linked player (eligible for this very match, even) receives the
// existing recap data but none of the four new approval fields -- a
// player's protected read is the per-match GET .../approval, never this
// full-week, unrelated-player-reachable recap.
func TestWeekRecapPrivacy_LinkedPlayer_NoApprovalFields(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, _ := loginCookies(t, f.srv, "home@example.com", "hunter22")

	row := recapFirstMatchRow(t, f, cookies)
	assertApprovalFieldsAbsent(t, row)
}

// 3. A league administrator for a DIFFERENT league does not receive the
// fields.
func TestWeekRecapPrivacy_DifferentLeagueAdmin_NoApprovalFields(t *testing.T) {
	f := seedApprovalFixture(t)
	otherLeagueID := seedTestLeague(t, "Other League")
	seedPasswordUser(t, "otheradmin@example.com", "hunter22", auth.RoleLeagueAdmin, &otherLeagueID)
	cookies, _ := loginCookies(t, f.srv, "otheradmin@example.com", "hunter22")

	row := recapFirstMatchRow(t, f, cookies)
	assertApprovalFieldsAbsent(t, row)
}

// 4. A league administrator scoped to the season's own league receives
// the correct fields.
func TestWeekRecapPrivacy_SameLeagueAdmin_SeesApprovalFields(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	cookies, _ := loginCookies(t, f.srv, "admin@example.com", "hunter22")

	row := recapFirstMatchRow(t, f, cookies)
	assertApprovalFieldsPresent(t, row)
	if status, _ := row["home_approval_status"].(string); status != "pending" {
		t.Errorf("want home_approval_status=pending for a freshly-scored, unapproved match, got %v", row["home_approval_status"])
	}
}

// 5. A system administrator receives the correct fields, unconditionally
// (no season/league lookup needed for them).
func TestWeekRecapPrivacy_SystemAdmin_SeesApprovalFields(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, _ := loginCookies(t, f.srv, "sysadmin@example.com", "hunter22")

	row := recapFirstMatchRow(t, f, cookies)
	assertApprovalFieldsPresent(t, row)
}

// 6. Existing public recap behavior remains unchanged apart from
// redaction of the new protected fields: every pre-existing field is
// still present and correct for an unauthenticated caller.
func TestWeekRecapPrivacy_ExistingPublicFieldsUnchanged(t *testing.T) {
	f := seedApprovalFixture(t)
	// seedApprovalFixture's season_teams rows carry no explicit
	// season_name snapshot (defaults to '' NOT NULL), which hits a
	// pre-existing, unrelated COALESCE quirk in GetWeekRecapData
	// (COALESCE never falls through to teams.name for an empty-but-
	// non-null season_name) -- backfilling it here keeps this test
	// focused on the privacy correction rather than tripping over that
	// separate, out-of-scope defect.
	if _, err := db.DB.Exec(`UPDATE season_teams SET season_name='Home Team' WHERE season_id=? AND team_id=?`, f.seasonID, f.homeTeamID); err != nil {
		t.Fatalf("backfill home season_name: %v", err)
	}
	if _, err := db.DB.Exec(`UPDATE season_teams SET season_name='Away Team' WHERE season_id=? AND team_id=?`, f.seasonID, f.awayTeamID); err != nil {
		t.Fatalf("backfill away season_name: %v", err)
	}
	row := recapFirstMatchRow(t, f, nil)

	if id, _ := row["match_id"].(float64); int64(id) != f.matchID {
		t.Errorf("want match_id=%d, got %v", f.matchID, row["match_id"])
	}
	if homeName, _ := row["home_team_name"].(string); homeName != "Home Team" {
		t.Errorf("want home_team_name=%q, got %v", "Home Team", row["home_team_name"])
	}
	if awayName, _ := row["away_team_name"].(string); awayName != "Away Team" {
		t.Errorf("want away_team_name=%q, got %v", "Away Team", row["away_team_name"])
	}
	hasResult, ok := row["has_result"].(bool)
	if !ok || !hasResult {
		t.Errorf("want has_result=true (fixture is completed=1), got %v", row["has_result"])
	}
	if _, present := row["approved_at"]; present {
		t.Errorf("want approved_at absent (omitempty, not yet accepted), got present: %v", row["approved_at"])
	}
	weekClosed, ok := row["week_closed"].(bool)
	if !ok || weekClosed {
		t.Errorf("want week_closed=false, got %v", row["week_closed"])
	}
}
