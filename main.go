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

	"league_app/db"
	"league_app/handlers"
)

//go:embed web
var webFiles embed.FS

//go:embed scripts/seed.sql
var seedSQL string

func main() {
	port    := flag.Int("port", 8080, "HTTP port to listen on")
	dataDir := flag.String("data", defaultDataDir(), "Directory for the SQLite database and backups")
	seed    := flag.Bool("seed", false, "Load starter data (leagues, teams, players) then exit")
	resetDB := flag.Bool("reset-db", false, "Delete and recreate the database (WARNING: erases all data)")
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
		return
	}

	// Serve embedded web files
	webRoot, err := fs.Sub(webFiles, "web")
	if err != nil {
		log.Fatalf("web embed: %v", err)
	}

	mux := http.NewServeMux()

	// API routes
	handlers.Register(mux, *dataDir)

	// Static files (SPA)
	mux.Handle("/", http.FileServer(http.FS(webRoot)))

	addr := fmt.Sprintf(":%d", *port)
	url  := fmt.Sprintf("http://localhost:%d", *port)
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
