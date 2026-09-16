/**
 * 后端契约的类型镜像。
 *
 * 服务端用 protojson 编码，因此：
 *  - 枚举是名称字符串（"ROLE_ADMIN"），请求里也接受数字；
 *  - 64 位整数是字符串，浏览器侧一律用 toInt() 归一；
 *  - 时间戳是 RFC 3339 字符串；
 *  - 零值字段不会出现在响应里，所有字段按「可能缺失」处理。
 */

/** 64 位整数：响应里是字符串，请求里字符串与数字都接受。 */
export type Int64 = string | number

/** RFC 3339 时间戳字符串。 */
export type Timestamp = string

export type Role = 'ROLE_UNSPECIFIED' | 'ROLE_ADMIN' | 'ROLE_MANAGER' | 'ROLE_USER' | 'ROLE_GUEST'

export type UserStatus = 'USER_STATUS_UNSPECIFIED' | 'USER_STATUS_ACTIVE' | 'USER_STATUS_DISABLED' | 'USER_STATUS_DELETED'

export type Permission =
  | 'PERMISSION_UNSPECIFIED'
  | 'PERMISSION_VIEW'
  | 'PERMISSION_DOWNLOAD'
  | 'PERMISSION_UPLOAD'
  | 'PERMISSION_EDIT'
  | 'PERMISSION_DELETE'
  | 'PERMISSION_TRASH_MANAGE'
  | 'PERMISSION_SHARE'
  | 'PERMISSION_ACL_MANAGE'
  | 'PERMISSION_USER_MANAGE'
  | 'PERMISSION_AUDIT_READ'
  | 'PERMISSION_STORAGE_MANAGE'
  | 'PERMISSION_SYSTEM_MANAGE'

export type NodeKind = 'NODE_KIND_UNSPECIFIED' | 'NODE_KIND_FOLDER' | 'NODE_KIND_FILE'

export type NodeStatus = 'NODE_STATUS_UNSPECIFIED' | 'NODE_STATUS_ACTIVE' | 'NODE_STATUS_TRASHED'

export type Visibility = 'VISIBILITY_UNSPECIFIED' | 'VISIBILITY_PRIVATE' | 'VISIBILITY_INTERNAL' | 'VISIBILITY_PUBLIC'

export type SubjectType = 'SUBJECT_TYPE_UNSPECIFIED' | 'SUBJECT_TYPE_USER' | 'SUBJECT_TYPE_ROLE' | 'SUBJECT_TYPE_EVERYONE'

export type Effect = 'EFFECT_UNSPECIFIED' | 'EFFECT_ALLOW' | 'EFFECT_DENY'

export type ConflictPolicy =
  | 'CONFLICT_POLICY_UNSPECIFIED'
  | 'CONFLICT_POLICY_FAIL'
  | 'CONFLICT_POLICY_RENAME'
  | 'CONFLICT_POLICY_OVERWRITE'

export type UploadStatus =
  | 'UPLOAD_STATUS_UNSPECIFIED'
  | 'UPLOAD_STATUS_PENDING'
  | 'UPLOAD_STATUS_IN_PROGRESS'
  | 'UPLOAD_STATUS_COMPLETED'
  | 'UPLOAD_STATUS_ABORTED'
  | 'UPLOAD_STATUS_EXPIRED'

export type UploadMode = 'UPLOAD_MODE_UNSPECIFIED' | 'UPLOAD_MODE_PRESIGNED' | 'UPLOAD_MODE_PROXY'

export type ShareStatus = 'SHARE_STATUS_UNSPECIFIED' | 'SHARE_STATUS_ACTIVE' | 'SHARE_STATUS_EXPIRED' | 'SHARE_STATUS_REVOKED'

export interface User {
  id?: string
  username?: string
  nickname?: string
  email?: string
  avatarUrl?: string
  role?: Role
  rank?: number
  permissions?: Permission[]
  permissionsMask?: Int64
  status?: UserStatus
  quotaBytes?: Int64
  usedBytes?: Int64
  fileCount?: Int64
  folderCount?: Int64
  remark?: string
  mustChangePassword?: boolean
  lastLoginAt?: Timestamp
  createdBy?: string
  createdAt?: Timestamp
  updatedAt?: Timestamp
  manageable?: boolean
  permissionsEditable?: boolean
  maxGrantableRank?: number
}

export interface UserSet {
  users?: User[]
  nextPageToken?: string
  totalSize?: Int64
}

export interface UserStats {
  userId?: string
  username?: string
  quotaBytes?: Int64
  usedBytes?: Int64
  fileCount?: Int64
  folderCount?: Int64
  trashedBytes?: Int64
  trashedCount?: Int64
  usageRatio?: number
}

export interface PermissionInfo {
  permission?: Permission
  name?: string
  displayName?: string
  description?: string
  category?: string
}

export interface PermissionCatalog {
  permissions?: PermissionInfo[]
  granted?: Permission[]
  grantedMask?: Int64
}

export interface RolePreset {
  role?: Role
  name?: string
  displayName?: string
  description?: string
  defaultRank?: number
  permissions?: Permission[]
  permissionsMask?: Int64
}

export interface RolePresetSet {
  presets?: RolePreset[]
  maxGrantableRank?: number
  callerRank?: number
}

export interface AuthConfig {
  passwordKeyId?: string
  passwordEncoding?: string
  passwordPublicKey?: string
  accessTokenTtlSeconds?: number
  refreshTokenTtlSeconds?: number
  plainPasswordAllowed?: boolean
  /** 部署是否允许免登录的只读访客会话（AuthService.GuestLogin）。 */
  guestLoginEnabled?: boolean
  serverTime?: Timestamp
}

export interface LoginReply {
  accessToken?: string
  refreshToken?: string
  tokenType?: string
  expiresIn?: number
  refreshExpiresIn?: number
  user?: User
}

export interface Session {
  id?: string
  ip?: string
  userAgent?: string
  createdAt?: Timestamp
  lastUsedAt?: Timestamp
  expiresAt?: Timestamp
  active?: boolean
  current?: boolean
}

export interface SessionSet {
  sessions?: Session[]
  nextPageToken?: string
}

export interface Node {
  id?: string
  parentId?: string
  name?: string
  kind?: NodeKind
  ownerId?: string
  ownerName?: string
  size?: Int64
  mimeType?: string
  extension?: string
  etag?: string
  status?: NodeStatus
  description?: string
  visibility?: Visibility
  passwordProtected?: boolean
  passwordHint?: string
  locked?: boolean
  hasThumbnail?: boolean
  metadata?: Record<string, string>
  path?: string
  displayPath?: string
  depth?: number
  childCount?: Int64
  fileCount?: Int64
  folderCount?: Int64
  subtreeSize?: Int64
  effectivePermissions?: Permission[]
  effectivePermissionsMask?: Int64
  owned?: boolean
  shared?: boolean
  shareCount?: number
  versionCount?: number
  currentVersionId?: string
  createdAt?: Timestamp
  updatedAt?: Timestamp
  createdBy?: string
  updatedBy?: string
  trashedAt?: Timestamp
  originalParentId?: string
}

export interface NodeSet {
  nodes?: Node[]
  nextPageToken?: string
  totalSize?: Int64
  totalBytes?: Int64
}

export interface NodeTree {
  node?: Node
  children?: NodeTree[]
}

export interface NodePath {
  ancestors?: Node[]
  node?: Node
  displayPath?: string
}

export interface NodeStats {
  nodeId?: string
  fileCount?: Int64
  folderCount?: Int64
  totalSize?: Int64
  trashedCount?: Int64
  trashedSize?: Int64
  largestFileSize?: Int64
  largestFileName?: string
  sizeByCategory?: Record<string, Int64>
}

export interface AclEntry {
  id?: string
  nodeId?: string
  subjectType?: SubjectType
  subjectId?: string
  subjectName?: string
  effect?: Effect
  permissions?: Permission[]
  permissionsMask?: Int64
  inherit?: boolean
  createdAt?: Timestamp
  createdBy?: string
}

export interface NodeAcl {
  nodeId?: string
  entries?: AclEntry[]
  inheritedEntries?: AclEntry[]
  effectivePermissions?: Permission[]
  effectivePermissionsMask?: Int64
  owned?: boolean
  editable?: boolean
}

export interface NodeUnlock {
  nodeId?: string
  unlockToken?: string
  expiresIn?: number
  expiresAt?: Timestamp
}

export interface NodeVersion {
  id?: string
  nodeId?: string
  version?: number
  size?: Int64
  mimeType?: string
  etag?: string
  comment?: string
  current?: boolean
  createdBy?: string
  createdAt?: Timestamp
}

export interface NodeVersionSet {
  versions?: NodeVersion[]
  nextPageToken?: string
}

export interface UploadPart {
  partNumber?: number
  size?: Int64
  etag?: string
  uploadUrl?: string
  urlExpiresAt?: Timestamp
  headers?: Record<string, string>
  createdAt?: Timestamp
}

export interface UploadPartSet {
  uploadId?: string
  parts?: UploadPart[]
}

export interface UploadSession {
  id?: string
  parentId?: string
  name?: string
  kind?: NodeKind
  size?: Int64
  mimeType?: string
  chunkSize?: Int64
  totalParts?: number
  uploadedParts?: number[]
  receivedBytes?: Int64
  status?: UploadStatus
  mode?: UploadMode
  conflictPolicy?: ConflictPolicy
  parts?: UploadPart[]
  nodeId?: string
  createdAt?: Timestamp
  updatedAt?: Timestamp
  expiresAt?: Timestamp
  lastError?: string
}

export interface UploadSessionSet {
  uploads?: UploadSession[]
  nextPageToken?: string
  totalSize?: Int64
}

export interface SignedUrl {
  url?: string
  method?: string
  expiresAt?: Timestamp
  expiresIn?: number
  headers?: Record<string, string>
  nodeId?: string
  size?: Int64
  fileName?: string
  mimeType?: string
}

export interface Share {
  id?: string
  token?: string
  nodeId?: string
  node?: Node
  ownerId?: string
  ownerName?: string
  name?: string
  description?: string
  permissions?: Permission[]
  permissionsMask?: Int64
  passwordProtected?: boolean
  passwordHint?: string
  expiresAt?: Timestamp
  maxDownloads?: Int64
  downloadCount?: Int64
  viewCount?: Int64
  status?: ShareStatus
  url?: string
  editable?: boolean
  createdAt?: Timestamp
  updatedAt?: Timestamp
  createdBy?: string
}

export interface ShareSet {
  shares?: Share[]
  nextPageToken?: string
  totalSize?: Int64
}

export interface ShareAccess {
  share?: Share
  node?: Node
  children?: Node[]
  nextPageToken?: string
  accessToken?: string
  expiresIn?: number
  expiresAt?: Timestamp
}

export interface AuditLog {
  id?: string
  actorId?: string
  actorName?: string
  action?: string
  actionDisplay?: string
  targetType?: string
  targetId?: string
  targetName?: string
  success?: boolean
  errorReason?: string
  detail?: Record<string, string>
  ip?: string
  userAgent?: string
  requestId?: string
  createdAt?: Timestamp
}

export interface AuditLogSet {
  logs?: AuditLog[]
  nextPageToken?: string
  totalSize?: Int64
}

export interface AuditActionCount {
  action?: string
  actionDisplay?: string
  count?: Int64
}

export interface AuditActorCount {
  actorId?: string
  actorName?: string
  count?: Int64
}

export interface AuditSummary {
  from?: Timestamp
  to?: Timestamp
  total?: Int64
  failureCount?: Int64
  byAction?: AuditActionCount[]
  byActor?: AuditActorCount[]
  byDay?: Record<string, Int64>
}

export interface SystemInfo {
  name?: string
  version?: string
  apiVersion?: string
  features?: string[]
  maxUploadSize?: Int64
  defaultChunkSize?: Int64
  minChunkSize?: Int64
  maxInlineSize?: Int64
  uploadSessionTtlSeconds?: number
  signedUrlTtlSeconds?: number
  signedUrlMaxTtlSeconds?: number
  uploadModes?: UploadMode[]
  storageBackend?: string
  databaseBackend?: string
  defaultVisibility?: Visibility
  registrationEnabled?: boolean
  auth?: AuthConfig
  serverTime?: Timestamp
  publicBaseUrl?: string
}

export interface HealthStatus {
  status?: string
  checks?: Record<string, string>
  uptimeSeconds?: Int64
  serverTime?: Timestamp
}

export interface OwnerUsage {
  ownerId?: string
  ownerName?: string
  usedBytes?: Int64
  fileCount?: Int64
  folderCount?: Int64
  quotaBytes?: Int64
}

export interface StorageStats {
  totalBytes?: Int64
  totalFiles?: Int64
  totalFolders?: Int64
  totalUsers?: Int64
  activeUsers?: Int64
  disabledUsers?: Int64
  trashedBytes?: Int64
  trashedNodes?: Int64
  uploadsInProgress?: Int64
  activeShares?: Int64
  versionsBytes?: Int64
  sizeByCategory?: Record<string, Int64>
  topOwners?: OwnerUsage[]
  backendUsedBytes?: Int64
}

export interface MaintenanceReport {
  dryRun?: boolean
  tasks?: string[]
  expiredUploads?: Int64
  deletedObjects?: Int64
  orphanObjects?: Int64
  expiredShares?: Int64
  recountedUsers?: Int64
  purgedNodes?: Int64
  warnings?: string[]
  durationMs?: Int64
  finishedAt?: Timestamp
}

export interface SystemSetting {
  key?: string
  value?: string
  type?: string
  description?: string
  writable?: boolean
}

export interface SystemSettingSet {
  settings?: SystemSetting[]
}

export interface DeleteNodesReply {
  deletedIds?: string[]
  affectedCount?: Int64
  reclaimedBytes?: Int64
}

export interface PurgeNodesReply {
  purgedIds?: string[]
  affectedCount?: Int64
  reclaimedBytes?: Int64
}
