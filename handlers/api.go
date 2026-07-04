// Package handlers wires HTTP routes to database operations.
package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"

	"league_app/backend/domainerr"
	"league_app/backend/domains/handicaps"
	"league_app/backend/domains/matches"
	"league_app/backend/domains/rules"
	"league_app/backend/domains/seasons"
	"league_app/backend/validation"
	"league_app/db"
	"league_app/models"
	"log"
	"math"
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
	// Leagues
	mux.HandleFunc("GET /api/leagues", listLeagues)
	mux.HandleFunc("POST /api/leagues", createLeague)
	mux.HandleFunc("GET /api/leagues/{id}", getLeague)
	mux.HandleFunc("PUT /api/leagues/{id}", updateLeague)
	mux.HandleFunc("DELETE /api/leagues/{id}", deleteLeague)

	// Players — scoped to ?league_id=
	mux.HandleFunc("GET /api/players", listPlayers)
	mux.HandleFunc("POST /api/players", createPlayer)
	mux.HandleFunc("GET /api/players/{id}", getPlayer)
	mux.HandleFunc("PUT /api/players/{id}", updatePlayer)
	mux.HandleFunc("DELETE /api/players/{id}", deletePlayer)

	// Teams — scoped to ?league_id=
	mux.HandleFunc("GET /api/teams", listTeams)
	mux.HandleFunc("POST /api/teams", createTeam)
	mux.HandleFunc("GET /api/teams/{id}", getTeam)
	mux.HandleFunc("PUT /api/teams/{id}", updateTeam)
	mux.HandleFunc("DELETE /api/teams/{id}", deleteTeam)

	// Seasons — scoped to ?league_id=
	mux.HandleFunc("GET /api/seasons", listSeasons)
	mux.HandleFunc("POST /api/seasons", createSeason)
	mux.HandleFunc("GET /api/seasons/{id}", getSeason)
	mux.HandleFunc("PUT /api/seasons/{id}", updateSeason)
	mux.HandleFunc("DELETE /api/seasons/{id}", deleteSeason)
	seasonMgr := deps.SeasonMgr
	mux.HandleFunc("POST /api/seasons/{id}/activate", func(w http.ResponseWriter, r *http.Request) {
		activateSeason(w, r, seasonMgr)
	})

	// Season sub-resources
	ruleMgr := deps.RuleMgr
	mux.HandleFunc("GET /api/seasons/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		listSeasonRules(w, r, ruleMgr)
	})
	mux.HandleFunc("POST /api/seasons/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		createSeasonRule(w, r, ruleMgr)
	})
	mux.HandleFunc("PUT /api/seasons/{id}/rules/{rid}", func(w http.ResponseWriter, r *http.Request) {
		updateSeasonRule(w, r, ruleMgr)
	})
	mux.HandleFunc("DELETE /api/seasons/{id}/rules/{rid}", func(w http.ResponseWriter, r *http.Request) {
		deleteSeasonRule(w, r, ruleMgr)
	})

	mux.HandleFunc("GET /api/seasons/{id}/skipped-weeks", listSkippedWeeks)
	mux.HandleFunc("POST /api/seasons/{id}/skipped-weeks", createSkippedWeek)
	mux.HandleFunc("DELETE /api/seasons/{id}/skipped-weeks/{sid}", deleteSkippedWeek)

	mux.HandleFunc("GET /api/seasons/{id}/bye-requests", listByeRequests)
	mux.HandleFunc("POST /api/seasons/{id}/bye-requests", func(w http.ResponseWriter, r *http.Request) {
		createByeRequest(w, r, seasonMgr)
	})
	mux.HandleFunc("PUT /api/seasons/{id}/bye-requests/{bid}", func(w http.ResponseWriter, r *http.Request) {
		updateByeRequest(w, r, seasonMgr)
	})
	mux.HandleFunc("DELETE /api/seasons/{id}/bye-requests/{bid}", deleteByeRequest)

	// Season teams and rosters
	mux.HandleFunc("GET /api/seasons/{id}/teams", listSeasonTeams)
	mux.HandleFunc("POST /api/seasons/{id}/teams", func(w http.ResponseWriter, r *http.Request) {
		addSeasonTeam(w, r, seasonMgr)
	})
	mux.HandleFunc("GET /api/seasons/{id}/previous", func(w http.ResponseWriter, r *http.Request) {
		getPreviousSeasonTeams(w, r, seasonMgr)
	})
	mux.HandleFunc("PUT /api/seasons/{id}/teams/{tid}", func(w http.ResponseWriter, r *http.Request) {
		updateSeasonTeam(w, r, seasonMgr)
	})
	mux.HandleFunc("DELETE /api/seasons/{id}/teams/{tid}", func(w http.ResponseWriter, r *http.Request) {
		removeSeasonTeam(w, r, seasonMgr)
	})
	mux.HandleFunc("GET /api/seasons/{id}/teams/{tid}/roster", listSeasonRoster)
	mux.HandleFunc("POST /api/seasons/{id}/teams/{tid}/roster", func(w http.ResponseWriter, r *http.Request) {
		addRosterPlayer(w, r, seasonMgr)
	})
	mux.HandleFunc("DELETE /api/seasons/{id}/teams/{tid}/roster/{pid}", func(w http.ResponseWriter, r *http.Request) {
		removeRosterPlayer(w, r, seasonMgr)
	})
	mux.HandleFunc("GET /api/seasons/{id}/players/available", listAvailablePlayers)
	mux.HandleFunc("GET /api/seasons/{id}/checklist", func(w http.ResponseWriter, r *http.Request) {
		getSeasonChecklist(w, r, seasonMgr)
	})

	// Matches — scoped to ?season_id= (season implies league)
	if deps.MatchMgr != nil {
		matchMgr := deps.MatchMgr
		mux.HandleFunc("GET /api/matches", func(w http.ResponseWriter, r *http.Request) {
			listMatches(w, r, matchMgr)
		})
		mux.HandleFunc("GET /api/matches/{id}", func(w http.ResponseWriter, r *http.Request) {
			getMatch(w, r, matchMgr)
		})
		mux.HandleFunc("PATCH /api/matches/{id}/assign", func(w http.ResponseWriter, r *http.Request) {
			assignMatchTeams(w, r, matchMgr)
		})
	}
	if deps.ScheduleMgr != nil {
		scheduleMgr := deps.ScheduleMgr
		mux.HandleFunc("POST /api/matches/generate", func(w http.ResponseWriter, r *http.Request) {
			generateSchedule(w, r, scheduleMgr)
		})
	}

	// Lineup plans — pre-game slot assignments per team/week
	if deps.LineupMgr != nil {
		lineupMgr := deps.LineupMgr
		mux.HandleFunc("GET /api/lineup-plans", func(w http.ResponseWriter, r *http.Request) {
			listLineupPlans(w, r, lineupMgr)
		})
		mux.HandleFunc("POST /api/lineup-plans", func(w http.ResponseWriter, r *http.Request) {
			saveTeamLineup(w, r, lineupMgr)
		})
		mux.HandleFunc("DELETE /api/lineup-plans/{id}", func(w http.ResponseWriter, r *http.Request) {
			deleteLineupPlan(w, r, lineupMgr)
		})
	}

	// Rule definitions — developer-owned, served by the backend
	mux.HandleFunc("GET /api/rules/definitions", listRuleDefinitions)

	// Week workflow -- Close Week gate
	// Routes are registered only when a WeekManager is wired in (always in production,
	// conditionally in tests that don't exercise week routes).
	if deps.WeekMgr != nil {
		weekMgr := deps.WeekMgr
		mux.HandleFunc("GET /api/seasons/{id}/weeks", func(w http.ResponseWriter, r *http.Request) {
			listWeeks(w, r, weekMgr)
		})
		mux.HandleFunc("GET /api/seasons/{id}/weeks/{week}/validate", func(w http.ResponseWriter, r *http.Request) {
			validateWeekHandler(w, r, weekMgr)
		})
		mux.HandleFunc("POST /api/seasons/{id}/weeks/{week}/close", func(w http.ResponseWriter, r *http.Request) {
			closeWeekHandler(w, r, weekMgr)
		})
		mux.HandleFunc("POST /api/seasons/{id}/weeks/{week}/reopen", func(w http.ResponseWriter, r *http.Request) {
			reopenWeekHandler(w, r, weekMgr)
		})
		mux.HandleFunc("GET /api/seasons/{id}/weeks/{week}/acknowledgments", func(w http.ResponseWriter, r *http.Request) {
			getWeekAcknowledgments(w, r, weekMgr)
		})
		mux.HandleFunc("GET /api/seasons/{id}/weeks/{week}/advance-preview", func(w http.ResponseWriter, r *http.Request) {
			getAdvancePreview(w, r, weekMgr)
		})
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

	// Round results, standings, and stats — gated on RoundMgr.
	if deps.RoundMgr != nil {
		roundMgr := deps.RoundMgr
		mux.HandleFunc("POST /api/matches/{id}/results", func(w http.ResponseWriter, r *http.Request) {
			submitResults(w, r, roundMgr)
		})
		mux.HandleFunc("DELETE /api/matches/{id}/results", func(w http.ResponseWriter, r *http.Request) {
			clearResults(w, r, roundMgr)
		})
		mux.HandleFunc("GET /api/matches/{id}/rounds", func(w http.ResponseWriter, r *http.Request) {
			getRounds(w, r, roundMgr)
		})
		mux.HandleFunc("POST /api/matches/{id}/rounds", func(w http.ResponseWriter, r *http.Request) {
			saveRounds(w, r, roundMgr)
		})
		mux.HandleFunc("GET /api/standings", func(w http.ResponseWriter, r *http.Request) {
			getStandings(w, r, roundMgr)
		})
		mux.HandleFunc("GET /api/player-stats", func(w http.ResponseWriter, r *http.Request) {
			getPlayerStats(w, r, roundMgr)
		})
	}

	// Backup
	mux.HandleFunc("POST /api/backup", func(w http.ResponseWriter, r *http.Request) {
		path, err := db.Backup(dataDir)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		jsonOK(w, map[string]string{"path": path})
	})
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

// ─── Leagues ─────────────────────────────────────────────────────────────────

func listLeagues(w http.ResponseWriter, r *http.Request) {
	rows, err := db.DB.Query(
		`SELECT id, name, game_format, COALESCE(day_of_week,''), created_at FROM leagues ORDER BY id`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var leagues []models.League
	for rows.Next() {
		var l models.League
		rows.Scan(&l.ID, &l.Name, &l.GameFormat, &l.DayOfWeek, &l.CreatedAt)
		leagues = append(leagues, l)
	}
	if leagues == nil {
		leagues = []models.League{}
	}
	jsonOK(w, leagues)
}

func createLeague(w http.ResponseWriter, r *http.Request) {
	var l models.League
	if err := decode(r, &l); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if strings.TrimSpace(l.Name) == "" {
		jsonError(w, "name is required", 400)
		return
	}
	if l.GameFormat == "" {
		l.GameFormat = "8ball"
	}
	res, err := db.DB.Exec(
		`INSERT INTO leagues (name, game_format, day_of_week) VALUES (?,?,?)`,
		l.Name, l.GameFormat, l.DayOfWeek)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	l.ID, _ = res.LastInsertId()
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, l)
}

func getLeague(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var l models.League
	err = db.DB.QueryRow(
		`SELECT id, name, game_format, COALESCE(day_of_week,''), created_at FROM leagues WHERE id=?`, id,
	).Scan(&l.ID, &l.Name, &l.GameFormat, &l.DayOfWeek, &l.CreatedAt)
	if err != nil {
		jsonError(w, "league not found", 404)
		return
	}
	jsonOK(w, l)
}

func updateLeague(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var l models.League
	if err := decode(r, &l); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	_, err = db.DB.Exec(
		`UPDATE leagues SET name=?, game_format=?, day_of_week=? WHERE id=?`,
		l.Name, l.GameFormat, l.DayOfWeek, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	l.ID = id
	jsonOK(w, l)
}

func deleteLeague(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if _, err := db.DB.Exec(`DELETE FROM leagues WHERE id=?`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// ─── Players — scoped to league via team ─────────────────────────────────────

func listPlayers(w http.ResponseWriter, r *http.Request) {
	leagueID, hasLeague := qparamInt(r, "league_id")
	var rows *sql.Rows
	var err error
	const sel = `SELECT p.id, p.player_number, p.first_name, p.last_name,
	                    p.first_name || ' ' || p.last_name,
	                    COALESCE(p.phone,''), COALESCE(p.email,''),
	                    p.team_id, COALESCE(t.name,''), COALESCE(t.league_id,0),
	                    p.handicap, p.admin_hold, COALESCE(p.active,1), COALESCE(p.note,''),
	                    p.created_at
	             FROM players p LEFT JOIN teams t ON t.id = p.team_id`
	if hasLeague {
		rows, err = db.DB.Query(sel+` WHERE t.league_id = ? ORDER BY p.last_name, p.first_name`, leagueID)
	} else {
		rows, err = db.DB.Query(sel + ` ORDER BY p.last_name, p.first_name`)
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var players []models.Player
	for rows.Next() {
		var p models.Player
		var adminHold int
		var activeInt int
		rows.Scan(&p.ID, &p.PlayerNumber, &p.FirstName, &p.LastName, &p.Name,
			&p.Phone, &p.Email, &p.TeamID, &p.TeamName, &p.LeagueID,
			&p.Handicap, &adminHold, &activeInt, &p.Note, &p.CreatedAt)
		p.AdminHold = adminHold == 1
		p.Active = activeInt == 1
		players = append(players, p)
	}
	if players == nil {
		players = []models.Player{}
	}
	jsonOK(w, players)
}

func createPlayer(w http.ResponseWriter, r *http.Request) {
	var p models.Player
	if err := decode(r, &p); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if strings.TrimSpace(p.FirstName) == "" && strings.TrimSpace(p.LastName) == "" {
		jsonError(w, "first or last name is required", 400)
		return
	}
	adminHold := 0
	if p.AdminHold {
		adminHold = 1
	}
	res, err := db.DB.Exec(
		`INSERT INTO players (player_number, first_name, last_name, phone, email, team_id, handicap, admin_hold)
		 VALUES (?,?,?,?,?,?,?,?)`,
		p.PlayerNumber, p.FirstName, p.LastName, p.Phone, p.Email, p.TeamID, p.Handicap, adminHold)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	p.ID, _ = res.LastInsertId()
	p.Name = p.FirstName + " " + p.LastName
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, p)
}

func getPlayer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var p models.Player
	var adminHold int
	var activeInt int
	err = db.DB.QueryRow(`
		SELECT p.id, p.player_number, p.first_name, p.last_name,
		       p.first_name || ' ' || p.last_name,
		       COALESCE(p.phone,''), COALESCE(p.email,''),
		       p.team_id, COALESCE(t.name,''), COALESCE(t.league_id,0),
		       p.handicap, p.admin_hold, COALESCE(p.active,1), COALESCE(p.note,''),
		       p.created_at
		FROM players p LEFT JOIN teams t ON t.id = p.team_id WHERE p.id=?`, id,
	).Scan(&p.ID, &p.PlayerNumber, &p.FirstName, &p.LastName, &p.Name,
		&p.Phone, &p.Email, &p.TeamID, &p.TeamName, &p.LeagueID,
		&p.Handicap, &adminHold, &activeInt, &p.Note, &p.CreatedAt)
	if err != nil {
		jsonError(w, "player not found", 404)
		return
	}
	p.AdminHold = adminHold == 1
	p.Active = activeInt == 1
	jsonOK(w, p)
}

func updatePlayer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var p models.Player
	if err := decode(r, &p); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	adminHold := 0
	if p.AdminHold {
		adminHold = 1
	}
	// player_number is NOT updated here — it is locked once set (only writable on create)
	_, err = db.DB.Exec(
		`UPDATE players SET first_name=?, last_name=?, phone=?, email=?,
		 team_id=?, handicap=?, admin_hold=? WHERE id=?`,
		p.FirstName, p.LastName, p.Phone, p.Email, p.TeamID, p.Handicap, adminHold, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	p.ID = id
	p.Name = p.FirstName + " " + p.LastName
	jsonOK(w, p)
}

func deletePlayer(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var historyCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM handicap_history WHERE player_id = ?`, id).Scan(&historyCount); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if historyCount > 0 {
		jsonError(w, "This player has handicap history records and cannot be deleted. Deactivate the player instead.", 409)
		return
	}
	if _, err := db.DB.Exec(`DELETE FROM players WHERE id=?`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// ─── Teams — scoped to league_id ─────────────────────────────────────────────

func listTeams(w http.ResponseWriter, r *http.Request) {
	leagueID, hasLeague := qparamInt(r, "league_id")
	var rows *sql.Rows
	var err error
	if hasLeague {
		rows, err = db.DB.Query(
			`SELECT id, league_id, name, COALESCE(team_number,''), captain_id, created_at FROM teams WHERE league_id=? ORDER BY name`,
			leagueID)
	} else {
		rows, err = db.DB.Query(
			`SELECT id, league_id, name, COALESCE(team_number,''), captain_id, created_at FROM teams ORDER BY league_id, name`)
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var teams []models.Team
	for rows.Next() {
		var t models.Team
		rows.Scan(&t.ID, &t.LeagueID, &t.Name, &t.TeamNumber, &t.CaptainID, &t.CreatedAt)
		teams = append(teams, t)
	}
	if teams == nil {
		teams = []models.Team{}
	}
	jsonOK(w, teams)
}

func createTeam(w http.ResponseWriter, r *http.Request) {
	var t models.Team
	if err := decode(r, &t); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if strings.TrimSpace(t.Name) == "" {
		jsonError(w, "name is required", 400)
		return
	}
	if t.LeagueID == 0 {
		jsonError(w, "league_id is required", 400)
		return
	}
	res, err := db.DB.Exec(`INSERT INTO teams (league_id, name) VALUES (?,?)`, t.LeagueID, t.Name)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	t.ID, _ = res.LastInsertId()
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, t)
}

func getTeam(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var t models.Team
	err = db.DB.QueryRow(
		`SELECT id, league_id, name, COALESCE(team_number,''), captain_id, created_at FROM teams WHERE id=?`, id,
	).Scan(&t.ID, &t.LeagueID, &t.Name, &t.TeamNumber, &t.CaptainID, &t.CreatedAt)
	if err != nil {
		jsonError(w, "team not found", 404)
		return
	}
	rows, _ := db.DB.Query(
		`SELECT id, player_number, first_name, last_name,
		        first_name || ' ' || last_name, handicap
		 FROM players WHERE team_id=? ORDER BY player_number`, id)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var p models.Player
			rows.Scan(&p.ID, &p.PlayerNumber, &p.FirstName, &p.LastName, &p.Name, &p.Handicap)
			p.TeamID = &t.ID
			p.LeagueID = t.LeagueID
			t.Players = append(t.Players, p)
		}
	}
	jsonOK(w, t)
}

func updateTeam(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var t models.Team
	if err := decode(r, &t); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	_, err = db.DB.Exec(`UPDATE teams SET name=?, captain_id=? WHERE id=?`, t.Name, t.CaptainID, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	t.ID = id
	jsonOK(w, t)
}

func deleteTeam(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	db.DB.Exec(`UPDATE players SET team_id=NULL WHERE team_id=?`, id)
	if _, err := db.DB.Exec(`DELETE FROM teams WHERE id=?`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// ─── Seasons — scoped to league_id ───────────────────────────────────────────

const seasonCols = `id, league_id, name, start_date, end_date, active, schedule_type, num_weeks, COALESCE(schedule_stale,0), COALESCE(teams_managed,0), activated_at, created_at`

func scanSeason(row interface{ Scan(...any) error }) (models.Season, error) {
	var s models.Season
	var active, stale, managed int
	err := row.Scan(&s.ID, &s.LeagueID, &s.Name, &s.StartDate, &s.EndDate,
		&active, &s.ScheduleType, &s.NumWeeks, &stale, &managed, &s.ActivatedAt, &s.CreatedAt)
	s.Active = active == 1
	s.ScheduleStale = stale == 1
	s.TeamsManaged = managed == 1
	if s.ScheduleType == "" {
		s.ScheduleType = "double_rr"
	}
	// modernc.org/sqlite converts DATE columns to time.Time, which serialises to
	// a full ISO-8601 timestamp. Trim to YYYY-MM-DD so date inputs work correctly.
	s.StartDate = normDatePtr(s.StartDate)
	s.EndDate = normDatePtr(s.EndDate)
	return s, err
}

// normDatePtr trims a date pointer to YYYY-MM-DD, discarding any time component
// added by the SQLite driver when it coerces DATE columns to time.Time.
func normDatePtr(s *string) *string {
	if s == nil || len(*s) <= 10 {
		return s
	}
	v := (*s)[:10]
	return &v
}

// normDateStr trims a date string to YYYY-MM-DD, discarding any time component.
func normDateStr(s string) string {
	if len(s) <= 10 {
		return s
	}
	return s[:10]
}

func listSeasons(w http.ResponseWriter, r *http.Request) {
	leagueID, hasLeague := qparamInt(r, "league_id")
	var rows *sql.Rows
	var err error
	q := `SELECT ` + seasonCols + ` FROM seasons`
	if hasLeague {
		rows, err = db.DB.Query(q+` WHERE league_id=? ORDER BY id DESC`, leagueID)
	} else {
		rows, err = db.DB.Query(q + ` ORDER BY league_id, id DESC`)
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var seasons []models.Season
	for rows.Next() {
		s, err := scanSeason(rows)
		if err != nil {
			continue
		}
		seasons = append(seasons, s)
	}
	if seasons == nil {
		seasons = []models.Season{}
	}
	jsonOK(w, seasons)
}

func createSeason(w http.ResponseWriter, r *http.Request) {
	var s models.Season
	if err := decode(r, &s); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if strings.TrimSpace(s.Name) == "" {
		jsonError(w, "name is required", 400)
		return
	}
	if s.LeagueID == 0 {
		jsonError(w, "league_id is required", 400)
		return
	}
	if s.ScheduleType == "" {
		s.ScheduleType = "double_rr"
	}
	res, err := db.DB.Exec(
		`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks, teams_managed) VALUES (?,?,?,?,?,1)`,
		s.LeagueID, s.Name, s.StartDate, s.ScheduleType, s.NumWeeks)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.ID, _ = res.LastInsertId()
	s.TeamsManaged = true

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, s)
}

func getSeason(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	row := db.DB.QueryRow(`SELECT `+seasonCols+` FROM seasons WHERE id=?`, id)
	s, err := scanSeason(row)
	if err != nil {
		jsonError(w, "season not found", 404)
		return
	}
	jsonOK(w, s)
}

func updateSeason(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var s models.Season
	if err := decode(r, &s); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if s.ScheduleType == "" {
		s.ScheduleType = "double_rr"
	}
	_, err = db.DB.Exec(
		`UPDATE seasons SET name=?, start_date=?, schedule_type=?, num_weeks=? WHERE id=?`,
		s.Name, s.StartDate, s.ScheduleType, s.NumWeeks, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.ID = id
	jsonOK(w, s)
}

func deleteSeason(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if _, err := db.DB.Exec(`DELETE FROM seasons WHERE id=?`, id); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

func activateSeason(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.Activate(r.Context(), id); err != nil {
		var blockErr *seasons.ChecklistBlockErr
		switch {
		case errors.As(err, &blockErr):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":    "season cannot be activated; resolve all blockers first",
				"blockers": blockErr.Blockers,
			})
		case errors.Is(err, seasons.ErrNotFound):
			jsonError(w, "season not found", 404)
		default:
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, map[string]string{"status": "activated"})
}

// ─── Matches ─────────────────────────────────────────────────────────────────

func listMatches(w http.ResponseWriter, r *http.Request, mgr MatchManager) {
	req := matches.ListMatchesRequest{}
	if v, ok := qparamInt(r, "season_id"); ok {
		req.SeasonID = v
	}
	if v, ok := qparamInt(r, "league_id"); ok {
		req.LeagueID = v
	}
	ms, err := mgr.ListMatches(r.Context(), req)
	if err != nil {
		mapMatchErr(w, err)
		return
	}
	jsonOK(w, ms)
}

func generateSchedule(w http.ResponseWriter, r *http.Request, mgr ScheduleManager) {
	var req matches.GenerateRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	result, err := mgr.GenerateSchedule(r.Context(), req)
	if err != nil {
		mapScheduleErr(w, err)
		return
	}
	jsonOK(w, result)
}

// mapScheduleErr translates schedule domain errors to HTTP responses.
func mapScheduleErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	if errors.As(err, &de) {
		switch de.Category {
		case domainerr.NotFound:
			jsonError(w, de.Message, http.StatusNotFound)
		case domainerr.InvalidInput:
			jsonError(w, de.Message, http.StatusBadRequest)
		default:
			jsonError(w, de.Message, http.StatusInternalServerError)
		}
		return
	}
	jsonError(w, err.Error(), http.StatusInternalServerError)
}

// assignMatchTeams assigns home/away teams to a blanket (unassigned) match slot.
func assignMatchTeams(w http.ResponseWriter, r *http.Request, mgr MatchManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req models.AssignMatchTeamsRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if err := mgr.AssignMatchTeams(r.Context(), id, req.HomeTeamID, req.AwayTeamID); err != nil {
		mapMatchErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "assigned"})
}

// mapMatchErr translates match domain errors to HTTP responses.
func mapMatchErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	if errors.As(err, &de) {
		switch de.Category {
		case domainerr.NotFound:
			jsonError(w, de.Message, http.StatusNotFound)
		case domainerr.InvalidInput:
			jsonError(w, de.Message, http.StatusBadRequest)
		default:
			jsonError(w, de.Message, http.StatusInternalServerError)
		}
		return
	}
	jsonError(w, err.Error(), http.StatusInternalServerError)
}

// ─── Rule Definitions ─────────────────────────────────────────────────────────

func listRuleDefinitions(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, rules.Definitions())
}

// ─── Season Rules ─────────────────────────────────────────────────────────────

func listSeasonRules(w http.ResponseWriter, r *http.Request, mgr RuleManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	rows, err := mgr.List(r.Context(), sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, rows)
}

func createSeasonRule(w http.ResponseWriter, r *http.Request, mgr RuleManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var ru models.SeasonRule
	if err := decode(r, &ru); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	ru.SeasonID = sid
	saved, err := mgr.Upsert(r.Context(), ru)
	if err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.InvalidInput {
			jsonError(w, de.Message, http.StatusBadRequest)
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, saved)
}

func updateSeasonRule(w http.ResponseWriter, r *http.Request, mgr RuleManager) {
	rid, err := pathID(r, "rid")
	if err != nil {
		jsonError(w, "invalid rule id", 400)
		return
	}
	var ru models.SeasonRule
	if err := decode(r, &ru); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if err := mgr.Update(r.Context(), rid, ru.RuleLabel, ru.RuleValue); err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) {
			switch de.Category {
			case domainerr.NotFound:
				jsonError(w, de.Message, http.StatusNotFound)
			case domainerr.InvalidInput:
				jsonError(w, de.Message, http.StatusBadRequest)
			default:
				jsonError(w, de.Message, http.StatusInternalServerError)
			}
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	ru.ID = rid
	jsonOK(w, ru)
}

func deleteSeasonRule(w http.ResponseWriter, r *http.Request, mgr RuleManager) {
	rid, err := pathID(r, "rid")
	if err != nil {
		jsonError(w, "invalid rule id", 400)
		return
	}
	if err := mgr.Delete(r.Context(), rid); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// ─── Skipped Weeks ────────────────────────────────────────────────────────────

func listSkippedWeeks(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	rows, err := db.DB.Query(
		`SELECT id, season_id, skip_date, reason FROM skipped_weeks WHERE season_id=? ORDER BY skip_date`, sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var weeks []models.SkippedWeek
	for rows.Next() {
		var sw models.SkippedWeek
		rows.Scan(&sw.ID, &sw.SeasonID, &sw.SkipDate, &sw.Reason)
		sw.SkipDate = normDateStr(sw.SkipDate)
		weeks = append(weeks, sw)
	}
	if weeks == nil {
		weeks = []models.SkippedWeek{}
	}
	jsonOK(w, weeks)
}

func createSkippedWeek(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var sw models.SkippedWeek
	if err := decode(r, &sw); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	sw.SeasonID = sid
	res, err := db.DB.Exec(
		`INSERT OR IGNORE INTO skipped_weeks (season_id, skip_date, reason) VALUES (?,?,?)`,
		sw.SeasonID, sw.SkipDate, sw.Reason)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	sw.ID, _ = res.LastInsertId()
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, sw)
}

func deleteSkippedWeek(w http.ResponseWriter, r *http.Request) {
	swid, err := pathID(r, "sid")
	if err != nil {
		jsonError(w, "invalid skip id", 400)
		return
	}
	db.DB.Exec(`DELETE FROM skipped_weeks WHERE id=?`, swid)
	jsonOK(w, map[string]string{"status": "deleted"})
}

// ─── Bye Requests ─────────────────────────────────────────────────────────────

func listByeRequests(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	rows, err := db.DB.Query(`
		SELECT br.id, br.season_id, br.team_id, t.name, br.week_number, br.reason, br.approved
		FROM bye_requests br
		JOIN teams t ON t.id = br.team_id
		WHERE br.season_id=? ORDER BY br.week_number, br.team_id`, sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var byes []models.ByeRequest
	for rows.Next() {
		var b models.ByeRequest
		var approved int
		rows.Scan(&b.ID, &b.SeasonID, &b.TeamID, &b.TeamName, &b.WeekNumber, &b.Reason, &approved)
		b.Approved = approved == 1
		byes = append(byes, b)
	}
	if byes == nil {
		byes = []models.ByeRequest{}
	}
	jsonOK(w, byes)
}

func createByeRequest(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req seasons.CreateByeRequestInput
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	b, err := mgr.CreateByeRequest(r.Context(), sid, req)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, b)
}

func updateByeRequest(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid season id", 400)
		return
	}
	bid, err := pathID(r, "bid")
	if err != nil {
		jsonError(w, "invalid bye id", 400)
		return
	}
	var body struct {
		Approved bool `json:"approved"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	b, err := mgr.UpdateByeRequest(r.Context(), sid, bid, body.Approved)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, b)
}

func deleteByeRequest(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid season id", 400)
		return
	}
	bid, err := pathID(r, "bid")
	if err != nil {
		jsonError(w, "invalid bye id", 400)
		return
	}
	res, err := db.DB.Exec(`DELETE FROM bye_requests WHERE id=? AND season_id=?`, bid, sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "bye request not found", 404)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

func getMatch(w http.ResponseWriter, r *http.Request, mgr MatchManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	detail, err := mgr.GetMatch(r.Context(), id)
	if err != nil {
		mapMatchErr(w, err)
		return
	}
	jsonOK(w, detail)
}

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
	if err := mgr.SubmitResults(r.Context(), id, req.Results); err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.Conflict {
			jsonError(w, de.Message, http.StatusConflict)
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "saved"})
}

func clearResults(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.ClearResults(r.Context(), id); err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.Conflict {
			jsonError(w, de.Message, http.StatusConflict)
			return
		}
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "cleared"})
}

// Week Workflow ---------------------------------------------------------------

func listWeeks(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	summaries, err := mgr.ListWeeks(r.Context(), seasonID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, summaries)
}

func validateWeekHandler(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weekNum, err := pathID(r, "week")
	if err != nil {
		jsonError(w, "invalid week", 400)
		return
	}
	result, err := mgr.ValidateWeek(r.Context(), seasonID, weekNum)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, result)
}

func closeWeekHandler(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weekNum, err := pathID(r, "week")
	if err != nil {
		jsonError(w, "invalid week", 400)
		return
	}

	type ackReq struct {
		Acknowledgments []matches.AckEntry `json:"acknowledgments"`
	}
	var body ackReq
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err != io.EOF {
			jsonError(w, "invalid close week request body", http.StatusBadRequest)
			return
		}
	}

	result, err := mgr.CloseWeek(r.Context(), matches.CloseWeekRequest{
		SeasonID:        seasonID,
		WeekNumber:      weekNum,
		Acknowledgments: body.Acknowledgments,
	})
	if err != nil {
		var wce *matches.WeekCloseErr
		if errors.As(err, &wce) {
			jsonValidation(w, wce.Result)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}

	// Best-effort advance result; close is already committed.
	ar, aerr := mgr.AdvanceData(r.Context(), seasonID, weekNum)
	if aerr != nil {
		jsonOK(w, map[string]any{
			"closed":               true,
			"week_number":          int(weekNum),
			"acknowledgment_count": result.AckCount,
		})
		return
	}
	ar.Message = "Week closed. Standings and player stats now include this week's results."
	jsonOK(w, map[string]any{
		"closed":               true,
		"week_number":          int(weekNum),
		"acknowledgment_count": result.AckCount,
		"advance_result":       ar,
	})
}

func reopenWeekHandler(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weekNum, err := pathID(r, "week")
	if err != nil {
		jsonError(w, "invalid week", 400)
		return
	}

	if err := mgr.ReopenWeek(r.Context(), seasonID, weekNum); err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) {
			switch de.Category {
			case domainerr.NotFound:
				jsonError(w, de.Message, http.StatusNotFound)
			case domainerr.Conflict:
				jsonError(w, de.Message, http.StatusConflict)
			default:
				jsonError(w, de.Message, http.StatusInternalServerError)
			}
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, map[string]any{"reopened": true, "week_number": int(weekNum)})
}

func getWeekAcknowledgments(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weekNum, err := pathID(r, "week")
	if err != nil {
		jsonError(w, "invalid week", 400)
		return
	}

	acks, err := mgr.ListAcknowledgments(r.Context(), seasonID, weekNum)
	if err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.NotFound {
			jsonError(w, de.Message, http.StatusNotFound)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, acks)
}

func getAdvancePreview(w http.ResponseWriter, r *http.Request, mgr WeekManager) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	weekNum, err := pathID(r, "week")
	if err != nil {
		jsonError(w, "invalid week", 400)
		return
	}

	preview, err := mgr.AdvancePreview(r.Context(), seasonID, weekNum)
	if err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) && de.Category == domainerr.NotFound {
			jsonError(w, de.Message, http.StatusNotFound)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, preview)
}

// --- Handicap Review ---------------------------------------------------------

// getHandicapRecommendations handles GET /api/seasons/{id}/handicap-recommendations.
// Delegates to the handicaps.Service; translates domainerr.Category to HTTP status.
// No DB access in this handler; all logic lives in the service and adapter.
//
// Error mapping:
//   - domainerr.NotFound     -> 404 with the safe domain Message
//   - domainerr.InvalidInput -> 400 with the safe domain Message
//   - domainerr.Internal     -> 500 with the safe domain Message
//   - any non-domain error   -> 500 with fixed text "internal error" (no cause leak)
func getHandicapRecommendations(w http.ResponseWriter, r *http.Request, svc HandicapRecommender) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	resp, err := svc.Recommendations(r.Context(), seasonID)
	if err != nil {
		var de *domainerr.Err
		if errors.As(err, &de) {
			switch de.Category {
			case domainerr.NotFound:
				jsonError(w, de.Message, http.StatusNotFound)
			case domainerr.InvalidInput:
				jsonError(w, de.Message, http.StatusBadRequest)
			default:
				jsonError(w, de.Message, http.StatusInternalServerError)
			}
		} else {
			// Non-domain error: never expose the cause to the client.
			jsonError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	jsonOK(w, resp)
}

// --- Handicap Apply ----------------------------------------------------------

// applyEntryDTO is the handler-local JSON shape for one entry in an apply request.
// It uses pointer types so missing fields can be detected as nil.
// Never exported; conversion to handicaps.ApplyEntry happens in postHandicapApply.
type applyEntryDTO struct {
	PlayerID              *int64   `json:"player_id"`
	ExpectedAssignedHC    *float64 `json:"expected_assigned_hc"`
	ExpectedRecommendedHC *float64 `json:"expected_recommended_hc"`
	RecToken              *string  `json:"rec_token"`
}

// applyRequestDTO is the handler-local JSON shape for the apply request body.
type applyRequestDTO struct {
	ApplyRequestID *string         `json:"apply_request_id"`
	Entries        []applyEntryDTO `json:"entries"`
}

// isFiniteFloat mirrors isFiniteHC for handler-side validation of decoded floats.
func isFiniteFloat(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

// postHandicapApply handles POST /api/seasons/{id}/handicap-apply.
//
// Error mapping:
//   - domainerr.InvalidInput  -> 400
//   - domainerr.NotFound      -> 404
//   - domainerr.Conflict      -> 409
//   - domainerr.Unprocessable -> 422
//   - *ApplyConflictErr       -> 409
//   - *ApplyRejectionErr      -> 422
//   - domainerr.Internal      -> 500
func postHandicapApply(w http.ResponseWriter, r *http.Request, svc HandicapApplier) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	var dto applyRequestDTO
	if err := decode(r, &dto); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}

	// Validate required fields at the handler boundary.
	if dto.ApplyRequestID == nil {
		jsonError(w, "apply_request_id is required", 400)
		return
	}
	if dto.Entries == nil {
		jsonError(w, "entries is required", 400)
		return
	}

	entries := make([]handicaps.ApplyEntry, 0, len(dto.Entries))
	for i, e := range dto.Entries {
		if e.PlayerID == nil {
			jsonError(w, fmt.Sprintf("entry[%d]: player_id is required", i), 400)
			return
		}
		if e.ExpectedAssignedHC == nil {
			jsonError(w, fmt.Sprintf("entry[%d]: expected_assigned_hc is required", i), 400)
			return
		}
		if !isFiniteFloat(*e.ExpectedAssignedHC) {
			jsonError(w, fmt.Sprintf("entry[%d]: expected_assigned_hc must be finite", i), 400)
			return
		}
		if e.ExpectedRecommendedHC == nil {
			jsonError(w, fmt.Sprintf("entry[%d]: expected_recommended_hc is required", i), 400)
			return
		}
		if !isFiniteFloat(*e.ExpectedRecommendedHC) {
			jsonError(w, fmt.Sprintf("entry[%d]: expected_recommended_hc must be finite", i), 400)
			return
		}
		if e.RecToken == nil {
			jsonError(w, fmt.Sprintf("entry[%d]: rec_token is required", i), 400)
			return
		}
		entries = append(entries, handicaps.ApplyEntry{
			PlayerID:              *e.PlayerID,
			ExpectedAssignedHC:    *e.ExpectedAssignedHC,
			ExpectedRecommendedHC: *e.ExpectedRecommendedHC,
			RecToken:              *e.RecToken,
			AppliedByUserID:       applyUserIDFromContext(r.Context()),
		})
	}

	req := handicaps.ApplyRequest{
		ApplyRequestID: *dto.ApplyRequestID,
		Entries:        entries,
	}

	result, err := svc.Apply(r.Context(), seasonID, req)
	if err != nil {
		var conflictErr *handicaps.ApplyConflictErr
		var rejectionErr *handicaps.ApplyRejectionErr
		var de *domainerr.Err

		switch {
		case errors.As(err, &conflictErr):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":     "apply conflicts must be resolved before retrying",
				"conflicts": conflictErr.Conflicts,
			})
		case errors.As(err, &rejectionErr):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error":      "one or more players are not eligible for apply",
				"rejections": rejectionErr.Rejections,
			})
		case errors.As(err, &de):
			switch de.Category {
			case domainerr.NotFound:
				jsonError(w, de.Message, http.StatusNotFound)
			case domainerr.InvalidInput:
				jsonError(w, de.Message, http.StatusBadRequest)
			case domainerr.Conflict:
				jsonError(w, de.Message, http.StatusConflict)
			case domainerr.Unprocessable:
				jsonError(w, de.Message, http.StatusUnprocessableEntity)
			default:
				jsonError(w, de.Message, http.StatusInternalServerError)
			}
		default:
			jsonError(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	jsonOK(w, result)
}

// ─── Users ───────────────────────────────────────────────────────────────────

// postUser handles POST /api/users. Creates a new user and returns the
// one-time cleartext API key. Gated by requireAdminToken.
func postUser(w http.ResponseWriter, r *http.Request, auth ApplyAuthResolver) {
	var body struct {
		Username string `json:"username"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Username) == "" {
		jsonError(w, "username is required", http.StatusBadRequest)
		return
	}

	user, key, err := auth.CreateApplyUser(r.Context(), body.Username)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			jsonError(w, "username already exists", http.StatusConflict)
			return
		}
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(models.CreateUserResponse{User: user, APIKey: key})
}

// listUsers handles GET /api/users. Returns all users without API key hashes.
// Gated by requireAdminToken.
func listUsers(w http.ResponseWriter, r *http.Request, auth ApplyAuthResolver) {
	users, err := auth.ListApplyUsers(r.Context())
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if users == nil {
		users = []models.User{}
	}
	jsonOK(w, users)
}

// ─── Standings ────────────────────────────────────────────────────────────────

func getStandings(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	sid, ok := qparamInt(r, "season_id")
	if !ok {
		leagueID, lok := qparamInt(r, "league_id")
		if !lok {
			jsonOK(w, []models.Standing{})
			return
		}
		if err := db.DB.QueryRow(
			`SELECT id FROM seasons WHERE league_id=? AND active=1 LIMIT 1`, leagueID,
		).Scan(&sid); err != nil {
			jsonOK(w, []models.Standing{})
			return
		}
	}
	standings, err := mgr.GetStandings(r.Context(), sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, standings)
}


// ─── 8-Ball Round Results ─────────────────────────────────────────────────────

func getRounds(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	rounds, err := mgr.GetRounds(r.Context(), id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, rounds)
}

func saveRounds(w http.ResponseWriter, r *http.Request, mgr RoundManager) {
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
	// Roster eligibility guard: cross-domain pre-TX check kept in handler per B3 decision.
	if ok, msg := seasons.RosterEligible(db.DB, matchID, 3); !ok {
		jsonError(w, msg, http.StatusUnprocessableEntity)
		return
	}
	err = mgr.SaveRounds(r.Context(), matches.SaveRoundsInput{MatchID: matchID, Rounds: req.Rounds})
	if err != nil {
		var vErr *matches.RoundValidationError
		if errors.As(err, &vErr) {
			jsonValidation(w, vErr.Result.Result)
			return
		}
		var de *domainerr.Err
		if errors.As(err, &de) {
			switch de.Category {
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
		return
	}
	jsonOK(w, map[string]any{"saved": len(req.Rounds)})
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

// ─── Lineup Plans ─────────────────────────────────────────────────────────────

func listLineupPlans(w http.ResponseWriter, r *http.Request, mgr LineupManager) {
	seasonID, hasSeason := qparamInt(r, "season_id")
	if !hasSeason {
		jsonError(w, "season_id required", 400)
		return
	}
	req := matches.ListLineupPlansRequest{SeasonID: seasonID}
	if v, ok := qparamInt(r, "week_number"); ok {
		req.WeekNumber = v
	}
	if v, ok := qparamInt(r, "team_id"); ok {
		req.TeamID = v
	}
	plans, err := mgr.ListLineupPlans(r.Context(), req)
	if err != nil {
		mapLineupErr(w, err)
		return
	}
	jsonOK(w, plans)
}

// saveTeamLineup atomically replaces all lineup slots for one team/week.
// Body: { season_id, team_id, week_number, player_ids: [id1, id2, id3] }
func saveTeamLineup(w http.ResponseWriter, r *http.Request, mgr LineupManager) {
	var req models.SaveTeamLineupRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if req.SeasonID == 0 || req.TeamID == 0 {
		jsonError(w, "season_id and team_id required", 400)
		return
	}
	if err := mgr.SaveTeamLineup(r.Context(), matches.SaveLineupRequest{
		SeasonID:   req.SeasonID,
		TeamID:     req.TeamID,
		WeekNumber: int64(req.WeekNumber),
		PlayerIDs:  req.PlayerIDs,
	}); err != nil {
		mapLineupErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "saved"})
}

func deleteLineupPlan(w http.ResponseWriter, r *http.Request, mgr LineupManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if err := mgr.DeleteLineupPlan(r.Context(), id); err != nil {
		mapLineupErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "deleted"})
}

// mapLineupErr translates lineup domain errors to HTTP responses.
func mapLineupErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	if errors.As(err, &de) {
		switch de.Category {
		case domainerr.NotFound:
			jsonError(w, de.Message, http.StatusNotFound)
		case domainerr.InvalidInput:
			jsonError(w, de.Message, http.StatusBadRequest)
		default:
			jsonError(w, de.Message, http.StatusInternalServerError)
		}
		return
	}
	jsonError(w, err.Error(), http.StatusInternalServerError)
}

// ─── Season Teams ──────────────────────────────────────────────────────────────

// seasonTeamRow scans one row from season_teams into a SeasonTeam.
const seasonTeamSelect = `
	SELECT st.id, st.season_id, st.team_id, t.name,
	       COALESCE(t.team_number,''),
	       CASE WHEN st.season_name != '' THEN st.season_name ELSE t.name END,
	       st.captain_id,
	       COALESCE(cp.first_name||' '||cp.last_name, ''),
	       (SELECT COUNT(*) FROM season_rosters sr
	        WHERE sr.season_id = st.season_id AND sr.team_id = st.team_id)
	FROM season_teams st
	JOIN teams t ON t.id = st.team_id
	LEFT JOIN players cp ON cp.id = st.captain_id`

func scanSeasonTeam(row interface{ Scan(...any) error }) (models.SeasonTeam, error) {
	var st models.SeasonTeam
	err := row.Scan(&st.ID, &st.SeasonID, &st.TeamID, &st.TeamName, &st.TeamNumber,
		&st.SeasonName, &st.CaptainID, &st.CaptainName, &st.RosterCount)
	return st, err
}

func listSeasonTeams(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	rows, err := db.DB.Query(seasonTeamSelect+` WHERE st.season_id=? ORDER BY st.id`, sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var out []models.SeasonTeam
	for rows.Next() {
		if st, err := scanSeasonTeam(rows); err == nil {
			out = append(out, st)
		}
	}
	if out == nil {
		out = []models.SeasonTeam{}
	}
	jsonOK(w, out)
}

func addSeasonTeam(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var req seasons.AddTeamRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	st, err := mgr.AddTeam(r.Context(), sid, req)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, st)
}

func updateSeasonTeam(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	var req seasons.UpdateTeamRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	st, err := mgr.UpdateTeam(r.Context(), sid, tid, req)
	if err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, st)
}

func removeSeasonTeam(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	if err := mgr.RemoveTeam(r.Context(), sid, tid); err != nil {
		mapSeasonErr(w, err)
		return
	}
	jsonOK(w, map[string]string{"status": "removed"})
}

// mapSeasonErr translates seasons domain errors to HTTP responses.
func mapSeasonErr(w http.ResponseWriter, err error) {
	var de *domainerr.Err
	switch {
	case errors.Is(err, seasons.ErrNotFound):
		jsonError(w, "season not found", http.StatusNotFound)
	case errors.As(err, &de):
		switch de.Category {
		case domainerr.NotFound:
			jsonError(w, de.Message, http.StatusNotFound)
		case domainerr.InvalidInput:
			jsonError(w, de.Message, http.StatusBadRequest)
		case domainerr.Unprocessable:
			jsonError(w, de.Message, http.StatusUnprocessableEntity)
		default:
			jsonError(w, de.Message, http.StatusInternalServerError)
		}
	default:
		jsonError(w, err.Error(), http.StatusInternalServerError)
	}
}

// ── Season Rosters ─────────────────────────────────────────────────────────────

func listSeasonRoster(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	rows, err := db.DB.Query(`
		SELECT sr.id, sr.season_id, sr.team_id, t.name,
		       sr.player_id, p.first_name||' '||p.last_name,
		       COALESCE(p.player_number,''), p.handicap
		FROM season_rosters sr
		JOIN teams t ON t.id = sr.team_id
		JOIN players p ON p.id = sr.player_id
		WHERE sr.season_id=? AND sr.team_id=?
		ORDER BY p.last_name, p.first_name`, sid, tid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var out []models.SeasonRosterEntry
	for rows.Next() {
		var e models.SeasonRosterEntry
		rows.Scan(&e.ID, &e.SeasonID, &e.TeamID, &e.TeamName, &e.PlayerID, &e.PlayerName, &e.PlayerNumber, &e.Handicap)
		out = append(out, e)
	}
	if out == nil {
		out = []models.SeasonRosterEntry{}
	}
	jsonOK(w, out)
}

type addRosterPlayerRequest struct {
	PlayerID int64 `json:"player_id"`
}

func addRosterPlayer(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	draft, err := mgr.IsDraft(r.Context(), sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if !draft {
		jsonError(w, "cannot modify rosters in an active season", 422)
		return
	}

	var req addRosterPlayerRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if req.PlayerID == 0 {
		jsonError(w, "player_id is required", 400)
		return
	}

	// Verify team is in this season.
	var stID int64
	if err := db.DB.QueryRow(
		`SELECT id FROM season_teams WHERE season_id=? AND team_id=?`, sid, tid,
	).Scan(&stID); err != nil {
		jsonError(w, "team is not in this season", 400)
		return
	}

	// Enforce one team per player per season.
	var existingTeam int64
	db.DB.QueryRow(
		`SELECT COALESCE(team_id,0) FROM season_rosters WHERE season_id=? AND player_id=?`,
		sid, req.PlayerID).Scan(&existingTeam)
	if existingTeam != 0 && existingTeam != tid {
		jsonError(w, "player is already on another team in this season", 400)
		return
	}

	res, err := db.DB.Exec(
		`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
		sid, tid, req.PlayerID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	rID, _ := res.LastInsertId()

	var entry models.SeasonRosterEntry
	db.DB.QueryRow(`
		SELECT sr.id, sr.season_id, sr.team_id, t.name,
		       sr.player_id, p.first_name||' '||p.last_name,
		       COALESCE(p.player_number,''), p.handicap
		FROM season_rosters sr
		JOIN teams t ON t.id = sr.team_id
		JOIN players p ON p.id = sr.player_id
		WHERE sr.id=?`, rID,
	).Scan(&entry.ID, &entry.SeasonID, &entry.TeamID, &entry.TeamName,
		&entry.PlayerID, &entry.PlayerName, &entry.PlayerNumber, &entry.Handicap)

	w.WriteHeader(http.StatusCreated)
	jsonOK(w, entry)
}

func removeRosterPlayer(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	tid, err := pathID(r, "tid")
	if err != nil {
		jsonError(w, "invalid team id", 400)
		return
	}
	pid, err := pathID(r, "pid")
	if err != nil {
		jsonError(w, "invalid player id", 400)
		return
	}
	draft, err := mgr.IsDraft(r.Context(), sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if !draft {
		jsonError(w, "cannot modify rosters in an active season", 422)
		return
	}
	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	res, err := tx.Exec(
		`DELETE FROM season_rosters WHERE season_id=? AND team_id=? AND player_id=?`, sid, tid, pid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "roster entry not found", 404)
		return
	}
	// Clear captain_id when the removed player is the team's current captain.
	// The UPDATE is a no-op when the removed player is not the captain.
	if _, err = tx.Exec(
		`UPDATE season_teams SET captain_id=NULL
		 WHERE season_id=? AND team_id=? AND captain_id=?`, sid, tid, pid); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if err = tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "removed"})
}

// ── Available Players ──────────────────────────────────────────────────────────

func listAvailablePlayers(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	// Resolve the league so we know which players to search.
	var leagueID int64
	if err := db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID); err != nil {
		jsonError(w, "season not found", 404)
		return
	}

	// Return all active system players not already rostered in this
	// season — including players with no team or players from other leagues.
	rows, err := db.DB.Query(`
		SELECT p.id, p.player_number, p.first_name, p.last_name,
		       p.first_name||' '||p.last_name,
		       COALESCE(p.phone,''), COALESCE(p.email,''),
		       p.team_id, COALESCE(t.name,''), COALESCE(t.league_id,0),
		       p.handicap, p.admin_hold, COALESCE(p.active,1), COALESCE(p.note,''),
		       p.created_at
		FROM players p
		LEFT JOIN teams t ON t.id = p.team_id
		WHERE p.id NOT IN (
		        SELECT player_id FROM season_rosters WHERE season_id=?
		      )
		  AND COALESCE(p.active,1) = 1
		ORDER BY p.last_name, p.first_name`, sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var players []models.Player
	for rows.Next() {
		var p models.Player
		var adminHold, activeInt int
		rows.Scan(&p.ID, &p.PlayerNumber, &p.FirstName, &p.LastName, &p.Name,
			&p.Phone, &p.Email, &p.TeamID, &p.TeamName, &p.LeagueID,
			&p.Handicap, &adminHold, &activeInt, &p.Note, &p.CreatedAt)
		p.AdminHold = adminHold == 1
		p.Active = activeInt == 1
		players = append(players, p)
	}
	if players == nil {
		players = []models.Player{}
	}
	jsonOK(w, players)
}

// ── Previous Season ────────────────────────────────────────────────────────────

func getPreviousSeasonTeams(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	result, err := mgr.PreviousSeason(r.Context(), sid)
	if err != nil {
		if errors.Is(err, seasons.ErrNotFound) {
			jsonError(w, "season not found", 404)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, result)
}

// ── Setup Checklist ────────────────────────────────────────────────────────────

func getSeasonChecklist(w http.ResponseWriter, r *http.Request, mgr SeasonManager) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	c, err := mgr.Checklist(r.Context(), sid)
	if err != nil {
		if errors.Is(err, seasons.ErrNotFound) {
			jsonError(w, "season not found", 404)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, c)
}
