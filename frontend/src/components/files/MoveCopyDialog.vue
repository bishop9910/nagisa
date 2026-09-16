<script setup lang="ts">
/** 移动 / 复制到指定目录：内置一棵只显示文件夹的浏览器。 */
import { computed, ref, watch } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppSelect from '@/components/ui/AppSelect.vue'
import FileIcon from './FileIcon.vue'
import { errorText, nodesApi } from '@/api'
import type { ConflictPolicy, Node } from '@/api/types'
import { CONFLICT_POLICY_OPTIONS } from '@/utils/constants'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{
  modelValue: boolean
  mode: 'move' | 'copy'
  nodes: Node[]
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'done', nodes: Node[]): void
}>()

const ui = useUiStore()

const currentId = ref('')
const ancestors = ref<Node[]>([])
const folders = ref<Node[]>([])
const loading = ref(false)
const error = ref('')
const conflictPolicy = ref<ConflictPolicy>('CONFLICT_POLICY_FAIL')

const title = computed(() => (props.mode === 'move' ? '移动到' : '复制到'))
const sourceIds = computed(() => props.nodes.map((node) => node.id ?? '').filter(Boolean))
/** 被移动的文件夹自身不能作为目标。 */
const blockedIds = computed(() => new Set(props.mode === 'move' ? sourceIds.value : []))
const targetLabel = computed(() => {
  if (!currentId.value) return '我的网盘'
  return ancestors.value[ancestors.value.length - 1]?.name ?? '目标目录'
})
const canSubmit = computed(() => {
  const target = currentId.value
  if (props.mode === 'move') {
    // 移动到自己所在的目录是空操作，但仍然合法；只是不能移进自身子树。
    if (blockedIds.value.has(target)) return false
  }
  return sourceIds.value.length > 0 && !loading.value
})

async function load(id: string): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    const page = await nodesApi.listNodes({
      parentId: id,
      pageSize: 200,
      filter: 'kind=1',
      orderBy: 'name',
    })
    folders.value = (page.nodes ?? []).filter((node) => node.kind === 'NODE_KIND_FOLDER')
  } catch (err) {
    error.value = errorText(err)
    folders.value = []
  } finally {
    loading.value = false
  }
}

async function enter(folder: Node): Promise<void> {
  if (blockedIds.value.has(folder.id ?? '')) {
    ui.toast.warning('不能选择该目录', '该目录位于要移动的文件夹内部')
    return
  }
  ancestors.value = [...ancestors.value, folder]
  currentId.value = folder.id ?? ''
  await load(currentId.value)
}

async function goUp(index: number): Promise<void> {
  if (index < 0) {
    ancestors.value = []
    currentId.value = ''
  } else {
    const parent = ancestors.value[index]
    ancestors.value = ancestors.value.slice(0, index + 1)
    currentId.value = parent?.id ?? ''
  }
  await load(currentId.value)
}

async function submit(): Promise<void> {
  if (!canSubmit.value) return
  loading.value = true
  try {
    const payload = {
      ids: sourceIds.value,
      targetParentId: currentId.value || undefined,
      conflictPolicy: conflictPolicy.value,
    }
    const result = props.mode === 'move' ? await nodesApi.moveNodes(payload) : await nodesApi.copyNodes(payload)
    ui.toast.success(props.mode === 'move' ? '已移动' : '已复制', `共 ${sourceIds.value.length} 项`)
    emit('done', result.nodes ?? [])
    emit('update:modelValue', false)
  } catch (err) {
    error.value = errorText(err)
  } finally {
    loading.value = false
  }
}

watch(
  () => props.modelValue,
  async (open) => {
    if (!open) return
    currentId.value = ''
    ancestors.value = []
    error.value = ''
    conflictPolicy.value = props.mode === 'move' ? 'CONFLICT_POLICY_FAIL' : 'CONFLICT_POLICY_RENAME'
    await load('')
  },
)
</script>

<template>
  <AppDialog
    :model-value="modelValue"
    :title="title"
    :description="`已选择 ${sourceIds.length} 项，请选择目标目录`"
    size="md"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="picker">
      <nav class="picker__crumbs">
        <button type="button" class="picker__crumb" @click="goUp(-1)">我的网盘</button>
        <template v-for="(node, index) in ancestors" :key="node.id">
          <AppIcon name="chevron-right" :size="13" class="faint" />
          <button type="button" class="picker__crumb" @click="goUp(index)">{{ node.name }}</button>
        </template>
      </nav>

      <div class="picker__list">
        <p v-if="loading" class="picker__state muted text-xs">加载中…</p>
        <p v-else-if="folders.length === 0" class="picker__state muted text-xs">当前目录下没有子文件夹</p>
        <button
          v-for="folder in folders"
          v-else
          :key="folder.id"
          type="button"
          class="picker__item"
          :disabled="blockedIds.has(folder.id ?? '')"
          @click="enter(folder)"
        >
          <FileIcon :node="folder" :size="16" />
          <span class="truncate">{{ folder.name }}</span>
          <AppIcon name="chevron-right" :size="14" class="faint" />
        </button>
      </div>

      <div class="picker__foot">
        <span class="text-xs muted">目标：<strong class="strong">{{ targetLabel }}</strong></span>
        <AppSelect
          v-model="conflictPolicy"
          size="sm"
          :options="CONFLICT_POLICY_OPTIONS.map((option) => ({ value: option.value, label: option.label }))"
        />
      </div>
      <p v-if="error" class="field__error">{{ error }}</p>
    </div>

    <template #footer>
      <AppButton variant="ghost" @click="emit('update:modelValue', false)">取消</AppButton>
      <AppButton variant="primary" :loading="loading" :disabled="!canSubmit" @click="submit">
        确定{{ mode === 'move' ? '移动' : '复制' }}
      </AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.picker {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.picker__crumbs {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  flex-wrap: wrap;
  font-size: var(--text-xs);
}

.picker__crumb {
  border: 0;
  background: var(--surface-2);
  color: var(--text-muted);
  padding: 3px var(--space-2);
  border-radius: var(--radius-sm);
  cursor: pointer;
  max-width: 160px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.picker__crumb:hover {
  background: var(--surface-3);
  color: var(--text);
}

.picker__list {
  height: 240px;
  overflow-y: auto;
  border: 1px solid var(--border);
  border-radius: var(--radius-md);
  padding: var(--space-1);
  background: var(--surface-2);
}

.picker__item {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
  border: 0;
  background: transparent;
  padding: var(--space-2) var(--space-3);
  border-radius: var(--radius-sm);
  cursor: pointer;
  font-size: var(--text-sm);
  color: var(--text);
  text-align: left;
}

.picker__item:hover:not(:disabled) {
  background: var(--surface);
}

.picker__item:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}

.picker__state {
  padding: var(--space-4);
  text-align: center;
}

.picker__foot {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  flex-wrap: wrap;
}
</style>
