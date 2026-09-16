/** FileService：分片上传、下载/打包/预览地址。 */

import type {
  ConflictPolicy,
  Node,
  SignedUrl,
  UploadMode,
  UploadPart,
  UploadPartSet,
  UploadSession,
  UploadSessionSet,
  UploadStatus,
} from './types'
import { http, uploadRaw } from './http'

export interface InitiateUploadInput {
  name: string
  size: number
  parentId?: string
  mimeType?: string
  chunkSize?: number
  mode?: UploadMode
  conflictPolicy?: ConflictPolicy
  description?: string
  metadata?: Record<string, string>
  resumableUploadId?: string
}

export function initiateUpload(input: InitiateUploadInput): Promise<UploadSession> {
  return http.post<UploadSession>('/v1/files/uploads/create', input, { nodeId: input.parentId })
}

export function listUploads(params: {
  pageSize?: number
  pageToken?: string
  status?: UploadStatus
  parentId?: string
} = {}): Promise<UploadSessionSet> {
  return http.get<UploadSessionSet>('/v1/files/uploads/list', { query: params })
}

export function getUpload(id: string): Promise<UploadSession> {
  return http.get<UploadSession>(`/v1/files/uploads/${encodeURIComponent(id)}`)
}

export function listUploadParts(uploadId: string, fromPartNumber?: number): Promise<UploadPartSet> {
  return http.get<UploadPartSet>(`/v1/files/uploads/${encodeURIComponent(uploadId)}/parts`, {
    query: { fromPartNumber },
  })
}

export function completeUpload(uploadId: string, options: { etag?: string; comment?: string } = {}): Promise<Node> {
  return http.post<Node>('/v1/files/uploads/complete', { uploadId, ...options })
}

export function abortUpload(uploadId: string): Promise<void> {
  return http.post<void>('/v1/files/uploads/abort', { uploadId })
}

export function confirmUploadPart(input: {
  uploadId: string
  partNumber: number
  etag: string
  size?: number
}): Promise<UploadPart> {
  return http.post<UploadPart>('/v1/files/uploads/parts/confirm', input)
}

/** 代理模式的二进制绑定：原始字节直接写进请求体，避免 base64 膨胀。 */
export function uploadRawPart(
  uploadId: string,
  partNumber: number,
  payload: Blob,
  handlers: { onProgress?: (loaded: number, total: number) => void; signal?: AbortSignal } = {},
): Promise<void> {
  return uploadRaw(`/v1/files/uploads/${encodeURIComponent(uploadId)}/parts/${partNumber}/raw`, payload, handlers)
}

/** base64 版本，仅在无法流式发送时作为兜底。 */
export function uploadChunk(
  uploadId: string,
  partNumber: number,
  contentBase64: string,
  etag?: string,
): Promise<UploadPart> {
  return http.put<UploadPart>(
    `/v1/files/uploads/${encodeURIComponent(uploadId)}/parts/${partNumber}`,
    { content: contentBase64, etag },
    { nodeId: undefined },
  )
}

/** 内联小文件上传：内容随请求体一起发送，不产生上传会话。 */
export function uploadSmallFile(input: {
  name: string
  contentBase64: string
  parentId?: string
  mimeType?: string
  conflictPolicy?: ConflictPolicy
  description?: string
}): Promise<Node> {
  return http.post<Node>('/v1/files/upload', input, { nodeId: input.parentId })
}

export function getDownloadUrl(
  nodeId: string,
  params: { expiresInSeconds?: number; inline?: boolean; fileName?: string } = {},
): Promise<SignedUrl> {
  return http.get<SignedUrl>(`/v1/files/${encodeURIComponent(nodeId)}/download-url`, {
    query: params,
    nodeId,
  })
}

export function getArchiveUrl(
  nodeId: string,
  params: { expiresInSeconds?: number; archiveName?: string } = {},
): Promise<SignedUrl> {
  return http.get<SignedUrl>(`/v1/files/${encodeURIComponent(nodeId)}/archive-url`, {
    query: params,
    nodeId,
  })
}

export function getPreviewUrl(
  nodeId: string,
  params: { expiresInSeconds?: number; thumbnail?: boolean } = {},
): Promise<SignedUrl> {
  return http.get<SignedUrl>(`/v1/files/${encodeURIComponent(nodeId)}/preview-url`, {
    query: params,
    nodeId,
  })
}
