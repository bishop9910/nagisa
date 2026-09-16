package crypt

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// ErrInvalidToken is returned when a JWT is malformed, expired or signed with
// the wrong key.
var ErrInvalidToken = errors.New("crypt: invalid token")

// Token types carried in the `typ` claim.
const (
	TokenTypeAccess = "access"
	TokenTypeNode   = "node"
	TokenTypeShare  = "share"
)

// Issuer mints and verifies the short lived JWTs the API uses.
type Issuer struct {
	secret []byte
	issuer string
}

// NewIssuer returns an issuer bound to the supplied HMAC secret.
func NewIssuer(secret, issuer string) *Issuer {
	return &Issuer{secret: []byte(secret), issuer: issuer}
}

// Claims is the payload carried by a netdisk JWT.
type Claims struct {
	Type string `json:"typ"`
	// Username is present on access tokens.
	Username string `json:"uname,omitempty"`
	// Role is the numeric account role.
	Role int32 `json:"role,omitempty"`
	// Rank is the account authority level.
	Rank int32 `json:"rank,omitempty"`
	// Perms is the account permission bitmask.
	Perms int64 `json:"perms,omitempty"`
	// Nodes lists the node ids a node token unlocks, including their subtrees.
	Nodes []string `json:"nodes,omitempty"`
	// ShareID is present on share access tokens.
	ShareID string `json:"share,omitempty"`
	jwt.RegisteredClaims
}

// Issue signs a claim set with the supplied lifetime.
func (i *Issuer) Issue(c Claims, subject string, ttl time.Duration) (string, error) {
	now := time.Now()
	c.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    i.issuer,
		Subject:   subject,
		ID:        uuid.Must(uuid.NewV7()).String(),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now.Add(-time.Second)),
		ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(i.secret)
}

// Verify parses and validates a token, returning its claims.
func (i *Issuer) Verify(token string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return i.secret, nil
	}, jwt.WithIssuer(i.issuer), jwt.WithExpirationRequired())
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	return claims, nil
}

// HashToken returns the hex SHA-256 of an opaque token, which is what gets
// stored for refresh tokens and share passwords never leave the server in
// recoverable form.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
