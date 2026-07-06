// <handicaps-page> — coordinator for the Handicap Review section.
// Owns the section layout, season selector, and <handicap-review> lifecycle.
//
// Public API:
//   refresh(allSeasons, activeSeason) — repopulate the season selector and
//     load the active season's recommendations, or reset when no seasons exist.
//     Called by the app shell when the Handicap tab is activated or when
//     league/season context changes.

import './handicap-review-component.js';

function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

class HandicapsPage extends HTMLElement {
  #allSeasons   = [];
  #activeSeason = null;

  connectedCallback() {
    this.innerHTML = `
      <div class="d-flex justify-content-between align-items-center mb-3">
        <h4 class="mb-0 fw-bold">Handicap Review</h4>
        <select class="form-select form-select-sm w-auto hcp-season-select"></select>
      </div>
      <handicap-review class="hcp-widget"></handicap-review>`;

    this.querySelector('.hcp-season-select').addEventListener('change', () => this.#onSelectChange());
  }

  refresh(allSeasons, activeSeason) {
    this.#allSeasons   = allSeasons   ?? [];
    this.#activeSeason = activeSeason ?? null;
    this.#populateSelect();
    this.#syncWidget();
  }

  // ── Private ──────────────────────────────────────────────────────────────────

  #populateSelect() {
    const sel = this.querySelector('.hcp-season-select');
    if (!sel) return;
    sel.innerHTML = this.#allSeasons.map(s =>
      `<option value="${s.id}"${s.active ? ' selected' : ''}>${esc(s.name)}</option>`
    ).join('') || '<option value="">No seasons</option>';
  }

  #syncWidget() {
    const widget = this.querySelector('.hcp-widget');
    if (!widget) return;
    const sel = this.querySelector('.hcp-season-select');
    const sid = sel?.value;
    if (sid) widget.loadSeason(sid);
    else     widget.reset();
  }

  #onSelectChange() {
    this.#syncWidget();
  }
}

customElements.define('handicaps-page', HandicapsPage);
