package biz

import (
	"context"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// usernamePattern is the accepted shape of an account name.
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{1,31}$`)

// User is an account.
type User struct {
	ID                 uuid.UUID
	Username           string
	Nickname           string
	Email              string
	AvatarURL          string
	PasswordHash       string
	Role               Role
	Rank               int32
	Permissions        PermMask
	Status             UserStatus
	QuotaBytes         int64
	UsedBytes          int64
	FileCount          int64
	FolderCount        int64
	Remark             string
	MustChangePassword bool
	LastLoginAt        *time.Time
	CreatedBy          uuid.UUID
	CreatedAt          time.Time
	UpdatedAt          time.Time

	// Locked marks one of the built-in accounts. Their role, rank and
	// permission set are fixed by configuration: no caller, not even an
	// administrator, may change them or delete the account. The flag is
	// computed on read and never stored.
	Locked bool
}

// Session is one refresh token family.
type Session struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	TokenHash  string
	IP         string
	UserAgent  string
	ExpiresAt  time.Time
	RevokedAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
}

// Active reports whether the session can still be used at the supplied time.
func (s *Session) Active(now time.Time) bool {
	return s != nil && s.RevokedAt == nil && now.Before(s.ExpiresAt)
}

// UserStats summarises one account's storage usage.
type UserStats struct {
	UserID       uuid.UUID
	Username     string
	QuotaBytes   int64
	UsedBytes    int64
	FileCount    int64
	FolderCount  int64
	TrashedBytes int64
	TrashedCount int64
	VersionBytes int64
	UsageRatio   float64
}

// UserRepo persists accounts and their sessions.
type UserRepo interface {
	FindUserByID(context.Context, uuid.UUID) (*User, error)
	FindUsersByIDs(context.Context, []uuid.UUID) ([]*User, error)
	FindUserByUsername(context.Context, string) (*User, error)
	ListUsers(context.Context, ...ListOption) ([]*User, error)
	CountUsers(context.Context, ...ListOption) (int64, error)
	CreateUser(context.Context, *User) (*User, error)
	UpdateUser(context.Context, *User) (*User, error)
	DeleteUser(context.Context, uuid.UUID) error
	// AddUsage applies a signed delta to the account counters. It never lets
	// used_bytes drop below zero.
	AddUsage(context.Context, uuid.UUID, int64, int64, int64) error
	UserStats(context.Context, uuid.UUID) (*UserStats, error)
	CountByRole(context.Context) (map[Role]int64, error)
	// RecountAllUsage recomputes every account's counters from the tree.
	RecountAllUsage(context.Context) (int64, error)

	CreateSession(context.Context, *Session) (*Session, error)
	FindSessionByHash(context.Context, string) (*Session, error)
	FindSessionByID(context.Context, uuid.UUID) (*Session, error)
	TouchSession(context.Context, uuid.UUID, time.Time) error
	RevokeSession(context.Context, uuid.UUID) error
	RevokeUserSessions(context.Context, uuid.UUID) error
	ListSessions(context.Context, uuid.UUID, bool, ...ListOption) ([]*Session, error)
	PurgeExpiredSessions(context.Context, time.Time) (int64, error)
}

// UserUsecaseOptions carries the account defaults taken from configuration.
type UserUsecaseOptions struct {
	DefaultQuotaBytes int64
	DefaultRolePreset string
	MinPasswordLength int
	// Names of the two built-in accounts. They are fixed by the deployment and
	// out of reach of every caller, including an administrator.
	AdminUsername string
	GuestUsername string
}

// UserUsecase implements account management and the authority rules.
type UserUsecase struct {
	repo UserRepo
	tx   TxManager

	defaultQuota  int64
	defaultPreset string
	minPassword   int
	builtin       map[string]struct{}
}

// NewUserUsecase returns a user usecase.
func NewUserUsecase(repo UserRepo, tx TxManager, opts UserUsecaseOptions) *UserUsecase {
	if opts.MinPasswordLength <= 0 {
		opts.MinPasswordLength = 8
	}
	builtin := make(map[string]struct{}, 2)
	for _, name := range []string{opts.AdminUsername, opts.GuestUsername} {
		if key := accountKey(name); key != "" {
			builtin[key] = struct{}{}
		}
	}
	return &UserUsecase{
		repo:          repo,
		tx:            tx,
		defaultQuota:  opts.DefaultQuotaBytes,
		defaultPreset: opts.DefaultRolePreset,
		minPassword:   opts.MinPasswordLength,
		builtin:       builtin,
	}
}

// accountKey normalises a username for the built-in lookup.
func accountKey(username string) string {
	return strings.ToLower(strings.TrimSpace(username))
}

// IsBuiltin reports whether an account is one of the two built-in ones.
func (uc *UserUsecase) IsBuiltin(user *User) bool {
	if user == nil {
		return false
	}
	_, ok := uc.builtin[accountKey(user.Username)]
	return ok
}

// markLocked fills the computed flag on accounts handed to callers.
func (uc *UserUsecase) markLocked(users ...*User) {
	for _, user := range users {
		if user != nil {
			user.Locked = uc.IsBuiltin(user)
		}
	}
}

// CreateUserInput describes a new account.
type CreateUserInput struct {
	Username    string
	Nickname    string
	Email       string
	AvatarURL   string
	Role        Role
	Rank        int32
	Permissions PermMask
	// PermissionsSet marks that the caller supplied an explicit permission
	// set, including a deliberately empty one. Without it an account inherits
	// the preset of its role.
	PermissionsSet     bool
	QuotaBytes         int64
	Remark             string
	PasswordHash       string
	Enabled            bool
	MustChangePassword bool
}

// CreateUser provisions an account. The caller must hold PERMISSION_USER_MANAGE
// and may only assign a rank below its own and a permission set it holds
// itself, which is what keeps the administrator hierarchy intact.
func (uc *UserUsecase) CreateUser(ctx context.Context, in *CreateUserInput) (*User, error) {
	actor := CallerFromContext(ctx)
	if err := actor.Require(PermUserManage); err != nil {
		return nil, err
	}
	if in == nil {
		return nil, ErrInvalidArgument
	}
	in.Username = strings.TrimSpace(in.Username)
	if !usernamePattern.MatchString(in.Username) {
		return nil, ErrInvalidArgument
	}
	if in.Role == RoleUnspecified {
		in.Role = RoleGuest
		if preset, ok := ParseRole(uc.defaultPreset); ok {
			in.Role = preset
		}
	}
	if _, ok := RolePresetFor(in.Role); !ok {
		return nil, ErrInvalidArgument
	}
	if in.Rank <= 0 {
		in.Rank = DefaultRankFor(in.Role)
	}
	if in.Rank >= actor.Rank {
		return nil, ErrPermissionDenied
	}
	if !in.PermissionsSet {
		// A brand new account that names no permission set starts with the
		// preset of its role, which keeps a guest read-only.
		in.Permissions = DefaultPermsFor(in.Role)
	}
	if !actor.Perms.Has(in.Permissions) {
		return nil, ErrPermissionDenied
	}
	if in.Role == RoleAdmin && actor.Role != RoleAdmin {
		return nil, ErrPermissionDenied
	}
	if in.QuotaBytes == 0 {
		in.QuotaBytes = uc.defaultQuota
	}
	if in.Nickname == "" {
		in.Nickname = in.Username
	}
	status := UserStatusDisabled
	if in.Enabled {
		status = UserStatusActive
	}
	user := &User{
		Username:           in.Username,
		Nickname:           in.Nickname,
		Email:              in.Email,
		AvatarURL:          in.AvatarURL,
		PasswordHash:       in.PasswordHash,
		Role:               in.Role,
		Rank:               in.Rank,
		Permissions:        in.Permissions,
		Status:             status,
		QuotaBytes:         in.QuotaBytes,
		Remark:             in.Remark,
		MustChangePassword: in.MustChangePassword,
		CreatedBy:          actor.UserID,
	}
	var created *User
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		existing, err := uc.repo.FindUserByUsername(ctx, user.Username)
		if err == nil && existing != nil {
			return ErrAlreadyExists
		}
		if err != nil && err != ErrNotFound {
			return err
		}
		created, err = uc.repo.CreateUser(ctx, user)
		return err
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetUser returns an account the caller is allowed to read.
func (uc *UserUsecase) GetUser(ctx context.Context, id uuid.UUID) (*User, error) {
	if id == uuid.Nil {
		return nil, ErrInvalidArgument
	}
	actor := CallerFromContext(ctx)
	if !actor.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	target, err := uc.repo.FindUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	uc.markLocked(target)
	if target.ID == actor.UserID {
		return target, nil
	}
	if err := actor.Require(PermUserManage); err != nil {
		return nil, err
	}
	if err := CanManageUser(actor, target); err != nil {
		return nil, err
	}
	return target, nil
}

// ListUsers returns a page of accounts.
func (uc *UserUsecase) ListUsers(ctx context.Context, opts ...ListOption) ([]*User, int64, error) {
	actor := CallerFromContext(ctx)
	if err := actor.Require(PermUserManage); err != nil {
		return nil, 0, err
	}
	users, err := uc.repo.ListUsers(ctx, opts...)
	if err != nil {
		return nil, 0, err
	}
	uc.markLocked(users...)
	total, err := uc.repo.CountUsers(ctx, opts...)
	if err != nil {
		return nil, 0, err
	}
	return users, total, nil
}

// UpdateUserInput carries the writable fields of an account.
type UpdateUserInput struct {
	ID                 uuid.UUID
	Nickname           *string
	Email              *string
	AvatarURL          *string
	Remark             *string
	Role               *Role
	Rank               *int32
	Permissions        *PermMask
	Status             *UserStatus
	QuotaBytes         *int64
	PasswordHash       *string
	MustChangePassword *bool
}

// UpdateUser applies a partial update after re-checking the authority rules.
func (uc *UserUsecase) UpdateUser(ctx context.Context, in *UpdateUserInput) (*User, error) {
	if in == nil || in.ID == uuid.Nil {
		return nil, ErrInvalidArgument
	}
	actor := CallerFromContext(ctx)
	if err := actor.Require(PermUserManage); err != nil {
		return nil, err
	}
	var updated *User
	err := uc.tx.WithTx(ctx, func(ctx context.Context) error {
		target, err := uc.repo.FindUserByID(ctx, in.ID)
		if err != nil {
			return err
		}
		// The built-in accounts carry the authority the deployment was
		// configured with, so they are out of reach for everybody.
		if uc.IsBuiltin(target) {
			return ErrPermissionDenied
		}
		if err := CanManageUser(actor, target); err != nil {
			return err
		}
		if in.Nickname != nil {
			target.Nickname = *in.Nickname
		}
		if in.Email != nil {
			target.Email = *in.Email
		}
		if in.AvatarURL != nil {
			target.AvatarURL = *in.AvatarURL
		}
		if in.Remark != nil {
			target.Remark = *in.Remark
		}
		if in.Role != nil {
			if _, ok := RolePresetFor(*in.Role); !ok {
				return ErrInvalidArgument
			}
			if *in.Role == RoleAdmin && actor.Role != RoleAdmin {
				return ErrPermissionDenied
			}
			target.Role = *in.Role
		}
		if in.Rank != nil {
			if *in.Rank <= 0 || *in.Rank >= actor.Rank {
				return ErrPermissionDenied
			}
			target.Rank = *in.Rank
		}
		if in.Permissions != nil {
			if !actor.Perms.Has(*in.Permissions) {
				return ErrPermissionDenied
			}
			target.Permissions = *in.Permissions
		}
		if in.Status != nil {
			if *in.Status == UserStatusDeleted {
				return ErrInvalidArgument
			}
			target.Status = *in.Status
		}
		if in.QuotaBytes != nil {
			target.QuotaBytes = *in.QuotaBytes
		}
		if in.PasswordHash != nil {
			target.PasswordHash = *in.PasswordHash
		}
		if in.MustChangePassword != nil {
			target.MustChangePassword = *in.MustChangePassword
		}
		updated, err = uc.repo.UpdateUser(ctx, target)
		if err != nil {
			return err
		}
		if in.PasswordHash != nil || (in.Status != nil && *in.Status != UserStatusActive) {
			if err := uc.repo.RevokeUserSessions(ctx, target.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

// DeleteUser soft deletes an account and revokes its sessions.
func (uc *UserUsecase) DeleteUser(ctx context.Context, id uuid.UUID) error {
	if id == uuid.Nil {
		return ErrInvalidArgument
	}
	actor := CallerFromContext(ctx)
	if err := actor.Require(PermUserManage); err != nil {
		return err
	}
	if id == actor.UserID {
		return ErrPermissionDenied
	}
	return uc.tx.WithTx(ctx, func(ctx context.Context) error {
		target, err := uc.repo.FindUserByID(ctx, id)
		if err != nil {
			return err
		}
		if uc.IsBuiltin(target) {
			return ErrPermissionDenied
		}
		if err := CanManageUser(actor, target); err != nil {
			return err
		}
		if err := uc.repo.RevokeUserSessions(ctx, id); err != nil {
			return err
		}
		return uc.repo.DeleteUser(ctx, id)
	})
}

// SetPermissions replaces an account's permission set.
func (uc *UserUsecase) SetPermissions(ctx context.Context, id uuid.UUID, perms PermMask) (*User, error) {
	return uc.UpdateUser(ctx, &UpdateUserInput{ID: id, Permissions: &perms})
}

// UpdateSelf lets an account change its own profile fields. Only the fields a
// user is allowed to touch on itself are accepted here.
func (uc *UserUsecase) UpdateSelf(ctx context.Context, nickname, email, avatar string) (*User, error) {
	actor := CallerFromContext(ctx)
	if !actor.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	return uc.UpdateUser(ctx, &UpdateUserInput{
		ID:        actor.UserID,
		Nickname:  &nickname,
		Email:     &email,
		AvatarURL: &avatar,
	})
}

// TouchLogin records a successful login.
func (uc *UserUsecase) TouchLogin(ctx context.Context, id uuid.UUID, at time.Time) error {
	return uc.tx.WithTx(ctx, func(ctx context.Context) error {
		user, err := uc.repo.FindUserByID(ctx, id)
		if err != nil {
			return err
		}
		user.LastLoginAt = &at
		_, err = uc.repo.UpdateUser(ctx, user)
		return err
	})
}

// Names resolves display names for decoration purposes only. It deliberately
// skips the authority check because it exposes nothing but the label already
// visible on the nodes the caller may read.
func (uc *UserUsecase) Names(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string, len(ids))
	wanted := make([]uuid.UUID, 0, len(ids))
	seen := map[uuid.UUID]struct{}{}
	for _, id := range ids {
		if id == uuid.Nil {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		wanted = append(wanted, id)
	}
	if len(wanted) == 0 {
		return out, nil
	}
	users, err := uc.repo.FindUsersByIDs(ctx, wanted)
	if err != nil {
		return out, err
	}
	for _, u := range users {
		name := u.Nickname
		if name == "" {
			name = u.Username
		}
		out[u.ID] = name
	}
	return out, nil
}

// Stats returns the storage counters of an account.
func (uc *UserUsecase) Stats(ctx context.Context, id uuid.UUID) (*UserStats, error) {
	actor := CallerFromContext(ctx)
	if !actor.IsAuthenticated() {
		return nil, ErrUnauthenticated
	}
	if id == uuid.Nil || id == actor.UserID {
		id = actor.UserID
	} else if err := actor.Require(PermUserManage); err != nil {
		return nil, err
	}
	return uc.repo.UserStats(ctx, id)
}

// ChangePassword stores a new password hash for the caller.
func (uc *UserUsecase) ChangePassword(ctx context.Context, id uuid.UUID, hash string) error {
	if id == uuid.Nil || hash == "" {
		return ErrInvalidArgument
	}
	return uc.tx.WithTx(ctx, func(ctx context.Context) error {
		user, err := uc.repo.FindUserByID(ctx, id)
		if err != nil {
			return err
		}
		user.PasswordHash = hash
		user.MustChangePassword = false
		if _, err := uc.repo.UpdateUser(ctx, user); err != nil {
			return err
		}
		return uc.repo.RevokeUserSessions(ctx, id)
	})
}

// MinPasswordLength is the configured minimum password length.
func (uc *UserUsecase) MinPasswordLength() int { return uc.minPassword }

// CanManageUser enforces the authority order: a caller may only act on an
// account whose rank is strictly lower than its own, and the built-in
// administrator is out of reach for everybody.
func CanManageUser(actor *Caller, target *User) error {
	if actor == nil || !actor.IsAuthenticated() {
		return ErrUnauthenticated
	}
	if target == nil {
		return ErrNotFound
	}
	if target.ID == actor.UserID {
		return ErrPermissionDenied
	}
	if target.Rank >= actor.Rank {
		return ErrPermissionDenied
	}
	return nil
}
