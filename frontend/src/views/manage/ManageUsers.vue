<script setup lang="ts">
/**
 * 账号管理：列表、创建、编辑、权限、重置密码、停用与删除。
 *
 * 服务端的硬约束（这里全部在界面上体现）：
 *  - 只能管理 rank 严格低于自己的账号；
 *  - 只能授予自己持有的权限；
 *  - 管理接口不允许作用于自己；
 *  - 内置超级管理员（rank=1000）任何人都无法管理。
 */
import { computed, onMounted, ref, watch } from 'vue'

import AppAvatar from '@/components/ui/AppAvatar.vue'
import AppBadge from '@/components/ui/AppBadge.vue'
import AppButton from '@/components/ui/AppButton.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppDrawer from '@/components/ui/AppDrawer.vue'
import AppDropdown from '@/components/ui/AppDropdown.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
import AppField from '@/components/ui/AppField.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppPagination from '@/components/ui/AppPagination.vue'
import AppProgress from '@/components/ui/AppProgress.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import type { DropdownItem } from '@/components/ui/types'
import { ApiError, errorText, usersApi } from '@/api'
import type { Permission, PermissionCatalog, RolePreset, User, UserStats } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useSystemStore } from '@/stores/system'
import { useUiStore } from '@/stores/ui'
import {
  PERMISSION_META,
  ROLE_LABEL,
  ROLE_TONE,
  USER_STATUS_LABEL,
  USER_STATUS_TONE,
  maskFromPermissions,
} from '@/utils/constants'
import { buildFilter, eq, orderBy } from '@/utils/filter'
import { formatBytes, formatDateTime, formatNumber, formatPercent, toInt } from '@/utils/format'

const auth = useAuthStore()
const ui = useUiStore()
const system = useSystemStore()

const users = ref<User[]>([])
const loading = ref(false)
const error = ref('')
const errorKind = ref('')
const keyword = ref('')
const roleFilter = ref('')
const statusFilter = ref('')
const includeDeleted = ref(false)
const orderField = ref('created_at')
const orderDesc = ref(true)
const totalSize = ref(0)
const page = ref(1)
const pageSize = ref(20)
const tokenStack = ref<string[]>([''])
const nextToken = ref('')

const presets = ref<RolePreset[]>([])
const catalog = ref<PermissionCatalog | null>(null)
const maxGrantableRank = ref(0)

/* 创建账号 */
const createOpen = ref(false)
const createBusy = ref(false)
const createError = ref('')
const createForm = ref({
  username: '',
  nickname: '',
  email: '',
  password: '',
  mustChangePassword: true,
  role: 'ROLE_GUEST',
  rank: 10,
  permissions: [] as Permission[],
  quotaGb: 0,
  remark: '',
  status: 'USER_STATUS_ACTIVE',
})

/* 编辑账号 */
const editOpen = ref(false)
const editBusy = ref(false)
const editError = ref('')
const editTarget = ref<User | null>(null)
const editForm = ref({
  nickname: '',
  email: '',
  avatarUrl: '',
  remark: '',
  role: 'ROLE_USER',
  rank: 100,
  quotaGb: 0,
  status: 'USER_STATUS_ACTIVE',
})

/* 权限 */
const permOpen = ref(false)
const permBusy = ref(false)
const permError = ref('')
const permTarget = ref<User | null>(null)
const permSelection = ref<Permission[]>([])

/* 重置密码 */
const resetOpen = ref(false)
const resetBusy = ref(false)
const resetError = ref('')
const resetTarget = ref<User | null>(null)
const resetForm = ref({ password: '', mustChangePassword: true })

/* 统计抽屉 */
const statsOpen = ref(false)
const statsTarget = ref<User | null>(null)
const stats = ref<UserStats | null>(null)
const statsLoading = ref(false)

const roleOptions = [
  { value: '', label: '全部角色' },
  { value: 'ROLE_ADMIN', label: '超级管理员' },
  { value: 'ROLE_MANAGER', label: '管理员' },
  { value: 'ROLE_USER', label: '普通用户' },
  { value: 'ROLE_GUEST', label: '访客' },
]

const statusOptions = [
  { value: '', label: '全部状态' },
  { value: 'USER_STATUS_ACTIVE', label: '正常' },
  { value: 'USER_STATUS_DISABLED', label: '已禁用' },
  { value: 'USER_STATUS_DELETED', label: '已删除' },
]

const orderOptions = [
  { value: 'created_at', label: '按创建时间' },
  { value: 'last_login_at', label: '按最近登录' },
  { value: 'used_bytes', label: '按已用空间' },
  { value: 'rank', label: '按等级' },
  { value: 'username', label: '按用户名' },
]

const roleNumber: Record<string, number> = {
  ROLE_ADMIN: 1,
  ROLE_MANAGER: 2,
  ROLE_USER: 3,
  ROLE_GUEST: 4,
}

const statusNumber: Record<string, number> = {
  USER_STATUS_ACTIVE: 1,
  USER_STATUS_DISABLED: 2,
  USER_STATUS_DELETED: 3,
}

/** 可授予的权限 = 调用方自己持有的权限。 */
const grantablePermissions = computed(() => catalog.value?.granted ?? PERMISSION_META.map((meta) => meta.name))

const canCreateAdmin = computed(() => auth.isSuperAdmin)

function buildQueryFilter(): string | undefined {
  return buildFilter(
    keyword.value.trim() ? `(username:"${keyword.value.trim()}" OR nickname:"${keyword.value.trim()}" OR email:"${keyword.value.trim()}")` : undefined,
    roleFilter.value ? eq('role', roleNumber[roleFilter.value] ?? 0) : undefined,
    statusFilter.value ? eq('status', statusNumber[statusFilter.value] ?? 0) : undefined,
  )
}

function quotaRatio(user: User): number {
  const quota = toInt(user.quotaBytes)
  if (quota <= 0) return 0
  return Math.min(1, toInt(user.usedBytes) / quota)
}

async function load(reset = false): Promise<void> {
  if (reset) {
    page.value = 1
    tokenStack.value = ['']
  }
  loading.value = true
  error.value = ''
  errorKind.value = ''
  try {
    const result = await usersApi.listUsers({
      pageSize: pageSize.value,
      pageToken: tokenStack.value[page.value - 1] || undefined,
      filter: buildQueryFilter(),
      orderBy: orderBy(orderField.value, orderDesc.value),
      includeDeleted: includeDeleted.value,
    })
    users.value = result.users ?? []
    totalSize.value = toInt(result.totalSize, users.value.length)
    nextToken.value = result.nextPageToken ?? ''
  } catch (err) {
    error.value = errorText(err)
    users.value = []
    // 若 /v1/users/list 被 /v1/users/{id} 抢先匹配，服务端会返回
    // NETDISK_INVALID_ARGUMENT（把 "list" 当成非法 id），这里给出可执行的提示。
    if (err instanceof ApiError && err.reason === 'NETDISK_INVALID_ARGUMENT') {
      errorKind.value = 'route-shadowed'
    }
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

function onPageSize(size: number): void {
  pageSize.value = size
  void load(true)
}

/* ---------- 创建 ---------- */

function applyPreset(role: string): void {
  const preset = presets.value.find((item) => item.role === role)
  if (!preset) return
  createForm.value.rank = preset.defaultRank ?? 10
  createForm.value.permissions = (preset.permissions ?? []).filter((permission) =>
    grantablePermissions.value.includes(permission),
  )
}

function openCreate(): void {
  createForm.value = {
    username: '',
    nickname: '',
    email: '',
    password: '',
    mustChangePassword: true,
    role: 'ROLE_GUEST',
    rank: 10,
    permissions: [],
    quotaGb: 0,
    remark: '',
    status: 'USER_STATUS_ACTIVE',
  }
  createError.value = ''
  applyPreset('ROLE_GUEST')
  createOpen.value = true
}

function toggleCreatePermission(permission: Permission, checked: boolean): void {
  const set = new Set(createForm.value.permissions)
  if (checked) set.add(permission)
  else set.delete(permission)
  createForm.value.permissions = Array.from(set)
}

async function submitCreate(): Promise<void> {
  createError.value = ''
  if (!createForm.value.username.trim()) {
    createError.value = '请填写用户名（4-32 位字母、数字、下划线）'
    return
  }
  if ([...createForm.value.password].length < system.minPasswordLength) {
    createError.value = `初始密码至少 ${system.minPasswordLength} 位`
    return
  }
  createBusy.value = true
  try {
    await usersApi.createUser({
      username: createForm.value.username.trim(),
      password: createForm.value.password,
      nickname: createForm.value.nickname.trim() || undefined,
      email: createForm.value.email.trim() || undefined,
      role: createForm.value.role,
      rank: createForm.value.rank,
      permissions: createForm.value.permissions,
      quotaBytes: Math.max(0, Math.round(createForm.value.quotaGb * 1024 * 1024 * 1024)),
      remark: createForm.value.remark.trim() || undefined,
      status: createForm.value.status,
      mustChangePassword: createForm.value.mustChangePassword,
    })
    ui.toast.success('账号已创建', createForm.value.username)
    createOpen.value = false
    await load(true)
  } catch (err) {
    createError.value = errorText(err)
  } finally {
    createBusy.value = false
  }
}

/* ---------- 编辑 ---------- */

function openEdit(user: User): void {
  editTarget.value = user
  editError.value = ''
  editForm.value = {
    nickname: user.nickname ?? '',
    email: user.email ?? '',
    avatarUrl: user.avatarUrl ?? '',
    remark: user.remark ?? '',
    role: user.role ?? 'ROLE_USER',
    rank: user.rank ?? 100,
    quotaGb: toInt(user.quotaBytes) / (1024 * 1024 * 1024),
    status: user.status ?? 'USER_STATUS_ACTIVE',
  }
  editOpen.value = true
}

async function submitEdit(): Promise<void> {
  const user = editTarget.value
  if (!user?.id) return
  editBusy.value = true
  editError.value = ''
  try {
    await usersApi.updateUser({
      id: user.id,
      fields: {
        nickname: editForm.value.nickname,
        email: editForm.value.email,
        avatarUrl: editForm.value.avatarUrl,
        remark: editForm.value.remark,
        role: editForm.value.role as User['role'],
        rank: editForm.value.rank,
        quotaBytes: Math.max(0, Math.round(editForm.value.quotaGb * 1024 * 1024 * 1024)),
        status: editForm.value.status as User['status'],
      },
      updateMask: ['nickname', 'email', 'avatarUrl', 'remark', 'role', 'rank', 'quotaBytes', 'status'],
    })
    ui.toast.success('账号已更新')
    editOpen.value = false
    await load()
  } catch (err) {
    editError.value = errorText(err)
  } finally {
    editBusy.value = false
  }
}

/* ---------- 权限 ---------- */

function openPermissions(user: User): void {
  permTarget.value = user
  permError.value = ''
  permSelection.value = (user.permissions ?? []).slice()
  permOpen.value = true
}

function togglePerm(permission: Permission, checked: boolean): void {
  const set = new Set(permSelection.value)
  if (checked) set.add(permission)
  else set.delete(permission)
  permSelection.value = Array.from(set)
}

async function submitPermissions(): Promise<void> {
  const user = permTarget.value
  if (!user?.id) return
  permBusy.value = true
  permError.value = ''
  try {
    await usersApi.setUserPermissions(user.id, permSelection.value, maskFromPermissions(permSelection.value))
    ui.toast.success('权限已更新')
    permOpen.value = false
    await load()
  } catch (err) {
    permError.value = errorText(err)
  } finally {
    permBusy.value = false
  }
}

/* ---------- 重置密码 ---------- */

function openReset(user: User): void {
  resetTarget.value = user
  resetForm.value = { password: '', mustChangePassword: true }
  resetError.value = ''
  resetOpen.value = true
}

async function submitReset(): Promise<void> {
  const user = resetTarget.value
  if (!user?.id) return
  if ([...resetForm.value.password].length < system.minPasswordLength) {
    resetError.value = `新密码至少 ${system.minPasswordLength} 位`
    return
  }
  resetBusy.value = true
  try {
    await usersApi.resetUserPassword(user.id, resetForm.value.password, resetForm.value.mustChangePassword)
    ui.toast.success('密码已重置', '该账号的其它会话已被吊销')
    resetOpen.value = false
  } catch (err) {
    resetError.value = errorText(err)
  } finally {
    resetBusy.value = false
  }
}

/* ---------- 统计与删除 ---------- */

async function openStats(user: User): Promise<void> {
  statsTarget.value = user
  stats.value = null
  statsOpen.value = true
  statsLoading.value = true
  try {
    stats.value = await usersApi.getUserStats(user.id ?? '')
  } catch (err) {
    ui.toast.error('统计加载失败', errorText(err))
  } finally {
    statsLoading.value = false
  }
}

async function removeUser(user: User, trashNodes: boolean): Promise<void> {
  const ok = await ui.confirm({
    title: '删除账号',
    message: `「${user.nickname || user.username}」会被软删除并吊销全部会话。${
      trashNodes ? '其名下节点会一并移入回收站。' : '其名下节点会保留在原处。'
    }`,
    tone: 'danger',
    confirmText: '删除账号',
    requireText: user.username,
  })
  if (!ok) return
  try {
    await usersApi.deleteUser(user.id ?? '', trashNodes)
    ui.toast.success('账号已删除')
    await load()
  } catch (err) {
    ui.toast.error('删除失败', errorText(err))
  }
}

async function onRowAction(payload: { key: string; user: User }): Promise<void> {
  const { key, user } = payload
  switch (key) {
    case 'edit':
      openEdit(user)
      break
    case 'permissions':
      openPermissions(user)
      break
    case 'reset':
      openReset(user)
      break
    case 'stats':
      await openStats(user)
      break
    case 'delete':
      await removeUser(user, false)
      break
    case 'delete-trash':
      await removeUser(user, true)
      break
    case 'disable':
      await setStatus(user, 'USER_STATUS_DISABLED')
      break
    case 'enable':
      await setStatus(user, 'USER_STATUS_ACTIVE')
      break
    default:
      break
  }
}

async function setStatus(user: User, status: string): Promise<void> {
  try {
    await usersApi.updateUser({ id: user.id ?? '', fields: { status: status as User['status'] }, updateMask: ['status'] })
    ui.toast.success(status === 'USER_STATUS_ACTIVE' ? '账号已启用' : '账号已禁用')
    await load()
  } catch (err) {
    ui.toast.error('操作失败', errorText(err))
  }
}

function rowActions(user: User): DropdownItem[] {
  const manageable = user.manageable !== false
  const items: DropdownItem[] = [
    { key: 'edit', label: '编辑资料', icon: 'edit', disabled: !manageable },
    { key: 'permissions', label: '设置权限', icon: 'shield', disabled: !manageable || user.permissionsEditable === false },
    { key: 'reset', label: '重置密码', icon: 'key', disabled: !manageable },
    { key: 'stats', label: '用量统计', icon: 'chart-bar' },
    { key: 'divider', divider: true },
  ]
  if (user.status === 'USER_STATUS_ACTIVE') {
    items.push({ key: 'disable', label: '禁用账号', icon: 'lock', disabled: !manageable })
  } else if (user.status === 'USER_STATUS_DISABLED') {
    items.push({ key: 'enable', label: '启用账号', icon: 'unlock', disabled: !manageable })
  }
  items.push({ key: 'delete', label: '删除账号', icon: 'trash', danger: true, disabled: !manageable })
  items.push({ key: 'delete-trash', label: '删除并回收文件', icon: 'trash', danger: true, disabled: !manageable })
  return items
}

watch([keyword, roleFilter, statusFilter, includeDeleted, orderField, orderDesc], () => void load(true))

onMounted(async () => {
  const [presetResult, catalogResult] = await Promise.all([
    usersApi.listRolePresets().catch(() => null),
    usersApi.listPermissionCatalog().catch(() => null),
  ])
  presets.value = presetResult?.presets ?? []
  maxGrantableRank.value = presetResult?.maxGrantableRank ?? 0
  catalog.value = catalogResult
  await load()
})
</script>

<template>
  <div class="users">
    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">账号列表</p>
          <p class="card__subtitle">
            共 {{ formatNumber(totalSize) }} 个账号 · 你的等级 {{ auth.user?.rank }} · 可授予的最高等级
            {{ maxGrantableRank }}
          </p>
        </div>
        <div class="flex gap-2">
          <AppButton size="sm" variant="ghost" icon="refresh" label="刷新" @click="load()" />
          <AppButton size="sm" variant="primary" icon="user-plus" @click="openCreate">创建账号</AppButton>
        </div>
      </header>

      <div class="users__toolbar">
        <AppInput
          v-model="keyword"
          size="sm"
          icon="search"
          clearable
          placeholder="用户名 / 昵称 / 邮箱"
          style="width: 220px"
        />
        <AppSelect v-model="roleFilter" size="sm" :options="roleOptions" style="width: 140px" />
        <AppSelect v-model="statusFilter" size="sm" :options="statusOptions" style="width: 140px" />
        <label class="flex items-center gap-2 text-xs muted">
          <AppSwitch v-model="includeDeleted" label="包含已删除账号" />
          包含已删除
        </label>
        <span class="toolbar__spacer" />
        <AppSelect v-model="orderField" size="sm" :options="orderOptions" style="width: 150px" />
        <AppButton size="sm" variant="ghost" :icon="orderDesc ? 'arrow-down' : 'arrow-up'" @click="orderDesc = !orderDesc">
          {{ orderDesc ? '降序' : '升序' }}
        </AppButton>
      </div>

      <div v-if="errorKind === 'route-shadowed'" class="users__hint">
        <AppIcon name="alert-triangle" :size="16" />
        <div>
          <p class="strong">服务端路由冲突：GET /v1/users/list 被 GET /v1/users/{id} 抢先匹配</p>
          <p class="text-xs muted">
            返回 NETDISK_INVALID_ARGUMENT 说明 "list" 被当成了非法的账号 id。该问题已在
            <span class="mono">api/netdisk/v1/user.proto</span> 中修正（把
            <span class="mono">rpc ListUsers</span> 挪到 <span class="mono">rpc GetUser</span> 之前），
            如果你仍看到这个提示，说明正在运行的后端二进制是修复前的版本：重新执行
            <span class="mono">.\scripts\build.ps1 build</span> 并重启服务即可。
          </p>
        </div>
      </div>

      <div class="table-wrap">
        <table class="table">
          <thead>
            <tr>
              <th>账号</th>
              <th style="width: 110px">角色</th>
              <th style="width: 80px">等级</th>
              <th style="width: 100px">状态</th>
              <th style="width: 190px">配额使用</th>
              <th style="width: 150px">最近登录</th>
              <th style="width: 60px" />
            </tr>
          </thead>
          <tbody>
            <tr v-if="loading">
              <td colspan="7" class="users__state muted text-sm">加载中…</td>
            </tr>
            <tr v-else-if="users.length === 0">
              <td colspan="7" class="users__state">
                <AppEmpty size="sm" :icon="error ? 'alert-circle' : 'users'" :title="error ? '加载失败' : '没有账号'">
                  <AppButton v-if="error" size="sm" icon="refresh" @click="load()">重试</AppButton>
                </AppEmpty>
              </td>
            </tr>
            <template v-else>
              <tr v-for="user in users" :key="user.id" class="is-hoverable">
                <td>
                  <div class="users__identity">
                    <AppAvatar :name="user.nickname || user.username" :src="user.avatarUrl" :size="30" />
                    <div class="users__identity-text">
                      <p class="users__name truncate">
                        {{ user.nickname || user.username }}
                        <AppBadge v-if="user.id === auth.user?.id" tone="brand" size="sm">我</AppBadge>
                        <AppBadge v-if="user.mustChangePassword" tone="warning" size="sm">待改密</AppBadge>
                      </p>
                      <p class="users__sub truncate">
                        @{{ user.username }}<template v-if="user.email"> · {{ user.email }}</template>
                      </p>
                    </div>
                  </div>
                </td>
                <td>
                  <AppBadge :tone="ROLE_TONE[user.role ?? 'ROLE_UNSPECIFIED']">
                    {{ ROLE_LABEL[user.role ?? 'ROLE_UNSPECIFIED'] }}
                  </AppBadge>
                </td>
                <td class="is-num">{{ user.rank }}</td>
                <td>
                  <AppBadge :tone="USER_STATUS_TONE[user.status ?? 'USER_STATUS_UNSPECIFIED']">
                    {{ USER_STATUS_LABEL[user.status ?? 'USER_STATUS_UNSPECIFIED'] }}
                  </AppBadge>
                </td>
                <td>
                  <AppProgress
                    :value="quotaRatio(user) * 100"
                    :height="4"
                    :tone="quotaRatio(user) > 0.9 ? 'danger' : 'brand'"
                  />
                  <p class="text-xs muted mt-1">
                    {{ formatBytes(user.usedBytes) }} /
                    {{ toInt(user.quotaBytes) > 0 ? formatBytes(user.quotaBytes) : '不限' }}
                    <template v-if="toInt(user.quotaBytes) > 0">（{{ formatPercent(quotaRatio(user)) }}）</template>
                  </p>
                </td>
                <td class="text-xs muted">{{ formatDateTime(user.lastLoginAt) }}</td>
                <td class="is-actions">
                  <AppDropdown :items="rowActions(user)" :width="200" @select="(key) => onRowAction({ key, user })">
                    <template #trigger>
                      <button type="button" class="users__more" aria-label="账号操作">
                        <AppIcon name="more-vertical" :size="16" />
                      </button>
                    </template>
                  </AppDropdown>
                </td>
              </tr>
            </template>
          </tbody>
        </table>
      </div>

      <AppPagination
        :page="page"
        :page-size="pageSize"
        :total="totalSize"
        :page-sizes="[20, 50, 100]"
        :has-prev="page > 1"
        :has-next="Boolean(nextToken)"
        :disabled="loading"
        @prev="prevPage"
        @next="nextPage"
        @update:page-size="onPageSize"
      />
    </section>

    <!-- 创建账号 -->
    <AppDialog v-model="createOpen" title="创建账号" description="等级必须低于你自己；权限只能是你的子集。" size="lg">
      <div class="form-grid">
        <AppField label="用户名" required hint="4-32 位，字母开头的字母数字与 _ . -">
          <AppInput v-model="createForm.username" placeholder="例如 alice" />
        </AppField>
        <AppField label="初始密码" required :hint="`至少 ${system.minPasswordLength} 位，浏览器内加密后提交`">
          <AppInput v-model="createForm.password" type="password" placeholder="初始密码" />
        </AppField>
        <AppField label="昵称">
          <AppInput v-model="createForm.nickname" placeholder="显示名称" />
        </AppField>
        <AppField label="邮箱">
          <AppInput v-model="createForm.email" type="email" placeholder="可选" />
        </AppField>
        <AppField label="角色预设" hint="切换角色会套用该预设的等级与权限">
          <AppSelect
            v-model="createForm.role"
            :options="[
              { value: 'ROLE_GUEST', label: '访客（只读）' },
              { value: 'ROLE_USER', label: '普通用户' },
              { value: 'ROLE_MANAGER', label: '管理员' },
              ...(canCreateAdmin ? [{ value: 'ROLE_ADMIN', label: '超级管理员' }] : []),
            ]"
            @update:model-value="applyPreset"
          />
        </AppField>
        <AppField label="等级（rank）" :hint="`必须严格小于 ${auth.user?.rank}，且不超过 ${maxGrantableRank}`">
          <AppInput v-model="createForm.rank" type="number" :min="1" :max="maxGrantableRank" />
        </AppField>
        <AppField label="配额（GB）" hint="0 表示不限制">
          <AppInput v-model="createForm.quotaGb" type="number" :min="0" step="1" />
        </AppField>
        <AppField label="状态">
          <AppSelect
            v-model="createForm.status"
            :options="[
              { value: 'USER_STATUS_ACTIVE', label: '正常' },
              { value: 'USER_STATUS_DISABLED', label: '禁用' },
            ]"
          />
        </AppField>
        <AppField label="备注">
          <AppInput v-model="createForm.remark" placeholder="管理员可见的说明" />
        </AppField>
        <div class="field form-row-full">
          <span class="field__label">权限（{{ createForm.permissions.length }} 项）</span>
          <div class="users__perms">
            <AppCheckbox
              v-for="meta in PERMISSION_META"
              :key="meta.name"
              :model-value="createForm.permissions.includes(meta.name)"
              :disabled="!grantablePermissions.includes(meta.name)"
              :label="meta.display"
              @update:model-value="toggleCreatePermission(meta.name, $event)"
            />
          </div>
          <span class="field__hint">灰掉的权限是你自己也没有的，无法授予他人。</span>
        </div>
        <AppField label="要求首次登录修改密码" inline>
          <AppSwitch v-model="createForm.mustChangePassword" label="要求首次登录修改密码" />
        </AppField>
      </div>
      <p v-if="createError" class="field__error mt-3">{{ createError }}</p>
      <template #footer>
        <AppButton variant="ghost" @click="createOpen = false">取消</AppButton>
        <AppButton variant="primary" :loading="createBusy" icon="user-plus" @click="submitCreate">创建</AppButton>
      </template>
    </AppDialog>

    <!-- 编辑账号 -->
    <AppDialog v-model="editOpen" title="编辑账号" :description="editTarget?.username" size="lg">
      <div class="form-grid">
        <AppField label="昵称">
          <AppInput v-model="editForm.nickname" />
        </AppField>
        <AppField label="邮箱">
          <AppInput v-model="editForm.email" type="email" />
        </AppField>
        <AppField label="头像地址">
          <AppInput v-model="editForm.avatarUrl" placeholder="https://…" />
        </AppField>
        <AppField label="角色">
          <AppSelect
            v-model="editForm.role"
            :options="[
              { value: 'ROLE_GUEST', label: '访客' },
              { value: 'ROLE_USER', label: '普通用户' },
              { value: 'ROLE_MANAGER', label: '管理员' },
              ...(canCreateAdmin ? [{ value: 'ROLE_ADMIN', label: '超级管理员' }] : []),
            ]"
          />
        </AppField>
        <AppField label="等级（rank）" :hint="`必须严格小于 ${auth.user?.rank}`">
          <AppInput v-model="editForm.rank" type="number" :min="1" :max="maxGrantableRank" />
        </AppField>
        <AppField label="配额（GB）" hint="0 表示不限制">
          <AppInput v-model="editForm.quotaGb" type="number" :min="0" step="1" />
        </AppField>
        <AppField label="状态" hint="改为非正常状态会吊销该账号的全部会话">
          <AppSelect
            v-model="editForm.status"
            :options="[
              { value: 'USER_STATUS_ACTIVE', label: '正常' },
              { value: 'USER_STATUS_DISABLED', label: '禁用' },
            ]"
          />
        </AppField>
        <AppField label="备注">
          <AppInput v-model="editForm.remark" />
        </AppField>
      </div>
      <p v-if="editError" class="field__error mt-3">{{ editError }}</p>
      <template #footer>
        <AppButton variant="ghost" @click="editOpen = false">取消</AppButton>
        <AppButton variant="primary" :loading="editBusy" @click="submitEdit">保存</AppButton>
      </template>
    </AppDialog>

    <!-- 设置权限 -->
    <AppDialog v-model="permOpen" title="设置权限" :description="permTarget?.username" size="md">
      <p class="text-xs muted mb-3">
        整体替换该账号的权限集合；只能选择你自己持有的权限，留空表示清空权限。
      </p>
      <div class="users__perms users__perms--column">
        <label v-for="meta in PERMISSION_META" :key="meta.name" class="users__perm-row">
          <AppCheckbox
            :model-value="permSelection.includes(meta.name)"
            :disabled="!grantablePermissions.includes(meta.name)"
            @update:model-value="togglePerm(meta.name, $event)"
          >
            <span class="users__perm-title">{{ meta.display }}</span>
            <span class="users__perm-desc">{{ meta.description }}</span>
          </AppCheckbox>
        </label>
      </div>
      <p class="text-xs faint mt-3">掩码：{{ maskFromPermissions(permSelection) }}</p>
      <p v-if="permError" class="field__error mt-2">{{ permError }}</p>
      <template #footer>
        <AppButton variant="ghost" @click="permOpen = false">取消</AppButton>
        <AppButton variant="primary" :loading="permBusy" @click="submitPermissions">保存</AppButton>
      </template>
    </AppDialog>

    <!-- 重置密码 -->
    <AppDialog v-model="resetOpen" title="重置密码" :description="resetTarget?.username" size="sm">
      <AppField label="新密码" required :hint="`至少 ${system.minPasswordLength} 位；提交前在浏览器内加密`">
        <AppInput v-model="resetForm.password" type="password" />
      </AppField>
      <AppField label="要求首次登录修改密码" inline>
        <AppSwitch v-model="resetForm.mustChangePassword" label="要求首次登录修改密码" />
      </AppField>
      <p v-if="resetError" class="field__error mt-2">{{ resetError }}</p>
      <template #footer>
        <AppButton variant="ghost" @click="resetOpen = false">取消</AppButton>
        <AppButton variant="primary" :loading="resetBusy" icon="key" @click="submitReset">重置</AppButton>
      </template>
    </AppDialog>

    <!-- 用量统计 -->
    <AppDrawer v-model="statsOpen" :title="statsTarget?.nickname || statsTarget?.username || '用量统计'" size="sm">
      <div v-if="statsLoading" class="muted text-sm">加载中…</div>
      <div v-else-if="stats" class="users__stats">
        <div class="stat">
          <span class="stat__label">已用 / 配额</span>
          <span class="stat__value">{{ formatBytes(stats.usedBytes) }}</span>
          <span class="stat__hint">
            配额 {{ toInt(stats.quotaBytes) > 0 ? formatBytes(stats.quotaBytes) : '不限' }} ·
            {{ formatPercent(stats.usageRatio) }}
          </span>
        </div>
        <div class="stat">
          <span class="stat__label">文件 / 文件夹</span>
          <span class="stat__value">{{ formatNumber(stats.fileCount) }} / {{ formatNumber(stats.folderCount) }}</span>
        </div>
        <div class="stat">
          <span class="stat__label">回收站</span>
          <span class="stat__value">{{ formatBytes(stats.trashedBytes) }}</span>
          <span class="stat__hint">{{ formatNumber(stats.trashedCount) }} 个节点</span>
        </div>
        <div class="stat">
          <span class="stat__label">账号 ID</span>
          <span class="stat__value mono text-xs break-all">{{ stats.userId }}</span>
        </div>
      </div>
      <AppEmpty v-else size="sm" icon="chart-bar" title="没有统计数据" />
    </AppDrawer>
  </div>
</template>

<style scoped>
.users {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.users__toolbar {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3) var(--space-5);
  border-bottom: 1px solid var(--border-subtle);
  flex-wrap: wrap;
}

.users__state {
  padding: var(--space-6) !important;
  text-align: center;
}

.users__hint {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  margin: 0 var(--space-5) var(--space-4);
  padding: var(--space-3);
  border-radius: var(--radius-md);
  background: var(--warning-50);
  color: var(--warning-600);
}

.users__hint .strong {
  color: var(--warning-600);
}

.users__identity {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  min-width: 0;
}

.users__identity-text {
  min-width: 0;
}

.users__name {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text-strong);
}

.users__sub {
  font-size: var(--text-2xs);
  color: var(--text-muted);
}

.users__more {
  width: 28px;
  height: 28px;
  border: 0;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--text-muted);
  cursor: pointer;
  font-size: 18px;
  line-height: 1;
}

.users__more:hover {
  background: var(--surface-3);
  color: var(--text);
}

.users__perms {
  display: flex;
  align-items: center;
  gap: var(--space-3) var(--space-4);
  flex-wrap: wrap;
}

.users__perms--column {
  flex-direction: column;
  align-items: stretch;
  gap: var(--space-2);
}

.users__perm-row {
  display: block;
}

.users__perm-title {
  font-size: var(--text-sm);
  font-weight: 600;
  color: var(--text);
  display: block;
}

.users__perm-desc {
  font-size: var(--text-2xs);
  color: var(--text-muted);
  display: block;
}

.users__stats {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
</style>
