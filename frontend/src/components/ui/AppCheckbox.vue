<script setup lang="ts">
/** 复选框：表单里的多选与权限位勾选都用它。 */
import AppIcon from './AppIcon.vue'

const props = withDefaults(
  defineProps<{
    modelValue?: boolean
    disabled?: boolean
    indeterminate?: boolean
    label?: string
  }>(),
  {},
)

const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()
</script>

<template>
  <label class="checkbox" :class="{ 'is-disabled': disabled }">
    <input
      class="checkbox__input"
      type="checkbox"
      :checked="modelValue"
      :disabled="disabled"
      @change="emit('update:modelValue', ($event.target as HTMLInputElement).checked)"
    />
    <span class="checkbox__box" :class="{ 'is-checked': modelValue || indeterminate }">
      <AppIcon v-if="indeterminate && !modelValue" name="minus" :size="12" :stroke="2.4" />
      <AppIcon v-else-if="modelValue" name="check" :size="12" :stroke="2.6" />
    </span>
    <span v-if="label || $slots.default" class="checkbox__label"><slot>{{ label }}</slot></span>
  </label>
</template>

<style scoped>
.checkbox {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  cursor: pointer;
  user-select: none;
  min-width: 0;
}

.checkbox.is-disabled {
  opacity: 0.55;
  cursor: not-allowed;
}

.checkbox__input {
  position: absolute;
  opacity: 0;
  width: 0;
  height: 0;
}

.checkbox__box {
  width: 16px;
  height: 16px;
  border-radius: var(--radius-xs);
  border: 1.5px solid var(--border-strong);
  background: var(--surface);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #fff;
  flex: none;
  transition:
    background var(--transition-fast),
    border-color var(--transition-fast);
}

.checkbox:not(.is-disabled):hover .checkbox__box {
  border-color: var(--accent);
}

.checkbox__input:focus-visible + .checkbox__box {
  box-shadow: var(--focus-ring);
}

.checkbox__box.is-checked {
  background: var(--accent);
  border-color: var(--accent);
}

.checkbox__label {
  font-size: var(--text-sm);
  min-width: 0;
}
</style>
