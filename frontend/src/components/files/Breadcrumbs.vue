<script setup lang="ts">
/** 面包屑：根目录 → 当前目录，可下拉选择兄弟目录。 */
import { computed } from 'vue'

import AppIcon from '@/components/ui/AppIcon.vue'
import { ROOT_LABEL } from '@/stores/files'

/** 面包屑只用到 id 与 name，因此不要求完整的 Node，回收站这类导航也能复用。 */
interface CrumbNode {
  id?: string
  name?: string
}

const props = withDefaults(
  defineProps<{
    /** 当前目录 id（空串表示根）。 */
    folderId: string
    /** 从根到当前目录的祖先链。 */
    ancestors?: CrumbNode[]
    /** 当前目录节点（根目录为 null）。 */
    folder?: CrumbNode | null
    /** 根那一层的显示名，回收站里是「回收站」。 */
    rootLabel?: string
  }>(),
  { ancestors: () => [], folder: null, rootLabel: ROOT_LABEL },
)

const emit = defineEmits<{ (e: 'navigate', id: string): void }>()

interface Crumb {
  id: string
  name: string
}

const crumbs = computed<Crumb[]>(() => {
  const list: Crumb[] = (props.ancestors ?? []).map((node) => ({ id: node.id ?? '', name: node.name || '未命名' }))
  list.push({ id: props.folderId, name: props.folder?.name || props.rootLabel })
  return list
})

/** 折叠中间层级，只保留最后三段，避免深路径撑破布局。 */
const visibleCrumbs = computed(() => {
  const list = crumbs.value
  if (list.length <= 3) return list.map((crumb, index) => ({ crumb, index, ellipsis: false }))
  const head = list[0]
  const tail = list.slice(-2)
  if (!head) return []
  return [
    { crumb: head, index: 0, ellipsis: false },
    { crumb: { id: '', name: '…' }, index: -1, ellipsis: true },
    ...tail.map((crumb, offset) => ({ crumb, index: list.length - 2 + offset, ellipsis: false })),
  ]
})

function navigateTo(id: string): void {
  emit('navigate', id)
}
</script>

<template>
  <nav class="crumbs" aria-label="路径">
    <template v-for="(item, position) in visibleCrumbs" :key="`${item.crumb.id}-${position}`">
      <AppIcon v-if="position > 0" name="chevron-right" :size="14" class="crumbs__sep" />
      <span v-if="item.ellipsis" class="crumbs__ellipsis">…</span>
      <button
        v-else
        type="button"
        class="crumbs__item"
        :class="{ 'is-current': item.index === crumbs.length - 1 }"
        @click="navigateTo(item.crumb.id)"
      >
        <AppIcon v-if="item.index === 0" name="home" :size="14" />
        <span class="truncate">{{ item.crumb.name }}</span>
      </button>
    </template>
  </nav>
</template>

<style scoped>
.crumbs {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  min-width: 0;
  flex-wrap: wrap;
}

.crumbs__sep {
  color: var(--text-faint);
  flex: none;
}

.crumbs__ellipsis {
  color: var(--text-faint);
  font-size: var(--text-sm);
}

.crumbs__item {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  border: 0;
  background: transparent;
  color: var(--text-muted);
  font-size: var(--text-sm);
  padding: 3px var(--space-2);
  border-radius: var(--radius-sm);
  cursor: pointer;
  max-width: 220px;
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
}

.crumbs__item:hover {
  background: var(--surface-hover);
  color: var(--text);
}

.crumbs__item.is-current {
  color: var(--text-strong);
  font-weight: 620;
}
</style>
