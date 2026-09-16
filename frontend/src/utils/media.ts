/** 文件类型识别：图标、预览方式与分类统计都基于这里。 */

import type { Node } from '@/api/types'
import { extensionOf } from './format'

export type MediaKind =
  | 'folder'
  | 'image'
  | 'video'
  | 'audio'
  | 'pdf'
  | 'text'
  | 'code'
  | 'archive'
  | 'document'
  | 'spreadsheet'
  | 'presentation'
  | 'file'

const IMAGE_EXT = new Set(['jpg', 'jpeg', 'png', 'gif', 'webp', 'bmp', 'svg', 'ico', 'avif', 'heic', 'tiff'])
const VIDEO_EXT = new Set(['mp4', 'mkv', 'mov', 'avi', 'webm', 'flv', 'wmv', 'm4v', 'ts', 'mpg', 'mpeg'])
const AUDIO_EXT = new Set(['mp3', 'wav', 'flac', 'aac', 'ogg', 'm4a', 'wma', 'opus', 'aiff'])
const TEXT_EXT = new Set(['txt', 'log', 'md', 'markdown', 'srt', 'vtt', 'csv', 'tsv'])
const CODE_EXT = new Set([
  'js', 'mjs', 'cjs', 'ts', 'tsx', 'jsx', 'vue', 'html', 'htm', 'css', 'scss', 'less', 'json', 'yaml', 'yml', 'toml',
  'ini', 'conf', 'sh', 'bash', 'ps1', 'bat', 'go', 'rs', 'py', 'rb', 'java', 'kt', 'c', 'h', 'cpp', 'hpp', 'cs', 'php',
  'swift', 'sql', 'proto', 'xml', 'lua', 'dart',
])
const ARCHIVE_EXT = new Set(['zip', 'rar', '7z', 'tar', 'gz', 'bz2', 'xz', 'zst', 'iso'])
const DOC_EXT = new Set(['doc', 'docx', 'odt', 'rtf', 'pages'])
const SHEET_EXT = new Set(['xls', 'xlsx', 'ods', 'numbers'])
const SLIDE_EXT = new Set(['ppt', 'pptx', 'odp', 'key'])

/** 判断条目属于哪一类内容。 */
export function mediaKindOf(node: Pick<Node, 'kind' | 'mimeType' | 'name'> | undefined | null): MediaKind {
  if (!node) return 'file'
  if (node.kind === 'NODE_KIND_FOLDER') return 'folder'
  const mime = (node.mimeType ?? '').toLowerCase()
  if (mime.startsWith('image/')) return 'image'
  if (mime.startsWith('video/')) return 'video'
  if (mime.startsWith('audio/')) return 'audio'
  if (mime === 'application/pdf') return 'pdf'
  if (mime.startsWith('text/')) {
    const ext = extensionOf(node.name)
    if (CODE_EXT.has(ext)) return 'code'
    return 'text'
  }
  const ext = extensionOf(node.name)
  if (IMAGE_EXT.has(ext)) return 'image'
  if (VIDEO_EXT.has(ext)) return 'video'
  if (AUDIO_EXT.has(ext)) return 'audio'
  if (ext === 'pdf') return 'pdf'
  if (ARCHIVE_EXT.has(ext)) return 'archive'
  if (DOC_EXT.has(ext)) return 'document'
  if (SHEET_EXT.has(ext)) return 'spreadsheet'
  if (SLIDE_EXT.has(ext)) return 'presentation'
  if (CODE_EXT.has(ext)) return 'code'
  if (TEXT_EXT.has(ext)) return 'text'
  return 'file'
}

/** 图标名（Icon 组件内置集合）。 */
export function iconForKind(kind: MediaKind): string {
  switch (kind) {
    case 'folder':
      return 'folder'
    case 'image':
      return 'image'
    case 'video':
      return 'video'
    case 'audio':
      return 'audio'
    case 'pdf':
      return 'pdf'
    case 'text':
      return 'file-text'
    case 'code':
      return 'file-code'
    case 'archive':
      return 'archive'
    case 'document':
      return 'file-doc'
    case 'spreadsheet':
      return 'file-sheet'
    case 'presentation':
      return 'file-slide'
    default:
      return 'file'
  }
}

/** 图标色调：让列表一眼能分辨类型。 */
export function toneForKind(kind: MediaKind): string {
  switch (kind) {
    case 'folder':
      return 'brand'
    case 'image':
      return 'success'
    case 'video':
      return 'danger'
    case 'audio':
      return 'warning'
    case 'pdf':
      return 'danger'
    case 'archive':
      return 'warning'
    case 'document':
    case 'spreadsheet':
    case 'presentation':
      return 'info'
    case 'code':
      return 'brand'
    default:
      return 'neutral'
  }
}

export type PreviewMode = 'image' | 'video' | 'audio' | 'pdf' | 'text' | 'none'

/** 预览方式；与后端 GetPreviewUrl 支持的 MIME 前缀保持一致。 */
export function previewModeOf(node: Pick<Node, 'kind' | 'mimeType' | 'name'> | undefined | null): PreviewMode {
  const kind = mediaKindOf(node)
  switch (kind) {
    case 'image':
      return 'image'
    case 'video':
      return 'video'
    case 'audio':
      return 'audio'
    case 'pdf':
      return 'pdf'
    case 'text':
    case 'code':
      return 'text'
    default:
      return 'none'
  }
}

export function isPreviewable(node: Pick<Node, 'kind' | 'mimeType' | 'name'> | undefined | null): boolean {
  return previewModeOf(node) !== 'none'
}

/** 网格视图里是否用缩略图（图片与视频有明显收益）。 */
export function supportsThumbnail(node: Pick<Node, 'kind' | 'mimeType' | 'name'> | undefined | null): boolean {
  const kind = mediaKindOf(node)
  return kind === 'image' || kind === 'video'
}
