
// ── State ────────────────────────────────────────────────────────────────────
let allLeagues  = [];
let activeLeague = null;
let allTeams    = [];
let allPlayers  = [];
let allSeasons  = [];
let activeSeason = null;
let reopenWeekContext = null;
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
    case 'players':   loadPlayers(); break;
    case 'schedule':  populateScheduleSeasonSelect(); break;
    case 'lineup':    document.querySelector('lineup-page')?.refresh(allSeasons, activeSeason, allTeams, allPlayers); break;
    case 'entry':
      document.querySelector('match-entry-page')?.refresh(allSeasons, activeSeason, allPlayers, activeLeague, _entryPreSelectSeasonId, _entryPreSelectMatchId);
      _entryPreSelectSeasonId = null;
      _entryPreSelectMatchId  = null;
      break;
    case 'standings': populateSeasonSelect('standings-season-select', loadStandings); break;
    case 'stats':     populateSeasonSelect('stats-season-select', loadPlayerStats); break;
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
      populateScheduleSeasonSelect(previewSeasonId);
      if (openPoster) openSchedulePoster();
    }, 50);
  }
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

// ── Assign teams to blanket match slots ───────────────────────────────────────
function openAssignMatchTeams(matchId) {
  document.getElementById('assign-match-id').value = matchId;
  const opts = '<option value="">(unassigned)</option>' +
    allTeams.map(t => `<option value="${t.id}">${t.name}</option>`).join('');
  document.getElementById('assign-home-select').innerHTML = opts;
  document.getElementById('assign-away-select').innerHTML = opts;
  openModal('assign-match-modal');
}

async function confirmAssignMatchTeams() {
  const matchId  = parseInt(document.getElementById('assign-match-id').value);
  const homeId   = document.getElementById('assign-home-select').value;
  const awayId   = document.getElementById('assign-away-select').value;
  const body = {
    home_team_id: homeId ? parseInt(homeId) : null,
    away_team_id: awayId ? parseInt(awayId) : null
  };
  try {
    await api('PATCH', `/matches/${matchId}/assign`, body);
    closeModal('assign-match-modal');
    toast('Teams assigned');
    loadSchedule(); // refresh schedule view
  } catch(e) { toast(e.message, 'danger'); }
}

// ── Teams ─────────────────────────────────────────────────────────────────────
function loadTeams() {
  const page = document.querySelector('teams-page');
  if (page) page.refresh(activeLeague?.id ?? null, activeSeason?.id ?? null);
}

// ── Players ───────────────────────────────────────────────────────────────────
async function loadPlayers() {
  if (!activeLeague) return;
  allPlayers = await api('GET', `/players?league_id=${activeLeague.id}`);
  const tbody = document.querySelector('#players-table tbody');
  tbody.innerHTML = allPlayers.map(p => `
    <tr>
      <td class="text-muted small">${p.player_number||'—'}</td>
      <td class="fw-semibold">${p.last_name}${p.admin_hold ? ' <span class="badge bg-warning text-dark ms-1" style="font-size:.65rem">Hold</span>' : ''}</td>
      <td>${p.first_name}</td>
      <td><span class="badge bg-secondary badge-hc">${p.handicap}</span></td>
      <td>${p.team_name || '<span class="text-muted">—</span>'}</td>
      <td class="text-muted small">${p.phone||'—'}</td>
      <td class="text-end">
        <button class="btn btn-outline-secondary btn-sm py-0 me-1" onclick="editPlayer(${p.id})"><i class="bi bi-pencil"></i></button>
        <button class="btn btn-outline-danger btn-sm py-0" onclick="deletePlayer(${p.id})"><i class="bi bi-trash"></i></button>
      </td>
    </tr>`).join('') || '<tr><td colspan="7" class="text-center text-muted py-3">No players yet</td></tr>';
}

function setPlayerModalMode(isEdit) {
  const numInput = document.getElementById('player-number');
  const lockIcon = document.getElementById('player-number-lock');
  // Lock player number when editing
  numInput.readOnly = isEdit;
  numInput.classList.toggle('bg-light', isEdit);
  lockIcon.classList.toggle('d-none', !isEdit);
  // Show admin hold only for 9-ball leagues
  const is9ball = activeLeague?.game_format === '9ball';
  document.getElementById('admin-hold-row').classList.toggle('d-none', !is9ball);
}

function openNewPlayer() {
  document.getElementById('player-modal-title').textContent = 'Add Player';
  document.getElementById('player-id').value = '';
  document.getElementById('player-number').value = '';
  document.getElementById('player-first-name').value = '';
  document.getElementById('player-last-name').value = '';
  document.getElementById('player-phone').value = '';
  document.getElementById('player-email').value = '';
  document.getElementById('player-handicap').value = '0';
  document.getElementById('player-admin-hold').checked = false;
  setPlayerModalMode(false);
  populateTeamDropdown(null);
  openModal('player-modal');
}

function editPlayer(id) {
  const p = allPlayers.find(x => x.id === id);
  if (!p) return;
  document.getElementById('player-modal-title').textContent = 'Edit Player';
  document.getElementById('player-id').value = p.id;
  document.getElementById('player-number').value = p.player_number || '';
  document.getElementById('player-first-name').value = p.first_name || '';
  document.getElementById('player-last-name').value = p.last_name || '';
  document.getElementById('player-phone').value = p.phone || '';
  document.getElementById('player-email').value = p.email || '';
  document.getElementById('player-handicap').value = p.handicap;
  document.getElementById('player-admin-hold').checked = !!p.admin_hold;
  setPlayerModalMode(true);
  populateTeamDropdown(p.team_id);
  openModal('player-modal');
}

function populateTeamDropdown(selectedId) {
  const sel = document.getElementById('player-team');
  sel.innerHTML = '<option value="">— No Team —</option>' +
    allTeams.map(t => `<option value="${t.id}" ${t.id==selectedId?'selected':''}>${t.name}</option>`).join('');
}

async function savePlayer() {
  const id        = document.getElementById('player-id').value;
  const teamVal   = document.getElementById('player-team').value;
  const firstName = document.getElementById('player-first-name').value.trim();
  const lastName  = document.getElementById('player-last-name').value.trim();
  if (!firstName && !lastName) { toast('First or last name is required','warning'); return; }
  const body = {
    player_number: id ? undefined : document.getElementById('player-number').value.trim(), // only on create
    first_name:    firstName,
    last_name:     lastName,
    phone:         document.getElementById('player-phone').value.trim(),
    email:         document.getElementById('player-email').value.trim(),
    handicap:      parseFloat(document.getElementById('player-handicap').value) || 0,
    admin_hold:    document.getElementById('player-admin-hold').checked,
    team_id:       teamVal ? parseInt(teamVal) : null,
    league_id:     activeLeague?.id
  };
  // Include player_number on create
  if (!id) body.player_number = document.getElementById('player-number').value.trim();
  try {
    if (id) await api('PUT', `/players/${id}`, body);
    else    await api('POST', '/players', body);
    closeModal('player-modal');
    toast('Player saved');
    allPlayers = await api('GET', `/players?league_id=${activeLeague.id}`);
    const activeSec = document.querySelector('[data-section].active')?.dataset.section;
    if (activeSec === 'teams') loadTeams();
    else loadPlayers();
  } catch(e) { toast(e.message,'danger'); }
}

async function deletePlayer(id) {
  if (!confirm('Remove this player?')) return;
  try {
    await api('DELETE', `/players/${id}`);
    toast('Deleted');
    allPlayers = await api('GET', `/players?league_id=${activeLeague.id}`);
    loadPlayers();
  } catch(e) { toast(e.message,'danger'); }
}

// ── Season selects ────────────────────────────────────────────────────────────
function populateSeasonSelect(selId, callback) {
  const sel = document.getElementById(selId);
  sel.innerHTML = allSeasons.map(s =>
    `<option value="${s.id}" ${s.active?'selected':''}>${s.name}</option>`
  ).join('') || '<option value="">No seasons</option>';
  if (callback) callback();
}

// Populate the user-facing Schedule page selector.
// Without a previewSeasonId, shows only active seasons (user-facing mode).
// With a previewSeasonId (called from the manage panel), shows that season
// regardless of active status so admins can preview draft schedules.
function populateScheduleSeasonSelect(previewSeasonId = null) {
  const sel       = document.getElementById('schedule-season-select');
  const contentEl = document.getElementById('schedule-content');

  if (previewSeasonId !== null) {
    const s = allSeasons.find(x => x.id === previewSeasonId);
    if (s) {
      sel.innerHTML = `<option value="${s.id}">${s.name}${s.active ? '' : ' (draft)'}</option>`;
      sel.value = s.id;
      loadSchedule();
      return;
    }
  }

  // User-facing: active seasons only.
  const active = allSeasons.filter(s => s.active);
  if (active.length === 0) {
    sel.innerHTML = '<option value="">— No active season —</option>';
    contentEl.innerHTML = `<div class="text-center py-5 text-muted">
      <i class="bi bi-calendar-x" style="font-size:2rem"></i>
      <div class="mt-2 fw-semibold">No active season</div>
      <div class="small mt-1">Go to <a href="#" onclick="navTo('seasons');return false;" class="text-decoration-none">Seasons</a> to activate one.</div>
    </div>`;
    return;
  }
  sel.innerHTML = active.map(s => `<option value="${s.id}">${s.name}</option>`).join('');
  loadSchedule();
}

// Fetches acknowledgment history for a week and renders it into container.
// Sets container.dataset.loaded = '1' after first successful fetch so subsequent
// toggle calls just show/hide without re-fetching.
async function loadWeekAcknowledgments(seasonId, weekNum, container) {
  try {
    const acks = await api('GET', `/seasons/${seasonId}/weeks/${weekNum}/acknowledgments`);
    container.dataset.loaded = '1';
    if (!acks || acks.length === 0) {
      container.innerHTML = '<p class="text-muted small mb-0">No acknowledgment records found.</p>';
      return;
    }
    const rows = acks.map((a, idx) => {
      const matchBadge = a.match_id
        ? `<span class="badge bg-secondary me-1">Match ${escapeHTML(String(a.match_id))}</span>`
        : '';
      const field = a.field ? ` <span class="text-muted">(${escapeHTML(a.field)})</span>` : '';
      const notes = a.notes ? ` - <span class="fst-italic">${escapeHTML(a.notes)}</span>` : '';
      const ts = escapeHTML(String(a.acknowledged_at));
      const border = idx < acks.length - 1 ? ' border-bottom' : '';
      return `<li class="small py-1${border}">`
        + `<span class="text-muted me-2">${ts}</span>`
        + `${matchBadge}<strong>${escapeHTML(a.warning_code)}</strong>${field}${notes}`
        + `</li>`;
    }).join('');
    container.innerHTML = `<ul class="list-unstyled mb-0">${rows}</ul>`;
  } catch (_err) {
    container.innerHTML = '<p class="text-danger small mb-0">Failed to load acknowledgment history.</p>';
  }
}

// ── Schedule ──────────────────────────────────────────────────────────────────
document.getElementById('schedule-content').addEventListener('click', e => {
  const reopenBtn = e.target.closest('[data-action="reopen-week"]');
  if (reopenBtn) {
    confirmReopenWeek(parseInt(reopenBtn.dataset.seasonId, 10), parseInt(reopenBtn.dataset.weekNum, 10), reopenBtn.dataset.matchDate || '');
    return;
  }
  const reviewBtn = e.target.closest('[data-action="review-close-week"]');
  if (reviewBtn) {
    reviewCloseWeek(
      parseInt(reviewBtn.dataset.seasonId, 10),
      parseInt(reviewBtn.dataset.weekNum, 10),
      parseInt(reviewBtn.dataset.ackCount, 10) || 0
    );
    return;
  }
  const toggleAcksBtn = e.target.closest('[data-action="toggle-week-acks"]');
  if (toggleAcksBtn) {
    const wn = toggleAcksBtn.dataset.weekNum;
    const container = document.getElementById('week-acks-' + wn);
    if (!container) return;
    if (container.classList.contains('d-none')) {
      container.classList.remove('d-none');
      if (container.dataset.loaded !== '1') {
        const sid = parseInt(document.getElementById('schedule-season-select').value, 10);
        loadWeekAcknowledgments(sid, parseInt(wn, 10), container);
      }
    } else {
      container.classList.add('d-none');
    }
    return;
  }
  const entryBtn = e.target.closest('[data-action="open-match-entry"]');
  if (entryBtn) openMatchEntry(parseInt(entryBtn.dataset.matchId, 10), parseInt(entryBtn.dataset.seasonId, 10) || null);
});

document.getElementById('reopen-week-modal').addEventListener('hidden.bs.modal', () => {
  reopenWeekContext = null;
});

document.getElementById('close-week-modal').addEventListener('click', e => {
  const entryBtn = e.target.closest('[data-action="open-match-entry"]');
  if (entryBtn) {
    openMatchEntry(parseInt(entryBtn.dataset.matchId, 10), parseInt(entryBtn.dataset.seasonId, 10) || null);
    return;
  }
  const priorAcksBtn = e.target.closest('[data-action="load-prior-acks"]');
  if (priorAcksBtn) {
    const container = document.getElementById('cwm-prior-acks');
    if (!container) return;
    if (container.dataset.loaded === '1') {
      container.classList.toggle('d-none');
    } else {
      container.classList.remove('d-none');
      loadWeekAcknowledgments(
        parseInt(priorAcksBtn.dataset.seasonId, 10),
        parseInt(priorAcksBtn.dataset.weekNum, 10),
        container
      );
    }
  }
});

document.getElementById('reopen-week-confirm-btn').addEventListener('click', async () => {
  if (!reopenWeekContext) return;
  const { seasonId, weekNum, modal } = reopenWeekContext;
  reopenWeekContext = null;
  try {
    await api('POST', `/seasons/${seasonId}/weeks/${weekNum}/reopen`);
    modal.hide();
    toast('Week ' + weekNum + ' reopened.', 'success');
    loadSchedule();
    loadStandings();
    const statsSeasonId = document.getElementById('stats-season-select').value;
    if (statsSeasonId) loadPlayerStats();
  } catch (e) {
    toast('Reopen failed: ' + (e.message || e), 'danger');
  }
});

async function loadSchedule() {
  const seasonId = document.getElementById('schedule-season-select').value;
  if (!seasonId) { document.getElementById('schedule-content').innerHTML = '<div class="text-muted">No season selected.</div>'; return; }
  const [matches, weekList] = await Promise.all([
    api('GET', `/matches?season_id=${seasonId}`),
    api('GET', `/seasons/${seasonId}/weeks`).catch(() => []),
  ]);
  const weekStatusMap = {};
  (weekList || []).forEach(ws => { weekStatusMap[ws.week_number] = ws; });
  const byWeek = {};
  matches.forEach(m => { (byWeek[m.week_number] = byWeek[m.week_number]||[]).push(m); });
  const weeks = Object.keys(byWeek).sort((a,b)=>a-b);

  const isUnassigned = m => !m.home_team_id || m.home_team_name === '(unassigned)' ||
                            !m.away_team_id || m.away_team_name === '(unassigned)';

  // Build the complete set of teams that appear anywhere in this season's schedule,
  // so we can determine which team has a natural bye each week.
  const seasonTeamIds = new Set();
  const seasonTeamNames = {};
  matches.forEach(m => {
    if (m.home_team_id) { seasonTeamIds.add(m.home_team_id); seasonTeamNames[m.home_team_id] = m.home_team_name; }
    if (m.away_team_id) { seasonTeamIds.add(m.away_team_id); seasonTeamNames[m.away_team_id] = m.away_team_name; }
  });

  document.getElementById('schedule-content').innerHTML = weeks.length ? weeks.map(w => {
    const playingThisWeek = new Set();
    byWeek[w].forEach(m => {
      if (m.home_team_id) playingThisWeek.add(m.home_team_id);
      if (m.away_team_id) playingThisWeek.add(m.away_team_id);
    });
    const byeTeams = [...seasonTeamIds].filter(id => !playingThisWeek.has(id));
    const byeRow = byeTeams.length
      ? `<tr class="table-light">
          <td colspan="4" class="text-muted small fst-italic ps-3 py-1">
            <i class="bi bi-person-x me-1"></i>Bye: ${byeTeams.map(id => seasonTeamNames[id] || '?').join(', ')}
          </td>
          <td></td>
        </tr>`
      : '';

    const ws = weekStatusMap[w];
    const isClosed = ws && ws.status === 'closed';
    const closedLabel = (ws && ws.closed_at) ? `Closed &middot; ${displayDate(ws.closed_at)}` : 'Closed';
    const statusChip = isClosed
      ? `<span class="badge bg-success ms-2">${closedLabel}</span>`
      : '<span class="badge bg-secondary ms-2">Open</span>';
    const countBadge = ws
      ? `<span class="text-muted small ms-2">${ws.completed_count}/${ws.match_count} done</span>`
      : '';
    const matchDate = byWeek[w][0].match_date;
    const ackCount = ws ? ws.ack_count : 0;
    const ackToggle = ackCount > 0
      ? `<button class="btn btn-link btn-sm py-0 ms-1 text-secondary" data-action="toggle-week-acks" data-week-num="${w}" title="Show/hide prior close acknowledgments"><i class="bi bi-clock-history"></i> ${ackCount} prior ack${ackCount !== 1 ? 's' : ''}</button>`
      : '';
    const closeBtn = isClosed
      ? `<button class="btn btn-sm btn-outline-warning py-0 ms-2" data-action="reopen-week" data-season-id="${escapeHTML(String(seasonId))}" data-week-num="${w}" data-match-date="${escapeHTML(matchDate || '')}" title="Remove this week from official standings to correct scores, then re-close"><i class="bi bi-arrow-counterclockwise"></i> Reopen</button>`
      : `<button class="btn btn-sm btn-outline-primary py-0 ms-2" data-action="review-close-week" data-season-id="${escapeHTML(String(seasonId))}" data-week-num="${w}" data-ack-count="${ackCount}">Review &amp; Close</button>`;
    const ackSection = ackCount > 0
      ? `<div class="px-3 pb-2 d-none" id="week-acks-${w}" data-loaded="0"></div>`
      : '';

    return `
    <div class="card mb-2">
      <div class="card-header week-header py-1 d-flex align-items-center justify-content-between">
        <span>Week ${w} - ${displayDate(byWeek[w][0].match_date)}</span>
        <span class="d-flex align-items-center">${countBadge}${statusChip}${ackToggle}${closeBtn}</span>
      </div>
      <div class="card-body p-0">
        <table class="table table-sm mb-0" style="table-layout:fixed">
          <colgroup>
            <col style="width:37%">
            <col style="width:5%">
            <col style="width:37%">
            <col style="width:11%">
            <col style="width:10%">
          </colgroup>
          <thead><tr>
            <th>Home</th>
            <th class="text-center">vs</th>
            <th>Away</th>
            <th>Status</th>
            <th></th>
          </tr></thead>
          <tbody>
            ${byWeek[w].map(m => `<tr>
              <td class="text-truncate">${m.home_team_name || '<span class="text-muted fst-italic">(unassigned)</span>'}</td>
              <td class="text-center text-muted">vs</td>
              <td class="text-truncate">${m.away_team_name || '<span class="text-muted fst-italic">(unassigned)</span>'}</td>
              <td>${m.completed
                ? '<span class="badge bg-success">Done</span>'
                : '<span class="badge bg-secondary">Pending</span>'}</td>
              <td class="text-end">${isUnassigned(m)
                ? `<button class="btn btn-outline-primary btn-sm py-0" onclick="openAssignMatchTeams(${m.id})"><i class="bi bi-people"></i> Assign</button>`
                : !isClosed
                  ? `<button class="btn btn-outline-secondary btn-sm py-0" data-action="open-match-entry" data-season-id="${escapeHTML(String(seasonId))}" data-match-id="${m.id}"><i class="bi bi-pencil-square"></i> Score Entry</button>`
                  : ''}</td>
            </tr>`).join('')}
            ${byeRow}
          </tbody>
        </table>
        ${ackSection}
      </div>
    </div>`;
  }).join('')
  : '<div class="text-muted text-center py-4">No matches scheduled yet. Use Seasons → Manage → Schedule.</div>';
}

// _renderHandicapRecs returns an HTML recommendations table when hc.recommendations
// is present and non-empty, otherwise returns ''. No Apply button is included.
function _renderHandicapRecs(hc) {
  if (!hc || !Array.isArray(hc.recommendations) || hc.recommendations.length === 0) return '';
  const rows = hc.recommendations.map(r => {
    let recCell, noteCell;
    if (r.skipped) {
      recCell = '<span class="text-muted">N/A</span>';
      noteCell = r.reason === 'admin_hold'
        ? '<span class="badge bg-secondary">Admin Hold</span>'
        : r.reason === 'no_data'
          ? '<span class="text-muted fst-italic">No data</span>'
          : '<span class="text-muted fst-italic">' + escapeHTML(r.reason || '') + '</span>';
    } else {
      recCell = escapeHTML(fmtHC(r.recommended_handicap));
      noteCell = r.reason === 'no_change'
        ? '<span class="text-muted fst-italic">No change</span>'
        : r.reason === 'capped'
          ? '<span class="badge bg-warning text-dark">Capped</span>'
          : '';
    }
    const trClass = r.skipped ? ' class="text-muted"' : '';
    return `<tr${trClass}><td>${escapeHTML(r.player_name)}</td><td>${escapeHTML(fmtHC(r.current_handicap))}</td><td>${recCell}</td><td>${r.matches_played}</td><td>${noteCell}</td></tr>`;
  }).join('');
  return `<div class="mt-2">
    <p class="small fw-bold text-danger mb-1"><i class="bi bi-exclamation-circle me-1"></i>Recommendations are <strong>not applied automatically</strong> -- review and update manually if needed.</p>
    <table class="table table-sm table-borderless small mb-0">
      <thead><tr><th class="text-muted fw-normal small">Player</th><th class="text-muted fw-normal small">Current</th><th class="text-muted fw-normal small">Recommended</th><th class="text-muted fw-normal small">Matches</th><th class="text-muted fw-normal small">Notes</th></tr></thead>
      <tbody>${rows}</tbody>
    </table>
  </div>`;
}

function fmtHC(v) { return (v >= 0 ? '+' : '') + v; }

function _renderAdvancePreview(preview) {
  const cw = preview.current_week;
  const nextNum = preview.next_week_number;
  const nw = preview.next_week;
  const hc = preview.handicap;
  let rows = '';
  rows += `<tr>
    <td class="text-muted pe-3 text-nowrap">This week</td>
    <td>${escapeHTML(String(cw.completed_count))}/${escapeHTML(String(cw.match_count))} scored &nbsp;${preview.can_close
      ? '<span class="badge bg-success">Ready</span>'
      : '<span class="badge bg-danger">Has errors</span>'}</td>
  </tr>`;
  if (nw) {
    const assignNote = nw.unassigned_count > 0
      ? ` &middot; <span class="text-warning">${nw.unassigned_count} unassigned</span>` : '';
    const lineupNote = nw.missing_lineup_team_ids && nw.missing_lineup_team_ids.length > 0
      ? ` &middot; <span class="text-warning">${nw.missing_lineup_team_ids.length} missing lineup</span>`
      : ' &middot; <span class="text-success">Lineups set</span>';
    rows += `<tr>
      <td class="text-muted pe-3 text-nowrap">Week ${escapeHTML(String(nextNum))}</td>
      <td>${escapeHTML(String(nw.match_count))} match${nw.match_count !== 1 ? 'es' : ''}${assignNote}${lineupNote}</td>
    </tr>`;
  } else {
    rows += `<tr>
      <td class="text-muted pe-3 text-nowrap">Next week</td>
      <td class="fst-italic text-muted">No further weeks scheduled</td>
    </tr>`;
  }
  rows += `<tr>
    <td class="text-muted pe-3 text-nowrap">Handicap</td>
    <td class="fst-italic text-muted">${escapeHTML(hc.message)}</td>
  </tr>`;
  return `<div class="mt-3 pt-2 border-top">
    <p class="small fw-semibold text-secondary mb-1"><i class="bi bi-calendar-arrow-up me-1"></i>Advance Preview</p>
    <table class="table table-sm table-borderless small mb-0"><tbody>${rows}</tbody></table>
    ${_renderHandicapRecs(hc)}
  </div>`;
}

function _renderCloseSuccess(closeData, weekNum) {
  const ar = closeData.advance_result || {};
  const cw = ar.closed_week || {};
  const nw = ar.next_week;
  const nextNum = ar.next_week_number;
  const hc = ar.handicap || {};
  const ackCount = closeData.acknowledgment_count || 0;

  let rows = '';
  rows += `<tr>
    <td class="text-muted pe-3 text-nowrap">Week ${escapeHTML(String(weekNum))}</td>
    <td>${escapeHTML(String(cw.completed_count || 0))}/${escapeHTML(String(cw.match_count || 0))} scored &nbsp;<span class="badge bg-success">Closed</span></td>
  </tr>`;
  if (nw) {
    const assignNote = nw.unassigned_count > 0
      ? ` &middot; <span class="text-warning">${nw.unassigned_count} unassigned</span>` : '';
    const lineupNote = nw.missing_lineup_team_ids && nw.missing_lineup_team_ids.length > 0
      ? ` &middot; <span class="text-warning">${nw.missing_lineup_team_ids.length} missing lineup</span>`
      : ' &middot; <span class="text-success">Lineups set</span>';
    rows += `<tr>
      <td class="text-muted pe-3 text-nowrap">Week ${escapeHTML(String(nextNum))}</td>
      <td>${escapeHTML(String(nw.match_count))} match${nw.match_count !== 1 ? 'es' : ''}${assignNote}${lineupNote}</td>
    </tr>`;
  } else {
    rows += `<tr>
      <td class="text-muted pe-3 text-nowrap">Next week</td>
      <td class="fst-italic text-muted">No further weeks scheduled</td>
    </tr>`;
  }
  rows += `<tr>
    <td class="text-muted pe-3 text-nowrap">Handicap</td>
    <td class="fst-italic text-muted">${escapeHTML(hc.message || 'No changes applied.')}</td>
  </tr>`;

  const ackNote = ackCount > 0
    ? ` (${ackCount} warning${ackCount !== 1 ? 's' : ''} acknowledged)` : '';
  return `<div class="alert alert-success py-2 mb-3 small d-flex align-items-center gap-2">
    <i class="bi bi-check-circle-fill flex-shrink-0"></i>
    <span><strong>Week ${escapeHTML(String(weekNum))} closed.</strong>${escapeHTML(ackNote)} Standings and player stats updated.</span>
  </div>
  <table class="table table-sm table-borderless small mb-0"><tbody>${rows}</tbody></table>
  ${_renderHandicapRecs(hc)}`;
}

// Called by warning acknowledgment checkboxes to update the Confirm Close button state.
function _cwmUpdateConfirmState() {
  const checks = document.querySelectorAll('#close-week-modal-body .cwm-ack-check');
  document.getElementById('close-week-confirm-btn').disabled = [...checks].some(c => !c.checked);
}

async function reviewCloseWeek(seasonId, weekNum, ackCount = 0) {
  const [result, preview] = await Promise.all([
    api('GET', `/seasons/${seasonId}/weeks/${weekNum}/validate`),
    api('GET', `/seasons/${seasonId}/weeks/${weekNum}/advance-preview`).catch(() => null),
  ]);
  const msgs = result.messages || [];
  const errors = msgs.filter(m => m.level === 'error');
  const warnings = msgs.filter(m => m.level === 'warning');

  let body = '';

  // Prior history notice: week was previously closed with acknowledged warnings.
  if (ackCount > 0) {
    const label = ackCount === 1 ? '1 prior acknowledgment' : `${escapeHTML(String(ackCount))} prior acknowledgments`;
    body += `<div class="alert alert-secondary py-2 mb-3 small">
      <i class="bi bi-clock-history me-1"></i>
      This week has ${label} from a previous close.
      <button type="button" class="btn btn-link btn-sm p-0 ms-1" data-action="load-prior-acks" data-season-id="${escapeHTML(String(seasonId))}" data-week-num="${escapeHTML(String(weekNum))}">View</button>
      <div class="mt-2 d-none" id="cwm-prior-acks" data-loaded="0"></div>
    </div>`
  }

  if (errors.length) {
    const weekErrors = errors.filter(m => !m.match_id);
    const matchGroups = {};
    errors.filter(m => m.match_id).forEach(m => {
      (matchGroups[m.match_id] = matchGroups[m.match_id] || []).push(m);
    });
    let errHtml = weekErrors.length
      ? `<ul class="mt-1 mb-2">${weekErrors.map(m => `<li>${escapeHTML(m.message)}</li>`).join('')}</ul>`
      : '';
    Object.entries(matchGroups).forEach(([mid, msgs]) => {
      errHtml += `<div class="mb-2"><button class="badge bg-warning text-dark border-0 me-1" data-action="open-match-entry" data-season-id="${escapeHTML(String(seasonId))}" data-match-id="${mid}" title="Go to match entry">Match ${mid}</button><ul class="mb-0 mt-1">${msgs.map(m => `<li>${escapeHTML(m.message)}</li>`).join('')}</ul></div>`;
    });
    body += `<div class="mb-3"><strong class="text-danger"><i class="bi bi-x-circle me-1"></i>Errors - must fix before close</strong>${errHtml}</div>`;
  }

  if (warnings.length && !errors.length) {
    // Build an acknowledgment checklist: one checkbox + optional notes per warning.
    const ackItems = warnings.map((w, i) => {
      const matchLabel = w.match_id
        ? `<span class="badge bg-warning text-dark me-1">Match ${w.match_id}</span>`
        : '';
      const fieldLabel = w.field
        ? ` <span class="text-muted small">(${escapeHTML(w.field)})</span>`
        : '';
      return `<div class="border rounded p-2 mb-2">
        <div class="form-check mb-1">
          <input class="form-check-input cwm-ack-check" type="checkbox" id="cwm-ack-${i}"
                 data-idx="${i}">
          <label class="form-check-label fw-semibold" for="cwm-ack-${i}">
            ${matchLabel}${escapeHTML(w.code)}${fieldLabel}
          </label>
        </div>
        <div class="ps-3 text-muted small mb-1">${escapeHTML(w.message)}</div>
        <input type="text" class="form-control form-control-sm cwm-ack-notes"
               data-idx="${i}" placeholder="Notes (optional)">
      </div>`;
    }).join('');
    body += `<div class="mb-3">
      <strong class="text-warning"><i class="bi bi-exclamation-triangle me-1"></i>Warnings - acknowledge each before close</strong>
      <div class="mt-2">${ackItems}</div>
    </div>`;
  } else if (warnings.length) {
    // Errors are present; show warnings as read-only context.
    const warnItems = warnings.map(w => {
      const matchBadge = w.match_id ? `<span class="badge bg-warning text-dark me-1">Match ${w.match_id}</span>` : '';
      const fieldLabel = w.field ? ` <span class="text-muted small">(${escapeHTML(w.field)})</span>` : '';
      return `<li>${matchBadge}${escapeHTML(w.message)}${fieldLabel}</li>`;
    }).join('');
    body += `<div class="mb-3"><strong class="text-warning"><i class="bi bi-exclamation-triangle me-1"></i>Warnings</strong><ul class="mt-1 mb-0">${warnItems}</ul></div>`;
  }

  if (!errors.length && !warnings.length) {
    body = '<p class="text-success mb-2"><i class="bi bi-check-circle me-1"></i>All checks passed. Ready to close.</p>';
  }
  if (!errors.length) {
    body += '<div class="alert alert-info py-2 mb-0 small"><i class="bi bi-info-circle me-1"></i>Closing makes this week\'s results official. Standings and player stats will update immediately and will reflect this week\'s outcomes.</div>';
  }
  if (preview) {
    body += _renderAdvancePreview(preview);
  }

  const modal = new bootstrap.Modal(document.getElementById('close-week-modal'));
  const modalBody = document.getElementById('close-week-modal-body');
  modalBody.innerHTML = body;
  const confirmBtn = document.getElementById('close-week-confirm-btn');

  // Disable if errors exist, or if there are warnings that need acknowledgment.
  confirmBtn.disabled = errors.length > 0 || warnings.length > 0;
  confirmBtn.textContent = (warnings.length && !errors.length) ? 'Acknowledge & Close' : 'Confirm Close';
  modalBody.querySelectorAll('.cwm-ack-check').forEach(check => {
    check.addEventListener('change', _cwmUpdateConfirmState);
  });

  confirmBtn.onclick = async () => {
    // Collect acknowledgments from any warning checkboxes.
    const acks = [];
    if (warnings.length && !errors.length) {
      warnings.forEach((w, i) => {
        const noteEl = modalBody.querySelector(`.cwm-ack-notes[data-idx="${i}"]`);
        acks.push({
          match_id: w.match_id || 0,
          warning_code: w.code,
          field: w.field || '',
          notes: noteEl ? noteEl.value.trim() : ''
        });
      });
    }
    try {
      const closeData = await api('POST', `/seasons/${seasonId}/weeks/${weekNum}/close`, { acknowledgments: acks });
      // Replace modal body with success summary; let admin read before dismissing.
      modalBody.innerHTML = _renderCloseSuccess(closeData, weekNum);
      confirmBtn.textContent = 'Done';
      confirmBtn.disabled = false;
      confirmBtn.onclick = () => modal.hide();
      loadSchedule();
      loadStandings();
      const statsSeasonId = document.getElementById('stats-season-select').value;
      if (statsSeasonId) loadPlayerStats();
    } catch (e) {
      toast('Close failed: ' + (e.message || e), 'danger');
    }
  };
  modal.show();
}

function confirmReopenWeek(seasonId, weekNum, matchDate) {
  const modalBody = document.getElementById('reopen-week-modal-body');
  const dateStr = matchDate ? ` (${displayDate(matchDate)})` : '';
  modalBody.innerHTML = `
    <p class="mb-2">Are you sure you want to reopen Week ${weekNum}${dateStr}?</p>
    <div class="alert alert-warning mb-0">
      <i class="bi bi-exclamation-triangle me-1"></i>
      <strong>Reopening this week will:</strong>
      <ul class="mb-0 mt-1">
        <li>Remove it from official standings and player stats until re-closed</li>
        <li>Allow scores to be corrected or re-saved</li>
        <li>Preserve all previously saved scores (nothing is deleted)</li>
        <li>Preserve prior warning acknowledgments as historical record</li>
        <li>Allow the week to be re-closed normally when corrections are complete</li>
      </ul>
    </div>`;
  const modal = new bootstrap.Modal(document.getElementById('reopen-week-modal'));
  reopenWeekContext = { seasonId, weekNum, modal };
  modal.show();
}

function openMatchEntry(matchId, seasonId) {
  const openModal = document.querySelector('.modal.show');
  if (openModal) bootstrap.Modal.getInstance(openModal)?.hide();
  _entryPreSelectSeasonId = seasonId || null;
  _entryPreSelectMatchId = matchId;
  document.querySelector('[data-section="entry"]').click();
}

// ── Schedule Poster ───────────────────────────────────────────────────────────

// Pool ball colours (solid 1-8, then stripes repeat tinted)
const POSTER_BALLS = [
  { bg:'#f5c500', fg:'#000' }, // 1 yellow
  { bg:'#1547b8', fg:'#fff' }, // 2 blue
  { bg:'#d91414', fg:'#fff' }, // 3 red
  { bg:'#6a24a0', fg:'#fff' }, // 4 purple
  { bg:'#f07a00', fg:'#000' }, // 5 orange
  { bg:'#1a7828', fg:'#fff' }, // 6 green
  { bg:'#8b0000', fg:'#fff' }, // 7 maroon
  { bg:'#222',    fg:'#fff' }, // 8 black
];

function pocketHTML() {
  return ['tl','tm','tr','bl','bm','br'].map(p =>
    `<div class="poster-pocket poster-pocket-${p}"></div>`
  ).join('');
}

function posterDateLabel(matchDate, weekNumber) {
  if (!matchDate) return `Wk ${weekNumber}`;
  const datePart = String(matchDate).split('T')[0];
  const parts = datePart.split('-').map(n => parseInt(n, 10));
  if (parts.length === 3 && parts.every(Number.isFinite)) {
    return `${parts[1]}/${parts[2]}`;
  }
  const d = new Date(matchDate);
  return Number.isNaN(d.getTime()) ? `Wk ${weekNumber}` : `${d.getMonth()+1}/${d.getDate()}`;
}

async function openSchedulePoster() {
  const seasonId = document.getElementById('schedule-season-select').value;
  if (!seasonId) { toast('Select a season first', 'warning'); return; }

  const [matches, players] = await Promise.all([
    api('GET', `/matches?season_id=${seasonId}`),
    api('GET', `/players?league_id=${activeLeague.id}`)
  ]);

  if (!matches.length) { toast('No matches scheduled yet', 'warning'); return; }

  const season = allSeasons.find(s => s.id == seasonId);

  // ── Assign team numbers (alphabetical by name for predictability) ──
  const seenIds = new Set();
  matches.forEach(m => { seenIds.add(m.home_team_id); seenIds.add(m.away_team_id); });

  // Build team list from match data (we already have team names on matches)
  const teamMap = {};
  matches.forEach(m => {
    if (m.home_team_id) teamMap[m.home_team_id] = m.home_team_name;
    if (m.away_team_id) teamMap[m.away_team_id] = m.away_team_name;
  });
  const seasonTeams = Object.entries(teamMap)
    .map(([id, name]) => ({ id: parseInt(id), name }))
    .sort((a, b) => a.name.localeCompare(b.name));
  const teamNum = {}; // teamId → 1-based number
  seasonTeams.forEach((t, i) => teamNum[t.id] = i + 1);

  // ── Players by team ──
  const playersByTeam = {};
  players.forEach(p => {
    if (p.team_id && teamMap[p.team_id]) {
      (playersByTeam[p.team_id] = playersByTeam[p.team_id] || []).push(p.name);
    }
  });

  // ── Group matches by week ──
  const byWeek = {};
  matches.forEach(m => {
    (byWeek[m.week_number] = byWeek[m.week_number] || []).push(m);
  });
  const weeks = Object.keys(byWeek).sort((a, b) => a - b);

  // ── Ball row decoration ──
  const balls = seasonTeams.slice(0, 8).map((_, i) => {
    const b = POSTER_BALLS[i];
    return `<div class="poster-ball" style="background:${b.bg};color:${b.fg}">${i+1}</div>`;
  }).join('');

  // ── Schedule rows ──
  const rows = weeks.map(wn => {
    const wMatches = byWeek[wn].sort((a, b) =>
      (a.match_number ?? 999) - (b.match_number ?? 999)
    );
    const dateStr  = posterDateLabel(wMatches[0].match_date, wn);

    const playingIds = new Set();
    wMatches.forEach(m => { playingIds.add(m.home_team_id); playingIds.add(m.away_team_id); });
    const byeTeams = seasonTeams.filter(t => !playingIds.has(t.id));
    const byeStr   = byeTeams.length
      ? `BYE: ${byeTeams.map(t => teamNum[t.id]).join(', ')}`
      : '';

    const pairings = wMatches.map(m => {
      const h  = teamNum[m.home_team_id] || '?';
      const a  = teamNum[m.away_team_id] || '?';
      const mn = m.match_number != null
        ? `<sup class="poster-matchnum">${m.match_number}</sup>` : '';
      return `<span class="poster-pairing">${h}–${a}${mn}</span>`;
    }).join('');

    return `<tr>
      <td class="poster-wk">${wn}</td>
      <td class="poster-date">${dateStr}</td>
      <td class="poster-pairings">${pairings}</td>
      <td class="poster-bye-col">${byeStr ? `<span class="poster-bye">${byeStr}</span>` : ''}</td>
    </tr>`;
  }).join('');

  // ── Teams grid ──
  const teamCards = seasonTeams.map(t => {
    const players = (playersByTeam[t.id] || []).join(', ');
    return `<div>
      <div class="poster-team-name">${teamNum[t.id]}. ${t.name}</div>
      ${players ? `<div class="poster-team-players">${players}</div>` : ''}
    </div>`;
  }).join('');

  // ── Title ──
  const leagueName = activeLeague?.name || 'League';
  const shortName  = leagueName.replace(/\bLeague\b/i, '').trim();
  const title      = shortName ? shortName + ' Schedule' : 'Schedule';
  const subtitle   = season?.name || '';

  // ── Render ──
  const posterHTML = `
    <div class="poster-outer">
      ${pocketHTML()}
      <div class="poster-balls">${balls}</div>
      <div class="poster-inner">
        <div class="poster-title">${title}</div>
        ${subtitle ? `<div class="poster-subtitle">${subtitle}</div>` : ''}
        <hr class="poster-divider">
        <table class="poster-sched">
          <thead><tr>
            <th class="poster-wk">Wk</th>
            <th class="poster-date">Date</th>
            <th class="poster-pairings">Pairings</th>
            <th class="poster-bye-col">Bye</th>
          </tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
    </div>
    <div class="poster-teams-box">
      ${pocketHTML()}
      <div class="poster-inner">
        <div class="poster-teams-hdr">Teams</div>
        <div class="poster-teams-grid">${teamCards}</div>
      </div>
    </div>`;

  document.getElementById('poster-content').innerHTML = posterHTML;
  document.getElementById('schedule-table-view').classList.add('d-none');
  document.getElementById('schedule-poster-view').classList.remove('d-none');
}

function closeSchedulePoster() {
  document.getElementById('schedule-poster-view').classList.add('d-none');
  document.getElementById('schedule-table-view').classList.remove('d-none');
}


// ── Standings ─────────────────────────────────────────────────────────────────
async function loadStandings() {
  const seasonId = document.getElementById('standings-season-select').value;
  if (!seasonId) return;
  const standings = await api('GET', `/standings?season_id=${seasonId}`);
  const tbody = document.querySelector('#standings-table tbody');
  tbody.innerHTML = standings.map((s,i) => `
    <tr ${i===0?'class="table-warning fw-bold"':''}>
      <td>${i+1}</td>
      <td>${s.team_name}</td>
      <td>${s.played}</td>
      <td>${s.wins}</td>
      <td>${s.losses}</td>
      <td>${s.ties}</td>
      <td class="fw-bold">${s.points}</td>
      <td>${s.games_won}</td>
      <td>${s.games_lost}</td>
      <td>${(s.win_pct*100).toFixed(1)}%</td>
    </tr>`).join('') || '<tr><td colspan="10" class="text-center text-muted py-3">No completed matches yet</td></tr>';
}

// ── Player Stats ──────────────────────────────────────────────────────────────
async function loadPlayerStats() {
  const seasonId = document.getElementById('stats-season-select').value;
  if (!seasonId) return;
  const stats = await api('GET', `/player-stats?season_id=${seasonId}`);
  const tbody = document.querySelector('#stats-table tbody');
  tbody.innerHTML = stats.map(s => `
    <tr>
      <td class="text-muted small">${s.player_number||'—'}</td>
      <td>${s.player_name}</td>
      <td>${s.team_name}</td>
      <td><span class="badge bg-secondary badge-hc">${s.handicap}</span></td>
      <td>${s.sets_won}</td>
      <td>${s.sets_lost}</td>
      <td>${s.games_won}</td>
      <td>${s.games_lost}</td>
      <td>${(s.win_pct*100).toFixed(1)}%</td>
    </tr>`).join('') || '<tr><td colspan="9" class="text-center text-muted py-3">No stats yet</td></tr>';
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
