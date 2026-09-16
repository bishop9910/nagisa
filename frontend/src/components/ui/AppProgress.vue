<script setup lang="ts">
/** 进度条：上传、配额使用率。 */
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    value?: number
    max?: number
    tone?: 'brand' | 'success' | 'warning' | 'danger'
    height?: number
    showLabel?: boolean
  }>(),
  { value: 0, max: 100, tone: 'brand', height: 6, showLabel: false },
)

const ratio = computed(() => {
  if (props.max <= 0) return 0
  return Math.max(0, Math.min(1, props.value / props.max))
})

const percent = computed(() => Math.round(ratio.value * 100))
</script>

<template>
  <div class="progress-wrap">
    <div class="progress" :style="{ height: `${height}px` }" role="progressbar" :aria-valuenow="percent" aria-valuemin="0" aria-valuemax="100">
      <div class="progress__bar" :class="`progress__bar--${tone}`" :style="{ width: `${percent}%` }" />
    </div>
    <span v-if="showLabel" class="progress-label">{{ percent }}%</span>
  </div>
</template>

<style scoped>
.progress-wrap {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
}

.progress__bar--brand {
  background: var(--accent);
}

.progress__bar--success {
  background: var(--success-600);
}

.progress__bar--warning {
  background: var(--warning-600);
}

.progress__bar--danger {
  background: var(--danger-600);
}

.progress-label {
  font-size: var(--text-2xs);
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
  min-width: 2.5rem;
  text-align: right;
}
</style>
