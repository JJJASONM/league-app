package matches

import (
	"database/sql"
	"fmt"
	"league_app/backend/validation"
	"league_app/models"
)

// Stable validation codes for Close Week checks.
const (
	// CodeWeekMatchNoScores fires when a match has no round_results row with a game winner.
	CodeWeekMatchNoScores = "WEEK_MATCH_NO_SCORES"

	// CodeWeekMatchUnassigned fires when home_team_id or away_team_id is missing.
	CodeWeekMatchUnassigned = "WEEK_MATCH_UNASSIGNED"

	// CodeWeekPlayerDuplicate fires when a player appears more than once in a round within the same match.
	CodeWeekPlayerDuplicate = "WEEK_PLAYER_DUPLICATE"

	// CodeWeekInternalError fires when a database operation fails unexpectedly during validation.
	CodeWeekInternalError = "WEEK_INTERNAL_ERROR"
)

// ValidateWeek checks all matches in season/week for Close Week readiness.
// A match passes when it has at least one round_results row containing a game winner
// (any game score of 10 on either side). cfg carries the season handicap multiplier
// and min_ball_handicap rule.
//
// Errors block close. In Phase 1, warnings do not block close (they are surfaced only).
func ValidateWeek(dbConn *sql.DB, seasonID int64, weekNumber int, cfg RoundConfig) validation.Result {
	var res validation.Result

	type matchInfo struct {
		id         int64
		homeTeamID sql.NullInt64
		awayTeamID sql.NullInt64
	}

	rows, err := dbConn.Query(`
		SELECT id, home_team_id, away_team_id
		FROM matches
		WHERE season_id=? AND week_number=?
		ORDER BY id`, seasonID, weekNumber)
	if err != nil {
		res.AddError(CodeWeekInternalError, "", fmt.Sprintf("query failed: %v", err))
		return res
	}
	defer rows.Close()

	var infos []matchInfo
	for rows.Next() {
		var mi matchInfo
		rows.Scan(&mi.id, &mi.homeTeamID, &mi.awayTeamID)
		infos = append(infos, mi)
	}

	for _, mi := range infos {
		field := fmt.Sprintf("match_%d", mi.id)

		if !mi.homeTeamID.Valid || !mi.awayTeamID.Valid {
			matchID := mi.id
			res.AddError(CodeWeekMatchUnassigned, field,
				fmt.Sprintf("match %d: home or away team is not assigned", mi.id))
			res.Messages[len(res.Messages)-1].MatchID = &matchID
			continue
		}

		// Load all round_results for this match.
		// Use handicap snapshots where available; fall back to current player handicap.
		rrRows, err := dbConn.Query(`
			SELECT round_number, home_player_id, away_player_id,
			       game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
			       home_handicap_used, away_handicap_used
			FROM round_results
			WHERE match_id=?
			ORDER BY round_number, home_player_id`, mi.id)
		if err != nil {
			matchID := mi.id
			res.AddError(CodeWeekInternalError, field,
				fmt.Sprintf("match %d: round results query failed: %v", mi.id, err))
			res.Messages[len(res.Messages)-1].MatchID = &matchID
			continue
		}

		var rounds []models.RoundResult
		var pairingHC []PairingHC

		var rowErr error
		for rrRows.Next() {
			var rr models.RoundResult
			var homeHCUsed, awayHCUsed sql.NullFloat64
			if rowErr = rrRows.Scan(
				&rr.RoundNumber, &rr.HomePlayerID, &rr.AwayPlayerID,
				&rr.Game1Home, &rr.Game1Away,
				&rr.Game2Home, &rr.Game2Away,
				&rr.Game3Home, &rr.Game3Away,
				&homeHCUsed, &awayHCUsed); rowErr != nil {
				break
			}
			var h, a float64
			if homeHCUsed.Valid {
				h = homeHCUsed.Float64
			} else {
				if rowErr = dbConn.QueryRow(`SELECT handicap FROM players WHERE id=?`, rr.HomePlayerID).Scan(&h); rowErr != nil {
					break
				}
			}
			if awayHCUsed.Valid {
				a = awayHCUsed.Float64
			} else {
				if rowErr = dbConn.QueryRow(`SELECT handicap FROM players WHERE id=?`, rr.AwayPlayerID).Scan(&a); rowErr != nil {
					break
				}
			}
			pairingHC = append(pairingHC, PairingHC{HomeHC: h, AwayHC: a})
			rounds = append(rounds, rr)
		}
		rrRows.Close()
		if rowErr == nil {
			rowErr = rrRows.Err()
		}
		if rowErr != nil {
			matchID := mi.id
			res.AddError(CodeWeekInternalError, field,
				fmt.Sprintf("match %d: round data read failed: %v", mi.id, rowErr))
			res.Messages[len(res.Messages)-1].MatchID = &matchID
			continue
		}

		// Detect duplicate player participation: within any round, a player must
		// appear at most once across all home and away slots.
		playersByRound := map[int]map[int64]struct{}{}
		dupFound := false
		for _, rr := range rounds {
			rn := rr.RoundNumber
			if playersByRound[rn] == nil {
				playersByRound[rn] = map[int64]struct{}{}
			}
			for _, pid := range []int64{rr.HomePlayerID, rr.AwayPlayerID} {
				if _, seen := playersByRound[rn][pid]; seen {
					matchID := mi.id
					res.AddError(CodeWeekPlayerDuplicate, field,
						fmt.Sprintf("match %d: player %d appears more than once in round %d", mi.id, pid, rn))
					res.Messages[len(res.Messages)-1].MatchID = &matchID
					dupFound = true
				}
				playersByRound[rn][pid] = struct{}{}
			}
		}
		if dupFound {
			continue
		}

		// Require at least one game winner (score of 10) in the saved round data.
		hasGameWinner := false
		for _, rr := range rounds {
			if rr.Game1Home == 10 || rr.Game1Away == 10 ||
				rr.Game2Home == 10 || rr.Game2Away == 10 ||
				rr.Game3Home == 10 || rr.Game3Away == 10 {
				hasGameWinner = true
				break
			}
		}
		if !hasGameWinner {
			matchID := mi.id
			res.AddError(CodeWeekMatchNoScores, field,
				fmt.Sprintf("match %d: no game winners in saved scoresheet data", mi.id))
			res.Messages[len(res.Messages)-1].MatchID = &matchID
			continue
		}

		ssRes := ValidateRounds(rounds, pairingHC, cfg)
		before := len(res.Messages)
		res.Messages = append(res.Messages, ssRes.Messages...)
		matchID := mi.id
		for i := before; i < len(res.Messages); i++ {
			res.Messages[i].MatchID = &matchID
		}
	}

	return res
}
