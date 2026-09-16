/** 浏览器侧的下载触发：用隐藏的 <a> 触发导航，避免被弹窗拦截。 */

import type { SignedUrl } from '@/api/types'
import { resolveUrl } from '@/api/http'

/** 用签名地址触发一次下载；跨域地址同样适用。 */
export function triggerDownload(signed: SignedUrl, fallbackName?: string): void {
  if (!signed.url) throw new Error('服务端未返回下载地址')
  const href = resolveUrl(signed.url)
  const anchor = document.createElement('a')
  anchor.href = href
  anchor.rel = 'noopener'
  const name = signed.fileName || fallbackName
  if (name) anchor.download = name
  anchor.style.display = 'none'
  document.body.appendChild(anchor)
  anchor.click()
  window.setTimeout(() => anchor.remove(), 0)
}

/** 在新标签页打开（预览、分享链接）。 */
export function openInNewTab(url?: string): void {
  if (!url) return
  window.open(resolveUrl(url), '_blank', 'noopener')
}

/** 复制文本到剪贴板，返回是否成功。 */
export async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text)
    return true
  } catch {
    return false
  }
}
