package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Sealer encrypts short secrets at rest — the TOTP secret in
// core.user_accounts.mfa_totp_secret_enc (§6.1, §13.1). The key comes from the
// environment (a secret manager outside local development).
type Sealer struct {
	aead cipher.AEAD
}

// NewSealer builds a Sealer from a base64-encoded 32-byte key.
func NewSealer(encodedKey string) (*Sealer, error) {
	material := strings.TrimSpace(encodedKey)
	if material == "" {
		return nil, errors.New("auth: encryption key is required")
	}
	key, err := base64.StdEncoding.DecodeString(material)
	if err != nil {
		return nil, fmt.Errorf("auth: encryption key is not valid base64: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("auth: encryption key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("auth: new cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("auth: new gcm: %w", err)
	}
	return &Sealer{aead: aead}, nil
}

// GenerateEncryptionKey returns a fresh base64-encoded 32-byte key. It exists
// so local development and the CLI can bootstrap without hand-editing bytes.
func GenerateEncryptionKey() (string, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return "", fmt.Errorf("auth: generate encryption key: %w", err)
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// Seal encrypts plaintext as nonce||ciphertext.
func (s *Sealer) Seal(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, s.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("auth: seal nonce: %w", err)
	}
	return s.aead.Seal(nonce, nonce, plaintext, nil), nil
}

// Open decrypts a value produced by Seal.
func (s *Sealer) Open(ciphertext []byte) ([]byte, error) {
	ns := s.aead.NonceSize()
	if len(ciphertext) < ns {
		return nil, errors.New("auth: ciphertext is too short")
	}
	nonce, body := ciphertext[:ns], ciphertext[ns:]
	plaintext, err := s.aead.Open(nil, nonce, body, nil)
	if err != nil {
		return nil, fmt.Errorf("auth: open: %w", err)
	}
	return plaintext, nil
}
