<script setup lang="ts">
/**
 * 角色与权限：四个内置角色预设、完整权限目录与授权规则说明。
 * 数据来自 GET /v1/users/roles/list 与 GET /v1/users/permissions/catalog。
 */
import { computed, onMounted, ref } from 'vue'

import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import { errorText, usersApi } from '@/api'
import type { PermissionCatalog, PermissionInfo, RolePresetSet } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { ROLE_LABEL, ROLE_TONE, PERMISSION_META, maskFromPermissions } from '@/utils/constants'
import { formatNumber, toInt } from '@/utils/format'

const auth = useAuthStore()
const ui = useUiStore()

const presets = ref<RolePresetSet | null>(null)
const catalog = ref<PermissionCatalog | null>(null)
const loading = ref(false)
const error = ref('')

const contentPermissions = computed(() =>
  (catalog.value?.permissions ?? []).filter((item) => (item.category ?? 'content') === 'content'),
)
const adminPermissions = computed(() =>
  (catalog.value?.permissions ?? []).filter((item) => (item.category ?? 'content') !== 'content'),
)
const granted = computed(() => catalog.value?.granted ?? [])

function hasPermission(permission?: string): boolean {
  if (!permission) return false
  return granted.value.includes(permission as (typeof granted.value)[number])
}

/** 权限枚举名 → 中文名（以服务端目录为准，缺失时回落到本地字典）。 */
function permissionDisplay(permission?: string): string {
  if (!permission) return '—'
  const fromCatalog = (catalog.value?.permissions ?? []).find((item) => item.permission === permission)
  return fromCatalog?.displayName ?? PERMISSION_META.find((meta) => meta.name === permission)?.display ?? permission
}

/** 权限枚举名 → 位值。 */
function permissionBit(permission?: string): number {
  return PERMISSION_META.find((meta) => meta.name === permission)?.bit ?? 0
}

function presetCardRole(preset: { role?: string }): 'ROLE_ADMIN' | 'ROLE_MANAGER' | 'ROLE_USER' | 'ROLE_GUEST' {
  const role = preset.role
  if (role === 'ROLE_ADMIN' || role === 'ROLE_MANAGER' || role === 'ROLE_USER' || role === 'ROLE_GUEST') return role
  return 'ROLE_GUEST'
}

async function load(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const [presetResult, catalogResult] = await Promise.all([
      usersApi.listRolePresets(),
      usersApi.listPermissionCatalog(),
    ])
    presets.value = presetResult
    catalog.value = catalogResult
  } catch (err) {
    error.value = errorText(err)
  } finally {
    loading.value = false
  }
}

async function copyMask(): Promise<void> {
  const mask = maskFromPermissions(granted.value)
  try {
    await navigator.clipboard.writeText(String(mask))
    ui.toast.success('权限掩码已复制', String(mask))
  } catch {
    ui.toast.error('复制失败', '请手动记录：' + mask)
  }
}

onMounted(() => void load())
</script>

<template>
  <div class="perms">
    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">角色预设</p>
          <p class="card__subtitle">
            你的等级 {{ presets?.callerRank ?? auth.user?.rank }} · 可授予的最高等级
            {{ presets?.maxGrantableRank ?? '—' }}（等级必须严格低于你自己）
          </p>
        </div>
        <AppButton size="sm" variant="ghost" icon="refresh" :loading="loading" @click="load">刷新</AppButton>
      </header>
      <div class="card__body">
        <p v-if="loading" class="muted text-sm">加载中…</p>
        <AppEmpty
          v-else-if="!presets || (presets.presets ?? []).length === 0"
          size="sm"
          :icon="error ? 'alert-circle' : 'shield'"
          :title="error ? '加载失败' : '没有角色预设'"
          :description="error"
        >
          <AppButton v-if="error" size="sm" icon="refresh" @click="load">重试</AppButton>
        </AppEmpty>
        <div v-else class="perms__presets">
          <article v-for="preset in presets.presets ?? []" :key="preset.role" class="perms__preset">
            <header class="perms__preset-head">
              <AppBadge :tone="ROLE_TONE[presetCardRole(preset)]">{{ preset.displayName || ROLE_LABEL[presetCardRole(preset)] }}</AppBadge>
              <span class="mono faint">{{ preset.role }}</span>
            </header>
            <p class="perms__preset-desc">{{ preset.description }}</p>
            <dl class="perms__preset-meta">
              <div>
                <dt>默认等级</dt>
                <dd>{{ preset.defaultRank }}</dd>
              </div>
              <div>
                <dt>权限数</dt>
                <dd>{{ (preset.permissions ?? []).length }} 项</dd>
              </div>
              <div>
                <dt>掩码</dt>
                <dd class="mono">{{ toInt(preset.permissionsMask) }}</dd>
              </div>
            </dl>
            <ul class="perms__preset-list">
              <li
                v-for="permission in preset.permissions ?? []"
                :key="permission"
                :class="{ 'is-missing': !hasPermission(permission) }"
              >
                <AppIcon :name="hasPermission(permission) ? 'check' : 'minus'" :size="13" />
                {{ permissionDisplay(permission) }}
              </li>
            </ul>
          </article>
        </div>
      </div>
    </section>

    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">权限目录</p>
          <p class="card__subtitle">
            共 {{ (catalog?.permissions ?? []).length }} 项 · 你持有 {{ granted.length }} 项 · 掩码
            {{ toInt(catalog?.grantedMask) }}
          </p>
        </div>
        <AppButton size="sm" variant="ghost" icon="copy" @click="copyMask">复制掩码</AppButton>
      </header>

      <div class="perms__group">
        <p class="perms__group-title">内容类（可用于节点访问名单）</p>
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th style="width: 130px">权限</th>
                <th style="width: 110px">机器名</th>
                <th style="width: 110px">位掩码</th>
                <th>说明</th>
                <th style="width: 110px">我是否持有</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in contentPermissions" :key="item.permission">
                <td class="text-sm strong">{{ item.displayName }}</td>
                <td class="mono text-xs">{{ item.name }}</td>
                <td class="mono text-xs">{{ permissionBit(item.permission) }}</td>
                <td class="text-xs muted">{{ item.description }}</td>
                <td>
                  <AppBadge :tone="hasPermission(item.permission) ? 'success' : 'neutral'" size="sm">
                    {{ hasPermission(item.permission) ? '持有' : '未持有' }}
                  </AppBadge>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>

      <div class="perms__group">
        <p class="perms__group-title">管理类（不能写进节点名单与分享链接）</p>
        <div class="table-wrap">
          <table class="table">
            <thead>
              <tr>
                <th style="width: 130px">权限</th>
                <th style="width: 110px">机器名</th>
                <th>说明</th>
                <th style="width: 110px">我是否持有</th>
              </tr>
            </thead>
            <tbody>
              <tr v-for="item in adminPermissions" :key="item.permission">
                <td class="text-sm strong">{{ item.displayName }}</td>
                <td class="mono text-xs">{{ item.name }}</td>
                <td class="text-xs muted">{{ item.description }}</td>
                <td>
                  <AppBadge :tone="hasPermission(item.permission) ? 'success' : 'neutral'" size="sm">
                    {{ hasPermission(item.permission) ? '持有' : '未持有' }}
                  </AppBadge>
                </td>
              </tr>
            </tbody>
          </table>
        </div>
      </div>
    </section>

    <section class="card card--pad perms__rules">
      <h3 class="card__title">授权规则（服务端强制，界面按其约束渲染）</h3>
      <ol>
        <li><strong>严格等级序：</strong>只能创建、读取、修改、删除 rank 严格小于自己的账号；只能授予低于自己的等级。</li>
        <li><strong>只能授予自己拥有的权限：</strong>新建账号、改权限或改角色时提交的集合必须是自身权限的子集。</li>
        <li><strong>内置管理员不可管理：</strong>rank = 1000（{{ formatNumber(1000) }}）的账号对任何人都不可管理。</li>
        <li><strong>管理路径拒绝自我管理：</strong>改资料、改权限、重置密码、删除账号都不能作用于自己。</li>
        <li><strong>ROLE_ADMIN 例外：</strong>只有 ROLE_ADMIN 的调用方才能创建或指派超级管理员。</li>
      </ol>
    </section>
  </div>
</template>

<style scoped>
.perms {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.perms__presets {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(260px, 1fr));
  gap: var(--space-3);
}

.perms__preset {
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  padding: var(--space-4);
  background: var(--surface-2);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.perms__preset-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.perms__preset-desc {
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.perms__preset-meta {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-2);
  margin: 0;
}

.perms__preset-meta dt {
  font-size: var(--text-2xs);
  color: var(--text-faint);
}

.perms__preset-meta dd {
  margin: 2px 0 0;
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-strong);
}

.perms__preset-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 4px var(--space-3);
}

.perms__preset-list li {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: var(--text-2xs);
  color: var(--accent-text);
}

.perms__preset-list li.is-missing {
  color: var(--text-faint);
}

.perms__group {
  padding: var(--space-4) var(--space-5) var(--space-5);
}

.perms__group-title {
  font-size: var(--text-xs);
  font-weight: 600;
  color: var(--text-muted);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  margin-bottom: var(--space-2);
}

.perms__group + .perms__group {
  border-top: 1px solid var(--border-subtle);
}

.perms__rules ol {
  margin: var(--space-3) 0 0;
  padding-left: var(--space-5);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  font-size: var(--text-sm);
  color: var(--text-muted);
}

.perms__rules strong {
  color: var(--text);
}
</style>
