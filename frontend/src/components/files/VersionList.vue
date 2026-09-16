<script setup lang="ts">
/** 文件历史版本列表：还原、删除与「当前版本」标记。 */
import { ref, watch } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import { errorText, nodesApi } from '@/api'
import type { NodeVersion } from '@/api/types'
import { useUiStore } from '@/stores/ui'
import { formatBytes, formatDateTime, toInt } from '@/utils/format'

const props = defineProps<{
  nodeId: string
  canEdit?: boolean
  canDelete?: boolean
}>()

const emit = defineEmits<{ (e: 'changed'): void }>()

const ui = useUiStore()
const versions = ref<NodeVersion[]>([])
const loading = ref(false)
const busy = ref('')
const error = ref('')

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const page = await nodesApi.listNodeVersions(props.nodeId, { pageSize: 100 })
    versions.value = page.versions ?? []
  } catch (err) {
    error.value = errorText(err)
    versions.value = []
  } finally {
    loading.value = false
  }
}

async function restore(version: NodeVersion): Promise<void> {
  const ok = await ui.confirm({
    title: '还原到该版本',
    message: `会把当前内容先保存为一个新版本，再把节点内容替换为 v${version.version}。`,
    confirmText: '还原',
  })
  if (!ok) return
  busy.value = version.id ?? ''
  try {
    await nodesApi.restoreNodeVersion(props.nodeId, version.id ?? '')
    ui.toast.success('已还原', `当前内容为 v${version.version}`)
    await load()
    emit('changed')
  } catch (err) {
    ui.toast.error('还原失败', errorText(err))
  } finally {
    busy.value = ''
  }
}

async function remove(version: NodeVersion): Promise<void> {
  const ok = await ui.confirm({
    title: '删除该版本',
    message: `v${version.version} 的内容将被释放，且无法恢复。`,
    tone: 'danger',
    confirmText: '删除',
  })
  if (!ok) return
  busy.value = version.id ?? ''
  try {
    await nodesApi.deleteNodeVersion(props.nodeId, version.id ?? '')
    ui.toast.success('版本已删除')
    await load()
    emit('changed')
  } catch (err) {
    ui.toast.error('删除失败', errorText(err))
  } finally {
    busy.value = ''
  }
}

watch(() => props.nodeId, () => void load(), { immediate: true })
</script>

<template>
  <div class="versions">
    <div class="versions__head">
      <p class="text-xs muted">覆盖上传会保留历史版本（受服务端 upload.keep_versions 与 max_versions 控制）。</p>
      <AppButton size="sm" variant="ghost" icon="refresh" label="刷新" @click="load" />
    </div>

    <p v-if="loading" class="muted text-xs">加载中…</p>
    <AppEmpty
      v-else-if="versions.length === 0"
      size="sm"
      icon="history"
      title="暂无历史版本"
      description="首次上传不会产生版本记录"
    />
    <ul v-else class="versions__list">
      <li v-for="version in versions" :key="version.id" class="versions__item">
        <div class="versions__main">
          <p class="versions__title">
            v{{ version.version }}
            <span v-if="version.current" class="badge badge--brand">当前</span>
          </p>
          <p class="versions__meta">
            <span>{{ formatBytes(version.size) }}</span>
            <span>·</span>
            <span>{{ formatDateTime(version.createdAt) }}</span>
            <template v-if="version.createdBy">
              <span>·</span>
              <span>{{ version.createdBy }}</span>
            </template>
            <template v-if="version.comment">
              <span>·</span>
              <span>{{ version.comment }}</span>
            </template>
          </p>
          <p v-if="version.etag" class="versions__etag mono truncate" :title="version.etag">
            <AppIcon name="hash" :size="12" />
            {{ version.etag }}
          </p>
        </div>
        <div class="versions__actions">
          <AppButton
            v-if="!version.current"
            size="sm"
            :disabled="canEdit === false"
            :loading="busy === version.id"
            icon="restore"
            @click="restore(version)"
          >
            还原
          </AppButton>
          <AppButton
            v-if="!version.current"
            size="sm"
            variant="ghost"
            icon="trash"
            label="删除版本"
            :disabled="canDelete === false"
            @click="remove(version)"
          />
        </div>
      </li>
    </ul>
    <p v-if="error" class="field__error">{{ error }}</p>
    <p v-if="versions.length > 0" class="text-xs faint mt-2">
      共 {{ versions.length }} 个版本，最新版本 v{{ toInt(versions[0]?.version) }}
    </p>
  </div>
</template>

<style scoped>
.versions {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.versions__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.versions__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.versions__item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: var(--surface-2);
}

.versions__main {
  flex: 1 1 auto;
  min-width: 0;
}

.versions__title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  font-weight: 620;
  color: var(--text-strong);
}

.versions__meta {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-wrap: wrap;
  font-size: var(--text-2xs);
  color: var(--text-muted);
  margin-top: 2px;
}

.versions__etag {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 10px;
  color: var(--text-faint);
  margin-top: 2px;
}

.versions__actions {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex: none;
}
</style>
