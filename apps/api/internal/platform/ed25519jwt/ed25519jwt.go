// Package ed25519jwt issues and inspects the compact JWTs Atlas uses for
// realtime and short-lived challenge tokens (docs/ATLAS.md §6.1, ADR-008).
//
// Go is the only issuer: it signs with an Ed25519 key and publishes the public
// half as JWKS for the Elixir orchestrator to verify. Keeping this small (no
// third-party JWT dependency) makes the exact wire format explicit, which
// matters when a second language verifies it.
package ed25519jwt

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"
)

// signingAlgorithm is the JOSE algorithm name for Ed25519.
const signingAlgorithm = "EdDSA"

// Key is an Ed25519 signing key with a stable id.
type Key struct {
	kid  string
	priv ed25519.PrivateKey
	pub  ed25519.PublicKey
}

// KeyID derives a deterministic key id from the public key, so restarts and
// multiple issuers agree on it.
func KeyID(pub ed25519.PublicKey) string {
	sum := sha256.Sum256(pub)
	return "atlas-" + base64.RawURLEncoding.EncodeToString(sum[:8])
}

// Generate creates a fresh signing key.
func Generate() (*Key, error) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return &Key{kid: KeyID(pub), priv: priv, pub: pub}, nil
}

// FromPrivateKey wraps an existing Ed25519 private key.
func FromPrivateKey(priv ed25519.PrivateKey) *Key {
	pub := priv.Public().(ed25519.PublicKey)
	return &Key{kid: KeyID(pub), priv: priv, pub: pub}
}

// ParsePrivateKey accepts a PKCS#8 Ed25519 key as PEM or base64-encoded DER.
// Empty input is an error; callers decide whether to generate an ephemeral key.
func ParsePrivateKey(material string) (ed25519.PrivateKey, error) {
	material = strings.TrimSpace(material)
	if material == "" {
		return nil, errors.New("empty private key material")
	}

	der := []byte(material)
	if !strings.Contains(material, "-----BEGIN") {
		decoded, err := base64.StdEncoding.DecodeString(material)
		if err != nil {
			return nil, fmt.Errorf("private key is neither PEM nor base64 DER: %w", err)
		}
		der = decoded
	} else {
		block, _ := pem.Decode([]byte(material))
		if block == nil {
			return nil, errors.New("private key PEM is malformed")
		}
		der = block.Bytes
	}

	parsed, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, fmt.Errorf("parse PKCS#8 private key: %w", err)
	}
	priv, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not Ed25519")
	}
	return priv, nil
}

// KeyID returns the key's id.
func (k *Key) KeyID() string { return k.kid }

// PublicKey returns the public half.
func (k *Key) PublicKey() ed25519.PublicKey { return k.pub }

// Claims is the payload of an Atlas-issued token.
type Claims struct {
	Issuer    string
	Subject   string
	Audience  string
	IssuedAt  time.Time
	ExpiresAt time.Time
	NotBefore time.Time
	JWTID     string
	Scopes    []string
	// Purpose distinguishes short-lived challenge tokens (e.g. "mfa") from
	// realtime tokens. Empty means a normal token.
	Purpose string
}

type jsonClaims struct {
	Issuer    string   `json:"iss"`
	Subject   string   `json:"sub"`
	Audience  string   `json:"aud"`
	IssuedAt  int64    `json:"iat"`
	ExpiresAt int64    `json:"exp"`
	NotBefore int64    `json:"nbf,omitempty"`
	JWTID     string   `json:"jti,omitempty"`
	Scopes    []string `json:"scopes,omitempty"`
	Purpose   string   `json:"purpose,omitempty"`
}

// Sign returns a signed compact JWS for the claims.
func (k *Key) Sign(c Claims) (string, error) {
	header := map[string]string{"alg": signingAlgorithm, "typ": "JWT", "kid": k.kid}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", err
	}

	payload, err := json.Marshal(claimsJSON(c))
	if err != nil {
		return "", err
	}

	signingInput := b64(headerJSON) + "." + b64(payload)
	signature := ed25519.Sign(k.priv, []byte(signingInput))
	return signingInput + "." + b64(signature), nil
}

func claimsJSON(c Claims) jsonClaims {
	return jsonClaims{
		Issuer:    c.Issuer,
		Subject:   c.Subject,
		Audience:  c.Audience,
		IssuedAt:  c.IssuedAt.Unix(),
		ExpiresAt: c.ExpiresAt.Unix(),
		NotBefore: unixOrZero(c.NotBefore),
		JWTID:     c.JWTID,
		Scopes:    c.Scopes,
		Purpose:   c.Purpose,
	}
}

func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// Verify checks a compact JWS produced by any Ed25519 key and returns its
// claims. It exists for tests and tooling; Elixir performs the realtime check
// against the JWKS document.
func Verify(token string, pub ed25519.PublicKey) (map[string]any, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, errors.New("token is not a compact JWS")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, errors.New("signature is not base64url")
	}
	if !ed25519.Verify(pub, []byte(parts[0]+"."+parts[1]), signature) {
		return nil, errors.New("signature does not verify")
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	var claims map[string]any
	if err := json.Unmarshal(raw, &claims); err != nil {
		return nil, err
	}
	if exp, ok := claims["exp"].(float64); ok && time.Now().Unix() > int64(exp) {
		return nil, errors.New("token is expired")
	}
	return claims, nil
}

// JWK is a public key as a JSON Web Key (RFC 8037).
type JWK struct {
	Kty    string   `json:"kty"`
	Crv    string   `json:"crv"`
	X      string   `json:"x"`
	Kid    string   `json:"kid"`
	Alg    string   `json:"alg"`
	Use    string   `json:"use"`
	KeyOps []string `json:"key_ops,omitempty"`
}

// JWK returns the public JWK for this key.
func (k *Key) JWK() JWK {
	return JWK{
		Kty:    "OKP",
		Crv:    "Ed25519",
		X:      base64.RawURLEncoding.EncodeToString(k.pub),
		Kid:    k.kid,
		Alg:    signingAlgorithm,
		Use:    "sig",
		KeyOps: []string{"verify"},
	}
}

// JWKS is a JWK Set document (§6.1).
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWKS returns the key set for this single key.
func (k *Key) JWKS() JWKS { return JWKS{Keys: []JWK{k.JWK()}} }

// SigningKey returns the underlying private key (e.g. unforgeable tests).
func (k *Key) SigningKey() ed25519.PrivateKey { return k.priv }

func b64(b []byte) string { return base64.RawURLEncoding.EncodeToString(b) }
