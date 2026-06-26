
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
    case 'seasons':   loadSeasons(); break;
    case 'teams':     loadTeams(); break;
    case 'players':   loadPlayers(); break;
    case 'schedule':  populateScheduleSeasonSelect(); break;
    case 'lineup':    loadLineupSection(); break;
    case 'entry':     populateSeasonSelect('entry-season-select', loadEntryMatches); break;
    case 'standings': populateSeasonSelect('standings-season-select', loadStandings); break;
    case 'stats':     populateSeasonSelect('stats-season-select', loadPlayerStats); break;
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

// ── Seasons ───────────────────────────────────────────────────────────────────
let currentMgmtSeasonId = null;
let currentMgmtSeasonTeamCount = 0;
let currentMgmtSeasonManaged = false;

const scheduleTypeLabel = {
  single_rr: 'Single RR', double_rr: 'Double RR',
  split: 'Split', custom: 'Custom', blanket: 'Blanket'
};
const scheduleTypeFull = {
  single_rr: 'Single Round Robin',
  double_rr: 'Double Round Robin',
  split:     'Split Season',
  custom:    'Custom (Fixed Weeks)',
  blanket:   'Blanket (Empty Slots)'
};

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

async function loadSeasons() {
  if (!activeLeague) return;
  allSeasons = await api('GET', `/seasons?league_id=${activeLeague.id}`);

  // Also load matches for all seasons in one call so we can show counts without
  // N separate requests. We fetch matches for each season lazily only if the
  // list is small; otherwise show a neutral placeholder.
  // Strategy: load matches for the active season only; show "–" for others.
  let matchCounts = {}; // seasonId -> { total, done }
  if (allSeasons.length <= 8) {
    await Promise.all(allSeasons.map(async s => {
      try {
        const ms = await api('GET', `/matches?season_id=${s.id}`);
        matchCounts[s.id] = { total: ms.length, done: ms.filter(m => m.completed).length };
      } catch(_) {}
    }));
  }

  const tbody = document.querySelector('#seasons-table tbody');
  tbody.innerHTML = allSeasons.map(s => {
    const mc = matchCounts[s.id];
    let schedInfo = '';
    if (mc) {
      if (mc.total === 0) {
        schedInfo = '<span class="text-warning small"><i class="bi bi-exclamation-triangle"></i> No schedule</span>';
      } else {
        schedInfo = `<span class="text-muted small">${mc.done}/${mc.total} done</span>`;
      }
    }
    const typeLabel = scheduleTypeLabel[s.schedule_type] || s.schedule_type || '—';
    const weeksNote = (s.schedule_type === 'custom' || s.schedule_type === 'blanket') && s.num_weeks
      ? ` · ${s.num_weeks}wk` : '';

    return `<tr>
      <td>
        <span class="fw-semibold">${s.name}</span>
        ${s.active ? '<span class="badge bg-success ms-1" style="font-size:.68rem">Active</span>' : ''}
      </td>
      <td class="text-muted small">${fmtDateRange(s.start_date, s.end_date)}</td>
      <td>
        <span class="badge bg-secondary" style="font-size:.7rem">${typeLabel}${weeksNote}</span>
      </td>
      <td>${schedInfo}</td>
      <td class="text-end" style="white-space:nowrap">
        <button class="btn btn-outline-primary btn-sm py-0 me-1" onclick="manageSeason(${s.id})"
          title="Manage: schedule, skip weeks, bye requests, activation">
          <i class="bi bi-sliders"></i> Manage
        </button>
        ${(!s.active && !s.activated_at) ? `<button class="btn btn-outline-success btn-sm py-0 me-1" onclick="activateSeason(${s.id})">Activate</button>` : ''}
        <button class="btn btn-outline-secondary btn-sm py-0 me-1" onclick="editSeason(${s.id})"
          title="Edit Details: name, start date, schedule type, rules">
          <i class="bi bi-pencil"></i> Edit Details
        </button>
        <button class="btn btn-outline-danger btn-sm py-0" onclick="deleteSeason(${s.id})"
          title="Delete this season and all its matches"><i class="bi bi-trash"></i></button>
      </td>
    </tr>`;
  }).join('') || '<tr><td colspan="5" class="text-center text-muted py-3">No seasons yet. Click "New Season" to add one.</td></tr>';
}

function openNewSeason() {
  document.getElementById('season-modal-title').textContent = 'New Season';
  document.getElementById('season-id').value = '';
  document.getElementById('season-name').value = '';
  document.getElementById('season-start').value = '';
  document.getElementById('season-type').value = 'double_rr';
  const sel = document.getElementById('season-copy-from');
  sel.innerHTML = '<option value="">— All teams in league —</option>' +
    allSeasons.map(s => `<option value="${s.id}">${s.name}</option>`).join('');

  // Show rules in "pending" mode — changes are buffered until the season is saved.
  document.getElementById('season-rules-section').classList.remove('d-none');
  document.getElementById('season-rules-collapse').classList.remove('show');
  const rulesToggle = document.querySelector('[data-bs-target="#season-rules-collapse"]');
  rulesToggle?.classList.add('collapsed');
  rulesToggle?.setAttribute('aria-expanded', 'false');
  document.getElementById('rules-editor').initNew();

  openModal('season-modal');
}

function editSeason(id) {
  const s = allSeasons.find(x => x.id === id);
  if (!s) return;
  document.getElementById('season-modal-title').textContent = 'Edit Season Details';
  document.getElementById('season-id').value = s.id;
  document.getElementById('season-name').value = s.name;
  document.getElementById('season-start').value = s.start_date ? s.start_date.slice(0, 10) : '';
  document.getElementById('season-type').value = s.schedule_type || 'double_rr';
  const sel = document.getElementById('season-copy-from');
  sel.innerHTML = '<option value="">— All teams in league —</option>' +
    allSeasons.filter(x => x.id !== id).map(x => `<option value="${x.id}">${x.name}</option>`).join('');
  // Show rules for existing seasons.
  document.getElementById('season-rules-section').classList.remove('d-none');
  document.getElementById('season-rules-collapse').classList.add('show');
  const rulesToggle = document.querySelector('[data-bs-target="#season-rules-collapse"]');
  rulesToggle?.classList.remove('collapsed');
  rulesToggle?.setAttribute('aria-expanded', 'true');
  document.getElementById('rules-editor').loadSeason(id);
  openModal('season-modal');
}

async function saveSeason() {
  const id = document.getElementById('season-id').value;
  const copyFrom = parseInt(document.getElementById('season-copy-from').value) || 0;
  const body = {
    name:          document.getElementById('season-name').value.trim(),
    start_date:    document.getElementById('season-start').value || null,
    schedule_type: document.getElementById('season-type').value || 'double_rr',
    league_id:     activeLeague?.id
  };
  if (!body.name)      { toast('Name is required','warning'); return; }
  if (!body.league_id) { toast('No active league selected','warning'); return; }
  try {
    let saved;
    if (id) saved = await api('PUT', `/seasons/${id}`, body);
    else    saved = await api('POST', '/seasons', body);
    // Flush any rule changes that were staged before the season record existed.
    if (!id && saved?.id) {
      await document.getElementById('rules-editor').flushPending(saved.id);
    }
    closeModal('season-modal');
    toast('Season saved');
    allSeasons = await api('GET', `/seasons?league_id=${activeLeague.id}`);
    activeSeason = allSeasons.find(s => s.active) || null;
    document.getElementById('active-season-label').textContent =
      activeSeason ? '📅 ' + activeSeason.name : 'No active season';
    await loadSeasons();
    // If new season: open management panel with copy-from pre-selected
    if (!id && saved?.id) {
      manageSeason(saved.id, copyFrom);
    }
  } catch(e) { toast(e.message,'danger'); }
}

async function activateSeason(id) {
  try {
    await api('POST', `/seasons/${id}/activate`);
    toast('Season activated');
    allSeasons = await api('GET', `/seasons?league_id=${activeLeague.id}`);
    activeSeason = allSeasons.find(s => s.active) || null;
    document.getElementById('active-season-label').textContent =
      activeSeason ? '📅 ' + activeSeason.name : 'No active season';
    await loadSeasons();
    if (currentMgmtSeasonId === id) await manageSeason(id);
  } catch(e) { toast(e.message,'danger'); }
}

async function deleteSeason(id) {
  if (!confirm('Delete this season and all its matches?')) return;
  try {
    await api('DELETE', `/seasons/${id}`);
    toast('Deleted');
    if (currentMgmtSeasonId === id) closeMgmt();
    loadSeasons();
  } catch(e) { toast(e.message,'danger'); }
}

// ── Season Management Panel ───────────────────────────────────────────────────
async function manageSeason(id, preselectedFromSeasonId) {
  currentMgmtSeasonId = id;
  const s = allSeasons.find(x => x.id === id);
  if (!s) return;

  // Ensure the Seasons section is the active view (panel lives inside it).
  if (!document.getElementById('section-seasons').classList.contains('active')) {
    document.querySelectorAll('.section').forEach(el => el.classList.remove('active'));
    document.querySelectorAll('[data-section]').forEach(el => el.classList.remove('active'));
    document.getElementById('section-seasons').classList.add('active');
    document.querySelector('[data-section="seasons"]')?.classList.add('active');
  }

  // ── Header ──
  document.getElementById('season-mgmt').classList.remove('d-none');
  document.getElementById('season-mgmt-title').textContent = s.name;
  document.getElementById('season-mgmt-active').classList.toggle('d-none', !s.active);

  // Meta line: date range · schedule type
  const meta = [
    fmtDateRange(s.start_date, s.end_date),
    scheduleTypeFull[s.schedule_type] || s.schedule_type
  ].filter(Boolean).join(' · ');
  document.getElementById('season-mgmt-meta').textContent = meta;

  // "Set Active" button — show only when not active
  const isManaged = s.teams_managed === true || s.teams_managed === 1;
  const isDraft = !s.active && !s.activated_at;
  currentMgmtSeasonManaged = isManaged;

  const setActiveBtn = document.getElementById('season-mgmt-set-active-btn');
  setActiveBtn.style.display = isDraft ? '' : 'none';
  setActiveBtn.disabled = true;

  // ── Schedule setup ──
  const fromSel = document.getElementById('gen-from-season');
  document.getElementById('gen-from-season-col').classList.toggle('d-none', isManaged);
  document.getElementById('gen-managed-teams-note').classList.toggle('d-none', !isManaged);
  if (isManaged) {
    fromSel.innerHTML = '<option value="">Registered teams in this season</option>';
  } else {
    fromSel.innerHTML = '<option value="">All teams in league</option>' +
      allSeasons.filter(x => x.id !== id).map(x =>
        `<option value="${x.id}" ${x.id === preselectedFromSeasonId ? 'selected' : ''}>${x.name}</option>`
      ).join('');
  }

  document.getElementById('gen-start-date').value = s.start_date ? s.start_date.slice(0, 10) : '';
  if (s.schedule_type) document.getElementById('gen-type').value = s.schedule_type;
  if (s.num_weeks)     document.getElementById('gen-numweeks').value = s.num_weeks;
  onGenTypeChange();
  document.getElementById('season-schedule-preview-note').classList.toggle('d-none', s.active);

  let seasonTeams = [];
  let checklist = null;
  try { seasonTeams = await api('GET', `/seasons/${id}/teams`); }
  catch(e) { toast(`Could not load registered teams: ${e.message}`, 'danger'); }
  if (isDraft) {
    try { checklist = await api('GET', `/seasons/${id}/checklist`); }
    catch(e) { toast(`Could not load setup checklist: ${e.message}`, 'danger'); }
  }

  const byeTeams = isManaged ? seasonTeams : allTeams.map(t => ({
    team_id: t.id,
    season_name: t.name,
    team_name: t.name,
    roster_count: 0
  }));
  currentMgmtSeasonTeamCount = byeTeams.length;

  renderSeasonChecklist(checklist, isDraft);
  renderSeasonRegisteredTeams(seasonTeams, isDraft ? checklist : null, isManaged);

  if (isDraft && checklist) {
    setActiveBtn.disabled = !checklist.can_activate;
    setActiveBtn.classList.toggle('disabled', !checklist.can_activate);
    setActiveBtn.title = checklist.can_activate ? '' : 'Resolve setup blockers before activating this season.';
  }

  // Populate bye-team dropdown from the season roster for managed seasons.
  const teamSel = document.getElementById('bye-team-select');
  teamSel.innerHTML = byeTeams.map(t =>
    `<option value="${t.team_id}">${escapeHTML(t.season_name || t.team_name)}</option>`).join('') ||
    '<option value="">No teams registered</option>';

  showSeasonTab('schedule');
  document.getElementById('season-mgmt').scrollIntoView({ behavior: 'smooth', block: 'start' });

  await Promise.all([
    loadSkippedWeeks(id),
    loadByeRequests(id, currentMgmtSeasonTeamCount),
  ]);
}

function closeMgmt() {
  document.getElementById('season-mgmt').classList.add('d-none');
  currentMgmtSeasonId = null;
  currentMgmtSeasonTeamCount = 0;
  currentMgmtSeasonManaged = false;
}

function activateMgmtSeason() {
  if (currentMgmtSeasonId) activateSeason(currentMgmtSeasonId);
}

function renderSeasonChecklist(checklist, isDraft) {
  const section = document.getElementById('season-checklist-section');
  const body = section.querySelector('.card-body');
  if (!checklist) {
    section.classList.add('d-none');
    body.innerHTML = '';
    return;
  }

  const blockers = checklist.blockers || [];
  const warnings = checklist.warnings || [];
  const rows = [];
  blockers.forEach(item => rows.push(`
    <div class="text-danger d-flex gap-2 align-items-start mb-1">
      <i class="bi bi-x-circle-fill"></i>
      <span><strong>${escapeHTML(item.code)}</strong>: ${escapeHTML(item.message)}</span>
    </div>`));
  warnings.forEach(item => rows.push(`
    <div class="text-warning d-flex gap-2 align-items-start mb-1">
      <i class="bi bi-exclamation-triangle-fill"></i>
      <span><strong>${escapeHTML(item.code)}</strong>: ${escapeHTML(item.message)}</span>
    </div>`));

  const status = checklist.can_activate
    ? '<div class="text-success"><i class="bi bi-check-circle-fill"></i> Setup checklist is clear.</div>'
    : '<div class="fw-semibold text-danger mb-1"><i class="bi bi-clipboard-x"></i> Resolve setup blockers before activation.</div>';
  const activeNote = isDraft ? '' : '<div class="text-muted">Activation controls are hidden for active and historical seasons.</div>';
  body.innerHTML = `
    <div class="fw-semibold mb-1"><i class="bi bi-clipboard-check"></i> Setup Checklist</div>
    ${status}
    ${rows.join('') || ''}
    ${activeNote}`;
  section.classList.remove('d-none');
}

function renderSeasonRegisteredTeams(teams, checklist, isManaged) {
  const section = document.getElementById('season-registered-teams-section');
  const body = section.querySelector('.card-body');
  if (!isManaged) {
    section.classList.add('d-none');
    body.innerHTML = '';
    return;
  }

  const blockers = checklist?.blockers || [];
  const warnings = checklist?.warnings || [];
  const items = [...blockers, ...warnings];
  const rows = teams.map(t => {
    const name = t.season_name || t.team_name;
    const teamIssues = items.filter(item => item.message && item.message.indexOf(`team "${name}"`) >= 0);
    const issueHtml = teamIssues.map(item => `
      <div class="${(checklist?.blockers || []).includes(item) ? 'text-danger' : 'text-warning'} small">
        <i class="bi bi-exclamation-triangle"></i> ${escapeHTML(item.message)}
      </div>`).join('');
    return `
      <div class="border rounded px-2 py-1 mb-1">
        <div class="d-flex justify-content-between gap-2">
          <span class="fw-semibold">${escapeHTML(name)}</span>
          <span class="badge bg-secondary">${t.roster_count} player${t.roster_count === 1 ? '' : 's'}</span>
        </div>
        <div class="text-muted">
          Captain: ${t.captain_name ? escapeHTML(t.captain_name) : '<span class="fst-italic">No captain</span>'}
        </div>
        ${issueHtml}
      </div>`;
  }).join('');

  body.innerHTML = `
    <div class="fw-semibold mb-1"><i class="bi bi-people"></i> Registered Teams (${teams.length})</div>
    ${rows || '<div class="text-muted fst-italic">No teams registered for this season yet.</div>'}`;
  section.classList.remove('d-none');
}

// Quick-nav from manage panel → Schedule tab (admin preview; bypasses active-only filter).
function goToSchedule() {
  if (!currentMgmtSeasonId) return;
  const id = currentMgmtSeasonId;
  navTo('schedule');
  setTimeout(() => populateScheduleSeasonSelect(id), 50);
}

// Quick-nav from manage panel → Schedule tab, then open poster.
function goToPoster() {
  if (!currentMgmtSeasonId) return;
  const id = currentMgmtSeasonId;
  navTo('schedule');
  setTimeout(() => { populateScheduleSeasonSelect(id); openSchedulePoster(); }, 100);
}

function showSeasonTab(tab) {
  document.querySelectorAll('[data-stab]').forEach(a => a.classList.remove('active'));
  document.querySelectorAll('#section-seasons [id^="stab-"]').forEach(d => d.classList.add('d-none'));
  document.querySelector(`[data-stab="${tab}"]`)?.classList.add('active');
  document.getElementById(`stab-${tab}`)?.classList.remove('d-none');
}

// Wire up management tab clicks
document.querySelectorAll('[data-stab]').forEach(link => {
  link.addEventListener('click', e => { e.preventDefault(); showSeasonTab(link.dataset.stab); });
});

function onGenTypeChange() {
  const t = document.getElementById('gen-type').value;
  document.getElementById('gen-numweeks-col').style.display = (t === 'custom' || t === 'blanket') ? '' : 'none';
  document.getElementById('gen-mpw-col').style.display = t === 'blanket' ? '' : 'none';
  const descs = {
    single_rr: '<strong>Single Round Robin</strong> — Each team plays every other team once. Good for short seasons.',
    double_rr: '<strong>Double Round Robin</strong> — Each team plays every other team twice: once as home, once as visitor. Standard full-season format.',
    split:     '<strong>Split Season</strong> — Double round-robin split into two equal halves. Standings reset at the midpoint; teams play for each half separately.',
    custom:    '<strong>Custom (Fixed Weeks)</strong> — Repeats the pairing rotation for exactly the number of weeks you specify. Use when you want to control the exact length.',
    blanket:   '<strong>Blanket (Empty Slots)</strong> — Creates N weeks of blank match slots without assigning teams. Use "Matches per Week" to control how many slots appear each week, then assign teams to slots manually.'
  };
  const el = document.getElementById('gen-type-desc');
  el.innerHTML = descs[t] || '';
}

async function generateSchedule() {
  if (!currentMgmtSeasonId) { toast('No season selected', 'warning'); return; }
  const type      = document.getElementById('gen-type').value;
  const startDate = document.getElementById('gen-start-date').value;
  const numWeeks  = parseInt(document.getElementById('gen-numweeks').value) || 0;
  const mpw       = parseInt(document.getElementById('gen-mpw').value) || 1;
  const fromSeason = currentMgmtSeasonManaged
    ? 0
    : (parseInt(document.getElementById('gen-from-season').value) || 0);

  // Fetch skip dates for this season
  let skipDates = [];
  try {
    const skips = await api('GET', `/seasons/${currentMgmtSeasonId}/skipped-weeks`);
    skipDates = skips.map(s => s.skip_date);
  } catch(_) {}

  const body = {
    season_id:        currentMgmtSeasonId,
    start_date:       startDate || '',
    schedule_type:    type,
    num_weeks:        numWeeks,
    matches_per_week: mpw,
    skip_dates:       skipDates,
    from_season_id:   fromSeason
  };

  if (!confirm(`This will replace all unplayed matches for this season. Completed matches are preserved. Continue?`)) return;

  try {
    const res = await api('POST', '/matches/generate', body);
    toast(`Schedule generated: ${res.matches_created} matches`);
    // Refresh seasons to pick up newly computed end_date
    allSeasons = await api('GET', `/seasons?league_id=${activeLeague.id}`);
    activeSeason = allSeasons.find(s => s.active) || null;
    document.getElementById('active-season-label').textContent =
      activeSeason ? '📅 ' + activeSeason.name : 'No active season';
    await loadSeasons();
    if (currentMgmtSeasonId) await manageSeason(currentMgmtSeasonId);
  } catch(e) { toast(e.message, 'danger'); }
}

// ── Skip Weeks ────────────────────────────────────────────────────────────────
async function loadSkippedWeeks(seasonId) {
  let skips = [];
  try { skips = await api('GET', `/seasons/${seasonId}/skipped-weeks`); } catch(_) {}
  const tbody = document.querySelector('#skips-table tbody');
  tbody.innerHTML = skips.length
    ? skips.map(sk => `<tr data-skip-date="${sk.skip_date}">
        <td>${displayDate(sk.skip_date)}</td>
        <td>${sk.reason || '<span class="text-muted">—</span>'}</td>
        <td class="text-end">
          <button class="btn btn-outline-danger btn-sm py-0" onclick="deleteSkippedWeek(${sk.id})"><i class="bi bi-trash"></i></button>
        </td>
      </tr>`).join('')
    : '<tr><td colspan="3" class="text-muted text-center py-2">No skipped dates yet</td></tr>';
}

async function addSkippedWeek() {
  if (!currentMgmtSeasonId) return;
  const date   = document.getElementById('skip-date-input').value;
  const reason = document.getElementById('skip-reason-input').value.trim();
  if (!date) { toast('Date is required', 'warning'); return; }
  try {
    await api('POST', `/seasons/${currentMgmtSeasonId}/skipped-weeks`, { skip_date: date, reason });
    document.getElementById('skip-date-input').value = '';
    document.getElementById('skip-reason-input').value = '';
    toast('Skip date added');
    await loadSkippedWeeks(currentMgmtSeasonId);
  } catch(e) { toast(e.message, 'danger'); }
}

async function deleteSkippedWeek(id) {
  if (!confirm('Remove this skip date?')) return;
  try {
    await api('DELETE', `/seasons/${currentMgmtSeasonId}/skipped-weeks/${id}`);
    toast('Removed');
    await loadSkippedWeeks(currentMgmtSeasonId);
  } catch(e) { toast(e.message, 'danger'); }
}

// ── Bye Requests ──────────────────────────────────────────────────────────────
async function loadByeRequests(seasonId, seasonTeamCount = currentMgmtSeasonTeamCount) {
  let byes = [];
  try { byes = await api('GET', `/seasons/${seasonId}/bye-requests`); } catch(_) {}

  // Show a banner when the team count is even (bye requests cannot be applied).
  const teamCount = seasonTeamCount;
  const isEven = teamCount > 0 && teamCount % 2 === 0;
  const banner = isEven
    ? `<div class="alert alert-warning py-2 mb-2 small">
        <i class="bi bi-exclamation-triangle-fill"></i>
        <strong>${teamCount} teams in this season (even).</strong>
        Bye requests require an odd number of teams — every team plays each week.
      </div>`
    : teamCount % 2 === 1
      ? `<div class="alert alert-info py-2 mb-2 small">
          <i class="bi bi-info-circle"></i>
          ${teamCount} teams (odd) — one team sits out each week.
          Approve a request below to assign which team gets the natural bye on a given week.
        </div>`
      : '';

  const tableContainer = document.querySelector('#stab-byes .card-body');
  let bannerEl = tableContainer.querySelector('.bye-banner');
  if (!bannerEl) {
    bannerEl = document.createElement('div');
    bannerEl.className = 'bye-banner';
    tableContainer.insertBefore(bannerEl, tableContainer.firstChild);
  }
  bannerEl.innerHTML = banner;

  const tbody = document.querySelector('#byes-table tbody');
  tbody.innerHTML = byes.length
    ? byes.map(b => `<tr>
        <td>${b.team_name || b.team_id}</td>
        <td>${b.week_number || 'TBD'}</td>
        <td>${b.reason || '<span class="text-muted">—</span>'}</td>
        <td>
          <div class="form-check form-switch mb-0">
            <input class="form-check-input" type="checkbox" role="switch" id="bye_app_${b.id}"
              ${b.approved ? 'checked' : ''} onchange="toggleByeApproval(${b.id}, this.checked)">
          </div>
        </td>
        <td class="text-end">
          <button class="btn btn-outline-danger btn-sm py-0" onclick="deleteByeRequest(${b.id})"><i class="bi bi-trash"></i></button>
        </td>
      </tr>`).join('')
    : '<tr><td colspan="5" class="text-muted text-center py-2">No bye requests yet</td></tr>';
}

async function addByeRequest() {
  if (!currentMgmtSeasonId) return;
  const teamId = document.getElementById('bye-team-select').value;
  const week   = parseInt(document.getElementById('bye-week-input').value) || 0;
  const reason = document.getElementById('bye-reason-input').value.trim();
  if (!teamId) { toast('Select a team', 'warning'); return; }
  try {
    await api('POST', `/seasons/${currentMgmtSeasonId}/bye-requests`, {
      team_id: parseInt(teamId), week_number: week, reason
    });
    document.getElementById('bye-reason-input').value = '';
    document.getElementById('bye-week-input').value = '0';
    toast('Bye request added');
    await loadByeRequests(currentMgmtSeasonId, currentMgmtSeasonTeamCount);
  } catch(e) { toast(e.message, 'danger'); }
}

async function toggleByeApproval(id, approved) {
  try {
    await api('PUT', `/seasons/${currentMgmtSeasonId}/bye-requests/${id}`, { approved });
    toast(approved ? 'Bye approved' : 'Bye unapproved');
    await loadByeRequests(currentMgmtSeasonId, currentMgmtSeasonTeamCount);
  } catch(e) { toast(e.message, 'danger'); }
}

async function deleteByeRequest(id) {
  if (!confirm('Remove this bye request?')) return;
  try {
    await api('DELETE', `/seasons/${currentMgmtSeasonId}/bye-requests/${id}`);
    toast('Removed');
    await loadByeRequests(currentMgmtSeasonId, currentMgmtSeasonTeamCount);
  } catch(e) { toast(e.message, 'danger'); }
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

// Remove a player from their team (unassign, don't delete)
async function removeFromTeam(playerId) {
  const p = allPlayers.find(x => x.id === playerId);
  if (!p) return;
  if (!confirm(`Remove ${p.name} from their team? The player won't be deleted.`)) return;
  try {
    await api('PUT', `/players/${playerId}`, {
      first_name: p.first_name, last_name: p.last_name,
      phone:      p.phone,      email:     p.email,
      handicap:   p.handicap,   admin_hold: p.admin_hold,
      team_id:    null
    });
    toast(`${p.name} removed from team`);
    loadTeams();
  } catch(e) { toast(e.message, 'danger'); }
}

// Open player modal pre-set to a specific team
function openNewPlayerOnTeam(teamId) {
  document.getElementById('player-modal-title').textContent = 'Add Player to Roster';
  document.getElementById('player-id').value = '';
  document.getElementById('player-number').value = '';
  document.getElementById('player-first-name').value = '';
  document.getElementById('player-last-name').value = '';
  document.getElementById('player-phone').value = '';
  document.getElementById('player-email').value = '';
  document.getElementById('player-handicap').value = '0';
  document.getElementById('player-admin-hold').checked = false;
  setPlayerModalMode(false);
  populateTeamDropdown(teamId);
  openModal('player-modal');
}

// Assign an existing (unassigned) player to a team
async function openAssignPlayer(teamId) {
  // Fetch all players with no team filter to find unassigned ones
  let all;
  try { all = await api('GET', '/players'); } catch(e) { toast(e.message,'danger'); return; }
  const unassigned = all.filter(p => !p.team_id);
  if (!unassigned.length) {
    toast('No unassigned players available. Use "New Player" to create one.', 'warning');
    return;
  }
  // Build a quick inline dropdown — show a small modal
  document.getElementById('assign-team-id').value = teamId;
  const sel = document.getElementById('assign-player-select');
  sel.innerHTML = unassigned.map(p => `<option value="${p.id}">${p.name}${p.player_number?' (#'+p.player_number+')':''}</option>`).join('');
  openModal('assign-modal');
}

async function confirmAssign() {
  const teamId   = parseInt(document.getElementById('assign-team-id').value);
  const playerId = parseInt(document.getElementById('assign-player-select').value);
  const p = (await api('GET', `/players/${playerId}`));
  try {
    await api('PUT', `/players/${playerId}`, {
      first_name: p.first_name,
      last_name:  p.last_name,
      phone:      p.phone,
      email:      p.email,
      handicap:   p.handicap,
      admin_hold: p.admin_hold,
      team_id:    teamId
    });
    closeModal('assign-modal');
    toast('Player assigned to team');
    allPlayers = await api('GET', `/players?league_id=${activeLeague.id}`);
    loadTeams();
  } catch(e) { toast(e.message,'danger'); }
}

function openNewTeam() {
  document.getElementById('team-modal-title').textContent = 'New Team';
  document.getElementById('team-id').value = '';
  document.getElementById('team-name').value = '';
  openModal('team-modal');
}

function editTeam(id) {
  const t = allTeams.find(x => x.id === id);
  if (!t) return;
  document.getElementById('team-modal-title').textContent = 'Edit Team';
  document.getElementById('team-id').value = t.id;
  document.getElementById('team-name').value = t.name;
  openModal('team-modal');
}

async function saveTeam() {
  const id = document.getElementById('team-id').value;
  const body = {
    name: document.getElementById('team-name').value.trim(),
    league_id: activeLeague?.id
  };
  if (!body.name) { toast('Name is required','warning'); return; }
  if (!body.league_id) { toast('No active league selected','warning'); return; }
  try {
    if (id) await api('PUT', `/teams/${id}`, body);
    else    await api('POST', '/teams', body);
    closeModal('team-modal');
    toast('Team saved');
    document.getElementById('team-id').value = '';
    document.getElementById('team-modal-title').textContent = 'New Team';
    allTeams = await api('GET', `/teams?league_id=${activeLeague.id}`);
    loadTeams();
  } catch(e) { toast(e.message,'danger'); }
}

async function deleteTeam(id) {
  if (!confirm('Delete this team? Players will be unassigned.')) return;
  try {
    await api('DELETE', `/teams/${id}`);
    toast('Deleted');
    allTeams = await api('GET', `/teams?league_id=${activeLeague.id}`);
    loadTeams();
  } catch(e) { toast(e.message,'danger'); }
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

// ── Lineup Planning ───────────────────────────────────────────────────────────
function loadLineupSection() {
  populateSeasonSelect('lu-season-sel', loadLineupWeeks);
}

async function loadLineupWeeks() {
  const seasonId = document.getElementById('lu-season-sel').value;
  if (!seasonId) return;
  const matches = await api('GET', `/matches?season_id=${seasonId}`);
  const weeks = {};
  matches.forEach(m => { if (!weeks[m.week_number]) weeks[m.week_number] = m.match_date || 'TBD'; });
  const sorted = Object.keys(weeks).sort((a,b) => parseInt(a)-parseInt(b));
  const weekSel = document.getElementById('lu-week-sel');
  weekSel.innerHTML =
    '<option value="0">⭐ Default Lineup</option>' +
    (sorted.length
      ? sorted.map(w => `<option value="${w}">Week ${w} — ${displayDate(weeks[w])}</option>`).join('')
      : '');
  loadLineupForWeek();
}

async function loadLineupForWeek() {
  const seasonId = document.getElementById('lu-season-sel').value;
  const weekNum  = parseInt(document.getElementById('lu-week-sel').value || '0');
  if (!seasonId) return;

  const container = document.getElementById('lineup-cards');
  container.innerHTML = '<div class="col text-muted py-3"><i class="bi bi-hourglass-split"></i> Loading…</div>';

  const plans = await api('GET', `/lineup-plans?season_id=${seasonId}&week_number=${weekNum}`);
  const plansByTeam = {};
  plans.forEach(p => { (plansByTeam[p.team_id] = plansByTeam[p.team_id] || []).push(p); });

  if (weekNum === 0) {
    // Default lineup view — one card per team, no match context
    if (!allTeams.length) {
      container.innerHTML = '<div class="col text-muted py-3">No teams found.</div>';
      return;
    }
    container.innerHTML = allTeams.map(t =>
      renderTeamLineupCard(t, plansByTeam[t.id] || [], parseInt(seasonId), 0)
    ).join('');
    return;
  }

  const matches = await api('GET', `/matches?season_id=${seasonId}`);
  const weekMatches = matches.filter(m => m.week_number == weekNum);
  if (!weekMatches.length) {
    container.innerHTML = '<div class="col"><div class="text-muted py-3">No matches this week.</div></div>';
    return;
  }
  container.innerHTML = weekMatches.map(m =>
    renderLineupCard(m, plansByTeam, parseInt(seasonId), weekNum)
  ).join('');
}

// Renders a standalone team card (used for Default Lineup, week=0)
function renderTeamLineupCard(team, plans, seasonId, weekNum) {
  const roster = allPlayers.filter(p => p.team_id == team.id);
  const badge = plans.length >= 3
    ? '<span class="badge bg-success ms-1" style="font-size:.68rem">✓ Set</span>'
    : plans.length > 0
      ? `<span class="badge bg-warning text-dark ms-1" style="font-size:.68rem">${plans.length}/3</span>`
      : '<span class="badge bg-secondary ms-1" style="font-size:.68rem">Not set</span>';
  const slots = [0,1,2].map(i => {
    const saved = plans[i];
    const opts = '<option value="0">— not set —</option>' +
      roster.map(p =>
        `<option value="${p.id}" ${saved && saved.player_id == p.id ? 'selected' : ''}>${p.name} (${p.handicap >= 0 ? '+' : ''}${p.handicap})</option>`
      ).join('');
    return `<div class="d-flex align-items-center gap-2 mb-1">
      <span class="text-muted fw-bold" style="width:22px;font-size:.8rem">P${i+1}</span>
      <select class="form-select form-select-sm" id="lu_${team.id}_${i}">${opts}</select>
    </div>`;
  }).join('');
  return `<div class="col-12 col-md-6 col-xl-4">
    <div class="card h-100">
      <div class="card-header py-2 fw-semibold small d-flex justify-content-between align-items-center">
        <span>${team.name}${badge}</span>
      </div>
      <div class="card-body">
        ${slots}
        <button class="btn btn-primary btn-sm w-100 mt-2"
          onclick="saveTeamLineup(${seasonId},${weekNum},${team.id})">
          <i class="bi bi-check-lg"></i> Save Default Lineup
        </button>
      </div>
    </div>
  </div>`;
}

function renderLineupCard(match, plansByTeam, seasonId, weekNum) {
  function teamCol(teamId, teamName, slotPrefix) {
    const plans  = plansByTeam[teamId] || [];
    const roster = allPlayers.filter(p => p.team_id == teamId);
    const badge  = plans.length >= 3
      ? '<span class="badge bg-success ms-1" style="font-size:.68rem">✓ Set</span>'
      : plans.length > 0
        ? `<span class="badge bg-warning text-dark ms-1" style="font-size:.68rem">${plans.length}/3</span>`
        : '<span class="badge bg-secondary ms-1" style="font-size:.68rem">Not set</span>';
    const slots = [0,1,2].map(i => {
      const saved = plans[i];
      const opts = '<option value="0">— not set —</option>' +
        roster.map(p =>
          `<option value="${p.id}" ${saved && saved.player_id == p.id ? 'selected' : ''}>` +
          `${p.name} (${p.handicap >= 0 ? '+' : ''}${p.handicap})</option>`
        ).join('');
      return `<div class="d-flex align-items-center gap-2 mb-1">
        <span class="text-muted fw-bold" style="width:22px;font-size:.8rem">${slotPrefix}${i+1}</span>
        <select class="form-select form-select-sm" id="lu_${teamId}_${i}">${opts}</select>
      </div>`;
    }).join('');
    return `<div class="col-6">
      <div class="small fw-semibold mb-2">${slotPrefix==='H'?'🏠':'🚗'} ${teamName}${badge}</div>
      ${slots}
      <button class="btn btn-primary btn-sm w-100 mt-2"
        onclick="saveTeamLineup(${seasonId},${weekNum},${teamId})">
        <i class="bi bi-check-lg"></i> Save Lineup
      </button>
    </div>`;
  }
  return `<div class="col-12 col-xl-6">
    <div class="card">
      <div class="card-header py-2 fw-semibold small d-flex justify-content-between">
        <span>${match.home_team_name} <span class="text-muted fw-normal">vs</span> ${match.away_team_name}</span>
        <span class="text-muted">Week ${match.week_number}${match.match_date?' · '+displayDate(match.match_date):''}</span>
      </div>
      <div class="card-body">
        <div class="row g-3">
          ${teamCol(match.home_team_id, match.home_team_name, 'H')}
          ${teamCol(match.away_team_id, match.away_team_name, 'A')}
        </div>
      </div>
    </div>
  </div>`;
}

async function saveTeamLineup(seasonId, weekNum, teamId) {
  const playerIds = [0,1,2]
    .map(i => parseInt(document.getElementById(`lu_${teamId}_${i}`)?.value) || 0)
    .filter(id => id > 0);
  if (playerIds.length === 0) { toast('Select at least one player','warning'); return; }
  if (new Set(playerIds).size !== playerIds.length) { toast('Duplicate player selected','warning'); return; }
  try {
    await api('POST', '/lineup-plans', { season_id: seasonId, team_id: teamId, week_number: weekNum, player_ids: playerIds });
    toast('Lineup saved');
    loadLineupForWeek();
  } catch(e) { toast(e.message,'danger'); }
}

// ── Match Entry ───────────────────────────────────────────────────────────────
let currentMatch       = null;
let scoresheetHomeTeam = [];  // 3 Player objects  (ordered: slot 1, 2, 3)
let scoresheetAwayTeam = [];  // 3 Player objects
// scoresheetGames[r*3+p]: g1w/g2w/g3w = 'home'|'away'|'', g1lb/g2lb/g3lb = 0-7
let scoresheetGames = Array.from({length:9}, () => ({
  g1w:'', g1lb:0, g2w:'', g2lb:0, g3w:'', g3lb:0
}));
let scoresheetSeasonRules = {};

async function loadEntryMatches() {
  if (_entryPreSelectSeasonId !== null) {
    document.getElementById('entry-season-select').value = String(_entryPreSelectSeasonId);
    _entryPreSelectSeasonId = null;
  }
  const seasonId = document.getElementById('entry-season-select').value;
  if (!seasonId) return;
  const matches = await api('GET', `/matches?season_id=${seasonId}`);
  const sel = document.getElementById('entry-match-select');
  sel.innerHTML = matches.map(m =>
    `<option value="${m.id}">[W${m.week_number}] ${m.home_team_name} vs ${m.away_team_name} ${m.completed?'✓':''}</option>`
  ).join('') || '<option>No matches</option>';
  if (_entryPreSelectMatchId !== null) {
    sel.value = String(_entryPreSelectMatchId);
    _entryPreSelectMatchId = null;
  }
  loadMatchEntry();
}

async function loadMatchEntry() {
  const matchId = document.getElementById('entry-match-select').value;
  if (!matchId) return;

  const lineupDiv     = document.getElementById('entry-lineup');
  const scoresheetDiv = document.getElementById('entry-scoresheet');
  lineupDiv.classList.add('d-none');
  scoresheetDiv.classList.add('d-none');
  scoresheetDiv.innerHTML = '';

  let detail;
  try { detail = await api('GET', `/matches/${matchId}`); }
  catch(e) { toast(e.message, 'danger'); return; }

  currentMatch = detail.match;
  const m = currentMatch;

  scoresheetSeasonRules = {};
  try {
    const rulesList = await api('GET', `/seasons/${m.season_id}/rules`);
    for (const r of rulesList) { scoresheetSeasonRules[r.rule_key] = r.rule_value; }
  } catch(_) {}

  if (!activeLeague || activeLeague.game_format !== '8ball') {
    scoresheetDiv.classList.remove('d-none');
    scoresheetDiv.innerHTML = '<div class="text-muted p-3">9-ball scoresheet coming soon.</div>';
    return;
  }

  const homePlayers = allPlayers.filter(p => p.team_id == m.home_team_id);
  const awayPlayers = allPlayers.filter(p => p.team_id == m.away_team_id);

  // 1. Existing round results take highest priority (already played)
  let existingRounds = [];
  try { existingRounds = await api('GET', `/matches/${matchId}/rounds`); } catch(_) {}
  if (existingRounds.length > 0) {
    const r1 = existingRounds.filter(r => r.round_number === 1).sort((a,b) => a.id - b.id);
    if (r1.length === 3) {
      const hp = r1.map(r => homePlayers.find(p => p.id === r.home_player_id)).filter(Boolean);
      const ap = r1.map(r => awayPlayers.find(p => p.id === r.away_player_id)).filter(Boolean);
      if (hp.length === 3 && ap.length === 3) {
        scoresheetHomeTeam = hp;
        scoresheetAwayTeam = ap;
        renderScoresheet(existingRounds);
        return;
      }
    }
  }

  // 2. Lineup plans (pre-game) — week-specific, then fall back to default (week 0)
  let weekPlans = [], defaultPlans = [];
  try { weekPlans    = await api('GET', `/lineup-plans?season_id=${m.season_id}&week_number=${m.week_number}`); } catch(_) {}
  try { defaultPlans = await api('GET', `/lineup-plans?season_id=${m.season_id}&week_number=0`); } catch(_) {}

  // Per team: prefer week-specific; fall back to default if fewer than 3
  function resolvePlans(teamId) {
    const week = weekPlans.filter(p => p.team_id == teamId);
    return week.length >= 3 ? week : defaultPlans.filter(p => p.team_id == teamId);
  }
  const homePlans = resolvePlans(m.home_team_id);
  const awayPlans = resolvePlans(m.away_team_id);

  if (homePlans.length >= 3 && awayPlans.length >= 3) {
    const hp = homePlans.slice(0,3).map(lp => homePlayers.find(p => p.id === lp.player_id)).filter(Boolean);
    const ap = awayPlans.slice(0,3).map(lp => awayPlayers.find(p => p.id === lp.player_id)).filter(Boolean);
    if (hp.length === 3 && ap.length === 3) {
      scoresheetHomeTeam = hp;
      scoresheetAwayTeam = ap;
      scoresheetGames = Array.from({length:9}, () => ({g1w:'',g1lb:0,g2w:'',g2lb:0,g3w:'',g3lb:0}));
      renderScoresheet([]);
      return;
    }
  }

  // 3. Fallback: inline quick picker — pre-populate from whatever plans we have
  showLineupPicker(m, homePlayers, awayPlayers, homePlans, awayPlans);
}

function showLineupPicker(m, homePlayers, awayPlayers, homePlans = [], awayPlans = []) {
  const lineupDiv = document.getElementById('entry-lineup');
  lineupDiv.classList.remove('d-none');

  // Build option list, marking the pre-selected plan player
  const makeOpts = (players, plans, idx) => {
    const preselect = plans[idx]?.player_id;
    return players.map(p =>
      `<option value="${p.id}" ${p.id == preselect ? 'selected' : ''}>${p.name} (${p.handicap >= 0 ? '+' : ''}${p.handicap})</option>`
    ).join('');
  };

  const hasHomePlans = homePlans.length > 0;
  const hasAwayPlans = awayPlans.length > 0;
  const hint = (hasHomePlans || hasAwayPlans)
    ? '<span class="badge bg-info text-dark ms-2" style="font-size:.7rem">Pre-filled from saved lineup</span>'
    : `<span class="text-muted fw-normal small">Set in advance on the <a href="#" onclick="navTo(\'lineup\');return false">Lineup</a> page</span>`;

  lineupDiv.innerHTML = `<div class="card mb-3">
    <div class="card-header fw-semibold py-2 d-flex justify-content-between align-items-center">
      <span>Confirm Tonight's Lineup</span>${hint}
    </div>
    <div class="card-body">
      <div class="row g-3">
        <div class="col-md-6">
          <h6 class="fw-bold mb-2" style="color:#1a5cb8">🏠 ${m.home_team_name}</h6>
          ${[1,2,3].map(i => `<div class="d-flex align-items-center gap-2 mb-2">
            <span class="text-muted fw-bold" style="width:22px">H${i}</span>
            <select class="form-select form-select-sm" id="ph_${i}">
              <option value="">— select —</option>${makeOpts(homePlayers, homePlans, i-1)}
            </select></div>`).join('')}
        </div>
        <div class="col-md-6">
          <h6 class="fw-bold mb-2" style="color:#9b1c1c">🚗 ${m.away_team_name}</h6>
          ${[1,2,3].map(i => `<div class="d-flex align-items-center gap-2 mb-2">
            <span class="text-muted fw-bold" style="width:22px">A${i}</span>
            <select class="form-select form-select-sm" id="pa_${i}">
              <option value="">— select —</option>${makeOpts(awayPlayers, awayPlans, i-1)}
            </select></div>`).join('')}
        </div>
      </div>
      <button class="btn btn-primary mt-2" onclick="confirmLineup()">
        <i class="bi bi-arrow-right"></i> Go to Scoresheet
      </button>
    </div>
  </div>`;
}

function confirmLineup() {
  const m = currentMatch;
  if (!m) return;
  const homePlayers = allPlayers.filter(p => p.team_id == m.home_team_id);
  const awayPlayers = allPlayers.filter(p => p.team_id == m.away_team_id);

  const hp = [1,2,3].map(i => homePlayers.find(p => p.id == document.getElementById(`ph_${i}`)?.value)).filter(Boolean);
  const ap = [1,2,3].map(i => awayPlayers.find(p => p.id == document.getElementById(`pa_${i}`)?.value)).filter(Boolean);

  if (hp.length !== 3) { toast('Select all 3 home players','warning'); return; }
  if (ap.length !== 3) { toast('Select all 3 away players','warning'); return; }
  if (new Set(hp.map(p=>p.id)).size !== 3) { toast('Duplicate home player','warning'); return; }
  if (new Set(ap.map(p=>p.id)).size !== 3) { toast('Duplicate away player','warning'); return; }

  scoresheetHomeTeam = hp;
  scoresheetAwayTeam = ap;
  scoresheetGames    = Array.from({length:9}, () => ({g1w:'',g1lb:0,g2w:'',g2lb:0,g3w:'',g3lb:0}));
  document.getElementById('entry-lineup').classList.add('d-none');
  renderScoresheet([]);
}

// ── Scoring helpers ───────────────────────────────────────────────────────────
function gameScores(winner, lb) {
  if (!winner) return { h: 0, a: 0 };
  return winner === 'home' ? { h: 10, a: lb } : { h: lb, a: 10 };
}

function calcHandicap(hHC, aHC) {
  const multiplier = parseFloat(scoresheetSeasonRules['handicap_multiplier']) || 2.55;
  const minBall    = parseInt(scoresheetSeasonRules['min_ball_handicap'])     || 0;
  const to         = hHC < aHC ? 'home' : aHC < hHC ? 'away' : '';
  const diff       = Math.abs(hHC - aHC);
  const rawPts     = Math.round(diff * multiplier);
  const pts        = !to ? 0 : (minBall > 0 && rawPts < minBall) ? 0 : rawPts;
  return { pts, to };
}

function pairingTotals(idx) {
  const g  = scoresheetGames[idx];
  const ri = Math.floor(idx / 3);
  const pi = idx % 3;
  const hp = scoresheetHomeTeam[pi];
  const ap = scoresheetAwayTeam[(pi + ri) % 3];
  const s1 = gameScores(g.g1w, g.g1lb);
  const s2 = gameScores(g.g2w, g.g2lb);
  const s3 = gameScores(g.g3w, g.g3lb);
  const rawH = s1.h + s2.h + s3.h;
  const rawA = s1.a + s2.a + s3.a;
  const hc   = calcHandicap(hp.handicap, ap.handicap);
  const adjH = rawH + (hc.to === 'home' ? hc.pts : 0);
  const adjA = rawA + (hc.to === 'away' ? hc.pts : 0);
  const hGames   = [g.g1w, g.g2w, g.g3w].filter(w => w === 'home').length;
  const aGames   = [g.g1w, g.g2w, g.g3w].filter(w => w === 'away').length;
  const played   = hGames + aGames;
  const hasScore = played > 0;
  const remaining = 3 - played;
  // A player wins once the opponent cannot catch them even with max points (10/game) in all remaining games.
  // Handicap alone (no games entered) never produces a winner.
  let winner = '';
  if (hasScore) {
    if (adjH > adjA + remaining * 10)      winner = 'home';
    else if (adjA > adjH + remaining * 10) winner = 'away';
    else if (remaining === 0) {
      if (hGames > aGames) winner = 'home';
      else if (aGames > hGames) winner = 'away';
    }
  }
  return { rawH, rawA, adjH, adjA, hc, winner, hp, ap, hasScore };
}

// ── Scoresheet — portrait letter layout matching physical Page 1 ─────────────
function renderScoresheet(existingRounds) {
  const m    = currentMatch;
  const home = scoresheetHomeTeam;
  const away = scoresheetAwayTeam;

  // Hydrate scoresheetGames from saved round data
  scoresheetGames = Array.from({length:9}, () => ({g1w:'',g1lb:0,g2w:'',g2lb:0,g3w:'',g3lb:0}));
  existingRounds.forEach(rr => {
    const ri = rr.round_number - 1;
    const pi = home.findIndex(p => p.id === rr.home_player_id);
    if (pi < 0) return;
    const idx = ri * 3 + pi;
    const from = (hs, as) => hs===10 ? {w:'home',lb:as} : as===10 ? {w:'away',lb:hs} : {w:'',lb:0};
    const g1 = from(rr.game1_home, rr.game1_away);
    const g2 = from(rr.game2_home, rr.game2_away);
    const g3 = from(rr.game3_home, rr.game3_away);
    scoresheetGames[idx] = {g1w:g1.w,g1lb:g1.lb,g2w:g2.w,g2lb:g2.lb,g3w:g3.w,g3lb:g3.lb};
  });

  const fmtHC = v => (v >= 0 ? '+' : '') + v;
  const leagueName = activeLeague?.name || 'League';

  // ── No-print toolbar ──
  let html = `<div class="no-print d-flex justify-content-between align-items-center mb-2 gap-2 flex-wrap">
    <div class="d-flex align-items-center gap-2">
      <span class="fw-semibold small">Week ${m.week_number}${m.match_date?' · '+displayDate(m.match_date):''}</span>
      ${m.completed
        ? '<span class="badge bg-success">Completed</span>'
        : '<span class="badge bg-secondary">Pending</span>'}
    </div>
    <div class="d-flex gap-2">
      <button class="btn btn-sm btn-outline-secondary" onclick="loadMatchEntry()">
        <i class="bi bi-arrow-left"></i> Change Lineup</button>
      <button class="btn btn-sm btn-outline-secondary" onclick="printScoresheet()">
        <i class="bi bi-printer"></i> Print</button>
      <button class="btn btn-sm btn-success" onclick="saveScoresheet()">
        <i class="bi bi-check-lg"></i> Save</button>
      ${m.completed ? `<button class="btn btn-sm btn-outline-danger" onclick="clearMatchResults(${m.id})">
        <i class="bi bi-x-lg"></i> Clear</button>` : ''}
    </div>
  </div>`;

  // ── Printable scoresheet ──
  html += `<div id="ss-print-area"><div class="ss-sheet">`;

  // Title bar
  html += `<div class="ss-title-bar">
    <span class="ss-main-title">${leagueName} Scoresheet</span>
    <div class="ss-title-meta">
      <span>${displayDate(m.match_date, 'Date TBD')}</span>
      ${m.match_number != null ? `<span class="ss-match-num">${m.match_number}</span>` : ''}
    </div>
  </div>`;

  // Team header row
  html += `<table class="ss-teams-tbl">
    <tr>
      <td class="ss-tn-name" style="color:#1e3a8a">${escapeHTML(m.home_team_name || '')}</td>
      <td class="ss-tn-label">Home Team</td>
      <td class="ss-tn-sep"></td>
      <td class="ss-tn-name" style="color:#7f1d1d">${escapeHTML(m.away_team_name || '')}</td>
      <td class="ss-tn-label">Visiting Team</td>
    </tr>
    <tr>
      <td colspan="5" class="ss-tn-table">Table: ${escapeHTML(m.table_numbers || '')}</td>
    </tr>
  </table>`;

  // Player roster
  html += `<table class="ss-roster-tbl">
    <thead><tr>
      <th colspan="4" style="text-align:center;color:#1e3a8a">Home Team</th>
      <th class="ss-roster-sep"></th>
      <th colspan="4" style="text-align:center;color:#7f1d1d">Visiting Team</th>
    </tr></thead>
    <tbody>`;
  for (let i = 0; i < 3; i++) {
    const hp = home[i], ap = away[i];
    html += `<tr>
      <td class="ss-slot-h">H${i+1}</td>
      <td class="ss-pnum-col">${hp.player_number||''}</td>
      <td>${hp.name}</td>
      <td class="ss-hc-roster">${fmtHC(hp.handicap)}</td>
      <td class="ss-roster-sep"></td>
      <td class="ss-slot-v">V${i+1}</td>
      <td class="ss-pnum-col">${ap.player_number||''}</td>
      <td>${ap.name}</td>
      <td class="ss-hc-roster">${fmtHC(ap.handicap)}</td>
    </tr>`;
  }
  html += `</tbody></table>`;

  html += `<p class="ss-entry-hint no-print">Enter <strong>10</strong> for the game winner. Loser score is balls made (0&ndash;7).</p>`;

  // Main scoring table
  html += `<table class="ss-score-tbl">
    <thead><tr>
      <th style="width:22px"></th>
      <th class="ss-th-left" style="width:115px">Player</th>
      <th style="width:46px">Game 1</th>
      <th style="width:46px">Game 2</th>
      <th style="width:46px">Game 3</th>
      <th style="width:38px">Score</th>
      <th style="width:42px">Rating</th>
      <th style="width:40px">Ball HC</th>
      <th style="width:48px">Adj&nbsp;Score</th>
      <th style="width:102px">Circle Winner</th>
    </tr></thead>
    <tbody>`;

  for (let r = 0; r < 3; r++) {
    html += `<tr class="ss-rnd-row"><td colspan="10" id="ss-rnd-${r}">Round ${r+1}</td></tr>`;

    for (let p = 0; p < 3; p++) {
      const hp  = home[p];
      const ap  = away[(p + r) % 3];
      const idx = r * 3 + p;

      html += `<tr class="ss-home-row" id="ss-hrow-${idx}">
        <td class="ss-slot" style="color:#1e3a8a">H${p+1}</td>
        <td class="ss-pname-cell">${escapeHTML(hp.name)}</td>
        <td class="ss-game-cell">${ssGameInput(idx,1,'home')}</td>
        <td class="ss-game-cell">${ssGameInput(idx,2,'home')}</td>
        <td class="ss-game-cell">${ssGameInput(idx,3,'home')}</td>
        <td class="ss-score-cell" id="ss-score-h-${idx}">--</td>
        <td class="ss-rating-cell">${fmtHC(hp.handicap)}</td>
        <td class="ss-hc-cell" id="ss-hc-${idx}" rowspan="2">--</td>
        <td class="ss-adj-cell" id="ss-adj-h-${idx}">--</td>
        <td class="ss-winner-cell" id="ss-winner-${idx}" rowspan="2">
          <div class="cw-opt" id="ss-cw-h-${idx}">&#9675; H</div>
          <div class="cw-opt" id="ss-cw-a-${idx}">&#9675; V</div>
        </td>
      </tr>
      <tr class="ss-away-row" id="ss-arow-${idx}">
        <td class="ss-slot" style="color:#7f1d1d">V${(p+r)%3+1}</td>
        <td class="ss-pname-cell">${escapeHTML(ap.name)}</td>
        <td class="ss-game-cell">${ssGameInput(idx,1,'away')}</td>
        <td class="ss-game-cell">${ssGameInput(idx,2,'away')}</td>
        <td class="ss-game-cell">${ssGameInput(idx,3,'away')}</td>
        <td class="ss-score-cell" id="ss-score-a-${idx}">--</td>
        <td class="ss-rating-cell">${fmtHC(ap.handicap)}</td>
        <td class="ss-adj-cell" id="ss-adj-a-${idx}">--</td>
      </tr>`;
    }
  }

  html += `</tbody></table>`;

  // Match summary
  html += `<div class="no-print mt-2" id="ss-summary">${buildScoreSummary()}</div>`;

  // Signature lines
  html += `<div class="ss-sigs">
    <div class="ss-sig-line">Home Team Captain Signature: <span class="ss-sig-blank"></span></div>
    <div class="ss-sig-line">Visiting Team Captain Signature: <span class="ss-sig-blank"></span></div>
  </div>`;

  html += `</div>`; // close .ss-sheet
  html += buildScorekeeperPage();
  html += `</div>`; // close #ss-print-area

  const sd = document.getElementById('entry-scoresheet');
  sd.innerHTML = html;
  sd.classList.remove('d-none');

  for (let i = 0; i < 9; i++) updateSSPairing(i);
  updateSSFinal();
}

// -- Scoresheet print: injects portrait @page override so poster landscape rule is bypassed --
function printScoresheet() {
  // Regenerate Page 2 from current scoresheetGames so any scores entered after
  // the initial render are included before printing.
  const area = document.getElementById('ss-print-area');
  if (area) {
    const oldP2 = area.querySelector('.ss-p2-sheet');
    if (oldP2) {
      const tmp = document.createElement('div');
      tmp.innerHTML = buildScorekeeperPage();
      oldP2.replaceWith(tmp.firstElementChild);
    }
  }
  const s = document.createElement('style');
  s.id = 'ss-page-override';
  s.textContent = '@page { size: letter portrait; margin: 0.45in; }';
  document.head.appendChild(s);
  window.addEventListener('afterprint', () => { document.getElementById('ss-page-override')?.remove(); }, { once: true });
  window.print();
}

// -- Scoresheet Page 2: Match Scorekeeping summary --
function buildScorekeeperPage() {
  const m    = currentMatch;
  const home = scoresheetHomeTeam;
  const away = scoresheetAwayTeam;

  // Per-player stat accumulators indexed by home[0..2] / away[0..2]
  const hStats = Array.from({length: 3}, () => ({gW: 0, gL: 0, sW: 0, sL: 0, diff: 0}));
  const aStats = Array.from({length: 3}, () => ({gW: 0, gL: 0, sW: 0, sL: 0, diff: 0}));
  let homeRoundsWon = 0, awayRoundsWon = 0;
  const hasAnyScore = scoresheetGames.some(g => g.g1w || g.g2w || g.g3w);

  for (let r = 0; r < 3; r++) {
    let rh = 0, ra = 0;
    for (let p = 0; p < 3; p++) {
      const idx   = r * 3 + p;
      const g     = scoresheetGames[idx];
      const {rawH, rawA, winner} = pairingTotals(idx);
      const apPos = (p + r) % 3;
      const hs    = hStats[p];
      const as_   = aStats[apPos];

      for (let gn = 1; gn <= 3; gn++) {
        const w = g[`g${gn}w`];
        if (w === 'home') { hs.gW++; as_.gL++; }
        else if (w === 'away') { hs.gL++; as_.gW++; }
      }
      if (winner === 'home')      { hs.sW++; as_.sL++; rh++; }
      else if (winner === 'away') { hs.sL++; as_.sW++; ra++; }

      hs.diff  += rawH - rawA;
      as_.diff += rawA - rawH;
    }
    if (rh > ra) homeRoundsWon++;
    else if (ra > rh) awayRoundsWon++;
  }

  const fmtD = n => n > 0 ? `+${n}` : `${n}`;
  const pRow = (label, player, st) => `
    <tr>
      <td class="ss-p2-pos">${label}</td>
      <td class="ss-p2-pnum">${escapeHTML(player.player_number || '')}</td>
      <td>${escapeHTML(player.name || '')}</td>
      <td class="ss-p2-num">${st.gW}</td>
      <td class="ss-p2-num">${st.gL}</td>
      <td class="ss-p2-num">${st.sW}</td>
      <td class="ss-p2-num">${st.sL}</td>
      <td class="ss-p2-diff-col">${fmtD(st.diff)}</td>
    </tr>`;

  const dateStr   = displayDate(m.match_date, 'Date TBD');
  const matchMeta = [
    m.match_number != null ? 'Match ' + escapeHTML(String(m.match_number)) : null,
    m.table_numbers        ? 'Table ' + escapeHTML(String(m.table_numbers)) : null,
    'Week ' + escapeHTML(String(m.week_number)),
  ].filter(Boolean).join(' &middot; ');

  return `<div class="ss-p2-sheet">
    <div class="no-print ss-p2-screen-label">
      <i class="bi bi-printer me-1" aria-hidden="true"></i>Printable scorekeeping summary &mdash; page&nbsp;2 of&nbsp;2
    </div>
    <div class="ss-p2-header">
      <span class="ss-p2-title">Match Scorekeeping</span>
      <div class="ss-p2-meta">
        <span>${escapeHTML(dateStr)}</span>
        <span>${matchMeta}</span>
      </div>
    </div>
    <div class="ss-p2-instructions">
      <strong>Rounds Won:</strong> Count the rounds (sets) won by each team &mdash; 3 rounds per match.
      The team with more rounds wins the match.&ensp;
      <strong>Diff:</strong> Each player&rsquo;s raw ball-score total minus their opponent&rsquo;s
      across all their sets, before handicap is applied. Positive means they outscored their opponent on raw balls.&ensp;
      <strong>Handicap:</strong> Spot added to the lower-rated player&rsquo;s raw total.
      Formula: <em>((x&minus;y)&times;3)&times;.85</em> &nbsp;or&nbsp; <em>(x&minus;y)&times;2.55</em>,
      where x and y are the players&rsquo; handicap ratings.
    </div>
    <div class="ss-p2-rounds">
      <div class="ss-p2-round-line">Rounds Won &nbsp;<strong>${escapeHTML(m.home_team_name || '')}</strong>: ${hasAnyScore ? homeRoundsWon : '______'}</div>
      <div class="ss-p2-round-line">Rounds Won &nbsp;<strong>${escapeHTML(m.away_team_name || '')}</strong>: ${hasAnyScore ? awayRoundsWon : '______'}</div>
    </div>
    <table class="ss-p2-table">
      <thead><tr>
        <th class="ss-p2-pos">Pos</th>
        <th class="ss-p2-pnum">No.</th>
        <th style="text-align:left">Player</th>
        <th class="ss-p2-num">Games<br>Won</th>
        <th class="ss-p2-num">Games<br>Lost</th>
        <th class="ss-p2-num">Sets<br>Won</th>
        <th class="ss-p2-num">Sets<br>Lost</th>
        <th class="ss-p2-diff-col">Diff</th>
      </tr></thead>
      <tbody>
        <tr class="ss-p2-team-row"><td colspan="8">Home &mdash; ${escapeHTML(m.home_team_name || '')}</td></tr>
        ${home.map((p, i) => pRow(`${i + 1}H`, p, hStats[i])).join('')}
        <tr class="ss-p2-team-row ss-p2-away-row"><td colspan="8">Visiting &mdash; ${escapeHTML(m.away_team_name || '')}</td></tr>
        ${away.map((p, i) => pRow(`${i + 1}V`, p, aStats[i])).join('')}
      </tbody>
    </table>
    <div class="ss-sigs">
      <div class="ss-sig-line">Home Team Capt. Signature: <span class="ss-sig-blank"></span></div>
      <div class="ss-sig-line">Visiting Team Capt. Signature: <span class="ss-sig-blank"></span></div>
    </div>
    <div class="ss-p2-hc-note">Handicap formula: ((x&minus;y)&times;3)&times;.85 &nbsp;&nbsp;or&nbsp;&nbsp; (x&minus;y)&times;2.55</div>
  </div>`;
}

// -- Scoresheet cell + interaction helpers --

// Builds a numeric score input for one player/game slot.
// Reads current scoresheetGames so the initial render is pre-populated.
function ssGameInput(idx, gn, side) {
  const g  = scoresheetGames[idx];
  const w  = g['g' + gn + 'w'];
  const lb = g['g' + gn + 'lb'];
  const val = w === side ? '10' : (w && w !== side) ? String(lb) : '';
  return '<input type="number" class="ss-score-inp" id="ss-inp-' + idx + '-' + gn + '-' + side + '"' +
    ' min="0" max="10"' + (val !== '' ? ' value="' + val + '"' : '') +
    ' oninput="ssInpChange(' + idx + ',' + gn + ',\'' + side + '\')"' +
    ' onkeydown="ssInpKey(event,' + idx + ',' + gn + ',\'' + side + '\')'+'">';
}

// Clamps a score input to 0-10 in place; returns NaN for blank.
function normalizeScoreInput(el) {
  if (!el || el.value === '') return NaN;
  const raw = parseInt(el.value, 10);
  if (isNaN(raw)) { el.value = ''; return NaN; }
  const v = Math.min(10, Math.max(0, raw));
  el.value = String(v);
  return v;
}

// Handle score input change: infer winner from the pair of values for this game
function ssInpChange(idx, gn, side) {
  const hEl = document.getElementById('ss-inp-' + idx + '-' + gn + '-home');
  const aEl = document.getElementById('ss-inp-' + idx + '-' + gn + '-away');
  const hv  = normalizeScoreInput(hEl);
  const av  = normalizeScoreInput(aEl);
  const g   = scoresheetGames[idx];
  const wk  = 'g' + gn + 'w';
  const lk  = 'g' + gn + 'lb';
  if (hv === 10 && av !== 10) {
    g[wk] = 'home'; g[lk] = isNaN(av) ? 0 : Math.min(7, Math.max(0, av));
    if (aEl) { aEl.value = String(g[lk]); if (side === 'home') { aEl.focus(); aEl.select(); } }
  } else if (av === 10 && hv !== 10) {
    g[wk] = 'away'; g[lk] = isNaN(hv) ? 0 : Math.min(7, Math.max(0, hv));
    if (hEl) { hEl.value = String(g[lk]); if (side === 'away') { hEl.focus(); hEl.select(); } }
  } else if (hv === 10 && av === 10) {
    g[wk] = side; g[lk] = 0;
    if (side === 'home' && aEl) { aEl.value = '0'; aEl.focus(); aEl.select(); }
    if (side === 'away' && hEl) { hEl.value = '0'; hEl.focus(); hEl.select(); }
  } else {
    g[wk] = ''; g[lk] = 0;
  }
  updateSSPairing(idx);
  updateSSFinal();
}

// Tab/Shift-Tab within score inputs: interleaved H->V per game
// Tab order: H1G1->V1G1->H1G2->V1G2->H1G3->V1G3->H2G1->...
function ssInpKey(e, idx, gn, side) {
  if (e.key !== 'Tab') return;
  e.preventDefault();
  if (!e.shiftKey) {
    const nextEl = side === 'home'
      ? document.getElementById('ss-inp-' + idx + '-' + gn + '-away')
      : gn < 3
        ? document.getElementById('ss-inp-' + idx + '-' + (gn + 1) + '-home')
        : idx < 8
          ? document.getElementById('ss-inp-' + (idx + 1) + '-1-home')
          : null;
    if (nextEl) nextEl.focus();
  } else {
    const prevEl = side === 'away'
      ? document.getElementById('ss-inp-' + idx + '-' + gn + '-home')
      : gn > 1
        ? document.getElementById('ss-inp-' + idx + '-' + (gn - 1) + '-away')
        : idx > 0
          ? document.getElementById('ss-inp-' + (idx - 1) + '-3-away')
          : null;
    if (prevEl) prevEl.focus();
  }
}
// Update CSS classes on score inputs to reflect winner/loser/empty state.
function updateSSInputClasses(idx) {
  const g = scoresheetGames[idx];
  for (let gn = 1; gn <= 3; gn++) {
    const w = g['g' + gn + 'w'];
    for (const side of ['home', 'away']) {
      const el = document.getElementById('ss-inp-' + idx + '-' + gn + '-' + side);
      if (!el) continue;
      let cls = 'ss-score-inp';
      if (w === side)           cls += ' ss-inp-winner';
      else if (w && w !== side) cls += ' ss-inp-loser';
      else if (el.value === '') cls += ' ss-inp-empty';
      el.className = cls;
    }
  }
}

// Rebuild all cells for one pairing (both rows)
function updateSSPairing(idx) {
  const { rawH, rawA, adjH, adjA, hc, winner, hasScore } = pairingTotals(idx);

  const hcEl = document.getElementById('ss-hc-' + idx);
  if (hcEl) hcEl.textContent = String(hc.pts);

  const sh = document.getElementById('ss-score-h-' + idx);
  const sa = document.getElementById('ss-score-a-' + idx);
  const ah = document.getElementById('ss-adj-h-' + idx);
  const aa = document.getElementById('ss-adj-a-' + idx);
  if (sh) sh.textContent = hasScore ? rawH : '--';
  if (sa) sa.textContent = hasScore ? rawA : '--';
  if (ah) { ah.textContent = hasScore ? adjH : '--';
    ah.className = 'ss-adj-cell' + (winner === 'home' ? ' ss-adj-win' : ''); }
  if (aa) { aa.textContent = hasScore ? adjA : '--';
    aa.className = 'ss-adj-cell' + (winner === 'away' ? ' ss-adj-win' : ''); }

  const cwH = document.getElementById('ss-cw-h-' + idx);
  const cwA = document.getElementById('ss-cw-a-' + idx);
  if (cwH) cwH.className = 'cw-opt' + (winner === 'home' ? ' cw-sel-h' : '');
  if (cwA) cwA.className = 'cw-opt' + (winner === 'away' ? ' cw-sel-a' : '');
  updateSSInputClasses(idx);
}

function updateSSFinal() {
  const wrap = document.getElementById('ss-summary');
  if (wrap) wrap.innerHTML = buildScoreSummary();
  for (let r = 0; r < 3; r++) {
    let rh = 0, ra = 0;
    for (let p = 0; p < 3; p++) {
      const { winner } = pairingTotals(r * 3 + p);
      if (winner === 'home') rh++;
      else if (winner === 'away') ra++;
    }
    const el = document.getElementById('ss-rnd-' + r);
    if (!el) continue;
    if (rh >= 2) {
      el.innerHTML = 'Round ' + (r + 1) + ' <span class="ss-rnd-badge ss-rnd-badge-h">H wins</span>';
      el.parentElement.className = 'ss-rnd-row ss-rnd-win-h';
    } else if (ra >= 2) {
      el.innerHTML = 'Round ' + (r + 1) + ' <span class="ss-rnd-badge ss-rnd-badge-v">V wins</span>';
      el.parentElement.className = 'ss-rnd-row ss-rnd-win-v';
    } else {
      el.textContent = 'Round ' + (r + 1);
      el.parentElement.className = 'ss-rnd-row';
    }
  }
}

function buildScoreSummary() {
  let hw = 0, aw = 0;
  for (let i = 0; i < 9; i++) {
    const {winner} = pairingTotals(i);
    if (winner==='home') hw++; else if (winner==='away') aw++;
  }
  const m = currentMatch;
  return `<div class="score-summary">
    <div class="text-center">
      <div class="team-name">${m?.home_team_name||'Home'} (Home)</div>
      <div class="team-score">${hw}</div>
    </div>
    <div class="text-center" style="opacity:.5;font-size:1.2rem">–</div>
    <div class="text-center">
      <div class="team-name">${m?.away_team_name||'Away'} (Visitor)</div>
      <div class="team-score">${aw}</div>
    </div>
  </div>`;
}

async function saveScoresheet() {
  const m = currentMatch;
  if (!m) return;

  const rounds = [];
  for (let r = 0; r < 3; r++) {
    for (let p = 0; p < 3; p++) {
      const idx = r * 3 + p;
      const g   = scoresheetGames[idx];
      const hp  = scoresheetHomeTeam[p];
      const ap  = scoresheetAwayTeam[(p + r) % 3];

      const s1 = gameScores(g.g1w, g.g1lb);
      const s2 = gameScores(g.g2w, g.g2lb);
      const s3 = gameScores(g.g3w, g.g3lb);

      rounds.push({
        round_number:   r + 1,
        home_player_id: hp.id,
        away_player_id: ap.id,
        game1_home: s1.h, game1_away: s1.a,
        game2_home: s2.h, game2_away: s2.a,
        game3_home: s3.h, game3_away: s3.a
      });
    }
  }

  try {
    await api('POST', `/matches/${m.id}/rounds`, { rounds });
    toast('Scoresheet saved');
    loadEntryMatches();
  } catch(e) { toast(e.message, 'danger'); }
}

async function clearMatchResults(matchId) {
  if (!confirm('Clear all results for this match?')) return;
  try {
    await api('DELETE', `/matches/${matchId}/results`);
    toast('Results cleared');
    loadMatchEntry();
  } catch(e) { toast(e.message, 'danger'); }
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
