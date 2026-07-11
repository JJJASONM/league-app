package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"league_app/backend/domains/seasons"
	"league_app/models"
)

// SeasonStore implements seasons.SeasonStore against a SQLite database.
type SeasonStore struct {
	db *sql.DB
}

// NewSeasonStore returns a SeasonStore backed by db.
func NewSeasonStore(db *sql.DB) *SeasonStore {
	return &SeasonStore{db: db}
}

// IsDraft returns true when activated_at IS NULL.
func (s *SeasonStore) IsDraft(ctx context.Context, seasonID int64) (bool, error) {
	var activatedAt *string
	err := s.db.QueryRowContext(ctx,
		`SELECT activated_at FROM seasons WHERE id=?`, seasonID,
	).Scan(&activatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, fmt.Errorf("season %d: %w", seasonID, seasons.ErrNotFound)
	}
	if err != nil {
		return false, fmt.Errorf("is-draft season %d: %w", seasonID, err)
	}
	return activatedAt == nil, nil
}

// GetMeta returns lifecycle columns for the given season.
// Returns ErrNotFound (wrapped) when the season does not exist.
func (s *SeasonStore) GetMeta(ctx context.Context, seasonID int64) (seasons.SeasonMeta, error) {
	var m seasons.SeasonMeta
	var startDate, endDate sql.NullString
	var stale, managed int
	// strftime forces TEXT return so the SQLite driver does not convert DATE to time.Time.
	err := s.db.QueryRowContext(ctx,
		`SELECT league_id,
		        strftime('%Y-%m-%d', start_date),
		        strftime('%Y-%m-%d', end_date),
		        COALESCE(schedule_stale,0), COALESCE(teams_managed,0)
		 FROM seasons WHERE id=?`, seasonID,
	).Scan(&m.LeagueID, &startDate, &endDate, &stale, &managed)
	if errors.Is(err, sql.ErrNoRows) {
		return seasons.SeasonMeta{}, fmt.Errorf("season %d: %w", seasonID, seasons.ErrNotFound)
	}
	if err != nil {
		return seasons.SeasonMeta{}, fmt.Errorf("get season meta %d: %w", seasonID, err)
	}
	if startDate.Valid && startDate.String != "" {
		v := startDate.String
		m.StartDate = &v
	}
	if endDate.Valid && endDate.String != "" {
		v := endDate.String
		m.EndDate = &v
	}
	m.ScheduleStale = stale == 1
	m.TeamsManaged = managed == 1
	return m, nil
}

// GetTeamSummaries returns per-team checklist data ordered by season_teams.id.
func (s *SeasonStore) GetTeamSummaries(ctx context.Context, seasonID int64) ([]seasons.TeamSummary, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT st.team_id,
		       CASE WHEN st.season_name != '' THEN st.season_name ELSE t.name END,
		       st.captain_id,
		       (SELECT COUNT(*) FROM season_rosters sr
		        WHERE sr.season_id = st.season_id AND sr.team_id = st.team_id) AS roster_count,
		       CASE WHEN st.captain_id IS NULL THEN 0
		            ELSE (SELECT COUNT(*) FROM season_rosters sr2
		                  WHERE sr2.season_id = st.season_id AND sr2.team_id = st.team_id
		                    AND sr2.player_id = st.captain_id)
		       END AS captain_on_roster
		FROM season_teams st
		JOIN teams t ON t.id = st.team_id
		WHERE st.season_id = ?
		ORDER BY st.id`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("get team summaries season %d: %w", seasonID, err)
	}
	defer rows.Close()

	out := []seasons.TeamSummary{}
	for rows.Next() {
		var ts seasons.TeamSummary
		var onRoster int
		if err := rows.Scan(&ts.TeamID, &ts.Name, &ts.CaptainID, &ts.RosterCount, &onRoster); err != nil {
			return nil, fmt.Errorf("scan team summary: %w", err)
		}
		ts.CaptainOnRoster = onRoster > 0
		out = append(out, ts)
	}
	return out, rows.Err()
}

// GetMatchCount returns the total number of matches for the season.
func (s *SeasonStore) GetMatchCount(ctx context.Context, seasonID int64) (int, error) {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM matches WHERE season_id=?`, seasonID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("get match count season %d: %w", seasonID, err)
	}
	return n, nil
}

// Activate atomically deactivates all other seasons in leagueID and activates seasonID.
func (s *SeasonStore) Activate(ctx context.Context, seasonID, leagueID int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("activate season %d: begin tx: %w", seasonID, err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`UPDATE seasons SET active=0 WHERE league_id=?`, leagueID,
	); err != nil {
		return fmt.Errorf("activate season %d: deactivate others: %w", seasonID, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE seasons SET active=1, activated_at=COALESCE(activated_at, CURRENT_TIMESTAMP) WHERE id=?`,
		seasonID,
	); err != nil {
		return fmt.Errorf("activate season %d: set active: %w", seasonID, err)
	}
	return tx.Commit()
}

// MarkStaleIfScheduled sets schedule_stale=1 when unplayed matches exist.
func (s *SeasonStore) MarkStaleIfScheduled(ctx context.Context, seasonID int64) error {
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM matches WHERE season_id=? AND completed=0`, seasonID,
	).Scan(&n); err != nil {
		return fmt.Errorf("mark stale season %d: count: %w", seasonID, err)
	}
	if n > 0 {
		if _, err := s.db.ExecContext(ctx,
			`UPDATE seasons SET schedule_stale=1 WHERE id=?`, seasonID,
		); err != nil {
			return fmt.Errorf("mark stale season %d: update: %w", seasonID, err)
		}
	}
	return nil
}

// FindActiveWithNoEndDate returns the active season in the league with no end_date,
// excluding excludeSeasonID.
func (s *SeasonStore) FindActiveWithNoEndDate(ctx context.Context, leagueID, excludeSeasonID int64) (*models.Season, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT `+seasonCols+`
		FROM seasons
		WHERE league_id=? AND id!=? AND active=1 AND end_date IS NULL
		LIMIT 1`, leagueID, excludeSeasonID)
	return scanSeasonRow(row)
}

// FindClosestPriorByEndDate returns the season with the greatest end_date before
// beforeDate, or the most recent if beforeDate is nil.
func (s *SeasonStore) FindClosestPriorByEndDate(ctx context.Context, leagueID, excludeSeasonID int64, beforeDate *string) (*models.Season, error) {
	var row *sql.Row
	if beforeDate != nil && *beforeDate != "" {
		row = s.db.QueryRowContext(ctx, `
			SELECT `+seasonCols+`
			FROM seasons
			WHERE league_id=? AND id!=? AND end_date IS NOT NULL AND end_date < ?
			ORDER BY end_date DESC LIMIT 1`, leagueID, excludeSeasonID, *beforeDate)
	} else {
		row = s.db.QueryRowContext(ctx, `
			SELECT `+seasonCols+`
			FROM seasons
			WHERE league_id=? AND id!=? AND end_date IS NOT NULL
			ORDER BY end_date DESC LIMIT 1`, leagueID, excludeSeasonID)
	}
	return scanSeasonRow(row)
}

// GetSeasonTeams returns teams registered in the season via season_teams.
func (s *SeasonStore) GetSeasonTeams(ctx context.Context, seasonID int64) ([]seasons.SeasonTeamEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT st.team_id,
		       t.name,
		       CASE WHEN st.season_name != '' THEN st.season_name ELSE t.name END,
		       st.captain_id
		FROM season_teams st
		JOIN teams t ON t.id = st.team_id
		WHERE st.season_id = ?
		ORDER BY st.id`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("get season teams %d: %w", seasonID, err)
	}
	defer rows.Close()

	out := []seasons.SeasonTeamEntry{}
	for rows.Next() {
		var e seasons.SeasonTeamEntry
		if err := rows.Scan(&e.TeamID, &e.TeamName, &e.SeasonName, &e.CaptainID); err != nil {
			return nil, fmt.Errorf("scan season team: %w", err)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// GetMatchTeams returns distinct teams that appeared in the season's matches.
func (s *SeasonStore) GetMatchTeams(ctx context.Context, seasonID int64) ([]seasons.SeasonTeamEntry, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT t.id, t.name
		FROM teams t
		JOIN matches m ON (m.home_team_id = t.id OR m.away_team_id = t.id)
		WHERE m.season_id = ?
		ORDER BY t.name`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("get match teams %d: %w", seasonID, err)
	}
	defer rows.Close()

	out := []seasons.SeasonTeamEntry{}
	for rows.Next() {
		var e seasons.SeasonTeamEntry
		if err := rows.Scan(&e.TeamID, &e.TeamName); err != nil {
			return nil, fmt.Errorf("scan match team: %w", err)
		}
		e.SeasonName = e.TeamName
		out = append(out, e)
	}
	return out, rows.Err()
}

// FindActiveSeasonByLeague returns the ID of the active season in leagueID.
// Returns (0, false, nil) when no active season exists.
func (s *SeasonStore) FindActiveSeasonByLeague(ctx context.Context, leagueID int64) (int64, bool, error) {
	var id int64
	err := s.db.QueryRowContext(ctx,
		`SELECT id FROM seasons WHERE league_id=? AND active=1 LIMIT 1`, leagueID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("find active season league %d: %w", leagueID, err)
	}
	return id, true, nil
}

// RosterEligible returns (true, "", nil) when both teams in a match have at
// least minPlayers season-roster players, or when the season is not managed.
// Returns (true, "", nil) when the match is not found.
// Returns (false, msg, nil) when a team is ineligible.
func (s *SeasonStore) RosterEligible(ctx context.Context, matchID int64, minPlayers int) (bool, string, error) {
	var seasonID, homeID, awayID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT season_id, COALESCE(home_team_id,0), COALESCE(away_team_id,0)
		 FROM matches WHERE id=?`, matchID,
	).Scan(&seasonID, &homeID, &awayID)
	if errors.Is(err, sql.ErrNoRows) {
		return true, "", nil // match not found; let other validation catch it
	}
	if err != nil {
		return false, "", fmt.Errorf("roster eligible match %d: %w", matchID, err)
	}

	var teamsManaged int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COALESCE(teams_managed,0) FROM seasons WHERE id=?`, seasonID,
	).Scan(&teamsManaged); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, "", fmt.Errorf("roster eligible season %d: %w", seasonID, err)
	}
	if teamsManaged == 0 {
		return true, "", nil
	}

	check := func(teamID int64, label string) (bool, string, error) {
		var n int
		if err := s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM season_rosters WHERE season_id=? AND team_id=?`,
			seasonID, teamID).Scan(&n); err != nil {
			return false, "", fmt.Errorf("roster count team %d: %w", teamID, err)
		}
		if n < minPlayers {
			return false, fmt.Sprintf(
				"%s team has %d season-roster player(s); %d required to use a scoresheet",
				label, n, minPlayers), nil
		}
		return true, "", nil
	}

	if ok, msg, err := check(homeID, "home"); err != nil || !ok {
		return ok, msg, err
	}
	if ok, msg, err := check(awayID, "away"); err != nil || !ok {
		return ok, msg, err
	}
	return true, "", nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

const seasonCols = `id, league_id, name, start_date, end_date, active, schedule_type,
	COALESCE(num_weeks,0), COALESCE(schedule_stale,0), created_at`

// scanSeasonRow scans a *sql.Row into a *models.Season.
// Returns nil, nil for sql.ErrNoRows.
func scanSeasonRow(row *sql.Row) (*models.Season, error) {
	var s models.Season
	var active, stale int
	var createdAt time.Time
	err := row.Scan(
		&s.ID, &s.LeagueID, &s.Name, &s.StartDate, &s.EndDate,
		&active, &s.ScheduleType, &s.NumWeeks, &stale, &createdAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan season: %w", err)
	}
	s.Active = active == 1
	s.ScheduleStale = stale == 1
	s.CreatedAt = createdAt
	s.StartDate = trimDatePtr(s.StartDate)
	s.EndDate = trimDatePtr(s.EndDate)
	return &s, nil
}

// trimDatePtr trims whitespace from a *string date value and returns nil
// for blank strings, matching the normDatePtr behaviour in handlers.
func trimDatePtr(s *string) *string {
	if s == nil {
		return nil
	}
	v := strings.TrimSpace(*s)
	if v == "" {
		return nil
	}
	return &v
}
