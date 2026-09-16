package data

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"nagisa/internal/biz"
	"nagisa/internal/conf"

	"google.golang.org/protobuf/types/known/durationpb"
)

const stubBucket = "nagisa"

// stubS3 is the smallest S3 endpoint the object store talks to. It records what
// the client asked for, so the protocol behaviour can be asserted without
// running a real object storage.
type stubS3 struct {
	srv *httptest.Server

	mu            sync.Mutex
	ops           []string
	bodies        map[string][]byte
	copies        []string
	parts         []string
	deleted       []string
	listKeys      []string
	createBody    string
	aborted       bool
	created       bool
	objectMissing bool
	bucketMissing bool
	failPart      int32
}

func newStubS3(t *testing.T) *stubS3 {
	t.Helper()
	s := &stubS3{bodies: map[string][]byte{}}
	s.srv = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.srv.Close)
	return s
}

func newStubTLSS3(t *testing.T) *stubS3 {
	t.Helper()
	s := &stubS3{bodies: map[string][]byte{}}
	s.srv = httptest.NewTLSServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubS3) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	query := r.URL.Query()
	key := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, "/"+stubBucket), "/")

	s.mu.Lock()
	s.ops = append(s.ops, opName(r, query, key))
	s.mu.Unlock()

	switch {
	case r.Method == http.MethodHead && key == "":
		if s.flag(&s.bucketMissing) {
			writeS3Error(w, http.StatusNotFound, "NoSuchBucket", "the bucket does not exist")
			return
		}
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodPut && key == "":
		s.mu.Lock()
		s.created = true
		s.bucketMissing = false
		s.createBody = string(body)
		s.mu.Unlock()
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodHead:
		if s.flag(&s.objectMissing) {
			writeS3Error(w, http.StatusNotFound, "NotFound", "the object does not exist")
			return
		}
		w.Header().Set("Content-Length", "11")
		w.Header().Set("ETag", `"head-etag"`)
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Last-Modified", "Mon, 01 Jan 2024 00:00:00 GMT")
		w.WriteHeader(http.StatusOK)

	case r.Method == http.MethodGet && query.Get("list-type") == "2":
		s.mu.Lock()
		keys := append([]string(nil), s.listKeys...)
		s.mu.Unlock()
		var b strings.Builder
		b.WriteString(`<ListBucketResult><Name>` + stubBucket + `</Name><Prefix></Prefix><MaxKeys>1000</MaxKeys>`)
		b.WriteString(`<IsTruncated>false</IsTruncated><KeyCount>` + strconv.Itoa(len(keys)) + `</KeyCount>`)
		for _, listed := range keys {
			b.WriteString(`<Contents><Key>` + listed + `</Key><LastModified>2024-01-01T00:00:00.000Z</LastModified>`)
			b.WriteString(`<ETag>&quot;list-etag&quot;</ETag><Size>3</Size><StorageClass>STANDARD</StorageClass></Contents>`)
		}
		b.WriteString(`</ListBucketResult>`)
		writeXML(w, http.StatusOK, b.String())

	case r.Method == http.MethodGet:
		if s.flag(&s.objectMissing) {
			writeS3Error(w, http.StatusNotFound, "NoSuchKey", "the key does not exist")
			return
		}
		w.Header().Set("Content-Length", "11")
		w.Header().Set("ETag", `"get-etag"`)
		_, _ = w.Write([]byte("hello world"))

	case r.Method == http.MethodPost && query.Has("uploads"):
		writeXML(w, http.StatusOK, `<InitiateMultipartUploadResult><Bucket>`+stubBucket+
			`</Bucket><Key>`+key+`</Key><UploadId>stub-upload-id</UploadId></InitiateMultipartUploadResult>`)

	case r.Method == http.MethodPut && query.Has("uploadId"):
		part := query.Get("partNumber")
		s.mu.Lock()
		s.copies = append(s.copies, r.Header.Get("x-amz-copy-source"))
		shouldFail := s.failPart != 0 && part == strconv.Itoa(int(s.failPart))
		s.mu.Unlock()
		if shouldFail {
			writeS3Error(w, http.StatusInternalServerError, "InternalError", "copy failed")
			return
		}
		writeXML(w, http.StatusOK, `<CopyPartResult><ETag>&quot;copy-`+part+
			`&quot;</ETag><LastModified>2024-01-01T00:00:00.000Z</LastModified></CopyPartResult>`)

	case r.Method == http.MethodPost && query.Has("uploadId"):
		var req struct {
			Parts []struct {
				PartNumber int32 `xml:"PartNumber"`
			} `xml:"Part"`
		}
		_ = xml.Unmarshal(body, &req)
		s.mu.Lock()
		for _, p := range req.Parts {
			s.parts = append(s.parts, strconv.Itoa(int(p.PartNumber)))
		}
		s.mu.Unlock()
		writeXML(w, http.StatusOK, `<CompleteMultipartUploadResult><Location>http://stub/`+key+
			`</Location><Bucket>`+stubBucket+`</Bucket><Key>`+key+`</Key><ETag>&quot;composed-etag&quot;</ETag></CompleteMultipartUploadResult>`)

	case r.Method == http.MethodDelete && query.Has("uploadId"):
		s.mu.Lock()
		s.aborted = true
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	case r.Method == http.MethodPost && query.Has("delete"):
		var req struct {
			Objects []struct {
				Key string `xml:"Key"`
			} `xml:"Object"`
		}
		_ = xml.Unmarshal(body, &req)
		s.mu.Lock()
		for _, obj := range req.Objects {
			s.deleted = append(s.deleted, obj.Key)
		}
		s.mu.Unlock()
		writeXML(w, http.StatusOK, `<DeleteResult></DeleteResult>`)

	case r.Method == http.MethodDelete:
		s.mu.Lock()
		s.deleted = append(s.deleted, key)
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	case r.Method == http.MethodPut:
		s.mu.Lock()
		s.bodies[key] = body
		s.mu.Unlock()
		w.Header().Set("ETag", `"stub-etag"`)
		w.WriteHeader(http.StatusOK)

	default:
		writeS3Error(w, http.StatusBadRequest, "InvalidRequest", "unexpected request")
	}
}

// opName names a request the way a test reads it back as a sequence.
func opName(r *http.Request, query url.Values, key string) string {
	switch {
	case r.Method == http.MethodHead && key == "":
		return "head-bucket"
	case r.Method == http.MethodHead:
		return "head-object"
	case r.Method == http.MethodPut && key == "":
		return "create-bucket"
	case r.Method == http.MethodPost && query.Has("uploads"):
		return "create-multipart"
	case r.Method == http.MethodPut && query.Has("uploadId"):
		return "copy-part"
	case r.Method == http.MethodPost && query.Has("uploadId"):
		return "complete-multipart"
	case r.Method == http.MethodDelete && query.Has("uploadId"):
		return "abort-multipart"
	case r.Method == http.MethodPost && query.Has("delete"):
		return "delete-objects"
	case r.Method == http.MethodDelete:
		return "delete-object"
	case r.Method == http.MethodPut:
		return "put-object"
	case r.Method == http.MethodGet && query.Get("list-type") == "2":
		return "list-objects"
	case r.Method == http.MethodGet:
		return "get-object"
	default:
		return r.Method
	}
}

func (s *stubS3) flag(target *bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *target
}

type stubRecord struct {
	ops        []string
	copies     []string
	parts      []string
	deleted    []string
	createBody string
	aborted    bool
	created    bool
}

func (s *stubS3) recorded() stubRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	return stubRecord{
		ops:        append([]string(nil), s.ops...),
		copies:     append([]string(nil), s.copies...),
		parts:      append([]string(nil), s.parts...),
		deleted:    append([]string(nil), s.deleted...),
		createBody: s.createBody,
		aborted:    s.aborted,
		created:    s.created,
	}
}

func (s *stubS3) stored(key string) []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.bodies[key]
}

func (s *stubS3) setList(keys ...string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listKeys = keys
}

func (s *stubS3) setObjectMissing(missing bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objectMissing = missing
}

func testStore(t *testing.T, stub *stubS3, opts ...func(*conf.Data_ObjectStorage)) *objectStore {
	t.Helper()
	oc := &conf.Data_ObjectStorage{
		Endpoint:   strings.TrimPrefix(stub.srv.URL, "http://"),
		AccessKey:  "access",
		SecretKey:  "secret",
		Bucket:     stubBucket,
		Prefix:     "netdisk",
		PresignTtl: durationpb.New(30 * time.Minute),
	}
	for _, opt := range opts {
		opt(oc)
	}
	store, err := newObjectStore(&conf.Data{ObjectStorage: oc})
	if err != nil {
		t.Fatalf("new object store: %v", err)
	}
	return store
}

func TestPutObjectStreamsABodyOfKnownLength(t *testing.T) {
	stub := newStubS3(t)
	store := testStore(t, stub)

	// A pipe cannot be rewound, so this fails if the client insists on a
	// seekable body instead of streaming the length it was given.
	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("hello world"))
		_ = pw.Close()
	}()
	etag, err := store.PutObject(context.Background(), "files/owner/node", pr, 11, "text/plain", map[string]string{"upload-id": "u"})
	if err != nil {
		t.Fatalf("put object: %v", err)
	}
	if etag != "stub-etag" {
		t.Errorf("etag = %q, want the unquoted stub-etag", etag)
	}
	if got := string(stub.stored("netdisk/files/owner/node")); got != "hello world" {
		t.Errorf("stored body = %q, want hello world", got)
	}
}

func TestPutObjectStoresAnEmptyBody(t *testing.T) {
	stub := newStubS3(t)
	store := testStore(t, stub)

	// An empty upload sends no length at all, which is what the service layer
	// hands over for a zero byte part.
	if _, err := store.PutObject(context.Background(), "files/owner/empty", bytes.NewReader(nil), 0, "", nil); err != nil {
		t.Fatalf("put object: %v", err)
	}
	if got := stub.stored("netdisk/files/owner/empty"); len(got) != 0 {
		t.Errorf("stored body = %q, want empty", got)
	}
}

// The payload hash the SDK would normally sign needs a body it can rewind. Over
// plain HTTP there is no trailing checksum fallback, so a streamed part has to
// send an unsigned payload instead of failing.
func TestPutObjectStreamsOverTLS(t *testing.T) {
	stub := newStubTLSS3(t)
	store := testStore(t, stub, func(oc *conf.Data_ObjectStorage) {
		oc.InsecureSkipVerify = true
	})

	pr, pw := io.Pipe()
	go func() {
		_, _ = pw.Write([]byte("hello world"))
		_ = pw.Close()
	}()
	if _, err := store.PutObject(context.Background(), "files/owner/node", pr, 11, "text/plain", nil); err != nil {
		t.Fatalf("put object over TLS: %v", err)
	}
	if got := string(stub.stored("netdisk/files/owner/node")); got != "hello world" {
		t.Errorf("stored body = %q, want hello world", got)
	}
}

func TestGetObjectAsksForTheRequestedRange(t *testing.T) {
	stub := newStubS3(t)
	store := testStore(t, stub)

	body, info, err := store.GetObject(context.Background(), "files/owner/node", 2, 4)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	defer func() { _ = body.Close() }()
	if info.Size != 11 {
		t.Errorf("size = %d, want 11", info.Size)
	}
	if info.Etag != "head-etag" {
		t.Errorf("etag = %q, want the unquoted head-etag", info.Etag)
	}
	if info.ContentType != "text/plain" {
		t.Errorf("content type = %q, want text/plain", info.ContentType)
	}
	if !info.LastModified.Equal(time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("last modified = %s", info.LastModified)
	}
	rec := stub.recorded()
	if strings.Join(rec.ops, ",") != "head-object,get-object" {
		t.Errorf("ops = %v, want [head-object get-object]", rec.ops)
	}
}

func TestGetObjectPastTheEndReturnsAnEmptyBody(t *testing.T) {
	stub := newStubS3(t)
	store := testStore(t, stub)

	body, info, err := store.GetObject(context.Background(), "files/owner/node", 11, 0)
	if err != nil {
		t.Fatalf("get object: %v", err)
	}
	defer func() { _ = body.Close() }()
	got, _ := io.ReadAll(body)
	if len(got) != 0 {
		t.Errorf("body = %q, want empty", got)
	}
	if info.Size != 11 {
		t.Errorf("size = %d, want 11", info.Size)
	}
	// A range at the end must not become a 416 from the backend.
	rec := stub.recorded()
	if strings.Join(rec.ops, ",") != "head-object" {
		t.Errorf("ops = %v, want [head-object]", rec.ops)
	}
}

func TestMissingObjectsMapToTheDomainError(t *testing.T) {
	stub := newStubS3(t)
	stub.setObjectMissing(true)
	store := testStore(t, stub)

	if _, err := store.StatObject(context.Background(), "files/owner/gone"); err != biz.ErrNotFound {
		t.Errorf("stat error = %v, want biz.ErrNotFound", err)
	}
	if _, _, err := store.GetObject(context.Background(), "files/owner/gone", 0, 0); err != biz.ErrNotFound {
		t.Errorf("get error = %v, want biz.ErrNotFound", err)
	}
}

func TestComposeObjectCopiesEveryPartServerSide(t *testing.T) {
	stub := newStubS3(t)
	store := testStore(t, stub)

	etag, err := store.ComposeObject(context.Background(), "files/owner/merged",
		[]string{"uploads/session/parts/1", "uploads/session/parts/2", "uploads/session/parts/3"},
		"application/pdf")
	if err != nil {
		t.Fatalf("compose object: %v", err)
	}
	if etag != "composed-etag" {
		t.Errorf("etag = %q, want the unquoted composed-etag", etag)
	}
	rec := stub.recorded()
	want := "create-multipart,copy-part,copy-part,copy-part,complete-multipart"
	if strings.Join(rec.ops, ",") != want {
		t.Errorf("ops = %v, want %s", rec.ops, want)
	}
	wantCopies := "nagisa/netdisk/uploads/session/parts/1," +
		"nagisa/netdisk/uploads/session/parts/2," +
		"nagisa/netdisk/uploads/session/parts/3"
	if strings.Join(rec.copies, ",") != wantCopies {
		t.Errorf("copy sources = %v, want %s", rec.copies, wantCopies)
	}
	if strings.Join(rec.parts, ",") != "1,2,3" {
		t.Errorf("completed parts = %v, want 1,2,3 in order", rec.parts)
	}
	if rec.aborted {
		t.Error("the multipart upload was aborted after a successful completion")
	}
}

func TestComposeObjectAbortsWhenACopyFails(t *testing.T) {
	stub := newStubS3(t)
	stub.mu.Lock()
	stub.failPart = 2
	stub.mu.Unlock()
	store := testStore(t, stub)

	if _, err := store.ComposeObject(context.Background(), "files/owner/merged",
		[]string{"uploads/session/parts/1", "uploads/session/parts/2"}, ""); err == nil {
		t.Fatal("compose object: want an error")
	}
	rec := stub.recorded()
	if !rec.aborted {
		t.Errorf("ops = %v: the failed multipart upload was not aborted", rec.ops)
	}
	if rec.ops[len(rec.ops)-1] != "abort-multipart" {
		t.Errorf("ops = %v, want the abort last", rec.ops)
	}
}

func TestComposeObjectRejectsAnEmptySourceList(t *testing.T) {
	stub := newStubS3(t)
	store := testStore(t, stub)

	if _, err := store.ComposeObject(context.Background(), "files/owner/merged", nil, ""); err != biz.ErrInvalidArgument {
		t.Errorf("error = %v, want biz.ErrInvalidArgument", err)
	}
	if rec := stub.recorded(); len(rec.ops) != 0 {
		t.Errorf("ops = %v, want no requests", rec.ops)
	}
}

func TestCopySourceEscapesEveryKeySegment(t *testing.T) {
	got := copySource("nagisa", "netdisk/files/owner/我的 照片.jpg")
	want := "nagisa/netdisk/files/owner/%E6%88%91%E7%9A%84%20%E7%85%A7%E7%89%87.jpg"
	if got != want {
		t.Errorf("copy source = %q, want %q", got, want)
	}
}

func TestListPrefixStripsTheConfiguredPrefix(t *testing.T) {
	stub := newStubS3(t)
	stub.setList("netdisk/uploads/session/parts/1", "netdisk/uploads/session/parts/2")
	store := testStore(t, stub)

	objects, err := store.ListPrefix(context.Background(), "uploads/session")
	if err != nil {
		t.Fatalf("list prefix: %v", err)
	}
	if len(objects) != 2 {
		t.Fatalf("objects = %d, want 2", len(objects))
	}
	if objects[0].Key != "uploads/session/parts/1" {
		t.Errorf("key = %q, want the configured prefix stripped", objects[0].Key)
	}
	if objects[0].Etag != "list-etag" {
		t.Errorf("etag = %q, want the unquoted list-etag", objects[0].Etag)
	}
	if objects[0].Size != 3 {
		t.Errorf("size = %d, want 3", objects[0].Size)
	}
}

func TestRemovePrefixDeletesTheObjectsAndThePrefixItself(t *testing.T) {
	stub := newStubS3(t)
	stub.setList("netdisk/uploads/session/parts/1", "netdisk/uploads/session/parts/2")
	store := testStore(t, stub)

	if err := store.RemovePrefix(context.Background(), "uploads/session"); err != nil {
		t.Fatalf("remove prefix: %v", err)
	}
	want := "netdisk/uploads/session/parts/1," +
		"netdisk/uploads/session/parts/2," +
		"netdisk/uploads/session"
	if rec := stub.recorded(); strings.Join(rec.deleted, ",") != want {
		t.Errorf("deleted = %v, want %s", rec.deleted, want)
	}
}

func TestDisabledStoreRejectsContentOperations(t *testing.T) {
	store, err := newObjectStore(&conf.Data{ObjectStorage: &conf.Data_ObjectStorage{}})
	if err != nil {
		t.Fatalf("new object store: %v", err)
	}
	if store.BackendName() != "none" {
		t.Errorf("backend = %q, want none", store.BackendName())
	}
	// Cleanup stays silent so the maintenance jobs can retry without a backend.
	if err := store.RemovePrefix(context.Background(), "uploads/session"); err != nil {
		t.Errorf("remove prefix: %v", err)
	}
	if err := store.RemoveObject(context.Background(), "files/owner/node"); err != nil {
		t.Errorf("remove object: %v", err)
	}
	if err := store.HealthCheck(context.Background()); err != nil {
		t.Errorf("health check: %v", err)
	}
	if _, err := store.PutObject(context.Background(), "k", strings.NewReader("x"), 1, "", nil); err != biz.ErrUnavailable {
		t.Errorf("put error = %v, want biz.ErrUnavailable", err)
	}
	if _, err := store.PresignGetObject(context.Background(), "k", "", "", "", time.Minute); err != biz.ErrUnavailable {
		t.Errorf("presign error = %v, want biz.ErrUnavailable", err)
	}
}

func TestPresignUsesThePublicHost(t *testing.T) {
	stub := newStubS3(t)
	store := testStore(t, stub, func(oc *conf.Data_ObjectStorage) {
		oc.PublicEndpoint = "https://files.example.com"
	})

	put, err := store.PresignPutObject(context.Background(), "uploads/session/parts/1", 30*time.Minute)
	if err != nil {
		t.Fatalf("presign put: %v", err)
	}
	parsed, err := url.Parse(put.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Host != "files.example.com" || parsed.Scheme != "https" {
		t.Errorf("url = %s://%s, want https://files.example.com", parsed.Scheme, parsed.Host)
	}
	if parsed.Path != "/nagisa/netdisk/uploads/session/parts/1" {
		t.Errorf("url path = %q, want the bucket and key in the path", parsed.Path)
	}
	if query := parsed.Query(); query.Get("X-Amz-Signature") == "" || query.Get("X-Amz-Expires") != "1800" {
		t.Errorf("query = %v, want a signature and the 1800 second expiry", query)
	}
	if put.Method != http.MethodPut {
		t.Errorf("method = %q, want PUT", put.Method)
	}
	if put.Headers != nil {
		t.Errorf("headers = %v, want none: only the host is signed", put.Headers)
	}

	get, err := store.PresignGetObject(context.Background(), "files/owner/node", "报告.pdf", "application/pdf", "attachment", time.Hour)
	if err != nil {
		t.Fatalf("presign get: %v", err)
	}
	parsed, err = url.Parse(get.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Host != "files.example.com" {
		t.Errorf("url host = %q, want the public host", parsed.Host)
	}
	query := parsed.Query()
	if !strings.Contains(query.Get("response-content-disposition"), "filename*=UTF-8''") {
		t.Errorf("content disposition = %q, want a UTF-8 file name", query.Get("response-content-disposition"))
	}
	if query.Get("response-content-type") != "application/pdf" {
		t.Errorf("content type = %q, want application/pdf", query.Get("response-content-type"))
	}
	if get.Method != http.MethodGet {
		t.Errorf("method = %q, want GET", get.Method)
	}
}

func TestPresignFallsBackToTheServiceEndpoint(t *testing.T) {
	stub := newStubS3(t)
	store := testStore(t, stub)

	out, err := store.PresignGetObject(context.Background(), "files/owner/node", "", "", "", time.Minute)
	if err != nil {
		t.Fatalf("presign get: %v", err)
	}
	parsed, err := url.Parse(out.URL)
	if err != nil {
		t.Fatalf("parse url: %v", err)
	}
	if parsed.Host != strings.TrimPrefix(stub.srv.URL, "http://") {
		t.Errorf("url host = %q, want the configured endpoint", parsed.Host)
	}
	if query := parsed.Query(); query.Has("response-content-disposition") {
		t.Errorf("query = %v, want no response overrides without a file name", query)
	}
}

func TestHealthCheckReportsAMissingBucket(t *testing.T) {
	stub := newStubS3(t)
	stub.mu.Lock()
	stub.bucketMissing = true
	stub.mu.Unlock()
	store := testStore(t, stub)

	err := store.HealthCheck(context.Background())
	if err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("health check error = %v, want a missing bucket report", err)
	}
}

func TestAutoCreateBucketProbesThenCreates(t *testing.T) {
	stub := newStubS3(t)
	stub.mu.Lock()
	stub.bucketMissing = true
	stub.mu.Unlock()
	store := testStore(t, stub, func(oc *conf.Data_ObjectStorage) {
		oc.AutoCreateBucket = true
		oc.Region = "eu-west-1"
	})

	if store.BackendName() != "s3" {
		t.Errorf("backend = %q, want s3", store.BackendName())
	}
	rec := stub.recorded()
	if strings.Join(rec.ops, ",") != "head-bucket,create-bucket" {
		t.Errorf("ops = %v, want [head-bucket create-bucket]", rec.ops)
	}
	if !strings.Contains(rec.createBody, "eu-west-1") {
		t.Errorf("create bucket body = %q, want the configured region", rec.createBody)
	}
}

func TestAutoCreateBucketSkipsAnExistingBucket(t *testing.T) {
	stub := newStubS3(t)
	store := testStore(t, stub, func(oc *conf.Data_ObjectStorage) {
		oc.AutoCreateBucket = true
	})

	if store.BackendName() != "s3" {
		t.Errorf("backend = %q, want s3", store.BackendName())
	}
	rec := stub.recorded()
	if strings.Join(rec.ops, ",") != "head-bucket" {
		t.Errorf("ops = %v, want only the probe", rec.ops)
	}
	if rec.created {
		t.Error("the bucket was created although the probe found it")
	}
}

func TestSplitEndpoint(t *testing.T) {
	cases := []struct {
		endpoint string
		secure   bool
		host     string
		scheme   string
	}{
		{"127.0.0.1:8333", false, "127.0.0.1:8333", "http"},
		{"127.0.0.1:8333", true, "127.0.0.1:8333", "https"},
		{"http://s3.internal:8333", true, "s3.internal:8333", "https"},
		{"https://s3.example.com", false, "s3.example.com", "https"},
	}
	for _, tc := range cases {
		host, scheme := splitEndpoint(tc.endpoint, tc.secure)
		if host != tc.host || scheme != tc.scheme {
			t.Errorf("splitEndpoint(%q, %v) = %q, %q; want %q, %q",
				tc.endpoint, tc.secure, host, scheme, tc.host, tc.scheme)
		}
	}
}

func writeXML(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(xml.Header + body))
}

func writeS3Error(w http.ResponseWriter, status int, code, message string) {
	writeXML(w, status, "<Error><Code>"+code+"</Code><Message>"+message+"</Message></Error>")
}
