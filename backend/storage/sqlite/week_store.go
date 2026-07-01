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
		status := "open"
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
		VALUES (?, ?, 'closed', CURRENT_TIMESTAMP)
		ON CONFLICT(season_id, week_number) DO UPDATE
		SET status='closed', closed_at=CURRENT_TIMESTAMP`,
		seasonID, weekNum); err != nil {
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
		UPDATE league_weeks SET status='open', closed_at=NULL
		WHERE season_id=? AND week_number=?`, seasonID, weekNum); err != nil {
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
