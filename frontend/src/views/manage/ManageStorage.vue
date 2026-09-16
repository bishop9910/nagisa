<script setup lang="ts">
/** 存储与维护：全局统计、按类型分布、Top 用户与维护任务执行。 */
import { computed, onMounted, ref } from 'vue'

import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppField from '@/components/ui/AppField.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppProgress from '@/components/ui/AppProgress.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import { errorText, systemApi } from '@/api'
import type { MaintenanceReport, StorageStats } from '@/api/types'
import { useUiStore } from '@/stores/ui'
import { MAINTENANCE_TASKS, SIZE_CATEGORY_LABEL } from '@/utils/constants'
import { formatBytes, formatDateTime, formatNumber, formatPercent, toInt } from '@/utils/format'

const ui = useUiStore()

const stats = ref<StorageStats | null>(null)
const loading = ref(false)
const topOwners = ref(10)

const tasks = ref<string[]>(['expire_uploads', 'purge_deletions', 'expire_shares', 'recount_usage'])
const dryRun = ref(true)
const retentionDays = ref<number | null>(null)
const running = ref(false)
const report = ref<MaintenanceReport | null>(null)

const categoryRows = computed(() => {
  const map = stats.value?.sizeByCategory ?? {}
  const rows = Object.entries(map).map(([key, value]) => ({
    key,
    label: SIZE_CATEGORY_LABEL[key] ?? key,
    value: toInt(value),
  }))
  const total = rows.reduce((sum, row) => sum + row.value, 0) || 1
  return rows.map((row) => ({ ...row, ratio: row.value / total })).sort((a, b) => b.value - a.value)
})

async function load(): Promise<void> {
  loading.value = true
  try {
    stats.value = await systemApi.getStorageStats(topOwners.value)
  } catch (err) {
    ui.toast.error('统计加载失败', errorText(err))
  } finally {
    loading.value = false
  }
}

function toggleTask(task: string, checked: boolean): void {
  const set = new Set(tasks.value)
  if (checked) set.add(task)
  else set.delete(task)
  tasks.value = Array.from(set)
}

async function runMaintenance(): Promise<void> {
  running.value = true
  report.value = null
  try {
    report.value = await systemApi.runMaintenance({
      dryRun: dryRun.value,
      tasks: tasks.value.length > 0 ? tasks.value : undefined,
      trashRetentionDays: retentionDays.value ?? undefined,
    })
    ui.toast.success(dryRun.value ? '演练完成' : '维护任务已执行')
    if (!dryRun.value) await load()
  } catch (err) {
    ui.toast.error('执行失败', errorText(err))
  } finally {
    running.value = false
  }
}

const reportRows = computed(() => {
  const data = report.value
  if (!data) return []
  return [
    { label: '过期上传会话', value: toInt(data.expiredUploads) },
    { label: '清理的待删除对象', value: toInt(data.deletedObjects) },
    { label: '回收的孤儿对象', value: toInt(data.orphanObjects) },
    { label: '置为过期的分享', value: toInt(data.expiredShares) },
    { label: '重算用量的账号', value: toInt(data.recountedUsers) },
    { label: '彻底删除的节点', value: toInt(data.purgedNodes) },
  ]
})

onMounted(() => void load())
</script>

<template>
  <div class="storage">
    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">存储统计</p>
          <p class="card__subtitle">来自 GET /v1/system/storage/stats</p>
        </div>
        <div class="flex items-center gap-2">
          <AppInput
            v-model="topOwners"
            type="number"
            size="sm"
            :min="1"
            :max="100"
            style="width: 110px"
            placeholder="Top N"
          />
          <AppButton size="sm" variant="ghost" icon="refresh" :loading="loading" @click="load">刷新</AppButton>
        </div>
      </header>
      <div class="card__body">
        <div v-if="!stats" class="muted text-sm">加载中…</div>
        <template v-else>
          <div class="stat-grid">
            <div class="stat">
              <span class="stat__label">总占用</span>
              <span class="stat__value">{{ formatBytes(stats.totalBytes) }}</span>
              <span class="stat__hint">后端已用 {{ formatBytes(stats.backendUsedBytes) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">文件</span>
              <span class="stat__value">{{ formatNumber(stats.totalFiles) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">文件夹</span>
              <span class="stat__value">{{ formatNumber(stats.totalFolders) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">账号 / 正常 / 禁用</span>
              <span class="stat__value">
                {{ formatNumber(stats.totalUsers) }} / {{ formatNumber(stats.activeUsers) }} /
                {{ formatNumber(stats.disabledUsers) }}
              </span>
            </div>
            <div class="stat">
              <span class="stat__label">回收站</span>
              <span class="stat__value">{{ formatBytes(stats.trashedBytes) }}</span>
              <span class="stat__hint">{{ formatNumber(stats.trashedNodes) }} 个节点</span>
            </div>
            <div class="stat">
              <span class="stat__label">历史版本</span>
              <span class="stat__value">{{ formatBytes(stats.versionsBytes) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">进行中的上传</span>
              <span class="stat__value">{{ formatNumber(stats.uploadsInProgress) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">有效分享</span>
              <span class="stat__value">{{ formatNumber(stats.activeShares) }}</span>
            </div>
          </div>

          <div v-if="categoryRows.length > 0" class="storage__categories">
            <p class="text-xs muted mb-2">按类型分布</p>
            <div v-for="row in categoryRows" :key="row.key" class="storage__category">
              <span class="text-xs">{{ row.label }}</span>
              <AppProgress :value="row.ratio * 100" :height="5" />
              <span class="text-xs muted nowrap">{{ formatBytes(row.value) }} · {{ formatPercent(row.ratio) }}</span>
            </div>
          </div>
        </template>
      </div>
    </section>

    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">占用最多的账号</p>
          <p class="card__subtitle">按 used_bytes 降序</p>
        </div>
      </header>
      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>账号</th>
              <th class="is-num" style="width: 130px">已用</th>
              <th class="is-num" style="width: 130px">配额</th>
              <th style="width: 180px">占比</th>
              <th class="is-num" style="width: 110px">文件 / 文件夹</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="!stats || (stats.topOwners ?? []).length === 0">
              <td colspan="5">
                <AppEmpty size="sm" icon="users" title="没有用量数据" description="还没有账号占用存储" />
              </td>
            </tr>
            <template v-else>
              <tr v-for="owner in stats?.topOwners ?? []" :key="owner.ownerId">
                <td>
                  <span class="text-sm strong">{{ owner.ownerName || owner.ownerId }}</span>
                  <span class="mono faint"> {{ owner.ownerId }}</span>
                </td>
                <td class="is-num text-xs">{{ formatBytes(owner.usedBytes) }}</td>
                <td class="is-num text-xs">
                  {{ toInt(owner.quotaBytes) > 0 ? formatBytes(owner.quotaBytes) : '不限' }}
                </td>
                <td>
                  <AppProgress
                    :value="toInt(owner.quotaBytes) > 0 ? (toInt(owner.usedBytes) / toInt(owner.quotaBytes)) * 100 : 0"
                    :height="4"
                    :tone="
                      toInt(owner.quotaBytes) > 0 && toInt(owner.usedBytes) / toInt(owner.quotaBytes) > 0.9
                        ? 'danger'
                        : 'brand'
                    "
                  />
                  <p class="text-xs faint mt-1">
                    {{
                      toInt(owner.quotaBytes) > 0
                        ? formatPercent(toInt(owner.usedBytes) / toInt(owner.quotaBytes))
                        : '不限制'
                    }}
                  </p>
                </td>
                <td class="is-num text-xs">
                  {{ formatNumber(owner.fileCount) }} / {{ formatNumber(owner.folderCount) }}
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>
    </section>

    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">维护任务</p>
          <p class="card__subtitle">POST /v1/system/maintenance/run · 未知任务名会写进 warnings 而不报错</p>
        </div>
      </header>
      <div class="card__body storage__maintenance">
        <div class="storage__task-list">
          <label v-for="task in MAINTENANCE_TASKS" :key="task.value" class="storage__task">
            <AppCheckbox
              :model-value="tasks.includes(task.value)"
              @update:model-value="toggleTask(task.value, $event)"
            >
              <span class="storage__task-title">{{ task.label }}</span>
              <span class="storage__task-hint">{{ task.hint }}</span>
            </AppCheckbox>
          </label>
        </div>

        <div class="storage__maintenance-controls">
          <AppField label="只演练不修改（dry run）" inline>
            <AppSwitch v-model="dryRun" label="只演练不修改" />
          </AppField>
          <AppField label="回收站保留天数" hint="留空则使用 storage.trash_retention 的默认值">
            <AppInput v-model="retentionDays" type="number" :min="1" placeholder="例如 30" />
          </AppField>
          <AppButton variant="primary" icon="wrench" :loading="running" @click="runMaintenance">
            {{ dryRun ? '演练一次' : '执行维护' }}
          </AppButton>
          <p v-if="!dryRun" class="storage__warn">
            <AppIcon name="alert-triangle" :size="15" />
            关闭演练后任务会真正删除对象与节点，请确认参数无误。
          </p>
        </div>

        <div v-if="report" class="storage__report">
          <div class="storage__report-head">
            <p class="card__title">
              执行结果
              <AppBadge :tone="report.dryRun ? 'warning' : 'success'" size="sm">
                {{ report.dryRun ? '演练' : '已执行' }}
              </AppBadge>
            </p>
            <span class="text-xs muted">
              用时 {{ toInt(report.durationMs) }} ms · {{ formatDateTime(report.finishedAt) }}
            </span>
          </div>
          <dl class="storage__report-grid">
            <div v-for="row in reportRows" :key="row.label">
              <dt>{{ row.label }}</dt>
              <dd>{{ formatNumber(row.value) }}</dd>
            </div>
          </dl>
          <p v-if="(report.tasks ?? []).length > 0" class="text-xs faint mt-2">
            本次任务：{{ (report.tasks ?? []).join('、') }}
          </p>
          <ul v-if="(report.warnings ?? []).length > 0" class="storage__warnings">
            <li v-for="warning in report.warnings" :key="warning">
              <AppIcon name="alert-triangle" :size="14" />
              {{ warning }}
            </li>
          </ul>
        </div>
      </div>
    </section>
  </div>
</template>

<style scoped>
.storage {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.storage__categories {
  margin-top: var(--space-5);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.storage__category {
  display: grid;
  grid-template-columns: 96px 1fr 180px;
  align-items: center;
  gap: var(--space-3);
}

.storage__maintenance {
  display: grid;
  grid-template-columns: minmax(0, 1.4fr) minmax(0, 1fr);
  gap: var(--space-5);
  align-items: start;
}

.storage__task-list {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.storage__task {
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  background: var(--surface-2);
}

.storage__task-title {
  display: block;
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text);
}

.storage__task-hint {
  display: block;
  font-size: var(--text-2xs);
  color: var(--text-muted);
}

.storage__maintenance-controls {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-4);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: var(--surface-2);
}

.storage__warn {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  font-size: var(--text-2xs);
  color: var(--warning-600);
}

.storage__report {
  grid-column: 1 / -1;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-4);
}

.storage__report-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  margin-bottom: var(--space-3);
  flex-wrap: wrap;
}

.storage__report-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(150px, 1fr));
  gap: var(--space-3);
  margin: 0;
}

.storage__report-grid dt {
  font-size: var(--text-2xs);
  color: var(--text-muted);
}

.storage__report-grid dd {
  margin: 2px 0 0;
  font-size: var(--text-lg);
  font-weight: 620;
  color: var(--text-strong);
}

.storage__warnings {
  list-style: none;
  margin: var(--space-3) 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  font-size: var(--text-xs);
  color: var(--warning-600);
}

.storage__warnings li {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

@media (max-width: 1024px) {
  .storage__maintenance {
    grid-template-columns: 1fr;
  }
}
</style>
