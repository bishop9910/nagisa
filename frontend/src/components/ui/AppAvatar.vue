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

/** 头像色块的饱和度与明度，色相之外的两个固定分量。 */
const TILE_SATURATION = 0.62
const TILE_LIGHTNESS = 0.52

function hslToRgb(hue: number, saturation: number, lightness: number): [number, number, number] {
  const c = (1 - Math.abs(2 * lightness - 1)) * saturation
  const x = c * (1 - Math.abs(((hue / 60) % 2) - 1))
  const m = lightness - c / 2
  const sector = Math.floor(((hue % 360) + 360) % 360 / 60)
  const table: [number, number, number][] = [
    [c, x, 0],
    [x, c, 0],
    [0, c, x],
    [0, x, c],
    [x, 0, c],
    [c, 0, x],
  ]
  const [r, g, b] = table[sector] ?? table[0]!
  return [(r + m) * 255, (g + m) * 255, (b + m) * 255]
}

/** WCAG 相对亮度，用来判断该配白字还是深字。 */
function relativeLuminance(rgb: [number, number, number]): number {
  const [r, g, b] = rgb.map((channel) => {
    const value = channel / 255
    return value <= 0.03928 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4
  }) as [number, number, number]
  return 0.2126 * r + 0.7152 * g + 0.0722 * b
}

const tileColor = computed(() => `hsl(${hue.value} ${TILE_SATURATION * 100}% ${TILE_LIGHTNESS * 100}%)`)

/**
 * 首字压在浅色块上（黄、青、绿这些色相）白字只剩 1.6:1，等于看不见，
 * 所以按底色亮度挑字色：亮底配深字，暗底配白字，两档都在 4.5:1 以上。
 */
const textColor = computed(() => {
  if (props.src) return 'var(--text-inverse)'
  const luminance = relativeLuminance(hslToRgb(hue.value, TILE_SATURATION, TILE_LIGHTNESS))
  return luminance >= 0.205 ? '#0d1424' : '#ffffff'
})
</script>

<template>
  <span
    class="avatar"
    :style="{
      width: `${size}px`,
      height: `${size}px`,
      fontSize: `${Math.max(10, Math.round(size * 0.42))}px`,
      background: src ? 'transparent' : tileColor,
      color: textColor,
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
