// <lineup-page> - Lineup Planning section coordinator.
//
// Public API:
//   refresh(allSeasons, activeSeason, allTeams, allPlayers)
//     Called by the app shell when the Lineup tab activates or when
//     league/season context changes. Repopulates selectors and loads cards.

import { fetchMatches, fetchLineupPlans, saveLineupPlans } from './lineup-api-service.js';
import { fmtDate } from '../../components/date-display.js';

function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

class LineupPage extends HTMLElement {
  #allTeams   = [];
  #allPlayers = [];

  connectedCallback() {
    this.innerHTML = `
      <div class="d-flex justify-content-between align-items-center mb-2">
        <h4 class="mb-0 fw-bold">Lineup Planning</h4>
        <div class="d-flex gap-2 align-items-center">
          <select class="form-select form-select-sm w-auto lp-season-sel"></select>
          <select class="form-select form-select-sm lp-week-sel" style="min-width:170px"></select>
        </div>
      </div>
      <p class="text-muted small mb-3">Set a <strong>Default Lineup</strong> once per team, then override specific weeks as needed. Lineups load automatically in Match Entry.</p>
      <div class="row g-3 lp-cards"></div>`;

    this.addEventListener('change', e => {
      if (e.target.matches('.lp-season-sel')) this.#loadWeeks();
      if (e.target.matches('.lp-week-sel'))   this.#loadCards();
    });
    this.addEventListener('click', e => {
      const btn = e.target.closest('[data-action="save-lineup"]');
      if (btn) this.#onSaveClick(btn);
    });
  }

  refresh(allSeasons, activeSeason, allTeams, allPlayers) {
    this.#allTeams   = allTeams   ?? [];
    this.#allPlayers = allPlayers ?? [];
    this.#populateSeasonSelect(allSeasons ?? []);
    this.#loadWeeks();
  }

  #populateSeasonSelect(allSeasons) {
    const sel = this.querySelector('.lp-season-sel');
    if (!sel) return;
    sel.innerHTML = allSeasons.map(s =>
      `<option value="${s.id}"${s.active ? ' selected' : ''}>${esc(s.name)}</option>`
    ).join('') || '<option value="">No seasons</option>';
  }

  async #loadWeeks() {
    const seasonId = this.querySelector('.lp-season-sel')?.value;
    if (!seasonId) return;

    const weekSel = this.querySelector('.lp-week-sel');
    weekSel.innerHTML = '<option>Loading...</option>';

    let matches;
    try { matches = await fetchMatches(seasonId); } catch (e) { toast(e.message, 'danger'); return; }

    const weeks = {};
    matches.forEach(m => { if (!weeks[m.week_number]) weeks[m.week_number] = m.match_date || 'TBD'; });
    const sorted = Object.keys(weeks).sort((a, b) => parseInt(a) - parseInt(b));

    weekSel.innerHTML =
      '<option value="0">Default Lineup</option>' +
      (sorted.length
        ? sorted.map(w => `<option value="${w}">Week ${w} - ${fmtDate(weeks[w])}</option>`).join('')
        : '');

    await this.#loadCards();
  }

  async #loadCards() {
    const seasonId = this.querySelector('.lp-season-sel')?.value;
    const weekNum  = parseInt(this.querySelector('.lp-week-sel')?.value || '0');
    if (!seasonId) return;

    const container = this.querySelector('.lp-cards');
    container.innerHTML = '<div class="col text-muted py-3"><i class="bi bi-hourglass-split"></i> Loading...</div>';

    let plans;
    try { plans = await fetchLineupPlans(seasonId, weekNum); } catch (e) { toast(e.message, 'danger'); return; }

    const plansByTeam = {};
    plans.forEach(p => { (plansByTeam[p.team_id] = plansByTeam[p.team_id] || []).push(p); });

    if (weekNum === 0) {
      if (!this.#allTeams.length) {
        container.innerHTML = '<div class="col text-muted py-3">No teams found.</div>';
        return;
      }
      container.innerHTML = this.#allTeams.map(t =>
        this.#renderTeamCard(t, plansByTeam[t.id] || [], parseInt(seasonId), 0)
      ).join('');
      return;
    }

    let matches;
    try { matches = await fetchMatches(seasonId); } catch (e) { toast(e.message, 'danger'); return; }

    const weekMatches = matches.filter(m => m.week_number == weekNum);
    if (!weekMatches.length) {
      container.innerHTML = '<div class="col"><div class="text-muted py-3">No matches this week.</div></div>';
      return;
    }
    container.innerHTML = weekMatches.map(m =>
      this.#renderMatchCard(m, plansByTeam, parseInt(seasonId), weekNum)
    ).join('');
  }

  #badgeHtml(planCount) {
    if (planCount >= 3)
      return '<span class="badge bg-success ms-1" style="font-size:.68rem">Set</span>';
    if (planCount > 0)
      return `<span class="badge bg-warning text-dark ms-1" style="font-size:.68rem">${planCount}/3</span>`;
    return '<span class="badge bg-secondary ms-1" style="font-size:.68rem">Not set</span>';
  }

  #playerSlotsHtml(teamId, plans, idPrefix) {
    const roster = this.#allPlayers.filter(p => p.team_id == teamId);
    return [0, 1, 2].map(i => {
      const saved = plans[i];
      const opts  = '<option value="0">-- not set --</option>' +
        roster.map(p =>
          `<option value="${p.id}"${saved && saved.player_id == p.id ? ' selected' : ''}>${esc(p.name)} (${p.handicap >= 0 ? '+' : ''}${p.handicap})</option>`
        ).join('');
      return `<div class="d-flex align-items-center gap-2 mb-1">
        <span class="text-muted fw-bold" style="width:22px;font-size:.8rem">${idPrefix}${i + 1}</span>
        <select class="form-select form-select-sm" id="lu_${teamId}_${i}">${opts}</select>
      </div>`;
    }).join('');
  }

  #renderTeamCard(team, plans, seasonId, weekNum) {
    const badge = this.#badgeHtml(plans.length);
    const slots = this.#playerSlotsHtml(team.id, plans, 'P');
    return `<div class="col-12 col-md-6 col-xl-4">
      <div class="card h-100">
        <div class="card-header py-2 fw-semibold small d-flex justify-content-between align-items-center">
          <span>${esc(team.name)}${badge}</span>
        </div>
        <div class="card-body">
          ${slots}
          <button class="btn btn-primary btn-sm w-100 mt-2"
            data-action="save-lineup"
            data-season-id="${seasonId}"
            data-week-num="${weekNum}"
            data-team-id="${team.id}">
            <i class="bi bi-check-lg"></i> Save Default Lineup
          </button>
        </div>
      </div>
    </div>`;
  }

  #teamColHtml(teamId, teamName, prefix, plansByTeam, seasonId, weekNum) {
    const plans = plansByTeam[teamId] || [];
    const badge = this.#badgeHtml(plans.length);
    const slots = this.#playerSlotsHtml(teamId, plans, prefix);
    const label = prefix === 'H' ? 'Home' : 'Away';
    return `<div class="col-6">
      <div class="small fw-semibold mb-2">${label}: ${esc(teamName)}${badge}</div>
      ${slots}
      <button class="btn btn-primary btn-sm w-100 mt-2"
        data-action="save-lineup"
        data-season-id="${seasonId}"
        data-week-num="${weekNum}"
        data-team-id="${teamId}">
        <i class="bi bi-check-lg"></i> Save Lineup
      </button>
    </div>`;
  }

  #renderMatchCard(match, plansByTeam, seasonId, weekNum) {
    const dateStr = match.match_date ? ' - ' + fmtDate(match.match_date) : '';
    return `<div class="col-12 col-xl-6">
      <div class="card">
        <div class="card-header py-2 fw-semibold small d-flex justify-content-between">
          <span>${esc(match.home_team_name)} <span class="text-muted fw-normal">vs</span> ${esc(match.away_team_name)}</span>
          <span class="text-muted">Week ${match.week_number}${dateStr}</span>
        </div>
        <div class="card-body">
          <div class="row g-3">
            ${this.#teamColHtml(match.home_team_id, match.home_team_name, 'H', plansByTeam, seasonId, weekNum)}
            ${this.#teamColHtml(match.away_team_id, match.away_team_name, 'A', plansByTeam, seasonId, weekNum)}
          </div>
        </div>
      </div>
    </div>`;
  }

  async #onSaveClick(btn) {
    const seasonId = parseInt(btn.dataset.seasonId);
    const weekNum  = parseInt(btn.dataset.weekNum);
    const teamId   = parseInt(btn.dataset.teamId);

    const playerIds = [0, 1, 2]
      .map(i => parseInt(this.querySelector(`#lu_${teamId}_${i}`)?.value) || 0)
      .filter(id => id > 0);

    if (playerIds.length === 0) { toast('Select at least one player', 'warning'); return; }
    if (new Set(playerIds).size !== playerIds.length) { toast('Duplicate player selected', 'warning'); return; }

    try {
      await saveLineupPlans({ season_id: seasonId, team_id: teamId, week_number: weekNum, player_ids: playerIds });
      toast('Lineup saved');
      await this.#loadCards();
    } catch (e) { toast(e.message, 'danger'); }
  }
}

customElements.define('lineup-page', LineupPage);
