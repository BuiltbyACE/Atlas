package auth

import (
	"encoding/base64"
	"testing"
)

func TestSealRoundTrip(t *testing.T) {
	key, err := GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	sealer, err := NewSealer(key)
	if err != nil {
		t.Fatalf("NewSealer: %v", err)
	}

	secret := []byte("JBSWY3DPEHPK3PXP")
	ciphertext, err := sealer.Seal(secret)
	if err != nil {
		t.Fatalf("Seal: %v", err)
	}
	if string(ciphertext) == string(secret) {
		t.Fatal("ciphertext equals plaintext")
	}

	plaintext, err := sealer.Open(ciphertext)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if string(plaintext) != string(secret) {
		t.Errorf("round trip = %q, want %q", plaintext, secret)
	}
}

func TestNewSealerRejectsBadKeys(t *testing.T) {
	cases := map[string]string{
		"empty":     "",
		"not b64":   "!!!not-base64!!!",
		"too short": base64.StdEncoding.EncodeToString([]byte("0123456789")),
	}
	for name, key := range cases {
		if _, err := NewSealer(key); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestOpenRejectsTamperedCiphertext(t *testing.T) {
	key, _ := GenerateEncryptionKey()
	sealer, _ := NewSealer(key)

	ciphertext, _ := sealer.Seal([]byte("secret"))
	ciphertext[len(ciphertext)-1] ^= 0xff

	if _, err := sealer.Open(ciphertext); err == nil {
		t.Fatal("expected tampered ciphertext to fail authentication")
	}
}

func TestOpenRejectsShortCiphertext(t *testing.T) {
	key, _ := GenerateEncryptionKey()
	sealer, _ := NewSealer(key)
	if _, err := sealer.Open([]byte{1, 2, 3}); err == nil {
		t.Fatal("expected a short ciphertext to be rejected")
	}
}
