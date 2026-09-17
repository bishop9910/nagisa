<script setup lang="ts">
/** 账号设置：资料与用量、修改密码、登录会话与自身权限。 */
import { computed, onMounted, ref, watch } from 'vue'

import AppAvatar from '@/components/ui/AppAvatar.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppProgress from '@/components/ui/AppProgress.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import AppTabs from '@/components/ui/AppTabs.vue'
import { authApi, errorText, usersApi } from '@/api'
import type { PermissionCatalog, Session, UserStats } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useSystemStore } from '@/stores/system'
import { useUiStore } from '@/stores/ui'
import {
  PERMISSION_META,
  ROLE_LABEL,
  ROLE_TONE,
  USER_STATUS_LABEL,
  USER_STATUS_TONE,
  hasPermission,
} from '@/utils/constants'
import { formatBytes, formatDateTime, formatDuration, formatNumber, formatPercent, toInt } from '@/utils/format'

const auth = useAuthStore()
const ui = useUiStore()
const system = useSystemStore()

const tab = ref('profile')
const stats = ref<UserStats | null>(null)
const catalog = ref<PermissionCatalog | null>(null)
const sessions = ref<Session[]>([])
const includeInactive = ref(false)
const loadingSessions = ref(false)
const statsLoading = ref(false)

const passwordForm = ref({ current: '', next: '', confirm: '' })
const passwordBusy = ref(false)
const passwordError = ref('')
const passwordDone = ref(false)

const quotaBytes = computed(() => toInt(auth.user?.quotaBytes))
const usedBytes = computed(() => toInt(auth.user?.usedBytes))
const usageRatio = computed(() => (quotaBytes.value > 0 ? Math.min(1, usedBytes.value / quotaBytes.value) : 0))

const grantedMeta = computed(() =>
  PERMISSION_META.filter((meta) => hasPermission(auth.permissionsMask, meta.name)),
)

const unGrantedMeta = computed(() =>
  PERMISSION_META.filter((meta) => !hasPermission(auth.permissionsMask, meta.name)),
)

/**
 * 超级管理员是内置账号：角色、等级与权限由部署固定，管理后台里不可被任何人管理，
 * 它的口令也来自部署（auth.admin_password / ADMIN_PASSWORD），所以账号设置里不给它改密入口。
 */
const tabs = computed(() => {
  const list = [
    { key: 'profile', label: '资料与用量' },
    { key: 'security', label: '密码' },
    { key: 'sessions', label: '登录会话', badge: sessions.value.length },
    { key: 'permissions', label: '我的权限', badge: grantedMeta.value.length },
  ]
  return auth.isSuperAdmin ? list.filter((item) => item.key !== 'security') : list
})

async function loadStats(): Promise<void> {
  statsLoading.value = true
  try {
    stats.value = await usersApi.getUserStats('me')
  } catch {
    stats.value = null
  } finally {
    statsLoading.value = false
  }
}

async function loadCatalog(): Promise<void> {
  try {
    catalog.value = await usersApi.listPermissionCatalog()
  } catch {
    catalog.value = null
  }
}

async function loadSessions(): Promise<void> {
  loadingSessions.value = true
  try {
    const result = await authApi.listSessions({ pageSize: 50, includeInactive: includeInactive.value })
    sessions.value = result.sessions ?? []
  } catch (err) {
    ui.toast.error('会话加载失败', errorText(err))
    sessions.value = []
  } finally {
    loadingSessions.value = false
  }
}

async function revoke(session: Session): Promise<void> {
  if (!session.id) return
  const ok = await ui.confirm({
    title: '吊销会话',
    message: session.current ? '这会结束当前设备的登录，需要重新登录。' : '该设备上的刷新令牌会立即失效。',
    tone: 'danger',
    confirmText: '吊销',
  })
  if (!ok) return
  try {
    await authApi.revokeSession(session.id)
    ui.toast.success('会话已吊销')
    if (session.current) {
      await auth.logout()
      window.location.href = '/login'
      return
    }
    await loadSessions()
  } catch (err) {
    ui.toast.error('吊销失败', errorText(err))
  }
}

async function changePassword(): Promise<void> {
  passwordError.value = ''
  passwordDone.value = false
  if (!passwordForm.value.current || !passwordForm.value.next) {
    passwordError.value = '请填写当前密码与新密码'
    return
  }
  if ([...passwordForm.value.next].length < system.minPasswordLength) {
    passwordError.value = `新密码至少 ${system.minPasswordLength} 位`
    return
  }
  if (passwordForm.value.next !== passwordForm.value.confirm) {
    passwordError.value = '两次输入的新密码不一致'
    return
  }
  passwordBusy.value = true
  try {
    await authApi.changePassword(passwordForm.value.current, passwordForm.value.next)
    passwordForm.value = { current: '', next: '', confirm: '' }
    passwordDone.value = true
    ui.toast.success('密码已修改', '其它设备的会话已被吊销')
    await loadSessions()
  } catch (err) {
    passwordError.value = errorText(err)
  } finally {
    passwordBusy.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadStats(), loadCatalog()])
  await loadSessions()
})

// 身份可能在挂载之后才拿到（令牌恢复期间），切到超级管理员时把改密页收回来。
watch(
  () => auth.isSuperAdmin,
  (isAdmin) => {
    if (isAdmin && tab.value === 'security') tab.value = 'profile'
  },
)
</script>

<template>
  <div class="page">
    <header class="page__header">
      <div>
        <h1 class="page__title">账号设置</h1>
        <p class="page__desc">查看自己的资料与用量、修改密码、管理登录会话与权限。</p>
      </div>
      <div class="page__actions">
        <AppButton icon="refresh" variant="ghost" label="刷新" @click="loadStats(); loadSessions()" />
      </div>
    </header>

    <div class="account">
      <aside class="card account__card">
        <div class="account__identity">
          <AppAvatar :name="auth.displayName" :src="auth.user?.avatarUrl" :size="56" />
          <div class="account__identity-text">
            <p class="account__name">{{ auth.displayName }}</p>
            <p class="account__username mono">@{{ auth.user?.username }}</p>
          </div>
        </div>
        <div class="account__badges">
          <AppBadge :tone="ROLE_TONE[auth.role]">{{ ROLE_LABEL[auth.role] }}</AppBadge>
          <AppBadge :tone="USER_STATUS_TONE[auth.user?.status ?? 'USER_STATUS_UNSPECIFIED']">
            {{ USER_STATUS_LABEL[auth.user?.status ?? 'USER_STATUS_UNSPECIFIED'] }}
          </AppBadge>
          <AppBadge tone="neutral">等级 {{ auth.user?.rank ?? '—' }}</AppBadge>
        </div>

        <div class="account__usage">
          <div class="account__usage-head">
            <span class="text-xs muted">已用空间</span>
            <span class="text-xs strong">{{ formatPercent(quotaBytes > 0 ? usageRatio : 0) }}</span>
          </div>
          <AppProgress :value="usageRatio * 100" :tone="usageRatio > 0.9 ? 'danger' : usageRatio > 0.7 ? 'warning' : 'brand'" />
          <p class="text-xs muted mt-2">
            {{ formatBytes(usedBytes) }} / {{ quotaBytes > 0 ? formatBytes(quotaBytes) : '不限制' }}
          </p>
        </div>

        <dl class="account__kv">
          <div>
            <dt>文件</dt>
            <dd>{{ formatNumber(auth.user?.fileCount) }}</dd>
          </div>
          <div>
            <dt>文件夹</dt>
            <dd>{{ formatNumber(auth.user?.folderCount) }}</dd>
          </div>
          <div>
            <dt>回收站</dt>
            <dd>{{ formatNumber(stats?.trashedCount) }} · {{ formatBytes(stats?.trashedBytes) }}</dd>
          </div>
          <div>
            <dt>最近登录</dt>
            <dd>{{ formatDateTime(auth.user?.lastLoginAt) }}</dd>
          </div>
          <div>
            <dt>创建时间</dt>
            <dd>{{ formatDateTime(auth.user?.createdAt) }}</dd>
          </div>
        </dl>

        <p class="account__notice">
          <AppIcon name="info" :size="14" />
          自助修改资料目前没有对应接口：账号信息需要管理员在「管理后台 → 账号管理」中维护。
        </p>

        <p v-if="auth.isSuperAdmin" class="account__notice">
          <AppIcon name="shield" :size="14" />
          超级管理员是内置账号：角色、等级与权限由部署固定，管理后台里不可被任何人管理，所以这里不提供改密入口。
          初始口令来自部署配置 auth.admin_password（留空则在首次启动日志里打印一次）。
        </p>
      </aside>

      <section class="card account__panel">
        <header class="card__header">
          <AppTabs v-model="tab" :tabs="tabs" />
        </header>

        <div class="account__panel-body">
          <template v-if="tab === 'profile'">
            <div class="stat-grid">
              <div class="stat">
                <span class="stat__label">账号 ID</span>
                <span class="stat__value mono text-sm break-all">{{ auth.user?.id }}</span>
              </div>
              <div class="stat">
                <span class="stat__label">邮箱</span>
                <span class="stat__value text-sm">{{ auth.user?.email || '未填写' }}</span>
              </div>
              <div class="stat">
                <span class="stat__label">昵称</span>
                <span class="stat__value text-sm">{{ auth.user?.nickname || '—' }}</span>
              </div>
              <div class="stat">
                <span class="stat__label">可管理等级上限</span>
                <span class="stat__value">{{ auth.user?.maxGrantableRank ?? '—' }}</span>
              </div>
              <div class="stat">
                <span class="stat__label">配额</span>
                <span class="stat__value">{{ quotaBytes > 0 ? formatBytes(quotaBytes) : '不限' }}</span>
              </div>
              <div class="stat">
                <span class="stat__label">已用</span>
                <span class="stat__value">{{ formatBytes(usedBytes) }}</span>
              </div>
            </div>

            <h3 class="account__section-title">用量明细</h3>
            <div v-if="statsLoading" class="muted text-sm">加载中…</div>
            <dl v-else-if="stats" class="account__kv account__kv--wide">
              <div>
                <dt>文件数量</dt>
                <dd>{{ formatNumber(stats.fileCount) }}</dd>
              </div>
              <div>
                <dt>文件夹数量</dt>
                <dd>{{ formatNumber(stats.folderCount) }}</dd>
              </div>
              <div>
                <dt>回收站占用</dt>
                <dd>{{ formatBytes(stats.trashedBytes) }}（{{ formatNumber(stats.trashedCount) }} 项）</dd>
              </div>
              <div>
                <dt>配额使用率</dt>
                <dd>{{ formatPercent(stats.usageRatio) }}</dd>
              </div>
            </dl>
          </template>

          <template v-else-if="tab === 'security'">
            <div class="account__form">
              <h3 class="account__section-title">修改密码</h3>
              <p class="text-xs muted">
                当前密码与新密码都会在浏览器内用服务端公钥加密后提交；修改成功后其它设备的会话会被全部吊销。
              </p>
              <label class="field">
                <span class="field__label">当前密码</span>
                <AppInput v-model="passwordForm.current" type="password" autocomplete="current-password" placeholder="当前密码" />
              </label>
              <label class="field">
                <span class="field__label">新密码</span>
                <AppInput v-model="passwordForm.next" type="password" autocomplete="new-password" :placeholder="`至少 ${system.minPasswordLength} 位`" />
              </label>
              <label class="field">
                <span class="field__label">确认新密码</span>
                <AppInput v-model="passwordForm.confirm" type="password" autocomplete="new-password" placeholder="再次输入" />
              </label>
              <p v-if="passwordError" class="field__error">{{ passwordError }}</p>
              <p v-if="passwordDone" class="account__ok">
                <AppIcon name="check-circle" :size="15" />
                密码已更新
              </p>
              <div class="flex gap-2">
                <AppButton variant="primary" :loading="passwordBusy" icon="key" @click="changePassword">修改密码</AppButton>
              </div>
            </div>
          </template>

          <template v-else-if="tab === 'sessions'">
            <div class="account__sessions-head">
              <label class="flex items-center gap-2 text-xs muted">
                <AppSwitch v-model="includeInactive" label="显示已失效会话" @update:model-value="loadSessions" />
                显示已过期 / 已吊销的会话
              </label>
              <AppButton size="sm" variant="ghost" icon="refresh" @click="loadSessions">刷新</AppButton>
            </div>

            <p v-if="loadingSessions" class="muted text-sm">加载中…</p>
            <AppEmpty
              v-else-if="sessions.length === 0"
              size="sm"
              icon="monitor"
              title="没有会话记录"
              description="登录会话会在每次刷新令牌时轮换"
            />
            <ul v-else class="account__sessions">
              <li v-for="session in sessions" :key="session.id" class="account__session">
                <div class="account__session-main">
                  <p class="account__session-title">
                    <AppIcon name="monitor" :size="15" />
                    <span>{{ session.ip || '未知地址' }}</span>
                    <AppBadge v-if="session.current" tone="brand" size="sm">当前设备</AppBadge>
                    <AppBadge :tone="session.active ? 'success' : 'neutral'" size="sm">
                      {{ session.active ? '有效' : '已失效' }}
                    </AppBadge>
                  </p>
                  <p class="account__session-meta truncate" :title="session.userAgent">{{ session.userAgent || '未知客户端' }}</p>
                  <p class="account__session-meta">
                    <span>创建 {{ formatDateTime(session.createdAt) }}</span>
                    <span>·</span>
                    <span>最近使用 {{ formatDateTime(session.lastUsedAt) }}</span>
                    <span>·</span>
                    <span>{{ formatDateTime(session.expiresAt) }} 到期</span>
                  </p>
                </div>
                <AppButton
                  size="sm"
                  variant="ghost"
                  icon="logout"
                  :disabled="!session.active"
                  @click="revoke(session)"
                >
                  吊销
                </AppButton>
              </li>
            </ul>
          </template>

          <template v-else>
            <h3 class="account__section-title">已授予的权限（{{ grantedMeta.length }} / {{ PERMISSION_META.length }}）</h3>
            <ul class="account__perms">
              <li v-for="meta in grantedMeta" :key="meta.name" class="account__perm is-granted">
                <AppIcon name="check-circle" :size="16" />
                <div>
                  <p class="account__perm-title">{{ meta.display }}</p>
                  <p class="account__perm-desc">{{ meta.description }}</p>
                </div>
              </li>
            </ul>

            <template v-if="unGrantedMeta.length > 0">
              <h3 class="account__section-title">未授予</h3>
              <ul class="account__perms">
                <li v-for="meta in unGrantedMeta" :key="meta.name" class="account__perm">
                  <AppIcon name="minus" :size="16" />
                  <div>
                    <p class="account__perm-title">{{ meta.display }}</p>
                    <p class="account__perm-desc">{{ meta.description }}</p>
                  </div>
                </li>
              </ul>
            </template>

            <p v-if="catalog?.grantedMask" class="text-xs faint mt-4">
              权限位掩码：{{ toInt(catalog.grantedMask) }}
            </p>
          </template>
        </div>
      </section>
    </div>
  </div>
</template>

<style scoped>
.account {
  display: grid;
  grid-template-columns: 320px 1fr;
  gap: var(--space-4);
  align-items: start;
}

.account__card {
  padding: var(--space-5);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.account__identity {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.account__identity-text {
  min-width: 0;
}

.account__name {
  font-size: var(--text-lg);
  font-weight: 640;
  color: var(--text-strong);
}

.account__username {
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.account__badges {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.account__usage {
  background: var(--surface-2);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  padding: var(--space-3);
}

.account__usage-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-2);
}

.account__kv {
  display: flex;
  flex-direction: column;
  margin: 0;
  font-size: var(--text-xs);
}

.account__kv > div {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-2) 0;
  border-bottom: 1px dashed var(--border-subtle);
}

.account__kv > div:last-child {
  border-bottom: 0;
}

.account__kv dt {
  color: var(--text-muted);
}

.account__kv dd {
  margin: 0;
  color: var(--text);
  text-align: right;
}

.account__kv--wide > div {
  padding: var(--space-3) 0;
}

.account__notice {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  font-size: var(--text-2xs);
  color: var(--text-muted);
  background: var(--surface-2);
  border-radius: var(--radius-sm);
  padding: var(--space-2) var(--space-3);
}

.account__panel-body {
  padding: var(--space-5);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.account__section-title {
  font-size: var(--text-md);
  font-weight: 620;
}

.account__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  max-width: 420px;
}

.account__ok {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--success-600);
  font-size: var(--text-sm);
}

.account__sessions-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.account__sessions {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.account__session {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: var(--surface-2);
}

.account__session-main {
  flex: 1 1 auto;
  min-width: 0;
}

.account__session-title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-strong);
}

.account__session-meta {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-wrap: wrap;
  font-size: var(--text-2xs);
  color: var(--text-muted);
  margin-top: 2px;
}

.account__perms {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
  gap: var(--space-2);
}

.account__perm {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-3);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  color: var(--text-faint);
}

.account__perm.is-granted {
  color: var(--accent-text);
  border-color: var(--border);
  background: var(--surface-2);
}

.account__perm-title {
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text);
}

.account__perm-desc {
  font-size: var(--text-2xs);
  color: var(--text-muted);
  margin-top: 2px;
}

@media (max-width: 1024px) {
  .account {
    grid-template-columns: 1fr;
  }
}
</style>
