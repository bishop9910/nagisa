<script setup lang="ts">
/**
 * 回收站：查看、还原、彻底删除与清空。
 * 没有 PERMISSION_TRASH_MANAGE 时只能看到并清理自己拥有的条目。
 */
import { computed, onMounted, ref, watch } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppPagination from '@/components/ui/AppPagination.vue'
import AppSegmented from '@/components/ui/AppSegmented.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import FileTable from '@/components/files/FileTable.vue'
import { errorText, nodesApi } from '@/api'
import type { Node } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { formatBytes, formatNumber, toInt } from '@/utils/format'
import { has, orderBy } from '@/utils/filter'

const auth = useAuthStore()
const ui = useUiStore()

const nodes = ref<Node[]>([])
const loading = ref(false)
const error = ref('')
const keyword = ref('')
const ownerScope = ref('all')
const orderField = ref('trashed_at')
const orderDesc = ref(true)
const totalSize = ref(0)
const totalBytes = ref(0)

const page = ref(1)
const pageSize = ref(50)
const tokenStack = ref<string[]>([''])
const nextToken = ref('')

const selection = ref<string[]>([])
const emptyOpen = ref(false)
const emptyBefore = ref('')
const emptying = ref(false)

const canManage = computed(() => auth.has('PERMISSION_TRASH_MANAGE'))
const selectedNodes = computed(() => nodes.value.filter((node) => selection.value.includes(node.id ?? '')))
const ownerOptions = [
  { value: 'all', label: '全部条目' },
  { value: 'mine', label: '仅我拥有的' },
]
const orderOptions = [
  { value: 'trashed_at', label: '按删除时间' },
  { value: 'name', label: '按名称' },
  { value: 'size', label: '按大小' },
  { value: 'updated_at', label: '按修改时间' },
]

function buildFilter(): string | undefined {
  const parts: (string | undefined)[] = []
  if (keyword.value.trim()) parts.push(has('name', keyword.value.trim()))
  if (ownerScope.value === 'mine' && auth.user?.id) parts.push(`owner_id="${auth.user.id}"`)
  const filters = parts.filter((part): part is string => Boolean(part))
  return filters.length > 0 ? filters.join(' AND ') : undefined
}

async function load(reset = false): Promise<void> {
  if (reset) {
    page.value = 1
    tokenStack.value = ['']
  }
  loading.value = true
  error.value = ''
  try {
    const result = await nodesApi.listTrash({
      pageSize: pageSize.value,
      pageToken: tokenStack.value[page.value - 1] || undefined,
      filter: buildFilter(),
      orderBy: orderBy(orderField.value, orderDesc.value),
    })
    nodes.value = result.nodes ?? []
    totalSize.value = toInt(result.totalSize, nodes.value.length)
    totalBytes.value = toInt(result.totalBytes)
    nextToken.value = result.nextPageToken ?? ''
    selection.value = []
  } catch (err) {
    error.value = errorText(err)
    nodes.value = []
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

async function restore(list: Node[]): Promise<void> {
  const ids = list.map((node) => node.id ?? '').filter(Boolean)
  if (ids.length === 0) return
  try {
    await nodesApi.restoreNodes({ ids, conflictPolicy: 'CONFLICT_POLICY_RENAME' })
    ui.toast.success('已还原', `共 ${ids.length} 项`)
    await load()
  } catch (err) {
    ui.toast.error('还原失败', errorText(err))
  }
}

async function purge(list: Node[]): Promise<void> {
  const ids = list.map((node) => node.id ?? '').filter(Boolean)
  if (ids.length === 0) return
  const ok = await ui.confirm({
    title: '彻底删除',
    message: `将永久删除 ${ids.length} 项及其子树，无法恢复。`,
    tone: 'danger',
    confirmText: '永久删除',
    requireText: ids.length > 3 ? '永久删除' : undefined,
  })
  if (!ok) return
  try {
    const result = await nodesApi.purgeNodes(ids)
    ui.toast.success('已彻底删除', `释放 ${formatBytes(result.reclaimedBytes)}`)
    await load()
  } catch (err) {
    ui.toast.error('删除失败', errorText(err))
  }
}

async function emptyTrash(): Promise<void> {
  emptying.value = true
  try {
    const result = await nodesApi.emptyTrash(
      emptyBefore.value ? new Date(`${emptyBefore.value}T00:00:00`).toISOString() : undefined,
    )
    ui.toast.success('回收站已清理', `删除 ${toInt(result.affectedCount)} 项，释放 ${formatBytes(result.reclaimedBytes)}`)
    emptyOpen.value = false
    emptyBefore.value = ''
    await load(true)
  } catch (err) {
    ui.toast.error('清空失败', errorText(err))
  } finally {
    emptying.value = false
  }
}

function onNodeAction(payload: { key: string; node: Node }): void {
  if (payload.key === 'restore') void restore([payload.node])
  if (payload.key === 'purge') void purge([payload.node])
}

function toggleSelect(id: string): void {
  selection.value = selection.value.includes(id)
    ? selection.value.filter((item) => item !== id)
    : [...selection.value, id]
}

function toggleAll(): void {
  selection.value =
    selection.value.length === nodes.value.length ? [] : nodes.value.map((node) => node.id ?? '').filter(Boolean)
}

function onPageSize(size: number): void {
  pageSize.value = size
  void load(true)
}

watch([keyword, ownerScope, orderField, orderDesc], () => void load(true))

onMounted(() => void load())
</script>

<template>
  <div class="page">
    <header class="page__header">
      <div>
        <h1 class="page__title">回收站</h1>
        <p class="page__desc">
          删除的条目会先进入回收站，行与对象都保留；彻底删除后对象由维护任务回收。
          <template v-if="!canManage">当前账号只能看到并清理自己拥有的条目。</template>
        </p>
      </div>
      <div class="page__actions">
        <AppButton icon="refresh" variant="ghost" label="刷新" @click="load()" />
        <AppButton
          variant="danger"
          icon="trash"
          :disabled="nodes.length === 0"
          @click="emptyOpen = true"
        >
          清空回收站
        </AppButton>
      </div>
    </header>

    <div class="card card--flat trash__toolbar">
      <AppInput v-model="keyword" size="sm" icon="search" clearable placeholder="按名称筛选" style="width: 220px" />
      <AppSegmented v-model="ownerScope" size="sm" :options="ownerOptions" />
      <span class="toolbar__spacer" />
      <AppSelect v-model="orderField" size="sm" :options="orderOptions" />
      <AppButton size="sm" variant="ghost" :icon="orderDesc ? 'arrow-down' : 'arrow-up'" @click="orderDesc = !orderDesc">
        {{ orderDesc ? '降序' : '升序' }}
      </AppButton>
    </div>

    <div v-if="selection.length > 0" class="card trash__selection">
      <span class="text-sm">已选择 <strong>{{ selection.length }}</strong> 项</span>
      <span class="toolbar__spacer" />
      <AppButton size="sm" icon="restore" @click="restore(selectedNodes)">还原</AppButton>
      <AppButton size="sm" variant="danger" icon="trash" @click="purge(selectedNodes)">彻底删除</AppButton>
      <AppButton size="sm" variant="ghost" @click="selection = []">取消选择</AppButton>
    </div>

    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">回收站内容</p>
          <p class="card__subtitle">
            共 {{ formatNumber(totalSize) }} 项
            <template v-if="totalBytes > 0">· 本页 {{ formatBytes(totalBytes) }}</template>
          </p>
        </div>
      </header>

      <FileTable
        v-if="nodes.length > 0 || loading"
        :nodes="nodes"
        :selection="selection"
        :loading="loading"
        context="trash"
        :can-restore="true"
        :can-purge="canManage"
        @open="() => undefined"
        @action="onNodeAction"
        @toggle-select="toggleSelect"
        @toggle-all="toggleAll"
      />
      <AppEmpty
        v-else
        :icon="error ? 'alert-circle' : 'trash'"
        :title="error ? '加载失败' : '回收站是空的'"
        :description="error || '删除的文件与文件夹会出现在这里'"
      >
        <AppButton v-if="error" icon="refresh" @click="load()">重试</AppButton>
      </AppEmpty>

      <AppPagination
        v-if="nodes.length > 0"
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

    <AppDialog v-model="emptyOpen" title="清空回收站" size="sm">
      <p class="text-sm muted">
        会彻底删除你在回收站中可见的全部条目，包括它们的历史版本与对象。该操作不可撤销。
      </p>
      <label class="field mt-4">
        <span class="field__label">只清理该日期之前删除的条目（可选）</span>
        <AppInput v-model="emptyBefore" type="date" />
      </label>
      <template #footer>
        <AppButton variant="ghost" @click="emptyOpen = false">取消</AppButton>
        <AppButton variant="danger" :loading="emptying" icon="trash" @click="emptyTrash">确认清空</AppButton>
      </template>
    </AppDialog>
  </div>
</template>

<style scoped>
.trash__toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  flex-wrap: wrap;
}

.trash__selection {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-4);
  border-color: var(--accent);
  background: var(--surface-active);
  flex-wrap: wrap;
}
</style>
