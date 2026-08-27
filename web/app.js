
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
    case 'seasons':   document.querySelector('seasons-page')?.refresh(state.activeLeague, state.allSeasons, state.allTeams); break;
    case 'teams':     loadTeams(); break;
    case 'players':   document.querySelector('players-page')?.refresh(state.allTeams, state.activeLeague); break;
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
    case 'users':     document.querySelector('users-management-page')?.refresh(); break;
    case 'player-overview':
      document.querySelector('player-overview-page')?.refresh(
        state.allPlayers,
        state.activeSeason,
        appContext.consumeOverviewPreselect()
      );
      break;
    case 'weekly-summary':
      document.querySelector('weekly-summary-page')?.refresh(state.allSeasons, state.activeSeason);
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

// Users Admin Screen Phase 1: resolves the Admin Key currently set for this
// tab to a real user identity (username/role) via GET /api/users/me, so the
// shell can show "who am I" in the modal and gate the Users nav entry to
// system_admin/admin. Returns the resolved identity, or null when no key is
// set or the key does not resolve (expired, revoked, wrong key) -- a 401/403
// here is an expected, quiet outcome, not an error to toast on its own;
// callers that just saved a key decide how to react to a null result.
async function resolveCurrentIdentity() {
  if (!hasAdminKey()) {
    appContext.setCurrentIdentity(null);
    updateIdentityUI();
    return null;
  }
  let identity = null;
  try {
    identity = await api('GET', '/users/me');
  } catch (_) {
    identity = null;
  }
  appContext.setCurrentIdentity(identity);
  updateIdentityUI();
  return identity;
}

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
  const canManageUsers = !!identity && (identity.role === 'system_admin' || identity.role === 'admin');
  document.getElementById('nav-item-users')?.classList.toggle('d-none', !canManageUsers);
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
