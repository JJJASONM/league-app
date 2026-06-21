package matches

import (
	"league_app/logic"
	"league_app/models"
	"testing"
)

var defaultCfg = RoundConfig{Multiplier: logic.Multiplier}

func findCode(codes []string, target string) bool {
	for _, c := range codes {
		if c == target {
			return true
		}
	}
	return false
}

func errorCodes(res ScoresheetResult) []string {
	var out []string
	for _, m := range res.Errors() {
		out = append(out, m.Code)
	}
	return out
}

func warnCodes(res ScoresheetResult) []string {
	var out []string
	for _, m := range res.Warnings() {
		out = append(out, m.Code)
	}
	return out
}

// 1. Valid completed pairing -- home wins 2 games to 1, all scores legal.
func TestValidCompletedPairing(t *testing.T) {
	rounds := []models.RoundResult{{
		RoundNumber:  1,
		HomePlayerID: 1, AwayPlayerID: 2,
		Game1Home: 10, Game1Away: 5,
		Game2Home: 4, Game2Away: 10,
		Game3Home: 10, Game3Away: 3,
	}}
	res := ValidateRounds(rounds, map[int64]float64{1: 0, 2: 0}, defaultCfg)
	if res.HasErrors() {
		t.Errorf("unexpected errors: %v", errorCodes(res))
	}
	if len(res.Warnings()) != 0 {
		t.Errorf("unexpected warnings: %v", warnCodes(res))
	}
	if res.PairingWinners[0] != "home" {
		t.Errorf("expected pairing winner = home, got %q", res.PairingWinners[0])
	}
}

// 2. Valid mathematical early-stop -- home leads 20-0 after 2 games; opponent cannot catch up.
func TestValidEarlyStop(t *testing.T) {
	// adjH=20, adjA=0, remaining=1: 20 > 0+10 -> home wins early
	rounds := []models.RoundResult{{
		RoundNumber:  1,
		HomePlayerID: 1, AwayPlayerID: 2,
		Game1Home: 10, Game1Away: 0,
		Game2Home: 10, Game2Away: 0,
		// Game3 not entered
	}}
	res := ValidateRounds(rounds, map[int64]float64{1: 0, 2: 0}, defaultCfg)
	if res.HasErrors() {
		t.Errorf("unexpected errors: %v", errorCodes(res))
	}
	if res.PairingWinners[0] != "home" {
		t.Errorf("expected early-stop winner = home, got %q", res.PairingWinners[0])
	}
}

// 3. No-score sheet -- blank submission produces SCORESHEET_NO_SCORES warning, no errors.
func TestNoScoresWarning(t *testing.T) {
	rounds := []models.RoundResult{{
		RoundNumber:  1,
		HomePlayerID: 1, AwayPlayerID: 2,
		// all zeros
	}}
	res := ValidateRounds(rounds, map[int64]float64{1: 0, 2: 0}, defaultCfg)
	if res.HasErrors() {
		t.Errorf("no-score sheet should not produce errors: %v", errorCodes(res))
	}
	if !findCode(warnCodes(res), CodeNoScores) {
		t.Errorf("expected %s warning, got warnings: %v", CodeNoScores, warnCodes(res))
	}
}

// 4. Incomplete game -- non-zero scores with no winner produces SCORESHEET_GAME_INCOMPLETE warning.
func TestGameIncompleteWarning(t *testing.T) {
	rounds := []models.RoundResult{{
		RoundNumber:  1,
		HomePlayerID: 1, AwayPlayerID: 2,
		Game1Home: 5, Game1Away: 3, // non-zero but no 10
	}}
	res := ValidateRounds(rounds, map[int64]float64{1: 0, 2: 0}, defaultCfg)
	if res.HasErrors() {
		t.Errorf("incomplete game should be a warning, not an error: %v", errorCodes(res))
	}
	if !findCode(warnCodes(res), CodeGameIncomplete) {
		t.Errorf("expected %s warning, got warnings: %v", CodeGameIncomplete, warnCodes(res))
	}
}

// 5. Loser score above 7 -- produces SCORESHEET_LOSER_SCORE_RANGE error.
func TestLoserScoreAbove7(t *testing.T) {
	rounds := []models.RoundResult{{
		RoundNumber:  1,
		HomePlayerID: 1, AwayPlayerID: 2,
		Game1Home: 10, Game1Away: 8, // away loser score > 7
	}}
	res := ValidateRounds(rounds, map[int64]float64{1: 0, 2: 0}, defaultCfg)
	if !res.HasErrors() {
		t.Error("expected error for loser score > 7")
	}
	if !findCode(errorCodes(res), CodeLoserRange) {
		t.Errorf("expected %s error, got errors: %v", CodeLoserRange, errorCodes(res))
	}
}

// 6. Both sides score 10 in the same game -- produces SCORESHEET_GAME_BOTH_WINNERS error.
func TestBothWinnersError(t *testing.T) {
	rounds := []models.RoundResult{{
		RoundNumber:  1,
		HomePlayerID: 1, AwayPlayerID: 2,
		Game1Home: 10, Game1Away: 10,
	}}
	res := ValidateRounds(rounds, map[int64]float64{1: 0, 2: 0}, defaultCfg)
	if !res.HasErrors() {
		t.Error("expected error when both players score 10")
	}
	if !findCode(errorCodes(res), CodeBothWinners) {
		t.Errorf("expected %s error, got errors: %v", CodeBothWinners, errorCodes(res))
	}
}

// 7. Score outside 0-10 -- produces SCORESHEET_GAME_SCORE_RANGE error.
func TestScoreOutOfRange(t *testing.T) {
	rounds := []models.RoundResult{{
		RoundNumber:  1,
		HomePlayerID: 1, AwayPlayerID: 2,
		Game1Home: 11, Game1Away: 5, // 11 exceeds maximum
	}}
	res := ValidateRounds(rounds, map[int64]float64{1: 0, 2: 0}, defaultCfg)
	if !res.HasErrors() {
		t.Error("expected error for score > 10")
	}
	if !findCode(errorCodes(res), CodeScoreRange) {
		t.Errorf("expected %s error, got errors: %v", CodeScoreRange, errorCodes(res))
	}
}

// 8. Adjusted-score tie broken by games won -- home wins more games when adjusted totals are equal.
//
// Setup: homeHC=1.0, awayHC=0.0 -> CalcSpotM gives 3 pts to away (lower-rated).
//
//	G1: H wins 10-7  G2: A wins 7-10  G3: H wins 10-7
//	rawH=27  rawA=24  adjH=27  adjA=24+3=27  -> tie; home wins 2 games to 1.
func TestAdjScoreTieGamesWonTiebreak(t *testing.T) {
	rounds := []models.RoundResult{{
		RoundNumber:  1,
		HomePlayerID: 1, AwayPlayerID: 2,
		Game1Home: 10, Game1Away: 7,
		Game2Home: 7, Game2Away: 10,
		Game3Home: 10, Game3Away: 7,
	}}
	playerHC := map[int64]float64{1: 1.0, 2: 0.0}
	res := ValidateRounds(rounds, playerHC, defaultCfg)
	if res.HasErrors() {
		t.Errorf("unexpected errors: %v", errorCodes(res))
	}
	if res.PairingWinners[0] != "home" {
		t.Errorf("expected home to win tiebreak (2 games to 1), got %q", res.PairingWinners[0])
	}
}

// 9. Handicap alone cannot create a winner -- large HC diff with no games played = no winner.
func TestHandicapOnlyNoWinner(t *testing.T) {
	rounds := []models.RoundResult{{
		RoundNumber:  1,
		HomePlayerID: 1, AwayPlayerID: 2,
		// all zeros -- no games played
	}}
	playerHC := map[int64]float64{1: 0.0, 2: 5.0}
	res := ValidateRounds(rounds, playerHC, defaultCfg)
	if res.HasErrors() {
		t.Errorf("no-game submission should not be an error: %v", errorCodes(res))
	}
	if res.PairingWinners[0] != "" {
		t.Errorf("handicap alone must not determine a winner, got %q", res.PairingWinners[0])
	}
}

// 10. Round win after 2 mathematically determined pairings.
//
// Round 1 pairings 0 and 1 are home early-stop wins (20-0 after 2 games).
// Pairing 2 and rounds 2-3 are blank. Round 1 winner must be "home".
func TestRoundWinAfterTwoPairings(t *testing.T) {
	early := models.RoundResult{
		HomePlayerID: 1, AwayPlayerID: 2,
		Game1Home: 10, Game1Away: 0,
		Game2Home: 10, Game2Away: 0,
		// adjH=20 > adjA(0)+remaining(1)*10 -> home wins early
	}
	blank := models.RoundResult{HomePlayerID: 1, AwayPlayerID: 2}

	var rounds []models.RoundResult
	for r := 1; r <= 3; r++ {
		for p := 0; p < 3; p++ {
			rr := blank
			rr.RoundNumber = r
			if r == 1 && p < 2 {
				rr = early
				rr.RoundNumber = r
			}
			rounds = append(rounds, rr)
		}
	}

	res := ValidateRounds(rounds, map[int64]float64{1: 0.0, 2: 0.0}, defaultCfg)

	if res.HasErrors() {
		t.Errorf("unexpected errors: %v", errorCodes(res))
	}
	if res.RoundWinners[1] != "home" {
		t.Errorf("expected round 1 winner = home, got %q", res.RoundWinners[1])
	}
	if res.PairingWinners[0] != "home" {
		t.Errorf("pairing 0: expected home, got %q", res.PairingWinners[0])
	}
	if res.PairingWinners[1] != "home" {
		t.Errorf("pairing 1: expected home, got %q", res.PairingWinners[1])
	}
	if res.PairingWinners[2] != "" {
		t.Errorf("pairing 2: expected undecided, got %q", res.PairingWinners[2])
	}
}
