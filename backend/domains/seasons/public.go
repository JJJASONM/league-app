// Package seasons owns season-level business logic.
// SeasonService (service.go) owns the lifecycle: activation, checklist, and
// previous-season lookup. RosterEligible remains here as a standalone function
// used by the scoresheet handler via *sql.DB.
package seasons

import (
	"database/sql"
	"fmt"
)

// RosterEligible returns true when both teams in a match have at least minPlayers
// season-roster players. When the season has no season_teams records (legacy data),
// the check is skipped and true is returned.
func RosterEligible(db *sql.DB, matchID int64, minPlayers int) (bool, string) {
	var seasonID, homeID, awayID int64
	if err := db.QueryRow(
		`SELECT season_id, COALESCE(home_team_id,0), COALESCE(away_team_id,0)
		 FROM matches WHERE id=?`, matchID,
	).Scan(&seasonID, &homeID, &awayID); err != nil {
		return true, "" // match not found; let other validation catch it
	}

	// If season is not managed (legacy), skip the check (backward-compatible).
	var teamsManaged int
	db.QueryRow(`SELECT COALESCE(teams_managed,0) FROM seasons WHERE id=?`, seasonID).Scan(&teamsManaged)
	if teamsManaged == 0 {
		return true, ""
	}

	check := func(teamID int64, label string) (bool, string) {
		var n int
		db.QueryRow(
			`SELECT COUNT(*) FROM season_rosters WHERE season_id=? AND team_id=?`,
			seasonID, teamID).Scan(&n)
		if n < minPlayers {
			return false, fmt.Sprintf(
				"%s team has %d season-roster player(s); %d required to use a scoresheet",
				label, n, minPlayers)
		}
		return true, ""
	}

	if ok, msg := check(homeID, "home"); !ok {
		return false, msg
	}
	if ok, msg := check(awayID, "away"); !ok {
		return false, msg
	}
	return true, ""
}
