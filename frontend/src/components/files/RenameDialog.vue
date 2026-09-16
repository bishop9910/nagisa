<script setup lang="ts">
/** 重命名对话框。 */
import { ref, watch } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppInput from '@/components/ui/AppInput.vue'
import { errorText, nodesApi } from '@/api'
import type { Node } from '@/api/types'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{
  modelValue: boolean
  node?: Node | null
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'renamed', node: Node): void
}>()

const ui = useUiStore()
const name = ref('')
const error = ref('')
const loading = ref(false)

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      name.value = props.node?.name ?? ''
      error.value = ''
    }
  },
)

async function submit(): Promise<void> {
  const trimmed = name.value.trim()
  if (!trimmed) {
    error.value = '名称不能为空'
    return
  }
  if (!props.node?.id) return
  if (trimmed === props.node.name) {
    emit('update:modelValue', false)
    return
  }
  loading.value = true
  try {
    const updated = await nodesApi.updateNode({
      id: props.node.id,
      updateMask: ['name'],
      fields: { name: trimmed },
      conflictPolicy: 'CONFLICT_POLICY_FAIL',
    })
    ui.toast.success('已重命名')
    emit('renamed', updated)
    emit('update:modelValue', false)
  } catch (err) {
    error.value = errorText(err)
  } finally {
    loading.value = false
  }
}
</script>

<template>
  <AppDialog
    :model-value="modelValue"
    title="重命名"
    size="sm"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <AppInput v-model="name" size="lg" :invalid="Boolean(error)" placeholder="新名称" @enter="submit" />
    <p v-if="error" class="field__error mt-2">{{ error }}</p>
    <template #footer>
      <AppButton variant="ghost" @click="emit('update:modelValue', false)">取消</AppButton>
      <AppButton variant="primary" :loading="loading" @click="submit">保存</AppButton>
    </template>
  </AppDialog>
</template>
