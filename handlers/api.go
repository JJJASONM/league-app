// Package handlers wires HTTP routes to domain services.
package handlers

import (
	"context"
	"encoding/json"
	"reflect"

	"league_app/backend/domains/rules"
	"league_app/backend/validation"
	"league_app/db"
	"league_app/models"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// Register mounts all API routes onto mux.
func Register(mux *http.ServeMux, dataDir string, deps Dependencies) {
	if deps.HandicapSvc == nil {
		panic("handlers.Register: deps.HandicapSvc must not be nil")
	}
	// Reject a typed-nil: an interface holding a nil concrete pointer is not nil
	// by == comparison but will panic on the first method call.
	if v := reflect.ValueOf(deps.HandicapSvc); v.Kind() == reflect.Ptr && v.IsNil() {
		panic("handlers.Register: deps.HandicapSvc must not be a typed nil")
	}
	// Guard HandicapApplier only when the Apply route will be mounted.
	// When AdminToken is empty the route is not registered, so a nil applier is fine.
	if deps.AdminToken != "" {
		if deps.HandicapApplier == nil {
			panic("handlers.Register: deps.HandicapApplier must not be nil when LEAGUE_ADMIN_TOKEN is set")
		}
		if v := reflect.ValueOf(deps.HandicapApplier); v.Kind() == reflect.Ptr && v.IsNil() {
			panic("handlers.Register: deps.HandicapApplier must not be a typed nil when LEAGUE_ADMIN_TOKEN is set")
		}
	}
	if deps.RuleMgr == nil {
		panic("handlers.Register: deps.RuleMgr must not be nil")
	}
	if v := reflect.ValueOf(deps.RuleMgr); v.Kind() == reflect.Ptr && v.IsNil() {
		panic("handlers.Register: deps.RuleMgr must not be a typed nil")
	}
	if deps.SeasonMgr == nil {
		panic("handlers.Register: deps.SeasonMgr must not be nil")
	}
	if v := reflect.ValueOf(deps.SeasonMgr); v.Kind() == reflect.Ptr && v.IsNil() {
		panic("handlers.Register: deps.SeasonMgr must not be a typed nil")
	}
	if deps.LeagueMgr == nil {
		panic("handlers.Register: deps.LeagueMgr must not be nil")
	}
	if v := reflect.ValueOf(deps.LeagueMgr); v.Kind() == reflect.Ptr && v.IsNil() {
		panic("handlers.Register: deps.LeagueMgr must not be a typed nil")
	}
	if deps.PlayerMgr == nil {
		panic("handlers.Register: deps.PlayerMgr must not be nil")
	}
	if v := reflect.ValueOf(deps.PlayerMgr); v.Kind() == reflect.Ptr && v.IsNil() {
		panic("handlers.Register: deps.PlayerMgr must not be a typed nil")
	}
	if deps.TeamMgr == nil {
		panic("handlers.Register: deps.TeamMgr must not be nil")
	}
	if v := reflect.ValueOf(deps.TeamMgr); v.Kind() == reflect.Ptr && v.IsNil() {
		panic("handlers.Register: deps.TeamMgr must not be a typed nil")
	}
	// Health-check route registration lives in api_health_routes.go.
	registerHealthRoute(mux)

	// Leagues, players, and teams route registration live in
	// api_leagues_routes.go, api_players_routes.go, and api_teams_routes.go.
	registerLeagueRoutes(mux, deps.LeagueMgr, deps.ApplyAuth)
	registerPlayerRoutes(mux, deps.PlayerMgr, deps.ApplyAuth)
	registerTeamRoutes(mux, deps.TeamMgr, deps.ApplyAuth)

	// Season CRUD, activation, rules, skipped-weeks, bye-requests, and season
	// team/roster route registration live in api_season_setup_routes.go.
	// seasonMgr is also used later by the round-results block and by
	// registerSeasonCloseRoutes below.
	seasonMgr := deps.SeasonMgr
	registerSeasonSetupRoutes(mux, seasonMgr, deps.RuleMgr, deps.ApplyAuth)

	// Match read and assignment route registration lives in api_match_routes.go.
	// Scoped to ?season_id= (season implies league).
	if deps.MatchMgr != nil {
		registerMatchRoutes(mux, deps.MatchMgr, deps.ApplyAuth)
	}
	// Schedule generation and pushback route registration live in
	// api_schedule_routes.go.
	if deps.ScheduleMgr != nil {
		registerScheduleGenerateRoute(mux, deps.ScheduleMgr, deps.ApplyAuth)
	}
	if deps.PushbackMgr != nil {
		registerPushbackPreviewRoute(mux, deps.PushbackMgr)
	}
	if deps.PushbackApplyMgr != nil {
		registerPushbackApplyRoute(mux, deps.PushbackApplyMgr, deps.ApplyAuth)
	}

	// Lineup plan route registration lives in api_lineup_routes.go.
	if deps.LineupMgr != nil {
		registerLineupRoutes(mux, deps.LineupMgr, deps.ApplyAuth)
	}

	// Rule definitions — developer-owned, served by the backend
	mux.HandleFunc("GET /api/rules/definitions", listRuleDefinitions)

	// Week workflow route registration lives in api_week_routes.go.
	// Routes are registered only when a WeekManager is wired in (always in production,
	// conditionally in tests that don't exercise week routes).
	if deps.WeekMgr != nil {
		registerWeekRoutes(mux, deps.WeekMgr, deps.ApplyAuth)
	}
	hcSvc := deps.HandicapSvc
	mux.HandleFunc("GET /api/seasons/{id}/handicap-recommendations", func(w http.ResponseWriter, r *http.Request) {
		getHandicapRecommendations(w, r, hcSvc)
	})

	// Apply route — only mounted when LEAGUE_ADMIN_TOKEN is configured.
	// Dual-tier auth: personal API key (ApplyAuth) checked first; AdminToken is the fallback.
	if deps.AdminToken != "" {
		applier := deps.HandicapApplier
		mux.HandleFunc("POST /api/seasons/{id}/handicap-apply",
			requireApplyAuth(deps.AdminToken, deps.ApplyAuth, func(w http.ResponseWriter, r *http.Request) {
				postHandicapApply(w, r, applier)
			}),
		)
		log.Println("Apply route: MOUNTED")
	} else {
		log.Println("Apply route: NOT MOUNTED - LEAGUE_ADMIN_TOKEN not set")
	}

	// User management — gated by the static admin token.
	// Only registered when the Apply route is mounted (AdminToken is non-empty).
	if deps.AdminToken != "" && deps.ApplyAuth != nil {
		auth := deps.ApplyAuth
		mux.HandleFunc("POST /api/users",
			requireAdminToken(deps.AdminToken, func(w http.ResponseWriter, r *http.Request) {
				postUser(w, r, auth)
			}),
		)
		mux.HandleFunc("GET /api/users",
			requireAdminToken(deps.AdminToken, func(w http.ResponseWriter, r *http.Request) {
				listUsers(w, r, auth)
			}),
		)
	}

	// Match results, rounds, standings, and player-stats route registration
	// live in api_match_results_routes.go. Gated on RoundMgr.
	if deps.RoundMgr != nil {
		registerMatchResultsRoutes(mux, deps.RoundMgr, seasonMgr, deps.ApplyAuth)
	}

	// Season close, close-preview, and reopen route registration live in
	// api_season_close_routes.go. Requires both WeekMgr (ListWeeks) and
	// RoundMgr (GetStandings).
	if deps.WeekMgr != nil && deps.RoundMgr != nil {
		registerSeasonCloseRoutes(mux, seasonMgr, deps.WeekMgr, deps.RoundMgr, deps.ApplyAuth)
	}

	// Backup -- system-admin only (Phase 6). league_admin is rejected here,
	// unlike the clearanceAuth-wrapped routes above.
	mux.HandleFunc("POST /api/backup",
		systemAdminAuth(deps.ApplyAuth, func(w http.ResponseWriter, r *http.Request) {
			path, err := db.Backup(dataDir)
			if err != nil {
				jsonError(w, err.Error(), 500)
				return
			}
			jsonOK(w, map[string]string{"path": path})
		}),
	)
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("encode error: %v", err)
	}
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// jsonValidation returns HTTP 422 with a validation.Result body.
// Callers should return immediately after calling this.
func jsonValidation(w http.ResponseWriter, result validation.Result) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnprocessableEntity)
	_ = json.NewEncoder(w).Encode(result)
}

func pathID(r *http.Request, key string) (int64, error) {
	s := r.PathValue(key)
	if s == "" {
		parts := strings.Split(r.URL.Path, "/")
		for i, p := range parts {
			if p == key && i+1 < len(parts) {
				s = parts[i+1]
				break
			}
		}
	}
	return strconv.ParseInt(s, 10, 64)
}

func qparam(r *http.Request, key string) string { return r.URL.Query().Get(key) }

func qparamInt(r *http.Request, key string) (int64, bool) {
	s := qparam(r, key)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseInt(s, 10, 64)
	return v, err == nil
}

func decode(r *http.Request, v any) error { return json.NewDecoder(r.Body).Decode(v) }

// requireAdminToken wraps a handler, enforcing bearer-token authorization.
// Responds 401 when no Authorization header is present (RFC 7235: includes
// WWW-Authenticate header), 403 when the token is present but does not match.
func requireAdminToken(token string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="league-admin"`)
			jsonError(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if auth != "Bearer "+token {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

// requireApplyAuth is dual-tier middleware for the Apply route.
// Tier 1: bearer token matched against a personal API key via resolver → sets user ID in context.
// Tier 2: bearer token matched against the static adminToken → allows with nil user ID (logs deprecation).
// Returns 401 when no Authorization header is present, 403 when neither tier matches.
func requireApplyAuth(adminToken string, resolver ApplyAuthResolver, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="league-admin"`)
			jsonError(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(auth, "Bearer ") {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")

		// Tier 1: personal API key lookup.
		if resolver != nil {
			user, err := resolver.ResolveApplyUserByAPIKey(r.Context(), token)
			if err != nil {
				log.Printf("apply auth: key resolution error: %v", err)
				jsonError(w, "internal error", http.StatusInternalServerError)
				return
			}
			if user != nil {
				ctx := context.WithValue(r.Context(), applyUserIDKey{}, user.ID)
				next(w, r.WithContext(ctx))
				return
			}
		}

		// Tier 2: static admin token fallback.
		if token == adminToken {
			log.Println("apply auth: LEAGUE_ADMIN_TOKEN used — deprecated, create a personal API key")
			next(w, r)
			return
		}

		jsonError(w, "forbidden", http.StatusForbidden)
	}
}

// applyUserIDFromContext returns the user ID stored by requireApplyAuth, or nil
// when the request was authenticated via the static admin token fallback.
func applyUserIDFromContext(ctx context.Context) *int64 {
	v, _ := ctx.Value(applyUserIDKey{}).(int64)
	if v == 0 {
		return nil
	}
	return &v
}

// clearanceUserFromContext returns the *models.User stored by requirePersonalKeyAuth,
// or nil when no user is in the request context.
func clearanceUserFromContext(ctx context.Context) *models.User {
	u, _ := ctx.Value(clearanceUserKey{}).(*models.User)
	return u
}

// requirePersonalKeyAuth is personal-key-only middleware for clearance routes.
// No static LEAGUE_ADMIN_TOKEN fallback -- that path is reserved for handicap-apply.
// Returns 401 (with WWW-Authenticate) when Authorization is absent, 403 when the
// token does not resolve to an active user, 500 on resolver error.
// The resolved *models.User is stored in context for downstream role checks.
func requirePersonalKeyAuth(resolver ApplyAuthResolver, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Bearer realm="league-admin"`)
			jsonError(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if !strings.HasPrefix(auth, "Bearer ") {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		user, err := resolver.ResolveApplyUserByAPIKey(r.Context(), token)
		if err != nil {
			log.Printf("clearance auth: key resolution error: %v", err)
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if user == nil {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		ctx := context.WithValue(r.Context(), clearanceUserKey{}, user)
		next(w, r.WithContext(ctx))
	}
}

// requireLeagueAdminRole checks that the user in context has an allowed role.
// Allowed: "league_admin", "admin" (backward-compatible alias), "system_admin".
// Returns 403 when no user is in context or when the role is not in the allowed set.
func requireLeagueAdminRole(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := clearanceUserFromContext(r.Context())
		if user == nil {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		switch user.Role {
		case "league_admin", "admin", "system_admin":
			next(w, r)
		default:
			jsonError(w, "forbidden", http.StatusForbidden)
		}
	}
}

// clearanceAuth wraps h with personal-key auth and league_admin role check when
// resolver is non-nil. When resolver is nil, h is returned unmodified so routes
// remain accessible in test and minimal-dependency setups without ApplyAuth.
func clearanceAuth(resolver ApplyAuthResolver, h http.HandlerFunc) http.HandlerFunc {
	if resolver == nil {
		return h
	}
	return requirePersonalKeyAuth(resolver, requireLeagueAdminRole(h))
}

// requireSystemAdminRole checks that the user in context has an allowed role
// for system-level operations. Allowed: "system_admin", "admin" (legacy
// system-admin-compatible alias). Unlike requireLeagueAdminRole, "league_admin"
// is not allowed here. Returns 403 when no user is in context or when the role
// is not in the allowed set.
func requireSystemAdminRole(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := clearanceUserFromContext(r.Context())
		if user == nil {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		switch user.Role {
		case "system_admin", "admin":
			next(w, r)
		default:
			jsonError(w, "forbidden", http.StatusForbidden)
		}
	}
}

// systemAdminAuth wraps h with personal-key auth and requireSystemAdminRole
// when resolver is non-nil. When resolver is nil, h is returned unmodified,
// matching the nil-resolver compatibility behavior of clearanceAuth so
// existing test setups without ApplyAuth remain unaffected.
func systemAdminAuth(resolver ApplyAuthResolver, h http.HandlerFunc) http.HandlerFunc {
	if resolver == nil {
		return h
	}
	return requirePersonalKeyAuth(resolver, requireSystemAdminRole(h))
}

// ─── Leagues ─────────────────────────────────────────────────────────────────

// ─── Matches ─────────────────────────────────────────────────────────────────

// ─── Rule Definitions ─────────────────────────────────────────────────────────

func listRuleDefinitions(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, rules.Definitions())
}
