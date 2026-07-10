// <schedule-page> - Schedule section component.
//
// Public API:
//   refresh(allSeasons, allTeams, activeLeague)
//     Called by the app shell when Schedule activates or league context changes.
//   loadForSeason(previewSeasonId)
//     Called by the shell to display a specific season (from season-nav-request).
//   openPoster()
//     Called by the shell to switch to poster view (from season-nav-request).

import {
  fetchSeasonMatches,
  fetchSeasonWeeks,
  fetchWeekValidation,
  fetchAdvancePreview,
  fetchWeekAcknowledgments,
  closeWeek,
  reopenWeek,
  assignMatchTeams,
  fetchLeaguePlayers,
} from './schedule-api-service.js';
import { fmtDate } from '../../components/date-display.js';
import {
  REASON_ADMIN_HOLD,
  REASON_NO_DATA,
  REASON_NO_CHANGE,
  REASON_CAPPED,
} from '../handicaps/handicap-codes.js';

const WEEK_STATUS_CLOSED = 'closed';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

function fmtHC(v) { return (v >= 0 ? '+' : '') + v; }

function posterDateLabel(matchDate, weekNumber) {
  if (!matchDate) return 'Wk ' + weekNumber;
  const datePart = String(matchDate).split('T')[0];
  const parts = datePart.split('-').map(n => parseInt(n, 10));
  if (parts.length === 3 && parts.every(Number.isFinite)) {
    return parts[1] + '/' + parts[2];
  }
  const d = new Date(matchDate);
  return Number.isNaN(d.getTime()) ? 'Wk ' + weekNumber : (d.getMonth() + 1) + '/' + d.getDate();
}

const POSTER_BALLS = [
  { bg: '#f5c500', fg: '#000' },
  { bg: '#1547b8', fg: '#fff' },
  { bg: '#d91414', fg: '#fff' },
  { bg: '#6a24a0', fg: '#fff' },
  { bg: '#f07a00', fg: '#000' },
  { bg: '#1a7828', fg: '#fff' },
  { bg: '#8b0000', fg: '#fff' },
  { bg: '#222',    fg: '#fff' },
];

function pocketHTML() {
  return ['tl', 'tm', 'tr', 'bl', 'bm', 'br'].map(p =>
    '<div class="poster-pocket poster-pocket-' + p + '"></div>'
  ).join('');
}

class SchedulePage extends HTMLElement {
  #allSeasons    = [];
  #allTeams      = [];
  #activeLeague  = null;
  #reopenWeekCtx = null;
  #assignMatchId = null;

  connectedCallback() {
    this.innerHTML = `
      <div class="sp-table-view">
        <div class="d-flex justify-content-between align-items-center mb-3">
          <h4 class="mb-0 fw-bold">Schedule</h4>
          <div class="d-flex gap-2 align-items-center">
            <select class="form-select form-select-sm w-auto sp-season-sel"></select>
            <button class="btn btn-sm btn-outline-success" data-action="open-poster">
              <i class="bi bi-image"></i> Poster View
            </button>
          </div>
        </div>
        <div class="sp-content"></div>
      </div>
      <div class="sp-poster-view d-none">
        <div class="no-print d-flex justify-content-between align-items-center mb-3">
          <button class="btn btn-sm btn-outline-secondary" data-action="close-poster">
            <i class="bi bi-arrow-left"></i> Table View
          </button>
          <div class="d-flex align-items-center gap-2 flex-wrap justify-content-end">
            <span class="text-muted" style="font-size:.75rem">
              <i class="bi bi-info-circle"></i> For full color, enable
              <strong>Background graphics</strong> in the print dialog.
            </span>
            <button class="btn btn-sm btn-outline-secondary" data-action="print-poster">
              <i class="bi bi-printer"></i> Print
            </button>
          </div>
        </div>
        <div id="poster-print-area">
          <div class="sp-poster-content"></div>
        </div>
      </div>`;

    this.addEventListener('change', e => {
      if (e.target.matches('.sp-season-sel')) this.#loadSchedule();
    });

    this.addEventListener('click', e => {
      if (e.target.closest('[data-action="open-poster"]'))  { this.#openPosterView(); return; }
      if (e.target.closest('[data-action="close-poster"]')) { this.#closePosterView(); return; }
      if (e.target.closest('[data-action="print-poster"]')) { window.print(); return; }
      if (e.target.closest('[data-action="go-seasons"]'))   { e.preventDefault(); navTo('seasons'); return; }

      const assignBtn = e.target.closest('[data-action="assign-match"]');
      if (assignBtn) { this.#openAssignModal(parseInt(assignBtn.dataset.matchId, 10)); return; }

      const entryBtn = e.target.closest('[data-action="open-match-entry"]');
      if (entryBtn) {
        openMatchEntry(parseInt(entryBtn.dataset.matchId, 10), parseInt(entryBtn.dataset.seasonId, 10) || null);
        return;
      }

      const reopenBtn = e.target.closest('[data-action="reopen-week"]');
      if (reopenBtn) {
        this.#confirmReopenWeek(
          parseInt(reopenBtn.dataset.seasonId, 10),
          parseInt(reopenBtn.dataset.weekNum, 10),
          reopenBtn.dataset.matchDate || ''
        );
        return;
      }

      const reviewBtn = e.target.closest('[data-action="review-close-week"]');
      if (reviewBtn) {
        this.#reviewCloseWeek(
          parseInt(reviewBtn.dataset.seasonId, 10),
          parseInt(reviewBtn.dataset.weekNum, 10),
          parseInt(reviewBtn.dataset.ackCount, 10) || 0
        );
        return;
      }

      const toggleBtn = e.target.closest('[data-action="toggle-week-acks"]');
      if (toggleBtn) {
        const wn = toggleBtn.dataset.weekNum;
        const container = this.querySelector('#week-acks-' + wn);
        if (!container) return;
        if (container.classList.contains('d-none')) {
          container.classList.remove('d-none');
          if (container.dataset.loaded !== '1') {
            const sid = parseInt(this.querySelector('.sp-season-sel')?.value, 10);
            this.#loadWeekAcknowledgments(sid, parseInt(wn, 10), container);
          }
        } else {
          container.classList.add('d-none');
        }
      }
    });

    // Wire global modal event handlers.
    document.getElementById('reopen-week-modal')
      ?.addEventListener('hidden.bs.modal', () => { this.#reopenWeekCtx = null; });

    document.getElementById('reopen-week-confirm-btn')
      ?.addEventListener('click', async () => {
        if (!this.#reopenWeekCtx) return;
        const { seasonId, weekNum, modal } = this.#reopenWeekCtx;
        this.#reopenWeekCtx = null;
        try {
          await reopenWeek(seasonId, weekNum);
          modal.hide();
          toast('Week ' + weekNum + ' reopened.', 'success');
          await this.#loadSchedule();
          this.#dispatchDataChanged();
        } catch (e) {
          toast('Reopen failed: ' + (e.message || e), 'danger');
        }
      });

    document.getElementById('close-week-modal')
      ?.addEventListener('click', e => {
        const entryBtn = e.target.closest('[data-action="open-match-entry"]');
        if (entryBtn) {
          openMatchEntry(parseInt(entryBtn.dataset.matchId, 10), parseInt(entryBtn.dataset.seasonId, 10) || null);
          return;
        }
        const priorBtn = e.target.closest('[data-action="load-prior-acks"]');
        if (priorBtn) {
          const container = document.getElementById('cwm-prior-acks');
          if (!container) return;
          if (container.dataset.loaded === '1') {
            container.classList.toggle('d-none');
          } else {
            container.classList.remove('d-none');
            this.#loadWeekAcknowledgments(
              parseInt(priorBtn.dataset.seasonId, 10),
              parseInt(priorBtn.dataset.weekNum, 10),
              container
            );
          }
        }
      });

    document.getElementById('assign-confirm-btn')
      ?.addEventListener('click', () => this.#confirmAssign());
  }

  refresh(allSeasons, allTeams, activeLeague) {
    this.#allSeasons  = allSeasons  ?? [];
    this.#allTeams    = allTeams    ?? [];
    this.#activeLeague = activeLeague;
    this.#populate(null);
  }

  loadForSeason(previewSeasonId) {
    this.#populate(previewSeasonId);
  }

  openPoster() {
    this.#openPosterView();
  }

  #populate(previewSeasonId) {
    const sel       = this.querySelector('.sp-season-sel');
    const contentEl = this.querySelector('.sp-content');
    if (!sel || !contentEl) return;

    if (previewSeasonId !== null) {
      const s = this.#allSeasons.find(x => x.id === previewSeasonId);
      if (s) {
        sel.innerHTML = `<option value="${s.id}">${esc(s.name)}${s.active ? '' : ' (draft)'}</option>`;
        sel.value = s.id;
        this.#loadSchedule();
        return;
      }
    }

    const active = this.#allSeasons.filter(s => s.active);
    if (active.length === 0) {
      sel.innerHTML = '<option value="">-- No active season --</option>';
      contentEl.innerHTML = `<div class="text-center py-5 text-muted">
        <i class="bi bi-calendar-x" style="font-size:2rem"></i>
        <div class="mt-2 fw-semibold">No active season</div>
        <div class="small mt-1">Go to <a href="#" data-action="go-seasons" class="text-decoration-none">Seasons</a> to activate one.</div>
      </div>`;
      return;
    }
    sel.innerHTML = active.map(s => `<option value="${s.id}">${esc(s.name)}</option>`).join('');
    this.#loadSchedule();
  }

  async #loadSchedule() {
    const seasonId = this.querySelector('.sp-season-sel')?.value;
    const contentEl = this.querySelector('.sp-content');
    if (!seasonId) { contentEl.innerHTML = '<div class="text-muted">No season selected.</div>'; return; }

    let matches, weekList;
    try {
      [matches, weekList] = await Promise.all([
        fetchSeasonMatches(seasonId),
        fetchSeasonWeeks(seasonId).catch(() => []),
      ]);
    } catch (e) {
      contentEl.innerHTML = `<div class="text-danger">${esc(e.message)}</div>`;
      return;
    }

    const weekStatusMap = {};
    (weekList || []).forEach(ws => { weekStatusMap[ws.week_number] = ws; });

    const byWeek = {};
    matches.forEach(m => { (byWeek[m.week_number] = byWeek[m.week_number] || []).push(m); });
    const weeks = Object.keys(byWeek).sort((a, b) => a - b);

    const seasonTeamIds   = new Set();
    const seasonTeamNames = {};
    matches.forEach(m => {
      if (m.home_team_id) { seasonTeamIds.add(m.home_team_id); seasonTeamNames[m.home_team_id] = m.home_team_name; }
      if (m.away_team_id) { seasonTeamIds.add(m.away_team_id); seasonTeamNames[m.away_team_id] = m.away_team_name; }
    });

    contentEl.innerHTML = weeks.length
      ? weeks.map(w => this.#renderWeekCard(w, byWeek[w], weekStatusMap[w], seasonId, seasonTeamIds, seasonTeamNames)).join('')
      : '<div class="text-muted text-center py-4">No matches scheduled yet. Use Seasons to generate a schedule.</div>';
  }

  #renderWeekCard(w, weekMatches, ws, seasonId, seasonTeamIds, seasonTeamNames) {
    const isUnassigned = m => !m.home_team_id || m.home_team_name === '(unassigned)' ||
                              !m.away_team_id || m.away_team_name === '(unassigned)';

    const playingThisWeek = new Set();
    weekMatches.forEach(m => {
      if (m.home_team_id) playingThisWeek.add(m.home_team_id);
      if (m.away_team_id) playingThisWeek.add(m.away_team_id);
    });
    const byeTeams = [...seasonTeamIds].filter(id => !playingThisWeek.has(id));
    const byeRow   = byeTeams.length
      ? `<tr class="table-light">
          <td colspan="4" class="text-muted small fst-italic ps-3 py-1">
            <i class="bi bi-person-x me-1"></i>Bye: ${byeTeams.map(id => esc(seasonTeamNames[id] || '?')).join(', ')}
          </td>
          <td></td>
        </tr>`
      : '';

    const isClosed    = ws && ws.status === WEEK_STATUS_CLOSED;
    const closedLabel = (ws && ws.closed_at) ? 'Closed &middot; ' + fmtDate(ws.closed_at) : 'Closed';
    const statusChip  = isClosed
      ? `<span class="badge bg-success ms-2">${closedLabel}</span>`
      : '<span class="badge bg-secondary ms-2">Open</span>';
    const countBadge  = ws
      ? `<span class="text-muted small ms-2">${ws.completed_count}/${ws.match_count} done</span>`
      : '';
    const matchDate = weekMatches[0].match_date;
    const ackCount  = ws ? ws.ack_count : 0;
    const ackToggle = ackCount > 0
      ? `<button class="btn btn-link btn-sm py-0 ms-1 text-secondary" data-action="toggle-week-acks" data-week-num="${w}" title="Show/hide prior close acknowledgments"><i class="bi bi-clock-history"></i> ${ackCount} prior ack${ackCount !== 1 ? 's' : ''}</button>`
      : '';
    const closeBtn = isClosed
      ? `<button class="btn btn-sm btn-outline-warning py-0 ms-2" data-action="reopen-week" data-season-id="${esc(String(seasonId))}" data-week-num="${w}" data-match-date="${esc(matchDate || '')}" title="Reopen to correct scores, then re-close"><i class="bi bi-arrow-counterclockwise"></i> Reopen</button>`
      : `<button class="btn btn-sm btn-outline-primary py-0 ms-2" data-action="review-close-week" data-season-id="${esc(String(seasonId))}" data-week-num="${w}" data-ack-count="${ackCount}">Review &amp; Close</button>`;
    const ackSection = ackCount > 0
      ? `<div class="px-3 pb-2 d-none" id="week-acks-${w}" data-loaded="0"></div>`
      : '';

    const rows = weekMatches.map(m => `<tr>
      <td class="text-truncate">${m.home_team_name || '<span class="text-muted fst-italic">(unassigned)</span>'}</td>
      <td class="text-center text-muted">vs</td>
      <td class="text-truncate">${m.away_team_name || '<span class="text-muted fst-italic">(unassigned)</span>'}</td>
      <td>${m.completed
        ? '<span class="badge bg-success">Done</span>'
        : '<span class="badge bg-secondary">Pending</span>'}</td>
      <td class="text-end">${isUnassigned(m)
        ? `<button class="btn btn-outline-primary btn-sm py-0" data-action="assign-match" data-match-id="${m.id}"><i class="bi bi-people"></i> Assign</button>`
        : !isClosed
          ? `<button class="btn btn-outline-secondary btn-sm py-0" data-action="open-match-entry" data-season-id="${esc(String(seasonId))}" data-match-id="${m.id}"><i class="bi bi-pencil-square"></i> Score Entry</button>`
          : ''}</td>
    </tr>`).join('');

    return `
    <div class="card mb-2">
      <div class="card-header week-header py-1 d-flex align-items-center justify-content-between">
        <span>Week ${w} - ${fmtDate(weekMatches[0].match_date)}</span>
        <span class="d-flex align-items-center">${countBadge}${statusChip}${ackToggle}${closeBtn}</span>
      </div>
      <div class="card-body p-0">
        <table class="table table-sm mb-0" style="table-layout:fixed">
          <colgroup>
            <col style="width:37%">
            <col style="width:5%">
            <col style="width:37%">
            <col style="width:11%">
            <col style="width:10%">
          </colgroup>
          <thead><tr>
            <th>Home</th>
            <th class="text-center">vs</th>
            <th>Away</th>
            <th>Status</th>
            <th></th>
          </tr></thead>
          <tbody>
            ${rows}
            ${byeRow}
          </tbody>
        </table>
        ${ackSection}
      </div>
    </div>`;
  }

  async #loadWeekAcknowledgments(seasonId, weekNum, container) {
    try {
      const acks = await fetchWeekAcknowledgments(seasonId, weekNum);
      container.dataset.loaded = '1';
      if (!acks || acks.length === 0) {
        container.innerHTML = '<p class="text-muted small mb-0">No acknowledgment records found.</p>';
        return;
      }
      const rows = acks.map((a, idx) => {
        const matchBadge = a.match_id
          ? `<span class="badge bg-secondary me-1">Match ${esc(String(a.match_id))}</span>`
          : '';
        const field  = a.field ? ` <span class="text-muted">(${esc(a.field)})</span>` : '';
        const notes  = a.notes ? ` - <span class="fst-italic">${esc(a.notes)}</span>` : '';
        const ts     = esc(String(a.acknowledged_at));
        const border = idx < acks.length - 1 ? ' border-bottom' : '';
        return `<li class="small py-1${border}">`
          + `<span class="text-muted me-2">${ts}</span>`
          + `${matchBadge}<strong>${esc(a.warning_code)}</strong>${field}${notes}`
          + `</li>`;
      }).join('');
      container.innerHTML = `<ul class="list-unstyled mb-0">${rows}</ul>`;
    } catch (_) {
      container.innerHTML = '<p class="text-danger small mb-0">Failed to load acknowledgment history.</p>';
    }
  }

  #renderHandicapRecs(hc) {
    if (!hc || !Array.isArray(hc.recommendations) || hc.recommendations.length === 0) return '';
    const rows = hc.recommendations.map(r => {
      let recCell, noteCell;
      if (r.skipped) {
        recCell  = '<span class="text-muted">N/A</span>';
        noteCell = r.reason === REASON_ADMIN_HOLD
          ? '<span class="badge bg-secondary">Admin Hold</span>'
          : r.reason === REASON_NO_DATA
            ? '<span class="text-muted fst-italic">No data</span>'
            : `<span class="text-muted fst-italic">${esc(r.reason || '')}</span>`;
      } else {
        recCell  = esc(fmtHC(r.recommended_handicap));
        noteCell = r.reason === REASON_NO_CHANGE
          ? '<span class="text-muted fst-italic">No change</span>'
          : r.reason === REASON_CAPPED
            ? '<span class="badge bg-warning text-dark">Capped</span>'
            : '';
      }
      const trClass = r.skipped ? ' class="text-muted"' : '';
      return `<tr${trClass}><td>${esc(r.player_name)}</td><td>${esc(fmtHC(r.current_handicap))}</td><td>${recCell}</td><td>${r.matches_played}</td><td>${noteCell}</td></tr>`;
    }).join('');
    return `<div class="mt-2">
      <p class="small fw-bold text-danger mb-1"><i class="bi bi-exclamation-circle me-1"></i>Recommendations are <strong>not applied automatically</strong> -- review and update manually if needed.</p>
      <table class="table table-sm table-borderless small mb-0">
        <thead><tr>
          <th class="text-muted fw-normal small">Player</th>
          <th class="text-muted fw-normal small">Current</th>
          <th class="text-muted fw-normal small">Recommended</th>
          <th class="text-muted fw-normal small">Matches</th>
          <th class="text-muted fw-normal small">Notes</th>
        </tr></thead>
        <tbody>${rows}</tbody>
      </table>
    </div>`;
  }

  #renderAdvancePreview(preview) {
    const cw      = preview.current_week;
    const nextNum = preview.next_week_number;
    const nw      = preview.next_week;
    const hc      = preview.handicap;
    let rows = `<tr>
      <td class="text-muted pe-3 text-nowrap">This week</td>
      <td>${esc(String(cw.completed_count))}/${esc(String(cw.match_count))} scored &nbsp;${preview.can_close
        ? '<span class="badge bg-success">Ready</span>'
        : '<span class="badge bg-danger">Has errors</span>'}</td>
    </tr>`;
    if (nw) {
      const assignNote = nw.unassigned_count > 0
        ? ` &middot; <span class="text-warning">${nw.unassigned_count} unassigned</span>` : '';
      const lineupNote = nw.missing_lineup_team_ids && nw.missing_lineup_team_ids.length > 0
        ? ` &middot; <span class="text-warning">${nw.missing_lineup_team_ids.length} missing lineup</span>`
        : ' &middot; <span class="text-success">Lineups set</span>';
      rows += `<tr>
        <td class="text-muted pe-3 text-nowrap">Week ${esc(String(nextNum))}</td>
        <td>${esc(String(nw.match_count))} match${nw.match_count !== 1 ? 'es' : ''}${assignNote}${lineupNote}</td>
      </tr>`;
    } else {
      rows += `<tr>
        <td class="text-muted pe-3 text-nowrap">Next week</td>
        <td class="fst-italic text-muted">No further weeks scheduled</td>
      </tr>`;
    }
    rows += `<tr>
      <td class="text-muted pe-3 text-nowrap">Handicap</td>
      <td class="fst-italic text-muted">${esc(hc.message)}</td>
    </tr>`;
    return `<div class="mt-3 pt-2 border-top">
      <p class="small fw-semibold text-secondary mb-1"><i class="bi bi-calendar-arrow-up me-1"></i>Advance Preview</p>
      <table class="table table-sm table-borderless small mb-0"><tbody>${rows}</tbody></table>
      ${this.#renderHandicapRecs(hc)}
    </div>`;
  }

  #renderCloseSuccess(closeData, weekNum) {
    const ar       = closeData.advance_result || {};
    const cw       = ar.closed_week || {};
    const nw       = ar.next_week;
    const nextNum  = ar.next_week_number;
    const hc       = ar.handicap || {};
    const ackCount = closeData.acknowledgment_count || 0;

    let rows = `<tr>
      <td class="text-muted pe-3 text-nowrap">Week ${esc(String(weekNum))}</td>
      <td>${esc(String(cw.completed_count || 0))}/${esc(String(cw.match_count || 0))} scored &nbsp;<span class="badge bg-success">Closed</span></td>
    </tr>`;
    if (nw) {
      const assignNote = nw.unassigned_count > 0
        ? ` &middot; <span class="text-warning">${nw.unassigned_count} unassigned</span>` : '';
      const lineupNote = nw.missing_lineup_team_ids && nw.missing_lineup_team_ids.length > 0
        ? ` &middot; <span class="text-warning">${nw.missing_lineup_team_ids.length} missing lineup</span>`
        : ' &middot; <span class="text-success">Lineups set</span>';
      rows += `<tr>
        <td class="text-muted pe-3 text-nowrap">Week ${esc(String(nextNum))}</td>
        <td>${esc(String(nw.match_count))} match${nw.match_count !== 1 ? 'es' : ''}${assignNote}${lineupNote}</td>
      </tr>`;
    } else {
      rows += `<tr>
        <td class="text-muted pe-3 text-nowrap">Next week</td>
        <td class="fst-italic text-muted">No further weeks scheduled</td>
      </tr>`;
    }
    rows += `<tr>
      <td class="text-muted pe-3 text-nowrap">Handicap</td>
      <td class="fst-italic text-muted">${esc(hc.message || 'No changes applied.')}</td>
    </tr>`;

    const ackNote = ackCount > 0
      ? ' (' + ackCount + ' warning' + (ackCount !== 1 ? 's' : '') + ' acknowledged)' : '';
    return `<div class="alert alert-success py-2 mb-3 small d-flex align-items-center gap-2">
      <i class="bi bi-check-circle-fill flex-shrink-0"></i>
      <span><strong>Week ${esc(String(weekNum))} closed.</strong>${esc(ackNote)} Standings and player stats updated.</span>
    </div>
    <table class="table table-sm table-borderless small mb-0"><tbody>${rows}</tbody></table>
    ${this.#renderHandicapRecs(hc)}`;
  }

  #cwmUpdateConfirmState() {
    const checks = document.querySelectorAll('#close-week-modal-body .cwm-ack-check');
    document.getElementById('close-week-confirm-btn').disabled = [...checks].some(c => !c.checked);
  }

  async #reviewCloseWeek(seasonId, weekNum, ackCount = 0) {
    let result, preview;
    try {
      [result, preview] = await Promise.all([
        fetchWeekValidation(seasonId, weekNum),
        fetchAdvancePreview(seasonId, weekNum).catch(() => null),
      ]);
    } catch (e) {
      toast('Failed to load week data: ' + e.message, 'danger');
      return;
    }

    const msgs     = result.messages || [];
    const errors   = msgs.filter(m => m.level === 'error');
    const warnings = msgs.filter(m => m.level === 'warning');

    let body = '';

    if (ackCount > 0) {
      const label = ackCount === 1 ? '1 prior acknowledgment' : esc(String(ackCount)) + ' prior acknowledgments';
      body += `<div class="alert alert-secondary py-2 mb-3 small">
        <i class="bi bi-clock-history me-1"></i>
        This week has ${label} from a previous close.
        <button type="button" class="btn btn-link btn-sm p-0 ms-1" data-action="load-prior-acks" data-season-id="${esc(String(seasonId))}" data-week-num="${esc(String(weekNum))}">View</button>
        <div class="mt-2 d-none" id="cwm-prior-acks" data-loaded="0"></div>
      </div>`;
    }

    if (errors.length) {
      const weekErrors  = errors.filter(m => !m.match_id);
      const matchGroups = {};
      errors.filter(m => m.match_id).forEach(m => {
        (matchGroups[m.match_id] = matchGroups[m.match_id] || []).push(m);
      });
      let errHtml = weekErrors.length
        ? `<ul class="mt-1 mb-2">${weekErrors.map(m => `<li>${esc(m.message)}</li>`).join('')}</ul>`
        : '';
      Object.entries(matchGroups).forEach(([mid, grpMsgs]) => {
        errHtml += `<div class="mb-2"><button class="badge bg-warning text-dark border-0 me-1" data-action="open-match-entry" data-season-id="${esc(String(seasonId))}" data-match-id="${mid}" title="Go to match entry">Match ${mid}</button><ul class="mb-0 mt-1">${grpMsgs.map(m => `<li>${esc(m.message)}</li>`).join('')}</ul></div>`;
      });
      body += `<div class="mb-3"><strong class="text-danger"><i class="bi bi-x-circle me-1"></i>Errors - must fix before close</strong>${errHtml}</div>`;
    }

    if (warnings.length && !errors.length) {
      const ackItems = warnings.map((w, i) => {
        const matchLabel = w.match_id
          ? `<span class="badge bg-warning text-dark me-1">Match ${w.match_id}</span>`
          : '';
        const fieldLabel = w.field ? ` <span class="text-muted small">(${esc(w.field)})</span>` : '';
        return `<div class="border rounded p-2 mb-2">
          <div class="form-check mb-1">
            <input class="form-check-input cwm-ack-check" type="checkbox" id="cwm-ack-${i}" data-idx="${i}">
            <label class="form-check-label fw-semibold" for="cwm-ack-${i}">
              ${matchLabel}${esc(w.code)}${fieldLabel}
            </label>
          </div>
          <div class="ps-3 text-muted small mb-1">${esc(w.message)}</div>
          <input type="text" class="form-control form-control-sm cwm-ack-notes" data-idx="${i}" placeholder="Notes (optional)">
        </div>`;
      }).join('');
      body += `<div class="mb-3">
        <strong class="text-warning"><i class="bi bi-exclamation-triangle me-1"></i>Warnings - acknowledge each before close</strong>
        <div class="mt-2">${ackItems}</div>
      </div>`;
    } else if (warnings.length) {
      const warnItems = warnings.map(w => {
        const matchBadge = w.match_id ? `<span class="badge bg-warning text-dark me-1">Match ${w.match_id}</span>` : '';
        const fieldLabel = w.field ? ` <span class="text-muted small">(${esc(w.field)})</span>` : '';
        return `<li>${matchBadge}${esc(w.message)}${fieldLabel}</li>`;
      }).join('');
      body += `<div class="mb-3"><strong class="text-warning"><i class="bi bi-exclamation-triangle me-1"></i>Warnings</strong><ul class="mt-1 mb-0">${warnItems}</ul></div>`;
    }

    if (!errors.length && !warnings.length) {
      body = '<p class="text-success mb-2"><i class="bi bi-check-circle me-1"></i>All checks passed. Ready to close.</p>';
    }
    if (!errors.length) {
      body += '<div class="alert alert-info py-2 mb-0 small"><i class="bi bi-info-circle me-1"></i>Closing makes this week\'s results official. Standings and player stats will update immediately and will reflect this week\'s outcomes.</div>';
    }
    if (preview) body += this.#renderAdvancePreview(preview);

    const modal     = new bootstrap.Modal(document.getElementById('close-week-modal'));
    const modalBody = document.getElementById('close-week-modal-body');
    modalBody.innerHTML = body;
    const confirmBtn = document.getElementById('close-week-confirm-btn');

    confirmBtn.disabled    = errors.length > 0 || warnings.length > 0;
    confirmBtn.textContent = (warnings.length && !errors.length) ? 'Acknowledge & Close' : 'Confirm Close';
    modalBody.querySelectorAll('.cwm-ack-check').forEach(chk => {
      chk.addEventListener('change', () => this.#cwmUpdateConfirmState());
    });

    confirmBtn.onclick = async () => {
      const acks = [];
      if (warnings.length && !errors.length) {
        warnings.forEach((w, i) => {
          const noteEl = modalBody.querySelector(`.cwm-ack-notes[data-idx="${i}"]`);
          acks.push({
            match_id:     w.match_id || 0,
            warning_code: w.code,
            field:        w.field || '',
            notes:        noteEl ? noteEl.value.trim() : '',
          });
        });
      }
      try {
        const closeData = await closeWeek(seasonId, weekNum, { acknowledgments: acks });
        modalBody.innerHTML    = this.#renderCloseSuccess(closeData, weekNum);
        confirmBtn.textContent = 'Done';
        confirmBtn.disabled    = false;
        confirmBtn.onclick     = () => modal.hide();
        await this.#loadSchedule();
        this.#dispatchDataChanged();
      } catch (e) {
        toast('Close failed: ' + (e.message || e), 'danger');
      }
    };
    modal.show();
  }

  #confirmReopenWeek(seasonId, weekNum, matchDate) {
    const modalBody = document.getElementById('reopen-week-modal-body');
    const dateStr   = matchDate ? ' (' + fmtDate(matchDate) + ')' : '';
    modalBody.innerHTML = `
      <p class="mb-2">Are you sure you want to reopen Week ${weekNum}${esc(dateStr)}?</p>
      <div class="alert alert-warning mb-0">
        <i class="bi bi-exclamation-triangle me-1"></i>
        <strong>Reopening this week will:</strong>
        <ul class="mb-0 mt-1">
          <li>Remove it from official standings and player stats until re-closed</li>
          <li>Allow scores to be corrected or re-saved</li>
          <li>Preserve all previously saved scores (nothing is deleted)</li>
          <li>Preserve prior warning acknowledgments as historical record</li>
          <li>Allow the week to be re-closed normally when corrections are complete</li>
        </ul>
      </div>`;
    const modal = new bootstrap.Modal(document.getElementById('reopen-week-modal'));
    this.#reopenWeekCtx = { seasonId, weekNum, modal };
    modal.show();
  }

  #openAssignModal(matchId) {
    this.#assignMatchId = matchId;
    const opts = '<option value="">(unassigned)</option>' +
      this.#allTeams.map(t => `<option value="${t.id}">${esc(t.name)}</option>`).join('');
    document.getElementById('assign-home-select').innerHTML = opts;
    document.getElementById('assign-away-select').innerHTML = opts;
    new bootstrap.Modal(document.getElementById('assign-match-modal')).show();
  }

  async #confirmAssign() {
    const matchId = this.#assignMatchId;
    if (!matchId) return;
    const homeId = document.getElementById('assign-home-select').value;
    const awayId = document.getElementById('assign-away-select').value;
    try {
      await assignMatchTeams(matchId, {
        home_team_id: homeId ? parseInt(homeId) : null,
        away_team_id: awayId ? parseInt(awayId) : null,
      });
      bootstrap.Modal.getInstance(document.getElementById('assign-match-modal'))?.hide();
      toast('Teams assigned');
      this.#assignMatchId = null;
      await this.#loadSchedule();
    } catch (e) { toast(e.message, 'danger'); }
  }

  async #openPosterView() {
    const seasonId = this.querySelector('.sp-season-sel')?.value;
    if (!seasonId) { toast('Select a season first', 'warning'); return; }

    let matches, players;
    try {
      [matches, players] = await Promise.all([
        fetchSeasonMatches(seasonId),
        fetchLeaguePlayers(this.#activeLeague.id),
      ]);
    } catch (e) { toast(e.message, 'danger'); return; }

    if (!matches.length) { toast('No matches scheduled yet', 'warning'); return; }

    const season  = this.#allSeasons.find(s => s.id == seasonId);
    const teamMap = {};
    matches.forEach(m => {
      if (m.home_team_id) teamMap[m.home_team_id] = m.home_team_name;
      if (m.away_team_id) teamMap[m.away_team_id] = m.away_team_name;
    });
    const seasonTeams = Object.entries(teamMap)
      .map(([id, name]) => ({ id: parseInt(id), name }))
      .sort((a, b) => a.name.localeCompare(b.name));
    const teamNum = {};
    seasonTeams.forEach((t, i) => teamNum[t.id] = i + 1);

    const playersByTeam = {};
    players.forEach(p => {
      if (p.team_id && teamMap[p.team_id]) {
        (playersByTeam[p.team_id] = playersByTeam[p.team_id] || []).push(p.name);
      }
    });

    const byWeek = {};
    matches.forEach(m => { (byWeek[m.week_number] = byWeek[m.week_number] || []).push(m); });
    const weeks = Object.keys(byWeek).sort((a, b) => a - b);

    const balls = seasonTeams.slice(0, 8).map((_, i) => {
      const b = POSTER_BALLS[i];
      return `<div class="poster-ball" style="background:${b.bg};color:${b.fg}">${i + 1}</div>`;
    }).join('');

    const rows = weeks.map(wn => {
      const wMatches  = byWeek[wn].sort((a, b) => (a.match_number ?? 999) - (b.match_number ?? 999));
      const dateStr   = posterDateLabel(wMatches[0].match_date, wn);
      const playingIds = new Set();
      wMatches.forEach(m => { playingIds.add(m.home_team_id); playingIds.add(m.away_team_id); });
      const byeTeams = seasonTeams.filter(t => !playingIds.has(t.id));
      const byeStr   = byeTeams.length ? 'BYE: ' + byeTeams.map(t => teamNum[t.id]).join(', ') : '';
      const pairings = wMatches.map(m => {
        const h  = teamNum[m.home_team_id] || '?';
        const a  = teamNum[m.away_team_id] || '?';
        const mn = m.match_number != null ? `<sup class="poster-matchnum">${m.match_number}</sup>` : '';
        return `<span class="poster-pairing">${h}-${a}${mn}</span>`;
      }).join('');
      return `<tr>
        <td class="poster-wk">${wn}</td>
        <td class="poster-date">${dateStr}</td>
        <td class="poster-pairings">${pairings}</td>
        <td class="poster-bye-col">${byeStr ? `<span class="poster-bye">${byeStr}</span>` : ''}</td>
      </tr>`;
    }).join('');

    const teamCards = seasonTeams.map(t => {
      const pls = (playersByTeam[t.id] || []).join(', ');
      return `<div>
        <div class="poster-team-name">${teamNum[t.id]}. ${esc(t.name)}</div>
        ${pls ? `<div class="poster-team-players">${esc(pls)}</div>` : ''}
      </div>`;
    }).join('');

    const leagueName = this.#activeLeague?.name || 'League';
    const shortName  = leagueName.replace(/\bLeague\b/i, '').trim();
    const title      = shortName ? shortName + ' Schedule' : 'Schedule';
    const subtitle   = season?.name || '';

    this.querySelector('.sp-poster-content').innerHTML = `
      <div class="poster-outer">
        ${pocketHTML()}
        <div class="poster-balls">${balls}</div>
        <div class="poster-inner">
          <div class="poster-title">${esc(title)}</div>
          ${subtitle ? `<div class="poster-subtitle">${esc(subtitle)}</div>` : ''}
          <hr class="poster-divider">
          <table class="poster-sched">
            <thead><tr>
              <th class="poster-wk">Wk</th>
              <th class="poster-date">Date</th>
              <th class="poster-pairings">Pairings</th>
              <th class="poster-bye-col">Bye</th>
            </tr></thead>
            <tbody>${rows}</tbody>
          </table>
        </div>
      </div>
      <div class="poster-teams-box">
        ${pocketHTML()}
        <div class="poster-inner">
          <div class="poster-teams-hdr">Teams</div>
          <div class="poster-teams-grid">${teamCards}</div>
        </div>
      </div>`;

    this.querySelector('.sp-table-view').classList.add('d-none');
    this.querySelector('.sp-poster-view').classList.remove('d-none');
  }

  #closePosterView() {
    this.querySelector('.sp-poster-view').classList.add('d-none');
    this.querySelector('.sp-table-view').classList.remove('d-none');
  }

  #dispatchDataChanged() {
    this.dispatchEvent(new CustomEvent('schedule-data-changed', { bubbles: true }));
  }
}

customElements.define('schedule-page', SchedulePage);
