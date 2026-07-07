// <standings-section> and <stats-section> components.
//
// Public API (both elements):
//   refresh(allSeasons)
//     Called by the shell when the section activates or league context changes.
//     Populates the season selector and loads data.
//   reload()
//     Called by the shell after schedule close/reopen to refresh data for the
//     currently selected season without re-populating the selector.

import { fetchStandings, fetchPlayerStats } from './standings-api-service.js';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

class StandingsSection extends HTMLElement {
  #allSeasons = [];

  connectedCallback() {
    this.innerHTML = `
      <div class="d-flex justify-content-between align-items-center mb-3">
        <h4 class="mb-0 fw-bold">Standings</h4>
        <select class="form-select form-select-sm w-auto st-season-sel"></select>
      </div>
      <div class="card">
        <div class="card-body p-0">
          <table class="table table-hover mb-0">
            <thead><tr>
              <th>#</th><th>Team</th><th>P</th><th>W</th><th>L</th><th>T</th>
              <th>Pts</th><th>GW</th><th>GL</th><th>Win%</th>
            </tr></thead>
            <tbody class="st-tbody">
              <tr><td colspan="10" class="text-center text-muted py-3">Select a season above.</td></tr>
            </tbody>
          </table>
        </div>
      </div>`;

    this.querySelector('.st-season-sel').addEventListener('change', () => this.#load());
  }

  refresh(allSeasons) {
    this.#allSeasons = allSeasons ?? [];
    this.#populateSelect();
    this.#load();
  }

  reload() {
    this.#load();
  }

  #populateSelect() {
    const sel = this.querySelector('.st-season-sel');
    if (!sel) return;
    sel.innerHTML = this.#allSeasons.map(s =>
      `<option value="${s.id}" ${s.active ? 'selected' : ''}>${esc(s.name)}</option>`
    ).join('') || '<option value="">No seasons</option>';
  }

  async #load() {
    const seasonId = this.querySelector('.st-season-sel')?.value;
    const tbody = this.querySelector('.st-tbody');
    if (!tbody) return;
    if (!seasonId) {
      tbody.innerHTML = '<tr><td colspan="10" class="text-center text-muted py-3">Select a season above.</td></tr>';
      return;
    }
    try {
      const standings = await fetchStandings(seasonId);
      tbody.innerHTML = standings.map((s, i) => `
        <tr ${i === 0 ? 'class="table-warning fw-bold"' : ''}>
          <td>${i + 1}</td>
          <td>${esc(s.team_name)}</td>
          <td>${s.played}</td>
          <td>${s.wins}</td>
          <td>${s.losses}</td>
          <td>${s.ties}</td>
          <td class="fw-bold">${s.points}</td>
          <td>${s.games_won}</td>
          <td>${s.games_lost}</td>
          <td>${(s.win_pct * 100).toFixed(1)}%</td>
        </tr>`).join('') ||
        '<tr><td colspan="10" class="text-center text-muted py-3">No completed matches yet</td></tr>';
    } catch (e) {
      tbody.innerHTML = `<tr><td colspan="10" class="text-danger text-center py-3">${esc(e.message)}</td></tr>`;
    }
  }
}

class StatsSection extends HTMLElement {
  #allSeasons = [];

  connectedCallback() {
    this.innerHTML = `
      <div class="d-flex justify-content-between align-items-center mb-3">
        <h4 class="mb-0 fw-bold">Player Stats</h4>
        <select class="form-select form-select-sm w-auto pst-season-sel"></select>
      </div>
      <div class="card">
        <div class="card-body p-0">
          <table class="table table-hover mb-0">
            <thead><tr>
              <th>#</th><th>Player</th><th>Team</th><th>Handicap</th>
              <th>SW</th><th>SL</th><th>GW</th><th>GL</th><th>Win%</th>
            </tr></thead>
            <tbody class="pst-tbody">
              <tr><td colspan="9" class="text-center text-muted py-3">Select a season above.</td></tr>
            </tbody>
          </table>
        </div>
      </div>`;

    this.querySelector('.pst-season-sel').addEventListener('change', () => this.#load());
  }

  refresh(allSeasons) {
    this.#allSeasons = allSeasons ?? [];
    this.#populateSelect();
    this.#load();
  }

  reload() {
    this.#load();
  }

  #populateSelect() {
    const sel = this.querySelector('.pst-season-sel');
    if (!sel) return;
    sel.innerHTML = this.#allSeasons.map(s =>
      `<option value="${s.id}" ${s.active ? 'selected' : ''}>${esc(s.name)}</option>`
    ).join('') || '<option value="">No seasons</option>';
  }

  async #load() {
    const seasonId = this.querySelector('.pst-season-sel')?.value;
    const tbody = this.querySelector('.pst-tbody');
    if (!tbody) return;
    if (!seasonId) {
      tbody.innerHTML = '<tr><td colspan="9" class="text-center text-muted py-3">Select a season above.</td></tr>';
      return;
    }
    try {
      const stats = await fetchPlayerStats(seasonId);
      tbody.innerHTML = stats.map(s => `
        <tr>
          <td class="text-muted small">${esc(String(s.player_number || '\u2014'))}</td>
          <td>${esc(s.player_name)}</td>
          <td>${esc(s.team_name)}</td>
          <td><span class="badge bg-secondary badge-hc">${s.handicap}</span></td>
          <td>${s.sets_won}</td>
          <td>${s.sets_lost}</td>
          <td>${s.games_won}</td>
          <td>${s.games_lost}</td>
          <td>${(s.win_pct * 100).toFixed(1)}%</td>
        </tr>`).join('') ||
        '<tr><td colspan="9" class="text-center text-muted py-3">No stats yet</td></tr>';
    } catch (e) {
      tbody.innerHTML = `<tr><td colspan="9" class="text-danger text-center py-3">${esc(e.message)}</td></tr>`;
    }
  }
}

customElements.define('standings-section', StandingsSection);
customElements.define('stats-section', StatsSection);
