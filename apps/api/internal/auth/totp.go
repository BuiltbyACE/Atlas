package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// TOTP parameters fixed by RFC 6238 and the authenticator apps Atlas targets.
const (
	totpDigits = 6
	totpPeriod = 30 * time.Second
	// totpSkewSteps accepts the previous and next window, tolerating clock drift.
	totpSkewSteps = 1
	// totpSecretBytes is the standard authenticator secret size (160 bits).
	totpSecretBytes = 20
)

var base32NoPad = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateTOTPSecret returns a new base32 secret (without padding).
func GenerateTOTPSecret() (string, error) {
	buf := make([]byte, totpSecretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("auth: read totp secret: %w", err)
	}
	return base32NoPad.EncodeToString(buf), nil
}

// TOTPURI builds the otpauth:// provisioning URI for an authenticator app.
func TOTPURI(issuer, account, secret string) string {
	label := url.PathEscape(issuer + ":" + account)
	q := url.Values{}
	q.Set("secret", secret)
	q.Set("issuer", issuer)
	q.Set("algorithm", "SHA1")
	q.Set("digits", fmt.Sprintf("%d", totpDigits))
	q.Set("period", fmt.Sprintf("%d", int(totpPeriod.Seconds())))
	return "otpauth://totp/" + label + "?" + q.Encode()
}

// VerifyTOTP reports whether code is valid for the secret at time t, allowing
// one period of clock drift either way.
func VerifyTOTP(secret, code string, t time.Time) bool {
	code = strings.TrimSpace(code)
	if len(code) != totpDigits {
		return false
	}
	key, err := decodeTOTPSecret(secret)
	if err != nil {
		return false
	}
	for step := -totpSkewSteps; step <= totpSkewSteps; step++ {
		candidate := codeAt(key, t.Add(time.Duration(step)*totpPeriod), totpDigits)
		if hmac.Equal([]byte(candidate), []byte(code)) {
			return true
		}
	}
	return false
}

func decodeTOTPSecret(secret string) ([]byte, error) {
	normalized := strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(secret), " ", ""))
	b, err := base32NoPad.DecodeString(normalized)
	if err != nil {
		return nil, ErrTokenInvalid
	}
	return b, nil
}

// codeAt computes the HOTP truncation (RFC 4226) for the time window.
func codeAt(key []byte, t time.Time, digits int) string {
	counter := uint64(t.Unix()) / uint64(totpPeriod.Seconds())
	return hotp(key, counter, digits)
}

func hotp(key []byte, counter uint64, digits int) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf[:])
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0x0f
	truncated := (uint32(sum[offset]&0x7f) << 24) |
		(uint32(sum[offset+1]) << 16) |
		(uint32(sum[offset+2]) << 8) |
		uint32(sum[offset+3])

	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	return fmt.Sprintf("%0*d", digits, truncated%mod)
}
