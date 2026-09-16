<script setup lang="ts">
/** 下拉菜单：行内操作菜单、用户菜单、排序菜单共用。 */
import { onBeforeUnmount, onMounted, ref } from 'vue'
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

const open = ref(false)
const root = ref<HTMLElement | null>(null)

function toggle(): void {
  if (props.disabled) return
  open.value = !open.value
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
  if (root.value && !root.value.contains(event.target as Node)) close()
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') close()
}

onMounted(() => {
  document.addEventListener('mousedown', onDocumentPointer)
  document.addEventListener('keydown', onKeydown)
})

onBeforeUnmount(() => {
  document.removeEventListener('mousedown', onDocumentPointer)
  document.removeEventListener('keydown', onKeydown)
})

defineExpose({ close })
</script>

<template>
  <div ref="root" class="dropdown">
    <div class="dropdown__trigger" @click="toggle">
      <slot name="trigger" :open="open" :toggle="toggle" />
    </div>
    <Transition name="pop">
      <div
        v-if="open"
        class="dropdown__menu"
        :class="align === 'end' ? 'dropdown__menu--end' : 'dropdown__menu--start'"
        :style="{ width: `${width}px` }"
        role="menu"
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
  </div>
</template>

<style scoped>
.dropdown {
  position: relative;
  display: inline-flex;
}

.dropdown__trigger {
  display: inline-flex;
}

.dropdown__menu {
  position: absolute;
  top: calc(100% + 6px);
  z-index: var(--z-sticky);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
  padding: var(--space-1);
  max-height: 60vh;
  overflow-y: auto;
}

.dropdown__menu--end {
  right: 0;
}

.dropdown__menu--start {
  left: 0;
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
