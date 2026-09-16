<script setup lang="ts">
/** 空状态：无数据、无搜索结果、无权限都使用同一套结构。 */
import AppIcon from './AppIcon.vue'

withDefaults(
  defineProps<{
    icon?: string
    title?: string
    description?: string
    size?: 'sm' | 'md'
  }>(),
  { icon: 'inbox', size: 'md' },
)
</script>

<template>
  <div class="empty" :class="`empty--${size}`">
    <div class="empty__icon">
      <AppIcon :name="icon" :size="size === 'sm' ? 20 : 26" />
    </div>
    <p v-if="title" class="empty__title">{{ title }}</p>
    <p v-if="description" class="empty__desc">{{ description }}</p>
    <div v-if="$slots.default" class="empty__actions">
      <slot />
    </div>
  </div>
</template>

<style scoped>
.empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  text-align: center;
  gap: var(--space-2);
  padding: var(--space-12) var(--space-6);
  color: var(--text-muted);
}

.empty--sm {
  padding: var(--space-8) var(--space-4);
}

.empty__icon {
  width: 52px;
  height: 52px;
  border-radius: var(--radius-xl);
  background: var(--accent-soft);
  color: var(--accent-text);
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: var(--space-2);
}

.empty--sm .empty__icon {
  width: 40px;
  height: 40px;
}

.empty__title {
  color: var(--text-strong);
  font-weight: 620;
  font-size: var(--text-md);
}

.empty__desc {
  font-size: var(--text-sm);
  max-width: 32rem;
}

.empty__actions {
  margin-top: var(--space-3);
  display: flex;
  gap: var(--space-2);
}
</style>
