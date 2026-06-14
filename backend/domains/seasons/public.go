// Package seasons owns season-level business logic: setup checklist and
// previous-season lookup. Handlers call these functions; they accept *sql.DB
// so they are testable independently of the global db.DB singleton.
package seasons

import (
	"database/sql"
	"fmt"
	"league_app/models"
)

// Checklist computes activation blockers and warnings for a draft season.
//
// Team-based checks (TEAMS_TOO_FEW, TEAM_NO_PLAYERS, TEAM_NO_CAPTAIN,
// CAPTAIN_NOT_ON_ROSTER, SCHEDULE_STALE, NO_SCHEDULE, NO_END_DATE) are only
// evaluated for managed seasons (teams_managed=1). Legacy seasons (teams_managed=0,
// which is the DEFAULT for rows created before Phase One) bypass all enforcement
// and always return can_activate=true.
func Checklist(db *sql.DB, seasonID int64) (models.SetupChecklist, error) {
	c := models.SetupChecklist{
		Blockers: []models.ChecklistItem{},
		Warnings: []models.ChecklistItem{},
	}

	var endDate *string
	var scheduleStale, teamsManaged int
	err := db.QueryRow(
		`SELECT end_date, COALESCE(schedule_stale,0), COALESCE(teams_managed,0) FROM seasons WHERE id=?`, seasonID,
	).Scan(&endDate, &scheduleStale, &teamsManaged)
	if err == sql.ErrNoRows {
		return c, fmt.Errorf("season %d not found", seasonID)
	}
	if err != nil {
		return c, err
	}

	// Skip all enforcement for legacy seasons (teams_managed=0).
	if teamsManaged == 0 {
		c.CanActivate = true
		return c, nil
	}

	var teamCount int
	db.QueryRow(`SELECT COUNT(*) FROM season_teams WHERE season_id=?`, seasonID).Scan(&teamCount)

	// ── Team count ────────────────────────────────────────────────────────────
	if teamCount < 2 {
		c.Blockers = append(c.Blockers, models.ChecklistItem{
			Code:    "TEAMS_TOO_FEW",
			Message: fmt.Sprintf("season has %d participating team; at least 2 required", teamCount),
		})
	}

	// ── Per-team checks ───────────────────────────────────────────────────────
	rows, err := db.Query(`
		SELECT st.team_id,
		       CASE WHEN st.season_name != '' THEN st.season_name ELSE t.name END,
		       st.captain_id,
		       (SELECT COUNT(*) FROM season_rosters sr
		        WHERE sr.season_id = st.season_id AND sr.team_id = st.team_id)
		FROM season_teams st
		JOIN teams t ON t.id = st.team_id
		WHERE st.season_id = ?
		ORDER BY st.id`, seasonID)
	if err != nil {
		return c, err
	}
	defer rows.Close()

	for rows.Next() {
		var tid int64
		var name string
		var capID *int64
		var rosterCount int
		if err := rows.Scan(&tid, &name, &capID, &rosterCount); err != nil {
			continue
		}

		if rosterCount == 0 {
			c.Blockers = append(c.Blockers, models.ChecklistItem{
				Code:    "TEAM_NO_PLAYERS",
				Message: fmt.Sprintf("team %q has no rostered players", name),
				TeamID:  tid,
			})
		} else if rosterCount < 3 {
			c.Warnings = append(c.Warnings, models.ChecklistItem{
				Code:    "TEAM_FEW_PLAYERS",
				Message: fmt.Sprintf("team %q has %d player(s); 3 or more recommended for match play", name, rosterCount),
				TeamID:  tid,
			})
		}

		if capID == nil {
			c.Blockers = append(c.Blockers, models.ChecklistItem{
				Code:    "TEAM_NO_CAPTAIN",
				Message: fmt.Sprintf("team %q has no captain assigned", name),
				TeamID:  tid,
			})
		} else {
			var onRoster int
			db.QueryRow(
				`SELECT COUNT(*) FROM season_rosters
				 WHERE season_id=? AND team_id=? AND player_id=?`,
				seasonID, tid, *capID).Scan(&onRoster)
			if onRoster == 0 {
				c.Blockers = append(c.Blockers, models.ChecklistItem{
					Code:    "CAPTAIN_NOT_ON_ROSTER",
					Message: fmt.Sprintf("team %q captain is not on the season roster", name),
					TeamID:  tid,
				})
			}
		}
	}
	rows.Close()

	// ── Schedule checks ───────────────────────────────────────────────────────
	if scheduleStale == 1 {
		c.Blockers = append(c.Blockers, models.ChecklistItem{
			Code:    "SCHEDULE_STALE",
			Message: "schedule is stale after team changes; regenerate before activating",
		})
	}

	var matchCount int
	db.QueryRow(`SELECT COUNT(*) FROM matches WHERE season_id=?`, seasonID).Scan(&matchCount)
	if matchCount == 0 {
		c.Blockers = append(c.Blockers, models.ChecklistItem{
			Code:    "NO_SCHEDULE",
			Message: "no schedule has been generated for this season",
		})
	} else if endDate == nil || *endDate == "" {
		c.Blockers = append(c.Blockers, models.ChecklistItem{
			Code:    "NO_END_DATE",
			Message: "season has no calculable end date; regenerate the schedule",
		})
	}

	c.CanActivate = len(c.Blockers) == 0
	return c, nil
}

// PreviousSeason returns the immediately previous season for the given season
// within the same league. Lookup priority:
//  1. A season in the same league that is currently active (active=1) and has no
//     end_date — this is the "currently running" season that hasn't been closed yet.
//  2. The season with the greatest end_date that is earlier than the draft season's
//     start_date. If the draft has no start_date, the most recently ended season is
//     returned.
//
// Returns nil, nil when no prior season exists.
func PreviousSeason(db *sql.DB, seasonID, leagueID int64, startDate *string) (*models.Season, error) {
	const cols = `id, league_id, name, start_date, end_date, active, schedule_type,
	              COALESCE(num_weeks,0), COALESCE(schedule_stale,0), created_at`

	scan := func(row *sql.Row) (*models.Season, error) {
		var s models.Season
		var active, stale int
		err := row.Scan(&s.ID, &s.LeagueID, &s.Name, &s.StartDate, &s.EndDate,
			&active, &s.ScheduleType, &s.NumWeeks, &stale, &s.CreatedAt)
		if err == sql.ErrNoRows {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		s.Active = active == 1
		s.ScheduleStale = stale == 1
		return &s, nil
	}

	// Priority 1: active season in the same league with no end_date.
	if s, err := scan(db.QueryRow(`SELECT `+cols+`
		FROM seasons
		WHERE league_id=? AND id!=? AND active=1 AND end_date IS NULL
		LIMIT 1`, leagueID, seasonID)); s != nil || err != nil {
		return s, err
	}

	// Priority 2: closest prior end_date.
	var row *sql.Row
	if startDate != nil && *startDate != "" {
		row = db.QueryRow(`SELECT `+cols+`
			FROM seasons
			WHERE league_id=? AND id!=? AND end_date IS NOT NULL AND end_date < ?
			ORDER BY end_date DESC LIMIT 1`,
			leagueID, seasonID, *startDate)
	} else {
		row = db.QueryRow(`SELECT `+cols+`
			FROM seasons
			WHERE league_id=? AND id!=? AND end_date IS NOT NULL
			ORDER BY end_date DESC LIMIT 1`,
			leagueID, seasonID)
	}
	return scan(row)
}

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
