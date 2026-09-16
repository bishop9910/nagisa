<script setup lang="ts">
/** 徽标：状态、角色、类型标签。 */
import AppIcon from './AppIcon.vue'

withDefaults(
  defineProps<{
    tone?: 'neutral' | 'brand' | 'success' | 'warning' | 'danger' | 'info'
    icon?: string
    size?: 'sm' | 'md'
  }>(),
  { tone: 'neutral', size: 'md' },
)
</script>

<template>
  <span class="badge" :class="[`badge--${tone}`, size === 'sm' ? 'badge--sm' : '']">
    <AppIcon v-if="icon" :name="icon" :size="size === 'sm' ? 11 : 12" :stroke="2" />
    <slot />
  </span>
</template>

<style scoped>
.badge--sm {
  height: 18px;
  padding: 0 6px;
  font-size: 10px;
}
</style>
