<script setup lang="ts">
/**
 * 传输列表：服务端侧的上传会话（可续传、可取消）与本次会话的本地队列。
 * 两者都对得上同一套接口：ListUploads / GetUpload / ListUploadParts / AbortUpload。
 */
import { computed, onMounted, ref, watch } from 'vue'

import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppPagination from '@/components/ui/AppPagination.vue'
import AppProgress from '@/components/ui/AppProgress.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import AppTabs from '@/components/ui/AppTabs.vue'
import { errorText, filesApi } from '@/api'
import type { UploadSession, UploadStatus } from '@/api/types'
import { useUiStore } from '@/stores/ui'
import { useUploadStore } from '@/stores/upload'
import { UPLOAD_MODE_LABEL, UPLOAD_STATUS_LABEL, UPLOAD_STATUS_TONE } from '@/utils/constants'
import { formatBytes, formatDateTime, formatExpiry, formatNumber, toInt } from '@/utils/format'

const ui = useUiStore()
const uploads = useUploadStore()

const tab = ref('sessions')
const sessions = ref<UploadSession[]>([])
const loading = ref(false)
const error = ref('')
const statusFilter = ref<'' | UploadStatus>('')
const page = ref(1)
const pageSize = ref(20)
const tokenStack = ref<string[]>([''])
const nextToken = ref('')
const totalSize = ref(0)

const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'UPLOAD_STATUS_PENDING', label: '待开始' },
  { value: 'UPLOAD_STATUS_IN_PROGRESS', label: '上传中' },
  { value: 'UPLOAD_STATUS_COMPLETED', label: '已完成' },
  { value: 'UPLOAD_STATUS_ABORTED', label: '已取消' },
  { value: 'UPLOAD_STATUS_EXPIRED', label: '已过期' },
]

const tabs = computed(() => [
  { key: 'sessions', label: '服务端会话', badge: toInt(totalSize) || sessions.value.length },
  { key: 'local', label: '本次浏览器队列', badge: uploads.tasks.length },
])

function ratio(session: UploadSession): number {
  const size = toInt(session.size)
  if (size <= 0) return session.status === 'UPLOAD_STATUS_COMPLETED' ? 1 : 0
  return Math.min(1, toInt(session.receivedBytes) / size)
}

async function load(reset = false): Promise<void> {
  if (reset) {
    page.value = 1
    tokenStack.value = ['']
  }
  loading.value = true
  error.value = ''
  try {
    const result = await filesApi.listUploads({
      pageSize: pageSize.value,
      pageToken: tokenStack.value[page.value - 1] || undefined,
      status: statusFilter.value || undefined,
    })
    sessions.value = result.uploads ?? []
    totalSize.value = toInt(result.totalSize, sessions.value.length)
    nextToken.value = result.nextPageToken ?? ''
  } catch (err) {
    error.value = errorText(err)
    sessions.value = []
  } finally {
    loading.value = false
  }
}

async function abort(session: UploadSession): Promise<void> {
  if (!session.id) return
  const ok = await ui.confirm({
    title: '取消上传会话',
    message: `「${session.name}」的暂存分片会被删除，已上传的部分无法恢复。`,
    tone: 'danger',
    confirmText: '取消会话',
  })
  if (!ok) return
  try {
    await filesApi.abortUpload(session.id)
    ui.toast.success('会话已取消')
    await load()
  } catch (err) {
    ui.toast.error('取消失败', errorText(err))
  }
}

function resumeInBrowser(session: UploadSession): void {
  ui.toast.info('续传提示', '请在原文件所在设备上重新选择同一个文件继续上传；服务端会复用同名会话。')
}

function nextPage(): void {
  if (!nextToken.value) return
  tokenStack.value = [...tokenStack.value.slice(0, page.value), nextToken.value]
  page.value += 1
  void load()
}

function prevPage(): void {
  if (page.value <= 1) return
  page.value -= 1
  void load()
}

function onPageSize(size: number): void {
  pageSize.value = size
  void load(true)
}

watch(statusFilter, () => void load(true))
watch(tab, (value) => {
  if (value === 'sessions') void load()
})

onMounted(() => void load())
</script>

<template>
  <div class="page">
    <header class="page__header">
      <div>
        <h1 class="page__title">传输列表</h1>
        <p class="page__desc">
          服务端保存的是分片上传会话，浏览器无法直接续传时，重新选择同名文件即可复用会话继续。
        </p>
      </div>
      <div class="page__actions">
        <AppButton icon="refresh" variant="ghost" label="刷新" @click="load()" />
        <AppButton
          v-if="uploads.finished.length > 0"
          variant="ghost"
          icon="trash"
          @click="uploads.clearFinished()"
        >
          清空本地记录
        </AppButton>
      </div>
    </header>

    <section class="card">
      <header class="card__header">
        <AppTabs v-model="tab" :tabs="tabs" />
      </header>

      <template v-if="tab === 'sessions'">
        <div class="uploads__toolbar">
          <AppSelect v-model="statusFilter" size="sm" :options="statusOptions" style="width: 160px" />
          <span class="toolbar__spacer" />
          <span class="text-xs muted">共 {{ formatNumber(totalSize) }} 个会话</span>
        </div>

        <div v-if="loading" class="uploads__state muted text-sm">加载中…</div>
        <AppEmpty
          v-else-if="sessions.length === 0"
          :icon="error ? 'alert-circle' : 'upload'"
          :title="error ? '加载失败' : '没有上传会话'"
          :description="error || '上传文件时会自动创建分片会话'"
        >
          <AppButton v-if="error" icon="refresh" @click="load()">重试</AppButton>
        </AppEmpty>
        <ul v-else class="uploads__list">
          <li v-for="session in sessions" :key="session.id" class="uploads__item">
            <div class="uploads__main">
              <p class="uploads__title">
                <span class="truncate">{{ session.name }}</span>
                <AppBadge :tone="UPLOAD_STATUS_TONE[session.status ?? 'UPLOAD_STATUS_UNSPECIFIED']" size="sm">
                  {{ UPLOAD_STATUS_LABEL[session.status ?? 'UPLOAD_STATUS_UNSPECIFIED'] }}
                </AppBadge>
                <AppBadge size="sm">{{ UPLOAD_MODE_LABEL[session.mode ?? 'UPLOAD_MODE_UNSPECIFIED'] }}</AppBadge>
              </p>
              <p class="uploads__meta">
                <span>{{ formatBytes(session.size) }}</span>
                <span>·</span>
                <span>{{ (session.uploadedParts ?? []).length }} / {{ session.totalParts ?? 0 }} 片</span>
                <span>·</span>
                <span>{{ formatBytes(session.chunkSize) }}/片</span>
                <span>·</span>
                <span>创建 {{ formatDateTime(session.createdAt) }}</span>
                <span>·</span>
                <span>{{ formatExpiry(session.expiresAt) }}</span>
              </p>
              <p v-if="session.lastError" class="uploads__error">{{ session.lastError }}</p>
              <AppProgress
                class="mt-2"
                :value="ratio(session) * 100"
                :height="4"
                :tone="session.status === 'UPLOAD_STATUS_COMPLETED' ? 'success' : 'brand'"
              />
            </div>
            <div class="uploads__actions">
              <AppButton size="sm" icon="refresh" @click="resumeInBrowser(session)">如何续传</AppButton>
              <AppButton
                size="sm"
                variant="ghost"
                icon="x"
                label="取消会话"
                :disabled="['UPLOAD_STATUS_COMPLETED', 'UPLOAD_STATUS_ABORTED'].includes(session.status ?? '')"
                @click="abort(session)"
              />
            </div>
          </li>
        </ul>

        <AppPagination
          v-if="sessions.length > 0"
          :page="page"
          :page-size="pageSize"
          :total="totalSize"
          :page-sizes="[20, 50, 100]"
          :has-prev="page > 1"
          :has-next="Boolean(nextToken)"
          :disabled="loading"
          @prev="prevPage"
          @next="nextPage"
          @update:page-size="onPageSize"
        />
      </template>

      <template v-else>
        <div v-if="uploads.tasks.length === 0" class="uploads__state">
          <AppEmpty size="sm" icon="upload" title="本次会话还没有上传记录" description="在文件页选择上传后会出现在这里" />
        </div>
        <ul v-else class="uploads__list">
          <li v-for="task in uploads.tasks" :key="task.id" class="uploads__item">
            <div class="uploads__main">
              <p class="uploads__title">
                <span class="truncate">{{ task.name }}</span>
                <AppBadge size="sm">{{ task.status }}</AppBadge>
                <AppBadge v-if="!task.inline" size="sm">{{ UPLOAD_MODE_LABEL[task.mode] }}</AppBadge>
              </p>
              <p class="uploads__meta">
                <span>{{ formatBytes(uploads.taskLoaded(task)) }} / {{ formatBytes(task.size) }}</span>
                <span>·</span>
                <span>目标：{{ task.parentLabel }}</span>
                <template v-if="!task.inline">
                  <span>·</span>
                  <span>{{ task.parts.filter((part) => part.done).length }} / {{ task.totalParts }} 片</span>
                </template>
              </p>
              <p v-if="task.error" class="uploads__error">{{ task.error }}</p>
              <AppProgress class="mt-2" :value="uploads.taskRatio(task) * 100" :height="4" />
            </div>
            <div class="uploads__actions">
              <AppButton
                v-if="task.status === 'uploading' || task.status === 'queued' || task.status === 'preparing'"
                size="sm"
                variant="ghost"
                icon="pause"
                label="暂停"
                @click="uploads.pause(task)"
              />
              <AppButton
                v-else-if="task.status === 'paused' || task.status === 'error'"
                size="sm"
                variant="ghost"
                :icon="task.status === 'error' ? 'refresh' : 'play'"
                :label="task.status === 'error' ? '重试' : '继续'"
                @click="task.status === 'error' ? uploads.retry(task) : uploads.resume(task)"
              />
              <AppButton
                size="sm"
                variant="ghost"
                :icon="['done', 'error', 'canceled'].includes(task.status) ? 'x' : 'x-circle'"
                :label="['done', 'error', 'canceled'].includes(task.status) ? '移除记录' : '取消上传'"
                @click="['done', 'error', 'canceled'].includes(task.status) ? uploads.remove(task.id) : uploads.cancel(task)"
              />
            </div>
          </li>
        </ul>
      </template>
    </section>
  </div>
</template>

<style scoped>
.uploads__toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-5);
  border-bottom: 1px solid var(--border-subtle);
}

.uploads__state {
  padding: var(--space-6);
  text-align: center;
}

.uploads__list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.uploads__item {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5);
  border-bottom: 1px solid var(--border-subtle);
}

.uploads__item:last-child {
  border-bottom: 0;
}

.uploads__main {
  flex: 1 1 auto;
  min-width: 0;
}

.uploads__title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-strong);
  min-width: 0;
}

.uploads__meta {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-wrap: wrap;
  font-size: var(--text-2xs);
  color: var(--text-muted);
  margin-top: 3px;
}

.uploads__error {
  font-size: var(--text-2xs);
  color: var(--danger-600);
  margin-top: 3px;
}

.uploads__actions {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex: none;
}

@media (max-width: 768px) {
  .uploads__item {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
