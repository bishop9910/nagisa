<script setup lang="ts">
/** 下拉选择：原生 select 保证键盘与移动端体验，外观统一由外层负责。 */
import { computed } from 'vue'
import AppIcon from './AppIcon.vue'
import type { SelectOption } from './types'

const props = withDefaults(
  defineProps<{
    modelValue?: string | number | null
    options?: SelectOption[]
    placeholder?: string
    size?: 'sm' | 'md' | 'lg'
    disabled?: boolean
    invalid?: boolean
    id?: string
  }>(),
  { size: 'md', options: () => [] },
)

const emit = defineEmits<{ (e: 'update:modelValue', value: string): void }>()

const value = computed(() => (props.modelValue === null || props.modelValue === undefined ? '' : String(props.modelValue)))
</script>

<template>
  <div class="select" :class="[`select--${size}`, { 'is-invalid': invalid, 'is-disabled': disabled }]">
    <select
      :id="id"
      class="select__field"
      :value="value"
      :disabled="disabled"
      :aria-invalid="invalid ? 'true' : undefined"
      @change="emit('update:modelValue', ($event.target as HTMLSelectElement).value)"
    >
      <option v-if="placeholder" value="" disabled>{{ placeholder }}</option>
      <slot>
        <option v-for="option in options" :key="option.value" :value="option.value" :disabled="option.disabled">
          {{ option.label }}
        </option>
      </slot>
    </select>
    <AppIcon name="chevron-down" class="select__caret" :size="16" />
  </div>
</template>

<style scoped>
.select {
  position: relative;
  display: flex;
  align-items: center;
  background: var(--surface);
  border: 1px solid var(--border-strong);
  border-radius: var(--radius-md);
  transition:
    border-color var(--transition-fast),
    box-shadow var(--transition-fast);
}

.select:hover:not(.is-disabled) {
  border-color: var(--text-faint);
}

.select:focus-within {
  border-color: var(--accent);
  box-shadow: var(--focus-ring);
}

.select.is-invalid {
  border-color: var(--danger-600);
}

.select.is-disabled {
  opacity: 0.65;
  background: var(--surface-2);
}

.select--sm {
  height: 28px;
}

.select--md {
  height: 34px;
}

.select--lg {
  height: 42px;
}

.select__field {
  appearance: none;
  border: 0;
  outline: none;
  background: transparent;
  width: 100%;
  height: 100%;
  padding: 0 30px 0 var(--space-3);
  font-size: var(--text-sm);
  color: var(--text);
  cursor: pointer;
  /* 选项名过长时在控件里省略，而不是把宽度顶开。 */
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.select--lg .select__field {
  font-size: var(--text-md);
}

.select__caret {
  position: absolute;
  right: var(--space-2);
  color: var(--text-faint);
  pointer-events: none;
}
</style>
