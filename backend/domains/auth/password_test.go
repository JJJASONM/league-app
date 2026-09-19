package auth_test

import (
	"strings"
	"testing"
	"time"

	"league_app/backend/domains/auth"
)

func TestHashPassword_VerifyRoundTrip(t *testing.T) {
	encoded, err := auth.HashPassword("correct horse battery staple", auth.DefaultArgon2Params())
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	ok, err := auth.VerifyPassword("correct horse battery staple", encoded)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Error("want correct password to verify")
	}
}

func TestHashPassword_EncodedFormatIsSelfDescribing(t *testing.T) {
	p := auth.Argon2Params{Memory: 8 * 1024, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	encoded, err := auth.HashPassword("pw", p)
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=8192,t=2,p=1$") {
		t.Errorf("want self-describing encoded prefix, got %q", encoded)
	}
}

func TestVerifyPassword_WrongPasswordFails(t *testing.T) {
	encoded, _ := auth.HashPassword("right-password", auth.DefaultArgon2Params())
	ok, err := auth.VerifyPassword("wrong-password", encoded)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if ok {
		t.Error("want wrong password to fail verification")
	}
}

func TestVerifyPassword_TwoHashesOfSamePasswordDiffer(t *testing.T) {
	p := auth.Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	a, _ := auth.HashPassword("same-password", p)
	b, _ := auth.HashPassword("same-password", p)
	if a == b {
		t.Error("want two hashes of the same password to differ (random salt per call)")
	}
}

func TestVerifyPassword_RejectsMalformedEncodedHash(t *testing.T) {
	cases := []string{
		"",
		"not-an-argon2-hash",
		"$argon2id$v=99$m=8192,t=1,p=1$c2FsdA$aGFzaA",           // unsupported version
		"$argon2id$v=19$m=0,t=1,p=1$c2FsdA$aGFzaA",              // memory=0
		"$argon2id$v=19$m=99999999999,t=1,p=1$c2FsdA$aGFzaA",    // memory way over ceiling
		"$argon2id$v=19$m=8192,t=999,p=1$c2FsdA$aGFzaA",         // iterations over ceiling
		"$argon2id$v=19$m=8192,t=1,p=999$c2FsdA$aGFzaA",         // parallelism over ceiling
		"$argon2id$v=19$m=8192,t=1,p=1$not-base64!!!$aGFzaA",    // invalid salt encoding
	}
	for _, c := range cases {
		if _, err := auth.VerifyPassword("anything", c); err == nil {
			t.Errorf("want error verifying against malformed hash %q, got nil", c)
		}
	}
}

func TestNeedsRehash_DetectsBelowTargetParameters(t *testing.T) {
	low := auth.Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
	high := auth.Argon2Params{Memory: 64 * 1024, Iterations: 3, Parallelism: 2, SaltLength: 16, KeyLength: 32}

	encoded, _ := auth.HashPassword("pw", low)

	needs, err := auth.NeedsRehash(encoded, high)
	if err != nil {
		t.Fatalf("NeedsRehash: %v", err)
	}
	if !needs {
		t.Error("want needs-rehash=true when stored params are below target")
	}

	needs, err = auth.NeedsRehash(encoded, low)
	if err != nil {
		t.Fatalf("NeedsRehash: %v", err)
	}
	if needs {
		t.Error("want needs-rehash=false when stored params already meet target")
	}
}

func TestCalibrateArgon2_ReturnsParamsMeetingOrExceedingTarget(t *testing.T) {
	// A tiny target so this test runs fast and deterministically finds a
	// satisfying iteration count without depending on real hardware speed.
	p, elapsed, err := auth.CalibrateArgon2(1 * time.Millisecond)
	if err != nil {
		t.Fatalf("CalibrateArgon2: %v", err)
	}
	if p.Iterations < 1 {
		t.Errorf("want at least 1 iteration, got %d", p.Iterations)
	}
	if elapsed <= 0 {
		t.Error("want a positive measured duration")
	}
}
