package sqlite

import (
	"context"
	"database/sql"

	"league_app/backend/domains/matches"
	"league_app/models"
)

// matchSelect is the standard column list for match queries.
// Uses LEFT JOIN so unassigned (blanket) slots with NULL team IDs are included.
const matchSelect = `
	SELECT m.id, m.season_id,
	       COALESCE(m.home_team_id,0), COALESCE(ht.name,'(unassigned)'),
	       COALESCE(m.away_team_id,0), COALESCE(at.name,'(unassigned)'),
	       m.match_date, m.week_number, m.completed, m.created_at
	FROM matches m
	LEFT JOIN teams ht ON ht.id = m.home_team_id
	LEFT JOIN teams at ON at.id = m.away_team_id`

// MatchStore is the SQLite implementation of matches.MatchStore.
type MatchStore struct {
	db *sql.DB
}

// NewMatchStore returns a MatchStore backed by the given database connection.
func NewMatchStore(db *sql.DB) *MatchStore {
	return &MatchStore{db: db}
}

// ListMatches returns matches filtered by the request, ordered by week_number then id.
// Both filter fields are optional.
func (s *MatchStore) ListMatches(ctx context.Context, req matches.ListMatchesRequest) ([]models.Match, error) {
	var rows *sql.Rows
	var err error
	switch {
	case req.SeasonID != 0:
		rows, err = s.db.QueryContext(ctx,
			matchSelect+` WHERE m.season_id=? ORDER BY m.week_number, m.id`, req.SeasonID)
	case req.LeagueID != 0:
		rows, err = s.db.QueryContext(ctx,
			matchSelect+`
			JOIN seasons s ON s.id = m.season_id
			WHERE s.league_id=? ORDER BY m.week_number, m.id`, req.LeagueID)
	default:
		rows, err = s.db.QueryContext(ctx,
			matchSelect+` ORDER BY m.week_number, m.id`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ms []models.Match
	for rows.Next() {
		var m models.Match
		var completed int
		if err := rows.Scan(&m.ID, &m.SeasonID, &m.HomeTeamID, &m.HomeTeamName,
			&m.AwayTeamID, &m.AwayTeamName, &m.MatchDate, &m.WeekNumber, &completed, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.Completed = completed == 1
		m.MatchDate = normMatchDatePtr(m.MatchDate)
		ms = append(ms, m)
	}
	return ms, rows.Err()
}

// GetMatch returns the match and its player results for the given ID.
// Returns matches.ErrMatchNotFound when no match with that ID exists.
func (s *MatchStore) GetMatch(ctx context.Context, id int64) (models.MatchDetail, error) {
	var m models.Match
	var completed int
	err := s.db.QueryRowContext(ctx, matchSelect+` WHERE m.id=?`, id).
		Scan(&m.ID, &m.SeasonID, &m.HomeTeamID, &m.HomeTeamName,
			&m.AwayTeamID, &m.AwayTeamName, &m.MatchDate, &m.WeekNumber, &completed, &m.CreatedAt)
	if err == sql.ErrNoRows {
		return models.MatchDetail{}, matches.ErrMatchNotFound
	}
	if err != nil {
		return models.MatchDetail{}, err
	}
	m.Completed = completed == 1
	m.MatchDate = normMatchDatePtr(m.MatchDate)

	resRows, err := s.db.QueryContext(ctx, `
		SELECT mr.id, mr.match_id, mr.player_id,
		       p.first_name || ' ' || p.last_name, mr.team_id,
		       mr.sets_won, mr.sets_lost, mr.games_won, mr.games_lost, mr.diff, mr.created_at
		FROM match_results mr JOIN players p ON p.id = mr.player_id
		WHERE mr.match_id=?`, id)
	if err != nil {
		return models.MatchDetail{}, err
	}
	defer resRows.Close()

	results := []models.MatchResult{}
	for resRows.Next() {
		var res models.MatchResult
		if err := resRows.Scan(&res.ID, &res.MatchID, &res.PlayerID, &res.PlayerName, &res.TeamID,
			&res.SetsWon, &res.SetsLost, &res.GamesWon, &res.GamesLost, &res.Diff, &res.CreatedAt); err != nil {
			return models.MatchDetail{}, err
		}
		results = append(results, res)
	}
	if err := resRows.Err(); err != nil {
		return models.MatchDetail{}, err
	}
	return models.MatchDetail{Match: m, Results: results}, nil
}

// AssignMatchTeams sets home_team_id and away_team_id on the match. Either
// value may be nil to NULL the column.
func (s *MatchStore) AssignMatchTeams(ctx context.Context, id int64, homeTeamID, awayTeamID *int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE matches SET home_team_id=?, away_team_id=? WHERE id=?`,
		homeTeamID, awayTeamID, id)
	return err
}

// normMatchDatePtr truncates a date pointer to YYYY-MM-DD, discarding any time
// component added by the SQLite driver when it coerces DATE columns to time.Time.
func normMatchDatePtr(s *string) *string {
	if s == nil || len(*s) <= 10 {
		return s
	}
	v := (*s)[:10]
	return &v
}
