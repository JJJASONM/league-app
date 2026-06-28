// Package rules owns rule definitions and authoritative value validation.
package rules

import (
	"fmt"
	"strconv"
)

type ValueType string

const (
	TypeNumber  ValueType = "number"
	TypeInteger ValueType = "integer"
	TypeBoolean ValueType = "boolean"
	TypeChoice  ValueType = "choice"
)

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

type Definition struct {
	Domain       string    `json:"domain"`
	Group        string    `json:"group"`
	GroupLabel   string    `json:"group_label"`
	GroupOrder   int       `json:"group_order"`
	Order        int       `json:"order"`
	Key          string    `json:"key"`
	Label        string    `json:"label"`
	Type         ValueType `json:"type"`
	DefaultValue string    `json:"default_value"`
	Minimum      *float64  `json:"minimum,omitempty"`
	Maximum      *float64  `json:"maximum,omitempty"`
	Step         *float64  `json:"step,omitempty"`
	Options      []Option  `json:"options,omitempty"`
	Help         string    `json:"help"`
	Status       string    `json:"status"`
	Version      string    `json:"version"`
}

func number(value float64) *float64 {
	return &value
}

var definitions = []Definition{
	{
		Domain: "handicaps", Group: "handicap", GroupLabel: "Handicap Settings",
		GroupOrder: 10, Order: 10, Key: "max_individual_handicap",
		Label: "Max individual handicap on scoresheet", Type: TypeNumber,
		DefaultValue: "4.5", Step: number(0.01),
		Help:   "Caps each player's handicap value used in spot calculations. Players above this are treated as if they have this value.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "handicaps", Group: "handicap", GroupLabel: "Handicap Settings",
		GroupOrder: 10, Order: 15, Key: "min_ball_handicap",
		Label: "Minimum ball handicap", Type: TypeInteger,
		DefaultValue: "0", Minimum: number(0), Step: number(1),
		Help:   "Threshold: a computed spot below this value is treated as no spot (0). Zero disables the threshold. Example: min=2, computed=1 -> no spot; min=2, computed=2 -> spot applies; equal-rated players always 0.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "handicaps", Group: "handicap", GroupLabel: "Handicap Settings",
		GroupOrder: 10, Order: 20, Key: "handicap_multiplier",
		Label: "Handicap multiplier", Type: TypeNumber,
		DefaultValue: "2.55", Minimum: number(0.01), Step: number(0.01),
		Help:   "FileMaker formula: 0.85 × 3 = 2.55. Change only if the league formally agrees to a different rate. Affects all new match entries.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "handicaps", Group: "handicap", GroupLabel: "Handicap Settings",
		GroupOrder: 10, Order: 30, Key: "handicap_rounding",
		Label: "Rounding method", Type: TypeChoice, DefaultValue: "nearest",
		Options: []Option{
			{Value: "nearest", Label: "Round to nearest whole ball"},
			{Value: "floor", Label: "Always round down (floor)"},
			{Value: "ceiling", Label: "Always round up (ceiling)"},
		},
		Help:   "How fractional handicap spots are converted to whole balls on the scoresheet.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "handicaps", Group: "handicap", GroupLabel: "Handicap Settings",
		GroupOrder: 10, Order: 40, Key: "max_pairing_spot",
		Label: "Max spot per pairing", Type: TypeInteger,
		DefaultValue: "15", Minimum: number(0), Step: number(1),
		Help:   "Maximum handicap spot applied to any single pairing, regardless of handicap difference.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "handicaps", Group: "handicap", GroupLabel: "Handicap Settings",
		GroupOrder: 10, Order: 50, Key: "max_match_spot",
		Label: "Max total spot per match", Type: TypeInteger,
		DefaultValue: "15", Minimum: number(0), Step: number(1),
		Help:   "Maximum combined handicap spots across all pairings in one match.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "handicaps", Group: "handicap", GroupLabel: "Handicap Settings",
		GroupOrder: 10, Order: 60, Key: "handicap_update_method",
		Label: "Handicap update method", Type: TypeChoice, DefaultValue: "manual_review",
		Options: []Option{
			{Value: "manual_review", Label: "Manual review (operator approves each change)"},
			{Value: "game_diff_average", Label: "Game differential average (auto-compute)"},
			{Value: "kicker_average_preview", Label: "Kicker average (preview only, no auto-update)"},
		},
		Help:   "How player handicap changes are proposed at the end of each season.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "matches", Group: "lineup", GroupLabel: "Lineup Settings",
		GroupOrder: 20, Order: 10, Key: "lineup_players_per_team",
		Label: "Players per team per match", Type: TypeInteger,
		DefaultValue: "3", Minimum: number(1), Maximum: number(6), Step: number(1),
		Help:   "Number of active players each team fields each match night.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "matches", Group: "lineup", GroupLabel: "Lineup Settings",
		GroupOrder: 20, Order: 20, Key: "games_per_pairing",
		Label: "Games per pairing", Type: TypeInteger,
		DefaultValue: "3", Minimum: number(1), Maximum: number(5), Step: number(1),
		Help:   "Number of games played in each individual player matchup.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "matches", Group: "lineup", GroupLabel: "Lineup Settings",
		GroupOrder: 20, Order: 30, Key: "allow_substitutes",
		Label: "Allow substitutes", Type: TypeBoolean, DefaultValue: "true",
		Help:   "Whether teams may field substitute players not on their regular roster.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "handicaps", Group: "handicap", GroupLabel: "Handicap Settings",
		GroupOrder: 10, Order: 70, Key: "handicap_current_game_window",
		Label: "Current game window", Type: TypeInteger,
		DefaultValue: "15", Minimum: number(1), Step: number(1),
		Help:   "Number of most-recent eligible 8-ball individual game racks used for the current-window handicap calculation. Only racks with a valid opponent handicap snapshot count.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "handicaps", Group: "handicap", GroupLabel: "Handicap Settings",
		GroupOrder: 10, Order: 80, Key: "handicap_min_games_for_recommendation",
		Label: "Minimum games for recommendation", Type: TypeInteger,
		DefaultValue: "15", Minimum: number(1), Step: number(1),
		Help:   "Minimum included 8-ball game racks required before a handicap recommendation is generated. Racks excluded due to a missing opponent snapshot do not count toward this threshold.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "schedules", Group: "scheduling", GroupLabel: "Scheduling Settings",
		GroupOrder: 30, Order: 10, Key: "allow_bye_requests",
		Label: "Allow bye requests", Type: TypeBoolean, DefaultValue: "true",
		Help:   "Teams may submit a request to skip a scheduled week.",
		Status: "draft", Version: "0.1",
	},
	{
		Domain: "schedules", Group: "scheduling", GroupLabel: "Scheduling Settings",
		GroupOrder: 30, Order: 20, Key: "require_bye_approval",
		Label: "Require bye approval", Type: TypeBoolean, DefaultValue: "true",
		Help:   "Bye requests must be approved by the operator before taking effect.",
		Status: "draft", Version: "0.1",
	},
}

// Definitions returns a copy so callers cannot mutate the registry.
func Definitions() []Definition {
	result := make([]Definition, len(definitions))
	copy(result, definitions)
	return result
}

func Find(key string) (Definition, bool) {
	for _, definition := range definitions {
		if definition.Key == key {
			return definition, true
		}
	}
	return Definition{}, false
}

// ValidateValue validates developer-defined rules. Unknown keys remain valid
// for backward-compatible informational custom rules.
func ValidateValue(key, value string) error {
	definition, known := Find(key)
	if !known {
		return nil
	}

	switch definition.Type {
	case TypeBoolean:
		if value != "true" && value != "false" {
			return fmt.Errorf("%s must be true or false", definition.Label)
		}
	case TypeChoice:
		for _, option := range definition.Options {
			if option.Value == value {
				return nil
			}
		}
		return fmt.Errorf("%s has an unsupported value", definition.Label)
	case TypeNumber, TypeInteger:
		numberValue, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return fmt.Errorf("%s must be a number", definition.Label)
		}
		if definition.Type == TypeInteger && numberValue != float64(int64(numberValue)) {
			return fmt.Errorf("%s must be a whole number", definition.Label)
		}
		if definition.Minimum != nil && numberValue < *definition.Minimum {
			return fmt.Errorf("%s must be at least %v", definition.Label, *definition.Minimum)
		}
		if definition.Maximum != nil && numberValue > *definition.Maximum {
			return fmt.Errorf("%s must be at most %v", definition.Label, *definition.Maximum)
		}
	}
	return nil
}
