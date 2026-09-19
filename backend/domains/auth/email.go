package auth

import (
	"fmt"
	"regexp"
	"strings"
)

// emailPattern is a deliberately simple, strict-enough check for a V1
// login identifier -- not a full RFC 5322 parser. Requires exactly one
// '@', a non-empty local part and domain, and at least one '.' in the
// domain, with no remaining whitespace.
var emailPattern = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// NormalizeEmail is the one canonical normalization function used
// everywhere an email is compared or stored as a login identity (Users/
// Roles Phase 1 correction: "one canonical normalization function... do
// not silently use two competing login identifiers"). It trims
// surrounding whitespace, lowercases the result, and rejects empty or
// malformed values. Callers must never normalize an email ad hoc.
func NormalizeEmail(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if lower == "" {
		return "", fmt.Errorf("email is required")
	}
	if strings.ContainsAny(lower, " \t\r\n") {
		return "", fmt.Errorf("email must not contain whitespace")
	}
	if !emailPattern.MatchString(lower) {
		return "", fmt.Errorf("email is not a valid address")
	}
	return lower, nil
}
