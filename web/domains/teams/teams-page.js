// <teams-page> - coordinator for the Teams screen.
// Owns the two-panel layout, the season selector, the viewing banner,
// and draft-season management controls.
//
// Routed events:
//   season-changed          -> refresh list, banner, draft mode; clear draft editors
//   team-selected           -> show team detail; show draft editors when draft
//   team-remove-requested   -> confirm + DELETE; refreshes UI only when the user
//                              is still viewing the same unlocked draft on completion
//   draft-team-mutated      -> toast always; refreshes UI only when the user is
//                              still viewing the same unlocked draft on completion
//   roster-remove-requested -> confirm + DELETE player from draft roster;
//                              refreshes UI only when still on the same draft team
//   draft-roster-mutated    -> toast always; refreshes UI only when still on
//                              the same draft team on completion
//   draft-captain-mutated   -> toast always; refreshes UI only when still on
//                              the same draft team on completion
//   draft-name-mutated      -> toast always; refreshes UI only when still on
//                              the same draft team on completion
//
// Public API:
//   refresh(leagueId, activeSeasonId) - immediately clear list, detail, banner,
//     draft controls, and draft editors, then start loading seasons for the league.
//     Called by loadTeams() in app.js when the active league or season changes.

class TeamsPage extends HTMLElement {
  #submitting       = false; // team add/remove guard
  #rosterSubmitting = false; // roster player remove guard (coordinator-side)
  #isDraft          = false;
  #selectedTeam     = null;

  connectedCallback() {
    this.innerHTML = `
      <div class="d-flex justify-content-between align-items-center mb-2 flex-wrap gap-2">
        <h4 class="mb-0 fw-bold">Teams</h4>
        <season-selector></season-selector>
      </div>
      <div class="teams-viewing-banner d-none mb-2" role="alert"></div>
      <draft-team-actions class="d-none mb-3"></draft-team-actions>
      <div class="row g-3">
        <div class="col-lg-5 col-xl-4">
          <season-team-list></season-team-list>
        </div>
        <div class="col-lg-7 col-xl-8">
          <season-team-detail></season-team-detail>
          <draft-team-name-editor class="d-none mt-3"></draft-team-name-editor>
          <draft-captain-editor class="d-none mt-3"></draft-captain-editor>
          <draft-roster-editor class="d-none mt-3"></draft-roster-editor>
        </div>
      </div>`;

    this.addEventListener('season-changed',          e => this.#onSeasonChanged(e));
    this.addEventListener('team-selected',           e => this.#onTeamSelected(e));
    this.addEventListener('team-remove-requested',   e => this.#onRemoveRequested(e));
    this.addEventListener('draft-team-mutated',      e => this.#onDraftTeamMutated(e));
    this.addEventListener('roster-remove-requested', e => this.#onRosterRemoveRequested(e));
    this.addEventListener('draft-roster-mutated',    e => this.#onDraftRosterMutated(e));
    this.addEventListener('draft-captain-mutated',   e => this.#onDraftCaptainMutated(e));
    this.addEventListener('draft-name-mutated',      e => this.#onDraftNameMutated(e));
  }

  refresh(leagueId, activeSeasonId) {
    this.#isDraft      = false;
    this.#selectedTeam = null;
    this.#list().reset();
    this.#detail().clear();
    this.#updateBanner(null);
    this.#updateDraftMode(null);
    this.#clearNameEditor();
    this.#clearCaptainEditor();
    this.#clearRosterEditor();
    this.#selector().load(leagueId ?? null, activeSeasonId ?? null);
  }

  // -- Internal -----------------------------------------------------------------

  #selector()           { return this.querySelector('season-selector'); }
  #list()               { return this.querySelector('season-team-list'); }
  #detail()             { return this.querySelector('season-team-detail'); }
  #draftActions()       { return this.querySelector('draft-team-actions'); }
  #draftNameEditor()    { return this.querySelector('draft-team-name-editor'); }
  #draftCaptainEditor() { return this.querySelector('draft-captain-editor'); }
  #draftRosterEditor()  { return this.querySelector('draft-roster-editor'); }

  #onSeasonChanged(e) {
    const { season } = e.detail;
    this.#isDraft      = !!(season && !season.activated_at);
    this.#selectedTeam = null;
    this.#list().refresh(season?.id ?? null, season?.name ?? '');
    this.#detail().clear();
    this.#updateBanner(season);
    this.#updateDraftMode(season);
    this.#clearNameEditor();
    this.#clearCaptainEditor();
    this.#clearRosterEditor();
  }

  #onTeamSelected(e) {
    const { seasonId, teamId, team } = e.detail;
    this.#selectedTeam = team;
    this.#detail().showTeam(seasonId, teamId, team, this.#isDraft);
    if (this.#isDraft) {
      const nameEl = this.#draftNameEditor();
      nameEl.classList.remove('d-none');
      nameEl.load(seasonId, teamId, team);

      const captainEl = this.#draftCaptainEditor();
      captainEl.classList.remove('d-none');
      captainEl.load(seasonId, teamId, team);

      const rosterEl = this.#draftRosterEditor();
      rosterEl.classList.remove('d-none');
      rosterEl.load(seasonId, teamId);
    }
  }

  async #onRemoveRequested(e) {
    if (this.#submitting) return;
    const { seasonId, teamId, teamName } = e.detail;
    const msg = `Remove "${teamName}" from this draft season?\n\n`
              + `The team's draft-season roster will also be removed.`;
    if (!confirm(msg)) return;
    this.#submitting = true;
    try {
      await api('DELETE', `/seasons/${seasonId}/teams/${teamId}`);
      toast(`${teamName} removed from draft season`);
      // Guard: only refresh UI if the user is still viewing the same unlocked draft.
      const sel = this.#selector().selectedSeason;
      if (sel?.id === seasonId && !sel.activated_at) {
        this.#afterMutation(seasonId);
      }
    } catch (err) {
      // Suppress stale error: only toast when still viewing the same unlocked draft.
      const errSel = this.#selector().selectedSeason;
      if (errSel?.id === seasonId && !errSel.activated_at) {
        toast(err.message, 'danger');
      }
    } finally {
      this.#submitting = false;
    }
  }

  #onDraftTeamMutated(e) {
    const { seasonId, message } = e.detail;
    // Always show the toast -- the backend mutation succeeded.
    toast(message);
    // Guard: only refresh UI if the user is still viewing the same unlocked draft.
    const sel = this.#selector().selectedSeason;
    if (sel?.id === seasonId && !sel.activated_at) {
      this.#afterMutation(seasonId);
    }
  }

  async #onRosterRemoveRequested(e) {
    if (this.#rosterSubmitting) return;
    const { seasonId, teamId, playerId, playerName } = e.detail;
    const msg = `Remove "${playerName}" from this draft-season roster?\n\n`
              + `This only affects the roster for this season. The player record is not deleted.`;
    if (!confirm(msg)) return;
    this.#rosterSubmitting = true;
    try {
      await api('DELETE', `/seasons/${seasonId}/teams/${teamId}/roster/${playerId}`);
      toast(`${playerName} removed from roster`);
      // Guard: only refresh if still viewing the same draft team.
      const sel = this.#selector().selectedSeason;
      if (sel?.id === seasonId && !sel.activated_at && this.#selectedTeam?.team_id === teamId) {
        this.#afterRosterMutation(seasonId, teamId);
      }
    } catch (err) {
      // Suppress stale error: only toast when still viewing the same unlocked draft team.
      const errSel = this.#selector().selectedSeason;
      if (errSel?.id === seasonId && !errSel.activated_at && this.#selectedTeam?.team_id === teamId) {
        toast(err.message, 'danger');
      }
    } finally {
      this.#rosterSubmitting = false;
    }
  }

  #onDraftRosterMutated(e) {
    const { seasonId, teamId, message } = e.detail;
    toast(message);
    // Guard: only refresh if still viewing the same draft team.
    const sel = this.#selector().selectedSeason;
    if (sel?.id === seasonId && !sel.activated_at && this.#selectedTeam?.team_id === teamId) {
      this.#afterRosterMutation(seasonId, teamId);
    }
  }

  #onDraftCaptainMutated(e) {
    const { seasonId, teamId, updatedTeam, message } = e.detail;
    // Always show the toast -- the backend mutation succeeded.
    toast(message);
    // Guard: only refresh UI if still viewing the same draft team.
    const sel = this.#selector().selectedSeason;
    if (sel?.id === seasonId && !sel.activated_at && this.#selectedTeam?.team_id === teamId) {
      this.#afterCaptainMutation(seasonId, teamId, updatedTeam);
    }
  }

  #onDraftNameMutated(e) {
    const { seasonId, teamId, updatedTeam, message } = e.detail;
    // Always show the toast -- the backend mutation succeeded.
    toast(message);
    // Guard: only refresh UI if still viewing the same draft team.
    const sel = this.#selector().selectedSeason;
    if (sel?.id === seasonId && !sel.activated_at && this.#selectedTeam?.team_id === teamId) {
      this.#afterNameMutation(seasonId, teamId, updatedTeam);
    }
  }

  // Refresh list, clear detail, and reload copy choices after a team add/remove.
  #afterMutation(seasonId) {
    this.#selectedTeam = null;
    const name = this.#selector().selectedSeason?.name ?? '';
    this.#list().refresh(seasonId, name);
    this.#detail().clear();
    this.#draftActions().load(seasonId);
    this.#clearNameEditor();
    this.#clearCaptainEditor();
    this.#clearRosterEditor();
  }

  // Refresh roster detail, available-player choices, captain choices, and team
  // count badges after a roster player add or remove.  Preserves team selection.
  #afterRosterMutation(seasonId, teamId) {
    const name = this.#selector().selectedSeason?.name ?? '';
    this.#detail().showTeam(seasonId, teamId, this.#selectedTeam, true);
    this.#draftCaptainEditor().load(seasonId, teamId, this.#selectedTeam);
    this.#draftRosterEditor().load(seasonId, teamId);
    this.#list().refreshCounts(seasonId, name, teamId);
  }

  // Update #selectedTeam to the server-returned value, re-render detail and
  // captain editor with new data, refresh team list count badges (captain name
  // appears in card).  Does not touch the roster editor.
  #afterCaptainMutation(seasonId, teamId, updatedTeam) {
    this.#selectedTeam = updatedTeam;
    const name = this.#selector().selectedSeason?.name ?? '';
    this.#detail().showTeam(seasonId, teamId, updatedTeam, true);
    this.#draftNameEditor().load(seasonId, teamId, updatedTeam);
    this.#draftCaptainEditor().load(seasonId, teamId, updatedTeam);
    this.#list().refreshCounts(seasonId, name, teamId);
  }

  // Update #selectedTeam, re-render detail heading with new season_name, reload
  // name editor (resets input to saved value), reload captain editor (forwarded
  // team has fresh captain_id), refresh team list card (card shows season_name).
  #afterNameMutation(seasonId, teamId, updatedTeam) {
    this.#selectedTeam = updatedTeam;
    const name = this.#selector().selectedSeason?.name ?? '';
    this.#detail().showTeam(seasonId, teamId, updatedTeam, true);
    this.#draftNameEditor().load(seasonId, teamId, updatedTeam);
    this.#draftCaptainEditor().load(seasonId, teamId, updatedTeam);
    this.#list().refreshCounts(seasonId, name, teamId);
  }

  #clearNameEditor() {
    const el = this.#draftNameEditor();
    el.classList.add('d-none');
    el.clear();
  }

  #clearCaptainEditor() {
    const el = this.#draftCaptainEditor();
    el.classList.add('d-none');
    el.clear();
  }

  #clearRosterEditor() {
    const el = this.#draftRosterEditor();
    el.classList.add('d-none');
    el.clear();
  }

  #updateBanner(season) {
    const el = this.querySelector('.teams-viewing-banner');
    if (!el) return;
    if (!season || season.active) {
      el.className = 'teams-viewing-banner d-none mb-2';
      el.innerHTML = '';
      return;
    }
    const historical = !!season.activated_at;
    const label = historical ? 'Historical' : 'Draft';
    const color = historical ? 'warning' : 'info';
    el.className = `teams-viewing-banner alert alert-${color} py-1 px-3 small mb-2`;
    el.innerHTML = `<i class="bi bi-clock-history me-1" aria-hidden="true"></i>
      Viewing <strong>${esc(season.name)}</strong> &mdash; ${label} season`;
  }

  #updateDraftMode(season) {
    const isDraft = !!(season && !season.activated_at);
    const draftEl = this.#draftActions();
    if (isDraft) {
      draftEl.classList.remove('d-none');
      draftEl.load(season.id);
    } else {
      draftEl.classList.add('d-none');
      // Cancel any pending load and wipe stale content so it cannot flash
      // as current data if the user returns to this draft before the new
      // load resolves.
      draftEl.clear();
    }
    this.#list().setEditable(isDraft);
  }
}

function esc(s) {
  return String(s ?? '')
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

customElements.define('teams-page', TeamsPage);
