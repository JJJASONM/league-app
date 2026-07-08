// <dashboard-page> component.
//
// Public API:
//   refresh(activeLeague, activeSeason, allTeams, allPlayers)
//     Called by the shell when the Dashboard section activates or context changes.
//     Stores context, then fetches matches and standings and renders the section.
//
// Custom events dispatched (bubbles: true):
//   dashboard-nav-request  { detail: { section } }
//     Fired when the user clicks a navigation action button.
//     The shell handles this by calling navTo(section).

import { fetchDashboardMatches, fetchDashboardStandings } from './dashboard-api-service.js';

class DashboardPage extends HTMLElement {
  #activeLeague = null;
  #activeSeason = null;
  #allTeams     = [];
  #allPlayers   = [];

  connectedCallback() {
    this.innerHTML = `
      <div class="d-flex justify-content-between align-items-center mb-3">
        <div>
          <h4 class="mb-0 fw-bold">What Needs Doing</h4>
          <div class="text-muted small db-league-label"></div>
        </div>
        <button class="btn btn-outline-secondary btn-sm" data-action="refresh">
          <i class="bi bi-arrow-clockwise"></i> Refresh
        </button>
      </div>
      <div class="row g-3">
        <div class="col-lg-7">
          <div class="card">
            <div class="db-actions">
              <div class="text-muted text-center py-4">Loading\u2026</div>
            </div>
          </div>
        </div>
        <div class="col-lg-5">
          <div class="card mb-3">
            <div class="card-header fw-semibold py-2">Upcoming This Week</div>
            <div class="card-body p-0">
              <table class="table table-sm mb-0 db-upcoming"></table>
            </div>
          </div>
          <div class="card">
            <div class="card-header fw-semibold py-2">Season Standings</div>
            <div class="card-body p-0">
              <table class="table table-sm mb-0 db-standings"></table>
            </div>
          </div>
        </div>
      </div>`;

    this.addEventListener('click', e => {
      if (e.target.closest('[data-action="refresh"]')) {
        this.dispatchEvent(new CustomEvent('dashboard-refresh-request', { bubbles: true }));
        return;
      }
      const navBtn = e.target.closest('[data-navigate]');
      if (navBtn) {
        this.dispatchEvent(new CustomEvent('dashboard-nav-request', {
          bubbles: true,
          detail: { section: navBtn.dataset.navigate },
        }));
      }
    });
  }

  refresh(activeLeague, activeSeason, allTeams, allPlayers) {
    this.#activeLeague = activeLeague;
    this.#activeSeason = activeSeason;
    this.#allTeams     = allTeams   ?? [];
    this.#allPlayers   = allPlayers ?? [];
    this.#load();
  }

  // -- Private ------------------------------------------------------------------

  async #load() {
    if (!this.#activeLeague) return;

    const fmtLabel = { '8ball': '8-Ball', '9ball': '9-Ball', '10ball': '10-Ball', 'straight': 'Straight Pool' };
    const leagueLabel = this.querySelector('.db-league-label');
    if (leagueLabel) {
      leagueLabel.textContent = this.#activeLeague.name +
        (this.#activeLeague.game_format
          ? ' \u00b7 ' + (fmtLabel[this.#activeLeague.game_format] ?? this.#activeLeague.game_format)
          : '');
    }

    const actionsEl   = this.querySelector('.db-actions');
    const upcomingEl  = this.querySelector('.db-upcoming');
    const standingsEl = this.querySelector('.db-standings');

    if (!this.#activeSeason) {
      if (actionsEl) actionsEl.innerHTML = this.#actionItem('urgent', 'bi-exclamation-circle-fill',
        'No active season',
        'Create or activate a season before entering matches.',
        `<button class="btn btn-sm btn-outline-danger action-btn" data-navigate="seasons">Go to Seasons</button>`);
      if (upcomingEl)  upcomingEl.innerHTML  = '<tbody><tr><td class="text-muted text-center py-3">No active season</td></tr></tbody>';
      if (standingsEl) standingsEl.innerHTML = '<tbody><tr><td class="text-muted text-center py-3">No active season</td></tr></tbody>';
      return;
    }

    let matches = [], standings = [];
    try {
      [matches, standings] = await Promise.all([
        fetchDashboardMatches(this.#activeSeason.id),
        fetchDashboardStandings(this.#activeSeason.id),
      ]);
    } catch (e) {
      if (actionsEl) actionsEl.innerHTML = `<div class="text-danger p-3">${e.message}</div>`;
      return;
    }

    const today = new Date(); today.setHours(0, 0, 0, 0);
    const todayStr = today.toISOString().slice(0, 10);
    const nextWeek = new Date(today); nextWeek.setDate(today.getDate() + 7);

    const pending   = matches.filter(m => !m.completed);
    const completed = matches.filter(m =>  m.completed);

    const overdue = pending.filter(m => m.match_date && m.match_date <= todayStr);
    const upcoming = pending.filter(m => {
      if (!m.match_date) return false;
      const d = new Date(m.match_date + 'T00:00:00');
      return d > today && d <= nextWeek;
    });
    const undated = pending.filter(m => !m.match_date);

    const overdueByWeek = {};
    overdue.forEach(m => { (overdueByWeek[m.week_number] = overdueByWeek[m.week_number] || []).push(m); });

    const sections = [];

    // Setup checks
    const setupItems = [];
    if (this.#allTeams.length === 0)
      setupItems.push(this.#actionItem('urgent', 'bi-people-fill', 'No teams in this league',
        'Add teams before generating a schedule.',
        `<button class="btn btn-sm btn-outline-danger action-btn" data-navigate="teams">Add Teams</button>`));
    if (this.#allPlayers.length === 0)
      setupItems.push(this.#actionItem('warn', 'bi-person-badge', 'No players added',
        'Add players to teams so scoresheets can be generated.',
        `<button class="btn btn-sm btn-outline-warning action-btn" data-navigate="players">Add Players</button>`));
    if (matches.length === 0)
      setupItems.push(this.#actionItem('warn', 'bi-calendar-week', 'No schedule generated',
        'Generate the round-robin schedule for ' + this.#activeSeason.name + '.',
        `<button class="btn btn-sm btn-outline-warning action-btn" data-navigate="seasons">Generate Schedule</button>`));
    if (setupItems.length)
      sections.push('<div class="dash-section-header">Setup</div>' + setupItems.join(''));

    // Weekly workflow
    const weeklyItems = [];

    if (overdue.length > 0) {
      const weeks = Object.keys(overdueByWeek).sort((a, b) => a - b);
      weeks.forEach(w => {
        const ms = overdueByWeek[w];
        weeklyItems.push(this.#actionItem('urgent', 'bi-pencil-square',
          'Week ' + w + ' \u2014 scores not entered (' + ms.length + ' match' + (ms.length > 1 ? 'es' : '') + ')',
          ms.map(m => m.home_team_name + ' vs ' + m.away_team_name).join(' &nbsp;\u00b7&nbsp; '),
          `<button class="btn btn-sm btn-outline-danger action-btn" data-navigate="entry">Enter Scores</button>`));
      });
    }

    if (upcoming.length > 0) {
      const nextDate = upcoming[0].match_date;
      weeklyItems.push(this.#actionItem('warn', 'bi-file-earmark-text',
        'Scoresheets not yet created for ' + upcoming.length + ' upcoming match' + (upcoming.length > 1 ? 'es' : ''),
        'Next match date: ' + displayDate(nextDate) + '. Generate scoresheets before league night.',
        `<button class="btn btn-sm btn-outline-secondary action-btn" disabled title="Coming soon">Scoresheets (soon)</button>`));
    }

    if (completed.length > 0) {
      const lastWeek = Math.max(...completed.map(m => m.week_number));
      weeklyItems.push(this.#actionItem('warn', 'bi-envelope',
        'Weekly email not yet sent for Week ' + lastWeek,
        'Results, standings update, and any announcements.',
        `<button class="btn btn-sm btn-outline-secondary action-btn" disabled title="Coming soon">Compose Email (soon)</button>`));
    }

    if (weeklyItems.length)
      sections.push('<div class="dash-section-header">This Week</div>' + weeklyItems.join(''));

    if (setupItems.length === 0 && weeklyItems.length === 0) {
      sections.push(this.#actionItem('ok', 'bi-check-circle-fill',
        'All caught up!',
        completed.length + ' matches recorded \u00b7 ' + pending.length + ' remaining in ' + this.#activeSeason.name + '.',
        ''));
    }

    if (undated.length > 0) {
      sections.push('<div class="dash-section-header">Notes</div>' +
        this.#actionItem('info', 'bi-calendar-x',
          undated.length + ' match' + (undated.length > 1 ? 'es' : '') + ' have no date assigned',
          'Dates can be set when editing the schedule.',
          `<button class="btn btn-sm btn-outline-secondary action-btn" data-navigate="schedule">View Schedule</button>`));
    }

    if (actionsEl) actionsEl.innerHTML = sections.join('') ||
      '<div class="text-muted text-center py-4">Nothing to show yet.</div>';

    if (upcomingEl) upcomingEl.innerHTML = `
      <thead><tr><th>Date</th><th>Home</th><th>Away</th></tr></thead>
      <tbody>${upcoming.length
        ? upcoming.map(m => `<tr>
            <td class="text-muted small">${displayDate(m.match_date)}</td>
            <td>${m.home_team_name}</td>
            <td>${m.away_team_name}</td></tr>`).join('')
        : '<tr><td colspan="3" class="text-muted text-center py-3">No matches in the next 7 days</td></tr>'
      }</tbody>`;

    if (standingsEl) {
      const top = standings.slice(0, 6);
      standingsEl.innerHTML = `
        <thead><tr><th>#</th><th>Team</th><th>Pts</th><th>W-L</th></tr></thead>
        <tbody>${top.length
          ? top.map((s, i) => `<tr${i === 0 ? ' class="fw-semibold"' : ''}>
              <td>${i + 1}</td><td>${s.team_name}</td>
              <td>${s.points}</td><td class="text-muted">${s.wins}-${s.losses}</td></tr>`).join('')
          : '<tr><td colspan="4" class="text-muted text-center py-3">No completed matches</td></tr>'
        }</tbody>`;
    }
  }

  #actionItem(level, icon, title, detail, btnHtml) {
    return `<div class="action-item ${level}">
      <div class="action-icon"><i class="bi ${icon}"></i></div>
      <div class="flex-grow-1">
        <div class="action-title">${title}</div>
        ${detail ? `<div class="action-detail">${detail}</div>` : ''}
      </div>
      ${btnHtml || ''}
    </div>`;
  }
}

customElements.define('dashboard-page', DashboardPage);
