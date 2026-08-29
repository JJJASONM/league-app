// <finances-page> - Financial Phase 1: a league-admin screen for recording
// per-player season dues payments and per-team season payouts. All four
// backing routes require an Admin Key with league_admin/admin/system_admin
// role -- this screen has no fallback for viewers without one; a failed
// load shows the api() client's own 401/403 message inline.
//
// Payout amounts are always admin-entered; standings are shown for
// reference only and never used to compute a payout automatically.
//
// Public API:
//   refresh(allSeasons, activeSeason)
//     Called by the app shell when the Finances section activates or
//     league/season context changes.

import {
  fetchSeasonDues, recordDuesPayment,
  fetchSeasonPayouts, recordPayout,
} from './finances-api-service.js';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

function fmtMoney(v) {
  const n = Number(v) || 0;
  return '$' + n.toFixed(2);
}

const DUES_MODAL_ID   = 'finances-dues-modal';
const PAYOUT_MODAL_ID = 'finances-payout-modal';

class FinancesPage extends HTMLElement {
  #allSeasons     = [];
  #dues           = null;
  #payouts        = null;
  // Selected-record state for the two modals. Component fields, not hidden
  // form inputs -- the modal only ever displays a name that was set in the
  // same call that set the matching ID, so the displayed name and the
  // submitted ID cannot drift apart the way a separately-populated hidden
  // input could.
  #duesPlayerId   = null;
  #payoutTeamId   = null;

  connectedCallback() {
    this.innerHTML = `
      <h4 class="mb-3 fw-bold">Financial</h4>
      <div class="row g-2 mb-3 align-items-end">
        <div class="col-auto">
          <label class="form-label small mb-1">Season</label>
          <select class="form-select form-select-sm fin-season-sel"></select>
        </div>
      </div>
      <div class="fin-body"></div>`;

    this.#ensureDuesModal();
    this.#ensurePayoutModal();

    document.getElementById('fin-dues-save-btn')
      .addEventListener('click', () => this.#saveDuesPayment());
    document.getElementById('fin-payout-save-btn')
      .addEventListener('click', () => this.#savePayout());

    this.addEventListener('change', e => {
      if (e.target.matches('.fin-season-sel')) this.#load();
    });

    this.addEventListener('click', e => {
      const dueBtn = e.target.closest('[data-action="record-dues-payment"]');
      if (dueBtn) { this.#openDuesModal(parseInt(dueBtn.dataset.playerId, 10), dueBtn.dataset.playerName); return; }
      const payoutBtn = e.target.closest('[data-action="record-payout"]');
      if (payoutBtn) { this.#openPayoutModal(parseInt(payoutBtn.dataset.teamId, 10), payoutBtn.dataset.teamName); return; }
    });
  }

  refresh(allSeasons, activeSeason) {
    this.#allSeasons = allSeasons ?? [];
    this.#populateSeasonSelect(activeSeason);
    this.#load();
  }

  // -- Private ------------------------------------------------------------------

  #populateSeasonSelect(activeSeason) {
    const sel = this.querySelector('.fin-season-sel');
    if (!sel) return;
    sel.innerHTML = this.#allSeasons.map(s =>
      `<option value="${s.id}"${s.active ? ' selected' : ''}>${esc(s.name)}</option>`
    ).join('') || '<option value="">No seasons</option>';
    if (activeSeason) sel.value = String(activeSeason.id);
  }

  async #load() {
    const seasonId = this.querySelector('.fin-season-sel')?.value;
    const body = this.querySelector('.fin-body');
    if (!seasonId || !body) { if (body) body.innerHTML = ''; return; }

    try {
      [this.#dues, this.#payouts] = await Promise.all([
        fetchSeasonDues(seasonId),
        fetchSeasonPayouts(seasonId),
      ]);
    } catch (e) {
      body.innerHTML = `<div class="alert alert-danger">${esc(e.message)}</div>`;
      return;
    }
    body.innerHTML = this.#renderBody();
  }

  #renderBody() {
    return this.#renderDues() + this.#renderPayouts();
  }

  #renderDues() {
    const dues = this.#dues || { players: [] };
    const duesAmountNote = dues.dues_amount
      ? `<span class="text-muted small ms-2">Dues amount: ${esc(dues.dues_amount)}</span>`
      : '';

    const rows = (dues.players || []).map(p => {
      const latest = p.payments && p.payments.length ? p.payments[0] : null;
      return `<tr>
        <td>${esc(p.player_name)}</td>
        <td class="text-muted small">${esc(p.team_name || '')}</td>
        <td>${p.paid
          ? '<span class="badge bg-success">Paid</span>'
          : '<span class="badge bg-secondary">Unpaid</span>'}</td>
        <td class="text-end">${fmtMoney(p.total_paid)}</td>
        <td class="text-muted small">${latest ? esc(latest.paid_at) : ''}</td>
        <td class="text-end">
          <button class="btn btn-outline-primary btn-sm py-0" data-action="record-dues-payment"
            data-player-id="${p.player_id}" data-player-name="${esc(p.player_name)}">Record Payment</button>
        </td>
      </tr>`;
    }).join('') || '<tr><td colspan="6" class="text-center text-muted py-3">No rostered players this season</td></tr>';

    return `<div class="card mb-3">
      <div class="card-header fw-semibold py-2">Dues${duesAmountNote}</div>
      <div class="card-body p-0">
        <table class="table table-sm mb-0">
          <thead><tr><th>Player</th><th>Team</th><th>Status</th><th class="text-end">Total Paid</th><th>Last Payment</th><th></th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
    </div>`;
  }

  #renderPayouts() {
    const payouts = this.#payouts || { teams: [] };

    const rows = (payouts.teams || []).map(t => {
      const standing = t.standing
        ? `${t.standing.wins}-${t.standing.losses}${t.standing.ties ? '-' + t.standing.ties : ''}, ${t.standing.points} pts`
        : '<span class="text-muted">No standing yet</span>';
      return `<tr>
        <td>${esc(t.team_name)}</td>
        <td class="text-muted small">${standing}</td>
        <td class="text-end">${fmtMoney(t.total_paid)}</td>
        <td class="text-end">
          <button class="btn btn-outline-primary btn-sm py-0" data-action="record-payout"
            data-team-id="${t.team_id}" data-team-name="${esc(t.team_name)}">Record Payout</button>
        </td>
      </tr>`;
    }).join('') || '<tr><td colspan="4" class="text-center text-muted py-3">No season teams</td></tr>';

    return `<div class="card mb-3">
      <div class="card-header fw-semibold py-2">
        Payouts
        <span class="text-muted small ms-2">Standings shown for reference only -- amounts are admin-entered</span>
      </div>
      <div class="card-body p-0">
        <table class="table table-sm mb-0">
          <thead><tr><th>Team</th><th>Standing</th><th class="text-end">Total Paid</th><th></th></tr></thead>
          <tbody>${rows}</tbody>
        </table>
      </div>
    </div>`;
  }

  #ensureDuesModal() {
    if (document.getElementById(DUES_MODAL_ID)) return;
    const el = document.createElement('div');
    el.innerHTML = `
<div class="modal fade" id="${DUES_MODAL_ID}" tabindex="-1">
  <div class="modal-dialog">
    <div class="modal-content">
      <div class="modal-header">
        <h5 class="modal-title">Record Dues Payment</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
      </div>
      <div class="modal-body">
        <p class="mb-3">Player: <strong id="fin-dues-player-name"></strong></p>
        <div class="mb-3">
          <label class="form-label">Amount *</label>
          <input type="number" class="form-control" id="fin-dues-amount" min="0.01" step="0.01">
        </div>
        <div class="mb-3">
          <label class="form-label">Paid Date *</label>
          <input type="date" class="form-control" id="fin-dues-paid-at">
        </div>
        <div class="mb-1">
          <label class="form-label">Note</label>
          <input type="text" class="form-control" id="fin-dues-note">
        </div>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
        <button type="button" class="btn btn-primary" id="fin-dues-save-btn">Record Payment</button>
      </div>
    </div>
  </div>
</div>`;
    document.body.appendChild(el.firstElementChild);
  }

  #ensurePayoutModal() {
    if (document.getElementById(PAYOUT_MODAL_ID)) return;
    const el = document.createElement('div');
    el.innerHTML = `
<div class="modal fade" id="${PAYOUT_MODAL_ID}" tabindex="-1">
  <div class="modal-dialog">
    <div class="modal-content">
      <div class="modal-header">
        <h5 class="modal-title">Record Payout</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
      </div>
      <div class="modal-body">
        <p class="mb-3">Team: <strong id="fin-payout-team-name"></strong></p>
        <div class="mb-3">
          <label class="form-label">Amount *</label>
          <input type="number" class="form-control" id="fin-payout-amount" min="0.01" step="0.01">
        </div>
        <div class="mb-1">
          <label class="form-label">Note</label>
          <input type="text" class="form-control" id="fin-payout-note" placeholder="e.g. 1st place">
        </div>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
        <button type="button" class="btn btn-primary" id="fin-payout-save-btn">Record Payout</button>
      </div>
    </div>
  </div>
</div>`;
    document.body.appendChild(el.firstElementChild);
  }

  #openDuesModal(playerId, playerName) {
    this.#duesPlayerId = playerId;
    document.getElementById('fin-dues-player-name').textContent = playerName;
    document.getElementById('fin-dues-amount').value = '';
    document.getElementById('fin-dues-paid-at').value = new Date().toISOString().slice(0, 10);
    document.getElementById('fin-dues-note').value = '';
    new bootstrap.Modal(document.getElementById(DUES_MODAL_ID)).show();
  }

  #openPayoutModal(teamId, teamName) {
    this.#payoutTeamId = teamId;
    document.getElementById('fin-payout-team-name').textContent = teamName;
    document.getElementById('fin-payout-amount').value = '';
    document.getElementById('fin-payout-note').value = '';
    new bootstrap.Modal(document.getElementById(PAYOUT_MODAL_ID)).show();
  }

  async #saveDuesPayment() {
    const seasonId  = this.querySelector('.fin-season-sel')?.value;
    const playerId  = this.#duesPlayerId;
    const amount    = parseFloat(document.getElementById('fin-dues-amount').value);
    const paidAt    = document.getElementById('fin-dues-paid-at').value;
    const note      = document.getElementById('fin-dues-note').value.trim();
    if (!playerId) { toast('No player selected', 'danger'); return; }
    if (!amount || amount <= 0) { toast('Enter an amount greater than zero', 'warning'); return; }
    if (!paidAt) { toast('Enter a paid date', 'warning'); return; }
    try {
      await recordDuesPayment(seasonId, { player_id: playerId, amount, paid_at: paidAt, note });
      bootstrap.Modal.getInstance(document.getElementById(DUES_MODAL_ID))?.hide();
      toast('Payment recorded');
      await this.#load();
    } catch (e) {
      toast(e.message, 'danger');
    }
  }

  async #savePayout() {
    const seasonId = this.querySelector('.fin-season-sel')?.value;
    const teamId   = this.#payoutTeamId;
    const amount   = parseFloat(document.getElementById('fin-payout-amount').value);
    const note     = document.getElementById('fin-payout-note').value.trim();
    if (!teamId) { toast('No team selected', 'danger'); return; }
    if (!amount || amount <= 0) { toast('Enter an amount greater than zero', 'warning'); return; }
    try {
      await recordPayout(seasonId, { team_id: teamId, amount, note });
      bootstrap.Modal.getInstance(document.getElementById(PAYOUT_MODAL_ID))?.hide();
      toast('Payout recorded');
      await this.#load();
    } catch (e) {
      toast(e.message, 'danger');
    }
  }
}

customElements.define('finances-page', FinancesPage);
