<script setup lang="ts">
/** 文件类型图标：按类型给色调，列表与网格共用。 */
import { computed } from 'vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import type { Node } from '@/api/types'
import { iconForKind, mediaKindOf, toneForKind } from '@/utils/media'

const props = withDefaults(
  defineProps<{
    node?: Node | null
    size?: number
    /** 用带底色的方块承载图标（网格卡片用）。 */
    boxed?: boolean
  }>(),
  { size: 18, boxed: false },
)

const kind = computed(() => mediaKindOf(props.node))
const icon = computed(() => iconForKind(kind.value))
const tone = computed(() => toneForKind(kind.value))
</script>

<template>
  <span class="file-icon" :class="[`file-icon--${tone}`, { 'file-icon--boxed': boxed }]" :style="boxed ? { width: `${size * 2}px`, height: `${size * 2}px` } : undefined">
    <AppIcon :name="icon" :size="size" />
    <span v-if="node?.passwordProtected" class="file-icon__lock" :title="node.passwordHint || '受密码保护'">
      <AppIcon name="lock" :size="10" :stroke="2.2" />
    </span>
  </span>
</template>

<style scoped>
.file-icon {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex: none;
  color: var(--text-muted);
}

.file-icon--boxed {
  border-radius: var(--radius-md);
  background: var(--surface-2);
}

.file-icon--brand {
  color: var(--brand-600);
}

.file-icon--brand.file-icon--boxed {
  background: var(--accent-soft);
}

.file-icon--success {
  color: var(--success-600);
}

.file-icon--success.file-icon--boxed {
  background: var(--success-50);
}

.file-icon--danger {
  color: var(--danger-600);
}

.file-icon--danger.file-icon--boxed {
  background: var(--danger-50);
}

.file-icon--warning {
  color: var(--warning-600);
}

.file-icon--warning.file-icon--boxed {
  background: var(--warning-50);
}

.file-icon--info {
  color: var(--info-600);
}

.file-icon--info.file-icon--boxed {
  background: var(--info-50);
}

.file-icon__lock {
  position: absolute;
  right: -3px;
  bottom: -3px;
  width: 14px;
  height: 14px;
  border-radius: var(--radius-pill);
  background: var(--warning-600);
  color: #fff;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border: 1.5px solid var(--surface);
}
</style>
