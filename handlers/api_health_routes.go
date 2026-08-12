package handlers

import (
	"net/http"

	"league_app/db"
)

// registerHealthRoute mounts the health-check route onto mux.
// GET /healthz is unauthenticated and lightweight: it reports 200 when the
// process is running and the database connection is usable, or 503 when the
// database is not reachable. Suitable for staging/IIS health checks.
func registerHealthRoute(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := db.DB.PingContext(r.Context()); err != nil {
			jsonError(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		jsonOK(w, map[string]string{"status": "ok"})
	})
}
