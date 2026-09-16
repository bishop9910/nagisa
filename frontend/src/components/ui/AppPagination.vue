<script setup lang="ts">
/** 分页器：AIP 的 page_token 只有「上一页/下一页」，这里额外展示当前页与总数。 */
import { computed } from 'vue'
import AppButton from './AppButton.vue'
import AppSelect from './AppSelect.vue'

const props = withDefaults(
  defineProps<{
    page: number
    pageSize: number
    total?: number
    hasPrev: boolean
    hasNext: boolean
    pageSizes?: number[]
    disabled?: boolean
  }>(),
  { total: 0, pageSizes: () => [20, 50, 100] },
)

const emit = defineEmits<{
  (e: 'prev'): void
  (e: 'next'): void
  (e: 'update:pageSize', value: number): void
}>()

const pageCount = computed(() => {
  if (!props.total || !props.pageSize) return 0
  return Math.max(1, Math.ceil(props.total / props.pageSize))
})

const rangeText = computed(() => {
  if (!props.total) return `第 ${props.page} 页`
  const from = (props.page - 1) * props.pageSize + 1
  const to = Math.min(props.total, props.page * props.pageSize)
  return `${from}-${to} / 共 ${props.total} 条`
})

const sizeOptions = computed(() => props.pageSizes.map((size) => ({ value: size, label: `${size} 条/页` })))
</script>

<template>
  <div class="pagination">
    <span class="pagination__range">{{ rangeText }}</span>
    <div class="pagination__spacer" />
    <AppSelect
      v-if="pageSizes.length > 1"
      size="sm"
      :model-value="pageSize"
      :options="sizeOptions"
      @update:model-value="emit('update:pageSize', Number($event))"
    />
    <AppButton size="sm" icon="chevron-left" label="上一页" :disabled="!hasPrev || disabled" @click="emit('prev')" />
    <span class="pagination__page">{{ page }}<template v-if="pageCount"> / {{ pageCount }}</template></span>
    <AppButton size="sm" icon="chevron-right" label="下一页" :disabled="!hasNext || disabled" @click="emit('next')" />
  </div>
</template>

<style scoped>
.pagination {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border-top: 1px solid var(--border-subtle);
  flex-wrap: wrap;
}

.pagination__range {
  font-size: var(--text-xs);
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}

.pagination__spacer {
  flex: 1 1 auto;
}

.pagination__page {
  font-size: var(--text-xs);
  color: var(--text-muted);
  min-width: 3.5rem;
  text-align: center;
  font-variant-numeric: tabular-nums;
}
</style>
