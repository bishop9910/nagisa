<script setup lang="ts">
/** 轻提示宿主：挂在 App 根部，由 ui store 驱动。 */
import { storeToRefs } from 'pinia'
import AppIcon from './AppIcon.vue'
import { useUiStore } from '@/stores/ui'

const ui = useUiStore()
const { toasts } = storeToRefs(ui)

const ICON: Record<string, string> = {
  success: 'check-circle',
  error: 'x-circle',
  warning: 'alert-triangle',
  info: 'info',
}
</script>

<template>
  <Teleport to="body">
    <div class="toasts" role="region" aria-live="polite" aria-label="通知">
      <TransitionGroup name="toast">
        <div v-for="toast in toasts" :key="toast.id" class="toast" :class="`toast--${toast.type}`">
          <AppIcon :name="ICON[toast.type] ?? 'info'" :size="18" />
          <div class="toast__content">
            <p class="toast__title">{{ toast.title }}</p>
            <p v-if="toast.message" class="toast__message">{{ toast.message }}</p>
          </div>
          <button type="button" class="toast__close" aria-label="关闭通知" @click="ui.dismissToast(toast.id)">
            <AppIcon name="x" :size="14" />
          </button>
        </div>
      </TransitionGroup>
    </div>
  </Teleport>
</template>

<style scoped>
.toasts {
  position: fixed;
  top: var(--space-4);
  right: var(--space-4);
  z-index: var(--z-toast);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  max-width: min(92vw, 400px);
  pointer-events: none;
}

.toast {
  display: flex;
  align-items: flex-start;
  gap: var(--space-3);
  background: var(--surface);
  border: 1px solid var(--border);
  border-left: 3px solid var(--accent);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-lg);
  padding: var(--space-3) var(--space-3);
  pointer-events: auto;
}

.toast--success {
  border-left-color: var(--success-600);
  color: var(--success-600);
}

.toast--error {
  border-left-color: var(--danger-600);
  color: var(--danger-600);
}

.toast--warning {
  border-left-color: var(--warning-600);
  color: var(--warning-600);
}

.toast--info {
  border-left-color: var(--info-600);
  color: var(--info-600);
}

.toast__content {
  flex: 1 1 auto;
  min-width: 0;
  color: var(--text);
}

.toast__title {
  font-size: var(--text-sm);
  font-weight: 600;
}

.toast__message {
  font-size: var(--text-xs);
  color: var(--text-muted);
  margin-top: 2px;
  word-break: break-word;
}

.toast__close {
  border: 0;
  background: transparent;
  color: var(--text-faint);
  cursor: pointer;
  padding: 2px;
  border-radius: var(--radius-xs);
  display: inline-flex;
}

.toast__close:hover {
  background: var(--surface-3);
  color: var(--text);
}

.toast-enter-active,
.toast-leave-active {
  transition:
    opacity var(--transition-base),
    transform var(--transition-base);
}

.toast-enter-from,
.toast-leave-to {
  opacity: 0;
  transform: translateX(12px);
}
</style>
