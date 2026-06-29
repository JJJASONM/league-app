package handicaps

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"

	"league_app/backend/domainerr"
	"league_app/models"
)

// Domain error codes owned by the handicaps package.
const (
	CodeSeasonNotFound = "HC_SEASON_NOT_FOUND" // domainerr.NotFound
	CodeInvalidRule    = "HC_INVALID_RULE"      // domainerr.Internal
	CodeDataError      = "HC_DATA_ERROR"         // domainerr.Internal
)

// Service orchestrates handicap recommendation reads. It holds no SQL and
// does not import database/sql.
type Service struct {
	store Store
}

// NewService returns a new Service backed by the given Store.
func NewService(store Store) *Service {
	return &Service{store: store}
}

// Recommendations computes the HandicapReviewResponse for the given season.
//
// Returns a *domainerr.Err with Category=NotFound when the season does not exist.
// Returns a *domainerr.Err with Category=Internal on data access or rule errors.
// The error Message is always safe for HTTP response bodies; Cause (via Unwrap) is not.
func (s *Service) Recommendations(ctx context.Context, seasonID int64) (models.HandicapReviewResponse, error) {
	var out models.HandicapReviewResponse
	txErr := s.store.RunTx(ctx, func(tx Store) error {
		var err error
		out, err = s.compute(ctx, tx, seasonID)
		return err
	})
	if txErr != nil {
		var de *domainerr.Err
		if errors.As(txErr, &de) {
			return models.HandicapReviewResponse{}, txErr
		}
		return models.HandicapReviewResponse{}, domainerr.Wrap(CodeDataError, domainerr.Internal, "internal error", txErr)
	}
	return out, nil
}

// compute performs all reads using the transaction-scoped Store tx.
// It must not access s.store directly.
func (s *Service) compute(ctx context.Context, tx Store, seasonID int64) (models.HandicapReviewResponse, error) {
	exists, err := tx.SeasonExists(ctx, seasonID)
	if err != nil {
		return models.HandicapReviewResponse{}, domainerr.Wrap(CodeDataError, domainerr.Internal, "internal error", err)
	}
	if !exists {
		return models.HandicapReviewResponse{}, domainerr.New(CodeSeasonNotFound, domainerr.NotFound, "season not found")
	}

	rules, err := tx.SeasonHandicapRules(ctx, seasonID)
	if err != nil {
		return models.HandicapReviewResponse{}, domainerr.Wrap(CodeDataError, domainerr.Internal, "internal error", err)
	}

	// Interpret update method; nil or blank defaults to "manual_review".
	method := "manual_review"
	if rules.UpdateMethod != nil && *rules.UpdateMethod != "" {
		method = *rules.UpdateMethod
	}

	emptyResp := func(status, message string, weeksClosed int) models.HandicapReviewResponse {
		return models.HandicapReviewResponse{
			SeasonID:        seasonID,
			Method:          method,
			Status:          status,
			Message:         message,
			WeeksClosed:     weeksClosed,
			Recommendations: []models.HandicapReviewRec{},
		}
	}

	// Unknown method falls through to game_diff_average. This preserves the current
	// handler behavior: the switch has no default case; any unrecognized value reaches
	// the recommendations path. A future correction may add an explicit "unsupported"
	// return for unknown methods.
	switch method {
	case "manual_review":
		return emptyResp("no_auto_apply",
			"No handicap changes are applied automatically. Update player handicaps manually via the Players tab.",
			0), nil
	case "kicker_average_preview":
		return emptyResp("unsupported",
			"kicker_average_preview is not yet implemented. No changes are applied automatically.",
			0), nil
	}

	// game_diff_average path (and any unrecognized method).
	weeksClosed, err := tx.ClosedWeekCount(ctx, seasonID)
	if err != nil {
		return models.HandicapReviewResponse{}, domainerr.Wrap(CodeDataError, domainerr.Internal, "internal error", err)
	}
	if weeksClosed == 0 {
		return emptyResp("no_data",
			"No closed weeks available. Close a week to generate handicap recommendations.",
			0), nil
	}

	window, threshold, err := s.interpretWindowConfig(rules)
	if err != nil {
		return models.HandicapReviewResponse{}, err
	}

	maxHC := s.interpretMaxHC(rules)

	roster, err := tx.SeasonRoster(ctx, seasonID)
	if err != nil {
		return models.HandicapReviewResponse{}, domainerr.Wrap(CodeDataError, domainerr.Internal, "internal error", err)
	}
	if len(roster) == 0 {
		return models.HandicapReviewResponse{
			SeasonID:        seasonID,
			Method:          method,
			Status:          "preview",
			Message:         "No handicap changes recommended. Review complete.",
			WeeksClosed:     weeksClosed,
			Recommendations: []models.HandicapReviewRec{},
		}, nil
	}

	playerIDs := make([]int64, len(roster))
	rosterSet := make(map[int64]bool, len(roster))
	for i, e := range roster {
		playerIDs[i] = e.PlayerID
		rosterSet[e.PlayerID] = true
	}

	racks, err := tx.EligibleRacks(ctx, playerIDs)
	if err != nil {
		return models.HandicapReviewResponse{}, domainerr.Wrap(CodeDataError, domainerr.Internal, "internal error", err)
	}

	recs := s.buildRecs(roster, rosterSet, racks, window, threshold, maxHC)

	changed := 0
	for _, rec := range recs {
		if rec.RecommendedHC != nil && rec.Reason != "no_change" {
			changed++
		}
	}
	var msg string
	switch {
	case changed == 0:
		msg = "No handicap changes recommended. Review complete."
	case changed == 1:
		msg = "1 player has a recommended handicap change (not yet applied)."
	default:
		msg = fmt.Sprintf("%d players have recommended handicap changes (not yet applied).", changed)
	}

	return models.HandicapReviewResponse{
		SeasonID:        seasonID,
		Method:          method,
		Status:          "preview",
		Message:         msg,
		WeeksClosed:     weeksClosed,
		Recommendations: recs,
	}, nil
}

// interpretWindowConfig parses window and threshold from the raw rule row.
// Missing or blank values default to 15. Zero, negative, or non-integer values
// return CodeInvalidRule so the operator can correct them in the Rules tab.
func (s *Service) interpretWindowConfig(rules HandicapRuleRow) (window, threshold int, _ error) {
	const defaultVal = 15
	window = defaultVal
	threshold = defaultVal

	parse := func(ptr *string, key string) (int, error) {
		if ptr == nil || *ptr == "" {
			return defaultVal, nil
		}
		n, err := strconv.Atoi(*ptr)
		if err != nil || n <= 0 {
			return 0, domainerr.New(CodeInvalidRule, domainerr.Internal,
				fmt.Sprintf("rule %s: %q must be a positive integer", key, *ptr))
		}
		return n, nil
	}

	var err error
	if window, err = parse(rules.WindowSize, "handicap_current_game_window"); err != nil {
		return 0, 0, err
	}
	if threshold, err = parse(rules.Threshold, "handicap_min_games_for_recommendation"); err != nil {
		return 0, 0, err
	}
	return window, threshold, nil
}

// interpretMaxHC parses max_individual_handicap. Invalid or absent values silently
// default to 4.5. This matches the current handler behavior (seasonMaxIndividualHC
// also silently defaults on parse failure).
func (s *Service) interpretMaxHC(rules HandicapRuleRow) float64 {
	if rules.MaxHC == nil || *rules.MaxHC == "" {
		return 4.5
	}
	f, err := strconv.ParseFloat(*rules.MaxHC, 64)
	if err != nil || f <= 0 {
		return 4.5
	}
	return f
}

// buildRecs accumulates rack samples and constructs per-player HandicapReviewRec entries.
// Rack slot ordering (game3, game2, game1 within each row) preserves most-recent-first
// ordering established by the EligibleRacks query.
func (s *Service) buildRecs(
	roster []RosterEntry,
	rosterSet map[int64]bool,
	racks []RackRow,
	window, threshold int,
	maxHC float64,
) []models.HandicapReviewRec {
	type rackAccum struct {
		scoreEligible   int
		missingSnapshot int
		samples         []RackSample
	}
	accum := make(map[int64]*rackAccum, len(roster))
	for _, e := range roster {
		accum[e.PlayerID] = &rackAccum{}
	}

	processSlots := func(pid int64, slots [][2]int, opponentHC *float64) {
		acc, ok := accum[pid]
		if !ok {
			return
		}
		for _, s := range slots {
			player, opp := s[0], s[1]
			eligible := (player == 10 && opp >= 0 && opp <= 7) ||
				(opp == 10 && player >= 0 && player <= 7)
			if !eligible {
				continue
			}
			acc.scoreEligible++
			if opponentHC == nil {
				acc.missingSnapshot++
				continue
			}
			acc.samples = append(acc.samples, RackSample{
				OpponentHC: *opponentHC,
				RackDiff:   float64(player - opp),
			})
		}
	}

	for _, rr := range racks {
		// Slots iterated game3, game2, game1 (DESC) within each row to preserve
		// most-recent-first ordering established by the EligibleRacks query.
		homeSlots := [][2]int{{rr.G3H, rr.G3A}, {rr.G2H, rr.G2A}, {rr.G1H, rr.G1A}}
		awaySlots := [][2]int{{rr.G3A, rr.G3H}, {rr.G2A, rr.G2H}, {rr.G1A, rr.G1H}}

		// When the reviewed player is HOME, opponent HC = away_handicap_used; vice versa.
		if rosterSet[rr.HomePlayerID] {
			processSlots(rr.HomePlayerID, homeSlots, rr.AwayHCUsed)
		}
		if rosterSet[rr.AwayPlayerID] {
			processSlots(rr.AwayPlayerID, awaySlots, rr.HomeHCUsed)
		}
	}

	recs := make([]models.HandicapReviewRec, 0, len(roster))
	for _, e := range roster {
		acc := accum[e.PlayerID]
		included := acc.scoreEligible - acc.missingSnapshot
		result := ComputeImpliedHandicap(acc.samples, window)

		rec := models.HandicapReviewRec{
			PlayerID:             e.PlayerID,
			PlayerName:           e.PlayerName,
			TeamName:             e.TeamName,
			AdminHold:            e.AdminHold,
			AssignedHC:           e.AssignedHC,
			ScoreEligibleRacks:   acc.scoreEligible,
			MissingSnapshotRacks: acc.missingSnapshot,
			IncludedRacks:        included,
			WindowSize:           window,
			EligibilityThreshold: threshold,
			LifetimeRacks:        result.LifetimeRacks,
			WindowRacks:          result.WindowRacks,
		}

		if result.LifetimeRacks > 0 {
			lhc := result.LifetimeImplied
			rec.LifetimeHC = &lhc
			whc := result.WindowImplied
			rec.WindowHC = &whc
		}

		switch {
		case included == 0:
			rec.Reason = "no_data"
		case e.AdminHold:
			rec.Reason = "admin_hold"
		case result.WindowRacks < threshold:
			rec.Reason = "below_threshold"
		default:
			recommended := result.WindowImplied
			capped := false
			if recommended > maxHC {
				recommended = math.Round(maxHC*100) / 100
				capped = true
			} else if recommended < -maxHC {
				recommended = math.Round(-maxHC*100) / 100
				capped = true
			}
			rec.RecommendedHC = &recommended
			change := math.Round((recommended-e.AssignedHC)*100) / 100
			rec.ChangeAmount = &change
			switch {
			case capped:
				rec.Reason = "capped"
			case recommended == math.Round(e.AssignedHC*100)/100:
				rec.Reason = "no_change"
			}
		}

		recs = append(recs, rec)
	}
	return recs
}
