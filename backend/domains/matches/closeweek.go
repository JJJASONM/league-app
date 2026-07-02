package matches

import (
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

// validateWeekData checks all matches in the supplied WeekValidationData for
// Close Week readiness. This is a pure function: all data is pre-fetched by
// WeekStore.GetWeekValidationData; no database connection is required.
func validateWeekData(data WeekValidationData, cfg RoundConfig) validation.Result {
	var res validation.Result

	for _, mi := range data.Matches {
		field := fmt.Sprintf("match_%d", mi.MatchID)

		if mi.HomeTeamID == nil || mi.AwayTeamID == nil {
			matchID := mi.MatchID
			res.AddError(CodeWeekMatchUnassigned, field,
				fmt.Sprintf("match %d: home or away team is not assigned", mi.MatchID))
			res.Messages[len(res.Messages)-1].MatchID = &matchID
			continue
		}

		var rounds []models.RoundResult
		var pairingHC []PairingHC
		for _, row := range mi.Rounds {
			rounds = append(rounds, models.RoundResult{
				RoundNumber:  row.RoundNumber,
				HomePlayerID: row.HomePlayerID,
				AwayPlayerID: row.AwayPlayerID,
				Game1Home:    row.Game1Home,
				Game1Away:    row.Game1Away,
				Game2Home:    row.Game2Home,
				Game2Away:    row.Game2Away,
				Game3Home:    row.Game3Home,
				Game3Away:    row.Game3Away,
			})
			pairingHC = append(pairingHC, PairingHC{HomeHC: row.HomeHC, AwayHC: row.AwayHC})
		}

		playersByRound := map[int]map[int64]struct{}{}
		dupFound := false
		for _, rr := range rounds {
			rn := rr.RoundNumber
			if playersByRound[rn] == nil {
				playersByRound[rn] = map[int64]struct{}{}
			}
			for _, pid := range []int64{rr.HomePlayerID, rr.AwayPlayerID} {
				if _, seen := playersByRound[rn][pid]; seen {
					matchID := mi.MatchID
					res.AddError(CodeWeekPlayerDuplicate, field,
						fmt.Sprintf("match %d: player %d appears more than once in round %d", mi.MatchID, pid, rn))
					res.Messages[len(res.Messages)-1].MatchID = &matchID
					dupFound = true
				}
				playersByRound[rn][pid] = struct{}{}
			}
		}
		if dupFound {
			continue
		}

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
			matchID := mi.MatchID
			res.AddError(CodeWeekMatchNoScores, field,
				fmt.Sprintf("match %d: no game winners in saved scoresheet data", mi.MatchID))
			res.Messages[len(res.Messages)-1].MatchID = &matchID
			continue
		}

		ssRes := ValidateRounds(rounds, pairingHC, cfg)
		before := len(res.Messages)
		res.Messages = append(res.Messages, ssRes.Messages...)
		matchID := mi.MatchID
		for i := before; i < len(res.Messages); i++ {
			res.Messages[i].MatchID = &matchID
		}
	}

	return res
}
