package data

import (
	"context"
	"fmt"
	"time"

	"nagisa/internal/biz"
	"nagisa/internal/data/ent"
	"nagisa/internal/data/ent/predicate"
	"nagisa/internal/data/ent/share"

	"github.com/go-kratos/aip-go/ents"
	"github.com/google/uuid"
)

// defaultSharePageSize is used when the caller does not set a limit.
const defaultSharePageSize = 50

// shareRepo persists share links.
type shareRepo struct {
	data *Data
}

// NewShareRepo returns a share repository.
func NewShareRepo(d *Data) biz.ShareRepo { return &shareRepo{data: d} }

// CreateShare stores a new share link.
func (r *shareRepo) CreateShare(ctx context.Context, s *biz.Share) (*biz.Share, error) {
	if s == nil {
		return nil, biz.ErrInvalidArgument
	}
	created, err := newShareCreate(r.data.Exec(ctx).Share(), newSharePO(s)).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: create share: %w", err)
	}
	return toBizShare(created), nil
}

// FindShareByID loads one link by its identifier.
func (r *shareRepo) FindShareByID(ctx context.Context, id uuid.UUID) (*biz.Share, error) {
	po, err := r.data.Exec(ctx).Share().Query().Where(share.ID(id)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: find share by id: %w", err)
	}
	return toBizShare(po), nil
}

// FindShareByToken loads one link by its public token.
func (r *shareRepo) FindShareByToken(ctx context.Context, token string) (*biz.Share, error) {
	po, err := r.data.Exec(ctx).Share().Query().Where(share.Token(token)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: find share by token: %w", err)
	}
	return toBizShare(po), nil
}

// ListShares returns a page of share links.
func (r *shareRepo) ListShares(ctx context.Context, opts ...biz.ListOption) ([]*biz.Share, error) {
	o := biz.ResolveListOptions(defaultSharePageSize, opts...)
	rows, err := shareQuery(r.data.Exec(ctx).Share().Query(), o).
		Offset(o.Offset).
		Limit(o.Limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list shares: %w", err)
	}
	return toBizShares(rows), nil
}

// CountShares counts the links matching the options, ignoring pagination.
func (r *shareRepo) CountShares(ctx context.Context, opts ...biz.ListOption) (int64, error) {
	o := biz.ResolveListOptions(defaultSharePageSize, opts...)
	n, err := shareQuery(r.data.Exec(ctx).Share().Query(), o).Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: count shares: %w", err)
	}
	return int64(n), nil
}

// ListSharesByNode returns the links attached to one node.
func (r *shareRepo) ListSharesByNode(ctx context.Context, nodeID uuid.UUID, opts ...biz.ListOption) ([]*biz.Share, error) {
	o := biz.ResolveListOptions(defaultSharePageSize, opts...)
	base := r.data.Exec(ctx).Share().Query().Where(share.NodeID(nodeID))
	rows, err := shareQuery(base, o).
		Offset(o.Offset).
		Limit(o.Limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list shares by node: %w", err)
	}
	return toBizShares(rows), nil
}

// UpdateShare persists the mutable attributes of a link.
func (r *shareRepo) UpdateShare(ctx context.Context, s *biz.Share) (*biz.Share, error) {
	if s == nil || s.ID == uuid.Nil {
		return nil, biz.ErrInvalidArgument
	}
	update := r.data.Exec(ctx).Share().UpdateOneID(s.ID).
		SetName(s.Name).
		SetDescription(s.Description).
		SetPermissions(s.Permissions).
		SetPasswordHash(s.PasswordHash).
		SetPasswordHint(s.PasswordHint).
		SetMaxDownloads(s.MaxDownloads).
		SetStatus(s.Status)
	if s.ExpiresAt != nil {
		update.SetExpiresAt(*s.ExpiresAt)
	} else {
		update.ClearExpiresAt()
	}
	po, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: update share: %w", err)
	}
	return toBizShare(po), nil
}

// DeleteShare revokes a link.
func (r *shareRepo) DeleteShare(ctx context.Context, id uuid.UUID) error {
	if err := r.data.Exec(ctx).Share().DeleteOneID(id).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return biz.ErrNotFound
		}
		return fmt.Errorf("data: delete share: %w", err)
	}
	return nil
}

// AddShareView records one resolution of a link.
func (r *shareRepo) AddShareView(ctx context.Context, id uuid.UUID) error {
	n, err := r.data.Exec(ctx).Share().Update().
		Where(share.ID(id)).
		AddViewCount(1).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("data: add share view: %w", err)
	}
	if n == 0 {
		return biz.ErrNotFound
	}
	return nil
}

// AddShareDownload records one served download against the link budget.
func (r *shareRepo) AddShareDownload(ctx context.Context, id uuid.UUID) error {
	n, err := r.data.Exec(ctx).Share().Update().
		Where(share.ID(id)).
		AddDownloadCount(1).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("data: add share download: %w", err)
	}
	if n == 0 {
		return biz.ErrNotFound
	}
	return nil
}

// CountSharesByNodes returns the number of active links per node.
func (r *shareRepo) CountSharesByNodes(ctx context.Context, nodeIDs []uuid.UUID) (map[uuid.UUID]int64, error) {
	counts := make(map[uuid.UUID]int64, len(nodeIDs))
	if len(nodeIDs) == 0 {
		return counts, nil
	}
	var rows []struct {
		NodeID uuid.UUID `json:"node_id,omitempty"`
		Count  int64     `json:"count,omitempty"`
	}
	err := r.data.Exec(ctx).Share().Query().
		Where(share.NodeIDIn(nodeIDs...), share.Status(biz.ShareStatusActive)).
		GroupBy(share.FieldNodeID).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("data: count shares by nodes: %w", err)
	}
	for _, row := range rows {
		counts[row.NodeID] = row.Count
	}
	return counts, nil
}

// ListExpiredShares returns the active links that expired before now.
func (r *shareRepo) ListExpiredShares(ctx context.Context, now time.Time, limit int) ([]*biz.Share, error) {
	query := r.data.Exec(ctx).Share().Query().
		Where(
			share.Status(biz.ShareStatusActive),
			share.ExpiresAtNotNil(),
			share.ExpiresAtLT(now),
		).
		Order(share.ByExpiresAt())
	if limit > 0 {
		query = query.Limit(limit)
	}
	rows, err := query.All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list expired shares: %w", err)
	}
	return toBizShares(rows), nil
}

// shareQuery applies the standard AIP filter and ordering to a list query.
// The identifier is appended last so offset pagination stays stable when the
// caller orders by a non unique column.
func shareQuery(q *ent.ShareQuery, o biz.ListOptions) *ent.ShareQuery {
	return q.
		Where(predicate.Share(ents.ApplyFilter(o.Filter))).
		Order(share.OrderOption(ents.ApplyOrderBy(o.OrderBy)), share.ByID())
}

// newShareCreate builds the insert statement of a share PO.
func newShareCreate(c *ent.ShareClient, po *ent.Share) *ent.ShareCreate {
	builder := c.Create().
		SetToken(po.Token).
		SetNodeID(po.NodeID).
		SetOwnerID(po.OwnerID).
		SetName(po.Name).
		SetDescription(po.Description).
		SetPermissions(po.Permissions).
		SetPasswordHash(po.PasswordHash).
		SetPasswordHint(po.PasswordHint).
		SetMaxDownloads(po.MaxDownloads).
		SetDownloadCount(po.DownloadCount).
		SetViewCount(po.ViewCount).
		SetStatus(po.Status).
		SetCreatedBy(po.CreatedBy)
	if po.ID != uuid.Nil {
		builder.SetID(po.ID)
	}
	if po.ExpiresAt != nil {
		builder.SetExpiresAt(*po.ExpiresAt)
	}
	return builder
}

// newSharePO converts a domain share into its storage shape.
func newSharePO(s *biz.Share) *ent.Share {
	return &ent.Share{
		ID:            s.ID,
		Token:         s.Token,
		NodeID:        s.NodeID,
		OwnerID:       s.OwnerID,
		Name:          s.Name,
		Description:   s.Description,
		Permissions:   s.Permissions,
		PasswordHash:  s.PasswordHash,
		PasswordHint:  s.PasswordHint,
		ExpiresAt:     s.ExpiresAt,
		MaxDownloads:  s.MaxDownloads,
		DownloadCount: s.DownloadCount,
		ViewCount:     s.ViewCount,
		Status:        s.Status,
		CreatedBy:     s.CreatedBy,
	}
}

// toBizShare converts a stored share into its domain shape.
func toBizShare(po *ent.Share) *biz.Share {
	if po == nil {
		return nil
	}
	return &biz.Share{
		ID:            po.ID,
		Token:         po.Token,
		NodeID:        po.NodeID,
		OwnerID:       po.OwnerID,
		Name:          po.Name,
		Description:   po.Description,
		Permissions:   po.Permissions,
		PasswordHash:  po.PasswordHash,
		PasswordHint:  po.PasswordHint,
		ExpiresAt:     po.ExpiresAt,
		MaxDownloads:  po.MaxDownloads,
		DownloadCount: po.DownloadCount,
		ViewCount:     po.ViewCount,
		Status:        po.Status,
		CreatedBy:     po.CreatedBy,
		CreatedAt:     po.CreatedAt,
		UpdatedAt:     po.UpdatedAt,
	}
}

// toBizShares converts a list of stored shares.
func toBizShares(rows []*ent.Share) []*biz.Share {
	out := make([]*biz.Share, 0, len(rows))
	for _, row := range rows {
		out = append(out, toBizShare(row))
	}
	return out
}
