<script setup lang="ts">
/** 分享对话框：管理某个节点已有的链接，并新建链接。 */
import { computed, ref, watch } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import AppTextarea from '@/components/ui/AppTextarea.vue'
import { errorText, sharesApi } from '@/api'
import type { Node, Permission, Share } from '@/api/types'
import { useSystemStore } from '@/stores/system'
import { useUiStore } from '@/stores/ui'
import { SHARE_STATUS_LABEL, SHARE_STATUS_TONE } from '@/utils/constants'
import { formatDateTime, formatExpiry, fromLocalInputValue, toInt } from '@/utils/format'
import { resolveShareUrl } from '@/utils/share'

const props = defineProps<{
  modelValue: boolean
  node?: Node | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'changed'): void
}>()

const ui = useUiStore()
const system = useSystemStore()

const existing = ref<Share[]>([])
const loadingList = ref(false)
const creating = ref(false)
const error = ref('')

const form = ref({
  name: '',
  description: '',
  permissions: ['PERMISSION_VIEW', 'PERMISSION_DOWNLOAD'] as Permission[],
  password: '',
  passwordHint: '',
  expiresAt: '',
  maxDownloads: 0,
  unlimitedDownloads: true,
  customToken: '',
})

const isFolder = computed(() => props.node?.kind === 'NODE_KIND_FOLDER')
const publicShareAllowed = computed(() => system.hasFeature('public_share') || system.info === null)

const permissionOptions = computed(() => {
  const options: { value: Permission; label: string; hint: string }[] = [
    { value: 'PERMISSION_VIEW', label: '查看', hint: '浏览目录与元信息' },
    { value: 'PERMISSION_DOWNLOAD', label: '下载', hint: '下载原始文件或打包内容' },
  ]
  if (isFolder.value) {
    options.push({ value: 'PERMISSION_UPLOAD', label: '上传', hint: '允许访客向该目录上传' })
  }
  return options
})

function togglePermission(permission: Permission, checked: boolean): void {
  const set = new Set(form.value.permissions)
  if (checked) set.add(permission)
  else set.delete(permission)
  form.value.permissions = Array.from(set)
}

async function loadShares(): Promise<void> {
  if (!props.node?.id) return
  loadingList.value = true
  try {
    const page = await sharesApi.listSharesByNode(props.node.id, { pageSize: 50 })
    existing.value = page.shares ?? []
  } catch (err) {
    error.value = errorText(err)
  } finally {
    loadingList.value = false
  }
}

function shareUrl(share: Share): string {
  return resolveShareUrl(share, system.publicBaseUrl)
}

async function copy(text: string, label: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
    ui.toast.success(`${label}已复制`)
  } catch {
    ui.toast.error('复制失败', '浏览器拒绝了剪贴板访问，请手动复制')
  }
}

function openShare(share: Share): void {
  window.open(shareUrl(share), '_blank', 'noopener')
}

async function submit(): Promise<void> {
  if (!props.node?.id) return
  if (form.value.permissions.length === 0) {
    error.value = '至少选择一项能力'
    return
  }
  creating.value = true
  error.value = ''
  try {
    await sharesApi.createShare({
      nodeId: props.node.id,
      name: form.value.name.trim() || undefined,
      description: form.value.description.trim() || undefined,
      permissions: form.value.permissions,
      password: form.value.password || undefined,
      passwordHint: form.value.passwordHint.trim() || undefined,
      expiresAt: fromLocalInputValue(form.value.expiresAt),
      maxDownloads: form.value.unlimitedDownloads ? 0 : Math.max(0, Number(form.value.maxDownloads) || 0),
      token: form.value.customToken.trim() || undefined,
    })
    ui.toast.success('分享链接已创建')
    form.value = { ...form.value, password: '', passwordHint: '', customToken: '', expiresAt: '' }
    await loadShares()
    emit('changed')
  } catch (err) {
    error.value = errorText(err)
  } finally {
    creating.value = false
  }
}

async function revoke(share: Share): Promise<void> {
  if (!share.id) return
  const ok = await ui.confirm({
    title: '撤销分享链接',
    message: '撤销后该链接立即失效，访问者会看到「链接不存在」。',
    tone: 'danger',
    confirmText: '撤销',
  })
  if (!ok) return
  try {
    await sharesApi.deleteShare(share.id)
    ui.toast.success('已撤销')
    await loadShares()
    emit('changed')
  } catch (err) {
    ui.toast.error('撤销失败', errorText(err))
  }
}

watch(
  () => props.modelValue,
  async (open) => {
    if (!open) return
    error.value = ''
    form.value = {
      name: props.node?.name ?? '',
      description: '',
      permissions: ['PERMISSION_VIEW', 'PERMISSION_DOWNLOAD'],
      password: '',
      passwordHint: '',
      expiresAt: '',
      maxDownloads: 0,
      unlimitedDownloads: true,
      customToken: '',
    }
    await loadShares()
  },
)
</script>

<template>
  <AppDialog
    :model-value="modelValue"
    title="分享"
    :description="node ? `为「${node.name}」创建或管理分享链接` : ''"
    size="lg"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <p v-if="!publicShareAllowed" class="share__warn">
      <AppIcon name="alert-triangle" :size="16" />
      当前部署关闭了匿名分享（storage.allow_public_share=false），创建会失败。
    </p>

    <section class="share__section">
      <header class="share__section-head">
        <h4 class="card__title">已有链接</h4>
        <AppButton size="sm" variant="ghost" icon="refresh" label="刷新" @click="loadShares" />
      </header>
      <p v-if="loadingList" class="muted text-xs">加载中…</p>
      <p v-else-if="existing.length === 0" class="muted text-xs">还没有分享链接</p>
      <ul v-else class="share__list">
        <li v-for="share in existing" :key="share.id" class="share__item">
          <div class="share__item-main">
            <p class="share__item-title">
              {{ share.name || '未命名链接' }}
              <span class="badge" :class="`badge--${SHARE_STATUS_TONE[share.status ?? 'SHARE_STATUS_UNSPECIFIED']}`">
                {{ SHARE_STATUS_LABEL[share.status ?? 'SHARE_STATUS_UNSPECIFIED'] }}
              </span>
              <span v-if="share.passwordProtected" class="badge badge--warning">密码</span>
            </p>
            <p class="share__item-meta mono truncate" :title="shareUrl(share)">{{ shareUrl(share) }}</p>
            <p class="share__item-meta">
              <span>浏览 {{ toInt(share.viewCount) }}</span>
              <span>·</span>
              <span>
                下载 {{ toInt(share.downloadCount) }}
                <template v-if="toInt(share.maxDownloads) > 0"> / {{ toInt(share.maxDownloads) }}</template>
              </span>
              <span>·</span>
              <span>{{ share.expiresAt ? formatExpiry(share.expiresAt) : '永不过期' }}</span>
              <span>·</span>
              <span>创建于 {{ formatDateTime(share.createdAt) }}</span>
            </p>
          </div>
          <div class="share__item-actions">
            <AppButton size="sm" icon="copy" @click="copy(shareUrl(share), '链接')">复制</AppButton>
            <AppButton size="sm" variant="ghost" icon="external" label="打开" @click="openShare(share)" />
            <AppButton
              v-if="share.editable !== false"
              size="sm"
              variant="ghost"
              icon="trash"
              label="撤销"
              @click="revoke(share)"
            />
          </div>
        </li>
      </ul>
    </section>

    <section class="share__section">
      <h4 class="card__title mb-2">新建链接</h4>
      <div class="form-grid">
        <label class="field">
          <span class="field__label">链接名称</span>
          <AppInput v-model="form.name" placeholder="例如：项目资料（可选）" />
        </label>
        <label class="field">
          <span class="field__label">自定义令牌</span>
          <AppInput v-model="form.customToken" placeholder="8-64 位字母数字（可选）" />
        </label>
        <div class="field form-row-full">
          <span class="field__label">能力</span>
          <div class="share__perms">
            <AppCheckbox
              v-for="option in permissionOptions"
              :key="option.value"
              :model-value="form.permissions.includes(option.value)"
              :label="option.label"
              @update:model-value="togglePermission(option.value, $event)"
            />
          </div>
          <span class="field__hint">分享链接只能传达查看、下载与上传三项能力</span>
        </div>
        <label class="field">
          <span class="field__label">访问密码</span>
          <AppInput v-model="form.password" type="password" placeholder="留空表示无需密码" />
        </label>
        <label class="field">
          <span class="field__label">密码提示</span>
          <AppInput v-model="form.passwordHint" placeholder="例如：公司缩写（可选）" />
        </label>
        <label class="field">
          <span class="field__label">过期时间</span>
          <AppInput v-model="form.expiresAt" type="datetime-local" />
        </label>
        <div class="field">
          <span class="field__label">下载额度</span>
          <div class="flex items-center gap-3">
            <AppSwitch v-model="form.unlimitedDownloads" label="不限下载次数" />
            <span class="text-xs muted">{{ form.unlimitedDownloads ? '不限次数' : '限制次数' }}</span>
            <AppInput
              v-if="!form.unlimitedDownloads"
              v-model="form.maxDownloads"
              type="number"
              min="1"
              size="sm"
              style="width: 110px"
            />
          </div>
        </div>
        <label class="field form-row-full">
          <span class="field__label">说明</span>
          <AppTextarea v-model="form.description" :rows="2" placeholder="展示给访问者的备注（可选）" />
        </label>
      </div>
      <p v-if="error" class="field__error mt-2">{{ error }}</p>
    </section>

    <template #footer>
      <AppButton variant="ghost" @click="emit('update:modelValue', false)">关闭</AppButton>
      <AppButton variant="primary" icon="share" :loading="creating" @click="submit">创建链接</AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.share__warn {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  background: var(--warning-50);
  color: var(--warning-600);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  font-size: var(--text-xs);
  margin-bottom: var(--space-4);
}

.share__section + .share__section {
  margin-top: var(--space-6);
  padding-top: var(--space-5);
  border-top: 1px solid var(--border-subtle);
}

.share__section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-2);
}

.share__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.share__item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3);
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  background: var(--surface-2);
}

.share__item-main {
  flex: 1 1 auto;
  min-width: 0;
}

.share__item-title {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-strong);
}

.share__item-meta {
  display: flex;
  align-items: center;
  gap: 5px;
  font-size: var(--text-2xs);
  color: var(--text-muted);
  margin-top: 2px;
  flex-wrap: wrap;
}

.share__item-actions {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex: none;
}

.share__perms {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  flex-wrap: wrap;
}

@media (max-width: 640px) {
  .share__item {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
