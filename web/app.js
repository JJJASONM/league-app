
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
document.querySelectorAll('[data-section]').forEach(link => {
  link.addEventListener('click', e => {
    e.preventDefault();
    const sec = link.dataset.section;
    document.querySelectorAll('[data-section]').forEach(l => l.classList.remove('active'));
    link.classList.add('active');
    document.querySelectorAll('.section').forEach(s => s.classList.remove('active'));
    document.getElementById('section-' + sec).classList.add('active');
    loadSection(sec);
  });
});

function loadSection(sec) {
  if (!activeLeague) return;
  switch(sec) {
    case 'dashboard': loadDashboard(); break;
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
    loadDashboard();
  }
}
init();

// ── Dashboard ─────────────────────────────────────────────────────────────────
async function loadDashboard() {
  if (!activeLeague) return;
  await loadLeagueData();

  const fmtLabel = { '8ball':'8-Ball','9ball':'9-Ball','10ball':'10-Ball','straight':'Straight Pool' };
  document.getElementById('dash-league-label').textContent =
    activeLeague.name + (activeLeague.game_format ? ' · ' + fmtLabel[activeLeague.game_format] : '');

  const actionsEl  = document.getElementById('dash-actions');
  const upcomingEl = document.getElementById('dash-upcoming');
  const standingsEl = document.getElementById('dash-standings');

  // ── No active season ──────────────────────────────────────────────────────
  if (!activeSeason) {
    actionsEl.innerHTML = actionItem('urgent','bi-exclamation-circle-fill',
      'No active season',
      'Create or activate a season before entering matches.',
      `<button class="btn btn-sm btn-outline-danger action-btn" onclick="navTo('seasons')">Go to Seasons</button>`);
    upcomingEl.innerHTML = '<tbody><tr><td class="text-muted text-center py-3">No active season</td></tr></tbody>';
    standingsEl.innerHTML = '<tbody><tr><td class="text-muted text-center py-3">No active season</td></tr></tbody>';
    return;
  }

  let matches = [], standings = [];
  try {
    [matches, standings] = await Promise.all([
      api('GET', `/matches?season_id=${activeSeason.id}`),
      api('GET', `/standings?season_id=${activeSeason.id}`)
    ]);
  } catch(e) { actionsEl.innerHTML = `<div class="text-danger p-3">${e.message}</div>`; return; }

  const today = new Date(); today.setHours(0,0,0,0);
  const todayStr = today.toISOString().slice(0,10);
  const nextWeek = new Date(today); nextWeek.setDate(today.getDate()+7);

  const pending   = matches.filter(m => !m.completed);
  const completed = matches.filter(m =>  m.completed);

  // Past matches with no results (match_date <= today, not completed)
  const overdue = pending.filter(m => m.match_date && m.match_date <= todayStr);
  // Upcoming in next 7 days
  const upcoming = pending.filter(m => {
    if (!m.match_date) return false;
    const d = new Date(m.match_date + 'T00:00:00');
    return d > today && d <= nextWeek;
  });
  // Matches with no date set
  const undated = pending.filter(m => !m.match_date);

  // Group overdue by week
  const overdueByWeek = {};
  overdue.forEach(m => { (overdueByWeek[m.week_number] = overdueByWeek[m.week_number]||[]).push(m); });

  // ── Build action items ────────────────────────────────────────────────────
  const sections = [];

  // Setup checks
  const setupItems = [];
  if (allTeams.length === 0)
    setupItems.push(actionItem('urgent','bi-people-fill','No teams in this league',
      'Add teams before generating a schedule.',
      `<button class="btn btn-sm btn-outline-danger action-btn" onclick="navTo('teams')">Add Teams</button>`));
  if (allPlayers.length === 0)
    setupItems.push(actionItem('warn','bi-person-badge','No players added',
      'Add players to teams so scoresheets can be generated.',
      `<button class="btn btn-sm btn-outline-warning action-btn" onclick="navTo('players')">Add Players</button>`));
  if (matches.length === 0)
    setupItems.push(actionItem('warn','bi-calendar-week','No schedule generated',
      'Generate the round-robin schedule for ' + activeSeason.name + '.',
      `<button class="btn btn-sm btn-outline-warning action-btn" onclick="navTo('seasons')">Generate Schedule</button>`));
  if (setupItems.length) {
    sections.push(`<div class="dash-section-header">Setup</div>` + setupItems.join(''));
  }

  // Weekly workflow
  const weeklyItems = [];

  if (overdue.length > 0) {
    const weeks = Object.keys(overdueByWeek).sort((a,b)=>a-b);
    weeks.forEach(w => {
      const ms = overdueByWeek[w];
      weeklyItems.push(actionItem('urgent','bi-pencil-square',
        `Week ${w} — scores not entered (${ms.length} match${ms.length>1?'es':''})`,
        ms.map(m => `${m.home_team_name} vs ${m.away_team_name}`).join(' &nbsp;·&nbsp; '),
        `<button class="btn btn-sm btn-outline-danger action-btn" onclick="navTo('entry')">Enter Scores</button>`));
    });
  }

  // Scoresheet reminder — placeholder until feature is built
  if (upcoming.length > 0) {
    const nextDate = upcoming[0].match_date;
    weeklyItems.push(actionItem('warn','bi-file-earmark-text',
      `Scoresheets not yet created for ${upcoming.length} upcoming match${upcoming.length>1?'es':''}`,
      `Next match date: ${displayDate(nextDate)}. Generate scoresheets before league night.`,
      `<button class="btn btn-sm btn-outline-secondary action-btn" disabled title="Coming soon">Scoresheets (soon)</button>`));
  }

  // Email template reminder — placeholder until feature is built
  if (completed.length > 0) {
    const lastWeek = Math.max(...completed.map(m => m.week_number));
    weeklyItems.push(actionItem('warn','bi-envelope',
      `Weekly email not yet sent for Week ${lastWeek}`,
      'Results, standings update, and any announcements.',
      `<button class="btn btn-sm btn-outline-secondary action-btn" disabled title="Coming soon">Compose Email (soon)</button>`));
  }

  if (weeklyItems.length) {
    sections.push(`<div class="dash-section-header">This Week</div>` + weeklyItems.join(''));
  }

  // All clear check
  if (setupItems.length === 0 && weeklyItems.length === 0) {
    sections.push(actionItem('ok','bi-check-circle-fill',
      'All caught up!',
      `${completed.length} matches recorded · ${pending.length} remaining in ${activeSeason.name}.`,
      ''));
  }

  // Info: undated matches
  if (undated.length > 0) {
    sections.push(`<div class="dash-section-header">Notes</div>` +
      actionItem('info','bi-calendar-x',
        `${undated.length} match${undated.length>1?'es':''} have no date assigned`,
        'Dates can be set when editing the schedule.',
        `<button class="btn btn-sm btn-outline-secondary action-btn" onclick="navTo('schedule')">View Schedule</button>`));
  }

  actionsEl.innerHTML = sections.join('') ||
    '<div class="text-muted text-center py-4">Nothing to show yet.</div>';

  // ── Upcoming panel ────────────────────────────────────────────────────────
  upcomingEl.innerHTML = `
    <thead><tr><th>Date</th><th>Home</th><th>Away</th></tr></thead>
    <tbody>${upcoming.length
      ? upcoming.map(m=>`<tr>
          <td class="text-muted small">${displayDate(m.match_date)}</td>
          <td>${m.home_team_name}</td>
          <td>${m.away_team_name}</td></tr>`).join('')
      : '<tr><td colspan="3" class="text-muted text-center py-3">No matches in the next 7 days</td></tr>'
    }</tbody>`;

  // ── Standings panel ───────────────────────────────────────────────────────
  const top = standings.slice(0,6);
  standingsEl.innerHTML = `
    <thead><tr><th>#</th><th>Team</th><th>Pts</th><th>W-L</th></tr></thead>
    <tbody>${top.length
      ? top.map((s,i)=>`<tr ${i===0?'class="fw-semibold"':''}>
          <td>${i+1}</td><td>${s.team_name}</td>
          <td>${s.points}</td><td class="text-muted">${s.wins}-${s.losses}</td></tr>`).join('')
      : '<tr><td colspan="4" class="text-muted text-center py-3">No completed matches</td></tr>'
    }</tbody>`;
}

// Renders a single action item row
function actionItem(level, icon, title, detail, btnHtml) {
  return `<div class="action-item ${level}">
    <div class="action-icon"><i class="bi ${icon}"></i></div>
    <div class="flex-grow-1">
      <div class="action-title">${title}</div>
      ${detail ? `<div class="action-detail">${detail}</div>` : ''}
    </div>
    ${btnHtml || ''}
  </div>`;
}

// Navigate to a section by name
function navTo(sec) {
  document.querySelectorAll('[data-section]').forEach(l => l.classList.remove('active'));
  const link = document.querySelector(`[data-section="${sec}"]`);
  if (link) link.classList.add('active');
  document.querySelectorAll('.section').forEach(s => s.classList.remove('active'));
  document.getElementById('section-' + sec)?.classList.add('active');
  loadSection(sec);
}

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

// ── Season utilities (shared; used by schedule, lineup, and scoresheet) ────────

// Format a YYYY-MM-DD or ISO date string as "Jul 6, 2026". Returns fallback if empty.
function displayDate(raw, fallback = 'TBD') {
  if (!raw) return fallback;
  const parts = raw.slice(0, 10).split('-').map(Number);
  if (parts.length !== 3 || parts.some(isNaN)) return fallback;
  const [y, mo, d] = parts;
  const dt = new Date(y, mo - 1, d);
  if (isNaN(dt)) return fallback;
  return dt.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
}

// fmtDate kept for season date-range display (delegates to displayDate).
function fmtDate(raw, fallback = 'TBD') { return displayDate(raw, fallback); }

function fmtDateRange(start, end) {
  return `${displayDate(start, '—')} – ${displayDate(end, 'TBD')}`;
}

// ── Teams ─────────────────────────────────────────────────────────────────────
function loadTeams() {
  const page = document.querySelector('teams-page');
  if (page) page.refresh(activeLeague?.id ?? null, activeSeason?.id ?? null);
}

// ── Leagues management modal ──────────────────────────────────────────────────

// Fetches team counts for every known league and re-renders the table + checklist.
// Called on open and after every add/delete so counts stay accurate.
async function refreshLeaguesTable() {
  const counts = {};
  await Promise.all(allLeagues.map(async l => {
    try {
      const ts = await api('GET', `/teams?league_id=${l.id}`);
      counts[l.id] = ts.length;
    } catch(_) { counts[l.id] = 0; }
  }));
  renderLeaguesTable(counts);
}

async function openLeagueModal() {
  await refreshLeaguesTable();
  openModal('league-modal');
}

function renderLeaguesTable(counts = {}) {
  const formatLabel = { '8ball':'8-Ball','9ball':'9-Ball','10ball':'10-Ball','straight':'Straight' };
  const tbody = document.querySelector('#leagues-table tbody');
  tbody.innerHTML = allLeagues.map(l => {
    const n = counts[l.id] ?? '—';
    const teamOk = typeof n === 'number' && n >= 2;
    const teamBadge = typeof n === 'number'
      ? `<span class="badge ${teamOk ? 'bg-success' : 'bg-warning text-dark'}" style="font-size:.7rem">
          ${teamOk ? '<i class="bi bi-check-lg"></i> ' : '<i class="bi bi-exclamation-triangle"></i> '}${n} team${n !== 1 ? 's' : ''}
        </span>`
      : '<span class="text-muted small">—</span>';
    return `
    <tr ${activeLeague && l.id === activeLeague.id ? 'class="table-primary"' : ''}>
      <td class="fw-semibold">${l.name}</td>
      <td>${formatLabel[l.game_format]||l.game_format}</td>
      <td>${l.day_of_week||'—'}</td>
      <td>${teamBadge}</td>
      <td class="text-end">
        <button class="btn btn-outline-danger btn-sm py-0" onclick="deleteLeague(${l.id})"><i class="bi bi-trash"></i></button>
      </td>
    </tr>`;
  }).join('') || '<tr><td colspan="5" class="text-center text-muted py-2">No leagues yet</td></tr>';

  // Verification checklist per league.
  const checklist = document.getElementById('league-checklist');
  if (!checklist) return;
  checklist.innerHTML = allLeagues.map(l => {
    const n = counts[l.id] ?? 0;
    const hasTeams  = n >= 2;
    const needsOdd  = n > 0 && n % 2 === 1;
    const item = (ok, text) =>
      `<li class="${ok ? 'text-success' : 'text-muted'}">
        <i class="bi ${ok ? 'bi-check-circle-fill' : 'bi-circle'} me-1"></i>${text}
      </li>`;
    return `<div class="mb-2">
      <div class="fw-semibold small mb-1">${l.name}</div>
      <ul class="list-unstyled ms-1 mb-0" style="font-size:.82rem">
        ${item(n >= 2, `At least 2 teams configured (${n} now)`)}
        ${item(needsOdd, `Odd team count enables natural bye rotation (${n} teams)`)}
        ${item(false, 'Review teams and rosters before generating a season schedule')}
      </ul>
    </div>`;
  }).join('') || '<div class="text-muted small">No leagues to check.</div>';
}

async function addLeague() {
  const name   = document.getElementById('new-league-name').value.trim();
  const format = document.getElementById('new-league-format').value;
  const day    = document.getElementById('new-league-day').value;
  if (!name) { toast('League name is required','warning'); return; }
  try {
    const newL = await api('POST', '/leagues', { name, game_format: format, day_of_week: day||null });
    toast('League added');
    document.getElementById('new-league-name').value = '';
    allLeagues = await api('GET', '/leagues');
    // Update sidebar select
    const sel = document.getElementById('league-select');
    sel.innerHTML = allLeagues.map(l =>
      `<option value="${l.id}" ${activeLeague && l.id === activeLeague.id ? 'selected' : ''}>${l.name}</option>`
    ).join('');
    await refreshLeaguesTable();
  } catch(e) { toast(e.message,'danger'); }
}

async function deleteLeague(id) {
  if (!confirm('Delete this league and ALL its teams, seasons and matches? This cannot be undone.')) return;
  try {
    await api('DELETE', `/leagues/${id}`);
    toast('League deleted');
    allLeagues = await api('GET', '/leagues');
    // If we deleted the active league, switch to first available
    if (activeLeague?.id === id) {
      activeLeague = allLeagues[0] || null;
      if (activeLeague) localStorage.setItem('activeLeagueId', activeLeague.id);
      else              localStorage.removeItem('activeLeagueId');
    }
    const sel = document.getElementById('league-select');
    sel.innerHTML = allLeagues.map(l =>
      `<option value="${l.id}" ${activeLeague && l.id === activeLeague.id ? 'selected' : ''}>${l.name}</option>`
    ).join('') || '<option value="">No leagues</option>';
    if (activeLeague) await loadLeagueData();
    await refreshLeaguesTable();
    loadSection('dashboard');
  } catch(e) { toast(e.message,'danger'); }
}

// ── Backup ────────────────────────────────────────────────────────────────────
async function backup() {
  try {
    const res = await api('POST', '/backup');
    toast('Backup saved: ' + res.path.split(/[/\\]/).pop());
  } catch(e) { toast(e.message,'danger'); }
}
