<script setup lang="ts">
/** 左侧导航：主导航、管理后台分区与容量概览。 */
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import AppIcon from '@/components/ui/AppIcon.vue'
import AppProgress from '@/components/ui/AppProgress.vue'
import { usersApi } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useSystemStore } from '@/stores/system'
import { useUploadStore } from '@/stores/upload'
import { formatBytes, toInt } from '@/utils/format'

const emit = defineEmits<{ (e: 'navigate'): void }>()

const route = useRoute()
const auth = useAuthStore()
const system = useSystemStore()
const uploads = useUploadStore()

interface NavItem {
  name: string
  label: string
  icon: string
  badge?: number
}

const mainNav = computed<NavItem[]>(() => {
  // 访客只有浏览与下载，留给他看的菜单就只剩能用的那几个，避免整页空状态。
  const items: NavItem[] = [
    { name: 'files', label: '我的文件', icon: 'folder' },
    { name: 'search', label: '搜索', icon: 'search' },
  ]
  if (auth.has('PERMISSION_SHARE')) items.push({ name: 'shares', label: '我的分享', icon: 'share' })
  if (auth.has('PERMISSION_TRASH_MANAGE')) items.push({ name: 'trash', label: '回收站', icon: 'trash' })
  if (auth.has('PERMISSION_UPLOAD')) {
    items.push({ name: 'uploads', label: '传输列表', icon: 'upload', badge: uploads.running.length })
  }
  if (!auth.isGuest) items.push({ name: 'account', label: '账号设置', icon: 'settings' })
  return items
})

const manageNav = computed<NavItem[]>(() => {
  const items: NavItem[] = []
  items.push({ name: 'manage-overview', label: '系统概览', icon: 'activity' })
  if (auth.canManageUsers) items.push({ name: 'manage-users', label: '账号管理', icon: 'users' })
  if (auth.canReadAudit) items.push({ name: 'manage-audit', label: '审计日志', icon: 'list-checks' })
  if (auth.canManageStorage) items.push({ name: 'manage-storage', label: '存储与维护', icon: 'database' })
  if (auth.canManageUsers) items.push({ name: 'manage-permissions', label: '角色与权限', icon: 'shield-check' })
  if (auth.canManageSystem) items.push({ name: 'manage-settings', label: '系统设置', icon: 'sliders' })
  return items
})

const usedBytes = ref(0)
const quotaBytes = ref(0)
const usageRatio = computed(() => (quotaBytes.value > 0 ? usedBytes.value / quotaBytes.value : 0))
const usageTone = computed(() => (usageRatio.value > 0.9 ? 'danger' : usageRatio.value > 0.7 ? 'warning' : 'brand'))

async function loadUsage(): Promise<void> {
  try {
    const stats = await usersApi.getUserStats('me')
    usedBytes.value = toInt(stats.usedBytes)
    quotaBytes.value = toInt(stats.quotaBytes)
  } catch {
    /* 用量是辅助信息，失败时静默。 */
  }
}

function isActive(name: string): boolean {
  return route.name === name
}

function isManageSection(): boolean {
  return route.path.startsWith('/manage') || route.path.startsWith('/@manage')
}

onMounted(async () => {
  await system.loadInfo()
  // 访客不显示容量条，省掉这个只为装饰存在的请求。
  if (!auth.isGuest) await loadUsage()
})

defineExpose({ loadUsage })
</script>

<template>
  <aside class="sidebar">
    <div class="sidebar__brand">
      <span class="sidebar__logo"><AppIcon name="cloud" :size="20" /></span>
      <span class="sidebar__brand-text">
        <strong>{{ system.name }}</strong>
        <small>{{ system.version }}</small>
      </span>
    </div>

    <nav class="sidebar__section" aria-label="主导航">
      <RouterLink
        v-for="item in mainNav"
        :key="item.name"
        class="sidebar__item"
        :class="{ 'is-active': isActive(item.name) }"
        :to="{ name: item.name }"
        @click="emit('navigate')"
      >
        <AppIcon :name="item.icon" :size="18" />
        <span class="sidebar__label">{{ item.label }}</span>
        <span v-if="item.badge" class="sidebar__badge">{{ item.badge }}</span>
      </RouterLink>
    </nav>

    <nav v-if="manageNav.length > 1" class="sidebar__section" aria-label="管理后台">
      <p class="sidebar__section-title">
        <AppIcon name="shield" :size="14" />
        管理后台
      </p>
      <RouterLink
        v-for="item in manageNav"
        :key="item.name"
        class="sidebar__item"
        :class="{ 'is-active': isActive(item.name) && isManageSection() }"
        :to="{ name: item.name }"
        @click="emit('navigate')"
      >
        <AppIcon :name="item.icon" :size="18" />
        <span class="sidebar__label">{{ item.label }}</span>
      </RouterLink>
    </nav>

    <div class="sidebar__foot">
      <div v-if="!auth.isGuest" class="sidebar__usage">
        <div class="sidebar__usage-head">
          <span>存储空间</span>
          <span class="mono">{{ formatBytes(usedBytes) }}</span>
        </div>
        <AppProgress :value="usageRatio * 100" :tone="usageTone" :height="5" />
        <p class="sidebar__usage-hint">
          {{ quotaBytes > 0 ? `共 ${formatBytes(quotaBytes)}` : '不限制容量' }}
        </p>
      </div>
      <p v-if="auth.isGuest" class="sidebar__hint">
        <AppIcon name="eye" :size="14" />
        访客模式：只读浏览，需要上传请先登录
      </p>
      <p v-if="!system.storageAvailable" class="sidebar__warn">
        <AppIcon name="cloud-off" :size="14" />
        对象存储未配置，上传与下载不可用
      </p>
    </div>
  </aside>
</template>

<style scoped>
.sidebar {
  width: var(--layout-sidebar);
  flex: none;
  background: var(--surface);
  border-right: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-3);
  position: sticky;
  top: 0;
  height: 100vh;
  overflow-y: auto;
}

.sidebar__brand {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-1) var(--space-2) var(--space-3);
}

.sidebar__logo {
  width: 34px;
  height: 34px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, var(--brand-500), var(--brand-700));
  color: #fff;
  display: flex;
  align-items: center;
  justify-content: center;
  flex: none;
  box-shadow: var(--shadow-sm);
}

.sidebar__brand-text {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.sidebar__brand-text strong {
  font-size: var(--text-md);
  color: var(--text-strong);
  letter-spacing: -0.01em;
}

.sidebar__brand-text small {
  color: var(--text-faint);
  font-size: var(--text-2xs);
}

.sidebar__section {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.sidebar__section-title {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  font-size: var(--text-2xs);
  font-weight: 600;
  color: var(--text-faint);
  text-transform: uppercase;
  letter-spacing: 0.06em;
  padding: var(--space-2) var(--space-2) var(--space-1);
}

.sidebar__item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  color: var(--text-muted);
  font-size: var(--text-sm);
  font-weight: 540;
  text-decoration: none;
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
}

.sidebar__item:hover {
  background: var(--surface-hover);
  color: var(--text);
  text-decoration: none;
}

.sidebar__item.is-active {
  background: var(--accent-soft);
  color: var(--accent-text);
}

.sidebar__label {
  flex: 1 1 auto;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.sidebar__badge {
  min-width: 20px;
  height: 20px;
  padding: 0 6px;
  border-radius: var(--radius-pill);
  background: var(--accent);
  color: #fff;
  font-size: var(--text-2xs);
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.sidebar__foot {
  margin-top: auto;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.sidebar__usage {
  background: var(--surface-2);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.sidebar__usage-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.sidebar__usage-hint {
  font-size: var(--text-2xs);
  color: var(--text-faint);
}

.sidebar__warn {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-2xs);
  color: var(--warning-600);
  background: var(--warning-50);
  border-radius: var(--radius-sm);
  padding: var(--space-2);
}

.sidebar__hint {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-2xs);
  color: var(--text-muted);
  background: var(--surface-2);
  border-radius: var(--radius-sm);
  padding: var(--space-2);
}

@media (max-width: 768px) {
  .sidebar {
    position: fixed;
    left: 0;
    top: 0;
    z-index: var(--z-drawer);
    transform: translateX(-100%);
    transition: transform var(--transition-slow);
    box-shadow: var(--shadow-xl);
  }

  :global(.shell.is-mobile-open) .sidebar {
    transform: translateX(0);
  }
}

/* 折叠态：只保留图标，给内容区让出空间。 */
:global(.shell.is-collapsed) .sidebar {
  width: 68px;
  padding: var(--space-4) var(--space-2);
  align-items: center;
}

:global(.shell.is-collapsed) .sidebar__label,
:global(.shell.is-collapsed) .sidebar__badge,
:global(.shell.is-collapsed) .sidebar__brand-text,
:global(.shell.is-collapsed) .sidebar__section-title,
:global(.shell.is-collapsed) .sidebar__usage,
:global(.shell.is-collapsed) .sidebar__warn,
:global(.shell.is-collapsed) .sidebar__hint {
  display: none;
}

:global(.shell.is-collapsed) .sidebar__item {
  justify-content: center;
  padding: var(--space-2);
}

:global(.shell.is-collapsed) .sidebar__section,
:global(.shell.is-collapsed) .sidebar__foot {
  width: 100%;
}
</style>
