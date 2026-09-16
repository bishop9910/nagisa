/**
 * HTTP 客户端。
 *
 * 只有一件事要做：把 protojson 的信封、鉴权头、解封令牌与 401 自动续期
 * 收在一个地方，业务层只写 `await http.get('/v1/nodes/list', { parentId })`。
 */

/** 生产环境同源部署时为空串；前后端分离时由 VITE_API_BASE 指定。 */
const API_BASE = (import.meta.env.VITE_API_BASE ?? '').replace(/\/+$/, '')

/** 后端所有业务路径都以 /v1 开头。 */
export const API_PREFIX = '/v1'

export interface ApiErrorBody {
  code?: number
  reason?: string
  message?: string
  metadata?: Record<string, string>
}

/** 后端错误信封，客户端按 reason 分支，而不是解析 message。 */
export class ApiError extends Error {
  readonly code: number
  readonly reason: string
  readonly metadata: Record<string, string>

  constructor(body: ApiErrorBody) {
    super(body.message ?? '请求失败')
    this.name = 'ApiError'
    this.code = body.code ?? 0
    this.reason = body.reason ?? ''
    this.metadata = body.metadata ?? {}
  }

  get isUnauthenticated(): boolean {
    return this.code === 401 || this.reason === 'NETDISK_UNAUTHENTICATED'
  }

  get isLocked(): boolean {
    return this.reason === 'NETDISK_NODE_LOCKED'
  }

  get isPermissionDenied(): boolean {
    return this.reason === 'NETDISK_PERMISSION_DENIED'
  }

  /** 受密码保护的节点在错误里会带上提示。 */
  get passwordHint(): string {
    return this.metadata['password_hint'] ?? ''
  }
}

const REASON_TEXT: Record<string, string> = {
  NETDISK_NOT_FOUND: '资源不存在，或对当前账号不可见',
  NETDISK_INVALID_ARGUMENT: '请求参数不合法',
  NETDISK_UNAUTHENTICATED: '登录状态已失效，请重新登录',
  NETDISK_PERMISSION_DENIED: '没有执行该操作的权限',
  NETDISK_CONFLICT: '操作与当前状态冲突',
  NETDISK_INTERNAL: '服务端内部错误',
  NETDISK_UNAVAILABLE: '依赖服务不可用：对象存储未配置或不可达',
  NETDISK_QUOTA_EXCEEDED: '已超出账号配额',
  NETDISK_STORAGE_ERROR: '对象存储操作失败',
  NETDISK_NODE_LOCKED: '该目录受密码保护，需要先解锁',
  NETDISK_NAME_CONFLICT: '目标目录已存在同名条目',
  NETDISK_CYCLE_DETECTED: '不能把文件夹移动到自身或其子目录',
  NETDISK_UPLOAD_INCOMPLETE: '上传分片不完整',
  NETDISK_UPLOAD_EXPIRED: '上传会话已过期',
  NETDISK_TOO_LARGE: '内容超过配置的大小上限',
  NETDISK_UNSUPPORTED: '当前资源不支持该操作',
  NETDISK_RESOURCE_EXHAUSTED: '额度已用尽',
  NETDISK_PRECONDITION_FAILED: '校验失败：摘要或 ETag 不一致',
  NETDISK_ALREADY_EXISTS: '资源已存在',
  NETDISK_ABORTED: '操作已被取消',
  NETDISK_ACCOUNT_DISABLED: '账号已被禁用',
}

/** 把任意异常翻译成可以直接展示的中文。 */
export function errorText(error: unknown): string {
  if (error instanceof ApiError) {
    if (error.reason && REASON_TEXT[error.reason]) {
      const detail = error.message && error.message !== error.reason ? error.message : ''
      const known = REASON_TEXT[error.reason] ?? ''
      // 服务端的 message 有时比通稿更有信息量（例如具体字段名），一并带上。
      return detail && detail.length < 120 ? `${known}（${detail}）` : known
    }
    return error.message || `请求失败（HTTP ${error.code}）`
  }
  if (error instanceof Error) return error.message
  return String(error)
}

/** 令牌与解封令牌的提供方，由 auth store 注册，避免循环依赖。 */
export interface TokenProvider {
  /** 当前访问令牌。 */
  getAccessToken(): string | null
  /** 用刷新令牌换一对新令牌；返回是否成功。 */
  refresh(): Promise<boolean>
  /** 续期失败时的兜底：清理会话并跳转登录。 */
  onUnauthenticated(): void
  /** 目标节点（或父目录）可用的解封令牌。 */
  nodeTokenFor?(targetNodeId?: string | null): string | undefined
}

let tokens: TokenProvider | null = null

export function setTokenProvider(provider: TokenProvider | null): void {
  tokens = provider
}

export interface RequestOptions {
  /** 查询参数：接受任何对象，空值与 undefined 会被跳过。 */
  query?: object
  body?: unknown
  headers?: Record<string, string>
  signal?: AbortSignal
  /** 目标节点，用于挑选 X-Node-Token；也可由请求参数推断。 */
  nodeId?: string | null
  /** 是否在 401 时自动用刷新令牌重试，默认开启。 */
  autoRefresh?: boolean
  /** 是否跳过令牌（公开接口）。 */
  anonymous?: boolean
  /** 原始请求体（流式上传等）。 */
  rawBody?: BodyInit
  /** 完全自定义：返回原始 Response，不做 JSON 解析。 */
  raw?: boolean
}

/** 把查询参数拼进 URL：空值跳过，数组按重复键展开。 */
export function buildQuery(query?: object): string {
  if (!query) return ''
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query as Record<string, unknown>)) {
    if (value === undefined || value === null) continue
    if (typeof value === 'string' && value === '') continue
    if (typeof value === 'boolean') {
      params.set(key, value ? 'true' : 'false')
      continue
    }
    if (Array.isArray(value)) {
      for (const item of value) {
        if (item === undefined || item === null || item === '') continue
        params.append(key, String(item))
      }
      continue
    }
    params.set(key, String(value))
  }
  const text = params.toString()
  return text ? `?${text}` : ''
}

/** 拼出绝对路径：同源部署返回相对路径，分离部署返回完整地址。 */
export function apiUrl(path: string, query?: object): string {
  const normalized = path.startsWith('/') ? path : `/${path}`
  return `${API_BASE}${normalized}${buildQuery(query)}`
}

/** 把 64 位整数与枚举等值安全地放进请求体：只保留真正要提交的字段。 */
export function compact<T extends Record<string, unknown>>(input: T): T {
  const output: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(input)) {
    if (value === undefined || value === null) continue
    if (typeof value === 'string' && value === '' && key !== 'parentId' && key !== 'targetParentId') continue
    output[key] = value
  }
  return output as T
}

async function parseError(response: Response): Promise<ApiError> {
  let body: ApiErrorBody = {}
  try {
    const text = await response.text()
    if (text) body = JSON.parse(text) as ApiErrorBody
  } catch {
    body = {}
  }
  return new ApiError({
    code: body.code ?? response.status,
    reason: body.reason ?? '',
    message: body.message ?? response.statusText,
    metadata: body.metadata,
  })
}

/** 单个 refresh 在途时，其它 401 请求等它，避免并发轮换互相吊销。 */
let refreshing: Promise<boolean> | null = null

async function refreshSession(): Promise<boolean> {
  if (!tokens) return false
  if (!refreshing) {
    refreshing = tokens
      .refresh()
      .catch(() => false)
      .finally(() => {
        refreshing = null
      })
  }
  return refreshing
}

async function send(
  method: string,
  path: string,
  options: RequestOptions,
  allowRetry: boolean,
): Promise<Response> {
  const headers = new Headers(options.headers ?? {})
  const hasBody = options.body !== undefined || options.rawBody !== undefined
  if (hasBody && !headers.has('Content-Type') && options.rawBody === undefined) {
    headers.set('Content-Type', 'application/json')
  }
  if (!options.anonymous && tokens) {
    const access = tokens.getAccessToken()
    if (access) headers.set('Authorization', `Bearer ${access}`)
    const nodeToken = tokens.nodeTokenFor?.(options.nodeId)
    if (nodeToken) headers.set('X-Node-Token', nodeToken)
  }
  const response = await fetch(apiUrl(`${API_PREFIX}${normalizePath(path)}`, options.query), {
    method,
    headers,
    body: options.rawBody ?? (options.body !== undefined ? JSON.stringify(options.body) : undefined),
    signal: options.signal,
    credentials: 'same-origin',
  })

  if (response.status === 401 && allowRetry && !options.anonymous && options.autoRefresh !== false) {
    const renewed = await refreshSession()
    if (renewed) {
      return send(method, path, options, false)
    }
    tokens?.onUnauthenticated()
  }
  return response
}

function normalizePath(path: string): string {
  if (path.startsWith('/v1/')) return path.slice(3)
  return path.startsWith('/') ? path : `/${path}`
}

async function request<T>(method: string, path: string, options: RequestOptions = {}): Promise<T> {
  const response = await send(method, path, options, true)
  if (!response.ok) {
    const error = await parseError(response)
    if (error.isUnauthenticated && !options.anonymous) tokens?.onUnauthenticated()
    throw error
  }
  if (response.status === 204) return undefined as T
  const text = await response.text()
  if (!text) return undefined as T
  return JSON.parse(text) as T
}

export const http = {
  get: <T>(path: string, options?: RequestOptions) => request<T>('GET', path, options),
  post: <T>(path: string, body?: unknown, options?: RequestOptions) =>
    request<T>('POST', path, { ...options, body }),
  put: <T>(path: string, body?: unknown, options?: RequestOptions) => request<T>('PUT', path, { ...options, body }),
  delete: <T>(path: string, options?: RequestOptions) => request<T>('DELETE', path, options),
  /** 需要自己处理响应（例如读取 ETag 响应头）时使用。 */
  raw: (method: string, path: string, options?: RequestOptions) =>
    send(method, path, { ...options, raw: true }, true),
}

/** 下载/预览地址：可能是同源签名地址，也可能指向对象存储。 */
export function resolveUrl(url?: string): string {
  if (!url) return ''
  if (/^https?:\/\//i.test(url)) return url
  if (url.startsWith('/')) return `${API_BASE}${url}`
  return `${API_BASE}/${url}`
}

/**
 * 带进度的原始字节上传。
 *
 * fetch 无法上报上传进度，而分片上传的进度条是这个页面的核心反馈，
 * 因此这里用 XHR，并保持与 fetch 相同的错误信封解析。
 */
export function uploadRaw(
  url: string,
  payload: Blob,
  handlers: {
    onProgress?: (loaded: number, total: number) => void
    signal?: AbortSignal
    headers?: Record<string, string>
  } = {},
): Promise<void> {
  return new Promise((resolve, reject) => {
    const xhr = new XMLHttpRequest()
    xhr.open('PUT', resolveUrl(url), true)
    xhr.setRequestHeader('Content-Type', 'application/octet-stream')
    for (const [key, value] of Object.entries(handlers.headers ?? {})) {
      xhr.setRequestHeader(key, value)
    }
    if (tokens) {
      const access = tokens.getAccessToken()
      if (access) xhr.setRequestHeader('Authorization', `Bearer ${access}`)
    }
    xhr.upload.addEventListener('progress', (event) => {
      if (event.lengthComputable) handlers.onProgress?.(event.loaded, event.total)
    })
    xhr.addEventListener('load', () => {
      if (xhr.status >= 200 && xhr.status < 300) {
        resolve()
        return
      }
      let body: ApiErrorBody = {}
      try {
        body = JSON.parse(xhr.responseText) as ApiErrorBody
      } catch {
        body = { code: xhr.status, message: xhr.statusText }
      }
      reject(new ApiError({ ...body, code: body.code ?? xhr.status }))
    })
    xhr.addEventListener('error', () => reject(new ApiError({ code: 0, message: '网络错误，上传中断' })))
    xhr.addEventListener('abort', () => reject(new ApiError({ code: 0, message: '上传已取消' })))
    if (handlers.signal) {
      if (handlers.signal.aborted) {
        xhr.abort()
        return
      }
      handlers.signal.addEventListener('abort', () => xhr.abort(), { once: true })
    }
    xhr.send(payload)
  })
}

/** 直接对对象存储的预签名地址 PUT 一片，并回读 ETag。 */
export async function putPresignedPart(
  url: string,
  payload: Blob,
  headers: Record<string, string> = {},
  signal?: AbortSignal,
): Promise<string> {
  const response = await fetch(resolveUrl(url), { method: 'PUT', body: payload, headers, signal })
  if (!response.ok) {
    throw new ApiError({ code: response.status, message: `分片直传失败（HTTP ${response.status}）` })
  }
  return response.headers.get('ETag') ?? ''
}
