// <weekly-summary-page> - Weekly Summary Phase 1: an admin overview of one
// week's scoring/approval/processing status, handicap changes, and
// next-week scoresheet readiness -- built on the existing Week Recap
// endpoint (now including approved_at/processed_at/week_closed per match,
// Weekly Summary Phase 1). Presentation-only; Close Week itself stays on
// the Schedule page, only linked to from here.
//
// Public API:
//   refresh(allSeasons, activeSeason)
//     Called by the app shell when the Weekly Summary section activates
//     or league/season context changes.

import { fetchSeasonWeeks, fetchWeekRecap, processMatch } from './weekly-summary-api-service.js';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

function fmtHC(v) { return (v >= 0 ? '+' : '') + v; }

class WeeklySummaryPage extends HTMLElement {
  #allSeasons = [];
  #weeks      = [];

  connectedCallback() {
    this.innerHTML = `
      <h4 class="mb-3 fw-bold">Weekly Summary</h4>
      <div class="row g-2 mb-3 align-items-end">
        <div class="col-auto">
          <label class="form-label small mb-1">Season</label>
          <select class="form-select form-select-sm ws-season-sel"></select>
        </div>
        <div class="col-auto">
          <label class="form-label small mb-1">Week</label>
          <select class="form-select form-select-sm ws-week-sel"></select>
        </div>
      </div>
      <div class="ws-body"></div>`;

    this.addEventListener('change', e => {
      if (e.target.matches('.ws-season-sel')) this.#loadWeeks();
      if (e.target.matches('.ws-week-sel'))  this.#loadRecap();
    });

    this.addEventListener('click', e => {
      if (e.target.closest('[data-action="process-approved"]')) { this.#processApproved(); return; }
      const meBtn = e.target.closest('[data-action="open-match-entry"]');
      if (meBtn) {
        openMatchEntry(parseInt(meBtn.dataset.matchId, 10), parseInt(meBtn.dataset.seasonId, 10));
        return;
      }
      const hcBtn = e.target.closest('[data-action="open-handicap-for-week"]');
      if (hcBtn) {
        openHandicapForWeek(parseInt(hcBtn.dataset.seasonId, 10), parseInt(hcBtn.dataset.weekNum, 10));
        return;
      }
      const schedBtn = e.target.closest('[data-action="open-schedule"]');
      if (schedBtn) {
        this.dispatchEvent(new CustomEvent('season-nav-request', {
          bubbles: true,
          detail: { section: 'schedule', previewSeasonId: parseInt(schedBtn.dataset.seasonId, 10) },
        }));
      }
    });
  }

  refresh(allSeasons, activeSeason) {
    this.#allSeasons = allSeasons ?? [];
    this.#populateSeasonSelect(activeSeason);
    this.#loadWeeks();
  }

  // -- Private ------------------------------------------------------------------

  #populateSeasonSelect(activeSeason) {
    const sel = this.querySelector('.ws-season-sel');
    if (!sel) return;
    sel.innerHTML = this.#allSeasons.map(s =>
      `<option value="${s.id}"${s.active ? ' selected' : ''}>${esc(s.name)}</option>`
    ).join('') || '<option value="">No seasons</option>';
    if (activeSeason) sel.value = String(activeSeason.id);
  }

  async #loadWeeks() {
    const seasonId = this.querySelector('.ws-season-sel')?.value;
    const weekSel  = this.querySelector('.ws-week-sel');
    const body     = this.querySelector('.ws-body');
    if (!seasonId || !weekSel) return;
    try {
      this.#weeks = await fetchSeasonWeeks(seasonId);
    } catch (e) {
      toast(e.message, 'danger');
      return;
    }
    weekSel.innerHTML = this.#weeks.map(w =>
      `<option value="${w.week_number}">Week ${w.week_number} (${w.completed_count}/${w.match_count} scored)${w.status === 'closed' ? ' - Closed' : ''}</option>`
    ).join('') || '<option value="">No weeks</option>';
    if (this.#weeks.length === 0 && body) body.innerHTML = '';
    await this.#loadRecap();
  }

  async #loadRecap() {
    const seasonId = this.querySelector('.ws-season-sel')?.value;
    const weekNum  = this.querySelector('.ws-week-sel')?.value;
    const body     = this.querySelector('.ws-body');
    if (!seasonId || !weekNum || !body) { if (body) body.innerHTML = ''; return; }

    let recap;
    try {
      recap = await fetchWeekRecap(seasonId, weekNum);
    } catch (e) {
      body.innerHTML = `<div class="alert alert-danger">${esc(e.message)}</div>`;
      return;
    }
    body.innerHTML = this.#renderRecap(recap, parseInt(seasonId, 10));
  }

  // Unscored / Scored / Approved / Processed / Closed status ladder.
  // week_closed is the terminal display state and always renders as
  // Closed regardless of processed_at -- Close Week only auto-processes
  // approved matches (Phase 1B), so an unapproved match can still reach
  // week_closed=true with processed_at unset.
  #matchStatus(m) {
    if (m.week_closed)  return { label: 'Closed',    cls: 'bg-success' };
    if (m.processed_at) return { label: 'Processed', cls: 'bg-primary' };
    if (m.approved_at)  return { label: 'Approved',  cls: 'bg-info text-dark' };
    if (m.has_result)   return { label: 'Scored',    cls: 'bg-secondary' };
    return { label: 'Unscored', cls: 'bg-warning text-dark' };
  }

  #renderRecap(recap, seasonId) {
    const rows = recap.matches || [];
    const approvedUnprocessed = rows.filter(m => m.approved_at && !m.processed_at && !m.week_closed);

    const matchRows = rows.map(m => {
      const status = this.#matchStatus(m);
      return `<tr>
        <td>${esc(m.home_team_name || '(unassigned)')}</td>
        <td class="text-center text-muted small">vs</td>
        <td>${esc(m.away_team_name || '(unassigned)')}</td>
        <td><span class="badge ${status.cls}">${status.label}</span></td>
        <td class="text-end">
          <button class="btn btn-outline-secondary btn-sm py-0" data-action="open-match-entry"
            data-match-id="${m.match_id}" data-season-id="${seasonId}">Open</button>
        </td>
      </tr>`;
    }).join('') || '<tr><td colspan="5" class="text-center text-muted py-3">No matches this week</td></tr>';

    // Incomplete-week-safe copy: this week does not need to be fully
    // scored for the rest of the screen (handicap preview, next-week
    // readiness) to be meaningful -- it reflects data entered so far.
    const incompleteNote = recap.missing_count > 0
      ? `<div class="alert alert-warning py-2 small mb-3">
          <i class="bi bi-exclamation-triangle me-1"></i>${recap.missing_count} match${recap.missing_count !== 1 ? 'es' : ''}
          still need${recap.missing_count === 1 ? 's' : ''} scores. Handicap changes and stats below
          reflect data entered so far, not necessarily the week's final results.
        </div>`
      : '';

    const processBtn = approvedUnprocessed.length > 0
      ? `<button class="btn btn-primary btn-sm" data-action="process-approved">
          <i class="bi bi-gear"></i> Process Approved Scores (${approvedUnprocessed.length})</button>`
      : '';

    const closedBadge = recap.status === 'closed'
      ? '<span class="badge bg-success">Week Closed</span>'
      : '<span class="badge bg-secondary">Week Open</span>';

    let nextWeekSection = '';
    if (recap.next_week_number) {
      const nw = recap.next_week || {};
      nextWeekSection = `<div class="card mb-3">
        <div class="card-header fw-semibold py-2">Week ${recap.next_week_number} Readiness</div>
        <div class="card-body small">
          ${nw.match_count || 0} match${(nw.match_count || 0) !== 1 ? 'es' : ''} &middot;
          ${nw.unassigned_count > 0
            ? `<span class="text-warning">${nw.unassigned_count} unassigned</span>`
            : '<span class="text-success">All assigned</span>'} &middot;
          ${nw.missing_lineup_team_ids && nw.missing_lineup_team_ids.length > 0
            ? `<span class="text-warning">${nw.missing_lineup_team_ids.length} team(s) missing a lineup</span>`
            : '<span class="text-success">Lineups ready</span>'}
        </div>
      </div>`;
    }

    const hcChanges = recap.handicap_changes || [];
    const hcChangesRows = hcChanges.map(c => `<tr>
      <td>${esc(c.player_name || '')}</td>
      <td class="text-center">${fmtHC(c.old_handicap)} &rarr; ${fmtHC(c.new_handicap)}</td>
    </tr>`).join('');

    const preview = recap.handicap || {};
    const previewRecs = preview.recommendations || [];
    const previewRows = previewRecs.map(r => `<tr>
      <td>${esc(r.player_name || '')}</td>
      <td class="text-center">${fmtHC(r.current_handicap)} &rarr; ${fmtHC(r.recommended_handicap)}</td>
    </tr>`).join('');

    const hcSection = `<div class="card mb-3">
      <div class="card-header fw-semibold py-2 d-flex justify-content-between align-items-center">
        <span>Handicap</span>
        <button class="btn btn-link btn-sm py-0" data-action="open-handicap-for-week"
          data-season-id="${seasonId}" data-week-num="${recap.week_number}">Review &amp; Apply</button>
      </div>
      <div class="card-body">
        <div class="mb-3">
          <div class="fw-semibold small mb-1">Recorded this week</div>
          ${hcChangesRows
            ? `<table class="table table-sm mb-0"><thead><tr><th>Player</th><th class="text-center">Change</th></tr></thead><tbody>${hcChangesRows}</tbody></table>`
            : '<p class="text-muted small mb-0">No handicap changes recorded for this week yet.</p>'}
        </div>
        <div>
          <div class="fw-semibold small mb-1">Season-wide recommendations</div>
          ${preview.message ? `<p class="small text-muted">${esc(preview.message)}</p>` : ''}
          ${previewRows
            ? `<table class="table table-sm mb-0"><thead><tr><th>Player</th><th class="text-center">Change</th></tr></thead><tbody>${previewRows}</tbody></table>`
            : ''}
        </div>
      </div>
    </div>`;

    return `
      <div class="d-flex justify-content-between align-items-center mb-2">
        <div>${closedBadge}</div>
        <div class="d-flex gap-2">
          ${processBtn}
          <button class="btn btn-outline-secondary btn-sm" data-action="open-schedule" data-season-id="${seasonId}">
            Open in Schedule</button>
        </div>
      </div>
      ${incompleteNote}
      <div class="card mb-3">
        <div class="card-header fw-semibold py-2">Matches</div>
        <div class="card-body p-0">
          <table class="table table-sm mb-0"><tbody>${matchRows}</tbody></table>
        </div>
      </div>
      ${nextWeekSection}
      ${hcSection}`;
  }

  async #processApproved() {
    const seasonId = this.querySelector('.ws-season-sel')?.value;
    const weekNum  = this.querySelector('.ws-week-sel')?.value;
    if (!seasonId || !weekNum) return;

    let recap;
    try {
      recap = await fetchWeekRecap(seasonId, weekNum);
    } catch (e) { toast(e.message, 'danger'); return; }

    const toProcess = (recap.matches || []).filter(m => m.approved_at && !m.processed_at && !m.week_closed);
    if (toProcess.length === 0) { toast('Nothing to process'); return; }

    let succeeded = 0;
    for (const m of toProcess) {
      try { await processMatch(m.match_id); succeeded++; }
      catch (e) { toast(`Match ${m.match_id}: ${e.message}`, 'danger'); }
    }
    toast(`Processed ${succeeded} of ${toProcess.length} match${toProcess.length !== 1 ? 'es' : ''}`);
    await this.#loadRecap();
  }
}

customElements.define('weekly-summary-page', WeeklySummaryPage);
