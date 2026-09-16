<script setup lang="ts">
/** 文本输入框：支持前后缀图标、清除按钮与错误态。 */
import { computed, useAttrs } from 'vue'
import AppIcon from './AppIcon.vue'

defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    modelValue?: string | number | null
    type?: string
    placeholder?: string
    size?: 'sm' | 'md' | 'lg'
    icon?: string
    clearable?: boolean
    invalid?: boolean
    disabled?: boolean
    readonly?: boolean
    min?: number | string
    max?: number | string
    step?: number | string
    autocomplete?: string
    id?: string
  }>(),
  { type: 'text', size: 'md' },
)

const emit = defineEmits<{
  (e: 'update:modelValue', value: string): void
  (e: 'enter', value: string): void
  (e: 'blur', event: FocusEvent): void
  (e: 'focus', event: FocusEvent): void
}>()

const attrs = useAttrs()
const value = computed(() => (props.modelValue === null || props.modelValue === undefined ? '' : String(props.modelValue)))

function onInput(event: Event): void {
  emit('update:modelValue', (event.target as HTMLInputElement).value)
}
</script>

<template>
  <div class="input" :class="[`input--${size}`, { 'is-invalid': invalid, 'is-disabled': disabled }]">
    <AppIcon v-if="icon" :name="icon" class="input__icon" :size="16" />
    <input
      v-bind="attrs"
      :id="id"
      class="input__field"
      :type="type"
      :value="value"
      :placeholder="placeholder"
      :disabled="disabled"
      :readonly="readonly"
      :min="min"
      :max="max"
      :step="step"
      :autocomplete="autocomplete"
      :aria-invalid="invalid ? 'true' : undefined"
      @input="onInput"
      @keyup.enter="emit('enter', value)"
      @blur="emit('blur', $event)"
      @focus="emit('focus', $event)"
    />
    <button
      v-if="clearable && value && !disabled"
      type="button"
      class="input__clear"
      aria-label="清空"
      @click="emit('update:modelValue', '')"
    >
      <AppIcon name="x" :size="14" />
    </button>
    <slot name="suffix" />
  </div>
</template>

<style scoped>
.input {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  background: var(--surface);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  padding: 0 var(--space-3);
  transition:
    border-color var(--transition-fast),
    box-shadow var(--transition-fast);
}

.input:hover:not(.is-disabled) {
  border-color: var(--text-faint);
}

.input:focus-within {
  border-color: var(--accent);
  box-shadow: var(--focus-ring);
}

.input.is-invalid {
  border-color: var(--danger-600);
}

.input.is-disabled {
  background: var(--surface-2);
  opacity: 0.7;
}

.input--sm {
  height: 28px;
}

.input--md {
  height: 34px;
}

.input--lg {
  height: 42px;
}

.input__field {
  flex: 1 1 auto;
  min-width: 0;
  border: 0;
  outline: none;
  background: transparent;
  font-size: var(--text-sm);
  color: var(--text);
}

.input--lg .input__field {
  font-size: var(--text-md);
}

.input__field::placeholder {
  color: var(--text-faint);
}

.input__icon {
  color: var(--text-faint);
}

.input__clear {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 0;
  background: transparent;
  color: var(--text-faint);
  cursor: pointer;
  padding: 2px;
  border-radius: var(--radius-xs);
}

.input__clear:hover {
  color: var(--text);
  background: var(--surface-3);
}
</style>
