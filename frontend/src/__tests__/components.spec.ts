import { afterEach, describe, expect, it } from 'vitest'
import { defineComponent, nextTick } from 'vue'
import { enableAutoUnmount, mount, type VueWrapper } from '@vue/test-utils'

import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppDropdown from '@/components/ui/AppDropdown.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppField from '@/components/ui/AppField.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppPagination from '@/components/ui/AppPagination.vue'
import AppSegmented from '@/components/ui/AppSegmented.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import NodeActions from '@/components/files/NodeActions.vue'
import type { Node } from '@/api/types'

// 下拉菜单会 Teleport 到 body，用例之间必须卸载，否则残留的菜单会被下一个用例找到。
enableAutoUnmount(afterEach)

/** 最近一次事件负载（tsconfig.vitest 的 lib 为空，不使用 Array.prototype.at）。 */
function lastPayload(wrapper: VueWrapper, event: string): unknown[] | undefined {
  const events = wrapper.emitted(event)
  if (!events || events.length === 0) return undefined
  return events[events.length - 1]
}

describe('AppIcon', () => {
  it('渲染内置图标路径，未知名字回落为通用文件图标', () => {
    const known = mount(AppIcon, { props: { name: 'folder' } })
    expect(known.findAll('path').length).toBeGreaterThan(0)

    const unknown = mount(AppIcon, { props: { name: '不存在的图标' } })
    expect(unknown.findAll('path').length).toBeGreaterThan(0)
    expect(unknown.attributes('aria-hidden')).toBe('true')
  })
})

describe('AppButton', () => {
  it('渲染文字并抛出 click 事件', async () => {
    const wrapper = mount(AppButton, { slots: { default: '上传' } })
    expect(wrapper.text()).toContain('上传')
    await wrapper.trigger('click')
    expect(wrapper.emitted('click')).toHaveLength(1)
  })

  it('loading 状态下禁用并设置 aria-busy', () => {
    const wrapper = mount(AppButton, { props: { loading: true }, slots: { default: '保存' } })
    expect(wrapper.attributes('disabled')).toBeDefined()
    expect(wrapper.attributes('aria-busy')).toBe('true')
  })

  it('仅图标按钮使用 label 作为无障碍名称', () => {
    const wrapper = mount(AppButton, { props: { icon: 'trash', label: '删除' } })
    expect(wrapper.attributes('aria-label')).toBe('删除')
    expect(wrapper.text()).toBe('')
  })

  // 踩过的坑：isIconOnly 只看 icon 与 label，不看插槽，于是所有「图标 + 文字」的
  // 按钮（创建链接、上传、下载、搜索……）都只剩一个图标，文字和可访问名称全没了。
  it('带插槽的按钮即使有 icon 也要保留文字', () => {
    const wrapper = mount(AppButton, { props: { icon: 'share' }, slots: { default: '创建链接' } })
    expect(wrapper.text()).toBe('创建链接')
    expect(wrapper.classes()).not.toContain('btn--icon')
    expect(wrapper.attributes('aria-label')).toBeUndefined()
  })
})

describe('表单控件', () => {
  it('AppInput 双向绑定并支持清空', async () => {
    const wrapper = mount(AppInput, { props: { modelValue: 'abc', clearable: true } })
    const input = wrapper.find('input')
    expect((input.element as HTMLInputElement).value).toBe('abc')

    await input.setValue('abcd')
    expect(lastPayload(wrapper, 'update:modelValue')).toEqual(['abcd'])

    await wrapper.find('.input__clear').trigger('click')
    expect(lastPayload(wrapper, 'update:modelValue')).toEqual([''])
  })

  it('AppInput 回车时抛出 enter 事件', async () => {
    const wrapper = mount(AppInput, { props: { modelValue: 'alice' } })
    await wrapper.find('input').trigger('keyup.enter')
    expect(wrapper.emitted('enter')?.[0]).toEqual(['alice'])
  })

  it('AppCheckbox 切换选中状态', async () => {
    const wrapper = mount(AppCheckbox, { props: { modelValue: false, label: '查看' } })
    expect(wrapper.text()).toContain('查看')
    await wrapper.find('input').setValue(true)
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([true])
  })

  it('AppSwitch 暴露 role=switch 与 aria-checked', async () => {
    const wrapper = mount(AppSwitch, { props: { modelValue: false, label: '启用' } })
    expect(wrapper.attributes('role')).toBe('switch')
    expect(wrapper.attributes('aria-checked')).toBe('false')
    await wrapper.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual([true])
  })

  it('AppSegmented 只在选项间切换', async () => {
    const wrapper = mount(AppSegmented, {
      props: {
        modelValue: 'list',
        options: [
          { value: 'list', label: '列表' },
          { value: 'grid', label: '网格' },
        ],
      },
    })
    const buttons = wrapper.findAll('.segmented__item')
    expect(buttons).toHaveLength(2)
    expect(buttons[0]?.attributes('aria-selected')).toBe('true')
    await buttons[1]?.trigger('click')
    expect(wrapper.emitted('update:modelValue')?.[0]).toEqual(['grid'])
  })

  it('AppField 展示标签、必填标记与错误信息', () => {
    const wrapper = mount(AppField, {
      props: { label: '用户名', required: true, error: '不能为空', hint: '提示' },
    })
    expect(wrapper.text()).toContain('用户名')
    expect(wrapper.find('.req').exists()).toBe(true)
    expect(wrapper.find('.field__error').text()).toBe('不能为空')
    expect(wrapper.find('.field__hint').exists()).toBe(false)
  })
})

describe('展示组件', () => {
  it('AppBadge 按色调渲染', () => {
    const wrapper = mount(AppBadge, { props: { tone: 'danger' }, slots: { default: '失败' } })
    expect(wrapper.classes()).toContain('badge--danger')
    expect(wrapper.text()).toBe('失败')
  })

  it('AppEmpty 渲染标题、说明与操作槽', () => {
    const wrapper = mount(AppEmpty, {
      props: { title: '这里还是空的', description: '拖拽文件到此处' },
      slots: { default: '<button>上传文件</button>' },
    })
    expect(wrapper.text()).toContain('这里还是空的')
    expect(wrapper.text()).toContain('拖拽文件到此处')
    expect(wrapper.find('.empty__actions button').exists()).toBe(true)
  })

  it('AppPagination 按可用性禁用翻页按钮', async () => {
    const wrapper = mount(AppPagination, {
      props: { page: 1, pageSize: 20, total: 100, hasPrev: false, hasNext: true, pageSizes: [20] },
    })
    expect(wrapper.text()).toContain('1-20')
    const buttons = wrapper.findAll('button')
    const prev = buttons.find((button) => button.attributes('aria-label') === '上一页')
    const next = buttons.find((button) => button.attributes('aria-label') === '下一页')
    expect(prev?.attributes('disabled')).toBeDefined()
    expect(next?.attributes('disabled')).toBeUndefined()

    await next?.trigger('click')
    expect(wrapper.emitted('next')).toHaveLength(1)
  })
})

describe('下拉菜单', () => {
  /** 菜单挂在 body 上，所以只能从 document 里找。 */
  function menuInBody(): HTMLElement | null {
    return document.body.querySelector('.dropdown__menu')
  }

  it('点击触发器展开菜单，选择后回调并收起', async () => {
    const wrapper = mount(AppDropdown, {
      props: {
        items: [
          { key: 'open', label: '打开' },
          { key: 'delete', label: '删除', danger: true },
        ],
      },
      slots: { trigger: '<button class="probe-trigger">更多</button>' },
    })
    expect(menuInBody()).toBeNull()

    await wrapper.find('.probe-trigger').trigger('click')

    expect(menuInBody()?.textContent).toContain('打开')
    expect(menuInBody()?.textContent).toContain('删除')

    document.body.querySelectorAll<HTMLElement>('.dropdown__item')[1]?.click()
    await wrapper.vm.$nextTick()

    expect(wrapper.emitted('select')?.[0]).toEqual(['delete'])
    await nextTick()
    expect(menuInBody()).toBeNull()
  })

  it('菜单不留在触发器所在的容器里（不然会把表格撑出滚动条或被裁掉）', async () => {
    const wrapper = mount(AppDropdown, {
      props: { items: [{ key: 'open', label: '打开' }] },
      slots: { trigger: '<button class="probe-trigger">更多</button>' },
    })

    await wrapper.find('.probe-trigger').trigger('click')

    const menu = menuInBody()
    expect(menu).not.toBeNull()
    expect(wrapper.find('.dropdown__menu').exists()).toBe(false)
    expect(wrapper.element.contains(menu)).toBe(false)
    expect(wrapper.element.children).toHaveLength(1)
  })

  it('点菜单项本身不会先被外部点击逻辑关掉', async () => {
    const wrapper = mount(AppDropdown, {
      props: { items: [{ key: 'open', label: '打开' }] },
      slots: { trigger: '<button class="probe-trigger">更多</button>' },
    })
    await wrapper.find('.probe-trigger').trigger('click')

    menuInBody()?.querySelector('.dropdown__item')?.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }))

    expect(menuInBody()).not.toBeNull()
  })

  it('触发器上的点击不会冒泡给外层容器', async () => {
    const rowClicks: string[] = []
    const Host = defineComponent({
      components: { AppDropdown },
      setup: () => ({ items: [{ key: 'open', label: '打开' }], onRow: () => rowClicks.push('row') }),
      template: `<div class="row" @click="onRow">
        <AppDropdown :items="items">
          <template #trigger><button class="probe-trigger">更多</button></template>
        </AppDropdown>
      </div>`,
    })
    const wrapper = mount(Host)

    await wrapper.find('.probe-trigger').trigger('click')

    expect(rowClicks).toEqual([])
    expect(menuInBody()).not.toBeNull()
  })
})

describe('文件行的操作菜单', () => {
  // 踩过的坑：触发器按钮上写 @click.stop 会把事件拦在 toggle 之前，点 ⋯ 毫无反应。
  it('点「更多操作」能展开菜单', async () => {
    const wrapper = mount(NodeActions, {
      props: {
        node: {
          id: 'node-1',
          name: 'docs',
          kind: 'NODE_KIND_FOLDER',
          effectivePermissionsMask: '4095',
        } as unknown as Node,
      },
    })

    await wrapper.find('.node-actions').trigger('click')

    const menu = document.body.querySelector('.dropdown__menu')
    expect(menu).not.toBeNull()
    expect(menu?.textContent).toContain('打开')
    expect(menu?.textContent).toContain('下载为 ZIP')
    // 菜单不在单元格里，格子只剩那个 28px 的按钮。
    expect(wrapper.find('.dropdown__menu').exists()).toBe(false)
  })

  it('回收站里的文件夹多一个「打开」入口，用来走进被删的层级', async () => {
    const wrapper = mount(NodeActions, {
      props: {
        context: 'trash',
        node: {
          id: 'folder-1',
          name: 'docs',
          kind: 'NODE_KIND_FOLDER',
        } as unknown as Node,
      },
    })

    await wrapper.find('.node-actions').trigger('click')

    const menu = document.body.querySelector('.dropdown__menu')
    expect(menu?.textContent).toContain('打开')
    expect(menu?.textContent).toContain('还原')
    expect(menu?.textContent).toContain('彻底删除')
  })
})
