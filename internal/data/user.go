package data

import (
	"context"
	"fmt"
	"time"

	"nagisa/internal/biz"
	"nagisa/internal/data/ent"
	"nagisa/internal/data/ent/node"
	"nagisa/internal/data/ent/refreshsession"
	"nagisa/internal/data/ent/user"

	"github.com/go-kratos/aip-go/ents"
	"github.com/google/uuid"
)

// defaultUserPageSize is used when the caller does not set a limit.
const defaultUserPageSize = 50

// userRepo persists accounts and their refresh sessions.
type userRepo struct {
	data *Data
}

// NewUserRepo returns an account repository.
func NewUserRepo(d *Data) biz.UserRepo { return &userRepo{data: d} }

// FindUserByID loads one account. A soft deleted row is reported as missing.
func (r *userRepo) FindUserByID(ctx context.Context, id uuid.UUID) (*biz.User, error) {
	po, err := r.data.Exec(ctx).User().Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: find user by id: %w", err)
	}
	if po.Status == biz.UserStatusDeleted {
		return nil, biz.ErrNotFound
	}
	return toBizUser(po), nil
}

// FindUsersByIDs loads several accounts at once. Ids that match no row are
// simply absent from the result, which is never an error.
func (r *userRepo) FindUsersByIDs(ctx context.Context, ids []uuid.UUID) ([]*biz.User, error) {
	if len(ids) == 0 {
		return []*biz.User{}, nil
	}
	rows, err := r.data.Exec(ctx).User().Query().Where(user.IDIn(ids...)).All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: find users by ids: %w", err)
	}
	return toBizUsers(rows), nil
}

// FindUserByUsername loads the account carrying the exact name.
func (r *userRepo) FindUserByUsername(ctx context.Context, username string) (*biz.User, error) {
	po, err := r.data.Exec(ctx).User().Query().Where(user.UsernameEQ(username)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: find user by username: %w", err)
	}
	return toBizUser(po), nil
}

// ListUsers returns a page of accounts.
func (r *userRepo) ListUsers(ctx context.Context, opts ...biz.ListOption) ([]*biz.User, error) {
	o := biz.ResolveListOptions(defaultUserPageSize, opts...)
	rows, err := r.data.Exec(ctx).User().Query().
		Where(ents.ApplyFilter(o.Filter)).
		Order(ents.ApplyOrderBy(o.OrderBy), user.ByID()).
		Offset(o.Offset).
		Limit(o.Limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list users: %w", err)
	}
	return toBizUsers(rows), nil
}

// CountUsers counts the accounts matching the options, ignoring pagination.
func (r *userRepo) CountUsers(ctx context.Context, opts ...biz.ListOption) (int64, error) {
	o := biz.ResolveListOptions(defaultUserPageSize, opts...)
	n, err := r.data.Exec(ctx).User().Query().
		Where(ents.ApplyFilter(o.Filter)).
		Count(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: count users: %w", err)
	}
	return int64(n), nil
}

// CreateUser stores a new account, letting ent mint the identifier.
func (r *userRepo) CreateUser(ctx context.Context, u *biz.User) (*biz.User, error) {
	if u == nil {
		return nil, biz.ErrInvalidArgument
	}
	created, err := newUserCreate(r.data.Exec(ctx).User(), newUserPO(u)).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: create user: %w", err)
	}
	return toBizUser(created), nil
}

// UpdateUser writes every mutable column of an account. The username is
// immutable, and the provisioning audit columns are left untouched.
func (r *userRepo) UpdateUser(ctx context.Context, u *biz.User) (*biz.User, error) {
	if u == nil || u.ID == uuid.Nil {
		return nil, biz.ErrInvalidArgument
	}
	update := r.data.Exec(ctx).User().UpdateOneID(u.ID).
		SetNickname(u.Nickname).
		SetEmail(u.Email).
		SetAvatarURL(u.AvatarURL).
		SetPasswordHash(u.PasswordHash).
		SetRole(u.Role).
		SetRank(u.Rank).
		SetPermissions(u.Permissions).
		SetStatus(u.Status).
		SetQuotaBytes(u.QuotaBytes).
		SetUsedBytes(u.UsedBytes).
		SetFileCount(u.FileCount).
		SetFolderCount(u.FolderCount).
		SetRemark(u.Remark).
		SetMustChangePassword(u.MustChangePassword)
	if u.LastLoginAt != nil {
		update = update.SetLastLoginAt(*u.LastLoginAt)
	} else {
		update = update.ClearLastLoginAt()
	}
	po, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: update user: %w", err)
	}
	return toBizUser(po), nil
}

// DeleteUser soft deletes an account.
func (r *userRepo) DeleteUser(ctx context.Context, id uuid.UUID) error {
	affected, err := r.data.Exec(ctx).User().Update().
		Where(user.IDEQ(id)).
		SetStatus(biz.UserStatusDeleted).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("data: delete user: %w", err)
	}
	if affected == 0 {
		return biz.ErrNotFound
	}
	return nil
}

// AddUsage applies a signed delta to the account counters and keeps every
// counter at or above zero.
func (r *userRepo) AddUsage(ctx context.Context, id uuid.UUID, deltaBytes, deltaFiles, deltaFolders int64) error {
	ex := r.data.Exec(ctx)
	affected, err := ex.User().Update().
		Where(user.IDEQ(id)).
		AddUsedBytes(deltaBytes).
		AddFileCount(deltaFiles).
		AddFolderCount(deltaFolders).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("data: add user usage: %w", err)
	}
	if affected == 0 {
		// MySQL reports changed rather than matched rows, so a no-op delta also
		// lands here and the lookup tells the two cases apart.
		if _, err := r.FindUserByID(ctx, id); err != nil {
			return err
		}
	}
	if deltaBytes < 0 || deltaFiles < 0 || deltaFolders < 0 {
		if _, err := ex.User().Update().
			Where(user.IDEQ(id), user.UsedBytesLT(0)).
			SetUsedBytes(0).
			Save(ctx); err != nil {
			return fmt.Errorf("data: clamp used bytes: %w", err)
		}
		if _, err := ex.User().Update().
			Where(user.IDEQ(id), user.FileCountLT(0)).
			SetFileCount(0).
			Save(ctx); err != nil {
			return fmt.Errorf("data: clamp file count: %w", err)
		}
		if _, err := ex.User().Update().
			Where(user.IDEQ(id), user.FolderCountLT(0)).
			SetFolderCount(0).
			Save(ctx); err != nil {
			return fmt.Errorf("data: clamp folder count: %w", err)
		}
	}
	return nil
}

// UserStats folds the node tree into the storage summary of one account.
func (r *userRepo) UserStats(ctx context.Context, id uuid.UUID) (*biz.UserStats, error) {
	account, err := r.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	ex := r.data.Exec(ctx)
	files, err := nodeUsageTotals(ctx, ex.Node().Query().Where(
		node.OwnerIDEQ(id),
		node.KindEQ(biz.NodeKindFile),
		node.StatusEQ(biz.NodeStatusActive),
	))
	if err != nil {
		return nil, fmt.Errorf("data: user file usage: %w", err)
	}
	folders, err := nodeUsageTotals(ctx, ex.Node().Query().Where(
		node.OwnerIDEQ(id),
		node.KindEQ(biz.NodeKindFolder),
		node.StatusEQ(biz.NodeStatusActive),
	))
	if err != nil {
		return nil, fmt.Errorf("data: user folder usage: %w", err)
	}
	trashed, err := nodeUsageTotals(ctx, ex.Node().Query().Where(
		node.OwnerIDEQ(id),
		node.StatusEQ(biz.NodeStatusTrashed),
	))
	if err != nil {
		return nil, fmt.Errorf("data: user trashed usage: %w", err)
	}
	stats := &biz.UserStats{
		UserID:       account.ID,
		Username:     account.Username,
		QuotaBytes:   account.QuotaBytes,
		UsedBytes:    files.Sum,
		FileCount:    files.Count,
		FolderCount:  folders.Count,
		TrashedCount: trashed.Count,
		TrashedBytes: trashed.Sum,
	}
	if stats.QuotaBytes > 0 {
		stats.UsageRatio = float64(stats.UsedBytes) / float64(stats.QuotaBytes)
	}
	return stats, nil
}

// CountByRole counts the live accounts of each role.
func (r *userRepo) CountByRole(ctx context.Context) (map[biz.Role]int64, error) {
	var rows []struct {
		Role  biz.Role `json:"role,omitempty"`
		Count int64    `json:"count,omitempty"`
	}
	err := r.data.Exec(ctx).User().Query().
		Where(user.StatusNEQ(biz.UserStatusDeleted)).
		GroupBy(user.FieldRole).
		Aggregate(ent.Count()).
		Scan(ctx, &rows)
	if err != nil {
		return nil, fmt.Errorf("data: count users by role: %w", err)
	}
	counts := make(map[biz.Role]int64, len(rows))
	for _, row := range rows {
		counts[row.Role] = row.Count
	}
	return counts, nil
}

// RecountAllUsage rebuilds every account counter from the node tree and
// reports how many accounts were written.
func (r *userRepo) RecountAllUsage(ctx context.Context) (int64, error) {
	ex := r.data.Exec(ctx)
	files, err := nodeUsageByOwner(ctx, ex.Node().Query().Where(
		node.KindEQ(biz.NodeKindFile),
		node.StatusEQ(biz.NodeStatusActive),
	))
	if err != nil {
		return 0, fmt.Errorf("data: group file usage: %w", err)
	}
	folders, err := nodeUsageByOwner(ctx, ex.Node().Query().Where(
		node.KindEQ(biz.NodeKindFolder),
		node.StatusEQ(biz.NodeStatusActive),
	))
	if err != nil {
		return 0, fmt.Errorf("data: group folder usage: %w", err)
	}
	accounts, err := ex.User().Query().Select(user.FieldID).All(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: list accounts to recount: %w", err)
	}
	var updated int64
	for _, account := range accounts {
		affected, err := ex.User().Update().
			Where(user.IDEQ(account.ID)).
			SetUsedBytes(files[account.ID].Sum).
			SetFileCount(files[account.ID].Count).
			SetFolderCount(folders[account.ID].Count).
			Save(ctx)
		if err != nil {
			return 0, fmt.Errorf("data: recount user usage: %w", err)
		}
		updated += int64(affected)
	}
	return updated, nil
}

// CreateSession records a new refresh token family.
func (r *userRepo) CreateSession(ctx context.Context, s *biz.Session) (*biz.Session, error) {
	if s == nil {
		return nil, biz.ErrInvalidArgument
	}
	created, err := newSessionCreate(r.data.Exec(ctx).RefreshSession(), newSessionPO(s)).Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: create session: %w", err)
	}
	return toBizSession(created), nil
}

// FindSessionByHash loads the session carrying the token hash.
func (r *userRepo) FindSessionByHash(ctx context.Context, hash string) (*biz.Session, error) {
	po, err := r.data.Exec(ctx).RefreshSession().Query().
		Where(refreshsession.TokenHashEQ(hash)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: find session by hash: %w", err)
	}
	return toBizSession(po), nil
}

// FindSessionByID loads one session.
func (r *userRepo) FindSessionByID(ctx context.Context, id uuid.UUID) (*biz.Session, error) {
	po, err := r.data.Exec(ctx).RefreshSession().Get(ctx, id)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, biz.ErrNotFound
		}
		return nil, fmt.Errorf("data: find session by id: %w", err)
	}
	return toBizSession(po), nil
}

// TouchSession records that a refresh token was just used.
func (r *userRepo) TouchSession(ctx context.Context, id uuid.UUID, at time.Time) error {
	if err := r.data.Exec(ctx).RefreshSession().UpdateOneID(id).
		SetLastUsedAt(at).
		Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return biz.ErrNotFound
		}
		return fmt.Errorf("data: touch session: %w", err)
	}
	return nil
}

// RevokeSession stops one session from being used again.
func (r *userRepo) RevokeSession(ctx context.Context, id uuid.UUID) error {
	if err := r.data.Exec(ctx).RefreshSession().UpdateOneID(id).
		SetRevokedAt(time.Now()).
		Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return biz.ErrNotFound
		}
		return fmt.Errorf("data: revoke session: %w", err)
	}
	return nil
}

// RevokeUserSessions revokes every session of a user that is still usable.
func (r *userRepo) RevokeUserSessions(ctx context.Context, id uuid.UUID) error {
	_, err := r.data.Exec(ctx).RefreshSession().Update().
		Where(refreshsession.UserIDEQ(id), refreshsession.RevokedAtIsNil()).
		SetRevokedAt(time.Now()).
		Save(ctx)
	if err != nil {
		return fmt.Errorf("data: revoke user sessions: %w", err)
	}
	return nil
}

// ListSessions returns a page of sessions. Unless includeInactive is set, only
// the sessions that are neither revoked nor expired are returned.
func (r *userRepo) ListSessions(ctx context.Context, id uuid.UUID, includeInactive bool, opts ...biz.ListOption) ([]*biz.Session, error) {
	o := biz.ResolveListOptions(defaultUserPageSize, opts...)
	query := r.data.Exec(ctx).RefreshSession().Query().
		Where(refreshsession.UserIDEQ(id)).
		Where(ents.ApplyFilter(o.Filter))
	if !includeInactive {
		query = query.Where(
			refreshsession.RevokedAtIsNil(),
			refreshsession.ExpiresAtGT(time.Now()),
		)
	}
	rows, err := query.
		Order(ents.ApplyOrderBy(o.OrderBy), refreshsession.ByID()).
		Offset(o.Offset).
		Limit(o.Limit).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("data: list sessions: %w", err)
	}
	return toBizSessions(rows), nil
}

// PurgeExpiredSessions deletes the sessions that expired before the instant.
func (r *userRepo) PurgeExpiredSessions(ctx context.Context, before time.Time) (int64, error) {
	deleted, err := r.data.Exec(ctx).RefreshSession().Delete().
		Where(refreshsession.ExpiresAtLT(before)).
		Exec(ctx)
	if err != nil {
		return 0, fmt.Errorf("data: purge sessions: %w", err)
	}
	return int64(deleted), nil
}

// nodeUsage is one row of a node aggregate: a row count and a byte total.
type nodeUsage struct {
	OwnerID uuid.UUID `json:"owner_id,omitempty"`
	Count   int64     `json:"count,omitempty"`
	Sum     int64     `json:"sum,omitempty"`
}

// nodeUsageTotals folds a node query into a single count and byte total. An
// empty result leaves both at zero, because SUM over no row is NULL.
func nodeUsageTotals(ctx context.Context, q *ent.NodeQuery) (nodeUsage, error) {
	var rows []nodeUsage
	if err := q.Aggregate(ent.Count(), ent.Sum(node.FieldSize)).Scan(ctx, &rows); err != nil {
		return nodeUsage{}, err
	}
	if len(rows) == 0 {
		return nodeUsage{}, nil
	}
	return rows[0], nil
}

// nodeUsageByOwner groups the node counters of a query by owner.
func nodeUsageByOwner(ctx context.Context, q *ent.NodeQuery) (map[uuid.UUID]nodeUsage, error) {
	var rows []nodeUsage
	if err := q.GroupBy(node.FieldOwnerID).
		Aggregate(ent.Count(), ent.Sum(node.FieldSize)).
		Scan(ctx, &rows); err != nil {
		return nil, err
	}
	usage := make(map[uuid.UUID]nodeUsage, len(rows))
	for _, row := range rows {
		usage[row.OwnerID] = row
	}
	return usage, nil
}

// newUserPO converts a domain account into its storage shape.
func newUserPO(u *biz.User) *ent.User {
	return &ent.User{
		ID:                 u.ID,
		Username:           u.Username,
		Nickname:           u.Nickname,
		Email:              u.Email,
		AvatarURL:          u.AvatarURL,
		PasswordHash:       u.PasswordHash,
		Role:               u.Role,
		Rank:               u.Rank,
		Permissions:        u.Permissions,
		Status:             u.Status,
		QuotaBytes:         u.QuotaBytes,
		UsedBytes:          u.UsedBytes,
		FileCount:          u.FileCount,
		FolderCount:        u.FolderCount,
		Remark:             u.Remark,
		MustChangePassword: u.MustChangePassword,
		LastLoginAt:        u.LastLoginAt,
		CreatedBy:          u.CreatedBy,
	}
}

// newUserCreate builds the insert statement of an account.
func newUserCreate(c *ent.UserClient, po *ent.User) *ent.UserCreate {
	builder := c.Create().
		SetUsername(po.Username).
		SetNickname(po.Nickname).
		SetEmail(po.Email).
		SetAvatarURL(po.AvatarURL).
		SetPasswordHash(po.PasswordHash).
		SetRole(po.Role).
		SetRank(po.Rank).
		SetPermissions(po.Permissions).
		SetStatus(po.Status).
		SetQuotaBytes(po.QuotaBytes).
		SetUsedBytes(po.UsedBytes).
		SetFileCount(po.FileCount).
		SetFolderCount(po.FolderCount).
		SetRemark(po.Remark).
		SetMustChangePassword(po.MustChangePassword).
		SetCreatedBy(po.CreatedBy)
	if po.ID != uuid.Nil {
		builder.SetID(po.ID)
	}
	if po.LastLoginAt != nil {
		builder.SetLastLoginAt(*po.LastLoginAt)
	}
	return builder
}

// toBizUser converts a stored account into its domain shape.
func toBizUser(po *ent.User) *biz.User {
	if po == nil {
		return nil
	}
	return &biz.User{
		ID:                 po.ID,
		Username:           po.Username,
		Nickname:           po.Nickname,
		Email:              po.Email,
		AvatarURL:          po.AvatarURL,
		PasswordHash:       po.PasswordHash,
		Role:               po.Role,
		Rank:               po.Rank,
		Permissions:        po.Permissions,
		Status:             po.Status,
		QuotaBytes:         po.QuotaBytes,
		UsedBytes:          po.UsedBytes,
		FileCount:          po.FileCount,
		FolderCount:        po.FolderCount,
		Remark:             po.Remark,
		MustChangePassword: po.MustChangePassword,
		LastLoginAt:        po.LastLoginAt,
		CreatedBy:          po.CreatedBy,
		CreatedAt:          po.CreatedAt,
		UpdatedAt:          po.UpdatedAt,
	}
}

// toBizUsers converts a list of stored accounts.
func toBizUsers(rows []*ent.User) []*biz.User {
	out := make([]*biz.User, 0, len(rows))
	for _, row := range rows {
		out = append(out, toBizUser(row))
	}
	return out
}

// newSessionPO converts a domain session into its storage shape.
func newSessionPO(s *biz.Session) *ent.RefreshSession {
	return &ent.RefreshSession{
		ID:         s.ID,
		UserID:     s.UserID,
		TokenHash:  s.TokenHash,
		IP:         s.IP,
		UserAgent:  s.UserAgent,
		ExpiresAt:  s.ExpiresAt,
		RevokedAt:  s.RevokedAt,
		LastUsedAt: s.LastUsedAt,
	}
}

// newSessionCreate builds the insert statement of a refresh session.
func newSessionCreate(c *ent.RefreshSessionClient, po *ent.RefreshSession) *ent.RefreshSessionCreate {
	builder := c.Create().
		SetUserID(po.UserID).
		SetTokenHash(po.TokenHash).
		SetIP(po.IP).
		SetUserAgent(po.UserAgent).
		SetExpiresAt(po.ExpiresAt)
	if po.ID != uuid.Nil {
		builder.SetID(po.ID)
	}
	if po.RevokedAt != nil {
		builder.SetRevokedAt(*po.RevokedAt)
	}
	if po.LastUsedAt != nil {
		builder.SetLastUsedAt(*po.LastUsedAt)
	}
	return builder
}

// toBizSession converts a stored session into its domain shape.
func toBizSession(po *ent.RefreshSession) *biz.Session {
	if po == nil {
		return nil
	}
	return &biz.Session{
		ID:         po.ID,
		UserID:     po.UserID,
		TokenHash:  po.TokenHash,
		IP:         po.IP,
		UserAgent:  po.UserAgent,
		ExpiresAt:  po.ExpiresAt,
		RevokedAt:  po.RevokedAt,
		LastUsedAt: po.LastUsedAt,
		CreatedAt:  po.CreatedAt,
	}
}

// toBizSessions converts a list of stored sessions.
func toBizSessions(rows []*ent.RefreshSession) []*biz.Session {
	out := make([]*biz.Session, 0, len(rows))
	for _, row := range rows {
		out = append(out, toBizSession(row))
	}
	return out
}
