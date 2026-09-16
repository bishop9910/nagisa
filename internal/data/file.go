package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"nagisa/internal/biz"
	"nagisa/internal/data/ent"
	"nagisa/internal/data/ent/nodeversion"
	"nagisa/internal/data/ent/pendingdeletion"
	"nagisa/internal/data/ent/upload"
	"nagisa/internal/data/ent/uploadpart"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/aip-go/ents"
	"github.com/google/uuid"
)

// fileRepo persists uploads, parts, versions and the deletion queue.
type fileRepo struct {
	data *Data
}

// NewFileRepo creates a new FileRepo instance.
func NewFileRepo(d *Data) biz.FileRepo { return &fileRepo{data: d} }

func toBizUpload(po *ent.Upload) *biz.Upload {
	if po == nil {
		return nil
	}
	return &biz.Upload{
		ID:            po.ID,
		ParentID:      po.ParentID,
		Name:          po.Name,
		OwnerID:       po.OwnerID,
		Size:          po.Size,
		MimeType:      po.MimeType,
		ChunkSize:     po.ChunkSize,
		TotalParts:    po.TotalParts,
		Status:        po.Status,
		Mode:          po.Mode,
		Policy:        po.Policy,
		Path:          po.Path,
		Etag:          po.Etag,
		NodeID:        po.NodeID,
		Description:   po.Description,
		Metadata:      po.Metadata,
		ReceivedBytes: po.ReceivedBytes,
		LastError:     po.LastError,
		ExpiresAt:     po.ExpiresAt,
		CreatedAt:     po.CreatedAt,
		UpdatedAt:     po.UpdatedAt,
	}
}

func toBizUploadPart(po *ent.UploadPart) *biz.UploadPart {
	if po == nil {
		return nil
	}
	return &biz.UploadPart{
		ID:         po.ID,
		UploadID:   po.UploadID,
		PartNumber: po.PartNumber,
		Size:       po.Size,
		Etag:       po.Etag,
		StorageKey: po.StorageKey,
		CreatedAt:  po.CreatedAt,
	}
}

func toBizVersion(po *ent.NodeVersion) *biz.NodeVersion {
	if po == nil {
		return nil
	}
	return &biz.NodeVersion{
		ID:         po.ID,
		NodeID:     po.NodeID,
		Version:    po.Version,
		StorageKey: po.StorageKey,
		Size:       po.Size,
		MimeType:   po.MimeType,
		Etag:       po.Etag,
		Comment:    po.Comment,
		CreatedBy:  po.CreatedBy,
		CreatedAt:  po.CreatedAt,
	}
}

func toBizPendingDeletion(po *ent.PendingDeletion) *biz.PendingDeletion {
	if po == nil {
		return nil
	}
	return &biz.PendingDeletion{
		ID:        po.ID,
		Key:       po.StorageKey,
		Reason:    po.Reason,
		Attempts:  po.Attempts,
		LastError: po.LastError,
		CreatedAt: po.CreatedAt,
	}
}

func (r *fileRepo) CreateUpload(ctx context.Context, u *biz.Upload) (*biz.Upload, error) {
	builder := r.data.Exec(ctx).Upload().Create().
		SetParentID(u.ParentID).
		SetName(u.Name).
		SetNameLower(strings.ToLower(u.Name)).
		SetOwnerID(u.OwnerID).
		SetSize(u.Size).
		SetMimeType(u.MimeType).
		SetChunkSize(u.ChunkSize).
		SetTotalParts(u.TotalParts).
		SetStatus(u.Status).
		SetMode(u.Mode).
		SetPolicy(u.Policy).
		SetPath(u.Path).
		SetEtag(u.Etag).
		SetDescription(u.Description).
		SetReceivedBytes(u.ReceivedBytes).
		SetExpiresAt(u.ExpiresAt)
	if u.ID != uuid.Nil {
		builder = builder.SetID(u.ID)
	}
	if u.Metadata != nil {
		builder = builder.SetMetadata(u.Metadata)
	}
	if u.NodeID != nil {
		builder = builder.SetNodeID(*u.NodeID)
	}
	po, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: create upload: %w", err)
	}
	return toBizUpload(po), nil
}

func (r *fileRepo) FindUploadByID(ctx context.Context, id uuid.UUID) (*biz.Upload, error) {
	po, err := r.data.Exec(ctx).Upload().Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: find upload: %w", err)
	}
	return toBizUpload(po), nil
}

// FindReusableUpload returns an open session for the same destination, which
// makes a retried InitiateUpload idempotent instead of leaking a second
// staging prefix.
func (r *fileRepo) FindReusableUpload(ctx context.Context, ownerID, parentID uuid.UUID, name string) (*biz.Upload, error) {
	po, err := r.data.Exec(ctx).Upload().Query().
		Where(
			upload.OwnerIDEQ(ownerID),
			upload.ParentIDEQ(parentID),
			upload.NameLowerEQ(strings.ToLower(name)),
			upload.StatusIn(biz.UploadStatusPending, biz.UploadStatusInProgress),
			upload.ExpiresAtGT(time.Now()),
		).
		Order(upload.ByCreatedAt()).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: find reusable upload: %w", err)
	}
	return toBizUpload(po), nil
}

func (r *fileRepo) ListUploads(ctx context.Context, ownerID *uuid.UUID, status *biz.UploadStatus, opts ...biz.ListOption) ([]*biz.Upload, error) {
	options := biz.ResolveListOptions(20, opts...)
	query := r.data.Exec(ctx).Upload().Query()
	if ownerID != nil {
		query = query.Where(upload.OwnerIDEQ(*ownerID))
	}
	if status != nil {
		query = query.Where(upload.StatusEQ(*status))
	}
	pos, err := query.
		Where(ents.ApplyFilter(options.Filter)).
		Order(ents.ApplyOrderBy(options.OrderBy), upload.ByCreatedAt()).
		Offset(options.Offset).
		Limit(options.Limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list uploads: %w", err)
	}
	out := make([]*biz.Upload, 0, len(pos))
	for _, po := range pos {
		out = append(out, toBizUpload(po))
	}
	return out, nil
}

func (r *fileRepo) CountUploads(ctx context.Context, ownerID *uuid.UUID, status *biz.UploadStatus) (int64, error) {
	query := r.data.Exec(ctx).Upload().Query()
	if ownerID != nil {
		query = query.Where(upload.OwnerIDEQ(*ownerID))
	}
	if status != nil {
		query = query.Where(upload.StatusEQ(*status))
	}
	count, err := query.Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: count uploads: %w", err)
	}
	return int64(count), nil
}

func (r *fileRepo) UpdateUpload(ctx context.Context, u *biz.Upload) (*biz.Upload, error) {
	builder := r.data.Exec(ctx).Upload().UpdateOneID(u.ID).
		SetName(u.Name).
		SetNameLower(strings.ToLower(u.Name)).
		SetSize(u.Size).
		SetMimeType(u.MimeType).
		SetChunkSize(u.ChunkSize).
		SetTotalParts(u.TotalParts).
		SetStatus(u.Status).
		SetMode(u.Mode).
		SetPolicy(u.Policy).
		SetEtag(u.Etag).
		SetDescription(u.Description).
		SetReceivedBytes(u.ReceivedBytes).
		SetLastError(u.LastError).
		SetExpiresAt(u.ExpiresAt)
	if u.Metadata != nil {
		builder = builder.SetMetadata(u.Metadata)
	}
	if u.NodeID != nil {
		builder = builder.SetNodeID(*u.NodeID)
	} else {
		builder = builder.ClearNodeID()
	}
	po, err := builder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: update upload: %w", err)
	}
	return toBizUpload(po), nil
}

func (r *fileRepo) DeleteUpload(ctx context.Context, id uuid.UUID) error {
	if err := r.data.Exec(ctx).Upload().DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("data: delete upload: %w", err)
	}
	return nil
}

func (r *fileRepo) ListExpiredUploads(ctx context.Context, now time.Time, limit int) ([]*biz.Upload, error) {
	if limit <= 0 {
		limit = 500
	}
	pos, err := r.data.Exec(ctx).Upload().Query().
		Where(
			upload.ExpiresAtLT(now),
			upload.StatusIn(biz.UploadStatusPending, biz.UploadStatusInProgress),
		).
		Order(upload.ByExpiresAt()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list expired uploads: %w", err)
	}
	out := make([]*biz.Upload, 0, len(pos))
	for _, po := range pos {
		out = append(out, toBizUpload(po))
	}
	return out, nil
}

// PutUploadPart records a part, replacing any earlier attempt with the same
// number so a retried part cannot corrupt the assembly order.
func (r *fileRepo) PutUploadPart(ctx context.Context, p *biz.UploadPart) (*biz.UploadPart, error) {
	ex := r.data.Exec(ctx)
	if _, err := ex.UploadPart().Delete().
		Where(uploadpart.UploadIDEQ(p.UploadID), uploadpart.PartNumberEQ(p.PartNumber)).
		Exec(ctx); err != nil {
		return nil, fmt.Errorf("data: replace upload part: %w", err)
	}
	builder := ex.UploadPart().Create().
		SetUploadID(p.UploadID).
		SetPartNumber(p.PartNumber).
		SetSize(p.Size).
		SetEtag(p.Etag).
		SetStorageKey(p.StorageKey)
	if p.ID != uuid.Nil {
		builder = builder.SetID(p.ID)
	}
	po, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: create upload part: %w", err)
	}
	return toBizUploadPart(po), nil
}

func (r *fileRepo) ListUploadParts(ctx context.Context, uploadID uuid.UUID, from int32) ([]*biz.UploadPart, error) {
	query := r.data.Exec(ctx).UploadPart().Query().Where(uploadpart.UploadIDEQ(uploadID))
	if from > 0 {
		query = query.Where(uploadpart.PartNumberGT(from))
	}
	pos, err := query.Order(uploadpart.ByPartNumber()).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list upload parts: %w", err)
	}
	out := make([]*biz.UploadPart, 0, len(pos))
	for _, po := range pos {
		out = append(out, toBizUploadPart(po))
	}
	return out, nil
}

func (r *fileRepo) DeleteUploadParts(ctx context.Context, uploadID uuid.UUID) error {
	if _, err := r.data.Exec(ctx).UploadPart().Delete().Where(uploadpart.UploadIDEQ(uploadID)).Exec(ctx); err != nil {
		return fmt.Errorf("data: delete upload parts: %w", err)
	}
	return nil
}

func (r *fileRepo) CreateVersion(ctx context.Context, v *biz.NodeVersion) (*biz.NodeVersion, error) {
	builder := r.data.Exec(ctx).NodeVersion().Create().
		SetNodeID(v.NodeID).
		SetVersion(v.Version).
		SetStorageKey(v.StorageKey).
		SetSize(v.Size).
		SetMimeType(v.MimeType).
		SetEtag(v.Etag).
		SetComment(v.Comment).
		SetCreatedBy(v.CreatedBy)
	if v.ID != uuid.Nil {
		builder = builder.SetID(v.ID)
	}
	po, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: create node version: %w", err)
	}
	return toBizVersion(po), nil
}

func (r *fileRepo) ListVersions(ctx context.Context, nodeID uuid.UUID, opts ...biz.ListOption) ([]*biz.NodeVersion, error) {
	options := biz.ResolveListOptions(50, opts...)
	pos, err := r.data.Exec(ctx).NodeVersion().Query().
		Where(nodeversion.NodeIDEQ(nodeID)).
		Where(ents.ApplyFilter(options.Filter)).
		Order(nodeversion.ByVersion(sql.OrderDesc())).
		Offset(options.Offset).
		Limit(options.Limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list node versions: %w", err)
	}
	out := make([]*biz.NodeVersion, 0, len(pos))
	for _, po := range pos {
		out = append(out, toBizVersion(po))
	}
	return out, nil
}

func (r *fileRepo) FindVersionByID(ctx context.Context, id uuid.UUID) (*biz.NodeVersion, error) {
	po, err := r.data.Exec(ctx).NodeVersion().Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: find node version: %w", err)
	}
	return toBizVersion(po), nil
}

func (r *fileRepo) DeleteVersion(ctx context.Context, id uuid.UUID) error {
	if err := r.data.Exec(ctx).NodeVersion().DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("data: delete node version: %w", err)
	}
	return nil
}

func (r *fileRepo) CountVersions(ctx context.Context, nodeID uuid.UUID) (int64, error) {
	count, err := r.data.Exec(ctx).NodeVersion().Query().Where(nodeversion.NodeIDEQ(nodeID)).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: count node versions: %w", err)
	}
	return int64(count), nil
}

// NextVersionNumber returns the next revision number of a node.
func (r *fileRepo) NextVersionNumber(ctx context.Context, nodeID uuid.UUID) (int32, error) {
	po, err := r.data.Exec(ctx).NodeVersion().Query().
		Where(nodeversion.NodeIDEQ(nodeID)).
		Order(nodeversion.ByVersion(sql.OrderDesc())).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 1, nil
		}
		return 0, fmt.Errorf("data: next version number: %w", err)
	}
	return po.Version + 1, nil
}

func (r *fileRepo) EnqueueDeletion(ctx context.Context, key, reason string) error {
	if key == "" {
		return nil
	}
	if _, err := r.data.Exec(ctx).PendingDeletion().Create().
		SetStorageKey(key).
		SetReason(reason).
		Save(ctx); err != nil {
		return fmt.Errorf("data: enqueue deletion: %w", err)
	}
	return nil
}

func (r *fileRepo) ListPendingDeletions(ctx context.Context, limit int) ([]*biz.PendingDeletion, error) {
	if limit <= 0 {
		limit = 500
	}
	pos, err := r.data.Exec(ctx).PendingDeletion().Query().
		Order(pendingdeletion.ByCreatedAt()).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list pending deletions: %w", err)
	}
	out := make([]*biz.PendingDeletion, 0, len(pos))
	for _, po := range pos {
		out = append(out, toBizPendingDeletion(po))
	}
	return out, nil
}

func (r *fileRepo) DeletePendingDeletion(ctx context.Context, id uuid.UUID) error {
	if err := r.data.Exec(ctx).PendingDeletion().DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("data: delete pending deletion: %w", err)
	}
	return nil
}

func (r *fileRepo) MarkDeletionAttempt(ctx context.Context, id uuid.UUID, lastErr string) error {
	if _, err := r.data.Exec(ctx).PendingDeletion().UpdateOneID(id).
		AddAttempts(1).
		SetLastError(lastErr).
		Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("data: mark deletion attempt: %w", err)
	}
	return nil
}
