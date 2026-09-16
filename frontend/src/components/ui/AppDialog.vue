<script setup lang="ts">
/** 模态对话框：Teleport 到 body，锁滚动、Esc 关闭、遮罩点击关闭。 */
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import AppIcon from './AppIcon.vue'

const props = withDefaults(
  defineProps<{
    modelValue?: boolean
    title?: string
    description?: string
    size?: 'sm' | 'md' | 'lg' | 'xl'
    closeOnOverlay?: boolean
    hideFooter?: boolean
    /** 关闭按钮之外的额外约束（例如提交中不允许关闭）。 */
    persistent?: boolean
  }>(),
  { size: 'md', closeOnOverlay: true, hideFooter: false, persistent: false },
)

const emit = defineEmits<{ (e: 'update:modelValue', value: boolean): void }>()

const panel = ref<HTMLElement | null>(null)
let previousOverflow = ''

function close(): void {
  if (props.persistent) return
  emit('update:modelValue', false)
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') {
    event.stopPropagation()
    close()
  }
}

watch(
  () => props.modelValue,
  async (open) => {
    if (typeof document === 'undefined') return
    if (open) {
      previousOverflow = document.body.style.overflow
      document.body.style.overflow = 'hidden'
      document.addEventListener('keydown', onKeydown)
      await nextTick()
      const focusable = panel.value?.querySelector<HTMLElement>(
        'input:not([type="hidden"]), textarea, select, button:not(.dialog__close)',
      )
      focusable?.focus()
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
      <div v-if="modelValue" class="dialog" role="presentation" @click.self="closeOnOverlay && close()">
        <div
          ref="panel"
          class="dialog__panel"
          :class="`dialog__panel--${size}`"
          role="dialog"
          aria-modal="true"
          :aria-label="title"
        >
          <header v-if="title || $slots.header" class="dialog__header">
            <div class="dialog__heading">
              <h3 class="dialog__title">{{ title }}</h3>
              <p v-if="description" class="dialog__desc">{{ description }}</p>
            </div>
            <div class="dialog__header-actions">
              <slot name="header" />
              <button v-if="!persistent" type="button" class="dialog__close" aria-label="关闭" @click="close">
                <AppIcon name="x" :size="18" />
              </button>
            </div>
          </header>
          <div class="dialog__body">
            <slot />
          </div>
          <footer v-if="!hideFooter" class="dialog__footer">
            <slot name="footer" />
          </footer>
        </div>
      </div>
    </Transition>
  </Teleport>
</template>

<style scoped>
.dialog {
  position: fixed;
  inset: 0;
  z-index: var(--z-modal);
  background: var(--overlay);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-4);
  backdrop-filter: blur(2px);
}

.dialog__panel {
  width: 100%;
  max-height: calc(100vh - 2 * var(--space-6));
  display: flex;
  flex-direction: column;
  background: var(--surface);
  border: 1px solid var(--border);
  border-radius: var(--radius-xl);
  box-shadow: var(--shadow-xl);
  overflow: hidden;
}

.dialog__panel--sm {
  max-width: 420px;
}

.dialog__panel--md {
  max-width: 560px;
}

.dialog__panel--lg {
  max-width: 760px;
}

.dialog__panel--xl {
  max-width: 1040px;
}

.dialog__header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-4);
  padding: var(--space-5) var(--space-5) var(--space-3);
}

.dialog__title {
  font-size: var(--text-lg);
  font-weight: 640;
}

.dialog__desc {
  margin-top: 4px;
  color: var(--text-muted);
  font-size: var(--text-sm);
}

.dialog__header-actions {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.dialog__close {
  border: 0;
  background: transparent;
  color: var(--text-faint);
  cursor: pointer;
  border-radius: var(--radius-sm);
  padding: 4px;
  display: inline-flex;
}

.dialog__close:hover {
  background: var(--surface-3);
  color: var(--text);
}

.dialog__body {
  padding: 0 var(--space-5) var(--space-5);
  overflow-y: auto;
  flex: 1 1 auto;
}

.dialog__footer {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: var(--space-2);
  padding: var(--space-4) var(--space-5);
  border-top: 1px solid var(--border-subtle);
  background: var(--surface-2);
}

.dialog__footer:empty {
  display: none;
}
</style>
