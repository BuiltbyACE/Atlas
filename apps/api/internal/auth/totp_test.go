package auth

import (
	"encoding/base32"
	"strings"
	"testing"
	"time"
)

// The RFC test secret is the ASCII bytes "12345678901234567890".
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func rfcKey(t *testing.T) []byte {
	t.Helper()
	b, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(rfcSecret)
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}
	if string(b) != "12345678901234567890" {
		t.Fatalf("unexpected secret bytes: %q", b)
	}
	return b
}

// RFC 6238 Appendix B (SHA-1, 8 digits).
func TestTOTPRFC6238(t *testing.T) {
	vectors := []struct {
		unix int64
		code string
	}{
		{59, "94287082"},
		{1111111109, "07081804"},
		{1111111111, "14050471"},
		{1234567890, "89005924"},
		{2000000000, "69279037"},
		{20000000000, "65353130"},
	}
	secret := rfcKey(t)
	for _, v := range vectors {
		if got := hotp(secret, uint64(v.unix)/30, 8); got != v.code {
			t.Errorf("time %d: got %s, want %s", v.unix, got, v.code)
		}
	}
}

// RFC 4226 Appendix D (6-digit HOTP).
func TestHOTPRFC4226(t *testing.T) {
	codes := []string{"755224", "287082", "359152", "969429", "338314", "254676", "287922", "162583", "399871", "520489"}
	secret := rfcKey(t)
	for counter, want := range codes {
		if got := hotp(secret, uint64(counter), 6); got != want {
			t.Errorf("counter %d: got %s, want %s", counter, got, want)
		}
	}
}

func TestGenerateTOTPSecretIsUsable(t *testing.T) {
	secret, err := GenerateTOTPSecret()
	if err != nil {
		t.Fatalf("GenerateTOTPSecret: %v", err)
	}
	if _, err := decodeTOTPSecret(secret); err != nil {
		t.Fatalf("generated secret does not decode: %v", err)
	}
}

func TestVerifyTOTPWindow(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	key := rfcKey(t)
	at := time.Unix(1111111111, 0)
	code := codeAt(key, at, totpDigits)

	if !VerifyTOTP(secret, code, at) {
		t.Error("expected the current code to verify")
	}
	if !VerifyTOTP(secret, code, at.Add(-1*totpPeriod)) {
		t.Error("expected one step of drift back to verify")
	}
	if VerifyTOTP(secret, code, at.Add(5*time.Minute)) {
		t.Error("expected a much later time to reject the code")
	}
	if VerifyTOTP(secret, "00000", at) {
		t.Error("expected a malformed code to be rejected")
	}
}

func TestTOTPURIContainsDetails(t *testing.T) {
	uri := TOTPURI("SafariStack Atlas", "asha@safaristack.co.ke", "ABCDEF")
	for _, want := range []string{"otpauth://totp/", "secret=ABCDEF", "issuer=", "period=30", "digits=6"} {
		if !strings.Contains(uri, want) {
			t.Errorf("URI %q is missing %q", uri, want)
		}
	}
}
