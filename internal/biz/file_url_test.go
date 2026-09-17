package biz

import (
	"errors"
	"net/url"
	"testing"
	"time"

	"nagisa/internal/pkg/urlsign"

	"github.com/google/uuid"
)

// fakeStore implements only what the signed URL helpers touch. The embedded
// interface satisfies the rest of ObjectStore and panics if a test ever walks
// into a method it did not stub, which keeps this double to a few lines.
type fakeStore struct {
	ObjectStore
	publicHost string
}

func (f fakeStore) PublicHost() string { return f.publicHost }

func TestBrowserCanReachStorage(t *testing.T) {
	cases := []struct {
		name       string
		publicHost string
		want       bool
	}{
		{"declared public endpoint", "files.example.com", true},
		{"no public endpoint", "", false},
	}
	for _, tc := range cases {
		if got := browserCanReachStorage(fakeStore{publicHost: tc.publicHost}); got != tc.want {
			t.Errorf("%s: browserCanReachStorage = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestStreamedURLIsRelativeAndVerifiable pins the contract that makes a preview
// work from any device: without a configured base URL the signed link carries
// no host, so the browser resolves it against whatever origin it used. A
// hardcoded host here is exactly the bug that sent other devices to 127.0.0.1.
func TestStreamedURLIsRelativeAndVerifiable(t *testing.T) {
	node := &Node{ID: uuid.New(), Kind: NodeKindFile, Name: "报告.pdf", MimeType: "application/pdf", Size: 2048}
	subject := uuid.New().String()
	signer := urlsign.New("", "test-secret")

	signed, err := streamedContentURL(signer, node, subject, 30*time.Minute, true, node.Name)
	if err != nil {
		t.Fatalf("streamedContentURL: %v", err)
	}
	if signed.URL == "" {
		t.Fatal("streamedURL returned an empty URL")
	}
	parsed, err := url.Parse(signed.URL)
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", signed.URL, err)
	}
	if parsed.IsAbs() || parsed.Host != "" {
		t.Errorf("URL %q must stay relative so the browser picks the origin", signed.URL)
	}
	if want := "/v1/files/" + node.ID.String() + "/content"; parsed.Path != want {
		t.Errorf("path = %q, want %q", parsed.Path, want)
	}
	if signed.Method != "GET" || signed.NodeID != node.ID || signed.FileName != node.Name {
		t.Errorf("signed URL metadata = %+v", signed)
	}

	query := parsed.Query()
	if query.Get("extra") != "inline" {
		t.Errorf("extra = %q, want inline", query.Get("extra"))
	}
	verify := func(extra, nodeID string) error {
		return signer.Verify("content", "GET", parsed.Path, nodeID, extra,
			query.Get("exp"), query.Get("sig"), query.Get("sub"), time.Now())
	}
	if err := verify(query.Get("extra"), node.ID.String()); err != nil {
		t.Fatalf("the signature does not verify against the content route: %v", err)
	}
	// The signed URL is a capability, so nothing about it may be swappable.
	if err := verify("attachment", node.ID.String()); err == nil {
		t.Error("flipping the disposition must invalidate the signature")
	}
	if err := verify(query.Get("extra"), uuid.NewString()); err == nil {
		t.Error("the signature must be bound to one node")
	}
}

func TestStreamedURLKeepsConfiguredBase(t *testing.T) {
	node := &Node{ID: uuid.New(), Kind: NodeKindFile, Name: "a.txt", MimeType: "text/plain"}
	// A trailing slash must not produce a doubled separator.
	signer := urlsign.New("https://netdisk.example.com/", "s")

	signed, err := streamedContentURL(signer, node, uuid.New().String(), time.Minute, false, node.Name)
	if err != nil {
		t.Fatalf("streamedContentURL: %v", err)
	}
	want := "https://netdisk.example.com/v1/files/" + node.ID.String() + "/content"
	if len(signed.URL) <= len(want) || signed.URL[:len(want)] != want {
		t.Errorf("URL = %q, want the configured base prefix %q", signed.URL, want)
	}
	if parsed, err := url.Parse(signed.URL); err == nil && parsed.Query().Get("extra") != "attachment" {
		t.Errorf("extra = %q, want attachment for a plain download", parsed.Query().Get("extra"))
	}
}

func TestStreamedURLWithoutSigner(t *testing.T) {
	node := &Node{ID: uuid.New(), Kind: NodeKindFile, Name: "a.txt", MimeType: "text/plain"}
	if _, err := streamedContentURL(nil, node, "", time.Minute, false, node.Name); !errors.Is(err, ErrUnsupported) {
		t.Errorf("err = %v, want ErrUnsupported", err)
	}
}
