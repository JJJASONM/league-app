// <my-matches-page> - Player View's "My Matches" list (Player Score
// Approval Phase 1C). Shows every match the signed-in linked player is
// eligible to act on for at least one side, using the player's OWN
// league/season context (resolved entirely server-side by
// GET /players/{id}/eligible-matches) -- never the shell's admin-selected
// league/season, and never a frontend-side eligibility approximation.
//
// Public API:
//   refresh(currentIdentity)
//
// Opening a match dispatches 'my-match-open-request' (bubbling), which the
// shell (web/app.js) handles by activating Match Entry in player mode,
// carrying this row's own eligible_home/eligible_away flags through
// unchanged -- Match Entry never re-derives eligibility itself.

import { fetchPlayerEligibleMatches } from './player-eligible-matches-api-service.js';

function esc(s) {
  return String(s ?? '').replace(/[&<>"']/g, ch =>
    ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[ch])
  );
}

function stateBadge(state) {
  if (state === 'approved') return '<span class="badge bg-success">Approved</span>';
  if (state === 'correction_requested') return '<span class="badge bg-warning text-dark">Correction Requested</span>';
  return '<span class="badge bg-secondary">Pending</span>';
}

class MyMatchesPage extends HTMLElement {
  #playerId = null;

  connectedCallback() {
    this.innerHTML = `
      <h4 class="mb-3 fw-bold">My Matches</h4>
      <div class="mm-list"></div>`;

    this.addEventListener('click', e => {
      const btn = e.target.closest('[data-action="open-my-match"]');
      if (!btn) return;
      this.dispatchEvent(new CustomEvent('my-match-open-request', {
        bubbles: true,
        detail: {
          matchId: parseInt(btn.dataset.matchId, 10),
          seasonId: parseInt(btn.dataset.seasonId, 10),
          eligibleHome: btn.dataset.eligibleHome === '1',
          eligibleAway: btn.dataset.eligibleAway === '1',
        },
      }));
    });
  }

  async refresh(currentIdentity) {
    this.#playerId = currentIdentity?.player_id ?? null;
    const listEl = this.querySelector('.mm-list');
    if (!listEl) return;

    if (this.#playerId == null) {
      listEl.innerHTML = '<div class="text-muted p-3">No linked player account.</div>';
      return;
    }

    listEl.innerHTML = '<div class="text-muted p-3">Loading your matches&hellip;</div>';
    let rows;
    try {
      rows = await fetchPlayerEligibleMatches(this.#playerId);
    } catch (e) {
      listEl.innerHTML = `<div class="text-danger p-3">${esc(e.message)}</div>`;
      return;
    }

    if (rows.length === 0) {
      listEl.innerHTML = '<div class="text-muted p-3">No matches to work on right now.</div>';
      return;
    }

    listEl.innerHTML = `<div class="list-group">${rows.map(m => this.#rowHtml(m)).join('')}</div>`;
  }

  #rowHtml(m) {
    // A player can be eligible for both sides only in unusual data
    // situations (e.g. an actual-participant record on the side opposite
    // their current roster team) -- home is preferred for display when
    // both are true, matching this row's single "Open" action either way
    // (Match Entry itself only shows mutation controls for sides the
    // eligible_home/eligible_away flags actually mark).
    const opponent = m.eligible_home ? m.away_team_name : m.home_team_name;
    const homeOrAway = m.eligible_home ? 'Home' : 'Away';
    const scoreStatus = m.completed ? 'Scored' : 'Not scored yet';
    return `<div class="list-group-item d-flex justify-content-between align-items-center flex-wrap gap-2">
      <div>
        <div class="fw-semibold">Week ${m.week_number} vs ${esc(opponent)} <span class="text-muted small">(${homeOrAway})</span></div>
        <div class="small text-muted">
          ${esc(scoreStatus)}
          &middot; Home ${stateBadge(m.home_approval.state)}
          &middot; Away ${stateBadge(m.away_approval.state)}
        </div>
      </div>
      <button class="btn btn-sm btn-primary" data-action="open-my-match"
        data-match-id="${m.match_id}" data-season-id="${m.season_id}"
        data-eligible-home="${m.eligible_home ? '1' : '0'}" data-eligible-away="${m.eligible_away ? '1' : '0'}">
        Open
      </button>
    </div>`;
  }
}

customElements.define('my-matches-page', MyMatchesPage);
