<script setup lang="ts">
/** 表单字段包装：标签、必填标记、提示与错误信息。 */
withDefaults(
  defineProps<{
    label?: string
    hint?: string
    error?: string
    required?: boolean
    /** 标签与控件同行（窄表单里更紧凑）。 */
    inline?: boolean
  }>(),
  {},
)
</script>

<template>
  <label class="field" :class="{ 'field--inline': inline }">
    <span v-if="label" class="field__label">
      {{ label }}
      <span v-if="required" class="req" aria-hidden="true">*</span>
      <slot name="labelExtra" />
    </span>
    <slot />
    <span v-if="error" class="field__error">{{ error }}</span>
    <span v-else-if="hint" class="field__hint">{{ hint }}</span>
  </label>
</template>

<style scoped>
.field--inline {
  flex-direction: row;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-4);
}

.field--inline .field__label {
  margin: 0;
}
</style>
