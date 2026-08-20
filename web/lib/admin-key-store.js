// Holds the admin's personal API key for this browser tab so the shared
// api() client can attach it to admin mutation requests.
//
// Storage: sessionStorage only (never localStorage) -- the key is gone when
// the tab closes, matching "paste once per browser session." The static
// LEAGUE_ADMIN_TOKEN never appears in browser code; this is always a
// personal key created via POST /api/users.
//
// This is intentionally separate from the Handicap Apply screen's own
// #adminToken field (web/domains/handicaps/handicap-review-component.js),
// which keeps its own narrower, already-working session-memory-only
// mechanism unchanged.

const ADMIN_KEY_STORAGE_KEY = 'leagueapp.adminKey';

function getAdminKey() {
  try {
    return window.sessionStorage.getItem(ADMIN_KEY_STORAGE_KEY) || '';
  } catch (_) {
    return '';
  }
}

function setAdminKey(key) {
  try {
    if (key) window.sessionStorage.setItem(ADMIN_KEY_STORAGE_KEY, key);
    else window.sessionStorage.removeItem(ADMIN_KEY_STORAGE_KEY);
  } catch (_) {
    // sessionStorage unavailable (e.g. disabled by browser settings) --
    // admin actions simply stay unauthenticated for this tab.
  }
}

function clearAdminKey() {
  setAdminKey('');
}

function hasAdminKey() {
  return getAdminKey() !== '';
}
