<script setup lang="ts">
/**
 * 我的文件：目录浏览、上传、预览、下载、重命名、移动、复制、分享、删除。
 * 目录状态由 files store 持有，URL 里的 folderId 是唯一事实来源，便于分享与刷新。
 */
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppDropdown from '@/components/ui/AppDropdown.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppSegmented from '@/components/ui/AppSegmented.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import AppTooltip from '@/components/ui/AppTooltip.vue'
import Breadcrumbs from '@/components/files/Breadcrumbs.vue'
import FileGrid from '@/components/files/FileGrid.vue'
import FileTable from '@/components/files/FileTable.vue'
import MoveCopyDialog from '@/components/files/MoveCopyDialog.vue'
import NodeSettingsDrawer from '@/components/files/NodeSettingsDrawer.vue'
import PreviewDialog from '@/components/files/PreviewDialog.vue'
import RenameDialog from '@/components/files/RenameDialog.vue'
import ShareDialog from '@/components/files/ShareDialog.vue'
import UnlockDialog from '@/components/files/UnlockDialog.vue'
import { filesApi, nodesApi, errorText } from '@/api'
import type { Node } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useFilesStore } from '@/stores/files'
import { useSystemStore } from '@/stores/system'
import { useUiStore } from '@/stores/ui'
import { useUploadStore } from '@/stores/upload'
import { formatBytes, formatNumber, toInt } from '@/utils/format'
import { isPreviewable } from '@/utils/media'
import { triggerDownload } from '@/utils/download'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const files = useFilesStore()
const system = useSystemStore()
const ui = useUiStore()
const uploads = useUploadStore()

const keyword = ref('')
const dragging = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const folderInput = ref<HTMLInputElement | null>(null)

const renameTarget = ref<Node | null>(null)
const renameOpen = ref(false)
const drawerTarget = ref<Node | null>(null)
const drawerOpen = ref(false)
const previewTarget = ref<Node | null>(null)
const previewOpen = ref(false)
const shareTarget = ref<Node | null>(null)
const shareOpen = ref(false)
const unlockOpen = ref(false)
const newFolderOpen = ref(false)
const newFolderName = ref('')
const newFolderBusy = ref(false)
const newFolderError = ref('')
const moveCopyOpen = ref(false)
const moveCopyMode = ref<'move' | 'copy'>('move')
const moveCopyNodes = ref<Node[]>([])

const kindOptions = [
  { value: '', label: '全部', title: '全部条目' },
  { value: 'NODE_KIND_FOLDER', label: '文件夹', icon: 'folder' },
  { value: 'NODE_KIND_FILE', label: '文件', icon: 'file' },
]

const viewOptions = [
  { value: 'list', icon: 'list', title: '列表视图' },
  { value: 'grid', icon: 'grid', title: '网格视图' },
]

const sortItems = [
  { key: 'name', label: '按名称', icon: 'sort' },
  { key: 'size', label: '按大小', icon: 'sort' },
  { key: 'updated_at', label: '按修改时间', icon: 'sort' },
  { key: 'created_at', label: '按创建时间', icon: 'sort' },
  { key: 'divider', label: '', divider: true },
  { key: 'direction', label: files.sort.desc ? '改为升序' : '改为降序', icon: 'arrow-up' },
]

const selectedCount = computed(() => files.selection.length)
const selectionHasFolder = computed(() => files.selectedNodes.some((node) => node.kind === 'NODE_KIND_FOLDER'))
const canTrashManage = computed(() => auth.has('PERMISSION_TRASH_MANAGE'))
const uploadTargetLabel = computed(() => files.folder?.name || '我的网盘')

const dropHint = computed(() => {
  if (files.locked) return '该目录需要先解锁'
  // 访客看不到任何东西通常不是他的问题，而是没有内容对外开放。
  if (auth.isGuest) return '还没有向访客开放的内容：把文件夹的可见范围设为「内部」或「公开」即可出现在这里'
  return `拖拽文件到「${uploadTargetLabel.value}」即可上传`
})

/** URL 与 store 同步：刷新、前进后退、直接粘贴链接都走这一条路径。 */
watch(
  () => route.params.folderId,
  async (value) => {
    const id = typeof value === 'string' ? value : ''
    await files.open(id)
  },
  { immediate: false },
)

watch(keyword, (value) => {
  files.setKeyword(value.trim())
  void files.loadNodes()
})

/** 上传完成后刷新当前目录，让新文件立刻出现。 */
watch(
  () => uploads.lastCompleted,
  (payload) => {
    if (!payload) return
    if (payload.parentId === files.folderId) {
      void files.reload({ withPath: false })
    }
  },
)

function openNode(node: Node): void {
  if (node.kind === 'NODE_KIND_FOLDER') {
    if (node.locked) {
      drawerTarget.value = node
      unlockOpen.value = true
      return
    }
    void router.push({ name: 'files', params: { folderId: node.id } })
    return
  }
  if (isPreviewable(node)) {
    previewTarget.value = node
    previewOpen.value = true
    return
  }
  void downloadNode(node)
}

async function downloadNode(node: Node): Promise<void> {
  try {
    if (node.kind === 'NODE_KIND_FOLDER') {
      const signed = await filesApi.getArchiveUrl(node.id ?? '', { archiveName: node.name })
      triggerDownload(signed, `${node.name}.zip`)
      ui.toast.success('开始打包下载', `${node.name}.zip`)
    } else {
      const signed = await filesApi.getDownloadUrl(node.id ?? '', { fileName: node.name })
      triggerDownload(signed, node.name)
    }
  } catch (err) {
    ui.toast.error('下载失败', errorText(err))
  }
}

async function downloadSelection(): Promise<void> {
  const nodes = files.selectedNodes
  if (nodes.length === 0) return
  for (const node of nodes) {
    await downloadNode(node)
  }
  files.clearSelection()
}

function pickFiles(): void {
  fileInput.value?.click()
}

function onFilesPicked(event: Event): void {
  const input = event.target as HTMLInputElement
  const picked = Array.from(input.files ?? [])
  if (picked.length > 0) {
    uploads.enqueue(picked, { parentId: files.folderId, parentLabel: uploadTargetLabel.value })
    ui.toast.info(`已加入上传队列`, `${picked.length} 个文件`)
  }
  input.value = ''
}

function onUploadMenu(key: string): void {
  if (key === 'folder') folderInput.value?.click()
  else void router.push({ name: 'uploads' })
}

function createFolderDialog(): void {
  newFolderName.value = ''
  newFolderError.value = ''
  newFolderOpen.value = true
}

function onDrop(event: DragEvent): void {
  dragging.value = false
  const dropped = Array.from(event.dataTransfer?.files ?? [])
  if (dropped.length === 0) return
  uploads.enqueue(dropped, { parentId: files.folderId, parentLabel: uploadTargetLabel.value })
  ui.toast.info('已加入上传队列', `${dropped.length} 个文件`)
}

async function createFolder(): Promise<void> {
  const name = newFolderName.value.trim()
  if (!name) {
    newFolderError.value = '请输入文件夹名称'
    return
  }
  newFolderBusy.value = true
  newFolderError.value = ''
  try {
    await nodesApi.createFolder({
      name,
      parentId: files.folderId || undefined,
      conflictPolicy: 'CONFLICT_POLICY_FAIL',
    })
    ui.toast.success('文件夹已创建')
    newFolderOpen.value = false
    newFolderName.value = ''
    await files.reload()
  } catch (err) {
    newFolderError.value = errorText(err)
  } finally {
    newFolderBusy.value = false
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
    case 'rename':
      renameTarget.value = node
      renameOpen.value = true
      break
    case 'move':
    case 'copy':
      moveCopyMode.value = key === 'move' ? 'move' : 'copy'
      moveCopyNodes.value = [node]
      moveCopyOpen.value = true
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
    case 'delete':
      void deleteNodes([node])
      break
    default:
      break
  }
}

async function deleteNodes(nodes: Node[]): Promise<void> {
  const ids = nodes.map((node) => node.id ?? '').filter(Boolean)
  if (ids.length === 0) return
  const label = nodes.length === 1 ? `「${nodes[0]?.name}」` : `选中的 ${ids.length} 项`
  const ok = await ui.confirm({
    title: '移入回收站',
    message: `${label}会被移入回收站，可在回收站中还原。`,
    tone: 'danger',
    confirmText: '移入回收站',
  })
  if (!ok) return
  try {
    const result = await nodesApi.deleteNodes(ids)
    ui.toast.success('已移入回收站', `影响 ${toInt(result.affectedCount)} 个节点`)
    files.removeLocal(ids)
    files.clearSelection()
  } catch (err) {
    ui.toast.error('删除失败', errorText(err))
  }
}

function onSelectionDelete(): void {
  void deleteNodes(files.selectedNodes)
}

function onSelectionMove(mode: 'move' | 'copy'): void {
  moveCopyMode.value = mode
  moveCopyNodes.value = files.selectedNodes
  moveCopyOpen.value = true
}

function onMoved(): void {
  files.clearSelection()
  void files.reload()
}

function onRenamed(): void {
  void files.reload()
}

function onSortPick(key: string): void {
  if (key === 'direction') {
    files.setSort(files.sort.field)
    return
  }
  if (key === 'divider') return
  const field = key as 'name' | 'size' | 'updated_at' | 'created_at'
  if (files.sort.field === field) {
    files.setSort(field)
    return
  }
  files.sort = { field, desc: field !== 'name' }
  void files.loadNodes()
}

async function onUnlocked(): Promise<void> {
  await files.reload({ withPath: true })
}

function onDrawerDeleted(node: Node): void {
  drawerOpen.value = false
  void deleteNodes([node])
}

async function refresh(): Promise<void> {
  await files.reload({ withPath: true })
}

onMounted(async () => {
  await system.loadInfo()
  const id = typeof route.params.folderId === 'string' ? route.params.folderId : ''
  await files.open(id)
  if (files.locked) unlockOpen.value = true
})
</script>

<template>
  <div
    class="page files"
    :class="{ 'files--drop': dragging }"
    @dragover.prevent="dragging = true"
    @dragleave.prevent="dragging = false"
    @drop.prevent="onDrop"
  >
    <header class="files__head">
      <div class="files__head-main">
        <Breadcrumbs
          :folder-id="files.folderId"
          :ancestors="files.ancestors"
          :folder="files.folder"
          @navigate="(id) => router.push({ name: 'files', params: id ? { folderId: id } : {} })"
        />
        <p class="files__meta">
          <span>{{ formatNumber(files.totalSize) }} 项</span>
          <template v-if="files.totalBytes > 0">
            <span>·</span>
            <span>{{ formatBytes(files.totalBytes) }}</span>
          </template>
          <template v-if="files.folder?.description">
            <span>·</span>
            <span class="truncate">{{ files.folder.description }}</span>
          </template>
        </p>
      </div>

      <div class="files__head-actions">
        <AppButton icon="refresh" variant="ghost" label="刷新" @click="refresh" />
        <AppButton
          icon="folder-plus"
          :disabled="!files.canUpload"
          @click="newFolderOpen = true"
        >
          新建文件夹
        </AppButton>
        <AppButton variant="primary" icon="upload" :disabled="!files.canUpload" @click="pickFiles">上传</AppButton>
        <AppDropdown
          :items="[
            { key: 'folder', label: '上传文件夹', icon: 'folder-plus' },
            { key: 'uploads', label: '打开传输列表', icon: 'list' },
          ]"
          @select="onUploadMenu"
        >
          <template #trigger>
            <AppButton icon="more-vertical" label="更多上传操作" />
          </template>
        </AppDropdown>
      </div>
    </header>

    <div v-if="!files.locked" class="files__toolbar card card--flat">
      <AppInput
        v-model="keyword"
        size="sm"
        icon="search"
        clearable
        placeholder="在当前目录中筛选"
        class="files__filter"
      />
      <AppSegmented
        :model-value="files.kindFilter || ''"
        size="sm"
        :options="kindOptions"
        @update:model-value="(value) => files.setKindFilter((value || '') as '' | 'NODE_KIND_FOLDER' | 'NODE_KIND_FILE')"
      />
      <label v-if="canTrashManage" class="files__switch text-xs muted">
        <AppSwitch :model-value="files.includeTrashed" label="显示回收站内容" @update:model-value="files.setIncludeTrashed($event)" />
        显示回收站内容
      </label>
      <span class="toolbar__spacer" />
      <AppDropdown :items="sortItems" :width="180" @select="onSortPick">
        <template #trigger>
          <AppButton size="sm" icon="sort">
            {{
              files.sort.field === 'name'
                ? '名称'
                : files.sort.field === 'size'
                  ? '大小'
                  : files.sort.field === 'updated_at'
                    ? '修改时间'
                    : '创建时间'
            }}{{ files.sort.desc ? ' ↓' : ' ↑' }}
          </AppButton>
        </template>
      </AppDropdown>
      <AppSegmented
        :model-value="files.viewMode"
        size="sm"
        :options="viewOptions"
        @update:model-value="(value) => files.setViewMode(value as 'list' | 'grid')"
      />
    </div>

    <div v-if="selectedCount > 0" class="files__selection card">
      <span class="text-sm">已选择 <strong>{{ selectedCount }}</strong> 项</span>
      <span class="toolbar__spacer" />
      <AppButton size="sm" icon="download" @click="downloadSelection">下载</AppButton>
      <AppButton
        size="sm"
        icon="move"
        :disabled="!files.canEdit"
        @click="onSelectionMove('move')"
      >
        移动
      </AppButton>
      <AppButton
        size="sm"
        icon="copy"
        :disabled="!files.canUpload"
        @click="onSelectionMove('copy')"
      >
        复制
      </AppButton>
      <AppButton size="sm" variant="danger" icon="trash" :disabled="!files.canDelete" @click="onSelectionDelete">
        删除
      </AppButton>
      <AppButton size="sm" variant="ghost" @click="files.clearSelection()">取消选择</AppButton>
    </div>

    <section v-if="files.locked" class="card card--pad files__locked">
      <AppEmpty
        icon="lock"
        title="该目录受密码保护"
        :description="files.lockedHint ? `密码提示：${files.lockedHint}` : '输入访问密码后即可浏览目录内容'"
      >
        <AppButton variant="primary" icon="unlock" @click="unlockOpen = true">输入密码解锁</AppButton>
      </AppEmpty>
    </section>

    <section v-else-if="files.error" class="card card--pad">
      <AppEmpty icon="alert-circle" title="加载失败" :description="files.error">
        <AppButton icon="refresh" @click="refresh">重试</AppButton>
      </AppEmpty>
    </section>

    <section v-else-if="!files.loading && files.nodes.length === 0" class="card card--pad">
      <AppEmpty
        :icon="keyword ? 'search' : 'folder-open'"
        :title="keyword ? '没有匹配的条目' : auth.isGuest ? '访客可见的内容为空' : '这里还是空的'"
        :description="keyword ? '换个关键词试试' : dropHint"
      >
        <AppButton v-if="!keyword && files.canUpload" variant="primary" icon="upload" @click="pickFiles">
          上传文件
        </AppButton>
        <AppButton v-if="!keyword && files.canUpload" icon="folder-plus" @click="newFolderOpen = true">
          新建文件夹
        </AppButton>
      </AppEmpty>
    </section>

    <section v-else class="card files__list">
      <FileTable
        v-if="files.viewMode === 'list'"
        :nodes="files.nodes"
        :selection="files.selection"
        :sort="files.sort"
        :loading="files.loading"
        :can-edit="files.canEdit"
        :can-delete="files.canDelete"
        :can-share="files.canShare"
        :can-download="files.canDownload"
        @open="openNode"
        @action="onNodeAction"
        @toggle-select="files.toggleSelect"
        @toggle-all="files.selection.length === files.nodes.length ? files.clearSelection() : files.selectAll()"
        @sort="files.setSort"
      />
      <FileGrid
        v-else
        :nodes="files.nodes"
        :selection="files.selection"
        :loading="files.loading"
        :can-edit="files.canEdit"
        :can-delete="files.canDelete"
        :can-share="files.canShare"
        :can-download="files.canDownload"
        @open="openNode"
        @action="onNodeAction"
        @toggle-select="files.toggleSelect"
      />
      <div v-if="files.nextPageToken" class="files__more">
        <AppButton :loading="files.loadingMore" @click="files.loadMore">加载更多</AppButton>
      </div>
    </section>

    <p v-if="dragging" class="files__drop-hint">松开即可上传到「{{ uploadTargetLabel }}」</p>

    <input ref="fileInput" type="file" multiple class="hidden" @change="onFilesPicked" />
    <input
      ref="folderInput"
      type="file"
      multiple
      webkitdirectory
      directory
      class="hidden"
      @change="onFilesPicked"
    />

    <AppDialog v-model="newFolderOpen" title="新建文件夹" size="sm">
      <AppInput
        v-model="newFolderName"
        size="lg"
        placeholder="文件夹名称"
        :invalid="Boolean(newFolderError)"
        @enter="createFolder"
      />
      <p v-if="newFolderError" class="field__error mt-2">{{ newFolderError }}</p>
      <p class="field__hint mt-2">将创建在「{{ uploadTargetLabel }}」下。</p>
      <template #footer>
        <AppButton variant="ghost" @click="newFolderOpen = false">取消</AppButton>
        <AppButton variant="primary" :loading="newFolderBusy" @click="createFolder">创建</AppButton>
      </template>
    </AppDialog>

    <RenameDialog v-model="renameOpen" :node="renameTarget" @renamed="onRenamed" />
    <MoveCopyDialog
      v-model="moveCopyOpen"
      :mode="moveCopyMode"
      :nodes="moveCopyNodes"
      @done="onMoved"
    />
    <PreviewDialog v-model="previewOpen" :node="previewTarget" @download="downloadNode" />
    <ShareDialog v-model="shareOpen" :node="shareTarget" @changed="files.reload()" />
    <NodeSettingsDrawer
      v-model="drawerOpen"
      :node="drawerTarget"
      @updated="files.reload()"
      @deleted="onDrawerDeleted"
    />
    <UnlockDialog
      v-model="unlockOpen"
      :node-id="files.folderId || drawerTarget?.id || ''"
      :node-name="files.folder?.name || drawerTarget?.name"
      :hint="files.lockedHint || drawerTarget?.passwordHint"
      @unlocked="onUnlocked"
    />
  </div>
</template>

<style scoped>
.files {
  position: relative;
}

.files--drop::after {
  content: '';
  position: absolute;
  inset: 0;
  border: 2px dashed var(--accent);
  border-radius: var(--radius-lg);
  background: color-mix(in srgb, var(--accent-soft) 60%, transparent);
  pointer-events: none;
  z-index: var(--z-sticky);
}

.files__head {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: var(--space-4);
  flex-wrap: wrap;
}

.files__head-main {
  min-width: 0;
}

.files__meta {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: var(--text-xs);
  color: var(--text-muted);
  margin-top: 4px;
  max-width: 60ch;
}

.files__head-actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.files__toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  flex-wrap: wrap;
}

.files__filter {
  width: min(260px, 100%);
}

.files__switch {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
}

.files__selection {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-4);
  border-color: var(--accent);
  background: var(--surface-active);
  flex-wrap: wrap;
}

.files__list {
  overflow: hidden;
}

.files__more {
  display: flex;
  justify-content: center;
  padding: var(--space-4);
}

.files__locked {
  display: flex;
  justify-content: center;
}

.files__drop-hint {
  position: absolute;
  left: 50%;
  bottom: var(--space-8);
  transform: translateX(-50%);
  background: var(--accent);
  color: #fff;
  font-size: var(--text-sm);
  padding: var(--space-2) var(--space-4);
  border-radius: var(--radius-pill);
  box-shadow: var(--shadow-lg);
  z-index: calc(var(--z-sticky) + 1);
}

@media (max-width: 768px) {
  .files__head-actions {
    width: 100%;
  }
}
</style>
