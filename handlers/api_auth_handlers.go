package handlers

import (
	"net/http"
	"time"

	"league_app/backend/domains/auth"
)

// identityResponse is the shape returned by both /api/auth/login and
// /api/auth/me -- "user id, email, active, player_id/player_name when
// linked, complete role assignments, available workspaces" per the
// Users/Roles Phase 1 spec.
type identityResponse struct {
	User struct {
		ID         int64  `json:"id"`
		Email      string `json:"email"`
		Active     bool   `json:"active"`
		PlayerID   *int64 `json:"player_id,omitempty"`
		PlayerName string `json:"player_name,omitempty"`
	} `json:"user"`
	RoleAssignments []assignmentJSON `json:"role_assignments"`
	Workspaces      []string         `json:"workspaces"`
}

type assignmentJSON struct {
	RoleCode string `json:"role_code"`
	LeagueID *int64 `json:"league_id,omitempty"`
}

func toIdentityResponse(id auth.Identity) identityResponse {
	var resp identityResponse
	resp.User.ID = id.UserID
	resp.User.Email = id.Email
	resp.User.Active = id.Active
	resp.User.PlayerID = id.PlayerID
	resp.User.PlayerName = id.PlayerName
	resp.Workspaces = id.Workspaces()
	if resp.Workspaces == nil {
		resp.Workspaces = []string{}
	}
	for _, a := range id.Assignments {
		resp.RoleAssignments = append(resp.RoleAssignments, assignmentJSON{RoleCode: string(a.RoleCode), LeagueID: a.LeagueID})
	}
	if resp.RoleAssignments == nil {
		resp.RoleAssignments = []assignmentJSON{}
	}
	return resp
}

// setAuthCookies sets the HttpOnly session cookie and the JS-readable CSRF
// cookie together, with matching expiry. Secure is omitted only when
// insecureLocal is true -- an explicit, logged-at-startup opt-in for
// same-computer HTTP testing (see main.go); it must never be true for any
// deployment reachable remotely.
func setAuthCookies(w http.ResponseWriter, sessionToken, csrfToken string, expiresAt time.Time, insecureLocal bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: true,
		Secure:   !insecureLocal,
		SameSite: http.SameSiteLaxMode,
	})
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    csrfToken,
		Path:     "/",
		Expires:  expiresAt,
		HttpOnly: false,
		Secure:   !insecureLocal,
		SameSite: http.SameSiteLaxMode,
	})
}

// clearAuthCookies expires both cookies immediately (logout).
func clearAuthCookies(w http.ResponseWriter, insecureLocal bool) {
	past := time.Unix(0, 0)
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", Expires: past, MaxAge: -1, HttpOnly: true, Secure: !insecureLocal, SameSite: http.SameSiteLaxMode})
	http.SetCookie(w, &http.Cookie{Name: csrfCookieName, Value: "", Path: "/", Expires: past, MaxAge: -1, HttpOnly: false, Secure: !insecureLocal, SameSite: http.SameSiteLaxMode})
}

func loginHandler(w http.ResponseWriter, r *http.Request, authMgr AuthManager, insecureLocal bool) {
	var body struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	result, err := authMgr.Login(r.Context(), body.Email, body.Password, r.UserAgent(), clientIP(r))
	if err != nil {
		switch err {
		case auth.ErrAccountInactive:
			jsonError(w, "account is inactive", http.StatusForbidden)
		default:
			// Generic message for every other failure mode (unknown
			// email, wrong password, no password set) -- deliberately
			// indistinguishable, per the email-enumeration control.
			jsonError(w, "invalid email or password", http.StatusUnauthorized)
		}
		return
	}
	setAuthCookies(w, result.SessionToken, result.CSRFToken, result.ExpiresAt, insecureLocal)
	jsonOK(w, toIdentityResponse(result.Identity))
}

func logoutHandler(w http.ResponseWriter, r *http.Request, authMgr AuthManager, insecureLocal bool) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		_ = authMgr.Logout(r.Context(), cookie.Value)
	}
	clearAuthCookies(w, insecureLocal)
	jsonOK(w, map[string]string{"status": "logged out"})
}

func meHandler(w http.ResponseWriter, r *http.Request, authMgr AuthManager) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" {
		jsonError(w, "not signed in", http.StatusUnauthorized)
		return
	}
	resolved, err := authMgr.ResolveSession(r.Context(), cookie.Value)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	if resolved == nil {
		jsonError(w, "not signed in", http.StatusUnauthorized)
		return
	}
	jsonOK(w, toIdentityResponse(resolved.Identity))
}

func passwordSetupHandler(w http.ResponseWriter, r *http.Request, authMgr AuthManager) {
	var body struct {
		SetupToken  string `json:"setup_token"`
		NewPassword string `json:"new_password"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.SetupToken == "" || len(body.NewPassword) < 8 {
		jsonError(w, "setup_token and a new_password of at least 8 characters are required", http.StatusBadRequest)
		return
	}
	if err := authMgr.CompletePasswordSetup(r.Context(), body.SetupToken, body.NewPassword); err != nil {
		if err == auth.ErrInvalidSetupToken {
			jsonError(w, "invalid or expired setup token", http.StatusBadRequest)
			return
		}
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "password set"})
}

// clientIP extracts a best-effort client address for session audit fields
// only (never used for any authorization or trust decision -- this
// codebase does not sit behind a configured trusted-proxy chain, so
// X-Forwarded-For is deliberately not consulted here).
func clientIP(r *http.Request) string {
	return r.RemoteAddr
}
