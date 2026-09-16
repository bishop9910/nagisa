<script setup lang="ts">
/** 文件列表（表格式）：支持多选、排序、双击进入、右键/菜单操作。 */
import { computed } from 'vue'

import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import FileIcon from './FileIcon.vue'
import NodeActions from './NodeActions.vue'
import type { Node } from '@/api/types'
import type { SortState } from '@/stores/files'
import { formatBytes, formatRelative, toInt } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    nodes: Node[]
    selection?: string[]
    sort?: SortState
    loading?: boolean
    /** files：我的文件；trash：回收站；share：匿名分享页。 */
    context?: 'files' | 'trash' | 'share'
    canEdit?: boolean
    canDelete?: boolean
    canShare?: boolean
    canDownload?: boolean
    canRestore?: boolean
    canPurge?: boolean
    canManageShare?: boolean
    emptyText?: string
  }>(),
  { selection: () => [], context: 'files' },
)

const emit = defineEmits<{
  (e: 'open', node: Node): void
  (e: 'action', payload: { key: string; node: Node }): void
  (e: 'toggle-select', id: string): void
  (e: 'toggle-all'): void
  (e: 'sort', field: SortState['field']): void
}>()

const allSelected = computed(
  () => props.nodes.length > 0 && props.nodes.every((node) => props.selection.includes(node.id ?? '')),
)

const someSelected = computed(
  () => props.nodes.some((node) => props.selection.includes(node.id ?? '')) && !allSelected.value,
)

interface Column {
  key: string
  label: string
  field?: SortState['field']
  width?: string
  numeric?: boolean
  hideSmall?: boolean
}

const columns = computed<Column[]>(() => {
  if (props.context === 'trash') {
    return [
      { key: 'name', label: '名称', field: 'name' },
      { key: 'owner', label: '所有者', width: '140px', hideSmall: true },
      { key: 'size', label: '大小', field: 'size', width: '110px', numeric: true },
      { key: 'trashedAt', label: '删除时间', width: '150px', hideSmall: true },
    ]
  }
  return [
    { key: 'name', label: '名称', field: 'name' },
    { key: 'owner', label: '所有者', width: '140px', hideSmall: true },
    { key: 'size', label: '大小', field: 'size', width: '110px', numeric: true },
    { key: 'updatedAt', label: '修改时间', field: 'updated_at', width: '150px', hideSmall: true },
  ]
})

function sortIcon(field?: SortState['field']): string {
  if (!field || props.sort?.field !== field) return 'sort'
  return props.sort.desc ? 'arrow-down' : 'arrow-up'
}

function sizeText(node: Node): string {
  if (node.kind === 'NODE_KIND_FOLDER') {
    const count = toInt(node.childCount)
    return count > 0 ? `${count} 项` : '—'
  }
  return formatBytes(node.size)
}
</script>

<template>
  <div class="table-wrap">
    <table class="table file-table">
      <thead>
        <tr>
          <th class="file-table__check">
            <AppCheckbox
              :model-value="allSelected"
              :indeterminate="someSelected"
              aria-label="全选"
              @update:model-value="emit('toggle-all')"
            />
          </th>
          <th
            v-for="column in columns"
            :key="column.key"
            :style="column.width ? { width: column.width } : undefined"
            :class="{ 'is-num': column.numeric, hideSm: column.hideSmall }"
          >
            <button v-if="column.field" type="button" class="table-sort" @click="emit('sort', column.field)">
              {{ column.label }}
              <AppIcon :name="sortIcon(column.field)" :size="13" />
            </button>
            <template v-else>{{ column.label }}</template>
          </th>
          <th class="file-table__actions" />
        </tr>
      </thead>
      <tbody>
        <tr v-if="loading">
          <td :colspan="columns.length + 2" class="file-table__state">
            <span class="skeleton" style="display: block; height: 14px; margin: 10px 0" />
            <span class="skeleton" style="display: block; height: 14px; margin: 10px 0; width: 80%" />
            <span class="skeleton" style="display: block; height: 14px; margin: 10px 0; width: 62%" />
          </td>
        </tr>
        <tr v-else-if="nodes.length === 0">
          <td :colspan="columns.length + 2" class="file-table__state">
            <div class="file-table__empty">
              <p class="strong">{{ emptyText || '这里还没有内容' }}</p>
              <p class="muted text-xs">拖拽文件到此处，或使用上方的上传按钮</p>
            </div>
          </td>
        </tr>
        <template v-else>
          <tr
            v-for="node in nodes"
            :key="node.id"
            class="is-hoverable"
            :class="{ 'is-selected': selection.includes(node.id ?? '') }"
            @dblclick="emit('open', node)"
          >
            <td class="file-table__check">
              <AppCheckbox
                :model-value="selection.includes(node.id ?? '')"
                :aria-label="`选择 ${node.name}`"
                @update:model-value="emit('toggle-select', node.id ?? '')"
              />
            </td>
            <td>
              <button type="button" class="file-table__name" @click="emit('open', node)">
                <FileIcon :node="node" :size="18" />
                <span class="truncate" :title="node.name">{{ node.name }}</span>
                <AppIcon v-if="node.shared" name="link" :size="13" class="file-table__flag" />
              </button>
            </td>
            <td class="hideSm">
              <span class="muted text-xs">{{ node.ownerName || '—' }}</span>
            </td>
            <td class="is-num">
              <span class="text-xs muted">{{ sizeText(node) }}</span>
            </td>
            <td class="hideSm">
              <span class="text-xs muted">
                {{ formatRelative(context === 'trash' ? node.trashedAt : node.updatedAt) }}
              </span>
            </td>
            <td class="is-actions">
              <NodeActions
                :node="node"
                :context="context"
                :can-edit="canEdit"
                :can-delete="canDelete"
                :can-share="canShare"
                :can-download="canDownload"
                :can-restore="canRestore"
                :can-purge="canPurge"
                :can-manage-share="canManageShare"
                @action="(key) => emit('action', { key, node })"
              />
            </td>
          </tr>
        </template>
      </tbody>
    </table>
  </div>
</template>

<style scoped>
.file-table__check {
  width: 40px;
  padding-right: 0 !important;
}

.file-table__actions {
  width: 52px;
}

.file-table__name {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  border: 0;
  background: transparent;
  padding: 0;
  cursor: pointer;
  color: var(--text);
  font-size: var(--text-sm);
  font-weight: 560;
  max-width: 100%;
  text-align: left;
}

.file-table__name:hover span {
  color: var(--accent-text);
}

.file-table__flag {
  color: var(--text-faint);
}

.file-table__state {
  padding: var(--space-6) var(--space-4) !important;
}

.file-table__empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-8) 0;
  text-align: center;
}

@media (max-width: 900px) {
  .hideSm {
    display: none;
  }
}
</style>
