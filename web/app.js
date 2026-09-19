
// --- Navigation ---------------------------------------------------------------
function activateSection(sec) {
  document.querySelectorAll('[data-section]').forEach(l => l.classList.remove('active'));
  document.querySelector(`[data-section="${sec}"]`)?.classList.add('active');
  document.querySelectorAll('.section').forEach(s => s.classList.remove('active'));
  document.getElementById('section-' + sec)?.classList.add('active');
  loadSection(sec);
}

document.querySelectorAll('[data-section]').forEach(link => {
  link.addEventListener('click', e => {
    e.preventDefault();
    activateSection(link.dataset.section);
  });
});

// Sidebar event wiring - shell-owned; registered here rather than as inline HTML attributes.
document.getElementById('league-select')?.addEventListener('change', switchLeague);
document.querySelector('[data-action="manage-leagues"]')?.addEventListener('click', openLeagueModal);
document.querySelector('[data-action="backup"]')?.addEventListener('click', backup);
document.querySelector('[data-action="admin-key"]')?.addEventListener('click', openAdminKeyModal);
document.getElementById('admin-key-save-btn')?.addEventListener('click', saveAdminKey);
document.getElementById('admin-key-clear-btn')?.addEventListener('click', clearAdminKeyAndClose);
document.getElementById('admin-key-input')?.addEventListener('keydown', e => {
  if (e.key === 'Enter') saveAdminKey();
});

function loadSection(sec) {
  const state = appContext.getState();
  if (!state.activeLeague) return;
  switch(sec) {
    case 'dashboard': document.querySelector('dashboard-page')?.refresh(state.activeLeague, state.activeSeason, state.allTeams, state.allPlayers); break;
    case 'league-admin':
      document.querySelector('league-admin-page')?.refresh(
        state.activeLeague, state.activeSeason, state.allSeasons,
        state.allTeams, state.allPlayers, state.currentIdentity
      );
      break;
    case 'seasons':   document.querySelector('seasons-page')?.refresh(state.activeLeague, state.allSeasons, state.allTeams); break;
    case 'teams':     loadTeams(); break;
    case 'players':
      document.querySelector('players-page')?.refresh(
        state.allTeams,
        state.activeLeague,
        hasFinanceAdminRole(state.currentIdentity)
      );
      break;
    case 'schedule':  document.querySelector('schedule-page')?.refresh(state.allSeasons, state.allTeams, state.activeLeague); break;
    case 'lineup':    document.querySelector('lineup-page')?.refresh(state.allSeasons, state.activeSeason, state.allTeams, state.allPlayers); break;
    case 'entry':
      {
        const preselect = appContext.consumeEntryPreselect();
        document.querySelector('match-entry-page')?.refresh(
          state.allSeasons,
          state.activeSeason,
          state.allPlayers,
          state.activeLeague,
          preselect.seasonId,
          preselect.matchId
        );
      }
      break;
    case 'standings': document.querySelector('standings-section')?.refresh(state.allSeasons); break;
    case 'stats':     document.querySelector('stats-section')?.refresh(state.allSeasons); break;
    case 'handicap':  document.querySelector('handicaps-page')?.refresh(state.allSeasons, state.activeSeason); break;
    case 'users':
      document.querySelector('users-management-page')?.refresh(state.allPlayers);
      break;
    case 'player-overview':
      document.querySelector('player-overview-page')?.refresh(
        state.allPlayers,
        state.activeSeason,
        appContext.consumeOverviewPreselect(),
        // Player Account Access Phase 1: a role=player viewer only ever
        // sees their own linked player's overview -- lock the page to
        // that id regardless of which nav entry ("My Overview" or the
        // admin "Player Overview" picker, though the latter is hidden
        // for this role anyway) triggered the navigation.
        isPlayerRole(state.currentIdentity) ? state.currentIdentity.player_id : null
      );
      break;
    case 'weekly-summary':
      document.querySelector('weekly-summary-page')?.refresh(state.allSeasons, state.activeSeason);
      break;
    case 'finances':
      document.querySelector('finances-page')?.refresh(state.allSeasons, state.activeSeason);
      break;
    case 'communications':
      document.querySelector('communication-page')?.refresh(
        state.allSeasons, state.activeSeason, state.allTeams, state.allPlayers
      );
      break;
  }
}


const appContext = window.LeagueAppContext.createShellContext({
  api,
  labelEl: document.getElementById('active-season-label'),
  selectEl: document.getElementById('league-select'),
  storage: window.localStorage,
  toast,
});

// --- League selector ----------------------------------------------------------
async function switchLeague() {
  const id = parseInt(document.getElementById('league-select').value);
  await appContext.switchLeague(id);
  // reload the currently visible section
  const sec = document.querySelector('[data-section].active')?.dataset.section || 'dashboard';
  loadSection(sec);
}

async function loadLeagueData() {
  await appContext.loadLeagueData();
}

// --- Bootstrap ----------------------------------------------------------------
async function init() {
  await appContext.init();
  const state = appContext.getState();
  if (state.activeLeague) {
    document.querySelector('dashboard-page')?.refresh(
      state.activeLeague,
      state.activeSeason,
      state.allTeams,
      state.allPlayers
    );
  }
}
init();



// Cross-domain navigation entry point; delegates to activateSection.
function navTo(sec) { activateSection(sec); }

function openMatchEntry(matchId, seasonId) {
  appContext.setEntryPreselect(seasonId, matchId);
  navTo('entry');
}

function openHandicapForWeek(seasonId, weekNum) {
  navTo('handicap');
  document.querySelector('handicaps-page')?.openForWeek(seasonId, weekNum);
}

function openPlayerOverview(playerId) {
  appContext.setOverviewPreselect(playerId);
  navTo('player-overview');
}

// --- Seasons domain bridge ----------------------------------------------------
// The seasons domain component fires these events; the shell updates cross-domain
// state (allSeasons, activeSeason) and responds to navigation requests.

document.addEventListener('season-state-changed', e => {
  appContext.applySeasonState(e.detail);
});

document.addEventListener('season-nav-request', e => {
  const { section, previewSeasonId, openPoster } = e.detail;
  navTo(section);
  if (previewSeasonId != null) {
    setTimeout(() => {
      const sp = document.querySelector('schedule-page');
      sp?.loadForSeason(previewSeasonId);
      if (openPoster) sp?.openPoster();
    }, 50);
  }
});

document.addEventListener('schedule-data-changed', () => {
  document.querySelector('standings-section')?.reload();
  document.querySelector('stats-section')?.reload();
});

document.addEventListener('players-data-changed', e => {
  appContext.applyPlayersState(e.detail.players);
  const activeSec = document.querySelector('[data-section].active')?.dataset.section;
  if (activeSec === 'teams') loadTeams();
});

document.addEventListener('player-overview-nav-request', e => {
  openPlayerOverview(e.detail.playerId);
});

document.addEventListener('dashboard-nav-request', e => navTo(e.detail.section));

// League Admin Screen Phase 1: the hub fires this to jump to an existing
// operational screen. Same one-line pattern as dashboard-nav-request, kept
// as a distinct name so the hub's intent stays self-documenting.
document.addEventListener('admin-nav-request', e => navTo(e.detail.section));

document.addEventListener('dashboard-refresh-request', async () => {
  await loadLeagueData();
  const state = appContext.getState();
  document.querySelector('dashboard-page')?.refresh(state.activeLeague, state.activeSeason, state.allTeams, state.allPlayers);
});

// --- Teams --------------------------------------------------------------------
function loadTeams() {
  const state = appContext.getState();
  const page = document.querySelector('teams-page');
  if (page) page.refresh(state.activeLeague?.id ?? null, state.activeSeason?.id ?? null);
}

// --- Leagues management modal -------------------------------------------------

function openLeagueModal() {
  document.querySelector('leagues-page')?.openModal(appContext.getState().activeLeague);
}

document.addEventListener('leagues-list-changed', async e => {
  await appContext.applyLeaguesChanged(e.detail);
  const state = appContext.getState();
  if (e.detail.deletedId != null) {
    if (state.activeLeague) await loadLeagueData();
    loadSection('dashboard');
  }
});


async function backup() {
  try {
    const res = await api('POST', '/backup');
    toast('Backup saved: ' + res.path.split(/[/\\]/).pop());
  } catch(e) { toast(e.message,'danger'); }
}

// --- Admin Key modal ------------------------------------------------------------
// Lets an admin paste a personal API key (see web/lib/admin-key-store.js) so the
// shared api() client can attach it to admin mutation requests in this tab.

function updateAdminKeyButton() {
  const label = document.getElementById('admin-key-status');
  if (label) label.textContent = hasAdminKey() ? 'Admin Key (set)' : 'Admin Key';
}

// adaptSessionIdentity converts a Users/Roles Phase 1 GET /api/auth/me
// response ({user:{...}, role_assignments:[...], workspaces:[...]}) into
// the same flat {username, role, player_id} shape every existing nav-
// gating check in this file already understands, so session-based
// identities work through the EXISTING checks (hasFinanceAdminRole,
// isPlayerRole, canManageUsers) unchanged. role becomes 'system_admin' or
// 'league_admin' when any matching assignment exists, else 'player' when
// linked, else ''. The original role_assignments/workspaces are kept
// under _roleAssignments/_workspaces for the workspace switcher, which
// needs the real (possibly dual) capability set, not the collapsed
// single-role view.
function adaptSessionIdentity(sessionIdentity) {
  if (!sessionIdentity) return null;
  const assignments = sessionIdentity.role_assignments || [];
  const isSystemAdmin = assignments.some(a => a.role_code === 'system_admin');
  const isLeagueAdmin = assignments.some(a => a.role_code === 'league_admin');
  let role = '';
  if (isSystemAdmin) role = 'system_admin';
  else if (isLeagueAdmin) role = 'league_admin';
  else if (sessionIdentity.user.player_id != null) role = 'player';
  return {
    username: sessionIdentity.user.email,
    role,
    player_id: sessionIdentity.user.player_id,
    _sessionAuth: true,
    _roleAssignments: assignments,
    _workspaces: sessionIdentity.workspaces || [],
  };
}

// resolveCurrentIdentity resolves "who is using this browser tab" from
// whichever credential is active: a Users/Roles Phase 1 session (tried
// first -- this is now the primary path) or, failing that, the legacy
// Admin Key (Bearer API key) via GET /api/users/me. Returns the resolved
// identity, or null when neither credential is set or resolves -- a
// 401/403 here is an expected, quiet outcome, not an error to toast on
// its own; callers that just logged in / saved a key decide how to react
// to a null result.
async function resolveCurrentIdentity() {
  let identity = null;
  try {
    const sessionIdentity = await api('GET', '/auth/me');
    identity = adaptSessionIdentity(sessionIdentity);
  } catch (_) {
    identity = null;
  }

  if (!identity && hasAdminKey()) {
    try {
      identity = await api('GET', '/users/me');
    } catch (_) {
      identity = null;
    }
  }

  appContext.setCurrentIdentity(identity);
  updateIdentityUI();
  updateAuthGateUI(identity);
  return identity;
}

// updateAuthGateUI shows the login screen (and hides the app shell) when
// no identity resolved at all, per Users/Roles Phase 1: "Unauthenticated
// users see login, not the admin application shell." Once any identity
// resolves (session or legacy Admin Key), the shell is shown. Also drives
// the workspace picker's visibility/options and re-applies the currently
// selected workspace's nav visibility.
function updateAuthGateUI(identity) {
  const loginContainer = document.getElementById('login-screen-container');
  const shell = document.getElementById('app-shell');
  const signOutBtn = document.getElementById('sign-out-btn');
  if (!loginContainer || !shell) return;

  if (!identity) {
    loginContainer.classList.remove('d-none');
    shell.classList.add('d-none');
    return;
  }
  loginContainer.classList.add('d-none');
  shell.classList.remove('d-none');
  signOutBtn?.classList.toggle('d-none', !identity._sessionAuth);

  applyWorkspacePicker(identity);
}

const WORKSPACE_STORAGE_KEY = 'leagueapp.workspace';

function applyWorkspacePicker(identity) {
  const picker = document.getElementById('workspace-picker');
  const select = document.getElementById('workspace-select');
  if (!picker || !select) return;

  const workspaces = (identity && identity._workspaces) || [];
  if (workspaces.length < 2) {
    picker.classList.add('d-none');
    applyWorkspace(workspaces[0] || (isPlayerRole(identity) ? 'player' : 'admin'));
    return;
  }

  picker.classList.remove('d-none');
  const saved = window.localStorage.getItem(WORKSPACE_STORAGE_KEY);
  const initial = workspaces.includes(saved) ? saved : workspaces[0];
  select.value = initial;
  applyWorkspace(initial);
}

// applyWorkspace toggles nav visibility for the chosen workspace.
// Presentation only -- see auth.Authorize on the backend for the actual
// enforcement; this never grants or removes anything by itself.
function applyWorkspace(workspace) {
  window.localStorage.setItem(WORKSPACE_STORAGE_KEY, workspace);
  const identity = appContext.getState().currentIdentity;
  const isPlayerWorkspace = workspace === 'player';

  // Player View: only "My Overview" (already gated to a linked player_id
  // by isPlayerRole); every admin-only nav item is hidden regardless of
  // what this identity would otherwise be allowed to see in Admin View.
  document.getElementById('nav-item-my-overview')?.classList.toggle('d-none', !(isPlayerWorkspace && isPlayerRole(identity)));

  const showAdminNav = !isPlayerWorkspace;
  const canManageFinances = showAdminNav && hasFinanceAdminRole(identity);
  const canManageUsers = showAdminNav && !!identity && (identity.role === 'system_admin' || identity.role === 'admin');
  document.getElementById('nav-item-users')?.classList.toggle('d-none', !canManageUsers);
  document.getElementById('nav-item-finances')?.classList.toggle('d-none', !canManageFinances);
  document.getElementById('nav-item-communications')?.classList.toggle('d-none', !canManageFinances);
  document.getElementById('nav-item-league-admin')?.classList.toggle('d-none', !canManageFinances);
  document.getElementById('nav-item-player-overview')?.classList.toggle('d-none', !canManageFinances);
  document.getElementById('backup-btn')?.classList.toggle('d-none', !canManageUsers);

  if (isPlayerWorkspace) {
    activateSection('player-overview');
  }
}

document.getElementById('workspace-select')?.addEventListener('change', (e) => applyWorkspace(e.target.value));

document.addEventListener('auth-login-success', async () => {
  await resolveCurrentIdentity();
  toast('Signed in');
});

// Login screen's "Use Admin Key instead" link (Users/Roles Phase 1 Phase
// 1 correction): the Admin Key modal and its storage/validation logic are
// unchanged and fully owned by openAdminKeyModal/saveAdminKey/
// clearAdminKeyAndClose below -- this only gives the login screen a way to
// reach it, since #admin-key-btn itself lives inside the hidden #app-shell.
document.addEventListener('use-admin-key-requested', () => {
  openAdminKeyModal();
});

document.getElementById('sign-out-btn')?.addEventListener('click', async () => {
  try {
    await api('POST', '/auth/logout', {});
  } catch (_) {
    // logout is best-effort client-side regardless of server response
  }
  window.localStorage.removeItem(WORKSPACE_STORAGE_KEY);
  await resolveCurrentIdentity();
  toast('Signed out');
});

// hasFinanceAdminRole is the single definition of "can see money data"
// (league_admin/admin/system_admin -- the same role set clearanceAuth
// allows on Financial Phase 1's routes and, since the Phase 2 auth
// correction, Player Overview's route too). Used to gate the Financial
// nav entry, the Player Overview nav entry, and the Players list's
// "View Overview" row action -- one definition shared by all three so
// they cannot drift out of sync with each other or with the backend.
function hasFinanceAdminRole(identity) {
  return !!identity &&
    (identity.role === 'system_admin' || identity.role === 'admin' || identity.role === 'league_admin');
}

// isPlayerRole (Player Account Access Phase 1) is the single definition of
// "this is a player's own account, not an admin's" -- a resolved identity
// with role="player" and a linked player_id. Used to show the "My
// Overview" nav entry and to lock Player Overview to that one player;
// never grants anything hasFinanceAdminRole gates (Users, Financial, the
// admin Player Overview picker, Backup).
function isPlayerRole(identity) {
  return !!identity && identity.role === 'player' && identity.player_id != null;
}

// updateIdentityUI updates the Admin Key modal's status line only. Nav
// visibility is owned entirely by applyWorkspace (Users/Roles Phase 1) --
// see resolveCurrentIdentity/updateAuthGateUI, which call applyWorkspace
// after this runs, so nav state always reflects the current workspace,
// not a second, competing set of rules here.
function updateIdentityUI() {
  const identity = appContext.getState().currentIdentity;
  const statusEl = document.getElementById('admin-key-current-status');
  if (statusEl) {
    statusEl.textContent = identity
      ? `Signed in as ${identity.username} (${identity.role})`
      : hasAdminKey()
        ? 'A key is set for this tab, but it did not resolve to a recognized user.'
        : 'No key is currently set -- admin actions will fail until one is set.';
  }
}

function openAdminKeyModal() {
  const input = document.getElementById('admin-key-input');
  if (input) input.value = '';
  updateIdentityUI();
  new bootstrap.Modal(document.getElementById('admin-key-modal')).show();
}

async function saveAdminKey() {
  const input = document.getElementById('admin-key-input');
  const raw = input?.value.trim();
  if (!raw) { toast('Enter a key, or use Clear to remove the current one', 'warning'); return; }
  // api() already adds the "Bearer " prefix -- strip one here in case a
  // tester pastes the whole Authorization header value by mistake.
  const val = raw.replace(/^Bearer\s+/i, '');
  setAdminKey(val);
  updateAdminKeyButton();
  const identity = await resolveCurrentIdentity();
  if (identity) {
    bootstrap.Modal.getInstance(document.getElementById('admin-key-modal'))?.hide();
    toast('Admin key set for this tab');
  } else {
    // Key is kept (stored) so the user can Clear or replace it, but the
    // modal stays open and the identity status line already shows "did not
    // resolve" (via resolveCurrentIdentity -> updateIdentityUI) -- do not
    // imply success with a green toast and a closed modal.
    toast('That key was not recognized -- admin actions will fail until a valid key is set', 'danger');
  }
}

async function clearAdminKeyAndClose() {
  clearAdminKey();
  updateAdminKeyButton();
  await resolveCurrentIdentity();
  bootstrap.Modal.getInstance(document.getElementById('admin-key-modal'))?.hide();
  toast('Admin key cleared');
}

updateAdminKeyButton();
resolveCurrentIdentity();
