package handlers

import (
	"net/http"

	"league_app/backend/domains/auth"
)

// registerAuthRoutes mounts the Users/Roles Phase 1 login/logout/me/
// password-setup endpoints, plus the system-admin account-administration
// actions needed to test this phase (provision, issue setup token,
// grant/revoke roles, activate/deactivate, revoke sessions/keys). Only
// called when deps.AuthMgr is non-nil, so a deployment that has not wired
// the new auth domain (e.g. a minimal test setup) keeps working exactly
// as before, API keys only.
func registerAuthRoutes(mux *http.ServeMux, deps Dependencies) {
	if deps.AuthMgr == nil {
		return
	}

	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		loginHandler(w, r, deps.AuthMgr, deps.InsecureLocalCookies)
	})
	mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
		logoutHandler(w, r, deps.AuthMgr, deps.InsecureLocalCookies)
	})
	mux.HandleFunc("GET /api/auth/me", func(w http.ResponseWriter, r *http.Request) {
		meHandler(w, r, deps.AuthMgr)
	})
	mux.HandleFunc("POST /api/auth/password-setup", func(w http.ResponseWriter, r *http.Request) {
		passwordSetupHandler(w, r, deps.AuthMgr)
	})

	if deps.AuthUserMgr == nil || deps.RoleAssignmentMgr == nil {
		return
	}

	mux.HandleFunc("POST /api/auth/admin/users",
		requireAction(deps, auth.ActionSystemUserAdmin, noScope, func(w http.ResponseWriter, r *http.Request) {
			provisionUserHandler(w, r, deps.AuthUserMgr)
		}),
	)
	mux.HandleFunc("POST /api/auth/admin/users/{id}/setup-token",
		requireAction(deps, auth.ActionSystemUserAdmin, noScope, func(w http.ResponseWriter, r *http.Request) {
			issueSetupTokenHandler(w, r, deps.AuthMgr)
		}),
	)
	mux.HandleFunc("POST /api/auth/admin/users/{id}/deactivate",
		requireAction(deps, auth.ActionSystemUserAdmin, noScope, func(w http.ResponseWriter, r *http.Request) {
			deactivateUserHandler(w, r, deps.AuthMgr, deps.APIKeyMgr)
		}),
	)
	mux.HandleFunc("POST /api/auth/admin/users/{id}/reactivate",
		requireAction(deps, auth.ActionSystemUserAdmin, noScope, func(w http.ResponseWriter, r *http.Request) {
			reactivateUserHandler(w, r, deps.AuthMgr)
		}),
	)
	mux.HandleFunc("POST /api/auth/admin/users/{id}/revoke-api-keys",
		requireAction(deps, auth.ActionSystemUserAdmin, noScope, func(w http.ResponseWriter, r *http.Request) {
			revokeAPIKeysHandler(w, r, deps.APIKeyMgr)
		}),
	)
	mux.HandleFunc("GET /api/auth/admin/users/{id}/roles",
		requireAction(deps, auth.ActionSystemUserAdmin, noScope, func(w http.ResponseWriter, r *http.Request) {
			listRoleAssignmentsHandler(w, r, deps.RoleAssignmentMgr)
		}),
	)
	mux.HandleFunc("POST /api/auth/admin/users/{id}/roles",
		requireAction(deps, auth.ActionSystemUserAdmin, noScope, func(w http.ResponseWriter, r *http.Request) {
			grantRoleHandler(w, r, deps.RoleAssignmentMgr)
		}),
	)
	mux.HandleFunc("POST /api/auth/admin/users/{id}/roles/revoke",
		requireAction(deps, auth.ActionSystemUserAdmin, noScope, func(w http.ResponseWriter, r *http.Request) {
			revokeRoleHandler(w, r, deps.RoleAssignmentMgr)
		}),
	)
}
