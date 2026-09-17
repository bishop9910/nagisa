<script setup lang="ts">
/** 按钮：统一高度、圆角、按压反馈与加载态。 */
import { computed, useSlots } from 'vue'
import AppIcon from './AppIcon.vue'

const props = withDefaults(
  defineProps<{
    variant?: 'primary' | 'default' | 'ghost' | 'soft' | 'danger'
    size?: 'sm' | 'md' | 'lg'
    type?: 'button' | 'submit' | 'reset'
    icon?: string
    iconRight?: string
    loading?: boolean
    disabled?: boolean
    block?: boolean
    /** 仅图标按钮的无障碍名称。 */
    label?: string
    active?: boolean
  }>(),
  { variant: 'default', size: 'md', type: 'button' },
)

defineEmits<{ (e: 'click', event: MouseEvent): void }>()

const slots = useSlots()

// 带默认插槽的按钮即使有 icon 也不是「仅图标」：漏掉这一条会把按钮文字整个丢掉。
const isIconOnly = computed(() => Boolean(props.icon) && !props.label && !props.iconRight && !slots.default)
</script>

<template>
  <button
    :type="type"
    class="btn"
    :class="[`btn--${variant}`, `btn--${size}`, { 'btn--block': block, 'btn--icon': isIconOnly, 'is-active': active }]"
    :disabled="disabled || loading"
    :aria-label="label"
    :aria-busy="loading ? 'true' : undefined"
    :title="label"
    @click="$emit('click', $event)"
  >
    <AppIcon v-if="loading" name="refresh" class="btn__spin" :size="size === 'sm' ? 14 : 16" />
    <AppIcon v-else-if="icon" :name="icon" :size="size === 'sm' ? 14 : 16" />
    <span v-if="!isIconOnly" class="btn__label"><slot /></span>
    <AppIcon v-if="iconRight" :name="iconRight" :size="size === 'sm' ? 14 : 16" />
  </button>
</template>

<style scoped>
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: var(--space-2);
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  font-weight: 580;
  cursor: pointer;
  white-space: nowrap;
  transition:
    background var(--transition-fast),
    border-color var(--transition-fast),
    color var(--transition-fast),
    box-shadow var(--transition-fast),
    transform var(--transition-fast);
  user-select: none;
}

.btn:active:not(:disabled) {
  transform: translateY(0.5px);
}

.btn:disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.btn--sm {
  height: 28px;
  padding: 0 var(--space-3);
  font-size: var(--text-xs);
}

.btn--md {
  height: 34px;
  padding: 0 var(--space-4);
  font-size: var(--text-sm);
}

.btn--lg {
  height: 42px;
  padding: 0 var(--space-5);
  font-size: var(--text-md);
}

.btn--icon {
  padding: 0;
  width: 34px;
}

.btn--icon.btn--sm {
  width: 28px;
}

.btn--icon.btn--lg {
  width: 42px;
}

.btn--block {
  width: 100%;
}

.btn--primary {
  background: var(--accent);
  color: var(--accent-contrast);
  box-shadow: var(--shadow-xs);
}

.btn--primary:hover:not(:disabled) {
  background: var(--accent-hover);
}

.btn--default {
  background: var(--surface);
  border-color: var(--border-strong);
  color: var(--text);
}

.btn--default:hover:not(:disabled) {
  background: var(--surface-hover);
  border-color: var(--text-faint);
}

.btn--soft {
  background: var(--accent-soft);
  color: var(--accent-text);
}

.btn--soft:hover:not(:disabled) {
  background: var(--accent-soft-hover);
}

.btn--ghost {
  background: transparent;
  color: var(--text-muted);
}

.btn--ghost:hover:not(:disabled) {
  background: var(--surface-hover);
  color: var(--text);
}

.btn--danger {
  background: var(--danger-solid);
  color: var(--accent-contrast);
}

.btn--danger:hover:not(:disabled) {
  background: var(--danger-solid-hover);
}

.btn.is-active {
  background: var(--accent-soft);
  border-color: transparent;
  color: var(--accent-text);
}

.btn__label {
  overflow: hidden;
  text-overflow: ellipsis;
}

.btn__spin {
  animation: spin 900ms linear infinite;
}
</style>
