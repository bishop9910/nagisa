<script setup lang="ts">
/** 管理概览：部署信息、健康检查与全局存储摘要。 */
import { computed, onMounted, ref } from 'vue'

import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppProgress from '@/components/ui/AppProgress.vue'
import { errorText, systemApi } from '@/api'
import type { HealthStatus, StorageStats } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useSystemStore } from '@/stores/system'
import { useUiStore } from '@/stores/ui'
import { formatBytes, formatDuration, formatNumber, toInt } from '@/utils/format'

const auth = useAuthStore()
const system = useSystemStore()
const ui = useUiStore()

const storage = ref<StorageStats | null>(null)
const health = ref<HealthStatus | null>(null)
const loadingStorage = ref(false)
const probing = ref(false)

const categoryRows = computed(() => {
  const map = storage.value?.sizeByCategory ?? {}
  const rows = Object.entries(map).map(([key, value]) => ({ key, value: toInt(value) }))
  const total = rows.reduce((sum, row) => sum + row.value, 0) || 1
  return rows
    .map((row) => ({ ...row, ratio: row.value / total }))
    .sort((a, b) => b.value - a.value)
})

const healthTone = computed(() => {
  const status = health.value?.status
  if (status === 'ok') return 'success'
  if (status === 'degraded') return 'warning'
  return 'danger'
})

async function loadStorage(): Promise<void> {
  if (!auth.canManageStorage) return
  loadingStorage.value = true
  try {
    storage.value = await systemApi.getStorageStats(10)
  } catch (err) {
    ui.toast.error('存储统计加载失败', errorText(err))
  } finally {
    loadingStorage.value = false
  }
}

async function probe(deep: boolean): Promise<void> {
  probing.value = true
  try {
    health.value = await systemApi.healthCheck(deep)
  } catch (err) {
    ui.toast.error('健康检查失败', errorText(err))
  } finally {
    probing.value = false
  }
}

onMounted(async () => {
  await system.loadInfo(true)
  health.value = await system.loadHealth(false)
  await loadStorage()
})
</script>

<template>
  <div class="overview">
    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">部署信息</p>
          <p class="card__subtitle">来自 GET /v1/system/info（公开接口）</p>
        </div>
        <AppButton size="sm" variant="ghost" icon="refresh" @click="system.loadInfo(true)">刷新</AppButton>
      </header>
      <div class="card__body">
        <div class="stat-grid">
          <div class="stat">
            <span class="stat__label">服务名称</span>
            <span class="stat__value text-lg">{{ system.name }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">构建版本</span>
            <span class="stat__value text-lg">{{ system.version }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">API 版本</span>
            <span class="stat__value text-lg">{{ system.apiVersion }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">对象存储</span>
            <span class="stat__value text-lg">{{ system.info?.storageBackend || '—' }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">数据库</span>
            <span class="stat__value text-lg">{{ system.databaseBackend }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">默认可见范围</span>
            <span class="stat__value text-lg">{{ system.info?.defaultVisibility || '—' }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">最大单文件</span>
            <span class="stat__value text-lg">{{ system.maxUploadSize > 0 ? formatBytes(system.maxUploadSize) : '不限' }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">默认分片</span>
            <span class="stat__value text-lg">{{ formatBytes(system.defaultChunkSize) }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">内联上传上限</span>
            <span class="stat__value text-lg">{{ formatBytes(system.maxInlineSize) }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">上传会话有效期</span>
            <span class="stat__value text-lg">{{ formatDuration(system.uploadSessionTtl) }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">签名地址有效期</span>
            <span class="stat__value text-lg">{{ formatDuration(system.signedUrlTtl) }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">对外基地址</span>
            <span class="stat__value text-sm break-all">{{ system.publicBaseUrl || '（相对路径）' }}</span>
          </div>
        </div>

        <div class="overview__features">
          <span class="text-xs muted">能力</span>
          <AppBadge v-for="feature in system.features" :key="feature" tone="brand">{{ feature }}</AppBadge>
          <AppBadge v-if="system.uploadModes.length" tone="info">
            {{ system.uploadModes.map((mode) => (mode === 'UPLOAD_MODE_PROXY' ? '代理上传' : '预签名上传')).join(' / ') }}
          </AppBadge>
        </div>
      </div>
    </section>

    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">健康检查</p>
          <p class="card__subtitle">
            <AppBadge :tone="healthTone">{{ health?.status || '未知' }}</AppBadge>
            <span class="ml-2 text-xs muted">运行 {{ formatDuration(health?.uptimeSeconds) }}</span>
          </p>
        </div>
        <div class="flex gap-2">
          <AppButton size="sm" :loading="probing" icon="activity" @click="probe(false)">快速检查</AppButton>
          <AppButton size="sm" variant="primary" :loading="probing" icon="zap" @click="probe(true)">深度探测</AppButton>
        </div>
      </header>
      <div class="card__body">
        <ul class="overview__checks">
          <li v-for="(value, key) in health?.checks ?? {}" :key="key">
            <AppIcon :name="value === 'ok' ? 'check-circle' : 'alert-triangle'" :size="16" />
            <span class="strong">{{ key }}</span>
            <span class="text-xs muted">{{ value }}</span>
          </li>
        </ul>
        <p class="text-xs faint mt-2">
          深度探测会真正访问数据库与对象存储；接口公开，但只有它能反映真实依赖状态。
        </p>
      </div>
    </section>

    <section v-if="auth.canManageStorage" class="card">
      <header class="card__header">
        <div>
          <p class="card__title">存储概览</p>
          <p class="card__subtitle">来自 GET /v1/system/storage/stats</p>
        </div>
        <AppButton size="sm" variant="ghost" icon="refresh" :loading="loadingStorage" @click="loadStorage">刷新</AppButton>
      </header>
      <div class="card__body">
        <div v-if="!storage" class="muted text-sm">加载中…</div>
        <template v-else>
          <div class="stat-grid">
            <div class="stat">
              <span class="stat__label">总占用</span>
              <span class="stat__value">{{ formatBytes(storage.totalBytes) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">文件 / 文件夹</span>
              <span class="stat__value">{{ formatNumber(storage.totalFiles) }} / {{ formatNumber(storage.totalFolders) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">账号</span>
              <span class="stat__value">{{ formatNumber(storage.totalUsers) }}</span>
              <span class="stat__hint">
                正常 {{ formatNumber(storage.activeUsers) }} · 禁用 {{ formatNumber(storage.disabledUsers) }}
              </span>
            </div>
            <div class="stat">
              <span class="stat__label">回收站</span>
              <span class="stat__value">{{ formatBytes(storage.trashedBytes) }}</span>
              <span class="stat__hint">{{ formatNumber(storage.trashedNodes) }} 个节点</span>
            </div>
            <div class="stat">
              <span class="stat__label">历史版本占用</span>
              <span class="stat__value">{{ formatBytes(storage.versionsBytes) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">进行中的上传</span>
              <span class="stat__value">{{ formatNumber(storage.uploadsInProgress) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">有效分享</span>
              <span class="stat__value">{{ formatNumber(storage.activeShares) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">后端已用</span>
              <span class="stat__value">{{ formatBytes(storage.backendUsedBytes) }}</span>
            </div>
          </div>

          <div v-if="categoryRows.length" class="overview__categories">
            <p class="text-xs muted mb-2">按类型分布</p>
            <div v-for="row in categoryRows" :key="row.key" class="overview__category">
              <span class="text-xs">{{ row.key }}</span>
              <AppProgress :value="row.ratio * 100" :height="5" />
              <span class="text-xs muted">{{ formatBytes(row.value) }}</span>
            </div>
          </div>
        </template>
      </div>
    </section>
  </div>
</template>

<style scoped>
.overview {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.overview__features {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
  margin-top: var(--space-4);
  padding-top: var(--space-4);
  border-top: 1px solid var(--border-subtle);
}

.overview__checks {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.overview__checks li {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  color: var(--accent-text);
}

.overview__categories {
  margin-top: var(--space-5);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.overview__category {
  display: grid;
  grid-template-columns: 96px 1fr 90px;
  align-items: center;
  gap: var(--space-3);
}

.ml-2 {
  margin-left: var(--space-2);
}
</style>
