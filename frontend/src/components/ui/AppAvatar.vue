<script setup lang="ts">
/** 头像：有头像地址就用图片，否则用昵称首字生成色块。 */
import { computed } from 'vue'

const props = withDefaults(
  defineProps<{
    name?: string
    src?: string
    size?: number
  }>(),
  { size: 32 },
)

const initial = computed(() => {
  const label = (props.name ?? '').trim()
  if (!label) return '?'
  return label.slice(0, 1).toUpperCase()
})

/** 由名字派生一个稳定的色相，避免整屏头像颜色雷同。 */
const hue = computed(() => {
  const label = props.name ?? ''
  let hash = 0
  for (let i = 0; i < label.length; i += 1) {
    hash = (hash * 31 + label.charCodeAt(i)) % 360
  }
  return hash
})
</script>

<template>
  <span
    class="avatar"
    :style="{
      width: `${size}px`,
      height: `${size}px`,
      fontSize: `${Math.max(10, Math.round(size * 0.42))}px`,
      background: src ? 'transparent' : `hsl(${hue} 62% 52%)`,
    }"
  >
    <img v-if="src" :src="src" :alt="name" class="avatar__img" />
    <template v-else>{{ initial }}</template>
  </span>
</template>

<style scoped>
.avatar {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: var(--radius-pill);
  color: #fff;
  font-weight: 620;
  flex: none;
  overflow: hidden;
  user-select: none;
}

.avatar__img {
  width: 100%;
  height: 100%;
  object-fit: cover;
}
</style>
