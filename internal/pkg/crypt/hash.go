// Package crypt holds the primitives the netdisk server uses to protect
// credentials and to hand out short lived capabilities: password hashing,
// RSA password decoding, signed URL tokens and JWT access tokens.
package crypt

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// ErrPasswordMismatch is returned when a password does not match its hash.
var ErrPasswordMismatch = errors.New("crypt: password mismatch")

// DefaultBcryptCost is the work factor used for new hashes.
const DefaultBcryptCost = 12

// Hasher hashes and verifies account and folder passwords.
type Hasher struct {
	cost int
}

// NewHasher returns a hasher using the supplied bcrypt cost, clamped to the
// range bcrypt accepts.
func NewHasher(cost int) *Hasher {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		cost = DefaultBcryptCost
	}
	return &Hasher{cost: cost}
}

// Hash returns a bcrypt hash of the password.
func (h *Hasher) Hash(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), h.cost)
	if err != nil {
		return "", fmt.Errorf("crypt: hash password: %w", err)
	}
	return string(b), nil
}

// Verify reports whether the password matches the hash.
func (h *Hasher) Verify(hash, password string) error {
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		return ErrPasswordMismatch
	}
	return nil
}

// RandomToken returns a URL safe random string carrying n bytes of entropy.
func RandomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("crypt: random: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
