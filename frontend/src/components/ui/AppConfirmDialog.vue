<script setup lang="ts">
/** 全局确认对话框：由 ui.confirm() 的 Promise 驱动。 */
import { computed, ref, watch } from 'vue'
import { storeToRefs } from 'pinia'
import AppButton from './AppButton.vue'
import AppDialog from './AppDialog.vue'
import AppInput from './AppInput.vue'
import { useUiStore } from '@/stores/ui'

const ui = useUiStore()
const { confirmState } = storeToRefs(ui)

const typed = ref('')

watch(
  () => confirmState.value.open,
  (open) => {
    if (open) typed.value = ''
  },
)

const canConfirm = computed(() => {
  const required = confirmState.value.requireText
  if (!required) return true
  return typed.value.trim() === required
})

const danger = computed(() => confirmState.value.tone === 'danger')
</script>

<template>
  <AppDialog
    :model-value="confirmState.open"
    :title="confirmState.title"
    size="sm"
    :close-on-overlay="false"
    @update:model-value="ui.resolveConfirm(false)"
  >
    <p v-if="confirmState.message" class="confirm__message">{{ confirmState.message }}</p>
    <div v-if="confirmState.requireText" class="confirm__typed">
      <p class="confirm__hint">
        请输入 <span class="mono strong">{{ confirmState.requireText }}</span> 以确认：
      </p>
      <AppInput v-model="typed" :placeholder="confirmState.requireText" @enter="canConfirm && ui.resolveConfirm(true)" />
    </div>
    <template #footer>
      <AppButton variant="ghost" @click="ui.resolveConfirm(false)">
        {{ confirmState.cancelText || '取消' }}
      </AppButton>
      <AppButton :variant="danger ? 'danger' : 'primary'" :disabled="!canConfirm" @click="ui.resolveConfirm(true)">
        {{ confirmState.confirmText || '确定' }}
      </AppButton>
    </template>
  </AppDialog>
</template>

<style scoped>
.confirm__message {
  color: var(--text-muted);
  font-size: var(--text-sm);
  line-height: var(--leading-normal);
}

.confirm__typed {
  margin-top: var(--space-4);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.confirm__hint {
  font-size: var(--text-xs);
  color: var(--text-muted);
}
</style>
