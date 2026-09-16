<script setup lang="ts">
/** 多行文本域。 */
import { computed, useAttrs } from 'vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    modelValue?: string | null
    rows?: number
    placeholder?: string
    invalid?: boolean
    disabled?: boolean
    maxlength?: number
  }>(),
  { rows: 4 },
)

const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()

const attrs = useAttrs()
const value = computed(() => props.modelValue ?? '')
</script>

<template>
  <textarea
    v-bind="attrs"
    class="textarea"
    :class="{ 'is-invalid': invalid }"
    :rows="rows"
    :value="value"
    :placeholder="placeholder"
    :disabled="disabled"
    :maxlength="maxlength"
    @input="emit('update:modelValue', ($event.target as HTMLTextAreaElement).value)"
  />
</template>

<style scoped>
.textarea {
  width: 100%;
  background: var(--surface);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  font-size: var(--text-sm);
  line-height: var(--leading-normal);
  resize: vertical;
  outline: none;
  transition:
    border-color var(--transition-fast),
    box-shadow var(--transition-fast);
}

.textarea:hover:not(:disabled) {
  border-color: var(--text-faint);
}

.textarea:focus {
  border-color: var(--accent);
  box-shadow: var(--focus-ring);
}

.textarea.is-invalid {
  border-color: var(--danger-600);
}

.textarea:disabled {
  background: var(--surface-2);
  opacity: 0.7;
}
</style>
