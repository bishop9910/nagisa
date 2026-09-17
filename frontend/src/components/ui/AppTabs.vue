<script setup lang="ts">
/** 标签页：账号中心、节点详情、管理后台内部的二级导航。 */
import type { TabItem } from './types'

withDefaults(
  defineProps<{
    modelValue: string
    tabs?: TabItem[]
  }>(),
  { tabs: () => [] },
)

const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()
</script>

<template>
  <div class="tabs" role="tablist">
    <button
      v-for="tab in tabs"
      :key="tab.key"
      type="button"
      class="tabs__item"
      :class="{ 'is-active': tab.key === modelValue }"
      role="tab"
      :aria-selected="tab.key === modelValue ? 'true' : 'false'"
      :disabled="tab.disabled"
      @click="emit('update:modelValue', tab.key)"
    >
      <slot name="tab" :tab="tab">
        <span>{{ tab.label }}</span>
        <span v-if="tab.badge !== undefined && tab.badge !== ''" class="tabs__badge">{{ tab.badge }}</span>
      </slot>
    </button>
  </div>
</template>

<style scoped>
.tabs {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  border-bottom: 1px solid var(--border-subtle);
  /* 只留横向滚动：overflow-x: auto 会把纵向也算成 auto，选中下划线只要溢出 1px
     就会在标签栏里长出一条竖向滚动条，所以纵向彻底关掉。滚动条本身也藏起来，
     横向靠滚轮 / 触控板 / 键盘焦点滚动，标签栏里不再出现一根灰条。 */
  overflow-x: auto;
  overflow-y: hidden;
  scrollbar-width: none;
}

.tabs::-webkit-scrollbar {
  width: 0;
  height: 0;
}

.tabs__item {
  position: relative;
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  border: 0;
  background: transparent;
  color: var(--text-muted);
  font-size: var(--text-sm);
  font-weight: 560;
  padding: var(--space-3) var(--space-3);
  cursor: pointer;
  white-space: nowrap;
  transition: color var(--transition-fast);
}

.tabs__item:hover:not(:disabled) {
  color: var(--text);
}

.tabs__item.is-active {
  color: var(--accent-text);
}

.tabs__item.is-active::after {
  content: '';
  position: absolute;
  left: var(--space-2);
  right: var(--space-2);
  /* 贴住底边而不是压出去：压到滚动容器外面就会被算成溢出。 */
  bottom: 0;
  height: 2px;
  border-radius: var(--radius-pill);
  background: var(--accent);
}

.tabs__item:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.tabs__badge {
  min-width: 18px;
  height: 18px;
  padding: 0 5px;
  border-radius: var(--radius-pill);
  background: var(--surface-3);
  color: var(--text-muted);
  font-size: var(--text-2xs);
  display: inline-flex;
  align-items: center;
  justify-content: center;
}
</style>
