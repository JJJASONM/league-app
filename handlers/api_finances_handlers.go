package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/finances"
	"league_app/models"
)

// financeSeasonRoster returns every rostered player in the season across
// all season teams, by concatenating ListRoster per team from
// ListSeasonTeams. No new SeasonManager method needed -- Financial Phase 1
// composes this in the handler rather than adding a season-wide roster
// method, the same minimal-diff choice Player Overview made for its own
// team resolution.
func financeSeasonRoster(ctx context.Context, seasonMgr SeasonManager, seasonID int64) ([]models.SeasonRosterEntry, error) {
	teams, err := seasonMgr.ListSeasonTeams(ctx, seasonID)
	if err != nil {
		return nil, err
	}
	var all []models.SeasonRosterEntry
	for _, t := range teams {
		roster, err := seasonMgr.ListRoster(ctx, seasonID, t.TeamID)
		if err != nil {
			return nil, err
		}
		all = append(all, roster...)
	}
	return all, nil
}

// getSeasonDues handles GET /api/seasons/{id}/finances/dues. Composes
// every rostered player for the season with their dues_payments history
// (paid = at least one payment row exists) and the season's configured
// dues_amount, if any (from the season_rules "dues_amount" freeform key --
// informational display only, not enforced). Gated by clearanceAuth --
// finance reads are protected, unlike most other domains' open GETs.
func getSeasonDues(w http.ResponseWriter, r *http.Request, financeMgr FinanceManager, seasonMgr SeasonManager, ruleMgr RuleManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid season id", http.StatusBadRequest)
		return
	}
	if _, err := seasonMgr.GetSeason(r.Context(), seasonID); err != nil {
		mapSeasonErr(w, err)
		return
	}

	roster, err := financeSeasonRoster(r.Context(), seasonMgr, seasonID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	payments, err := financeMgr.ListDuesPayments(r.Context(), seasonID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	byPlayer := map[int64][]models.DuesPayment{}
	for _, p := range payments {
		byPlayer[p.PlayerID] = append(byPlayer[p.PlayerID], p)
	}

	duesAmount := ""
	if rules, err := ruleMgr.List(r.Context(), seasonID); err == nil {
		for _, rule := range rules {
			if rule.RuleKey == "dues_amount" {
				duesAmount = rule.RuleValue
				break
			}
		}
	}

	players := make([]models.PlayerDuesRow, 0, len(roster))
	for _, entry := range roster {
		playerPayments := byPlayer[entry.PlayerID]
		if playerPayments == nil {
			playerPayments = []models.DuesPayment{}
		}
		var total float64
		for _, p := range playerPayments {
			total += p.Amount
		}
		players = append(players, models.PlayerDuesRow{
			PlayerID:     entry.PlayerID,
			PlayerName:   entry.PlayerName,
			PlayerNumber: entry.PlayerNumber,
			TeamID:       entry.TeamID,
			TeamName:     entry.TeamName,
			Paid:         len(playerPayments) > 0,
			TotalPaid:    total,
			Payments:     playerPayments,
		})
	}

	jsonOK(w, models.SeasonDuesResponse{
		SeasonID:   seasonID,
		DuesAmount: duesAmount,
		Players:    players,
	})
}

// postDuesPayment handles POST /api/seasons/{id}/finances/dues-payments.
// Requires the player to be rostered for the season (found via
// financeSeasonRoster) -- team_id is denormalized from that roster entry
// onto the stored row. Gated by clearanceAuth.
func postDuesPayment(w http.ResponseWriter, r *http.Request, financeMgr FinanceManager, seasonMgr SeasonManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid season id", http.StatusBadRequest)
		return
	}

	var body struct {
		PlayerID int64   `json:"player_id"`
		Amount   float64 `json:"amount"`
		PaidAt   string  `json:"paid_at"`
		Note     string  `json:"note"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	roster, err := financeSeasonRoster(r.Context(), seasonMgr, seasonID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	var teamID *int64
	found := false
	for _, entry := range roster {
		if entry.PlayerID == body.PlayerID {
			v := entry.TeamID
			teamID = &v
			found = true
			break
		}
	}
	if !found {
		jsonError(w, "player is not rostered for this season", http.StatusNotFound)
		return
	}

	payment, err := financeMgr.RecordDuesPayment(r.Context(), finances.RecordDuesPaymentInput{
		SeasonID:         seasonID,
		PlayerID:         body.PlayerID,
		TeamID:           teamID,
		Amount:           body.Amount,
		PaidAt:           body.PaidAt,
		RecordedByUserID: approvingUserID(r),
		Note:             body.Note,
	})
	if err != nil {
		mapFinanceErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(payment)
}

// getSeasonPayouts handles GET /api/seasons/{id}/finances/payouts. Composes
// every season team with its payout history and current standing (shown
// for reference only). Gated by clearanceAuth.
func getSeasonPayouts(w http.ResponseWriter, r *http.Request, financeMgr FinanceManager, seasonMgr SeasonManager, roundMgr RoundManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid season id", http.StatusBadRequest)
		return
	}
	if _, err := seasonMgr.GetSeason(r.Context(), seasonID); err != nil {
		mapSeasonErr(w, err)
		return
	}

	seasonTeams, err := seasonMgr.ListSeasonTeams(r.Context(), seasonID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}

	payouts, err := financeMgr.ListPayouts(r.Context(), seasonID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	byTeam := map[int64][]models.Payout{}
	for _, p := range payouts {
		byTeam[p.TeamID] = append(byTeam[p.TeamID], p)
	}

	standingByTeam := map[int64]models.Standing{}
	if standings, err := roundMgr.GetStandings(r.Context(), seasonID); err == nil {
		for _, st := range standings {
			standingByTeam[st.TeamID] = st
		}
	}

	rows := make([]models.TeamPayoutRow, 0, len(seasonTeams))
	for _, t := range seasonTeams {
		teamPayouts := byTeam[t.TeamID]
		if teamPayouts == nil {
			teamPayouts = []models.Payout{}
		}
		var total float64
		for _, p := range teamPayouts {
			total += p.Amount
		}
		row := models.TeamPayoutRow{
			TeamID:    t.TeamID,
			TeamName:  t.TeamName,
			TotalPaid: total,
			Payouts:   teamPayouts,
		}
		if st, ok := standingByTeam[t.TeamID]; ok {
			row.Standing = &st
		}
		rows = append(rows, row)
	}

	jsonOK(w, models.SeasonPayoutsResponse{
		SeasonID: seasonID,
		Teams:    rows,
	})
}

// postPayout handles POST /api/seasons/{id}/finances/payouts. Requires
// team_id to be a season team. Amount is always admin-entered -- standings
// are never read here. Gated by clearanceAuth.
func postPayout(w http.ResponseWriter, r *http.Request, financeMgr FinanceManager, seasonMgr SeasonManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid season id", http.StatusBadRequest)
		return
	}

	var body struct {
		TeamID int64   `json:"team_id"`
		Amount float64 `json:"amount"`
		Note   string  `json:"note"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}

	seasonTeams, err := seasonMgr.ListSeasonTeams(r.Context(), seasonID)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	found := false
	for _, t := range seasonTeams {
		if t.TeamID == body.TeamID {
			found = true
			break
		}
	}
	if !found {
		jsonError(w, "team is not part of this season", http.StatusNotFound)
		return
	}

	payout, err := financeMgr.RecordPayout(r.Context(), finances.RecordPayoutInput{
		SeasonID:         seasonID,
		TeamID:           body.TeamID,
		Amount:           body.Amount,
		RecordedByUserID: approvingUserID(r),
		Note:             body.Note,
	})
	if err != nil {
		mapFinanceErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(payout)
}

// mapFinanceErr maps a domainerr.Err from the finances service to an HTTP
// status. Mirrors mapTeamErr/mapPlayerErr's shape for the domains that
// don't also carry a package-level ErrNotFound sentinel.
func mapFinanceErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	if errors.As(err, &de) {
		switch de.Category {
		case domainerr.NotFound:
			jsonError(w, de.Message, http.StatusNotFound)
		case domainerr.InvalidInput:
			jsonError(w, de.Message, http.StatusBadRequest)
		case domainerr.Conflict:
			jsonError(w, de.Message, http.StatusConflict)
		default:
			jsonError(w, de.Message, http.StatusInternalServerError)
		}
		return
	}
	jsonError(w, "internal error", http.StatusInternalServerError)
}
