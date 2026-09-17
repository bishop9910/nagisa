<script setup lang="ts">
/**
 * 系统信息：只读展示服务标识、运行状态，以及服务端真正生效的策略与上限。
 *
 * 数据全部来自服务端（/v1/system/info、/v1/system/health、/v1/system/settings/list），
 * 页面不写回任何值：可以在网页里改的配置项目前没有，能改的那几项来自服务端配置文件。
 */
import { computed, onMounted, ref } from 'vue'

import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import { errorText, systemApi } from '@/api'
import type { HealthStatus, SystemSetting } from '@/api/types'
import { useSystemStore } from '@/stores/system'
import { formatBytes, formatDateTime, formatDuration, toInt } from '@/utils/format'

const system = useSystemStore()

const settings = ref<SystemSetting[]>([])
const health = ref<HealthStatus | null>(null)
const loading = ref(false)
const probing = ref(false)
const error = ref('')

const auth = computed(() => system.info?.auth ?? {})

/** 上传模式是枚举，直接展示枚举名没有意义，翻成人话。 */
const uploadModeLabel = computed(() =>
  system.uploadModes.map((mode) => (mode === 'UPLOAD_MODE_PROXY' ? '代理上传' : '预签名上传')).join(' / '),
)

const healthTone = computed(() => {
  const status = health.value?.status
  if (status === 'ok') return 'success'
  if (status === 'degraded') return 'warning'
  return 'danger'
})

const healthLabel = computed(() => {
  const status = health.value?.status
  if (status === 'ok') return '正常'
  if (status === 'degraded') return '降级'
  if (status === 'down') return '不可用'
  return '未知'
})

const checks = computed(() => Object.entries(health.value?.checks ?? {}))

/** 服务端没下发的时长显示成「—」，而不是「0 秒」。 */
function duration(value: unknown): string {
  const seconds = toInt(value)
  return seconds > 0 ? formatDuration(seconds) : '—'
}

function typeTone(type?: string): 'neutral' | 'brand' | 'info' | 'warning' {
  if (type === 'int') return 'info'
  if (type === 'bool') return 'brand'
  if (type === 'json') return 'warning'
  return 'neutral'
}

async function loadHealth(): Promise<void> {
  try {
    health.value = await systemApi.healthCheck(false)
  } catch (err) {
    health.value = { status: 'down', checks: { api: errorText(err) } }
  }
}

async function loadSettings(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const result = await systemApi.listSystemSettings()
    settings.value = result.settings ?? []
  } catch (err) {
    error.value = errorText(err)
    settings.value = []
  } finally {
    loading.value = false
  }
}

async function refresh(): Promise<void> {
  probing.value = true
  try {
    await Promise.all([system.loadInfo(true), loadHealth(), loadSettings()])
  } finally {
    probing.value = false
  }
}

onMounted(() => void refresh())
</script>

<template>
  <div class="system-info">
    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">服务与运行状态</p>
          <p class="card__subtitle">来自 GET /v1/system/info 与 GET /v1/system/health</p>
        </div>
        <AppButton size="sm" variant="ghost" icon="refresh" :loading="probing" @click="refresh">重新读取</AppButton>
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
            <span class="stat__label">已运行</span>
            <span class="stat__value text-lg">{{ duration(health?.uptimeSeconds) }}</span>
            <span class="stat__hint">上次启动至今</span>
          </div>
          <div class="stat">
            <span class="stat__label">健康状态</span>
            <span class="stat__value text-lg">
              <AppBadge :tone="healthTone">{{ healthLabel }}</AppBadge>
            </span>
            <span class="stat__hint">{{ checks.length }} 项依赖检查</span>
          </div>
          <div class="stat">
            <span class="stat__label">服务端时间</span>
            <span class="stat__value text-lg">{{ formatDateTime(system.info?.serverTime) }}</span>
            <span class="stat__hint">与本机时间的偏差即部署时钟</span>
          </div>
        </div>

        <div v-if="checks.length" class="system-info__checks">
          <ul>
            <li v-for="[key, value] in checks" :key="key">
              <AppIcon :name="value === 'ok' ? 'check-circle' : 'alert-triangle'" :size="15" />
              <span class="strong">{{ key }}</span>
              <span class="text-xs muted truncate" :title="value">{{ value }}</span>
            </li>
          </ul>
        </div>
      </div>
    </section>

    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">生效的策略与上限</p>
          <p class="card__subtitle">服务端当前真正执行的值，网页里改不了，改动请修改服务端配置后重启</p>
        </div>
      </header>
      <div class="card__body">
        <div v-if="system.features.length || uploadModeLabel" class="system-info__features">
          <span class="text-xs muted">服务端能力</span>
          <AppBadge v-for="feature in system.features" :key="feature" tone="brand">{{ feature }}</AppBadge>
          <AppBadge v-if="uploadModeLabel" tone="info">{{ uploadModeLabel }}</AppBadge>
        </div>

        <div class="system-info__grid">
          <div class="system-info__panel">
            <p class="system-info__panel-title"><AppIcon name="upload" :size="14" />传输与上传</p>
            <dl class="kv">
              <div>
                <dt>最大单文件</dt>
                <dd>{{ system.maxUploadSize > 0 ? formatBytes(system.maxUploadSize) : '不限制' }}</dd>
              </div>
              <div>
                <dt>默认分片大小</dt>
                <dd class="mono">{{ formatBytes(system.defaultChunkSize) }}</dd>
              </div>
              <div>
                <dt>最小分片大小</dt>
                <dd class="mono">{{ formatBytes(system.minChunkSize) }}</dd>
              </div>
              <div>
                <dt>内联上传上限</dt>
                <dd class="mono">{{ formatBytes(system.maxInlineSize) }}</dd>
              </div>
              <div>
                <dt>上传会话有效期</dt>
                <dd>{{ duration(system.uploadSessionTtl) }}</dd>
              </div>
              <div>
                <dt>签名地址有效期</dt>
                <dd>{{ duration(system.signedUrlTtl) }}</dd>
              </div>
              <div>
                <dt>签名地址最长有效期</dt>
                <dd>{{ duration(system.info?.signedUrlMaxTtlSeconds) }}</dd>
              </div>
            </dl>
          </div>

          <div class="system-info__panel">
            <p class="system-info__panel-title"><AppIcon name="key" :size="14" />口令与会话</p>
            <dl class="kv">
              <div>
                <dt>口令最小长度</dt>
                <dd>{{ system.minPasswordLength }} 位</dd>
              </div>
              <div>
                <dt>口令传输编码</dt>
                <dd class="mono">{{ auth.passwordEncoding || '—' }}</dd>
              </div>
              <div>
                <dt>允许明文口令</dt>
                <dd>
                  <AppBadge :tone="auth.plainPasswordAllowed ? 'warning' : 'success'" size="sm">
                    {{ auth.plainPasswordAllowed ? '允许（不推荐）' : '不允许' }}
                  </AppBadge>
                </dd>
              </div>
              <div>
                <dt>口令公钥标识</dt>
                <dd class="mono truncate" :title="auth.passwordKeyId">{{ auth.passwordKeyId || '—' }}</dd>
              </div>
              <div>
                <dt>访问令牌有效期</dt>
                <dd>{{ duration(auth.accessTokenTtlSeconds) }}</dd>
              </div>
              <div>
                <dt>刷新令牌有效期</dt>
                <dd>{{ duration(auth.refreshTokenTtlSeconds) }}</dd>
              </div>
              <div>
                <dt>免登录访客</dt>
                <dd>
                  <AppBadge :tone="auth.guestLoginEnabled ? 'success' : 'neutral'" size="sm">
                    {{ auth.guestLoginEnabled ? '已开启' : '已关闭' }}
                  </AppBadge>
                </dd>
              </div>
            </dl>
          </div>

          <div class="system-info__panel">
            <p class="system-info__panel-title"><AppIcon name="database" :size="14" />存储与后端</p>
            <dl class="kv">
              <div>
                <dt>对象存储后端</dt>
                <dd>
                  <span class="mono">{{ system.storageBackend }}</span>
                  <AppBadge :tone="system.storageAvailable ? 'success' : 'danger'" size="sm">
                    {{ system.storageAvailable ? '可用' : '未配置' }}
                  </AppBadge>
                </dd>
              </div>
              <div>
                <dt>数据库后端</dt>
                <dd class="mono">{{ system.databaseBackend }}</dd>
              </div>
              <div>
                <dt>默认可见范围</dt>
                <dd class="mono">{{ system.info?.defaultVisibility || '—' }}</dd>
              </div>
              <div>
                <dt>开放注册</dt>
                <dd>
                  <AppBadge :tone="system.info?.registrationEnabled ? 'success' : 'neutral'" size="sm">
                    {{ system.info?.registrationEnabled ? '已开启' : '已关闭' }}
                  </AppBadge>
                </dd>
              </div>
              <div>
                <dt>对外基地址</dt>
                <dd class="break-all">{{ system.publicBaseUrl || '（相对路径）' }}</dd>
              </div>
            </dl>
          </div>
        </div>
      </div>
    </section>

    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">服务端登记的运行参数</p>
          <p class="card__subtitle">
            共 {{ settings.length }} 项 · 来自 GET /v1/system/settings/list · 只读展示
          </p>
        </div>
        <AppButton size="sm" variant="ghost" icon="refresh" :loading="loading" @click="loadSettings">重新加载</AppButton>
      </header>

      <p v-if="loading && settings.length === 0" class="system-info__state muted text-sm">加载中…</p>
      <AppEmpty
        v-else-if="settings.length === 0"
        size="sm"
        :icon="error ? 'alert-circle' : 'sliders'"
        :title="error ? '读取失败' : '服务端没有登记任何参数'"
        :description="error"
      >
        <AppButton v-if="error" size="sm" icon="refresh" @click="loadSettings">重试</AppButton>
      </AppEmpty>

      <template v-else>
        <ul class="params">
          <li v-for="row in settings" :key="row.key" class="params__row">
            <div class="params__main">
              <p class="params__key">
                <span class="mono">{{ row.key }}</span>
                <AppBadge :tone="typeTone(row.type)" size="sm">{{ row.type }}</AppBadge>
                <AppBadge v-if="row.writable === false" tone="warning" size="sm" icon="lock">只读</AppBadge>
              </p>
              <p class="params__desc">{{ row.description || '没有说明' }}</p>
            </div>
            <span class="params__value mono break-all">{{ row.value ?? '—' }}</span>
          </li>
        </ul>

        <p class="system-info__notice">
          <AppIcon name="info" :size="14" />
          这些条目是服务端登记的策略快照。真正生效的策略与上限以服务端配置文件（configs/config.yaml）
          与启动参数为准，改动后重启生效。
        </p>
      </template>
    </section>
  </div>
</template>

<style scoped>
.system-info {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.system-info__checks {
  margin-top: var(--space-4);
  padding-top: var(--space-4);
  border-top: 1px solid var(--border-subtle);
}

.system-info__checks ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(220px, 1fr));
  gap: var(--space-2) var(--space-4);
}

.system-info__checks li {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  color: var(--text-muted);
  min-width: 0;
}

.system-info__features {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
  padding-bottom: var(--space-4);
  margin-bottom: var(--space-4);
  border-bottom: 1px solid var(--border-subtle);
}

.system-info__grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
  gap: var(--space-4);
}

.system-info__panel {
  background: var(--surface-2);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  padding: var(--space-4);
}

.system-info__panel-title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  font-weight: 620;
  color: var(--text-strong);
  margin-bottom: var(--space-2);
}

.kv {
  display: flex;
  flex-direction: column;
  margin: 0;
  font-size: var(--text-xs);
}

.kv > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-2) 0;
  border-bottom: 1px dashed var(--border-subtle);
}

.kv > div:last-child {
  border-bottom: 0;
  padding-bottom: 0;
}

.kv dt {
  color: var(--text-muted);
  flex: none;
}

.kv dd {
  margin: 0;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--text);
  text-align: right;
}

.system-info__state {
  padding: var(--space-6);
  text-align: center;
}

.params {
  list-style: none;
  margin: 0;
  padding: 0;
}

.params__row {
  display: flex;
  align-items: center;
  gap: var(--space-5);
  padding: var(--space-4) var(--space-5);
  border-bottom: 1px solid var(--border-subtle);
}

.params__row:last-child {
  border-bottom: 0;
}

.params__main {
  flex: 1 1 auto;
  min-width: 0;
}

.params__key {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-strong);
}

.params__desc {
  font-size: var(--text-xs);
  color: var(--text-muted);
  margin-top: 3px;
}

.params__value {
  flex: none;
  max-width: 46%;
  text-align: right;
  color: var(--text);
  background: var(--surface-2);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-sm);
  padding: 3px var(--space-2);
}

.system-info__notice {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  margin: 0 var(--space-5) var(--space-5);
  padding: var(--space-3);
  background: var(--surface-2);
  border-radius: var(--radius-md);
  font-size: var(--text-xs);
  color: var(--text-muted);
}

@media (max-width: 768px) {
  .params__row {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2);
  }

  .params__value {
    max-width: 100%;
    text-align: left;
  }
}
</style>
