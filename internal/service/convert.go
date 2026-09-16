package service

import (
	"strings"

	v1 "nagisa/api/netdisk/v1"
	"nagisa/internal/biz"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// This file holds the DTO to DO conversions shared by the netdisk services.
// Handlers stay free of repeated plumbing while each one still owns the shape
// of its own reply.

func convertRole(in biz.Role) v1.Role {
	switch in {
	case biz.RoleAdmin:
		return v1.Role_ROLE_ADMIN
	case biz.RoleManager:
		return v1.Role_ROLE_MANAGER
	case biz.RoleUser:
		return v1.Role_ROLE_USER
	case biz.RoleGuest:
		return v1.Role_ROLE_GUEST
	default:
		return v1.Role_ROLE_UNSPECIFIED
	}
}

func parseRole(in v1.Role) biz.Role {
	switch in {
	case v1.Role_ROLE_ADMIN:
		return biz.RoleAdmin
	case v1.Role_ROLE_MANAGER:
		return biz.RoleManager
	case v1.Role_ROLE_USER:
		return biz.RoleUser
	case v1.Role_ROLE_GUEST:
		return biz.RoleGuest
	default:
		return biz.RoleUnspecified
	}
}

func convertUserStatus(in biz.UserStatus) v1.UserStatus {
	switch in {
	case biz.UserStatusActive:
		return v1.UserStatus_USER_STATUS_ACTIVE
	case biz.UserStatusDisabled:
		return v1.UserStatus_USER_STATUS_DISABLED
	case biz.UserStatusDeleted:
		return v1.UserStatus_USER_STATUS_DELETED
	default:
		return v1.UserStatus_USER_STATUS_UNSPECIFIED
	}
}

func parseUserStatus(in v1.UserStatus) biz.UserStatus {
	switch in {
	case v1.UserStatus_USER_STATUS_ACTIVE:
		return biz.UserStatusActive
	case v1.UserStatus_USER_STATUS_DISABLED:
		return biz.UserStatusDisabled
	case v1.UserStatus_USER_STATUS_DELETED:
		return biz.UserStatusDeleted
	default:
		return biz.UserStatusUnspecified
	}
}

func convertNodeKind(in biz.NodeKind) v1.NodeKind {
	if in == biz.NodeKindFolder {
		return v1.NodeKind_NODE_KIND_FOLDER
	}
	if in == biz.NodeKindFile {
		return v1.NodeKind_NODE_KIND_FILE
	}
	return v1.NodeKind_NODE_KIND_UNSPECIFIED
}

func parseNodeKind(in v1.NodeKind) biz.NodeKind {
	switch in {
	case v1.NodeKind_NODE_KIND_FOLDER:
		return biz.NodeKindFolder
	case v1.NodeKind_NODE_KIND_FILE:
		return biz.NodeKindFile
	default:
		return biz.NodeKindUnspecified
	}
}

func convertNodeStatus(in biz.NodeStatus) v1.NodeStatus {
	switch in {
	case biz.NodeStatusActive:
		return v1.NodeStatus_NODE_STATUS_ACTIVE
	case biz.NodeStatusTrashed:
		return v1.NodeStatus_NODE_STATUS_TRASHED
	default:
		return v1.NodeStatus_NODE_STATUS_UNSPECIFIED
	}
}

func convertVisibility(in biz.Visibility) v1.Visibility {
	switch in {
	case biz.VisibilityPrivate:
		return v1.Visibility_VISIBILITY_PRIVATE
	case biz.VisibilityInternal:
		return v1.Visibility_VISIBILITY_INTERNAL
	case biz.VisibilityPublic:
		return v1.Visibility_VISIBILITY_PUBLIC
	default:
		return v1.Visibility_VISIBILITY_UNSPECIFIED
	}
}

func parseVisibility(in v1.Visibility) biz.Visibility {
	switch in {
	case v1.Visibility_VISIBILITY_PRIVATE:
		return biz.VisibilityPrivate
	case v1.Visibility_VISIBILITY_INTERNAL:
		return biz.VisibilityInternal
	case v1.Visibility_VISIBILITY_PUBLIC:
		return biz.VisibilityPublic
	default:
		return biz.VisibilityUnspecified
	}
}

func convertSubjectType(in biz.SubjectType) v1.SubjectType {
	switch in {
	case biz.SubjectTypeUser:
		return v1.SubjectType_SUBJECT_TYPE_USER
	case biz.SubjectTypeRole:
		return v1.SubjectType_SUBJECT_TYPE_ROLE
	case biz.SubjectTypeEveryone:
		return v1.SubjectType_SUBJECT_TYPE_EVERYONE
	default:
		return v1.SubjectType_SUBJECT_TYPE_UNSPECIFIED
	}
}

func parseSubjectType(in v1.SubjectType) biz.SubjectType {
	switch in {
	case v1.SubjectType_SUBJECT_TYPE_USER:
		return biz.SubjectTypeUser
	case v1.SubjectType_SUBJECT_TYPE_ROLE:
		return biz.SubjectTypeRole
	case v1.SubjectType_SUBJECT_TYPE_EVERYONE:
		return biz.SubjectTypeEveryone
	default:
		return biz.SubjectTypeUnspecified
	}
}

func convertEffect(in biz.Effect) v1.Effect {
	switch in {
	case biz.EffectAllow:
		return v1.Effect_EFFECT_ALLOW
	case biz.EffectDeny:
		return v1.Effect_EFFECT_DENY
	default:
		return v1.Effect_EFFECT_UNSPECIFIED
	}
}

func parseEffect(in v1.Effect) biz.Effect {
	switch in {
	case v1.Effect_EFFECT_ALLOW:
		return biz.EffectAllow
	case v1.Effect_EFFECT_DENY:
		return biz.EffectDeny
	default:
		return biz.EffectUnspecified
	}
}

func parseConflictPolicy(in v1.ConflictPolicy) biz.ConflictPolicy {
	switch in {
	case v1.ConflictPolicy_CONFLICT_POLICY_FAIL:
		return biz.ConflictPolicyFail
	case v1.ConflictPolicy_CONFLICT_POLICY_RENAME:
		return biz.ConflictPolicyRename
	case v1.ConflictPolicy_CONFLICT_POLICY_OVERWRITE:
		return biz.ConflictPolicyOverwrite
	default:
		return biz.ConflictPolicyUnspecified
	}
}

func convertConflictPolicy(in biz.ConflictPolicy) v1.ConflictPolicy {
	switch in {
	case biz.ConflictPolicyFail:
		return v1.ConflictPolicy_CONFLICT_POLICY_FAIL
	case biz.ConflictPolicyRename:
		return v1.ConflictPolicy_CONFLICT_POLICY_RENAME
	case biz.ConflictPolicyOverwrite:
		return v1.ConflictPolicy_CONFLICT_POLICY_OVERWRITE
	default:
		return v1.ConflictPolicy_CONFLICT_POLICY_UNSPECIFIED
	}
}

func convertUploadStatus(in biz.UploadStatus) v1.UploadStatus {
	switch in {
	case biz.UploadStatusPending:
		return v1.UploadStatus_UPLOAD_STATUS_PENDING
	case biz.UploadStatusInProgress:
		return v1.UploadStatus_UPLOAD_STATUS_IN_PROGRESS
	case biz.UploadStatusCompleted:
		return v1.UploadStatus_UPLOAD_STATUS_COMPLETED
	case biz.UploadStatusAborted:
		return v1.UploadStatus_UPLOAD_STATUS_ABORTED
	case biz.UploadStatusExpired:
		return v1.UploadStatus_UPLOAD_STATUS_EXPIRED
	default:
		return v1.UploadStatus_UPLOAD_STATUS_UNSPECIFIED
	}
}

func parseUploadStatus(in v1.UploadStatus) biz.UploadStatus {
	switch in {
	case v1.UploadStatus_UPLOAD_STATUS_PENDING:
		return biz.UploadStatusPending
	case v1.UploadStatus_UPLOAD_STATUS_IN_PROGRESS:
		return biz.UploadStatusInProgress
	case v1.UploadStatus_UPLOAD_STATUS_COMPLETED:
		return biz.UploadStatusCompleted
	case v1.UploadStatus_UPLOAD_STATUS_ABORTED:
		return biz.UploadStatusAborted
	case v1.UploadStatus_UPLOAD_STATUS_EXPIRED:
		return biz.UploadStatusExpired
	default:
		return biz.UploadStatusUnspecified
	}
}

func convertUploadMode(in biz.UploadMode) v1.UploadMode {
	switch in {
	case biz.UploadModePresigned:
		return v1.UploadMode_UPLOAD_MODE_PRESIGNED
	case biz.UploadModeProxy:
		return v1.UploadMode_UPLOAD_MODE_PROXY
	default:
		return v1.UploadMode_UPLOAD_MODE_UNSPECIFIED
	}
}

func parseUploadMode(in v1.UploadMode) biz.UploadMode {
	switch in {
	case v1.UploadMode_UPLOAD_MODE_PRESIGNED:
		return biz.UploadModePresigned
	case v1.UploadMode_UPLOAD_MODE_PROXY:
		return biz.UploadModeProxy
	default:
		return biz.UploadModeUnspecified
	}
}

func convertShareStatus(in biz.ShareStatus) v1.ShareStatus {
	switch in {
	case biz.ShareStatusActive:
		return v1.ShareStatus_SHARE_STATUS_ACTIVE
	case biz.ShareStatusExpired:
		return v1.ShareStatus_SHARE_STATUS_EXPIRED
	case biz.ShareStatusRevoked:
		return v1.ShareStatus_SHARE_STATUS_REVOKED
	default:
		return v1.ShareStatus_SHARE_STATUS_UNSPECIFIED
	}
}

func parseShareStatus(in v1.ShareStatus) biz.ShareStatus {
	switch in {
	case v1.ShareStatus_SHARE_STATUS_ACTIVE:
		return biz.ShareStatusActive
	case v1.ShareStatus_SHARE_STATUS_EXPIRED:
		return biz.ShareStatusExpired
	case v1.ShareStatus_SHARE_STATUS_REVOKED:
		return biz.ShareStatusRevoked
	default:
		return biz.ShareStatusUnspecified
	}
}

// parseUUID keeps the id parsing at the service boundary in one place: a
// client that sends a malformed identifier gets INVALID_ARGUMENT instead of a
// storage error.
func parseUUID(raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, biz.ErrInvalidArgument
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, biz.ErrInvalidArgument
	}
	return id, nil
}

// parseOptionalUUID treats an empty value as the nil id.
func parseOptionalUUID(raw string) (uuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return uuid.Nil, nil
	}
	return parseUUID(raw)
}

func parseUUIDs(raw []string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(raw))
	for _, r := range raw {
		id, err := parseUUID(r)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// convertPermissions renders a mask as both the enum list and the bitmask.
func convertPermissions(mask biz.PermMask) ([]v1.Permission, int64) {
	return mask.PermissionValues(), int64(mask)
}

// parsePermissions accepts either the enum list or the bitmask form.
func parsePermissions(perms []v1.Permission, mask int64) (biz.PermMask, bool) {
	if len(perms) > 0 {
		return biz.PermMaskFrom(perms), true
	}
	if mask != 0 {
		return biz.PermMaskFromRaw(mask), true
	}
	return biz.PermNone, false
}

// convertUser renders an account for a reply. Caller specific flags are filled
// by the caller of this helper.
func convertUser(in *biz.User) *v1.User {
	if in == nil {
		return nil
	}
	permissionValues, permissionMask := convertPermissions(in.Permissions)
	return &v1.User{
		Id:                 in.ID.String(),
		Username:           in.Username,
		Nickname:           in.Nickname,
		Email:              in.Email,
		AvatarUrl:          in.AvatarURL,
		Role:               convertRole(in.Role),
		Rank:               in.Rank,
		Permissions:        permissionValues,
		PermissionsMask:    permissionMask,
		Status:             convertUserStatus(in.Status),
		QuotaBytes:         in.QuotaBytes,
		UsedBytes:          in.UsedBytes,
		FileCount:          in.FileCount,
		FolderCount:        in.FolderCount,
		Remark:             in.Remark,
		MustChangePassword: in.MustChangePassword,
		LastLoginAt:        timestamppb.New(timeOrZero(in.LastLoginAt)),
		CreatedBy:          uuidOrEmpty(in.CreatedBy),
		CreatedAt:          timestamppb.New(in.CreatedAt),
		UpdatedAt:          timestamppb.New(in.UpdatedAt),
	}
}

// convertAclEntry renders one access control entry.
func convertAclEntry(in *biz.AclEntry, subjectName string) *v1.AclEntry {
	if in == nil {
		return nil
	}
	permissionValues, permissionMask := convertPermissions(in.Permissions)
	return &v1.AclEntry{
		Id:              uuidOrEmpty(in.ID),
		NodeId:          uuidOrEmpty(in.NodeID),
		SubjectType:     convertSubjectType(in.SubjectType),
		SubjectId:       in.SubjectID,
		SubjectName:     subjectName,
		Effect:          convertEffect(in.Effect),
		Permissions:     permissionValues,
		PermissionsMask: permissionMask,
		Inherit:         in.Inherit,
		CreatedAt:       timestamppb.New(in.CreatedAt),
		CreatedBy:       uuidOrEmpty(in.CreatedBy),
	}
}

// convertNodeOwned renders one node for a reply. The access argument carries
// the caller specific flags, displayPath the human readable location.
func convertNodeOwned(node *biz.Node, access biz.NodeAccess, displayPath, ownerName string, shareCount int32) *v1.Node {
	if node == nil {
		return nil
	}
	permissionValues, permissionMask := convertPermissions(access.Perms)
	out := &v1.Node{
		Id:                       uuidOrEmpty(node.ID),
		ParentId:                 uuidOrEmpty(node.ParentID),
		Name:                     node.Name,
		Kind:                     convertNodeKind(node.Kind),
		OwnerId:                  uuidOrEmpty(node.OwnerID),
		OwnerName:                ownerName,
		Size:                     node.Size,
		MimeType:                 node.MimeType,
		Extension:                node.Extension,
		Etag:                     node.Etag,
		Status:                   convertNodeStatus(node.Status),
		Description:              node.Description,
		Visibility:               convertVisibility(node.Visibility),
		PasswordProtected:        node.PasswordHash != "",
		PasswordHint:             node.PasswordHint,
		Locked:                   access.Locked,
		HasThumbnail:             node.HasThumbnail,
		Metadata:                 node.Metadata,
		Path:                     node.Path,
		DisplayPath:              displayPath,
		Depth:                    node.Depth,
		ChildCount:               node.ChildCount,
		FileCount:                node.FileCount,
		FolderCount:              node.FolderCount,
		SubtreeSize:              node.SubtreeSize,
		EffectivePermissions:     permissionValues,
		EffectivePermissionsMask: permissionMask,
		Owned:                    access.Owned,
		ShareCount:               shareCount,
		Shared:                   shareCount > 0,
		VersionCount:             node.VersionCount,
		CreatedAt:                timestamppb.New(node.CreatedAt),
		UpdatedAt:                timestamppb.New(node.UpdatedAt),
		CreatedBy:                uuidOrEmpty(node.CreatedBy),
		UpdatedBy:                uuidOrEmpty(node.UpdatedBy),
		OriginalParentId:         uuidOrEmpty(node.OriginalParentID),
	}
	if node.CurrentVersionID != nil {
		out.CurrentVersionId = node.CurrentVersionID.String()
	}
	if node.TrashedAt != nil {
		out.TrashedAt = timestamppb.New(*node.TrashedAt)
	}
	return out
}

// convertUploadPart renders one stored part, including the presigned URL when
// the session uses the direct transport.
func convertUploadPart(in *biz.UploadPart) *v1.UploadPart {
	if in == nil {
		return nil
	}
	out := &v1.UploadPart{
		PartNumber: in.PartNumber,
		Size:       in.Size,
		Etag:       in.Etag,
		CreatedAt:  timestamppb.New(in.CreatedAt),
	}
	if in.Presign != nil {
		out.UploadUrl = in.Presign.URL
		out.Headers = in.Presign.Headers
		out.UrlExpiresAt = timestamppb.New(in.Presign.ExpiresAt)
	}
	return out
}

// convertUpload renders a session together with its parts.
func convertUpload(in *biz.Upload, parts []*biz.UploadPart) *v1.UploadSession {
	if in == nil {
		return nil
	}
	out := &v1.UploadSession{
		Id:             uuidOrEmpty(in.ID),
		ParentId:       uuidOrEmpty(in.ParentID),
		Name:           in.Name,
		Kind:           v1.NodeKind_NODE_KIND_FILE,
		Size:           in.Size,
		MimeType:       in.MimeType,
		ChunkSize:      in.ChunkSize,
		TotalParts:     in.TotalParts,
		Status:         convertUploadStatus(in.Status),
		Mode:           convertUploadMode(in.Mode),
		ConflictPolicy: convertConflictPolicy(in.Policy),
		ReceivedBytes:  in.ReceivedBytes,
		LastError:      in.LastError,
		CreatedAt:      timestamppb.New(in.CreatedAt),
		UpdatedAt:      timestamppb.New(in.UpdatedAt),
		ExpiresAt:      timestamppb.New(in.ExpiresAt),
	}
	if in.NodeID != nil {
		out.NodeId = in.NodeID.String()
	}
	out.UploadedParts = make([]int32, 0, len(parts))
	out.Parts = make([]*v1.UploadPart, 0, len(parts))
	for _, p := range parts {
		if p.Etag != "" {
			out.UploadedParts = append(out.UploadedParts, p.PartNumber)
		}
		out.Parts = append(out.Parts, convertUploadPart(p))
	}
	return out
}

// convertVersion renders one retained revision.
func convertVersion(in *biz.NodeVersion, currentStorageKey string) *v1.NodeVersion {
	if in == nil {
		return nil
	}
	return &v1.NodeVersion{
		Id:        uuidOrEmpty(in.ID),
		NodeId:    uuidOrEmpty(in.NodeID),
		Version:   in.Version,
		Size:      in.Size,
		MimeType:  in.MimeType,
		Etag:      in.Etag,
		Comment:   in.Comment,
		Current:   currentStorageKey != "" && in.StorageKey == currentStorageKey,
		CreatedBy: uuidOrEmpty(in.CreatedBy),
		CreatedAt: timestamppb.New(in.CreatedAt),
	}
}

// convertShare renders a share link.
func convertShare(in *biz.Share, url string, editable bool, node *v1.Node) *v1.Share {
	if in == nil {
		return nil
	}
	permissionValues, permissionMask := convertPermissions(in.Permissions)
	out := &v1.Share{
		Id:                uuidOrEmpty(in.ID),
		Token:             in.Token,
		NodeId:            uuidOrEmpty(in.NodeID),
		Node:              node,
		OwnerId:           uuidOrEmpty(in.OwnerID),
		Name:              in.Name,
		Description:       in.Description,
		Permissions:       permissionValues,
		PermissionsMask:   permissionMask,
		PasswordProtected: in.PasswordHash != "",
		PasswordHint:      in.PasswordHint,
		MaxDownloads:      in.MaxDownloads,
		DownloadCount:     in.DownloadCount,
		ViewCount:         in.ViewCount,
		Status:            convertShareStatus(in.Status),
		Url:               url,
		Editable:          editable,
		CreatedAt:         timestamppb.New(in.CreatedAt),
		UpdatedAt:         timestamppb.New(in.UpdatedAt),
		CreatedBy:         uuidOrEmpty(in.CreatedBy),
	}
	if in.ExpiresAt != nil {
		out.ExpiresAt = timestamppb.New(*in.ExpiresAt)
	}
	return out
}

// convertSignedURL renders a signed request for a client.
func convertSignedURL(in *biz.SignedURL) *v1.SignedUrl {
	if in == nil {
		return nil
	}
	return &v1.SignedUrl{
		Url:       in.URL,
		Method:    in.Method,
		ExpiresAt: timestamppb.New(in.ExpiresAt),
		ExpiresIn: int32(timeUntil(in.ExpiresAt).Seconds()),
		Headers:   in.Headers,
		NodeId:    uuidOrEmpty(in.NodeID),
		Size:      in.Size,
		FileName:  in.FileName,
		MimeType:  in.MimeType,
	}
}

// convertAuditLog renders one audit entry.
func convertAuditLog(in *biz.AuditLog) *v1.AuditLog {
	if in == nil {
		return nil
	}
	return &v1.AuditLog{
		Id:            uuidOrEmpty(in.ID),
		ActorId:       uuidOrEmpty(in.ActorID),
		ActorName:     in.ActorName,
		Action:        in.Action,
		ActionDisplay: biz.ActionLabel(in.Action),
		TargetType:    in.TargetType,
		TargetId:      in.TargetID,
		TargetName:    in.TargetName,
		Success:       in.Success,
		ErrorReason:   in.ErrorReason,
		Detail:        in.Detail,
		Ip:            in.IP,
		UserAgent:     in.UserAgent,
		RequestId:     in.RequestID,
		CreatedAt:     timestamppb.New(in.CreatedAt),
	}
}

// convertSession renders one refresh session.
func convertSession(in *biz.Session) *v1.Session {
	if in == nil {
		return nil
	}
	out := &v1.Session{
		Id:        uuidOrEmpty(in.ID),
		Ip:        in.IP,
		UserAgent: in.UserAgent,
		CreatedAt: timestamppb.New(in.CreatedAt),
		ExpiresAt: timestamppb.New(in.ExpiresAt),
		Active:    in.Active(nowFunc()),
	}
	if in.LastUsedAt != nil {
		out.LastUsedAt = timestamppb.New(*in.LastUsedAt)
	}
	return out
}

// uuidOrEmpty renders a uuid, mapping the nil value onto an empty string so
// the JSON reply stays readable.
func uuidOrEmpty(id uuid.UUID) string {
	if id == uuid.Nil {
		return ""
	}
	return id.String()
}
