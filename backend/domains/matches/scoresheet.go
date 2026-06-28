// Package matches owns match participation and scoresheet validation.
package matches

import (
	"fmt"
	"league_app/backend/validation"
	"league_app/logic"
	"league_app/models"
)

// Stable validation codes for 8-ball scoresheet round data.
// Codes are machine-readable API contracts; display text may be updated without breaking callers.
const (
	// CodeNoScores fires (warning) when no game on the submitted sheet has a winner.
	CodeNoScores = "SCORESHEET_NO_SCORES"

	// CodeBothWinners fires (error) when both home and away score 10 in the same game.
	CodeBothWinners = "SCORESHEET_GAME_BOTH_WINNERS"

	// CodeScoreRange fires (error) when a game score falls outside the valid 0-10 range.
	CodeScoreRange = "SCORESHEET_GAME_SCORE_RANGE"

	// CodeLoserRange fires (error) when the loser's score exceeds 7 in a completed game.
	CodeLoserRange = "SCORESHEET_LOSER_SCORE_RANGE"

	// CodeGameIncomplete fires (warning) when a game has non-zero scores but no winner.
	// The normal scoresheet UI prevents this; it signals API-level data problems.
	CodeGameIncomplete = "SCORESHEET_GAME_INCOMPLETE"

	// CodePairingUndetermined is reserved for Close Week finalization.
	CodePairingUndetermined = "SCORESHEET_PAIRING_UNDETERMINED"

	// CodeRoundIncomplete is reserved for Close Week finalization.
	CodeRoundIncomplete = "SCORESHEET_ROUND_INCOMPLETE"

	// CodePairingHCLengthMismatch fires when pairingHC length does not match rounds length.
	// This is a caller error; no pairing scores are computed.
	CodePairingHCLengthMismatch = "SCORESHEET_PAIRING_HC_LENGTH_MISMATCH"
)

// RoundConfig holds per-season scoring configuration for a validation pass.
type RoundConfig struct {
	// Multiplier is the season handicap multiplier; defaults to logic.Multiplier (2.55).
	Multiplier float64
	// MinBallHC is the minimum-ball threshold; 0 = disabled.
	// A computed spot below this threshold is suppressed to 0 (threshold semantics, not floor).
	MinBallHC int
}

// PairingHC holds the effective handicap for one pairing, indexed in parallel with
// the rounds slice passed to ValidateRounds. Each pairing is validated using exactly
// its own stored (or resolved) handicap values, not a player-level lookup that would
// overwrite when the same player appears in multiple pairings.
type PairingHC struct {
	HomeHC float64
	AwayHC float64
}

// ScoresheetResult extends validation.Result with per-pairing and per-round outcome tracking.
// The embedded Result carries all errors and warnings.
// PairingWinners and RoundWinners are informational -- they are not currently returned to callers
// in the 422 body; they are available for Close Week finalization.
type ScoresheetResult struct {
	validation.Result

	// PairingWinners is indexed by submission slot (0-based).
	// Each entry is "home", "away", or "" (undecided or no scores).
	PairingWinners []string `json:"pairing_winners,omitempty"`

	// RoundWinners maps round number (1-based) to "home", "away", or "".
	// A round is won by the first team to claim 2 pairing wins in that round.
	RoundWinners map[int]string `json:"round_winners,omitempty"`
}

// ValidateRounds validates a submitted set of 8-ball round results for a match.
// pairingHC is indexed in parallel with rounds: pairingHC[i] holds the effective
// handicaps for rounds[i]. Each pairing is validated with exactly the handicaps
// that will be stored, not a player-level map that would silently overwrite when
// the same player appears in multiple pairings.
//
// Errors block save (caller returns HTTP 422).
// Warnings do not block save; they are returned for future Close Week use.
func ValidateRounds(rounds []models.RoundResult, pairingHC []PairingHC, cfg RoundConfig) ScoresheetResult {
	var res ScoresheetResult
	res.PairingWinners = make([]string, len(rounds))
	res.RoundWinners = map[int]string{}

	// Length mismatch is a caller error: no pairing can be validated safely.
	if len(pairingHC) != len(rounds) {
		res.AddError(CodePairingHCLengthMismatch, "",
			fmt.Sprintf("pairingHC length %d does not match rounds length %d", len(pairingHC), len(rounds)))
		return res
	}

	// Sheet-level: warn when no game has a winner across the entire submission.
	anyScore := false
	for _, rr := range rounds {
		for _, s := range []int{rr.Game1Home, rr.Game1Away, rr.Game2Home, rr.Game2Away, rr.Game3Home, rr.Game3Away} {
			if s == 10 {
				anyScore = true
				break
			}
		}
		if anyScore {
			break
		}
	}
	if !anyScore {
		res.AddWarning(CodeNoScores, "", "no game scores have been entered")
	}

	roundHomeWins := map[int]int{}
	roundAwayWins := map[int]int{}
	roundHasDecided := map[int]bool{}

	for i, rr := range rounds {
		pairingField := fmt.Sprintf("rounds[%d]", i)
		games := [3][2]int{
			{rr.Game1Home, rr.Game1Away},
			{rr.Game2Home, rr.Game2Away},
			{rr.Game3Home, rr.Game3Away},
		}

		var rawH, rawA, gamesPlayed int
		gameWinners := [3]string{}

		for gn, pair := range games {
			h, a := pair[0], pair[1]
			gameField := fmt.Sprintf("%s.game%d", pairingField, gn+1)

			// Score range: 0-10
			if h < 0 || h > 10 {
				res.AddError(CodeScoreRange, gameField+"_home",
					fmt.Sprintf("R%d slot %d G%d: home score %d out of range 0-10",
						rr.RoundNumber, i%3+1, gn+1, h))
			}
			if a < 0 || a > 10 {
				res.AddError(CodeScoreRange, gameField+"_away",
					fmt.Sprintf("R%d slot %d G%d: away score %d out of range 0-10",
						rr.RoundNumber, i%3+1, gn+1, a))
			}

			// Impossible: both score 10
			if h == 10 && a == 10 {
				res.AddError(CodeBothWinners, gameField,
					fmt.Sprintf("R%d slot %d G%d: both players cannot score 10",
						rr.RoundNumber, i%3+1, gn+1))
				continue
			}

			// Loser score must be 0-7 when a winner exists
			if h == 10 && a > 7 {
				res.AddError(CodeLoserRange, gameField+"_away",
					fmt.Sprintf("R%d slot %d G%d: loser (away) score %d exceeds maximum of 7",
						rr.RoundNumber, i%3+1, gn+1, a))
			}
			if a == 10 && h > 7 {
				res.AddError(CodeLoserRange, gameField+"_home",
					fmt.Sprintf("R%d slot %d G%d: loser (home) score %d exceeds maximum of 7",
						rr.RoundNumber, i%3+1, gn+1, h))
			}

			// Incomplete game: non-zero scores but no declared winner (impossible from normal UI)
			if h != 10 && a != 10 && (h > 0 || a > 0) {
				res.AddWarning(CodeGameIncomplete, gameField,
					fmt.Sprintf("R%d slot %d G%d: non-zero scores with no declared winner",
						rr.RoundNumber, i%3+1, gn+1))
			}

			// Accumulate only games where one side scored 10 (a played, decided game)
			if h == 10 || a == 10 {
				rawH += h
				rawA += a
				gamesPlayed++
				if h == 10 {
					gameWinners[gn] = "home"
				} else {
					gameWinners[gn] = "away"
				}
			}
		}

		// Handicap spot for this pairing (indexed by submission slot, not player ID).
		hHC := pairingHC[i].HomeHC
		aHC := pairingHC[i].AwayHC
		spot := logic.CalcSpotM(hHC, aHC, cfg.Multiplier)

		spotPts := spot.Pts
		if cfg.MinBallHC > 0 && spotPts < cfg.MinBallHC {
			spotPts = 0
		}

		adjH := rawH
		adjA := rawA
		if spot.To == "home" {
			adjH += spotPts
		} else if spot.To == "away" {
			adjA += spotPts
		}

		// Pairing winner: early-stop or full-completion with games-won tiebreak.
		// Handicap alone never determines a winner -- hasScore guard is required.
		remaining := 3 - gamesPlayed
		hasScore := gamesPlayed > 0

		winner := ""
		if hasScore {
			switch {
			case adjH > adjA+remaining*10:
				winner = "home"
			case adjA > adjH+remaining*10:
				winner = "away"
			case remaining == 0:
				hGames, aGames := 0, 0
				for _, gw := range gameWinners {
					if gw == "home" {
						hGames++
					} else if gw == "away" {
						aGames++
					}
				}
				if hGames > aGames {
					winner = "home"
				} else if aGames > hGames {
					winner = "away"
				}
			}
		}

		res.PairingWinners[i] = winner

		if winner != "" {
			roundHasDecided[rr.RoundNumber] = true
			if winner == "home" {
				roundHomeWins[rr.RoundNumber]++
			} else {
				roundAwayWins[rr.RoundNumber]++
			}
		}
	}

	// Determine round winners.
	// CodeRoundIncomplete is reserved for Close Week and is not emitted here.
	for rn := range roundHasDecided {
		switch {
		case roundHomeWins[rn] >= 2:
			res.RoundWinners[rn] = "home"
		case roundAwayWins[rn] >= 2:
			res.RoundWinners[rn] = "away"
		}
	}

	return res
}
