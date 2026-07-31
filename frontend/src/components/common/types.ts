/**
 * Common component types
 */

export interface Column {
  key: string
  label: string
  sortable?: boolean
  class?: string
  formatter?: (value: any, row: any) => string
}

// 表头拖拽重排的统一事件契约，调用方据此更新自己的唯一顺序状态。
export type ColumnReorderPosition = 'before' | 'after'

export interface ColumnReorderEvent {
  sourceKey: string
  targetKey: string
  position: ColumnReorderPosition
}
