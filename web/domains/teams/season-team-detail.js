// <season-team-detail> - displays the season-specific detail and roster for one team.
//
// Public API:
//   showTeam(seasonId, teamId, team, editable) - fetch the roster and render the detail
//     panel.  team is the SeasonTeam object from the list; roster is fetched independently.
//     When editable is true (draft season), roster rows include remove buttons that emit
//     roster-remove-requested.  Call with null args (or via clear()) to return to initial.
//   clear() - return to the "select a team" prompt.
//
// Emits (bubbling):
//   roster-remove-requested - detail { seasonId, teamId, playerId, playerName }
//     Fired when a remove button is clicked in editable mode.  The coordinator
//     shows a confirm dialog and handles the DELETE.

class SeasonTeamDetail extends HTMLElement {
  #seasonId = null;
  #teamId   = null;
  #editable = false;
  #loadSeq  = 0;

  connectedCallback() {
    this.innerHTML = this.#initialHTML();
    this.addEventListener('click', e => this.#onClick(e));
  }

  showTeam(seasonId, teamId, team, editable = false) {
    this.#seasonId = seasonId ?? null;
    this.#teamId   = teamId   ?? null;
    this.#editable = editable;
    if (!this.#seasonId || !this.#teamId) {
      ++this.#loadSeq; // cancel any in-flight roster fetch
      this.innerHTML = this.#initialHTML();
      return;
    }
    this.#load(team);
  }

  clear() {
    this.showTeam(null, null, null);
  }

  // -- State templates ----------------------------------------------------------

  async #load(team) {
    const seq = ++this.#loadSeq;
    this.innerHTML = this.#loadingHTML();
    let roster;
    try {
      roster = await api('GET', `/seasons/${this.#seasonId}/teams/${this.#teamId}/roster`);
    } catch (e) {
      if (seq !== this.#loadSeq) return;
      this.innerHTML = this.#errorHTML(e.message);
      return;
    }
    if (seq !== this.#loadSeq) return;
    this.innerHTML = this.#detailHTML(team, roster);
  }

  #initialHTML() {
    return `<div class="text-muted small py-3 px-1 fst-italic">Select a team to view details.</div>`;
  }

  #loadingHTML() {
    return `
      <div class="text-muted text-center py-4 small">
        <span class="spinner-border spinner-border-sm me-2" role="status" aria-hidden="true"></span>Loading roster...
      </div>`;
  }

  #errorHTML(msg) {
    return `
      <div class="alert alert-danger py-2 px-3 small">
        <i class="bi bi-exclamation-triangle me-1" aria-hidden="true"></i>Failed to load: ${esc(msg)}
      </div>`;
  }

  #detailHTML(team, roster) {
    const numBadge = team.team_number
      ? `<span class="badge bg-secondary me-2" style="font-size:.72rem">#${esc(team.team_number)}</span>`
      : '';

    const permanentRow = (team.team_name && team.team_name !== team.season_name)
      ? `<div class="text-muted small mb-1">
           <i class="bi bi-tag me-1" aria-hidden="true"></i>Permanent name: ${esc(team.team_name)}
         </div>`
      : '';

    const captainRow = team.captain_name
      ? `<div class="small mb-1">
           <i class="bi bi-person-check me-1" aria-hidden="true"></i>
           <span class="fw-semibold">Captain:</span> ${esc(team.captain_name)}
         </div>`
      : `<div class="small text-muted mb-1 fst-italic">No captain assigned</div>`;

    // Use roster.length for the count badge so it reflects the freshly-fetched
    // roster rather than the potentially-stale roster_count on the team object.
    const removeColHeader = this.#editable
      ? `<th style="font-size:.75rem;padding:.35rem .5rem;width:2rem"></th>`
      : '';

    const rosterSection = roster.length === 0
      ? `<div class="text-muted small fst-italic py-2 px-3">No players on roster yet.</div>`
      : `<table class="table table-sm mb-0">
           <thead>
             <tr>
               <th style="font-size:.75rem;padding:.35rem .75rem;width:2.5rem">#</th>
               <th style="font-size:.75rem;padding:.35rem .75rem">Player</th>
               <th style="font-size:.75rem;padding:.35rem .75rem;width:3.5rem">HC</th>
               ${removeColHeader}
             </tr>
           </thead>
           <tbody>${roster.map(p => this.#playerRow(p)).join('')}</tbody>
         </table>`;

    return `
      <div class="card">
        <div class="team-card-header">
          <span>${numBadge}<span class="fw-semibold">${esc(team.season_name)}</span></span>
          <span class="badge bg-light text-dark border" style="font-size:.75rem" title="Roster size">
            <i class="bi bi-people me-1" aria-hidden="true"></i>${roster.length}
          </span>
        </div>
        <div class="card-body py-2 px-3">
          ${permanentRow}${captainRow}
        </div>
        <div class="card-body p-0 border-top">${rosterSection}</div>
      </div>`;
  }

  #playerRow(p) {
    const num = p.player_number
      ? esc(p.player_number)
      : `<span class="text-muted">-</span>`;
    const hc  = (p.handicap != null)
      ? `<span class="badge bg-secondary" style="font-size:.72rem">${p.handicap}</span>`
      : `<span class="text-muted">-</span>`;
    const removeCell = this.#editable
      ? `<td class="text-end pe-2">
           <button type="button"
                   class="roster-remove-btn btn btn-link btn-sm text-danger p-0"
                   data-player-id="${p.player_id}"
                   data-player-name="${esc(p.player_name)}"
                   aria-label="Remove ${esc(p.player_name)} from roster"
                   title="Remove from roster">
             <i class="bi bi-x-circle" aria-hidden="true"></i>
           </button>
         </td>`
      : '';
    return `
      <tr class="roster-row">
        <td class="text-muted">${num}</td>
        <td>${esc(p.player_name)}</td>
        <td>${hc}</td>
        ${removeCell}
      </tr>`;
  }

  // -- Interaction ------------------------------------------------------------

  #onClick(e) {
    const btn = e.target.closest('.roster-remove-btn[data-player-id]');
    if (!btn) return;
    const playerId   = parseInt(btn.dataset.playerId, 10);
    const playerName = btn.dataset.playerName; // HTML parser decoded entities
    this.dispatchEvent(new CustomEvent('roster-remove-requested', {
      bubbles: true,
      detail: { seasonId: this.#seasonId, teamId: this.#teamId, playerId, playerName },
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

customElements.define('season-team-detail', SeasonTeamDetail);
