package handlers

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"

	"league_app/backend/domains/auth"
	"league_app/models"
)

// Cookie names for the Users/Roles Phase 1 session model. sessionCookieName
// is HttpOnly (never read by JS); csrfCookieName deliberately is not, so
// the shared frontend API client can read it and mirror it into the
// X-CSRF-Token header on mutating requests (see web/lib/api-client.js).
const (
	sessionCookieName = "session_token"
	csrfCookieName    = "csrf_token"
	csrfHeaderName    = "X-CSRF-Token"
)

// resolveIdentity determines the caller's auth.Identity from whichever
// credential is present. CREDENTIAL PRECEDENCE (PM correction, Users/Roles
// Phase 1 final round): an active password session ALWAYS takes precedence
// over a Bearer API key. The session cookie is checked first; a Bearer
// header is only consulted as a fallback when no session cookie resolves.
// This is the one place that decides "who is this request," so this
// ordering is the entire contract -- no other code path may re-derive or
// override it. The reason this matters: GET /api/auth/me (what the shell
// displays) already preferred the session, but this function previously
// tried Bearer FIRST, so a stale/different Admin Key left in a browser
// tab's sessionStorage from an earlier test could execute mutations as
// that key's identity while the shell displayed the session's identity --
// a split identity. Returns (nil, nil, nil) when neither credential
// resolves -- callers treat that as "authentication required," not an
// error. The returned *auth.Session is non-nil only for the session-cookie
// path, so callers can verify CSRF against it for mutating requests -- and
// since the session path is now checked (and, if valid, always taken)
// first, CSRF is still enforced on a session-authenticated mutation even
// when a request also happens to carry a stale Bearer header.
//
// The Bearer path builds its Identity from deps.RoleAssignmentMgr directly
// (buildIdentityFromLegacyUser) rather than requiring deps.AuthMgr --
// AuthMgr (session login/logout) and RoleAssignmentMgr (scoped role
// storage) are separate optional dependencies, and a personal API key must
// keep authorizing correctly even in configurations that wire one without
// the other (e.g. every existing test that only sets deps.ApplyAuth).
func resolveIdentity(r *http.Request, deps Dependencies) (*auth.Identity, *auth.Session, error) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil && cookie.Value != "" && deps.AuthMgr != nil {
		resolved, err := deps.AuthMgr.ResolveSession(r.Context(), cookie.Value)
		if err != nil {
			return nil, nil, err
		}
		if resolved != nil {
			return &resolved.Identity, &resolved.Session, nil
		}
		// Session cookie present but expired/invalid: fall through to a
		// Bearer key, matching "the Admin Key remains the fallback when no
		// session exists" -- an unresolvable session is, for this purpose,
		// no session.
	}

	if authHeader := r.Header.Get("Authorization"); authHeader != "" {
		if !strings.HasPrefix(authHeader, "Bearer ") {
			return nil, nil, nil
		}
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if deps.ApplyAuth == nil {
			return nil, nil, nil
		}
		user, err := deps.ApplyAuth.ResolveApplyUserByAPIKey(r.Context(), token)
		if err != nil {
			return nil, nil, err
		}
		if user == nil {
			return nil, nil, nil
		}
		identity, err := buildIdentityFromLegacyUser(r.Context(), user, deps)
		if err != nil {
			return nil, nil, err
		}
		return &identity, nil, nil
	}

	return nil, nil, nil
}

// buildIdentityFromLegacyUser builds a full auth.Identity for a resolved
// Bearer-key *models.User. When deps.RoleAssignmentMgr is wired (always
// true in production -- see main.go), the identity's Assignments come
// straight from the real, per-league role_assignments table, exactly like
// a session-authenticated identity -- there is no separate policy here,
// only a separate credential-resolution step feeding the same
// auth.Authorize. When RoleAssignmentMgr is nil, the scoping subsystem was
// never wired at all -- a state that exists only in minimal test
// Dependencies structs (every real deployment wires both together in
// main.go) -- and this falls back to the pre-Phase-1 flat mapping so those
// tests keep their historical pass/fail behavior: admin/system_admin
// become the global system_admin role (matching the old
// requireSystemAdminRole/requireLeagueAdminRole treatment of "admin" as a
// system_admin-tier alias), and league_admin becomes system_admin too,
// since the old flat model granted league_admin access to every league
// unconditionally, which auth.Authorize's scoped league_admin case cannot
// otherwise express without a real per-league assignment.
func buildIdentityFromLegacyUser(ctx context.Context, user *models.User, deps Dependencies) (auth.Identity, error) {
	identity := auth.Identity{
		UserID:   user.ID,
		Username: user.Username,
		Active:   user.Active,
		PlayerID: user.PlayerID,
	}
	if deps.RoleAssignmentMgr != nil {
		assignments, err := deps.RoleAssignmentMgr.ListForUser(ctx, user.ID)
		if err != nil {
			return auth.Identity{}, err
		}
		identity.Assignments = assignments
		return identity, nil
	}
	switch user.Role {
	case "system_admin", "admin", "league_admin":
		identity.Assignments = []auth.Assignment{{RoleCode: auth.RoleSystemAdmin}}
	}
	return identity, nil
}

// credentialPresented reports whether the request carries any credential
// at all (an Authorization header or a session cookie), regardless of
// whether that credential actually resolves to anyone. Used to preserve
// the pre-Phase-1 distinction between "no credential" (401, with
// WWW-Authenticate -- matches RFC 7235) and "credential present but
// invalid/unresolved" (403) that requirePersonalKeyAuth and every
// clearanceAuth-protected route's tests already depend on.
func credentialPresented(r *http.Request) bool {
	if r.Header.Get("Authorization") != "" {
		return true
	}
	if c, err := r.Cookie(sessionCookieName); err == nil && c.Value != "" {
		return true
	}
	return false
}

// verifyCSRF checks the X-CSRF-Token header against sess's stored hash
// using a constant-time comparison. Only ever called for session-cookie-
// authenticated mutating requests -- Bearer-key requests never reach this
// check (see requireAction), since a custom Authorization header cannot be
// attached by a browser to a cross-site request the way an ambient cookie
// can, making Bearer auth inherently CSRF-immune.
func verifyCSRF(r *http.Request, sess *auth.Session) bool {
	header := r.Header.Get(csrfHeaderName)
	if header == "" {
		return false
	}
	sum := sha256.Sum256([]byte(header))
	got := hex.EncodeToString(sum[:])
	return subtle.ConstantTimeCompare([]byte(got), []byte(sess.CSRFTokenHash)) == 1
}

func isMutatingMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// identityFromContext returns the auth.Identity resolved by requireAction
// for this request, or nil if requireAction was not used (e.g. a route
// still on the legacy clearanceAuth chain).
func identityFromContext(ctx context.Context) *auth.Identity {
	id, _ := ctx.Value(identityContextKey{}).(auth.Identity)
	if id.UserID == 0 {
		return nil
	}
	return &id
}

// requireAction is the HTTP-layer bridge to auth.Authorize -- the single
// centralized authorization policy entry point. It resolves the caller's
// identity (session cookie takes precedence over a Bearer key, see
// resolveIdentity), enforces CSRF on session-cookie-authenticated
// mutations, resolves the request's
// target scope via scopeFn, and calls auth.Authorize. On success the
// resolved identity is stored in context for the handler (see
// identityFromContext) -- e.g. league creation needs the acting identity
// to grant the atomic self-scope.
//
// scopeFn resolves the request's authoritative scope from whatever
// identifier the route actually carries (a league_id path param directly,
// or a season_id/team_id resolved to its owning league via the relevant
// manager) -- this is deliberately the caller's responsibility, not
// requireAction's, so auth.Authorize itself stays a pure function with no
// database access.
func requireAction(deps Dependencies, action auth.Action, scopeFn func(r *http.Request) (auth.Scope, bool, error), next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		identity, sess, err := resolveIdentity(r, deps)
		if err != nil {
			jsonError(w, "internal error", http.StatusInternalServerError)
			return
		}
		if identity == nil {
			// Matches the pre-Phase-1 requirePersonalKeyAuth distinction:
			// no credential at all -> 401 (RFC 7235, WWW-Authenticate set);
			// a credential was presented but did not resolve -> 403. Many
			// existing route tests assert this exact split.
			if credentialPresented(r) {
				jsonError(w, "forbidden", http.StatusForbidden)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="league-admin"`)
			jsonError(w, "authentication required", http.StatusUnauthorized)
			return
		}
		if sess != nil && isMutatingMethod(r.Method) {
			if !verifyCSRF(r, sess) {
				jsonError(w, "forbidden", http.StatusForbidden)
				return
			}
		}

		scope, ok, err := scopeFn(r)
		if err != nil {
			jsonError(w, "invalid id", http.StatusBadRequest)
			return
		}
		if !ok {
			jsonError(w, "not found", http.StatusNotFound)
			return
		}
		if !auth.Authorize(*identity, action, scope) {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}

		ctx := context.WithValue(r.Context(), identityContextKey{}, *identity)
		next(w, r.WithContext(ctx))
	}
}

// noScope is a scopeFn for actions that need no resource scope at all
// (e.g. league creation itself, or global system_admin actions) -- it
// always succeeds with an empty Scope.
func noScope(*http.Request) (auth.Scope, bool, error) {
	return auth.Scope{}, true, nil
}

// leagueIDPathScope is a scopeFn for routes where the league is directly
// the {id} path parameter (e.g. PUT/DELETE /api/leagues/{id}).
func leagueIDPathScope(r *http.Request) (auth.Scope, bool, error) {
	id, err := pathID(r, "id")
	if err != nil {
		return auth.Scope{}, false, err
	}
	return auth.Scope{LeagueID: &id}, true, nil
}

// legacyLeagueAdminRoles and legacySystemAdminRoles are the pre-Phase-1
// flat role sets, preserved only for guardedAction's fallback branch (see
// its doc comment) -- they mirror requireLeagueAdminRole and
// requireSystemAdminRole exactly.
var (
	legacyLeagueAdminRoles = map[string]bool{"league_admin": true, "admin": true, "system_admin": true}
	legacySystemAdminRoles = map[string]bool{"admin": true, "system_admin": true}
)

// legacyFlatRoleAuth is the pre-Phase-1 Bearer-only, unscoped role gate:
// resolve a personal API key, then require the resolved user's flat role
// string to be in allowedRoles. Used only by guardedAction's fallback
// branch, for Dependencies configurations that never wired
// RoleAssignmentMgr at all (every real deployment wires it in main.go;
// this exists so minimal test Dependencies structs keep their historical
// pass/fail behavior without duplicating a second authorization policy
// for real traffic).
func legacyFlatRoleAuth(resolver ApplyAuthResolver, allowedRoles map[string]bool, next http.HandlerFunc) http.HandlerFunc {
	return requirePersonalKeyAuth(resolver, func(w http.ResponseWriter, r *http.Request) {
		user := clearanceUserFromContext(r.Context())
		if user == nil || !allowedRoles[user.Role] {
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	})
}

// guardedAction is the single route-auth gate used by every mutation this
// phase's PM review required to move onto session-or-Bearer, scope-aware
// authorization (replacing the old clearanceAuth/systemAdminAuth call
// sites one for one). It resolves to exactly one of three behaviors,
// gated on TWO independent conditions -- PM correction: the original
// version gated everything on `deps.ApplyAuth == nil` alone, which meant
// a partially wired auth stack (session support fully configured, but no
// Bearer resolver) fell through to "fully open" instead of enforcing the
// session auth that WAS configured. The distinguishing condition is now
// explicit: only a Dependencies with NOTHING auth-related wired at all
// falls open; wiring even one piece (AuthMgr or RoleAssignmentMgr) is
// treated as a real, if partial, auth configuration that must fail
// closed, never open.
//
//   - deps.ApplyAuth == nil AND deps.AuthMgr == nil AND
//     deps.RoleAssignmentMgr == nil: the auth subsystem was never wired
//     into this Dependencies AT ALL -- the route is returned unmodified
//     (fully open). This is the exact, and only, condition that
//     identifies a deliberately minimal test setup exercising unrelated
//     handler logic (every clearanceAuth/systemAdminAuth-era test in this
//     package that predates session support uses this shape). It is
//     never true in production -- main.go always wires all three
//     together.
//   - deps.AuthMgr == nil AND deps.RoleAssignmentMgr == nil (but
//     ApplyAuth is wired): session support and scoping were BOTH never
//     wired -- exactly the shape of a pre-session-support test
//     Dependencies (ApplyAuth only). Falls back to legacyFlatRoleAuth,
//     the historical flat Bearer-only role check, for those tests'
//     historical pass/fail behavior. Never true in production.
//   - otherwise -- including a Dependencies with AuthMgr and/or
//     RoleAssignmentMgr wired but ApplyAuth nil, a real (if unusual)
//     session-only configuration: every request, whether authenticated
//     by session cookie or Bearer API key, is routed through
//     requireAction -> auth.Authorize. resolveIdentity already handles a
//     nil ApplyAuth gracefully for the Bearer path (it simply never
//     resolves a Bearer credential, exactly as if none were presented),
//     so session authentication works correctly even when Bearer support
//     is absent -- there is no code path here that silently opens the
//     route.
func guardedAction(deps Dependencies, action auth.Action, scopeFn func(r *http.Request) (auth.Scope, bool, error), allowedLegacyRoles map[string]bool, next http.HandlerFunc) http.HandlerFunc {
	if deps.ApplyAuth == nil && deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
		return next
	}
	if deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
		return legacyFlatRoleAuth(deps.ApplyAuth, allowedLegacyRoles, next)
	}
	return requireAction(deps, action, scopeFn, next)
}

// guardedLeagueAdminAction is guardedAction with the standard
// league_admin/admin/system_admin legacy role set -- the direct
// replacement for every clearanceAuth(applyAuth, ...) call site.
func guardedLeagueAdminAction(deps Dependencies, action auth.Action, scopeFn func(r *http.Request) (auth.Scope, bool, error), next http.HandlerFunc) http.HandlerFunc {
	return guardedAction(deps, action, scopeFn, legacyLeagueAdminRoles, next)
}

// guardedSystemAdminAction is guardedAction with the system-admin-only
// legacy role set -- the direct replacement for every
// systemAdminAuth(applyAuth, ...) call site.
func guardedSystemAdminAction(deps Dependencies, action auth.Action, scopeFn func(r *http.Request) (auth.Scope, bool, error), next http.HandlerFunc) http.HandlerFunc {
	return guardedAction(deps, action, scopeFn, legacySystemAdminRoles, next)
}

// staticTokenBypass reports whether r carries deps.AdminToken as its
// Bearer credential -- the static LEAGUE_ADMIN_TOKEN env-var fallback kept
// for bootstrap (creating the very first system_admin user, and
// unattended handicap-apply automation) before any personal key or
// session exists. It is a genuinely separate credential, not a user
// identity, so it cannot be expressed as an auth.Identity/scope and stays
// a bypass rather than flowing through auth.Authorize.
func staticTokenBypass(r *http.Request, adminToken string) bool {
	if adminToken == "" {
		return false
	}
	authHeader := r.Header.Get("Authorization")
	return strings.HasPrefix(authHeader, "Bearer ") && strings.TrimPrefix(authHeader, "Bearer ") == adminToken
}

// requireApplyAuthOrScopedAction is requireApplyAuth's dual-tier pattern
// (personal key/session first, static admin token as fallback) rewired
// onto guardedLeagueAdminAction for its personal-key/session tier, so a
// session-authenticated or Bearer-key league_admin is scoped to their own
// league exactly like every other guarded route, instead of the old
// flat, Bearer-only clearanceAuth chain. Used by handicap-apply.
func requireApplyAuthOrScopedAction(deps Dependencies, action auth.Action, scopeFn func(r *http.Request) (auth.Scope, bool, error), next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if staticTokenBypass(r, deps.AdminToken) {
			next(w, r)
			return
		}
		if deps.ApplyAuth == nil && deps.AuthMgr == nil && deps.RoleAssignmentMgr == nil {
			// No auth subsystem wired at all -- unlike guardedAction's
			// other call sites, the Apply route's historical contract
			// (requireApplyAuth) always enforces the static-token tier
			// even with nothing else wired, so this must not fall through
			// to "fully open." This mirrors guardedAction's own
			// fully-open condition exactly (PM correction: the check must
			// be "the entire auth subsystem is absent," not just
			// "ApplyAuth is absent" -- a session-only configuration
			// (AuthMgr/RoleAssignmentMgr wired, ApplyAuth nil) falls
			// through to guardedLeagueAdminAction below instead, where
			// session authentication works normally.
			if r.Header.Get("Authorization") == "" {
				w.Header().Set("WWW-Authenticate", `Bearer realm="league-admin"`)
				jsonError(w, "authentication required", http.StatusUnauthorized)
				return
			}
			jsonError(w, "forbidden", http.StatusForbidden)
			return
		}
		guardedLeagueAdminAction(deps, action, scopeFn, next)(w, r)
	}
}

// requireAdminTokenOrGuardedSystemAdminAction is
// requireAdminTokenOrSystemAdminAuth's dual-tier pattern rewired onto
// guardedSystemAdminAction for its personal-key/session tier, so a
// session-authenticated system_admin can provision/list users, not only a
// Bearer personal key. Used by POST/GET /api/users.
func requireAdminTokenOrGuardedSystemAdminAction(deps Dependencies, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if staticTokenBypass(r, deps.AdminToken) {
			next(w, r)
			return
		}
		guardedSystemAdminAction(deps, auth.ActionSystemUserAdmin, noScope, next)(w, r)
	}
}
