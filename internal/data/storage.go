package data

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nagisa/internal/biz"
	"nagisa/internal/conf"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/go-kratos/kratos/v3/log"
)

const (
	// deleteBatchSize is how many keys DeleteObjects accepts in one request.
	deleteBatchSize = 1000
	// maxComposeParts is the multipart part count S3 allows.
	maxComposeParts = 10000
	// defaultPresignTTL applies when the configuration does not set one.
	defaultPresignTTL = 30 * time.Minute
	// defaultRegion is the signing region for backends that do not care.
	defaultRegion = "us-east-1"
)

// objectStore talks to an S3 compatible backend. Objects are stored under a
// configured prefix so one bucket can host several deployments.
type objectStore struct {
	client  *s3.Client
	presign *s3.PresignClient
	// public is a second client bound to the address browsers use. Presigned
	// URLs are signed for a specific host, so a deployment behind a proxy or a
	// different DNS name needs its own signer.
	public        *s3.Client
	publicPresign *s3.PresignClient
	bucket        string
	prefix        string
	presignTTL    time.Duration
	endpoint      string
	publicHost    string
	backendName   string
}

func newObjectStore(c *conf.Data) (*objectStore, error) {
	oc := c.GetObjectStorage()
	if oc == nil || oc.GetEndpoint() == "" {
		return &objectStore{backendName: "none", presignTTL: defaultPresignTTL}, nil
	}
	client, err := newS3Client(oc.GetEndpoint(), oc.GetUseSsl(), oc)
	if err != nil {
		return nil, err
	}
	store := &objectStore{
		client:      client,
		presign:     s3.NewPresignClient(client),
		bucket:      oc.GetBucket(),
		prefix:      strings.Trim(oc.GetPrefix(), "/"),
		presignTTL:  oc.GetPresignTtl().AsDuration(),
		endpoint:    oc.GetEndpoint(),
		backendName: "s3",
	}
	if store.bucket == "" {
		store.bucket = "netdisk"
	}
	if store.presignTTL <= 0 {
		store.presignTTL = defaultPresignTTL
	}
	if pub := oc.GetPublicEndpoint(); pub != "" {
		if pub != oc.GetEndpoint() {
			public, err := newS3Client(pub, oc.GetUseSsl(), oc)
			if err != nil {
				return nil, err
			}
			store.public = public
			store.publicPresign = s3.NewPresignClient(public)
		}
		store.publicHost, _ = splitEndpoint(pub, oc.GetUseSsl())
	}
	if oc.GetAutoCreateBucket() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if _, err := client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(store.bucket)}); err != nil {
			if !isNotFound(err) {
				return nil, fmt.Errorf("data: probe bucket: %w", err)
			}
			in := &s3.CreateBucketInput{Bucket: aws.String(store.bucket)}
			if region := oc.GetRegion(); region != "" && region != defaultRegion {
				in.CreateBucketConfiguration = &types.CreateBucketConfiguration{
					LocationConstraint: types.BucketLocationConstraint(region),
				}
			}
			if _, err := client.CreateBucket(ctx, in); err != nil {
				return nil, fmt.Errorf("data: create bucket %q: %w", store.bucket, err)
			}
			log.Info("created object storage bucket", "bucket", store.bucket)
		}
	}
	return store, nil
}

// newS3Client builds a path style client for one endpoint. Path style keeps the
// bucket in the path, which is what self hosted S3 implementations serve and
// what the signed URLs handed to browsers look like.
func newS3Client(rawEndpoint string, secure bool, oc *conf.Data_ObjectStorage) (*s3.Client, error) {
	host, scheme := splitEndpoint(rawEndpoint, secure)
	if host == "" {
		return nil, errors.New("data: empty object storage endpoint")
	}
	region := oc.GetRegion()
	if region == "" {
		region = defaultRegion
	}
	cfg := aws.Config{
		Region:      region,
		Credentials: credentials.NewStaticCredentialsProvider(oc.GetAccessKey(), oc.GetSecretKey(), ""),
	}
	if oc.GetInsecureSkipVerify() {
		cfg.HTTPClient = &http.Client{Transport: &http.Transport{TLSClientConfig: insecureTLS()}}
	}
	return s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(scheme + "://" + host)
		o.UsePathStyle = true
		// The SDK adds CRC32 integrity headers to uploads by default. Older S3
		// compatible backends reject them, so only send what an operation
		// requires and only validate checksums the backend actually returns.
		o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
		o.ResponseChecksumValidation = aws.ResponseChecksumValidationWhenRequired
		if scheme != "https" {
			// Signing the payload means reading the body and rewinding it, which
			// a streamed upload cannot do, and without TLS there is no trailing
			// checksum fallback. Send an unsigned payload instead: every self
			// hosted S3 implementation accepts it, and port 8333 on an internal
			// address is the only place a plain endpoint makes sense.
			o.APIOptions = append(o.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
		}
	}), nil
}

// splitEndpoint separates the host from the scheme. The configuration accepts
// the endpoint with or without a scheme: use_ssl decides, and an explicit
// https:// prefix turns it on.
func splitEndpoint(rawEndpoint string, secure bool) (string, string) {
	host := strings.TrimPrefix(strings.TrimPrefix(rawEndpoint, "https://"), "http://")
	if secure || strings.HasPrefix(rawEndpoint, "https://") {
		return host, "https"
	}
	return host, "http"
}

// NewObjectStore exposes the object store as the domain interface.
func NewObjectStore(d *Data) biz.ObjectStore { return d.objects }

// key applies the configured prefix.
func (o *objectStore) key(k string) string {
	k = strings.TrimPrefix(k, "/")
	if o.prefix == "" {
		return k
	}
	return o.prefix + "/" + k
}

// Enabled reports whether an object storage backend is configured.
func (o *objectStore) Enabled() bool { return o != nil && o.client != nil }

// BackendName returns the backend identifier reported by SystemService.
func (o *objectStore) BackendName() string {
	if o == nil {
		return "none"
	}
	return o.backendName
}

func (o *objectStore) PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string, meta map[string]string) (string, error) {
	if !o.Enabled() {
		return "", biz.ErrUnavailable
	}
	in := &s3.PutObjectInput{
		Bucket:      aws.String(o.bucket),
		Key:         aws.String(o.key(key)),
		Body:        r,
		ContentType: contentTypeOrNil(contentType),
		Metadata:    meta,
	}
	// A zero size means an empty object; a positive one lets the client stream
	// the body without buffering it to measure the length.
	if size > 0 {
		in.ContentLength = aws.Int64(size)
	}
	out, err := o.client.PutObject(ctx, in)
	if err != nil {
		return "", fmt.Errorf("data: put object %q: %w", key, err)
	}
	return trimETag(out.ETag), nil
}

func (o *objectStore) GetObject(ctx context.Context, key string, offset, length int64) (io.ReadCloser, *biz.ObjectInfo, error) {
	if !o.Enabled() {
		return nil, nil, biz.ErrUnavailable
	}
	objKey := o.key(key)
	head, err := o.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(o.bucket), Key: aws.String(objKey)})
	if err != nil {
		if isNotFound(err) {
			return nil, nil, biz.ErrNotFound
		}
		return nil, nil, fmt.Errorf("data: stat object %q: %w", key, err)
	}
	info := &biz.ObjectInfo{
		Key: key, Size: aws.ToInt64(head.ContentLength), Etag: trimETag(head.ETag),
		ContentType: aws.ToString(head.ContentType), LastModified: timeOrZero(head.LastModified),
	}
	if offset >= info.Size {
		return io.NopCloser(strings.NewReader("")), info, nil
	}
	in := &s3.GetObjectInput{Bucket: aws.String(o.bucket), Key: aws.String(objKey)}
	if offset > 0 || length > 0 {
		last := info.Size - 1
		if length > 0 && offset+length-1 < last {
			last = offset + length - 1
		}
		in.Range = aws.String(fmt.Sprintf("bytes=%d-%d", offset, last))
	}
	out, err := o.client.GetObject(ctx, in)
	if err != nil {
		if isNotFound(err) {
			return nil, nil, biz.ErrNotFound
		}
		return nil, nil, fmt.Errorf("data: get object %q: %w", key, err)
	}
	return out.Body, info, nil
}

func (o *objectStore) StatObject(ctx context.Context, key string) (*biz.ObjectInfo, error) {
	if !o.Enabled() {
		return nil, biz.ErrUnavailable
	}
	head, err := o.client.HeadObject(ctx, &s3.HeadObjectInput{Bucket: aws.String(o.bucket), Key: aws.String(o.key(key))})
	if err != nil {
		if isNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: stat object %q: %w", key, err)
	}
	return &biz.ObjectInfo{
		Key: key, Size: aws.ToInt64(head.ContentLength), Etag: trimETag(head.ETag),
		ContentType: aws.ToString(head.ContentType), LastModified: timeOrZero(head.LastModified),
	}, nil
}

func (o *objectStore) RemoveObject(ctx context.Context, key string) error {
	if !o.Enabled() {
		return nil
	}
	// Deleting a key that does not exist is a success in S3, so this stays
	// idempotent for the cleanup jobs that retry it.
	if _, err := o.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(o.bucket),
		Key:    aws.String(o.key(key)),
	}); err != nil {
		return fmt.Errorf("data: remove object %q: %w", key, err)
	}
	return nil
}

// RemovePrefix removes every object below a prefix, including the prefix
// itself when it happens to be an object.
func (o *objectStore) RemovePrefix(ctx context.Context, prefix string) error {
	if !o.Enabled() {
		return nil
	}
	if prefix == "" {
		return nil
	}
	base := o.key(prefix)
	pages := s3.NewListObjectsV2Paginator(o.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(o.bucket),
		Prefix: aws.String(base + "/"),
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("data: remove prefix %q: %w", prefix, err)
		}
		if err := o.deleteObjects(ctx, page.Contents); err != nil {
			return fmt.Errorf("data: remove prefix %q: %w", prefix, err)
		}
	}
	// The prefix may also be an object on its own, for example an upload that
	// stored a single part under its session key.
	if err := o.deleteObjects(ctx, []types.Object{{Key: aws.String(base)}}); err != nil {
		return fmt.Errorf("data: remove prefix %q: %w", prefix, err)
	}
	return nil
}

// deleteObjects removes keys in batches, skipping the ones that are already
// gone so repeated cleanup stays silent.
func (o *objectStore) deleteObjects(ctx context.Context, objects []types.Object) error {
	for start := 0; start < len(objects); start += deleteBatchSize {
		end := start + deleteBatchSize
		if end > len(objects) {
			end = len(objects)
		}
		ids := make([]types.ObjectIdentifier, 0, end-start)
		for _, obj := range objects[start:end] {
			if obj.Key != nil {
				ids = append(ids, types.ObjectIdentifier{Key: obj.Key})
			}
		}
		if len(ids) == 0 {
			continue
		}
		out, err := o.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(o.bucket),
			Delete: &types.Delete{Objects: ids},
		})
		if err != nil {
			return err
		}
		for _, failed := range out.Errors {
			code := aws.ToString(failed.Code)
			if isNotFoundCode(code) {
				continue
			}
			return fmt.Errorf("remove object %q: %s", aws.ToString(failed.Key), code)
		}
	}
	return nil
}

func (o *objectStore) ListPrefix(ctx context.Context, prefix string) ([]biz.ObjectInfo, error) {
	if !o.Enabled() {
		return nil, nil
	}
	out := make([]biz.ObjectInfo, 0, 64)
	pages := s3.NewListObjectsV2Paginator(o.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(o.bucket),
		Prefix: aws.String(o.key(prefix)),
	})
	for pages.HasMorePages() {
		page, err := pages.NextPage(ctx)
		if err != nil {
			return out, fmt.Errorf("data: list prefix %q: %w", prefix, err)
		}
		for _, obj := range page.Contents {
			out = append(out, biz.ObjectInfo{
				Key: strings.TrimPrefix(aws.ToString(obj.Key), o.prefix+"/"), Size: aws.ToInt64(obj.Size),
				Etag: trimETag(obj.ETag), LastModified: timeOrZero(obj.LastModified),
			})
		}
	}
	return out, nil
}

// ComposeObject assembles the staged parts into the destination object with
// server side copies, so the bytes never pass through this process. Every
// source is copied as one multipart part, which S3 caps at 5 GiB; a staged part
// can never be larger than that because it was uploaded as one itself.
func (o *objectStore) ComposeObject(ctx context.Context, dstKey string, srcKeys []string, contentType string) (string, error) {
	if !o.Enabled() {
		return "", biz.ErrUnavailable
	}
	if len(srcKeys) == 0 {
		return "", biz.ErrInvalidArgument
	}
	if len(srcKeys) > maxComposeParts {
		return "", fmt.Errorf("data: compose object %q: %d parts exceed the S3 limit of %d", dstKey, len(srcKeys), maxComposeParts)
	}
	objKey := o.key(dstKey)
	created, err := o.client.CreateMultipartUpload(ctx, &s3.CreateMultipartUploadInput{
		Bucket:      aws.String(o.bucket),
		Key:         aws.String(objKey),
		ContentType: contentTypeOrNil(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("data: compose object %q: %w", dstKey, err)
	}
	uploadID := created.UploadId
	abort := func() {
		_, _ = o.client.AbortMultipartUpload(context.WithoutCancel(ctx), &s3.AbortMultipartUploadInput{
			Bucket:   aws.String(o.bucket),
			Key:      aws.String(objKey),
			UploadId: uploadID,
		})
	}
	parts := make([]types.CompletedPart, 0, len(srcKeys))
	for i, src := range srcKeys {
		partNumber := int32(i + 1)
		copied, err := o.client.UploadPartCopy(ctx, &s3.UploadPartCopyInput{
			Bucket:     aws.String(o.bucket),
			Key:        aws.String(objKey),
			UploadId:   uploadID,
			PartNumber: aws.Int32(partNumber),
			CopySource: aws.String(copySource(o.bucket, o.key(src))),
		})
		if err != nil {
			abort()
			return "", fmt.Errorf("data: compose object %q: %w", dstKey, err)
		}
		parts = append(parts, types.CompletedPart{
			ETag:       copyPartETag(copied),
			PartNumber: aws.Int32(partNumber),
		})
	}
	done, err := o.client.CompleteMultipartUpload(ctx, &s3.CompleteMultipartUploadInput{
		Bucket:          aws.String(o.bucket),
		Key:             aws.String(objKey),
		UploadId:        uploadID,
		MultipartUpload: &types.CompletedMultipartUpload{Parts: parts},
	})
	if err != nil {
		abort()
		return "", fmt.Errorf("data: compose object %q: %w", dstKey, err)
	}
	return trimETag(done.ETag), nil
}

func (o *objectStore) PresignPutObject(ctx context.Context, key string, expires time.Duration) (*biz.PresignedRequest, error) {
	if !o.Enabled() {
		return nil, biz.ErrUnavailable
	}
	// Nothing but the host is signed, and the host travels inside the URL, so
	// the client sends no extra headers.
	signed, err := o.presigner().PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(o.bucket),
		Key:    aws.String(o.key(key)),
	}, s3.WithPresignExpires(expires))
	if err != nil {
		return nil, fmt.Errorf("data: presign put %q: %w", key, err)
	}
	return &biz.PresignedRequest{
		URL:       signed.URL,
		Method:    signed.Method,
		ExpiresAt: time.Now().Add(expires),
	}, nil
}

func (o *objectStore) PresignGetObject(ctx context.Context, key, fileName, contentType, disposition string, expires time.Duration) (*biz.PresignedRequest, error) {
	if !o.Enabled() {
		return nil, biz.ErrUnavailable
	}
	in := &s3.GetObjectInput{
		Bucket: aws.String(o.bucket),
		Key:    aws.String(o.key(key)),
	}
	// These two are response header overrides: they are part of the signature
	// and answered by the object storage, not by this service.
	if fileName != "" {
		in.ResponseContentDisposition = aws.String(contentDisposition(disposition, fileName))
	}
	if contentType != "" {
		in.ResponseContentType = aws.String(contentType)
	}
	signed, err := o.presigner().PresignGetObject(ctx, in, s3.WithPresignExpires(expires))
	if err != nil {
		return nil, fmt.Errorf("data: presign get %q: %w", key, err)
	}
	return &biz.PresignedRequest{
		URL:       signed.URL,
		Method:    signed.Method,
		ExpiresAt: time.Now().Add(expires),
	}, nil
}

func (o *objectStore) HealthCheck(ctx context.Context) error {
	if !o.Enabled() {
		return nil
	}
	if _, err := o.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(o.bucket)}); err != nil {
		if isNotFound(err) {
			return fmt.Errorf("data: bucket %q does not exist", o.bucket)
		}
		return err
	}
	return nil
}

// presigner returns the client whose host the presigned URL must be valid for.
func (o *objectStore) presigner() *s3.PresignClient {
	if o.publicPresign != nil {
		return o.publicPresign
	}
	return o.presign
}

// PublicHost reports the host presigned URLs are valid for once the operator
// declared a browser facing address. An empty value means they are signed for
// the endpoint the server itself uses, which a browser on another machine
// cannot reach.
func (o *objectStore) PublicHost() string { return o.publicHost }

// contentTypeOrNil keeps an empty content type out of the request so the
// backend can apply its own default.
func contentTypeOrNil(contentType string) *string {
	if contentType == "" {
		return nil
	}
	return aws.String(contentType)
}

// copySource builds the x-amz-copy-source value. Every key segment is escaped
// separately so that the slashes that separate the bucket from the key and the
// key from its own segments survive.
func copySource(bucket, key string) string {
	segments := strings.Split(key, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return bucket + "/" + strings.Join(segments, "/")
}

// copyPartETag reads the etag uploaded parts are completed with.
func copyPartETag(out *s3.UploadPartCopyOutput) *string {
	if out == nil || out.CopyPartResult == nil {
		return nil
	}
	return out.CopyPartResult.ETag
}

// trimETag drops the quotes S3 wraps around etags. The stored etag is quoted
// again where it is sent back as a header.
func trimETag(etag *string) string {
	return strings.Trim(aws.ToString(etag), `"`)
}

// timeOrZero dereferences an optional timestamp.
func timeOrZero(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// isNotFound reports whether an error means "this object or bucket is absent".
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var noSuchKey *types.NoSuchKey
	var noSuchBucket *types.NoSuchBucket
	var notFound *types.NotFound
	if errors.As(err, &noSuchKey) || errors.As(err, &noSuchBucket) || errors.As(err, &notFound) {
		return true
	}
	var responseErr *smithyhttp.ResponseError
	if errors.As(err, &responseErr) && responseErr.HTTPStatusCode() == http.StatusNotFound {
		return true
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return isNotFoundCode(apiErr.ErrorCode())
	}
	return false
}

// isNotFoundCode covers the spellings backends use for a missing key.
func isNotFoundCode(code string) bool {
	switch code {
	case "NoSuchKey", "NoSuchBucket", "NotFound", "404":
		return true
	default:
		return false
	}
}

// insecureTLS builds a TLS configuration that skips certificate verification.
// It exists for local deployments using a self signed certificate.
func insecureTLS() *tls.Config {
	return &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in development setting
}

// contentDisposition builds an RFC 6266 header value with a UTF-8 file name.
func contentDisposition(disposition, fileName string) string {
	if disposition == "" {
		disposition = "attachment"
	}
	ascii := strings.Map(func(r rune) rune {
		if r < 32 || r > 126 || r == '"' || r == '\\' {
			return '_'
		}
		return r
	}, fileName)
	return fmt.Sprintf(`%s; filename="%s"; filename*=UTF-8''%s`, disposition, ascii, url.PathEscape(fileName))
}
