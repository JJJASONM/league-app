// <players-page> component.
//
// Public API:
//   refresh(allTeams, activeLeague)
//     Called by the shell when the Players section activates or league context changes.
//     Fetches and renders the current player list.
//
// Custom events dispatched (bubbles: true):
//   players-data-changed  { detail: { players } }
//     Fired after every successful load, save, or delete so the shell can
//     keep its allPlayers global in sync.

import { fetchPlayers, createPlayer, updatePlayer, removePlayer } from './players-api-service.js';
import { GAME_FORMAT_9BALL } from '../leagues/game-format-codes.js';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

const MODAL_ID    = 'player-modal';
const QA_MODAL_ID = 'player-quick-add-modal';
const DASH = '\u2014';

class PlayersPage extends HTMLElement {
  #allPlayers   = [];
  #allTeams     = [];
  #activeLeague = null;

  connectedCallback() {
    this.innerHTML = `
      <div class="d-flex justify-content-between align-items-center mb-3">
        <h4 class="mb-0 fw-bold">Players</h4>
        <div class="d-flex gap-2">
          <button class="btn btn-primary btn-sm" data-action="add-player">
            <i class="bi bi-plus-lg"></i> Add Player
          </button>
          <button class="btn btn-outline-primary btn-sm" data-action="quick-add-player">
            <i class="bi bi-lightning-fill"></i> Quick Add
          </button>
        </div>
      </div>
      <div class="card">
        <div class="card-body p-0">
          <table class="table table-hover mb-0">
            <thead><tr>
              <th>#</th><th>Last Name</th><th>First Name</th><th>Diff</th>
              <th>Team</th><th>Phone</th><th></th>
            </tr></thead>
            <tbody class="pl-tbody"></tbody>
          </table>
        </div>
      </div>`;

    this.#ensureModal();
    this.#ensureQuickAddModal();

    document.getElementById('player-save-btn')
      .addEventListener('click', () => this.#savePlayer());
    document.getElementById('player-qa-save-btn')
      .addEventListener('click', () => this.#saveQuickAdd());

    this.addEventListener('click', e => {
      if (e.target.closest('[data-action="add-player"]'))       { this.#openNewPlayer(); return; }
      if (e.target.closest('[data-action="quick-add-player"]')) { this.#openQuickAdd();  return; }
      const editBtn = e.target.closest('[data-action="edit-player"]');
      if (editBtn) { this.#editPlayer(parseInt(editBtn.dataset.playerId, 10)); return; }
      const delBtn = e.target.closest('[data-action="delete-player"]');
      if (delBtn)  { this.#deletePlayer(parseInt(delBtn.dataset.playerId, 10)); return; }
    });
  }

  refresh(allTeams, activeLeague) {
    this.#allTeams    = allTeams    ?? [];
    this.#activeLeague = activeLeague;
    this.#load();
  }

  // -- Private ------------------------------------------------------------------

  #ensureModal() {
    if (document.getElementById(MODAL_ID)) return;
    const el = document.createElement('div');
    el.innerHTML = `
<div class="modal fade" id="player-modal" tabindex="-1">
  <div class="modal-dialog">
    <div class="modal-content">
      <div class="modal-header">
        <h5 class="modal-title" id="player-modal-title">Add Player</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
      </div>
      <div class="modal-body">
        <input type="hidden" id="player-id">
        <div class="mb-3">
          <label class="form-label">Player # <span class="text-muted small">(e.g. 42 &mdash; locked once saved)</span></label>
          <div class="input-group">
            <input type="text" class="form-control" id="player-number" maxlength="4" placeholder="42">
            <span class="input-group-text d-none" id="player-number-lock" title="Player number is locked">
              <i class="bi bi-lock-fill text-secondary"></i>
            </span>
          </div>
        </div>
        <div class="row g-2 mb-3">
          <div class="col">
            <label class="form-label">First Name *</label>
            <input type="text" class="form-control" id="player-first-name">
          </div>
          <div class="col">
            <label class="form-label">Last Name *</label>
            <input type="text" class="form-control" id="player-last-name">
          </div>
        </div>
        <div class="row g-2 mb-3">
          <div class="col">
            <label class="form-label">Phone</label>
            <input type="tel" class="form-control" id="player-phone" placeholder="(555) 555-5555">
          </div>
          <div class="col">
            <label class="form-label">Email</label>
            <input type="email" class="form-control" id="player-email" placeholder="player@example.com">
          </div>
        </div>
        <div class="mb-3">
          <label class="form-label">Diff Rating</label>
          <input type="number" class="form-control" id="player-handicap" value="0" step="0.01">
          <div class="form-text">8-ball: (games won - games lost) / matches played. 9-ball: race-to number (e.g. 5, 7).</div>
        </div>
        <div class="mb-3 d-none" id="admin-hold-row">
          <div class="form-check">
            <input class="form-check-input" type="checkbox" id="player-admin-hold">
            <label class="form-check-label" for="player-admin-hold">
              Admin Hold <span class="text-muted small">(handicap locked at Administrative Discretion)</span>
            </label>
          </div>
        </div>
        <div class="mb-1">
          <label class="form-label">Team</label>
          <select class="form-select" id="player-team"></select>
        </div>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
        <button type="button" class="btn btn-primary" id="player-save-btn">Save</button>
      </div>
    </div>
  </div>
</div>`;
    document.body.appendChild(el.firstElementChild);
  }

  async #load() {
    if (!this.#activeLeague) return;
    try {
      this.#allPlayers = await fetchPlayers(this.#activeLeague.id);
      this.#renderList();
      this.#dispatchDataChanged();
    } catch (e) {
      toast(e.message, 'danger');
    }
  }

  #renderList() {
    const tbody = this.querySelector('.pl-tbody');
    if (!tbody) return;
    tbody.innerHTML = this.#allPlayers.map(p => `
      <tr>
        <td class="text-muted small">${esc(String(p.player_number || DASH))}</td>
        <td class="fw-semibold">${esc(p.last_name)}${p.admin_hold
          ? ' <span class="badge bg-warning text-dark ms-1" style="font-size:.65rem">Hold</span>'
          : ''}</td>
        <td>${esc(p.first_name)}</td>
        <td><span class="badge bg-secondary badge-hc">${p.handicap}</span></td>
        <td>${p.team_name ? esc(p.team_name) : `<span class="text-muted">${DASH}</span>`}</td>
        <td class="text-muted small">${esc(String(p.phone || DASH))}</td>
        <td class="text-end">
          <button class="btn btn-outline-secondary btn-sm py-0 me-1"
            data-action="edit-player" data-player-id="${p.id}"><i class="bi bi-pencil"></i></button>
          <button class="btn btn-outline-danger btn-sm py-0"
            data-action="delete-player" data-player-id="${p.id}"><i class="bi bi-trash"></i></button>
        </td>
      </tr>`).join('') ||
      '<tr><td colspan="7" class="text-center text-muted py-3">No players yet</td></tr>';
  }

  #openNewPlayer() {
    document.getElementById('player-modal-title').textContent = 'Add Player';
    document.getElementById('player-id').value          = '';
    document.getElementById('player-number').value      = '';
    document.getElementById('player-first-name').value  = '';
    document.getElementById('player-last-name').value   = '';
    document.getElementById('player-phone').value       = '';
    document.getElementById('player-email').value       = '';
    document.getElementById('player-handicap').value    = '0';
    document.getElementById('player-admin-hold').checked = false;
    this.#setModalMode(false);
    this.#populateTeamDropdown(null);
    new bootstrap.Modal(document.getElementById(MODAL_ID)).show();
  }

  #editPlayer(id) {
    const p = this.#allPlayers.find(x => x.id === id);
    if (!p) return;
    document.getElementById('player-modal-title').textContent = 'Edit Player';
    document.getElementById('player-id').value          = p.id;
    document.getElementById('player-number').value      = p.player_number || '';
    document.getElementById('player-first-name').value  = p.first_name || '';
    document.getElementById('player-last-name').value   = p.last_name || '';
    document.getElementById('player-phone').value       = p.phone || '';
    document.getElementById('player-email').value       = p.email || '';
    document.getElementById('player-handicap').value    = p.handicap;
    document.getElementById('player-admin-hold').checked = !!p.admin_hold;
    this.#setModalMode(true);
    this.#populateTeamDropdown(p.team_id);
    new bootstrap.Modal(document.getElementById(MODAL_ID)).show();
  }

  #setModalMode(isEdit) {
    const numInput = document.getElementById('player-number');
    const lockIcon = document.getElementById('player-number-lock');
    numInput.readOnly = isEdit;
    numInput.classList.toggle('bg-light', isEdit);
    lockIcon.classList.toggle('d-none', !isEdit);
    const is9ball = this.#activeLeague?.game_format === GAME_FORMAT_9BALL;
    document.getElementById('admin-hold-row').classList.toggle('d-none', !is9ball);
  }

  #populateTeamDropdown(selectedId) {
    document.getElementById('player-team').innerHTML =
      `<option value="">${DASH} No Team ${DASH}</option>` +
      this.#allTeams.map(t =>
        `<option value="${t.id}" ${t.id == selectedId ? 'selected' : ''}>${esc(t.name)}</option>`
      ).join('');
  }

  async #savePlayer() {
    const id        = document.getElementById('player-id').value;
    const teamVal   = document.getElementById('player-team').value;
    const firstName = document.getElementById('player-first-name').value.trim();
    const lastName  = document.getElementById('player-last-name').value.trim();
    if (!firstName && !lastName) { toast('First or last name is required', 'warning'); return; }
    const body = {
      first_name: firstName,
      last_name:  lastName,
      phone:      document.getElementById('player-phone').value.trim(),
      email:      document.getElementById('player-email').value.trim(),
      handicap:   parseFloat(document.getElementById('player-handicap').value) || 0,
      admin_hold: document.getElementById('player-admin-hold').checked,
      team_id:    teamVal ? parseInt(teamVal) : null,
      league_id:  this.#activeLeague?.id,
    };
    if (!id) body.player_number = document.getElementById('player-number').value.trim();
    try {
      if (id) await updatePlayer(id, body);
      else    await createPlayer(body);
      bootstrap.Modal.getInstance(document.getElementById(MODAL_ID))?.hide();
      toast('Player saved');
      this.#allPlayers = await fetchPlayers(this.#activeLeague.id);
      this.#renderList();
      this.#dispatchDataChanged();
    } catch (e) {
      toast(e.message, 'danger');
    }
  }

  async #deletePlayer(id) {
    if (!confirm('Remove this player?')) return;
    try {
      await removePlayer(id);
      toast('Deleted');
      this.#allPlayers = await fetchPlayers(this.#activeLeague.id);
      this.#renderList();
      this.#dispatchDataChanged();
    } catch (e) {
      toast(e.message, 'danger');
    }
  }

  #dispatchDataChanged() {
    this.dispatchEvent(new CustomEvent('players-data-changed', {
      bubbles: true,
      detail:  { players: this.#allPlayers },
    }));
  }

  #ensureQuickAddModal() {
    if (document.getElementById(QA_MODAL_ID)) return;
    const el = document.createElement('div');
    el.innerHTML = `
<div class="modal fade" id="player-quick-add-modal" tabindex="-1">
  <div class="modal-dialog">
    <div class="modal-content">
      <div class="modal-header">
        <h5 class="modal-title">Quick Add Player</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
      </div>
      <div class="modal-body">
        <div class="row g-2 mb-3">
          <div class="col">
            <label class="form-label">First Name</label>
            <input type="text" class="form-control" id="player-qa-first-name">
          </div>
          <div class="col">
            <label class="form-label">Last Name</label>
            <input type="text" class="form-control" id="player-qa-last-name">
          </div>
        </div>
        <div class="mb-3">
          <label class="form-label">Diff Rating</label>
          <input type="number" class="form-control" id="player-qa-handicap" value="0" step="0.01">
        </div>
        <div class="mb-1">
          <label class="form-label">Team</label>
          <select class="form-select" id="player-qa-team"></select>
        </div>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
        <button type="button" class="btn btn-primary" id="player-qa-save-btn">Add Player</button>
      </div>
    </div>
  </div>
</div>`;
    document.body.appendChild(el.firstElementChild);
  }

  #openQuickAdd() {
    document.getElementById('player-qa-first-name').value = '';
    document.getElementById('player-qa-last-name').value  = '';
    document.getElementById('player-qa-handicap').value   = '0';
    document.getElementById('player-qa-team').innerHTML =
      `<option value="">${DASH} No Team ${DASH}</option>` +
      this.#allTeams.map(t =>
        `<option value="${t.id}">${esc(t.name)}</option>`
      ).join('');
    new bootstrap.Modal(document.getElementById(QA_MODAL_ID)).show();
  }

  async #saveQuickAdd() {
    const firstName = document.getElementById('player-qa-first-name').value.trim();
    const lastName  = document.getElementById('player-qa-last-name').value.trim();
    if (!firstName && !lastName) { toast('First or last name is required', 'warning'); return; }
    const teamVal = document.getElementById('player-qa-team').value;
    const body = {
      first_name: firstName,
      last_name:  lastName,
      handicap:   parseFloat(document.getElementById('player-qa-handicap').value) || 0,
      team_id:    teamVal ? parseInt(teamVal) : null,
      league_id:  this.#activeLeague?.id,
    };
    try {
      await createPlayer(body);
      bootstrap.Modal.getInstance(document.getElementById(QA_MODAL_ID))?.hide();
      toast('Player added');
      this.#allPlayers = await fetchPlayers(this.#activeLeague.id);
      this.#renderList();
      this.#dispatchDataChanged();
    } catch (e) {
      toast(e.message, 'danger');
    }
  }
}

customElements.define('players-page', PlayersPage);
