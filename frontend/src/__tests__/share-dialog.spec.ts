import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { createPinia, setActivePinia } from 'pinia'
import { nextTick } from 'vue'

import ShareDialog from '@/components/files/ShareDialog.vue'
import type { Node, SystemInfo } from '@/api/types'
import { useSystemStore } from '@/stores/system'

// 弹窗 Teleport 到 body，用例之间必须卸载，否则残留的面板会被下一个用例找到。
enableAutoUnmount(afterEach)

/**
 * 分享口令受服务端的 auth.min_password_length 约束，越界时服务端只回一句笼统的
 * 「请求参数不合法（invalid argument）」（见 docs/api-guide.md 的 CreateShare 错误表），
 * 所以口令与自定义令牌必须在本地先拦一次。这里盯的是「拦住了就不该发请求」，
 * 以及「长度以服务端下发的为准」。
 */
describe('分享弹窗的提交前校验', () => {
  const fetchMock = vi.fn()

  beforeEach(() => {
    setActivePinia(createPinia())
    fetchMock.mockReset()
    fetchMock.mockResolvedValue(
      new Response(JSON.stringify({ shares: [] }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
    // 口令最小长度由 /v1/system/info 的 auth.minPasswordLength 下发。这里故意取 10，
    // 好确认页面用的是服务端策略而不是写死的 8。
    const system = useSystemStore()
    system.info = { features: ['public_share'], auth: { minPasswordLength: 10 } } as SystemInfo
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    document.body.innerHTML = ''
  })

  const node = {
    id: '01a0ae53-67b4-7196-8f39-68340c79a50e',
    name: '信息安全专业人才培养方案（2024版）.pdf',
    kind: 'NODE_KIND_FILE',
  } as unknown as Node

  /** 自定义令牌输入框：按 placeholder 找，免得依赖字段顺序。 */
  const TOKEN_INPUT = 'input[placeholder="留空则由服务端生成"]'

  async function openDialog() {
    const wrapper = mount(ShareDialog, { props: { modelValue: false, node } })
    await wrapper.setProps({ modelValue: true })
    await flushPromises()
    return wrapper
  }

  /** 创建链接相关的请求。 */
  function createCalls(): RequestInit[] {
    return fetchMock.mock.calls
      .filter((call) => String(call[0]).includes('/v1/shares/create'))
      .map((call) => call[1] as RequestInit)
  }

  async function fill(selector: string, value: string): Promise<void> {
    const input = document.body.querySelector<HTMLInputElement>(selector)
    if (!input) throw new Error(`找不到输入框：${selector}`)
    input.value = value
    input.dispatchEvent(new Event('input'))
    // 表单变化会清掉上一次的提示（watch(form)），先让它结算再点提交。
    await nextTick()
  }

  /** 创建链接的按钮挂在 Teleport 到 body 的面板里。 */
  async function clickCreate(): Promise<void> {
    const button = Array.from(document.body.querySelectorAll('button')).find((item) =>
      item.textContent?.includes('创建链接'),
    )
    if (!button) throw new Error('找不到「创建链接」按钮')
    button.click()
    await flushPromises()
  }

  it('口令短于服务端策略时不出网，并直接说清要几位', async () => {
    await openDialog()
    expect(document.body.textContent).toContain('至少 10 位')

    await fill('input[type="password"]', '123456789')
    await clickCreate()

    expect(createCalls()).toHaveLength(0)
    expect(document.body.textContent).toContain('访问密码至少 10 位')
  })

  it('自定义令牌不合法时不出网', async () => {
    await openDialog()

    await fill(TOKEN_INPUT, '我的链接')
    await clickCreate()

    expect(createCalls()).toHaveLength(0)
    expect(document.body.textContent).toContain('自定义令牌需要 8-64 位')
  })

  it('合法输入只提交一次，并把令牌原样带上', async () => {
    await openDialog()

    await fill(TOKEN_INPUT, 'share-token-1')
    await clickCreate()

    const calls = createCalls()
    expect(calls).toHaveLength(1)
    const body = JSON.parse(String(calls[0]?.body)) as Record<string, unknown>
    expect(body.token).toBe('share-token-1')
    expect(body.nodeId).toBe(node.id)
  })
})
