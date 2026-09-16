/** UI 组件的公共类型。放在独立文件里，避免在 <script setup> 中写 export。 */

export interface SelectOption {
  value: string | number
  label: string
  disabled?: boolean
}

export interface SegmentOption {
  value: string
  label?: string
  icon?: string
  title?: string
}

export interface DropdownItem {
  key: string
  label?: string
  icon?: string
  /** 渲染成分隔线，此时 key 仅作占位。 */
  divider?: boolean
  danger?: boolean
  disabled?: boolean
  hint?: string
}

export interface TabItem {
  key: string
  label: string
  icon?: string
  badge?: string | number
  disabled?: boolean
}

export interface FieldOption<T extends string> {
  value: T
  label: string
  hint?: string
}
