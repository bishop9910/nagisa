<script setup lang="ts">
/**
 * 全局搜索：对应 NodeService.SearchNodes。
 * 支持关键词、类型、MIME、所有者、体积区间、时间区间与描述匹配。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AppButton from '@/components/ui/AppButton.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppSegmented from '@/components/ui/AppSegmented.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import AppPagination from '@/components/ui/AppPagination.vue'
import FileTable from '@/components/files/FileTable.vue'
import MoveCopyDialog from '@/components/files/MoveCopyDialog.vue'
import NodeSettingsDrawer from '@/components/files/NodeSettingsDrawer.vue'
import PreviewDialog from '@/components/files/PreviewDialog.vue'
import ShareDialog from '@/components/files/ShareDialog.vue'
import { errorText, filesApi, nodesApi, usersApi } from '@/api'
import type { Node, User } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { formatNumber, fromLocalInputValue, toInt } from '@/utils/format'
import { orderBy } from '@/utils/filter'
import { isPreviewable } from '@/utils/media'
import { triggerDownload } from '@/utils/download'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const ui = useUiStore()

const keyword = ref('')
const kind = ref('')
const mime = ref('')
const ownerId = ref('')
const minSize = ref<number | null>(null)
const maxSize = ref<number | null>(null)
const updatedAfter = ref('')
const updatedBefore = ref('')
const matchDescription = ref(false)
const orderField = ref('updated_at')
const orderDesc = ref(true)

const nodes = ref<Node[]>([])
const loading = ref(false)
const error = ref('')
const totalSize = ref(0)
const page = ref(1)
const pageSize = ref(50)
/** SearchNodes 用 page_token 翻页，这里缓存走过的 token 以便回退。 */
const tokenStack = ref<string[]>([''])
const nextToken = ref('')

const previewTarget = ref<Node | null>(null)
const previewOpen = ref(false)
const shareTarget = ref<Node | null>(null)
const shareOpen = ref(false)
const drawerTarget = ref<Node | null>(null)
const drawerOpen = ref(false)
const moveCopyOpen = ref(false)
const moveCopyMode = ref<'move' | 'copy'>('move')
const moveCopyNodes = ref<Node[]>([])

const users = ref<User[]>([])
const hasResults = computed(() => nodes.value.length > 0)

const kindOptions = [
  { value: '', label: '全部' },
  { value: 'NODE_KIND_FOLDER', label: '文件夹' },
  { value: 'NODE_KIND_FILE', label: '文件' },
]

const mimeOptions = [
  { value: '', label: '所有类型' },
  { value: 'image/', label: '图片' },
  { value: 'video/', label: '视频' },
  { value: 'audio/', label: '音频' },
  { value: 'text/', label: '文本' },
  { value: 'application/pdf', label: 'PDF' },
]

const orderOptions = [
  { value: 'updated_at', label: '按修改时间' },
  { value: 'created_at', label: '按创建时间' },
  { value: 'name', label: '按名称' },
  { value: 'size', label: '按大小' },
]

const ownerOptions = computed(() => [
  { value: '', label: '所有人' },
  ...users.value.map((user) => ({ value: user.id ?? '', label: `${user.nickname || user.username}（${user.username}）` })),
])

async function search(reset = true): Promise<void> {
  if (reset) {
    page.value = 1
    tokenStack.value = ['']
  }
  loading.value = true
  error.value = ''
  try {
    const result = await nodesApi.searchNodes({
      query: keyword.value.trim() || undefined,
      kind: kind.value || undefined,
      mimeType: mime.value || undefined,
      ownerId: ownerId.value || undefined,
      minSize: minSize.value ?? undefined,
      maxSize: maxSize.value ?? undefined,
      updatedAfter: fromLocalInputValue(updatedAfter.value),
      updatedBefore: fromLocalInputValue(updatedBefore.value),
      matchDescription: matchDescription.value || undefined,
      pageSize: pageSize.value,
      pageToken: tokenStack.value[page.value - 1] || undefined,
      orderBy: orderBy(orderField.value, orderDesc.value),
    })
    nodes.value = result.nodes ?? []
    totalSize.value = toInt(result.totalSize, nodes.value.length)
    nextToken.value = result.nextPageToken ?? ''
  } catch (err) {
    error.value = errorText(err)
    nodes.value = []
  } finally {
    loading.value = false
  }
}

function clearFilters(): void {
  kind.value = ''
  mime.value = ''
  ownerId.value = ''
  minSize.value = null
  maxSize.value = null
  updatedAfter.value = ''
  updatedBefore.value = ''
  matchDescription.value = false
  void search()
}

function nextPage(): void {
  if (!nextToken.value) return
  tokenStack.value = [...tokenStack.value.slice(0, page.value), nextToken.value]
  page.value += 1
  void search(false)
}

function prevPage(): void {
  if (page.value <= 1) return
  page.value -= 1
  void search(false)
}

async function downloadNode(node: Node): Promise<void> {
  try {
    if (node.kind === 'NODE_KIND_FOLDER') {
      const signed = await filesApi.getArchiveUrl(node.id ?? '', { archiveName: node.name })
      triggerDownload(signed, `${node.name}.zip`)
    } else {
      const signed = await filesApi.getDownloadUrl(node.id ?? '', { fileName: node.name })
      triggerDownload(signed, node.name)
    }
  } catch (err) {
    ui.toast.error('下载失败', errorText(err))
  }
}

function openNode(node: Node): void {
  if (node.kind === 'NODE_KIND_FOLDER') {
    void router.push({ name: 'files', params: { folderId: node.id } })
    return
  }
  if (isPreviewable(node)) {
    previewTarget.value = node
    previewOpen.value = true
  } else {
    void downloadNode(node)
  }
}

async function deleteNodes(list: Node[]): Promise<void> {
  const ids = list.map((node) => node.id ?? '').filter(Boolean)
  if (ids.length === 0) return
  const ok = await ui.confirm({
    title: '移入回收站',
    message: `将移入回收站：${list.map((node) => node.name).join('、')}`,
    tone: 'danger',
    confirmText: '移入回收站',
  })
  if (!ok) return
  try {
    await nodesApi.deleteNodes(ids)
    ui.toast.success('已移入回收站')
    nodes.value = nodes.value.filter((node) => !ids.includes(node.id ?? ''))
  } catch (err) {
    ui.toast.error('删除失败', errorText(err))
  }
}

function onNodeAction(payload: { key: string; node: Node }): void {
  const { key, node } = payload
  switch (key) {
    case 'open':
      openNode(node)
      break
    case 'preview':
      previewTarget.value = node
      previewOpen.value = true
      break
    case 'download':
      void downloadNode(node)
      break
    case 'share':
    case 'shares':
      shareTarget.value = node
      shareOpen.value = true
      break
    case 'details':
    case 'versions':
      drawerTarget.value = node
      drawerOpen.value = true
      break
    case 'move':
    case 'copy':
      moveCopyMode.value = key === 'move' ? 'move' : 'copy'
      moveCopyNodes.value = [node]
      moveCopyOpen.value = true
      break
    case 'delete':
      void deleteNodes([node])
      break
    default:
      break
  }
}

function onSortField(field: string): void {
  const allowed = ['name', 'size', 'updated_at', 'created_at']
  if (!allowed.includes(field)) return
  if (orderField.value === field) {
    orderDesc.value = !orderDesc.value
    return
  }
  orderField.value = field
  orderDesc.value = field !== 'name'
}

function onPageSize(size: number): void {
  pageSize.value = size
  void search()
}

watch(
  () => route.query.q,
  (value) => {
    const next = typeof value === 'string' ? value : ''
    if (next !== keyword.value) {
      keyword.value = next
      void search()
    }
  },
)

watch([kind, mime, ownerId, matchDescription, orderField, orderDesc], () => void search())

onMounted(async () => {
  keyword.value = typeof route.query.q === 'string' ? route.query.q : ''
  if (auth.canManageUsers) {
    try {
      const page = await usersApi.listUsers({ pageSize: 200, orderBy: 'username' })
      users.value = page.users ?? []
    } catch {
      users.value = []
    }
  }
  await search()
})
</script>

<template>
  <div class="page">
    <header class="page__header">
      <div>
        <h1 class="page__title">搜索</h1>
        <p class="page__desc">按名称或描述在整个可见范围内查找，结果逐个做访问判定，锁定与无权限的条目不会出现。</p>
      </div>
    </header>

    <form class="card card--pad search__form" @submit.prevent="search()">
      <div class="search__row">
        <AppInput
          v-model="keyword"
          size="lg"
          icon="search"
          clearable
          placeholder="输入文件名或描述关键词，留空表示匹配全部"
          class="grow"
        />
        <AppButton type="submit" variant="primary" size="lg" icon="search">搜索</AppButton>
      </div>

      <div class="search__filters">
        <AppSegmented v-model="kind" size="sm" :options="kindOptions" />
        <AppSelect v-model="mime" size="sm" :options="mimeOptions" />
        <AppSelect
          v-if="auth.canManageUsers && users.length > 0"
          v-model="ownerId"
          size="sm"
          :options="ownerOptions"
        />
        <label class="flex items-center gap-2 text-xs muted">
          有效时间
          <AppInput v-model="updatedAfter" type="datetime-local" size="sm" style="width: 190px" />
          <span>至</span>
          <AppInput v-model="updatedBefore" type="datetime-local" size="sm" style="width: 190px" />
        </label>
        <label class="flex items-center gap-2 text-xs muted">
          体积
          <AppInput v-model="minSize" type="number" min="0" size="sm" placeholder="≥ 字节" style="width: 120px" />
          <AppInput v-model="maxSize" type="number" min="0" size="sm" placeholder="≤ 字节" style="width: 120px" />
        </label>
        <label class="flex items-center gap-2 text-xs muted">
          <AppSwitch v-model="matchDescription" label="匹配描述" />
          匹配描述
        </label>
        <span class="toolbar__spacer" />
        <AppSelect v-model="orderField" size="sm" :options="orderOptions" />
        <AppButton size="sm" variant="ghost" :icon="orderDesc ? 'arrow-down' : 'arrow-up'" @click="orderDesc = !orderDesc">
          {{ orderDesc ? '降序' : '升序' }}
        </AppButton>
        <AppButton size="sm" variant="ghost" icon="filter" @click="clearFilters">重置</AppButton>
      </div>
    </form>

    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">搜索结果</p>
          <p class="card__subtitle">
            {{ hasResults ? `本页 ${nodes.length} 条，匹配总数 ${formatNumber(totalSize)}` : '还没有结果' }}
          </p>
        </div>
      </header>

      <FileTable
        v-if="hasResults || loading"
        :nodes="nodes"
        :loading="loading"
        :can-edit="auth.has('PERMISSION_EDIT')"
        :can-delete="auth.has('PERMISSION_DELETE')"
        :can-share="auth.has('PERMISSION_SHARE')"
        :can-download="auth.has('PERMISSION_DOWNLOAD')"
        @open="openNode"
        @action="onNodeAction"
        @sort="onSortField"
      />
      <AppEmpty
        v-else
        :icon="error ? 'alert-circle' : 'search'"
        :title="error ? '搜索失败' : '没有匹配的条目'"
        :description="error || '换个关键词，或放宽筛选条件'"
      >
        <AppButton v-if="error" icon="refresh" @click="search()">重试</AppButton>
      </AppEmpty>

      <AppPagination
        v-if="hasResults"
        :page="page"
        :page-size="pageSize"
        :total="totalSize"
        :has-prev="page > 1"
        :has-next="Boolean(nextToken)"
        :page-sizes="[20, 50, 100]"
        :disabled="loading"
        @prev="prevPage"
        @next="nextPage"
        @update:page-size="onPageSize"
      />
    </section>

    <PreviewDialog v-model="previewOpen" :node="previewTarget" @download="downloadNode" />
    <ShareDialog v-model="shareOpen" :node="shareTarget" @changed="search()" />
    <NodeSettingsDrawer v-model="drawerOpen" :node="drawerTarget" @updated="search()" @deleted="search()" />
    <MoveCopyDialog v-model="moveCopyOpen" :mode="moveCopyMode" :nodes="moveCopyNodes" @done="search()" />
  </div>
</template>

<style scoped>
.search__form {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.search__row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.search__filters {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
}
</style>
