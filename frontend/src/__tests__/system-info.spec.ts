import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

import ManageSystemInfo from '@/views/manage/ManageSystemInfo.vue'
import { useAuthStore } from '@/stores/auth'

/**
 * 系统信息页是只读的：它取代了原来那个「运行参数」编辑器——那些参数在服务端没有任何
 * 逻辑回读，写进去也不生效，所以页面不再提供任何写回入口，只展示真正生效的值。
 */

/** 存储管理（1<<10）+ 系统信息（1<<11）：管理员默认两样都有。 */
const ADMIN_MASK = 1024 | 2048

function stubApi(routes: Record<string, () => unknown>): string[] {
  const calls: string[] = []
  const fetchMock = vi.fn(async (url: unknown, init: RequestInit = {}) => {
    const key = `${init.method ?? 'GET'} ${String(url).split('?')[0]}`
    calls.push(key)
    const handler = routes[key]
    if (!handler) {
      return new Response(JSON.stringify({ code: 404, reason: 'NETDISK_NOT_FOUND' }), { status: 404 })
    }
    return new Response(JSON.stringify(handler()), {
      status: 200,
      headers: { 'Content-Type': 'application/json' },
    })
  })
  vi.stubGlobal('fetch', fetchMock)
  return calls
}

const info = {
  name: 'Nagisa 网盘',
  version: 'v1.3.6',
  apiVersion: 'v1',
  features: ['public_share'],
  uploadModes: ['UPLOAD_MODE_PROXY'],
  maxUploadSize: '0',
  defaultChunkSize: '8388608',
  minChunkSize: '5242880',
  maxInlineSize: '4194304',
  uploadSessionTtlSeconds: 86400,
  signedUrlTtlSeconds: 1800,
  signedUrlMaxTtlSeconds: 604800,
  storageBackend: 's3',
  databaseBackend: 'sqlite',
  defaultVisibility: 'VISIBILITY_PRIVATE',
  publicBaseUrl: '',
  auth: {
    passwordEncoding: 'RSA-OAEP-SHA256',
    passwordKeyId: 'key-1',
    plainPasswordAllowed: false,
    guestLoginEnabled: true,
    minPasswordLength: 10,
    accessTokenTtlSeconds: 7200,
    refreshTokenTtlSeconds: 2592000,
  },
}

const settings = {
  settings: [
    { key: 'upload.max_versions', value: '20', type: 'int', description: '保留的历史版本数', writable: false },
    { key: 'share.allow_public', value: 'true', type: 'bool', description: '允许公开分享' },
  ],
}

async function mountView(permissionsMask = ADMIN_MASK) {
  setActivePinia(createPinia())
  const auth = useAuthStore()
  auth.setSession({
    accessToken: 'token',
    user: { username: 'admin', nickname: '超级管理员', role: 'ROLE_ADMIN', permissionsMask: String(permissionsMask) } as never,
  })
  const wrapper = mount(ManageSystemInfo)
  await flushPromises()
  return wrapper
}

describe('系统信息页', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('展示生效的策略，并回落到服务端下发的口令最小长度', async () => {
    stubApi({
      'GET /v1/system/info': () => info,
      'GET /v1/system/health': () => ({ status: 'ok', checks: { database: 'ok' }, uptimeSeconds: 7200 }),
      'GET /v1/system/settings/list': () => settings,
    })

    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).toContain('服务与运行状态')
    expect(text).toContain('生效的策略与上限')
    expect(text).toContain('Nagisa 网盘')
    expect(text).toContain('v1.3.6')
    // 口令最小长度来自 /v1/system/info，不再写死 8。
    expect(text).toContain('10 位')
    // 上传上限按服务端下发的值展示，0 表示不限制。
    expect(text).toContain('不限制')
  })

  it('只读展示服务端登记的运行参数，不给任何写回入口', async () => {
    stubApi({
      'GET /v1/system/info': () => info,
      'GET /v1/system/health': () => ({ status: 'ok', checks: {} }),
      'GET /v1/system/settings/list': () => settings,
    })

    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).toContain('服务端登记的运行参数')
    expect(text).toContain('upload.max_versions')
    expect(text).toContain('保留的历史版本数')
    // writable=false 的条目带只读标记。
    expect(text).toContain('只读')
    // 页面里既没有输入框，也没有保存/撤销这类会让人以为能改的按钮。
    expect(wrapper.findAll('input')).toHaveLength(0)
    expect(wrapper.findAll('textarea')).toHaveLength(0)
    expect(text).not.toContain('保存')
    expect(text).not.toContain('撤销修改')
  })

  it('参数列表读不到时给出重试入口，不影响上面的只读信息', async () => {
    stubApi({
      'GET /v1/system/info': () => info,
      'GET /v1/system/health': () => ({ status: 'ok', checks: {} }),
    })

    const wrapper = await mountView()
    const text = wrapper.text()

    expect(text).toContain('读取失败')
    expect(text).toContain('重试')
    expect(text).toContain('生效的策略与上限')
  })

  // 列表接口在服务端由 storage_manage 把关（写入才是 system_manage），
  // 只有「系统信息」权限的账号不该发出这个注定 403 的请求。
  it('没有存储管理权限时不请求参数列表，并说明原因', async () => {
    const calls = stubApi({
      'GET /v1/system/info': () => info,
      'GET /v1/system/health': () => ({ status: 'ok', checks: {} }),
      'GET /v1/system/settings/list': () => settings,
    })

    const wrapper = await mountView(2048)

    expect(calls).not.toContain('GET /v1/system/settings/list')
    expect(wrapper.text()).toContain('需要「存储管理」权限')
    // 公开信息照旧展示。
    expect(wrapper.text()).toContain('生效的策略与上限')
  })
})
