import { describe, expect, it } from 'vitest'

import {
  formatBytes,
  formatDuration,
  formatNumber,
  formatPercent,
  formatRelative,
  fromLocalInputValue,
  toInt,
  toLocalInputValue,
} from '@/utils/format'
import { buildFilter, buildOrderBy, eq, gt, has, lte, orderBy, quote } from '@/utils/filter'
import { iconForKind, mediaKindOf, previewModeOf, supportsThumbnail } from '@/utils/media'
import {
  maskFromPermissions,
  permissionsFromMask,
  hasPermission,
  hasAnyPermission,
  PERMISSION_ALL,
  PERMISSION_NODE_SCOPE,
  PERMISSION_SHARE_SCOPE,
} from '@/utils/constants'
import { resolveShareUrl } from '@/utils/share'

describe('format 工具', () => {
  it('归一化 protojson 的 64 位整数（字符串或数字）', () => {
    expect(toInt('12582912')).toBe(12582912)
    expect(toInt(42)).toBe(42)
    expect(toInt(undefined)).toBe(0)
    expect(toInt('不是数字', 7)).toBe(7)
  })

  it('按 1024 进制格式化体积', () => {
    expect(formatBytes(0)).toBe('0 B')
    expect(formatBytes(512)).toBe('512 B')
    expect(formatBytes(1024)).toBe('1.0 KB')
    expect(formatBytes('12582912')).toBe('12.0 MB')
    expect(formatBytes(1024 * 1024 * 1024 * 3)).toBe('3.0 GB')
  })

  it('格式化千分位与百分比', () => {
    expect(formatNumber('1234567')).toContain('1')
    expect(formatPercent(0.5)).toBe('50%')
    expect(formatPercent(0)).toBe('0%')
    expect(formatPercent(1)).toBe('100%')
  })

  it('相对时间对未知与未来值都给得出结果', () => {
    expect(formatRelative(undefined)).toBe('—')
    expect(formatRelative(new Date(Date.now() - 30_000).toISOString())).toBe('刚刚')
    expect(formatRelative(new Date(Date.now() - 3 * 3600_000).toISOString())).toBe('3 小时前')
  })

  it('时长格式化保留两级单位', () => {
    expect(formatDuration(0)).toBe('0 秒')
    expect(formatDuration(90)).toBe('1 分 30 秒')
    expect(formatDuration(90000)).toBe('1 天 1 小时')
  })

  it('本地时间输入值与 RFC 3339 互转', () => {
    const iso = '2026-02-01T09:30:00.000Z'
    const local = toLocalInputValue(iso)
    expect(local).not.toBe('')
    expect(fromLocalInputValue(local)).toBe(iso)
    expect(fromLocalInputValue('')).toBeUndefined()
  })
})

describe('AIP 过滤表达式', () => {
  it('转义字符串字面量', () => {
    expect(quote('a"b')).toBe('"a\\"b"')
  })

  it('按字段类型生成比较表达式', () => {
    expect(eq('status', 1)).toBe('status=1')
    expect(eq('username', 'alice')).toBe('username="alice"')
    expect(has('name', 'report')).toBe('name:"report"')
    expect(gt('size', 1024)).toBe('size>1024')
    expect(lte('rank', 500)).toBe('rank<=500')
  })

  it('组合非空条件，全空时返回 undefined', () => {
    expect(buildFilter()).toBeUndefined()
    expect(buildFilter(undefined, false, '')).toBeUndefined()
    expect(buildFilter('a=1', undefined, 'b=2')).toBe('a=1 AND b=2')
    expect(buildFilter('a=1 OR b=2', 'c=3')).toBe('(a=1 OR b=2) AND c=3')
  })

  it('生成 order_by（AIP 语法用 "field desc"，不是 "-field"）', () => {
    expect(orderBy('name')).toBe('name')
    expect(orderBy('updated_at', true)).toBe('updated_at desc')
    expect(buildOrderBy([{ field: 'name' }])).toBe('name')
    expect(buildOrderBy([{ field: 'size', desc: true }, { field: 'name' }])).toBe('size desc,name')
  })
})

describe('文件类型识别', () => {
  it('优先按 MIME 判断', () => {
    expect(mediaKindOf({ kind: 'NODE_KIND_FOLDER' })).toBe('folder')
    expect(mediaKindOf({ kind: 'NODE_KIND_FILE', mimeType: 'image/png' })).toBe('image')
    expect(mediaKindOf({ kind: 'NODE_KIND_FILE', mimeType: 'video/mp4' })).toBe('video')
    expect(mediaKindOf({ kind: 'NODE_KIND_FILE', mimeType: 'application/pdf' })).toBe('pdf')
    expect(mediaKindOf({ kind: 'NODE_KIND_FILE', mimeType: 'text/plain', name: 'a.txt' })).toBe('text')
    expect(mediaKindOf({ kind: 'NODE_KIND_FILE', mimeType: 'text/plain', name: 'main.go' })).toBe('code')
  })

  it('MIME 缺失时按扩展名兜底', () => {
    expect(mediaKindOf({ kind: 'NODE_KIND_FILE', name: 'photo.HEIC' })).toBe('image')
    expect(mediaKindOf({ kind: 'NODE_KIND_FILE', name: 'archive.zip' })).toBe('archive')
    expect(mediaKindOf({ kind: 'NODE_KIND_FILE', name: 'sheet.xlsx' })).toBe('spreadsheet')
    expect(mediaKindOf({ kind: 'NODE_KIND_FILE', name: 'unknown.bin' })).toBe('file')
  })

  it('预览方式与图标与后端能力一致', () => {
    expect(previewModeOf({ kind: 'NODE_KIND_FILE', mimeType: 'image/jpeg' })).toBe('image')
    expect(previewModeOf({ kind: 'NODE_KIND_FILE', mimeType: 'application/pdf' })).toBe('pdf')
    expect(previewModeOf({ kind: 'NODE_KIND_FILE', mimeType: 'application/zip' })).toBe('none')
    expect(iconForKind('folder')).toBe('folder')
    expect(supportsThumbnail({ kind: 'NODE_KIND_FILE', mimeType: 'image/png' })).toBe(true)
    expect(supportsThumbnail({ kind: 'NODE_KIND_FILE', mimeType: 'application/pdf' })).toBe(false)
  })
})

describe('权限位掩码', () => {
  it('权限名与掩码可以互相转换', () => {
    const mask = maskFromPermissions(['PERMISSION_VIEW', 'PERMISSION_DOWNLOAD'])
    expect(mask).toBe(3)
    expect(permissionsFromMask(3)).toEqual(['PERMISSION_VIEW', 'PERMISSION_DOWNLOAD'])
    expect(maskFromPermissions([])).toBe(0)
    expect(maskFromPermissions(undefined)).toBe(0)
  })

  it('摘要与成员判断', () => {
    expect(hasPermission(255, 'PERMISSION_ACL_MANAGE')).toBe(true)
    expect(hasPermission(3, 'PERMISSION_UPLOAD')).toBe(false)
    expect(hasAnyPermission(1, 'PERMISSION_UPLOAD', 'PERMISSION_VIEW')).toBe(true)
    expect(PERMISSION_ALL).toBe(4095)
    expect(PERMISSION_NODE_SCOPE).toBe(255)
    expect(PERMISSION_SHARE_SCOPE).toBe(7)
  })
})

describe('分享链接', () => {
  // 服务端 public_base_url 留空时下发的是 /s/<token>，直接复制出去是没法用的，
  // 必须补成绝对地址；配了 public_base_url 时服务端给的就是绝对地址，原样用。
  it('相对地址补成绝对地址', () => {
    expect(resolveShareUrl({ url: '/s/abc123' })).toBe(`${window.location.origin}/s/abc123`)
    expect(resolveShareUrl({ url: '/s/abc123' }, 'https://netdisk.example.com')).toBe(
      'https://netdisk.example.com/s/abc123',
    )
  })

  it('绝对地址原样返回', () => {
    expect(resolveShareUrl({ url: 'https://netdisk.example.com/s/abc123' })).toBe(
      'https://netdisk.example.com/s/abc123',
    )
  })

  it('服务端没给 url 时用 token 拼', () => {
    expect(resolveShareUrl({ token: 'tok-1' }, 'https://netdisk.example.com/')).toBe(
      'https://netdisk.example.com/s/tok-1',
    )
  })
})
