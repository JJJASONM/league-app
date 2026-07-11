
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
  }
}

// --- API helpers --------------------------------------------------------------
async function api(method, path, body) {
  const opts = { method, headers: {'Content-Type':'application/json'} };
  if (body !== undefined) opts.body = JSON.stringify(body);
  const res = await fetch('/api' + path, opts);
  const data = await res.json();
  if (!res.ok) {
    if (Array.isArray(data.messages) && data.messages.length > 0) {
      const errs = data.messages.filter(m => m.level === 'error');
      const list = (errs.length ? errs : data.messages).map(m => m.message).join('; ');
      throw new Error(list);
    }
    throw new Error(data.error || 'Request failed');
  }
  return data;
}

function escapeHTML(value) {
  return String(value ?? '').replace(/[&<>"']/g, ch => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  }[ch]));
}

// Returns "?league_id=X" or "" if no active league
function lid() {
  return appContext.getLeagueQuery();
}
// Returns "&league_id=X" for appending to existing query strings
function lidAmp() {
  return appContext.getLeagueQueryAmp();
}

// --- Toast --------------------------------------------------------------------
function toast(msg, type='success') {
  const el = document.createElement('div');
  el.className = `toast align-items-center text-bg-${type} border-0 show mb-2`;
  el.setAttribute('role','alert');
  el.innerHTML = `<div class="d-flex"><div class="toast-body">${msg}</div>
    <button type="button" class="btn-close btn-close-white me-2 m-auto" data-bs-dismiss="toast"></button></div>`;
  document.getElementById('toast-container').appendChild(el);
  setTimeout(() => el.remove(), 3500);
}

function openModal(id)  { new bootstrap.Modal(document.getElementById(id)).show(); }
function closeModal(id) { bootstrap.Modal.getInstance(document.getElementById(id))?.hide(); }

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
