// Auth API service (Users/Roles Phase 1): thin wrappers over the shared
// api() client for the login/logout/me/password-setup endpoints. No
// element/UI code here -- see login-page-component.js for that.

// login also clears any Admin Key stored in this tab (Phase 1 final
// correction) so a successful password sign-in never leaves the browser
// in a split session/Bearer state. api()'s own credential precedence
// (session always wins while active -- see web/lib/api-client.js) already
// prevents that split from affecting request execution; this just removes
// the stale key outright rather than leaving it dormant.
export async function login(email, password) {
  const identity = await api('POST', '/auth/login', { email, password });
  if (typeof clearAdminKey === 'function') clearAdminKey();
  return identity;
}

export async function logout() {
  return api('POST', '/auth/logout', {});
}

export async function getCurrentSessionIdentity() {
  try {
    return await api('GET', '/auth/me');
  } catch (_) {
    return null;
  }
}

export async function completePasswordSetup(setupToken, newPassword) {
  return api('POST', '/auth/password-setup', { setup_token: setupToken, new_password: newPassword });
}
