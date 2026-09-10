// <league-admin-page> - League Admin Screen Phase 1: a read-only
// operational command center for league_admin/admin/system_admin users. It
// links the existing operational workflows (weekly scoring, lineups/subs,
// finances, communication, player/user, setup) together and surfaces their
// current state from data those screens already expose. It adds no new
// business rules, no new backend routes, and no new permission model --
// the app shell gates this screen to the same role set Financial, Player
// Overview, and Communication already use.
//
// Every card degrades to a plain "jump to that screen" link when its data
// is unavailable or not cheap to fetch -- it never fabricates a count.
//
// Public API:
//   refresh(activeLeague, activeSeason, allSeasons, allTeams, allPlayers, identity)
//     Called by the app shell when the League Admin section activates or
//     league/season context changes. allPlayers is the full league player
//     list, used to resolve lineup readiness the same way Match Entry does
//     (a substitute's player_id may not be on the team's own roster).
//     identity is the resolved Admin Key identity (for the
//     system_admin/admin-only Users link); the shell has already confirmed
//     the viewer is at least league_admin before showing this screen.
//
// Custom events dispatched (bubbles: true):
//   admin-nav-request { detail: { section } }
//     Fired when the user clicks a jump/link button. The shell handles it
//     by calling navTo(section) -- the same one-line pattern
//     dashboard-nav-request already uses.

import { fetchSeasonWeeks, fetchWeekRecap, fetchLineupPlans, fetchSeasonDues } from './league-admin-api-service.js';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

// Mirrors weekly-summary-page-component.js's #matchStatus ladder
// (Unscored / Scored / Approved / Processed / Closed), as a key rather
// than a badge -- that method is private to that component.
function matchStatusKey(m) {
  if (m.week_closed) return 'closed';
  if (m.processed_at) return 'processed';
  if (m.approved_at) return 'approved';
  if (m.has_result) return 'scored';
  return 'missing';
}

class LeagueAdminPage extends HTMLElement {
  #activeLeague = null;
  #activeSeason = null;
  #allTeams     = [];
  #allPlayers   = [];
  #identity     = null;

  connectedCallback() {
    this.innerHTML = `
      <h4 class="mb-1 fw-bold">League Admin</h4>
      <p class="text-muted small">
        Operational command center -- what needs attention now, and where to
        go next. Read-only; each card links to the screen that acts on it.
      </p>
      <div class="la-body"></div>`;

    this.addEventListener('click', e => {
      const navBtn = e.target.closest('[data-navigate]');
      if (navBtn) {
        e.preventDefault();
        this.dispatchEvent(new CustomEvent('admin-nav-request', {
          bubbles: true,
          detail: { section: navBtn.dataset.navigate },
        }));
      }
    });
  }

  refresh(activeLeague, activeSeason, allSeasons, allTeams, allPlayers, identity) {
    this.#activeLeague = activeLeague ?? null;
    this.#activeSeason = activeSeason ?? null;
    this.#allTeams     = allTeams   ?? [];
    this.#allPlayers   = allPlayers ?? [];
    this.#identity     = identity   ?? null;
    this.#load();
  }

  // -- Private ------------------------------------------------------------------

  #canManageUsers() {
    const r = this.#identity?.role;
    return r === 'system_admin' || r === 'admin';
  }

  #pickFocusWeek(weeks) {
    const withMatches = (weeks || []).filter(w => w.match_count > 0);
    if (withMatches.length === 0) return null;
    const open = withMatches.find(w => w.status !== 'closed');
    return (open || withMatches[withMatches.length - 1]).week_number;
  }

  async #load() {
    const body = this.querySelector('.la-body');
    if (!body) return;

    if (!this.#activeSeason) {
      body.innerHTML = this.#renderNoSeason();
      return;
    }

    const seasonId = this.#activeSeason.id;
    body.innerHTML = '<div class="text-muted text-center py-4">Loading...</div>';

    // Season-wide week status + dues are the two cheap season-level reads.
    let weeks = null, dues = null;
    try {
      [weeks, dues] = await Promise.all([
        fetchSeasonWeeks(seasonId).catch(() => null),
        fetchSeasonDues(seasonId).catch(() => null),
      ]);
    } catch (_) {
      weeks = weeks || null;
      dues = dues || null;
    }

    const focusWeek = this.#pickFocusWeek(weeks);

    // For the focus week: one recap, the week-specific lineup plans, and
    // the default (week 0) lineup plans -- the last two feed the same
    // week-specific/default fallback resolution Match Entry uses.
    let recap = null, weekPlans = null, defaultPlans = null;
    if (focusWeek != null) {
      [recap, weekPlans, defaultPlans] = await Promise.all([
        fetchWeekRecap(seasonId, focusWeek).catch(() => null),
        fetchLineupPlans(seasonId, focusWeek).catch(() => null),
        fetchLineupPlans(seasonId, 0).catch(() => null),
      ]);
    }

    body.innerHTML =
      this.#renderSeasonWeekCard(weeks, focusWeek) +
      `<div class="row g-3">
        ${this.#col(this.#renderScoreProcessingCard(weeks, recap, focusWeek))}
        ${this.#col(this.#renderLineupsCard(recap, weekPlans, defaultPlans, focusWeek))}
        ${this.#col(this.#renderMoneyCard(dues))}
        ${this.#col(this.#renderCommunicationCard())}
        ${this.#col(this.#renderPlayerUserCard())}
      </div>` +
      this.#renderJumpToCard();
  }

  #col(inner) { return `<div class="col-lg-6">${inner}</div>`; }

  #card(title, bodyHtml) {
    return `<div class="card h-100">
      <div class="card-header fw-semibold py-2">${esc(title)}</div>
      <div class="card-body">${bodyHtml}</div>
    </div>`;
  }

  #navBtn(section, label, cls = 'btn-outline-secondary') {
    return `<button class="btn btn-sm ${cls}" data-navigate="${esc(section)}">${esc(label)}</button>`;
  }

  #renderNoSeason() {
    return `<div class="alert alert-warning">
      <i class="bi bi-exclamation-triangle me-1"></i>
      No active season for this league. Activate or create one to see weekly
      status.
    </div>
    <div class="d-flex flex-wrap gap-2">
      ${this.#navBtn('seasons', 'Seasons')}
      ${this.#navBtn('teams', 'Teams')}
      ${this.#navBtn('players', 'Players')}
    </div>`;
  }

  #renderSeasonWeekCard(weeks, focusWeek) {
    const seasonName = esc(this.#activeSeason.name);
    let weekLine;
    if (!weeks) {
      weekLine = '<span class="text-muted">Week status unavailable.</span>';
    } else if (focusWeek == null) {
      weekLine = '<span class="text-muted">No schedule generated yet.</span> ' +
        this.#navBtn('seasons', 'Generate Schedule', 'btn-outline-primary');
    } else {
      const w = weeks.find(x => x.week_number === focusWeek);
      const status = w && w.status === 'closed' ? 'Closed' : 'Open';
      weekLine = `Current focus: <strong>Week ${focusWeek}</strong>
        <span class="badge ${status === 'Closed' ? 'bg-success' : 'bg-secondary'}">${status}</span>`;
    }
    return `<div class="card mb-3">
      <div class="card-body py-2">
        <span class="text-muted small">Season</span>
        <span class="fw-semibold ms-1">${seasonName}</span>
        <span class="mx-2 text-muted">&middot;</span>
        ${weekLine}
      </div>
    </div>`;
  }

  #renderScoreProcessingCard(weeks, recap, focusWeek) {
    const links = `<div class="d-flex flex-wrap gap-2 mt-3">
      ${this.#navBtn('weekly-summary', 'Weekly Summary', 'btn-outline-primary')}
      ${this.#navBtn('schedule', 'Schedule')}
    </div>`;

    if (focusWeek == null) {
      return this.#card('Weekly Score Processing',
        '<p class="text-muted small mb-0">No weeks with matches yet.</p>' + links);
    }

    let seasonLine = '';
    if (weeks) {
      const totMatches = weeks.reduce((n, w) => n + w.match_count, 0);
      const totScored  = weeks.reduce((n, w) => n + w.completed_count, 0);
      const closedWeeks = weeks.filter(w => w.status === 'closed').length;
      const weeksWithMatches = weeks.filter(w => w.match_count > 0).length;
      seasonLine = `<p class="small text-muted mb-2">
        Season: ${totScored}/${totMatches} matches scored &middot;
        ${closedWeeks}/${weeksWithMatches} weeks closed
      </p>`;
    }

    let ladder;
    if (!recap) {
      ladder = `<p class="text-muted small mb-0">Week ${focusWeek} detail unavailable -- open Weekly Summary.</p>`;
    } else {
      const counts = { missing: 0, scored: 0, approved: 0, processed: 0, closed: 0 };
      (recap.matches || []).forEach(m => { counts[matchStatusKey(m)] += 1; });
      const chip = (label, n, cls) =>
        `<span class="badge ${cls} me-1">${label}: ${n}</span>`;
      ladder = `<div class="mb-1"><span class="small text-muted">Week ${focusWeek}:</span></div>
        <div>
          ${chip('Missing', counts.missing, 'bg-warning text-dark')}
          ${chip('Scored', counts.scored, 'bg-secondary')}
          ${chip('Approved', counts.approved, 'bg-info text-dark')}
          ${chip('Processed', counts.processed, 'bg-primary')}
          ${chip('Closed', counts.closed, 'bg-success')}
        </div>`;
    }

    return this.#card('Weekly Score Processing', seasonLine + ladder + links);
  }

  #renderLineupsCard(recap, weekPlans, defaultPlans, focusWeek) {
    const links = `<div class="d-flex flex-wrap gap-2 mt-3">
      ${this.#navBtn('lineup', 'Lineups', 'btn-outline-primary')}
      ${this.#navBtn('entry', 'Match Entry')}
    </div>`;

    if (focusWeek == null || !recap || !weekPlans) {
      return this.#card('Lineups & Substitutes',
        '<p class="text-muted small mb-0">Lineup readiness needs a scheduled week -- open Lineups to set them.</p>' + links);
    }
    const dflt = defaultPlans || [];

    const teamIds = new Set();
    (recap.matches || []).forEach(m => {
      if (m.home_team_id != null) teamIds.add(m.home_team_id);
      if (m.away_team_id != null) teamIds.add(m.away_team_id);
    });

    // Resolve each playing team's lineup the same way Match Entry does:
    // use the week-specific plans when the team has at least 3, otherwise
    // fall back to the default (week 0) plans; the team is ready only when
    // the first 3 resolved rows all resolve to a real player. Players are
    // matched against the full league player list, not the team roster,
    // because a valid substitute (Substitute Workflow Phase 1) may carry a
    // player_id that is not on this team's roster.
    let readyCount = 0;
    const resolvedByTeam = {};
    teamIds.forEach(teamId => {
      const wk = weekPlans.filter(p => p.team_id == teamId);
      const resolvedPlans = wk.length >= 3 ? wk : dflt.filter(p => p.team_id == teamId);
      resolvedByTeam[teamId] = resolvedPlans;
      const resolvedPlayers = resolvedPlans.slice(0, 3)
        .map(lp => this.#allPlayers.find(p => p.id === lp.player_id))
        .filter(Boolean);
      if (resolvedPlayers.length === 3) readyCount += 1;
    });

    // Substitutes: count is_sub rows only within the resolved first-3
    // lineup slots of teams actually playing the focus week, so a stray
    // lineup row for a non-playing team cannot inflate the number.
    let subCount = 0;
    Object.values(resolvedByTeam).forEach(plans => {
      subCount += plans.slice(0, 3).filter(p => p.is_sub).length;
    });

    const allReady = teamIds.size > 0 && readyCount === teamIds.size;
    return this.#card('Lineups & Substitutes', `
      <div class="mb-1">
        <span class="badge ${allReady ? 'bg-success' : 'bg-warning text-dark'}">
          Lineups ready: ${readyCount}/${teamIds.size} teams
        </span>
      </div>
      <div>
        <span class="badge ${subCount > 0 ? 'bg-info text-dark' : 'bg-light text-muted'}">
          Substitutes in use (Week ${focusWeek}): ${subCount}
        </span>
      </div>
      <p class="small text-muted mt-2 mb-0">
        Readiness uses the same week-specific / default-lineup fallback and
        3-player resolution as Match Entry.
      </p>
      ${links}`);
  }

  #renderMoneyCard(dues) {
    const links = `<div class="d-flex flex-wrap gap-2 mt-3">
      ${this.#navBtn('finances', 'Financial', 'btn-outline-primary')}
    </div>`;

    if (!dues || !Array.isArray(dues.players)) {
      return this.#card('Money',
        '<p class="text-muted small mb-0">Dues status unavailable -- open the Financial screen.</p>' + links);
    }

    const total = dues.players.length;
    const unpaid = dues.players.filter(p => !p.paid).length;
    const cls = unpaid > 0 ? 'bg-warning text-dark' : 'bg-success';
    return this.#card('Money', `
      <div><span class="badge ${cls}">Unpaid dues: ${unpaid}/${total} players</span></div>
      ${unpaid > 0
        ? '<p class="small text-muted mt-2 mb-0">Use Communication &rarr; Team Dues Reminder to send a per-team list.</p>'
        : '<p class="small text-muted mt-2 mb-0">Everyone rostered has at least one payment recorded.</p>'}
      ${links}`);
  }

  #renderCommunicationCard() {
    return this.#card('Communication', `
      <p class="small mb-2">Generate copy/paste messages from current data:</p>
      <ul class="small mb-0 ps-3">
        <li>Weekly Team Summary</li>
        <li>Team Dues Reminder</li>
        <li>Player Summary</li>
      </ul>
      <div class="d-flex flex-wrap gap-2 mt-3">
        ${this.#navBtn('communications', 'Communication', 'btn-outline-primary')}
      </div>`);
  }

  #renderPlayerUserCard() {
    const usersBtn = this.#canManageUsers() ? this.#navBtn('users', 'Users') : '';
    return this.#card('Players & Users', `
      <p class="small mb-2">Look up a player's schedule/stats/dues, or manage accounts.</p>
      <div class="d-flex flex-wrap gap-2">
        ${this.#navBtn('players', 'Players', 'btn-outline-primary')}
        ${this.#navBtn('player-overview', 'Player Overview')}
        ${usersBtn}
      </div>`);
  }

  #renderJumpToCard() {
    const usersBtn = this.#canManageUsers() ? this.#navBtn('users', 'Users') : '';
    return `<div class="card mt-3">
      <div class="card-header fw-semibold py-2">Jump to</div>
      <div class="card-body d-flex flex-wrap gap-2">
        ${this.#navBtn('schedule', 'Schedule')}
        ${this.#navBtn('weekly-summary', 'Weekly Summary')}
        ${this.#navBtn('finances', 'Financial')}
        ${this.#navBtn('communications', 'Communication')}
        ${this.#navBtn('players', 'Players')}
        ${this.#navBtn('teams', 'Teams')}
        ${this.#navBtn('seasons', 'Seasons')}
        ${this.#navBtn('handicap', 'Handicap')}
        ${usersBtn}
      </div>
    </div>`;
  }
}

customElements.define('league-admin-page', LeagueAdminPage);
