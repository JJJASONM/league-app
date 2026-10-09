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
  approveMatch as apiApproveMatch,
  processMatch as apiProcessMatch,
  unprocessMatch as apiUnprocessMatch,
  unapproveMatch as apiUnapproveMatch,
  setLineupSubstitute,
  clearLineupSubstitute,
} from './match-entry-api-service.js';
import {
  fetchMatchApproval,
  fetchMatchApprovalEvents,
  approveTeamSide,
  withdrawTeamApproval,
  requestTeamCorrection,
} from './team-approval-api-service.js';
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
  #allSeasons   = [];
  #activeLeague = null;
  #currentMatch = null;
  #homeTeam     = [];
  #awayTeam     = [];
  #games        = [];
  #seasonRules  = {};
  // Substitute Workflow Phase 1: the lineup_plans row backing each of the 3
  // home/away scoresheet slots, when known (null when the slot wasn't
  // resolved from a saved lineup_plans row -- e.g. already-scored matches,
  // or a fallback pick with no prior plan). Used to show "Sub for X" and to
  // know which row id to call the substitute endpoint against.
  #homeLineupPlans = [null, null, null];
  #awayLineupPlans = [null, null, null];
  // Most recently fetched lineup_plans rows per side (used by #confirmLineup
  // to recover a slot's row id when the manually-confirmed player matches
  // an already-planned one).
  #homePlansCache = [];
  #awayPlansCache = [];
  // { side: 'home'|'away', index, lineupPlanId } for the open substitute modal.
  #subContext = null;
  // Mirrors #renderScoresheet's canEditScores for the roster table's
  // Substitute/Undo controls, set at the top of each render.
  #scoresheetEditable = false;

  // Player Score Approval Phase 1C: 'admin' (default) or 'player'. Set
  // only by refresh()/refreshForPlayer() -- refresh() always resets this
  // to 'admin' first, so a leftover player-mode session (dual-role
  // identity switching workspaces, or simply reusing this persistent
  // custom element across navigations) can never leave admin-only
  // controls or another player's approval state rendered under a
  // subsequent admin visit.
  #mode = 'admin';
  // { playerId, eligibleHome, eligibleAway } when #mode === 'player';
  // eligibleHome/eligibleAway come from the "My Matches" list row that
  // opened this match (its own ResolveApprovalEligibility-backed flags) --
  // never re-derived here from team_id.
  #playerCtx = null;
  // The last-fetched MatchApprovalSnapshot (player mode only) -- home/away
  // state plus the score_revision every team-approval action and player
  // score write applies to. PM correction (2026-09-30): null whenever it
  // could not be loaded, and a null #approval must never be treated as
  // "no approvals yet" (that would be score_revision 0 with
  // pending/pending) -- every player-mode render checks `!this.#approval`
  // directly and forces the scoresheet read-only (no Save/Clear, no
  // team-approval mutation controls, a Retry action instead) rather than
  // risk a write going out with an undefined revision.
  #approval = null;
  // Player Score Approval Phase 1D: the most recent "match"-scoped
  // approval event's event_type ("admin_accepted" | "admin_override"),
  // fetched only in admin mode and only once the match is already
  // approved_at -- used solely to show whether an already-accepted match
  // was accepted normally or via override. Never consulted for any
  // decision; purely a display fact from the authoritative event log.
  #lastAcceptEvent = null;

  #reset() {
    this.#mode = 'admin';
    this.#playerCtx = null;
    this.#approval = null;
    this.#lastAcceptEvent = null;
    this.#currentMatch = null;
    this.#homeTeam = [];
    this.#awayTeam = [];
    // PM correction (2026-09-30): previously left stale from whichever
    // mode/league was last loaded -- #allSeasons in particular feeds the
    // shared seasonClosed lock calculation in #renderScoresheet, so a
    // leftover admin-selected league's season list could silently report
    // the WRONG season's closed_at state (or none at all) for a player
    // mode match in a different league entirely.
    this.#allSeasons = [];
    this.#allPlayers = [];
    this.#activeLeague = null;
  }

  connectedCallback() {
    this.innerHTML = `
      <h4 class="mb-3 fw-bold">Match Entry</h4>
      <div class="row g-2 mb-3 align-items-end me-admin-picker">
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

    this.#ensureSubstituteModal();
    document.getElementById('me-sub-confirm-btn')
      .addEventListener('click', () => this.#confirmSubstitute());

    this.addEventListener('change', e => {
      if (e.target.matches('.me-season-sel')) this.#loadMatches();
      if (e.target.matches('.me-match-sel'))  this.#loadMatchEntry();
    });
    this.addEventListener('click', e => {
      if (e.target.closest('[data-action="confirm-lineup"]'))   { this.#confirmLineup(); return; }
      if (e.target.closest('[data-action="save-scoresheet"]'))  { this.#saveScoresheet(); return; }
      if (e.target.closest('[data-action="print-scoresheet"]')) { this.#printScoresheet(); return; }
      if (e.target.closest('[data-action="change-lineup"]'))    { this.#reload(); return; }
      if (e.target.closest('[data-action="go-lineup"]'))        { e.preventDefault(); navTo('lineup'); return; }
      const clearBtn = e.target.closest('[data-action="clear-results"]');
      if (clearBtn) { this.#clearResults(parseInt(clearBtn.dataset.matchId)); return; }
      const acceptBtn = e.target.closest('[data-action="accept-match"]');
      if (acceptBtn) { this.#acceptMatch(parseInt(acceptBtn.dataset.matchId)); return; }
      if (e.target.closest('[data-action="retry-admin-approval"]')) { this.#reload(); return; }
      const processBtn = e.target.closest('[data-action="process-match"]');
      if (processBtn) { this.#processMatch(parseInt(processBtn.dataset.matchId)); return; }
      const unprocessBtn = e.target.closest('[data-action="unprocess-match"]');
      if (unprocessBtn) { this.#unprocessMatch(parseInt(unprocessBtn.dataset.matchId)); return; }
      const unapproveBtn = e.target.closest('[data-action="unapprove-match"]');
      if (unapproveBtn) { this.#unapproveMatch(parseInt(unapproveBtn.dataset.matchId)); return; }
      const subBtn = e.target.closest('[data-action="open-substitute"]');
      if (subBtn) { this.#openSubstituteModal(subBtn.dataset.side, parseInt(subBtn.dataset.index, 10)); return; }
      const undoSubBtn = e.target.closest('[data-action="clear-substitute"]');
      if (undoSubBtn) { e.preventDefault(); this.#clearSubstitute(undoSubBtn.dataset.side, parseInt(undoSubBtn.dataset.index, 10)); return; }
      // Player Score Approval Phase 1C: team-side approval controls.
      const teamApproveBtn = e.target.closest('[data-action="team-approve"]');
      if (teamApproveBtn) { this.#teamApprove(teamApproveBtn.dataset.side); return; }
      const teamWithdrawBtn = e.target.closest('[data-action="team-withdraw"]');
      if (teamWithdrawBtn) { this.#teamWithdraw(teamWithdrawBtn.dataset.side); return; }
      const teamCorrectionBtn = e.target.closest('[data-action="team-correction"]');
      if (teamCorrectionBtn) { this.querySelector(`#me-correction-${teamCorrectionBtn.dataset.side}`)?.classList.remove('d-none'); return; }
      const teamCorrectionSubmitBtn = e.target.closest('[data-action="team-correction-submit"]');
      if (teamCorrectionSubmitBtn) { this.#teamRequestCorrection(teamCorrectionSubmitBtn.dataset.side); return; }
      if (e.target.closest('[data-action="retry-player-load"]'))  { this.#reload(); return; }
      if (e.target.closest('[data-action="back-to-my-matches"]')) { navTo('my-matches'); return; }
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
    this.#reset();
    this.querySelector('.me-admin-picker')?.classList.remove('d-none');
    this.#allPlayers   = allPlayers   ?? [];
    this.#allSeasons   = allSeasons   ?? [];
    this.#activeLeague = activeLeague;
    this.#populateSeasonSelect(allSeasons ?? [], preSelectSeasonId);
    this.#loadMatches(preSelectMatchId);
  }

  // refreshForPlayer (Player Score Approval Phase 1C) opens one specific
  // match directly in player mode -- no season/match picker (a linked
  // player only ever reaches a match through "My Matches", never a
  // league-wide dropdown), no admin-only controls, and its own
  // league/season/player-list resolution scoped to that match's OWN
  // league (see #loadMatchEntryForPlayer) rather than whatever league an
  // admin previously had selected in this same browser tab.
  // eligibleHome/eligibleAway come from the "My Matches" list row that
  // opened this match -- the authoritative eligibility flags are never
  // re-derived here.
  refreshForPlayer(playerId, matchId, seasonId, eligibleHome, eligibleAway) {
    this.#reset();
    this.#mode = 'player';
    this.#playerCtx = { playerId, eligibleHome: !!eligibleHome, eligibleAway: !!eligibleAway };
    this.querySelector('.me-admin-picker')?.classList.add('d-none');
    this.#loadMatchEntryForPlayer(matchId);
  }

  // #reload() re-fetches and re-renders the currently open match using
  // whichever loader matches the active mode -- shared by every action
  // (substitute confirm/undo, change-lineup, and a stale-revision recovery
  // in player mode) that must reflect a just-made change without knowing
  // or caring which mode is active.
  async #reload() {
    const m = this.#currentMatch;
    if (!m) return;
    if (this.#mode === 'player') await this.#loadMatchEntryForPlayer(m.id);
    else await this.#loadMatchEntry();
  }

  // #resolvePlayer looks up a player by id in #allPlayers (admin mode: the
  // full league roster) first, falling back to the name/handicap already
  // embedded on the round-result or lineup-plan row itself when not found
  // (player mode: #allPlayers is only populated lazily, for the inline
  // picker fallback -- see #loadMatchEntryForPlayer). player_number is
  // only ever known via the full roster, so the fallback leaves it blank
  // rather than guessing.
  #resolvePlayer(id, name, handicap) {
    if (id == null) return null;
    const found = this.#allPlayers.find(p => p.id === id);
    if (found) return found;
    return { id, name: name || '', handicap: handicap || 0, player_number: '' };
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
    this.#homeLineupPlans = [null, null, null];
    this.#awayLineupPlans = [null, null, null];

    let detail;
    try { detail = await fetchMatch(matchId); }
    catch (e) { toast(e.message, 'danger'); return; }

    this.#currentMatch = detail.match;
    const m = this.#currentMatch;

    // Player Score Approval Phase 1D: the admin Accept action and the
    // admin approval panel both require the authoritative approval
    // snapshot -- fetched here unconditionally, same as the player
    // loader, so a failed fetch (#approval stays null) disables Accept
    // rather than risk sending expected_revision from guessed state.
    // #reset() already cleared both fields; only re-populate here, never
    // leave a previous match's snapshot/event rendered against this one.
    try { this.#approval = await fetchMatchApproval(matchId); }
    catch (_) { this.#approval = null; }
    this.#lastAcceptEvent = null;
    if (this.#approval && m.approved_at) {
      try {
        const events = await fetchMatchApprovalEvents(matchId);
        const lastMatchEvent = events.filter(ev => ev.event_scope === 'match').pop();
        this.#lastAcceptEvent = lastMatchEvent ? lastMatchEvent.event_type : null;
      } catch (_) { this.#lastAcceptEvent = null; }
    }

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
    // Resolved against the full player list, not the team roster -- a
    // substitute's player_id may not carry this team's team_id.
    let existingRounds = [];
    try { existingRounds = await fetchRounds(matchId); } catch (_) {}
    if (existingRounds.length > 0) {
      const r1 = existingRounds.filter(r => r.round_number === 1).sort((a, b) => a.id - b.id);
      if (r1.length === 3) {
        const hp = r1.map(r => this.#resolvePlayer(r.home_player_id, r.home_player_name, r.home_handicap)).filter(Boolean);
        const ap = r1.map(r => this.#resolvePlayer(r.away_player_id, r.away_player_name, r.away_handicap)).filter(Boolean);
        if (hp.length === 3 && ap.length === 3) {
          this.#homeTeam = hp;
          this.#awayTeam = ap;
          this.#renderScoresheet(existingRounds);
          return;
        }
      }
    }

    // 2. Lineup plans - week-specific, then fall back to default (week 0).
    // Resolved against the full player list for the same reason as above --
    // a substitute recorded in lineup_plans may not carry this team's
    // team_id, so the team-filtered roster arrays would silently drop them.
    let weekPlans = [], defaultPlans = [];
    try { weekPlans    = await fetchLineupPlans(m.season_id, m.week_number); } catch (_) {}
    try { defaultPlans = await fetchLineupPlans(m.season_id, 0); } catch (_) {}

    const resolvePlans = teamId => {
      const week = weekPlans.filter(p => p.team_id == teamId);
      return week.length >= 3 ? week : defaultPlans.filter(p => p.team_id == teamId);
    };
    const homePlans = resolvePlans(m.home_team_id);
    const awayPlans = resolvePlans(m.away_team_id);
    this.#homePlansCache = homePlans;
    this.#awayPlansCache = awayPlans;

    if (homePlans.length >= 3 && awayPlans.length >= 3) {
      const hp = homePlans.slice(0, 3).map(lp => this.#resolvePlayer(lp.player_id, lp.player_name, lp.handicap)).filter(Boolean);
      const ap = awayPlans.slice(0, 3).map(lp => this.#resolvePlayer(lp.player_id, lp.player_name, lp.handicap)).filter(Boolean);
      if (hp.length === 3 && ap.length === 3) {
        this.#homeTeam = hp;
        this.#awayTeam = ap;
        this.#homeLineupPlans = homePlans.slice(0, 3);
        this.#awayLineupPlans = awayPlans.slice(0, 3);
        this.#resetGames();
        this.#renderScoresheet([]);
        return;
      }
    }

    // 3. Fallback: inline quick picker.
    this.#showLineupPicker(m, homePlayers, awayPlayers, homePlans, awayPlans);
  }

  // #loadMatchEntryForPlayer mirrors #loadMatchEntry for a single match
  // opened in player mode, but resolves its OWN league/season context from
  // the match itself (via its season's league_id) rather than the shell's
  // admin-selected league -- the PM requirement this whole mode exists to
  // satisfy. #allPlayers is deliberately left empty unless the inline
  // lineup-picker fallback (step 3) is actually reached, since steps 1 and
  // 2 already carry each player's name/handicap on the row itself (see
  // #resolvePlayer) and never need the full roster.
  async #loadMatchEntryForPlayer(matchId) {
    const lineupDiv     = this.querySelector('.me-lineup');
    const scoresheetDiv = this.querySelector('.me-scoresheet');
    lineupDiv.classList.add('d-none');
    scoresheetDiv.classList.add('d-none');
    scoresheetDiv.innerHTML = '';
    this.#homeLineupPlans = [null, null, null];
    this.#awayLineupPlans = [null, null, null];
    this.#allPlayers = [];
    this.#approval = null;

    let detail;
    try { detail = await fetchMatch(matchId); }
    catch (e) { toast(e.message, 'danger'); return; }
    this.#currentMatch = detail.match;
    const m = this.#currentMatch;

    let season = null;
    try { season = await api('GET', `/seasons/${m.season_id}`); } catch (_) {}
    // PM correction (2026-09-30): #allSeasons feeds #renderScoresheet's
    // shared seasonClosed lock calculation -- it must reflect THIS
    // match's own season every time this loads, never a stale list left
    // over from a previous Admin View visit to a different league.
    this.#allSeasons = season ? [season] : [];
    this.#activeLeague = null;
    if (season) {
      try { this.#activeLeague = await api('GET', `/leagues/${season.league_id}`); } catch (_) {}
    }

    // PM correction (2026-09-30): a failed fetch here must not be treated
    // as "no approvals yet" -- #approval stays null, which
    // #renderScoresheet/#renderTeamApprovalPanel check directly to force
    // the scoresheet read-only (no Save/Clear, no team-approval controls)
    // with a Retry action, instead of rendering editable controls that
    // would send a player write with no expected_revision.
    try { this.#approval = await fetchMatchApproval(matchId); }
    catch (e) { this.#approval = null; }

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

    // 1. Existing round results take highest priority (already played).
    let existingRounds = [];
    try { existingRounds = await fetchRounds(matchId); } catch (_) {}
    if (existingRounds.length > 0) {
      const r1 = existingRounds.filter(r => r.round_number === 1).sort((a, b) => a.id - b.id);
      if (r1.length === 3) {
        const hp = r1.map(r => this.#resolvePlayer(r.home_player_id, r.home_player_name, r.home_handicap)).filter(Boolean);
        const ap = r1.map(r => this.#resolvePlayer(r.away_player_id, r.away_player_name, r.away_handicap)).filter(Boolean);
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
    this.#homePlansCache = homePlans;
    this.#awayPlansCache = awayPlans;

    if (homePlans.length >= 3 && awayPlans.length >= 3) {
      const hp = homePlans.slice(0, 3).map(lp => this.#resolvePlayer(lp.player_id, lp.player_name, lp.handicap)).filter(Boolean);
      const ap = awayPlans.slice(0, 3).map(lp => this.#resolvePlayer(lp.player_id, lp.player_name, lp.handicap)).filter(Boolean);
      if (hp.length === 3 && ap.length === 3) {
        this.#homeTeam = hp;
        this.#awayTeam = ap;
        this.#homeLineupPlans = homePlans.slice(0, 3);
        this.#awayLineupPlans = awayPlans.slice(0, 3);
        this.#resetGames();
        this.#renderScoresheet([]);
        return;
      }
    }

    // 3. Fallback: inline quick picker. Needs the match's own league's full
    // player list (never the shell's admin-selected league) to offer
    // roster/substitute options -- fetched lazily, only when this step is
    // actually reached.
    try {
      this.#allPlayers = season ? await api('GET', `/players?league_id=${season.league_id}`) : [];
    } catch (_) {
      this.#allPlayers = [];
    }
    const homePlayers = this.#allPlayers.filter(p => p.team_id == m.home_team_id);
    const awayPlayers = this.#allPlayers.filter(p => p.team_id == m.away_team_id);
    this.#showLineupPicker(m, homePlayers, awayPlayers, homePlans, awayPlans);
  }

  #showLineupPicker(m, homePlayers, awayPlayers, homePlans = [], awayPlans = []) {
    const lineupDiv = this.querySelector('.me-lineup');
    lineupDiv.classList.remove('d-none');

    // Substitute Workflow Phase 1: the roster picker offers every player in
    // the league, not just this team's own roster, so a substitute from
    // another team can be selected here too -- grouped so the team's own
    // players are still easy to find first.
    const makeOpts = (rosterPlayers, plans, idx) => {
      const preselect = plans[idx]?.player_id;
      const optRow = p => `<option value="${p.id}"${p.id == preselect ? ' selected' : ''}>${esc(p.name)} (${p.handicap >= 0 ? '+' : ''}${p.handicap})</option>`;
      const rosterIds = new Set(rosterPlayers.map(p => p.id));
      const others = this.#allPlayers.filter(p => !rosterIds.has(p.id));
      let html = `<optgroup label="This Team">${rosterPlayers.map(optRow).join('')}</optgroup>`;
      if (others.length > 0) {
        html += `<optgroup label="Other Players (Substitute)">${others.map(optRow).join('')}</optgroup>`;
      }
      return html;
    };

    const hasHomePlans = homePlans.length > 0;
    const hasAwayPlans = awayPlans.length > 0;
    // PM correction (2026-09-30): the "Lineup" link below opens the
    // admin-only Lineup section, which is hidden entirely in Player View
    // -- a player following it would land nowhere useful. Player mode
    // gets plain guidance instead; the inline picker itself (not this
    // link) is how a player actually sets the lineup here.
    const hint = (hasHomePlans || hasAwayPlans)
      ? '<span class="badge bg-info text-dark ms-2" style="font-size:.7rem">Pre-filled from saved lineup</span>'
      : this.#mode === 'player'
        ? '<span class="text-muted fw-normal small">No lineup has been set for this match yet -- select players below</span>'
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

    // Resolved against the full player list (not the team roster) so a
    // manually-picked substitute -- who may not carry this team's team_id --
    // still resolves correctly.
    const hp = [1, 2, 3].map(i => this.#allPlayers.find(p => p.id == this.querySelector(`#me-ph-${i}`)?.value)).filter(Boolean);
    const ap = [1, 2, 3].map(i => this.#allPlayers.find(p => p.id == this.querySelector(`#me-pa-${i}`)?.value)).filter(Boolean);

    if (hp.length !== 3) { toast('Select all 3 home players', 'warning'); return; }
    if (ap.length !== 3) { toast('Select all 3 away players', 'warning'); return; }
    if (new Set(hp.map(p => p.id)).size !== 3) { toast('Duplicate home player', 'warning'); return; }
    if (new Set(ap.map(p => p.id)).size !== 3) { toast('Duplicate away player', 'warning'); return; }

    this.#homeTeam = hp;
    this.#awayTeam = ap;
    // Recover each slot's lineup_plans row id when the confirmed player
    // matches what was already planned, so the roster table's Substitute
    // control works immediately after a manual confirm too.
    this.#homeLineupPlans = hp.map(p => this.#homePlansCache.find(lp => lp.player_id === p.id) ?? null);
    this.#awayLineupPlans = ap.map(p => this.#awayPlansCache.find(lp => lp.player_id === p.id) ?? null);
    this.#resetGames();
    this.querySelector('.me-lineup').classList.add('d-none');
    this.#renderScoresheet([]);
  }

  #resetGames() {
    this.#games = Array.from({ length: 9 }, () =>
      ({ g1w: '', g1lb: 0, g2w: '', g2lb: 0, g3w: '', g3lb: 0 })
    );
  }

  // -- Substitute Workflow Phase 1 -----------------------------------------

  // Renders the roster table's per-slot substitute control: a "Sub" button
  // when the slot has a known lineup_plans row and isn't currently
  // substituted, a "Sub for X" badge with an Undo link when it is, or
  // nothing when no lineup_plans row is known for this slot (e.g. an
  // already-scored match) or scores are locked.
  //
  // PM correction (2026-09-30): the Sub/Undo mutation controls call
  // POST/DELETE /lineup-plans/{id}/substitute, which are ActionLineupMutate-
  // gated admin-only routes -- a player session gets 403 from them. Player
  // mode must never render either control, only the read-only "Sub for X"
  // attribution when relevant; this is a presentation gate only, it does
  // NOT authorize player sessions for lineup-management routes.
  #subControlHtml(side, index, plan) {
    if (!plan) return '';
    if (this.#mode !== 'admin') {
      if (!plan.is_sub) return '';
      const origName = this.#allPlayers.find(p => p.id === plan.sub_for_id)?.name || 'original player';
      return ` <span class="badge bg-warning text-dark no-print" style="font-size:.6rem">Sub for ${esc(origName)}</span>`;
    }
    if (plan.is_sub) {
      const origName = this.#allPlayers.find(p => p.id === plan.sub_for_id)?.name || 'original player';
      const undo = this.#scoresheetEditable
        ? ` <a href="#" class="no-print" data-action="clear-substitute" data-side="${side}" data-index="${index}">Undo</a>`
        : '';
      return ` <span class="badge bg-warning text-dark no-print" style="font-size:.6rem">Sub for ${esc(origName)}</span>${undo}`;
    }
    if (!this.#scoresheetEditable) return '';
    return ` <button class="btn btn-outline-secondary btn-sm py-0 no-print" style="font-size:.6rem"
      data-action="open-substitute" data-side="${side}" data-index="${index}">Sub</button>`;
  }

  #ensureSubstituteModal() {
    if (document.getElementById('me-sub-modal')) return;
    const el = document.createElement('div');
    el.innerHTML = `
<div class="modal fade" id="me-sub-modal" tabindex="-1">
  <div class="modal-dialog">
    <div class="modal-content">
      <div class="modal-header">
        <h5 class="modal-title">Substitute Player</h5>
        <button type="button" class="btn-close" data-bs-dismiss="modal"></button>
      </div>
      <div class="modal-body">
        <p class="mb-3">Substituting for: <strong id="me-sub-original-name"></strong></p>
        <div class="mb-1">
          <label class="form-label">Substitute Player</label>
          <select class="form-select" id="me-sub-player-select"></select>
        </div>
      </div>
      <div class="modal-footer">
        <button type="button" class="btn btn-secondary" data-bs-dismiss="modal">Cancel</button>
        <button type="button" class="btn btn-primary" id="me-sub-confirm-btn">Confirm Substitute</button>
      </div>
    </div>
  </div>
</div>`;
    document.body.appendChild(el.firstElementChild);
  }

  #openSubstituteModal(side, index) {
    const plans = side === 'home' ? this.#homeLineupPlans : this.#awayLineupPlans;
    const plan = plans[index];
    if (!plan) return;
    const team = side === 'home' ? this.#homeTeam : this.#awayTeam;
    const currentPlayer = team[index];
    this.#subContext = { side, index, lineupPlanId: plan.id };
    document.getElementById('me-sub-original-name').textContent = currentPlayer.name;
    // Exclude every player already occupying one of the match's 6 slots
    // (both teams), including the current slot's own player -- picking a
    // player already in this match (on either side) would put them in two
    // slots at once. The backend enforces this too (POST .../substitute
    // returns 409 "that player is already in this match"); this filter
    // just keeps the picker from offering an obviously-invalid choice.
    const inMatchIds = new Set([...this.#homeTeam, ...this.#awayTeam].map(p => p.id));
    const sel = document.getElementById('me-sub-player-select');
    sel.innerHTML = this.#allPlayers
      .filter(p => !inMatchIds.has(p.id))
      .map(p => `<option value="${p.id}">${esc(p.name)} (${p.handicap >= 0 ? '+' : ''}${p.handicap})</option>`)
      .join('');
    new bootstrap.Modal(document.getElementById('me-sub-modal')).show();
  }

  async #confirmSubstitute() {
    const ctx = this.#subContext;
    if (!ctx) return;
    const subPlayerId = parseInt(document.getElementById('me-sub-player-select').value, 10);
    if (!subPlayerId) { toast('Select a substitute player', 'warning'); return; }
    try {
      await setLineupSubstitute(ctx.lineupPlanId, subPlayerId);
      bootstrap.Modal.getInstance(document.getElementById('me-sub-modal'))?.hide();
      toast('Substitute recorded');
      await this.#reload();
    } catch (e) {
      toast(e.message, 'danger');
    }
  }

  async #clearSubstitute(side, index) {
    const plans = side === 'home' ? this.#homeLineupPlans : this.#awayLineupPlans;
    const plan = plans[index];
    if (!plan) return;
    try {
      await clearLineupSubstitute(plan.id);
      toast('Substitute cleared');
      await this.#reload();
    } catch (e) {
      toast(e.message, 'danger');
    }
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

    const leagueName   = this.#activeLeague?.name || 'League';
    const seasonClosed = !!(this.#allSeasons.find(s => s.id == m.season_id)?.closed_at);
    const weekClosed   = !!m.week_closed;

    // Weekly Score Processing Phase 1A/1B: admin-attested approval/processing
    // state. processed implies approved in every real flow, but both are
    // checked independently to mirror the backend's own guard. Phase 1C
    // correction: the backend's approve/process/unprocess/unapprove guards
    // (and SaveRounds/SubmitResults/ClearResults) also reject a closed week
    // independent of the season -- locked mirrors that so the UI never shows
    // a button the backend would 409.
    const locked       = seasonClosed || weekClosed;
    const isApproved  = !!m.approved_at;
    const isProcessed = !!m.processed_at;
    const isPlayerMode = this.#mode === 'player';
    // PM correction (2026-09-30): a player-mode write requires the
    // score_revision from a successfully-loaded approval snapshot --
    // when that fetch failed, #approval is null and the scoresheet must
    // render read-only (no Save/Clear) rather than send a write with an
    // undefined revision. Never affects admin mode, which doesn't use
    // #approval at all for this.
    const approvalUnavailable = isPlayerMode && !this.#approval;
    const canEditScores = !locked && !isApproved && !isProcessed && !approvalUnavailable;
    // Substitute Workflow Phase 1: substitute edits respect the same lock
    // the backend enforces (season closed, week closed, approved, processed),
    // so the Sub/Undo controls are shown under the same condition as Save.
    // PM correction (2026-09-30): the dedicated Sub/Undo mutation
    // controls themselves are admin-only (ActionLineupMutate-gated
    // routes) regardless of editability -- see #subControlHtml, which
    // checks #mode directly rather than relying on this flag alone.
    this.#scoresheetEditable = canEditScores;

    const statusBadge = isProcessed
      ? '<span class="badge bg-primary ms-1"><i class="bi bi-gear-fill me-1"></i>Processed</span>'
      : isApproved
        ? '<span class="badge bg-info text-dark ms-1"><i class="bi bi-check2-circle me-1"></i>Approved</span>'
        : '';

    // Player Score Approval Phase 1C: these four admin-only controls (the
    // Weekly Score Processing Phase 1A/1B match-level workflow, distinct
    // from the team-side approval panel below) must never render in player
    // mode -- gated on #mode first, before any of their own state checks.
    const isAdminMode = this.#mode === 'admin';
    // Player Score Approval Phase 1D: Accept replaces the old Approve
    // button's request shape (which never sent expected_revision/override
    // and would now 400 against AdminAcceptMatch). Disabled with a Retry
    // path instead of rendered at all when the approval snapshot failed
    // to load -- #acceptMatch's own expected_revision source -- rather
    // than let an admin accept against guessed state.
    const canAttemptAccept = isAdminMode && !locked && m.completed && !isApproved && !isProcessed;
    const approveBtn = canAttemptAccept
      ? (this.#approval
          ? `<button class="btn btn-sm btn-outline-primary" data-action="accept-match" data-match-id="${m.id}">
              <i class="bi bi-check2-circle"></i> Accept</button>`
          : `<button class="btn btn-sm btn-outline-secondary" data-action="retry-admin-approval" data-match-id="${m.id}">
              <i class="bi bi-arrow-clockwise"></i> Retry Approval Status</button>`)
      : '';
    const processBtn = (isAdminMode && !locked && isApproved && !isProcessed)
      ? `<button class="btn btn-sm btn-outline-primary" data-action="process-match" data-match-id="${m.id}">
          <i class="bi bi-gear"></i> Process</button>` : '';
    const unprocessBtn = (isAdminMode && !locked && isProcessed)
      ? `<button class="btn btn-sm btn-outline-secondary" data-action="unprocess-match" data-match-id="${m.id}">
          <i class="bi bi-arrow-counterclockwise"></i> Unprocess</button>` : '';
    const unapproveBtn = (isAdminMode && !locked && isApproved && !isProcessed)
      ? `<button class="btn btn-sm btn-outline-secondary" data-action="unapprove-match" data-match-id="${m.id}">
          <i class="bi bi-x-circle"></i> Unapprove</button>` : '';

    // Closed-week correction path takes priority over the approved/processed
    // hint: reopening the week (Schedule page) is the first required step
    // before unprocess/unapprove/edit apply, regardless of approval state.
    // PM correction (2026-09-30): a player cannot reopen a week, unprocess,
    // or unapprove -- the admin-instruction wording below must never be
    // shown in player mode, which gets its own "contact a league
    // administrator" wording instead.
    const weekClosedHint = (weekClosed && !seasonClosed)
      ? isPlayerMode
        ? `<div class="alert alert-warning py-1 px-2 small mb-2 no-print">
            <i class="bi bi-lock me-1"></i>This week is closed. Contact a league administrator.
          </div>`
        : `<div class="alert alert-warning py-1 px-2 small mb-2 no-print">
            <i class="bi bi-lock me-1"></i>This match's week is closed. Reopen the week on the
            Schedule page first${(isApproved || isProcessed) ? ', then unprocess/unapprove to edit scores again' : ' to edit scores again'}.
          </div>`
      : '';

    const lockedHint = (!locked && (isApproved || isProcessed))
      ? isPlayerMode
        ? `<div class="alert alert-info py-1 px-2 small mb-2 no-print">
            <i class="bi bi-lock me-1"></i>This match has been accepted or processed and is locked.
            Contact a league administrator for corrections.
          </div>`
        : `<div class="alert alert-info py-1 px-2 small mb-2 no-print">
            <i class="bi bi-lock me-1"></i>${isProcessed
              ? 'Scores are processed and locked. Unprocess, then unapprove, to edit scores again.'
              : 'Scores are approved and locked. Unapprove to edit scores again.'}
          </div>`
      : '';

    let html = `<div class="no-print d-flex justify-content-between align-items-center mb-2 gap-2 flex-wrap">
      <div class="d-flex align-items-center gap-2">
        <span class="fw-semibold small">Week ${m.week_number}${m.match_date ? ' - ' + fmtDate(m.match_date) : ''}</span>
        ${m.completed
          ? '<span class="badge bg-success">Completed</span>'
          : '<span class="badge bg-secondary">Pending</span>'}
        ${statusBadge}
        ${seasonClosed ? '<span class="badge bg-secondary ms-1"><i class="bi bi-lock me-1"></i>Season Closed</span>' : ''}
        ${weekClosed && !seasonClosed ? '<span class="badge bg-secondary ms-1"><i class="bi bi-lock me-1"></i>Week Closed</span>' : ''}
      </div>
      <div class="d-flex gap-2">
        ${isPlayerMode
          ? `<button class="btn btn-sm btn-outline-secondary" data-action="back-to-my-matches">
              <i class="bi bi-arrow-left"></i> Back to My Matches</button>`
          : `<button class="btn btn-sm btn-outline-secondary" data-action="change-lineup">
              <i class="bi bi-arrow-left"></i> Change Lineup</button>`}
        <button class="btn btn-sm btn-outline-secondary" data-action="print-scoresheet">
          <i class="bi bi-printer"></i> Print</button>
        ${canEditScores ? `<button class="btn btn-sm btn-success" data-action="save-scoresheet">
          <i class="bi bi-check-lg"></i> Save</button>` : ''}
        ${m.completed && canEditScores ? `<button class="btn btn-sm btn-outline-danger" data-action="clear-results" data-match-id="${m.id}">
          <i class="bi bi-x-lg"></i> Clear</button>` : ''}
        ${approveBtn}${processBtn}${unprocessBtn}${unapproveBtn}
      </div>
    </div>
    ${weekClosedHint}${lockedHint}`;

    if (this.#mode === 'player') html += this.#renderTeamApprovalPanel();
    else html += this.#renderAdminApprovalPanel();

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
        <td>${esc(hp.name)}${this.#subControlHtml('home', i, this.#homeLineupPlans[i])}</td>
        <td class="ss-hc-roster">${fmtHC(hp.handicap)}</td>
        <td class="ss-roster-sep"></td>
        <td class="ss-slot-v">V${i + 1}</td>
        <td class="ss-pnum-col">${ap.player_number || ''}</td>
        <td>${esc(ap.name)}${this.#subControlHtml('away', i, this.#awayLineupPlans[i])}</td>
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

  // -- Player Score Approval Phase 1C: team-side approval panel -----------

  // #renderTeamApprovalPanel builds the player-mode-only card showing both
  // sides' current approval state and, only for the side(s) #playerCtx
  // marks eligible AND whose action is currently valid against the
  // backend's own guards, the mutation controls to act on it. Carries its
  // own id (#me-approval-panel) so a mutation's response -- itself the
  // fresh MatchApprovalSnapshot -- can be re-rendered in place via
  // #refreshApprovalPanel without reloading the whole match.
  //
  // PM correction (2026-09-30): previously offered Approve/Request
  // Correction before the match was even scored, and kept offering every
  // control regardless of season/week/admin-acceptance/processed lock --
  // all of which the backend's own guards (classifyTeamActionBlock,
  // matchStillEditableSQL) would reject. This mirrors those guards in
  // presentation: Approve/Request Correction require a completed,
  // unlocked match; Withdraw requires the side to already be approved AND
  // the match to remain unlocked; nothing renders at all once the season
  // is closed, the week is closed, the match is admin-accepted, or it is
  // processed -- those are league-admin actions, never a player's, so the
  // explanation never implies the player could do them.
  #renderTeamApprovalPanel() {
    const snap = this.#approval;
    if (!snap) {
      return `<div class="alert alert-warning small no-print" id="me-approval-panel">
        Approval status could not be loaded, so scores are shown read-only.
        <button class="btn btn-sm btn-outline-secondary ms-2" data-action="retry-player-load">Retry</button>
      </div>`;
    }
    const m = this.#currentMatch;
    const ctx = this.#playerCtx;

    const seasonClosed = !!(this.#allSeasons.find(s => s.id == m.season_id)?.closed_at);
    const weekClosed   = !!m.week_closed;
    const isApproved   = !!m.approved_at;
    const isProcessed  = !!m.processed_at;
    // "locked" here mirrors the backend's matchStillEditableSQL exactly:
    // not admin-accepted, not processed, week open, season open. Distinct
    // from #renderScoresheet's own `locked` (season/week only) -- team
    // actions are additionally blocked by admin acceptance/processing,
    // which score edits are NOT (editing after acceptance is simply
    // impossible because canEditScores already excludes it).
    const teamActionsLocked = seasonClosed || weekClosed || isApproved || isProcessed;
    const lockReason = seasonClosed
      ? 'This season is closed.'
      : weekClosed
        ? 'This week is closed. Contact a league administrator.'
        : (isProcessed || isApproved)
          ? 'This match has been accepted or processed and is locked. Contact a league administrator for corrections.'
          : '';

    const sideCard = (side, state, teamName, eligible) => {
      const label = state.state === 'approved' ? 'Approved'
        : state.state === 'correction_requested' ? 'Correction Requested'
        : 'Pending';
      const badgeClass = state.state === 'approved' ? 'bg-success'
        : state.state === 'correction_requested' ? 'bg-warning text-dark'
        : 'bg-secondary';
      const actor = state.actor_name_snapshot
        ? `<div class="small text-muted">By ${esc(state.actor_name_snapshot)}</div>` : '';
      const note = state.correction_note
        ? `<div class="small text-danger">&ldquo;${esc(state.correction_note)}&rdquo;</div>` : '';
      const revNote = state.state_score_revision != null
        ? `<div class="small text-muted">Applies to revision ${state.state_score_revision}</div>` : '';

      let controls = '';
      if (eligible) {
        const canApprove = !teamActionsLocked && m.completed && state.state !== 'approved';
        const canWithdraw = !teamActionsLocked && state.state === 'approved';
        const canRequestCorrection = !teamActionsLocked && m.completed;
        if (canApprove) {
          controls += `<button class="btn btn-sm btn-success me-1 no-print" data-action="team-approve" data-side="${side}">Approve</button>`;
        }
        if (canWithdraw) {
          controls += `<button class="btn btn-sm btn-outline-secondary me-1 no-print" data-action="team-withdraw" data-side="${side}">Withdraw</button>`;
        }
        if (canRequestCorrection) {
          controls += `<button class="btn btn-sm btn-outline-warning no-print" data-action="team-correction" data-side="${side}">Request Correction</button>
            <div class="mt-2 d-none" id="me-correction-${side}">
              <input type="text" class="form-control form-control-sm mb-1" id="me-correction-note-${side}"
                placeholder="What needs to be corrected? (required)">
              <button class="btn btn-sm btn-warning no-print" data-action="team-correction-submit" data-side="${side}">Submit</button>
            </div>`;
        }
        if (!canApprove && !canWithdraw && !canRequestCorrection) {
          controls = `<div class="small text-muted">${lockReason || 'Enter and save scores before approving.'}</div>`;
        }
      }
      return `<div class="col-md-6">
        <div class="border rounded p-2 h-100">
          <div class="d-flex justify-content-between align-items-center">
            <strong>${esc(teamName)}</strong>
            <span class="badge ${badgeClass}">${label}</span>
          </div>
          ${actor}${note}${revNote}
          <div class="mt-2">${controls}</div>
        </div>
      </div>`;
    };
    return `<div class="card mb-3 no-print" id="me-approval-panel">
      <div class="card-header fw-semibold py-2">Team Approval</div>
      <div class="card-body">
        <p class="small text-muted mb-2">
          Each team approves the score once for the match to be finalized by a league admin.
          Approval applies to the scores currently shown (revision ${snap.score_revision}) --
          editing scores clears any prior approval. Team approval does not by itself process the
          match or update handicaps; an admin completes those steps separately.
        </p>
        <div class="row g-2">
          ${sideCard('home', snap.home, m.home_team_name, ctx.eligibleHome)}
          ${sideCard('away', snap.away, m.away_team_name, ctx.eligibleAway)}
        </div>
      </div>
    </div>`;
  }

  #refreshApprovalPanel() {
    const panel = this.querySelector('#me-approval-panel');
    if (panel) panel.outerHTML = this.#renderTeamApprovalPanel();
  }

  // -- Player Score Approval Phase 1D: admin-mode approval status --------

  // #renderAdminApprovalPanel is the admin-mode counterpart to
  // #renderTeamApprovalPanel: read-only (an administrator never acts for
  // a team side here -- only players do, via the player-mode panel above),
  // paired with the Accept button in the toolbar. Shows exactly the
  // snapshot #acceptMatch's own accept/override decision uses, so an
  // administrator can see up front whether a normal Accept will succeed
  // or require override, and why. #approvalSideLabel's "current" vs
  // "stale" distinction mirrors matches.sideApprovedAtRevision's equality
  // check for DISPLAY only -- the backend re-verifies this independently
  // and authoritatively at write time regardless of what this shows.
  #renderAdminApprovalPanel() {
    const m = this.#currentMatch;
    if (!m.completed) return '';
    const snap = this.#approval;
    if (!snap) {
      return `<div class="alert alert-warning small no-print" id="me-admin-approval-panel">
        Team approval status could not be loaded. Accepting is disabled until this is retried.
        <button class="btn btn-sm btn-outline-secondary ms-2" data-action="retry-admin-approval">Retry</button>
      </div>`;
    }

    const sideBlock = (teamName, side) => {
      const lbl = this.#approvalSideLabel(side, snap.score_revision);
      const actor = side.actor_name_snapshot
        ? `<div class="small text-muted">By ${esc(side.actor_name_snapshot)}</div>` : '';
      const note = side.correction_note
        ? `<div class="small text-danger">&ldquo;${esc(side.correction_note)}&rdquo;</div>` : '';
      return `<div class="col-md-6">
        <div class="border rounded p-2 h-100">
          <div class="d-flex justify-content-between align-items-center">
            <strong>${esc(teamName)}</strong>
            <span class="badge ${lbl.cls}">${lbl.text}</span>
          </div>
          ${actor}${note}
        </div>
      </div>`;
    };

    const homeReady = this.#sideReadyAtRevision(snap.home, snap.score_revision);
    const awayReady = this.#sideReadyAtRevision(snap.away, snap.score_revision);
    const readiness = m.approved_at ? '' : `<div class="small mt-2 ${homeReady && awayReady ? 'text-success' : 'text-muted'}">
        ${homeReady && awayReady
          ? 'Both teams have approved the current scores. Ready to accept.'
          : 'One or both teams have not approved the current scores. Accepting will require administrator override.'}
      </div>`;
    const acceptedHow = (m.approved_at && this.#lastAcceptEvent)
      ? `<div class="small ${this.#lastAcceptEvent === 'admin_override' ? 'text-warning' : 'text-success'} mt-1">
          ${this.#lastAcceptEvent === 'admin_override'
            ? 'Accepted by administrator override.'
            : 'Accepted normally (both teams had approved).'}
        </div>`
      : '';

    return `<div class="card mb-3 no-print" id="me-admin-approval-panel">
      <div class="card-header fw-semibold py-2 d-flex justify-content-between align-items-center">
        <span>Team Approval Status</span>
        <span class="small text-muted">Revision ${snap.score_revision}</span>
      </div>
      <div class="card-body">
        <div class="row g-2">
          ${sideBlock(m.home_team_name, snap.home)}
          ${sideBlock(m.away_team_name, snap.away)}
        </div>
        ${readiness}${acceptedHow}
      </div>
    </div>`;
  }

  // #sideReadyAtRevision mirrors matches.sideApprovedAtRevision exactly
  // (state === "approved" AND state_score_revision === expectedRevision)
  // for display purposes only -- shared by #renderAdminApprovalPanel and
  // #acceptMatch so the two can never disagree about what "ready" means.
  #sideReadyAtRevision(side, expectedRevision) {
    return side.state === 'approved' && side.state_score_revision === expectedRevision;
  }

  #approvalSideLabel(side, scoreRevision) {
    if (side.state === 'correction_requested') return { text: 'Correction Requested', cls: 'bg-warning text-dark' };
    if (side.state === 'approved') {
      return this.#sideReadyAtRevision(side, scoreRevision)
        ? { text: 'Approved (current)', cls: 'bg-success' }
        : { text: 'Approved (stale revision)', cls: 'bg-warning text-dark' };
    }
    return { text: 'Pending', cls: 'bg-secondary' };
  }

  // #approvalReasonText explains, for one side, why it does not count as
  // a current approval -- used only to build #acceptMatch's override
  // confirmation message.
  #approvalReasonText(side, scoreRevision) {
    if (side.state === 'correction_requested') {
      return 'correction requested' + (side.correction_note ? ` ("${side.correction_note}")` : '');
    }
    if (side.state === 'approved') {
      return `approved at revision ${side.state_score_revision}, but the current revision is ${scoreRevision} (stale)`;
    }
    return 'not yet approved';
  }

  // A 409 here means the score_revision this action targeted is no longer
  // current -- someone else edited or the player's own other tab acted
  // first. Per the stop condition, this must never silently retry or
  // overwrite: reload the authoritative match/rounds/approval state and
  // let the player review it before trying again.
  async #handleApprovalActionError(e) {
    if (e.status === 409) {
      toast('This match changed elsewhere. Reloading the latest scores and approval status for you to review.', 'warning');
      await this.#reload();
      return;
    }
    toast(e.message, 'danger');
  }

  async #teamApprove(side) {
    const rev = this.#approval?.score_revision;
    if (rev == null) return;
    try {
      this.#approval = await approveTeamSide(this.#currentMatch.id, side, rev);
      toast((side === 'home' ? 'Home' : 'Away') + ' team approved');
      this.#refreshApprovalPanel();
    } catch (e) {
      await this.#handleApprovalActionError(e);
    }
  }

  async #teamWithdraw(side) {
    const rev = this.#approval?.score_revision;
    if (rev == null) return;
    try {
      this.#approval = await withdrawTeamApproval(this.#currentMatch.id, side, rev);
      toast((side === 'home' ? 'Home' : 'Away') + ' approval withdrawn');
      this.#refreshApprovalPanel();
    } catch (e) {
      await this.#handleApprovalActionError(e);
    }
  }

  async #teamRequestCorrection(side) {
    const rev = this.#approval?.score_revision;
    if (rev == null) return;
    const input = this.querySelector(`#me-correction-note-${side}`);
    const note = input?.value.trim() || '';
    if (!note) { toast('Enter what needs to be corrected', 'warning'); return; }
    try {
      this.#approval = await requestTeamCorrection(this.#currentMatch.id, side, rev, note);
      toast('Correction requested');
      this.#refreshApprovalPanel();
    } catch (e) {
      await this.#handleApprovalActionError(e);
    }
  }

  // PM correction (2026-10-01): #scoresheetEditable already gates
  // Save/Clear and the Sub/Undo controls, but the score inputs themselves
  // stayed enabled regardless -- a read-only scoresheet (player-mode
  // approval snapshot unavailable, closed season, closed week, approved,
  // or processed; every condition #scoresheetEditable already represents,
  // see #renderScoresheet) must disable entry at the input level too, not
  // just hide the buttons around it. No second editability policy: this
  // reads #scoresheetEditable directly, the same flag every other
  // read-only gate in this component already uses.
  #ssGameInput(idx, gn, side) {
    const g   = this.#games[idx];
    const w   = g['g' + gn + 'w'];
    const lb  = g['g' + gn + 'lb'];
    const val = w === side ? '10' : (w && w !== side) ? String(lb) : '';
    return `<input type="number" class="ss-score-inp" id="ss-inp-${idx}-${gn}-${side}"` +
      ` data-idx="${idx}" data-gn="${gn}" data-side="${side}"` +
      ` min="0" max="10"${val !== '' ? ` value="${val}"` : ''}${this.#scoresheetEditable ? '' : ' disabled'}>`;
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
    // Player Score Approval Phase 1C: a player-mode write always sends the
    // score_revision from the match's last-fetched approval snapshot --
    // the backend requires it unconditionally for a non-admin write and
    // rejects a stale one with 409. Admin writes stay exactly as before
    // (expectedRevision undefined -> omitted from the request body).
    const expectedRevision = this.#mode === 'player' ? this.#approval?.score_revision : undefined;
    try {
      await saveRounds(m.id, rounds, expectedRevision);
      toast('Scoresheet saved');
      if (this.#mode === 'player') await this.#reload();
      else await this.#loadMatches();
    } catch (e) {
      if (this.#mode === 'player') { await this.#handleApprovalActionError(e); return; }
      toast(e.message, 'danger');
    }
  }

  async #clearResults(matchId) {
    if (!confirm('Clear all results for this match?')) return;
    const expectedRevision = this.#mode === 'player' ? this.#approval?.score_revision : undefined;
    try {
      await apiClearResults(matchId, expectedRevision);
      toast('Results cleared');
      await this.#reload();
    } catch (e) {
      if (this.#mode === 'player') { await this.#handleApprovalActionError(e); return; }
      toast(e.message, 'danger');
    }
  }

  // --- Weekly Score Processing Phase 1A/1B admin actions ---
  // No note-entry UI in Phase 1C -- approve is admin-attested with an empty
  // note by default, matching "keep shell changes minimal." Each action
  // reloads the match so the status badges and button set reflect the new
  // state immediately.

  // #acceptMatch is Player Score Approval Phase 1D's rewiring of the old
  // #approveMatch onto AdminAcceptMatch's actual contract. expected_revision
  // always comes from the last-fetched approval snapshot (#approval), never
  // from the match row directly, per the PM's "use the latest authoritative
  // approval snapshot" requirement. override is false whenever both sides
  // are approved at the current revision; otherwise an explicit confirm()
  // naming exactly which side(s) are not satisfied is required before
  // sending override:true -- a declined confirm sends nothing at all, so a
  // failed normal accept can never be silently retried as an override.
  // Every outcome (success or error) ends in #reload(), so a stale-revision
  // or changed-team-state 409 is reflected immediately rather than risking
  // the UI showing state this click no longer agrees with.
  async #acceptMatch(matchId) {
    const snap = this.#approval;
    if (!snap) {
      toast('Approval status is unavailable; retry before accepting.', 'danger');
      return;
    }
    const rev = snap.score_revision;
    const homeReady = this.#sideReadyAtRevision(snap.home, rev);
    const awayReady = this.#sideReadyAtRevision(snap.away, rev);
    let override = false;
    if (!homeReady || !awayReady) {
      const reasons = [];
      if (!homeReady) reasons.push(`Home: ${this.#approvalReasonText(snap.home, rev)}`);
      if (!awayReady) reasons.push(`Away: ${this.#approvalReasonText(snap.away, rev)}`);
      const ok = confirm(
        'This match does not have both team approvals at the current score revision:\n\n' +
        reasons.join('\n') +
        '\n\nAccept anyway as an administrator override?'
      );
      if (!ok) return;
      override = true;
    }
    try {
      await apiApproveMatch(matchId, rev, override);
      toast(override ? 'Match accepted (administrator override)' : 'Match accepted');
    } catch (e) {
      toast(e.message, 'danger');
    } finally {
      await this.#reload();
    }
  }

  async #processMatch(matchId) {
    try {
      await apiProcessMatch(matchId);
      toast('Match processed');
      await this.#loadMatchEntry();
    } catch (e) { toast(e.message, 'danger'); }
  }

  async #unprocessMatch(matchId) {
    try {
      await apiUnprocessMatch(matchId);
      toast('Match unprocessed');
      await this.#loadMatchEntry();
    } catch (e) { toast(e.message, 'danger'); }
  }

  async #unapproveMatch(matchId) {
    try {
      await apiUnapproveMatch(matchId);
      toast('Match unapproved');
      await this.#loadMatchEntry();
    } catch (e) { toast(e.message, 'danger'); }
  }
}

customElements.define('match-entry-page', MatchEntryPage);
