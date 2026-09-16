import { beforeEach, describe, expect, it, vi } from 'vitest'

/**
 * 免登录访客会话的前端行为：没有令牌时自动进入只读访客态，续期走 GuestLogin
 * 而不是刷新令牌，部署关掉开关时保持未登录。
 */

/** 记录请求并给出固定的 JSON 回复；未声明的路由返回 404 错误信封。 */
function stubApi(routes: Record<string, () => unknown>): string[] {
  const calls: string[] = []
  const fetchMock = vi.fn(async (url: unknown, init: RequestInit = {}) => {
    const key = `${init.method ?? 'GET'} ${String(url).split('?')[0]}`
    calls.push(key)
    const handler = routes[key]
    if (!handler) {
      return new Response(JSON.stringify({ code: 404, reason: 'NETDISK_NOT_FOUND' }), { status: 404 })
    }
    const result = handler()
    if (result instanceof Response) return result
    return new Response(JSON.stringify(result), { status: 200, headers: { 'Content-Type': 'application/json' } })
  })
  vi.stubGlobal('fetch', fetchMock)
  return calls
}

/** 服务端认定令牌不可用时返回的错误信封。 */
function unauthenticated(): Response {
  return new Response(JSON.stringify({ code: 401, reason: 'NETDISK_UNAUTHENTICATED' }), { status: 401 })
}

/** 本地存着一对已经不作数的正式账号令牌。 */
function storeStaleSession(): void {
  localStorage.setItem(
    'nagisa.session',
    JSON.stringify({
      accessToken: 'stale-token',
      refreshToken: 'stale-refresh',
      expiresAt: Date.now() + 3_600_000,
      guest: false,
    }),
  )
}

const guestReply = {
  accessToken: 'guest-token',
  tokenType: 'Bearer',
  expiresIn: 7200,
  user: { username: 'guest', nickname: '访客', role: 'ROLE_GUEST', permissionsMask: '3' },
}

/** 每个用例都拿一份干净的模块状态：auth config 在模块内被缓存。 */
async function loadStore() {
  vi.resetModules()
  const { createPinia, setActivePinia } = await import('pinia')
  setActivePinia(createPinia())
  const { useAuthStore } = await import('@/stores/auth')
  return useAuthStore()
}

describe('访客免登录', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('开启时自动进入只读访客态', async () => {
    const calls = stubApi({
      'GET /v1/auth/config': () => ({ guestLoginEnabled: true }),
      'POST /v1/auth/guest': () => guestReply,
    })
    const auth = await loadStore()

    await auth.bootstrap()

    expect(auth.isAuthenticated).toBe(true)
    expect(auth.isGuest).toBe(true)
    expect(auth.displayName).toBe('访客')
    // 访客没有刷新令牌，也就没有可以吊销的服务端会话。
    expect(auth.refreshToken).toBeNull()
    expect(auth.has('PERMISSION_VIEW')).toBe(true)
    expect(auth.has('PERMISSION_DOWNLOAD')).toBe(true)
    expect(auth.has('PERMISSION_UPLOAD')).toBe(false)
    expect(auth.has('PERMISSION_USER_MANAGE')).toBe(false)
    expect(calls).toEqual(['GET /v1/auth/config', 'POST /v1/auth/guest'])
  })

  it('关闭时保持未登录，且不去请求访客接口', async () => {
    const calls = stubApi({ 'GET /v1/auth/config': () => ({ guestLoginEnabled: false }) })
    const auth = await loadStore()

    await auth.bootstrap()

    expect(auth.isAuthenticated).toBe(false)
    expect(auth.isGuest).toBe(false)
    expect(calls).toEqual(['GET /v1/auth/config'])
  })

  it('续期重新换取访客令牌，而不是走刷新令牌', async () => {
    const calls = stubApi({
      'GET /v1/auth/config': () => ({ guestLoginEnabled: true }),
      'POST /v1/auth/guest': () => guestReply,
    })
    const auth = await loadStore()

    expect(await auth.enterGuestMode()).toBe(true)
    expect(await auth.refresh()).toBe(true)

    expect(calls.filter((call) => call === 'POST /v1/auth/guest')).toHaveLength(2)
    expect(calls).not.toContain('POST /v1/auth/refresh')
    expect(auth.isGuest).toBe(true)
  })

  it('正式登录后不再是访客身份', async () => {
    stubApi({
      // 没有公钥时 encodePassword 会退化成明文，这里只关心登录后的身份状态。
      'GET /v1/auth/config': () => ({ guestLoginEnabled: true, plainPasswordAllowed: true }),
      'POST /v1/auth/login': () => ({
        accessToken: 'admin-token',
        refreshToken: 'refresh-token',
        expiresIn: 7200,
        user: { username: 'admin', role: 'ROLE_ADMIN', permissionsMask: '4095' },
      }),
    })
    const auth = await loadStore()

    await auth.login('admin', 'secret', false)

    expect(auth.isGuest).toBe(false)
    expect(auth.refreshToken).toBe('refresh-token')
    expect(auth.has('PERMISSION_USER_MANAGE')).toBe(true)
  })

  it('本地令牌已失效时落到只读访客，而不是把人丢到登录页', async () => {
    storeStaleSession()
    const calls = stubApi({
      'GET /v1/auth/config': () => ({ guestLoginEnabled: true }),
      'GET /v1/auth/me': () => unauthenticated(),
      'POST /v1/auth/guest': () => guestReply,
    })
    const auth = await loadStore()
    const reasons: string[] = []
    auth.setExpiredHandler((reason) => reasons.push(reason))

    await auth.bootstrap()

    expect(auth.isAuthenticated).toBe(true)
    expect(auth.isGuest).toBe(true)
    expect(reasons).toEqual(['guest-fallback'])
    // 401 与 bootstrap 都会走一次收尾，访客接口只能被换一次。
    expect(calls.filter((call) => call === 'POST /v1/auth/guest')).toHaveLength(1)
  })

  it('访客入口关着且本地令牌已失效时才按未登录处理', async () => {
    storeStaleSession()
    const calls = stubApi({
      'GET /v1/auth/config': () => ({ guestLoginEnabled: false }),
      'GET /v1/auth/me': () => unauthenticated(),
    })
    const auth = await loadStore()
    const reasons: string[] = []
    auth.setExpiredHandler((reason) => reasons.push(reason))

    await auth.bootstrap()

    expect(auth.isAuthenticated).toBe(false)
    expect(auth.isGuest).toBe(false)
    expect(reasons).toEqual(['expired'])
    expect(calls).not.toContain('POST /v1/auth/guest')
  })

  it('正式会话在请求中失效时降级为访客会话', async () => {
    const calls = stubApi({
      'GET /v1/auth/config': () => ({ guestLoginEnabled: true }),
      'GET /v1/auth/me': () => unauthenticated(),
      'POST /v1/auth/guest': () => guestReply,
    })
    const auth = await loadStore()
    auth.setSession({ accessToken: 'dead-token', refreshToken: 'dead-refresh', expiresIn: 7200 })
    const reasons: string[] = []
    auth.setExpiredHandler((reason) => reasons.push(reason))

    await expect(auth.loadCurrentUser()).rejects.toThrow()

    await vi.waitFor(() => expect(reasons).toEqual(['guest-fallback']))
    expect(auth.isGuest).toBe(true)
    expect(calls).toContain('POST /v1/auth/guest')
  })
})
