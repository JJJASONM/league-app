package matches

import (
	"context"
	"fmt"

	"league_app/backend/domainerr"
	"league_app/logic"
	"league_app/models"
)

// RoundValidationError is returned by SaveRounds when scoresheet validation fails.
// The handler maps it to HTTP 422 with the structured validation body.
type RoundValidationError struct {
	Result ScoresheetResult
}

func (e *RoundValidationError) Error() string {
	return fmt.Sprintf("round save blocked: %d validation error(s)", len(e.Result.Errors()))
}

// RoundService orchestrates scoresheet save/read, standings, and player stats
// for the matches domain.
type RoundService struct {
	store RoundStore
}

// NewRoundService returns a RoundService backed by the given store.
func NewRoundService(store RoundStore) *RoundService {
	return &RoundService{store: store}
}

// SaveRounds validates and persists one match's round results inside a single transaction.
// Returns *RoundValidationError on scoresheet validation failure (HTTP 422).
// Returns domainerr.Conflict when the week is already closed (HTTP 409).
// Returns domainerr.Unprocessable for ambiguous snapshot resolution (HTTP 422).
func (s *RoundService) SaveRounds(ctx context.Context, input SaveRoundsInput) error {
	closed, err := s.store.IsWeekClosed(ctx, input.MatchID)
	if err != nil {
		return fmt.Errorf("save rounds: week-closed check: %w", err)
	}
	if closed {
		return domainerr.New("WEEK_CLOSED", domainerr.Conflict,
			"week is closed; reopen before editing scores")
	}

	return s.store.RunTx(ctx, func(tx RoundStore) error {
		mc, err := tx.LoadMatchContext(ctx, input.MatchID)
		if err != nil {
			return fmt.Errorf("save rounds: load match context: %w", err)
		}

		cfg, err := tx.SeasonRoundConfig(ctx, mc.SeasonID)
		if err != nil {
			return fmt.Errorf("save rounds: season %d round config: %w", mc.SeasonID, err)
		}

		// Load current handicaps for each unique player referenced in the submission.
		playerIDs := make(map[int64]struct{})
		for _, rr := range input.Rounds {
			playerIDs[rr.HomePlayerID] = struct{}{}
			playerIDs[rr.AwayPlayerID] = struct{}{}
		}
		currentHC := make(map[int64]float64, len(playerIDs))
		for pid := range playerIDs {
			hc, err := tx.LoadPlayerHandicap(ctx, pid)
			if err != nil {
				return fmt.Errorf("save rounds: player %d: %w", pid, err)
			}
			currentHC[pid] = hc
		}

		// Load prior snapshots grouped by round number for preservation logic.
		priors, err := tx.LoadPriorSnapshots(ctx, input.MatchID)
		if err != nil {
			return fmt.Errorf("save rounds: load prior snapshots: %w", err)
		}
		priorByRound := map[int][]PriorSnapshotRow{}
		for _, pr := range priors {
			priorByRound[pr.RoundNumber] = append(priorByRound[pr.RoundNumber], pr)
		}

		// Resolve effective HC per submission slot (snapshot preservation):
		// - same home player as prior → preserve home snapshot; preserve away if also same
		// - new home player → search by away; preserve away snapshot if unique match
		// - no match → use current HC for both
		type effectiveHC struct{ home, away float64 }
		effectiveBySlot := make([]effectiveHC, len(input.Rounds))
		for i, rr := range input.Rounds {
			homeHC := currentHC[rr.HomePlayerID]
			awayHC := currentHC[rr.AwayPlayerID]
			ps := priorByRound[rr.RoundNumber]

			var matchedByHome *PriorSnapshotRow
			for j := range ps {
				if ps[j].HomePlayerID == rr.HomePlayerID {
					matchedByHome = &ps[j]
					break
				}
			}

			if matchedByHome != nil {
				if matchedByHome.HomeHandicapUsed.Valid {
					homeHC = matchedByHome.HomeHandicapUsed.Float64
				}
				if matchedByHome.AwayPlayerID == rr.AwayPlayerID && matchedByHome.AwayHandicapUsed.Valid {
					awayHC = matchedByHome.AwayHandicapUsed.Float64
				}
			} else {
				var matchedByAway *PriorSnapshotRow
				awayCount := 0
				for j := range ps {
					if ps[j].AwayPlayerID == rr.AwayPlayerID {
						matchedByAway = &ps[j]
						awayCount++
					}
				}
				if awayCount > 1 {
					return domainerr.New("ROUND_AMBIGUOUS_SNAPSHOT", domainerr.Unprocessable,
						fmt.Sprintf("round %d: away player %d appears in %d prior pairings; cannot determine which snapshot to preserve",
							rr.RoundNumber, rr.AwayPlayerID, awayCount))
				}
				if awayCount == 1 && matchedByAway.AwayHandicapUsed.Valid {
					awayHC = matchedByAway.AwayHandicapUsed.Float64
				}
			}
			effectiveBySlot[i] = effectiveHC{homeHC, awayHC}
		}

		// Validate rounds with exactly the HCs that will be stored.
		pairingHCSlice := make([]PairingHC, len(input.Rounds))
		for i, e := range effectiveBySlot {
			pairingHCSlice[i] = PairingHC{HomeHC: e.home, AwayHC: e.away}
		}
		vResult := ValidateRounds(input.Rounds, pairingHCSlice, cfg)
		if vResult.HasErrors() {
			return &RoundValidationError{Result: vResult}
		}

		// Delete and re-insert round_results with full HC snapshots.
		if err := tx.DeleteRoundResults(ctx, input.MatchID); err != nil {
			return fmt.Errorf("save rounds: delete round results: %w", err)
		}
		for i, rr := range input.Rounds {
			ehc := effectiveBySlot[i]
			spot := logic.CalcSpotM(ehc.home, ehc.away, cfg.Multiplier)
			row := RoundResultRow{
				MatchID:         input.MatchID,
				RoundNumber:     rr.RoundNumber,
				HomePlayerID:    rr.HomePlayerID,
				AwayPlayerID:    rr.AwayPlayerID,
				Game1Home:       rr.Game1Home,
				Game1Away:       rr.Game1Away,
				Game2Home:       rr.Game2Home,
				Game2Away:       rr.Game2Away,
				Game3Home:       rr.Game3Home,
				Game3Away:       rr.Game3Away,
				HomeHCUsed:      ehc.home,
				AwayHCUsed:      ehc.away,
				HandicapPtsUsed: spot.Pts,
				HandicapTo:      spot.To,
			}
			if err := tx.InsertRoundResult(ctx, row); err != nil {
				return fmt.Errorf("save rounds: insert round result: %w", err)
			}
		}

		// Derive per-player game tallies from round scores.
		type tally struct{ gw, gl, sw, sl int }
		tallies := map[int64]*tally{}
		ensure := func(pid int64) *tally {
			if tallies[pid] == nil {
				tallies[pid] = &tally{}
			}
			return tallies[pid]
		}
		for _, rr := range input.Rounds {
			for _, pair := range [][2]int{
				{rr.Game1Home, rr.Game1Away},
				{rr.Game2Home, rr.Game2Away},
				{rr.Game3Home, rr.Game3Away},
			} {
				h, a := pair[0], pair[1]
				switch {
				case h == 10:
					ensure(rr.HomePlayerID).gw++
					ensure(rr.AwayPlayerID).gl++
				case a == 10:
					ensure(rr.AwayPlayerID).gw++
					ensure(rr.HomePlayerID).gl++
				}
			}
		}

		// Build player→team mapping from the match context.
		pTeam := map[int64]int64{}
		for _, rr := range input.Rounds {
			pTeam[rr.HomePlayerID] = mc.HomeTeamID
			pTeam[rr.AwayPlayerID] = mc.AwayTeamID
		}

		// Compute sets won/lost from round winners.
		for roundNum, winner := range vResult.RoundWinners {
			if winner == "" {
				continue
			}
			for _, rr := range input.Rounds {
				if rr.RoundNumber != roundNum {
					continue
				}
				if winner == "home" {
					ensure(rr.HomePlayerID).sw++
					ensure(rr.AwayPlayerID).sl++
				} else {
					ensure(rr.AwayPlayerID).sw++
					ensure(rr.HomePlayerID).sl++
				}
			}
		}

		// Replace match_results.
		if err := tx.DeleteMatchResults(ctx, input.MatchID); err != nil {
			return fmt.Errorf("save rounds: delete match results: %w", err)
		}
		for pid, t := range tallies {
			row := MatchResultRow{
				MatchID:   input.MatchID,
				PlayerID:  pid,
				TeamID:    pTeam[pid],
				GamesWon:  t.gw,
				GamesLost: t.gl,
				Diff:      float64(t.gw - t.gl),
				SetsWon:   t.sw,
				SetsLost:  t.sl,
			}
			if err := tx.InsertMatchResult(ctx, row); err != nil {
				return fmt.Errorf("save rounds: insert match result: %w", err)
			}
		}

		// Mark match completed when at least one game has been scored.
		anyScored := false
		for _, rr := range input.Rounds {
			for _, s := range []int{rr.Game1Home, rr.Game1Away, rr.Game2Home, rr.Game2Away, rr.Game3Home, rr.Game3Away} {
				if s == 10 {
					anyScored = true
					break
				}
			}
			if anyScored {
				break
			}
		}
		if anyScored {
			if err := tx.MarkMatchCompleted(ctx, input.MatchID); err != nil {
				return fmt.Errorf("save rounds: mark completed: %w", err)
			}
		}
		return nil
	})
}

// GetRounds returns all round results for a match with computed pairing fields.
// Falls back to the default handicap multiplier when the match or season config is unavailable.
func (s *RoundService) GetRounds(ctx context.Context, matchID int64) ([]models.RoundResult, error) {
	cfg := RoundConfig{Multiplier: logic.Multiplier}
	if mc, err := s.store.LoadMatchContext(ctx, matchID); err == nil {
		if cfg2, err2 := s.store.SeasonRoundConfig(ctx, mc.SeasonID); err2 == nil {
			cfg = cfg2
		}
	}
	rows, err := s.store.GetRoundResults(ctx, matchID)
	if err != nil {
		return nil, fmt.Errorf("get rounds: %w", err)
	}
	for i := range rows {
		computePairingResult(&rows[i], cfg.Multiplier)
	}
	if rows == nil {
		rows = []models.RoundResult{}
	}
	return rows, nil
}

// GetStandings returns computed standings for a season.
func (s *RoundService) GetStandings(ctx context.Context, seasonID int64) ([]models.Standing, error) {
	data, err := s.store.GetStandingsData(ctx, seasonID)
	if err != nil {
		return nil, fmt.Errorf("get standings: %w", err)
	}
	standings := logic.ComputeStandings(data.Matches, data.ResultMap, data.Teams)
	if standings == nil {
		standings = []models.Standing{}
	}
	return standings, nil
}

// GetPlayerStats returns aggregated match stats for a season or league scope.
// Returns an empty slice when neither SeasonID nor LeagueID is set.
func (s *RoundService) GetPlayerStats(ctx context.Context, req PlayerStatsRequest) ([]models.PlayerStat, error) {
	if req.SeasonID == 0 && req.LeagueID == 0 {
		return []models.PlayerStat{}, nil
	}
	stats, err := s.store.GetPlayerStats(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get player stats: %w", err)
	}
	for i := range stats {
		total := stats[i].GamesWon + stats[i].GamesLost
		if total > 0 {
			stats[i].WinPct = float64(stats[i].GamesWon) / float64(total)
		}
	}
	if stats == nil {
		stats = []models.PlayerStat{}
	}
	return stats, nil
}

// SubmitResults replaces match_results for a match and marks it completed.
// Returns domainerr.Conflict when the week is already closed.
func (s *RoundService) SubmitResults(ctx context.Context, matchID int64, results []models.MatchResult) error {
	closed, err := s.store.IsWeekClosed(ctx, matchID)
	if err != nil {
		return fmt.Errorf("submit results: week-closed check: %w", err)
	}
	if closed {
		return domainerr.New("WEEK_CLOSED", domainerr.Conflict,
			"week is closed; reopen before editing scores")
	}
	return s.store.SubmitMatchResults(ctx, matchID, results)
}

// ClearResults deletes match_results for a match and marks it incomplete.
// Returns domainerr.Conflict when the week is already closed.
func (s *RoundService) ClearResults(ctx context.Context, matchID int64) error {
	closed, err := s.store.IsWeekClosed(ctx, matchID)
	if err != nil {
		return fmt.Errorf("clear results: week-closed check: %w", err)
	}
	if closed {
		return domainerr.New("WEEK_CLOSED", domainerr.Conflict,
			"week is closed; reopen before editing scores")
	}
	return s.store.ClearMatchResults(ctx, matchID)
}

// computePairingResult fills derived fields on a RoundResult.
// When a HC snapshot is stored on the row, it takes priority over the current player HC
// so historical scoresheets remain stable. multiplier is the season handicap_multiplier.
func computePairingResult(rr *models.RoundResult, multiplier float64) {
	homeRaw := rr.Game1Home + rr.Game2Home + rr.Game3Home
	awayRaw := rr.Game1Away + rr.Game2Away + rr.Game3Away

	if rr.HandicapPtsUsed != nil && rr.HandicapToUsed != nil {
		rr.HandicapPts = *rr.HandicapPtsUsed
		rr.HandicapTo = *rr.HandicapToUsed
	} else {
		homeHC := rr.HomeHandicap
		if rr.HomeHandicapUsed != nil {
			homeHC = *rr.HomeHandicapUsed
		}
		awayHC := rr.AwayHandicap
		if rr.AwayHandicapUsed != nil {
			awayHC = *rr.AwayHandicapUsed
		}
		spot := logic.CalcSpotM(homeHC, awayHC, multiplier)
		rr.HandicapPts = spot.Pts
		rr.HandicapTo = spot.To
	}

	homeAdj := homeRaw
	awayAdj := awayRaw
	switch rr.HandicapTo {
	case "home":
		homeAdj += rr.HandicapPts
	case "away":
		awayAdj += rr.HandicapPts
	}

	rr.HomeTotalPts = homeAdj
	rr.AwayTotalPts = awayAdj

	switch {
	case homeAdj > awayAdj:
		rr.PairingWinner = "home"
	case awayAdj > homeAdj:
		rr.PairingWinner = "away"
	default:
		rr.PairingWinner = ""
	}
}
