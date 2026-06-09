// Package handlers wires HTTP routes to database operations.
package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"league_app/backend/domains/rules"
	"league_app/db"
	"league_app/logic"
	"league_app/models"
	"log"
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

const seasonCols = `id, league_id, name, start_date, end_date, active, schedule_type, num_weeks, created_at`

func scanSeason(row interface{ Scan(...any) error }) (models.Season, error) {
	var s models.Season
	var active int
	err := row.Scan(&s.ID, &s.LeagueID, &s.Name, &s.StartDate, &s.EndDate,
		&active, &s.ScheduleType, &s.NumWeeks, &s.CreatedAt)
	s.Active = active == 1
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
		`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?,?,?,?,?)`,
		s.LeagueID, s.Name, s.StartDate, s.ScheduleType, s.NumWeeks)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	s.ID, _ = res.LastInsertId()

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
	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()
	tx.Exec(`UPDATE seasons SET active=0 WHERE league_id=?`, leagueID)
	tx.Exec(`UPDATE seasons SET active=1 WHERE id=?`, id)
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
	opts := logic.ScheduleOptions{
		StartDate: startDate,
		SkipDates: skipDates,
		NumWeeks:  req.NumWeeks,
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
			// Use teams that appeared in a prior season's schedule
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
			rows, err := db.DB.Query(`
				SELECT t.id FROM teams t
				JOIN seasons s ON s.league_id = t.league_id
				WHERE s.id=? ORDER BY t.id`, req.SeasonID)
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

	// Update season: schedule_type, num_weeks, end_date
	tx.Exec(`UPDATE seasons SET schedule_type=?, num_weeks=?, end_date=? WHERE id=?`,
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
	res, err := db.DB.Exec(
		`INSERT OR IGNORE INTO bye_requests (season_id, team_id, week_number, reason) VALUES (?,?,?,?)`,
		b.SeasonID, b.TeamID, b.WeekNumber, b.Reason)
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	b.ID, _ = res.LastInsertId()
	w.WriteHeader(http.StatusCreated)
	jsonOK(w, b)
}

func updateByeRequest(w http.ResponseWriter, r *http.Request) {
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
	approved := 0
	if b.Approved {
		approved = 1
	}
	db.DB.Exec(`UPDATE bye_requests SET week_number=?, reason=?, approved=? WHERE id=?`,
		b.WeekNumber, b.Reason, approved, bid)
	b.ID = bid
	jsonOK(w, b)
}

func deleteByeRequest(w http.ResponseWriter, r *http.Request) {
	bid, err := pathID(r, "bid")
	if err != nil {
		jsonError(w, "invalid bye id", 400)
		return
	}
	db.DB.Exec(`DELETE FROM bye_requests WHERE id=?`, bid)
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
	db.DB.Exec(`DELETE FROM match_results WHERE match_id=?`, id)
	db.DB.Exec(`UPDATE matches SET completed=0 WHERE id=?`, id)
	jsonOK(w, map[string]string{"status": "cleared"})
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
		FROM matches WHERE season_id=? AND completed=1`, seasonID)
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
	var req models.SaveRoundsRequest
	if err := decode(r, &req); err != nil {
		jsonError(w, "invalid body", 400)
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

	tx, err := db.DB.Begin()
	if err != nil {
		jsonError(w, err.Error(), 500)
		return
	}
	defer tx.Rollback()

	// Replace all round results for this match
	tx.Exec(`DELETE FROM round_results WHERE match_id=?`, matchID)
	// Look up this season's handicap multiplier once before iterating rounds
	var saveSeasonID int64
	db.DB.QueryRow(`SELECT season_id FROM matches WHERE id=?`, matchID).Scan(&saveSeasonID)
	saveMult := seasonMultiplier(saveSeasonID)

	for _, rr := range req.Rounds {
		spot := logic.CalcSpotM(playerIDs[rr.HomePlayerID], playerIDs[rr.AwayPlayerID], saveMult)
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
	type tally struct{ gw, gl int }
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

	// Replace match_results
	tx.Exec(`DELETE FROM match_results WHERE match_id=?`, matchID)
	for pid, t := range tallies {
		diff := float64(t.gw - t.gl)
		_, err := tx.Exec(`
			INSERT INTO match_results (match_id, player_id, team_id, games_won, games_lost, diff)
			VALUES (?,?,?,?,?,?)`,
			matchID, pid, pTeam[pid], t.gw, t.gl, diff)
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
			SELECT p.id, p.player_number, p.first_name || ' ' || p.last_name,
			       COALESCE(t.name,''), p.handicap,
			       COALESCE(SUM(mr.sets_won),0), COALESCE(SUM(mr.sets_lost),0),
			       COALESCE(SUM(mr.games_won),0), COALESCE(SUM(mr.games_lost),0)
			FROM players p
			JOIN teams t ON t.id = p.team_id
			JOIN seasons s ON s.league_id = t.league_id AND s.id = ?
			LEFT JOIN match_results mr ON mr.player_id = p.id
			LEFT JOIN matches m ON m.id = mr.match_id AND m.season_id = ?
			GROUP BY p.id ORDER BY SUM(mr.sets_won) DESC, SUM(mr.games_won) DESC`
		args = []any{seasonID, seasonID}
	case leagueID != "":
		query = `
			SELECT p.id, p.player_number, p.first_name || ' ' || p.last_name,
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
