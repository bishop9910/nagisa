/** 枚举字典：把后端返回的机器名映射成中文标签、色调与位掩码。 */

import type {
  ConflictPolicy,
  Effect,
  NodeStatus,
  Permission,
  Role,
  ShareStatus,
  SubjectType,
  UploadMode,
  UploadStatus,
  UserStatus,
  Visibility,
} from '@/api/types'
import { toInt } from './format'

export type Tone = 'neutral' | 'brand' | 'success' | 'warning' | 'danger' | 'info'

export interface Option<T extends string> {
  value: T
  label: string
  hint?: string
}

export const ROLE_LABEL: Record<Role, string> = {
  ROLE_UNSPECIFIED: '未指定',
  ROLE_ADMIN: '超级管理员',
  ROLE_MANAGER: '管理员',
  ROLE_USER: '普通用户',
  ROLE_GUEST: '访客',
}

export const ROLE_TONE: Record<Role, Tone> = {
  ROLE_UNSPECIFIED: 'neutral',
  ROLE_ADMIN: 'danger',
  ROLE_MANAGER: 'brand',
  ROLE_USER: 'info',
  ROLE_GUEST: 'neutral',
}

export const ROLE_MACHINE: Record<Role, string> = {
  ROLE_UNSPECIFIED: '',
  ROLE_ADMIN: 'admin',
  ROLE_MANAGER: 'manager',
  ROLE_USER: 'user',
  ROLE_GUEST: 'guest',
}

export const ROLE_OPTIONS: Option<Role>[] = [
  { value: 'ROLE_ADMIN', label: '超级管理员' },
  { value: 'ROLE_MANAGER', label: '管理员' },
  { value: 'ROLE_USER', label: '普通用户' },
  { value: 'ROLE_GUEST', label: '访客' },
]

export const USER_STATUS_LABEL: Record<UserStatus, string> = {
  USER_STATUS_UNSPECIFIED: '未指定',
  USER_STATUS_ACTIVE: '正常',
  USER_STATUS_DISABLED: '已禁用',
  USER_STATUS_DELETED: '已删除',
}

export const USER_STATUS_TONE: Record<UserStatus, Tone> = {
  USER_STATUS_UNSPECIFIED: 'neutral',
  USER_STATUS_ACTIVE: 'success',
  USER_STATUS_DISABLED: 'warning',
  USER_STATUS_DELETED: 'danger',
}

export const USER_STATUS_OPTIONS: Option<UserStatus>[] = [
  { value: 'USER_STATUS_ACTIVE', label: '正常' },
  { value: 'USER_STATUS_DISABLED', label: '已禁用' },
]

export const VISIBILITY_LABEL: Record<Visibility, string> = {
  VISIBILITY_UNSPECIFIED: '继承默认',
  VISIBILITY_PRIVATE: '私有',
  VISIBILITY_INTERNAL: '内部可见',
  VISIBILITY_PUBLIC: '公开',
}

export const VISIBILITY_HINT: Record<Visibility, string> = {
  VISIBILITY_UNSPECIFIED: '按部署默认值处理',
  VISIBILITY_PRIVATE: '仅属主与被显式允许的主体可见',
  VISIBILITY_INTERNAL: '任何已登录账号都可查看与下载',
  VISIBILITY_PUBLIC: '匿名访问者也可查看与下载',
}

export const VISIBILITY_OPTIONS: Option<Visibility>[] = [
  { value: 'VISIBILITY_PRIVATE', label: '私有' },
  { value: 'VISIBILITY_INTERNAL', label: '内部可见' },
  { value: 'VISIBILITY_PUBLIC', label: '公开' },
]

export const CONFLICT_POLICY_LABEL: Record<ConflictPolicy, string> = {
  CONFLICT_POLICY_UNSPECIFIED: '默认（重命名）',
  CONFLICT_POLICY_FAIL: '同名时报错',
  CONFLICT_POLICY_RENAME: '自动重命名',
  CONFLICT_POLICY_OVERWRITE: '覆盖已有文件',
}

export const CONFLICT_POLICY_OPTIONS: Option<ConflictPolicy>[] = [
  { value: 'CONFLICT_POLICY_RENAME', label: '自动重命名', hint: '保留两份，新文件加 (1) 后缀' },
  { value: 'CONFLICT_POLICY_FAIL', label: '同名时报错', hint: '存在同名条目时直接失败' },
  { value: 'CONFLICT_POLICY_OVERWRITE', label: '覆盖已有文件', hint: '覆盖同名文件并保留历史版本' },
]

export const NODE_STATUS_LABEL: Record<NodeStatus, string> = {
  NODE_STATUS_UNSPECIFIED: '未指定',
  NODE_STATUS_ACTIVE: '正常',
  NODE_STATUS_TRASHED: '回收站',
}

export const UPLOAD_STATUS_LABEL: Record<UploadStatus, string> = {
  UPLOAD_STATUS_UNSPECIFIED: '未知',
  UPLOAD_STATUS_PENDING: '待开始',
  UPLOAD_STATUS_IN_PROGRESS: '上传中',
  UPLOAD_STATUS_COMPLETED: '已完成',
  UPLOAD_STATUS_ABORTED: '已取消',
  UPLOAD_STATUS_EXPIRED: '已过期',
}

export const UPLOAD_STATUS_TONE: Record<UploadStatus, Tone> = {
  UPLOAD_STATUS_UNSPECIFIED: 'neutral',
  UPLOAD_STATUS_PENDING: 'neutral',
  UPLOAD_STATUS_IN_PROGRESS: 'info',
  UPLOAD_STATUS_COMPLETED: 'success',
  UPLOAD_STATUS_ABORTED: 'neutral',
  UPLOAD_STATUS_EXPIRED: 'warning',
}

export const UPLOAD_MODE_LABEL: Record<UploadMode, string> = {
  UPLOAD_MODE_UNSPECIFIED: '默认',
  UPLOAD_MODE_PRESIGNED: '直传对象存储',
  UPLOAD_MODE_PROXY: '经服务端中转',
}

export const SHARE_STATUS_LABEL: Record<ShareStatus, string> = {
  SHARE_STATUS_UNSPECIFIED: '未知',
  SHARE_STATUS_ACTIVE: '生效中',
  SHARE_STATUS_EXPIRED: '已过期',
  SHARE_STATUS_REVOKED: '已撤销',
}

export const SHARE_STATUS_TONE: Record<ShareStatus, Tone> = {
  SHARE_STATUS_UNSPECIFIED: 'neutral',
  SHARE_STATUS_ACTIVE: 'success',
  SHARE_STATUS_EXPIRED: 'warning',
  SHARE_STATUS_REVOKED: 'neutral',
}

export const SUBJECT_TYPE_LABEL: Record<SubjectType, string> = {
  SUBJECT_TYPE_UNSPECIFIED: '未指定',
  SUBJECT_TYPE_USER: '指定账号',
  SUBJECT_TYPE_ROLE: '角色',
  SUBJECT_TYPE_EVERYONE: '所有人',
}

export const EFFECT_LABEL: Record<Effect, string> = {
  EFFECT_UNSPECIFIED: '未指定',
  EFFECT_ALLOW: '允许',
  EFFECT_DENY: '拒绝',
}

/** 权限位定义，顺序即枚举顺序。 */
export interface PermissionMeta {
  name: Permission
  bit: number
  machine: string
  display: string
  description: string
  category: 'content' | 'admin'
}

export const PERMISSION_META: PermissionMeta[] = [
  { name: 'PERMISSION_VIEW', bit: 1 << 0, machine: 'view', display: '查看', description: '浏览目录、读取元信息', category: 'content' },
  { name: 'PERMISSION_DOWNLOAD', bit: 1 << 1, machine: 'download', display: '下载', description: '下载原始文件与预览内容', category: 'content' },
  { name: 'PERMISSION_UPLOAD', bit: 1 << 2, machine: 'upload', display: '上传', description: '上传文件与新建文件夹', category: 'content' },
  { name: 'PERMISSION_EDIT', bit: 1 << 3, machine: 'edit', display: '编辑', description: '重命名、移动、修改元信息', category: 'content' },
  { name: 'PERMISSION_DELETE', bit: 1 << 4, machine: 'delete', display: '删除', description: '移入回收站', category: 'content' },
  { name: 'PERMISSION_TRASH_MANAGE', bit: 1 << 5, machine: 'trash_manage', display: '回收站管理', description: '还原或彻底清除', category: 'content' },
  { name: 'PERMISSION_SHARE', bit: 1 << 6, machine: 'share', display: '分享', description: '创建与管理分享链接', category: 'content' },
  { name: 'PERMISSION_ACL_MANAGE', bit: 1 << 7, machine: 'acl_manage', display: '权限管理', description: '设置描述、可见范围、名单与密码', category: 'content' },
  { name: 'PERMISSION_USER_MANAGE', bit: 1 << 8, machine: 'user_manage', display: '账号管理', description: '创建与维护账号并分配权限', category: 'admin' },
  { name: 'PERMISSION_AUDIT_READ', bit: 1 << 9, machine: 'audit_read', display: '审计日志', description: '查看操作审计记录', category: 'admin' },
  { name: 'PERMISSION_STORAGE_MANAGE', bit: 1 << 10, machine: 'storage_manage', display: '存储管理', description: '查看全局统计并执行维护任务', category: 'admin' },
  { name: 'PERMISSION_SYSTEM_MANAGE', bit: 1 << 11, machine: 'system_manage', display: '系统信息', description: '查看系统信息、运行状态与运行参数', category: 'admin' },
]

export const PERMISSION_ALL = 4095
/** 节点 ACL 能携带的权限（内容类 8 位）。 */
export const PERMISSION_NODE_SCOPE = 255
/** 分享链接能传达的权限（查看/下载/上传）。 */
export const PERMISSION_SHARE_SCOPE = 7

export const PERMISSION_BY_NAME = new Map<Permission, PermissionMeta>(PERMISSION_META.map((item) => [item.name, item]))

/** 权限名数组 → 位掩码。 */
export function maskFromPermissions(names?: Permission[] | null): number {
  if (!names) return 0
  let mask = 0
  for (const name of names) {
    const meta = PERMISSION_BY_NAME.get(name)
    if (meta) mask |= meta.bit
  }
  return mask
}

/** 位掩码 → 权限名数组。 */
export function permissionsFromMask(mask: unknown): Permission[] {
  const value = toInt(mask)
  return PERMISSION_META.filter((meta) => (value & meta.bit) !== 0).map((meta) => meta.name)
}

/** 掩码是否包含全部指定权限。 */
export function hasPermission(mask: unknown, ...names: Permission[]): boolean {
  const value = toInt(mask)
  return names.every((name) => {
    const meta = PERMISSION_BY_NAME.get(name)
    return meta ? (value & meta.bit) !== 0 : false
  })
}

/** 掩码是否包含任意一个指定权限。 */
export function hasAnyPermission(mask: unknown, ...names: Permission[]): boolean {
  const value = toInt(mask)
  return names.some((name) => {
    const meta = PERMISSION_BY_NAME.get(name)
    return meta ? (value & meta.bit) !== 0 : false
  })
}

/** 权限名的中文列表。 */
export function permissionLabels(mask: unknown): string[] {
  const value = toInt(mask)
  return PERMISSION_META.filter((meta) => (value & meta.bit) !== 0).map((meta) => meta.display)
}

/** 单个权限的中文名。 */
export function permissionLabel(name?: Permission): string {
  if (!name) return '—'
  return PERMISSION_BY_NAME.get(name)?.display ?? name
}

/** 存储统计的分类键 → 中文。 */
export const SIZE_CATEGORY_LABEL: Record<string, string> = {
  image: '图片',
  video: '视频',
  audio: '音频',
  text: '文本',
  application: '程序与文档',
  other: '其他',
}

/** 维护任务清单。 */
export const MAINTENANCE_TASKS: { value: string; label: string; hint: string }[] = [
  { value: 'expire_uploads', label: '过期上传会话', hint: '超过有效期的会话置为 EXPIRED 并释放暂存分片' },
  { value: 'purge_deletions', label: '清理待删除对象', hint: '消费对象键队列，真正释放存储空间' },
  { value: 'gc_orphans', label: '回收孤儿对象', hint: '清理无引用的暂存前缀，不影响进行中的上传' },
  { value: 'expire_shares', label: '过期分享链接', hint: '把已到期的分享置为 EXPIRED' },
  { value: 'recount_usage', label: '重算账号用量', hint: '按文件树重算每个账号的已用空间' },
  { value: 'purge_trash', label: '清理回收站', hint: '彻底删除超过保留期的回收站条目' },
]

/** 审计动作前缀分组，用于筛选器。 */
export const AUDIT_ACTION_GROUPS: { value: string; label: string }[] = [
  { value: 'auth.', label: '认证' },
  { value: 'user.', label: '账号' },
  { value: 'node.', label: '节点' },
  { value: 'file.', label: '文件传输' },
  { value: 'share.', label: '分享' },
  { value: 'system.', label: '系统' },
]
