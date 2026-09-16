<script setup lang="ts">
/** 右侧抽屉：详情、设置这类不打断主流程的面板都放在这里。 */
import { onBeforeUnmount, watch } from 'vue'
import AppIcon from './AppIcon.vue'

const props = withDefaults(
  defineProps<{
    modelValue?: boolean
    title?: string
    subtitle?: string
    size?: 'sm' | 'md' | 'lg'
  }>(),
  { size: 'md' },
)

const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()

let previousOverflow = ''

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') emit('update:modelValue', false)
}

watch(
  () => props.modelValue,
  (open) => {
    if (typeof document === 'undefined') return
    if (open) {
      previousOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
      document.addEventListener('keydown', onKeydown)
    } else {
      document.body.style.overflow = previousOverflow
      document.removeEventListener('keydown', onKeydown)
    }
  },
)

onBeforeUnmount(() => {
  if (typeof document === 'undefined') return
  document.body.style.overflow = previousOverflow
  document.removeEventListener('keydown', onKeydown)
})
</script>

<template>
  <Teleport to="body">
    <Transition name="fade">
      <div v-if="modelValue" class="drawer" role="presentation" @click.self="emit('update:modelValue', false)">
        <Transition name="slide" appear>
          <aside
            class="drawer__panel"
            :class="`drawer__panel--${size}`"
            role="dialog"
            aria-modal="true"
            :aria-label="title"
          >
            <header class="drawer__header">
              <div class="drawer__heading">
                <h3 class="drawer__title">{{ title }}</h3>
                <p v-if="subtitle" class="drawer__subtitle">{{ subtitle }}</p>
              </div>
              <div class="drawer__actions">
                <slot name="header" />
                <button type="button" class="drawer__close" aria-label="关闭" @click="emit('update:modelValue', false)">
                  <AppIcon name="x" :size="18" />
                </button>
              </div>
            </header>
            <div class="drawer__body">
              <slot />
            </div>
            <footer v-if="$slots.footer" class="drawer__footer">
              <slot name="footer" />
            </footer>
          </aside>
        </Transition>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.drawer {
  position: fixed;
  inset: 0;
  z-index: var(--z-drawer);
  background: var(--overlay);
  display: flex;
  justify-content: flex-end;
}

.drawer__panel {
  width: 100%;
  height: 100%;
  background: var(--surface);
  border-left: 1px solid var(--border);
  box-shadow: var(--shadow-xl);
  display: flex;
  flex-direction: column;
}

.drawer__panel--sm {
  max-width: 380px;
}

.drawer__panel--md {
  max-width: 460px;
}

.drawer__panel--lg {
  max-width: 620px;
}

.drawer__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-5);
  border-bottom: 1px solid var(--border-subtle);
}

.drawer__title {
  font-size: var(--text-lg);
  font-weight: 640;
}

.drawer__subtitle {
  margin-top: 2px;
  color: var(--text-muted);
  font-size: var(--text-xs);
}

.drawer__actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.drawer__close {
  border: 0;
  background: transparent;
  color: var(--text-faint);
  cursor: pointer;
  border-radius: var(--radius-sm);
  padding: 4px;
  display: inline-flex;
}

.drawer__close:hover {
  background: var(--surface-3);
  color: var(--text);
}

.drawer__body {
  flex: 1 1 auto;
  overflow-y: auto;
  padding: var(--space-5);
}

.drawer__footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-2);
  padding: var(--space-4) var(--space-5);
  border-top: 1px solid var(--border-subtle);
  background: var(--surface-2);
}

.slide-enter-active,
.slide-leave-active {
  transition: transform var(--transition-slow);
}

.slide-enter-from,
.slide-leave-to {
  transform: translateX(24px);
}
</style>
