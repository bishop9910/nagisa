<script setup lang="ts">
/** 开关：用于布尔型设置。 */
const props = withDefaults(
  defineProps<{
    modelValue?: boolean
    disabled?: boolean
    label?: string
    id?: string
  }>(),
  {},
)

const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()
</script>

<template>
  <button
    :id="id"
    type="button"
    class="switch"
    :class="{ 'is-on': modelValue, 'is-disabled': disabled }"
    role="switch"
    :aria-checked="modelValue ? 'true' : 'false'"
    :aria-label="label"
    :disabled="disabled"
    @click="emit('update:modelValue', !modelValue)"
  >
    <span class="switch__knob" />
  </button>
</template>

<style scoped>
.switch {
  position: relative;
  width: 40px;
  height: 22px;
  border-radius: var(--radius-pill);
  border: 1px solid var(--border-strong);
  background: var(--surface-3);
  cursor: pointer;
  padding: 0;
  transition:
    background var(--transition-base),
    border-color var(--transition-base);
  flex: none;
}

.switch.is-on {
  background: var(--accent);
  border-color: var(--accent);
}

.switch.is-disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.switch__knob {
  position: absolute;
  top: 2px;
  left: 2px;
  width: 16px;
  height: 16px;
  border-radius: var(--radius-pill);
  background: #fff;
  box-shadow: var(--shadow-xs);
  transition: transform var(--transition-base);
}

.switch.is-on .switch__knob {
  transform: translateX(18px);
}
</style>
