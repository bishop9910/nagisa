import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'

import AccountView from '@/views/AccountView.vue'
import { useAuthStore } from '@/stores/auth'

/**
 * 超级管理员是内置账号，管理后台里不可被任何人管理，自助改密入口对他没有意义，
 * 所以账号设置里不给他「密码」这一页；普通账号必须照旧能看到。
 */

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

async function mountView(user: Record<string, unknown>) {
  setActivePinia(createPinia())
  const auth = useAuthStore()
  auth.setSession({ accessToken: 'token', user: user as never })

  const wrapper = mount(AccountView)
  await flushPromises()
  return wrapper
}

function tabLabels(wrapper: Awaited<ReturnType<typeof mountView>>): string[] {
  return wrapper.findAll('.tabs__item').map((tab) => tab.text())
}

describe('账号设置的改密入口', () => {
  beforeEach(() => {
    localStorage.clear()
    stubApi({
      'GET /v1/users/stats/me': () => ({ fileCount: '2', folderCount: '1' }),
      'GET /v1/users/permissions/catalog': () => ({ grantedMask: '4095' }),
      'GET /v1/auth/sessions/list': () => ({ sessions: [] }),
    })
  })

  it('超级管理员看不到「密码」页，并且明确说明原因', async () => {
    const wrapper = await mountView({
      username: 'admin',
      nickname: '超级管理员',
      role: 'ROLE_ADMIN',
      permissionsMask: '4095',
    })

    const labels = tabLabels(wrapper)
    expect(labels).toContain('资料与用量')
    expect(labels).not.toContain('密码')
    expect(wrapper.text()).toContain('不提供改密入口')
    expect(wrapper.findAll('input[type="password"]')).toHaveLength(0)
  })

  it('普通账号照旧能改密码', async () => {
    const wrapper = await mountView({
      username: 'alice',
      nickname: 'Alice',
      role: 'ROLE_USER',
      permissionsMask: '15',
    })

    expect(tabLabels(wrapper)).toContain('密码')
    expect(wrapper.text()).not.toContain('不提供改密入口')

    const security = wrapper.findAll('.tabs__item').find((tab) => tab.text() === '密码')
    await security?.trigger('click')

    expect(wrapper.findAll('input[type="password"]')).toHaveLength(3)
  })
})
