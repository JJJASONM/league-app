// <seasons-page> — domain entry point for the Seasons section.
//
// Owns: season list, season management panel (schedule setup / skip weeks /
//   bye requests), and the season create/edit modal via <season-editor>.
//
// Public API:
//   refresh(league, allSeasons, allTeams)
//     Called by the application shell when the Seasons section becomes active
//     or when the selected league changes.
//
// Fires (bubbles: true):
//   season-state-changed — { allSeasons, activeSeason }
//     After any mutation that changes the season list or active season.
//     The shell updates its allSeasons / activeSeason globals and the sidebar
//     label in response.
//
//   season-nav-request — { section, previewSeasonId, openPoster }
//     When the user clicks "View Schedule" or "Poster View" from the
//     management panel.  The shell calls navTo() and optionally loads the
//     schedule section with a specific season pre-selected.

import {
  listSeasons, deleteSeason, activateSeason,
  generateSchedule,
  listSkippedWeeks, addSkippedWeek, removeSkippedWeek,
  listByeRequests, addByeRequest, updateByeRequest, removeByeRequest,
  listSeasonTeams, getSeasonChecklist,
} from './season-api-service.js';
import './season-editor-component.js';

// ── Constants ────────────────────────────────────────────────────────────────

const SCHEDULE_TYPE_LABEL = {
  single_rr: 'Single RR', double_rr: 'Double RR',
  split: 'Split', custom: 'Custom', blanket: 'Blanket',
};
const SCHEDULE_TYPE_FULL = {
  single_rr: 'Single Round Robin',
  double_rr: 'Double Round Robin',
  split:     'Split Season',
  custom:    'Custom (Fixed Weeks)',
  blanket:   'Blanket (Empty Slots)',
};

function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

// Use the shell's displayDate global for consistent date formatting.
function fmtDateRange(start, end) {
  return `${displayDate(start, '—')} – ${displayDate(end, 'TBD')}`;
}

// ── Component ─────────────────────────────────────────────────────────────────

class SeasonsPage extends HTMLElement {
  // Shell-provided context
  #league      = null;
  #allSeasons  = [];
  #allTeams    = [];

  // Management panel state
  #mgmtSeasonId = null;
  #teamCount    = 0;
  #isManaged    = false;

  connectedCallback() {
    this.innerHTML = this.#shellHTML();
    this.#wireStaticEvents();
    this.#onGenTypeChange(); // initialise description text
    this.addEventListener('season-editor-saved', e => this.#onEditorSaved(e));
    this.addEventListener('click',  e => this.#onClick(e));
    this.addEventListener('change', e => this.#onChange(e));
  }

  // Called by the application shell whenever this section becomes active or
  // the active league changes.
  refresh(league, allSeasons, allTeams) {
    this.#league     = league ?? null;
    this.#allSeasons = allSeasons ?? [];
    this.#allTeams   = allTeams ?? [];
    this.#renderSeasonList();
    // If the management panel was open, keep it in sync.
    if (this.#mgmtSeasonId) {
      const still = this.#allSeasons.find(s => s.id === this.#mgmtSeasonId);
      if (still) this.#manageSeason(this.#mgmtSeasonId);
      else this.#closeMgmt();
    }
  }

  // ── Shell HTML ───────────────────────────────────────────────────────────────

  #shellHTML() {
    return `
    <div class="d-flex justify-content-between align-items-center mb-3">
      <h4 class="mb-0 fw-bold">Seasons</h4>
      <button class="btn btn-primary btn-sm" data-action="new-season">
        <i class="bi bi-plus-lg"></i> New Season
      </button>
    </div>

    <!-- Season list -->
    <div class="card mb-3">
      <div class="card-body p-0">
        <table class="table table-hover mb-0 sdx-seasons-table">
          <thead><tr>
            <th style="width:30%">Season</th>
            <th>Dates</th>
            <th>Format</th>
            <th>Schedule</th>
            <th></th>
          </tr></thead>
          <tbody></tbody>
        </table>
      </div>
    </div>

    <!-- Management panel -->
    <div class="sdx-season-mgmt d-none">

      <!-- Panel header -->
      <div class="card mb-0" style="border-radius:12px 12px 0 0;border-bottom:none">
        <div class="card-body py-2 px-3">
          <div class="d-flex align-items-center gap-2 flex-wrap">
            <div>
              <span class="fw-bold fs-6 sdx-mgmt-title"></span>
              <span class="badge bg-success ms-1 d-none sdx-mgmt-active-badge">Active</span>
              <span class="text-muted small ms-2 sdx-mgmt-meta"></span>
            </div>
            <div class="ms-auto d-flex gap-2 flex-wrap">
              <button class="btn btn-sm btn-outline-secondary sdx-set-active-btn"
                data-action="activate-mgmt" style="display:none" disabled>
                <i class="bi bi-lightning"></i> Set Active
              </button>
              <button class="btn btn-sm btn-outline-primary" data-action="go-schedule">
                <i class="bi bi-calendar-week"></i> View Schedule
              </button>
              <button class="btn btn-sm btn-outline-success" data-action="go-poster">
                <i class="bi bi-image"></i> Poster View
              </button>
              <button class="btn btn-sm btn-outline-secondary" data-action="close-mgmt">
                <i class="bi bi-x"></i> Close
              </button>
            </div>
          </div>
        </div>
      </div>

      <div class="sdx-checklist-section card border-top-0 border-bottom-0 d-none">
        <div class="card-body py-2 px-3 small"></div>
      </div>

      <div class="sdx-registered-teams-section card border-top-0 border-bottom-0 d-none">
        <div class="card-body py-2 px-3 small"></div>
      </div>

      <!-- Tab bar -->
      <ul class="nav nav-tabs mb-0">
        <li class="nav-item"><a class="nav-link active" href="#" data-stab="schedule">Schedule Setup</a></li>
        <li class="nav-item"><a class="nav-link" href="#" data-stab="skips">Skip Weeks</a></li>
        <li class="nav-item"><a class="nav-link" href="#" data-stab="byes">Bye Requests</a></li>
      </ul>

      <!-- Tab: Schedule Setup -->
      <div class="sdx-stab sdx-stab-schedule card border-top-0" style="border-radius:0 0 12px 12px">
        <div class="card-body">
          <div class="row g-3 align-items-end mb-2">
            <div class="col-auto">
              <label class="form-label small mb-1 fw-semibold">Schedule Type</label>
              <select class="form-select form-select-sm sdx-gen-type">
                <option value="single_rr">Single Round Robin</option>
                <option value="double_rr" selected>Double Round Robin</option>
                <option value="split">Split Season</option>
                <option value="custom">Custom (Fixed Weeks)</option>
                <option value="blanket">Blanket (Empty Slots)</option>
              </select>
            </div>
            <div class="col-auto sdx-gen-numweeks-col" style="display:none">
              <label class="form-label small mb-1 fw-semibold">Number of Weeks</label>
              <input type="number" class="form-control form-control-sm sdx-gen-numweeks" value="16" min="1" style="width:90px">
            </div>
            <div class="col-auto sdx-gen-mpw-col" style="display:none">
              <label class="form-label small mb-1 fw-semibold">Matches per Week</label>
              <input type="number" class="form-control form-control-sm sdx-gen-mpw" value="3" min="1" style="width:90px">
            </div>
            <div class="col-auto">
              <label class="form-label small mb-1 fw-semibold">First Match Date</label>
              <input type="date" class="form-control form-control-sm sdx-gen-start-date">
            </div>
            <div class="col-auto sdx-gen-from-season-col">
              <label class="form-label small mb-1 fw-semibold">Use Teams From</label>
              <select class="form-select form-select-sm sdx-gen-from-season" style="min-width:180px">
                <option value="">All teams in league</option>
              </select>
            </div>
            <div class="col-auto d-none sdx-gen-managed-note">
              <div class="alert alert-info py-2 px-3 mb-0 small">
                <i class="bi bi-people"></i> Teams from this season's registered roster.
              </div>
            </div>
            <div class="col-auto">
              <button class="btn btn-success btn-sm" data-action="gen-schedule">
                <i class="bi bi-shuffle"></i> Generate Schedule
              </button>
            </div>
          </div>
          <div class="alert alert-light border py-2 px-3 mb-0 sdx-gen-type-desc" style="font-size:.82rem"></div>
          <div class="alert alert-warning py-2 px-3 mt-2 mb-0 small d-none sdx-schedule-preview-note">
            <i class="bi bi-eye"></i>
            Schedule is a preview - visible to admins only until this season is activated.
          </div>
          <div class="text-muted small mt-2">
            <i class="bi bi-info-circle"></i>
            Generating replaces only <strong>unplayed</strong> matches. Completed matches are preserved.
            Add any skip dates first (Skip Weeks tab) before generating.
          </div>
        </div>
      </div>

      <!-- Tab: Skip Weeks -->
      <div class="sdx-stab sdx-stab-skips card border-top-0 d-none" style="border-radius:0 0 12px 12px">
        <div class="card-body">
          <div class="row g-2 mb-3 align-items-end">
            <div class="col-auto">
              <label class="form-label small mb-1">Date to Skip</label>
              <input type="date" class="form-control form-control-sm sdx-skip-date-input">
            </div>
            <div class="col">
              <label class="form-label small mb-1">Reason</label>
              <input type="text" class="form-control form-control-sm sdx-skip-reason-input"
                placeholder="e.g. Thanksgiving, Christmas break">
            </div>
            <div class="col-auto">
              <button class="btn btn-primary btn-sm" data-action="add-skip">
                <i class="bi bi-plus-lg"></i> Add
              </button>
            </div>
          </div>
          <table class="table table-sm mb-0 sdx-skips-table">
            <thead><tr><th>Date</th><th>Reason</th><th></th></tr></thead>
            <tbody>
              <tr><td colspan="3" class="text-muted text-center py-2">No skipped dates yet</td></tr>
            </tbody>
          </table>
          <div class="text-muted small mt-2">After adding/removing skip dates, regenerate the schedule (Schedule tab) to apply changes.</div>
        </div>
      </div>

      <!-- Tab: Bye Requests -->
      <div class="sdx-stab sdx-stab-byes card border-top-0 d-none" style="border-radius:0 0 12px 12px">
        <div class="card-body">
          <div class="row g-2 mb-3 align-items-end">
            <div class="col-auto">
              <label class="form-label small mb-1">Team</label>
              <select class="form-select form-select-sm sdx-bye-team-select" style="min-width:160px"></select>
            </div>
            <div class="col-auto">
              <label class="form-label small mb-1">Week # <span class="text-muted">(required to approve)</span></label>
              <input type="number" class="form-control form-control-sm sdx-bye-week-input" value="1" min="0" style="width:75px">
            </div>
            <div class="col">
              <label class="form-label small mb-1">Reason</label>
              <input type="text" class="form-control form-control-sm sdx-bye-reason-input"
                placeholder="e.g. Team tournament, vacation">
            </div>
            <div class="col-auto">
              <button class="btn btn-primary btn-sm" data-action="add-bye">
                <i class="bi bi-plus-lg"></i> Add
              </button>
            </div>
          </div>
          <table class="table table-sm mb-0 sdx-byes-table">
            <thead><tr><th>Team</th><th>Week</th><th>Reason</th><th>Approved</th><th></th></tr></thead>
            <tbody>
              <tr><td colspan="5" class="text-muted text-center py-2">No bye requests yet</td></tr>
            </tbody>
          </table>
        </div>
      </div>

    </div><!-- /sdx-season-mgmt -->

    <!-- Season editor modal component -->
    <season-editor></season-editor>
    `;
  }

  // ── Static event wiring (one-time in connectedCallback) ───────────────────────

  #wireStaticEvents() {
    // Tab switching
    this.querySelectorAll('[data-stab]').forEach(link => {
      link.addEventListener('click', e => {
        e.preventDefault();
        this.#showTab(link.dataset.stab);
      });
    });
    // Schedule type description
    this.querySelector('.sdx-gen-type').addEventListener('change', () => this.#onGenTypeChange());
  }

  // ── Event delegation ──────────────────────────────────────────────────────────

  #onClick(e) {
    const btn = e.target.closest('[data-action]');
    if (!btn) return;
    const action = btn.dataset.action;

    switch (action) {
      case 'new-season':
        this.querySelector('season-editor').openNew(this.#league, this.#allSeasons);
        break;
      case 'manage':
        this.#manageSeason(parseInt(btn.dataset.seasonId));
        break;
      case 'activate':
        this.#activateSeason(parseInt(btn.dataset.seasonId));
        break;
      case 'edit': {
        const s = this.#allSeasons.find(x => x.id === parseInt(btn.dataset.seasonId));
        if (s) this.querySelector('season-editor').openEdit(s, this.#league, this.#allSeasons);
        break;
      }
      case 'delete':
        this.#deleteSeason(parseInt(btn.dataset.seasonId));
        break;
      case 'activate-mgmt':
        if (this.#mgmtSeasonId) this.#activateSeason(this.#mgmtSeasonId);
        break;
      case 'go-schedule':
        this.#emitNavRequest('schedule', this.#mgmtSeasonId);
        break;
      case 'go-poster':
        this.#emitNavRequest('schedule', this.#mgmtSeasonId, true);
        break;
      case 'close-mgmt':
        this.#closeMgmt();
        break;
      case 'gen-schedule':
        this.#generateSchedule();
        break;
      case 'add-skip':
        this.#addSkip();
        break;
      case 'delete-skip':
        this.#removeSkip(parseInt(btn.dataset.skipId));
        break;
      case 'add-bye':
        this.#addBye();
        break;
      case 'delete-bye':
        this.#removeBye(parseInt(btn.dataset.byeId));
        break;
    }
  }

  #onChange(e) {
    const el = e.target;
    if (el.dataset.action === 'toggle-bye') {
      this.#toggleByeApproval(parseInt(el.dataset.byeId), el.checked);
    }
  }

  // ── Season list ───────────────────────────────────────────────────────────────

  #renderSeasonList() {
    const tbody = this.querySelector('.sdx-seasons-table tbody');
    if (!tbody) return;
    tbody.innerHTML = this.#allSeasons.map(s => {
      const typeLabel  = SCHEDULE_TYPE_LABEL[s.schedule_type] || s.schedule_type || '—';
      const weeksNote  = (s.schedule_type === 'custom' || s.schedule_type === 'blanket') && s.num_weeks
        ? ` · ${s.num_weeks}wk` : '';
      const isActive   = !!s.active;
      const isDraft    = !s.active && !s.activated_at;
      return `<tr>
        <td>
          <span class="fw-semibold">${esc(s.name)}</span>
          ${isActive ? '<span class="badge bg-success ms-1" style="font-size:.68rem">Active</span>' : ''}
        </td>
        <td class="text-muted small">${fmtDateRange(s.start_date, s.end_date)}</td>
        <td><span class="badge bg-secondary" style="font-size:.7rem">${esc(typeLabel)}${esc(weeksNote)}</span></td>
        <td></td>
        <td class="text-end" style="white-space:nowrap">
          <button class="btn btn-outline-primary btn-sm py-0 me-1"
            data-action="manage" data-season-id="${s.id}"
            title="Manage: schedule, skip weeks, bye requests, activation">
            <i class="bi bi-sliders"></i> Manage
          </button>
          ${isDraft ? `<button class="btn btn-outline-success btn-sm py-0 me-1"
            data-action="activate" data-season-id="${s.id}">Activate</button>` : ''}
          <button class="btn btn-outline-secondary btn-sm py-0 me-1"
            data-action="edit" data-season-id="${s.id}"
            title="Edit Details: name, start date, schedule type, rules">
            <i class="bi bi-pencil"></i> Edit Details
          </button>
          <button class="btn btn-outline-danger btn-sm py-0"
            data-action="delete" data-season-id="${s.id}"
            title="Delete this season and all its matches">
            <i class="bi bi-trash"></i>
          </button>
        </td>
      </tr>`;
    }).join('') || '<tr><td colspan="5" class="text-center text-muted py-3">No seasons yet. Click "New Season" to add one.</td></tr>';
  }

  // ── Season management panel ───────────────────────────────────────────────────

  async #manageSeason(id, preselectedFromSeasonId) {
    this.#mgmtSeasonId = id;
    const s = this.#allSeasons.find(x => x.id === id);
    if (!s) return;

    const mgmt = this.querySelector('.sdx-season-mgmt');
    mgmt.classList.remove('d-none');

    // Header
    this.querySelector('.sdx-mgmt-title').textContent = s.name;
    this.querySelector('.sdx-mgmt-active-badge').classList.toggle('d-none', !s.active);

    const meta = [
      fmtDateRange(s.start_date, s.end_date),
      SCHEDULE_TYPE_FULL[s.schedule_type] || s.schedule_type,
    ].filter(Boolean).join(' · ');
    this.querySelector('.sdx-mgmt-meta').textContent = meta;

    const isDraft    = !s.active && !s.activated_at;
    const isManaged  = s.teams_managed === true || s.teams_managed === 1;
    this.#isManaged  = isManaged;

    const setActiveBtn = this.querySelector('.sdx-set-active-btn');
    setActiveBtn.style.display = isDraft ? '' : 'none';
    setActiveBtn.disabled      = true;

    // Schedule setup form
    const fromSel = this.querySelector('.sdx-gen-from-season');
    this.querySelector('.sdx-gen-from-season-col').classList.toggle('d-none', isManaged);
    this.querySelector('.sdx-gen-managed-note').classList.toggle('d-none', !isManaged);
    if (isManaged) {
      fromSel.innerHTML = '<option value="">Registered teams in this season</option>';
    } else {
      fromSel.innerHTML = '<option value="">All teams in league</option>' +
        this.#allSeasons
          .filter(x => x.id !== id)
          .map(x => `<option value="${x.id}" ${x.id === preselectedFromSeasonId ? 'selected' : ''}>${esc(x.name)}</option>`)
          .join('');
    }

    this.querySelector('.sdx-gen-start-date').value = s.start_date ? s.start_date.slice(0, 10) : '';
    if (s.schedule_type) this.querySelector('.sdx-gen-type').value = s.schedule_type;
    if (s.num_weeks)     this.querySelector('.sdx-gen-numweeks').value = s.num_weeks;
    this.#onGenTypeChange();
    this.querySelector('.sdx-schedule-preview-note').classList.toggle('d-none', s.active);

    // Load season teams and (if draft) the checklist
    let seasonTeams = [], checklist = null;
    try { seasonTeams = await listSeasonTeams(id); } catch(e) {
      toast(`Could not load registered teams: ${e.message}`, 'danger');
    }
    if (isDraft) {
      try { checklist = await getSeasonChecklist(id); } catch(e) {
        toast(`Could not load setup checklist: ${e.message}`, 'danger');
      }
    }

    // Bye-team dropdown
    const byeTeams = isManaged ? seasonTeams : this.#allTeams.map(t => ({
      team_id: t.id, season_name: t.name, team_name: t.name, roster_count: 0,
    }));
    this.#teamCount = byeTeams.length;

    const teamSel = this.querySelector('.sdx-bye-team-select');
    teamSel.innerHTML = byeTeams.map(t =>
      `<option value="${t.team_id}">${esc(t.season_name || t.team_name)}</option>`
    ).join('') || '<option value="">No teams registered</option>';

    // Set Active button state
    if (isDraft && checklist) {
      setActiveBtn.disabled = !checklist.can_activate;
      setActiveBtn.classList.toggle('disabled', !checklist.can_activate);
      setActiveBtn.title = checklist.can_activate
        ? '' : 'Resolve setup blockers before activating this season.';
    }

    this.#renderChecklist(checklist, isDraft);
    this.#renderRegisteredTeams(seasonTeams, isDraft ? checklist : null, isManaged);

    this.#showTab('schedule');
    mgmt.scrollIntoView({ behavior: 'smooth', block: 'start' });

    await Promise.all([
      this.#loadSkips(id),
      this.#loadByes(id),
    ]);
  }

  #closeMgmt() {
    this.querySelector('.sdx-season-mgmt').classList.add('d-none');
    this.#mgmtSeasonId = null;
    this.#teamCount    = 0;
    this.#isManaged    = false;
  }

  #renderChecklist(checklist, isDraft) {
    const section = this.querySelector('.sdx-checklist-section');
    const body    = section.querySelector('.card-body');
    if (!checklist) { section.classList.add('d-none'); body.innerHTML = ''; return; }

    const blockers = checklist.blockers || [];
    const warnings = checklist.warnings || [];
    const rows = [];
    blockers.forEach(item => rows.push(`
      <div class="text-danger d-flex gap-2 align-items-start mb-1">
        <i class="bi bi-x-circle-fill"></i>
        <span><strong>${esc(item.code)}</strong>: ${esc(item.message)}</span>
      </div>`));
    warnings.forEach(item => rows.push(`
      <div class="text-warning d-flex gap-2 align-items-start mb-1">
        <i class="bi bi-exclamation-triangle-fill"></i>
        <span><strong>${esc(item.code)}</strong>: ${esc(item.message)}</span>
      </div>`));

    const status = checklist.can_activate
      ? '<div class="text-success"><i class="bi bi-check-circle-fill"></i> Setup checklist is clear.</div>'
      : '<div class="fw-semibold text-danger mb-1"><i class="bi bi-clipboard-x"></i> Resolve setup blockers before activation.</div>';
    const activeNote = isDraft ? '' : '<div class="text-muted">Activation controls are hidden for active and historical seasons.</div>';
    body.innerHTML = `
      <div class="fw-semibold mb-1"><i class="bi bi-clipboard-check"></i> Setup Checklist</div>
      ${status}${rows.join('')}${activeNote}`;
    section.classList.remove('d-none');
  }

  #renderRegisteredTeams(teams, checklist, isManaged) {
    const section = this.querySelector('.sdx-registered-teams-section');
    const body    = section.querySelector('.card-body');
    if (!isManaged) { section.classList.add('d-none'); body.innerHTML = ''; return; }

    const blockers = checklist?.blockers || [];
    const warnings = checklist?.warnings || [];
    const items    = [...blockers, ...warnings];

    const rows = teams.map(t => {
      const name = t.season_name || t.team_name;
      const teamIssues = items.filter(item => item.message && item.message.indexOf(`team "${name}"`) >= 0);
      const issueHtml  = teamIssues.map(item => `
        <div class="${blockers.includes(item) ? 'text-danger' : 'text-warning'} small">
          <i class="bi bi-exclamation-triangle"></i> ${esc(item.message)}
        </div>`).join('');
      return `
        <div class="border rounded px-2 py-1 mb-1">
          <div class="d-flex justify-content-between gap-2">
            <span class="fw-semibold">${esc(name)}</span>
            <span class="badge bg-secondary">${t.roster_count} player${t.roster_count === 1 ? '' : 's'}</span>
          </div>
          <div class="text-muted">
            Captain: ${t.captain_name ? esc(t.captain_name) : '<span class="fst-italic">No captain</span>'}
          </div>
          ${issueHtml}
        </div>`;
    }).join('');

    body.innerHTML = `
      <div class="fw-semibold mb-1"><i class="bi bi-people"></i> Registered Teams (${teams.length})</div>
      ${rows || '<div class="text-muted fst-italic">No teams registered for this season yet.</div>'}`;
    section.classList.remove('d-none');
  }

  // ── Season CRUD ───────────────────────────────────────────────────────────────

  async #activateSeason(id) {
    try {
      await activateSeason(id);
      toast('Season activated');
      await this.#refreshState();
      if (this.#mgmtSeasonId === id) await this.#manageSeason(id);
    } catch(e) { toast(e.message, 'danger'); }
  }

  async #deleteSeason(id) {
    if (!confirm('Delete this season and all its matches?')) return;
    try {
      await deleteSeason(id);
      toast('Deleted');
      if (this.#mgmtSeasonId === id) this.#closeMgmt();
      await this.#refreshState();
    } catch(e) { toast(e.message, 'danger'); }
  }

  // ── Schedule generation ───────────────────────────────────────────────────────

  async #generateSchedule() {
    if (!this.#mgmtSeasonId) { toast('No season selected', 'warning'); return; }
    const type      = this.querySelector('.sdx-gen-type').value;
    const startDate = this.querySelector('.sdx-gen-start-date').value;
    const numWeeks  = parseInt(this.querySelector('.sdx-gen-numweeks').value) || 0;
    const mpw       = parseInt(this.querySelector('.sdx-gen-mpw').value) || 1;
    const fromSeason = this.#isManaged
      ? 0
      : (parseInt(this.querySelector('.sdx-gen-from-season').value) || 0);

    let skipDates = [];
    try {
      const skips = await listSkippedWeeks(this.#mgmtSeasonId);
      skipDates = skips.map(s => s.skip_date);
    } catch(_) {}

    const body = {
      season_id:        this.#mgmtSeasonId,
      start_date:       startDate || '',
      schedule_type:    type,
      num_weeks:        numWeeks,
      matches_per_week: mpw,
      skip_dates:       skipDates,
      from_season_id:   fromSeason,
    };

    if (!confirm('This will replace all unplayed matches for this season. Completed matches are preserved. Continue?')) return;

    try {
      const res = await generateSchedule(body);
      toast(`Schedule generated: ${res.matches_created} matches`);
      await this.#refreshState();
      if (this.#mgmtSeasonId) await this.#manageSeason(this.#mgmtSeasonId);
    } catch(e) { toast(e.message, 'danger'); }
  }

  // ── Skip weeks ────────────────────────────────────────────────────────────────

  async #loadSkips(seasonId) {
    let skips = [];
    try { skips = await listSkippedWeeks(seasonId); } catch(_) {}
    const tbody = this.querySelector('.sdx-skips-table tbody');
    tbody.innerHTML = skips.length
      ? skips.map(sk => `<tr>
          <td>${displayDate(sk.skip_date)}</td>
          <td>${sk.reason || '<span class="text-muted">—</span>'}</td>
          <td class="text-end">
            <button class="btn btn-outline-danger btn-sm py-0"
              data-action="delete-skip" data-skip-id="${sk.id}">
              <i class="bi bi-trash"></i>
            </button>
          </td>
        </tr>`).join('')
      : '<tr><td colspan="3" class="text-muted text-center py-2">No skipped dates yet</td></tr>';
  }

  async #addSkip() {
    if (!this.#mgmtSeasonId) return;
    const date   = this.querySelector('.sdx-skip-date-input').value;
    const reason = this.querySelector('.sdx-skip-reason-input').value.trim();
    if (!date) { toast('Date is required', 'warning'); return; }
    try {
      await addSkippedWeek(this.#mgmtSeasonId, { skip_date: date, reason });
      this.querySelector('.sdx-skip-date-input').value   = '';
      this.querySelector('.sdx-skip-reason-input').value = '';
      toast('Skip date added');
      await this.#loadSkips(this.#mgmtSeasonId);
    } catch(e) { toast(e.message, 'danger'); }
  }

  async #removeSkip(id) {
    if (!confirm('Remove this skip date?')) return;
    try {
      await removeSkippedWeek(this.#mgmtSeasonId, id);
      toast('Removed');
      await this.#loadSkips(this.#mgmtSeasonId);
    } catch(e) { toast(e.message, 'danger'); }
  }

  // ── Bye requests ──────────────────────────────────────────────────────────────

  async #loadByes(seasonId) {
    let byes = [];
    try { byes = await listByeRequests(seasonId); } catch(_) {}

    const teamCount = this.#teamCount;
    const isEven    = teamCount > 0 && teamCount % 2 === 0;
    const banner    = isEven
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

    const container = this.querySelector('.sdx-stab-byes .card-body');
    let bannerEl = container.querySelector('.sdx-bye-banner');
    if (!bannerEl) {
      bannerEl = document.createElement('div');
      bannerEl.className = 'sdx-bye-banner';
      container.insertBefore(bannerEl, container.firstChild);
    }
    bannerEl.innerHTML = banner;

    const tbody = this.querySelector('.sdx-byes-table tbody');
    tbody.innerHTML = byes.length
      ? byes.map(b => `<tr>
          <td>${esc(b.team_name || String(b.team_id))}</td>
          <td>${b.week_number || 'TBD'}</td>
          <td>${b.reason || '<span class="text-muted">—</span>'}</td>
          <td>
            <div class="form-check form-switch mb-0">
              <input class="form-check-input" type="checkbox" role="switch"
                data-action="toggle-bye" data-bye-id="${b.id}"
                ${b.approved ? 'checked' : ''}>
            </div>
          </td>
          <td class="text-end">
            <button class="btn btn-outline-danger btn-sm py-0"
              data-action="delete-bye" data-bye-id="${b.id}">
              <i class="bi bi-trash"></i>
            </button>
          </td>
        </tr>`).join('')
      : '<tr><td colspan="5" class="text-muted text-center py-2">No bye requests yet</td></tr>';
  }

  async #addBye() {
    if (!this.#mgmtSeasonId) return;
    const teamId = this.querySelector('.sdx-bye-team-select').value;
    const week   = parseInt(this.querySelector('.sdx-bye-week-input').value) || 0;
    const reason = this.querySelector('.sdx-bye-reason-input').value.trim();
    if (!teamId) { toast('Select a team', 'warning'); return; }
    try {
      await addByeRequest(this.#mgmtSeasonId, { team_id: parseInt(teamId), week_number: week, reason });
      this.querySelector('.sdx-bye-reason-input').value = '';
      this.querySelector('.sdx-bye-week-input').value   = '0';
      toast('Bye request added');
      await this.#loadByes(this.#mgmtSeasonId);
    } catch(e) { toast(e.message, 'danger'); }
  }

  async #toggleByeApproval(id, approved) {
    try {
      await updateByeRequest(this.#mgmtSeasonId, id, { approved });
      toast(approved ? 'Bye approved' : 'Bye unapproved');
      await this.#loadByes(this.#mgmtSeasonId);
    } catch(e) { toast(e.message, 'danger'); }
  }

  async #removeBye(id) {
    if (!confirm('Remove this bye request?')) return;
    try {
      await removeByeRequest(this.#mgmtSeasonId, id);
      toast('Removed');
      await this.#loadByes(this.#mgmtSeasonId);
    } catch(e) { toast(e.message, 'danger'); }
  }

  // ── Tab management ────────────────────────────────────────────────────────────

  #showTab(tab) {
    this.querySelectorAll('[data-stab]').forEach(a => a.classList.remove('active'));
    this.querySelectorAll('.sdx-stab').forEach(d => d.classList.add('d-none'));
    this.querySelector(`[data-stab="${tab}"]`)?.classList.add('active');
    this.querySelector(`.sdx-stab-${tab}`)?.classList.remove('d-none');
  }

  // ── Schedule type description ─────────────────────────────────────────────────

  #onGenTypeChange() {
    const t   = this.querySelector('.sdx-gen-type')?.value;
    const desc = {
      single_rr: '<strong>Single Round Robin</strong> — Each team plays every other team once. Good for short seasons.',
      double_rr: '<strong>Double Round Robin</strong> — Each team plays every other team twice: once as home, once as visitor. Standard full-season format.',
      split:     '<strong>Split Season</strong> — Double round-robin split into two equal halves. Standings reset at the midpoint; teams play for each half separately.',
      custom:    '<strong>Custom (Fixed Weeks)</strong> — Repeats the pairing rotation for exactly the number of weeks you specify. Use when you want to control the exact length.',
      blanket:   '<strong>Blanket (Empty Slots)</strong> — Creates N weeks of blank match slots without assigning teams. Use "Matches per Week" to control how many slots appear each week, then assign teams to slots manually.',
    };
    const descEl = this.querySelector('.sdx-gen-type-desc');
    if (descEl) descEl.innerHTML = desc[t] || '';

    const nwCol  = this.querySelector('.sdx-gen-numweeks-col');
    const mpwCol = this.querySelector('.sdx-gen-mpw-col');
    if (nwCol)  nwCol.style.display  = (t === 'custom' || t === 'blanket') ? '' : 'none';
    if (mpwCol) mpwCol.style.display = t === 'blanket' ? '' : 'none';
  }

  // ── After season editor saves ─────────────────────────────────────────────────

  async #onEditorSaved(e) {
    const { saved, isNew, copyFromSeasonId } = e.detail;
    await this.#refreshState();
    this.#renderSeasonList();
    if (isNew && saved?.id) {
      await this.#manageSeason(saved.id, copyFromSeasonId);
    }
  }

  // ── Shell state sync ──────────────────────────────────────────────────────────

  async #refreshState() {
    if (!this.#league?.id) return;
    const fresh = await listSeasons(this.#league.id);
    this.#allSeasons = fresh;
    const activeSeason = fresh.find(s => s.active) || null;
    this.#renderSeasonList();
    this.#emitStateChanged(fresh, activeSeason);
  }

  #emitStateChanged(allSeasons, activeSeason) {
    this.dispatchEvent(new CustomEvent('season-state-changed', {
      bubbles: true,
      detail: { allSeasons, activeSeason },
    }));
  }

  #emitNavRequest(section, previewSeasonId = null, openPoster = false) {
    this.dispatchEvent(new CustomEvent('season-nav-request', {
      bubbles: true,
      detail: { section, previewSeasonId, openPoster },
    }));
  }
}

customElements.define('seasons-page', SeasonsPage);
