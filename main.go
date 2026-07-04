package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"league_app/backend/domains/handicaps"
	"league_app/backend/domains/leagues"
	"league_app/backend/domains/matches"
	"league_app/backend/domains/rules"
	"league_app/backend/domains/seasons"
	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/handlers"
)

//go:embed web
var webFiles embed.FS

//go:embed scripts/seed.sql
var seedSQL string

func main() {
	port := flag.Int("port", 8080, "HTTP port to listen on")
	dataDir := flag.String("data", defaultDataDir(), "Directory for the SQLite database and backups")
	seed := flag.Bool("seed", false, "Load starter data (leagues, teams, players) then exit")
	resetDB := flag.Bool("reset-db", false, "Delete and recreate the database (WARNING: erases all data)")
	seedScoresheetFixtures := flag.Bool("seed-scoresheet-fixtures", false, "Load opt-in fictional scoresheet fixture data then exit")
	fixtureWeeks := flag.String("fixture-weeks", "", "Scoresheet fixture weeks to create: N for weeks 1..N, or all")
	fixtureWeek := flag.String("fixture-week", "", "Single scoresheet fixture week to create")
	flag.Parse()

	// --reset-db: wipe and recreate
	if *resetDB {
		dbPath := filepath.Join(*dataDir, "league.db")
		if err := os.Remove(dbPath); err != nil && !os.IsNotExist(err) {
			log.Fatalf("reset-db: %v", err)
		}
		log.Println("database deleted — will be recreated fresh.")
	}

	// Initialise database
	if err := db.Init(*dataDir); err != nil {
		log.Fatalf("database init: %v", err)
	}
	log.Printf("database: %s/league.db", *dataDir)

	// --seed: insert starter data and exit
	if *seed {
		if err := db.Seed(seedSQL); err != nil {
			log.Fatalf("seed failed: %v", err)
		}
		log.Println("seed complete — leagues, teams, and players loaded.")
		if !*seedScoresheetFixtures {
			return
		}
	}

	if (*fixtureWeeks != "" || *fixtureWeek != "") && !*seedScoresheetFixtures {
		log.Fatalf("fixture week flags require -seed-scoresheet-fixtures")
	}

	if *seedScoresheetFixtures {
		weeks, err := parseScoresheetFixtureWeeks(*fixtureWeeks, *fixtureWeek)
		if err != nil {
			log.Fatalf("scoresheet fixture options: %v", err)
		}
		summary, err := db.SeedScoresheetFixtures(weeks)
		if err != nil {
			log.Fatalf("scoresheet fixture seed failed: %v", err)
		}
		log.Printf("scoresheet fixtures loaded: league=%q season=%q weeks=%v matches=%d", summary.LeagueName, summary.SeasonName, summary.Weeks, summary.MatchCount)
		return
	}

	// Serve embedded web files
	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("web embed: %v", err)
	}

	mux := http.NewServeMux()

	// Wire domain services
	hcStore := sqlite.NewHandicapStore(db.DB)
	hcSvc := handicaps.NewService(hcStore)
	weekStore := sqlite.NewWeekStore(db.DB)
	ruleStore := sqlite.NewRuleStore(db.DB)
	weekSvc := matches.NewWeekService(weekStore, hcSvc, ruleStore)
	roundStore := sqlite.NewRoundStore(db.DB)
	roundSvc := matches.NewRoundService(roundStore, ruleStore)
	ruleSvc := rules.NewRuleService(ruleStore)
	seasonStore := sqlite.NewSeasonStore(db.DB)
	seasonSvc := seasons.NewSeasonService(seasonStore)
	leagueStore := sqlite.NewLeagueStore(db.DB)
	leagueSvc := leagues.NewLeagueService(leagueStore)
	scheduleStore := sqlite.NewScheduleStore(db.DB)
	scheduleSvc := matches.NewScheduleService(scheduleStore)
	matchStore := sqlite.NewMatchStore(db.DB)
	matchSvc := matches.NewMatchService(matchStore)
	lineupStore := sqlite.NewLineupStore(db.DB)
	lineupSvc := matches.NewLineupService(lineupStore)
	deps := handlers.Dependencies{
		HandicapSvc:     hcSvc,
		HandicapApplier: hcSvc,
		AdminToken:      os.Getenv("LEAGUE_ADMIN_TOKEN"),
		ApplyAuth:       sqlite.NewApplyAuthStore(db.DB),
		WeekMgr:         weekSvc,
		RoundMgr:        roundSvc,
		RuleMgr:         ruleSvc,
		LeagueMgr:       leagueSvc,
		SeasonMgr:       seasonSvc,
		MatchMgr:        matchSvc,
		ScheduleMgr:     scheduleSvc,
		LineupMgr:       lineupSvc,
	}

	// API routes
	handlers.Register(mux, *dataDir, deps)

	// Static files (SPA)
	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	addr := fmt.Sprintf(":%d", *port)
	url := fmt.Sprintf("http://localhost:%d", *port)
	log.Printf("pool league manager running at %s", url)

	// Open browser after a short delay so the server is up
	go func() {
		time.Sleep(300 * time.Millisecond)
		openBrowser(url)
	}()

	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// defaultDataDir returns a sensible data directory next to the executable.
func defaultDataDir() string {
	exe, err := os.Executable()
	if err != nil {
		return "data"
	}
	return filepath.Join(filepath.Dir(exe), "data")
}

func parseScoresheetFixtureWeeks(weeksArg, weekArg string) ([]int, error) {
	const maxFixtureWeek = 5
	if weeksArg != "" && weekArg != "" {
		return nil, fmt.Errorf("use either -fixture-weeks or -fixture-week, not both")
	}
	if weekArg != "" {
		n, err := parsePositiveIntFlag("-fixture-week", weekArg)
		if err != nil {
			return nil, err
		}
		if n > maxFixtureWeek {
			return nil, fmt.Errorf("-fixture-week must be between 1 and %d", maxFixtureWeek)
		}
		return []int{n}, nil
	}
	if weeksArg == "" {
		return []int{1}, nil
	}
	if weeksArg == "all" {
		return []int{1, 2, 3, 4, 5}, nil
	}
	n, err := parsePositiveIntFlag("-fixture-weeks", weeksArg)
	if err != nil {
		return nil, err
	}
	if n > maxFixtureWeek {
		return nil, fmt.Errorf("-fixture-weeks must be between 1 and %d or all", maxFixtureWeek)
	}
	weeks := make([]int, 0, n)
	for i := 1; i <= n; i++ {
		weeks = append(weeks, i)
	}
	return weeks, nil
}

func parsePositiveIntFlag(name, value string) (int, error) {
	var n int
	if _, err := fmt.Sscanf(value, "%d", &n); err != nil {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	if fmt.Sprintf("%d", n) != value {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	if n <= 0 {
		return 0, fmt.Errorf("%s must be greater than zero", name)
	}
	return n, nil
}

// openBrowser launches the system default browser.
func openBrowser(url string) {
	var cmd string
	var args []string
	switch runtime.GOOS {
	case "windows":
		cmd = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	case "darwin":
		cmd = "open"
		args = []string{url}
	default: // linux and others
		cmd = "xdg-open"
		args = []string{url}
	}
	if err := exec.Command(cmd, args...).Start(); err != nil {
		log.Printf("could not open browser: %v — navigate to %s", err, url)
	}
}
