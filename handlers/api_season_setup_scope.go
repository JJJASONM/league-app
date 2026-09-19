package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"league_app/backend/domains/auth"
)

// peekJSONBody decodes r's JSON body into v without consuming it for the
// handler that runs afterward -- needed because a scopeFn (which runs
// BEFORE the handler, to resolve the request's authoritative league
// before calling auth.Authorize) sometimes needs a field out of the body
// itself (e.g. league_id on season creation), and the handler still needs
// to decode the same body again.
func peekJSONBody(r *http.Request, v any) error {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(data))
	return json.Unmarshal(data, v)
}

// createSeasonScope resolves the authoritative league for a season-create
// request directly from its body's league_id -- the season does not exist
// yet, so there is no other identifier to resolve it from.
func createSeasonScope(r *http.Request) (auth.Scope, bool, error) {
	var body struct {
		LeagueID int64 `json:"league_id"`
	}
	if err := peekJSONBody(r, &body); err != nil {
		return auth.Scope{}, false, err
	}
	if body.LeagueID == 0 {
		return auth.Scope{}, false, nil
	}
	return auth.Scope{LeagueID: &body.LeagueID}, true, nil
}

// seasonIDPathScope resolves the authoritative league for a route whose
// {id} path parameter is a season_id, by looking up that season's own
// league_id -- the general pattern PM's spec calls for: "if a route
// starts from season_id ... resolve that resource to its authoritative
// league before authorizing."
func seasonIDPathScope(seasonMgr SeasonManager) func(r *http.Request) (auth.Scope, bool, error) {
	return func(r *http.Request) (auth.Scope, bool, error) {
		id, err := pathID(r, "id")
		if err != nil {
			return auth.Scope{}, false, err
		}
		season, err := seasonMgr.GetSeason(r.Context(), id)
		if err != nil {
			return auth.Scope{}, false, nil
		}
		leagueID := season.LeagueID
		return auth.Scope{LeagueID: &leagueID}, true, nil
	}
}
