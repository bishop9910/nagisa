<script setup lang="ts">
/**
 * 我的分享：列表、筛选、复制链接、改设置（能力/密码/过期/额度/状态）与删除。
 * 有 PERMISSION_USER_MANAGE 时能看到全部账号创建的链接。
 */
import { computed, onMounted, ref, watch } from 'vue'

import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppPagination from '@/components/ui/AppPagination.vue'
import AppSegmented from '@/components/ui/AppSegmented.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import { errorText, sharesApi } from '@/api'
import type { Permission, Share } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useSystemStore } from '@/stores/system'
import { useUiStore } from '@/stores/ui'
import { SHARE_STATUS_LABEL, SHARE_STATUS_TONE, maskFromPermissions } from '@/utils/constants'
import { formatDateTime, formatExpiry, fromLocalInputValue, toInt, toLocalInputValue } from '@/utils/format'
import { eq, has, orderBy } from '@/utils/filter'
import { copyText } from '@/utils/download'
import { resolveShareUrl } from '@/utils/share'

const auth = useAuthStore()
const system = useSystemStore()
const ui = useUiStore()

const shares = ref<Share[]>([])
const loading = ref(false)
const error = ref('')
const keyword = ref('')
const statusScope = ref('')
const orderField = ref('created_at')
const orderDesc = ref(true)
const totalSize = ref(0)
const page = ref(1)
const pageSize = ref(50)
const tokenStack = ref<string[]>([''])
const nextToken = ref('')

const editOpen = ref(false)
const editTarget = ref<Share | null>(null)
const saving = ref(false)
const editError = ref('')
const editForm = ref({
  name: '',
  description: '',
  permissions: [] as Permission[],
  password: '',
  passwordHint: '',
  removePassword: false,
  expiresAt: '',
  neverExpires: true,
  maxDownloads: 0,
  unlimitedDownloads: true,
  status: 'SHARE_STATUS_ACTIVE',
})

const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'SHARE_STATUS_ACTIVE', label: '生效中' },
  { value: 'SHARE_STATUS_EXPIRED', label: '已过期' },
  { value: 'SHARE_STATUS_REVOKED', label: '已撤销' },
]

const orderOptions = [
  { value: 'created_at', label: '按创建时间' },
  { value: 'expires_at', label: '按过期时间' },
  { value: 'download_count', label: '按下载次数' },
  { value: 'view_count', label: '按浏览次数' },
  { value: 'name', label: '按名称' },
]

const permissionOptions: { value: Permission; label: string }[] = [
  { value: 'PERMISSION_VIEW', label: '查看' },
  { value: 'PERMISSION_DOWNLOAD', label: '下载' },
  { value: 'PERMISSION_UPLOAD', label: '上传（仅文件夹）' },
]

const canManageAll = computed(() => auth.canManageUsers)

function shareUrl(share: Share): string {
  return resolveShareUrl(share, system.publicBaseUrl)
}

function buildFilter(): string | undefined {
  const parts: (string | undefined)[] = []
  if (keyword.value.trim()) parts.push(has('name', keyword.value.trim()))
  if (statusScope.value) parts.push(eq('status', statusScope.value === 'SHARE_STATUS_ACTIVE' ? 1 : statusScope.value === 'SHARE_STATUS_EXPIRED' ? 2 : 3))
  return parts.filter((part): part is string => Boolean(part)).join(' AND ') || undefined
}

async function load(reset = false): Promise<void> {
  if (reset) {
    page.value = 1
    tokenStack.value = ['']
  }
  loading.value = true
  error.value = ''
  try {
    const result = await sharesApi.listShares({
      pageSize: pageSize.value,
      pageToken: tokenStack.value[page.value - 1] || undefined,
      filter: buildFilter(),
      orderBy: orderBy(orderField.value, orderDesc.value),
    })
    shares.value = result.shares ?? []
    totalSize.value = toInt(result.totalSize, shares.value.length)
    nextToken.value = result.nextPageToken ?? ''
  } catch (err) {
    error.value = errorText(err)
    shares.value = []
  } finally {
    loading.value = false
  }
}

function nextPage(): void {
  if (!nextToken.value) return
  tokenStack.value = [...tokenStack.value.slice(0, page.value), nextToken.value]
  page.value += 1
  void load()
}

function prevPage(): void {
  if (page.value <= 1) return
  page.value -= 1
  void load()
}

async function copyLink(share: Share): Promise<void> {
  const ok = await copyText(shareUrl(share))
  if (ok) ui.toast.success('链接已复制')
  else ui.toast.error('复制失败', '请手动选择链接复制')
}

function openLink(share: Share): void {
  window.open(shareUrl(share), '_blank', 'noopener')
}

function openEdit(share: Share): void {
  editTarget.value = share
  editError.value = ''
  editForm.value = {
    name: share.name ?? '',
    description: share.description ?? '',
    permissions: share.permissions ?? ['PERMISSION_VIEW', 'PERMISSION_DOWNLOAD'],
    password: '',
    passwordHint: share.passwordHint ?? '',
    removePassword: false,
    expiresAt: toLocalInputValue(share.expiresAt),
    neverExpires: !share.expiresAt,
    maxDownloads: toInt(share.maxDownloads),
    unlimitedDownloads: toInt(share.maxDownloads) === 0,
    status: share.status ?? 'SHARE_STATUS_ACTIVE',
  }
  editOpen.value = true
}

function togglePermission(permission: Permission, checked: boolean): void {
  const set = new Set(editForm.value.permissions)
  if (checked) set.add(permission)
  else set.delete(permission)
  editForm.value.permissions = Array.from(set)
}

async function saveEdit(): Promise<void> {
  const share = editTarget.value
  if (!share?.id) return
  saving.value = true
  editError.value = ''
  try {
    await sharesApi.updateShare({
      id: share.id,
      fields: {
        name: editForm.value.name,
        description: editForm.value.description,
        permissions: editForm.value.permissions,
        permissionsMask: maskFromPermissions(editForm.value.permissions),
        passwordHint: editForm.value.passwordHint,
        expiresAt: editForm.value.neverExpires ? undefined : fromLocalInputValue(editForm.value.expiresAt),
        maxDownloads: editForm.value.unlimitedDownloads ? 0 : Math.max(0, Number(editForm.value.maxDownloads) || 0),
        status: editForm.value.status as Share['status'],
      },
      updateMask: [
        'name',
        'description',
        'permissionsMask',
        'passwordHint',
        'expiresAt',
        'maxDownloads',
        'status',
        ...(editForm.value.password || editForm.value.removePassword ? ['password'] : []),
      ],
      password: editForm.value.password || undefined,
      removePassword: editForm.value.removePassword,
    })
    ui.toast.success('分享已更新')
    editOpen.value = false
    await load()
  } catch (err) {
    editError.value = errorText(err)
  } finally {
    saving.value = false
  }
}

async function remove(share: Share): Promise<void> {
  const ok = await ui.confirm({
    title: '删除分享链接',
    message: `「${share.name || share.token}」会立即失效，访问者将看到「链接不存在」。`,
    tone: 'danger',
    confirmText: '删除',
  })
  if (!ok) return
  try {
    await sharesApi.deleteShare(share.id ?? '')
    ui.toast.success('已删除')
    await load()
  } catch (err) {
    ui.toast.error('删除失败', errorText(err))
  }
}

function onPageSize(size: number): void {
  pageSize.value = size
  void load(true)
}

watch([keyword, statusScope, orderField, orderDesc], () => void load(true))

onMounted(() => {
  void system.loadInfo()
  void load()
})
</script>

<template>
  <div class="page">
    <header class="page__header">
      <div>
        <h1 class="page__title">我的分享</h1>
        <p class="page__desc">
          分享链接自带能力集合、可选密码、过期时间与下载额度；
          {{ canManageAll ? '当前账号可以查看并管理所有账号创建的链接。' : '当前账号只能管理自己创建的链接。' }}
        </p>
      </div>
      <div class="page__actions">
        <AppButton icon="refresh" variant="ghost" label="刷新" @click="load()" />
      </div>
    </header>

    <div class="card card--flat shares__toolbar">
      <AppInput v-model="keyword" size="sm" icon="search" clearable placeholder="按名称筛选" style="width: 220px" />
      <AppSelect v-model="statusScope" size="sm" :options="statusOptions" style="width: 150px" />
      <span class="toolbar__spacer" />
      <AppSelect v-model="orderField" size="sm" :options="orderOptions" style="width: 160px" />
      <AppButton size="sm" variant="ghost" :icon="orderDesc ? 'arrow-down' : 'arrow-up'" @click="orderDesc = !orderDesc">
        {{ orderDesc ? '降序' : '升序' }}
      </AppButton>
    </div>

    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">分享链接</p>
          <p class="card__subtitle">共 {{ toInt(totalSize) }} 条</p>
        </div>
      </header>

      <div v-if="loading" class="shares__state muted text-sm">加载中…</div>
      <AppEmpty
        v-else-if="shares.length === 0"
        :icon="error ? 'alert-circle' : 'share'"
        :title="error ? '加载失败' : '还没有分享链接'"
        :description="error || '在文件列表中选择「分享」即可创建第一条链接'"
      >
        <AppButton v-if="error" icon="refresh" @click="load()">重试</AppButton>
        <RouterLink v-else to="/files"><AppButton variant="primary" icon="folder">去我的文件</AppButton></RouterLink>
      </AppEmpty>

      <ul v-else class="shares__list">
        <li v-for="share in shares" :key="share.id" class="shares__item">
          <div class="shares__main">
            <p class="shares__title">
              <AppIcon name="link" :size="15" />
              <span class="truncate">{{ share.name || share.token }}</span>
              <AppBadge :tone="SHARE_STATUS_TONE[share.status ?? 'SHARE_STATUS_UNSPECIFIED']" size="sm">
                {{ SHARE_STATUS_LABEL[share.status ?? 'SHARE_STATUS_UNSPECIFIED'] }}
              </AppBadge>
              <AppBadge v-if="share.passwordProtected" tone="warning" size="sm" icon="lock">密码</AppBadge>
            </p>
            <p class="shares__url mono truncate" :title="shareUrl(share)">{{ shareUrl(share) }}</p>
            <p class="shares__meta">
              <span>{{ share.node?.name || share.nodeId }}</span>
              <span>·</span>
              <span>{{ share.ownerName || share.ownerId }}</span>
              <span>·</span>
              <span>浏览 {{ toInt(share.viewCount) }} / 下载 {{ toInt(share.downloadCount) }}</span>
              <span>·</span>
              <span>{{ share.expiresAt ? formatExpiry(share.expiresAt) : '永不过期' }}</span>
              <span>·</span>
              <span>创建于 {{ formatDateTime(share.createdAt) }}</span>
            </p>
            <p class="shares__caps">
              <AppBadge v-for="permission in share.permissions ?? []" :key="permission" tone="brand" size="sm">
                {{ permission === 'PERMISSION_VIEW' ? '查看' : permission === 'PERMISSION_DOWNLOAD' ? '下载' : permission === 'PERMISSION_UPLOAD' ? '上传' : permission }}
              </AppBadge>
            </p>
          </div>
          <div class="shares__actions">
            <AppButton size="sm" icon="copy" @click="copyLink(share)">复制</AppButton>
            <AppButton size="sm" variant="ghost" icon="external" label="打开链接" @click="openLink(share)" />
            <AppButton
              size="sm"
              variant="ghost"
              icon="edit"
              label="编辑"
              :disabled="share.editable === false"
              @click="openEdit(share)"
            />
            <AppButton
              size="sm"
              variant="ghost"
              icon="trash"
              label="删除"
              :disabled="share.editable === false"
              @click="remove(share)"
            />
          </div>
        </li>
      </ul>

      <AppPagination
        v-if="shares.length > 0"
        :page="page"
        :page-size="pageSize"
        :total="totalSize"
        :has-prev="page > 1"
        :has-next="Boolean(nextToken)"
        :disabled="loading"
        @prev="prevPage"
        @next="nextPage"
        @update:page-size="onPageSize"
      />
    </section>

    <AppDialog v-model="editOpen" title="编辑分享" size="md">
      <div class="form-grid">
        <label class="field">
          <span class="field__label">名称</span>
          <AppInput v-model="editForm.name" placeholder="展示给访问者的标题" />
        </label>
        <label class="field">
          <span class="field__label">状态</span>
          <AppSelect
            v-model="editForm.status"
            :options="[
              { value: 'SHARE_STATUS_ACTIVE', label: '生效中' },
              { value: 'SHARE_STATUS_REVOKED', label: '已撤销（链接立即失效）' },
            ]"
          />
        </label>
        <div class="field form-row-full">
          <span class="field__label">能力</span>
          <div class="flex items-center gap-4 wrap">
            <AppCheckbox
              v-for="option in permissionOptions"
              :key="option.value"
              :model-value="editForm.permissions.includes(option.value)"
              :label="option.label"
              @update:model-value="togglePermission(option.value, $event)"
            />
          </div>
        </div>
        <label class="field">
          <span class="field__label">说明</span>
          <AppInput v-model="editForm.description" placeholder="可选" />
        </label>
        <label class="field">
          <span class="field__label">密码提示</span>
          <AppInput v-model="editForm.passwordHint" placeholder="可选" />
        </label>
        <div class="field">
          <span class="field__label">密码</span>
          <div v-if="editTarget?.passwordProtected && !editForm.removePassword" class="flex items-center gap-2">
            <AppBadge tone="warning" size="sm">已设置</AppBadge>
            <AppButton size="sm" variant="ghost" @click="editForm.removePassword = true">清除密码</AppButton>
          </div>
          <AppInput v-else v-model="editForm.password" type="password" placeholder="设置新密码（留空表示不修改）" />
        </div>
        <div class="field">
          <span class="field__label">过期时间</span>
          <div class="flex items-center gap-2">
            <AppSwitch v-model="editForm.neverExpires" label="永不过期" />
            <AppInput v-if="!editForm.neverExpires" v-model="editForm.expiresAt" type="datetime-local" />
            <span v-else class="text-xs muted">永不过期</span>
          </div>
        </div>
        <div class="field">
          <span class="field__label">下载额度</span>
          <div class="flex items-center gap-2">
            <AppSwitch v-model="editForm.unlimitedDownloads" label="不限次数" />
            <AppInput
              v-if="!editForm.unlimitedDownloads"
              v-model="editForm.maxDownloads"
              type="number"
              min="1"
              style="width: 120px"
            />
            <span v-else class="text-xs muted">不限次数</span>
          </div>
        </div>
      </div>
      <p v-if="editError" class="field__error mt-3">{{ editError }}</p>
      <template #footer>
        <AppButton variant="ghost" @click="editOpen = false">取消</AppButton>
        <AppButton variant="primary" :loading="saving" @click="saveEdit">保存</AppButton>
      </template>
    </AppDialog>
  </div>
</template>

<style scoped>
.shares__toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  flex-wrap: wrap;
}

.shares__state {
  padding: var(--space-6);
  text-align: center;
}

.shares__list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.shares__item {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-4) var(--space-5);
  border-bottom: 1px solid var(--border-subtle);
}

.shares__item:last-child {
  border-bottom: 0;
}

.shares__main {
  flex: 1 1 auto;
  min-width: 0;
}

.shares__title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  font-weight: 620;
  color: var(--text-strong);
  min-width: 0;
}

.shares__url {
  font-size: var(--text-2xs);
  color: var(--accent-text);
  margin-top: 3px;
}

.shares__meta {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-wrap: wrap;
  font-size: var(--text-2xs);
  color: var(--text-muted);
  margin-top: 3px;
}

.shares__caps {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex-wrap: wrap;
  margin-top: var(--space-2);
}

.shares__actions {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex: none;
}

@media (max-width: 768px) {
  .shares__item {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
