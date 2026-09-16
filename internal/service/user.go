package service

import (
	"context"
	"strings"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"

	"github.com/google/uuid"
	"go.einride.tech/aip/fieldmask"
	"go.einride.tech/aip/filtering"
	"google.golang.org/protobuf/types/known/emptypb"
)

// maxUsersPageSize is the page size cap documented by ListUsersRequest.
const maxUsersPageSize = 200

// UserService serves accounts, their permission sets and their quota.
type UserService struct {
	v1.UnimplementedUserServiceServer

	uc    *biz.UserUsecase
	auth  *biz.AuthUsecase
	nodes *biz.NodeUsecase
}

// NewUserService returns an account service adapter.
func NewUserService(uc *biz.UserUsecase, auth *biz.AuthUsecase, nodes *biz.NodeUsecase) *UserService {
	return &UserService{uc: uc, auth: auth, nodes: nodes}
}

// CreateUser provisions an account.
func (s *UserService) CreateUser(ctx context.Context, req *v1.CreateUserRequest) (*v1.User, error) {
	caller := biz.CallerFromContext(ctx)
	in := req.GetUser()
	if in == nil || strings.TrimSpace(in.GetUsername()) == "" || req.GetPassword() == "" {
		return nil, biz.ErrInvalidArgument
	}
	plain, err := s.auth.DecodePassword(req.GetPassword())
	if err != nil {
		return nil, err
	}
	hash, err := s.auth.HashPassword(plain)
	if err != nil {
		return nil, err
	}
	perms, permsSet := parsePermissions(in.GetPermissions(), in.GetPermissionsMask())
	created, err := s.uc.CreateUser(ctx, &biz.CreateUserInput{
		Username:           in.GetUsername(),
		Nickname:           in.GetNickname(),
		Email:              in.GetEmail(),
		AvatarURL:          in.GetAvatarUrl(),
		Role:               parseRole(in.GetRole()),
		Rank:               in.GetRank(),
		Permissions:        perms,
		PermissionsSet:     permsSet,
		QuotaBytes:         in.GetQuotaBytes(),
		Remark:             in.GetRemark(),
		PasswordHash:       hash,
		Enabled:            in.GetStatus() != v1.UserStatus_USER_STATUS_DISABLED,
		MustChangePassword: req.GetMustChangePassword(),
	})
	if err != nil {
		return nil, err
	}
	return decorateUser(convertUser(created), caller, created), nil
}

// GetUser returns one account.
func (s *UserService) GetUser(ctx context.Context, req *v1.GetUserRequest) (*v1.User, error) {
	caller := biz.CallerFromContext(ctx)
	id, err := resolveUserID(req.GetId(), caller)
	if err != nil {
		return nil, err
	}
	user, err := s.uc.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	return decorateUser(convertUser(user), caller, user), nil
}

// ListUsers returns a page of accounts.
func (s *UserService) ListUsers(ctx context.Context, req *v1.ListUsersRequest) (*v1.UserSet, error) {
	caller := biz.CallerFromContext(ctx)
	decl, err := declarations(
		filtering.DeclareIdent("username", filtering.TypeString),
		filtering.DeclareIdent("nickname", filtering.TypeString),
		filtering.DeclareIdent("email", filtering.TypeString),
		filtering.DeclareIdent("role", filtering.TypeInt),
		filtering.DeclareIdent("status", filtering.TypeInt),
		filtering.DeclareIdent("rank", filtering.TypeInt),
		filtering.DeclareIdent("created_at", filtering.TypeTimestamp),
		filtering.DeclareIdent("updated_at", filtering.TypeTimestamp),
		filtering.DeclareIdent("last_login_at", filtering.TypeTimestamp),
	)
	if err != nil {
		return nil, err
	}
	opts, size, token, err := parseList(req, decl, maxUsersPageSize,
		"username", "nickname", "role", "rank", "status", "used_bytes",
		"created_at", "updated_at", "last_login_at")
	if err != nil {
		return nil, err
	}
	users, total, err := s.uc.ListUsers(ctx, opts...)
	if err != nil {
		return nil, err
	}
	out := &v1.UserSet{
		Users:     make([]*v1.User, 0, len(users)),
		TotalSize: total,
	}
	for _, user := range users {
		out.Users = append(out.Users, decorateUser(convertUser(user), caller, user))
	}
	out.NextPageToken = nextPageToken(req, token, len(users), int(size))
	return out, nil
}

// UpdateUser applies a partial update to one account.
func (s *UserService) UpdateUser(ctx context.Context, req *v1.UpdateUserRequest) (*v1.User, error) {
	caller := biz.CallerFromContext(ctx)
	patch := req.GetUser()
	if patch == nil || len(req.GetUpdateMask().GetPaths()) == 0 {
		return nil, biz.ErrInvalidArgument
	}
	id, err := parseUUID(patch.GetId())
	if err != nil {
		return nil, err
	}
	current, err := s.uc.GetUser(ctx, id)
	if err != nil {
		return nil, err
	}
	// The stored account carries the values of every field the client left
	// out, so the mask is applied before the result is translated.
	merged := convertUser(current)
	fieldmask.Update(req.GetUpdateMask(), merged, patch)

	in := &biz.UpdateUserInput{ID: id}
	for _, path := range req.GetUpdateMask().GetPaths() {
		switch path {
		case "nickname":
			value := merged.GetNickname()
			in.Nickname = &value
		case "email":
			value := merged.GetEmail()
			in.Email = &value
		case "avatar_url":
			value := merged.GetAvatarUrl()
			in.AvatarURL = &value
		case "remark":
			value := merged.GetRemark()
			in.Remark = &value
		case "role":
			value := parseRole(merged.GetRole())
			in.Role = &value
		case "rank":
			value := merged.GetRank()
			in.Rank = &value
		case "permissions_mask":
			value, _ := parsePermissions(patch.GetPermissions(), merged.GetPermissionsMask())
			in.Permissions = &value
		case "status":
			value := parseUserStatus(merged.GetStatus())
			if value == biz.UserStatusUnspecified {
				return nil, biz.ErrInvalidArgument
			}
			in.Status = &value
		case "quota_bytes":
			value := merged.GetQuotaBytes()
			in.QuotaBytes = &value
		default:
			return nil, biz.ErrInvalidArgument
		}
	}
	updated, err := s.uc.UpdateUser(ctx, in)
	if err != nil {
		return nil, err
	}
	return decorateUser(convertUser(updated), caller, updated), nil
}

// DeleteUser soft deletes an account.
func (s *UserService) DeleteUser(ctx context.Context, req *v1.DeleteUserRequest) (*emptypb.Empty, error) {
	caller := biz.CallerFromContext(ctx)
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	if caller.IsAuthenticated() && id == caller.UserID {
		return nil, biz.ErrPermissionDenied
	}
	if err := s.uc.DeleteUser(ctx, id); err != nil {
		return nil, err
	}
	if req.GetTrashNodes() {
		// Deleting an account keeps its rows so the trail stays readable, so
		// the content is moved to the trash on request instead of being left
		// reachable in place.
		if _, err := s.nodes.TrashAllForOwner(ctx, id); err != nil {
			return nil, err
		}
	}
	return &emptypb.Empty{}, nil
}

// SetUserPermissions replaces an account's permission set.
func (s *UserService) SetUserPermissions(ctx context.Context, req *v1.SetUserPermissionsRequest) (*v1.User, error) {
	caller := biz.CallerFromContext(ctx)
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	perms, _ := parsePermissions(req.GetPermissions(), req.GetPermissionsMask())
	updated, err := s.uc.SetPermissions(ctx, id, perms)
	if err != nil {
		return nil, err
	}
	return decorateUser(convertUser(updated), caller, updated), nil
}

// ResetUserPassword sets a new password for another account.
func (s *UserService) ResetUserPassword(ctx context.Context, req *v1.ResetUserPasswordRequest) (*emptypb.Empty, error) {
	id, err := parseUUID(req.GetId())
	if err != nil {
		return nil, err
	}
	plain, err := s.auth.DecodePassword(req.GetPassword())
	if err != nil {
		return nil, err
	}
	hash, err := s.auth.HashPassword(plain)
	if err != nil {
		return nil, err
	}
	flag := req.GetMustChangePassword()
	if _, err := s.uc.UpdateUser(ctx, &biz.UpdateUserInput{
		ID:                 id,
		PasswordHash:       &hash,
		MustChangePassword: &flag,
	}); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ListRolePresets returns the built-in permission templates.
func (s *UserService) ListRolePresets(ctx context.Context, _ *v1.ListRolePresetsRequest) (*v1.RolePresetSet, error) {
	caller := biz.CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, biz.ErrUnauthenticated
	}
	presets := biz.RolePresets()
	out := &v1.RolePresetSet{
		Presets:          make([]*v1.RolePreset, 0, len(presets)),
		MaxGrantableRank: maxGrantableRank(caller),
		CallerRank:       caller.Rank,
	}
	for _, preset := range presets {
		values, mask := convertPermissions(preset.Perms)
		out.Presets = append(out.Presets, &v1.RolePreset{
			Role:            convertRole(preset.Role),
			Name:            preset.Name,
			DisplayName:     preset.Display,
			Description:     preset.Desc,
			DefaultRank:     preset.Rank,
			Permissions:     values,
			PermissionsMask: mask,
		})
	}
	return out, nil
}

// ListPermissionCatalog returns every permission with its label.
func (s *UserService) ListPermissionCatalog(ctx context.Context, _ *v1.ListPermissionCatalogRequest) (*v1.PermissionCatalog, error) {
	caller := biz.CallerFromContext(ctx)
	if !caller.IsAuthenticated() {
		return nil, biz.ErrUnauthenticated
	}
	entries := biz.PermissionEntries()
	granted, mask := convertPermissions(caller.Perms)
	out := &v1.PermissionCatalog{
		Permissions: make([]*v1.PermissionInfo, 0, len(entries)),
		Granted:     granted,
		GrantedMask: mask,
	}
	for _, entry := range entries {
		out.Permissions = append(out.Permissions, &v1.PermissionInfo{
			Permission:  entry.Permission,
			Name:        entry.Name,
			DisplayName: entry.Display,
			Description: entry.Description,
			Category:    entry.Category,
		})
	}
	return out, nil
}

// GetUserStats returns the storage counters of one account.
func (s *UserService) GetUserStats(ctx context.Context, req *v1.GetUserStatsRequest) (*v1.UserStats, error) {
	caller := biz.CallerFromContext(ctx)
	id, err := resolveUserID(req.GetId(), caller)
	if err != nil {
		return nil, err
	}
	stats, err := s.uc.Stats(ctx, id)
	if err != nil {
		return nil, err
	}
	return &v1.UserStats{
		UserId:       uuidOrEmpty(stats.UserID),
		Username:     stats.Username,
		QuotaBytes:   stats.QuotaBytes,
		UsedBytes:    stats.UsedBytes,
		FileCount:    stats.FileCount,
		FolderCount:  stats.FolderCount,
		TrashedBytes: stats.TrashedBytes,
		TrashedCount: stats.TrashedCount,
		UsageRatio:   stats.UsageRatio,
	}, nil
}

// decorateUser fills the flags that depend on the calling account. A built-in
// account is never manageable: its authority comes from the configuration.
func decorateUser(msg *v1.User, caller *biz.Caller, target *biz.User) *v1.User {
	if msg == nil || caller == nil {
		return msg
	}
	locked := target != nil && target.Locked
	msg.Manageable = !locked && biz.CanManageUser(caller, target) == nil
	msg.PermissionsEditable = !locked && caller.Has(biz.PermUserManage) && target != nil && target.Rank < caller.Rank
	msg.MaxGrantableRank = maxGrantableRank(caller)
	return msg
}

// maxGrantableRank is the exclusive upper bound the caller may assign to rank.
func maxGrantableRank(caller *biz.Caller) int32 {
	if caller == nil || caller.Rank <= 0 {
		return 0
	}
	return caller.Rank - 1
}

// resolveUserID resolves the "me" alias to the calling account.
func resolveUserID(raw string, caller *biz.Caller) (uuid.UUID, error) {
	if strings.EqualFold(strings.TrimSpace(raw), "me") {
		if !caller.IsAuthenticated() {
			return uuid.Nil, biz.ErrUnauthenticated
		}
		return caller.UserID, nil
	}
	return parseUUID(raw)
}
