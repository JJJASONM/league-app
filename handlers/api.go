// Package handlers wires HTTP routes to database operations.
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"league_app/backend/domains/matches"
	"league_app/backend/domains/rules"
	"league_app/backend/domains/seasons"
	"league_app/backend/validation"
	"league_app/db"
	"league_app/logic"
	"league_app/models"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Register mounts all API routes onto mux.
func Register(mux *http.ServeMux, dataDir string) {
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
	mux.HandleFunc("POST /api/seasons/{id}/activate", activateSeason)

	// Season sub-resources
	mux.HandleFunc("GET /api/seasons/{id}/rules", listSeasonRules)
	mux.HandleFunc("POST /api/seasons/{id}/rules", createSeasonRule)
	mux.HandleFunc("PUT /api/seasons/{id}/rules/{rid}", updateSeasonRule)
	mux.HandleFunc("DELETE /api/seasons/{id}/rules/{rid}", deleteSeasonRule)

	mux.HandleFunc("GET /api/seasons/{id}/skipped-weeks", listSkippedWeeks)
	mux.HandleFunc("POST /api/seasons/{id}/skipped-weeks", createSkippedWeek)
	mux.HandleFunc("DELETE /api/seasons/{id}/skipped-weeks/{sid}", deleteSkippedWeek)

	mux.HandleFunc("GET /api/seasons/{id}/bye-requests", listByeRequests)
	mux.HandleFunc("POST /api/seasons/{id}/bye-requests", createByeRequest)
	mux.HandleFunc("PUT /api/seasons/{id}/bye-requests/{bid}", updateByeRequest)
	mux.HandleFunc("DELETE /api/seasons/{id}/bye-requests/{bid}", deleteByeRequest)

	// Season teams and rosters
	mux.HandleFunc("GET /api/seasons/{id}/teams", listSeasonTeams)
	mux.HandleFunc("POST /api/seasons/{id}/teams", addSeasonTeam)
	mux.HandleFunc("GET /api/seasons/{id}/previous", getPreviousSeasonTeams)
	mux.HandleFunc("PUT /api/seasons/{id}/teams/{tid}", updateSeasonTeam)
	mux.HandleFunc("DELETE /api/seasons/{id}/teams/{tid}", removeSeasonTeam)
	mux.HandleFunc("GET /api/seasons/{id}/teams/{tid}/roster", listSeasonRoster)
	mux.HandleFunc("POST /api/seasons/{id}/teams/{tid}/roster", addRosterPlayer)
	mux.HandleFunc("DELETE /api/seasons/{id}/teams/{tid}/roster/{pid}", removeRosterPlayer)
	mux.HandleFunc("GET /api/seasons/{id}/players/available", listAvailablePlayers)
	mux.HandleFunc("GET /api/seasons/{id}/checklist", getSeasonChecklist)

	// Matches — scoped to ?season_id= (season implies league)
	mux.HandleFunc("GET /api/matches", listMatches)
	mux.HandleFunc("POST /api/matches/generate", generateSchedule)
	mux.HandleFunc("GET /api/matches/{id}", getMatch)
	mux.HandleFunc("PATCH /api/matches/{id}/assign", assignMatchTeams)
	mux.HandleFunc("POST /api/matches/{id}/results", submitResults)
	mux.HandleFunc("DELETE /api/matches/{id}/results", clearResults)
	mux.HandleFunc("GET /api/matches/{id}/rounds", getRounds)
	mux.HandleFunc("POST /api/matches/{id}/rounds", saveRounds)

	// Lineup plans — pre-game slot assignments per team/week
	mux.HandleFunc("GET /api/lineup-plans", listLineupPlans)
	mux.HandleFunc("POST /api/lineup-plans", saveTeamLineup)
	mux.HandleFunc("DELETE /api/lineup-plans/{id}", deleteLineupPlan)

	// Rule definitions — developer-owned, served by the backend
	mux.HandleFunc("GET /api/rules/definitions", listRuleDefinitions)

	// Week workflow -- Close Week gate
	mux.HandleFunc("GET /api/seasons/{id}/weeks", listWeeks)
	mux.HandleFunc("GET /api/seasons/{id}/weeks/{week}/validate", validateWeekHandler)
	mux.HandleFunc("POST /api/seasons/{id}/weeks/{week}/close", closeWeekHandler)
	mux.HandleFunc("POST /api/seasons/{id}/weeks/{week}/reopen", reopenWeekHandler)
	mux.HandleFunc("GET /api/seasons/{id}/weeks/{week}/acknowledgments", getWeekAcknowledgments)
	mux.HandleFunc("GET /api/seasons/{id}/weeks/{week}/advance-preview", getAdvancePreview)
	mux.HandleFunc("GET /api/seasons/{id}/handicap-recommendations", getHandicapRecommendations)

	// Standings & stats
	mux.HandleFunc("GET /api/standings", getStandings)
	mux.HandleFunc("GET /api/player-stats", getPlayerStats)

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

func activateSeason(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	// Only deactivate seasons within the same league
	var leagueID int64
	if err := db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, id).Scan(&leagueID); err != nil {
		jsonError(w, "season not found", 404)
		return
	}

	// Enforce checklist blockers when season_teams is in use.
	checklist, clErr := seasons.Checklist(db.DB, id)
	if clErr != nil {
		jsonError(w, clErr.Error(), 500)
		return
	}
	if !checklist.CanActivate {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":    "season cannot be activated; resolve all blockers first",
			"blockers": checklist.Blockers,
		})
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()
	tx.Exec(`UPDATE seasons SET active=0 WHERE league_id=?`, leagueID)
	// Set activated_at once on first activation (persistent setup lock).
	tx.Exec(`UPDATE seasons SET active=1, activated_at=COALESCE(activated_at, CURRENT_TIMESTAMP) WHERE id=?`, id)
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "activated"})
}

// ─── Matches ─────────────────────────────────────────────────────────────────

// matchSelectCols is the standard column list for match queries.
// Uses LEFT JOIN so unassigned (blanket) slots with NULL team IDs are included.
const matchSelect = `
	SELECT m.id, m.season_id,
	       COALESCE(m.home_team_id,0), COALESCE(ht.name,'(unassigned)'),
	       COALESCE(m.away_team_id,0), COALESCE(at.name,'(unassigned)'),
	       m.match_date, m.week_number, m.completed, m.created_at
	FROM matches m
	LEFT JOIN teams ht ON ht.id = m.home_team_id
	LEFT JOIN teams at ON at.id = m.away_team_id`

func listMatches(w http.ResponseWriter, r *http.Request) {
	seasonID := qparam(r, "season_id")
	leagueID := qparam(r, "league_id")

	var rows *sql.Rows
	var err error
	switch {
	case seasonID != "":
		rows, err = db.DB.Query(matchSelect+` WHERE m.season_id=? ORDER BY m.week_number, m.id`, seasonID)
	case leagueID != "":
		rows, err = db.DB.Query(matchSelect+`
			JOIN seasons s ON s.id = m.season_id
			WHERE s.league_id=? ORDER BY m.week_number, m.id`, leagueID)
	default:
		rows, err = db.DB.Query(matchSelect + ` ORDER BY m.week_number, m.id`)
	}
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var matches []models.Match
	for rows.Next() {
		var m models.Match
		var completed int
		rows.Scan(&m.ID, &m.SeasonID, &m.HomeTeamID, &m.HomeTeamName,
			&m.AwayTeamID, &m.AwayTeamName, &m.MatchDate, &m.WeekNumber, &completed, &m.CreatedAt)
		m.Completed = completed == 1
		m.MatchDate = normDatePtr(m.MatchDate)
		matches = append(matches, m)
	}
	if matches == nil {
		matches = []models.Match{}
	}
	jsonOK(w, matches)
}

func generateSchedule(w http.ResponseWriter, r *http.Request) {
	var req models.GenerateScheduleRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if req.ScheduleType == "" {
		req.ScheduleType = "double_rr"
	}

	// Parse start date
	startDate, _ := time.Parse("2006-01-02", req.StartDate)

	// Parse skip dates — accept YYYY-MM-DD or ISO timestamp (driver may return either)
	var skipDates []time.Time
	for _, ds := range req.SkipDates {
		ds = normDateStr(ds)
		if t, err := time.Parse("2006-01-02", ds); err == nil {
			skipDates = append(skipDates, t)
		}
	}
	// Load approved bye requests (specific week only) to influence team rotation.
	byeByWeek := make(map[int]int64)
	byeRows, _ := db.DB.Query(
		`SELECT team_id, week_number FROM bye_requests WHERE season_id=? AND approved=1 AND week_number > 0`,
		req.SeasonID)
	if byeRows != nil {
		for byeRows.Next() {
			var tid int64
			var wn int
			byeRows.Scan(&tid, &wn)
			byeByWeek[wn] = tid
		}
		byeRows.Close()
	}

	opts := logic.ScheduleOptions{
		StartDate: startDate,
		SkipDates: skipDates,
		NumWeeks:  req.NumWeeks,
		ByeByWeek: byeByWeek,
	}

	var entries []logic.ScheduleEntry
	var genErr error

	if req.ScheduleType == "blanket" {
		// Blank template — no teams assigned yet
		mpw := req.MatchesPerWeek
		if mpw < 1 {
			mpw = 1
		}
		entries, genErr = logic.BlanketTemplate(req.NumWeeks, mpw, opts)
	} else {
		// Collect team IDs
		var teamIDs []int64
		if req.FromSeasonID > 0 {
			// Prior-season schedule inference is legacy-only; managed seasons always use season_teams.
			var genTeamsManaged int
			db.DB.QueryRow(`SELECT COALESCE(teams_managed,0) FROM seasons WHERE id=?`, req.SeasonID).Scan(&genTeamsManaged)
			if genTeamsManaged == 1 {
				jsonError(w, "managed seasons generate from season_teams; from_season_id is not supported", 400)
				return
			}
			// Legacy: use teams that appeared in a prior season's schedule.
			rows, err := db.DB.Query(`
				SELECT DISTINCT home_team_id FROM matches WHERE season_id=? AND home_team_id IS NOT NULL
				UNION
				SELECT DISTINCT away_team_id FROM matches WHERE season_id=? AND away_team_id IS NOT NULL`,
				req.FromSeasonID, req.FromSeasonID)
			if err != nil {
				jsonError(w, err.Error(), 500)
				return
			}
			for rows.Next() {
				var id int64
				rows.Scan(&id)
				teamIDs = append(teamIDs, id)
			}
			rows.Close()
		} else {
			// Managed seasons always use season_teams; legacy seasons fall back to all league teams.
			var teamsManaged int
			db.DB.QueryRow(`SELECT COALESCE(teams_managed,0) FROM seasons WHERE id=?`, req.SeasonID).Scan(&teamsManaged)
			var stRows *sql.Rows
			var stErr error
			var stCount int
			db.DB.QueryRow(`SELECT COUNT(*) FROM season_teams WHERE season_id=?`, req.SeasonID).Scan(&stCount)
			if teamsManaged == 1 {
				if stCount == 0 {
					jsonError(w, "no teams registered in this season; add teams before generating a schedule", 400)
					return
				}
				stRows, stErr = db.DB.Query(
					`SELECT team_id FROM season_teams WHERE season_id=? ORDER BY id`, req.SeasonID)
			} else if stCount > 0 {
				stRows, stErr = db.DB.Query(
					`SELECT team_id FROM season_teams WHERE season_id=? ORDER BY id`, req.SeasonID)
			} else {
				stRows, stErr = db.DB.Query(`
					SELECT t.id FROM teams t
					JOIN seasons s ON s.league_id = t.league_id
					WHERE s.id=? ORDER BY t.id`, req.SeasonID)
			}
			if stErr != nil {
				jsonError(w, stErr.Error(), 500)
				return
			}
			for stRows.Next() {
				var id int64
				stRows.Scan(&id)
				teamIDs = append(teamIDs, id)
			}
			stRows.Close()
		}
		if len(teamIDs) < 2 {
			jsonError(w, "need at least 2 teams in this league to generate a schedule", 400)
			return
		}

		switch req.ScheduleType {
		case "single_rr":
			entries, genErr = logic.SingleRoundRobin(teamIDs, opts)
		case "split":
			entries, genErr = logic.SplitSeason(teamIDs, opts)
		case "custom":
			if req.NumWeeks < 1 {
				jsonError(w, "num_weeks is required for custom schedule", 400)
				return
			}
			entries, genErr = logic.CustomSchedule(teamIDs, opts)
		default: // "double_rr"
			entries, genErr = logic.DoubleRoundRobin(teamIDs, opts)
		}
	}

	if genErr != nil {
		jsonError(w, genErr.Error(), 400)
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	// Delete only unplayed matches so completed results are preserved
	tx.Exec(`DELETE FROM matches WHERE season_id=? AND completed=0`, req.SeasonID)

	stmt, err := tx.Prepare(
		`INSERT INTO matches (season_id, home_team_id, away_team_id, match_date, week_number) VALUES (?,?,?,?,?)`)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer stmt.Close()

	var lastDate string
	for _, e := range entries {
		var hid, aid any
		if e.HomeTeamID != 0 {
			hid = e.HomeTeamID
		}
		if e.AwayTeamID != 0 {
			aid = e.AwayTeamID
		}
		if _, err := stmt.Exec(req.SeasonID, hid, aid, nullStr(e.MatchDate), e.WeekNumber); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if e.MatchDate > lastDate {
			lastDate = e.MatchDate
		}
	}

	// Update season: schedule_type, num_weeks, end_date; reset stale flag
	tx.Exec(`UPDATE seasons SET schedule_type=?, num_weeks=?, end_date=?, schedule_stale=0 WHERE id=?`,
		req.ScheduleType, req.NumWeeks, nullStr(lastDate), req.SeasonID)

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]any{
		"matches_created": len(entries),
		"end_date":        lastDate,
	})
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// assignMatchTeams assigns home/away teams to a blanket (unassigned) match slot.
func assignMatchTeams(w http.ResponseWriter, r *http.Request) {
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
	_, err = db.DB.Exec(`UPDATE matches SET home_team_id=?, away_team_id=? WHERE id=?`,
		req.HomeTeamID, req.AwayTeamID, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "assigned"})
}

// ─── Rule Definitions ─────────────────────────────────────────────────────────

func listRuleDefinitions(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, rules.Definitions())
}

// ─── Season Rules ─────────────────────────────────────────────────────────────

func listSeasonRules(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	rows, err := db.DB.Query(
		`SELECT id, season_id, rule_key, rule_label, rule_value FROM season_rules WHERE season_id=? ORDER BY id`,
		sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var rules []models.SeasonRule
	for rows.Next() {
		var ru models.SeasonRule
		rows.Scan(&ru.ID, &ru.SeasonID, &ru.RuleKey, &ru.RuleLabel, &ru.RuleValue)
		rules = append(rules, ru)
	}
	if rules == nil {
		rules = []models.SeasonRule{}
	}
	jsonOK(w, rules)
}

func createSeasonRule(w http.ResponseWriter, r *http.Request) {
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
	if ru.RuleKey == "" {
		ru.RuleKey = fmt.Sprintf("rule_%d", time.Now().UnixMilli())
	}
	if err := rules.ValidateValue(ru.RuleKey, ru.RuleValue); err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}
	res, err := db.DB.Exec(
		`INSERT OR REPLACE INTO season_rules (season_id, rule_key, rule_label, rule_value) VALUES (?,?,?,?)`,
		ru.SeasonID, ru.RuleKey, ru.RuleLabel, ru.RuleValue)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	ru.ID, _ = res.LastInsertId()
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, ru)
}

func updateSeasonRule(w http.ResponseWriter, r *http.Request) {
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
	var ruleKey string
	db.DB.QueryRow(`SELECT rule_key FROM season_rules WHERE id=?`, rid).Scan(&ruleKey)
	if ruleKey != "" {
		if verr := rules.ValidateValue(ruleKey, ru.RuleValue); verr != nil {
			jsonError(w, verr.Error(), http.StatusBadRequest)
			return
		}
	}
	db.DB.Exec(`UPDATE season_rules SET rule_label=?, rule_value=? WHERE id=?`,
		ru.RuleLabel, ru.RuleValue, rid)
	ru.ID = rid
	jsonOK(w, ru)
}

func deleteSeasonRule(w http.ResponseWriter, r *http.Request) {
	rid, err := pathID(r, "rid")
	if err != nil {
		jsonError(w, "invalid rule id", 400)
		return
	}
	db.DB.Exec(`DELETE FROM season_rules WHERE id=?`, rid)
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

func createByeRequest(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var b models.ByeRequest
	if err := decode(r, &b); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	b.SeasonID = sid

	// Resolve the league for this season.
	var leagueID int64
	if err := db.DB.QueryRow(`SELECT league_id FROM seasons WHERE id=?`, sid).Scan(&leagueID); err != nil {
		jsonError(w, "season not found", 404)
		return
	}

	// Bye requests only make sense for seasons with an odd number of participating teams.
	// Managed seasons use season_teams count exclusively; legacy seasons fall back to all league teams.
	var byeTeamsManaged int
	db.DB.QueryRow(`SELECT COALESCE(teams_managed,0) FROM seasons WHERE id=?`, sid).Scan(&byeTeamsManaged)
	var teamCount int
	var stCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM season_teams WHERE season_id=?`, sid).Scan(&stCount)
	if byeTeamsManaged == 1 {
		teamCount = stCount
	} else if stCount > 0 {
		teamCount = stCount
	} else {
		db.DB.QueryRow(`SELECT COUNT(*) FROM teams WHERE league_id=?`, leagueID).Scan(&teamCount)
	}
	if teamCount%2 == 0 {
		jsonError(w, fmt.Sprintf("bye requests require an odd number of teams (%d teams — even)", teamCount), 400)
		return
	}

	// Ensure the requested team belongs to this season's league.
	var teamLeagueID int64
	if err := db.DB.QueryRow(`SELECT league_id FROM teams WHERE id=?`, b.TeamID).Scan(&teamLeagueID); err != nil || teamLeagueID != leagueID {
		jsonError(w, "team does not belong to this season's league", 400)
		return
	}

	// For managed seasons, the team must also be registered in season_teams.
	if byeTeamsManaged == 1 {
		var inSeason int
		db.DB.QueryRow(`SELECT COUNT(*) FROM season_teams WHERE season_id=? AND team_id=?`, sid, b.TeamID).Scan(&inSeason)
		if inSeason == 0 {
			jsonError(w, "team is not registered in this season", 400)
			return
		}
	}

	// Reject duplicates with a clear message instead of silently ignoring.
	var dup int
	db.DB.QueryRow(`SELECT COUNT(*) FROM bye_requests WHERE season_id=? AND team_id=? AND week_number=?`,
		sid, b.TeamID, b.WeekNumber).Scan(&dup)
	if dup > 0 {
		jsonError(w, "a bye request already exists for this team and week", 400)
		return
	}

	res, err := db.DB.Exec(
		`INSERT INTO bye_requests (season_id, team_id, week_number, reason) VALUES (?,?,?,?)`,
		sid, b.TeamID, b.WeekNumber, b.Reason)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	b.ID, _ = res.LastInsertId()
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, b)
}

func updateByeRequest(w http.ResponseWriter, r *http.Request) {
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
	var b models.ByeRequest
	if err := decode(r, &b); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}

	// Load stored week_number to validate before changing approved status.
	// Also enforces season scope — returns 404 if the request belongs to another season.
	var weekNum int
	switch err := db.DB.QueryRow(
		`SELECT week_number FROM bye_requests WHERE id=? AND season_id=?`, bid, sid).Scan(&weekNum); err {
	case nil:
	case sql.ErrNoRows:
		jsonError(w, "bye request not found", 404)
		return
	default:
		jsonError(w, err.Error(), 500)
		return
	}

	approved := 0
	if b.Approved {
		approved = 1
	}

	if approved == 1 {
		// Week 0 (TBD) requests cannot be approved — a specific week is required.
		if weekNum == 0 {
			jsonError(w, "cannot approve a TBD (week 0) request; set a specific week first", 400)
			return
		}
		// Only one approved bye per season+week to match the single natural bye slot.
		var conflict int
		db.DB.QueryRow(
			`SELECT COUNT(*) FROM bye_requests WHERE season_id=? AND week_number=? AND approved=1 AND id!=?`,
			sid, weekNum, bid).Scan(&conflict)
		if conflict > 0 {
			jsonError(w, fmt.Sprintf("another team already has an approved bye for week %d; unapprove it first", weekNum), 400)
			return
		}
	}

	res, err := db.DB.Exec(`UPDATE bye_requests SET approved=? WHERE id=? AND season_id=?`, approved, bid, sid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "bye request not found", 404)
		return
	}
	b.ID = bid
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

func getMatch(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	var m models.Match
	var completed int
	err = db.DB.QueryRow(matchSelect+` WHERE m.id=?`, id).Scan(&m.ID, &m.SeasonID, &m.HomeTeamID, &m.HomeTeamName,
		&m.AwayTeamID, &m.AwayTeamName, &m.MatchDate, &m.WeekNumber, &completed, &m.CreatedAt)
	if err != nil {
		jsonError(w, "match not found", 404)
		return
	}
	m.Completed = completed == 1
	m.MatchDate = normDatePtr(m.MatchDate)
	resRows, err := db.DB.Query(`
		SELECT mr.id, mr.match_id, mr.player_id,
		       p.first_name || ' ' || p.last_name, mr.team_id,
		       mr.sets_won, mr.sets_lost, mr.games_won, mr.games_lost, mr.diff, mr.created_at
		FROM match_results mr JOIN players p ON p.id = mr.player_id
		WHERE mr.match_id=?`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer resRows.Close()
	var results []models.MatchResult
	for resRows.Next() {
		var res models.MatchResult
		resRows.Scan(&res.ID, &res.MatchID, &res.PlayerID, &res.PlayerName, &res.TeamID,
			&res.SetsWon, &res.SetsLost, &res.GamesWon, &res.GamesLost, &res.Diff, &res.CreatedAt)
		results = append(results, res)
	}
	if results == nil {
		results = []models.MatchResult{}
	}
	jsonOK(w, models.MatchDetail{Match: m, Results: results})
}

func submitResults(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if matchWeekClosed(id) {
		jsonError(w, "week is closed; reopen before editing scores", http.StatusConflict)
		return
	}
	var req models.SubmitResultsRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()
	tx.Exec(`DELETE FROM match_results WHERE match_id=?`, id)
	for _, res := range req.Results {
		_, err := tx.Exec(`
			INSERT INTO match_results
			  (match_id, player_id, team_id, sets_won, sets_lost, games_won, games_lost, diff)
			VALUES (?,?,?,?,?,?,?,?)`,
			id, res.PlayerID, res.TeamID,
			res.SetsWon, res.SetsLost, res.GamesWon, res.GamesLost, res.Diff)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}
	tx.Exec(`UPDATE matches SET completed=1 WHERE id=?`, id)
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "saved"})
}

func clearResults(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if matchWeekClosed(id) {
		jsonError(w, "week is closed; reopen before editing scores", http.StatusConflict)
		return
	}
	db.DB.Exec(`DELETE FROM match_results WHERE match_id=?`, id)
	db.DB.Exec(`UPDATE matches SET completed=0 WHERE id=?`, id)
	jsonOK(w, map[string]string{"status": "cleared"})
}

// Week Workflow ---------------------------------------------------------------

func listWeeks(w http.ResponseWriter, r *http.Request) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	// Aggregate per-week match counts directly from matches table.
	type weekCount struct{ total, completed, closed int }
	counts := map[int]weekCount{}
	var weekOrder []int
	seen := map[int]bool{}

	matchRows, err := db.DB.Query(`
		SELECT week_number, completed, week_closed
		FROM matches WHERE season_id=?
		ORDER BY week_number`, seasonID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer matchRows.Close()
	for matchRows.Next() {
		var wn, comp, wc int
		matchRows.Scan(&wn, &comp, &wc)
		c := counts[wn]
		c.total++
		c.completed += comp
		c.closed += wc
		counts[wn] = c
		if !seen[wn] {
			weekOrder = append(weekOrder, wn)
			seen[wn] = true
		}
	}

	// Look up any existing league_weeks status rows for this season.
	type weekStatusRow struct {
		status   string
		closedAt *string
	}
	statusMap := map[int]weekStatusRow{}
	statusRows, err := db.DB.Query(`
		SELECT week_number, status, closed_at
		FROM league_weeks WHERE season_id=?`, seasonID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer statusRows.Close()
	for statusRows.Next() {
		var wn int
		var st string
		var ca *string
		statusRows.Scan(&wn, &st, &ca)
		statusMap[wn] = weekStatusRow{st, ca}
	}

	// Aggregate acknowledgment counts per week from week_close_acknowledgments.
	ackCounts := map[int]int{}
	ackCountRows, err := db.DB.Query(`
		SELECT week_number, COUNT(*) FROM week_close_acknowledgments
		WHERE season_id=? GROUP BY week_number`, seasonID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer ackCountRows.Close()
	for ackCountRows.Next() {
		var wn, cnt int
		if err := ackCountRows.Scan(&wn, &cnt); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		ackCounts[wn] = cnt
	}
	if err := ackCountRows.Err(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	var summaries []models.WeekSummary
	for _, wn := range weekOrder {
		c := counts[wn]
		st := statusMap[wn]
		status := "open"
		var closedAt *string
		if st.status != "" {
			status = st.status
			closedAt = st.closedAt
		}
		summaries = append(summaries, models.WeekSummary{
			WeekNumber:     wn,
			Status:         status,
			ClosedAt:       closedAt,
			MatchCount:     c.total,
			CompletedCount: c.completed,
			ClosedCount:    c.closed,
			AckCount:       ackCounts[wn],
		})
	}
	if summaries == nil {
		summaries = []models.WeekSummary{}
	}
	jsonOK(w, summaries)
}

func validateWeekHandler(w http.ResponseWriter, r *http.Request) {
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
	cfg := seasonRoundConfig(seasonID)
	result := matches.ValidateWeek(db.DB, seasonID, int(weekNum), cfg)
	jsonOK(w, result)
}

func closeWeekHandler(w http.ResponseWriter, r *http.Request) {
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

	// Parse optional acknowledgments from request body.
	type ackEntry struct {
		MatchID     int64  `json:"match_id"`
		WarningCode string `json:"warning_code"`
		Field       string `json:"field"`
		Notes       string `json:"notes"`
	}
	type ackRequest struct {
		Acknowledgments []ackEntry `json:"acknowledgments"`
	}
	var req ackRequest
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			jsonError(w, "invalid close week request body", http.StatusBadRequest)
			return
		}
	}

	cfg := seasonRoundConfig(seasonID)
	result := matches.ValidateWeek(db.DB, seasonID, int(weekNum), cfg)
	if result.HasErrors() {
		jsonValidation(w, result)
		return
	}

	// Build acknowledgment lookup: (match_id, warning_code, field) -> notes.
	type ackKey struct {
		matchID int64
		code    string
		field   string
	}
	ackSet := make(map[ackKey]string, len(req.Acknowledgments))
	for _, a := range req.Acknowledgments {
		ackSet[ackKey{a.MatchID, a.WarningCode, a.Field}] = a.Notes
	}

	// Every current warning must be acknowledged; stale/extra acks are ignored.
	var unacked []validation.Message
	for _, msg := range result.Warnings() {
		var mid int64
		if msg.MatchID != nil {
			mid = *msg.MatchID
		}
		if _, ok := ackSet[ackKey{mid, msg.Code, msg.Field}]; !ok {
			unacked = append(unacked, msg)
		}
	}
	if len(unacked) > 0 {
		var ackResult validation.Result
		for _, msg := range unacked {
			ackResult.AddError(msg.Code, msg.Field,
				"warning requires acknowledgment before close: "+msg.Message)
			if msg.MatchID != nil {
				id := *msg.MatchID
				ackResult.Messages[len(ackResult.Messages)-1].MatchID = &id
			}
		}
		jsonValidation(w, ackResult)
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	// Upsert the league_weeks row -- idempotent re-close updates closed_at.
	_, err = tx.Exec(`
		INSERT INTO league_weeks (season_id, week_number, status, closed_at)
		VALUES (?, ?, 'closed', CURRENT_TIMESTAMP)
		ON CONFLICT(season_id, week_number) DO UPDATE
		SET status='closed', closed_at=CURRENT_TIMESTAMP`,
		seasonID, weekNum)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Mark every match in the week as officially closed.
	_, err = tx.Exec(`
		UPDATE matches SET week_closed=1
		WHERE season_id=? AND week_number=?`,
		seasonID, weekNum)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Store one acknowledgment row per current warning in the same transaction.
	for _, msg := range result.Warnings() {
		var mid int64
		if msg.MatchID != nil {
			mid = *msg.MatchID
		}
		notes := ackSet[ackKey{mid, msg.Code, msg.Field}]
		var matchIDVal interface{}
		if mid != 0 {
			matchIDVal = mid
		}
		if _, err := tx.Exec(`
			INSERT INTO week_close_acknowledgments
			    (season_id, week_number, match_id, warning_code, field, notes)
			VALUES (?, ?, ?, ?, ?, ?)`,
			seasonID, weekNum, matchIDVal, msg.Code, msg.Field, notes); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	// Use the current close's warning count -- the only rows just inserted.
	// A cumulative DB count would overstate after reopen + re-close cycles.
	ackCount := len(result.Warnings())

	// Build advance result from post-commit DB state (best-effort; close already committed).
	ar, err := buildAdvanceResult(seasonID, weekNum)
	if err != nil {
		jsonOK(w, map[string]any{"closed": true, "week_number": int(weekNum), "acknowledgment_count": ackCount})
		return
	}
	ar.Message = "Week closed. Standings and player stats now include this week's results."

	jsonOK(w, map[string]any{
		"closed":               true,
		"week_number":          int(weekNum),
		"acknowledgment_count": ackCount,
		"advance_result":       ar,
	})
}

func reopenWeekHandler(w http.ResponseWriter, r *http.Request) {
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

	// Require at least one match to exist for this season/week.
	var matchCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM matches WHERE season_id=? AND week_number=?`,
		seasonID, weekNum).Scan(&matchCount)
	if matchCount == 0 {
		jsonError(w, "week not found: no matches for this season and week", http.StatusNotFound)
		return
	}

	// Require the week to be currently closed.
	var status string
	db.DB.QueryRow(`SELECT status FROM league_weeks WHERE season_id=? AND week_number=?`,
		seasonID, weekNum).Scan(&status)
	if status != "closed" {
		jsonError(w, "week is not closed", http.StatusConflict)
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	if _, err = tx.Exec(`
		UPDATE league_weeks SET status='open', closed_at=NULL
		WHERE season_id=? AND week_number=?`, seasonID, weekNum); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if _, err = tx.Exec(`
		UPDATE matches SET week_closed=0
		WHERE season_id=? AND week_number=?`, seasonID, weekNum); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]any{"reopened": true, "week_number": int(weekNum)})
}

func getWeekAcknowledgments(w http.ResponseWriter, r *http.Request) {
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

	// 404 when no matches exist for this season/week.
	var matchCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM matches WHERE season_id=? AND week_number=?`,
		seasonID, weekNum).Scan(&matchCount); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if matchCount == 0 {
		jsonError(w, "week not found: no matches for this season and week", http.StatusNotFound)
		return
	}

	rows, err := db.DB.Query(`
		SELECT id, season_id, week_number, match_id, warning_code, field, notes, acknowledged_at
		FROM week_close_acknowledgments
		WHERE season_id=? AND week_number=?
		ORDER BY acknowledged_at DESC`, seasonID, weekNum)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()

	var acks []models.CloseAck
	for rows.Next() {
		var a models.CloseAck
		if err := rows.Scan(&a.ID, &a.SeasonID, &a.WeekNumber, &a.MatchID,
			&a.WarningCode, &a.Field, &a.Notes, &a.AcknowledgedAt); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		acks = append(acks, a)
	}
	if err := rows.Err(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if acks == nil {
		acks = []models.CloseAck{}
	}
	jsonOK(w, acks)
}

// buildAdvanceResult queries the current DB state and returns an AdvanceResult.
// Called from both getAdvancePreview (before close) and closeWeekHandler (after commit).
// No writes are performed. The caller sets AdvanceResult.Message.
func buildAdvanceResult(seasonID, weekNum int64) (models.AdvanceResult, error) {
	// Match counts for the week (matchCount counted during scan, not a separate query).
	var matchCount, completedCount, closedCount int
	cRows, err := db.DB.Query(`SELECT completed, week_closed FROM matches WHERE season_id=? AND week_number=?`,
		seasonID, weekNum)
	if err != nil {
		return models.AdvanceResult{}, err
	}
	defer cRows.Close()
	for cRows.Next() {
		var comp, wc int
		if err := cRows.Scan(&comp, &wc); err != nil {
			return models.AdvanceResult{}, err
		}
		matchCount++
		completedCount += comp
		closedCount += wc
	}
	if err := cRows.Err(); err != nil {
		return models.AdvanceResult{}, err
	}

	// Week status: ErrNoRows means no league_weeks row (implicitly open).
	var weekStatus string
	switch err := db.DB.QueryRow(`SELECT COALESCE(status,'open') FROM league_weeks WHERE season_id=? AND week_number=?`,
		seasonID, weekNum).Scan(&weekStatus); err {
	case nil:
	case sql.ErrNoRows:
		weekStatus = "open"
	default:
		return models.AdvanceResult{}, err
	}
	if weekStatus == "" {
		weekStatus = "open"
	}

	// Find the next scheduled week (aggregate always returns one row).
	var nextWeekNum int
	if err := db.DB.QueryRow(`SELECT COALESCE(MIN(week_number),0) FROM matches WHERE season_id=? AND week_number>?`,
		seasonID, weekNum).Scan(&nextWeekNum); err != nil {
		return models.AdvanceResult{}, err
	}

	var nextWeekNumPtr *int
	var nextWeek *models.AdvancePreviewNextWeek

	if nextWeekNum > 0 {
		nextWeekNumPtr = &nextWeekNum

		var nextMatchCount int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM matches WHERE season_id=? AND week_number=?`,
			seasonID, nextWeekNum).Scan(&nextMatchCount); err != nil {
			return models.AdvanceResult{}, err
		}

		var assignedCount int
		if err := db.DB.QueryRow(`
			SELECT COUNT(*) FROM matches
			WHERE season_id=? AND week_number=?
			  AND home_team_id IS NOT NULL AND away_team_id IS NOT NULL`,
			seasonID, nextWeekNum).Scan(&assignedCount); err != nil {
			return models.AdvanceResult{}, err
		}

		var lineupPlanCount int
		if err := db.DB.QueryRow(`SELECT COUNT(*) FROM lineup_plans WHERE season_id=? AND week_number=?`,
			seasonID, nextWeekNum).Scan(&lineupPlanCount); err != nil {
			return models.AdvanceResult{}, err
		}

		// Teams that appear in next week's matches.
		teamRows, tErr := db.DB.Query(`
			SELECT DISTINCT t FROM (
				SELECT home_team_id AS t FROM matches
				WHERE season_id=? AND week_number=? AND home_team_id IS NOT NULL
				UNION
				SELECT away_team_id AS t FROM matches
				WHERE season_id=? AND week_number=? AND away_team_id IS NOT NULL
			)`, seasonID, nextWeekNum, seasonID, nextWeekNum)
		if tErr != nil {
			return models.AdvanceResult{}, tErr
		}
		defer teamRows.Close()
		var allTeamIDs []int64
		for teamRows.Next() {
			var tid int64
			if err := teamRows.Scan(&tid); err != nil {
				return models.AdvanceResult{}, err
			}
			allTeamIDs = append(allTeamIDs, tid)
		}
		if err := teamRows.Err(); err != nil {
			return models.AdvanceResult{}, err
		}

		missingTeamIDs := make([]int64, 0)
		for _, tid := range allTeamIDs {
			var planCount int
			if err := db.DB.QueryRow(`SELECT COUNT(*) FROM lineup_plans WHERE season_id=? AND week_number=? AND team_id=?`,
				seasonID, nextWeekNum, tid).Scan(&planCount); err != nil {
				return models.AdvanceResult{}, err
			}
			if planCount == 0 {
				missingTeamIDs = append(missingTeamIDs, tid)
			}
		}

		nextWeek = &models.AdvancePreviewNextWeek{
			MatchCount:           nextMatchCount,
			AssignedCount:        assignedCount,
			UnassignedCount:      nextMatchCount - assignedCount,
			LineupPlanCount:      lineupPlanCount,
			MissingLineupTeamIDs: missingTeamIDs,
		}
	}

	hc, err := buildHandicapPreview(seasonID, weekNum)
	if err != nil {
		return models.AdvanceResult{}, err
	}

	return models.AdvanceResult{
		ClosedWeek: models.AdvancePreviewWeekSummary{
			MatchCount:     matchCount,
			CompletedCount: completedCount,
			ClosedCount:    closedCount,
			Status:         weekStatus,
		},
		NextWeekNumber: nextWeekNumPtr,
		NextWeek:       nextWeek,
		Handicap:       hc,
	}, nil
}

// buildHandicapPreview computes read-only handicap recommendations for a season.
// No writes are performed to players, handicap_history, or any other table.
// Recommendations are draft preview logic (game_diff_average) -- not confirmed league policy.
func buildHandicapPreview(seasonID, _ int64) (models.AdvancePreviewHandicap, error) {
	method, err := seasonHandicapUpdateMethod(seasonID)
	if err != nil {
		return models.AdvancePreviewHandicap{}, err
	}
	switch method {
	case "kicker_average_preview":
		return models.AdvancePreviewHandicap{
			Method:  method,
			Status:  "unsupported",
			Message: "Kicker average handicap recommendations are not implemented yet. No handicap changes are applied automatically.",
		}, nil
	case "game_diff_average":
		maxHC, err := seasonMaxIndividualHC(seasonID)
		if err != nil {
			return models.AdvancePreviewHandicap{}, err
		}
		recs, err := computeGameDiffAverageRecs(seasonID, maxHC)
		if err != nil {
			return models.AdvancePreviewHandicap{}, err
		}
		changedCount := 0
		for _, r := range recs {
			if !r.Skipped && r.Reason != "no_change" {
				changedCount++
			}
		}
		var msg string
		switch {
		case changedCount == 1:
			msg = "1 player has a recommended handicap change (not yet applied)."
		case changedCount > 1:
			msg = fmt.Sprintf("%d players have recommended handicap changes (not yet applied).", changedCount)
		default:
			msg = "No handicap changes recommended. No changes are applied automatically."
		}
		return models.AdvancePreviewHandicap{
			Method:          method,
			Status:          "preview",
			Message:         msg,
			Recommendations: recs,
		}, nil
	default: // "manual_review" and any unknown method
		return models.AdvancePreviewHandicap{
			Method:  method,
			Status:  "no_auto_apply",
			Message: "No handicap changes are applied automatically.",
		}, nil
	}
}

// computeGameDiffAverageRecs returns per-player handicap recommendations using average
// game diff across all completed, officially closed matches for the season.
// Players are sourced from season_rosters (managed seasons) and/or match_results for
// closed matches (legacy seasons), so no_data entries can be represented.
// No writes are performed.
func computeGameDiffAverageRecs(seasonID int64, maxHC float64) ([]models.PlayerHandicapRec, error) {
	rows, err := db.DB.Query(`
		SELECT
		    p.id,
		    p.first_name || ' ' || p.last_name,
		    p.handicap,
		    p.admin_hold,
		    COUNT(mr.id),
		    COALESCE(SUM(mr.diff), 0.0)
		FROM (
		    SELECT DISTINCT player_id FROM season_rosters WHERE season_id = ?
		    UNION
		    SELECT DISTINCT mr2.player_id
		    FROM match_results mr2
		    JOIN matches m2 ON m2.id = mr2.match_id
		    WHERE m2.season_id = ? AND m2.completed = 1 AND m2.week_closed = 1
		) AS candidates
		JOIN players p ON p.id = candidates.player_id
		LEFT JOIN match_results mr ON mr.player_id = p.id
		    AND mr.match_id IN (
		        SELECT id FROM matches
		        WHERE season_id = ? AND completed = 1 AND week_closed = 1
		    )
		GROUP BY p.id, p.first_name, p.last_name, p.handicap, p.admin_hold
		ORDER BY p.first_name, p.last_name`,
		seasonID, seasonID, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []models.PlayerHandicapRec
	for rows.Next() {
		var (
			playerID   int64
			playerName string
			currentHC  float64
			adminHold  bool
			matchCount int
			totalDiff  float64
		)
		if err := rows.Scan(&playerID, &playerName, &currentHC, &adminHold, &matchCount, &totalDiff); err != nil {
			return nil, err
		}

		rec := models.PlayerHandicapRec{
			PlayerID:        playerID,
			PlayerName:      playerName,
			CurrentHandicap: currentHC,
			AdminHold:       adminHold,
			MatchesPlayed:   matchCount,
		}

		if adminHold {
			rec.Skipped             = true
			rec.Reason              = "admin_hold"
			rec.RecommendedHandicap = currentHC
			recs = append(recs, rec)
			continue
		}

		if matchCount == 0 {
			rec.Skipped             = true
			rec.Reason              = "no_data"
			rec.RecommendedHandicap = currentHC
			recs = append(recs, rec)
			continue
		}

		avg := totalDiff / float64(matchCount)
		recommended := math.Round(avg*10) / 10 // nearest 0.1

		capped := false
		if recommended > maxHC {
			recommended = math.Round(maxHC*10) / 10
			capped = true
		} else if recommended < -maxHC {
			recommended = math.Round(-maxHC*10) / 10
			capped = true
		}
		rec.RecommendedHandicap = recommended

		switch {
		case capped:
			rec.Reason = "capped"
		case math.Round(currentHC*10)/10 == recommended:
			rec.Reason = "no_change"
		}

		recs = append(recs, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return recs, nil
}

// seasonHandicapUpdateMethod returns the handicap_update_method rule for a season,
// defaulting to "manual_review" if absent or empty.
func seasonHandicapUpdateMethod(seasonID int64) (string, error) {
	var val string
	err := db.DB.QueryRow(
		`SELECT rule_value FROM season_rules WHERE season_id=? AND rule_key='handicap_update_method'`,
		seasonID).Scan(&val)
	if err == sql.ErrNoRows || val == "" {
		return "manual_review", nil
	}
	if err != nil {
		return "", err
	}
	return val, nil
}

// seasonMaxIndividualHC returns the max_individual_handicap rule for a season,
// defaulting to 4.5 if absent or invalid.
func seasonMaxIndividualHC(seasonID int64) (float64, error) {
	var val string
	err := db.DB.QueryRow(
		`SELECT rule_value FROM season_rules WHERE season_id=? AND rule_key='max_individual_handicap'`,
		seasonID).Scan(&val)
	if err == sql.ErrNoRows || val == "" {
		return 4.5, nil
	}
	if err != nil {
		return 0, err
	}
	f, parseErr := strconv.ParseFloat(val, 64)
	if parseErr != nil || f <= 0 {
		return 4.5, nil
	}
	return f, nil
}

func getAdvancePreview(w http.ResponseWriter, r *http.Request) {
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

	// 404 when no matches exist for this season/week.
	var matchCount int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM matches WHERE season_id=? AND week_number=?`,
		seasonID, weekNum).Scan(&matchCount); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if matchCount == 0 {
		jsonError(w, "week not found: no matches for this season and week", http.StatusNotFound)
		return
	}

	// Run validation to determine can_close and messages.
	cfg := seasonRoundConfig(seasonID)
	result := matches.ValidateWeek(db.DB, seasonID, int(weekNum), cfg)

	previewMsgs := make([]models.AdvancePreviewMessage, 0, len(result.Messages))
	for _, msg := range result.Messages {
		previewMsgs = append(previewMsgs, models.AdvancePreviewMessage{
			Code:    msg.Code,
			Field:   msg.Field,
			Message: msg.Message,
			Level:   string(msg.Level),
			MatchID: msg.MatchID,
		})
	}

	ar, err := buildAdvanceResult(seasonID, weekNum)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	preview := models.AdvancePreview{
		SeasonID:           seasonID,
		WeekNumber:         int(weekNum),
		CanClose:           !result.HasErrors(),
		ValidationMessages: previewMsgs,
		CurrentWeek:        ar.ClosedWeek,
		NextWeekNumber:     ar.NextWeekNumber,
		NextWeek:           ar.NextWeek,
		Handicap:           ar.Handicap,
	}
	jsonOK(w, preview)
}

// --- Handicap Review ---------------------------------------------------------

// computeHandicapReviewRecs returns per-player handicap recommendations for the
// Handicap Review screen. Extends the game_diff_average logic with team_name and
// change_amount. Recommendations recompute live; no rows are written anywhere.
func computeHandicapReviewRecs(seasonID int64, maxHC float64) ([]models.HandicapReviewRec, error) {
	rows, err := db.DB.Query(`
		SELECT
		    p.id,
		    p.first_name || ' ' || p.last_name,
		    COALESCE(t.name, ''),
		    p.handicap,
		    p.admin_hold,
		    COUNT(mr.id),
		    COALESCE(SUM(mr.diff), 0.0)
		FROM (
		    SELECT DISTINCT player_id FROM season_rosters WHERE season_id = ?
		    UNION
		    SELECT DISTINCT mr2.player_id
		    FROM match_results mr2
		    JOIN matches m2 ON m2.id = mr2.match_id
		    WHERE m2.season_id = ? AND m2.completed = 1 AND m2.week_closed = 1
		) AS candidates
		JOIN players p ON p.id = candidates.player_id
		LEFT JOIN teams t ON t.id = p.team_id
		LEFT JOIN match_results mr ON mr.player_id = p.id
		    AND mr.match_id IN (
		        SELECT id FROM matches
		        WHERE season_id = ? AND completed = 1 AND week_closed = 1
		    )
		GROUP BY p.id, p.first_name, p.last_name, t.name, p.handicap, p.admin_hold
		ORDER BY t.name, p.first_name, p.last_name`,
		seasonID, seasonID, seasonID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []models.HandicapReviewRec
	for rows.Next() {
		var (
			playerID   int64
			playerName string
			teamName   string
			currentHC  float64
			adminHold  bool
			matchCount int
			totalDiff  float64
		)
		if err := rows.Scan(&playerID, &playerName, &teamName, &currentHC, &adminHold, &matchCount, &totalDiff); err != nil {
			return nil, err
		}

		rec := models.HandicapReviewRec{
			PlayerID:        playerID,
			PlayerName:      playerName,
			TeamName:        teamName,
			CurrentHandicap: currentHC,
			AdminHold:       adminHold,
			MatchesPlayed:   matchCount,
		}

		if adminHold {
			rec.Skipped             = true
			rec.Reason              = "admin_hold"
			rec.RecommendedHandicap = currentHC
			recs = append(recs, rec)
			continue
		}

		if matchCount == 0 {
			rec.Skipped             = true
			rec.Reason              = "no_data"
			rec.RecommendedHandicap = currentHC
			recs = append(recs, rec)
			continue
		}

		avg := totalDiff / float64(matchCount)
		recommended := math.Round(avg*10) / 10

		capped := false
		if recommended > maxHC {
			recommended = math.Round(maxHC*10) / 10
			capped = true
		} else if recommended < -maxHC {
			recommended = math.Round(-maxHC*10) / 10
			capped = true
		}
		rec.RecommendedHandicap = recommended
		rec.ChangeAmount        = math.Round((recommended-currentHC)*10) / 10

		switch {
		case capped:
			rec.Reason = "capped"
		case math.Round(currentHC*10)/10 == recommended:
			rec.Reason = "no_change"
		}

		recs = append(recs, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return recs, nil
}

// getHandicapRecommendations handles GET /api/seasons/{id}/handicap-recommendations.
// Returns season-wide read-only handicap recommendations based on all closed weeks.
// No writes are performed to players, handicap_history, or any other table.
// Recommendations recompute live; reopening a week automatically removes its data
// from the next response because reopened matches have week_closed=0.
func getHandicapRecommendations(w http.ResponseWriter, r *http.Request) {
	seasonID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	var exists int
	if err := db.DB.QueryRow(`SELECT COUNT(*) FROM seasons WHERE id=?`, seasonID).Scan(&exists); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if exists == 0 {
		jsonError(w, "season not found", http.StatusNotFound)
		return
	}

	method, err := seasonHandicapUpdateMethod(seasonID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	emptyResponse := func(status, message string, weeksClosed int) {
		jsonOK(w, models.HandicapReviewResponse{
			SeasonID:        seasonID,
			Method:          method,
			Status:          status,
			Message:         message,
			WeeksClosed:     weeksClosed,
			Recommendations: []models.HandicapReviewRec{},
		})
	}

	switch method {
	case "manual_review":
		emptyResponse("no_auto_apply",
			"No handicap changes are applied automatically. Update player handicaps manually via the Players tab.",
			0)
		return
	case "kicker_average_preview":
		emptyResponse("unsupported",
			"kicker_average_preview is not yet implemented. No changes are applied automatically.",
			0)
		return
	}

	// game_diff_average path.
	var weeksClosed int
	if err := db.DB.QueryRow(
		`SELECT COUNT(DISTINCT week_number) FROM matches WHERE season_id=? AND week_closed=1`,
		seasonID).Scan(&weeksClosed); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if weeksClosed == 0 {
		emptyResponse("no_data",
			"No closed weeks available. Close a week to generate handicap recommendations.",
			0)
		return
	}

	maxHC, err := seasonMaxIndividualHC(seasonID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	recs, err := computeHandicapReviewRecs(seasonID, maxHC)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	changed := 0
	for _, rec := range recs {
		if !rec.Skipped && rec.Reason != "no_change" {
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

	jsonOK(w, models.HandicapReviewResponse{
		SeasonID:        seasonID,
		Method:          method,
		Status:          "preview",
		Message:         msg,
		WeeksClosed:     weeksClosed,
		Recommendations: recs,
	})
}

// ─── Standings ────────────────────────────────────────────────────────────────

func getStandings(w http.ResponseWriter, r *http.Request) {
	seasonID := qparam(r, "season_id")
	if seasonID == "" {
		leagueID, ok := qparamInt(r, "league_id")
		if !ok {
			jsonOK(w, []models.Standing{})
			return
		}
		var id int64
		if err := db.DB.QueryRow(
			`SELECT id FROM seasons WHERE league_id=? AND active=1 LIMIT 1`, leagueID,
		).Scan(&id); err != nil {
			jsonOK(w, []models.Standing{})
			return
		}
		seasonID = fmt.Sprintf("%d", id)
	}
	rows, err := db.DB.Query(`
		SELECT t.id, t.name FROM teams t
		JOIN seasons s ON s.league_id = t.league_id
		WHERE s.id=? ORDER BY t.name`, seasonID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var teams []models.Team
	for rows.Next() {
		var t models.Team
		rows.Scan(&t.ID, &t.Name)
		teams = append(teams, t)
	}
	rows.Close()

	matchRows, err := db.DB.Query(`
		SELECT id, season_id, home_team_id, away_team_id, match_date, week_number, completed, created_at
		FROM matches WHERE season_id=? AND completed=1 AND week_closed=1`, seasonID)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	var matches []models.Match
	for matchRows.Next() {
		var m models.Match
		var completed int
		matchRows.Scan(&m.ID, &m.SeasonID, &m.HomeTeamID, &m.AwayTeamID,
			&m.MatchDate, &m.WeekNumber, &completed, &m.CreatedAt)
		m.Completed = completed == 1
		matches = append(matches, m)
	}
	matchRows.Close()

	resultMap := make(map[int64][]models.MatchResult)
	for _, m := range matches {
		resRows, err := db.DB.Query(`
			SELECT id, match_id, player_id, team_id, sets_won, sets_lost,
			       games_won, games_lost, diff, created_at
			FROM match_results WHERE match_id=?`, m.ID)
		if err != nil {
			continue
		}
		for resRows.Next() {
			var res models.MatchResult
			resRows.Scan(&res.ID, &res.MatchID, &res.PlayerID, &res.TeamID,
				&res.SetsWon, &res.SetsLost, &res.GamesWon, &res.GamesLost, &res.Diff, &res.CreatedAt)
			resultMap[m.ID] = append(resultMap[m.ID], res)
		}
		resRows.Close()
	}
	standings := logic.ComputeStandings(matches, resultMap, teams)
	if standings == nil {
		standings = []models.Standing{}
	}
	jsonOK(w, standings)
}

// seasonMultiplier returns the handicap_multiplier rule value for a season,
// defaulting to logic.Multiplier (2.55) if the rule is absent or invalid.
func seasonMultiplier(seasonID int64) float64 {
	var val string
	db.DB.QueryRow(
		`SELECT rule_value FROM season_rules WHERE season_id=? AND rule_key='handicap_multiplier'`,
		seasonID).Scan(&val)
	if val == "" {
		return logic.Multiplier
	}
	f, err := strconv.ParseFloat(val, 64)
	if err != nil || f <= 0 {
		return logic.Multiplier
	}
	return f
}

// seasonRoundConfig builds a RoundConfig from the season's handicap_multiplier and
// min_ball_handicap rules. Used by saveRounds, validateWeekHandler, and closeWeekHandler
// so they all apply identical rule resolution.
func seasonRoundConfig(seasonID int64) matches.RoundConfig {
	mult := seasonMultiplier(seasonID)
	var minBallStr string
	db.DB.QueryRow(
		`SELECT rule_value FROM season_rules WHERE season_id=? AND rule_key='min_ball_handicap'`,
		seasonID).Scan(&minBallStr)
	minBallHC, _ := strconv.Atoi(minBallStr)
	return matches.RoundConfig{Multiplier: mult, MinBallHC: minBallHC}
}

// matchWeekClosed returns true when the match's week has been officially closed.
func matchWeekClosed(matchID int64) bool {
	var wc int
	db.DB.QueryRow(`SELECT week_closed FROM matches WHERE id=?`, matchID).Scan(&wc)
	return wc == 1
}

// ─── 8-Ball Round Results ─────────────────────────────────────────────────────

// computePairingResult fills the derived/computed fields on a RoundResult.
// multiplier is the season's handicap_multiplier (default logic.Multiplier = 2.55).
// If a handicap snapshot is stored on the row, that is preferred over current
// player handicap so historical scoresheets are stable.
func computePairingResult(rr *models.RoundResult, multiplier float64) {
	homeRaw := rr.Game1Home + rr.Game2Home + rr.Game3Home
	awayRaw := rr.Game1Away + rr.Game2Away + rr.Game3Away

	// Use stored snapshot when available; otherwise fall back to current player HC.
	if rr.HandicapPtsUsed != nil && rr.HandicapToUsed != nil {
		// Snapshot present — use it directly, no recomputation needed.
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
		rr.PairingWinner = "" // tie
	}
}

func getRounds(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	// Look up the season multiplier so historical scoresheets use the right value
	// when there is no snapshot (older rows). New rows have snapshots so this only
	// matters as a fallback.
	var seasonID int64
	db.DB.QueryRow(`SELECT season_id FROM matches WHERE id=?`, id).Scan(&seasonID)
	mult := seasonMultiplier(seasonID)

	rows, err := db.DB.Query(`
		SELECT rr.id, rr.match_id, rr.round_number,
		       rr.home_player_id, hp.first_name||' '||hp.last_name, hp.handicap,
		       rr.away_player_id, ap.first_name||' '||ap.last_name, ap.handicap,
		       rr.game1_home, rr.game1_away,
		       rr.game2_home, rr.game2_away,
		       rr.game3_home, rr.game3_away,
		       rr.home_handicap_used, rr.away_handicap_used,
		       rr.handicap_pts_used,  rr.handicap_to
		FROM round_results rr
		JOIN players hp ON hp.id = rr.home_player_id
		JOIN players ap ON ap.id = rr.away_player_id
		WHERE rr.match_id = ?
		ORDER BY rr.round_number, rr.id`, id)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var rounds []models.RoundResult
	for rows.Next() {
		var rr models.RoundResult
		if err := rows.Scan(&rr.ID, &rr.MatchID, &rr.RoundNumber,
			&rr.HomePlayerID, &rr.HomePlayerName, &rr.HomeHandicap,
			&rr.AwayPlayerID, &rr.AwayPlayerName, &rr.AwayHandicap,
			&rr.Game1Home, &rr.Game1Away,
			&rr.Game2Home, &rr.Game2Away,
			&rr.Game3Home, &rr.Game3Away,
			&rr.HomeHandicapUsed, &rr.AwayHandicapUsed,
			&rr.HandicapPtsUsed, &rr.HandicapToUsed); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		computePairingResult(&rr, mult)
		rounds = append(rounds, rr)
	}
	if rounds == nil {
		rounds = []models.RoundResult{}
	}
	jsonOK(w, rounds)
}

func saveRounds(w http.ResponseWriter, r *http.Request) {
	matchID, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if matchWeekClosed(matchID) {
		jsonError(w, "week is closed; reopen before editing scores", http.StatusConflict)
		return
	}
	var req models.SaveRoundsRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}

	// Block scoresheet entry when either team has fewer than 3 season-roster players.
	// Only enforced when season_teams is in use (see RosterEligible for details).
	if ok, msg := seasons.RosterEligible(db.DB, matchID, 3); !ok {
		jsonError(w, msg, http.StatusUnprocessableEntity)
		return
	}

	// Collect all player IDs to look up their current handicaps
	playerIDs := make(map[int64]float64)
	for _, rr := range req.Rounds {
		playerIDs[rr.HomePlayerID] = 0
		playerIDs[rr.AwayPlayerID] = 0
	}
	for pid := range playerIDs {
		var hc float64
		db.DB.QueryRow(`SELECT handicap FROM players WHERE id=?`, pid).Scan(&hc)
		playerIDs[pid] = hc
	}

	// Look up season rules before validation and save.
	var saveSeasonID int64
	db.DB.QueryRow(`SELECT season_id FROM matches WHERE id=?`, matchID).Scan(&saveSeasonID)
	saveCfg := seasonRoundConfig(saveSeasonID)

	// Backend validation -- errors block save; warnings are noted but do not block.
	// Warnings are not currently returned to the frontend; they are available for Close Week use.
	vResult := matches.ValidateRounds(req.Rounds, playerIDs, saveCfg)
	if vResult.HasErrors() {
		jsonValidation(w, vResult.Result)
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	// Replace all round results for this match
	tx.Exec(`DELETE FROM round_results WHERE match_id=?`, matchID)

	for _, rr := range req.Rounds {
		spot := logic.CalcSpotM(playerIDs[rr.HomePlayerID], playerIDs[rr.AwayPlayerID], saveCfg.Multiplier)
		_, err := tx.Exec(`
			INSERT INTO round_results
			  (match_id, round_number, home_player_id, away_player_id,
			   game1_home, game1_away, game2_home, game2_away, game3_home, game3_away,
			   home_handicap_used, away_handicap_used, handicap_pts_used, handicap_to)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			matchID, rr.RoundNumber, rr.HomePlayerID, rr.AwayPlayerID,
			rr.Game1Home, rr.Game1Away,
			rr.Game2Home, rr.Game2Away,
			rr.Game3Home, rr.Game3Away,
			playerIDs[rr.HomePlayerID], playerIDs[rr.AwayPlayerID],
			spot.Pts, spot.To)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	// Derive per-player match_results from round scores.
	// games_won  = individual games where the player scored 10 (won the game)
	// games_lost = individual games where they scored < 10 (lost)
	// diff       = games_won − games_lost for this match (numerator for Diff handicap)
	// sets_won / sets_lost = rounds won/lost by the player's team
	type tally struct{ gw, gl, sw, sl int }
	tallies := map[int64]*tally{}
	ensure := func(pid int64) *tally {
		if tallies[pid] == nil {
			tallies[pid] = &tally{}
		}
		return tallies[pid]
	}

	for _, rr := range req.Rounds {
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
				// If neither is 10 the game wasn't entered yet — skip
			}
		}
	}

	// Resolve team IDs from the match
	var homeTeamID, awayTeamID int64
	db.DB.QueryRow(`SELECT home_team_id, away_team_id FROM matches WHERE id=?`, matchID).
		Scan(&homeTeamID, &awayTeamID)

	pTeam := map[int64]int64{}
	for _, rr := range req.Rounds {
		pTeam[rr.HomePlayerID] = homeTeamID
		pTeam[rr.AwayPlayerID] = awayTeamID
	}

	// Compute per-player sets_won / sets_lost from round winners.
	for roundNum, winner := range vResult.RoundWinners {
		if winner == "" {
			continue
		}
		for _, rr := range req.Rounds {
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

	// Replace match_results
	tx.Exec(`DELETE FROM match_results WHERE match_id=?`, matchID)
	for pid, t := range tallies {
		diff := float64(t.gw - t.gl)
		_, err := tx.Exec(`
			INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff, sets_won, sets_lost)
			VALUES (?,?,?,?,?,?,?,?)`,
			matchID, pid, pTeam[pid], t.gw, t.gl, diff, t.sw, t.sl)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	}

	// Mark match completed whenever any game has been scored (score of 10 recorded)
	anyScored := false
	for _, rr := range req.Rounds {
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
		tx.Exec(`UPDATE matches SET completed=1 WHERE id=?`, matchID)
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]any{"saved": len(req.Rounds)})
}

func getPlayerStats(w http.ResponseWriter, r *http.Request) {
	seasonID := qparam(r, "season_id")
	leagueID := qparam(r, "league_id")
	var query string
	var args []any
	switch {
	case seasonID != "":
		query = `
			SELECT p.id, COALESCE(p.player_number,''), p.first_name || ' ' || p.last_name,
			       COALESCE(t.name,''), p.handicap,
			       COALESCE(SUM(mr.sets_won),0), COALESCE(SUM(mr.sets_lost),0),
			       COALESCE(SUM(mr.games_won),0), COALESCE(SUM(mr.games_lost),0)
			FROM players p
			JOIN teams t ON t.id = p.team_id
			JOIN seasons s ON s.league_id = t.league_id AND s.id = ?
			LEFT JOIN match_results mr ON mr.player_id = p.id
			    AND mr.match_id IN (
			        SELECT id FROM matches
			        WHERE season_id=? AND completed=1 AND week_closed=1
			    )
			GROUP BY p.id ORDER BY SUM(mr.sets_won) DESC, SUM(mr.games_won) DESC`
		args = []any{seasonID, seasonID}
	case leagueID != "":
		query = `
			SELECT p.id, COALESCE(p.player_number,''), p.first_name || ' ' || p.last_name,
			       COALESCE(t.name,''), p.handicap,
			       COALESCE(SUM(mr.sets_won),0), COALESCE(SUM(mr.sets_lost),0),
			       COALESCE(SUM(mr.games_won),0), COALESCE(SUM(mr.games_lost),0)
			FROM players p
			JOIN teams t ON t.id = p.team_id AND t.league_id = ?
			LEFT JOIN match_results mr ON mr.player_id = p.id
			GROUP BY p.id ORDER BY SUM(mr.sets_won) DESC, SUM(mr.games_won) DESC`
		args = []any{leagueID}
	default:
		jsonOK(w, []models.PlayerStat{})
		return
	}
	rows, err := db.DB.Query(query, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var stats []models.PlayerStat
	for rows.Next() {
		var s models.PlayerStat
		rows.Scan(&s.PlayerID, &s.PlayerNumber, &s.PlayerName, &s.TeamName, &s.Handicap,
			&s.SetsWon, &s.SetsLost, &s.GamesWon, &s.GamesLost)
		total := s.GamesWon + s.GamesLost
		if total > 0 {
			s.WinPct = float64(s.GamesWon) / float64(total)
		}
		stats = append(stats, s)
	}
	if stats == nil {
		stats = []models.PlayerStat{}
	}
	jsonOK(w, stats)
}

// ─── Lineup Plans ─────────────────────────────────────────────────────────────

func listLineupPlans(w http.ResponseWriter, r *http.Request) {
	seasonID, hasSeason := qparamInt(r, "season_id")
	if !hasSeason {
		jsonError(w, "season_id required", 400)
		return
	}
	weekNum, hasWeek := qparamInt(r, "week_number")
	teamID, hasTeam := qparamInt(r, "team_id")

	q := `SELECT lp.id, lp.season_id, lp.team_id, t.name,
	             lp.player_id, p.first_name || ' ' || p.last_name, p.handicap,
	             lp.week_number, lp.is_sub, lp.sub_for_id
	      FROM lineup_plans lp
	      JOIN teams t ON t.id = lp.team_id
	      JOIN players p ON p.id = lp.player_id
	      WHERE lp.season_id = ?`
	args := []any{seasonID}
	if hasWeek {
		q += ` AND lp.week_number = ?`
		args = append(args, weekNum)
	}
	if hasTeam {
		q += ` AND lp.team_id = ?`
		args = append(args, teamID)
	}
	q += ` ORDER BY lp.team_id, lp.id`

	rows, err := db.DB.Query(q, args...)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer rows.Close()
	var plans []models.LineupPlan
	for rows.Next() {
		var lp models.LineupPlan
		var isSub int
		if err := rows.Scan(&lp.ID, &lp.SeasonID, &lp.TeamID, &lp.TeamName,
			&lp.PlayerID, &lp.PlayerName, &lp.Handicap,
			&lp.WeekNumber, &isSub, &lp.SubForID); err != nil {
			continue
		}
		lp.IsSub = isSub == 1
		plans = append(plans, lp)
	}
	if plans == nil {
		plans = []models.LineupPlan{}
	}
	jsonOK(w, plans)
}

// saveTeamLineup atomically replaces all lineup slots for one team/week.
// Body: { season_id, team_id, week_number, player_ids: [id1, id2, id3] }
func saveTeamLineup(w http.ResponseWriter, r *http.Request) {
	var req models.SaveTeamLineupRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	if req.SeasonID == 0 || req.TeamID == 0 {
		jsonError(w, "season_id and team_id required", 400)
		return
	}
	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()
	tx.Exec(`DELETE FROM lineup_plans WHERE season_id=? AND team_id=? AND week_number=?`,
		req.SeasonID, req.TeamID, req.WeekNumber)
	for _, pid := range req.PlayerIDs {
		if pid == 0 {
			continue
		}
		tx.Exec(`INSERT OR IGNORE INTO lineup_plans (season_id, team_id, week_number, player_id, is_sub) VALUES (?,?,?,?,0)`,
			req.SeasonID, req.TeamID, req.WeekNumber, pid)
	}
	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	jsonOK(w, map[string]string{"status": "saved"})
}

func deleteLineupPlan(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	db.DB.Exec(`DELETE FROM lineup_plans WHERE id=?`, id)
	jsonOK(w, map[string]string{"status": "deleted"})
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

// addSeasonTeamRequest is the POST body for adding a team to a draft season.
// Exactly one of (FromTeamID+FromSeasonID) or Name must be set.
type addSeasonTeamRequest struct {
	FromTeamID   int64  `json:"from_team_id"`   // copy existing team from a prior season
	FromSeasonID int64  `json:"from_season_id"` // season the team last played in (0 = use team.players)
	Name         string `json:"name"`           // new team name (creates a teams record)
}

// markStaleIfScheduled sets schedule_stale=1 when unplayed matches already exist.
func markStaleIfScheduled(seasonID int64) {
	var n int
	db.DB.QueryRow(`SELECT COUNT(*) FROM matches WHERE season_id=? AND completed=0`, seasonID).Scan(&n)
	if n > 0 {
		db.DB.Exec(`UPDATE seasons SET schedule_stale=1 WHERE id=?`, seasonID)
	}
}

// isDraftSeason returns false when the season's setup has been locked by activation.
// activated_at is set once on first activation and never cleared; it survives
// another season becoming active (deactivation does not reset it).
func isDraftSeason(seasonID int64) bool {
	var activatedAt *string
	db.DB.QueryRow(`SELECT activated_at FROM seasons WHERE id=?`, seasonID).Scan(&activatedAt)
	return activatedAt == nil
}

func addSeasonTeam(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	if !isDraftSeason(sid) {
		jsonError(w, "cannot modify teams in an active season", 422)
		return
	}

	var req addSeasonTeamRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}

	var leagueID int64
	var startDate *string
	var draftTeamsManaged int
	if err := db.DB.QueryRow(`SELECT league_id, start_date, COALESCE(teams_managed,0) FROM seasons WHERE id=?`, sid).Scan(&leagueID, &startDate, &draftTeamsManaged); err != nil {
		jsonError(w, "season not found", 404)
		return
	}

	// For managed seasons, from_team_id always requires from_season_id.
	// New teams must be created via the name path; from_team_id copies from a prior season only.
	if draftTeamsManaged == 1 && req.FromTeamID > 0 && req.FromSeasonID == 0 {
		jsonError(w, "managed seasons require from_season_id with from_team_id; use name to create a new team", 400)
		return
	}

	// When from_season_id is provided, it must be the immediately previous season.
	if req.FromTeamID > 0 && req.FromSeasonID > 0 {
		prev, prevErr := seasons.PreviousSeason(db.DB, sid, leagueID, normDatePtr(startDate))
		if prevErr != nil {
			jsonError(w, prevErr.Error(), 500)
			return
		}
		if prev == nil || prev.ID != req.FromSeasonID {
			jsonError(w, "from_season_id must be the immediately previous season", 400)
			return
		}
	}

	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	var teamID int64
	var seasonName string
	var captainID *int64

	if req.FromTeamID > 0 {
		// ── Copy from a prior season ──────────────────────────────────────────
		// Verify the team belongs to this league.
		var tLID int64
		if err := tx.QueryRow(`SELECT league_id FROM teams WHERE id=?`, req.FromTeamID).Scan(&tLID); err != nil || tLID != leagueID {
			jsonError(w, "team not found in this league", 400)
			return
		}
		teamID = req.FromTeamID

		// Prefer name/captain from prior season_teams; fall back to teams row.
		if req.FromSeasonID > 0 {
			// Verify team participated in the previous season when it was managed.
			var prevManaged int
			tx.QueryRow(`SELECT COALESCE(teams_managed,0) FROM seasons WHERE id=?`, req.FromSeasonID).Scan(&prevManaged)
			if prevManaged == 1 {
				var inPrev int
				tx.QueryRow(`SELECT COUNT(*) FROM season_teams WHERE season_id=? AND team_id=?`,
					req.FromSeasonID, teamID).Scan(&inPrev)
				if inPrev == 0 {
					jsonError(w, "team did not participate in the previous season", 400)
					return
				}
			}
			tx.QueryRow(
				`SELECT CASE WHEN season_name != '' THEN season_name ELSE t.name END, captain_id
				 FROM season_teams st JOIN teams t ON t.id=st.team_id
				 WHERE st.season_id=? AND st.team_id=?`,
				req.FromSeasonID, teamID).Scan(&seasonName, &captainID)
		}
		if seasonName == "" {
			tx.QueryRow(`SELECT name FROM teams WHERE id=?`, teamID).Scan(&seasonName)
		}

		// Insert season_teams row.
		res, err := tx.Exec(
			`INSERT OR IGNORE INTO season_teams (season_id, team_id, season_name, captain_id)
			 VALUES (?,?,?,?)`, sid, teamID, seasonName, captainID)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		if n, _ := res.RowsAffected(); n == 0 {
			jsonError(w, "team is already in this season", 400)
			return
		}

		// Copy roster from prior season_rosters; fall back to players.team_id.
		var copiedFromRoster bool
		if req.FromSeasonID > 0 {
			rr, _ := tx.Query(
				`SELECT player_id FROM season_rosters WHERE season_id=? AND team_id=?`,
				req.FromSeasonID, teamID)
			if rr != nil {
				for rr.Next() {
					var pid int64
					rr.Scan(&pid)
					tx.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
						sid, teamID, pid)
					copiedFromRoster = true
				}
				rr.Close()
			}
		}
		if !copiedFromRoster && draftTeamsManaged == 0 {
			// Legacy fallback: copy all active players currently on this team.
			pr, _ := tx.Query(`SELECT id FROM players WHERE team_id=? AND COALESCE(active,1)=1`, teamID)
			if pr != nil {
				for pr.Next() {
					var pid int64
					pr.Scan(&pid)
					tx.Exec(`INSERT OR IGNORE INTO season_rosters (season_id, team_id, player_id) VALUES (?,?,?)`,
						sid, teamID, pid)
				}
				pr.Close()
			}
		}

	} else if strings.TrimSpace(req.Name) != "" {
		// ── Create a brand-new team ───────────────────────────────────────────
		res, err := tx.Exec(`INSERT INTO teams (league_id, name) VALUES (?,?)`, leagueID, req.Name)
		if err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
		teamID, _ = res.LastInsertId()
		seasonName = req.Name

		if _, err := tx.Exec(
			`INSERT INTO season_teams (season_id, team_id, season_name) VALUES (?,?,?)`,
			sid, teamID, seasonName); err != nil {
			jsonError(w, err.Error(), 500)
			return
		}
	} else {
		jsonError(w, "provide from_team_id (copy) or name (new team)", 400)
		return
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	markStaleIfScheduled(sid)

	row := db.DB.QueryRow(seasonTeamSelect+` WHERE st.season_id=? AND st.team_id=?`, sid, teamID)
	st, _ := scanSeasonTeam(row)
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, st)
}

// updateSeasonTeamRequest is the PUT body for season team metadata.
type updateSeasonTeamRequest struct {
	SeasonName string `json:"season_name"`
	CaptainID  *int64 `json:"captain_id"`
}

func updateSeasonTeam(w http.ResponseWriter, r *http.Request) {
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
	if !isDraftSeason(sid) {
		jsonError(w, "cannot modify teams in an active season", 422)
		return
	}
	var req updateSeasonTeamRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
		return
	}
	req.SeasonName = strings.TrimSpace(req.SeasonName)
	if req.SeasonName == "" {
		jsonError(w, "season_name is required", 400)
		return
	}
	// If captain is being set, verify they are on the season roster.
	if req.CaptainID != nil {
		var onRoster int
		db.DB.QueryRow(
			`SELECT COUNT(*) FROM season_rosters WHERE season_id=? AND team_id=? AND player_id=?`,
			sid, tid, *req.CaptainID).Scan(&onRoster)
		if onRoster == 0 {
			jsonError(w, "captain must be on this team's season roster", 400)
			return
		}
	}
	res, err := db.DB.Exec(
		`UPDATE season_teams SET season_name=?, captain_id=? WHERE season_id=? AND team_id=?`,
		req.SeasonName, req.CaptainID, sid, tid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "team not found in this season", 404)
		return
	}
	row := db.DB.QueryRow(seasonTeamSelect+` WHERE st.season_id=? AND st.team_id=?`, sid, tid)
	st, _ := scanSeasonTeam(row)
	jsonOK(w, st)
}

func removeSeasonTeam(w http.ResponseWriter, r *http.Request) {
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
	if !isDraftSeason(sid) {
		jsonError(w, "cannot modify teams in an active season", 422)
		return
	}

	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	// Remove from season_rosters first (cascade would handle ON DELETE CASCADE
	// from seasons, but here we delete from season_teams which doesn't cascade).
	tx.Exec(`DELETE FROM season_rosters WHERE season_id=? AND team_id=?`, sid, tid)
	res, err := tx.Exec(`DELETE FROM season_teams WHERE season_id=? AND team_id=?`, sid, tid)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		jsonError(w, "team not found in this season", 404)
		return
	}

	// Delete the teams record only when the team has never appeared in any match.
	var matchCount int
	tx.QueryRow(
		`SELECT COUNT(*) FROM matches WHERE home_team_id=? OR away_team_id=?`, tid, tid,
	).Scan(&matchCount)
	if matchCount == 0 {
		// Also check other seasons' season_teams (team may exist in another draft).
		var otherSeason int
		tx.QueryRow(`SELECT COUNT(*) FROM season_teams WHERE team_id=?`, tid).Scan(&otherSeason)
		if otherSeason == 0 {
			tx.Exec(`UPDATE players SET team_id=NULL WHERE team_id=?`, tid)
			tx.Exec(`DELETE FROM teams WHERE id=?`, tid)
		}
	}

	if err := tx.Commit(); err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	markStaleIfScheduled(sid)
	jsonOK(w, map[string]string{"status": "removed"})
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

func addRosterPlayer(w http.ResponseWriter, r *http.Request) {
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
	if !isDraftSeason(sid) {
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

func removeRosterPlayer(w http.ResponseWriter, r *http.Request) {
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
	if !isDraftSeason(sid) {
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

	// Correction 4: return all active system players not already rostered in this
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

type previousSeasonResponse struct {
	Season *models.Season      `json:"season"`
	Teams  []previousTeamEntry `json:"teams"`
}

type previousTeamEntry struct {
	TeamID     int64  `json:"team_id"`
	TeamName   string `json:"team_name"`
	SeasonName string `json:"season_name"`
	CaptainID  *int64 `json:"captain_id"`
}

func getPreviousSeasonTeams(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}

	var leagueID int64
	var startDate *string
	if err := db.DB.QueryRow(`SELECT league_id, start_date FROM seasons WHERE id=?`, sid).
		Scan(&leagueID, &startDate); err != nil {
		jsonError(w, "season not found", 404)
		return
	}
	startDate = normDatePtr(startDate)

	prev, err := seasons.PreviousSeason(db.DB, sid, leagueID, startDate)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}

	resp := previousSeasonResponse{Season: prev, Teams: []previousTeamEntry{}}
	if prev == nil {
		jsonOK(w, resp)
		return
	}

	// Prefer season_teams from the previous season; fall back to match participants.
	var stCount int
	db.DB.QueryRow(`SELECT COUNT(*) FROM season_teams WHERE season_id=?`, prev.ID).Scan(&stCount)

	if stCount > 0 {
		rows, err := db.DB.Query(`
			SELECT st.team_id, t.name,
			       CASE WHEN st.season_name != '' THEN st.season_name ELSE t.name END,
			       st.captain_id
			FROM season_teams st JOIN teams t ON t.id=st.team_id
			WHERE st.season_id=? ORDER BY st.id`, prev.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var e previousTeamEntry
				rows.Scan(&e.TeamID, &e.TeamName, &e.SeasonName, &e.CaptainID)
				resp.Teams = append(resp.Teams, e)
			}
		}
	} else {
		// Fall back: distinct teams that appeared in the prior season's matches.
		rows, err := db.DB.Query(`
			SELECT DISTINCT t.id, t.name FROM teams t
			JOIN matches m ON (m.home_team_id=t.id OR m.away_team_id=t.id)
			WHERE m.season_id=? ORDER BY t.name`, prev.ID)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var e previousTeamEntry
				rows.Scan(&e.TeamID, &e.TeamName)
				e.SeasonName = e.TeamName
				resp.Teams = append(resp.Teams, e)
			}
		}
	}

	jsonOK(w, resp)
}

// ── Setup Checklist ────────────────────────────────────────────────────────────

func getSeasonChecklist(w http.ResponseWriter, r *http.Request) {
	sid, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", 400)
		return
	}
	c, err := seasons.Checklist(db.DB, sid)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			jsonError(w, err.Error(), 404)
		} else {
			jsonError(w, err.Error(), 500)
		}
		return
	}
	jsonOK(w, c)
}
