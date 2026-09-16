/** 数值、字节、时间与百分比的展示格式化。 */

/** 把 protojson 的 64 位整数（字符串或数字）归一成 number。 */
export function toInt(value: unknown, fallback = 0): number {
  if (typeof value === 'number') return Number.isFinite(value) ? value : fallback
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value)
    return Number.isFinite(parsed) ? parsed : fallback
  }
  return fallback
}

/** 字节数 → 人类可读体积。 */
export function formatBytes(value: unknown, digits = 1): string {
  const bytes = toInt(value)
  if (bytes <= 0) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB', 'TB', 'PB']
  let index = 0
  let size = bytes
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024
    index += 1
  }
  const unit = units[index] ?? 'B'
  const fixed = index === 0 ? 0 : size >= 100 ? 0 : digits
  return `${size.toFixed(fixed)} ${unit}`
}

/** 千分位数字。 */
export function formatNumber(value: unknown): string {
  return toInt(value).toLocaleString('zh-CN')
}

/** 日期时间：2026-09-15 22:28。 */
export function formatDateTime(value?: string | null): string {
  const date = parseDate(value)
  if (!date) return '—'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(
    date.getMinutes(),
  )}`
}

/** 仅日期：2026-09-15。 */
export function formatDate(value?: string | null): string {
  const date = parseDate(value)
  if (!date) return '—'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}

/** 相对时间：刚刚 / 5 分钟前 / 3 天前。 */
export function formatRelative(value?: string | null): string {
  const date = parseDate(value)
  if (!date) return '—'
  const diff = Date.now() - date.getTime()
  if (diff < 0) {
    const ahead = -diff
    if (ahead < 60_000) return '即将'
    if (ahead < 3_600_000) return `${Math.round(ahead / 60_000)} 分钟后`
    if (ahead < 86_400_000) return `${Math.round(ahead / 3_600_000)} 小时后`
    return formatDate(value ?? undefined)
  }
  if (diff < 45_000) return '刚刚'
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)} 分钟前`
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)} 小时前`
  if (diff < 7 * 86_400_000) return `${Math.floor(diff / 86_400_000)} 天前`
  return formatDate(value ?? undefined)
}

/** <input type="datetime-local"> 需要的本地时间串。 */
export function toLocalInputValue(value?: string | null): string {
  const date = parseDate(value)
  if (!date) return ''
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(
    date.getMinutes(),
  )}`
}

/** 本地时间串 → RFC 3339。 */
export function fromLocalInputValue(value: string): string | undefined {
  if (!value) return undefined
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return undefined
  return date.toISOString()
}

/** 秒数 → 1 天 2 小时 3 分。 */
export function formatDuration(seconds: unknown): string {
  let rest = Math.max(0, Math.floor(toInt(seconds)))
  if (rest === 0) return '0 秒'
  const days = Math.floor(rest / 86_400)
  rest -= days * 86_400
  const hours = Math.floor(rest / 3_600)
  rest -= hours * 3_600
  const minutes = Math.floor(rest / 60)
  const secs = rest - minutes * 60
  const parts: string[] = []
  if (days) parts.push(`${days} 天`)
  if (hours) parts.push(`${hours} 小时`)
  if (minutes) parts.push(`${minutes} 分`)
  if (!days && !hours && secs) parts.push(`${secs} 秒`)
  return parts.slice(0, 2).join(' ')
}

/** 比例 → 百分比文本。 */
export function formatPercent(ratio: unknown, digits = 1): string {
  const value = typeof ratio === 'number' ? ratio : toInt(ratio)
  if (!Number.isFinite(value) || value <= 0) return '0%'
  const percent = value <= 1 ? value * 100 : value
  return `${percent.toFixed(percent >= 10 ? 0 : digits)}%`
}

/** 上传进度等场景的「已传 / 总量」。 */
export function formatProgress(done: unknown, total: unknown): string {
  return `${formatBytes(done)} / ${formatBytes(total)}`
}

/** 剩余有效期文案。 */
export function formatExpiry(value?: string | null): string {
  const date = parseDate(value)
  if (!date) return '永不过期'
  if (date.getTime() <= Date.now()) return '已过期'
  return `${formatDateTime(value ?? undefined)} 到期`
}

export function parseDate(value?: string | null): Date | null {
  if (!value) return null
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? null : date
}

/** 文件扩展名（小写，不含点）。 */
export function extensionOf(name?: string): string {
  if (!name) return ''
  const index = name.lastIndexOf('.')
  if (index <= 0 || index === name.length - 1) return ''
  return name.slice(index + 1).toLowerCase()
}
