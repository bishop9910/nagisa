<script setup lang="ts">
/** 上传面板：常驻右下角，展示队列、进度与逐项操作。 */
import { computed } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppProgress from '@/components/ui/AppProgress.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import AppTooltip from '@/components/ui/AppTooltip.vue'
import { useSystemStore } from '@/stores/system'
import { useUploadStore } from '@/stores/upload'
import { UPLOAD_MODE_LABEL } from '@/utils/constants'
import { formatBytes } from '@/utils/format'

const uploads = useUploadStore()
const system = useSystemStore()

const visible = computed(() => uploads.tasks.length > 0)

const statusText: Record<string, string> = {
  queued: '排队中',
  preparing: '准备中',
  uploading: '上传中',
  finalizing: '合并中',
  done: '已完成',
  error: '失败',
  canceled: '已取消',
  paused: '已暂停',
}

const modeOptions = computed(() =>
  (system.uploadModes.length > 0 ? system.uploadModes : (['UPLOAD_MODE_PRESIGNED'] as const)).map((mode) => ({
    value: mode,
    label: UPLOAD_MODE_LABEL[mode],
  })),
)

/** 瞬时速度按已上传字节与已用时间估算，够用且无需采样。 */
function speedOf(taskId: string): string {
  const task = uploads.tasks.find((item) => item.id === taskId)
  if (!task || !task.startedAt || task.status !== 'uploading') return ''
  const elapsed = (Date.now() - task.startedAt) / 1000
  if (elapsed < 1) return ''
  const bytes = uploads.taskLoaded(task)
  if (bytes <= 0) return ''
  return `${formatBytes(bytes / elapsed)}/s`
}

function toneOf(status: string): 'brand' | 'success' | 'danger' | 'warning' {
  if (status === 'done') return 'success'
  if (status === 'error') return 'danger'
  if (status === 'paused' || status === 'canceled') return 'warning'
  return 'brand'
}
</script>

<template>
  <Transition name="pop">
    <section v-if="visible" class="upload-panel" :class="{ 'is-collapsed': uploads.collapsed }" aria-label="上传队列">
      <header class="upload-panel__head">
        <button type="button" class="upload-panel__toggle" @click="uploads.collapsed = !uploads.collapsed">
          <AppIcon :name="uploads.collapsed ? 'chevron-up' : 'chevron-down'" :size="16" />
          <span>传输列表</span>
          <span class="upload-panel__count">{{ uploads.running.length }} / {{ uploads.tasks.length }}</span>
        </button>
        <div class="upload-panel__head-actions">
          <AppTooltip content="重试失败的传输">
            <AppButton
              size="sm"
              variant="ghost"
              icon="refresh"
              label="重试失败项"
              :disabled="uploads.finished.filter((task) => task.status === 'error').length === 0"
              @click="uploads.retryAllFailed()"
            />
          </AppTooltip>
          <AppTooltip content="清除已结束的记录">
            <AppButton
              size="sm"
              variant="ghost"
              icon="trash"
              label="清除已完成"
              :disabled="uploads.finished.length === 0"
              @click="uploads.clearFinished()"
            />
          </AppTooltip>
        </div>
      </header>

      <div v-if="!uploads.collapsed" class="upload-panel__body">
        <div class="upload-panel__summary">
          <AppProgress :value="uploads.uploadedBytes" :max="Math.max(1, uploads.totalBytes)" :height="5" />
          <span class="upload-panel__summary-text">
            {{ formatBytes(uploads.uploadedBytes) }} / {{ formatBytes(uploads.totalBytes) }}
          </span>
        </div>

        <ul class="upload-panel__list">
          <li v-for="task in uploads.tasks" :key="task.id" class="upload-item">
            <div class="upload-item__main">
              <p class="upload-item__name truncate" :title="task.name">{{ task.name }}</p>
              <p class="upload-item__meta">
                <span>{{ statusText[task.status] ?? task.status }}</span>
                <span>·</span>
                <span>{{ formatBytes(task.size) }}</span>
                <template v-if="task.mode && !task.inline">
                  <span>·</span>
                  <span>{{ UPLOAD_MODE_LABEL[task.mode] }}</span>
                </template>
                <template v-if="speedOf(task.id)">
                  <span>·</span>
                  <span>{{ speedOf(task.id) }}</span>
                </template>
              </p>
              <p v-if="task.error" class="upload-item__error truncate" :title="task.error">{{ task.error }}</p>
            </div>
            <div class="upload-item__side">
              <AppProgress
                v-if="['uploading', 'preparing', 'finalizing', 'paused', 'queued'].includes(task.status)"
                :value="uploads.taskRatio(task) * 100"
                :tone="toneOf(task.status)"
                :height="4"
              />
              <div class="upload-item__actions">
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
                  v-if="['done', 'error', 'canceled'].includes(task.status)"
                  size="sm"
                  variant="ghost"
                  icon="x"
                  label="移除记录"
                  @click="uploads.remove(task.id)"
                />
                <AppButton
                  v-else
                  size="sm"
                  variant="ghost"
                  icon="x"
                  label="取消上传"
                  @click="uploads.cancel(task)"
                />
              </div>
            </div>
          </li>
        </ul>

        <footer v-if="modeOptions.length > 1" class="upload-panel__foot">
          <span class="upload-panel__foot-label">传输方式</span>
          <AppSelect
            size="sm"
            :model-value="uploads.mode || modeOptions[0]?.value"
            :options="modeOptions"
            @update:model-value="uploads.setMode($event as 'UPLOAD_MODE_PROXY' | 'UPLOAD_MODE_PRESIGNED')"
          />
          <span class="upload-panel__foot-hint">
            {{ (uploads.mode || modeOptions[0]?.value) === 'UPLOAD_MODE_PROXY' ? '经服务端中转，兼容性最好' : '直传对象存储，需要对象存储允许跨域' }}
          </span>
        </footer>
      </div>
    </section>
  </Transition>
</template>

<style scoped>
.upload-panel {
  position: fixed;
  right: var(--space-5);
  bottom: var(--space-5);
  width: min(420px, calc(100vw - 2 * var(--space-4)));
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-xl);
  z-index: var(--z-drawer);
  overflow: hidden;
}

.upload-panel__head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--border-subtle);
  background: var(--surface-2);
}

.upload-panel.is-collapsed .upload-panel__head {
  border-bottom: 0;
}

.upload-panel__toggle {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  border: 0;
  background: transparent;
  color: var(--text-strong);
  font-size: var(--text-sm);
  font-weight: 600;
  cursor: pointer;
  padding: var(--space-1) var(--space-1);
}

.upload-panel__count {
  font-size: var(--text-2xs);
  color: var(--text-muted);
  background: var(--surface-3);
  border-radius: var(--radius-pill);
  padding: 1px 8px;
}

.upload-panel__head-actions {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.upload-panel__body {
  max-height: min(52vh, 460px);
  display: flex;
  flex-direction: column;
}

.upload-panel__summary {
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  border-bottom: 1px solid var(--border-subtle);
}

.upload-panel__summary-text {
  font-size: var(--text-2xs);
  color: var(--text-muted);
  font-variant-numeric: tabular-nums;
}

.upload-panel__list {
  list-style: none;
  margin: 0;
  padding: 0;
  overflow-y: auto;
}

.upload-item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3);
  border-bottom: 1px solid var(--border-subtle);
}

.upload-item:last-child {
  border-bottom: 0;
}

.upload-item__main {
  flex: 1 1 auto;
  min-width: 0;
}

.upload-item__name {
  font-size: var(--text-sm);
  color: var(--text-strong);
}

.upload-item__meta {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: var(--text-2xs);
  color: var(--text-muted);
  margin-top: 2px;
}

.upload-item__error {
  font-size: var(--text-2xs);
  color: var(--danger-600);
  margin-top: 2px;
}

.upload-item__side {
  width: 132px;
  flex: none;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  align-items: flex-end;
}

.upload-item__actions {
  display: flex;
  align-items: center;
  gap: var(--space-1);
}

.upload-panel__foot {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3);
  border-top: 1px solid var(--border-subtle);
  background: var(--surface-2);
  flex-wrap: wrap;
}

.upload-panel__foot-label {
  font-size: var(--text-2xs);
  color: var(--text-muted);
}

.upload-panel__foot-hint {
  font-size: var(--text-2xs);
  color: var(--text-faint);
  flex: 1 1 100%;
}

@media (max-width: 640px) {
  .upload-panel {
    right: var(--space-3);
    left: var(--space-3);
    bottom: var(--space-3);
    width: auto;
  }

  .upload-item__side {
    width: 96px;
  }
}
</style>
