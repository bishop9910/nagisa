/**
 * 文件浏览状态：当前目录、面包屑、列表、排序、筛选与多选。
 * 目录内容与后端 ListNodes 的响应一一对应，翻页用 nextPageToken 追加。
 */

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { nodesApi, ApiError } from '@/api'
import type { ConflictPolicy, Node, NodeKind } from '@/api/types'
import { buildFilter, buildOrderBy, eq, has } from '@/utils/filter'
import { toInt } from '@/utils/format'
import { useAuthStore } from './auth'

export type SortField = 'name' | 'size' | 'kind' | 'mime_type' | 'updated_at' | 'created_at'
export type ViewMode = 'list' | 'grid'

const VIEW_KEY = 'nagisa.view'
const PAGE_SIZE = 200

export interface SortState {
  field: SortField
  desc: boolean
}

/** 根目录的展示名与后端 storage.root_folder_name 的默认值一致。 */
export const ROOT_LABEL = '我的网盘'

export const useFilesStore = defineStore('files', () => {
  const auth = useAuthStore()

  const folderId = ref('')
  const folder = ref<Node | null>(null)
  const ancestors = ref<Node[]>([])
  const nodes = ref<Node[]>([])
  const totalSize = ref(0)
  const totalBytes = ref(0)
  const nextPageToken = ref('')
  const loading = ref(false)
  const loadingMore = ref(false)
  const error = ref('')
  /** 当前目录需要密码且尚未解锁时的提示（来自服务端 error metadata）。 */
  const lockedHint = ref('')
  const locked = ref(false)

  const keyword = ref('')
  const kindFilter = ref<'' | NodeKind>('')
  const includeTrashed = ref(false)
  const sort = ref<SortState>({ field: 'name', desc: false })

  const selection = ref<string[]>([])
  const viewMode = ref<ViewMode>(readViewMode())

  function readViewMode(): ViewMode {
    try {
      const saved = localStorage.getItem(VIEW_KEY)
      if (saved === 'list' || saved === 'grid') return saved
    } catch {
      /* ignore */
    }
    return 'list'
  }

  function setViewMode(mode: ViewMode): void {
    viewMode.value = mode
    try {
      localStorage.setItem(VIEW_KEY, mode)
    } catch {
      /* ignore */
    }
  }

  const breadcrumbs = computed(() => {
    const items = ancestors.value.map((node) => ({ id: node.id ?? '', name: node.name || '未命名' }))
    items.push({ id: folderId.value, name: folder.value?.name || ROOT_LABEL })
    return items
  })

  const chainIds = computed(() => [...ancestors.value.map((node) => node.id ?? ''), folderId.value].filter(Boolean))

  const isRoot = computed(() => folderId.value === '')

  /** 当前目录的有效权限位掩码（属主为 255）。 */
  const permissionMask = computed(() => toInt(folder.value?.effectivePermissionsMask))

  const selectedNodes = computed(() => nodes.value.filter((node) => selection.value.includes(node.id ?? '')))

  const canUpload = computed(() => (isRoot.value ? auth.has('PERMISSION_UPLOAD') : (permissionMask.value & 4) !== 0))

  const canEdit = computed(() => (isRoot.value ? auth.has('PERMISSION_EDIT') : (permissionMask.value & 8) !== 0))

  const canDelete = computed(() => (isRoot.value ? auth.has('PERMISSION_DELETE') : (permissionMask.value & 16) !== 0))

  const canShare = computed(() => (isRoot.value ? auth.has('PERMISSION_SHARE') : (permissionMask.value & 64) !== 0))

  const canDownload = computed(() =>
    isRoot.value ? auth.has('PERMISSION_DOWNLOAD') : (permissionMask.value & 2) !== 0,
  )

  const canManageAcl = computed(() =>
    isRoot.value ? auth.has('PERMISSION_ACL_MANAGE') : (permissionMask.value & 128) !== 0,
  )

  function currentFilter(): string | undefined {
    return buildFilter(
      keyword.value ? has('name', keyword.value) : undefined,
      kindFilter.value ? eq('kind', kindFilter.value === 'NODE_KIND_FOLDER' ? 1 : 2) : undefined,
    )
  }

  function currentOrder(): string | undefined {
    return buildOrderBy([{ field: sort.value.field, desc: sort.value.desc }])
  }

  /** 进入某个目录（空串表示根目录）。 */
  async function open(id: string): Promise<void> {
    folderId.value = id
    selection.value = []
    locked.value = false
    lockedHint.value = ''
    await Promise.all([loadPath(), loadNodes()])
  }

  async function loadPath(): Promise<void> {
    if (!folderId.value) {
      folder.value = null
      ancestors.value = []
      auth.setActiveChain([])
      return
    }
    try {
      const path = await nodesApi.getNodePath(folderId.value)
      folder.value = path.node ?? null
      ancestors.value = path.ancestors ?? []
      auth.setActiveChain(chainIds.value)
    } catch (err) {
      if (err instanceof ApiError && err.isLocked) {
        locked.value = true
        lockedHint.value = err.passwordHint
        return
      }
      throw err
    }
  }

  async function loadNodes(): Promise<void> {
    loading.value = true
    error.value = ''
    try {
      const page = await nodesApi.listNodes({
        parentId: folderId.value,
        pageSize: PAGE_SIZE,
        filter: currentFilter(),
        orderBy: currentOrder(),
        includeTrashed: includeTrashed.value || undefined,
      })
      nodes.value = page.nodes ?? []
      totalSize.value = toInt(page.totalSize, nodes.value.length)
      totalBytes.value = toInt(page.totalBytes)
      nextPageToken.value = page.nextPageToken ?? ''
      locked.value = false
    } catch (err) {
      if (err instanceof ApiError && err.isLocked) {
        locked.value = true
        lockedHint.value = err.passwordHint
        nodes.value = []
      } else {
        error.value = err instanceof Error ? err.message : String(err)
        nodes.value = []
      }
    } finally {
      loading.value = false
    }
  }

  /** 重新载入当前目录（可选再取一次路径信息）。 */
  async function reload(options: { withPath?: boolean } = {}): Promise<void> {
    if (options.withPath) await loadPath()
    await loadNodes()
  }

  async function loadMore(): Promise<void> {
    if (!nextPageToken.value || loadingMore.value) return
    loadingMore.value = true
    try {
      const page = await nodesApi.listNodes({
        parentId: folderId.value,
        pageSize: PAGE_SIZE,
        pageToken: nextPageToken.value,
        filter: currentFilter(),
        orderBy: currentOrder(),
        includeTrashed: includeTrashed.value || undefined,
      })
      nodes.value = [...nodes.value, ...(page.nodes ?? [])]
      nextPageToken.value = page.nextPageToken ?? ''
    } finally {
      loadingMore.value = false
    }
  }

  function setSort(field: SortField): void {
    if (sort.value.field === field) {
      sort.value = { field, desc: !sort.value.desc }
    } else {
      // 名称默认升序，其余默认降序（时间/体积更常见的是「新的、大的在前」）。
      sort.value = { field, desc: field !== 'name' && field !== 'kind' }
    }
    void loadNodes()
  }

  function setKeyword(value: string): void {
    keyword.value = value
  }

  function setKindFilter(value: '' | NodeKind): void {
    kindFilter.value = value
    void loadNodes()
  }

  function setIncludeTrashed(value: boolean): void {
    includeTrashed.value = value
    void loadNodes()
  }

  /* ---------- 选择 ---------- */

  function toggleSelect(id: string): void {
    selection.value = selection.value.includes(id)
      ? selection.value.filter((item) => item !== id)
      : [...selection.value, id]
  }

  function selectAll(): void {
    selection.value = nodes.value.map((node) => node.id ?? '').filter(Boolean)
  }

  function clearSelection(): void {
    selection.value = []
  }

  /** 删除或移动之后把列表同步到最优的新状态。 */
  function removeLocal(ids: string[]): void {
    nodes.value = nodes.value.filter((node) => !ids.includes(node.id ?? ''))
    selection.value = selection.value.filter((id) => !ids.includes(id))
    totalSize.value = Math.max(0, totalSize.value - ids.length)
  }

  /** 列表内的局部更新（重命名、覆盖上传后就地刷新一行）。 */
  function patchLocal(node: Node): void {
    if (!node.id) return
    nodes.value = nodes.value.map((item) => (item.id === node.id ? { ...item, ...node } : item))
  }

  function upsertLocal(node: Node): void {
    if (!node.id) return
    const exists = nodes.value.some((item) => item.id === node.id)
    nodes.value = exists ? nodes.value.map((item) => (item.id === node.id ? { ...item, ...node } : item)) : [node, ...nodes.value]
  }

  /** 冲突策略在多个入口共用，集中在这里生成默认值。 */
  function defaultConflictPolicy(): ConflictPolicy {
    return 'CONFLICT_POLICY_RENAME'
  }

  return {
    folderId,
    folder,
    ancestors,
    nodes,
    totalSize,
    totalBytes,
    nextPageToken,
    loading,
    loadingMore,
    error,
    lockedHint,
    locked,
    keyword,
    kindFilter,
    includeTrashed,
    sort,
    selection,
    viewMode,
    breadcrumbs,
    chainIds,
    isRoot,
    permissionMask,
    selectedNodes,
    canUpload,
    canEdit,
    canDelete,
    canShare,
    canDownload,
    canManageAcl,
    setViewMode,
    currentFilter,
    currentOrder,
    open,
    loadPath,
    loadNodes,
    reload,
    loadMore,
    setSort,
    setKeyword,
    setKindFilter,
    setIncludeTrashed,
    toggleSelect,
    selectAll,
    clearSelection,
    removeLocal,
    patchLocal,
    upsertLocal,
    defaultConflictPolicy,
  }
})
