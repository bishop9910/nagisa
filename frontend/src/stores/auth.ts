/**
 * 会话与权限。
 *
 * 负责：令牌持久化与轮换、当前账号、解封令牌（X-Node-Token）的解析规则。
 * http 层通过 setTokenProvider 反向依赖这里，避免两个模块互相 import。
 *
 * 免登录的访客会话也在这里：没有令牌时先问服务端是否允许访客登录，允许就换一个
 * 只读访问令牌进来，因此「打开即只读浏览」不需要用户做任何事。本地躺着一对已经
 * 失效的令牌时同样落到访客态（见 loseSession），只有连访客入口都关着才跳登录页。
 */

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { authApi, setTokenProvider } from '@/api'
import type { Permission, User } from '@/api/types'
import { hasAnyPermission, hasPermission, maskFromPermissions, PERMISSION_ALL } from '@/utils/constants'

const STORAGE_KEY = 'nagisa.session'

interface StoredSession {
  accessToken: string
  refreshToken: string
  /** 访问令牌到期时刻（毫秒时间戳）。 */
  expiresAt: number
  /** 会话来自免登录的访客身份：续期靠重新换取，而不是刷新令牌。 */
  guest?: boolean
}

interface NodeTokenEntry {
  token: string
  expiresAt: number
}

/** 会话丢失后的去向：已换成只读访客，或彻底没有会话。 */
export type SessionLossReason = 'guest-fallback' | 'expired'

function readStored(): StoredSession | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    const parsed = JSON.parse(raw) as StoredSession
    // 访客会话没有刷新令牌，所以只要求访问令牌存在。
    if (!parsed.accessToken) return null
    return parsed
  } catch {
    return null
  }
}

function writeStored(session: StoredSession | null): void {
  try {
    if (!session) localStorage.removeItem(STORAGE_KEY)
    else localStorage.setItem(STORAGE_KEY, JSON.stringify(session))
  } catch {
    /* 隐私模式下 localStorage 可能不可用，忽略即可。 */
  }
}

export const useAuthStore = defineStore('auth', () => {
  const stored = readStored()

  const accessToken = ref<string | null>(stored?.accessToken ?? null)
  const refreshToken = ref<string | null>(stored?.refreshToken ?? null)
  const expiresAt = ref<number>(stored?.expiresAt ?? 0)
  /** 当前会话是不是免登录换来的只读访客身份。 */
  const guest = ref<boolean>(stored?.guest ?? false)
  const user = ref<User | null>(null)
  const ready = ref(false)
  /** 上一次会话是被判定失效而丢掉的，而不是用户主动退出。 */
  const sessionExpired = ref(false)
  /** 受密码保护目录的解封令牌：node_id → token。 */
  const nodeTokens = ref<Record<string, NodeTokenEntry>>({})
  /** 当前浏览路径上的祖先节点，用于挑选合适的解封令牌。 */
  const activeChain = ref<string[]>([])

  let expiredHandler: ((reason: SessionLossReason) => void) | null = null
  /**
   * 会话代数：建立或清空会话都会 +1。失效通知常常晚到（http 层与 bootstrap 都会
   * 收尾），比较代数就知道这期间会话有没有被换掉，避免把刚换到的会话又拆一遍。
   */
  let sessionGeneration = 0

  const isAuthenticated = computed(() => Boolean(accessToken.value))
  const isGuest = computed(() => guest.value)
  const permissionsMask = computed(() => Number(user.value?.permissionsMask ?? 0))
  const displayName = computed(() => user.value?.nickname || user.value?.username || '未登录')
  const role = computed(() => user.value?.role ?? 'ROLE_UNSPECIFIED')

  function has(...names: Permission[]): boolean {
    return hasPermission(permissionsMask.value, ...names)
  }

  function hasAny(...names: Permission[]): boolean {
    return hasAnyPermission(permissionsMask.value, ...names)
  }

  const isSuperAdmin = computed(() => role.value === 'ROLE_ADMIN')
  const canManageUsers = computed(() => has('PERMISSION_USER_MANAGE'))
  const canReadAudit = computed(() => has('PERMISSION_AUDIT_READ'))
  const canManageStorage = computed(() => has('PERMISSION_STORAGE_MANAGE'))
  const canManageSystem = computed(() => has('PERMISSION_SYSTEM_MANAGE'))
  /** 是否有任何管理后台可见。 */
  const canEnterManage = computed(
    () => canManageUsers.value || canReadAudit.value || canManageStorage.value || canManageSystem.value,
  )

  function setSession(payload: {
    accessToken?: string
    refreshToken?: string
    expiresIn?: number
    user?: User
    guest?: boolean
  }): void {
    if (payload.accessToken) accessToken.value = payload.accessToken
    if (payload.refreshToken) refreshToken.value = payload.refreshToken
    if (payload.guest !== undefined) guest.value = payload.guest
    if (payload.expiresIn) expiresAt.value = Date.now() + payload.expiresIn * 1000
    if (payload.user) user.value = payload.user
    sessionExpired.value = false
    sessionGeneration += 1
    if (accessToken.value) {
      writeStored({
        accessToken: accessToken.value,
        refreshToken: refreshToken.value ?? '',
        expiresAt: expiresAt.value,
        guest: guest.value,
      })
    }
  }

  function clearSession(): void {
    accessToken.value = null
    refreshToken.value = null
    expiresAt.value = 0
    guest.value = false
    user.value = null
    nodeTokens.value = {}
    activeChain.value = []
    writeStored(null)
    sessionGeneration += 1
  }

  /** 用刷新令牌换一对新令牌；http 层在 401 时会调用它。 */
  async function refresh(): Promise<boolean> {
    // 访客没有刷新令牌，直接再换一个只读访问令牌。
    if (guest.value) return enterGuestMode()
    if (!refreshToken.value) return false
    try {
      const reply = await authApi.refreshToken(refreshToken.value)
      setSession({
        accessToken: reply.accessToken,
        refreshToken: reply.refreshToken,
        expiresIn: reply.expiresIn,
        user: reply.user ?? undefined,
      })
      return true
    } catch {
      return false
    }
  }

  /**
   * 以访客身份进入：不输入任何口令就换到一个只读访问令牌。
   * 部署没有开启访客登录、或访客账号被停用时返回 false，调用方按未登录处理。
   */
  async function enterGuestMode(): Promise<boolean> {
    try {
      const reply = await authApi.guestLogin()
      if (!reply.accessToken) return false
      setSession({
        accessToken: reply.accessToken,
        expiresIn: reply.expiresIn,
        guest: true,
        user: reply.user ?? undefined,
      })
      if (!reply.user) {
        // 没有账号信息就读不到权限位，宁可当这次没换成，也不留半个会话。
        try {
          await loadCurrentUser()
        } catch {
          clearSession()
          return false
        }
      }
      return true
    } catch {
      return false
    }
  }

  /** 部署是否提供免登录的访客会话。 */
  async function guestLoginEnabled(): Promise<boolean> {
    try {
      const config = await authApi.getAuthConfig()
      return config.guestLoginEnabled === true
    } catch {
      return false
    }
  }

  /**
   * 先问部署开没开访客入口，开了就换一个只读会话。
   * 「打开网页即只读浏览」靠的就是它：本地没有会话、会话过期、令牌被服务端作废，
   * 都从这里落到访客态，而不是被丢到登录页。
   */
  async function tryGuestSession(): Promise<boolean> {
    if (!(await guestLoginEnabled())) return false
    return enterGuestMode()
  }

  /**
   * 当前会话作废后的统一收尾：清掉本地状态，能换访客会话就继续只读浏览，
   * 换不到才通知上层跳登录页。
   *
   * `since` 是调用方发起那次失败请求前记下的会话代数：代数已经变了就说明这次失效
   * 早就收尾过（或已经换成新会话），直接返回。已经在收尾时则等同一个流程，避免并发
   * 重复换取。
   */
  let losing: Promise<void> | null = null

  async function loseSession(retryGuest = true, since?: number): Promise<void> {
    if (losing) return losing
    if (since !== undefined && since !== sessionGeneration) return
    losing = (async () => {
      clearSession()
      sessionExpired.value = true
      if (retryGuest && (await tryGuestSession())) {
        expiredHandler?.('guest-fallback')
        return
      }
      expiredHandler?.('expired')
    })()
    try {
      await losing
    } finally {
      losing = null
    }
  }

  /** 令牌彻底失效：先尝试降级为只读访客，连访客都进不去才通知上层跳登录页。 */
  function handleUnauthenticated(): void {
    if (!accessToken.value && !refreshToken.value) return
    // 访客会话刚刚被服务端拒绝过（续期走的就是重新换取），再试一次没有意义。
    void loseSession(!guest.value)
  }

  function setExpiredHandler(handler: ((reason: SessionLossReason) => void) | null): void {
    expiredHandler = handler
  }

  /* ---------- 解封令牌 ---------- */

  function setNodeToken(nodeId: string, token: string, expiresInSeconds: number): void {
    nodeTokens.value = {
      ...nodeTokens.value,
      [nodeId]: { token, expiresAt: Date.now() + Math.max(1, expiresInSeconds) * 1000 },
    }
  }

  function clearNodeToken(nodeId: string): void {
    const next = { ...nodeTokens.value }
    delete next[nodeId]
    nodeTokens.value = next
  }

  function setActiveChain(ids: string[]): void {
    activeChain.value = ids
  }

  /** 目标节点（或当前目录的某个祖先）上可用的解封令牌。 */
  function nodeTokenFor(target?: string | null): string | undefined {
    const now = Date.now()
    const usable = (id?: string | null): string | undefined => {
      if (!id) return undefined
      const entry = nodeTokens.value[id]
      if (!entry) return undefined
      if (entry.expiresAt <= now) return undefined
      return entry.token
    }
    const direct = usable(target)
    if (direct) return direct
    for (const id of activeChain.value) {
      const token = usable(id)
      if (token) return token
    }
    return undefined
  }

  /* ---------- 生命周期 ---------- */

  let bootstrapped: Promise<void> | null = null

  /** 首次进入应用时恢复会话并校验令牌。 */
  async function bootstrap(): Promise<void> {
    if (ready.value) return
    if (bootstrapped) return bootstrapped
    bootstrapped = (async () => {
      if (!isAuthenticated.value) {
        // 没有会话时先尝试免登录的访客身份：这就是「打开即只读浏览」。
        await tryGuestSession()
        ready.value = true
        return
      }
      // 到期前 30 秒就主动续期，避免请求正好撞上过期。
      if (expiresAt.value && expiresAt.value - Date.now() < 30_000) {
        const before = sessionGeneration
        const renewed = await refresh()
        if (!renewed) {
          // 续期失败：本地这对令牌已经用不了了，落到访客态而不是登录页。
          await loseSession(!guest.value, before)
          ready.value = true
          return
        }
      }
      const before = sessionGeneration
      try {
        user.value = await authApi.getCurrentUser()
      } catch {
        // 令牌在服务端已失效（改密、被禁用、服务端换了签名密钥都是这样）：
        // 清掉本地状态，能换访客会话就继续只读浏览，而不是把人丢到登录页。
        await loseSession(!guest.value, before)
      } finally {
        ready.value = true
      }
    })()
    try {
      await bootstrapped
    } finally {
      bootstrapped = null
    }
  }

  async function login(username: string, password: string, rememberMe: boolean): Promise<void> {
    const reply = await authApi.login({ username, password, rememberMe })
    setSession({
      accessToken: reply.accessToken,
      refreshToken: reply.refreshToken,
      expiresIn: reply.expiresIn,
      user: reply.user ?? undefined,
      guest: false,
    })
    if (!reply.user) {
      await loadCurrentUser()
    }
  }

  async function loadCurrentUser(): Promise<void> {
    user.value = await authApi.getCurrentUser()
  }

  async function logout(options: { revokeAll?: boolean } = {}): Promise<void> {
    const token = refreshToken.value
    try {
      // 访客会话没有服务端记录可吊销，清掉本地令牌就够了。
      if (!guest.value && (options.revokeAll || token)) {
        await authApi.logout({ refreshToken: token ?? undefined, revokeAll: options.revokeAll ?? false })
      }
    } catch {
      /* 退出失败也要清掉本地状态。 */
    } finally {
      clearSession()
    }
  }

  function patchUser(patch: Partial<User>): void {
    user.value = { ...(user.value ?? {}), ...patch }
  }

  // 注册给 http 层：令牌读取、续期与解封令牌解析。
  setTokenProvider({
    getAccessToken: () => accessToken.value,
    refresh,
    onUnauthenticated: handleUnauthenticated,
    nodeTokenFor,
  })

  return {
    accessToken,
    refreshToken,
    expiresAt,
    guest,
    user,
    ready,
    sessionExpired,
    nodeTokens,
    activeChain,
    isAuthenticated,
    isGuest,
    permissionsMask,
    displayName,
    role,
    isSuperAdmin,
    canManageUsers,
    canReadAudit,
    canManageStorage,
    canManageSystem,
    canEnterManage,
    allPermissions: PERMISSION_ALL,
    has,
    hasAny,
    maskFromPermissions,
    setSession,
    clearSession,
    setExpiredHandler,
    refresh,
    guestLoginEnabled,
    enterGuestMode,
    setNodeToken,
    clearNodeToken,
    setActiveChain,
    nodeTokenFor,
    bootstrap,
    login,
    loadCurrentUser,
    logout,
    patchUser,
  }
})
