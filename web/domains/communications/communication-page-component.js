// <communication-page> - League Communication Screen Phase 1: generates
// copy/paste message text for teams and players from data other domains
// already expose (Weekly Summary's week recap, Financial Phase 1's dues,
// Player Overview's aggregate). Admin-only (league_admin/admin/system_admin,
// same as Financial and Player Overview) -- gated by the app shell, not by
// this component.
//
// V1 is deliberately copy/paste only: there is no automated email sending,
// SMTP, mobile push, template storage, message history, or delivery
// tracking. The Copy button uses the Clipboard API when available and falls
// back to selecting the text (Ctrl+C) when it is not -- the Clipboard API
// requires a secure context, which staging (plain HTTP) is not, so the
// fallback path is a real, expected path here, not a rare edge case.
//
// Public API:
//   refresh(allSeasons, activeSeason, allTeams, allPlayers)
//     Called by the app shell when the Communication section activates or
//     league/season context changes. All four arrays/objects are already
//     scoped to the active league by the shell (same data Players/Teams/
//     Seasons screens already use), so a selected team/player/season here
//     can never cross into a different league's data.

import { fetchSeasonWeeks, fetchWeekRecap, fetchSeasonDues, fetchPlayerOverview } from './communication-api-service.js';
import {
  buildWeeklyTeamSummaryMessage,
  buildTeamDuesReminderMessage,
  buildPlayerSummaryMessage,
} from './communication-message-generators.js';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

const MESSAGE_TYPES = {
  'weekly-team-summary': { label: 'Weekly Team Summary', needsWeek: true, needsTeam: true, needsPlayer: false },
  'team-dues-reminder': { label: 'Team Dues Reminder', needsWeek: false, needsTeam: true, needsPlayer: false },
  'player-summary': { label: 'Player Summary', needsWeek: false, needsTeam: false, needsPlayer: true },
};

class CommunicationPage extends HTMLElement {
  #allSeasons = [];
  #allTeams   = [];
  #allPlayers = [];

  connectedCallback() {
    this.innerHTML = `
      <h4 class="mb-3 fw-bold">Communication</h4>
      <p class="text-muted small">
        Generates copy/paste message text from current league data. No email
        or mobile notifications are sent from this screen.
      </p>
      <div class="row g-2 mb-3 align-items-end">
        <div class="col-auto">
          <label class="form-label small mb-1">Message Type</label>
          <select class="form-select form-select-sm comm-type-sel">
            ${Object.entries(MESSAGE_TYPES).map(([key, t]) =>
              `<option value="${key}">${esc(t.label)}</option>`).join('')}
          </select>
        </div>
        <div class="col-auto">
          <label class="form-label small mb-1">Season</label>
          <select class="form-select form-select-sm comm-season-sel"></select>
        </div>
        <div class="col-auto comm-week-row d-none">
          <label class="form-label small mb-1">Week</label>
          <select class="form-select form-select-sm comm-week-sel"></select>
        </div>
        <div class="col-auto comm-team-row d-none">
          <label class="form-label small mb-1">Team</label>
          <select class="form-select form-select-sm comm-team-sel"></select>
        </div>
        <div class="col-auto comm-player-row d-none">
          <label class="form-label small mb-1">Player</label>
          <select class="form-select form-select-sm comm-player-sel"></select>
        </div>
      </div>
      <div class="comm-body"></div>`;

    // async so a type/season change can await the week reload below before
    // generating -- otherwise Weekly Team Summary could generate against a
    // stale week left over from the previous season (#loadWeeksIfNeeded is
    // itself async and was previously left unawaited here).
    this.addEventListener('change', async e => {
      if (e.target.matches('.comm-type-sel')) {
        this.#toggleFieldsForType();
        await this.#loadWeeksIfNeeded();
        await this.#generate();
      } else if (e.target.matches('.comm-season-sel')) {
        await this.#loadWeeksIfNeeded();
        await this.#generate();
      } else if (e.target.matches('.comm-week-sel, .comm-team-sel, .comm-player-sel')) {
        await this.#generate();
      }
    });

    this.addEventListener('click', e => {
      if (e.target.closest('[data-action="copy-message"]')) this.#copyMessage();
    });
  }

  refresh(allSeasons, activeSeason, allTeams, allPlayers) {
    this.#allSeasons = allSeasons ?? [];
    this.#allTeams   = allTeams   ?? [];
    this.#allPlayers = allPlayers ?? [];

    this.#populateSeasonSelect(activeSeason);
    this.#populateTeamSelect();
    this.#populatePlayerSelect();
    this.#toggleFieldsForType();
    this.#loadWeeksIfNeeded().then(() => this.#generate());
  }

  // -- Private ------------------------------------------------------------------

  #currentType() {
    const key = this.querySelector('.comm-type-sel')?.value || 'weekly-team-summary';
    return { key, ...MESSAGE_TYPES[key] };
  }

  #toggleFieldsForType() {
    const type = this.#currentType();
    this.querySelector('.comm-week-row')?.classList.toggle('d-none', !type.needsWeek);
    this.querySelector('.comm-team-row')?.classList.toggle('d-none', !type.needsTeam);
    this.querySelector('.comm-player-row')?.classList.toggle('d-none', !type.needsPlayer);
  }

  #populateSeasonSelect(activeSeason) {
    const sel = this.querySelector('.comm-season-sel');
    if (!sel) return;
    sel.innerHTML = this.#allSeasons.map(s =>
      `<option value="${s.id}"${s.active ? ' selected' : ''}>${esc(s.name)}</option>`
    ).join('') || '<option value="">No seasons</option>';
    if (activeSeason) sel.value = String(activeSeason.id);
  }

  #populateTeamSelect() {
    const sel = this.querySelector('.comm-team-sel');
    if (!sel) return;
    const sorted = [...this.#allTeams].sort((a, b) => a.name.localeCompare(b.name));
    sel.innerHTML = sorted.map(t => `<option value="${t.id}">${esc(t.name)}</option>`).join('') ||
      '<option value="">No teams</option>';
  }

  #populatePlayerSelect() {
    const sel = this.querySelector('.comm-player-sel');
    if (!sel) return;
    const sorted = [...this.#allPlayers].sort((a, b) => a.name.localeCompare(b.name));
    sel.innerHTML = sorted.map(p =>
      `<option value="${p.id}">${esc(p.name)}${p.team_name ? ' - ' + esc(p.team_name) : ''}</option>`
    ).join('') || '<option value="">No players</option>';
  }

  async #loadWeeksIfNeeded() {
    if (!this.#currentType().needsWeek) return;
    const seasonId = this.querySelector('.comm-season-sel')?.value;
    const weekSel  = this.querySelector('.comm-week-sel');
    if (!seasonId || !weekSel) return;
    // Clear to an empty-value placeholder before the await below, so a
    // stale week number from the previous season can never be read (by
    // this method's own caller or a #generate() call that slips in while
    // this fetch is in flight) -- #generate() already treats an empty
    // week value as "nothing to generate yet."
    weekSel.innerHTML = '<option value="">Loading weeks...</option>';
    let weeks;
    try {
      weeks = await fetchSeasonWeeks(seasonId);
    } catch (e) {
      weekSel.innerHTML = '<option value="">Failed to load weeks</option>';
      return;
    }
    weekSel.innerHTML = weeks.map(w =>
      `<option value="${w.week_number}">Week ${w.week_number}${w.status === 'closed' ? ' - Closed' : ''}</option>`
    ).join('') || '<option value="">No weeks</option>';
  }

  async #generate() {
    const body = this.querySelector('.comm-body');
    if (!body) return;
    const type = this.#currentType();
    const seasonId = this.querySelector('.comm-season-sel')?.value;
    const season = this.#allSeasons.find(s => String(s.id) === String(seasonId));
    if (!season) { body.innerHTML = ''; return; }

    let text;
    try {
      if (type.key === 'weekly-team-summary') {
        text = await this.#generateWeeklyTeamSummary(season);
      } else if (type.key === 'team-dues-reminder') {
        text = await this.#generateTeamDuesReminder(season);
      } else if (type.key === 'player-summary') {
        text = await this.#generatePlayerSummary(season);
      }
    } catch (e) {
      body.innerHTML = `<div class="alert alert-danger">${esc(e.message)}</div>`;
      return;
    }

    if (text == null) { body.innerHTML = ''; return; }
    body.innerHTML = this.#renderMessage(text);
  }

  async #generateWeeklyTeamSummary(season) {
    const teamId = this.querySelector('.comm-team-sel')?.value;
    const weekNumber = this.querySelector('.comm-week-sel')?.value;
    const team = this.#allTeams.find(t => String(t.id) === String(teamId));
    if (!team || !weekNumber) return null;
    const recap = await fetchWeekRecap(season.id, weekNumber);
    return buildWeeklyTeamSummaryMessage({ team, season, weekNumber: parseInt(weekNumber, 10), recap });
  }

  async #generateTeamDuesReminder(season) {
    const teamId = this.querySelector('.comm-team-sel')?.value;
    const team = this.#allTeams.find(t => String(t.id) === String(teamId));
    if (!team) return null;
    const dues = await fetchSeasonDues(season.id);
    return buildTeamDuesReminderMessage({ team, season, dues });
  }

  async #generatePlayerSummary(season) {
    const playerId = this.querySelector('.comm-player-sel')?.value;
    if (!playerId) return null;
    const overview = await fetchPlayerOverview(playerId, season.id);
    return buildPlayerSummaryMessage({ overview });
  }

  #renderMessage(text) {
    return `<div class="card">
      <div class="card-header fw-semibold py-2 d-flex justify-content-between align-items-center">
        <span>Generated Message</span>
        <button class="btn btn-outline-primary btn-sm py-0" data-action="copy-message">
          <i class="bi bi-clipboard"></i> Copy
        </button>
      </div>
      <div class="card-body">
        <textarea class="form-control comm-message-text" rows="16" readonly>${esc(text)}</textarea>
      </div>
    </div>`;
  }

  #selectMessageText() {
    const ta = this.querySelector('.comm-message-text');
    if (ta) { ta.focus(); ta.select(); }
  }

  async #copyMessage() {
    const text = this.querySelector('.comm-message-text')?.value;
    if (!text) return;
    // navigator.clipboard requires a secure context (HTTPS or localhost).
    // Staging is plain HTTP today, so the fallback below is a real path,
    // not a rare edge case -- select the text so Ctrl+C still works.
    try {
      if (navigator.clipboard && window.isSecureContext) {
        await navigator.clipboard.writeText(text);
        toast('Copied to clipboard');
        return;
      }
    } catch (_) {
      // fall through to the selection fallback below
    }
    this.#selectMessageText();
    toast('Could not copy automatically -- message text selected, use Ctrl+C', 'warning');
  }
}

customElements.define('communication-page', CommunicationPage);
