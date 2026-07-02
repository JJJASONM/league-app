package sqlite_test

import (
	"context"
	"errors"
	"testing"

	"league_app/backend/domainerr"
	"league_app/backend/storage/sqlite"
	"league_app/db"
	"league_app/models"
)

func initRuleDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := db.Init(dir); err != nil {
		t.Fatalf("db.Init: %v", err)
	}
	t.Cleanup(func() { db.DB.Close() })
}

// ruleStoreSeed inserts a league + season and returns the seasonID.
func ruleStoreSeed(t *testing.T) int64 {
	t.Helper()
	r, err := db.DB.Exec(`INSERT INTO leagues (name, game_format, day_of_week) VALUES ('L','8ball','Monday')`)
	if err != nil {
		t.Fatalf("insert league: %v", err)
	}
	lid, _ := r.LastInsertId()
	r, err = db.DB.Exec(
		`INSERT INTO seasons (league_id, name, start_date, schedule_type, num_weeks) VALUES (?,?,?,?,?)`,
		lid, "S1", "2026-01-01", "single_rr", 4)
	if err != nil {
		t.Fatalf("insert season: %v", err)
	}
	sid, _ := r.LastInsertId()
	return sid
}

func TestRuleStore_ListBySeasonID_EmptyWhenNoRules(t *testing.T) {
	initRuleDB(t)
	seasonID := ruleStoreSeed(t)
	store := sqlite.NewRuleStore(db.DB)

	rows, err := store.ListBySeasonID(context.Background(), seasonID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("want 0 rows, got %d", len(rows))
	}
}

func TestRuleStore_ListBySeasonID_ReturnsRowsOrderedByID(t *testing.T) {
	initRuleDB(t)
	seasonID := ruleStoreSeed(t)
	store := sqlite.NewRuleStore(db.DB)
	ctx := context.Background()

	_, err := store.Upsert(ctx, models.SeasonRule{SeasonID: seasonID, RuleKey: "k1", RuleLabel: "L1", RuleValue: "v1"})
	if err != nil {
		t.Fatalf("upsert 1: %v", err)
	}
	_, err = store.Upsert(ctx, models.SeasonRule{SeasonID: seasonID, RuleKey: "k2", RuleLabel: "L2", RuleValue: "v2"})
	if err != nil {
		t.Fatalf("upsert 2: %v", err)
	}

	rows, err := store.ListBySeasonID(ctx, seasonID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("want 2 rows, got %d", len(rows))
	}
	if rows[0].RuleKey != "k1" || rows[1].RuleKey != "k2" {
		t.Errorf("want k1,k2 order, got %q %q", rows[0].RuleKey, rows[1].RuleKey)
	}
}

func TestRuleStore_Upsert_InsertsNewRow(t *testing.T) {
	initRuleDB(t)
	seasonID := ruleStoreSeed(t)
	store := sqlite.NewRuleStore(db.DB)

	saved, err := store.Upsert(context.Background(), models.SeasonRule{
		SeasonID: seasonID, RuleKey: "allow_substitutes", RuleLabel: "Subs", RuleValue: "true",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if saved.ID == 0 {
		t.Error("want assigned ID, got 0")
	}
	if saved.RuleValue != "true" {
		t.Errorf("want RuleValue=true, got %q", saved.RuleValue)
	}
}

func TestRuleStore_Upsert_ReplacesExistingKey(t *testing.T) {
	initRuleDB(t)
	seasonID := ruleStoreSeed(t)
	store := sqlite.NewRuleStore(db.DB)
	ctx := context.Background()

	store.Upsert(ctx, models.SeasonRule{SeasonID: seasonID, RuleKey: "allow_substitutes", RuleLabel: "Subs", RuleValue: "true"})
	store.Upsert(ctx, models.SeasonRule{SeasonID: seasonID, RuleKey: "allow_substitutes", RuleLabel: "Subs", RuleValue: "false"})

	rows, _ := store.ListBySeasonID(ctx, seasonID)
	if len(rows) != 1 {
		t.Fatalf("want 1 row after replace, got %d", len(rows))
	}
	if rows[0].RuleValue != "false" {
		t.Errorf("want RuleValue=false after replace, got %q", rows[0].RuleValue)
	}
}

func TestRuleStore_GetByID_ReturnsRow(t *testing.T) {
	initRuleDB(t)
	seasonID := ruleStoreSeed(t)
	store := sqlite.NewRuleStore(db.DB)
	ctx := context.Background()

	inserted, _ := store.Upsert(ctx, models.SeasonRule{SeasonID: seasonID, RuleKey: "games_per_pairing", RuleLabel: "GPP", RuleValue: "3"})
	got, err := store.GetByID(ctx, inserted.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.RuleKey != "games_per_pairing" {
		t.Errorf("want RuleKey=games_per_pairing, got %q", got.RuleKey)
	}
}

func TestRuleStore_GetByID_NotFoundWhenAbsent(t *testing.T) {
	initRuleDB(t)
	ruleStoreSeed(t)
	store := sqlite.NewRuleStore(db.DB)

	_, err := store.GetByID(context.Background(), 9999)
	if err == nil {
		t.Fatal("want error for missing ID, got nil")
	}
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.NotFound {
		t.Errorf("want NotFound domainerr, got %v", err)
	}
}

func TestRuleStore_UpdateByID_UpdatesLabelAndValue(t *testing.T) {
	initRuleDB(t)
	seasonID := ruleStoreSeed(t)
	store := sqlite.NewRuleStore(db.DB)
	ctx := context.Background()

	inserted, _ := store.Upsert(ctx, models.SeasonRule{SeasonID: seasonID, RuleKey: "allow_substitutes", RuleLabel: "Old", RuleValue: "true"})
	if err := store.UpdateByID(ctx, inserted.ID, "New Label", "false"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got, _ := store.GetByID(ctx, inserted.ID)
	if got.RuleLabel != "New Label" || got.RuleValue != "false" {
		t.Errorf("want New Label/false, got %q/%q", got.RuleLabel, got.RuleValue)
	}
}

func TestRuleStore_GetValue_ReturnsFalseWhenAbsent(t *testing.T) {
	initRuleDB(t)
	seasonID := ruleStoreSeed(t)
	store := sqlite.NewRuleStore(db.DB)

	_, exists, err := store.GetValue(context.Background(), seasonID, "handicap_multiplier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Error("want exists=false for absent key, got true")
	}
}

func TestRuleStore_GetValue_ReturnsValueWhenPresent(t *testing.T) {
	initRuleDB(t)
	seasonID := ruleStoreSeed(t)
	store := sqlite.NewRuleStore(db.DB)
	ctx := context.Background()

	store.Upsert(ctx, models.SeasonRule{SeasonID: seasonID, RuleKey: "handicap_multiplier", RuleLabel: "M", RuleValue: "3.00"})

	value, exists, err := store.GetValue(ctx, seasonID, "handicap_multiplier")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("want exists=true for present key, got false")
	}
	if value != "3.00" {
		t.Errorf("want value=%q, got %q", "3.00", value)
	}
}

func TestRuleStore_GetValue_ReturnsTrueForBlankStoredValue(t *testing.T) {
	initRuleDB(t)
	seasonID := ruleStoreSeed(t)
	store := sqlite.NewRuleStore(db.DB)
	ctx := context.Background()

	store.Upsert(ctx, models.SeasonRule{SeasonID: seasonID, RuleKey: "allow_substitutes", RuleLabel: "Subs", RuleValue: ""})

	value, exists, err := store.GetValue(ctx, seasonID, "allow_substitutes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("want exists=true for blank-but-present key, got false")
	}
	if value != "" {
		t.Errorf("want empty value, got %q", value)
	}
}

func TestRuleStore_DeleteByID_RemovesRow(t *testing.T) {
	initRuleDB(t)
	seasonID := ruleStoreSeed(t)
	store := sqlite.NewRuleStore(db.DB)
	ctx := context.Background()

	inserted, _ := store.Upsert(ctx, models.SeasonRule{SeasonID: seasonID, RuleKey: "allow_substitutes", RuleLabel: "Subs", RuleValue: "true"})
	if err := store.DeleteByID(ctx, inserted.ID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_, err := store.GetByID(ctx, inserted.ID)
	var de *domainerr.Err
	if !errors.As(err, &de) || de.Category != domainerr.NotFound {
		t.Errorf("want NotFound after delete, got %v", err)
	}
}
