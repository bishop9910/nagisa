<script setup lang="ts">
/** 加载指示器。 */
import { computed } from 'vue'

const props = withDefaults(defineProps<{ size?: number; label?: string }>(), { size: 18 })

const dimension = computed(() => `${props.size}px`)
</script>

<template>
  <span class="spinner-wrap">
    <span class="spinner" :style="{ width: dimension, height: dimension }" role="status" :aria-label="label ?? '加载中'" />
    <span v-if="label" class="spinner__label">{{ label }}</span>
  </span>
</template>

<style scoped>
.spinner-wrap {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}

.spinner {
  display: inline-block;
  border: 2px solid var(--border-strong);
  border-top-color: var(--accent);
  border-radius: var(--radius-pill);
  animation: spin 700ms linear infinite;
}

.spinner__label {
  font-size: var(--text-xs);
  color: var(--text-muted);
}
</style>
