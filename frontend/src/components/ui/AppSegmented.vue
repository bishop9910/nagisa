<script setup lang="ts">
/** 分段控件：视图切换、范围筛选这类少量互斥选项。 */
import AppIcon from './AppIcon.vue'
import type { SegmentOption } from './types'

withDefaults(
  defineProps<{
    modelValue?: string
    options?: SegmentOption[]
    size?: 'sm' | 'md'
  }>(),
  { size: 'md', options: () => [] },
)

const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()
</script>

<template>
  <div class="segmented" :class="`segmented--${size}`" role="tablist">
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      class="segmented__item"
      :class="{ 'is-active': option.value === modelValue }"
      role="tab"
      :aria-selected="option.value === modelValue ? 'true' : 'false'"
      :title="option.title ?? option.label"
      @click="emit('update:modelValue', option.value)"
    >
      <AppIcon v-if="option.icon" :name="option.icon" :size="size === 'sm' ? 14 : 16" />
      <span v-if="option.label">{{ option.label }}</span>
    </button>
  </div>
</template>

<style scoped>
.segmented {
  display: inline-flex;
  align-items: center;
  gap: 2px;
  padding: 2px;
  border-radius: var(--radius-md);
  background: var(--surface-3);
  border: 1px solid var(--border-subtle);
}

.segmented__item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 0;
  background: transparent;
  color: var(--text-muted);
  border-radius: calc(var(--radius-md) - 2px);
  cursor: pointer;
  font-size: var(--text-sm);
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
}

.segmented--sm .segmented__item {
  height: 24px;
  padding: 0 var(--space-2);
  font-size: var(--text-xs);
}

.segmented--md .segmented__item {
  height: 28px;
  padding: 0 var(--space-3);
}

.segmented__item:hover {
  color: var(--text);
}

.segmented__item.is-active {
  background: var(--surface);
  color: var(--text-strong);
  box-shadow: var(--shadow-xs);
  font-weight: 600;
}
</style>
