package crypt

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ErrInvalidCiphertext is returned when a password field cannot be decoded.
var ErrInvalidCiphertext = errors.New("crypt: invalid or unreadable ciphertext")

// PasswordEncoding names the scheme clients must apply to password fields.
const PasswordEncoding = "rsa-oaep-sha256"

// keyBits is the modulus size of a generated password key.
const keyBits = 3072

// KeyBox decrypts the RSA-OAEP protected password fields sent by clients. It
// never sees a password in clear text on the wire: the client fetches the
// public key from AuthService.GetAuthConfig and encrypts locally.
type KeyBox struct {
	key       *rsa.PrivateKey
	publicPEM string
	keyID     string
}

// LoadOrCreateKeyBox reads a PKCS#8 PEM private key from inline PEM or from a
// file, and generates and persists a new key when neither is present.
func LoadOrCreateKeyBox(inlinePEM, path string) (*KeyBox, error) {
	key, err := parseKey([]byte(inlinePEM))
	if err != nil && inlinePEM != "" {
		return nil, err
	}
	if key == nil && path != "" {
		if raw, readErr := os.ReadFile(path); readErr == nil {
			key, err = parseKey(raw)
			if err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(readErr) {
			return nil, fmt.Errorf("crypt: read password key: %w", readErr)
		}
	}
	if key == nil {
		key, err = rsa.GenerateKey(rand.Reader, keyBits)
		if err != nil {
			return nil, fmt.Errorf("crypt: generate password key: %w", err)
		}
		if path != "" {
			if err := writeKey(path, key); err != nil {
				return nil, err
			}
		}
	}
	return newKeyBox(key)
}

func newKeyBox(key *rsa.PrivateKey) (*KeyBox, error) {
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("crypt: marshal public key: %w", err)
	}
	publicPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
	sum := sha256.Sum256(der)
	return &KeyBox{
		key:       key,
		publicPEM: publicPEM,
		keyID:     base64.RawURLEncoding.EncodeToString(sum[:8]),
	}, nil
}

func parseKey(raw []byte) (*rsa.PrivateKey, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, errors.New("crypt: password key is not PEM encoded")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("crypt: password key is not an RSA key")
		}
		return rsaKey, nil
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("crypt: parse password key: %w", err)
	}
	return key, nil
}

func writeKey(path string, key *rsa.PrivateKey) error {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("crypt: marshal password key: %w", err)
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("crypt: create key directory: %w", err)
		}
	}
	blob := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
	if err := os.WriteFile(path, blob, 0o600); err != nil {
		return fmt.Errorf("crypt: write password key: %w", err)
	}
	return nil
}

// KeyID identifies the current key so a client can detect a rotation.
func (b *KeyBox) KeyID() string { return b.keyID }

// PublicKeyPEM returns the PKCS#8 public key clients encrypt with.
func (b *KeyBox) PublicKeyPEM() string { return b.publicPEM }

// Decode turns a client supplied password field into clear text. The value is
// expected to be base64 of an RSA-OAEP(SHA-256) ciphertext. When allowPlain is
// set, a value that cannot be decoded is returned unchanged, which is only
// ever appropriate in a local development configuration.
func (b *KeyBox) Decode(value string, allowPlain bool) (string, error) {
	if value == "" {
		return "", nil
	}
	ciphertext, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		if allowPlain {
			return value, nil
		}
		return "", ErrInvalidCiphertext
	}
	plain, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, b.key, ciphertext, nil)
	if err != nil {
		if allowPlain {
			return value, nil
		}
		return "", ErrInvalidCiphertext
	}
	return string(plain), nil
}
