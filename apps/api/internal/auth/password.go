// Package auth implements Atlas identity: password sign-in (email or phone),
// TOTP multi-factor for privileged roles, opaque sessions, account invites and
// the short-lived tokens the realtime layer consumes (docs/ATLAS.md §6.1).
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Errors returned by this package. Handlers map them to problem responses;
// callers should match with errors.Is.
var (
	// ErrInvalidCredentials is returned when the identifier is unknown or the
	// password is wrong. The two cases are deliberately indistinguishable.
	ErrInvalidCredentials = errors.New("auth: invalid credentials")
	// ErrAccountInactive covers suspended, deactivated and invited accounts.
	ErrAccountInactive = errors.New("auth: account is not active")
	// ErrInvalidHash is returned for a password hash that is not argon2id PHC.
	ErrInvalidHash = errors.New("auth: not a valid argon2id password hash")
	// ErrTokenInvalid is returned for a malformed or unknown token.
	ErrTokenInvalid = errors.New("auth: token invalid")
)

// Argon2Params tune the password hash. The defaults track OWASP's
// recommendation for an interactive login (§6.1): require ~19 MiB.
type Argon2Params struct {
	Memory      uint32 // KiB
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

// DefaultArgon2Params returns the production parameters.
func DefaultArgon2Params() Argon2Params {
	return Argon2Params{Memory: 19 * 1024, Iterations: 2, Parallelism: 1, SaltLength: 16, KeyLength: 32}
}

// HashPassword returns an argon2id PHC string for the password.
func HashPassword(password string, p Argon2Params) (string, error) {
	if password == "" {
		return "", errors.New("auth: password is empty")
	}
	salt := make([]byte, p.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: read salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)

	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.Memory, p.Iterations, p.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword checks a password against a PHC-encoded argon2id hash. It
// returns ErrInvalidHash for a malformed stored value, and false for a
// mismatch; the two are different so operators can alert on bad data while
// callers treat both as a failed login.
func VerifyPassword(password, encoded string) (bool, error) {
	p, salt, expected, err := decodeHash(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), salt, p.Iterations, p.Memory, p.Parallelism, p.KeyLength)
	return subtle.ConstantTimeCompare(got, expected) == 1, nil
}

func decodeHash(encoded string) (Argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	if version != argon2.Version {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}

	var p Argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Iterations, &p.Parallelism); err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return Argon2Params{}, nil, nil, ErrInvalidHash
	}
	p.SaltLength = uint32(len(salt))
	p.KeyLength = uint32(len(key))
	return p, salt, key, nil
}
