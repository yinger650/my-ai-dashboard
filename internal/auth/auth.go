// Package auth implements password hashing, API token and session token
// generation/hashing, and constant-time comparisons.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Token scopes.
const (
	ScopeMachine = "machine_ingest"
	ScopeService = "service_ingest"
	ScopeViewer  = "viewer"
)

// Argon2id parameters (spec 17.6).
const (
	argonMemory      = 64 * 1024 // KiB => 64 MiB
	argonIterations  = 3
	argonParallelism = 2
	argonSaltLen     = 16
	argonKeyLen      = 32
)

func randBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, err
	}
	return b, nil
}

// HashPassword returns a PHC-formatted argon2id hash of pw.
func HashPassword(pw string) (string, error) {
	salt, err := randBytes(argonSaltLen)
	if err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(pw), salt, argonIterations, argonMemory, argonParallelism, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword reports whether pw matches the encoded argon2id hash.
func VerifyPassword(pw, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, errors.New("invalid hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return false, err
	}
	var m, t, p int
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, err
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(pw), salt, uint32(t), uint32(m), uint8(p), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// HashToken returns the hex SHA-256 of a token/session string.
func HashToken(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// ConstantTimeEqual compares two strings without leaking timing.
func ConstantTimeEqual(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// GenerateAPIToken creates a new API token for a scope and returns
// (fullToken, prefix, hash). The full token is shown to the admin once.
func GenerateAPIToken(scope string) (full, prefix, hash string, err error) {
	var tag string
	switch scope {
	case ScopeMachine:
		tag = "abp_m_"
	case ScopeService:
		tag = "abp_s_"
	case ScopeViewer:
		tag = "abp_v_"
	default:
		return "", "", "", fmt.Errorf("unknown scope %q", scope)
	}
	b, err := randBytes(32)
	if err != nil {
		return "", "", "", err
	}
	full = tag + base64.RawURLEncoding.EncodeToString(b)
	prefix = full
	if len(prefix) > 12 {
		prefix = prefix[:12]
	}
	return full, prefix, HashToken(full), nil
}

// ScopeFromToken infers the scope prefix from a token string.
func ScopeFromToken(full string) (string, bool) {
	switch {
	case strings.HasPrefix(full, "abp_m_"):
		return ScopeMachine, true
	case strings.HasPrefix(full, "abp_s_"):
		return ScopeService, true
	case strings.HasPrefix(full, "abp_v_"):
		return ScopeViewer, true
	}
	return "", false
}

// GenerateSessionToken returns a random session/CSRF token and its hash.
func GenerateSessionToken() (token, hash string, err error) {
	b, err := randBytes(32)
	if err != nil {
		return "", "", err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, HashToken(token), nil
}
