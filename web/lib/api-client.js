// Methods where a bodyless request can be rejected by a reverse proxy in
// front of the app (e.g. staging's IIS returns 411 Length Required for a
// bodyless POST) before it ever reaches the Go server. GET and DELETE are
// unaffected -- confirmed against staging -- so only these three get a
// synthesized empty body when the caller passes none.
const BODY_REQUIRED_METHODS = ['POST', 'PUT', 'PATCH'];

// Methods a session-cookie-authenticated request must attach the
// X-CSRF-Token header to (Users/Roles Phase 1). Bearer-key requests never
// need this -- a custom Authorization header cannot be attached to a
// cross-site request the way an ambient cookie can, so that path is
// inherently CSRF-immune and unaffected by any of this.
const CSRF_REQUIRED_METHODS = ['POST', 'PUT', 'PATCH', 'DELETE'];

// readCsrfCookie reads the JS-readable csrf_token cookie the server sets
// alongside the HttpOnly session cookie at login (and clears at logout).
// Returns '' whenever no session is active -- this doubles as this
// client's synchronous "is a session currently active" signal, since the
// cookie's lifetime is tied exactly to the session's (see setAuthCookies/
// clearAuthCookies in handlers/api_auth_handlers.go).
function readCsrfCookie() {
  const match = document.cookie.match(/(?:^|; )csrf_token=([^;]*)/);
  return match ? decodeURIComponent(match[1]) : '';
}

// CREDENTIAL PRECEDENCE (Users/Roles Phase 1 final correction): an active
// password session ALWAYS takes precedence over a stored Admin Key. This
// is the one place in the frontend that decides which credential a
// protected request executes as, so it is the entire contract -- no other
// code may attach an Authorization header or decide between the two.
//   - Session active (a csrf_token cookie is present): never attach the
//     Admin Key, even if one remains in sessionStorage from an earlier
//     session/test in this tab. Attach X-CSRF-Token on mutations instead,
//     so the identity the shell displays (GET /auth/me, which itself
//     prefers the session -- see app.js resolveCurrentIdentity) is always
//     the identity that executes the request.
//   - No session active: fall back to the Admin Key (Bearer), matching
//     the legacy/bootstrap/automation behavior.
// Logging in also clears any stored Admin Key outright (see
// web/domains/auth/auth-api-service.js login()) so this precedence is
// belt-and-suspenders, not the only thing preventing a split identity.
async function api(method, path, body) {
  const opts = { method, headers: {'Content-Type': 'application/json'} };
  const csrf = readCsrfCookie();
  const sessionActive = csrf !== '';
  const adminKey = typeof getAdminKey === 'function' ? getAdminKey() : '';
  if (sessionActive) {
    if (CSRF_REQUIRED_METHODS.includes(method)) {
      opts.headers['X-CSRF-Token'] = csrf;
    }
  } else if (adminKey) {
    opts.headers['Authorization'] = `Bearer ${adminKey}`;
  }
  if (body !== undefined) {
    opts.body = JSON.stringify(body);
  } else if (BODY_REQUIRED_METHODS.includes(method)) {
    opts.body = '{}';
  }
  const res = await fetch('/api' + path, opts);
  const data = await res.json();
  if (!res.ok) {
    const usingAdminKey = !sessionActive && !!adminKey;
    if (res.status === 401) {
      throw new Error(usingAdminKey
        ? 'Admin key required for this action. Set one from the sidebar (Admin Key).'
        : 'Please log in to do this.');
    }
    if (res.status === 403) {
      throw new Error(usingAdminKey
        ? 'Admin key was rejected for this action (missing role or invalid key). Set a valid key from the sidebar (Admin Key).'
        : 'You do not have permission to do this.');
    }
    if (Array.isArray(data.messages) && data.messages.length > 0) {
      const errs = data.messages.filter(m => m.level === 'error');
      const list = (errs.length ? errs : data.messages).map(m => m.message).join('; ');
      throw new Error(list);
    }
    throw new Error(data.error || 'Request failed');
  }
  return data;
}
