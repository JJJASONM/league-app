package handlers

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"league_app/backend/domains/auth"
)

// usernameSafeChars keeps derived usernames readable and collision-
// resistant without ever being shown as the account's identity (email is).
var usernameUnsafeChars = regexp.MustCompile(`[^a-z0-9._-]+`)

// deriveUsername auto-generates a username from an email's local part,
// purely to satisfy the legacy users.username NOT NULL UNIQUE column --
// per Users/Roles Phase 1 spec, this is never shown as the account's
// identity or accepted by the password-login endpoint.
func deriveUsername(normalizedEmail string) string {
	local := normalizedEmail
	if i := strings.IndexByte(local, '@'); i >= 0 {
		local = local[:i]
	}
	local = usernameUnsafeChars.ReplaceAllString(local, "-")
	local = strings.Trim(local, "-")
	if local == "" {
		local = "user"
	}
	return local
}

func provisionUserHandler(w http.ResponseWriter, r *http.Request, authUsers AuthUserManager) {
	var body struct {
		Email    string `json:"email"`
		PlayerID *int64 `json:"player_id,omitempty"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	email, err := auth.NormalizeEmail(body.Email)
	if err != nil {
		jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	base := deriveUsername(email)
	var record auth.UserRecord
	var lastErr error
	for attempt := 0; attempt < 20; attempt++ {
		username := base
		if attempt > 0 {
			username = fmt.Sprintf("%s-%d", base, attempt+1)
		}
		record, lastErr = authUsers.ProvisionUser(r.Context(), username, email, body.PlayerID)
		if lastErr == nil {
			w.WriteHeader(http.StatusCreated)
			jsonOK(w, record)
			return
		}
		if !strings.Contains(lastErr.Error(), "UNIQUE") {
			break
		}
	}
	if strings.Contains(lastErr.Error(), "UNIQUE") {
		jsonError(w, "an account with that email already exists", http.StatusConflict)
		return
	}
	jsonError(w, "internal error", http.StatusInternalServerError)
}

func issueSetupTokenHandler(w http.ResponseWriter, r *http.Request, authMgr AuthManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	token, err := authMgr.IssuePasswordSetupToken(r.Context(), id)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"setup_token": token})
}

func deactivateUserHandler(w http.ResponseWriter, r *http.Request, authMgr AuthManager, apiKeys auth.APIKeyStore) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := authMgr.Deactivate(r.Context(), id, apiKeys); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "deactivated"})
}

func reactivateUserHandler(w http.ResponseWriter, r *http.Request, authMgr AuthManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := authMgr.Reactivate(r.Context(), id); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "reactivated"})
}

func revokeAPIKeysHandler(w http.ResponseWriter, r *http.Request, apiKeys APIKeyManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := apiKeys.RevokeAllForUser(r.Context(), id); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "api keys revoked"})
}

func grantRoleHandler(w http.ResponseWriter, r *http.Request, roleMgr RoleAssignmentManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		RoleCode string `json:"role_code"`
		LeagueID *int64 `json:"league_id,omitempty"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	roleCode := auth.RoleCode(body.RoleCode)
	if roleCode != auth.RoleSystemAdmin && roleCode != auth.RoleLeagueAdmin {
		jsonError(w, "role_code must be system_admin or league_admin", http.StatusBadRequest)
		return
	}
	if roleCode == auth.RoleLeagueAdmin && body.LeagueID == nil {
		jsonError(w, "league_id is required for role_code=league_admin", http.StatusBadRequest)
		return
	}
	if roleCode == auth.RoleSystemAdmin && body.LeagueID != nil {
		jsonError(w, "league_id must not be set for role_code=system_admin", http.StatusBadRequest)
		return
	}

	var createdBy *int64
	if actor := identityFromContext(r.Context()); actor != nil {
		createdBy = &actor.UserID
	}
	if err := roleMgr.Grant(r.Context(), id, roleCode, body.LeagueID, createdBy); err != nil {
		jsonError(w, "grant rejected: "+err.Error(), http.StatusConflict)
		return
	}
	jsonOK(w, map[string]string{"status": "granted"})
}

func revokeRoleHandler(w http.ResponseWriter, r *http.Request, roleMgr RoleAssignmentManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	var body struct {
		RoleCode string `json:"role_code"`
		LeagueID *int64 `json:"league_id,omitempty"`
	}
	if err := decode(r, &body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := roleMgr.Revoke(r.Context(), id, auth.RoleCode(body.RoleCode), body.LeagueID); err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": "revoked"})
}

func listRoleAssignmentsHandler(w http.ResponseWriter, r *http.Request, roleMgr RoleAssignmentManager) {
	id, err := pathID(r, "id")
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}
	assignments, err := roleMgr.ListForUser(r.Context(), id)
	if err != nil {
		jsonError(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]assignmentJSON, 0, len(assignments))
	for _, a := range assignments {
		out = append(out, assignmentJSON{RoleCode: string(a.RoleCode), LeagueID: a.LeagueID})
	}
	jsonOK(w, out)
}
