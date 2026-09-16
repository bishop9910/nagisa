<script setup lang="ts">
/** 文件网格：卡片式浏览，图片与视频优先展示缩略图。 */
import { onBeforeUnmount, ref, watch } from 'vue'

import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import FileIcon from './FileIcon.vue'
import NodeActions from './NodeActions.vue'
import { filesApi } from '@/api'
import type { Node } from '@/api/types'
import { formatBytes, formatRelative } from '@/utils/format'
import { supportsThumbnail } from '@/utils/media'

const props = withDefaults(
  defineProps<{
    nodes: Node[]
    selection?: string[]
    loading?: boolean
    context?: 'files' | 'trash' | 'share'
    canEdit?: boolean
    canDelete?: boolean
    canShare?: boolean
    canDownload?: boolean
    canRestore?: boolean
    canPurge?: boolean
    canManageShare?: boolean
  }>(),
  { selection: () => [], context: 'files' },
)

const emit = defineEmits<{
  (e: 'open', node: Node): void
  (e: 'action', payload: { key: string; node: Node }): void
  (e: 'toggle-select', id: string): void
}>()

/** 缩略图按需签发，避免为整页文件一次性请求预览地址。 */
const thumbs = ref<Record<string, string>>({})
const inflight = ref<Set<string>>(new Set())
let disposed = false

async function ensureThumb(node: Node): Promise<void> {
  const id = node.id ?? ''
  if (!id || thumbs.value[id] || inflight.value.has(id)) return
  if (props.context !== 'files' || !supportsThumbnail(node)) return
  inflight.value.add(id)
  try {
    const signed = await filesApi.getPreviewUrl(id, { thumbnail: true })
    if (!disposed && signed.url) {
      thumbs.value = { ...thumbs.value, [id]: signed.url }
    }
  } catch {
    /* 预览地址拿不到就继续用图标。 */
  } finally {
    inflight.value.delete(id)
  }
}

watch(
  () => props.nodes.map((node) => node.id).join(','),
  () => {
    // 列表变化后清理已经离开的缩略图缓存。
    const alive = new Set(props.nodes.map((node) => node.id ?? ''))
    const next: Record<string, string> = {}
    for (const [key, value] of Object.entries(thumbs.value)) {
      if (alive.has(key)) next[key] = value
    }
    thumbs.value = next
  },
)

onBeforeUnmount(() => {
  disposed = true
})
</script>

<template>
  <div class="grid">
    <template v-if="loading">
      <div v-for="index in 6" :key="index" class="grid__card grid__card--skeleton">
        <span class="skeleton" style="height: 96px; display: block" />
        <span class="skeleton" style="height: 12px; width: 70%; display: block; margin-top: 10px" />
      </div>
    </template>
    <template v-else>
      <article
        v-for="node in nodes"
        :key="node.id"
        class="grid__card"
        :class="{ 'is-selected': selection.includes(node.id ?? '') }"
        tabindex="0"
        @dblclick="emit('open', node)"
        @keydown.enter="emit('open', node)"
        @mouseenter="ensureThumb(node)"
        @focus="ensureThumb(node)"
      >
      <div class="grid__preview">
        <img
          v-if="thumbs[node.id ?? '']"
          :src="thumbs[node.id ?? '']"
          :alt="node.name"
          class="grid__thumb"
          loading="lazy"
        />
        <FileIcon v-else :node="node" :size="26" boxed />
        <div class="grid__check" @click.stop>
          <AppCheckbox
            :model-value="selection.includes(node.id ?? '')"
            :aria-label="`选择 ${node.name}`"
            @update:model-value="emit('toggle-select', node.id ?? '')"
          />
        </div>
        <div class="grid__menu" @click.stop>
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
        </div>
        <span v-if="node.shared" class="grid__flag" title="已分享">
          <AppIcon name="link" :size="12" />
        </span>
      </div>
      <div class="grid__meta">
        <button type="button" class="grid__name truncate" :title="node.name" @click="emit('open', node)">
          {{ node.name }}
        </button>
        <p class="grid__sub">
          <span>{{ node.kind === 'NODE_KIND_FOLDER' ? '文件夹' : formatBytes(node.size) }}</span>
          <span>·</span>
          <span>{{ formatRelative(context === 'trash' ? node.trashedAt : node.updatedAt) }}</span>
        </p>
      </div>
      </article>
    </template>
  </div>
</template>

<style scoped>
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(168px, 1fr));
  gap: var(--space-3);
}

.grid__card {
  position: relative;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  cursor: pointer;
  transition:
    border-color var(--transition-fast),
    box-shadow var(--transition-fast),
    transform var(--transition-fast);
}

.grid__card:hover {
  border-color: var(--border-strong);
  box-shadow: var(--shadow-md);
  transform: translateY(-1px);
}

.grid__card.is-selected {
  border-color: var(--accent);
  background: var(--surface-active);
}

.grid__card--skeleton {
  cursor: default;
  pointer-events: none;
}

.grid__preview {
  position: relative;
  height: 96px;
  display: flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-md);
  background: var(--surface-2);
  overflow: hidden;
}

.grid__thumb {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.grid__check {
  position: absolute;
  top: var(--space-2);
  left: var(--space-2);
  opacity: 0;
  transition: opacity var(--transition-fast);
}

.grid__menu {
  position: absolute;
  top: var(--space-1);
  right: var(--space-1);
  opacity: 0;
  transition: opacity var(--transition-fast);
}

.grid__card:hover .grid__check,
.grid__card:hover .grid__menu,
.grid__card:focus-within .grid__check,
.grid__card:focus-within .grid__menu,
.grid__card.is-selected .grid__check {
  opacity: 1;
}

.grid__flag {
  position: absolute;
  bottom: var(--space-2);
  right: var(--space-2);
  background: var(--surface);
  color: var(--accent-text);
  border-radius: var(--radius-pill);
  padding: 3px;
  box-shadow: var(--shadow-xs);
}

.grid__meta {
  min-width: 0;
}

.grid__name {
  border: 0;
  background: transparent;
  padding: 0;
  font-size: var(--text-sm);
  font-weight: 560;
  color: var(--text);
  cursor: pointer;
  max-width: 100%;
  text-align: left;
}

.grid__name:hover {
  color: var(--accent-text);
}

.grid__sub {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: var(--text-2xs);
  color: var(--text-muted);
  margin-top: 2px;
}
</style>
