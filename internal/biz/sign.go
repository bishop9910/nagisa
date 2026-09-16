package biz

import "time"

// SignClaims describes what a signed URL authorises.
type SignClaims struct {
	// Scope names the operation, for example "download" or "archive".
	Scope string
	// Method is the HTTP method the URL must be redeemed with.
	Method string
	// Path is the request path the URL is bound to.
	Path string
	// Subject is the acting account, or the share id, or empty.
	Subject string
	// Node is the resource the URL points at.
	Node string
	// Extra carries additional constraints such as the disposition.
	Extra string
	// ExpiresAt is the instant the URL stops working.
	ExpiresAt time.Time
}

// URLSigner mints and verifies the signatures carried by the streaming
// endpoints the server serves itself. The parameter lists are deliberately
// primitive so the signer implementation does not have to depend on this
// package.
type URLSigner interface {
	Sign(scope, method, path, subject, node, extra string, expiresAt time.Time) (string, time.Time, error)
	Verify(scope, method, path, node, extra, exp, sig, sub string, now time.Time) error
	Base() string
}
