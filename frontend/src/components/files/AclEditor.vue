<script setup lang="ts">
/**
 * 访问名单编辑器。
 *
 * ACL 的语义要点（与 docs/permissions.md 一致）：
 *  - 只有内容类权限（view/download/upload/edit/delete/trash_manage/share/acl_manage）能写进条目；
 *  - 提交是整体替换，空数组表示清空；
 *  - 拒绝条目无论 inherit 与否都作用于整棵子树，允许条目只有 inherit 才向下继承；
 *  - 条目权限必须是调用方自身权限的子集，因此没有的权限会被禁用。
 */
import { computed, ref, watch } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import AppCheckbox from '@/components/ui/AppCheckbox.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import AppSwitch from '@/components/ui/AppSwitch.vue'
import { nodesApi, errorText } from '@/api'
import type { AclEntry, NodeAcl, Permission, SubjectType } from '@/api/types'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import {
  EFFECT_LABEL,
  PERMISSION_META,
  SUBJECT_TYPE_LABEL,
  maskFromPermissions,
  permissionsFromMask,
} from '@/utils/constants'
import { toInt } from '@/utils/format'

const props = defineProps<{
  nodeId: string
  acl?: NodeAcl | null
}>()

const emit = defineEmits<{ (e: 'saved', acl: NodeAcl): void }>()

const auth = useAuthStore()
const ui = useUiStore()

/** 服务端 GetNodeAcl 的 editable 字段：是否持有该节点的 acl_manage。 */
const editable = computed(() => props.acl?.editable !== false)
const nodeScope = PERMISSION_META.filter((meta) => meta.category === 'content')

interface DraftEntry {
  key: number
  subjectType: SubjectType
  subjectId: string
  effect: 'EFFECT_ALLOW' | 'EFFECT_DENY'
  permissions: Permission[]
  inherit: boolean
}

const drafts = ref<DraftEntry[]>([])
const inherited = computed(() => props.acl?.inheritedEntries ?? [])
const recursive = ref(false)
const saving = ref(false)
const error = ref('')

let seq = 0

function toDraft(entry: AclEntry): DraftEntry {
  seq += 1
  return {
    key: seq,
    subjectType: entry.subjectType ?? 'SUBJECT_TYPE_EVERYONE',
    subjectId: entry.subjectId ?? '',
    effect: entry.effect === 'EFFECT_DENY' ? 'EFFECT_DENY' : 'EFFECT_ALLOW',
    permissions: entry.permissions ?? permissionsFromMask(toInt(entry.permissionsMask)),
    inherit: entry.inherit ?? false,
  }
}

watch(
  () => props.acl,
  (acl) => {
    drafts.value = (acl?.entries ?? []).map(toDraft)
    error.value = ''
  },
  { immediate: true },
)

function addEntry(): void {
  seq += 1
  drafts.value = [
    ...drafts.value,
    {
      key: seq,
      subjectType: 'SUBJECT_TYPE_USER',
      subjectId: '',
      effect: 'EFFECT_ALLOW',
      permissions: ['PERMISSION_VIEW'],
      inherit: true,
    },
  ]
}

function removeEntry(key: number): void {
  drafts.value = drafts.value.filter((entry) => entry.key !== key)
}

function togglePermission(entry: DraftEntry, permission: Permission, checked: boolean): void {
  const set = new Set(entry.permissions)
  if (checked) set.add(permission)
  else set.delete(permission)
  entry.permissions = Array.from(set)
}

/** 只有自己持有的权限才能写进名单。 */
function permissionDisabled(permission: Permission): boolean {
  return !auth.has(permission)
}

const subjectOptions = computed(() =>
  (Object.keys(SUBJECT_TYPE_LABEL) as SubjectType[])
    .filter((type) => type !== 'SUBJECT_TYPE_UNSPECIFIED')
    .map((type) => ({ value: type, label: SUBJECT_TYPE_LABEL[type] })),
)

function permissionText(entry: AclEntry): string {
  const names = entry.permissions ?? permissionsFromMask(toInt(entry.permissionsMask))
  return names.map((name) => PERMISSION_META.find((meta) => meta.name === name)?.display ?? name).join('、')
}

async function save(): Promise<void> {
  if (!editable.value) return
  const missingSubject = drafts.value.find(
    (entry) => entry.subjectType !== 'SUBJECT_TYPE_EVERYONE' && !entry.subjectId.trim(),
  )
  if (missingSubject) {
    error.value = '选择账号或角色时必须填写主体标识'
    return
  }
  const emptyEntry = drafts.value.find((entry) => entry.permissions.length === 0)
  if (emptyEntry) {
    error.value = '每一条名单都要至少选择一个权限'
    return
  }
  saving.value = true
  error.value = ''
  try {
    const entries: AclEntry[] = drafts.value.map((entry) => ({
      subjectType: entry.subjectType,
      subjectId: entry.subjectType === 'SUBJECT_TYPE_EVERYONE' ? '' : entry.subjectId.trim(),
      effect: entry.effect,
      permissions: entry.permissions,
      permissionsMask: maskFromPermissions(entry.permissions),
      inherit: entry.inherit,
    }))
    const updated = await nodesApi.setNodeAcl(props.nodeId, entries, recursive.value)
    ui.toast.success('访问名单已更新')
    emit('saved', updated)
  } catch (err) {
    error.value = errorText(err)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="acl">
    <div class="acl__head">
      <div>
        <h4 class="card__title">访问名单</h4>
        <p class="text-xs muted">提交时整体替换该节点的条目；拒绝条目对整棵子树生效。</p>
      </div>
      <div class="acl__head-actions">
        <label class="flex items-center gap-2 text-xs muted">
          <AppSwitch v-model="recursive" label="递归写入子树" />
          递归写入子树
        </label>
        <AppButton size="sm" icon="plus" :disabled="!editable" @click="addEntry">添加条目</AppButton>
      </div>
    </div>

    <p v-if="!editable" class="acl__notice">
      <AppIcon name="lock" :size="15" />
      当前账号在该节点上没有权限管理能力，只能查看。
    </p>

    <ul v-if="drafts.length > 0" class="acl__list">
      <li v-for="entry in drafts" :key="entry.key" class="acl__row">
        <div class="acl__row-main">
          <AppSelect v-model="entry.subjectType" size="sm" :options="subjectOptions" :disabled="!editable" />
          <AppInput
            v-if="entry.subjectType !== 'SUBJECT_TYPE_EVERYONE'"
            v-model="entry.subjectId"
            size="sm"
            :disabled="!editable"
            :placeholder="entry.subjectType === 'SUBJECT_TYPE_USER' ? '账号 ID' : '角色机器名 admin / manager / user / guest'"
          />
          <AppSelect
            v-model="entry.effect"
            size="sm"
            :disabled="!editable"
            :options="[
              { value: 'EFFECT_ALLOW', label: '允许' },
              { value: 'EFFECT_DENY', label: '拒绝' },
            ]"
          />
        </div>
        <div class="acl__perms">
          <AppCheckbox
            v-for="meta in nodeScope"
            :key="meta.name"
            :model-value="entry.permissions.includes(meta.name)"
            :disabled="!editable || permissionDisabled(meta.name)"
            :label="meta.display"
            @update:model-value="togglePermission(entry, meta.name, $event)"
          />
        </div>
        <div class="acl__row-foot">
          <label class="flex items-center gap-2 text-xs">
            <AppSwitch
              :model-value="entry.inherit"
              :disabled="!editable"
              label="向下继承"
              @update:model-value="entry.inherit = $event"
            />
            向下继承
            <span v-if="entry.effect === 'EFFECT_DENY'" class="faint">（拒绝条目始终作用于整棵子树）</span>
          </label>
          <AppButton
            size="sm"
            variant="ghost"
            icon="trash"
            label="删除条目"
            :disabled="!editable"
            @click="removeEntry(entry.key)"
          />
        </div>
      </li>
    </ul>
    <p v-else class="text-xs muted">当前节点没有直接的名单条目。</p>

    <div v-if="inherited.length > 0" class="acl__inherited">
      <h4 class="card__title">继承自祖先的条目</h4>
      <ul class="acl__inherited-list">
        <li v-for="entry in inherited" :key="entry.id ?? `${entry.subjectId}-${entry.effect}`">
          <span class="badge" :class="entry.effect === 'EFFECT_DENY' ? 'badge--danger' : 'badge--success'">
            {{ EFFECT_LABEL[entry.effect ?? 'EFFECT_UNSPECIFIED'] }}
          </span>
          <span class="text-xs">
            {{ SUBJECT_TYPE_LABEL[entry.subjectType ?? 'SUBJECT_TYPE_UNSPECIFIED'] }}
            <template v-if="entry.subjectName">· {{ entry.subjectName }}</template>
            <template v-else-if="entry.subjectId">· {{ entry.subjectId }}</template>
          </span>
          <span class="text-xs muted">{{ permissionText(entry) }}</span>
        </li>
      </ul>
    </div>

    <p v-if="error" class="field__error">{{ error }}</p>
    <div v-if="editable" class="acl__footer">
      <AppButton variant="primary" :loading="saving" icon="check" @click="save">保存名单</AppButton>
    </div>

    <p class="acl__hint">
      可写权限范围：查看 / 下载 / 上传 / 编辑 / 删除 / 回收站管理 / 分享 / 权限管理。管理类权限无法通过名单授予。
    </p>
  </div>
</template>

<style scoped>
.acl {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.acl__head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.acl__head-actions {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.acl__notice {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  background: var(--surface-2);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  font-size: var(--text-xs);
  color: var(--text-muted);
}

.acl__list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.acl__row {
  border: 1px solid var(--border-subtle);
  background: var(--surface-2);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.acl__row-main {
  display: grid;
  grid-template-columns: 140px 1fr 110px;
  gap: var(--space-2);
}

.acl__perms {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  flex-wrap: wrap;
}

.acl__row-foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.acl__inherited-list {
  list-style: none;
  margin: var(--space-2) 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.acl__inherited-list li {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  flex-wrap: wrap;
}

.acl__footer {
  display: flex;
  justify-content: flex-end;
}

.acl__hint {
  font-size: var(--text-2xs);
  color: var(--text-faint);
}

@media (max-width: 720px) {
  .acl__row-main {
    grid-template-columns: 1fr;
  }
}
</style>
