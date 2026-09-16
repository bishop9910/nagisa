<script setup lang="ts">
/** 受密码保护目录的解锁对话框：失败提示、密钥提示与自动重试。 */
import { ref, watch } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import AppDialog from '@/components/ui/AppDialog.vue'
import AppInput from '@/components/ui/AppInput.vue'
import { nodesApi, errorText } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'

const props = defineProps<{
  modelValue: boolean
  nodeId: string
  nodeName?: string
  hint?: string
}>()

const emit = defineEmits<{
  (e: 'update:modelValue', value: boolean): void
  (e: 'unlocked', nodeId: string): void
}>()

const auth = useAuthStore()
const ui = useUiStore()

const password = ref('')
const error = ref('')
const loading = ref(false)

watch(
  () => props.modelValue,
  (open) => {
    if (open) {
      password.value = ''
      error.value = ''
    }
  },
)

async function submit(): Promise<void> {
  if (!password.value) {
    error.value = '请输入密码'
    return
  }
  loading.value = true
  error.value = ''
  try {
    const result = await nodesApi.unlockNode(props.nodeId, password.value)
    const unlockedId = result.nodeId || props.nodeId
    auth.setNodeToken(unlockedId, result.unlockToken ?? '', result.expiresIn ?? 1800)
    ui.toast.success('已解锁', props.nodeName ? `可以访问「${props.nodeName}」了` : undefined)
    emit('unlocked', unlockedId)
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
    title="需要密码"
    description="该目录受密码保护，解锁后可访问其整棵子树。"
    size="sm"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <div class="unlock">
      <p v-if="hint" class="unlock__hint">密码提示：{{ hint }}</p>
      <AppInput
        v-model="password"
        type="password"
        size="lg"
        autocomplete="off"
        placeholder="请输入目录密码"
        :invalid="Boolean(error)"
        @enter="submit"
      />
      <p v-if="error" class="field__error">{{ error }}</p>
    </div>
    <template #footer>
      <AppButton variant="ghost" @click="emit('update:modelValue', false)">取消</AppButton>
      <AppButton variant="primary" :loading="loading" icon="unlock" @click="submit">解锁</AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.unlock {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.unlock__hint {
  font-size: var(--text-xs);
  color: var(--text-muted);
  background: var(--surface-2);
  border-radius: var(--radius-sm);
  padding: var(--space-2) var(--space-3);
}
</style>
