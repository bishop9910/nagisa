import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { ApiError, apiUrl, buildQuery, errorText, http, setTokenProvider } from '@/api/http'
import { encryptPassword } from '@/utils/crypto'

describe('查询串构造', () => {
  it('跳过空值并按重复键展开数组', () => {
    expect(buildQuery()).toBe('')
    expect(buildQuery({ a: undefined, b: null, c: '' })).toBe('')
    expect(buildQuery({ pageSize: 20, recursive: true, filter: undefined })).toBe('?pageSize=20&recursive=true')
    expect(buildQuery({ ids: ['1', '2'] })).toBe('?ids=1&ids=2')
  })

  it('拼接带 /v1 前缀的地址', () => {
    expect(apiUrl('/v1/nodes/list', { parentId: 'abc' })).toBe('/v1/nodes/list?parentId=abc')
  })
})

describe('错误信封', () => {
  it('按 reason 翻译成中文，并保留服务端细节', () => {
    const locked = new ApiError({
      code: 403,
      reason: 'NETDISK_NODE_LOCKED',
      message: 'node is password protected',
      metadata: { password_hint: '公司缩写' },
    })
    expect(locked.isLocked).toBe(true)
    expect(locked.passwordHint).toBe('公司缩写')
    expect(errorText(locked)).toContain('密码')

    const conflict = new ApiError({ code: 409, reason: 'NETDISK_NAME_CONFLICT', message: 'name already exists' })
    expect(errorText(conflict)).toContain('同名')

    const unknown = new ApiError({ code: 500, message: 'boom' })
    expect(errorText(unknown)).toBe('boom')
    expect(errorText(new Error('socket closed'))).toBe('socket closed')
  })
})

describe('HTTP 客户端', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
    setTokenProvider({
      getAccessToken: () => 'access-token',
      refresh: vi.fn(async () => false),
      onUnauthenticated: vi.fn(),
      nodeTokenFor: (id) => (id === 'node-1' ? 'unlock-token' : undefined),
    })
  })

  afterEach(() => {
    setTokenProvider(null)
    vi.unstubAllGlobals()
  })

  it('带上 Bearer 与 X-Node-Token，并解析 JSON 响应', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ nodes: [], totalSize: '0' }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    const result = await http.get<{ totalSize: string }>('/v1/nodes/list', {
      query: { parentId: 'node-1' },
      nodeId: 'node-1',
    })

    expect(result.totalSize).toBe('0')
    const [url, init] = fetchMock.mock.calls[0] ?? []
    expect(String(url)).toBe('/v1/nodes/list?parentId=node-1')
    const headers = (init as RequestInit).headers as Headers
    expect(headers.get('Authorization')).toBe('Bearer access-token')
    expect(headers.get('X-Node-Token')).toBe('unlock-token')
  })

  it('非 2xx 响应抛出 ApiError 并保留 reason', async () => {
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ code: 403, reason: 'NETDISK_PERMISSION_DENIED', message: 'permission denied' }), {
        status: 403,
        headers: { 'Content-Type': 'application/json' },
      }),
    )

    await expect(http.get('/v1/nodes/list')).rejects.toMatchObject({
      code: 403,
      reason: 'NETDISK_PERMISSION_DENIED',
    })
  })

  it('401 时先用刷新令牌换新令牌再重试一次', async () => {
    const refresh = vi.fn(async () => true)
    setTokenProvider({
      getAccessToken: () => 'access-token',
      refresh,
      onUnauthenticated: vi.fn(),
    })

    fetchMock
      .mockResolvedValueOnce(new Response('{"code":401,"reason":"NETDISK_UNAUTHENTICATED"}', { status: 401 }))
      .mockResolvedValueOnce(new Response('{"ok":true}', { status: 200 }))

    const result = await http.get<{ ok: boolean }>('/v1/auth/me')
    expect(refresh).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledTimes(2)
    expect(result.ok).toBe(true)
  })

  it('公开接口不附加令牌', async () => {
    fetchMock.mockResolvedValue(new Response('{}', { status: 200 }))
    await http.get('/v1/system/info', { anonymous: true })
    const init = fetchMock.mock.calls[0]?.[1] as RequestInit
    expect((init.headers as Headers).get('Authorization')).toBeNull()
  })
})

describe('口令加密', () => {
  it('生成可解密的 RSA-OAEP(SHA-256) 密文', async () => {
    const pair = await crypto.subtle.generateKey(
      { name: 'RSA-OAEP', modulusLength: 2048, publicExponent: new Uint8Array([1, 0, 1]), hash: 'SHA-256' },
      true,
      ['encrypt', 'decrypt'],
    )
    const spki = await crypto.subtle.exportKey('spki', pair.publicKey)
    const der = new Uint8Array(spki)
    let binary = ''
    for (const byte of der) binary += String.fromCharCode(byte)
    const pem = `-----BEGIN PUBLIC KEY-----\n${btoa(binary)}\n-----END PUBLIC KEY-----`

    const cipher = await encryptPassword('Admin@12345', pem)
    expect(cipher).not.toContain('Admin@12345')

    const raw = Uint8Array.from(atob(cipher), (char) => char.charCodeAt(0))
    const plain = await crypto.subtle.decrypt({ name: 'RSA-OAEP' }, pair.privateKey, raw)
    expect(new TextDecoder().decode(plain)).toBe('Admin@12345')
  })

  it('公钥为空时给出明确错误', async () => {
    await expect(encryptPassword('secret', '')).rejects.toThrow()
  })
})
