
// ── State ────────────────────────────────────────────────────────────────────
let allLeagues  = [];
let activeLeague = null;
let allTeams    = [];
let allPlayers  = [];
let allSeasons  = [];
let activeSeason = null;
let _entryPreSelectSeasonId = null;
let _entryPreSelectMatchId = null;

// ── Navigation ───────────────────────────────────────────────────────────────
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

// Sidebar event wiring — shell-owned; registered here rather than as inline HTML attributes.
document.getElementById('league-select')?.addEventListener('change', switchLeague);
document.querySelector('[data-action="manage-leagues"]')?.addEventListener('click', openLeagueModal);
document.querySelector('[data-action="backup"]')?.addEventListener('click', backup);

function loadSection(sec) {
  if (!activeLeague) return;
  switch(sec) {
    case 'dashboard': document.querySelector('dashboard-page')?.refresh(activeLeague, activeSeason, allTeams, allPlayers); break;
    case 'seasons':   document.querySelector('seasons-page')?.refresh(activeLeague, allSeasons, allTeams); break;
    case 'teams':     loadTeams(); break;
    case 'players':   document.querySelector('players-page')?.refresh(allTeams, activeLeague); break;
    case 'schedule':  document.querySelector('schedule-page')?.refresh(allSeasons, allTeams, activeLeague); break;
    case 'lineup':    document.querySelector('lineup-page')?.refresh(allSeasons, activeSeason, allTeams, allPlayers); break;
    case 'entry':
      document.querySelector('match-entry-page')?.refresh(allSeasons, activeSeason, allPlayers, activeLeague, _entryPreSelectSeasonId, _entryPreSelectMatchId);
      _entryPreSelectSeasonId = null;
      _entryPreSelectMatchId  = null;
      break;
    case 'standings': document.querySelector('standings-section')?.refresh(allSeasons); break;
    case 'stats':     document.querySelector('stats-section')?.refresh(allSeasons); break;
    case 'handicap':  document.querySelector('handicaps-page')?.refresh(allSeasons, activeSeason); break;
  }
}

// ── API helpers ───────────────────────────────────────────────────────────────
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
  return activeLeague ? `?league_id=${activeLeague.id}` : '';
}
// Returns "&league_id=X" for appending to existing query strings
function lidAmp() {
  return activeLeague ? `&league_id=${activeLeague.id}` : '';
}

// ── Toast ─────────────────────────────────────────────────────────────────────
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

// ── League selector ───────────────────────────────────────────────────────────
async function switchLeague() {
  const id = parseInt(document.getElementById('league-select').value);
  activeLeague = allLeagues.find(l => l.id === id) || null;
  if (activeLeague) localStorage.setItem('activeLeagueId', activeLeague.id);
  await loadLeagueData();
  // reload the currently visible section
  const sec = document.querySelector('[data-section].active')?.dataset.section || 'dashboard';
  loadSection(sec);
}

async function loadLeagueData() {
  if (!activeLeague) return;
  const lid = activeLeague.id;
  try {
    [allTeams, allPlayers, allSeasons] = await Promise.all([
      api('GET', `/teams?league_id=${lid}`),
      api('GET', `/players?league_id=${lid}`),
      api('GET', `/seasons?league_id=${lid}`)
    ]);
    activeSeason = allSeasons.find(s => s.active) || null;
    const label = document.getElementById('active-season-label');
    label.textContent = activeSeason ? '📅 ' + activeSeason.name : 'No active season';
  } catch(e) { toast('Failed to load league data: ' + e.message, 'danger'); }
}

// ── Bootstrap ─────────────────────────────────────────────────────────────────
async function init() {
  try {
    allLeagues = await api('GET', '/leagues');
  } catch(e) {
    toast('Could not load leagues: ' + e.message, 'danger');
    allLeagues = [];
  }

  // Populate league dropdown
  const sel = document.getElementById('league-select');
  if (allLeagues.length === 0) {
    sel.innerHTML = '<option value="">No leagues — add one</option>';
    activeLeague = null;
  } else {
    sel.innerHTML = allLeagues.map(l =>
      `<option value="${l.id}">${l.name}</option>`
    ).join('');
    // Restore last-used league from localStorage
    const saved = parseInt(localStorage.getItem('activeLeagueId'));
    const restored = allLeagues.find(l => l.id === saved);
    activeLeague = restored || allLeagues[0];
    sel.value = activeLeague.id;
  }

  if (activeLeague) {
    await loadLeagueData();
    document.querySelector('dashboard-page')?.refresh(activeLeague, activeSeason, allTeams, allPlayers);
  }
}
init();



// Cross-domain navigation entry point; delegates to activateSection.
function navTo(sec) { activateSection(sec); }

// ── Seasons domain bridge ─────────────────────────────────────────────────────
// The seasons domain component fires these events; the shell updates cross-domain
// state (allSeasons, activeSeason) and responds to navigation requests.

document.addEventListener('season-state-changed', e => {
  allSeasons   = e.detail.allSeasons;
  activeSeason = e.detail.activeSeason;
  document.getElementById('active-season-label').textContent =
    activeSeason ? '📅 ' + activeSeason.name : 'No active season';
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
  allPlayers = e.detail.players;
  const activeSec = document.querySelector('[data-section].active')?.dataset.section;
  if (activeSec === 'teams') loadTeams();
});

document.addEventListener('dashboard-nav-request', e => navTo(e.detail.section));

document.addEventListener('dashboard-refresh-request', async () => {
  await loadLeagueData();
  document.querySelector('dashboard-page')?.refresh(activeLeague, activeSeason, allTeams, allPlayers);
});

// ── Teams ─────────────────────────────────────────────────────────────────────
function loadTeams() {
  const page = document.querySelector('teams-page');
  if (page) page.refresh(activeLeague?.id ?? null, activeSeason?.id ?? null);
}

// ── Leagues management modal ─────────────────────────────────────────────

function openLeagueModal() {
  document.querySelector('leagues-page')?.openModal(activeLeague);
}

document.addEventListener('leagues-list-changed', async e => {
  const { leagues, deletedId } = e.detail;
  allLeagues = leagues;

  if (deletedId != null && activeLeague?.id === deletedId) {
    activeLeague = allLeagues[0] || null;
    if (activeLeague) localStorage.setItem('activeLeagueId', String(activeLeague.id));
    else              localStorage.removeItem('activeLeagueId');
  }

  const sel = document.getElementById('league-select');
  if (allLeagues.length === 0) {
    sel.innerHTML = '<option value="">No leagues — add one</option>';
    activeLeague = null;
  } else {
    sel.innerHTML = allLeagues.map(l =>
      `<option value="${l.id}" ${activeLeague && l.id === activeLeague.id ? 'selected' : ''}>${l.name}</option>`
    ).join('');
  }

  if (deletedId != null) {
    if (activeLeague) await loadLeagueData();
    loadSection('dashboard');
  }
});


async function backup() {
  try {
    const res = await api('POST', '/backup');
    toast('Backup saved: ' + res.path.split(/[/\\]/).pop());
  } catch(e) { toast(e.message,'danger'); }
}
