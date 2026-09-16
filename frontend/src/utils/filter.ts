/**
 * AIP 过滤表达式构造器。
 *
 * 语法来自 go.einride.tech/aip/filtering：
 *   - 字符串等值 field="value"，包含/前缀 field:"value"；
 *   - 数值与时间比较 field>1、field>="2026-01-01T00:00:00Z"；
 *   - 布尔 field=true；组合用 AND / OR / NOT 与括号。
 * 未声明的标识符或语法错误会被服务端拒绝（400 NETDISK_INVALID_ARGUMENT）。
 */

/** 字符串字面量转义：只保留双引号包裹所需的最小变换。 */
export function quote(value: string): string {
  return `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"')}"`
}

/** 等值：field="value"。 */
export function eq(field: string, value: string | number | boolean): string {
  if (typeof value === 'string') return `${field}=${quote(value)}`
  return `${field}=${value}`
}

/** 包含/前缀匹配：field:"value"。 */
export function has(field: string, value: string): string {
  return `${field}:${quote(value)}`
}

export function gt(field: string, value: string | number): string {
  return `${field}>${typeof value === 'string' ? quote(value) : value}`
}

export function gte(field: string, value: string | number): string {
  return `${field}>=${typeof value === 'string' ? quote(value) : value}`
}

export function lt(field: string, value: string | number): string {
  return `${field}<${typeof value === 'string' ? quote(value) : value}`
}

export function lte(field: string, value: string | number): string {
  return `${field}<=${typeof value === 'string' ? quote(value) : value}`
}

/** 用 AND 连接非空片段，得到一个完整 filter；全空时返回 undefined。 */
export function buildFilter(...parts: (string | undefined | null | false)[]): string | undefined {
  const used = parts.filter((part): part is string => typeof part === 'string' && part.trim() !== '')
  if (used.length === 0) return undefined
  if (used.length === 1) return used[0]
  return used.map((part) => (part.includes(' OR ') ? `(${part})` : part)).join(' AND ')
}

/**
 * 把字段与降序标记组合成 AIP 的 order_by。
 *
 * 注意：AIP 的排序语法是 `field` 或 `field desc`，**不支持** `-field` 前缀
 * （解析器会因 '-' 不是合法字符而返回 400 NETDISK_INVALID_ARGUMENT）。
 */
export function orderBy(field: string, desc = false): string {
  return desc ? `${field} desc` : field
}

/** 排序字段数组 → order_by 字符串。 */
export function buildOrderBy(fields: { field: string; desc?: boolean }[]): string | undefined {
  if (fields.length === 0) return undefined
  return fields.map((item) => orderBy(item.field, item.desc)).join(',')
}
