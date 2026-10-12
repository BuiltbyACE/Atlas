package ed25519jwt

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func TestSignAndVerifyRoundTrip(t *testing.T) {
	key, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	now := time.Now()
	token, err := key.Sign(Claims{
		Issuer:    "atlas",
		Subject:   "user:123",
		Audience:  "atlas-realtime",
		IssuedAt:  now,
		ExpiresAt: now.Add(5 * time.Minute),
		Scopes:    []string{"user:123"},
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if strings.Count(token, ".") != 2 {
		t.Fatalf("token is not a compact JWS: %q", token)
	}

	claims, err := Verify(token, key.pub)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if claims["sub"] != "user:123" {
		t.Errorf("sub = %v", claims["sub"])
	}
	if claims["aud"] != "atlas-realtime" {
		t.Errorf("aud = %v", claims["aud"])
	}
}

func TestVerifyRejectsTamperedPayload(t *testing.T) {
	key, _ := Generate()
	now := time.Now()
	token, _ := key.Sign(Claims{
		Issuer:    "atlas",
		Subject:   "user:1",
		Audience:  "atlas-realtime",
		IssuedAt:  now,
		ExpiresAt: now.Add(time.Hour),
	})

	parts := strings.Split(token, ".")
	evil := base64.RawURLEncoding.EncodeToString([]byte(
		`{"iss":"atlas","sub":"user:999","aud":"atlas-realtime","iat":1,"exp":9999999999}`,
	))
	forged := parts[0] + "." + evil + "." + parts[2]

	if _, err := Verify(forged, key.pub); err == nil {
		t.Fatal("expected verification failure for a tampered payload")
	}
}

func TestVerifyRejectsExpiredToken(t *testing.T) {
	key, _ := Generate()
	token, err := key.Sign(Claims{
		Issuer:    "atlas",
		Subject:   "user:1",
		Audience:  "atlas-realtime",
		IssuedAt:  time.Now().Add(-10 * time.Minute),
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if _, err := Verify(token, key.pub); err == nil {
		t.Fatal("expected an expired-token error")
	}
}

func TestVerifyRejectsWrongKey(t *testing.T) {
	key, _ := Generate()
	other, _ := Generate()
	token, err := key.Sign(Claims{Issuer: "atlas", Subject: "user:1", Audience: "atlas-realtime", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if _, err := Verify(token, other.pub); err == nil {
		t.Fatal("expected signature verification to fail with the wrong key")
	}
}

func TestParsePrivateKeyFromPKCS8PEM(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	parsed, err := ParsePrivateKey(string(pemBytes))
	if err != nil {
		t.Fatalf("ParsePrivateKey(PEM): %v", err)
	}
	if !parsed.Equal(priv) {
		t.Error("parsed key does not match the original")
	}
}

func TestParsePrivateKeyFromBase64DER(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	der, _ := x509.MarshalPKCS8PrivateKey(priv)
	encoded := base64.StdEncoding.EncodeToString(der)

	parsed, err := ParsePrivateKey(encoded)
	if err != nil {
		t.Fatalf("ParsePrivateKey(base64): %v", err)
	}
	if !parsed.Equal(priv) {
		t.Error("parsed key does not match the original")
	}
}

func TestParsePrivateKeyRejectsEmpty(t *testing.T) {
	if _, err := ParsePrivateKey("   "); err == nil {
		t.Fatal("expected an error for empty material")
	}
}

func TestJWKSShape(t *testing.T) {
	key, err := Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	set := key.JWKS()
	if len(set.Keys) != 1 {
		t.Fatalf("keys = %d, want 1", len(set.Keys))
	}
	jwk := set.Keys[0]
	if jwk.Kty != "OKP" || jwk.Crv != "Ed25519" || jwk.Alg != "EdDSA" || jwk.Use != "sig" {
		t.Errorf("unexpected JWK: %+v", jwk)
	}
	raw, err := base64.RawURLEncoding.DecodeString(jwk.X)
	if err != nil {
		t.Fatalf("x is not base64url: %v", err)
	}
	if !ed25519.PublicKey(raw).Equal(key.pub) {
		t.Error("JWK x does not match the public key")
	}
	if jwk.Kid == "" {
		t.Error("kid must be set")
	}
}
