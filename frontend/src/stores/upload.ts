/**
 * 上传队列。
 *
 * 小文件走内联上传（UploadSmallFile，一次请求），大文件走分片：
 *   - 默认用代理模式（PUT .../parts/{n}/raw），字节经 Nagisa 转发，浏览器无需
 *     直连对象存储，也就不依赖对象存储侧的 CORS 配置；
 *   - 部署只提供预签名模式时自动切换：PUT 预签名地址取 ETag，再
 *     ConfirmUploadPart 回执。
 * 同名会话会被服务端幂等复用，因此暂停后继续、失败重试都不会产生第二份暂存。
 */

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { filesApi } from '@/api'
import { putPresignedPart } from '@/api/http'
import type { ConflictPolicy, UploadMode, UploadPart, UploadSession } from '@/api/types'
import { toInt } from '@/utils/format'
import { useSystemStore } from './system'

export type LocalUploadStatus =
  | 'queued'
  | 'preparing'
  | 'uploading'
  | 'finalizing'
  | 'done'
  | 'error'
  | 'canceled'
  | 'paused'

export interface UploadPartState {
  number: number
  size: number
  loaded: number
  done: boolean
  etag?: string
}

export interface UploadTask {
  id: string
  name: string
  size: number
  mimeType: string
  file: File
  parentId: string
  parentLabel: string
  status: LocalUploadStatus
  error?: string
  uploadId?: string
  mode: UploadMode
  chunkSize: number
  totalParts: number
  parts: UploadPartState[]
  nodeId?: string
  inline: boolean
  conflictPolicy: ConflictPolicy
  startedAt?: number
  finishedAt?: number
  controller?: AbortController
}

const MODE_KEY = 'nagisa.uploadMode'
const PART_CONCURRENCY = 3
const FILE_CONCURRENCY = 2
const MAX_PARTS = 9000

function newId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID()
  return `up-${Date.now()}-${Math.random().toString(16).slice(2)}`
}

function readMode(): UploadMode | '' {
  try {
    const saved = localStorage.getItem(MODE_KEY)
    if (saved === 'UPLOAD_MODE_PROXY' || saved === 'UPLOAD_MODE_PRESIGNED') return saved
  } catch {
    /* ignore */
  }
  return ''
}

async function blobToBase64(blob: Blob): Promise<string> {
  const buffer = new Uint8Array(await blob.arrayBuffer())
  let binary = ''
  const chunk = 0x8000
  for (let i = 0; i < buffer.length; i += chunk) {
    binary += String.fromCharCode(...buffer.subarray(i, i + chunk))
  }
  return btoa(binary)
}

export const useUploadStore = defineStore('upload', () => {
  const system = useSystemStore()

  const tasks = ref<UploadTask[]>([])
  const mode = ref<UploadMode | ''>(readMode())
  const collapsed = ref(false)
  /** 最近一次完成的上传，供文件列表刷新使用。 */
  const lastCompleted = ref<{ parentId: string; nodeId?: string; at: number } | null>(null)

  const activeCount = computed(
    () => tasks.value.filter((task) => task.status === 'uploading' || task.status === 'preparing' || task.status === 'finalizing').length,
  )
  const pending = computed(() => tasks.value.filter((task) => task.status === 'queued'))
  const running = computed(() =>
    tasks.value.filter((task) => !['done', 'canceled', 'error'].includes(task.status)),
  )
  const finished = computed(() => tasks.value.filter((task) => ['done', 'canceled', 'error'].includes(task.status)))

  const totalBytes = computed(() => running.value.reduce((sum, task) => sum + task.size, 0))
  const uploadedBytes = computed(() => running.value.reduce((sum, task) => sum + taskLoaded(task), 0))

  /** 读取状态的辅助函数：避免 TS 在 async 流程里把可变属性缩窄到某个字面量。 */
  function statusOf(task: UploadTask): LocalUploadStatus {
    return task.status
  }

  function taskLoaded(task: UploadTask): number {
    if (task.inline) return statusOf(task) === 'done' ? task.size : 0
    return task.parts.reduce((sum, part) => sum + (part.done ? part.size : part.loaded), 0)
  }

  function taskRatio(task: UploadTask): number {
    if (statusOf(task) === 'done') return 1
    if (task.size <= 0) return 0
    return Math.min(1, taskLoaded(task) / task.size)
  }

  /** 选择传输方式：部署只支持一种时直接用那一种。 */
  function resolveMode(): UploadMode {
    const available = system.uploadModes.length > 0 ? system.uploadModes : (['UPLOAD_MODE_PRESIGNED'] as UploadMode[])
    if (mode.value && available.includes(mode.value)) return mode.value
    if (available.includes('UPLOAD_MODE_PROXY')) return 'UPLOAD_MODE_PROXY'
    return available[0] ?? 'UPLOAD_MODE_PRESIGNED'
  }

  function setMode(next: UploadMode): void {
    mode.value = next
    try {
      localStorage.setItem(MODE_KEY, next)
    } catch {
      /* ignore */
    }
  }

  function planChunkSize(size: number): { chunkSize: number; totalParts: number } {
    const min = system.minChunkSize
    let chunkSize = Math.max(min, system.defaultChunkSize)
    let totalParts = Math.max(1, Math.ceil(size / chunkSize))
    while (totalParts > MAX_PARTS) {
      chunkSize *= 2
      totalParts = Math.max(1, Math.ceil(size / chunkSize))
    }
    return { chunkSize, totalParts }
  }

  function enqueue(
    files: File[],
    options: { parentId: string; parentLabel: string; conflictPolicy?: ConflictPolicy },
  ): UploadTask[] {
    const inlineLimit = Math.max(0, Math.min(system.maxInlineSize, 4 * 1024 * 1024))
    const created = files.map<UploadTask>((file) => {
      const { chunkSize, totalParts } = planChunkSize(file.size)
      return {
        id: newId(),
        name: file.name,
        size: file.size,
        mimeType: file.type || '',
        file,
        parentId: options.parentId,
        parentLabel: options.parentLabel,
        status: 'queued',
        mode: resolveMode(),
        chunkSize,
        totalParts,
        parts: [],
        inline: file.size > 0 && file.size <= inlineLimit,
        conflictPolicy: options.conflictPolicy ?? 'CONFLICT_POLICY_RENAME',
      }
    })
    tasks.value = [...created, ...tasks.value]
    collapsed.value = false
    void pump()
    return created
  }

  async function pump(): Promise<void> {
    if (activeCount.value >= FILE_CONCURRENCY) return
    const next = pending.value[0]
    if (!next) return
    void run(next)
    // 还有空位就继续补位。
    if (activeCount.value < FILE_CONCURRENCY) void pump()
  }

  async function run(task: UploadTask): Promise<void> {
    if (task.status === 'uploading' || task.status === 'preparing' || task.status === 'finalizing') return
    task.status = 'preparing'
    task.error = undefined
    task.startedAt = task.startedAt ?? Date.now()
    task.controller = new AbortController()
    try {
      if (task.inline) {
        await runInline(task)
      } else {
        await runChunked(task)
      }
      task.status = 'done'
      task.finishedAt = Date.now()
      lastCompleted.value = { parentId: task.parentId, nodeId: task.nodeId, at: Date.now() }
    } catch (err) {
      // 暂停与取消是用户的主动行为，不该记成失败。
      const status = statusOf(task)
      if (status !== 'paused' && status !== 'canceled') {
        task.status = 'error'
        task.error = err instanceof Error ? err.message : String(err)
        task.finishedAt = Date.now()
      }
    } finally {
      pump()
    }
  }

  async function runInline(task: UploadTask): Promise<void> {
    task.status = 'uploading'
    const contentBase64 = await blobToBase64(task.file)
    const node = await filesApi.uploadSmallFile({
      name: task.name,
      contentBase64,
      parentId: task.parentId || undefined,
      mimeType: task.mimeType || undefined,
      conflictPolicy: task.conflictPolicy ?? 'CONFLICT_POLICY_RENAME',
    })
    task.nodeId = node.id
  }

  async function runChunked(task: UploadTask): Promise<void> {
    // 会话创建是幂等的：同名同目录的未完成会话会被直接返回，因此续传就是
    // 带上 resumableUploadId 再调一次。
    const session = await filesApi.initiateUpload({
      name: task.name,
      size: task.size,
      parentId: task.parentId || undefined,
      mimeType: task.mimeType || undefined,
      chunkSize: task.chunkSize,
      mode: task.mode,
      conflictPolicy: task.conflictPolicy ?? 'CONFLICT_POLICY_RENAME',
      resumableUploadId: task.uploadId,
    })
    task.uploadId = session.id
    task.mode = session.mode ?? task.mode
    task.chunkSize = toInt(session.chunkSize) || task.chunkSize
    task.totalParts = session.totalParts ?? task.totalParts

    const uploaded = new Set(session.uploadedParts ?? [])
    task.parts = Array.from({ length: task.totalParts }, (_, index) => {
      const number = index + 1
      const start = index * task.chunkSize
      const size = Math.max(0, Math.min(task.chunkSize, task.size - start))
      const done = uploaded.has(number)
      return { number, size, loaded: done ? size : 0, done }
    })

    task.status = 'uploading'
    await runParts(task, session)
    task.status = 'finalizing'
    const node = await filesApi.completeUpload(task.uploadId ?? '', { comment: 'web upload' })
    task.nodeId = node.id
  }

  /** 分片上传：每个文件内部并发跑若干片，进度按字节累计。 */
  async function runParts(task: UploadTask, session: UploadSession): Promise<void> {
    const queue = task.parts.filter((part) => !part.done).map((part) => part.number)
    const presigned = new Map<number, UploadPart>()
    for (const part of session.parts ?? []) {
      if (part.partNumber) presigned.set(part.partNumber, part)
    }

    let cursor = 0
    let failure: unknown = null

    async function worker(): Promise<void> {
      while (cursor < queue.length && !failure) {
        if (task.status === 'paused' || task.status === 'canceled') return
        const number = queue[cursor]
        cursor += 1
        if (number === undefined) continue
        const part = task.parts.find((item) => item.number === number)
        if (!part || part.done) continue
        try {
          await uploadPart(task, part, presigned)
        } catch (err) {
          failure = err
        }
      }
    }

    await Promise.all(Array.from({ length: Math.min(PART_CONCURRENCY, queue.length || 1) }, () => worker()))

    if (failure) throw failure
    const status = statusOf(task)
    if (status === 'paused' || status === 'canceled') return
    const missing = task.parts.filter((part) => !part.done)
    if (missing.length > 0) {
      throw new Error(`仍有 ${missing.length} 个分片未完成`)
    }
  }

  async function uploadPart(
    task: UploadTask,
    part: UploadPartState,
    presigned: Map<number, UploadPart>,
  ): Promise<void> {
    const start = (part.number - 1) * task.chunkSize
    const blob = task.file.slice(start, start + part.size)
    part.loaded = 0

    if (task.mode === 'UPLOAD_MODE_PRESIGNED') {
      let descriptor = presigned.get(part.number)
      // 预签名地址会过期；地址缺失时向服务端重新签发一次。
      const expired = descriptor?.urlExpiresAt ? new Date(descriptor.urlExpiresAt).getTime() < Date.now() + 5000 : false
      if (!descriptor?.uploadUrl || expired) {
        const refreshed = await filesApi.listUploadParts(task.uploadId ?? '')
        const fresh = (refreshed.parts ?? []).find((item) => item.partNumber === part.number)
        if (!fresh?.uploadUrl) throw new Error(`分片 ${part.number} 未获得预签名地址`)
        descriptor = fresh
        presigned.set(part.number, fresh)
      }
      const etag = await putPresignedPart(
        descriptor.uploadUrl ?? '',
        blob,
        descriptor.headers ?? {},
        task.controller?.signal,
      )
      part.loaded = part.size
      await filesApi.confirmUploadPart({
        uploadId: task.uploadId ?? '',
        partNumber: part.number,
        etag: etag || (descriptor.etag ?? ''),
        size: part.size,
      })
      part.done = true
      part.etag = etag
      return
    }

    await filesApi.uploadRawPart(task.uploadId ?? '', part.number, blob, {
      signal: task.controller?.signal,
      onProgress: (loaded) => {
        part.loaded = Math.min(part.size, loaded)
      },
    })
    part.loaded = part.size
    part.done = true
  }

  function pause(task: UploadTask): void {
    if (['done', 'error', 'canceled'].includes(task.status)) return
    task.status = 'paused'
    task.controller?.abort()
  }

  function resume(task: UploadTask): void {
    if (task.status !== 'paused' && task.status !== 'error') return
    void run(task)
  }

  async function cancel(task: UploadTask): Promise<void> {
    task.status = 'canceled'
    task.finishedAt = Date.now()
    task.controller?.abort()
    if (task.uploadId && !task.inline) {
      try {
        await filesApi.abortUpload(task.uploadId)
      } catch {
        /* 会话可能已过期，忽略。 */
      }
    }
    pump()
  }

  function retry(task: UploadTask): void {
    task.error = undefined
    task.status = 'queued'
    void pump()
  }

  function remove(taskId: string): void {
    tasks.value = tasks.value.filter((task) => task.id !== taskId)
  }

  function clearFinished(): void {
    tasks.value = tasks.value.filter((task) => !['done', 'canceled', 'error'].includes(task.status))
  }

  function retryAllFailed(): void {
    for (const task of tasks.value) {
      if (task.status === 'error') {
        task.error = undefined
        task.status = 'queued'
      }
    }
    void pump()
  }

  return {
    tasks,
    mode,
    collapsed,
    lastCompleted,
    activeCount,
    pending,
    running,
    finished,
    totalBytes,
    uploadedBytes,
    setMode,
    enqueue,
    pump,
    run,
    pause,
    resume,
    cancel,
    retry,
    remove,
    clearFinished,
    retryAllFailed,
    taskLoaded,
    taskRatio,
  }
})
