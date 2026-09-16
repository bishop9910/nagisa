<script setup lang="ts">
/**
 * 系统设置：运行参数的读写。
 * 注意这些参数当前只被读写与展示，实际生效的上限来自 configs/config.yaml；
 * system.version 是只读键，写入会被服务端拒绝。
 */
import { computed, onMounted, ref } from 'vue'

import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import { errorText, systemApi } from '@/api'
import type { SystemSetting } from '@/api/types'
import { useUiStore } from '@/stores/ui'
import { useAuthStore } from '@/stores/auth'

const ui = useUiStore()
const auth = useAuthStore()

interface SettingRow extends SystemSetting {
  original: string
}

const rows = ref<SettingRow[]>([])
const loading = ref(false)
const saving = ref(false)
const error = ref('')

const canWrite = computed(() => auth.has('PERMISSION_SYSTEM_MANAGE'))
const dirty = computed(() => rows.value.filter((row) => (row.value ?? '') !== row.original))

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const result = await systemApi.listSystemSettings()
    rows.value = (result.settings ?? []).map((setting) => ({ ...setting, original: setting.value ?? '' }))
  } catch (err) {
    error.value = errorText(err)
    rows.value = []
  } finally {
    loading.value = false
  }
}

function boolValue(row: SettingRow): boolean {
  return (row.value ?? '').toLowerCase() === 'true'
}

function setBool(row: SettingRow, value: boolean): void {
  row.value = value ? 'true' : 'false'
}

async function save(): Promise<void> {
  const changed = dirty.value
  if (changed.length === 0) {
    ui.toast.info('没有需要保存的修改')
    return
  }
  saving.value = true
  error.value = ''
  try {
    const result = await systemApi.updateSystemSettings(
      changed.map((row) => ({ key: row.key, value: row.value, type: row.type, description: row.description, writable: row.writable })),
    )
    rows.value = (result.settings ?? []).map((setting) => ({ ...setting, original: setting.value ?? '' }))
    ui.toast.success('设置已保存', `共 ${changed.length} 项`)
  } catch (err) {
    error.value = errorText(err)
  } finally {
    saving.value = false
  }
}

function reset(): void {
  rows.value = rows.value.map((row) => ({ ...row, value: row.original }))
}

function typeTone(type?: string): 'neutral' | 'brand' | 'info' | 'warning' {
  if (type === 'int') return 'info'
  if (type === 'bool') return 'brand'
  if (type === 'json') return 'warning'
  return 'neutral'
}

/** json 型参数的输入提示。 */
function placeholderFor(row: SettingRow): string {
  if (row.type === 'json') return '{"key": "value"}'
  if (row.type === 'int') return '整数'
  return ''
}

onMounted(() => void load())
</script>

<template>
  <div class="settings">
    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">运行参数</p>
          <p class="card__subtitle">
            共 {{ rows.length }} 项 · 已修改 {{ dirty.length }} 项 ·
            {{ canWrite ? '你有权限写入' : '当前账号只有读取权限' }}
          </p>
        </div>
        <div class="flex gap-2">
          <AppButton size="sm" variant="ghost" icon="refresh" :loading="loading" @click="load">重新加载</AppButton>
          <AppButton size="sm" variant="ghost" icon="restore" :disabled="dirty.length === 0" @click="reset">撤销修改</AppButton>
          <AppButton
            size="sm"
            variant="primary"
            icon="check"
            :loading="saving"
            :disabled="!canWrite || dirty.length === 0"
            @click="save"
          >
            保存
          </AppButton>
        </div>
      </header>

      <div class="settings__notice">
        <AppIcon name="info" :size="15" />
        这些参数目前只被读写与展示，实际生效的策略与上限来自服务端配置文件；改动后请以重启日志为准。
      </div>

      <p v-if="loading" class="settings__state muted text-sm">加载中…</p>
      <AppEmpty
        v-else-if="rows.length === 0"
        :icon="error ? 'alert-circle' : 'sliders'"
        :title="error ? '加载失败' : '没有可配置项'"
        :description="error"
      >
        <AppButton v-if="error" size="sm" icon="refresh" @click="load">重试</AppButton>
      </AppEmpty>

      <ul v-else class="settings__list">
        <li v-for="row in rows" :key="row.key" class="settings__row">
          <div class="settings__main">
            <p class="settings__key">
              <span class="mono">{{ row.key }}</span>
              <AppBadge :tone="typeTone(row.type)" size="sm">{{ row.type }}</AppBadge>
              <AppBadge v-if="row.writable === false" tone="warning" size="sm" icon="lock">只读</AppBadge>
              <AppBadge v-if="(row.value ?? '') !== row.original" tone="brand" size="sm">已修改</AppBadge>
            </p>
            <p class="settings__desc">{{ row.description || '没有说明' }}</p>
          </div>
          <div class="settings__control">
            <AppSwitch
              v-if="row.type === 'bool'"
              :model-value="boolValue(row)"
              :disabled="!canWrite || row.writable === false"
              :label="row.key"
              @update:model-value="setBool(row, $event)"
            />
            <AppInput
              v-else
              v-model="row.value"
              size="sm"
              :type="row.type === 'int' ? 'number' : 'text'"
              :disabled="!canWrite || row.writable === false"
              :placeholder="placeholderFor(row)"
            />
          </div>
        </li>
      </ul>

      <p v-if="error && rows.length > 0" class="field__error settings__error">{{ error }}</p>
    </section>
  </div>
</template>

<style scoped>
.settings__notice {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0 var(--space-5);
  padding: var(--space-3);
  background: var(--warning-50);
  color: var(--warning-600);
  border-radius: var(--radius-md);
  font-size: var(--text-xs);
}

.settings__state {
  padding: var(--space-6);
  text-align: center;
}

.settings__list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.settings__row {
  display: flex;
  align-items: center;
  gap: var(--space-5);
  padding: var(--space-4) var(--space-5);
  border-bottom: 1px solid var(--border-subtle);
}

.settings__row:last-child {
  border-bottom: 0;
}

.settings__main {
  flex: 1 1 auto;
  min-width: 0;
}

.settings__key {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-strong);
}

.settings__desc {
  font-size: var(--text-xs);
  color: var(--text-muted);
  margin-top: 3px;
}

.settings__control {
  width: 260px;
  flex: none;
  display: flex;
  justify-content: flex-end;
}

.settings__error {
  padding: 0 var(--space-5) var(--space-4);
}

@media (max-width: 768px) {
  .settings__row {
    flex-direction: column;
    align-items: flex-start;
    gap: var(--space-2);
  }

  .settings__control {
    width: 100%;
    justify-content: flex-start;
  }
}
</style>
