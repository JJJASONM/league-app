// <draft-team-name-editor> - edit the season-specific team name for a draft-season team.
//
// No fetch needed: the team object is passed in via load().  The input is
// pre-filled from team.season_name.  On save the current captain_id is
// forwarded unchanged so the PUT endpoint never inadvertently blanks it.
//
// Public API:
//   load(seasonId, teamId, team) - render the name input pre-filled with team.season_name.
//   clear()                      - increment #ctx and wipe content.
//
// Emits (bubbling):
//   draft-name-mutated - detail { seasonId, teamId, updatedTeam, message }
//     Fired on successful PUT.  updatedTeam is the SeasonTeam returned by the API.
//
// Stale mutation-error suppression: #ctx is incremented by load() and clear();
//   captured before the mutation await; catch block writes to the DOM only when
//   this.#ctx === ctx (same pattern as <draft-captain-editor>).
// Unchanged guard: Save button starts disabled; enables only when the trimmed
//   input value differs from the current season_name AND is non-empty.

class DraftTeamNameEditor extends HTMLElement {
  #seasonId   = null;
  #teamId     = null;
  #team       = null;   // SeasonTeam -- captain_id forwarded in the PUT body
  #ctx        = 0;      // context token; incremented on every load() and clear()
  #submitting = false;

  connectedCallback() {
    this.addEventListener('input', e => this.#onInput(e));
    this.addEventListener('click', e => this.#onClick(e));
    this.addEventListener('keydown', e => {
      if (e.key === 'Enter' && e.target.matches('.name-input')) this.#saveName();
    });
  }

  load(seasonId, teamId, team) {
    ++this.#ctx;
    this.#seasonId = seasonId ?? null;
    this.#teamId   = teamId   ?? null;
    this.#team     = team     ?? null;
    this.innerHTML = this.#controlsHTML();
  }

  clear() {
    ++this.#ctx;
    this.#seasonId = null;
    this.#teamId   = null;
    this.#team     = null;
    this.innerHTML = '';
  }

  // -- Rendering ----------------------------------------------------------------

  #controlsHTML() {
    const seasonName    = this.#team?.season_name ?? '';
    const permanentName = this.#team?.team_name   ?? '';
    const permanentRow  = (permanentName && permanentName !== seasonName)
      ? `<div class="form-text">
           <i class="bi bi-tag me-1" aria-hidden="true"></i>Permanent name: ${esc(permanentName)}
         </div>`
      : '';
    return `
      <div class="card">
        <div class="card-header small fw-semibold py-2">Season Team Name</div>
        <div class="card-body py-3">
          <div class="d-flex gap-2 align-items-start flex-wrap">
            <div style="flex:1;min-width:12rem">
              <input type="text"
                     class="form-control form-control-sm name-input"
                     value="${esc(seasonName)}"
                     maxlength="100"
                     aria-label="Season team name"
                     placeholder="Team name for this season">
              ${permanentRow}
            </div>
            <button type="button"
                    class="btn btn-sm btn-outline-secondary name-save-btn"
                    disabled>Save Name</button>
          </div>
          <div class="name-error text-danger small mt-1" role="alert"></div>
        </div>
      </div>`;
  }

  // -- Mutation -----------------------------------------------------------------

  async #saveName() {
    if (this.#submitting) return;
    const input = this.querySelector('.name-input');
    const btn   = this.querySelector('.name-save-btn');
    if (!input) return;

    const newName = input.value.trim();
    if (!newName) {
      this.#showError('Team name is required.');
      return;
    }
    if (newName === (this.#team?.season_name ?? '')) return;

    const seasonId  = this.#seasonId;
    const teamId    = this.#teamId;
    const captainId = this.#team?.captain_id ?? null;
    const ctx       = this.#ctx;

    this.#submitting = true;
    if (btn) btn.disabled = true;
    this.#clearError();
    try {
      const updated = await api('PUT', `/seasons/${seasonId}/teams/${teamId}`, {
        season_name: newName,
        captain_id:  captainId,
      });
      this.#emit(seasonId, teamId, updated, `Team name updated to “${updated.season_name}”`);
    } catch (e) {
      if (this.#ctx === ctx) {
        this.#showError(e.message);
        if (btn) btn.disabled = false;
      }
    } finally {
      this.#submitting = false;
    }
  }

  #emit(seasonId, teamId, updatedTeam, message) {
    this.dispatchEvent(new CustomEvent('draft-name-mutated', {
      bubbles: true,
      detail: { seasonId, teamId, updatedTeam, message },
    }));
  }

  #showError(msg) {
    const el = this.querySelector('.name-error');
    if (el) el.textContent = msg;
  }

  #clearError() {
    const el = this.querySelector('.name-error');
    if (el) el.textContent = '';
  }

  // -- Interaction --------------------------------------------------------------

  #onInput(e) {
    if (!e.target.matches('.name-input')) return;
    const btn      = this.querySelector('.name-save-btn');
    if (!btn) return;
    const newVal   = e.target.value.trim();
    const original = this.#team?.season_name ?? '';
    btn.disabled = !newVal || newVal === original;
  }

  #onClick(e) {
    if (e.target.closest('.name-save-btn')) this.#saveName();
  }
}

function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

customElements.define('draft-team-name-editor', DraftTeamNameEditor);
