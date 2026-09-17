package biz

import (
	"strings"

	v1 "nagisa/api/netdisk/v1"

	"github.com/go-kratos/kratos/v3/errors"
)

// Typed errors. The reason string is taken from the api enum so clients can
// branch on it without matching messages.
var (
	ErrNotFound           = errors.NotFound(v1.ErrorReason_NETDISK_NOT_FOUND.String(), "resource not found")
	ErrInvalidArgument    = errors.BadRequest(v1.ErrorReason_NETDISK_INVALID_ARGUMENT.String(), "invalid argument")
	ErrUnauthenticated    = errors.Unauthorized(v1.ErrorReason_NETDISK_UNAUTHENTICATED.String(), "authentication required")
	ErrPermissionDenied   = errors.Forbidden(v1.ErrorReason_NETDISK_PERMISSION_DENIED.String(), "permission denied")
	ErrConflict           = errors.Conflict(v1.ErrorReason_NETDISK_CONFLICT.String(), "conflict")
	ErrInternal           = errors.InternalServer(v1.ErrorReason_NETDISK_INTERNAL.String(), "internal error")
	ErrUnavailable        = errors.ServiceUnavailable(v1.ErrorReason_NETDISK_UNAVAILABLE.String(), "service unavailable")
	ErrQuotaExceeded      = errors.Forbidden(v1.ErrorReason_NETDISK_QUOTA_EXCEEDED.String(), "storage quota exceeded")
	ErrStorage            = errors.InternalServer(v1.ErrorReason_NETDISK_STORAGE_ERROR.String(), "object storage error")
	ErrNodeLocked         = errors.Forbidden(v1.ErrorReason_NETDISK_NODE_LOCKED.String(), "node is password protected")
	ErrNameConflict       = errors.Conflict(v1.ErrorReason_NETDISK_NAME_CONFLICT.String(), "name already exists in the target folder")
	ErrCycleDetected      = errors.BadRequest(v1.ErrorReason_NETDISK_CYCLE_DETECTED.String(), "cannot move a folder into its own subtree")
	ErrUploadIncomplete   = errors.BadRequest(v1.ErrorReason_NETDISK_UPLOAD_INCOMPLETE.String(), "upload is incomplete")
	ErrUploadExpired      = errors.BadRequest(v1.ErrorReason_NETDISK_UPLOAD_EXPIRED.String(), "upload session has expired")
	ErrTooLarge           = errors.BadRequest(v1.ErrorReason_NETDISK_TOO_LARGE.String(), "payload too large")
	ErrUnsupported        = errors.BadRequest(v1.ErrorReason_NETDISK_UNSUPPORTED.String(), "operation not supported")
	ErrResourceExhausted  = errors.TooManyRequests(v1.ErrorReason_NETDISK_RESOURCE_EXHAUSTED.String(), "resource exhausted")
	ErrPreconditionFailed = errors.BadRequest(v1.ErrorReason_NETDISK_PRECONDITION_FAILED.String(), "precondition failed")
	ErrAlreadyExists      = errors.Conflict(v1.ErrorReason_NETDISK_ALREADY_EXISTS.String(), "resource already exists")
	ErrAborted            = errors.BadRequest(v1.ErrorReason_NETDISK_ABORTED.String(), "operation aborted")
	ErrAccountDisabled    = errors.Forbidden(v1.ErrorReason_NETDISK_ACCOUNT_DISABLED.String(), "account is disabled")
)

// InvalidArgument reports a rejected request field. The message names the
// field and the rule it broke, so a client can show something better than the
// generic "invalid argument" that the reason enum carries.
func InvalidArgument(message string) error {
	return errors.BadRequest(v1.ErrorReason_NETDISK_INVALID_ARGUMENT.String(), message)
}

// NodeLockedError reports that a node needs a password. The hint, when one is
// configured, travels in the error metadata so a client can prompt without a
// second round trip, and nothing else about the node is disclosed.
func NodeLockedError(hint string) error {
	if hint == "" {
		return ErrNodeLocked
	}
	return errors.Forbidden(
		v1.ErrorReason_NETDISK_NODE_LOCKED.String(),
		"node is password protected",
	).WithMetadata(map[string]string{"password_hint": hint})
}

// Role is the coarse account category. Values match the api enum.
type Role int32

// Account roles.
const (
	RoleUnspecified Role = 0
	RoleAdmin       Role = 1
	RoleManager     Role = 2
	RoleUser        Role = 3
	RoleGuest       Role = 4
)

// Ranks. Authority is a strict order: a caller may only manage accounts with a
// strictly lower rank.
const (
	RankAdmin   int32 = 1000
	RankManager int32 = 500
	RankUser    int32 = 100
	RankGuest   int32 = 10
)

// UserStatus is the account lifecycle state. Values match the api enum.
type UserStatus int32

// Account lifecycle states.
const (
	UserStatusUnspecified UserStatus = 0
	UserStatusActive      UserStatus = 1
	UserStatusDisabled    UserStatus = 2
	UserStatusDeleted     UserStatus = 3
)

// NodeKind distinguishes folders from files. Values match the api enum.
type NodeKind int32

// Node kinds.
const (
	NodeKindUnspecified NodeKind = 0
	NodeKindFolder      NodeKind = 1
	NodeKindFile        NodeKind = 2
)

// NodeStatus is the node lifecycle state. Values match the api enum.
type NodeStatus int32

// Node lifecycle states.
const (
	NodeStatusUnspecified NodeStatus = 0
	NodeStatusActive      NodeStatus = 1
	NodeStatusTrashed     NodeStatus = 2
)

// Visibility is the coarse access policy. Values match the api enum.
type Visibility int32

// Visibility levels, ordered from the most restrictive to the most open.
const (
	VisibilityUnspecified Visibility = 0
	VisibilityPrivate     Visibility = 1
	VisibilityInternal    Visibility = 2
	VisibilityPublic      Visibility = 3
)

// SubjectType selects what an ACL entry applies to. Values match the api enum.
type SubjectType int32

// ACL subject types.
const (
	SubjectTypeUnspecified SubjectType = 0
	SubjectTypeUser        SubjectType = 1
	SubjectTypeRole        SubjectType = 2
	SubjectTypeEveryone    SubjectType = 3
)

// Effect is the outcome of a matching ACL entry. Values match the api enum.
type Effect int32

// ACL effects.
const (
	EffectUnspecified Effect = 0
	EffectAllow       Effect = 1
	EffectDeny        Effect = 2
)

// ConflictPolicy decides what happens when a name is already taken.
type ConflictPolicy int32

// Conflict policies.
const (
	ConflictPolicyUnspecified ConflictPolicy = 0
	ConflictPolicyFail        ConflictPolicy = 1
	ConflictPolicyRename      ConflictPolicy = 2
	ConflictPolicyOverwrite   ConflictPolicy = 3
)

// UploadStatus is the multipart upload lifecycle state.
type UploadStatus int32

// Upload lifecycle states.
const (
	UploadStatusUnspecified UploadStatus = 0
	UploadStatusPending     UploadStatus = 1
	UploadStatusInProgress  UploadStatus = 2
	UploadStatusCompleted   UploadStatus = 3
	UploadStatusAborted     UploadStatus = 4
	UploadStatusExpired     UploadStatus = 5
)

// UploadMode selects how part payloads reach the server.
type UploadMode int32

// Upload transports.
const (
	UploadModeUnspecified UploadMode = 0
	UploadModePresigned   UploadMode = 1
	UploadModeProxy       UploadMode = 2
)

// ShareStatus is the share link lifecycle state.
type ShareStatus int32

// Share lifecycle states.
const (
	ShareStatusUnspecified ShareStatus = 0
	ShareStatusActive      ShareStatus = 1
	ShareStatusExpired     ShareStatus = 2
	ShareStatusRevoked     ShareStatus = 3
)

// PermMask is a bitmask of permissions. Bit i is set when the permission with
// api enum value i+1 is granted.
type PermMask int64

// PermNone grants nothing.
const PermNone PermMask = 0

// Permission bits. Bit i is set when the api Permission enum value i+1 is
// granted, which is the layout permBit produces and the layout the API
// exposes as permissions_mask. The iota therefore has to start at zero in its
// own block.
const (
	PermView PermMask = 1 << iota
	PermDownload
	PermUpload
	PermEdit
	PermDelete
	PermTrashManage
	PermShare
	PermAclManage
	PermUserManage
	PermAuditRead
	PermStorageManage
	PermSystemManage
)

// PermAll is the union of every permission.
const PermAll = PermView | PermDownload | PermUpload | PermEdit | PermDelete |
	PermTrashManage | PermShare | PermAclManage | PermUserManage |
	PermAuditRead | PermStorageManage | PermSystemManage

// PermNodeScope is the set of permissions that a node ACL can carry. The
// account level permissions that govern administration are deliberately
// excluded, so an ACL can never escalate an account into an administrator.
const PermNodeScope = PermView | PermDownload | PermUpload | PermEdit |
	PermDelete | PermTrashManage | PermShare | PermAclManage

// PermShareScope is the set of permissions a share link can convey.
const PermShareScope = PermView | PermDownload | PermUpload

// permBit maps an api enum value to its bit.
func permBit(p v1.Permission) PermMask {
	if p <= v1.Permission_PERMISSION_UNSPECIFIED || int(p) > 63 {
		return 0
	}
	return PermMask(1) << (int(p) - 1)
}

// Has reports whether every bit of want is present.
func (m PermMask) Has(want PermMask) bool {
	return m&want == want
}

// HasAny reports whether at least one bit of want is present.
func (m PermMask) HasAny(want PermMask) bool {
	return m&want != 0
}

// Add returns the union of m and other.
func (m PermMask) Add(other PermMask) PermMask {
	return m | other
}

// Remove returns m without the bits of other.
func (m PermMask) Remove(other PermMask) PermMask {
	return m &^ other
}

// And returns the intersection of m and other.
func (m PermMask) And(other PermMask) PermMask {
	return m & other
}

// IsZero reports whether no permission is granted.
func (m PermMask) IsZero() bool {
	return m == 0
}

// PermMaskFrom converts api permissions into a bitmask, silently ignoring
// values outside the enum.
func PermMaskFrom(perms []v1.Permission) PermMask {
	var mask PermMask
	for _, p := range perms {
		mask |= permBit(p)
	}
	return mask
}

// PermMaskFromRaw takes a bitmask that already uses the storage layout.
func PermMaskFromRaw(raw int64) PermMask {
	return PermMask(raw) & PermAll
}

// PermissionNames lists the api permission names carried by the mask, in enum
// order, so a reply can round-trip the `permissions` field.
func (m PermMask) PermissionNames() []string {
	names := make([]string, 0, 12)
	for _, p := range permissionCatalog {
		if m.Has(permBit(p.Permission)) {
			names = append(names, p.Name)
		}
	}
	return names
}

// PermissionValues flattens the mask into api enum values.
func (m PermMask) PermissionValues() []v1.Permission {
	values := make([]v1.Permission, 0, 12)
	for _, p := range permissionCatalog {
		if m.Has(permBit(p.Permission)) {
			values = append(values, p.Permission)
		}
	}
	return values
}

// permissionCatalog drives both the api catalogue reply and the mask helpers.
var permissionCatalog = []struct {
	Permission  v1.Permission
	Name        string
	Display     string
	Description string
	Category    string
}{
	{v1.Permission_PERMISSION_VIEW, "view", "查看", "浏览目录、读取文件与文件夹的元信息", "content"},
	{v1.Permission_PERMISSION_DOWNLOAD, "download", "下载", "下载原始文件与预览内容", "content"},
	{v1.Permission_PERMISSION_UPLOAD, "upload", "上传", "上传文件与新建文件夹", "content"},
	{v1.Permission_PERMISSION_EDIT, "edit", "编辑", "重命名、移动、修改描述与元信息", "content"},
	{v1.Permission_PERMISSION_DELETE, "delete", "删除", "将文件或文件夹移入回收站", "content"},
	{v1.Permission_PERMISSION_TRASH_MANAGE, "trash_manage", "回收站管理", "还原或彻底清除回收站内容", "content"},
	{v1.Permission_PERMISSION_SHARE, "share", "分享", "创建与管理分享链接", "content"},
	{v1.Permission_PERMISSION_ACL_MANAGE, "acl_manage", "权限管理", "设置文件夹描述、可见范围、访问名单与密码", "content"},
	{v1.Permission_PERMISSION_USER_MANAGE, "user_manage", "账号管理", "创建、修改、禁用、删除账号并分配权限", "admin"},
	{v1.Permission_PERMISSION_AUDIT_READ, "audit_read", "审计日志", "查看操作审计记录", "admin"},
	{v1.Permission_PERMISSION_STORAGE_MANAGE, "storage_manage", "存储管理", "查看全局存储统计并执行维护任务", "admin"},
	{v1.Permission_PERMISSION_SYSTEM_MANAGE, "system_manage", "系统管理", "修改系统级运行参数", "admin"},
}

// PermissionEntry describes one permission for client rendering.
type PermissionEntry struct {
	Permission  v1.Permission
	Name        string
	Display     string
	Description string
	Category    string
}

// PermissionEntries returns the catalogue in enum order.
func PermissionEntries() []PermissionEntry {
	out := make([]PermissionEntry, 0, len(permissionCatalog))
	for _, p := range permissionCatalog {
		out = append(out, PermissionEntry{
			Permission:  p.Permission,
			Name:        p.Name,
			Display:     p.Display,
			Description: p.Description,
			Category:    p.Category,
		})
	}
	return out
}

// RolePreset is a permission template offered when an account is created.
type RolePreset struct {
	Role      Role
	Name      string
	Display   string
	Desc      string
	Rank      int32
	Perms     PermMask
	Manageble bool
}

// rolePresets lists the built-in templates.
var rolePresets = []RolePreset{
	{
		Role: RoleAdmin, Name: "admin", Display: "超级管理员",
		Desc: "内置最高权限账号，可管理所有账号与系统设置",
		Rank: RankAdmin, Perms: PermAll,
	},
	{
		Role: RoleManager, Name: "manager", Display: "管理员",
		Desc: "由上级管理员提拔，可管理权限低于自己的账号",
		Rank: RankManager,
		Perms: PermView | PermDownload | PermUpload | PermEdit | PermDelete |
			PermTrashManage | PermShare | PermAclManage | PermUserManage |
			PermAuditRead | PermStorageManage,
	},
	{
		Role: RoleUser, Name: "user", Display: "普通用户",
		Desc: "管理自己的文件，不能管理其他账号",
		Rank: RankUser,
		Perms: PermView | PermDownload | PermUpload | PermEdit | PermDelete |
			PermTrashManage | PermShare | PermAclManage,
	},
	{
		Role: RoleGuest, Name: "guest", Display: "访客",
		Desc:  "默认只读，仅可浏览与下载",
		Rank:  RankGuest,
		Perms: PermView | PermDownload,
	},
}

// RolePresets returns the templates in descending authority order.
func RolePresets() []RolePreset {
	out := make([]RolePreset, len(rolePresets))
	copy(out, rolePresets)
	return out
}

// RolePresetFor returns the template of a role.
func RolePresetFor(role Role) (RolePreset, bool) {
	for _, p := range rolePresets {
		if p.Role == role {
			return p, true
		}
	}
	return RolePreset{}, false
}

// DefaultPermsFor returns the default permission set of a role.
func DefaultPermsFor(role Role) PermMask {
	if p, ok := RolePresetFor(role); ok {
		return p.Perms
	}
	return PermNone
}

// DefaultRankFor returns the default rank of a role.
func DefaultRankFor(role Role) int32 {
	if p, ok := RolePresetFor(role); ok {
		return p.Rank
	}
	return RankGuest
}

// ParseRole resolves a role from its machine name.
func ParseRole(name string) (Role, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "admin":
		return RoleAdmin, true
	case "manager", "admin_promoted":
		return RoleManager, true
	case "user", "member":
		return RoleUser, true
	case "guest":
		return RoleGuest, true
	default:
		return RoleUnspecified, false
	}
}

// ParseVisibility resolves a visibility from its machine name.
func ParseVisibility(name string) (Visibility, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "private":
		return VisibilityPrivate, true
	case "internal":
		return VisibilityInternal, true
	case "public":
		return VisibilityPublic, true
	default:
		return VisibilityUnspecified, false
	}
}

// ParseUploadMode resolves an upload transport from its machine name.
func ParseUploadMode(name string) (UploadMode, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "presigned", "direct":
		return UploadModePresigned, true
	case "proxy", "server":
		return UploadModeProxy, true
	default:
		return UploadModeUnspecified, false
	}
}
