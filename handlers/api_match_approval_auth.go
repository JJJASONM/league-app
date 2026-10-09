package handlers

import (
	"context"
	"net/http"

	"league_app/backend/domains/auth"
	"league_app/backend/domains/matches"
)

// This file collects the Player Score Approval Phase 1B authorization gates
// that do not fit the existing requireAction/scopeFn shape used everywhere
// else in this package. Every other guarded route in this codebase decides
// authorization from identity + a statically resolvable Scope (a league id,
// a player id) with no further database access inside auth.Authorize
// itself. These routes are different: whether an eligible linked player may
// act depends on a per-match, per-side domain fact
// (matches.RoundStore.ResolveApprovalEligibility) that requires knowing the
// CALLER's identity DURING scope resolution -- something the existing
// scopeFn signature (func(r *http.Request) (auth.Scope, bool, error), with
// no identity parameter) cannot express without changing every other
// scopeFn in the codebase. Rather than force that change through, these
// gates resolve identity once (mirroring requireAction's own precedence/
// CSRF handling exactly) and layer the eligibility check on top directly.

// scoreWriteModeKey is the context key recording whether a score-write
// mutation (SaveRounds/SubmitResults/ClearResults) was authorized via the
// existing admin path or via an eligible linked player's password session.
// Handlers read it (scoreWriteAsPlayer) to decide whether expected_revision
// is mandatory (player) or remains optional for backward compatibility
// (admin) -- PM rule: "Existing admin score-entry clients may retain their
// current optional-revision compatibility."
type scoreWriteModeKey struct{}

func scoreWriteAsPlayer(ctx context.Context) bool {
	v, _ := ctx.Value(scoreWriteModeKey{}).(bool)
	return v
}

// requireAdminOrEligiblePlayerScoreWrite gates a scoresheet-mutation route
// (POST/DELETE .../results, POST .../rounds) for EITHER the existing admin/
// automation path (auth.ActionMatchScoreMutate, completely unchanged) OR an
// eligible linked player using an active password session for at least one
// side of the match -- PM rule: "Allow an eligible linked player with a
// password session to use the applicable score-entry mutation routes...
// API-key authentication cannot use the player permission." Exactly one
// path is ever taken per request. Falls back to the existing
// guardedLeagueAdminAction behavior unchanged when the auth subsystem is
// not (fully) wired, matching every other route's historical test
// behavior -- there is no session/eligibility concept to add in that case.
func requireAdminOrEligiblePlayerScoreWrite(deps Dependencies, matchMgr MatchManager, seasonMgr SeasonManager, roundMgr RoundManager, next http.HandlerFunc) http.HandlerFunc {
	adminScope := matchIDPathScope(matchMgr, seasonMgr)

	if deps.ApplyAuth == nil && deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
		return next
	}
	if deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
		return legacyFlatRoleAuth(deps.ApplyAuth, legacyLeagueAdminRoles, next)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		identity, sess, err := resolveIdentity(r, deps)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if identity == nil {
			if credentialPresented(r) {
				jsonError(w, "forbidden", http.StatusForbidden)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="league-admin"`)
			jsonError(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if sess != nil && isMutatingMethod(r.Method) {
			if !verifyCSRF(r, sess) {
				jsonError(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		scope, ok, err := adminScope(r)
		if err != nil {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}
		if !ok {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}

		if auth.Authorize(*identity, auth.ActionMatchScoreMutate, scope) {
			ctx := context.WithValue(r.Context(), identityContextKey{}, *identity)
			ctx = context.WithValue(ctx, scoreWriteModeKey{}, false)
			next(w, r.WithContext(ctx))
			return
		}

		if identity.AuthMethod != auth.AuthMethodSession || identity.PlayerID == nil {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		matchID, err := pathID(r, "id")
		if err != nil {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}
		eligible, err := eligibleForEitherSide(r.Context(), roundMgr, matchID, *identity.PlayerID)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !eligible {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), identityContextKey{}, *identity)
		ctx = context.WithValue(ctx, scoreWriteModeKey{}, true)
		next(w, r.WithContext(ctx))
	}
}

// eligibleForEitherSide reports whether playerID is eligible (per
// matches.ResolveApprovalEligibility) for the home side, the away side, or
// both. Used where an action applies to the whole scoresheet rather than
// one specific side -- PM rule 3's "or eligibility for at least one side
// when the action applies to the full scoresheet."
func eligibleForEitherSide(ctx context.Context, roundMgr RoundManager, matchID, playerID int64) (bool, error) {
	home, err := roundMgr.ResolveApprovalEligibility(ctx, matchID, matches.ApprovalSideHome, playerID)
	if err != nil {
		return false, err
	}
	if home {
		return true, nil
	}
	return roundMgr.ResolveApprovalEligibility(ctx, matchID, matches.ApprovalSideAway, playerID)
}

// requireMatchTeamApprovalAction gates Player Score Approval Phase 1B's
// team-side mutation routes (approve-home, approve-away, withdraw-approval,
// request-correction). These are exclusively player-scoped actions -- PM's
// approved design gives an administrator no masquerade path onto them (an
// admin who must force acceptance without both team approvals uses
// AdminAcceptMatch's own override contract on the existing acceptance
// route instead). auth.Authorize's ActionMatchTeamApproval case checks only
// the credential shape (an active password session, a linked player);
// per-match, per-side eligibility, match existence, and state-transition
// legality are all left to the Phase 1A service methods the handlers call,
// which already enforce them authoritatively and report the correct
// domain status (404 not found, 422 ineligible, 409 stale/locked, 400
// invalid side/note) -- see mapTeamActionGuardErr and each service
// method's own fast-fail checks. This is a thin, purpose-built wrapper
// over the existing requireAction rather than guardedLeagueAdminAction,
// since there is no legacy flat-role equivalent for a brand-new,
// player-only action: a Dependencies with no auth subsystem wired at all,
// or only the pre-session ApplyAuth-only shape, correctly gets 401/403 for
// this action (there was never a legitimate "fully open" mode for it).
func requireMatchTeamApprovalAction(deps Dependencies, next http.HandlerFunc) http.HandlerFunc {
	return requireAction(deps, auth.ActionMatchTeamApproval, noScope, next)
}

// requireAdminOrEligiblePlayerRead gates Player Score Approval Phase 1B's
// protected reads (GET .../approval, GET .../approval-events): either a
// league/system administrator scoped to the match's league (via the
// existing auth.ActionMatchApprovalRead policy) or an eligible linked
// player using an active password session for either side of the match --
// PM rule: "Restrict them to: appropriately scoped administrators, or an
// eligible linked player using a password session for that match."
func requireAdminOrEligiblePlayerRead(deps Dependencies, matchMgr MatchManager, seasonMgr SeasonManager, roundMgr RoundManager, next http.HandlerFunc) http.HandlerFunc {
	scopeFn := matchIDPathScope(matchMgr, seasonMgr)

	if deps.ApplyAuth == nil && deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
		return next
	}
	if deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
		return legacyFlatRoleAuth(deps.ApplyAuth, legacyLeagueAdminRoles, next)
	}

	return func(w http.ResponseWriter, r *http.Request) {
		identity, _, err := resolveIdentity(r, deps)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if identity == nil {
			if credentialPresented(r) {
				jsonError(w, "forbidden", http.StatusForbidden)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="league-admin"`)
			jsonError(w, "authentication required", http.StatusUnauthorized)
			return
		}

		scope, ok, err := scopeFn(r)
		if err != nil {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}
		if !ok {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}

		if auth.Authorize(*identity, auth.ActionMatchApprovalRead, scope) {
			ctx := context.WithValue(r.Context(), identityContextKey{}, *identity)
			next(w, r.WithContext(ctx))
			return
		}

		if identity.AuthMethod != auth.AuthMethodSession || identity.PlayerID == nil {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		matchID, err := pathID(r, "id")
		if err != nil {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}
		eligible, err := eligibleForEitherSide(r.Context(), roundMgr, matchID, *identity.PlayerID)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !eligible {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), identityContextKey{}, *identity)
		next(w, r.WithContext(ctx))
	}
}

// canSeeApprovalMetadataInList is the read-authorization matrix's check
// for the PRE-EXISTING, still-unguarded GET /api/matches (list) route,
// which predates Player Score Approval and can return matches spanning
// several leagues in one response. Rather than resolving each returned
// match's own league (and, for a player, per-match eligibility)
// individually -- an unbounded number of extra queries per list request --
// this scopes admin-attested approval metadata (approver/processor user
// id, approval note) visibility to the EFFECTIVE filter the request
// actually used, mirroring MatchStore.ListMatches's own precedence
// (season_id wins when both are supplied; neither means unfiltered) via
// effectiveListLeagueID below -- never authorizing against a filter field
// the query itself ignored.
//
// PM correction: the original version granted ANY authenticated
// league_admin (via IsAnyLeagueAdmin) visibility into every league's
// approval metadata, including leagues that admin does not administer,
// whenever the request carried no filter at all -- an unfiltered request
// has no single effective league, so it is authorized for metadata only
// when the caller is system_admin (global, unconditional); a league_admin
// only sees metadata when a season_id/league_id filter is present AND
// resolves to a league they actually administer. A player's own
// eligibility is never consulted here (even for their own match in the
// list) -- they use GET /api/matches/{id} (exact per-match scoping) or
// the dedicated GET .../approval read instead. This never affects the
// list's schedule fields (teams, date, week, completed), which remain
// public exactly as today.
func canSeeApprovalMetadataInList(r *http.Request, deps Dependencies, seasonMgr SeasonManager, req matches.ListMatchesRequest) bool {
	if deps.ApplyAuth == nil && deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
		return true
	}
	identity, _, err := resolveIdentity(r, deps)
	if err != nil || identity == nil {
		return false
	}
	if identity.IsSystemAdmin() {
		return true
	}
	leagueID, ok := effectiveListLeagueID(r.Context(), seasonMgr, req)
	if !ok {
		return false
	}
	return identity.IsLeagueAdminFor(leagueID)
}

// effectiveListLeagueID mirrors MatchStore.ListMatches's own filter
// precedence exactly (backend/storage/sqlite/match_store.go: season_id
// wins when both are supplied; league_id is used only when season_id is
// absent; neither means unfiltered) so
// canSeeApprovalMetadataInList authorizes against the SAME league the
// query actually filtered on.
func effectiveListLeagueID(ctx context.Context, seasonMgr SeasonManager, req matches.ListMatchesRequest) (int64, bool) {
	if req.SeasonID != 0 {
		season, err := seasonMgr.GetSeason(ctx, req.SeasonID)
		if err != nil {
			return 0, false
		}
		return season.LeagueID, true
	}
	if req.LeagueID != 0 {
		return req.LeagueID, true
	}
	return 0, false
}

// canSeeApprovalMetadataInRecap is the read-authorization matrix's check
// for GET /api/seasons/{id}/weeks/{week}/recap (Player Score Approval
// Phase 1D privacy correction). The route ITSELF stays public -- Schedule,
// Communications, and other existing consumers rely on its non-sensitive
// recap data (teams, dates, set/game counts, approved_at/processed_at/
// week_closed) -- but the newer score_revision/home_approval_status/
// away_approval_status/approval_ready fields are the SAME protected
// approval metadata GET .../approval already restricts, so they must not
// be universally returned just because this route is not. Mirrors
// canSeeApprovalMetadataInList's admin-only rule exactly: system_admin
// unconditional; a league_admin only for the league that owns THIS
// season (resolved directly from the path's season id -- no filter
// ambiguity like the matches-list route has, since a recap is always for
// exactly one season). Deliberately NEVER an eligible-player path: an
// eligible linked player already has a protected PER-MATCH approval read
// (GET .../approval) for matches where they are eligible, and must not
// additionally receive every OTHER match's approval state for the whole
// week through this public, unrelated-player-reachable recap.
func canSeeApprovalMetadataInRecap(r *http.Request, deps Dependencies, seasonMgr SeasonManager, seasonID int64) bool {
	if deps.ApplyAuth == nil && deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
		return true
	}
	identity, _, err := resolveIdentity(r, deps)
	if err != nil || identity == nil {
		return false
	}
	if identity.IsSystemAdmin() {
		return true
	}
	season, err := seasonMgr.GetSeason(r.Context(), seasonID)
	if err != nil {
		return false
	}
	return identity.IsLeagueAdminFor(season.LeagueID)
}

// canSeeMatchDraftDetailPlayerEligible checks the eligible-session-player
// half of canSeeMatchDraftDetail. Split out so the league-admin half above
// can be resolved by callers that already have the season's league id in
// hand (avoiding a redundant season lookup) via
// canSeeMatchDraftDetailWithLeague.
func canSeeMatchDraftDetailPlayerEligible(r *http.Request, identity *auth.Identity, roundMgr RoundManager, matchID int64) bool {
	if identity.AuthMethod != auth.AuthMethodSession || identity.PlayerID == nil || roundMgr == nil {
		return false
	}
	eligible, err := eligibleForEitherSide(r.Context(), roundMgr, matchID, *identity.PlayerID)
	if err != nil {
		return false
	}
	return eligible
}

// canSeeMatchDraftDetailWithLeague is canSeeMatchDraftDetail's variant for
// callers that already know the match's league id (avoiding a redundant
// season lookup) -- used by getMatch/getRounds, which already fetch the
// season for other reasons.
func canSeeMatchDraftDetailWithLeague(r *http.Request, deps Dependencies, roundMgr RoundManager, leagueID, matchID int64) bool {
	if deps.ApplyAuth == nil && deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
		return true
	}
	identity, _, err := resolveIdentity(r, deps)
	if err != nil || identity == nil {
		return false
	}
	if auth.Authorize(*identity, auth.ActionMatchApprovalRead, auth.Scope{LeagueID: &leagueID}) {
		return true
	}
	return canSeeMatchDraftDetailPlayerEligible(r, identity, roundMgr, matchID)
}
