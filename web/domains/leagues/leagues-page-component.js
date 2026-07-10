// <leagues-page> component.
//
// Non-visual component that owns the Manage Leagues modal workflow.
//
// Public API:
//   openModal(activeLeague)
//     Called by the shell (via openLeagueModal()) when the user clicks
//     "Manage Leagues". Stores the active league for table highlighting,
//     ensures the modal is injected, shows it, and loads fresh data.
//
// Custom events dispatched (bubbles: true):
//   leagues-list-changed  { detail: { leagues, deletedId } }
//     Fired after a league is added (deletedId: null) or deleted (deletedId: id).
//     Shell updates allLeagues, rebuilds the league selector, and reloads context.

import { fetchAllLeagues, fetchLeagueTeams, createLeague, removeLeague } from './leagues-api-service.js';
import {
  GAME_FORMAT_8BALL,
  GAME_FORMAT_9BALL,
  GAME_FORMAT_10BALL,
  GAME_FORMAT_STRAIGHT,
} from './game-format-codes.js';

const MODAL_ID = 'leagues-manage-modal';

class LeaguesPage extends HTMLElement {
  #activeLeague = null;
  #leagues      = [];

  connectedCallback() {
    // Non-visual: no innerHTML. Modal is injected into document.body on first open.
  }

  async openModal(activeLeague) {
    this.#activeLeague = activeLeague ?? null;
    this.#ensureModal();
    bootstrap.Modal.getOrCreateInstance(document.getElementById(MODAL_ID)).show();
    await this.#refreshTable();
  }

  // -- Private ------------------------------------------------------------------

  #ensureModal() {
    if (document.getElementById(MODAL_ID)) return;

    const div = document.createElement('div');
    div.innerHTML = `
<div class="modal fade" id="${MODAL_ID}" tabindex="-1">
  <div class="modal-dialog modal-lg">
    <div class="modal-content">
      <div class="modal-header">
        <h5 class="modal-title">Manage Leagues</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
      </div>
      <div class="modal-body">
        <div class="mb-3 border-bottom pb-3">
          <h6 class="fw-semibold">Add New League</h6>
          <div class="row g-2">
            <div class="col-md-4">
              <label class="form-label small mb-1">League Name *</label>
              <input type="text" class="form-control form-control-sm" id="new-league-name" placeholder="e.g. Monday 8-Ball">
            </div>
            <div class="col-md-3">
              <label class="form-label small mb-1">Format</label>
              <select class="form-select form-select-sm" id="new-league-format">
                <option value="${GAME_FORMAT_8BALL}">8-Ball</option>
                <option value="${GAME_FORMAT_9BALL}">9-Ball</option>
                <option value="${GAME_FORMAT_10BALL}">10-Ball</option>
                <option value="${GAME_FORMAT_STRAIGHT}">Straight Pool</option>
              </select>
            </div>
            <div class="col-md-3">
              <label class="form-label small mb-1">Day of Week</label>
              <select class="form-select form-select-sm" id="new-league-day">
                <option value="">\u2014</option>
                <option value="Monday">Monday</option>
                <option value="Tuesday">Tuesday</option>
                <option value="Wednesday">Wednesday</option>
                <option value="Thursday">Thursday</option>
                <option value="Friday">Friday</option>
                <option value="Saturday">Saturday</option>
                <option value="Sunday">Sunday</option>
              </select>
            </div>
            <div class="col-md-2 d-flex align-items-end">
              <button class="btn btn-primary btn-sm w-100" data-action="add-league">
                <i class="bi bi-plus-lg"></i> Add
              </button>
            </div>
          </div>
        </div>
        <h6 class="fw-semibold mb-2">Existing Leagues</h6>
        <table class="table table-sm table-hover" id="leagues-table">
          <thead><tr><th>Name</th><th>Format</th><th>Day</th><th>Teams</th><th></th></tr></thead>
          <tbody></tbody>
        </table>
        <h6 class="fw-semibold mb-2 mt-3"><i class="bi bi-clipboard-check"></i> Setup Checklist</h6>
        <div id="league-checklist" class="text-muted small">Loading\u2026</div>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Close</button>
      </div>
    </div>
  </div>
</div>`;

    const modalEl = div.firstElementChild;
    document.body.appendChild(modalEl);

    modalEl.addEventListener('click', e => {
      if (e.target.closest('[data-action="add-league"]')) {
        this.#addLeague();
        return;
      }
      const delBtn = e.target.closest('[data-delete-league]');
      if (delBtn) this.#deleteLeague(parseInt(delBtn.dataset.deleteLeague, 10));
    });
  }

  async #refreshTable() {
    const tbody     = document.querySelector(`#${MODAL_ID} #leagues-table tbody`);
    const checklist = document.getElementById('league-checklist');
    if (tbody)     tbody.innerHTML      = '<tr><td colspan="5" class="text-muted text-center py-2">Loading\u2026</td></tr>';
    if (checklist) checklist.textContent = 'Loading\u2026';

    let leagues = [];
    try {
      leagues = await fetchAllLeagues();
    } catch (e) {
      toast('Failed to load leagues: ' + e.message, 'danger');
      return;
    }

    this.#leagues = leagues;

    const counts = {};
    await Promise.all(leagues.map(async l => {
      try {
        const ts = await fetchLeagueTeams(l.id);
        counts[l.id] = ts.length;
      } catch (_) { counts[l.id] = 0; }
    }));

    this.#renderTable(leagues, counts);
  }

  #renderTable(leagues, counts) {
    const formatLabel = { '8ball': '8-Ball', '9ball': '9-Ball', '10ball': '10-Ball', 'straight': 'Straight' };
    const tbody = document.querySelector(`#${MODAL_ID} #leagues-table tbody`);
    if (tbody) {
      tbody.innerHTML = leagues.map(l => {
        const n = counts[l.id] ?? '\u2014';
        const teamOk = typeof n === 'number' && n >= 2;
        const teamBadge = typeof n === 'number'
          ? `<span class="badge ${teamOk ? 'bg-success' : 'bg-warning text-dark'}" style="font-size:.7rem">
              ${teamOk ? '<i class="bi bi-check-lg"></i> ' : '<i class="bi bi-exclamation-triangle"></i> '}${n} team${n !== 1 ? 's' : ''}
            </span>`
          : '<span class="text-muted small">\u2014</span>';
        const isActive = this.#activeLeague && l.id === this.#activeLeague.id;
        return `<tr${isActive ? ' class="table-primary"' : ''}>
          <td class="fw-semibold">${l.name}</td>
          <td>${formatLabel[l.game_format] || l.game_format}</td>
          <td>${l.day_of_week || '\u2014'}</td>
          <td>${teamBadge}</td>
          <td class="text-end">
            <button class="btn btn-outline-danger btn-sm py-0" data-delete-league="${l.id}">
              <i class="bi bi-trash"></i>
            </button>
          </td>
        </tr>`;
      }).join('') || '<tr><td colspan="5" class="text-center text-muted py-2">No leagues yet</td></tr>';
    }

    const checklist = document.getElementById('league-checklist');
    if (checklist) {
      checklist.innerHTML = leagues.map(l => {
        const n = counts[l.id] ?? 0;
        const needsOdd = n > 0 && n % 2 === 1;
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
  }

  async #addLeague() {
    const nameEl   = document.getElementById('new-league-name');
    const formatEl = document.getElementById('new-league-format');
    const dayEl    = document.getElementById('new-league-day');
    const name     = nameEl?.value.trim() ?? '';
    const format   = formatEl?.value ?? GAME_FORMAT_8BALL;
    const day      = dayEl?.value ?? '';
    if (!name) { toast('League name is required', 'warning'); return; }
    try {
      await createLeague(name, format, day);
      toast('League added');
      if (nameEl) nameEl.value = '';
      await this.#refreshTable();
      this.dispatchEvent(new CustomEvent('leagues-list-changed', {
        bubbles: true,
        detail: { leagues: this.#leagues, deletedId: null },
      }));
    } catch (e) { toast(e.message, 'danger'); }
  }

  async #deleteLeague(id) {
    if (!confirm('Delete this league and ALL its teams, seasons and matches? This cannot be undone.')) return;
    try {
      await removeLeague(id);
      toast('League deleted');
      await this.#refreshTable();
      this.dispatchEvent(new CustomEvent('leagues-list-changed', {
        bubbles: true,
        detail: { leagues: this.#leagues, deletedId: id },
      }));
    } catch (e) { toast(e.message, 'danger'); }
  }
}

customElements.define('leagues-page', LeaguesPage);
