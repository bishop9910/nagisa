//go:build s3smoke

package data

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"nagisa/internal/biz"
	"nagisa/internal/conf"

	"google.golang.org/protobuf/types/known/durationpb"
)

// TestLiveS3RoundTrip drives the object store against a real S3 backend. The
// stub server in objectstore_test.go proves the protocol this client emits; this
// proves the backend accepts it, which is the part a mock cannot answer:
// signature acceptance, UploadPartCopy, presigned URLs and batch deletes.
//
//	$env:NAGISA_S3_SMOKE_ENDPOINT="127.0.0.1:8333"
//	go test -tags s3smoke ./internal/data/ -run LiveS3 -v
//
// Optional: NAGISA_S3_SMOKE_ACCESS_KEY, NAGISA_S3_SMOKE_SECRET_KEY,
// NAGISA_S3_SMOKE_BUCKET.
func TestLiveS3RoundTrip(t *testing.T) {
	endpoint := os.Getenv("NAGISA_S3_SMOKE_ENDPOINT")
	if endpoint == "" {
		t.Skip("set NAGISA_S3_SMOKE_ENDPOINT to run the live S3 smoke test")
	}
	store := liveStore(t, endpoint)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	prefix := "smoke-" + time.Now().UTC().Format("20060102-150405")
	partSize := int64(5 << 20) // the S3 minimum for every part but the last
	first := randomBytes(t, partSize)
	second := randomBytes(t, partSize)
	last := []byte("tail bytes")
	want := append(append(append([]byte{}, first...), second...), last...)

	parts := []string{prefix + "/parts/1", prefix + "/parts/2", prefix + "/parts/3"}
	for i, body := range [][]byte{first, second, last} {
		if _, err := store.PutObject(ctx, parts[i], bytes.NewReader(body), int64(len(body)), "application/octet-stream", nil); err != nil {
			t.Fatalf("put %s: %v", parts[i], err)
		}
	}

	final := prefix + "/files/merged"
	etag, err := store.ComposeObject(ctx, final, parts, "application/pdf")
	if err != nil {
		t.Fatalf("compose object: %v", err)
	}
	if etag == "" {
		t.Error("compose object returned an empty etag")
	}
	info, err := store.StatObject(ctx, final)
	if err != nil {
		t.Fatalf("stat composed object: %v", err)
	}
	if info.Size != int64(len(want)) {
		t.Errorf("composed size = %d, want %d", info.Size, len(want))
	}
	if info.ContentType != "application/pdf" {
		t.Errorf("composed content type = %q, want application/pdf", info.ContentType)
	}

	// A ranged read goes through HeadObject plus a Range request.
	body, _, err := store.GetObject(ctx, final, partSize-4, 8)
	if err != nil {
		t.Fatalf("ranged get: %v", err)
	}
	got, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		t.Fatalf("read ranged body: %v", err)
	}
	if !bytes.Equal(got, want[partSize-4:partSize+4]) {
		t.Errorf("ranged read = %q, want the bytes that straddle the part boundary", got)
	}

	// The download URL must carry the file name the client asked for.
	presigned, err := store.PresignGetObject(ctx, final, "报告 2026.pdf", "application/pdf", "attachment", 10*time.Minute)
	if err != nil {
		t.Fatalf("presign get: %v", err)
	}
	resp, err := http.Get(presigned.URL)
	if err != nil {
		t.Fatalf("fetch presigned url: %v", err)
	}
	downloaded, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatalf("read presigned body: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("presigned get status = %d, body = %s", resp.StatusCode, downloaded)
	}
	if !bytes.Equal(downloaded, want) {
		t.Errorf("presigned download = %d bytes, want %d", len(downloaded), len(want))
	}
	if disposition := resp.Header.Get("Content-Disposition"); !strings.Contains(disposition, "filename*=UTF-8''") {
		t.Errorf("content disposition = %q, want the requested file name", disposition)
	}

	// A presigned PUT is what the browser sends in the default upload mode.
	direct := prefix + "/direct/upload.bin"
	signed, err := store.PresignPutObject(ctx, direct, 10*time.Minute)
	if err != nil {
		t.Fatalf("presign put: %v", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, signed.URL, bytes.NewReader(last))
	if err != nil {
		t.Fatalf("build presigned put: %v", err)
	}
	uploaded, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("presigned put: %v", err)
	}
	_ = uploaded.Body.Close()
	if uploaded.StatusCode != http.StatusOK {
		t.Fatalf("presigned put status = %d", uploaded.StatusCode)
	}
	directInfo, err := store.StatObject(ctx, direct)
	if err != nil {
		t.Fatalf("stat direct upload: %v", err)
	}
	if directInfo.Size != int64(len(last)) {
		t.Errorf("direct upload size = %d, want %d", directInfo.Size, len(last))
	}

	objects, err := store.ListPrefix(ctx, prefix)
	if err != nil {
		t.Fatalf("list prefix: %v", err)
	}
	if len(objects) != 5 {
		t.Errorf("listed %d objects, want 5", len(objects))
	}

	if err := store.RemovePrefix(ctx, prefix); err != nil {
		t.Fatalf("remove prefix: %v", err)
	}
	if _, err := store.StatObject(ctx, final); err != biz.ErrNotFound {
		t.Errorf("stat after remove = %v, want biz.ErrNotFound", err)
	}
	if _, err := store.StatObject(ctx, direct); err != biz.ErrNotFound {
		t.Errorf("stat direct after remove = %v, want biz.ErrNotFound", err)
	}
}

func liveStore(t *testing.T, endpoint string) *objectStore {
	t.Helper()
	env := func(name, fallback string) string {
		if value := os.Getenv(name); value != "" {
			return value
		}
		return fallback
	}
	oc := &conf.Data_ObjectStorage{
		Endpoint:         endpoint,
		AccessKey:        env("NAGISA_S3_SMOKE_ACCESS_KEY", "nagisa"),
		SecretKey:        env("NAGISA_S3_SMOKE_SECRET_KEY", "nagisa-secret"),
		Bucket:           env("NAGISA_S3_SMOKE_BUCKET", "nagisa-smoke"),
		Prefix:           "smoke",
		PresignTtl:       durationpb.New(10 * time.Minute),
		AutoCreateBucket: true,
	}
	store, err := newObjectStore(&conf.Data{ObjectStorage: oc})
	if err != nil {
		t.Fatalf("new object store against %s: %v", endpoint, err)
	}
	return store
}

func randomBytes(t *testing.T, size int64) []byte {
	t.Helper()
	out := make([]byte, size)
	if _, err := rand.Read(out); err != nil {
		t.Fatalf("random bytes: %v", err)
	}
	return out
}
