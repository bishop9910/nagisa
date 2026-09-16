<script setup lang="ts">
/**
 * 审计日志：聚合摘要 + 明细查询。
 * 没有 PERMISSION_AUDIT_READ 的调用方只能看到自己作为操作者的记录（服务端自动收窄）。
 */
import { computed, onMounted, ref, watch } from 'vue'

import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppPagination from '@/components/ui/AppPagination.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import { auditApi, errorText } from '@/api'
import type { AuditLog, AuditSummary } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { AUDIT_ACTION_GROUPS } from '@/utils/constants'
import { buildFilter, eq, has, orderBy } from '@/utils/filter'
import { formatDateTime, formatNumber, fromLocalInputValue, toInt } from '@/utils/format'

const auth = useAuthStore()
const ui = useUiStore()

const summary = ref<AuditSummary | null>(null)
const summaryFrom = ref('')
const summaryTo = ref('')
const summaryPrefix = ref('')
const summaryLoading = ref(false)

const logs = ref<AuditLog[]>([])
const loading = ref(false)
const error = ref('')
const actorName = ref('')
const actionValue = ref('')
const successFilter = ref('')
const targetType = ref('')
const ipFilter = ref('')
const createdAfter = ref('')
const createdBefore = ref('')
const orderField = ref('created_at')
const orderDesc = ref(true)
const totalSize = ref(0)
const page = ref(1)
const pageSize = ref(50)
const tokenStack = ref<string[]>([''])
const nextToken = ref('')

const detailOpen = ref(false)
const detail = ref<AuditLog | null>(null)
const detailLoading = ref(false)

const targetOptions = [
  { value: '', label: '全部对象' },
  { value: 'node', label: '节点' },
  { value: 'user', label: '账号' },
  { value: 'share', label: '分享' },
  { value: 'system', label: '系统' },
]

const orderOptions = [
  { value: 'created_at', label: '按时间' },
  { value: 'action', label: '按动作' },
  { value: 'actor_name', label: '按操作者' },
]

const successOptions = [
  { value: '', label: '全部结果' },
  { value: 'true', label: '仅成功' },
  { value: 'false', label: '仅失败' },
]

const actionRows = computed(() => (summary.value?.byAction ?? []).slice(0, 12))
const actorRows = computed(() => (summary.value?.byActor ?? []).slice(0, 10))
const dayRows = computed(() => {
  const map = summary.value?.byDay ?? {}
  const rows = Object.entries(map).map(([day, count]) => ({ day, count: toInt(count) }))
  rows.sort((a, b) => a.day.localeCompare(b.day))
  const max = rows.reduce((acc, row) => Math.max(acc, row.count), 0) || 1
  return rows.map((row) => ({ ...row, ratio: row.count / max }))
})

function currentFilter(): string | undefined {
  // 布尔字段用 AIP 的裸标识符形式：`success` 表示成功，`NOT success` 表示失败。
  // 服务端也会把 `success=true` / `success=false` 归一化成这两种写法，这里用裸形式
  // 是为了在更早版本的后端上同样可用。
  const success = successFilter.value === 'true' ? 'success' : successFilter.value === 'false' ? 'NOT success' : undefined
  return buildFilter(
    actorName.value.trim() ? has('actor_name', actorName.value.trim()) : undefined,
    actionValue.value.trim() ? has('action', actionValue.value.trim()) : undefined,
    success,
    targetType.value ? eq('target_type', targetType.value) : undefined,
    ipFilter.value.trim() ? eq('ip', ipFilter.value.trim()) : undefined,
    createdAfter.value ? `created_at>="${new Date(createdAfter.value).toISOString()}"` : undefined,
    createdBefore.value ? `created_at<="${new Date(createdBefore.value).toISOString()}"` : undefined,
  )
}

async function loadSummary(): Promise<void> {
  if (!auth.canReadAudit) return
  summaryLoading.value = true
  try {
    summary.value = await auditApi.getAuditSummary({
      from: fromLocalInputValue(summaryFrom.value),
      to: fromLocalInputValue(summaryTo.value),
      actionPrefix: summaryPrefix.value || undefined,
    })
  } catch (err) {
    ui.toast.error('摘要加载失败', errorText(err))
  } finally {
    summaryLoading.value = false
  }
}

async function loadLogs(reset = false): Promise<void> {
  if (reset) {
    page.value = 1
    tokenStack.value = ['']
  }
  loading.value = true
  error.value = ''
  try {
    const result = await auditApi.listAuditLogs({
      pageSize: pageSize.value,
      pageToken: tokenStack.value[page.value - 1] || undefined,
      filter: currentFilter(),
      orderBy: orderBy(orderField.value, orderDesc.value),
    })
    logs.value = result.logs ?? []
    totalSize.value = toInt(result.totalSize, logs.value.length)
    nextToken.value = result.nextPageToken ?? ''
  } catch (err) {
    error.value = errorText(err)
    logs.value = []
  } finally {
    loading.value = false
  }
}

function nextPage(): void {
  if (!nextToken.value) return
  tokenStack.value = [...tokenStack.value.slice(0, page.value), nextToken.value]
  page.value += 1
  void loadLogs()
}

function prevPage(): void {
  if (page.value <= 1) return
  page.value -= 1
  void loadLogs()
}

function onPageSize(size: number): void {
  pageSize.value = size
  void loadLogs(true)
}

async function openDetail(log: AuditLog): Promise<void> {
  detail.value = log
  detailOpen.value = true
  if (!log.id) return
  detailLoading.value = true
  try {
    detail.value = await auditApi.getAuditLog(log.id)
  } catch {
    /* 列表里已有的字段足够展示 */
  } finally {
    detailLoading.value = false
  }
}

watch([actorName, actionValue, successFilter, targetType, ipFilter, createdAfter, createdBefore, orderField, orderDesc], () =>
  void loadLogs(true),
)

onMounted(async () => {
  await loadSummary()
  await loadLogs()
})
</script>

<template>
  <div class="audit">
    <section v-if="auth.canReadAudit" class="card">
      <header class="card__header">
        <div>
          <p class="card__title">操作摘要</p>
          <p class="card__subtitle">
            {{ summary?.from ? formatDateTime(summary.from) : '最近 7 天' }} 至
            {{ summary?.to ? formatDateTime(summary.to) : '现在' }}
          </p>
        </div>
        <AppButton size="sm" variant="ghost" icon="refresh" :loading="summaryLoading" @click="loadSummary">刷新</AppButton>
      </header>
      <div class="card__body audit__summary">
        <div class="audit__filters">
          <AppInput v-model="summaryFrom" type="datetime-local" size="sm" />
          <AppInput v-model="summaryTo" type="datetime-local" size="sm" />
          <AppSelect
            v-model="summaryPrefix"
            size="sm"
            :options="[{ value: '', label: '全部动作' }, ...AUDIT_ACTION_GROUPS.map((group) => ({ value: group.value, label: group.label }))]"
          />
          <AppButton size="sm" @click="loadSummary">应用</AppButton>
        </div>

        <div class="stat-grid">
          <div class="stat">
            <span class="stat__label">总操作数</span>
            <span class="stat__value">{{ formatNumber(summary?.total) }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">失败次数</span>
            <span class="stat__value">{{ formatNumber(summary?.failureCount) }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">覆盖动作</span>
            <span class="stat__value">{{ actionRows.length }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">活跃账号</span>
            <span class="stat__value">{{ actorRows.length }}</span>
          </div>
        </div>

        <div v-if="dayRows.length > 0" class="audit__chart">
          <p class="text-xs muted mb-2">按天分布</p>
          <div class="audit__bars">
            <div v-for="row in dayRows" :key="row.day" class="audit__bar-item" :title="`${row.day}：${row.count}`">
              <div class="audit__bar" :style="{ height: `${Math.max(4, row.ratio * 100)}%` }" />
              <span class="audit__bar-label">{{ row.day.slice(5) }}</span>
            </div>
          </div>
        </div>

        <div class="audit__tables">
          <div class="audit__table">
            <p class="text-xs muted mb-2">按动作</p>
            <ul class="audit__rank">
              <li v-for="row in actionRows" :key="row.action">
                <span class="truncate">{{ row.actionDisplay || row.action }}</span>
                <span class="mono">{{ formatNumber(row.count) }}</span>
              </li>
            </ul>
          </div>
          <div class="audit__table">
            <p class="text-xs muted mb-2">按操作者</p>
            <ul class="audit__rank">
              <li v-for="row in actorRows" :key="row.actorId || row.actorName">
                <span class="truncate">{{ row.actorName || row.actorId }}</span>
                <span class="mono">{{ formatNumber(row.count) }}</span>
              </li>
            </ul>
          </div>
        </div>
      </div>
    </section>

    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">审计明细</p>
          <p class="card__subtitle">共 {{ formatNumber(totalSize) }} 条记录</p>
        </div>
        <AppButton size="sm" variant="ghost" icon="refresh" @click="loadLogs()">刷新</AppButton>
      </header>

      <div class="audit__filters audit__filters--wrap">
        <AppInput v-model="actorName" size="sm" placeholder="操作者" style="width: 150px" />
        <AppInput v-model="actionValue" size="sm" placeholder="动作前缀，如 node." style="width: 180px" />
        <AppSelect v-model="successFilter" size="sm" :options="successOptions" style="width: 130px" />
        <AppSelect v-model="targetType" size="sm" :options="targetOptions" style="width: 130px" />
        <AppInput v-model="ipFilter" size="sm" placeholder="来源 IP" style="width: 140px" />
        <AppInput v-model="createdAfter" type="datetime-local" size="sm" style="width: 190px" />
        <AppInput v-model="createdBefore" type="datetime-local" size="sm" style="width: 190px" />
        <span class="toolbar__spacer" />
        <AppSelect v-model="orderField" size="sm" :options="orderOptions" style="width: 130px" />
        <AppButton size="sm" variant="ghost" :icon="orderDesc ? 'arrow-down' : 'arrow-up'" @click="orderDesc = !orderDesc">
          {{ orderDesc ? '降序' : '升序' }}
        </AppButton>
      </div>

      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th style="width: 150px">时间</th>
              <th style="width: 130px">操作者</th>
              <th style="width: 170px">动作</th>
              <th style="width: 160px">对象</th>
              <th style="width: 90px">结果</th>
              <th style="width: 130px">来源</th>
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td colspan="6" class="audit__state muted text-sm">加载中…</td>
            </tr>
            <tr v-else-if="logs.length === 0">
              <td colspan="6" class="audit__state">
                <AppEmpty
                  size="sm"
                  :icon="error ? 'alert-circle' : 'list-checks'"
                  :title="error ? '加载失败' : '没有审计记录'"
                  :description="error || '读取类调用不产生审计记录'"
                >
                  <AppButton v-if="error" size="sm" icon="refresh" @click="loadLogs()">重试</AppButton>
                </AppEmpty>
              </td>
            </tr>
            <template v-else>
              <tr v-for="log in logs" :key="log.id" class="is-hoverable" @click="openDetail(log)">
                <td class="text-xs muted nowrap">{{ formatDateTime(log.createdAt) }}</td>
                <td class="text-xs">{{ log.actorName || log.actorId || '匿名' }}</td>
                <td>
                  <span class="text-xs strong">{{ log.actionDisplay || log.action }}</span>
                  <span class="mono faint"> {{ log.action }}</span>
                </td>
                <td class="text-xs">
                  <template v-if="log.targetName || log.targetId">
                    {{ log.targetName || log.targetId }}
                    <span class="faint">（{{ log.targetType }}）</span>
                  </template>
                  <span v-else class="faint">—</span>
                </td>
                <td>
                  <AppBadge :tone="log.success ? 'success' : 'danger'" size="sm">
                    {{ log.success ? '成功' : log.errorReason || '失败' }}
                  </AppBadge>
                </td>
                <td class="text-xs muted">{{ log.ip || '—' }}</td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>

      <AppPagination
        :page="page"
        :page-size="pageSize"
        :total="totalSize"
        :page-sizes="[50, 100, 200]"
        :has-prev="page > 1"
        :has-next="Boolean(nextToken)"
        :disabled="loading"
        @prev="prevPage"
        @next="nextPage"
        @update:page-size="onPageSize"
      />
    </section>

    <AppDialog v-model="detailOpen" title="审计详情" size="md">
      <div v-if="detailLoading" class="muted text-sm">加载中…</div>
      <dl v-else-if="detail" class="audit__detail">
        <div><dt>时间</dt><dd>{{ formatDateTime(detail.createdAt) }}</dd></div>
        <div><dt>操作者</dt><dd>{{ detail.actorName || '—' }} <span class="mono faint">{{ detail.actorId }}</span></dd></div>
        <div><dt>动作</dt><dd>{{ detail.actionDisplay || detail.action }} <span class="mono faint">{{ detail.action }}</span></dd></div>
        <div><dt>对象</dt><dd>{{ detail.targetName || '—' }} <span class="mono faint">{{ detail.targetType }} / {{ detail.targetId }}</span></dd></div>
        <div><dt>结果</dt><dd>{{ detail.success ? '成功' : `失败：${detail.errorReason || '未知'}` }}</dd></div>
        <div><dt>来源</dt><dd>{{ detail.ip || '—' }}</dd></div>
        <div><dt>客户端</dt><dd class="break-all">{{ detail.userAgent || '—' }}</dd></div>
        <div><dt>请求 ID</dt><dd class="mono break-all">{{ detail.requestId || '—' }}</dd></div>
        <div v-if="detail.detail && Object.keys(detail.detail).length > 0">
          <dt>明细</dt>
          <dd>
            <ul class="audit__detail-map">
              <li v-for="(value, key) in detail.detail" :key="key">
                <span class="mono">{{ key }}</span>
                <span>{{ value }}</span>
              </li>
            </ul>
          </dd>
        </div>
      </dl>
      <template #footer>
        <AppButton variant="ghost" @click="detailOpen = false">关闭</AppButton>
      </template>
    </AppDialog>
  </div>
</template>

<style scoped>
.audit {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.audit__summary {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.audit__filters {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-5);
  border-bottom: 1px solid var(--border-subtle);
  flex-wrap: wrap;
}

.audit__filters--wrap {
  flex-wrap: wrap;
}

.audit__chart {
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  background: var(--surface-2);
}

.audit__bars {
  display: flex;
  align-items: flex-end;
  gap: var(--space-2);
  height: 120px;
  overflow-x: auto;
}

.audit__bar-item {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 4px;
  min-width: 26px;
  height: 100%;
  justify-content: flex-end;
}

.audit__bar {
  width: 16px;
  border-radius: var(--radius-xs);
  background: linear-gradient(180deg, var(--brand-400), var(--brand-600));
  min-height: 4px;
}

.audit__bar-label {
  font-size: 9px;
  color: var(--text-faint);
  white-space: nowrap;
}

.audit__tables {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: var(--space-4);
}

.audit__rank {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.audit__rank li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  font-size: var(--text-xs);
  color: var(--text-muted);
  padding: 3px 0;
  border-bottom: 1px dashed var(--border-subtle);
}

.audit__state {
  padding: var(--space-6) !important;
  text-align: center;
}

.audit__detail {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0;
  font-size: var(--text-xs);
}

.audit__detail > div {
  display: grid;
  grid-template-columns: 90px 1fr;
  gap: var(--space-3);
}

.audit__detail dt {
  color: var(--text-muted);
}

.audit__detail dd {
  margin: 0;
  min-width: 0;
}

.audit__detail-map {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.audit__detail-map li {
  display: flex;
  gap: var(--space-2);
}
</style>
