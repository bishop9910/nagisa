<script setup lang="ts">
/**
 * 匿名分享页（/s/:token）。
 *
 * 三条公开接口都用分享令牌鉴权：AccessShare 打开链接（受保护时换 access_token）、
 * ListShareChildren 逐层浏览、GetShareDownloadUrl 取下载地址。
 */
import { computed, onMounted, ref } from 'vue'
import { useRoute } from 'vue-router'

import AppAvatar from '@/components/ui/AppAvatar.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import FileGrid from '@/components/files/FileGrid.vue'
import FileTable from '@/components/files/FileTable.vue'
import { ApiError, errorText, sharesApi } from '@/api'
import type { Node, Share } from '@/api/types'
import { useUiStore } from '@/stores/ui'
import { formatBytes, formatExpiry, formatNumber, toInt } from '@/utils/format'
import { openInNewTab, triggerDownload } from '@/utils/download'
import { previewModeOf } from '@/utils/media'

const route = useRoute()
const ui = useUiStore()

const token = computed(() => String(route.params.token ?? ''))
const share = ref<Share | null>(null)
const root = ref<Node | null>(null)
const children = ref<Node[]>([])
const accessToken = ref('')
const currentFolderId = ref('')
const pathStack = ref<{ id: string; name: string }[]>([])

const loading = ref(false)
const error = ref('')
const needsPassword = ref(false)
const passwordHint = ref('')
const password = ref('')
const unlocking = ref(false)
const viewMode = ref<'list' | 'grid'>('list')
const busyId = ref('')

const canView = computed(() => hasCapability('PERMISSION_VIEW'))
const canDownload = computed(() => hasCapability('PERMISSION_DOWNLOAD'))
const isFolder = computed(() => root.value?.kind === 'NODE_KIND_FOLDER')
const unlockNotice = computed(() => share.value?.passwordProtected && !accessToken.value)

const breadcrumbs = computed(() => {
  const items = [{ id: '', name: root.value?.name || '共享内容' }]
  for (const item of pathStack.value) items.push({ id: item.id, name: item.name })
  return items
})

/** 链接传达的能力来自节点上的 effectivePermissions。 */
function hasCapability(permission: 'PERMISSION_VIEW' | 'PERMISSION_DOWNLOAD' | 'PERMISSION_UPLOAD'): boolean {
  const source = root.value ?? share.value?.node
  const list = source?.effectivePermissions ?? share.value?.permissions ?? []
  return list.includes(permission)
}

async function openShare(pwd?: string): Promise<boolean> {
  loading.value = true
  error.value = ''
  try {
    const result = await sharesApi.accessShare({
      token: token.value,
      password: pwd,
      accessToken: accessToken.value || undefined,
      nodeId: currentFolderId.value || undefined,
      pageSize: 200,
    })
    share.value = result.share ?? null
    root.value = result.node ?? null
    children.value = result.children ?? []
    if (result.accessToken) accessToken.value = result.accessToken
    needsPassword.value = false
    return true
  } catch (err) {
    if (err instanceof ApiError && err.isLocked) {
      needsPassword.value = true
      passwordHint.value = err.passwordHint || share.value?.passwordHint || ''
      return false
    }
    error.value = errorText(err)
    return false
  } finally {
    loading.value = false
  }
}

async function unlock(): Promise<void> {
  if (!password.value) {
    ui.toast.warning('请输入访问密码')
    return
  }
  unlocking.value = true
  try {
    const ok = await openShare(password.value)
    if (ok) {
      ui.toast.success('已解锁')
      password.value = ''
    }
  } finally {
    unlocking.value = false
  }
}

async function enterFolder(node: Node): Promise<void> {
  if (!node.id) return
  loading.value = true
  error.value = ''
  try {
    const result = await sharesApi.listShareChildren({
      token: token.value,
      nodeId: node.id,
      accessToken: accessToken.value || undefined,
      pageSize: 200,
    })
    children.value = result.nodes ?? []
    currentFolderId.value = node.id
    pathStack.value = [...pathStack.value, { id: node.id, name: node.name || '未命名' }]
  } catch (err) {
    error.value = errorText(err)
  } finally {
    loading.value = false
  }
}

async function goTo(index: number): Promise<void> {
  if (index === 0) {
    currentFolderId.value = ''
    pathStack.value = []
    await openShare()
    return
  }
  const target = pathStack.value[index - 1]
  if (!target) return
  loading.value = true
  try {
    const result = await sharesApi.listShareChildren({
      token: token.value,
      nodeId: target.id,
      accessToken: accessToken.value || undefined,
      pageSize: 200,
    })
    children.value = result.nodes ?? []
    currentFolderId.value = target.id
    pathStack.value = pathStack.value.slice(0, index)
  } catch (err) {
    error.value = errorText(err)
  } finally {
    loading.value = false
  }
}

async function download(node: Node): Promise<void> {
  busyId.value = node.id ?? ''
  try {
    const signed = await sharesApi.getShareDownloadUrl({
      token: token.value,
      nodeId: node.id,
      accessToken: accessToken.value || undefined,
    })
    triggerDownload(signed, node.name)
    ui.toast.success('开始下载', signed.fileName || node.name)
    if (share.value) void openShare()
  } catch (err) {
    ui.toast.error('下载失败', errorText(err))
  } finally {
    busyId.value = ''
  }
}

async function preview(node: Node): Promise<void> {
  const mode = previewModeOf(node)
  if (mode === 'none') {
    void download(node)
    return
  }
  busyId.value = node.id ?? ''
  try {
    const signed = await sharesApi.getShareDownloadUrl({
      token: token.value,
      nodeId: node.id,
      accessToken: accessToken.value || undefined,
      inline: true,
    })
    openInNewTab(signed.url)
  } catch (err) {
    ui.toast.error('预览失败', errorText(err))
  } finally {
    busyId.value = ''
  }
}

function openNode(node: Node): void {
  if (node.kind === 'NODE_KIND_FOLDER') void enterFolder(node)
  else void preview(node)
}

function onNodeAction(payload: { key: string; node: Node }): void {
  switch (payload.key) {
    case 'open':
      openNode(payload.node)
      break
    case 'download':
      void download(payload.node)
      break
    case 'preview':
      void preview(payload.node)
      break
    case 'details':
      ui.toast.info(payload.node.name ?? '', `${payload.node.mimeType || '文件夹'} · ${formatBytes(payload.node.size)}`)
      break
    default:
      break
  }
}

async function copyLink(): Promise<void> {
  try {
    await navigator.clipboard.writeText(window.location.href)
    ui.toast.success('链接已复制')
  } catch {
    ui.toast.error('复制失败')
  }
}

onMounted(async () => {
  if (!token.value) {
    error.value = '分享链接缺少令牌'
    return
  }
  await openShare()
})
</script>

<template>
  <div class="share-page">
    <header class="share-page__bar">
      <div class="share-page__brand">
        <span class="share-page__logo"><AppIcon name="cloud" :size="18" /></span>
        <span>Nagisa 网盘 · 分享</span>
      </div>
      <div class="share-page__bar-actions">
        <AppButton size="sm" variant="ghost" icon="copy" @click="copyLink">复制链接</AppButton>
        <RouterLink to="/files"><AppButton size="sm" icon="folder">我的网盘</AppButton></RouterLink>
      </div>
    </header>

    <main class="share-page__main">
      <section v-if="loading && !share" class="share-page__center">
        <AppSpinner :size="26" label="正在打开分享…" />
      </section>

      <section v-else-if="needsPassword" class="share-card card">
        <div class="share-card__lock">
          <span class="share-card__lock-icon"><AppIcon name="lock" :size="22" /></span>
          <h1 class="share-card__title">该分享需要访问密码</h1>
          <p v-if="passwordHint" class="share-card__hint">密码提示：{{ passwordHint }}</p>
          <div class="share-card__unlock">
            <AppInput
              v-model="password"
              type="password"
              size="lg"
              placeholder="请输入访问密码"
              @enter="unlock"
            />
            <AppButton variant="primary" size="lg" :loading="unlocking" icon="unlock" @click="unlock">解锁</AppButton>
          </div>
          <p v-if="error" class="field__error">{{ error }}</p>
        </div>
      </section>

      <section v-else-if="error && !share" class="share-card card">
        <AppEmpty icon="alert-circle" title="无法打开分享" :description="error">
          <RouterLink to="/files"><AppButton icon="home">返回我的网盘</AppButton></RouterLink>
        </AppEmpty>
      </section>

      <template v-else-if="share">
        <section class="share-card card">
          <div class="share-card__head">
            <div class="share-card__info">
              <AppAvatar :name="share.ownerName || '分享者'" :size="40" />
              <div class="share-card__info-text">
                <h1 class="share-card__title truncate">{{ share.name || root?.name || '分享内容' }}</h1>
                <p class="share-card__meta">
                  <span>{{ share.ownerName || '匿名分享者' }} 分享</span>
                  <span>·</span>
                  <span>{{ share.expiresAt ? formatExpiry(share.expiresAt) : '长期有效' }}</span>
                  <span>·</span>
                  <span>浏览 {{ toInt(share.viewCount) }} / 下载 {{ toInt(share.downloadCount) }}</span>
                  <template v-if="toInt(share.maxDownloads) > 0">
                    <span>·</span>
                    <span>额度 {{ toInt(share.downloadCount) }} / {{ toInt(share.maxDownloads) }}</span>
                  </template>
                </p>
                <p v-if="share.description" class="share-card__desc">{{ share.description }}</p>
              </div>
            </div>
            <div class="share-card__caps">
              <AppBadge v-if="canView" tone="brand" size="sm">查看</AppBadge>
              <AppBadge v-if="canDownload" tone="success" size="sm">下载</AppBadge>
              <AppBadge v-if="hasCapability('PERMISSION_UPLOAD')" tone="info" size="sm">上传</AppBadge>
              <AppBadge v-if="share.passwordProtected" tone="warning" size="sm" icon="lock">密码保护</AppBadge>
            </div>
          </div>

          <div class="share-card__toolbar">
            <nav class="share-card__crumbs" aria-label="分享路径">
              <button
                v-for="(crumb, index) in breadcrumbs"
                :key="crumb.id || 'root'"
                type="button"
                class="share-card__crumb"
                :class="{ 'is-current': index === breadcrumbs.length - 1 }"
                @click="goTo(index)"
              >
                {{ crumb.name }}
              </button>
            </nav>
            <span class="toolbar__spacer" />
            <AppButton
              size="sm"
              variant="ghost"
              :icon="viewMode === 'list' ? 'grid' : 'list'"
              :label="viewMode === 'list' ? '切换网格视图' : '切换列表视图'"
              @click="viewMode = viewMode === 'list' ? 'grid' : 'list'"
            />
            <AppButton size="sm" variant="ghost" icon="refresh" label="刷新" @click="openShare()" />
          </div>

          <p v-if="!isFolder" class="share-card__single">
            <AppIcon name="info" :size="15" />
            这是一个文件分享，直接预览或下载即可：
            <AppButton size="sm" variant="primary" icon="download" :loading="busyId === root?.id" @click="root && download(root)">
              下载文件
            </AppButton>
          </p>

          <div v-if="loading" class="share-card__state muted text-sm">加载中…</div>
          <AppEmpty
            v-else-if="isFolder && children.length === 0"
            size="sm"
            icon="folder-open"
            title="这个目录是空的"
            description="分享者还没有放入内容"
          />
          <template v-else-if="isFolder">
            <FileTable
              v-if="viewMode === 'list'"
              :nodes="children"
              context="share"
              :can-download="canDownload"
              @open="openNode"
              @action="onNodeAction"
            />
            <FileGrid
              v-else
              :nodes="children"
              context="share"
              :can-download="canDownload"
              @open="openNode"
              @action="onNodeAction"
            />
          </template>
        </section>

        <p class="share-page__foot">
          由 Nagisa 网盘托管 · 链接能力仅限
          {{ [canView ? '查看' : '', canDownload ? '下载' : '', hasCapability('PERMISSION_UPLOAD') ? '上传' : ''].filter(Boolean).join(' / ') || '浏览' }}
          <template v-if="toInt(share.maxDownloads) > 0">
            · 剩余下载额度 {{ Math.max(0, toInt(share.maxDownloads) - toInt(share.downloadCount)) }}
          </template>
        </p>
      </template>
    </main>
  </div>
</template>

<style scoped>
.share-page {
  min-height: 100vh;
  background: var(--bg-app);
  display: flex;
  flex-direction: column;
}

.share-page__bar {
  height: var(--layout-topbar);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: 0 var(--space-5);
  background: var(--surface);
  border-bottom: 1px solid var(--border);
}

.share-page__brand {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-strong);
}

.share-page__logo {
  width: 30px;
  height: 30px;
  border-radius: var(--radius-md);
  background: linear-gradient(135deg, var(--brand-500), var(--brand-700));
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.share-page__bar-actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.share-page__main {
  flex: 1 1 auto;
  width: 100%;
  max-width: 1100px;
  margin: 0 auto;
  padding: var(--space-6) var(--space-5) var(--space-10);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.share-page__center {
  display: flex;
  justify-content: center;
  padding: var(--space-16) 0;
}

.share-card {
  padding: var(--space-5);
}

.share-card__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-4);
  flex-wrap: wrap;
}

.share-card__info {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-width: 0;
}

.share-card__info-text {
  min-width: 0;
}

.share-card__title {
  font-size: var(--text-xl);
  font-weight: 660;
}

.share-card__meta {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-wrap: wrap;
  font-size: var(--text-2xs);
  color: var(--text-muted);
  margin-top: 3px;
}

.share-card__desc {
  font-size: var(--text-sm);
  color: var(--text-muted);
  margin-top: var(--space-2);
  max-width: 70ch;
}

.share-card__caps {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.share-card__toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  margin: var(--space-4) 0;
  padding-bottom: var(--space-3);
  border-bottom: 1px solid var(--border-subtle);
  flex-wrap: wrap;
}

.share-card__crumbs {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex-wrap: wrap;
}

.share-card__crumb {
  border: 0;
  background: transparent;
  color: var(--text-muted);
  font-size: var(--text-sm);
  padding: 3px var(--space-2);
  border-radius: var(--radius-sm);
  cursor: pointer;
}

.share-card__crumb:hover {
  background: var(--surface-hover);
  color: var(--text);
}

.share-card__crumb.is-current {
  color: var(--text-strong);
  font-weight: 620;
}

.share-card__single {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  color: var(--text-muted);
  background: var(--surface-2);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  flex-wrap: wrap;
}

.share-card__state {
  padding: var(--space-5);
  text-align: center;
}

.share-card__lock {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: var(--space-3);
  text-align: center;
  padding: var(--space-10) var(--space-5);
  max-width: 420px;
  margin: 0 auto;
}

.share-card__lock-icon {
  width: 52px;
  height: 52px;
  border-radius: var(--radius-xl);
  background: var(--warning-50);
  color: var(--warning-600);
  display: inline-flex;
  align-items: center;
  justify-content: center;
}

.share-card__hint {
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.share-card__unlock {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
}

.share-page__foot {
  text-align: center;
  font-size: var(--text-2xs);
  color: var(--text-faint);
}
</style>
