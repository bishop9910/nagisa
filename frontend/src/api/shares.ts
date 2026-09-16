/** ShareService：分享链接的创建、管理与三条匿名接口。 */

import type { NodeSet, Permission, Share, ShareAccess, ShareSet, SignedUrl } from './types'
import { encodePassword } from './auth'
import { http } from './http'

export interface CreateShareInput {
  nodeId: string
  name?: string
  description?: string
  permissions?: Permission[]
  password?: string
  passwordHint?: string
  expiresAt?: string
  maxDownloads?: number
  token?: string
}

export async function createShare(input: CreateShareInput): Promise<Share> {
  const password = input.password ? await encodePassword(input.password) : undefined
  return http.post<Share>(
    '/v1/shares/create',
    {
      nodeId: input.nodeId,
      name: input.name,
      description: input.description,
      permissions: input.permissions,
      password,
      passwordHint: input.passwordHint,
      expiresAt: input.expiresAt,
      maxDownloads: input.maxDownloads,
      token: input.token,
    },
    { nodeId: input.nodeId },
  )
}

export function listShares(params: { pageSize?: number; pageToken?: string; filter?: string; orderBy?: string } = {}) {
  return http.get<ShareSet>('/v1/shares/list', { query: params })
}

export function listSharesByNode(
  nodeId: string,
  params: { pageSize?: number; pageToken?: string } = {},
): Promise<ShareSet> {
  return http.get<ShareSet>('/v1/shares/by-node/list', { query: { nodeId, ...params }, nodeId })
}

export interface UpdateShareInput {
  id: string
  fields: Partial<Share>
  updateMask: string[]
  password?: string
  removePassword?: boolean
}

export async function updateShare(input: UpdateShareInput): Promise<Share> {
  const password = input.password ? await encodePassword(input.password) : undefined
  return http.put<Share>('/v1/shares/update', {
    share: { id: input.id, ...input.fields },
    updateMask: input.updateMask.join(','),
    password,
    removePassword: input.removePassword ?? false,
  })
}

export function deleteShare(id: string): Promise<void> {
  return http.delete<void>(`/v1/shares/${encodeURIComponent(id)}`)
}

export function getShare(id: string): Promise<Share> {
  return http.get<Share>(`/v1/shares/${encodeURIComponent(id)}`)
}

/* ---------- 以下三个接口是公开的，用分享令牌鉴权 ---------- */

export interface AccessShareInput {
  token: string
  password?: string
  accessToken?: string
  nodeId?: string
  pageSize?: number
  pageToken?: string
}

/** 打开分享：受密码保护时首次需要带密码，之后复用 accessToken。 */
export async function accessShare(input: AccessShareInput): Promise<ShareAccess> {
  const password = input.password ? await encodePassword(input.password) : undefined
  return http.post<ShareAccess>(
    '/v1/shares/access',
    {
      token: input.token,
      password,
      accessToken: input.accessToken,
      nodeId: input.nodeId,
      pageSize: input.pageSize,
      pageToken: input.pageToken,
    },
    { anonymous: true },
  )
}

export function listShareChildren(input: {
  token: string
  nodeId?: string
  accessToken?: string
  pageSize?: number
  pageToken?: string
  orderBy?: string
}): Promise<NodeSet> {
  return http.post<NodeSet>('/v1/shares/children/list', input, { anonymous: true })
}

export function getShareDownloadUrl(input: {
  token: string
  nodeId?: string
  accessToken?: string
  expiresInSeconds?: number
  inline?: boolean
}): Promise<SignedUrl> {
  return http.post<SignedUrl>('/v1/shares/download-url', input, { anonymous: true })
}
