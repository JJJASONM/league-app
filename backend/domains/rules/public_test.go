package rules

import "testing"

func TestHandicapWindowRulesPresent(t *testing.T) {
	_, ok1 := Find("handicap_current_game_window")
	_, ok2 := Find("handicap_min_games_for_recommendation")
	if !ok1 {
		t.Error("handicap_current_game_window not found in registry")
	}
	if !ok2 {
		t.Error("handicap_min_games_for_recommendation not found in registry")
	}
}

func TestDefinitionsReturnsCopy(t *testing.T) {
	first := Definitions()
	first[0].Label = "changed"

	second := Definitions()
	if second[0].Label == "changed" {
		t.Fatal("Definitions returned mutable registry storage")
	}
}

func TestValidateValue(t *testing.T) {
	tests := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{name: "valid boolean", key: "allow_substitutes", value: "true"},
		{name: "invalid boolean", key: "allow_substitutes", value: "yes", wantErr: true},
		{name: "valid integer", key: "lineup_players_per_team", value: "3"},
		{name: "fractional integer", key: "lineup_players_per_team", value: "3.5", wantErr: true},
		{name: "integer below minimum", key: "lineup_players_per_team", value: "0", wantErr: true},
		{name: "integer above maximum", key: "lineup_players_per_team", value: "7", wantErr: true},
		{name: "min_ball_handicap zero", key: "min_ball_handicap", value: "0"},
		{name: "min_ball_handicap positive", key: "min_ball_handicap", value: "3"},
		{name: "min_ball_handicap negative", key: "min_ball_handicap", value: "-1", wantErr: true},
		{name: "min_ball_handicap fractional", key: "min_ball_handicap", value: "1.5", wantErr: true},
		{name: "valid choice", key: "handicap_rounding", value: "nearest"},
		{name: "invalid choice", key: "handicap_rounding", value: "random", wantErr: true},
		{name: "legacy custom rule", key: "rule_123", value: "any text"},
		// handicap_current_game_window
		{name: "game_window valid", key: "handicap_current_game_window", value: "15"},
		{name: "game_window zero", key: "handicap_current_game_window", value: "0", wantErr: true},
		{name: "game_window fractional", key: "handicap_current_game_window", value: "7.5", wantErr: true},
		// handicap_min_games_for_recommendation
		{name: "min_games valid", key: "handicap_min_games_for_recommendation", value: "15"},
		{name: "min_games zero", key: "handicap_min_games_for_recommendation", value: "0", wantErr: true},
		{name: "min_games fractional", key: "handicap_min_games_for_recommendation", value: "7.5", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := ValidateValue(test.key, test.value)
			if (err != nil) != test.wantErr {
				t.Fatalf("ValidateValue(%q, %q) error = %v, wantErr %v",
					test.key, test.value, err, test.wantErr)
			}
		})
	}
}
