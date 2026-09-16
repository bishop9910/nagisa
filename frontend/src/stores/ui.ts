/** 界面状态：主题、轻提示与确认对话框。 */

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

export type ThemeMode = 'light' | 'dark' | 'system'
export type ToastType = 'success' | 'error' | 'warning' | 'info'

const THEME_KEY = 'nagisa.theme'

export interface Toast {
  id: number
  type: ToastType
  title: string
  message?: string
}

export interface ConfirmOptions {
  title: string
  message?: string
  confirmText?: string
  cancelText?: string
  tone?: 'brand' | 'danger'
  /** 危险操作要求输入指定文本才能继续。 */
  requireText?: string
}

interface ConfirmState extends ConfirmOptions {
  open: boolean
}

export const useUiStore = defineStore('ui', () => {
  /* ---------- 主题 ---------- */

  const theme = ref<ThemeMode>(readTheme())
  const resolvedTheme = ref<'light' | 'dark'>('light')

  function readTheme(): ThemeMode {
    try {
      const saved = localStorage.getItem(THEME_KEY)
      if (saved === 'light' || saved === 'dark' || saved === 'system') return saved
    } catch {
      /* ignore */
    }
    return 'system'
  }

  function applyTheme(): void {
    const prefersDark =
      typeof window !== 'undefined' && window.matchMedia('(prefers-color-scheme: dark)').matches
    const next = theme.value === 'system' ? (prefersDark ? 'dark' : 'light') : theme.value
    resolvedTheme.value = next
    if (typeof document !== 'undefined') {
      document.documentElement.dataset.theme = next
    }
  }

  function setTheme(mode: ThemeMode): void {
    theme.value = mode
    try {
      localStorage.setItem(THEME_KEY, mode)
    } catch {
      /* ignore */
    }
    applyTheme()
  }

  function toggleTheme(): void {
    setTheme(resolvedTheme.value === 'dark' ? 'light' : 'dark')
  }

  function watchSystemTheme(): void {
    if (typeof window === 'undefined') return
    window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => {
      if (theme.value === 'system') applyTheme()
    })
  }

  /* ---------- 轻提示 ---------- */

  const toasts = ref<Toast[]>([])
  let toastSeq = 0

  function pushToast(type: ToastType, title: string, message?: string, timeout = 4000): number {
    const id = (toastSeq += 1)
    toasts.value = [...toasts.value, { id, type, title, message }]
    if (timeout > 0) {
      window.setTimeout(() => dismissToast(id), timeout)
    }
    return id
  }

  function dismissToast(id: number): void {
    toasts.value = toasts.value.filter((item) => item.id !== id)
  }

  const toast = {
    success: (title: string, message?: string) => pushToast('success', title, message),
    error: (title: string, message?: string) => pushToast('error', title, message, 6000),
    warning: (title: string, message?: string) => pushToast('warning', title, message, 5000),
    info: (title: string, message?: string) => pushToast('info', title, message),
  }

  /* ---------- 确认对话框（Promise 形式，调用点写起来最省事） ---------- */

  const confirmState = ref<ConfirmState>({
    open: false,
    title: '',
    message: '',
    confirmText: '确定',
    cancelText: '取消',
    tone: 'brand',
  })

  let resolver: ((value: boolean) => void) | null = null

  function confirm(options: ConfirmOptions): Promise<boolean> {
    confirmState.value = {
      open: true,
      confirmText: '确定',
      cancelText: '取消',
      tone: 'brand',
      ...options,
    }
    return new Promise<boolean>((resolve) => {
      resolver = resolve
    })
  }

  function resolveConfirm(value: boolean): void {
    confirmState.value = { ...confirmState.value, open: false }
    resolver?.(value)
    resolver = null
  }

  const isDark = computed(() => resolvedTheme.value === 'dark')

  return {
    theme,
    resolvedTheme,
    isDark,
    setTheme,
    toggleTheme,
    applyTheme,
    watchSystemTheme,
    toasts,
    pushToast,
    dismissToast,
    toast,
    confirmState,
    confirm,
    resolveConfirm,
  }
})
