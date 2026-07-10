// <match-entry-page> - Match Entry section coordinator.
//
// Public API:
//   refresh(allSeasons, activeSeason, allPlayers, activeLeague,
//           preSelectSeasonId, preSelectMatchId)
//     Called by the app shell when Match Entry activates or league context
//     changes. preSelectSeasonId/preSelectMatchId come from openMatchEntry()
//     cross-section navigation and are consumed once then cleared by the shell.

import {
  fetchSeasonMatches,
  fetchMatch,
  fetchSeasonRules,
  fetchRounds,
  fetchLineupPlans,
  saveRounds,
  clearMatchResults as apiClearResults,
} from './match-entry-api-service.js';
import { fmtDate } from '../../components/date-display.js';
import { GAME_FORMAT_8BALL } from '../leagues/game-format-codes.js';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

function fmtHC(v) { return (v >= 0 ? '+' : '') + v; }

class MatchEntryPage extends HTMLElement {
  #allPlayers   = [];
  #activeLeague = null;
  #currentMatch = null;
  #homeTeam     = [];
  #awayTeam     = [];
  #games        = [];
  #seasonRules  = {};

  connectedCallback() {
    this.innerHTML = `
      <h4 class="mb-3 fw-bold">Match Entry</h4>
      <div class="row g-2 mb-3 align-items-end">
        <div class="col-auto">
          <label class="form-label small mb-1">Season</label>
          <select class="form-select form-select-sm me-season-sel"></select>
        </div>
        <div class="col-auto">
          <label class="form-label small mb-1">Match</label>
          <select class="form-select form-select-sm me-match-sel"></select>
        </div>
      </div>
      <div class="me-lineup d-none"></div>
      <div class="me-scoresheet d-none"></div>`;

    this.addEventListener('change', e => {
      if (e.target.matches('.me-season-sel')) this.#loadMatches();
      if (e.target.matches('.me-match-sel'))  this.#loadMatchEntry();
    });
    this.addEventListener('click', e => {
      if (e.target.closest('[data-action="confirm-lineup"]'))   { this.#confirmLineup(); return; }
      if (e.target.closest('[data-action="save-scoresheet"]'))  { this.#saveScoresheet(); return; }
      if (e.target.closest('[data-action="print-scoresheet"]')) { this.#printScoresheet(); return; }
      if (e.target.closest('[data-action="change-lineup"]'))    { this.#loadMatchEntry(); return; }
      if (e.target.closest('[data-action="go-lineup"]'))        { e.preventDefault(); navTo('lineup'); return; }
      const clearBtn = e.target.closest('[data-action="clear-results"]');
      if (clearBtn) { this.#clearResults(parseInt(clearBtn.dataset.matchId)); }
    });
    this.addEventListener('input', e => {
      if (e.target.matches('.ss-score-inp')) this.#ssInpChange(e.target);
    });
    this.addEventListener('keydown', e => {
      if (e.target.matches('.ss-score-inp')) this.#ssInpKey(e);
    });
  }

  refresh(allSeasons, activeSeason, allPlayers, activeLeague,
          preSelectSeasonId = null, preSelectMatchId = null) {
    this.#allPlayers   = allPlayers   ?? [];
    this.#activeLeague = activeLeague;
    this.#populateSeasonSelect(allSeasons ?? [], preSelectSeasonId);
    this.#loadMatches(preSelectMatchId);
  }

  #populateSeasonSelect(allSeasons, preSelectSeasonId) {
    const sel = this.querySelector('.me-season-sel');
    if (!sel) return;
    sel.innerHTML = allSeasons.map(s =>
      `<option value="${s.id}"${s.active ? ' selected' : ''}>${esc(s.name)}</option>`
    ).join('') || '<option value="">No seasons</option>';
    if (preSelectSeasonId != null) sel.value = String(preSelectSeasonId);
  }

  async #loadMatches(preSelectMatchId = null) {
    const seasonId = this.querySelector('.me-season-sel')?.value;
    if (!seasonId) return;

    // Preserve current match selection unless an explicit preselect is given.
    const keepId = preSelectMatchId ?? this.querySelector('.me-match-sel')?.value ?? null;

    let matches;
    try { matches = await fetchSeasonMatches(seasonId); }
    catch (e) { toast(e.message, 'danger'); return; }

    const sel = this.querySelector('.me-match-sel');
    sel.innerHTML = matches.map(m =>
      `<option value="${m.id}">[W${m.week_number}] ${esc(m.home_team_name)} vs ${esc(m.away_team_name)}${m.completed ? ' (Done)' : ''}</option>`
    ).join('') || '<option>No matches</option>';

    if (keepId !== null) sel.value = String(keepId);
    await this.#loadMatchEntry();
  }

  async #loadMatchEntry() {
    const matchId = this.querySelector('.me-match-sel')?.value;
    if (!matchId) return;

    const lineupDiv     = this.querySelector('.me-lineup');
    const scoresheetDiv = this.querySelector('.me-scoresheet');
    lineupDiv.classList.add('d-none');
    scoresheetDiv.classList.add('d-none');
    scoresheetDiv.innerHTML = '';

    let detail;
    try { detail = await fetchMatch(matchId); }
    catch (e) { toast(e.message, 'danger'); return; }

    this.#currentMatch = detail.match;
    const m = this.#currentMatch;

    this.#seasonRules = {};
    try {
      const rulesList = await fetchSeasonRules(m.season_id);
      for (const r of rulesList) this.#seasonRules[r.rule_key] = r.rule_value;
    } catch (_) {}

    if (!this.#activeLeague || this.#activeLeague.game_format !== GAME_FORMAT_8BALL) {
      scoresheetDiv.classList.remove('d-none');
      scoresheetDiv.innerHTML = '<div class="text-muted p-3">9-ball scoresheet coming soon.</div>';
      return;
    }

    const homePlayers = this.#allPlayers.filter(p => p.team_id == m.home_team_id);
    const awayPlayers = this.#allPlayers.filter(p => p.team_id == m.away_team_id);

    // 1. Existing round results take highest priority (already played).
    let existingRounds = [];
    try { existingRounds = await fetchRounds(matchId); } catch (_) {}
    if (existingRounds.length > 0) {
      const r1 = existingRounds.filter(r => r.round_number === 1).sort((a, b) => a.id - b.id);
      if (r1.length === 3) {
        const hp = r1.map(r => homePlayers.find(p => p.id === r.home_player_id)).filter(Boolean);
        const ap = r1.map(r => awayPlayers.find(p => p.id === r.away_player_id)).filter(Boolean);
        if (hp.length === 3 && ap.length === 3) {
          this.#homeTeam = hp;
          this.#awayTeam = ap;
          this.#renderScoresheet(existingRounds);
          return;
        }
      }
    }

    // 2. Lineup plans - week-specific, then fall back to default (week 0).
    let weekPlans = [], defaultPlans = [];
    try { weekPlans    = await fetchLineupPlans(m.season_id, m.week_number); } catch (_) {}
    try { defaultPlans = await fetchLineupPlans(m.season_id, 0); } catch (_) {}

    const resolvePlans = teamId => {
      const week = weekPlans.filter(p => p.team_id == teamId);
      return week.length >= 3 ? week : defaultPlans.filter(p => p.team_id == teamId);
    };
    const homePlans = resolvePlans(m.home_team_id);
    const awayPlans = resolvePlans(m.away_team_id);

    if (homePlans.length >= 3 && awayPlans.length >= 3) {
      const hp = homePlans.slice(0, 3).map(lp => homePlayers.find(p => p.id === lp.player_id)).filter(Boolean);
      const ap = awayPlans.slice(0, 3).map(lp => awayPlayers.find(p => p.id === lp.player_id)).filter(Boolean);
      if (hp.length === 3 && ap.length === 3) {
        this.#homeTeam = hp;
        this.#awayTeam = ap;
        this.#resetGames();
        this.#renderScoresheet([]);
        return;
      }
    }

    // 3. Fallback: inline quick picker.
    this.#showLineupPicker(m, homePlayers, awayPlayers, homePlans, awayPlans);
  }

  #showLineupPicker(m, homePlayers, awayPlayers, homePlans = [], awayPlans = []) {
    const lineupDiv = this.querySelector('.me-lineup');
    lineupDiv.classList.remove('d-none');

    const makeOpts = (players, plans, idx) => {
      const preselect = plans[idx]?.player_id;
      return players.map(p =>
        `<option value="${p.id}"${p.id == preselect ? ' selected' : ''}>${esc(p.name)} (${p.handicap >= 0 ? '+' : ''}${p.handicap})</option>`
      ).join('');
    };

    const hasHomePlans = homePlans.length > 0;
    const hasAwayPlans = awayPlans.length > 0;
    const hint = (hasHomePlans || hasAwayPlans)
      ? '<span class="badge bg-info text-dark ms-2" style="font-size:.7rem">Pre-filled from saved lineup</span>'
      : '<span class="text-muted fw-normal small">Set in advance on the <a href="#" data-action="go-lineup">Lineup</a> page</span>';

    lineupDiv.innerHTML = `<div class="card mb-3">
      <div class="card-header fw-semibold py-2 d-flex justify-content-between align-items-center">
        <span>Confirm Tonight's Lineup</span>${hint}
      </div>
      <div class="card-body">
        <div class="row g-3">
          <div class="col-md-6">
            <h6 class="fw-bold mb-2" style="color:#1a5cb8">Home: ${esc(m.home_team_name)}</h6>
            ${[1, 2, 3].map(i => `<div class="d-flex align-items-center gap-2 mb-2">
              <span class="text-muted fw-bold" style="width:22px">H${i}</span>
              <select class="form-select form-select-sm" id="me-ph-${i}">
                <option value="">-- select --</option>${makeOpts(homePlayers, homePlans, i - 1)}
              </select></div>`).join('')}
          </div>
          <div class="col-md-6">
            <h6 class="fw-bold mb-2" style="color:#9b1c1c">Away: ${esc(m.away_team_name)}</h6>
            ${[1, 2, 3].map(i => `<div class="d-flex align-items-center gap-2 mb-2">
              <span class="text-muted fw-bold" style="width:22px">A${i}</span>
              <select class="form-select form-select-sm" id="me-pa-${i}">
                <option value="">-- select --</option>${makeOpts(awayPlayers, awayPlans, i - 1)}
              </select></div>`).join('')}
          </div>
        </div>
        <button class="btn btn-primary mt-2" data-action="confirm-lineup">
          <i class="bi bi-arrow-right"></i> Go to Scoresheet
        </button>
      </div>
    </div>`;
  }

  #confirmLineup() {
    const m = this.#currentMatch;
    if (!m) return;
    const homePlayers = this.#allPlayers.filter(p => p.team_id == m.home_team_id);
    const awayPlayers = this.#allPlayers.filter(p => p.team_id == m.away_team_id);

    const hp = [1, 2, 3].map(i => homePlayers.find(p => p.id == this.querySelector(`#me-ph-${i}`)?.value)).filter(Boolean);
    const ap = [1, 2, 3].map(i => awayPlayers.find(p => p.id == this.querySelector(`#me-pa-${i}`)?.value)).filter(Boolean);

    if (hp.length !== 3) { toast('Select all 3 home players', 'warning'); return; }
    if (ap.length !== 3) { toast('Select all 3 away players', 'warning'); return; }
    if (new Set(hp.map(p => p.id)).size !== 3) { toast('Duplicate home player', 'warning'); return; }
    if (new Set(ap.map(p => p.id)).size !== 3) { toast('Duplicate away player', 'warning'); return; }

    this.#homeTeam = hp;
    this.#awayTeam = ap;
    this.#resetGames();
    this.querySelector('.me-lineup').classList.add('d-none');
    this.#renderScoresheet([]);
  }

  #resetGames() {
    this.#games = Array.from({ length: 9 }, () =>
      ({ g1w: '', g1lb: 0, g2w: '', g2lb: 0, g3w: '', g3lb: 0 })
    );
  }

  #gameScores(winner, lb) {
    if (!winner) return { h: 0, a: 0 };
    return winner === 'home' ? { h: 10, a: lb } : { h: lb, a: 10 };
  }

  #calcHandicap(hHC, aHC) {
    const multiplier = parseFloat(this.#seasonRules['handicap_multiplier']) || 2.55;
    const minBall    = parseInt(this.#seasonRules['min_ball_handicap']) || 0;
    const to         = hHC < aHC ? 'home' : aHC < hHC ? 'away' : '';
    const diff       = Math.abs(hHC - aHC);
    const rawPts     = Math.round(diff * multiplier);
    const pts        = !to ? 0 : (minBall > 0 && rawPts < minBall) ? 0 : rawPts;
    return { pts, to };
  }

  #pairingTotals(idx) {
    const g   = this.#games[idx];
    const ri  = Math.floor(idx / 3);
    const pi  = idx % 3;
    const hp  = this.#homeTeam[pi];
    const ap  = this.#awayTeam[(pi + ri) % 3];
    const s1  = this.#gameScores(g.g1w, g.g1lb);
    const s2  = this.#gameScores(g.g2w, g.g2lb);
    const s3  = this.#gameScores(g.g3w, g.g3lb);
    const rawH = s1.h + s2.h + s3.h;
    const rawA = s1.a + s2.a + s3.a;
    const hc   = this.#calcHandicap(hp.handicap, ap.handicap);
    const adjH = rawH + (hc.to === 'home' ? hc.pts : 0);
    const adjA = rawA + (hc.to === 'away' ? hc.pts : 0);
    const hGames    = [g.g1w, g.g2w, g.g3w].filter(w => w === 'home').length;
    const aGames    = [g.g1w, g.g2w, g.g3w].filter(w => w === 'away').length;
    const played    = hGames + aGames;
    const hasScore  = played > 0;
    const remaining = 3 - played;
    let winner = '';
    if (hasScore) {
      if      (adjH > adjA + remaining * 10) winner = 'home';
      else if (adjA > adjH + remaining * 10) winner = 'away';
      else if (remaining === 0) {
        if      (hGames > aGames) winner = 'home';
        else if (aGames > hGames) winner = 'away';
      }
    }
    return { rawH, rawA, adjH, adjA, hc, winner, hp, ap, hasScore };
  }

  #renderScoresheet(existingRounds) {
    const m    = this.#currentMatch;
    const home = this.#homeTeam;
    const away = this.#awayTeam;

    this.#resetGames();
    existingRounds.forEach(rr => {
      const ri = rr.round_number - 1;
      const pi = home.findIndex(p => p.id === rr.home_player_id);
      if (pi < 0) return;
      const idx  = ri * 3 + pi;
      const from = (hs, as) => hs === 10 ? { w: 'home', lb: as } : as === 10 ? { w: 'away', lb: hs } : { w: '', lb: 0 };
      const g1 = from(rr.game1_home, rr.game1_away);
      const g2 = from(rr.game2_home, rr.game2_away);
      const g3 = from(rr.game3_home, rr.game3_away);
      this.#games[idx] = { g1w: g1.w, g1lb: g1.lb, g2w: g2.w, g2lb: g2.lb, g3w: g3.w, g3lb: g3.lb };
    });

    const leagueName = this.#activeLeague?.name || 'League';

    let html = `<div class="no-print d-flex justify-content-between align-items-center mb-2 gap-2 flex-wrap">
      <div class="d-flex align-items-center gap-2">
        <span class="fw-semibold small">Week ${m.week_number}${m.match_date ? ' - ' + fmtDate(m.match_date) : ''}</span>
        ${m.completed
          ? '<span class="badge bg-success">Completed</span>'
          : '<span class="badge bg-secondary">Pending</span>'}
      </div>
      <div class="d-flex gap-2">
        <button class="btn btn-sm btn-outline-secondary" data-action="change-lineup">
          <i class="bi bi-arrow-left"></i> Change Lineup</button>
        <button class="btn btn-sm btn-outline-secondary" data-action="print-scoresheet">
          <i class="bi bi-printer"></i> Print</button>
        <button class="btn btn-sm btn-success" data-action="save-scoresheet">
          <i class="bi bi-check-lg"></i> Save</button>
        ${m.completed ? `<button class="btn btn-sm btn-outline-danger" data-action="clear-results" data-match-id="${m.id}">
          <i class="bi bi-x-lg"></i> Clear</button>` : ''}
      </div>
    </div>`;

    html += `<div id="ss-print-area"><div class="ss-sheet">`;

    html += `<div class="ss-title-bar">
      <span class="ss-main-title">${esc(leagueName)} Scoresheet</span>
      <div class="ss-title-meta">
        <span>${fmtDate(m.match_date, 'Date TBD')}</span>
        ${m.match_number != null ? `<span class="ss-match-num">${m.match_number}</span>` : ''}
      </div>
    </div>`;

    html += `<table class="ss-teams-tbl">
      <tr>
        <td class="ss-tn-name" style="color:#1e3a8a">${esc(m.home_team_name || '')}</td>
        <td class="ss-tn-label">Home Team</td>
        <td class="ss-tn-sep"></td>
        <td class="ss-tn-name" style="color:#7f1d1d">${esc(m.away_team_name || '')}</td>
        <td class="ss-tn-label">Visiting Team</td>
      </tr>
      <tr>
        <td colspan="5" class="ss-tn-table">Table: ${esc(m.table_numbers || '')}</td>
      </tr>
    </table>`;

    html += `<table class="ss-roster-tbl">
      <thead><tr>
        <th colspan="4" style="text-align:center;color:#1e3a8a">Home Team</th>
        <th class="ss-roster-sep"></th>
        <th colspan="4" style="text-align:center;color:#7f1d1d">Visiting Team</th>
      </tr></thead>
      <tbody>`;
    for (let i = 0; i < 3; i++) {
      const hp = home[i], ap = away[i];
      html += `<tr>
        <td class="ss-slot-h">H${i + 1}</td>
        <td class="ss-pnum-col">${hp.player_number || ''}</td>
        <td>${esc(hp.name)}</td>
        <td class="ss-hc-roster">${fmtHC(hp.handicap)}</td>
        <td class="ss-roster-sep"></td>
        <td class="ss-slot-v">V${i + 1}</td>
        <td class="ss-pnum-col">${ap.player_number || ''}</td>
        <td>${esc(ap.name)}</td>
        <td class="ss-hc-roster">${fmtHC(ap.handicap)}</td>
      </tr>`;
    }
    html += `</tbody></table>`;

    html += `<p class="ss-entry-hint no-print">Enter <strong>10</strong> for the game winner. Loser score is balls made (0&ndash;7).</p>`;

    html += `<table class="ss-score-tbl">
      <thead><tr>
        <th style="width:22px"></th>
        <th class="ss-th-left" style="width:115px">Player</th>
        <th style="width:46px">Game 1</th>
        <th style="width:46px">Game 2</th>
        <th style="width:46px">Game 3</th>
        <th style="width:38px">Score</th>
        <th style="width:42px">Rating</th>
        <th style="width:40px">Ball HC</th>
        <th style="width:48px">Adj&nbsp;Score</th>
        <th style="width:102px">Circle Winner</th>
      </tr></thead>
      <tbody>`;

    for (let r = 0; r < 3; r++) {
      html += `<tr class="ss-rnd-row"><td colspan="10" id="ss-rnd-${r}">Round ${r + 1}</td></tr>`;
      for (let p = 0; p < 3; p++) {
        const hp  = home[p];
        const ap  = away[(p + r) % 3];
        const idx = r * 3 + p;
        html += `<tr class="ss-home-row" id="ss-hrow-${idx}">
          <td class="ss-slot" style="color:#1e3a8a">H${p + 1}</td>
          <td class="ss-pname-cell">${esc(hp.name)}</td>
          <td class="ss-game-cell">${this.#ssGameInput(idx, 1, 'home')}</td>
          <td class="ss-game-cell">${this.#ssGameInput(idx, 2, 'home')}</td>
          <td class="ss-game-cell">${this.#ssGameInput(idx, 3, 'home')}</td>
          <td class="ss-score-cell" id="ss-score-h-${idx}">--</td>
          <td class="ss-rating-cell">${fmtHC(hp.handicap)}</td>
          <td class="ss-hc-cell" id="ss-hc-${idx}" rowspan="2">--</td>
          <td class="ss-adj-cell" id="ss-adj-h-${idx}">--</td>
          <td class="ss-winner-cell" id="ss-winner-${idx}" rowspan="2">
            <div class="cw-opt" id="ss-cw-h-${idx}">&#9675; H</div>
            <div class="cw-opt" id="ss-cw-a-${idx}">&#9675; V</div>
          </td>
        </tr>
        <tr class="ss-away-row" id="ss-arow-${idx}">
          <td class="ss-slot" style="color:#7f1d1d">V${(p + r) % 3 + 1}</td>
          <td class="ss-pname-cell">${esc(ap.name)}</td>
          <td class="ss-game-cell">${this.#ssGameInput(idx, 1, 'away')}</td>
          <td class="ss-game-cell">${this.#ssGameInput(idx, 2, 'away')}</td>
          <td class="ss-game-cell">${this.#ssGameInput(idx, 3, 'away')}</td>
          <td class="ss-score-cell" id="ss-score-a-${idx}">--</td>
          <td class="ss-rating-cell">${fmtHC(ap.handicap)}</td>
          <td class="ss-adj-cell" id="ss-adj-a-${idx}">--</td>
        </tr>`;
      }
    }

    html += `</tbody></table>`;
    html += `<div class="no-print mt-2" id="ss-summary">${this.#buildScoreSummary()}</div>`;
    html += `<div class="ss-sigs">
      <div class="ss-sig-line">Home Team Captain Signature: <span class="ss-sig-blank"></span></div>
      <div class="ss-sig-line">Visiting Team Captain Signature: <span class="ss-sig-blank"></span></div>
    </div>`;
    html += `</div>`; // .ss-sheet
    html += this.#buildScorekeeperPage();
    html += `</div>`; // #ss-print-area

    const sd = this.querySelector('.me-scoresheet');
    sd.innerHTML = html;
    sd.classList.remove('d-none');

    for (let i = 0; i < 9; i++) this.#updateSSPairing(i);
    this.#updateSSFinal();
  }

  #ssGameInput(idx, gn, side) {
    const g   = this.#games[idx];
    const w   = g['g' + gn + 'w'];
    const lb  = g['g' + gn + 'lb'];
    const val = w === side ? '10' : (w && w !== side) ? String(lb) : '';
    return `<input type="number" class="ss-score-inp" id="ss-inp-${idx}-${gn}-${side}"` +
      ` data-idx="${idx}" data-gn="${gn}" data-side="${side}"` +
      ` min="0" max="10"${val !== '' ? ` value="${val}"` : ''}>`;
  }

  #normalizeScoreInput(el) {
    if (!el || el.value === '') return NaN;
    const raw = parseInt(el.value, 10);
    if (isNaN(raw)) { el.value = ''; return NaN; }
    const v = Math.min(10, Math.max(0, raw));
    el.value = String(v);
    return v;
  }

  #ssInpChange(target) {
    const idx  = parseInt(target.dataset.idx);
    const gn   = parseInt(target.dataset.gn);
    const side = target.dataset.side;
    const hEl  = this.querySelector(`#ss-inp-${idx}-${gn}-home`);
    const aEl  = this.querySelector(`#ss-inp-${idx}-${gn}-away`);
    const hv   = this.#normalizeScoreInput(hEl);
    const av   = this.#normalizeScoreInput(aEl);
    const g    = this.#games[idx];
    const wk   = 'g' + gn + 'w';
    const lk   = 'g' + gn + 'lb';
    if (hv === 10 && av !== 10) {
      g[wk] = 'home'; g[lk] = isNaN(av) ? 0 : Math.min(7, Math.max(0, av));
      if (aEl) { aEl.value = String(g[lk]); if (side === 'home') { aEl.focus(); aEl.select(); } }
    } else if (av === 10 && hv !== 10) {
      g[wk] = 'away'; g[lk] = isNaN(hv) ? 0 : Math.min(7, Math.max(0, hv));
      if (hEl) { hEl.value = String(g[lk]); if (side === 'away') { hEl.focus(); hEl.select(); } }
    } else if (hv === 10 && av === 10) {
      g[wk] = side; g[lk] = 0;
      if (side === 'home' && aEl) { aEl.value = '0'; aEl.focus(); aEl.select(); }
      if (side === 'away' && hEl) { hEl.value = '0'; hEl.focus(); hEl.select(); }
    } else {
      g[wk] = ''; g[lk] = 0;
    }
    this.#updateSSPairing(idx);
    this.#updateSSFinal();
  }

  #ssInpKey(e) {
    if (e.key !== 'Tab') return;
    e.preventDefault();
    const idx  = parseInt(e.target.dataset.idx);
    const gn   = parseInt(e.target.dataset.gn);
    const side = e.target.dataset.side;
    if (!e.shiftKey) {
      const nextEl = side === 'home'
        ? this.querySelector(`#ss-inp-${idx}-${gn}-away`)
        : gn < 3
          ? this.querySelector(`#ss-inp-${idx}-${gn + 1}-home`)
          : idx < 8
            ? this.querySelector(`#ss-inp-${idx + 1}-1-home`)
            : null;
      if (nextEl) nextEl.focus();
    } else {
      const prevEl = side === 'away'
        ? this.querySelector(`#ss-inp-${idx}-${gn}-home`)
        : gn > 1
          ? this.querySelector(`#ss-inp-${idx}-${gn - 1}-away`)
          : idx > 0
            ? this.querySelector(`#ss-inp-${idx - 1}-3-away`)
            : null;
      if (prevEl) prevEl.focus();
    }
  }

  #updateSSInputClasses(idx) {
    const g = this.#games[idx];
    for (let gn = 1; gn <= 3; gn++) {
      const w = g['g' + gn + 'w'];
      for (const side of ['home', 'away']) {
        const el = this.querySelector(`#ss-inp-${idx}-${gn}-${side}`);
        if (!el) continue;
        let cls = 'ss-score-inp';
        if (w === side)           cls += ' ss-inp-winner';
        else if (w && w !== side) cls += ' ss-inp-loser';
        else if (el.value === '') cls += ' ss-inp-empty';
        el.className = cls;
      }
    }
  }

  #updateSSPairing(idx) {
    const { rawH, rawA, adjH, adjA, hc, winner, hasScore } = this.#pairingTotals(idx);

    const hcEl = this.querySelector(`#ss-hc-${idx}`);
    if (hcEl) hcEl.textContent = String(hc.pts);

    const sh = this.querySelector(`#ss-score-h-${idx}`);
    const sa = this.querySelector(`#ss-score-a-${idx}`);
    const ah = this.querySelector(`#ss-adj-h-${idx}`);
    const aa = this.querySelector(`#ss-adj-a-${idx}`);
    if (sh) sh.textContent = hasScore ? rawH : '--';
    if (sa) sa.textContent = hasScore ? rawA : '--';
    if (ah) { ah.textContent = hasScore ? adjH : '--';
      ah.className = 'ss-adj-cell' + (winner === 'home' ? ' ss-adj-win' : ''); }
    if (aa) { aa.textContent = hasScore ? adjA : '--';
      aa.className = 'ss-adj-cell' + (winner === 'away' ? ' ss-adj-win' : ''); }

    const cwH = this.querySelector(`#ss-cw-h-${idx}`);
    const cwA = this.querySelector(`#ss-cw-a-${idx}`);
    if (cwH) cwH.className = 'cw-opt' + (winner === 'home' ? ' cw-sel-h' : '');
    if (cwA) cwA.className = 'cw-opt' + (winner === 'away' ? ' cw-sel-a' : '');
    this.#updateSSInputClasses(idx);
  }

  #updateSSFinal() {
    const wrap = this.querySelector('#ss-summary');
    if (wrap) wrap.innerHTML = this.#buildScoreSummary();
    for (let r = 0; r < 3; r++) {
      let rh = 0, ra = 0;
      for (let p = 0; p < 3; p++) {
        const { winner } = this.#pairingTotals(r * 3 + p);
        if (winner === 'home') rh++;
        else if (winner === 'away') ra++;
      }
      const el = this.querySelector(`#ss-rnd-${r}`);
      if (!el) continue;
      if (rh >= 2) {
        el.innerHTML = `Round ${r + 1} <span class="ss-rnd-badge ss-rnd-badge-h">H wins</span>`;
        el.parentElement.className = 'ss-rnd-row ss-rnd-win-h';
      } else if (ra >= 2) {
        el.innerHTML = `Round ${r + 1} <span class="ss-rnd-badge ss-rnd-badge-v">V wins</span>`;
        el.parentElement.className = 'ss-rnd-row ss-rnd-win-v';
      } else {
        el.textContent = `Round ${r + 1}`;
        el.parentElement.className = 'ss-rnd-row';
      }
    }
  }

  #buildScoreSummary() {
    let hw = 0, aw = 0;
    for (let i = 0; i < 9; i++) {
      const { winner } = this.#pairingTotals(i);
      if (winner === 'home') hw++;
      else if (winner === 'away') aw++;
    }
    const m = this.#currentMatch;
    return `<div class="score-summary">
      <div class="text-center">
        <div class="team-name">${esc(m?.home_team_name || 'Home')} (Home)</div>
        <div class="team-score">${hw}</div>
      </div>
      <div class="text-center" style="opacity:.5;font-size:1.2rem">-</div>
      <div class="text-center">
        <div class="team-name">${esc(m?.away_team_name || 'Away')} (Visitor)</div>
        <div class="team-score">${aw}</div>
      </div>
    </div>`;
  }

  #buildScorekeeperPage() {
    const m    = this.#currentMatch;
    const home = this.#homeTeam;
    const away = this.#awayTeam;

    const hStats = Array.from({ length: 3 }, () => ({ gW: 0, gL: 0, sW: 0, sL: 0, diff: 0 }));
    const aStats = Array.from({ length: 3 }, () => ({ gW: 0, gL: 0, sW: 0, sL: 0, diff: 0 }));
    let homeRoundsWon = 0, awayRoundsWon = 0;
    const hasAnyScore = this.#games.some(g => g.g1w || g.g2w || g.g3w);

    for (let r = 0; r < 3; r++) {
      let rh = 0, ra = 0;
      for (let p = 0; p < 3; p++) {
        const idx   = r * 3 + p;
        const g     = this.#games[idx];
        const { rawH, rawA, winner } = this.#pairingTotals(idx);
        const apPos = (p + r) % 3;
        const hs    = hStats[p];
        const as_   = aStats[apPos];
        for (let gn = 1; gn <= 3; gn++) {
          const w = g[`g${gn}w`];
          if (w === 'home') { hs.gW++; as_.gL++; }
          else if (w === 'away') { hs.gL++; as_.gW++; }
        }
        if (winner === 'home')      { hs.sW++; as_.sL++; rh++; }
        else if (winner === 'away') { hs.sL++; as_.sW++; ra++; }
        hs.diff  += rawH - rawA;
        as_.diff += rawA - rawH;
      }
      if (rh > ra) homeRoundsWon++;
      else if (ra > rh) awayRoundsWon++;
    }

    const fmtD = n => n > 0 ? `+${n}` : `${n}`;
    const pRow = (label, player, st) => `
      <tr>
        <td class="ss-p2-pos">${label}</td>
        <td class="ss-p2-pnum">${esc(player.player_number || '')}</td>
        <td>${esc(player.name || '')}</td>
        <td class="ss-p2-num">${st.gW}</td>
        <td class="ss-p2-num">${st.gL}</td>
        <td class="ss-p2-num">${st.sW}</td>
        <td class="ss-p2-num">${st.sL}</td>
        <td class="ss-p2-diff-col">${fmtD(st.diff)}</td>
      </tr>`;

    const dateStr   = fmtDate(m.match_date, 'Date TBD');
    const matchMeta = [
      m.match_number != null ? 'Match ' + esc(String(m.match_number)) : null,
      m.table_numbers        ? 'Table ' + esc(String(m.table_numbers)) : null,
      'Week ' + esc(String(m.week_number)),
    ].filter(Boolean).join(' &middot; ');

    return `<div class="ss-p2-sheet">
      <div class="no-print ss-p2-screen-label">
        <i class="bi bi-printer me-1" aria-hidden="true"></i>Printable scorekeeping summary &mdash; page&nbsp;2 of&nbsp;2
      </div>
      <div class="ss-p2-header">
        <span class="ss-p2-title">Match Scorekeeping</span>
        <div class="ss-p2-meta">
          <span>${esc(dateStr)}</span>
          <span>${matchMeta}</span>
        </div>
      </div>
      <div class="ss-p2-instructions">
        <strong>Rounds Won:</strong> Count the rounds (sets) won by each team &mdash; 3 rounds per match.
        The team with more rounds wins the match.&ensp;
        <strong>Diff:</strong> Each player&rsquo;s raw ball-score total minus their opponent&rsquo;s
        across all their sets, before handicap is applied. Positive means they outscored their opponent on raw balls.&ensp;
        <strong>Handicap:</strong> Spot added to the lower-rated player&rsquo;s raw total.
        Formula: <em>((x&minus;y)&times;3)&times;.85</em> &nbsp;or&nbsp; <em>(x&minus;y)&times;2.55</em>,
        where x and y are the players&rsquo; handicap ratings.
      </div>
      <div class="ss-p2-rounds">
        <div class="ss-p2-round-line">Rounds Won &nbsp;<strong>${esc(m.home_team_name || '')}</strong>: ${hasAnyScore ? homeRoundsWon : '______'}</div>
        <div class="ss-p2-round-line">Rounds Won &nbsp;<strong>${esc(m.away_team_name || '')}</strong>: ${hasAnyScore ? awayRoundsWon : '______'}</div>
      </div>
      <table class="ss-p2-table">
        <thead><tr>
          <th class="ss-p2-pos">Pos</th>
          <th class="ss-p2-pnum">No.</th>
          <th style="text-align:left">Player</th>
          <th class="ss-p2-num">Games<br>Won</th>
          <th class="ss-p2-num">Games<br>Lost</th>
          <th class="ss-p2-num">Sets<br>Won</th>
          <th class="ss-p2-num">Sets<br>Lost</th>
          <th class="ss-p2-diff-col">Diff</th>
        </tr></thead>
        <tbody>
          <tr class="ss-p2-team-row"><td colspan="8">Home &mdash; ${esc(m.home_team_name || '')}</td></tr>
          ${home.map((p, i) => pRow(`${i + 1}H`, p, hStats[i])).join('')}
          <tr class="ss-p2-team-row ss-p2-away-row"><td colspan="8">Visiting &mdash; ${esc(m.away_team_name || '')}</td></tr>
          ${away.map((p, i) => pRow(`${i + 1}V`, p, aStats[i])).join('')}
        </tbody>
      </table>
      <div class="ss-sigs">
        <div class="ss-sig-line">Home Team Capt. Signature: <span class="ss-sig-blank"></span></div>
        <div class="ss-sig-line">Visiting Team Capt. Signature: <span class="ss-sig-blank"></span></div>
      </div>
      <div class="ss-p2-hc-note">Handicap formula: ((x&minus;y)&times;3)&times;.85 &nbsp;&nbsp;or&nbsp;&nbsp; (x&minus;y)&times;2.55</div>
    </div>`;
  }

  #printScoresheet() {
    const area = this.querySelector('#ss-print-area');
    if (area) {
      const oldP2 = area.querySelector('.ss-p2-sheet');
      if (oldP2) {
        const tmp = document.createElement('div');
        tmp.innerHTML = this.#buildScorekeeperPage();
        oldP2.replaceWith(tmp.firstElementChild);
      }
    }
    const s = document.createElement('style');
    s.id = 'ss-page-override';
    s.textContent = '@page { size: letter portrait; margin: 0.45in; }';
    document.head.appendChild(s);
    window.addEventListener('afterprint', () => {
      document.getElementById('ss-page-override')?.remove();
    }, { once: true });
    window.print();
  }

  async #saveScoresheet() {
    const m = this.#currentMatch;
    if (!m) return;
    const rounds = [];
    for (let r = 0; r < 3; r++) {
      for (let p = 0; p < 3; p++) {
        const idx = r * 3 + p;
        const g   = this.#games[idx];
        const hp  = this.#homeTeam[p];
        const ap  = this.#awayTeam[(p + r) % 3];
        const s1  = this.#gameScores(g.g1w, g.g1lb);
        const s2  = this.#gameScores(g.g2w, g.g2lb);
        const s3  = this.#gameScores(g.g3w, g.g3lb);
        rounds.push({
          round_number:   r + 1,
          home_player_id: hp.id,
          away_player_id: ap.id,
          game1_home: s1.h, game1_away: s1.a,
          game2_home: s2.h, game2_away: s2.a,
          game3_home: s3.h, game3_away: s3.a,
        });
      }
    }
    try {
      await saveRounds(m.id, rounds);
      toast('Scoresheet saved');
      await this.#loadMatches();
    } catch (e) { toast(e.message, 'danger'); }
  }

  async #clearResults(matchId) {
    if (!confirm('Clear all results for this match?')) return;
    try {
      await apiClearResults(matchId);
      toast('Results cleared');
      await this.#loadMatchEntry();
    } catch (e) { toast(e.message, 'danger'); }
  }
}

customElements.define('match-entry-page', MatchEntryPage);
