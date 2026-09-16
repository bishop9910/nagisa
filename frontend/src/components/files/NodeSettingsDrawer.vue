<script setup lang="ts">
/**
 * 节点详情抽屉：概览、设置（描述/可见范围/密码）、访问名单、历史版本。
 * 这里集中了「文件夹设置」的全部能力，文件和文件夹共用同一套界面。
 */
import { computed, ref, watch } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import AppDrawer from '@/components/ui/AppDrawer.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import AppTabs from '@/components/ui/AppTabs.vue'
import AppTextarea from '@/components/ui/AppTextarea.vue'
import AppSpinner from '@/components/ui/AppSpinner.vue'
import type { TabItem } from '@/components/ui/types'
import AclEditor from './AclEditor.vue'
import ShareDialog from './ShareDialog.vue'
import VersionList from './VersionList.vue'
import { errorText, nodesApi } from '@/api'
import type { Node, NodeAcl, NodeStats, Visibility } from '@/api/types'
import { useUiStore } from '@/stores/ui'
import { VISIBILITY_HINT, VISIBILITY_LABEL, VISIBILITY_OPTIONS } from '@/utils/constants'
import { formatBytes, formatDateTime, formatNumber, toInt } from '@/utils/format'
import { mediaKindOf } from '@/utils/media'

const props = defineProps<{
  modelValue: boolean
  node?: Node | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'updated', node: Node): void
  (e: 'deleted', node: Node): void
}>()

const ui = useUiStore()

const tab = ref('overview')
const detail = ref<Node | null>(null)
const stats = ref<NodeStats | null>(null)
const acl = ref<NodeAcl | null>(null)
const loading = ref(false)
const saving = ref(false)
const error = ref('')
const shareOpen = ref(false)

const form = ref({
  name: '',
  description: '',
  visibility: 'VISIBILITY_PRIVATE' as Visibility,
  password: '',
  passwordHint: '',
  removePassword: false,
})

const current = computed(() => detail.value ?? props.node ?? null)
const isFolder = computed(() => current.value?.kind === 'NODE_KIND_FOLDER')
const canEdit = computed(() => ((toInt(current.value?.effectivePermissionsMask) & 8) !== 0) || current.value?.owned === true)
const canAcl = computed(() => (toInt(current.value?.effectivePermissionsMask) & 128) !== 0)
const canShare = computed(() => (toInt(current.value?.effectivePermissionsMask) & 64) !== 0)
const canDelete = computed(() => (toInt(current.value?.effectivePermissionsMask) & 16) !== 0)

const tabs = computed<TabItem[]>(() => {
  const list: TabItem[] = [{ key: 'overview', label: '概览' }]
  if (canEdit.value || canAcl.value) list.push({ key: 'settings', label: '设置' })
  list.push({ key: 'acl', label: '访问名单' })
  if (!isFolder.value) list.push({ key: 'versions', label: '历史版本', badge: current.value?.versionCount })
  list.push({ key: 'meta', label: '元数据' })
  return list
})

const visibilityOptions = computed(() =>
  VISIBILITY_OPTIONS.map((option) => ({ value: option.value, label: option.label })),
)

const metadataEntries = computed(() => Object.entries(current.value?.metadata ?? {}))

const categoryRows = computed(() => {
  const map = stats.value?.sizeByCategory ?? {}
  return Object.entries(map).map(([key, value]) => ({ key, value: toInt(value) }))
})

async function load(): Promise<void> {
  if (!props.node?.id) return
  loading.value = true
  error.value = ''
  try {
    detail.value = await nodesApi.getNode(props.node.id)
    form.value = {
      name: detail.value.name ?? '',
      description: detail.value.description ?? '',
      visibility: detail.value.visibility ?? 'VISIBILITY_PRIVATE',
      password: '',
      passwordHint: detail.value.passwordHint ?? '',
      removePassword: false,
    }
    const [aclResult, statsResult] = await Promise.all([
      nodesApi.getNodeAcl(props.node.id).catch(() => null),
      isFolder.value ? nodesApi.getNodeStats(props.node.id).catch(() => null) : Promise.resolve(null),
    ])
    acl.value = aclResult
    stats.value = statsResult
  } catch (err) {
    error.value = errorText(err)
  } finally {
    loading.value = false
  }
}

async function save(): Promise<void> {
  if (!current.value?.id) return
  const mask: string[] = []
  const fields: Partial<Node> = {}
  if (form.value.description !== (current.value.description ?? '')) {
    mask.push('description')
    fields.description = form.value.description
  }
  if (form.value.visibility !== (current.value.visibility ?? 'VISIBILITY_PRIVATE')) {
    mask.push('visibility')
    fields.visibility = form.value.visibility
  }
  if (form.value.removePassword || form.value.password) {
    mask.push('password')
  }
  if (mask.length === 0) {
    ui.toast.info('没有需要保存的修改')
    return
  }
  saving.value = true
  try {
    const updated = await nodesApi.updateNode({
      id: current.value.id,
      updateMask: mask,
      fields,
      password: form.value.removePassword ? undefined : form.value.password || undefined,
      removePassword: form.value.removePassword,
    })
    detail.value = updated
    form.value.password = ''
    form.value.removePassword = false
    ui.toast.success('设置已保存')
    emit('updated', updated)
  } catch (err) {
    ui.toast.error('保存失败', errorText(err))
  } finally {
    saving.value = false
  }
}

function applyAcl(updated: NodeAcl): void {
  acl.value = updated
}

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      tab.value = 'overview'
      void load()
    } else {
      detail.value = null
      stats.value = null
      acl.value = null
    }
  },
)
</script>

<template>
  <AppDrawer
    :model-value="modelValue"
    :title="current?.name || '详细信息'"
    :subtitle="isFolder ? '文件夹' : current?.mimeType || '文件'"
    size="lg"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div v-if="loading" class="detail__loading">
      <AppSpinner :size="22" label="加载中…" />
    </div>
    <div v-else class="detail">
      <AppTabs v-model="tab" :tabs="tabs" />

      <section v-if="tab === 'overview'" class="detail__section">
        <dl class="kv">
          <div class="kv__row">
            <dt>名称</dt>
            <dd class="truncate">{{ current?.name }}</dd>
          </div>
          <div class="kv__row">
            <dt>类型</dt>
            <dd>{{ isFolder ? '文件夹' : mediaKindOf(current) }}</dd>
          </div>
          <div class="kv__row">
            <dt>大小</dt>
            <dd>{{ isFolder ? '—' : formatBytes(current?.size) }}</dd>
          </div>
          <div class="kv__row">
            <dt>所在路径</dt>
            <dd class="truncate mono" :title="current?.displayPath">{{ current?.displayPath || '/' }}</dd>
          </div>
          <div class="kv__row">
            <dt>所有者</dt>
            <dd>{{ current?.ownerName || current?.ownerId || '—' }}</dd>
          </div>
          <div class="kv__row">
            <dt>可见范围</dt>
            <dd>{{ VISIBILITY_LABEL[current?.visibility ?? 'VISIBILITY_UNSPECIFIED'] }}</dd>
          </div>
          <div class="kv__row">
            <dt>密码保护</dt>
            <dd>{{ current?.passwordProtected ? `是${current?.passwordHint ? `（提示：${current.passwordHint}）` : ''}` : '否' }}</dd>
          </div>
          <div class="kv__row">
            <dt>分享</dt>
            <dd>{{ toInt(current?.shareCount) > 0 ? `${toInt(current?.shareCount)} 条链接` : '未分享' }}</dd>
          </div>
          <div v-if="!isFolder" class="kv__row">
            <dt>版本数</dt>
            <dd>{{ toInt(current?.versionCount) }}</dd>
          </div>
          <div class="kv__row">
            <dt>创建时间</dt>
            <dd>{{ formatDateTime(current?.createdAt) }}</dd>
          </div>
          <div class="kv__row">
            <dt>修改时间</dt>
            <dd>{{ formatDateTime(current?.updatedAt) }}</dd>
          </div>
          <div class="kv__row">
            <dt>节点 ID</dt>
            <dd class="mono break-all">{{ current?.id }}</dd>
          </div>
          <div v-if="current?.etag" class="kv__row">
            <dt>ETag</dt>
            <dd class="mono break-all">{{ current.etag }}</dd>
          </div>
        </dl>

        <template v-if="stats">
          <h4 class="card__title mt-4">子树统计</h4>
          <div class="stat-grid mt-2">
            <div class="stat">
              <span class="stat__label">文件</span>
              <span class="stat__value">{{ formatNumber(stats.fileCount) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">文件夹</span>
              <span class="stat__value">{{ formatNumber(stats.folderCount) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">总大小</span>
              <span class="stat__value">{{ formatBytes(stats.totalSize) }}</span>
            </div>
            <div class="stat">
              <span class="stat__label">回收站条目</span>
              <span class="stat__value">{{ formatNumber(stats.trashedCount) }}</span>
            </div>
          </div>
          <p v-if="stats.largestFileName" class="text-xs muted mt-2">
            最大文件：{{ stats.largestFileName }}（{{ formatBytes(stats.largestFileSize) }}）
          </p>
          <ul v-if="categoryRows.length > 0" class="detail__categories">
            <li v-for="row in categoryRows" :key="row.key">
              <span class="text-xs">{{ row.key }}</span>
              <span class="text-xs muted">{{ formatBytes(row.value) }}</span>
            </li>
          </ul>
        </template>

        <div class="detail__actions">
          <AppButton v-if="canShare" icon="share" @click="shareOpen = true">分享设置</AppButton>
          <AppButton
            v-if="canDelete"
            variant="ghost"
            icon="trash"
            @click="current && emit('deleted', current)"
          >
            删除
          </AppButton>
        </div>
      </section>

      <section v-else-if="tab === 'settings'" class="detail__section">
        <label class="field">
          <span class="field__label">描述</span>
          <AppTextarea v-model="form.description" :rows="3" placeholder="记录这个目录的用途，搜索时可按描述匹配" />
        </label>
        <label class="field">
          <span class="field__label">可见范围</span>
          <AppSelect v-model="form.visibility" :options="visibilityOptions" />
          <span class="field__hint">{{ VISIBILITY_HINT[form.visibility] }}</span>
        </label>
        <div class="field">
          <span class="field__label">访问密码</span>
          <div v-if="current?.passwordProtected && !form.removePassword" class="detail__password-state">
            <span class="badge badge--warning"><AppIcon name="lock" :size="12" />已启用</span>
            <AppButton size="sm" variant="ghost" icon="unlock" @click="form.removePassword = true">清除密码</AppButton>
          </div>
          <div v-if="form.removePassword" class="detail__password-state">
            <span class="text-xs">保存后会清除密码</span>
            <AppButton size="sm" variant="ghost" @click="form.removePassword = false">撤销</AppButton>
          </div>
          <template v-else>
            <AppInput v-model="form.password" type="password" placeholder="设置新密码（留空表示不修改）" />
            <span class="field__hint">密码会在浏览器端用服务端公钥加密后再提交，明文不会离开本机。</span>
          </template>
        </div>
        <label class="field">
          <span class="field__label">密码提示</span>
          <AppInput v-model="form.passwordHint" placeholder="展示给需要解锁的访问者" />
        </label>
        <p v-if="!canAcl && !canEdit" class="field__hint">当前账号没有修改该节点设置的权限。</p>
        <div class="detail__actions">
          <AppButton variant="primary" :loading="saving" icon="check" :disabled="!canEdit && !canAcl" @click="save">
            保存设置
          </AppButton>
        </div>
      </section>

      <section v-else-if="tab === 'acl'" class="detail__section">
        <AclEditor v-if="current?.id" :node-id="current.id" :acl="acl" @saved="applyAcl" />
      </section>

      <section v-else-if="tab === 'versions'" class="detail__section">
        <VersionList
          v-if="current?.id"
          :node-id="current.id"
          :can-edit="canEdit"
          :can-delete="canDelete"
          @changed="load"
        />
      </section>

      <section v-else-if="tab === 'meta'" class="detail__section">
        <p v-if="metadataEntries.length === 0" class="text-xs muted">该节点没有附加的键值元数据。</p>
        <dl v-else class="kv">
          <div v-for="[key, value] in metadataEntries" :key="key" class="kv__row">
            <dt class="mono">{{ key }}</dt>
            <dd class="break-all">{{ value }}</dd>
          </div>
        </dl>
      </section>

      <p v-if="error" class="field__error">{{ error }}</p>
    </div>

    <template #footer>
      <AppButton variant="ghost" @click="emit('update:modelValue', false)">关闭</AppButton>
    </template>
  </AppDrawer>

  <ShareDialog v-model="shareOpen" :node="current" @changed="load" />
</template>

<style scoped>
.detail {
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.detail__loading {
  display: flex;
  justify-content: center;
  padding: var(--space-10) 0;
}

.detail__section {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.kv {
  display: flex;
  flex-direction: column;
  margin: 0;
  border: 1px solid var(--border-subtle);
  border-radius: var(--radius-md);
  overflow: hidden;
}

.kv__row {
  display: grid;
  grid-template-columns: 132px 1fr;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--border-subtle);
  font-size: var(--text-xs);
}

.kv__row:last-child {
  border-bottom: 0;
}

.kv__row:nth-child(odd) {
  background: var(--surface-2);
}

.kv dt {
  color: var(--text-muted);
}

.kv dd {
  margin: 0;
  color: var(--text);
  min-width: 0;
}

.detail__categories {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.detail__categories li {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: 2px 0;
}

.detail__actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.detail__password-state {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}
</style>
