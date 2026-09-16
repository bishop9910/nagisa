<script setup lang="ts">
/** 管理后台外壳：二级导航 + 子路由。 */
import { computed } from 'vue'
import { useRoute } from 'vue-router'

import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'

const route = useRoute()
const auth = useAuthStore()

interface ManageTab {
  name: string
  label: string
  icon: string
  visible: boolean
}

const tabs = computed<ManageTab[]>(() => [
  { name: 'manage-overview', label: '概览', icon: 'activity', visible: true },
  { name: 'manage-users', label: '账号', icon: 'users', visible: auth.canManageUsers },
  { name: 'manage-audit', label: '审计', icon: 'list-checks', visible: auth.canReadAudit },
  { name: 'manage-storage', label: '存储与维护', icon: 'database', visible: auth.canManageStorage },
  { name: 'manage-permissions', label: '角色与权限', icon: 'shield-check', visible: auth.canManageUsers },
  { name: 'manage-settings', label: '系统设置', icon: 'sliders', visible: auth.canManageSystem },
])

const visibleTabs = computed(() => tabs.value.filter((tab) => tab.visible))
</script>

<template>
  <div class="page manage">
    <header class="page__header">
      <div>
        <h1 class="page__title">管理后台</h1>
        <p class="page__desc">
          账号、审计、存储与运行参数的集中入口。可管理的范围由调用方的权限与等级决定：
          只能管理等级严格低于自己的账号，且只能授予自己持有的权限。
        </p>
      </div>
    </header>

    <nav class="manage__nav" aria-label="管理后台导航">
      <RouterLink
        v-for="tab in visibleTabs"
        :key="tab.name"
        class="manage__tab"
        :class="{ 'is-active': route.name === tab.name }"
        :to="{ name: tab.name }"
      >
        <AppIcon :name="tab.icon" :size="16" />
        {{ tab.label }}
      </RouterLink>
    </nav>

    <RouterView v-slot="{ Component }">
      <Transition name="route" mode="out-in">
        <component :is="Component" />
      </Transition>
    </RouterView>
  </div>
</template>

<style scoped>
.manage__nav {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-1);
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-lg);
  overflow-x: auto;
}

.manage__tab {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-md);
  color: var(--text-muted);
  font-size: var(--text-sm);
  font-weight: 560;
  text-decoration: none;
  white-space: nowrap;
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
}

.manage__tab:hover {
  background: var(--surface-hover);
  color: var(--text);
  text-decoration: none;
}

.manage__tab.is-active {
  background: var(--accent-soft);
  color: var(--accent-text);
}
</style>
