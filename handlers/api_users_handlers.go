package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"league_app/models"
)

// Users ---------------------------------------------------------------------

// creatableUserRoles are the roles a new user may be created with via
// POST /api/users. "admin" is a legacy alias kept valid on existing rows
// but is not offered for new creations (Users Admin Screen Phase 1).
// "player" was added by Player Account Access Phase 1 -- it requires a
// linked player_id (see postUser), unlike the two admin roles.
var creatableUserRoles = map[string]bool{
	"system_admin": true,
	"league_admin": true,
	"player":       true,
}

// postUser handles POST /api/users. Creates a new user with an explicit role
// and returns the one-time cleartext API key. Gated by
// requireAdminTokenOrSystemAdminAuth (static admin token, kept for
// bootstrap, or a resolved system_admin/admin personal key). For
// role="player" (Player Account Access Phase 1), body.PlayerID is required
// and validated against playerMgr before the user is created -- the
// resulting user can view only that one player's own Player Overview.
func postUser(w http.ResponseWriter, r *http.Request, auth ApplyAuthResolver, playerMgr PlayerManager) {
	var body struct {
		Username string `json:"username"`
		Role     string `json:"role"`
		PlayerID int64  `json:"player_id"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(body.Username) == "" {
		jsonError(w, "username is required", http.StatusBadRequest)
		return
	}
	if !creatableUserRoles[body.Role] {
		jsonError(w, "role must be system_admin, league_admin, or player", http.StatusBadRequest)
		return
	}

	if body.Role == "player" {
		if body.PlayerID == 0 {
			jsonError(w, "player_id is required for role=player", http.StatusBadRequest)
			return
		}
		if _, err := playerMgr.GetPlayer(r.Context(), body.PlayerID); err != nil {
			jsonError(w, "player_id does not reference an existing player", http.StatusBadRequest)
			return
		}
		user, key, err := auth.CreateApplyPlayerUser(r.Context(), body.Username, body.PlayerID)
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
		return
	}

	user, key, err := auth.CreateApplyUser(r.Context(), body.Username, body.Role)
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
// Gated by requireAdminTokenOrSystemAdminAuth.
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

// getMe handles GET /api/users/me. Returns the user resolved from the
// caller's own personal API key -- any active user, no role restriction,
// since the point of this endpoint is only "who am I." Gated by
// requirePersonalKeyAuth.
func getMe(w http.ResponseWriter, r *http.Request) {
	user := clearanceUserFromContext(r.Context())
	if user == nil {
		jsonError(w, "forbidden", http.StatusForbidden)
		return
	}
	jsonOK(w, user)
}
