import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

import ManageOverview from '@/views/manage/ManageOverview.vue'
import { useAuthStore } from '@/stores/auth'

/**
 * 管理概览只保留最基本的服务标识：详细的部署参数、运行状态与存储摘要都不在这里，
 * 它们分别在「系统信息」（system_manage）与「存储与维护」（storage_manage）里。
 */

const SYSTEM_MANAGE = 1 << 11

function stubApi(routes: Record<string, () => unknown>): void {
  const fetchMock = vi.fn(async (url: unknown, init: RequestInit = {}) => {
    const key = `${init.method ?? 'GET'} ${String(url).split('?')[0]}`
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
}

async function mountView(permissionsMask: number) {
  setActivePinia(createPinia())
  const auth = useAuthStore()
  auth.setSession({
    accessToken: 'token',
    user: { username: 'admin', role: 'ROLE_ADMIN', permissionsMask: String(permissionsMask) } as never,
  })
  const wrapper = mount(ManageOverview, { global: { stubs: { RouterLink: { template: '<a><slot /></a>' } } } })
  await flushPromises()
  return wrapper
}

const info = {
  name: 'Nagisa 网盘',
  version: 'v1.3.8',
  apiVersion: 'v1',
  features: ['acl', 'trash'],
  maxUploadSize: '0',
  uploadSessionTtlSeconds: 86400,
  storageBackend: 's3',
  databaseBackend: 'sqlite',
}

describe('管理概览', () => {
  beforeEach(() => {
    localStorage.clear()
  })

  it('只显示最基本的服务标识，并指引到系统信息', async () => {
    stubApi({ 'GET /v1/system/info': () => info })

    const wrapper = await mountView(SYSTEM_MANAGE)
    const text = wrapper.text()

    expect(text).toContain('Nagisa 网盘')
    expect(text).toContain('v1.3.8')
    expect(text).toContain('v1')

    // 详细的部署信息、健康检查与存储摘要都搬走了。
    expect(text).not.toContain('最大单文件')
    expect(text).not.toContain('上传会话有效期')
    expect(text).not.toContain('健康检查')
    expect(text).not.toContain('存储概览')
    expect(text).not.toContain('对象存储')

    expect(text).toContain('系统信息')
  })

  it('没有系统信息权限时不给出跳转指引', async () => {
    stubApi({ 'GET /v1/system/info': () => info })

    const wrapper = await mountView(1)

    expect(wrapper.text()).not.toContain('系统信息')
  })
})
