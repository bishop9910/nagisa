/** UserService：账号、权限目录、角色预设与用量统计。 */

import type {
  Permission,
  PermissionCatalog,
  RolePresetSet,
  User,
  UserSet,
  UserStats,
} from './types'
import { encodePassword } from './auth'
import { http } from './http'

export interface CreateUserInput {
  username: string
  password: string
  nickname?: string
  email?: string
  avatarUrl?: string
  role?: string
  rank?: number
  permissions?: Permission[]
  quotaBytes?: number
  remark?: string
  status?: string
  mustChangePassword?: boolean
}

export async function createUser(input: CreateUserInput): Promise<User> {
  const password = await encodePassword(input.password)
  return http.post<User>('/v1/users/create', {
    user: {
      username: input.username,
      nickname: input.nickname,
      email: input.email,
      avatarUrl: input.avatarUrl,
      role: input.role,
      rank: input.rank,
      permissions: input.permissions,
      quotaBytes: input.quotaBytes,
      remark: input.remark,
      status: input.status,
    },
    password,
    mustChangePassword: input.mustChangePassword ?? false,
  })
}

export function getUser(id: string): Promise<User> {
  return http.get<User>(`/v1/users/${encodeURIComponent(id)}`)
}

export interface ListUsersQuery {
  pageSize?: number
  pageToken?: string
  filter?: string
  orderBy?: string
  includeDeleted?: boolean
}

export function listUsers(query: ListUsersQuery = {}): Promise<UserSet> {
  return http.get<UserSet>('/v1/users/list', { query })
}

export interface UpdateUserInput {
  id: string
  fields: Partial<User>
  updateMask: string[]
}

export function updateUser({ id, fields, updateMask }: UpdateUserInput): Promise<User> {
  return http.put<User>('/v1/users/update', {
    user: { id, ...fields },
    updateMask: updateMask.join(','),
  })
}

export function deleteUser(id: string, trashNodes = false): Promise<void> {
  return http.delete<void>(`/v1/users/${encodeURIComponent(id)}`, { query: { trashNodes } })
}

export function setUserPermissions(id: string, permissions: Permission[], mask?: number): Promise<User> {
  const body: Record<string, unknown> = { id }
  if (permissions.length > 0) {
    body.permissions = permissions
  } else if (typeof mask === 'number') {
    body.permissionsMask = mask
  } else {
    body.permissions = []
  }
  return http.post<User>('/v1/users/permissions/set', body)
}

export async function resetUserPassword(id: string, password: string, mustChangePassword = false): Promise<void> {
  const encoded = await encodePassword(password)
  return http.post<void>('/v1/users/password/reset', { id, password: encoded, mustChangePassword })
}

export function listRolePresets(): Promise<RolePresetSet> {
  return http.get<RolePresetSet>('/v1/users/roles/list')
}

export function listPermissionCatalog(): Promise<PermissionCatalog> {
  return http.get<PermissionCatalog>('/v1/users/permissions/catalog')
}

export function getUserStats(id: string): Promise<UserStats> {
  return http.get<UserStats>(`/v1/users/stats/${encodeURIComponent(id)}`)
}
