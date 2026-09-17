import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createPinia, setActivePinia } from 'pinia'
import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'

import TrashView from '@/views/TrashView.vue'

// 下拉菜单会 Teleport 到 body，用例之间必须卸载。
enableAutoUnmount(afterEach)

/**
 * 回收站的层级：顶层只列被删的那一项，走进去才按 originalParentId 列文件夹里面的条目。
 */
describe('回收站的层级', () => {
  const folder = {
    id: 'folder-1',
    name: 'docs',
    kind: 'NODE_KIND_FOLDER',
    trashedAt: '2026-01-01T00:00:00Z',
  }
  const inner = {
    id: 'file-1',
    name: 'contract.pdf',
    kind: 'NODE_KIND_FILE',
    size: '2048',
    trashedAt: '2026-01-01T00:00:00Z',
  }

  const calls: string[] = []

  beforeEach(() => {
    calls.length = 0
    const fetchMock = vi.fn(async (url: unknown) => {
      const target = String(url)
      calls.push(target)
      const nodes = target.includes('originalParentId=') ? [inner] : [folder]
      return new Response(JSON.stringify({ nodes, totalSize: String(nodes.length) }), {
        status: 200,
        headers: { 'Content-Type': 'application/json' },
      })
    })
    vi.stubGlobal('fetch', fetchMock)
    setActivePinia(createPinia())
  })

  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('顶层只取一层，进入文件夹后按该文件夹再取一层', async () => {
    const wrapper = mount(TrashView, { global: { plugins: [createPinia()] } })
    await flushPromises()

    expect(calls[0]).toContain('/v1/nodes/trash/list')
    expect(calls[0]).not.toContain('originalParentId')
    expect(wrapper.text()).toContain('docs')

    await wrapper.find('.file-table__name').trigger('click')
    await flushPromises()

    expect(calls[1]).toContain('originalParentId=folder-1')
    expect(wrapper.text()).toContain('contract.pdf')
    // 面包屑给出回到顶层的入口。
    expect(wrapper.find('.crumbs').text()).toContain('回收站')
  })

  it('面包屑点回收站回到顶层', async () => {
    const wrapper = mount(TrashView, { global: { plugins: [createPinia()] } })
    await flushPromises()
    await wrapper.find('.file-table__name').trigger('click')
    await flushPromises()

    await wrapper.find('.crumbs__item').trigger('click')
    await flushPromises()

    expect(calls[calls.length - 1]).not.toContain('originalParentId')
    expect(wrapper.text()).toContain('docs')
  })
})
