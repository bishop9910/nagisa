<script setup lang="ts">
/** 下拉菜单：行内操作菜单、用户菜单、排序菜单共用。 */
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AppIcon from './AppIcon.vue'
import type { DropdownItem } from './types'

const props = withDefaults(
  defineProps<{
    items?: DropdownItem[]
    align?: 'start' | 'end'
    width?: number
    disabled?: boolean
  }>(),
  { items: () => [], align: 'end', width: 200 },
)

const emit = defineEmits<{ (e: 'select', key: string): void }>()

/** 菜单与触发器之间的间距，以及与视口边缘留出的最小距离。 */
const GAP = 6
const MARGIN = 8

const open = ref(false)
const root = ref<HTMLElement | null>(null)
const trigger = ref<HTMLElement | null>(null)
const menu = ref<HTMLElement | null>(null)
/** 菜单挂在 body 上，位置只能自己算：top/left 与可用高度。 */
const position = ref({ top: 0, left: 0, maxHeight: 0 })

/** 按触发器位置摆菜单：默认在下方，下方放不下就翻到上方，横向不越出视口。 */
function place(): void {
  const anchor = trigger.value
  if (!anchor || typeof window === 'undefined') return
  const rect = anchor.getBoundingClientRect()
  const menuHeight = menu.value?.offsetHeight ?? 0
  const menuWidth = menu.value?.offsetWidth ?? props.width
  const viewportWidth = window.innerWidth
  const viewportHeight = window.innerHeight

  const below = rect.bottom + GAP
  const fitsBelow = menuHeight === 0 || below + menuHeight + MARGIN <= viewportHeight
  const top = fitsBelow ? below : Math.max(MARGIN, rect.top - GAP - menuHeight)

  const wanted = props.align === 'end' ? rect.right - menuWidth : rect.left
  const left = Math.min(Math.max(MARGIN, wanted), Math.max(MARGIN, viewportWidth - menuWidth - MARGIN))

  position.value = {
    top,
    left,
    maxHeight: Math.max(120, Math.min(viewportHeight * 0.6, viewportHeight - top - MARGIN)),
  }
}

async function toggle(): Promise<void> {
  if (props.disabled) return
  open.value = !open.value
  if (!open.value) return
  await nextTick()
  place()
  // 渲染完才知道菜单的真实高度，再量一次，决定要不要翻到触发器上方。
  await nextTick()
  place()
}

function close(): void {
  open.value = false
}

function pick(item: DropdownItem): void {
  if (item.divider || item.disabled) return
  emit('select', item.key)
  close()
}

function onDocumentPointer(event: MouseEvent): void {
  if (!open.value) return
  const target = event.target as Node
  // 菜单挂在 body 上而不是 root 里，所以要单独认一次，否则点菜单项会先被关掉。
  if (root.value?.contains(target) || menu.value?.contains(target)) return
  close()
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') close()
}

// 菜单是 fixed 定位，页面或表格滚动时要跟着触发器走。
watch(open, (value) => {
  if (value) {
    window.addEventListener('scroll', place, true)
    window.addEventListener('resize', place)
  } else {
    window.removeEventListener('scroll', place, true)
    window.removeEventListener('resize', place)
  }
})

onMounted(() => {
  document.addEventListener('mousedown', onDocumentPointer)
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocumentPointer)
  document.removeEventListener('keydown', onKeydown)
  window.removeEventListener('scroll', place, true)
  window.removeEventListener('resize', place)
})

defineExpose({ close })
</script>

<template>
  <div ref="root" class="dropdown">
    <!-- 点击与阻止冒泡都归触发器外壳管：槽里放什么按钮都不会漏掉 toggle，也不会把
         行内菜单的点击漏给整行。需要自己控制展开状态时可用槽参数 toggle。 -->
    <div ref="trigger" class="dropdown__trigger" @click.stop="toggle">
      <slot name="trigger" :open="open" :toggle="toggle" />
    </div>
    <!-- 菜单挂到 body：表格、卡片这类滚动容器裁不到它，也不会被它撑出滚动条。 -->
    <Teleport to="body">
      <Transition name="pop">
        <div
          v-if="open"
          ref="menu"
          class="dropdown__menu"
          role="menu"
          :style="{
            top: `${position.top}px`,
            left: `${position.left}px`,
            width: `${width}px`,
            maxHeight: `${position.maxHeight}px`,
          }"
        >
          <template v-for="item in items" :key="item.key">
            <div v-if="item.divider" class="dropdown__divider" />
            <button
              v-else
              type="button"
              class="dropdown__item"
              :class="{ 'is-danger': item.danger, 'is-disabled': item.disabled }"
              role="menuitem"
              :disabled="item.disabled"
              @click="pick(item)"
            >
              <AppIcon v-if="item.icon" :name="item.icon" :size="16" />
              <span class="dropdown__label">{{ item.label }}</span>
              <span v-if="item.hint" class="dropdown__hint">{{ item.hint }}</span>
            </button>
          </template>
          <slot />
        </div>
      </Transition>
    </Teleport>
  </div>
</template>

<style scoped>
.dropdown {
  display: inline-flex;
  /* 出现在表格单元格里时是行内盒，默认坐在基线上，会被行高在下面多撑出几像素。 */
  vertical-align: middle;
}

.dropdown__trigger {
  display: inline-flex;
}

.dropdown__menu {
  position: fixed;
  z-index: var(--z-sticky);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
  padding: var(--space-1);
  max-height: 60vh;
  overflow-y: auto;
}

.dropdown__item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  border: 0;
  background: transparent;
  color: var(--text);
  font-size: var(--text-sm);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  cursor: pointer;
  text-align: left;
}

.dropdown__item:hover:not(.is-disabled) {
  background: var(--surface-hover);
}

.dropdown__item.is-danger {
  color: var(--danger-600);
}

.dropdown__item.is-danger:hover {
  background: var(--danger-50);
}

.dropdown__item.is-disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.dropdown__label {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.dropdown__hint {
  color: var(--text-faint);
  font-size: var(--text-2xs);
}

.dropdown__divider {
  height: 1px;
  background: var(--border-subtle);
  margin: var(--space-1) 0;
}
</style>
