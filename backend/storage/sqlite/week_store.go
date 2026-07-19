package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"league_app/backend/domains/matches"
	"league_app/models"
)

// WeekStore implements matches.WeekStore against a SQLite database.
// Use NewWeekStore; do not copy by value after first use.
type WeekStore struct {
	db *sql.DB
}

// NewWeekStore returns a WeekStore backed by db.
func NewWeekStore(db *sql.DB) *WeekStore {
	return &WeekStore{db: db}
}

// ListWeekSummaries returns one WeekSummary per week that has matches in the
// given season, ordered by week_number. Status defaults to "open" when no
// league_weeks row exists. AckCount accumulates across all close cycles.
func (s *WeekStore) ListWeekSummaries(ctx context.Context, seasonID int64) ([]models.WeekSummary, error) {
	type weekCount struct{ total, completed, closed int }
	counts := map[int]weekCount{}
	var weekOrder []int
	seen := map[int]bool{}

	matchRows, err := s.db.QueryContext(ctx, `
		SELECT week_number, completed, week_closed
		FROM matches WHERE season_id=?
		ORDER BY week_number`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("list week summaries: matches: %w", err)
	}
	defer matchRows.Close()
	for matchRows.Next() {
		var wn, comp, wc int
		matchRows.Scan(&wn, &comp, &wc)
		c := counts[wn]
		c.total++
		c.completed += comp
		c.closed += wc
		counts[wn] = c
		if !seen[wn] {
			weekOrder = append(weekOrder, wn)
			seen[wn] = true
		}
	}

	type statusRow struct {
		status   string
		closedAt *string
	}
	statusMap := map[int]statusRow{}
	statusRows, err := s.db.QueryContext(ctx, `
		SELECT week_number, status, closed_at
		FROM league_weeks WHERE season_id=?`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("list week summaries: league_weeks: %w", err)
	}
	defer statusRows.Close()
	for statusRows.Next() {
		var wn int
		var st string
		var ca *string
		statusRows.Scan(&wn, &st, &ca)
		statusMap[wn] = statusRow{st, ca}
	}

	ackCounts := map[int]int{}
	ackRows, err := s.db.QueryContext(ctx, `
		SELECT week_number, COUNT(*) FROM week_close_acknowledgments
		WHERE season_id=? GROUP BY week_number`, seasonID)
	if err != nil {
		return nil, fmt.Errorf("list week summaries: ack counts: %w", err)
	}
	defer ackRows.Close()
	for ackRows.Next() {
		var wn, cnt int
		if err := ackRows.Scan(&wn, &cnt); err != nil {
			return nil, fmt.Errorf("list week summaries: ack count scan: %w", err)
		}
		ackCounts[wn] = cnt
	}
	if err := ackRows.Err(); err != nil {
		return nil, fmt.Errorf("list week summaries: ack rows: %w", err)
	}

	summaries := make([]models.WeekSummary, 0, len(weekOrder))
	for _, wn := range weekOrder {
		c := counts[wn]
		st := statusMap[wn]
		status := matches.WeekStatusOpen
		var closedAt *string
		if st.status != "" {
			status = st.status
			closedAt = st.closedAt
		}
		summaries = append(summaries, models.WeekSummary{
			WeekNumber:     wn,
			Status:         status,
			ClosedAt:       closedAt,
			MatchCount:     c.total,
			CompletedCount: c.completed,
			ClosedCount:    c.closed,
			AckCount:       ackCounts[wn],
		})
	}
	if summaries == nil {
		summaries = []models.WeekSummary{}
	}
	return summaries, nil
}

// WeekMatchCount returns the number of matches for the given season/week.
func (s *WeekStore) WeekMatchCount(ctx context.Context, seasonID, weekNum int64) (int, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM matches
		WHERE season_id=? AND week_number=?`, seasonID, weekNum).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("week match count: %w", err)
	}
	return count, nil
}

// GetWeekStatus returns league_weeks.status for the given season/week.
// Returns "", nil when no row exists (implicitly "open").
func (s *WeekStore) GetWeekStatus(ctx context.Context, seasonID, weekNum int64) (string, error) {
	var status string
	err := s.db.QueryRowContext(ctx, `
		SELECT status FROM league_weeks
		WHERE season_id=? AND week_number=?`, seasonID, weekNum).Scan(&status)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("get week status: %w", err)
	}
	return status, nil
}

// CloseWeek atomically upserts the league_weeks row to "closed", sets
// matches.week_closed=1, and inserts one acknowledgment row per entry in acks.
func (s *WeekStore) CloseWeek(ctx context.Context, seasonID, weekNum int64, acks []matches.AckEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("close week: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `
		INSERT INTO league_weeks (season_id, week_number, status, closed_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(season_id, week_number) DO UPDATE
		SET status=excluded.status, closed_at=CURRENT_TIMESTAMP`,
		seasonID, weekNum, matches.WeekStatusClosed); err != nil {
		return fmt.Errorf("close week: upsert league_weeks: %w", err)
	}

	if _, err = tx.ExecContext(ctx, `
		UPDATE matches SET week_closed=1
		WHERE season_id=? AND week_number=?`,
		seasonID, weekNum); err != nil {
		return fmt.Errorf("close week: update matches: %w", err)
	}

	for _, a := range acks {
		var matchIDVal interface{}
		if a.MatchID != 0 {
			matchIDVal = a.MatchID
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO week_close_acknowledgments
			    (season_id, week_number, match_id, warning_code, field, notes)
			VALUES (?, ?, ?, ?, ?, ?)`,
			seasonID, weekNum, matchIDVal, a.WarningCode, a.Field, a.Notes); err != nil {
			return fmt.Errorf("close week: insert ack: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("close week: commit: %w", err)
	}
	return nil
}

// ReopenWeek atomically sets league_weeks.status back to "open" and clears
// matches.week_closed for all matches in the week.
func (s *WeekStore) ReopenWeek(ctx context.Context, seasonID, weekNum int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("reopen week: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err = tx.ExecContext(ctx, `
		UPDATE league_weeks SET status=?, closed_at=NULL
		WHERE season_id=? AND week_number=?`, matches.WeekStatusOpen, seasonID, weekNum); err != nil {
		return fmt.Errorf("reopen week: update league_weeks: %w", err)
	}
	if _, err = tx.ExecContext(ctx, `
		UPDATE matches SET week_closed=0
		WHERE season_id=? AND week_number=?`, seasonID, weekNum); err != nil {
		return fmt.Errorf("reopen week: update matches: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("reopen week: commit: %w", err)
	}
	return nil
}

// GetWeekAdvanceSummary returns match counts, week status, and next-week
// readiness for the advance-preview and close-result response. Read-only.
// Returns an empty summary without error when no matches exist for the week.
func (s *WeekStore) GetWeekAdvanceSummary(ctx context.Context, seasonID, weekNum int64) (matches.WeekAdvanceSummary, error) {
	// Match counts for the closing week.
	var matchCount, completedCount, closedCount int
	cRows, err := s.db.QueryContext(ctx, `
		SELECT completed, week_closed FROM matches
		WHERE season_id=? AND week_number=?`, seasonID, weekNum)
	if err != nil {
		return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: match counts: %w", err)
	}
	defer cRows.Close()
	for cRows.Next() {
		var comp, wc int
		if err := cRows.Scan(&comp, &wc); err != nil {
			return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: match counts scan: %w", err)
		}
		matchCount++
		completedCount += comp
		closedCount += wc
	}
	if err := cRows.Err(); err != nil {
		return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: match counts rows: %w", err)
	}

	// Week status from league_weeks; absence means open.
	var weekStatus string
	switch err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(status,'') FROM league_weeks
		WHERE season_id=? AND week_number=?`, seasonID, weekNum).Scan(&weekStatus); err {
	case nil:
	case sql.ErrNoRows:
		weekStatus = matches.WeekStatusOpen
	default:
		return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: week status: %w", err)
	}
	if weekStatus == "" {
		weekStatus = matches.WeekStatusOpen
	}

	// Next scheduled week (COALESCE returns 0 when no later week exists).
	var nextWeekNum int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(MIN(week_number),0) FROM matches
		WHERE season_id=? AND week_number>?`, seasonID, weekNum).Scan(&nextWeekNum); err != nil {
		return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: next week: %w", err)
	}

	var nextWeekNumPtr *int
	var nextWeek *models.AdvancePreviewNextWeek

	if nextWeekNum > 0 {
		nextWeekNumPtr = &nextWeekNum

		var nextMatchCount int
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM matches WHERE season_id=? AND week_number=?`,
			seasonID, nextWeekNum).Scan(&nextMatchCount); err != nil {
			return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: next week match count: %w", err)
		}

		var assignedCount int
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM matches
			WHERE season_id=? AND week_number=?
			  AND home_team_id IS NOT NULL AND away_team_id IS NOT NULL`,
			seasonID, nextWeekNum).Scan(&assignedCount); err != nil {
			return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: next week assigned count: %w", err)
		}

		var lineupPlanCount int
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM lineup_plans WHERE season_id=? AND week_number=?`,
			seasonID, nextWeekNum).Scan(&lineupPlanCount); err != nil {
			return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: next week lineup count: %w", err)
		}

		// Distinct team IDs in next week's matches.
		teamRows, err := s.db.QueryContext(ctx, `
			SELECT DISTINCT t FROM (
				SELECT home_team_id AS t FROM matches
				WHERE season_id=? AND week_number=? AND home_team_id IS NOT NULL
				UNION
				SELECT away_team_id AS t FROM matches
				WHERE season_id=? AND week_number=? AND away_team_id IS NOT NULL
			)`, seasonID, nextWeekNum, seasonID, nextWeekNum)
		if err != nil {
			return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: next week teams: %w", err)
		}
		var allTeamIDs []int64
		for teamRows.Next() {
			var tid int64
			if err := teamRows.Scan(&tid); err != nil {
				teamRows.Close()
				return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: next week teams scan: %w", err)
			}
			allTeamIDs = append(allTeamIDs, tid)
		}
		teamRows.Close()
		if err := teamRows.Err(); err != nil {
			return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: next week teams rows: %w", err)
		}

		missingTeamIDs := make([]int64, 0)
		for _, tid := range allTeamIDs {
			var planCount int
			if err := s.db.QueryRowContext(ctx, `
				SELECT COUNT(*) FROM lineup_plans
				WHERE season_id=? AND week_number=? AND team_id=?`,
				seasonID, nextWeekNum, tid).Scan(&planCount); err != nil {
				return matches.WeekAdvanceSummary{}, fmt.Errorf("advance summary: lineup check team %d: %w", tid, err)
			}
			if planCount == 0 {
				missingTeamIDs = append(missingTeamIDs, tid)
			}
		}

		nextWeek = &models.AdvancePreviewNextWeek{
			MatchCount:           nextMatchCount,
			AssignedCount:        assignedCount,
			UnassignedCount:      nextMatchCount - assignedCount,
			LineupPlanCount:      lineupPlanCount,
			MissingLineupTeamIDs: missingTeamIDs,
		}
	}

	return matches.WeekAdvanceSummary{
		ClosedWeek: models.AdvancePreviewWeekSummary{
			MatchCount:     matchCount,
			CompletedCount: completedCount,
			ClosedCount:    closedCount,
			Status:         weekStatus,
		},
		NextWeekNumber: nextWeekNumPtr,
		NextWeek:       nextWeek,
	}, nil
}

// GetWeekValidationData returns all match and round data needed for week
// validation. For each round_results row, home_handicap_used / away_handicap_used
// snapshots take priority; the current player handicap is used as fallback via
// a LEFT JOIN, defaulting to 0 when neither value is available.
func (s *WeekStore) GetWeekValidationData(ctx context.Context, seasonID, weekNum int64) (matches.WeekValidationData, error) {
	mRows, err := s.db.QueryContext(ctx,
		`SELECT id, home_team_id, away_team_id
		 FROM matches WHERE season_id=? AND week_number=? ORDER BY id`,
		seasonID, weekNum)
	if err != nil {
		return matches.WeekValidationData{}, fmt.Errorf("get week validation data: matches: %w", err)
	}
	defer mRows.Close()

	type rawMatch struct {
		id         int64
		homeTeamID sql.NullInt64
		awayTeamID sql.NullInt64
	}
	var rawMatches []rawMatch
	for mRows.Next() {
		var rm rawMatch
		if err := mRows.Scan(&rm.id, &rm.homeTeamID, &rm.awayTeamID); err != nil {
			return matches.WeekValidationData{}, fmt.Errorf("get week validation data: match scan: %w", err)
		}
		rawMatches = append(rawMatches, rm)
	}
	if err := mRows.Err(); err != nil {
		return matches.WeekValidationData{}, fmt.Errorf("get week validation data: match rows: %w", err)
	}

	result := matches.WeekValidationData{
		Matches: make([]matches.MatchValidationRow, 0, len(rawMatches)),
	}

	for _, rm := range rawMatches {
		var homeID, awayID *int64
		if rm.homeTeamID.Valid {
			v := rm.homeTeamID.Int64
			homeID = &v
		}
		if rm.awayTeamID.Valid {
			v := rm.awayTeamID.Int64
			awayID = &v
		}

		rrRows, err := s.db.QueryContext(ctx, `
			SELECT rr.round_number, rr.home_player_id, rr.away_player_id,
			       rr.game1_home, rr.game1_away,
			       rr.game2_home, rr.game2_away,
			       rr.game3_home, rr.game3_away,
			       COALESCE(rr.home_handicap_used, hp.handicap, 0) AS home_hc,
			       COALESCE(rr.away_handicap_used, ap.handicap, 0) AS away_hc
			FROM round_results rr
			LEFT JOIN players hp ON hp.id = rr.home_player_id
			LEFT JOIN players ap ON ap.id = rr.away_player_id
			WHERE rr.match_id = ?
			ORDER BY rr.round_number, rr.home_player_id`, rm.id)
		if err != nil {
			return matches.WeekValidationData{}, fmt.Errorf("get week validation data: rounds for match %d: %w", rm.id, err)
		}

		var rounds []matches.RoundValidationRow
		var rowErr error
		for rrRows.Next() {
			var row matches.RoundValidationRow
			if rowErr = rrRows.Scan(
				&row.RoundNumber, &row.HomePlayerID, &row.AwayPlayerID,
				&row.Game1Home, &row.Game1Away,
				&row.Game2Home, &row.Game2Away,
				&row.Game3Home, &row.Game3Away,
				&row.HomeHC, &row.AwayHC,
			); rowErr != nil {
				break
			}
			rounds = append(rounds, row)
		}
		rrRows.Close()
		if rowErr == nil {
			rowErr = rrRows.Err()
		}
		if rowErr != nil {
			return matches.WeekValidationData{}, fmt.Errorf("get week validation data: round scan for match %d: %w", rm.id, rowErr)
		}

		result.Matches = append(result.Matches, matches.MatchValidationRow{
			MatchID:    rm.id,
			HomeTeamID: homeID,
			AwayTeamID: awayID,
			Rounds:     rounds,
		})
	}

	return result, nil
}

// GetWeekRecapData returns match summaries and week status for the week-end recap.
// Team names prefer season_teams.season_name; falls back to teams.name for legacy seasons.
// HasResult is true when the match has completed=1.
func (s *WeekStore) GetWeekRecapData(ctx context.Context, seasonID, weekNum int64) (matches.WeekRecapData, error) {
	var status string
	var closedAt *string
	switch err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(status, ?), closed_at FROM league_weeks
		WHERE season_id=? AND week_number=?`,
		matches.WeekStatusOpen, seasonID, weekNum).Scan(&status, &closedAt); err {
	case nil:
	case sql.ErrNoRows:
		status = matches.WeekStatusOpen
	default:
		return matches.WeekRecapData{}, fmt.Errorf("get week recap data: week status: %w", err)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT
		    m.id,
		    m.home_team_id,
		    COALESCE(hst.season_name, ht.name, '') AS home_team_name,
		    m.away_team_id,
		    COALESCE(ast.season_name, awt.name, '') AS away_team_name,
		    strftime('%Y-%m-%d', m.match_date) AS match_date,
		    m.completed,
		    COALESCE(SUM(CASE WHEN mr.team_id = m.home_team_id THEN mr.sets_won ELSE 0 END), 0) AS home_sets_won,
		    COALESCE(SUM(CASE WHEN mr.team_id = m.away_team_id THEN mr.sets_won ELSE 0 END), 0) AS away_sets_won,
		    COALESCE(SUM(CASE WHEN mr.team_id = m.home_team_id THEN mr.games_won ELSE 0 END), 0) AS home_games_won,
		    COALESCE(SUM(CASE WHEN mr.team_id = m.away_team_id THEN mr.games_won ELSE 0 END), 0) AS away_games_won
		FROM matches m
		LEFT JOIN season_teams hst ON hst.season_id = m.season_id AND hst.team_id = m.home_team_id
		LEFT JOIN teams ht ON ht.id = m.home_team_id
		LEFT JOIN season_teams ast ON ast.season_id = m.season_id AND ast.team_id = m.away_team_id
		LEFT JOIN teams awt ON awt.id = m.away_team_id
		LEFT JOIN match_results mr ON mr.match_id = m.id
		WHERE m.season_id = ? AND m.week_number = ?
		GROUP BY m.id
		ORDER BY m.id`, seasonID, weekNum)
	if err != nil {
		return matches.WeekRecapData{}, fmt.Errorf("get week recap data: matches: %w", err)
	}
	defer rows.Close()

	matchRows := []models.RecapMatchRow{}
	for rows.Next() {
		var (
			mid                                    int64
			homeID                                 sql.NullInt64
			homeName                               string
			awayID                                 sql.NullInt64
			awayName                               string
			matchDate                              sql.NullString
			completed                              int
			homeSets, awaySets, homeGames, awayGames int
		)
		if err := rows.Scan(
			&mid, &homeID, &homeName, &awayID, &awayName,
			&matchDate, &completed,
			&homeSets, &awaySets, &homeGames, &awayGames,
		); err != nil {
			return matches.WeekRecapData{}, fmt.Errorf("get week recap data: scan: %w", err)
		}
		row := models.RecapMatchRow{
			MatchID:      mid,
			HomeTeamName: homeName,
			AwayTeamName: awayName,
			HasResult:    completed == 1,
			HomeSetsWon:  homeSets,
			AwaySetsWon:  awaySets,
			HomeGamesWon: homeGames,
			AwayGamesWon: awayGames,
		}
		if homeID.Valid {
			v := homeID.Int64
			row.HomeTeamID = &v
		}
		if awayID.Valid {
			v := awayID.Int64
			row.AwayTeamID = &v
		}
		if matchDate.Valid && matchDate.String != "" {
			dateStr := matchDate.String
			row.MatchDate = &dateStr
		}
		matchRows = append(matchRows, row)
	}
	if err := rows.Err(); err != nil {
		return matches.WeekRecapData{}, fmt.Errorf("get week recap data: rows: %w", err)
	}

	return matches.WeekRecapData{
		Status:   status,
		ClosedAt: closedAt,
		Matches:  matchRows,
	}, nil
}

// IsSeasonDraft reports whether the season is in draft state
// (active=0 AND activated_at IS NULL).
func (s *WeekStore) IsSeasonDraft(ctx context.Context, seasonID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM seasons WHERE id=? AND COALESCE(active,0)=0 AND activated_at IS NULL`, seasonID,
	).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("is season draft %d: %w", seasonID, err)
	}
	return n > 0, nil
}

// ListAcknowledgments returns all close acknowledgments for the week, ordered
// by acknowledged_at DESC.
func (s *WeekStore) ListAcknowledgments(ctx context.Context, seasonID, weekNum int64) ([]models.CloseAck, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, season_id, week_number, match_id, warning_code, field, notes, acknowledged_at
		FROM week_close_acknowledgments
		WHERE season_id=? AND week_number=?
		ORDER BY acknowledged_at DESC`, seasonID, weekNum)
	if err != nil {
		return nil, fmt.Errorf("list acknowledgments: %w", err)
	}
	defer rows.Close()

	var acks []models.CloseAck
	for rows.Next() {
		var a models.CloseAck
		if err := rows.Scan(&a.ID, &a.SeasonID, &a.WeekNumber, &a.MatchID,
			&a.WarningCode, &a.Field, &a.Notes, &a.AcknowledgedAt); err != nil {
			return nil, fmt.Errorf("list acknowledgments: scan: %w", err)
		}
		acks = append(acks, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list acknowledgments: rows: %w", err)
	}
	return acks, nil
}
