<script setup lang="ts">
/** 轻量提示：鼠标悬停与键盘聚焦都会显示，移动端退化为 title。 */
import { ref } from 'vue'

withDefaults(
  defineProps<{
    content?: string
    placement?: 'top' | 'bottom'
  }>(),
  { placement: 'top' },
)

const visible = ref(false)
</script>

<template>
  <span
    class="tooltip"
    @mouseenter="visible = true"
    @mouseleave="visible = false"
    @focusin="visible = true"
    @focusout="visible = false"
  >
    <slot />
    <Transition name="fade">
      <span v-if="visible && content" class="tooltip__bubble" :class="`tooltip__bubble--${placement}`" role="tooltip">
        {{ content }}
      </span>
    </Transition>
  </span>
</template>

<style scoped>
.tooltip {
  position: relative;
  display: inline-flex;
}

.tooltip__bubble {
  position: absolute;
  left: 50%;
  transform: translateX(-50%);
  /* 反色浮层：浅色主题深底白字，深色主题浅底深字，两套主题下都和背景拉开。 */
  background: var(--inverse-surface);
  color: var(--inverse-text);
  border: 1px solid color-mix(in srgb, var(--inverse-text) 16%, transparent);
  font-size: var(--text-2xs);
  font-weight: 560;
  padding: 5px var(--space-2);
  border-radius: var(--radius-sm);
  white-space: nowrap;
  z-index: var(--z-toast);
  pointer-events: none;
  box-shadow: var(--shadow-md);
}

.tooltip__bubble--top {
  bottom: calc(100% + 6px);
}

.tooltip__bubble--bottom {
  top: calc(100% + 6px);
}
</style>
