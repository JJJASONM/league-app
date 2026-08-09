package handlers

import "net/http"

// registerSeasonSetupRoutes mounts season CRUD, activation, rules,
// skipped-weeks, bye-requests, and season team/roster routes onto mux.
// GET reads are unprotected; mutations are gated by clearanceAuth.
func registerSeasonSetupRoutes(mux *http.ServeMux, seasonMgr SeasonManager, ruleMgr RuleManager, applyAuth ApplyAuthResolver) {
	mux.HandleFunc("GET /api/seasons", func(w http.ResponseWriter, r *http.Request) {
		listSeasons(w, r, seasonMgr)
	})
	mux.HandleFunc("POST /api/seasons",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			createSeason(w, r, seasonMgr)
		}),
	)
	mux.HandleFunc("GET /api/seasons/{id}", func(w http.ResponseWriter, r *http.Request) {
		getSeason(w, r, seasonMgr)
	})
	mux.HandleFunc("PUT /api/seasons/{id}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			updateSeason(w, r, seasonMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/seasons/{id}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			deleteSeason(w, r, seasonMgr)
		}),
	)
	mux.HandleFunc("POST /api/seasons/{id}/activate",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			activateSeason(w, r, seasonMgr)
		}),
	)

	mux.HandleFunc("GET /api/seasons/{id}/rules", func(w http.ResponseWriter, r *http.Request) {
		listSeasonRules(w, r, ruleMgr)
	})
	mux.HandleFunc("POST /api/seasons/{id}/rules",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			createSeasonRule(w, r, ruleMgr)
		}),
	)
	mux.HandleFunc("PUT /api/seasons/{id}/rules/{rid}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			updateSeasonRule(w, r, ruleMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/seasons/{id}/rules/{rid}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			deleteSeasonRule(w, r, ruleMgr)
		}),
	)

	mux.HandleFunc("GET /api/seasons/{id}/skipped-weeks", func(w http.ResponseWriter, r *http.Request) {
		listSkippedWeeks(w, r, seasonMgr)
	})
	mux.HandleFunc("POST /api/seasons/{id}/skipped-weeks",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			createSkippedWeek(w, r, seasonMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/seasons/{id}/skipped-weeks/{sid}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			deleteSkippedWeek(w, r, seasonMgr)
		}),
	)

	mux.HandleFunc("GET /api/seasons/{id}/bye-requests", func(w http.ResponseWriter, r *http.Request) {
		listByeRequests(w, r, seasonMgr)
	})
	mux.HandleFunc("POST /api/seasons/{id}/bye-requests",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			createByeRequest(w, r, seasonMgr)
		}),
	)
	mux.HandleFunc("PUT /api/seasons/{id}/bye-requests/{bid}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			updateByeRequest(w, r, seasonMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/seasons/{id}/bye-requests/{bid}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			deleteByeRequest(w, r, seasonMgr)
		}),
	)

	mux.HandleFunc("GET /api/seasons/{id}/teams", func(w http.ResponseWriter, r *http.Request) {
		listSeasonTeams(w, r, seasonMgr)
	})
	mux.HandleFunc("POST /api/seasons/{id}/teams",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			addSeasonTeam(w, r, seasonMgr)
		}),
	)
	mux.HandleFunc("GET /api/seasons/{id}/previous", func(w http.ResponseWriter, r *http.Request) {
		getPreviousSeasonTeams(w, r, seasonMgr)
	})
	mux.HandleFunc("PUT /api/seasons/{id}/teams/{tid}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			updateSeasonTeam(w, r, seasonMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/seasons/{id}/teams/{tid}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			removeSeasonTeam(w, r, seasonMgr)
		}),
	)
	mux.HandleFunc("GET /api/seasons/{id}/teams/{tid}/roster", func(w http.ResponseWriter, r *http.Request) {
		listSeasonRoster(w, r, seasonMgr)
	})
	mux.HandleFunc("POST /api/seasons/{id}/teams/{tid}/roster",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			addRosterPlayer(w, r, seasonMgr)
		}),
	)
	mux.HandleFunc("DELETE /api/seasons/{id}/teams/{tid}/roster/{pid}",
		clearanceAuth(applyAuth, func(w http.ResponseWriter, r *http.Request) {
			removeRosterPlayer(w, r, seasonMgr)
		}),
	)
	mux.HandleFunc("GET /api/seasons/{id}/players/available", func(w http.ResponseWriter, r *http.Request) {
		listAvailablePlayers(w, r, seasonMgr)
	})
	mux.HandleFunc("GET /api/seasons/{id}/checklist", func(w http.ResponseWriter, r *http.Request) {
		getSeasonChecklist(w, r, seasonMgr)
	})
}
