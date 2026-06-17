// <season-team-list> - displays teams participating in a specific season.
//
// Emits (bubbling) CustomEvents:
//   team-selected        - detail { seasonId, teamId, team }
//                          fired on card click / Enter / Space
//   team-remove-requested - detail { seasonId, teamId, teamName }
//                          fired when the remove button is activated
//                          (only emitted in editable mode)
//
// Public API:
//   reset()                                        - neutral state; cancels in-flight requests.
//   refresh(seasonId, seasonName)                  - load and render teams; clears selection.
//   refreshCounts(seasonId, seasonName, preserveTeamId) - re-fetch to update roster count badges;
//                                                    keeps preserveTeamId visually selected if present.
//   setEditable(editable)                          - toggle per-card remove buttons for draft seasons.

class SeasonTeamList extends HTMLElement {
  #seasonId        = null;
  #seasonName      = '';
  #selectedTeamId  = null;
  #teams           = [];
  #loadSeq         = 0;
  #editable        = false;

  connectedCallback() {
    this.innerHTML = '';
    this.addEventListener('click',   e => this.#onClick(e));
    this.addEventListener('keydown', e => this.#onKeydown(e));
  }

  // Immediately clear stale state and cancel any in-flight request.
  // Shows a neutral prompt; does not imply "no active season" since the
  // selector may still be loading.
  reset() {
    ++this.#loadSeq;
    this.#seasonId       = null;
    this.#seasonName     = '';
    this.#selectedTeamId = null;
    this.#teams          = [];
    this.innerHTML       = this.#resetHTML();
  }

  refresh(seasonId, seasonName) {
    this.#seasonId       = seasonId   ?? null;
    this.#seasonName     = seasonName ?? '';
    this.#selectedTeamId = null;
    this.#teams          = [];
    this.#load();
  }

  // Re-fetch team data to update roster count badges without showing a loading
  // spinner.  Restores the visual selection for preserveTeamId if the team is
  // still present in the response.  Called after a roster mutation so count
  // badges stay current without losing the user's selected team.
  refreshCounts(seasonId, seasonName, preserveTeamId) {
    this.#seasonId   = seasonId   ?? null;
    this.#seasonName = seasonName ?? '';
    // Do not reset #selectedTeamId or #teams.  #load(preserveTeamId) restores
    // the visual selection when the fresh response arrives.  Keeping #teams
    // populated during the in-flight fetch prevents #select() from dispatching
    // team: null if the user clicks a card before the response arrives.
    this.#load(preserveTeamId ?? null);
  }

  // Toggle remove buttons without reloading team data.
  setEditable(editable) {
    if (this.#editable === editable) return;
    this.#editable = editable;
    if (this.#teams.length > 0) {
      this.innerHTML = this.#teamsHTML(this.#teams);
      // Restore visual selection after re-render.
      if (this.#selectedTeamId !== null) {
        const card = this.querySelector(`.team-card[data-team-id="${this.#selectedTeamId}"]`);
        if (card) {
          card.classList.add('team-card--selected');
          card.setAttribute('aria-pressed', 'true');
        }
      }
    }
  }

  // -- State templates ----------------------------------------------------------

  // preserveTeamId: when provided (refreshCounts path), skip the loading spinner
  // and restore visual selection for that team if it appears in the response.
  async #load(preserveTeamId = null) {
    const seq = ++this.#loadSeq;
    if (!this.#seasonId) {
      this.innerHTML = this.#noSeasonHTML();
      return;
    }
    if (preserveTeamId === null) {
      this.innerHTML = this.#loadingHTML();
    }
    let teams;
    try {
      teams = await api('GET', `/seasons/${this.#seasonId}/teams`);
    } catch (e) {
      if (seq !== this.#loadSeq) return;
      this.innerHTML = this.#errorHTML(e.message);
      return;
    }
    if (seq !== this.#loadSeq) return;
    this.#teams = teams;
    if (preserveTeamId !== null && !teams.some(t => t.team_id === preserveTeamId)) {
      this.#selectedTeamId = null;
    }
    this.innerHTML = this.#teamsHTML(teams);
    if (this.#selectedTeamId !== null) {
      const card = this.querySelector(`.team-card[data-team-id="${this.#selectedTeamId}"]`);
      if (card) {
        card.classList.add('team-card--selected');
        card.setAttribute('aria-pressed', 'true');
      }
    }
  }

  #resetHTML() {
    return `
      <div class="text-muted small py-3 px-1 fst-italic">
        Select a season to view teams.
      </div>`;
  }

  #noSeasonHTML() {
    return `
      <div class="text-center py-5 text-muted">
        <i class="bi bi-calendar-x" style="font-size:2rem;display:block;margin-bottom:.5rem" aria-hidden="true"></i>
        <div class="fw-semibold mb-1">No active season</div>
        <div class="small">Select a season above to view its teams.</div>
      </div>`;
  }

  #loadingHTML() {
    return `
      <div class="text-muted text-center py-4 small">
        <span class="spinner-border spinner-border-sm me-2" role="status" aria-hidden="true"></span>Loading teams...
      </div>`;
  }

  #errorHTML(msg) {
    return `
      <div class="alert alert-danger py-2 px-3 small">
        <i class="bi bi-exclamation-triangle me-1" aria-hidden="true"></i>Failed to load teams: ${esc(msg)}
      </div>`;
  }

  #teamsHTML(teams) {
    const count = teams.length;
    const meta  = `${esc(this.#seasonName)} &mdash; ${count} team${count !== 1 ? 's' : ''}`;

    if (count === 0) {
      return `
        <div class="text-muted small mb-2">${meta}</div>
        <div class="text-muted py-3 small">
          <i class="bi bi-people me-1" aria-hidden="true"></i>No teams registered for this season yet.
        </div>`;
    }

    const cards = teams.map(t => this.#cardHTML(t)).join('');
    return `
      <div class="text-muted small mb-2">${meta}</div>
      <div class="row g-2">${cards}</div>`;
  }

  #cardHTML(t) {
    const numBadge = t.team_number
      ? `<span class="badge bg-secondary me-2" style="font-size:.72rem">#${esc(t.team_number)}</span>`
      : '';

    const captainLine = t.captain_name
      ? `<i class="bi bi-person-check me-1" aria-hidden="true"></i>${esc(t.captain_name)}`
      : `<span class="fst-italic">No captain assigned</span>`;

    const removeBtn = this.#editable
      ? `<button type="button"
                 class="btn btn-sm btn-link text-danger p-0 ms-2 team-remove-btn"
                 data-team-id="${t.team_id}"
                 data-team-name="${esc(t.season_name)}"
                 aria-label="Remove ${esc(t.season_name)} from this draft season"
                 title="Remove from draft season">
           <i class="bi bi-x-circle" aria-hidden="true"></i>
         </button>`
      : '';

    return `
      <div class="col-12">
        <div class="card team-card"
             data-team-id="${t.team_id}"
             tabindex="0"
             role="button"
             aria-pressed="false"
             aria-label="View ${esc(t.season_name)}">
          <div class="team-card-header">
            <span>${numBadge}<span class="fw-semibold">${esc(t.season_name)}</span></span>
            <span class="d-flex align-items-center gap-1">
              <span class="badge bg-light text-dark border" style="font-size:.75rem" title="Roster size">
                <i class="bi bi-people me-1" aria-hidden="true"></i>${t.roster_count}
              </span>
              ${removeBtn}
            </span>
          </div>
          <div class="card-body py-2 px-3 small text-muted">${captainLine}</div>
        </div>
      </div>`;
  }

  // -- Interaction --------------------------------------------------------------

  #onClick(e) {
    // Remove button takes priority; returning early prevents card selection.
    const removeBtn = e.target.closest('.team-remove-btn[data-team-id]');
    if (removeBtn) {
      this.#handleRemove(removeBtn);
      return;
    }
    const card = e.target.closest('.team-card[data-team-id]');
    if (!card) return;
    this.#select(parseInt(card.dataset.teamId, 10));
  }

  #onKeydown(e) {
    if (e.key !== 'Enter' && e.key !== ' ') return;
    if (e.target.tagName === 'BUTTON') return; // buttons handle their own activation
    const card = e.target.closest('.team-card[data-team-id]');
    if (!card) return;
    e.preventDefault();
    this.#select(parseInt(card.dataset.teamId, 10));
  }

  #select(teamId) {
    this.#selectedTeamId = teamId;
    this.querySelectorAll('.team-card').forEach(el => {
      const selected = parseInt(el.dataset.teamId, 10) === teamId;
      el.classList.toggle('team-card--selected', selected);
      el.setAttribute('aria-pressed', selected ? 'true' : 'false');
    });
    const team = this.#teams.find(t => t.team_id === teamId) ?? null;
    this.dispatchEvent(new CustomEvent('team-selected', {
      bubbles: true,
      detail: { seasonId: this.#seasonId, teamId, team },
    }));
  }

  #handleRemove(btn) {
    const teamId   = parseInt(btn.dataset.teamId, 10);
    const teamName = btn.dataset.teamName; // HTML parser already decoded entities
    this.dispatchEvent(new CustomEvent('team-remove-requested', {
      bubbles: true,
      detail: { seasonId: this.#seasonId, teamId, teamName },
    }));
  }
}

function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

customElements.define('season-team-list', SeasonTeamList);
