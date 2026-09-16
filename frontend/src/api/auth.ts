/** AuthService：握手参数、登录、续期、会话与改密。 */

import type { AuthConfig, LoginReply, SessionSet, User } from './types'
import { http } from './http'
import { encryptPassword } from '@/utils/crypto'

/** 握手参数只在密钥标识变化时重新拉取。 */
let cachedConfig: AuthConfig | null = null
let pending: Promise<AuthConfig> | null = null

export async function getAuthConfig(force = false): Promise<AuthConfig> {
  if (force) cachedConfig = null
  if (cachedConfig) return cachedConfig
  if (!pending) {
    pending = http
      .get<AuthConfig>('/v1/auth/config', { anonymous: true })
      .then((config) => {
        cachedConfig = config
        return config
      })
      .finally(() => {
        pending = null
      })
  }
  return pending
}

/** 预取公钥，登录页挂载时调用一次即可。 */
export async function warmupAuthConfig(): Promise<AuthConfig | null> {
  try {
    return await getAuthConfig()
  } catch {
    return null
  }
}

/** 用服务端公钥加密口令（所有口令字段都必须先经过这一步）。 */
export async function encodePassword(plain: string): Promise<string> {
  const config = await getAuthConfig()
  const pem = config.passwordPublicKey ?? ''
  if (!pem) {
    if (config.plainPasswordAllowed) return plain
    throw new Error('服务端未下发口令公钥，无法加密提交')
  }
  return encryptPassword(plain, pem)
}

export interface LoginInput {
  username: string
  password: string
  rememberMe?: boolean
}

export async function login(input: LoginInput): Promise<LoginReply> {
  const password = await encodePassword(input.password)
  return http.post<LoginReply>(
    '/v1/auth/login',
    {
      username: input.username,
      password,
      rememberMe: input.rememberMe ?? false,
      userAgent: typeof navigator === 'undefined' ? undefined : navigator.userAgent,
    },
    { anonymous: true },
  )
}

export function refreshToken(token: string): Promise<LoginReply> {
  return http.post<LoginReply>('/v1/auth/refresh', { refreshToken: token }, { anonymous: true, autoRefresh: false })
}

/**
 * 免登录的只读访客会话。返回的只有访问令牌，没有刷新令牌：访客没有需要轮换的会话，
 * 令牌过期后重新调用本接口即可。部署未开启时返回 NETDISK_UNSUPPORTED。
 */
export function guestLogin(): Promise<LoginReply> {
  return http.post<LoginReply>(
    '/v1/auth/guest',
    { userAgent: typeof navigator === 'undefined' ? undefined : navigator.userAgent },
    { anonymous: true, autoRefresh: false },
  )
}

export function logout(payload: { refreshToken?: string; revokeAll?: boolean }): Promise<void> {
  return http.post<void>('/v1/auth/logout', {
    refreshToken: payload.refreshToken,
    revokeAll: payload.revokeAll ?? false,
  })
}

export function getCurrentUser(): Promise<User> {
  return http.get<User>('/v1/auth/me')
}

export async function changePassword(current: string, next: string): Promise<void> {
  const [currentPassword, newPassword] = await Promise.all([encodePassword(current), encodePassword(next)])
  return http.post<void>('/v1/auth/password/change', { currentPassword, newPassword })
}

export function listSessions(params: { pageSize?: number; pageToken?: string; includeInactive?: boolean } = {}) {
  return http.get<SessionSet>('/v1/auth/sessions/list', { query: params })
}

export function revokeSession(id: string): Promise<void> {
  return http.post<void>('/v1/auth/sessions/revoke', { id })
}
