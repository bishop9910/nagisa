<script setup lang="ts">
/** 节点操作菜单：列表、网格、回收站共用同一套动作。 */
import { computed } from 'vue'

import AppDropdown from '@/components/ui/AppDropdown.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import type { Node } from '@/api/types'
import { isPreviewable } from '@/utils/media'
import { toInt } from '@/utils/format'

const props = withDefaults(
  defineProps<{
    node: Node
    /** files：正常目录；trash：回收站；share：匿名分享页。 */
    context?: 'files' | 'trash' | 'share'
    canEdit?: boolean
    canDelete?: boolean
    canShare?: boolean
    canDownload?: boolean
    canRestore?: boolean
    canPurge?: boolean
    canManageShare?: boolean
  }>(),
  { context: 'files' },
)

const emit = defineEmits<{ (e: 'action', key: string): void }>()

const isFolder = computed(() => props.node.kind === 'NODE_KIND_FOLDER')
const mask = computed(() => toInt(props.node.effectivePermissionsMask))
const owned = computed(() => props.node.owned !== false)

function has(bit: number, fallback: boolean): boolean {
  if (props.context === 'share') return (mask.value & bit) !== 0
  return fallback
}

const items = computed(() => {
  const node = props.node
  if (props.context === 'trash') {
    return [
      // 被删的文件夹保留了层级，所以先给一个入口走进去看里面的条目。
      ...(isFolder.value ? [{ key: 'open', label: '打开', icon: 'folder-open' }] : []),
      { key: 'restore', label: '还原', icon: 'restore', disabled: props.canRestore === false },
      { key: 'purge', label: '彻底删除', icon: 'trash', danger: true, disabled: props.canPurge === false },
    ]
  }
  if (props.context === 'share') {
    return [
      ...(!isFolder.value && isPreviewable(node) ? [{ key: 'preview', label: '预览', icon: 'eye' }] : []),
      ...(!isFolder.value && has(2, false) ? [{ key: 'download', label: '下载', icon: 'download' }] : []),
      ...(isFolder.value ? [{ key: 'open', label: '打开', icon: 'folder-open' }] : []),
      { key: 'details', label: '详细信息', icon: 'info' },
    ]
  }
  const list = []
  if (isFolder.value) {
    list.push({ key: 'open', label: '打开', icon: 'folder-open' })
  } else if (isPreviewable(node)) {
    list.push({ key: 'preview', label: '预览', icon: 'eye' })
  }
  list.push({
    key: 'download',
    label: isFolder.value ? '下载为 ZIP' : '下载',
    icon: 'download',
    disabled: has(2, props.canDownload !== false) === false,
  })
  if (!isFolder.value) {
    list.push({ key: 'versions', label: '历史版本', icon: 'history', hint: node.versionCount ? String(node.versionCount) : '' })
  }
  list.push({ key: 'divider-1', label: '', divider: true })
  list.push({ key: 'rename', label: '重命名', icon: 'edit', disabled: has(8, props.canEdit !== false) === false })
  list.push({ key: 'move', label: '移动到…', icon: 'move', disabled: has(8, props.canEdit !== false) === false })
  list.push({ key: 'copy', label: '复制到…', icon: 'copy', disabled: has(4, props.canEdit !== false) === false })
  list.push({
    key: 'share',
    label: '分享',
    icon: 'share',
    disabled: has(64, props.canShare !== false) === false || !owned.value,
  })
  if (node.shared) {
    list.push({ key: 'shares', label: '查看分享链接', icon: 'link', disabled: props.canManageShare === false })
  }
  list.push({ key: 'divider-2', label: '', divider: true })
  list.push({ key: 'details', label: '详细信息', icon: 'info' })
  list.push({ key: 'delete', label: '删除', icon: 'trash', danger: true, disabled: has(16, props.canDelete !== false) === false })
  return list
})

function onSelect(key: string): void {
  emit('action', key)
}
</script>

<template>
  <AppDropdown :items="items" :width="196" @select="onSelect">
    <template #trigger>
      <button type="button" class="node-actions" aria-label="更多操作">
        <AppIcon name="more-vertical" :size="16" />
      </button>
    </template>
  </AppDropdown>
</template>

<style scoped>
.node-actions {
  width: 28px;
  height: 28px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
}

.node-actions:hover {
  background: var(--surface-3);
  color: var(--text);
}
</style>
