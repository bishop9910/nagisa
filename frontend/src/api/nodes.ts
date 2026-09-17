/** NodeService：文件树、回收站、ACL、密码解锁与历史版本。 */

import type {
  AclEntry,
  ConflictPolicy,
  DeleteNodesReply,
  Node,
  NodeAcl,
  NodePath,
  NodeSet,
  NodeStats,
  NodeTree,
  NodeUnlock,
  NodeVersionSet,
  PurgeNodesReply,
  Visibility,
} from './types'
import { encodePassword } from './auth'
import { http } from './http'

export interface ListNodesQuery {
  parentId?: string
  pageSize?: number
  pageToken?: string
  filter?: string
  orderBy?: string
  recursive?: boolean
  maxDepth?: number
  includeTrashed?: boolean
}

export function listNodes(query: ListNodesQuery = {}): Promise<NodeSet> {
  return http.get<NodeSet>('/v1/nodes/list', { query, nodeId: query.parentId })
}

export interface SearchNodesQuery {
  query?: string
  scopeNodeId?: string
  kind?: string
  mimeType?: string
  ownerId?: string
  minSize?: number
  maxSize?: number
  updatedAfter?: string
  updatedBefore?: string
  matchDescription?: boolean
  pageSize?: number
  pageToken?: string
  orderBy?: string
}

export function searchNodes(query: SearchNodesQuery = {}): Promise<NodeSet> {
  return http.get<NodeSet>('/v1/nodes/search', { query, nodeId: query.scopeNodeId })
}

export function getNodeTree(params: { rootId?: string; depth?: number; pageSize?: number; filter?: string } = {}) {
  return http.get<NodeTree>('/v1/nodes/tree', { query: params, nodeId: params.rootId })
}

export interface ListTrashQuery {
  pageSize?: number
  pageToken?: string
  filter?: string
  orderBy?: string
  /** 列这个回收站文件夹里的条目；不带时只列每棵被删子树的顶层条目。 */
  originalParentId?: string
}

export function listTrash(query: ListTrashQuery = {}): Promise<NodeSet> {
  return http.get<NodeSet>('/v1/nodes/trash/list', { query })
}

export function getNode(id: string): Promise<Node> {
  return http.get<Node>(`/v1/nodes/${encodeURIComponent(id)}`, { nodeId: id })
}

export function getNodePath(id: string): Promise<NodePath> {
  return http.get<NodePath>(`/v1/nodes/${encodeURIComponent(id)}/path`, { nodeId: id })
}

/**
 * 节点子树的聚合统计。路由要求必须有 id，因此「整个网盘」的统计走
 * UserService.GetUserStats("me")，见 users.ts 的 getUserStats。
 */
export function getNodeStats(id: string): Promise<NodeStats> {
  return http.get<NodeStats>(`/v1/nodes/${encodeURIComponent(id)}/stats`, { nodeId: id })
}

export function getNodeAcl(id: string): Promise<NodeAcl> {
  return http.get<NodeAcl>(`/v1/nodes/${encodeURIComponent(id)}/acl`, { nodeId: id })
}

export interface CreateFolderInput {
  name: string
  parentId?: string
  description?: string
  visibility?: Visibility
  password?: string
  passwordHint?: string
  metadata?: Record<string, string>
  conflictPolicy?: ConflictPolicy
  acl?: AclEntry[]
}

export async function createFolder(input: CreateFolderInput): Promise<Node> {
  const password = input.password ? await encodePassword(input.password) : undefined
  return http.post<Node>('/v1/nodes/folders/create', {
    folder: {
      name: input.name,
      parentId: input.parentId,
      description: input.description,
      visibility: input.visibility,
      passwordHint: input.passwordHint,
      metadata: input.metadata,
    },
    conflictPolicy: input.conflictPolicy,
    password,
    acl: input.acl,
  })
}

export interface UpdateNodeInput {
  id: string
  updateMask: string[]
  fields: Partial<Node>
  password?: string
  removePassword?: boolean
  conflictPolicy?: ConflictPolicy
}

export async function updateNode(input: UpdateNodeInput): Promise<Node> {
  const password = input.password ? await encodePassword(input.password) : undefined
  return http.put<Node>('/v1/nodes/update', {
    node: { id: input.id, ...input.fields },
    updateMask: input.updateMask.join(','),
    password,
    removePassword: input.removePassword ?? false,
    conflictPolicy: input.conflictPolicy,
  })
}

export function setNodeAcl(nodeId: string, entries: AclEntry[], recursive = false): Promise<NodeAcl> {
  return http.post<NodeAcl>(
    `/v1/nodes/${encodeURIComponent(nodeId)}/acl`,
    { entries, recursive },
    { nodeId },
  )
}

export async function unlockNode(nodeId: string, password: string): Promise<NodeUnlock> {
  const encoded = await encodePassword(password)
  return http.post<NodeUnlock>(`/v1/nodes/${encodeURIComponent(nodeId)}/unlock`, { password: encoded }, { nodeId })
}

export function moveNodes(input: {
  ids: string[]
  targetParentId?: string
  conflictPolicy?: ConflictPolicy
  newNames?: string[]
}): Promise<NodeSet> {
  return http.post<NodeSet>('/v1/nodes/move', input)
}

export function copyNodes(input: {
  ids: string[]
  targetParentId?: string
  conflictPolicy?: ConflictPolicy
  newNames?: string[]
}): Promise<NodeSet> {
  return http.post<NodeSet>('/v1/nodes/copy', input)
}

export function deleteNodes(ids: string[], permanent = false): Promise<DeleteNodesReply> {
  return http.post<DeleteNodesReply>('/v1/nodes/delete', { ids, permanent })
}

export function restoreNodes(input: {
  ids: string[]
  targetParentId?: string
  conflictPolicy?: ConflictPolicy
}): Promise<NodeSet> {
  return http.post<NodeSet>('/v1/nodes/trash/restore', input)
}

export function purgeNodes(ids: string[]): Promise<PurgeNodesReply> {
  return http.post<PurgeNodesReply>('/v1/nodes/trash/purge', { ids })
}

export function emptyTrash(trashedBefore?: string): Promise<PurgeNodesReply> {
  return http.post<PurgeNodesReply>('/v1/nodes/trash/empty', { trashedBefore })
}

export function listNodeVersions(
  nodeId: string,
  params: { pageSize?: number; pageToken?: string } = {},
): Promise<NodeVersionSet> {
  return http.get<NodeVersionSet>(`/v1/nodes/${encodeURIComponent(nodeId)}/versions/list`, {
    query: params,
    nodeId,
  })
}

export function restoreNodeVersion(nodeId: string, versionId: string): Promise<Node> {
  return http.post<Node>('/v1/nodes/versions/restore', { nodeId, versionId }, { nodeId })
}

export function deleteNodeVersion(nodeId: string, versionId: string): Promise<void> {
  return http.delete<void>(`/v1/nodes/${encodeURIComponent(nodeId)}/versions/${encodeURIComponent(versionId)}`, {
    nodeId,
  })
}
