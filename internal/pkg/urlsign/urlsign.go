// Package urlsign mints and verifies the compact signatures that authorise a
// single request against the server's own streaming endpoints, which is how a
// browser downloads an original file or a zip archive without holding a token.
package urlsign

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"nagisa/internal/pkg/crypt"
)

// Signer binds the URL prefix of the deployment to the signing key.
type Signer struct {
	base   string
	signer *crypt.Signer
}

// New returns a signer for the supplied public base URL. An empty base means
// the URLs stay relative to the host that served the request.
func New(base, secret string) *Signer {
	return &Signer{base: strings.TrimRight(base, "/"), signer: crypt.NewSigner(secret)}
}

// Base returns the public base URL the signer was built with.
func (s *Signer) Base() string { return s.base }

// Sign returns the signed URL for a request and the instant it expires.
func (s *Signer) Sign(scope, method, path, subject, node, extra string, expiresAt time.Time) (string, time.Time, error) {
	claims := crypt.URLClaims{
		Scope:     scope,
		Method:    method,
		Path:      path,
		Subject:   subject,
		Node:      node,
		Extra:     extra,
		ExpiresAt: expiresAt,
	}
	signed, err := s.signer.SignURL(s.base+path, claims)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("urlsign: sign: %w", err)
	}
	return signed, expiresAt, nil
}

// Verify checks the signature parameters of an incoming request. The extra
// constraint is read back from the query when the caller did not pin it.
func (s *Signer) Verify(scope, method, path, node, extra, exp, sig, sub string, now time.Time) error {
	if extra == "" {
		extra = ""
	}
	claims := crypt.URLClaims{
		Scope:  scope,
		Method: method,
		Path:   path,
		Node:   node,
		Extra:  extra,
	}
	if err := s.signer.Verify(claims, exp, sig, sub, now); err != nil {
		return fmt.Errorf("urlsign: %w", err)
	}
	return nil
}

// QueryValue reads one parameter from a query map, which keeps the caller free
// of the url package.
func QueryValue(query map[string][]string, key string) string {
	return url.Values(query).Get(key)
}
