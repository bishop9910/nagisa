<script setup lang="ts">
/** 顶部操作栏：全局搜索、视图折叠、上传入口、主题切换与账号菜单。 */
import { computed, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AppAvatar from '@/components/ui/AppAvatar.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppDropdown from '@/components/ui/AppDropdown.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppTooltip from '@/components/ui/AppTooltip.vue'
import type { DropdownItem } from '@/components/ui/types'
import { useAuthStore } from '@/stores/auth'
import { useFilesStore } from '@/stores/files'
import { useUiStore } from '@/stores/ui'
import { useUploadStore } from '@/stores/upload'
import { ROLE_LABEL } from '@/utils/constants'

defineProps<{ collapsed: boolean }>()
const emit = defineEmits<{ (e: 'toggle-collapse'): void; (e: 'toggle-nav'): void }>()

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const files = useFilesStore()
const ui = useUiStore()
const uploads = useUploadStore()

const keyword = ref('')
const fileInput = ref<HTMLInputElement | null>(null)

const roleLabel = computed(() => (auth.isGuest ? '访客 · 只读' : (ROLE_LABEL[auth.role] ?? '账号')))
const targetLabel = computed(() => files.folder?.name || '我的网盘')
const canUploadHere = computed(() => auth.has('PERMISSION_UPLOAD'))

const userMenuItems = computed<DropdownItem[]>(() => {
  // 访客只有一个动作：换成正式账号登录。上传、分享、管理后台这些入口都由权限位
  // 自己挡住，所以这里不用再逐个判断。
  if (auth.isGuest) {
    return [{ key: 'login', label: '登录账号', icon: 'user' }]
  }
  const items: DropdownItem[] = [
    { key: 'account', label: '账号设置', icon: 'user' },
    {
      key: 'uploads',
      label: '传输列表',
      icon: 'upload',
      hint: uploads.running.length ? `${uploads.running.length} 进行中` : '',
    },
  ]
  if (auth.canEnterManage) items.push({ key: 'manage', label: '管理后台', icon: 'shield' })
  items.push({ key: 'divider', divider: true })
  items.push({ key: 'logout', label: '退出登录', icon: 'logout' })
  if (auth.refreshToken) items.push({ key: 'revoke', label: '退出全部设备', icon: 'x-circle', danger: true })
  return items
})

function submitSearch(): void {
  const query = keyword.value.trim()
  if (!query) return
  void router.push({ name: 'search', query: { q: query } })
}

function pickFiles(): void {
  fileInput.value?.click()
}

function onFilesPicked(event: Event): void {
  const input = event.target as HTMLInputElement
  const picked = Array.from(input.files ?? [])
  if (picked.length > 0) {
    uploads.enqueue(picked, { parentId: files.folderId, parentLabel: targetLabel.value })
  }
  input.value = ''
}

async function onUserMenu(key: string): Promise<void> {
  switch (key) {
    case 'login':
      void router.push({ name: 'login' })
      break
    case 'account':
      void router.push({ name: 'account' })
      break
    case 'uploads':
      void router.push({ name: 'uploads' })
      break
    case 'manage':
      void router.push({ name: 'manage-overview' })
      break
    case 'logout':
      if (await ui.confirm({ title: '退出登录', message: '确认退出当前账号？', confirmText: '退出' })) {
        await auth.logout()
        void router.replace({ name: 'login' })
      }
      break
    case 'revoke':
      if (
        await ui.confirm({
          title: '退出全部设备',
          message: '所有设备上的登录会话都会被吊销，需要重新登录。',
          tone: 'danger',
          confirmText: '全部退出',
        })
      ) {
        await auth.logout({ revokeAll: true })
        void router.replace({ name: 'login' })
      }
      break
    default:
      break
  }
}
</script>

<template>
  <header class="topbar">
    <button type="button" class="topbar__icon-btn topbar__nav" aria-label="打开导航" @click="emit('toggle-nav')">
      <AppIcon name="menu" :size="18" />
    </button>
    <AppTooltip :content="collapsed ? '展开侧栏' : '收起侧栏'">
      <button
        type="button"
        class="topbar__icon-btn hide-sm"
        :aria-label="collapsed ? '展开侧栏' : '收起侧栏'"
        @click="emit('toggle-collapse')"
      >
        <AppIcon name="panel-left" :size="18" />
      </button>
    </AppTooltip>

    <form class="topbar__search" role="search" @submit.prevent="submitSearch">
      <AppIcon name="search" :size="16" />
      <input
        v-model="keyword"
        class="topbar__search-input"
        type="search"
        placeholder="搜索文件与文件夹…"
        aria-label="全局搜索"
      />
      <button v-if="keyword" type="button" class="topbar__search-clear" aria-label="清空搜索" @click="keyword = ''">
        <AppIcon name="x" :size="14" />
      </button>
    </form>

    <div class="topbar__actions">
      <input ref="fileInput" type="file" multiple class="hidden" @change="onFilesPicked" />
      <AppButton
        v-if="canUploadHere"
        variant="primary"
        size="md"
        icon="upload"
        class="hide-sm"
        @click="pickFiles"
      >
        上传
      </AppButton>
      <AppTooltip :content="ui.isDark ? '切换到浅色' : '切换到深色'">
        <button
          type="button"
          class="topbar__icon-btn"
          :aria-label="ui.isDark ? '切换到浅色主题' : '切换到深色主题'"
          @click="ui.toggleTheme()"
        >
          <AppIcon :name="ui.isDark ? 'sun' : 'moon'" :size="18" />
        </button>
      </AppTooltip>

      <AppDropdown :items="userMenuItems" :width="216" @select="onUserMenu">
        <template #trigger>
          <button type="button" class="topbar__user" aria-label="账号菜单">
            <AppAvatar :name="auth.displayName" :src="auth.user?.avatarUrl" :size="30" />
            <span class="topbar__user-text hide-sm">
              <strong>{{ auth.displayName }}</strong>
              <small>{{ roleLabel }}</small>
            </span>
            <AppIcon name="chevron-down" :size="14" class="topbar__user-caret" />
          </button>
        </template>
      </AppDropdown>
    </div>
  </header>
</template>

<style scoped>
.topbar {
  position: sticky;
  top: 0;
  z-index: var(--z-sticky);
  height: var(--layout-topbar);
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: 0 var(--space-5);
  background: color-mix(in srgb, var(--surface) 86%, transparent);
  backdrop-filter: saturate(180%) blur(10px);
  border-bottom: 1px solid var(--border);
}

.topbar__icon-btn {
  width: 34px;
  height: 34px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  border-radius: var(--radius-md);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  transition:
    background var(--transition-fast),
    color var(--transition-fast);
}

.topbar__icon-btn:hover {
  background: var(--surface-hover);
  color: var(--text);
}

.topbar__nav {
  display: none;
}

.topbar__search {
  flex: 1 1 auto;
  max-width: 460px;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  height: 34px;
  padding: 0 var(--space-3);
  border-radius: var(--radius-pill);
  background: var(--surface-2);
  border: 1px solid var(--border-subtle);
  color: var(--text-faint);
  transition:
    border-color var(--transition-fast),
    box-shadow var(--transition-fast);
}

.topbar__search:focus-within {
  border-color: var(--accent);
  box-shadow: var(--focus-ring);
  background: var(--surface);
}

.topbar__search-input {
  flex: 1 1 auto;
  min-width: 0;
  border: 0;
  outline: none;
  background: transparent;
  font-size: var(--text-sm);
  color: var(--text);
}

.topbar__search-input::placeholder {
  color: var(--text-faint);
}

.topbar__search-clear {
  border: 0;
  background: transparent;
  color: var(--text-faint);
  cursor: pointer;
  display: inline-flex;
  padding: 2px;
  border-radius: var(--radius-xs);
}

.topbar__search-clear:hover {
  background: var(--surface-3);
  color: var(--text);
}

.topbar__actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.topbar__user {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  padding: 3px var(--space-2) 3px 3px;
  border-radius: var(--radius-pill);
  border: 1px solid var(--border-subtle);
  background: var(--surface);
  cursor: pointer;
  transition:
    border-color var(--transition-fast),
    background var(--transition-fast);
}

.topbar__user:hover {
  background: var(--surface-hover);
  border-color: var(--border-strong);
}

.topbar__user-text {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  line-height: 1.15;
  max-width: 120px;
}

.topbar__user-text strong {
  font-size: var(--text-xs);
  color: var(--text-strong);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  max-width: 120px;
}

.topbar__user-text small {
  font-size: 10px;
  color: var(--text-faint);
}

.topbar__user-caret {
  color: var(--text-faint);
}

@media (max-width: 768px) {
  .topbar {
    padding: 0 var(--space-3);
  }

  .topbar__nav {
    display: inline-flex;
  }

  .topbar__search {
    max-width: none;
  }

  .topbar__icon-btn {
    width: 40px;
    height: 40px;
  }
}
</style>
