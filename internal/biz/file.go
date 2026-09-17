package biz

import (
	"bytes"
	"context"
	"io"
	"mime"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ObjectInfo describes a stored object.
type ObjectInfo struct {
	Key          string
	Size         int64
	Etag         string
	ContentType  string
	LastModified time.Time
}

// PresignedRequest is a time limited request handed to a client so it can talk
// to object storage directly.
type PresignedRequest struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
}

// ObjectStore is the blob backend behind the netdisk. Implementations must be
// safe for concurrent use.
type ObjectStore interface {
	PutObject(ctx context.Context, key string, r io.Reader, size int64, contentType string, meta map[string]string) (string, error)
	GetObject(ctx context.Context, key string, offset, length int64) (io.ReadCloser, *ObjectInfo, error)
	StatObject(ctx context.Context, key string) (*ObjectInfo, error)
	RemoveObject(ctx context.Context, key string) error
	RemovePrefix(ctx context.Context, prefix string) error
	ListPrefix(ctx context.Context, prefix string) ([]ObjectInfo, error)
	// ComposeObject assembles the source objects into one destination object.
	// Every source but the last must be at least 5 MiB, which is an S3 rule.
	ComposeObject(ctx context.Context, dstKey string, srcKeys []string, contentType string) (string, error)
	PresignPutObject(ctx context.Context, key string, expires time.Duration) (*PresignedRequest, error)
	PresignGetObject(ctx context.Context, key, fileName, contentType, disposition string, expires time.Duration) (*PresignedRequest, error)
	// PublicHost is the host presigned URLs are signed for when the operator
	// declared a browser facing address. Empty means they are signed for the
	// endpoint the server itself uses, so a client on another machine cannot
	// redeem them.
	PublicHost() string
	HealthCheck(ctx context.Context) error
}

// Upload is one multipart upload session.
type Upload struct {
	ID            uuid.UUID
	ParentID      uuid.UUID
	Name          string
	OwnerID       uuid.UUID
	Size          int64
	MimeType      string
	ChunkSize     int64
	TotalParts    int32
	Status        UploadStatus
	Mode          UploadMode
	Policy        ConflictPolicy
	Path          string
	Etag          string
	NodeID        *uuid.UUID
	Description   string
	Metadata      map[string]string
	ReceivedBytes int64
	LastError     string
	ExpiresAt     time.Time
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// PartKey returns the storage key of one part of the session.
func (u *Upload) PartKey(partNumber int32) string {
	return u.Path + "/parts/" + itoa(partNumber)
}

// UploadPart is one stored part.
type UploadPart struct {
	ID         uuid.UUID
	UploadID   uuid.UUID
	PartNumber int32
	Size       int64
	Etag       string
	StorageKey string
	CreatedAt  time.Time
	// Presign is filled in transiently for presigned sessions so the service
	// layer can hand the client a direct upload URL.
	Presign *PresignedRequest
}

// NodeVersion is one retained content revision of a file node.
type NodeVersion struct {
	ID         uuid.UUID
	NodeID     uuid.UUID
	Version    int32
	StorageKey string
	Size       int64
	MimeType   string
	Etag       string
	Comment    string
	CreatedBy  uuid.UUID
	CreatedAt  time.Time
}

// PendingDeletion is a queued object deletion. Object storage is released
// after the database transaction commits and retried on failure, which keeps a
// crash from leaking or, worse, orphaning a live object.
type PendingDeletion struct {
	ID        uuid.UUID
	Key       string
	Reason    string
	Attempts  int32
	LastError string
	CreatedAt time.Time
}

// FileRepo persists uploads, parts, versions and the deletion queue.
type FileRepo interface {
	CreateUpload(context.Context, *Upload) (*Upload, error)
	FindUploadByID(context.Context, uuid.UUID) (*Upload, error)
	FindReusableUpload(context.Context, uuid.UUID, uuid.UUID, string) (*Upload, error)
	ListUploads(context.Context, *uuid.UUID, *UploadStatus, ...ListOption) ([]*Upload, error)
	CountUploads(context.Context, *uuid.UUID, *UploadStatus) (int64, error)
	UpdateUpload(context.Context, *Upload) (*Upload, error)
	DeleteUpload(context.Context, uuid.UUID) error
	ListExpiredUploads(context.Context, time.Time, int) ([]*Upload, error)

	PutUploadPart(context.Context, *UploadPart) (*UploadPart, error)
	ListUploadParts(context.Context, uuid.UUID, int32) ([]*UploadPart, error)
	DeleteUploadParts(context.Context, uuid.UUID) error

	CreateVersion(context.Context, *NodeVersion) (*NodeVersion, error)
	ListVersions(context.Context, uuid.UUID, ...ListOption) ([]*NodeVersion, error)
	FindVersionByID(context.Context, uuid.UUID) (*NodeVersion, error)
	DeleteVersion(context.Context, uuid.UUID) error
	CountVersions(context.Context, uuid.UUID) (int64, error)
	NextVersionNumber(context.Context, uuid.UUID) (int32, error)

	EnqueueDeletion(context.Context, string, string) error
	ListPendingDeletions(context.Context, int) ([]*PendingDeletion, error)
	DeletePendingDeletion(context.Context, uuid.UUID) error
	MarkDeletionAttempt(context.Context, uuid.UUID, string) error
}

// UploadLimits bounds the upload configuration.
type UploadLimits struct {
	DefaultChunkSize int64
	MinChunkSize     int64
	MaxChunkSize     int64
	MaxFileSize      int64
	MaxInlineSize    int64
	MaxParts         int32
	SessionTTL       time.Duration
	PresignTTL       time.Duration
	DefaultMode      UploadMode
	VerifyChecksum   bool
	KeepVersions     bool
	MaxVersions      int32
}

// FileUsecase implements chunked upload and content transfer.
type FileUsecase struct {
	nodes   *NodeUsecase
	repo    FileRepo
	tx      TxManager
	store   ObjectStore
	signer  URLSigner
	limit   UploadLimits
	baseURL string
}

// NewFileUsecase returns a file usecase.
func NewFileUsecase(nodes *NodeUsecase, repo FileRepo, tx TxManager, store ObjectStore, signer URLSigner, limit UploadLimits) *FileUsecase {
	if limit.DefaultChunkSize <= 0 {
		limit.DefaultChunkSize = 8 << 20
	}
	if limit.MinChunkSize <= 0 {
		limit.MinChunkSize = 5 << 20
	}
	if limit.MaxChunkSize <= 0 {
		limit.MaxChunkSize = 5 << 30
	}
	if limit.MaxParts <= 0 {
		limit.MaxParts = 10000
	}
	if limit.MaxInlineSize <= 0 {
		limit.MaxInlineSize = 4 << 20
	}
	if limit.SessionTTL <= 0 {
		limit.SessionTTL = 24 * time.Hour
	}
	if limit.PresignTTL <= 0 {
		limit.PresignTTL = 30 * time.Minute
	}
	if limit.DefaultMode == UploadModeUnspecified {
		limit.DefaultMode = UploadModePresigned
	}
	base := ""
	if signer != nil {
		base = signer.Base()
	}
	return &FileUsecase{nodes: nodes, repo: repo, tx: tx, store: store, signer: signer, limit: limit, baseURL: base}
}

// Limits exposes the effective upload configuration: the values in force after
// the defaults below were applied, which is what a client should be told about.
func (uc *FileUsecase) Limits() UploadLimits { return uc.limit }

// Store exposes the object store so callers can stream content.
func (uc *FileUsecase) Store() ObjectStore { return uc.store }

// NodeByID loads a node without an authorisation check. It exists for the
// streaming endpoints, whose caller is authorised by the URL signature rather
// than by an account.
func (uc *FileUsecase) NodeByID(ctx context.Context, id uuid.UUID) (*Node, error) {
	node, err := uc.nodes.repo.FindNodeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if node.Status == NodeStatusTrashed {
		return nil, ErrNotFound
	}
	return node, nil
}

// ListSubtree returns every descendant of a folder, root excluded.
func (uc *FileUsecase) ListSubtree(ctx context.Context, id uuid.UUID) ([]*Node, error) {
	root, err := uc.nodes.repo.FindNodeByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return uc.nodes.repo.ListNodes(ctx, NodeQuery{
		PathPrefix: root.Path,
		ExcludeIDs: []uuid.UUID{root.ID},
	}, ListLimit(200000))
}

// InitiateUploadInput describes a new multipart upload.
type InitiateUploadInput struct {
	ParentID    uuid.UUID
	Name        string
	Size        int64
	MimeType    string
	ChunkSize   int64
	Mode        UploadMode
	Policy      ConflictPolicy
	Etag        string
	Description string
	Metadata    map[string]string
	ResumeID    uuid.UUID
}

// InitiateUpload opens a multipart upload session and returns it with a
// presigned part URL for every part when the presigned transport is used.
//
// The quota and the destination name are checked here as well as at
// completion, so a client learns about a problem before it transfers anything.
func (uc *FileUsecase) InitiateUpload(ctx context.Context, in InitiateUploadInput) (*Upload, []*UploadPart, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, nil, ErrUnauthenticated
	}
	if err := caller.Require(PermUpload); err != nil {
		return nil, nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || strings.ContainsAny(name, "/\\") || len(name) > uc.nodes.MaxNameLength() {
		return nil, nil, ErrInvalidArgument
	}
	if in.Size < 0 {
		return nil, nil, ErrInvalidArgument
	}
	if uc.limit.MaxFileSize > 0 && in.Size > uc.limit.MaxFileSize {
		return nil, nil, ErrTooLarge
	}
	chunk := in.ChunkSize
	if chunk <= 0 {
		chunk = uc.limit.DefaultChunkSize
	}
	chunk = normalizeChunk(chunk, uc.limit)
	if in.Size == 0 {
		chunk = uc.limit.MinChunkSize
	}
	totalParts := int32(1)
	if in.Size > 0 {
		totalParts = int32((in.Size + chunk - 1) / chunk)
	}
	if totalParts > uc.limit.MaxParts {
		return nil, nil, ErrTooLarge
	}
	mode := in.Mode
	if mode == UploadModeUnspecified {
		mode = uc.limit.DefaultMode
	}
	policy := in.Policy
	if policy == ConflictPolicyUnspecified {
		policy = ConflictPolicyRename
	}
	mimeType := in.MimeType
	if mimeType == "" {
		mimeType = DetectMimeType(name)
	}
	if err := uc.checkQuota(ctx, caller, in.Size); err != nil {
		return nil, nil, err
	}

	var upload *Upload
	var parts []*UploadPart
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		parent, err := uc.nodes.loadWritableParent(ctx, caller, in.ParentID)
		if err != nil {
			return err
		}
		if in.ResumeID != uuid.Nil {
			existing, err := uc.repo.FindUploadByID(ctx, in.ResumeID)
			if err == nil && existing.OwnerID == caller.UserID && existing.Status != UploadStatusCompleted {
				if time.Now().Before(existing.ExpiresAt) {
					existing.Size = in.Size
					existing.ChunkSize = chunk
					existing.TotalParts = totalParts
					existing.Mode = mode
					if existing.Status == UploadStatusExpired || existing.Status == UploadStatusAborted {
						existing.Status = UploadStatusPending
					}
					if _, err := uc.repo.UpdateUpload(ctx, existing); err != nil {
						return err
					}
					upload = existing
					parts, err = uc.repo.ListUploadParts(ctx, existing.ID, 0)
					return err
				}
			}
		}
		if existing, err := uc.repo.FindReusableUpload(ctx, caller.UserID, in.ParentID, name); err == nil && existing != nil {
			existing.Size = in.Size
			existing.ChunkSize = chunk
			existing.TotalParts = totalParts
			existing.Mode = mode
			existing.ExpiresAt = time.Now().Add(uc.limit.SessionTTL)
			if _, err := uc.repo.UpdateUpload(ctx, existing); err != nil {
				return err
			}
			upload = existing
			parts, err = uc.repo.ListUploadParts(ctx, existing.ID, 0)
			return err
		}
		id := NewID()
		_ = parent
		created := &Upload{
			ID:          id,
			ParentID:    in.ParentID,
			Name:        name,
			OwnerID:     caller.UserID,
			Size:        in.Size,
			MimeType:    mimeType,
			ChunkSize:   chunk,
			TotalParts:  totalParts,
			Status:      UploadStatusPending,
			Mode:        mode,
			Policy:      policy,
			Path:        "uploads/" + id.String(),
			Etag:        in.Etag,
			Description: in.Description,
			Metadata:    in.Metadata,
			ExpiresAt:   time.Now().Add(uc.limit.SessionTTL),
		}
		saved, err := uc.repo.CreateUpload(ctx, created)
		if err != nil {
			return err
		}
		upload = saved
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	if upload.Mode == UploadModePresigned {
		filled, err := uc.signParts(ctx, upload, parts)
		if err != nil {
			return nil, nil, err
		}
		parts = filled
	}
	return upload, parts, nil
}

// signParts attaches a presigned PUT URL to every part of a session, keeping
// the progress already recorded for the parts that were uploaded.
func (uc *FileUsecase) signParts(ctx context.Context, upload *Upload, existing []*UploadPart) ([]*UploadPart, error) {
	byNumber := map[int32]*UploadPart{}
	for _, p := range existing {
		byNumber[p.PartNumber] = p
	}
	out := make([]*UploadPart, 0, upload.TotalParts)
	for n := int32(1); n <= upload.TotalParts; n++ {
		part := &UploadPart{UploadID: upload.ID, PartNumber: n, StorageKey: upload.PartKey(n)}
		if done, ok := byNumber[n]; ok {
			part.ID = done.ID
			part.Size = done.Size
			part.Etag = done.Etag
			part.CreatedAt = done.CreatedAt
		}
		if part.Etag == "" {
			req, err := uc.store.PresignPutObject(ctx, part.StorageKey, uc.limit.PresignTTL)
			if err != nil {
				return nil, err
			}
			// Presigned PUT must not carry a content type header or S3 will
			// reject the signature, so only the method is advertised.
			part.Presign = req
		}
		out = append(out, part)
	}
	return out, nil
}

// UploadChunkInput carries one part sent through the server.
type UploadChunkInput struct {
	UploadID   uuid.UUID
	PartNumber int32
	Size       int64
	Reader     io.Reader
	Etag       string
}

// UploadChunk stores one part through the server and records it.
func (uc *FileUsecase) UploadChunk(ctx context.Context, in UploadChunkInput) (*UploadPart, *Upload, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, nil, ErrUnauthenticated
	}
	upload, err := uc.loadOwnedUpload(ctx, caller, in.UploadID)
	if err != nil {
		return nil, nil, err
	}
	if err := uc.assertSessionOpen(upload); err != nil {
		return nil, nil, err
	}
	if in.PartNumber < 1 || in.PartNumber > upload.TotalParts {
		return nil, nil, ErrInvalidArgument
	}
	maxSize := upload.ChunkSize
	if in.PartNumber == upload.TotalParts && upload.Size > 0 {
		maxSize = upload.Size - int64(upload.TotalParts-1)*upload.ChunkSize
	}
	if in.Size > maxSize {
		return nil, nil, ErrTooLarge
	}
	key := upload.PartKey(in.PartNumber)
	etag, err := uc.store.PutObject(ctx, key, in.Reader, in.Size, "application/octet-stream", map[string]string{
		"upload-id":   upload.ID.String(),
		"part-number": itoa(in.PartNumber),
	})
	if err != nil {
		return nil, nil, err
	}
	if in.Etag != "" && etag != "" && !strings.EqualFold(strings.Trim(in.Etag, `"`), strings.Trim(etag, `"`)) {
		_ = uc.store.RemoveObject(ctx, key)
		return nil, nil, ErrPreconditionFailed
	}
	part := &UploadPart{
		ID:         NewID(),
		UploadID:   upload.ID,
		PartNumber: in.PartNumber,
		Size:       in.Size,
		Etag:       etag,
		StorageKey: key,
	}
	var saved *UploadPart
	var updated *Upload
	err = uc.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		saved, err = uc.repo.PutUploadPart(ctx, part)
		if err != nil {
			return err
		}
		updated, err = uc.refreshProgress(ctx, upload, 0)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return saved, updated, nil
}

// ConfirmUploadPart records a part that was uploaded straight to object
// storage. The object is inspected rather than trusted, so a client cannot
// claim a part it never sent.
func (uc *FileUsecase) ConfirmUploadPart(ctx context.Context, uploadID uuid.UUID, partNumber int32, etag string) (*UploadPart, *Upload, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, nil, ErrUnauthenticated
	}
	upload, err := uc.loadOwnedUpload(ctx, caller, uploadID)
	if err != nil {
		return nil, nil, err
	}
	if err := uc.assertSessionOpen(upload); err != nil {
		return nil, nil, err
	}
	if partNumber < 1 || partNumber > upload.TotalParts {
		return nil, nil, ErrInvalidArgument
	}
	key := upload.PartKey(partNumber)
	info, err := uc.store.StatObject(ctx, key)
	if err != nil {
		return nil, nil, ErrUploadIncomplete
	}
	// An empty part is only meaningful for an empty file, where the session
	// expects exactly one zero byte part. Anything else means the client
	// claimed a part it never sent.
	if info.Size < 0 || (info.Size == 0 && !(upload.Size == 0 && upload.TotalParts == 1 && partNumber == 1)) {
		return nil, nil, ErrUploadIncomplete
	}
	if etag != "" && info.Etag != "" && !strings.EqualFold(strings.Trim(etag, `"`), strings.Trim(info.Etag, `"`)) {
		return nil, nil, ErrPreconditionFailed
	}
	part := &UploadPart{
		ID:         NewID(),
		UploadID:   upload.ID,
		PartNumber: partNumber,
		Size:       info.Size,
		Etag:       info.Etag,
		StorageKey: key,
	}
	var saved *UploadPart
	var updated *Upload
	err = uc.tx.WithTx(ctx, func(ctx context.Context) error {
		var err error
		saved, err = uc.repo.PutUploadPart(ctx, part)
		if err != nil {
			return err
		}
		updated, err = uc.refreshProgress(ctx, upload, 0)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	return saved, updated, nil
}

// refreshProgress recomputes the received byte count of a session.
func (uc *FileUsecase) refreshProgress(ctx context.Context, upload *Upload, _ int64) (*Upload, error) {
	parts, err := uc.repo.ListUploadParts(ctx, upload.ID, 0)
	if err != nil {
		return nil, err
	}
	var bytes int64
	for _, p := range parts {
		bytes += p.Size
	}
	upload.ReceivedBytes = bytes
	if len(parts) > 0 && upload.Status == UploadStatusPending {
		upload.Status = UploadStatusInProgress
	}
	return uc.repo.UpdateUpload(ctx, upload)
}

// CompleteUpload assembles the parts, commits the node and releases the staged
// objects.
//
// The order matters: the object is assembled first, then a single database
// transaction creates the node, retires the session and updates the counters.
// If that transaction fails the assembled object is removed again, so the tree
// can never reference content that does not exist, and content can never
// outlive its row.
func (uc *FileUsecase) CompleteUpload(ctx context.Context, uploadID uuid.UUID, checksum, comment string) (*Node, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	upload, err := uc.loadOwnedUpload(ctx, caller, uploadID)
	if err != nil {
		return nil, err
	}
	if err := uc.assertSessionOpen(upload); err != nil {
		return nil, err
	}
	parts, err := uc.repo.ListUploadParts(ctx, upload.ID, 0)
	if err != nil {
		return nil, err
	}
	if int32(len(parts)) != upload.TotalParts {
		return nil, ErrUploadIncomplete
	}
	var total int64
	keys := make([]string, 0, len(parts))
	for i := int32(1); i <= upload.TotalParts; i++ {
		found := false
		for _, p := range parts {
			if p.PartNumber != i {
				continue
			}
			found = true
			total += p.Size
			keys = append(keys, p.StorageKey)
			if i < upload.TotalParts && p.Size < uc.limit.MinChunkSize {
				return nil, ErrUploadIncomplete
			}
		}
		if !found {
			return nil, ErrUploadIncomplete
		}
	}
	if upload.Size > 0 && total != upload.Size {
		return nil, ErrUploadIncomplete
	}
	nodeID := NewID()
	finalKey := ObjectKey(caller.UserID, nodeID, 0)
	etag, err := uc.store.ComposeObject(ctx, finalKey, keys, upload.MimeType)
	if err != nil {
		return nil, err
	}
	if checksum == "" {
		checksum = upload.Etag
	}
	if uc.limit.VerifyChecksum && checksum != "" && etag != "" && strings.HasPrefix(checksum, "sha256:") {
		if !strings.EqualFold(strings.TrimPrefix(checksum, "sha256:"), strings.TrimPrefix(etag, "sha256:")) {
			_ = uc.store.RemoveObject(ctx, finalKey)
			return nil, ErrPreconditionFailed
		}
	}
	var node *Node
	err = uc.tx.WithTx(ctx, func(ctx context.Context) error {
		created, err := uc.commitNode(ctx, caller, upload, nodeID, finalKey, total, etag, comment)
		if err != nil {
			return err
		}
		node = created
		return nil
	})
	if err != nil {
		_ = uc.store.RemoveObject(ctx, finalKey)
		return nil, err
	}
	// The staged parts and the session row are released after the commit; a
	// failure here is queued for the maintenance job instead of failing the
	// upload the client already owns.
	if err := uc.repo.DeleteUploadParts(ctx, upload.ID); err != nil {
		_ = uc.repo.EnqueueDeletion(ctx, upload.Path, "upload_parts")
	} else if err := uc.store.RemovePrefix(ctx, upload.Path); err != nil {
		_ = uc.repo.EnqueueDeletion(ctx, upload.Path, "upload_parts")
	}
	if err := uc.repo.DeleteUpload(ctx, upload.ID); err != nil {
		return nil, err
	}
	return node, nil
}

// commitNode creates or replaces the destination node inside the caller's
// transaction.
func (uc *FileUsecase) commitNode(ctx context.Context, caller *Caller, upload *Upload, nodeID uuid.UUID, finalKey string, size int64, etag, comment string) (*Node, error) {
	parent, err := uc.nodes.loadWritableParent(ctx, caller, upload.ParentID)
	if err != nil {
		return nil, err
	}
	if parent.Depth+1 >= uc.nodes.MaxDepth() {
		return nil, ErrInvalidArgument
	}

	// An existing sibling may be replaced when the session asked for it.
	var existing *Node
	siblings, err := uc.nodes.repo.ListNodes(ctx, NodeQuery{
		ParentID: &upload.ParentID, Names: []string{upload.Name},
	}, ListLimit(1))
	if err != nil {
		return nil, err
	}
	if len(siblings) > 0 {
		existing = siblings[0]
	}
	if existing != nil && upload.Policy != ConflictPolicyOverwrite {
		if upload.Policy == ConflictPolicyFail {
			return nil, ErrNameConflict
		}
		name, err := uc.nodes.repo.NextAvailableName(ctx, upload.ParentID, upload.Name)
		if err != nil {
			return nil, err
		}
		upload.Name = name
		existing = nil
	}
	if existing != nil {
		if existing.Kind != NodeKindFile {
			return nil, ErrNameConflict
		}
		access, err := uc.nodes.Evaluate(ctx, caller, existing)
		if err != nil {
			return nil, err
		}
		if !access.Perms.Has(PermEdit) {
			return nil, ErrPermissionDenied
		}
		// The quota belongs to whoever owns the content, which is not
		// necessarily the caller when a manager replaces somebody else's file.
		if err := uc.checkQuotaFor(ctx, existing.OwnerID, size-existing.Size); err != nil {
			return nil, err
		}
		version := int32(1)
		if uc.limit.KeepVersions {
			version, err = uc.repo.NextVersionNumber(ctx, existing.ID)
			if err != nil {
				return nil, err
			}
			if _, err := uc.repo.CreateVersion(ctx, &NodeVersion{
				ID:         NewID(),
				NodeID:     existing.ID,
				Version:    version,
				StorageKey: existing.StorageKey,
				Size:       existing.Size,
				MimeType:   existing.MimeType,
				Etag:       existing.Etag,
				Comment:    comment,
				CreatedBy:  caller.UserID,
			}); err != nil {
				return nil, err
			}
			existing.VersionCount++
			if uc.limit.MaxVersions > 0 && existing.VersionCount > uc.limit.MaxVersions {
				if err := uc.trimVersions(ctx, existing, uc.limit.MaxVersions); err != nil {
					return nil, err
				}
			}
		}
		existing.StorageKey = finalKey
		existing.Size = size
		existing.Etag = etag
		existing.MimeType = upload.MimeType
		existing.Extension = Extension(upload.Name)
		existing.CurrentVersionID = nil
		existing.UpdatedBy = caller.UserID
		saved, err := uc.nodes.repo.UpdateNode(ctx, existing)
		if err != nil {
			return nil, err
		}
		if err := uc.users().AddUsage(ctx, existing.OwnerID, size-existing.Size, 0, 0); err != nil {
			return nil, err
		}
		return saved, nil
	}

	if err := uc.checkQuota(ctx, caller, size); err != nil {
		return nil, err
	}
	node := &Node{
		ID:          nodeID,
		ParentID:    upload.ParentID,
		Name:        upload.Name,
		NameLower:   strings.ToLower(upload.Name),
		Kind:        NodeKindFile,
		OwnerID:     caller.UserID,
		Size:        size,
		MimeType:    upload.MimeType,
		Extension:   Extension(upload.Name),
		Etag:        etag,
		StorageKey:  finalKey,
		Status:      NodeStatusActive,
		Description: upload.Description,
		Visibility:  VisibilityPrivate,
		Metadata:    upload.Metadata,
		Path:        ChildPath(parent.Path, nodeID),
		Depth:       parent.Depth + 1,
		CreatedBy:   caller.UserID,
		UpdatedBy:   caller.UserID,
	}
	if parent.ID != uuid.Nil {
		node.Visibility = parent.Visibility
	}
	saved, err := uc.nodes.repo.CreateNode(ctx, node)
	if err != nil {
		return nil, err
	}
	if err := uc.users().AddUsage(ctx, caller.UserID, size, 1, 0); err != nil {
		return nil, err
	}
	return saved, nil
}

// trimVersions drops the oldest versions beyond keep.
func (uc *FileUsecase) trimVersions(ctx context.Context, node *Node, keep int32) error {
	versions, err := uc.repo.ListVersions(ctx, node.ID, ListLimit(1000))
	if err != nil {
		return err
	}
	if int32(len(versions)) <= keep {
		node.VersionCount = int32(len(versions))
		return nil
	}
	for _, v := range versions[keep:] {
		if err := uc.repo.DeleteVersion(ctx, v.ID); err != nil {
			return err
		}
		if err := uc.repo.EnqueueDeletion(ctx, v.StorageKey, "version_trim"); err != nil {
			return err
		}
	}
	node.VersionCount = keep
	return nil
}

// AbortUpload cancels a session and releases the staged parts.
func (uc *FileUsecase) AbortUpload(ctx context.Context, uploadID uuid.UUID) error {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return ErrUnauthenticated
	}
	upload, err := uc.loadOwnedUpload(ctx, caller, uploadID)
	if err != nil {
		return err
	}
	if upload.Status == UploadStatusCompleted {
		return ErrInvalidArgument
	}
	err = uc.tx.WithTx(ctx, func(ctx context.Context) error {
		upload.Status = UploadStatusAborted
		if _, err := uc.repo.UpdateUpload(ctx, upload); err != nil {
			return err
		}
		return uc.repo.DeleteUploadParts(ctx, upload.ID)
	})
	if err != nil {
		return err
	}
	if err := uc.store.RemovePrefix(ctx, upload.Path); err != nil {
		return uc.repo.EnqueueDeletion(ctx, upload.Path, "upload_abort")
	}
	return nil
}

// UploadSmallFile stores a payload carried inside the request.
func (uc *FileUsecase) UploadSmallFile(ctx context.Context, in UploadSmallFileInput) (*Node, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	if err := caller.Require(PermUpload); err != nil {
		return nil, err
	}
	if int64(len(in.Content)) > uc.limit.MaxInlineSize {
		return nil, ErrTooLarge
	}
	name := strings.TrimSpace(in.Name)
	if name == "" || strings.ContainsAny(name, "/\\") || len(name) > uc.nodes.MaxNameLength() {
		return nil, ErrInvalidArgument
	}
	mimeType := in.MimeType
	if mimeType == "" {
		mimeType = DetectMimeType(name)
	}
	nodeID := NewID()
	key := ObjectKey(caller.UserID, nodeID, 0)
	etag, err := uc.store.PutObject(ctx, key, bytes.NewReader(in.Content), int64(len(in.Content)), mimeType, nil)
	if err != nil {
		return nil, err
	}
	upload := &Upload{
		ID:          NewID(),
		ParentID:    in.ParentID,
		Name:        name,
		OwnerID:     caller.UserID,
		Size:        int64(len(in.Content)),
		MimeType:    mimeType,
		Policy:      in.Policy,
		Description: in.Description,
		Metadata:    in.Metadata,
		Status:      UploadStatusInProgress,
	}
	if upload.Policy == ConflictPolicyUnspecified {
		upload.Policy = ConflictPolicyRename
	}
	var node *Node
	err = uc.tx.WithTx(ctx, func(ctx context.Context) error {
		created, err := uc.commitNode(ctx, caller, upload, nodeID, key, upload.Size, etag, "inline upload")
		if err != nil {
			return err
		}
		node = created
		return nil
	})
	if err != nil {
		_ = uc.store.RemoveObject(ctx, key)
		return nil, err
	}
	return node, nil
}

// UploadSmallFileInput describes an inline upload.
type UploadSmallFileInput struct {
	ParentID    uuid.UUID
	Name        string
	Content     []byte
	MimeType    string
	Policy      ConflictPolicy
	Description string
	Metadata    map[string]string
}

// GetUpload returns one session the caller owns.
func (uc *FileUsecase) GetUpload(ctx context.Context, id uuid.UUID) (*Upload, []*UploadPart, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, nil, ErrUnauthenticated
	}
	upload, err := uc.loadOwnedUpload(ctx, caller, id)
	if err != nil {
		return nil, nil, err
	}
	parts, err := uc.repo.ListUploadParts(ctx, id, 0)
	if err != nil {
		return nil, nil, err
	}
	if upload.Mode == UploadModePresigned && upload.Status != UploadStatusCompleted {
		parts, err = uc.signParts(ctx, upload, parts)
		if err != nil {
			return nil, nil, err
		}
	}
	return upload, parts, nil
}

// ListUploads returns the caller's sessions.
func (uc *FileUsecase) ListUploads(ctx context.Context, status UploadStatus, parentID uuid.UUID, opts ...ListOption) ([]*Upload, int64, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, 0, ErrUnauthenticated
	}
	var statusPtr *UploadStatus
	if status != UploadStatusUnspecified {
		statusPtr = &status
	}
	owner := caller.UserID
	uploads, err := uc.repo.ListUploads(ctx, &owner, statusPtr, opts...)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.repo.CountUploads(ctx, &owner, statusPtr)
	if err != nil {
		return nil, 0, err
	}
	if parentID != uuid.Nil {
		filtered := make([]*Upload, 0, len(uploads))
		for _, u := range uploads {
			if u.ParentID == parentID {
				filtered = append(filtered, u)
			}
		}
		uploads = filtered
	}
	return uploads, total, nil
}

// ListUploadParts returns the parts of a session.
func (uc *FileUsecase) ListUploadParts(ctx context.Context, id uuid.UUID, from int32) ([]*UploadPart, *Upload, error) {
	upload, parts, err := uc.GetUpload(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	if from <= 0 {
		return parts, upload, nil
	}
	filtered := make([]*UploadPart, 0, len(parts))
	for _, p := range parts {
		if p.PartNumber > from {
			filtered = append(filtered, p)
		}
	}
	return filtered, upload, nil
}

// DownloadURL returns a signed URL for the original bytes of a file.
func (uc *FileUsecase) DownloadURL(ctx context.Context, nodeID uuid.UUID, expiresIn int32, inline bool, fileName string) (*SignedURL, error) {
	caller := CallerFromContext(ctx)
	node, access, err := uc.nodes.RequireAccess(ctx, caller, nodeID, PermDownload)
	if err != nil {
		return nil, err
	}
	if node.Kind != NodeKindFile {
		return nil, ErrUnsupported
	}
	if node.StorageKey == "" {
		return nil, ErrUnsupported
	}
	if fileName == "" {
		fileName = node.Name
	}
	_ = access
	if !browserCanReachStorage(uc.store) {
		return streamedContentURL(uc.signer, node, caller.UserID.String(), uc.presignTTL(expiresIn), inline, fileName)
	}
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	req, err := uc.store.PresignGetObject(ctx, node.StorageKey, fileName, node.MimeType, disposition, uc.presignTTL(expiresIn))
	if err != nil {
		return nil, err
	}
	return &SignedURL{
		URL:       req.URL,
		Method:    req.Method,
		Headers:   req.Headers,
		ExpiresAt: req.ExpiresAt,
		NodeID:    node.ID,
		Size:      node.Size,
		FileName:  fileName,
		MimeType:  node.MimeType,
	}, nil
}

// PreviewURL returns a signed URL for an inline preview of a file.
func (uc *FileUsecase) PreviewURL(ctx context.Context, nodeID uuid.UUID, expiresIn int32) (*SignedURL, error) {
	caller := CallerFromContext(ctx)
	node, _, err := uc.nodes.RequireAccess(ctx, caller, nodeID, PermDownload)
	if err != nil {
		return nil, err
	}
	if node.Kind != NodeKindFile || !IsPreviewable(node.MimeType) {
		return nil, ErrUnsupported
	}
	if !browserCanReachStorage(uc.store) {
		return streamedContentURL(uc.signer, node, caller.UserID.String(), uc.presignTTL(expiresIn), true, node.Name)
	}
	req, err := uc.store.PresignGetObject(ctx, node.StorageKey, node.Name, node.MimeType, "inline", uc.presignTTL(expiresIn))
	if err != nil {
		return nil, err
	}
	return &SignedURL{
		URL:       req.URL,
		Method:    req.Method,
		Headers:   req.Headers,
		ExpiresAt: req.ExpiresAt,
		NodeID:    node.ID,
		Size:      node.Size,
		FileName:  node.Name,
		MimeType:  node.MimeType,
	}, nil
}

// SignedURL is a resolved signed request returned to clients.
type SignedURL struct {
	URL       string
	Method    string
	Headers   map[string]string
	ExpiresAt time.Time
	NodeID    uuid.UUID
	Size      int64
	FileName  string
	MimeType  string
}

// Versions lists the retained revisions of a file node.
func (uc *FileUsecase) Versions(ctx context.Context, nodeID uuid.UUID, opts ...ListOption) ([]*NodeVersion, int64, error) {
	caller := CallerFromContext(ctx)
	if _, _, err := uc.nodes.RequireAccess(ctx, caller, nodeID, PermView); err != nil {
		return nil, 0, err
	}
	versions, err := uc.repo.ListVersions(ctx, nodeID, opts...)
	if err != nil {
		return nil, 0, err
	}
	total, err := uc.repo.CountVersions(ctx, nodeID)
	if err != nil {
		return nil, 0, err
	}
	return versions, total, nil
}

// RestoreVersion promotes a retained revision to be the current content.
func (uc *FileUsecase) RestoreVersion(ctx context.Context, nodeID, versionID uuid.UUID) (*Node, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	var out *Node
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		node, access, err := uc.nodes.RequireAccess(ctx, caller, nodeID, PermEdit)
		if err != nil {
			return err
		}
		if node.Kind != NodeKindFile {
			return ErrUnsupported
		}
		version, err := uc.repo.FindVersionByID(ctx, versionID)
		if err != nil {
			return err
		}
		if version.NodeID != node.ID {
			return ErrInvalidArgument
		}
		next, err := uc.repo.NextVersionNumber(ctx, node.ID)
		if err != nil {
			return err
		}
		if _, err := uc.repo.CreateVersion(ctx, &NodeVersion{
			ID:         NewID(),
			NodeID:     node.ID,
			Version:    next,
			StorageKey: node.StorageKey,
			Size:       node.Size,
			MimeType:   node.MimeType,
			Etag:       node.Etag,
			Comment:    "before restore",
			CreatedBy:  caller.UserID,
		}); err != nil {
			return err
		}
		delta := version.Size - node.Size
		node.StorageKey = version.StorageKey
		node.Size = version.Size
		node.MimeType = version.MimeType
		node.Etag = version.Etag
		node.UpdatedBy = caller.UserID
		saved, err := uc.nodes.repo.UpdateNode(ctx, node)
		if err != nil {
			return err
		}
		if err := uc.users().AddUsage(ctx, node.OwnerID, delta, 0, 0); err != nil {
			return err
		}
		_ = access
		out = saved
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteVersion drops one retained revision.
func (uc *FileUsecase) DeleteVersion(ctx context.Context, nodeID, versionID uuid.UUID) error {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return ErrUnauthenticated
	}
	var key string
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		if _, _, err := uc.nodes.RequireAccess(ctx, caller, nodeID, PermDelete); err != nil {
			return err
		}
		version, err := uc.repo.FindVersionByID(ctx, versionID)
		if err != nil {
			return err
		}
		if version.NodeID != nodeID {
			return ErrInvalidArgument
		}
		node, err := uc.nodes.repo.FindNodeByID(ctx, nodeID)
		if err != nil {
			return err
		}
		if node.VersionCount > 0 {
			node.VersionCount--
			if _, err := uc.nodes.repo.UpdateNode(ctx, node); err != nil {
				return err
			}
		}
		key = version.StorageKey
		return uc.repo.DeleteVersion(ctx, versionID)
	})
	if err != nil {
		return err
	}
	if key == "" {
		return nil
	}
	return uc.repo.EnqueueDeletion(ctx, key, "version_delete")
}

// ArchiveURL returns a signed URL that streams a folder, or a single file, as
// a zip archive. The archive is produced on the fly by the server, so the URL
// is bound to the caller, the node and the expiry instead of to an object key.
func (uc *FileUsecase) ArchiveURL(ctx context.Context, nodeID uuid.UUID, expiresIn int32, archiveName string) (*SignedURL, error) {
	caller := CallerFromContext(ctx)
	node, _, err := uc.nodes.RequireAccess(ctx, caller, nodeID, PermDownload)
	if err != nil {
		return nil, err
	}
	if uc.signer == nil {
		return nil, ErrUnsupported
	}
	if archiveName == "" {
		archiveName = node.Name
	}
	ttl := uc.presignTTL(expiresIn)
	path := "/v1/files/" + node.ID.String() + "/archive"
	expires := time.Now().Add(ttl)
	signed, _, err := uc.signer.Sign("archive", "GET", path, caller.UserID.String(), node.ID.String(), archiveName, expires)
	if err != nil {
		return nil, err
	}
	return &SignedURL{
		URL:       signed,
		Method:    "GET",
		ExpiresAt: expires,
		NodeID:    node.ID,
		Size:      0,
		FileName:  archiveName + ".zip",
		MimeType:  "application/zip",
	}, nil
}

// ContentURL returns a signed URL that streams the original bytes through the
// server. It is the fallback for clients that cannot reach object storage, and
// it honours byte ranges so a download can be resumed.
func (uc *FileUsecase) ContentURL(ctx context.Context, nodeID uuid.UUID, expiresIn int32, inline bool, fileName string) (*SignedURL, error) {
	caller := CallerFromContext(ctx)
	node, _, err := uc.nodes.RequireAccess(ctx, caller, nodeID, PermDownload)
	if err != nil {
		return nil, err
	}
	if node.Kind != NodeKindFile || node.StorageKey == "" {
		return nil, ErrUnsupported
	}
	if fileName == "" {
		fileName = node.Name
	}
	return streamedContentURL(uc.signer, node, caller.UserID.String(), uc.presignTTL(expiresIn), inline, fileName)
}

// streamedContentURL mints the signed URL of the server's own streaming route.
// It stays relative unless web.public_base_url is configured, so the browser
// resolves it against the origin it actually used; that is what lets the same
// build serve localhost, a LAN address and a reverse proxy host name. subject
// is the acting account or, for a share link, the share.
func streamedContentURL(signer URLSigner, node *Node, subject string, ttl time.Duration, inline bool, fileName string) (*SignedURL, error) {
	if signer == nil {
		return nil, ErrUnsupported
	}
	extra := "attachment"
	if inline {
		extra = "inline"
	}
	path := "/v1/files/" + node.ID.String() + "/content"
	expires := time.Now().Add(ttl)
	signed, _, err := signer.Sign("content", "GET", path, subject, node.ID.String(), extra, expires)
	if err != nil {
		return nil, err
	}
	return &SignedURL{
		URL:       signed,
		Method:    "GET",
		ExpiresAt: expires,
		NodeID:    node.ID,
		Size:      node.Size,
		FileName:  fileName,
		MimeType:  node.MimeType,
	}, nil
}

// browserCanReachStorage reports whether a presigned object storage URL is
// worth handing to a browser. Storage only has a browser facing address when
// object_storage.public_endpoint is configured; without it the signed host is
// the one the server talks to (127.0.0.1 by default), so another device would
// be sent to its own loopback and refuse the connection. Downloads and previews
// go through the server's own streaming route instead.
func browserCanReachStorage(store ObjectStore) bool {
	return store.PublicHost() != ""
}

// CopyNodesInput describes a copy batch.
type CopyNodesInput struct {
	IDs            []uuid.UUID
	TargetParentID uuid.UUID
	Policy         ConflictPolicy
	NewNames       []string
}

// CopyNodes duplicates nodes, including folder subtrees, into another folder.
// The whole batch runs in one transaction and the copied objects are written
// before their rows, so a failure can only leak an object, never create a node
// that points at nothing.
func (uc *FileUsecase) CopyNodes(ctx context.Context, in CopyNodesInput) ([]Outcome, error) {
	caller := CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	if len(in.IDs) == 0 {
		return nil, ErrInvalidArgument
	}
	if len(in.NewNames) > 0 && len(in.NewNames) != len(in.IDs) {
		return nil, ErrInvalidArgument
	}
	target, err := uc.nodes.loadWritableParent(ctx, caller, in.TargetParentID)
	if err != nil {
		return nil, err
	}
	var outcomes []Outcome
	err = uc.tx.WithTx(ctx, func(ctx context.Context) error {
		outcomes = outcomes[:0]
		for i, id := range in.IDs {
			src, access, err := uc.nodes.RequireAccess(ctx, caller, id, PermView)
			if err != nil {
				return err
			}
			if !access.Perms.Has(PermDownload) {
				return ErrPermissionDenied
			}
			name := src.Name
			if len(in.NewNames) > 0 && in.NewNames[i] != "" {
				name = in.NewNames[i]
			}
			copied, err := uc.copyTree(ctx, caller, src, target, name, in.Policy)
			if err != nil {
				return err
			}
			a, err := uc.nodes.Evaluate(ctx, caller, copied)
			if err != nil {
				return err
			}
			outcomes = append(outcomes, Outcome{Node: copied, Access: a})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return outcomes, nil
}

// copyTree duplicates one node and, for a folder, its whole subtree.
func (uc *FileUsecase) copyTree(ctx context.Context, caller *Caller, src, parent *Node, name string, policy ConflictPolicy) (*Node, error) {
	resolved, err := uc.nodes.resolveName(ctx, parent.ID, name, policy, uuid.Nil)
	if err != nil {
		return nil, err
	}
	id := NewID()
	copied := &Node{
		ID:           id,
		ParentID:     parent.ID,
		Name:         resolved,
		NameLower:    strings.ToLower(resolved),
		Kind:         src.Kind,
		OwnerID:      caller.UserID,
		Size:         src.Size,
		MimeType:     src.MimeType,
		Extension:    src.Extension,
		Etag:         src.Etag,
		Status:       NodeStatusActive,
		Description:  src.Description,
		Visibility:   src.Visibility,
		PasswordHash: src.PasswordHash,
		PasswordHint: src.PasswordHint,
		Metadata:     src.Metadata,
		Path:         ChildPath(parent.Path, id),
		Depth:        parent.Depth + 1,
		CreatedBy:    caller.UserID,
		UpdatedBy:    caller.UserID,
	}
	if parent.Depth+1 >= uc.nodes.MaxDepth() {
		return nil, ErrInvalidArgument
	}
	if src.IsFile() {
		if err := uc.checkQuota(ctx, caller, src.Size); err != nil {
			return nil, err
		}
		dstKey := ObjectKey(caller.UserID, id, 0)
		if err := uc.copyObject(ctx, src.StorageKey, dstKey); err != nil {
			return nil, err
		}
		copied.StorageKey = dstKey
	}
	saved, err := uc.nodes.repo.CreateNode(ctx, copied)
	if err != nil {
		return nil, err
	}
	if src.IsFolder() {
		children, err := uc.nodes.repo.ListNodes(ctx, NodeQuery{ParentID: &src.ID})
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			childParent := saved
			if _, err := uc.copyTree(ctx, caller, child, childParent, child.Name, policy); err != nil {
				return nil, err
			}
		}
		if err := uc.users().AddUsage(ctx, caller.UserID, 0, 0, 1); err != nil {
			return nil, err
		}
	} else {
		if err := uc.users().AddUsage(ctx, caller.UserID, src.Size, 1, 0); err != nil {
			return nil, err
		}
	}
	return saved, nil
}

// copyObject duplicates one stored object, streaming through the server so the
// operation works on backends without server side copy support.
func (uc *FileUsecase) copyObject(ctx context.Context, srcKey, dstKey string) error {
	if srcKey == "" {
		return ErrUnsupported
	}
	reader, info, err := uc.store.GetObject(ctx, srcKey, 0, 0)
	if err != nil {
		return err
	}
	defer func() {
		_ = reader.Close()
	}()
	if _, err := uc.store.PutObject(ctx, dstKey, reader, info.Size, info.ContentType, nil); err != nil {
		return err
	}
	return nil
}

// loadOwnedUpload loads a session and checks that the caller owns it.
func (uc *FileUsecase) loadOwnedUpload(ctx context.Context, caller *Caller, id uuid.UUID) (*Upload, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidArgument
	}
	upload, err := uc.repo.FindUploadByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if upload.OwnerID != caller.UserID && !caller.Has(PermStorageManage) {
		return nil, ErrPermissionDenied
	}
	return upload, nil
}

// assertSessionOpen verifies that a session may still receive parts.
func (uc *FileUsecase) assertSessionOpen(upload *Upload) error {
	switch upload.Status {
	case UploadStatusAborted:
		return ErrAborted
	case UploadStatusCompleted:
		return ErrInvalidArgument
	case UploadStatusExpired:
		return ErrUploadExpired
	}
	if time.Now().After(upload.ExpiresAt) {
		return ErrUploadExpired
	}
	return nil
}

// checkQuota fails when the extra bytes would exceed the caller's quota.
func (uc *FileUsecase) checkQuota(ctx context.Context, caller *Caller, extra int64) error {
	if !caller.IsAuthenticated() {
		return nil
	}
	return uc.checkQuotaFor(ctx, caller.UserID, extra)
}

// checkQuotaFor checks the quota of a specific account, which is the one the
// content will be billed to.
func (uc *FileUsecase) checkQuotaFor(ctx context.Context, ownerID uuid.UUID, extra int64) error {
	if extra <= 0 || ownerID == uuid.Nil {
		return nil
	}
	user, err := uc.users().FindUserByID(ctx, ownerID)
	if err != nil {
		return err
	}
	if user.QuotaBytes <= 0 {
		return nil
	}
	if user.UsedBytes+extra > user.QuotaBytes {
		return ErrQuotaExceeded
	}
	return nil
}

// users returns the user repo through the node usecase, which already holds it.
func (uc *FileUsecase) users() UserRepo { return uc.nodes.users }

func (uc *FileUsecase) presignTTL(requested int32) time.Duration {
	if requested <= 0 {
		return uc.limit.PresignTTL
	}
	ttl := time.Duration(requested) * time.Second
	if ttl > uc.limit.PresignTTL*4 {
		ttl = uc.limit.PresignTTL * 4
	}
	return ttl
}

// ObjectKey returns the storage key of the current content of a node.
func ObjectKey(owner, node uuid.UUID, _ int) string {
	return "files/" + owner.String() + "/" + node.String()
}

// normalizeChunk clamps a requested part size to the configured bounds and
// rounds it up to a multiple of 256 KiB.
func normalizeChunk(chunk int64, limit UploadLimits) int64 {
	const align = 256 << 10
	if chunk < limit.MinChunkSize {
		chunk = limit.MinChunkSize
	}
	if chunk > limit.MaxChunkSize {
		chunk = limit.MaxChunkSize
	}
	if rem := chunk % align; rem != 0 {
		chunk += align - rem
	}
	return chunk
}

// Extension returns the lower-case extension of a file name without the dot.
func Extension(name string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(name)), ".")
	if len(ext) > 32 {
		return ""
	}
	return ext
}

// DetectMimeType guesses a content type from a file name.
func DetectMimeType(name string) string {
	if ext := filepath.Ext(name); ext != "" {
		if ct := mime.TypeByExtension(ext); ct != "" {
			return ct
		}
	}
	return "application/octet-stream"
}

// IsPreviewable reports whether a content type can be shown inline.
func IsPreviewable(mimeType string) bool {
	if mimeType == "" {
		return false
	}
	return strings.HasPrefix(mimeType, "image/") ||
		strings.HasPrefix(mimeType, "video/") ||
		strings.HasPrefix(mimeType, "audio/") ||
		strings.HasPrefix(mimeType, "text/") ||
		mimeType == "application/pdf"
}

// itoa renders a part number for a storage key.
func itoa(v int32) string {
	return strconv.FormatInt(int64(v), 10)
}
