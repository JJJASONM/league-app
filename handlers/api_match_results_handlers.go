package handlers

import (
	"errors"
	"net/http"

	"league_app/backend/domainerr"
	"league_app/backend/domains/matches"
	"league_app/models"
)

// submitResults handles POST /api/matches/{id}/results for both the
// existing admin path (expected_revision optional, unchanged compatibility)
// and Player Score Approval Phase 1B's eligible-player path (mandatory
// expected_revision, enforced via requireAdminOrEligiblePlayerScoreWrite's
// scoreWriteModeKey context flag -- see that gate's doc comment for why
// eligibility cannot be expressed as a static auth.Scope). The response
// always includes the resulting score_revision, an additive JSON field the
// existing admin frontend (which does not read it) is unaffected by.
func submitResults(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req models.SubmitResultsRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if scoreWriteAsPlayer(r.Context()) && req.ExpectedRevision == nil {
		jsonError(w, "expected_revision is required", 400)
		return
	}
	rev, err := mgr.SubmitResultsWithRevision(r.Context(), id, req.Results, req.ExpectedRevision)
	if err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	jsonOK(w, map[string]any{"status": "saved", "score_revision": rev})
}

// clearResults handles DELETE /api/matches/{id}/results. The existing admin
// path sends no body (a bodyless DELETE decodes to a zero-value
// ClearResultsRequest, matching its historical unconditional-clear
// behavior); an eligible player's session-authenticated clear must supply
// expected_revision. See submitResults's doc comment for the shared
// admin/player contract reasoning.
func clearResults(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req models.ClearResultsRequest
	_ = decode(r, &req) // optional body -- absent/empty is the historical admin contract
	if scoreWriteAsPlayer(r.Context()) && req.ExpectedRevision == nil {
		jsonError(w, "expected_revision is required", 400)
		return
	}
	rev, err := mgr.ClearResultsWithRevision(r.Context(), id, req.ExpectedRevision)
	if err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	jsonOK(w, map[string]any{"status": "cleared", "score_revision": rev})
}

func getStandings(w http.ResponseWriter, r *http.Request, roundMgr RoundManager, seasonMgr SeasonManager) {
	sid, ok := qparamInt(r, "season_id")
	if !ok {
		leagueID, lok := qparamInt(r, "league_id")
		if !lok {
			jsonOK(w, []models.Standing{})
			return
		}
		var found bool
		var err error
		sid, found, err = seasonMgr.FindActiveSeasonByLeague(r.Context(), leagueID)
		if err != nil || !found {
			jsonOK(w, []models.Standing{})
			return
		}
	}
	standings, err := roundMgr.GetStandings(r.Context(), sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, standings)
}

// --- 8-Ball Round Results ---

// getRounds handles GET /api/matches/{id}/rounds. Full round detail
// (including live/draft game scores) is served unconditionally once the
// match is completed AND its week is closed -- "public final closed
// results," per the read-authorization matrix. Before that point, the
// scoresheet is draft/in-progress content and is not public (Phase 1B
// discovery: this route predates Player Score Approval and was never
// gated at all); an unauthorized caller gets an empty list rather than an
// error, matching this route's historical "always 200" shape and the
// principle that hiding draft content is a visibility decision, not an
// authentication failure, for what remains a schedule-adjacent public
// route. matchMgr/seasonMgr/deps are used only to resolve the match's
// completed/week_closed state and, when needed, its owning league for the
// admin/eligible-player check.
func getRounds(w http.ResponseWriter, r *http.Request, mgr RoundManager, matchMgr MatchManager, seasonMgr SeasonManager, deps Dependencies) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	detail, err := matchMgr.GetMatch(r.Context(), id)
	if err != nil {
		jsonError(w, "not found", http.StatusNotFound)
		return
	}
	if !(detail.Match.Completed && detail.Match.WeekClosed) {
		season, err := seasonMgr.GetSeason(r.Context(), detail.Match.SeasonID)
		leagueID := int64(0)
		if err == nil {
			leagueID = season.LeagueID
		}
		if !canSeeMatchDraftDetailWithLeague(r, deps, mgr, leagueID, id) {
			jsonOK(w, []models.RoundResult{})
			return
		}
	}
	rounds, err := mgr.GetRounds(r.Context(), id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, rounds)
}

// saveRounds handles POST /api/matches/{id}/rounds for both the admin path
// (expected_revision optional) and Player Score Approval Phase 1B's
// eligible-player path (mandatory expected_revision) -- see submitResults's
// doc comment for the shared reasoning. The pre-existing season-closed and
// roster-eligibility fast-fail checks are unchanged; they are
// non-authoritative (the atomic guard inside SaveRoundsWithRevision is the
// real protection) but preserve this route's existing friendly-error
// ordering for both callers.
func saveRounds(w http.ResponseWriter, r *http.Request, roundMgr RoundManager, seasonMgr SeasonManager) {
	matchID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req models.SaveRoundsRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if scoreWriteAsPlayer(r.Context()) && req.ExpectedRevision == nil {
		jsonError(w, "expected_revision is required", 400)
		return
	}
	if sc, _ := roundMgr.IsSeasonClosedForMatch(r.Context(), matchID); sc {
		jsonError(w, "season is closed; this action is not allowed", http.StatusConflict)
		return
	}
	if ok, msg, err := seasonMgr.RosterEligible(r.Context(), matchID, 3); err == nil && !ok {
		jsonError(w, msg, http.StatusUnprocessableEntity)
		return
	}
	rev, err := roundMgr.SaveRoundsWithRevision(r.Context(), matches.SaveRoundsInput{MatchID: matchID, Rounds: req.Rounds, ExpectedRevision: req.ExpectedRevision})
	if err != nil {
		var vErr *matches.RoundValidationError
		if errors.As(err, &vErr) {
			jsonValidation(w, vErr.Result.Result)
			return
		}
		writeMatchApprovalErr(w, err)
		return
	}
	jsonOK(w, map[string]any{"saved": len(req.Rounds), "score_revision": rev})
}

// --- Weekly Score Processing Phase 1A: match approval/processing ---

// approvingUserID returns the resolved clearance user's ID, or nil when no
// user is in context (e.g. clearanceAuth's nil-resolver test compatibility
// path). Admin-attested approval in Phase 1A does not require a user record.
func approvingUserID(r *http.Request) *int64 {
	u := clearanceUserFromContext(r.Context())
	if u == nil {
		return nil
	}
	id := u.ID
	return &id
}

// acceptingUserID resolves the acting administrator's user ID for
// AdminAcceptMatch, preferring the Users/Roles Phase 1 identity
// (requireAction/guardedLeagueAdminAction's context) since that is what
// every real session- or Bearer-key-authenticated request to this route
// carries; approvingUserID/clearanceUserFromContext is checked only as a
// fallback for legacy clearanceAuth-only test fixtures that never wire the
// newer identity context at all.
//
// Phase 1B discovery finding: approvingUserID alone always returns nil for
// this specific route in every real deployment, because
// guardedLeagueAdminAction routes requests through requireAction (which
// stores the identity under a different context key), never through
// requirePersonalKeyAuth (the only thing that sets clearanceUserKey) --
// approved_by_user_id was therefore silently never populated via
// POST /api/matches/{id}/approve. This is fixed here for the rewired
// AdminAcceptMatch path; the sibling process/unapprove/unprocess handlers
// are unchanged by this phase and keep using approvingUserID as before.
func acceptingUserID(r *http.Request) *int64 {
	if identity := identityFromContext(r.Context()); identity != nil {
		id := identity.UserID
		return &id
	}
	return approvingUserID(r)
}

func writeMatchApprovalErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	if errors.As(err, &de) {
		switch de.Category {
		case domainerr.NotFound:
			jsonError(w, de.Message, http.StatusNotFound)
		case domainerr.InvalidInput:
			// Player Score Approval Phase 1B is the first caller to route
			// domainerr.InvalidInput through this mapper (e.g.
			// CodeApprovalInvalidSide, CodeApprovalNoteRequired) -- added
			// here rather than left to the pre-existing default (500),
			// which would have misreported every bad-body/bad-side/missing-
			// note request as an internal error instead of 400.
			jsonError(w, de.Message, http.StatusBadRequest)
		case domainerr.Conflict:
			jsonError(w, de.Message, http.StatusConflict)
		case domainerr.Unprocessable:
			jsonError(w, de.Message, http.StatusUnprocessableEntity)
		default:
			jsonError(w, de.Message, http.StatusInternalServerError)
		}
		return
	}
	jsonError(w, err.Error(), http.StatusInternalServerError)
}

// approveMatch is Player Score Approval Phase 1B's rewiring of
// POST /api/matches/{id}/approve onto AdminAcceptMatch. This is a
// deliberate, PM-approved contract change for this specific route:
// expected_revision is now REQUIRED (AdminAcceptMatch has no
// optional-revision variant, unlike the score-write routes), and override
// must be explicit -- a missing or false override on a match without both
// current-revision team approvals returns 409 CodeApprovalOverrideRequired
// rather than silently proceeding. The existing frontend Approve button
// does not yet send this contract; admin UI integration is a later phase
// (see doc/domains/matches/README.md), and this backend branch should not
// be deployed alone if that would interrupt the current button.
func approveMatch(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req struct {
		Note             string `json:"note"`
		Override         bool   `json:"override"`
		ExpectedRevision *int   `json:"expected_revision"`
	}
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if req.ExpectedRevision == nil {
		jsonError(w, "expected_revision is required", 400)
		return
	}
	if err := mgr.AdminAcceptMatch(r.Context(), id, acceptingUserID(r), req.Note, req.Override, *req.ExpectedRevision); err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "approved"})
}

func processMatch(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.ProcessMatch(r.Context(), id, approvingUserID(r)); err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "processed"})
}

func unapproveMatch(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.UnapproveMatch(r.Context(), id); err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "unapproved"})
}

func unprocessMatch(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.UnprocessMatch(r.Context(), id); err != nil {
		writeMatchApprovalErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "unprocessed"})
}

func getPlayerStats(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	var req matches.PlayerStatsRequest
	if sid, ok := qparamInt(r, "season_id"); ok {
		req.SeasonID = sid
	} else if lid, ok := qparamInt(r, "league_id"); ok {
		req.LeagueID = lid
	} else {
		jsonOK(w, []models.PlayerStat{})
		return
	}
	stats, err := mgr.GetPlayerStats(r.Context(), req)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, stats)
}
