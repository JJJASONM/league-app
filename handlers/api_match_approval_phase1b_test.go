// Player Score Approval Phase 1B integration tests: authentication-method
// enforcement, eligibility/scope resolution, team-approval transitions,
// player-authenticated score writes, admin acceptance/override, and the
// read-authorization matrix. Uses the same real sqlite-backed auth stack as
// api_auth_integration_test.go (testServerWithAuth, loginCookies,
// seedPasswordUser) -- these are genuine end-to-end route tests, not
// handler-unit tests with stubbed managers.
package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"league_app/backend/domains/auth"
	"league_app/backend/domains/finances"
	"league_app/backend/domains/handicaps"
	"league_app/backend/domains/leagues"
	"league_app/backend/domains/matches"
	"league_app/backend/domains/players"
	"league_app/backend/domains/rules"
	"league_app/backend/domains/seasons"
	"league_app/backend/domains/teams"
	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/handlers"
)

// approvalFixture is one scored, completed match with two rostered players
// (one per side) ready for team-approval/admin-acceptance/read-matrix
// testing. score_revision is 0 (round_results is seeded via raw SQL,
// matching every other test fixture in this package -- it never advances
// the revision).
type approvalFixture struct {
	srv                        *httptest.Server
	leagueID, seasonID         int64
	homeTeamID, awayTeamID     int64
	homePlayerID, awayPlayerID int64
	matchID                    int64
}

func seedApprovalFixture(t *testing.T) approvalFixture {
	t.Helper()
	srv := testServerWithAuth(t)
	leagueID := seedTestLeague(t, "Approval League")

	// teams_managed=0: SaveRounds's pre-existing, unrelated RosterEligible
	// fast-fail check (>= 3 season-roster players per side) applies only to
	// a teams_managed season -- this fixture only needs one rostered
	// player per side for Phase 1A's eligibility rule, so the season stays
	// unmanaged (season_rosters rows are still insertable either way; the
	// BEFORE INSERT trigger only requires a season_teams row, not
	// teams_managed=1).
	res, err := db.DB.Exec(`INSERT INTO seasons (league_id, name, active, teams_managed) VALUES (?,?,1,0)`, leagueID, "Approval Season")
	if err != nil {
		t.Fatalf("seed season: %v", err)
	}
	seasonID, _ := res.LastInsertId()

	rh, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Home Team')`, leagueID)
	homeTeamID, _ := rh.LastInsertId()
	ra, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Away Team')`, leagueID)
	awayTeamID, _ := ra.LastInsertId()
	if _, err := db.DB.Exec(`INSERT INTO season_teams (season_id, team_id) VALUES (?,?),(?,?)`,
		seasonID, homeTeamID, seasonID, awayTeamID); err != nil {
		t.Fatalf("seed season_teams: %v", err)
	}

	rph, _ := db.DB.Exec(`INSERT INTO players (first_name,last_name,team_id,handicap) VALUES ('Home','Player',?,3.0)`, homeTeamID)
	homePlayerID, _ := rph.LastInsertId()
	rpa, _ := db.DB.Exec(`INSERT INTO players (first_name,last_name,team_id,handicap) VALUES ('Away','Player',?,3.0)`, awayTeamID)
	awayPlayerID, _ := rpa.LastInsertId()
	if _, err := db.DB.Exec(`INSERT INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?),(?,?,?)`,
		seasonID, homeTeamID, homePlayerID, seasonID, awayTeamID, awayPlayerID); err != nil {
		t.Fatalf("seed season_rosters: %v", err)
	}

	rm, err := db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,1)`,
		seasonID, homeTeamID, awayTeamID)
	if err != nil {
		t.Fatalf("seed match: %v", err)
	}
	matchID, _ := rm.LastInsertId()
	if _, err := db.DB.Exec(`
		INSERT INTO round_results
		    (match_id, round_number, home_player_id, away_player_id,
		     game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
		     home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
		VALUES (?,1,?,?,10,5,10,3,10,2,3.0,3.0,0,'')`,
		matchID, homePlayerID, awayPlayerID); err != nil {
		t.Fatalf("seed round_results: %v", err)
	}
	if _, err := db.DB.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID); err != nil {
		t.Fatalf("mark completed: %v", err)
	}

	return approvalFixture{
		srv: srv, leagueID: leagueID, seasonID: seasonID,
		homeTeamID: homeTeamID, awayTeamID: awayTeamID,
		homePlayerID: homePlayerID, awayPlayerID: awayPlayerID,
		matchID: matchID,
	}
}

func linkUserToPlayer(t *testing.T, userID, playerID int64) {
	t.Helper()
	if _, err := db.DB.Exec(`UPDATE users SET player_id=? WHERE id=?`, playerID, userID); err != nil {
		t.Fatalf("link player: %v", err)
	}
}

// seedUnrelatedPlayer creates a player with no roster/participation
// relationship to any match in f, and a session-authenticated user linked
// to it.
func seedUnrelatedPlayer(t *testing.T, f approvalFixture, email string) ([]*http.Cookie, string) {
	t.Helper()
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name) VALUES ('Unrelated', 'Player')`)
	if err != nil {
		t.Fatalf("seed unrelated player: %v", err)
	}
	playerID, _ := res.LastInsertId()
	userID := seedPasswordUser(t, email, "hunter22", "", nil)
	linkUserToPlayer(t, userID, playerID)
	return loginCookies(t, f.srv, email, "hunter22")
}

// noopHandicapApplier satisfies handlers.HandicapApplier for
// testServerWithStaticToken, which must wire one whenever AdminToken is
// non-empty (Register's own required-dependency guard) even though no
// test here exercises the Apply route itself.
type noopHandicapApplier struct{}

func (noopHandicapApplier) Apply(_ context.Context, _ int64, _ handicaps.ApplyRequest) (handicaps.ApplyResult, error) {
	return handicaps.ApplyResult{}, nil
}

// testServerWithStaticToken is a variant of testServerWithAuth that also
// configures the static LEAGUE_ADMIN_TOKEN bootstrap credential, needed
// only for the "static token rejected for player-scoped action" test --
// testServerWithAuth deliberately leaves AdminToken empty for every other
// Phase 1B test, since AuthMethodStaticToken is otherwise out of scope.
func testServerWithStaticToken(t *testing.T, adminToken string) *httptest.Server {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })

	mux := http.NewServeMux()
	hcStore := sqlite.NewHandicapStore(db.DB)
	hcSvc := handicaps.NewService(hcStore)
	weekStore := sqlite.NewWeekStore(db.DB)
	ruleStore := sqlite.NewRuleStore(db.DB)
	weekSvc := matches.NewWeekService(weekStore, hcSvc, ruleStore)
	roundStore := sqlite.NewRoundStore(db.DB)
	roundSvc := matches.NewRoundService(roundStore, ruleStore)
	ruleSvc := rules.NewRuleService(ruleStore)
	seasonStore := sqlite.NewSeasonStore(db.DB)
	seasonSvc := seasons.NewSeasonService(seasonStore)
	leagueStore := sqlite.NewLeagueStore(db.DB)
	leagueSvc := leagues.NewLeagueService(leagueStore)
	playerStore := sqlite.NewPlayerStore(db.DB)
	playerSvc := players.NewPlayerService(playerStore)
	teamStore := sqlite.NewTeamStore(db.DB)
	teamSvc := teams.NewTeamService(teamStore)
	matchStore := sqlite.NewMatchStore(db.DB)
	matchSvc := matches.NewMatchService(matchStore)
	lineupStore := sqlite.NewLineupStore(db.DB)
	lineupSvc := matches.NewLineupService(lineupStore, roundStore)
	financeStore := sqlite.NewFinanceStore(db.DB)
	financeSvc := finances.NewFinanceService(financeStore)

	applyAuthStore := sqlite.NewApplyAuthStore(db.DB)
	authUserStore := sqlite.NewAuthUserStore(db.DB)
	roleStore := sqlite.NewRoleAssignmentStore(db.DB)
	sessionStore := sqlite.NewSessionStore(db.DB)
	setupTokenStore := sqlite.NewPasswordSetupTokenStore(db.DB)
	apiKeyAdminStore := sqlite.NewUserAPIKeyAdminStore(db.DB)
	sessionSvc := auth.NewSessionService(authUserStore, roleStore, sessionStore, setupTokenStore, fastArgon2)

	deps := handlers.Dependencies{
		HandicapSvc: hcSvc, WeekMgr: weekSvc, RoundMgr: roundSvc, RuleMgr: ruleSvc,
		LeagueMgr: leagueSvc, PlayerMgr: playerSvc, TeamMgr: teamSvc, SeasonMgr: seasonSvc,
		MatchMgr: matchSvc, LineupMgr: lineupSvc, FinanceMgr: financeSvc,
		ApplyAuth: applyAuthStore, AuthMgr: sessionSvc, RoleAssignmentMgr: roleStore,
		AuthUserMgr: authUserStore, APIKeyMgr: apiKeyAdminStore,
		LeagueSelfGrantMgr:   sqlite.NewLeagueSelfGrantStore(db.DB),
		InsecureLocalCookies: true,
		AdminToken:           adminToken,
		HandicapApplier:      noopHandicapApplier{},
	}
	handlers.Register(mux, dir, deps)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// ============================================================
// Authentication method
// ============================================================

func TestApprovalAuth_SessionEligiblePlayer_CanApproveOwnSide(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, csrf := loginCookies(t, f.srv, "home@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	snap := decodeJSON[map[string]any](t, resp)
	home, _ := snap["home"].(map[string]any)
	if home["state"] != "approved" {
		t.Errorf("want home.state=approved in response snapshot, got %v", snap)
	}
}

func TestApprovalAuth_NoCredential_Returns401(t *testing.T) {
	f := seedApprovalFixture(t)
	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", resp.StatusCode)
	}
}

func TestApprovalAuth_PersonalAPIKey_Rejected(t *testing.T) {
	f := seedApprovalFixture(t)

	applyAuthStore := sqlite.NewApplyAuthStore(db.DB)
	_, cleartext, err := applyAuthStore.CreateApplyPlayerUser(context.Background(), "home-key-user", f.homePlayerID)
	if err != nil {
		t.Fatalf("CreateApplyPlayerUser: %v", err)
	}

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, nil, map[string]string{"Authorization": "Bearer " + cleartext})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (a personal API key must not authorize a player-scoped action, even when its user is linked to a player), got %d", resp.StatusCode)
	}
}

func TestApprovalAuth_StaticToken_Rejected(t *testing.T) {
	srv := testServerWithStaticToken(t, "bootstrap-token")
	leagueID := seedTestLeague(t, "Static Token League")
	res, _ := db.DB.Exec(`INSERT INTO seasons (league_id, name, active, teams_managed) VALUES (?,?,1,1)`, leagueID, "S")
	seasonID, _ := res.LastInsertId()
	rh, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'H')`, leagueID)
	homeTeamID, _ := rh.LastInsertId()
	ra, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'A')`, leagueID)
	awayTeamID, _ := ra.LastInsertId()
	rm, _ := db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number) VALUES (?,?,?,1)`, seasonID, homeTeamID, awayTeamID)
	matchID, _ := rm.LastInsertId()

	resp := authDo(t, srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", matchID),
		`{"expected_revision":0}`, nil, map[string]string{"Authorization": "Bearer bootstrap-token"})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (the static bootstrap token must not authorize a player-scoped action), got %d", resp.StatusCode)
	}
}

func TestApprovalAuth_SessionWithNoLinkedPlayer_Rejected(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "nolink@example.com", "hunter22", "", nil)
	cookies, csrf := loginCookies(t, f.srv, "nolink@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (a session with no linked player must be rejected), got %d", resp.StatusCode)
	}
}

func TestApprovalAuth_DeactivatedSession_Rejected(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, csrf := loginCookies(t, f.srv, "home@example.com", "hunter22")

	sessionSvc := auth.NewSessionService(
		sqlite.NewAuthUserStore(db.DB), sqlite.NewRoleAssignmentStore(db.DB),
		sqlite.NewSessionStore(db.DB), sqlite.NewPasswordSetupTokenStore(db.DB), fastArgon2,
	)
	if err := sessionSvc.Deactivate(context.Background(), userID, sqlite.NewUserAPIKeyAdminStore(db.DB)); err != nil {
		t.Fatalf("Deactivate: %v", err)
	}

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	// requireAction's existing, documented contract (shared by every other
	// guarded route, not something this phase introduces): a credential
	// that resolves to nothing -- an expired/revoked/deactivated session
	// cookie included -- is "present but invalid" (403), distinct from no
	// credential at all (401). GET /api/auth/me is a deliberate special
	// case with its own bespoke 401-only handling; requireAction-gated
	// routes are not.
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 for a deactivated account's session (requireAction's existing invalid-credential behavior), got %d", resp.StatusCode)
	}
}

// ============================================================
// Eligibility and scope
// ============================================================

func TestApprovalEligibility_AwayRosterPlayer_CanApproveAway(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "away@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.awayPlayerID)
	cookies, csrf := loginCookies(t, f.srv, "away@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-away", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestApprovalEligibility_SubstituteParticipant_CanApproveForPlayedSide(t *testing.T) {
	f := seedApprovalFixture(t)
	// A substitute: not on the season roster, but recorded as the home
	// participant in round_results for this specific match.
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name) VALUES ('Sub', 'Home')`)
	if err != nil {
		t.Fatalf("seed sub player: %v", err)
	}
	subPlayerID, _ := res.LastInsertId()
	if _, err := db.DB.Exec(`UPDATE round_results SET home_player_id=? WHERE match_id=?`, subPlayerID, f.matchID); err != nil {
		t.Fatalf("swap in substitute: %v", err)
	}

	userID := seedPasswordUser(t, "sub@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, subPlayerID)
	cookies, csrf := loginCookies(t, f.srv, "sub@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (an actual round participant is eligible even when not rostered), got %d: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestApprovalEligibility_OppositeSidePlayer_CannotApproveOtherSide(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, csrf := loginCookies(t, f.srv, "home@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-away", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("want 422 (ineligible for the away side; this is a domain status, not an authorization failure), got %d", resp.StatusCode)
	}
}

func TestApprovalEligibility_UnrelatedPlayer_Denied(t *testing.T) {
	f := seedApprovalFixture(t)
	cookies, csrf := seedUnrelatedPlayer(t, f, "unrelated@example.com")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("want 422 (unrelated player is ineligible), got %d", resp.StatusCode)
	}
}

func TestApprovalEligibility_CrossLeaguePlayer_Denied(t *testing.T) {
	f := seedApprovalFixture(t)
	// A player who exists but has no roster/participation relationship to
	// this match at all -- the same mechanism that denies an unrelated
	// same-league player naturally denies a cross-league one too, since
	// eligibility is resolved purely from this match's own roster/
	// participation rows.
	cookies, csrf := seedUnrelatedPlayer(t, f, "otherleague@example.com")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-away", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("want 422 (cross-league/unrelated player is ineligible), got %d", resp.StatusCode)
	}
}

// ============================================================
// Approval transitions
// ============================================================

func TestApprovalTransitions_ApproveBothSidesThenWithdraw(t *testing.T) {
	f := seedApprovalFixture(t)
	homeUserID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, homeUserID, f.homePlayerID)
	homeCookies, homeCSRF := loginCookies(t, f.srv, "home@example.com", "hunter22")
	awayUserID := seedPasswordUser(t, "away@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, awayUserID, f.awayPlayerID)
	awayCookies, awayCSRF := loginCookies(t, f.srv, "away@example.com", "hunter22")

	r1 := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, homeCookies, map[string]string{"X-CSRF-Token": homeCSRF})
	r1.Body.Close()
	if r1.StatusCode != http.StatusOK {
		t.Fatalf("approve-home: want 200, got %d", r1.StatusCode)
	}
	r2 := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-away", f.matchID),
		`{"expected_revision":0}`, awayCookies, map[string]string{"X-CSRF-Token": awayCSRF})
	defer r2.Body.Close()
	if r2.StatusCode != http.StatusOK {
		t.Fatalf("approve-away: want 200, got %d", r2.StatusCode)
	}
	snap := decodeJSON[map[string]any](t, r2)
	home, _ := snap["home"].(map[string]any)
	away, _ := snap["away"].(map[string]any)
	if home["state"] != "approved" || away["state"] != "approved" {
		t.Fatalf("want both sides approved in the returned snapshot, got %v", snap)
	}

	r3 := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/withdraw-approval", f.matchID),
		`{"side":"home","expected_revision":0}`, homeCookies, map[string]string{"X-CSRF-Token": homeCSRF})
	defer r3.Body.Close()
	if r3.StatusCode != http.StatusOK {
		t.Fatalf("withdraw-approval: want 200, got %d: %s", r3.StatusCode, readBody(t, r3))
	}
	snap3 := decodeJSON[map[string]any](t, r3)
	home3, _ := snap3["home"].(map[string]any)
	if home3["state"] != "pending" {
		t.Errorf("want home.state=pending after withdrawal, got %v", snap3)
	}
}

func TestApprovalTransitions_CorrectionRequiresNote(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, csrf := loginCookies(t, f.srv, "home@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/request-correction", f.matchID),
		`{"side":"home","expected_revision":0,"note":""}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 (a blank note is rejected), got %d", resp.StatusCode)
	}

	resp2 := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/request-correction", f.matchID),
		`{"side":"home","expected_revision":0,"note":"wrong game score"}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("want 200 with a nonblank note, got %d: %s", resp2.StatusCode, readBody(t, resp2))
	}
}

func TestApprovalTransitions_StaleRevision_Returns409ZeroWrites(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, csrf := loginCookies(t, f.srv, "home@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":99}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 for a stale expected_revision, got %d", resp.StatusCode)
	}

	var state string
	db.DB.QueryRow(`SELECT home_approval_state FROM matches WHERE id=?`, f.matchID).Scan(&state)
	if state != "pending" {
		t.Errorf("want zero writes on a stale-revision rejection (state still pending), got %q", state)
	}
}

func TestApprovalTransitions_LockedAfterAdminAccepted_Returns409(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	adminCookies, adminCSRF := loginCookies(t, f.srv, "admin@example.com", "hunter22")
	acceptResp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve", f.matchID),
		`{"expected_revision":0,"override":true}`, adminCookies, map[string]string{"X-CSRF-Token": adminCSRF})
	acceptResp.Body.Close()
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("admin accept setup: want 200, got %d", acceptResp.StatusCode)
	}

	userID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, csrf := loginCookies(t, f.srv, "home@example.com", "hunter22")
	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 (team actions are locked out once an admin has accepted), got %d", resp.StatusCode)
	}
}

// ============================================================
// Score writes
// ============================================================

func roundsBody(homePlayerID, awayPlayerID int64, expectedRevision *int) string {
	rev := ""
	if expectedRevision != nil {
		rev = fmt.Sprintf(`,"expected_revision":%d`, *expectedRevision)
	}
	return fmt.Sprintf(`{"rounds":[{"round_number":1,"home_player_id":%d,"away_player_id":%d,"game1_home":10,"game1_away":5,"game2_home":10,"game2_away":3,"game3_home":10,"game3_away":2}]%s}`,
		homePlayerID, awayPlayerID, rev)
}

func TestScoreWrite_PlayerSession_RequiresExpectedRevision(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, csrf := loginCookies(t, f.srv, "home@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/rounds", f.matchID),
		roundsBody(f.homePlayerID, f.awayPlayerID, nil), cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("want 400 (an eligible player's write must supply expected_revision), got %d: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestScoreWrite_PlayerSession_StaleRevision_Returns409(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, csrf := loginCookies(t, f.srv, "home@example.com", "hunter22")

	stale := 7
	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/rounds", f.matchID),
		roundsBody(f.homePlayerID, f.awayPlayerID, &stale), cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 for a stale revision, got %d: %s", resp.StatusCode, readBody(t, resp))
	}

	var rev int
	db.DB.QueryRow(`SELECT score_revision FROM matches WHERE id=?`, f.matchID).Scan(&rev)
	if rev != 0 {
		t.Errorf("want score_revision unchanged (0) after a rejected stale write, got %d", rev)
	}
}

func TestScoreWrite_PlayerSession_Success_ReturnsNewRevision(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, csrf := loginCookies(t, f.srv, "home@example.com", "hunter22")

	current := 0
	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/rounds", f.matchID),
		roundsBody(f.homePlayerID, f.awayPlayerID, &current), cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", resp.StatusCode, readBody(t, resp))
	}
	body := decodeJSON[map[string]any](t, resp)
	rev, _ := body["score_revision"].(float64)
	if int(rev) != 1 {
		t.Errorf("want score_revision=1 in the response after one edit, got %v", body["score_revision"])
	}
}

func TestScoreWrite_AdminSession_NoRevisionRequired_StillWorks(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	cookies, csrf := loginCookies(t, f.srv, "admin@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/rounds", f.matchID),
		roundsBody(f.homePlayerID, f.awayPlayerID, nil), cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (existing admin write remains compatible without expected_revision), got %d: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestScoreWrite_EditClearsPriorTeamApprovals(t *testing.T) {
	f := seedApprovalFixture(t)
	homeUserID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, homeUserID, f.homePlayerID)
	homeCookies, homeCSRF := loginCookies(t, f.srv, "home@example.com", "hunter22")
	approveResp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, homeCookies, map[string]string{"X-CSRF-Token": homeCSRF})
	approveResp.Body.Close()
	if approveResp.StatusCode != http.StatusOK {
		t.Fatalf("approve-home setup: want 200, got %d", approveResp.StatusCode)
	}

	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	adminCookies, adminCSRF := loginCookies(t, f.srv, "admin@example.com", "hunter22")
	editResp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/rounds", f.matchID),
		roundsBody(f.homePlayerID, f.awayPlayerID, nil), adminCookies, map[string]string{"X-CSRF-Token": adminCSRF})
	editResp.Body.Close()
	if editResp.StatusCode != http.StatusOK {
		t.Fatalf("admin edit: want 200, got %d", editResp.StatusCode)
	}

	var state string
	db.DB.QueryRow(`SELECT home_approval_state FROM matches WHERE id=?`, f.matchID).Scan(&state)
	if state != "pending" {
		t.Errorf("want home approval cleared back to pending after a score edit, got %q", state)
	}
}

// ============================================================
// Admin acceptance
// ============================================================

func TestAdminAccept_BothApproved_SucceedsWithoutOverride(t *testing.T) {
	f := seedApprovalFixture(t)
	homeUserID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, homeUserID, f.homePlayerID)
	homeCookies, homeCSRF := loginCookies(t, f.srv, "home@example.com", "hunter22")
	awayUserID := seedPasswordUser(t, "away@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, awayUserID, f.awayPlayerID)
	awayCookies, awayCSRF := loginCookies(t, f.srv, "away@example.com", "hunter22")
	authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, homeCookies, map[string]string{"X-CSRF-Token": homeCSRF}).Body.Close()
	authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-away", f.matchID),
		`{"expected_revision":0}`, awayCookies, map[string]string{"X-CSRF-Token": awayCSRF}).Body.Close()

	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	adminCookies, adminCSRF := loginCookies(t, f.srv, "admin@example.com", "hunter22")
	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve", f.matchID),
		`{"expected_revision":0,"override":false}`, adminCookies, map[string]string{"X-CSRF-Token": adminCSRF})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (both sides approved at the current revision needs no override), got %d: %s", resp.StatusCode, readBody(t, resp))
	}

	eventsResp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/matches/%d/approval-events", f.matchID), "", adminCookies, nil)
	defer eventsResp.Body.Close()
	var events []map[string]any
	json.NewDecoder(eventsResp.Body).Decode(&events)
	found := false
	for _, e := range events {
		if e["event_type"] == "admin_accepted" {
			found = true
		}
	}
	if !found {
		t.Errorf("want an admin_accepted event recorded, got %v", events)
	}
}

func TestAdminAccept_MissingApproval_RequiresOverride(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	adminCookies, adminCSRF := loginCookies(t, f.srv, "admin@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve", f.matchID),
		`{"expected_revision":0,"override":false}`, adminCookies, map[string]string{"X-CSRF-Token": adminCSRF})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("want 409 (override required when team approvals are missing), got %d", resp.StatusCode)
	}

	resp2 := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve", f.matchID),
		`{"expected_revision":0,"override":true}`, adminCookies, map[string]string{"X-CSRF-Token": adminCSRF})
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("want 200 with override:true, got %d: %s", resp2.StatusCode, readBody(t, resp2))
	}

	eventsResp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/matches/%d/approval-events", f.matchID), "", adminCookies, nil)
	defer eventsResp.Body.Close()
	var events []map[string]any
	json.NewDecoder(eventsResp.Body).Decode(&events)
	found := false
	for _, e := range events {
		if e["event_type"] == "admin_override" {
			found = true
		}
	}
	if !found {
		t.Errorf("want an admin_override event recorded, got %v", events)
	}
}

func TestAdminAccept_StaleRevision_Returns409ZeroWrites(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	adminCookies, adminCSRF := loginCookies(t, f.srv, "admin@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve", f.matchID),
		`{"expected_revision":99,"override":true}`, adminCookies, map[string]string{"X-CSRF-Token": adminCSRF})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("want 409 for a stale expected_revision, got %d", resp.StatusCode)
	}
	var approvedAt any
	db.DB.QueryRow(`SELECT approved_at FROM matches WHERE id=?`, f.matchID).Scan(&approvedAt)
	if approvedAt != nil {
		t.Errorf("want zero writes on a stale-revision admin-accept rejection, got approved_at=%v", approvedAt)
	}
}

// ============================================================
// Read privacy
// ============================================================

func TestReadPrivacy_PublicScheduleRemainsReadable(t *testing.T) {
	f := seedApprovalFixture(t)
	resp := authDo(t, f.srv, "GET", "/api/matches", "", nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 for the public match list, got %d", resp.StatusCode)
	}
	var ms []map[string]any
	json.NewDecoder(resp.Body).Decode(&ms)
	found := false
	for _, m := range ms {
		if int64(m["id"].(float64)) == f.matchID {
			found = true
		}
	}
	if !found {
		t.Error("want the match's schedule entry to remain visible in the public list")
	}
}

func TestReadPrivacy_PublicCannotReadDraftRounds(t *testing.T) {
	f := seedApprovalFixture(t) // completed=1, week_closed=0: draft/in-progress
	resp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/matches/%d/rounds", f.matchID), "", nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200 (this route stays public, just empty), got %d", resp.StatusCode)
	}
	var rounds []map[string]any
	json.NewDecoder(resp.Body).Decode(&rounds)
	if len(rounds) != 0 {
		t.Errorf("want an empty rounds list for a public caller on a draft/in-progress match, got %v", rounds)
	}
}

func TestReadPrivacy_EligiblePlayerCanReadOwnDraftRounds(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "home@example.com", "hunter22", "", nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, _ := loginCookies(t, f.srv, "home@example.com", "hunter22")

	resp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/matches/%d/rounds", f.matchID), "", cookies, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	var rounds []map[string]any
	json.NewDecoder(resp.Body).Decode(&rounds)
	if len(rounds) == 0 {
		t.Error("want the eligible player's own session to see the draft round detail")
	}
}

func TestReadPrivacy_FinalClosedResultsRemainPublic(t *testing.T) {
	f := seedApprovalFixture(t)
	if _, err := db.DB.Exec(`UPDATE matches SET week_closed=1 WHERE id=?`, f.matchID); err != nil {
		t.Fatalf("mark week closed: %v", err)
	}
	resp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/matches/%d/rounds", f.matchID), "", nil, nil)
	defer resp.Body.Close()
	var rounds []map[string]any
	json.NewDecoder(resp.Body).Decode(&rounds)
	if len(rounds) == 0 {
		t.Error("want full round detail publicly once completed AND week_closed (final)")
	}
}

func TestReadPrivacy_UnrelatedPlayerCannotReadApproval(t *testing.T) {
	f := seedApprovalFixture(t)
	cookies, _ := seedUnrelatedPlayer(t, f, "unrelated@example.com")

	resp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/matches/%d/approval", f.matchID), "", cookies, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 for an unrelated player reading approval detail, got %d", resp.StatusCode)
	}
}

func TestReadPrivacy_ScopedAdminCanReadApproval(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	cookies, _ := loginCookies(t, f.srv, "admin@example.com", "hunter22")

	resp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/matches/%d/approval", f.matchID), "", cookies, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 for a league_admin scoped to this match's league, got %d", resp.StatusCode)
	}
}

func TestReadPrivacy_PublicListDoesNotSerializeApprovalNote(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	adminCookies, adminCSRF := loginCookies(t, f.srv, "admin@example.com", "hunter22")
	acceptResp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve", f.matchID),
		`{"note":"secret admin note","expected_revision":0,"override":true}`, adminCookies, map[string]string{"X-CSRF-Token": adminCSRF})
	acceptResp.Body.Close()
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("admin accept setup: want 200, got %d", acceptResp.StatusCode)
	}

	resp := authDo(t, f.srv, "GET", "/api/matches", "", nil, nil)
	defer resp.Body.Close()
	raw := readBody(t, resp)
	if strings.Contains(raw, "secret admin note") {
		t.Errorf("want approval_note never serialized to a public caller, but the raw response contained it: %s", raw)
	}
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(raw)
}

// ============================================================
// PM correction round (2026-09-30)
// ============================================================

// --- Correction 1: ActionMatchTeamApproval must not inherit the
// system_admin bypass -- handler-level proof alongside the policy-level
// TestAuthorize_MatchTeamApproval_* tests in backend/domains/auth. ---

func TestApprovalAuth_SystemAdminAPIKeyLinkedToEligiblePlayer_Rejected(t *testing.T) {
	f := seedApprovalFixture(t)

	applyAuthStore := sqlite.NewApplyAuthStore(db.DB)
	user, cleartext, err := applyAuthStore.CreateApplyPlayerUser(context.Background(), "admin-key-user", f.homePlayerID)
	if err != nil {
		t.Fatalf("CreateApplyPlayerUser: %v", err)
	}
	roleStore := sqlite.NewRoleAssignmentStore(db.DB)
	if err := roleStore.Grant(context.Background(), user.ID, auth.RoleSystemAdmin, nil, nil); err != nil {
		t.Fatalf("grant system_admin: %v", err)
	}

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, nil, map[string]string{"Authorization": "Bearer " + cleartext})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (a system_admin's personal API key, even linked to an eligible player, must not authorize a player-scoped action), got %d", resp.StatusCode)
	}
}

func TestApprovalAuth_SystemAdminSessionNoLinkedPlayer_Rejected(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, csrf := loginCookies(t, f.srv, "sysadmin@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("want 403 (a system_admin browser session with no linked player must not authorize a player-scoped action -- role alone is never enough), got %d", resp.StatusCode)
	}
}

func TestApprovalAuth_SystemAdminSessionLinkedEligiblePlayer_Succeeds(t *testing.T) {
	f := seedApprovalFixture(t)
	userID := seedPasswordUser(t, "sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	linkUserToPlayer(t, userID, f.homePlayerID)
	cookies, csrf := loginCookies(t, f.srv, "sysadmin@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 (a system_admin browser session linked to an eligible player is authorized AS THAT PLAYER, following normal eligibility), got %d: %s", resp.StatusCode, readBody(t, resp))
	}
}

func TestApprovalAuth_SystemAdminSessionLinkedIneligiblePlayer_StillIneligible(t *testing.T) {
	f := seedApprovalFixture(t)
	res, err := db.DB.Exec(`INSERT INTO players (first_name, last_name) VALUES ('Sys', 'Admin')`)
	if err != nil {
		t.Fatalf("seed unrelated player: %v", err)
	}
	playerID, _ := res.LastInsertId()
	userID := seedPasswordUser(t, "sysadmin2@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	linkUserToPlayer(t, userID, playerID)
	cookies, csrf := loginCookies(t, f.srv, "sysadmin2@example.com", "hunter22")

	resp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve-home", f.matchID),
		`{"expected_revision":0}`, cookies, map[string]string{"X-CSRF-Token": csrf})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("want 422 (a system_admin's linked player must still satisfy real eligibility -- role grants no shortcut past it), got %d", resp.StatusCode)
	}
}

// --- Correction 2: GET /api/matches must scope approval metadata to the
// request's effective season_id/league_id filter, never to "any
// authenticated league_admin." ---

// seedSecondLeagueMatchWithAcceptedNote creates an independent league,
// season, and admin-accepted match carrying a distinctive secret note and
// actor id, entirely unrelated to seedApprovalFixture's league -- used to
// prove a league_admin scoped to a DIFFERENT league cannot see this
// note/actor id via any list-route query shape. Written directly via SQL
// (not through the admin-accept route) since only the read side is under
// test here.
func seedSecondLeagueMatchWithAcceptedNote(t *testing.T, note string) (leagueID, seasonID, matchID int64) {
	t.Helper()
	leagueID = seedTestLeague(t, "Other League")
	res, err := db.DB.Exec(`INSERT INTO seasons (league_id, name, active, teams_managed) VALUES (?,?,1,0)`, leagueID, "Other Season")
	if err != nil {
		t.Fatalf("seed other season: %v", err)
	}
	seasonID, _ = res.LastInsertId()
	rh, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Other Home')`, leagueID)
	homeTeamID, _ := rh.LastInsertId()
	ra, _ := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,'Other Away')`, leagueID)
	awayTeamID, _ := ra.LastInsertId()
	rm, err := db.DB.Exec(`INSERT INTO matches (season_id, home_team_id, away_team_id, week_number, completed) VALUES (?,?,?,1,1)`,
		seasonID, homeTeamID, awayTeamID)
	if err != nil {
		t.Fatalf("seed other match: %v", err)
	}
	matchID, _ = rm.LastInsertId()
	if _, err := db.DB.Exec(`UPDATE matches SET approved_at=CURRENT_TIMESTAMP, approved_by_user_id=999999, approval_note=? WHERE id=?`,
		note, matchID); err != nil {
		t.Fatalf("seed other match approval: %v", err)
	}
	return leagueID, seasonID, matchID
}

func TestListMatches_LeagueAdmin_SeesMetadataForAuthorizedSeasonFilter(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	adminCookies, adminCSRF := loginCookies(t, f.srv, "admin@example.com", "hunter22")
	acceptResp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve", f.matchID),
		`{"note":"home league secret note A","expected_revision":0,"override":true}`, adminCookies, map[string]string{"X-CSRF-Token": adminCSRF})
	acceptResp.Body.Close()
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("admin accept setup: want 200, got %d", acceptResp.StatusCode)
	}

	resp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/matches?season_id=%d", f.seasonID), "", adminCookies, nil)
	defer resp.Body.Close()
	raw := readBody(t, resp)
	if !strings.Contains(raw, "home league secret note A") {
		t.Errorf("want a league_admin querying their own authorized season to see approval_note, got %s", raw)
	}
}

func TestListMatches_LeagueAdmin_SeesMetadataForAuthorizedLeagueFilter(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	adminCookies, adminCSRF := loginCookies(t, f.srv, "admin@example.com", "hunter22")
	acceptResp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve", f.matchID),
		`{"note":"home league secret note B","expected_revision":0,"override":true}`, adminCookies, map[string]string{"X-CSRF-Token": adminCSRF})
	acceptResp.Body.Close()
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("admin accept setup: want 200, got %d", acceptResp.StatusCode)
	}

	resp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/matches?league_id=%d", f.leagueID), "", adminCookies, nil)
	defer resp.Body.Close()
	raw := readBody(t, resp)
	if !strings.Contains(raw, "home league secret note B") {
		t.Errorf("want a league_admin querying their own authorized league to see approval_note, got %s", raw)
	}
}

func TestListMatches_LeagueAdmin_CrossLeagueFilterRedacted(t *testing.T) {
	f := seedApprovalFixture(t)
	_, otherSeasonID, _ := seedSecondLeagueMatchWithAcceptedNote(t, "other league secret note C")
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	adminCookies, _ := loginCookies(t, f.srv, "admin@example.com", "hunter22")

	resp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/matches?season_id=%d", otherSeasonID), "", adminCookies, nil)
	defer resp.Body.Close()
	raw := readBody(t, resp)
	if strings.Contains(raw, "other league secret note C") {
		t.Errorf("want a league_admin querying a season belonging to a league they do NOT administer to get redacted metadata, got %s", raw)
	}
}

func TestListMatches_LeagueAdmin_UnfilteredRequestRedactsMetadataEvenForOwnLeague(t *testing.T) {
	f := seedApprovalFixture(t)
	_, _, _ = seedSecondLeagueMatchWithAcceptedNote(t, "other league secret note D")
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	adminCookies, adminCSRF := loginCookies(t, f.srv, "admin@example.com", "hunter22")
	acceptResp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve", f.matchID),
		`{"note":"home league secret note E","expected_revision":0,"override":true}`, adminCookies, map[string]string{"X-CSRF-Token": adminCSRF})
	acceptResp.Body.Close()
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("admin accept setup: want 200, got %d", acceptResp.StatusCode)
	}

	resp := authDo(t, f.srv, "GET", "/api/matches", "", adminCookies, nil)
	defer resp.Body.Close()
	raw := readBody(t, resp)
	if strings.Contains(raw, "other league secret note D") {
		t.Errorf("want an unfiltered league_admin request to never expose another league's approval_note, got %s", raw)
	}
	if strings.Contains(raw, "home league secret note E") {
		t.Errorf("want an unfiltered league_admin request to redact metadata even for the admin's OWN league (no single effective filter to authorize against), got %s", raw)
	}
}

func TestListMatches_SystemAdmin_SeesMetadataRegardlessOfFilter(t *testing.T) {
	f := seedApprovalFixture(t)
	_ = f
	_, _, _ = seedSecondLeagueMatchWithAcceptedNote(t, "system admin visible note F")
	seedPasswordUser(t, "sysadmin@example.com", "hunter22", auth.RoleSystemAdmin, nil)
	cookies, _ := loginCookies(t, f.srv, "sysadmin@example.com", "hunter22")

	resp := authDo(t, f.srv, "GET", "/api/matches", "", cookies, nil)
	defer resp.Body.Close()
	raw := readBody(t, resp)
	if !strings.Contains(raw, "system admin visible note F") {
		t.Errorf("want system_admin to see approval metadata regardless of filter (even unfiltered), got %s", raw)
	}
}

// --- Correction 3: a FINAL (completed && week_closed) match must still
// redact approval metadata for an unauthorized caller, independent of the
// results-visibility rule. ---

func TestReadPrivacy_FinalMatchDetail_RedactsApprovalMetadataForPublic(t *testing.T) {
	f := seedApprovalFixture(t)
	seedPasswordUser(t, "admin@example.com", "hunter22", auth.RoleLeagueAdmin, &f.leagueID)
	adminCookies, adminCSRF := loginCookies(t, f.srv, "admin@example.com", "hunter22")
	acceptResp := authDo(t, f.srv, "POST", fmt.Sprintf("/api/matches/%d/approve", f.matchID),
		`{"note":"final match secret note","expected_revision":0,"override":true}`, adminCookies, map[string]string{"X-CSRF-Token": adminCSRF})
	acceptResp.Body.Close()
	if acceptResp.StatusCode != http.StatusOK {
		t.Fatalf("admin accept setup: want 200, got %d", acceptResp.StatusCode)
	}
	if _, err := db.DB.Exec(`UPDATE matches SET week_closed=1 WHERE id=?`, f.matchID); err != nil {
		t.Fatalf("mark week closed: %v", err)
	}
	// A real match_results row so this test can also confirm final results
	// remain visible to the public alongside the metadata redaction.
	if _, err := db.DB.Exec(`INSERT INTO match_results (match_id, player_id, team_id, sets_won, sets_lost, games_won, games_lost, diff) VALUES (?,?,?,1,0,20,10,10.0)`,
		f.matchID, f.homePlayerID, f.homeTeamID); err != nil {
		t.Fatalf("seed match_results: %v", err)
	}

	resp := authDo(t, f.srv, "GET", fmt.Sprintf("/api/matches/%d", f.matchID), "", nil, nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}
	raw := readBody(t, resp)
	if strings.Contains(raw, "final match secret note") {
		t.Fatalf("want a public caller to never see approval_note on a final match, but the raw response contained it: %s", raw)
	}

	var detail struct {
		Match struct {
			ApprovalNote     string `json:"approval_note"`
			ApprovedByUserID *int64 `json:"approved_by_user_id"`
		} `json:"match"`
		Results []map[string]any `json:"results"`
	}
	if err := json.Unmarshal([]byte(raw), &detail); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if detail.Match.ApprovalNote != "" {
		t.Errorf("want approval_note absent/empty for a public caller, got %q", detail.Match.ApprovalNote)
	}
	if detail.Match.ApprovedByUserID != nil {
		t.Errorf("want approved_by_user_id absent for a public caller, got %v", *detail.Match.ApprovedByUserID)
	}
	if len(detail.Results) == 0 {
		t.Error("want final (completed && week_closed) results to remain visible to a public caller")
	}
}
