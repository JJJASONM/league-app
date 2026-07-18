(function () {
  function createShellContext(opts) {
    const api = opts.api;
    const toast = opts.toast;
    const selectEl = opts.selectEl;
    const labelEl = opts.labelEl;
    const storage = opts.storage || window.localStorage;

    const state = {
      allLeagues: [],
      activeLeague: null,
      allTeams: [],
      allPlayers: [],
      allSeasons: [],
      activeSeason: null,
      entryPreSelectSeasonId: null,
      entryPreSelectMatchId: null,
    };

    function updateActiveSeasonLabel() {
      labelEl.textContent = state.activeSeason
        ? '\u{1F4C5} ' + state.activeSeason.name
        : 'No active season';
    }

    function renderLeagueOptions() {
      if (state.allLeagues.length === 0) {
        selectEl.innerHTML = '<option value="">No leagues - add one</option>';
        return;
      }

      selectEl.innerHTML = state.allLeagues.map(function (league) {
        const selected = state.activeLeague && league.id === state.activeLeague.id ? ' selected' : '';
        return '<option value="' + league.id + '"' + selected + '>' + league.name + '</option>';
      }).join('');
    }

    async function loadLeagueData() {
      if (!state.activeLeague) return;
      const leagueId = state.activeLeague.id;
      try {
        const results = await Promise.all([
          api('GET', '/teams?league_id=' + leagueId),
          api('GET', '/players?league_id=' + leagueId),
          api('GET', '/seasons?league_id=' + leagueId),
        ]);
        state.allTeams = results[0];
        state.allPlayers = results[1];
        state.allSeasons = results[2];
        state.activeSeason = state.allSeasons.find(function (season) {
          return season.active;
        }) || null;
        updateActiveSeasonLabel();
      } catch (e) {
        toast('Failed to load league data: ' + e.message, 'danger');
      }
    }

    async function init() {
      try {
        state.allLeagues = await api('GET', '/leagues');
      } catch (e) {
        toast('Could not load leagues: ' + e.message, 'danger');
        state.allLeagues = [];
      }

      if (state.allLeagues.length === 0) {
        state.activeLeague = null;
        renderLeagueOptions();
        updateActiveSeasonLabel();
        return;
      }

      const saved = parseInt(storage.getItem('activeLeagueId'), 10);
      const restored = state.allLeagues.find(function (league) {
        return league.id === saved;
      });
      state.activeLeague = restored || state.allLeagues[0];

      renderLeagueOptions();
      selectEl.value = state.activeLeague.id;

      await loadLeagueData();
    }

    async function switchLeague(leagueId) {
      state.activeLeague = state.allLeagues.find(function (league) {
        return league.id === leagueId;
      }) || null;
      if (state.activeLeague) storage.setItem('activeLeagueId', String(state.activeLeague.id));
      await loadLeagueData();
    }

    function getState() {
      return state;
    }

    function setEntryPreselect(seasonId, matchId) {
      state.entryPreSelectSeasonId = seasonId;
      state.entryPreSelectMatchId = matchId;
    }

    function consumeEntryPreselect() {
      const detail = {
        seasonId: state.entryPreSelectSeasonId,
        matchId: state.entryPreSelectMatchId,
      };
      state.entryPreSelectSeasonId = null;
      state.entryPreSelectMatchId = null;
      return detail;
    }

    function applySeasonState(detail) {
      state.allSeasons = detail.allSeasons;
      state.activeSeason = detail.activeSeason;
      updateActiveSeasonLabel();
    }

    function applyPlayersState(players) {
      state.allPlayers = players;
    }

    async function applyLeaguesChanged(detail) {
      state.allLeagues = detail.leagues;

      if (detail.deletedId != null && state.activeLeague && state.activeLeague.id === detail.deletedId) {
        state.activeLeague = state.allLeagues[0] || null;
        if (state.activeLeague) storage.setItem('activeLeagueId', String(state.activeLeague.id));
        else storage.removeItem('activeLeagueId');
      }

      renderLeagueOptions();

      if (state.allLeagues.length === 0) {
        state.activeLeague = null;
        updateActiveSeasonLabel();
        return;
      }

      if (state.activeLeague) {
        selectEl.value = state.activeLeague.id;
      }
    }

    return {
      applyLeaguesChanged: applyLeaguesChanged,
      applyPlayersState: applyPlayersState,
      applySeasonState: applySeasonState,
      consumeEntryPreselect: consumeEntryPreselect,
      getState: getState,
      init: init,
      loadLeagueData: loadLeagueData,
      setEntryPreselect: setEntryPreselect,
      switchLeague: switchLeague,
    };
  }

  window.LeagueAppContext = {
    createShellContext: createShellContext,
  };
})();
