package urlsign

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSignAndVerifyRoundTrip(t *testing.T) {
	signer := New("https://files.example.com", "test-secret")
	expires := time.Now().Add(10 * time.Minute)

	signed, got, err := signer.Sign("content", "GET", "/v1/files/abc/content", "user-1", "abc", "attachment", expires)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	if !got.Equal(expires) {
		t.Errorf("returned expiry %v, want %v", got, expires)
	}
	parsed, err := url.Parse(signed)
	if err != nil {
		t.Fatalf("the signed value is not a URL: %v", err)
	}
	if !strings.HasPrefix(signed, "https://files.example.com/v1/files/abc/content?") {
		t.Fatalf("signed URL = %q", signed)
	}
	query := parsed.Query()
	if query.Get("sig") == "" || query.Get("exp") == "" {
		t.Fatalf("signed URL carries no signature: %q", signed)
	}
	if query.Get("extra") != "attachment" {
		t.Errorf("extra = %q", query.Get("extra"))
	}

	now := time.Now()
	if err := signer.Verify("content", "GET", "/v1/files/abc/content", "abc", "attachment",
		query.Get("exp"), query.Get("sig"), query.Get("sub"), now); err != nil {
		t.Fatalf("verify a fresh signature: %v", err)
	}
}

func TestVerifyRejectsTampering(t *testing.T) {
	signer := New("", "test-secret")
	expires := time.Now().Add(time.Hour)
	signed, _, err := signer.Sign("content", "GET", "/v1/files/abc/content", "user-1", "abc", "", expires)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	query, _ := url.ParseQuery(strings.SplitN(signed, "?", 2)[1])
	exp, sig := query.Get("exp"), query.Get("sig")
	now := time.Now()

	ok := func(name string, e error) {
		t.Helper()
		if e != nil {
			t.Errorf("%s: expected the signature to verify, got %v", name, e)
		}
	}
	bad := func(name string, e error) {
		t.Helper()
		if e == nil {
			t.Errorf("%s: a forged signature verified", name)
		}
	}

	ok("baseline", signer.Verify("content", "GET", "/v1/files/abc/content", "abc", "", exp, sig, "user-1", now))
	bad("wrong node", signer.Verify("content", "GET", "/v1/files/abc/content", "other", "", exp, sig, "user-1", now))
	bad("wrong path", signer.Verify("content", "GET", "/v1/files/other/content", "abc", "", exp, sig, "user-1", now))
	bad("wrong method", signer.Verify("content", "PUT", "/v1/files/abc/content", "abc", "", exp, sig, "user-1", now))
	bad("wrong scope", signer.Verify("archive", "GET", "/v1/files/abc/content", "abc", "", exp, sig, "user-1", now))
	bad("wrong subject", signer.Verify("content", "GET", "/v1/files/abc/content", "abc", "", exp, sig, "someone-else", now))
	bad("wrong extra", signer.Verify("content", "GET", "/v1/files/abc/content", "abc", "inline", exp, sig, "user-1", now))
	bad("forged signature", signer.Verify("content", "GET", "/v1/files/abc/content", "abc", "", exp, "not-a-signature", "user-1", now))
	bad("expired", signer.Verify("content", "GET", "/v1/files/abc/content", "abc", "",
		exp, sig, "user-1", expires.Add(time.Second)))
	bad("another key", New("", "different-secret").Verify("content", "GET", "/v1/files/abc/content", "abc", "", exp, sig, "user-1", now))
}

func TestSignIsIndependentOfQueryOrder(t *testing.T) {
	signer := New("", "test-secret")
	expires := time.Now().Add(time.Hour)
	signed, _, err := signer.Sign("archive", "GET", "/v1/files/abc/archive", "user-1", "abc", "bundle", expires)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	query, _ := url.ParseQuery(strings.SplitN(signed, "?", 2)[1])

	// Reordering the query parameters must not change the verdict, which is
	// what keeps a proxy that rewrites the query string from breaking a link.
	reordered := map[string][]string{}
	for key, value := range query {
		reordered[key] = value
	}
	if err := signer.Verify("archive", "GET", "/v1/files/abc/archive", "abc", "bundle",
		reordered["exp"][0], reordered["sig"][0], reordered["sub"][0], time.Now()); err != nil {
		t.Fatalf("verify after reordering: %v", err)
	}
	if QueryValue(reordered, "extra") != "bundle" {
		t.Errorf("QueryValue did not read extra")
	}
	if QueryValue(reordered, "missing") != "" {
		t.Errorf("QueryValue returned a value for a missing key")
	}
}
