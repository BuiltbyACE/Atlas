package auth

import (
	"errors"
	"testing"
)

// testParams keep the tests fast; production parameters are heavier.
func testArgon2Params() Argon2Params {
	return Argon2Params{Memory: 8 * 1024, Iterations: 1, Parallelism: 1, SaltLength: 16, KeyLength: 32}
}

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple", DefaultArgon2Params())
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("VerifyPassword: %v", err)
	}
	if !ok {
		t.Error("expected the correct password to verify")
	}

	ok, err = VerifyPassword("wrong password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword(wrong): %v", err)
	}
	if ok {
		t.Error("expected a wrong password to fail")
	}
}

func TestHashPasswordIsSalted(t *testing.T) {
	a, _ := HashPassword("hunter2", DefaultArgon2Params())
	b, _ := HashPassword("hunter2", DefaultArgon2Params())
	if a == b {
		t.Error("two hashes of the same password must differ (random salt)")
	}
}

func TestHashPasswordRejectsEmpty(t *testing.T) {
	if _, err := HashPassword("", DefaultArgon2Params()); err == nil {
		t.Fatal("expected an error for an empty password")
	}
}

func TestVerifyPasswordRejectsMalformed(t *testing.T) {
	for _, encoded := range []string{"", "not-a-hash", "$argon2i$v=19$m=1,t=1,p=1$c2FsdA$aGFzaA"} {
		if _, err := VerifyPassword("x", encoded); !errors.Is(err, ErrInvalidHash) {
			t.Errorf("VerifyPassword(%q) error = %v, want ErrInvalidHash", encoded, err)
		}
	}
}
