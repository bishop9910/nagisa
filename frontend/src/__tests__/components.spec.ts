import { describe, expect, it } from 'vitest'
import { mount, type VueWrapper } from '@vue/test-utils'

import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppField from '@/components/ui/AppField.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppPagination from '@/components/ui/AppPagination.vue'
import AppSegmented from '@/components/ui/AppSegmented.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'

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
