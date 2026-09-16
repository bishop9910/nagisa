package crypt

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ErrBadSignature is returned when a signed URL fails verification.
var ErrBadSignature = errors.New("crypt: bad signature")

// Signer produces and verifies the compact HMAC tokens that authorise a single
// URL. A token binds the HTTP method, the path, the subject, the node and the
// expiry, so a URL captured from a log cannot be replayed against another
// resource or after it has expired.
type Signer struct {
	secret []byte
}

// NewSigner returns a signer bound to the supplied secret.
func NewSigner(secret string) *Signer {
	return &Signer{secret: []byte(secret)}
}

// URLClaims is the payload a signed URL carries.
type URLClaims struct {
	// Scope names the operation, for example "download" or "archive".
	Scope string
	// Method is the HTTP method the URL must be redeemed with.
	Method string
	// Path is the request path the URL is bound to.
	Path string
	// Subject is the acting account id or share id. Empty for anonymous.
	Subject string
	// Node is the resource the URL points at.
	Node string
	// Extra carries additional constraints such as the disposition.
	Extra string
	// ExpiresAt is the instant the URL stops working.
	ExpiresAt time.Time
}

// canonical renders the URLClaims into the string that is signed.
func (c URLClaims) canonical() string {
	return strings.Join([]string{
		c.Scope,
		strings.ToUpper(c.Method),
		c.Path,
		c.Subject,
		c.Node,
		c.Extra,
		strconv.FormatInt(c.ExpiresAt.Unix(), 10),
	}, "\n")
}

// Sign returns the `exp` and `sig` query parameters for the URLClaims.
func (s *Signer) Sign(c URLClaims) url.Values {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(c.canonical()))
	q := url.Values{}
	q.Set("exp", strconv.FormatInt(c.ExpiresAt.Unix(), 10))
	q.Set("sig", base64.RawURLEncoding.EncodeToString(mac.Sum(nil)))
	if c.Extra != "" {
		q.Set("extra", c.Extra)
	}
	if c.Subject != "" {
		q.Set("sub", c.Subject)
	}
	return q
}

// SignURL appends the signature to a URL.
func (s *Signer) SignURL(base string, c URLClaims) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("crypt: parse url: %w", err)
	}
	q := u.Query()
	for k, v := range s.Sign(c) {
		q.Set(k, v[0])
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// Verify checks a signed URL. now is injected so the check is testable.
func (s *Signer) Verify(c URLClaims, exp, sig, sub string, now time.Time) error {
	expires, err := strconv.ParseInt(exp, 10, 64)
	if err != nil {
		return ErrBadSignature
	}
	if now.Unix() > expires {
		return ErrBadSignature
	}
	c.ExpiresAt = time.Unix(expires, 0)
	c.Subject = sub
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(c.canonical()))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(sig)) {
		return ErrBadSignature
	}
	return nil
}

// MustToken is a small helper for tests and for building share tokens.
func MustToken(n int) string {
	t, err := RandomToken(n)
	if err != nil {
		panic(err)
	}
	return t
}
