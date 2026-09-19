package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

// argon2Version is the Argon2 reference implementation version this
// package encodes and expects on decode (RFC 9106's "v=19"). A stored
// hash carrying any other version is rejected outright rather than
// guessed at.
const argon2Version = 19

// Argon2Params is a versioned, self-describing set of Argon2id tuning
// parameters. Because every encoded hash carries its own parameters (see
// HashPassword's output format), two accounts hashed under different
// settings both verify correctly with no schema change, and raising the
// target later only means "rehash on next successful login if the stored
// hash's parameters are below the new target" (see NeedsRehash) -- never a
// forced mass reset.
type Argon2Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// defaultArgon2Params is a reasonable starting point ONLY -- per PM
// instruction this must not be used as a hardcoded production target.
// CalibrateArgon2 measures actual hash duration on the real deployment
// hardware and returns the parameters that should actually be used; see
// cmd usage in QUICKSTART.md and the calibration evidence recorded in the
// handoff for this phase.
var defaultArgon2Params = Argon2Params{
	Memory:      64 * 1024, // 64 MiB
	Iterations:  3,
	Parallelism: 2,
	SaltLength:  16,
	KeyLength:   32,
}

// DefaultArgon2Params returns a copy of the built-in starting point, for
// callers (such as CalibrateArgon2) that need a baseline to measure from.
func DefaultArgon2Params() Argon2Params { return defaultArgon2Params }

// strict parser limits (see decodeArgon2Hash): a corrupted or maliciously
// crafted stored hash must never be able to force excessive memory/CPU use
// simply by being verified against.
const (
	maxArgon2MemoryKiB   = 1 * 1024 * 1024 // 1 GiB ceiling
	maxArgon2Iterations  = 50
	maxArgon2Parallelism = 16
	maxArgon2SaltLen     = 128
	maxArgon2HashLen     = 256
)

// HashPassword returns a versioned, self-describing Argon2id encoded hash
// string in the form:
//
//	$argon2id$v=19$m=<memory_kib>,t=<iterations>,p=<parallelism>$<salt>$<hash>
//
// A fresh random salt is generated per call -- never reused across
// passwords or accounts.
func HashPassword(password string, p Argon2Params) (string, error) {
	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate salt: %w", err)
	}
	hash := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version, p.Memory, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// VerifyPassword reports whether password matches the given encoded hash,
// using a constant-time comparison of the resulting key material (never a
// byte-by-byte == or a comparison that could short-circuit on the first
// differing byte).
func VerifyPassword(password, encoded string) (bool, error) {
	p, salt, hash, err := decodeArgon2Hash(encoded)
	if err != nil {
		return false, err
	}
	candidate := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, uint32(len(hash)))
	return subtle.ConstantTimeCompare(hash, candidate) == 1, nil
}

// NeedsRehash reports whether encoded's parameters fall below target,
// meaning it should be re-hashed (at next successful login) under the
// current target instead.
func NeedsRehash(encoded string, target Argon2Params) (bool, error) {
	p, _, _, err := decodeArgon2Hash(encoded)
	if err != nil {
		return false, err
	}
	return p.Memory < target.Memory || p.Iterations < target.Iterations || p.Parallelism < target.Parallelism, nil
}

// decodeArgon2Hash parses HashPassword's encoded format back into its
// parameters, salt, and hash bytes, rejecting anything that does not
// exactly match the expected shape and version, and rejecting any
// parameter or length outside a sane range -- a strict parser is a
// deliberate control: a corrupted or tampered stored value must fail
// closed, not be interpreted permissively and used to drive an expensive
// hash computation with attacker-influenced cost parameters.
func decodeArgon2Hash(encoded string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, fmt.Errorf("invalid encoded hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("invalid version segment")
	}
	if version != argon2Version {
		return Argon2Params{}, nil, nil, fmt.Errorf("unsupported argon2 version %d", version)
	}

	var p Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("invalid parameter segment")
	}
	if p.Memory == 0 || p.Memory > maxArgon2MemoryKiB ||
		p.Iterations == 0 || p.Iterations > maxArgon2Iterations ||
		p.Parallelism == 0 || p.Parallelism > maxArgon2Parallelism {
		return Argon2Params{}, nil, nil, fmt.Errorf("encoded hash parameters out of allowed range")
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("invalid salt encoding")
	}
	hash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, fmt.Errorf("invalid hash encoding")
	}
	if len(salt) == 0 || len(salt) > maxArgon2SaltLen || len(hash) == 0 || len(hash) > maxArgon2HashLen {
		return Argon2Params{}, nil, nil, fmt.Errorf("encoded hash salt/hash length out of allowed range")
	}
	return p, salt, hash, nil
}

// CalibrationProbePassword is a fixed, non-secret password used only to
// measure hash duration -- never used for a real account.
const CalibrationProbePassword = "argon2-calibration-probe"

// CalibrateArgon2 measures actual Argon2id hash duration on THIS hardware,
// starting from a baseline parameter set and increasing iterations until
// targetDuration is reached (capped at 20 iterations, a sane upper bound
// for a login-path hash). This exists specifically because PM instructed
// against hardcoding an unmeasured constant: the returned parameters and
// the measured duration for the final candidate are what should actually
// be reported and used, not defaultArgon2Params on its own.
func CalibrateArgon2(targetDuration time.Duration) (Argon2Params, time.Duration, error) {
	p := defaultArgon2Params
	var lastElapsed time.Duration
	for p.Iterations <= 20 {
		start := time.Now()
		if _, err := HashPassword(CalibrationProbePassword, p); err != nil {
			return Argon2Params{}, 0, fmt.Errorf("calibration probe: %w", err)
		}
		lastElapsed = time.Since(start)
		if lastElapsed >= targetDuration {
			return p, lastElapsed, nil
		}
		p.Iterations++
	}
	return p, lastElapsed, nil
}
