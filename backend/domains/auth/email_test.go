package auth_test

import (
	"testing"

	"league_app/backend/domains/auth"
)

func TestNormalizeEmail_TrimsAndLowercases(t *testing.T) {
	got, err := auth.NormalizeEmail("  Alice@Example.COM  ")
	if err != nil {
		t.Fatalf("NormalizeEmail: %v", err)
	}
	if got != "alice@example.com" {
		t.Errorf("want alice@example.com, got %q", got)
	}
}

func TestNormalizeEmail_RejectsEmptyAndMalformed(t *testing.T) {
	cases := []string{"", "   ", "not-an-email", "@example.com", "alice@", "alice@example", "ali ce@example.com"}
	for _, c := range cases {
		if _, err := auth.NormalizeEmail(c); err == nil {
			t.Errorf("want error normalizing %q, got nil", c)
		}
	}
}

func TestNormalizeEmail_TwoDifferentlyCasedInputsCollide(t *testing.T) {
	a, err := auth.NormalizeEmail("Bob@League.com")
	if err != nil {
		t.Fatalf("NormalizeEmail: %v", err)
	}
	b, err := auth.NormalizeEmail("bob@league.com")
	if err != nil {
		t.Fatalf("NormalizeEmail: %v", err)
	}
	if a != b {
		t.Errorf("want normalized forms to collide, got %q vs %q", a, b)
	}
}
