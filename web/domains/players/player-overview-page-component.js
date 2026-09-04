// <player-overview-page> - a read-only summary of one player's season --
// team, schedule, stats, current handicap, and season dues status (Player
// Overview Phase 2, backed by the finances domain added in Financial
// Phase 1). Presentation-only; all data comes from the backend's
// GET /api/players/{id}/overview aggregate.
//
// Public API:
//   refresh(allPlayers, activeSeason, preSelectPlayerId, lockedPlayerId)
//     Called by the app shell when the Player Overview section activates
//     or league/season context changes. preSelectPlayerId comes from
//     openPlayerOverview() cross-section navigation (the "View Overview"
//     button on the Players list) and is consumed once then cleared by
//     the shell, mirroring openMatchEntry's preselect pattern.
//     lockedPlayerId (Player Account Access Phase 1) is set only when the
//     viewer is a resolved role="player" identity viewing their own
//     overview via "My Overview" -- when set, the player-select dropdown
//     is hidden entirely and this is the only player ever loaded,
//     regardless of allPlayers/preSelectPlayerId (the backend also
//     enforces this; hiding the picker here is a UX courtesy, not the
//     access control). The locked load also omits season_id from the
//     overview request entirely, rather than passing activeSeason's id --
//     the shell's currently selected league/season may not be the linked
//     player's own league at all, so the backend is left to fall back to
//     that player's own league's active season. Admin loads (dropdown-
//     driven, including the Players-list "View Overview" preselect) are
//     unchanged and still pass activeSeason's id when present.

import { fetchPlayerOverview } from './players-api-service.js';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

function fmtHC(v) { return (v >= 0 ? '+' : '') + v; }

function fmtMoney(v) {
  const n = Number(v) || 0;
  return '$' + n.toFixed(2);
}

class PlayerOverviewPage extends HTMLElement {
  #allPlayers     = [];
  #activeSeason   = null;
  #lockedPlayerId = null;

  connectedCallback() {
    this.innerHTML = `
      <h4 class="mb-3 fw-bold">Player Overview</h4>
      <div class="row g-2 mb-3 align-items-end po-selector-row">
        <div class="col-auto">
          <label class="form-label small mb-1">Player</label>
          <select class="form-select form-select-sm po-player-sel"></select>
        </div>
      </div>
      <div class="po-body"></div>`;

    this.addEventListener('change', e => {
      if (e.target.matches('.po-player-sel')) this.#load();
    });
  }

  refresh(allPlayers, activeSeason, preSelectPlayerId = null, lockedPlayerId = null) {
    this.#allPlayers     = allPlayers   ?? [];
    this.#activeSeason   = activeSeason ?? null;
    this.#lockedPlayerId = lockedPlayerId;

    const selectorRow = this.querySelector('.po-selector-row');
    if (selectorRow) selectorRow.classList.toggle('d-none', lockedPlayerId != null);

    if (lockedPlayerId != null) {
      this.#load(lockedPlayerId);
    } else {
      this.#populateSelect(preSelectPlayerId);
      this.#load();
    }
  }

  // -- Private ------------------------------------------------------------------

  #populateSelect(preSelectPlayerId) {
    const sel = this.querySelector('.po-player-sel');
    if (!sel) return;
    const sorted = [...this.#allPlayers].sort((a, b) => a.name.localeCompare(b.name));
    sel.innerHTML = sorted.map(p =>
      `<option value="${p.id}">${esc(p.name)}${p.team_name ? ' - ' + esc(p.team_name) : ''}</option>`
    ).join('') || '<option value="">No players</option>';
    if (preSelectPlayerId != null) sel.value = String(preSelectPlayerId);
  }

  async #load(forcedPlayerId = null) {
    const playerId = forcedPlayerId ?? this.querySelector('.po-player-sel')?.value;
    const body = this.querySelector('.po-body');
    if (!playerId || !body) { if (body) body.innerHTML = ''; return; }

    // A locked (role=player, "My Overview") load must not depend on
    // whatever league/season the shell happens to have selected -- that
    // may not even be the linked player's own league. Omit season_id so
    // the backend falls back to the player's own league's active season.
    const seasonId = forcedPlayerId != null ? null : this.#activeSeason?.id;

    let overview;
    try {
      overview = await fetchPlayerOverview(playerId, seasonId);
    } catch (e) {
      body.innerHTML = `<div class="alert alert-danger">${esc(e.message)}</div>`;
      return;
    }
    body.innerHTML = this.#renderOverview(overview);
  }

  #renderOverview(overview) {
    const p = overview.player;
    const teamName = overview.team ? esc(overview.team.name) : '<span class="text-muted">No team</span>';

    let html = `<div class="card mb-3">
      <div class="card-body">
        <h5 class="mb-1">${esc(p.name)} <span class="text-muted small">#${esc(p.player_number || '')}</span></h5>
        <div class="text-muted small">
          ${teamName} &middot; ${esc(overview.season.name)}
        </div>
      </div>
    </div>`;

    html += `<div class="row g-3 mb-3">
      <div class="col-md-4">
        <div class="card h-100">
          <div class="card-header fw-semibold py-2">Current Handicap</div>
          <div class="card-body">
            <span class="badge bg-secondary fs-6">${fmtHC(overview.handicap.current)}</span>
          </div>
        </div>
      </div>
      <div class="col-md-8">
        <div class="card h-100">
          <div class="card-header fw-semibold py-2">Season Stats</div>
          <div class="card-body d-flex gap-4">
            <div><div class="text-muted small">Sets</div><div class="fw-bold">${overview.stats.sets_won}-${overview.stats.sets_lost}</div></div>
            <div><div class="text-muted small">Games</div><div class="fw-bold">${overview.stats.games_won}-${overview.stats.games_lost}</div></div>
            <div><div class="text-muted small">Win %</div><div class="fw-bold">${(overview.stats.win_pct * 100).toFixed(1)}%</div></div>
          </div>
        </div>
      </div>
    </div>`;

    html += `<div class="card mb-3">
      <div class="card-header fw-semibold py-2">Schedule</div>
      <div class="card-body p-0">
        <table class="table table-sm mb-0">
          <thead><tr><th>Week</th><th>Date</th><th>Opponent</th><th>Home/Away</th><th>Status</th></tr></thead>
          <tbody>
            ${overview.schedule.map(m => `<tr>
              <td>${m.week_number}</td>
              <td>${esc(m.match_date || 'TBD')}</td>
              <td>${esc(m.opponent_team_name)}</td>
              <td>${m.home_or_away === 'home' ? 'Home' : 'Away'}</td>
              <td>${m.completed
                ? '<span class="badge bg-success">Completed</span>'
                : '<span class="badge bg-secondary">Pending</span>'}</td>
            </tr>`).join('') ||
            '<tr><td colspan="5" class="text-center text-muted py-3">No scheduled matches this season</td></tr>'}
          </tbody>
        </table>
      </div>
    </div>`;

    html += this.#renderMoney(overview.money);

    return html;
  }

  #renderMoney(money) {
    if (!money.tracked) {
      return `<div class="alert alert-warning mb-0">
        <i class="bi bi-cash-coin me-1"></i>${esc(money.message)}
      </div>`;
    }

    const latest = money.payments && money.payments.length ? money.payments[0] : null;
    const duesAmountNote = money.dues_amount
      ? `<span class="text-muted small ms-2">Dues amount: ${esc(money.dues_amount)}</span>`
      : '';

    return `<div class="card mb-0">
      <div class="card-header fw-semibold py-2">Dues${duesAmountNote}</div>
      <div class="card-body d-flex align-items-center gap-4">
        ${money.paid
          ? '<span class="badge bg-success">Paid</span>'
          : '<span class="badge bg-secondary">Unpaid</span>'}
        <div><div class="text-muted small">Total Paid</div><div class="fw-bold">${fmtMoney(money.total_paid)}</div></div>
        <div><div class="text-muted small">Last Payment</div><div class="fw-bold">${latest ? esc(latest.paid_at) : '<span class="text-muted">None</span>'}</div></div>
      </div>
    </div>`;
  }
}

customElements.define('player-overview-page', PlayerOverviewPage);
