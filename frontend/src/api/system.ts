/** SystemService：部署信息、健康检查、存储统计、维护任务与运行参数。 */

import type { HealthStatus, MaintenanceReport, StorageStats, SystemInfo, SystemSetting, SystemSettingSet } from './types'
import { http } from './http'

export function getSystemInfo(): Promise<SystemInfo> {
  return http.get<SystemInfo>('/v1/system/info', { anonymous: true })
}

export function healthCheck(deep = false): Promise<HealthStatus> {
  return http.get<HealthStatus>('/v1/system/health', { query: { deep }, anonymous: true })
}

export function getStorageStats(topOwners?: number): Promise<StorageStats> {
  return http.get<StorageStats>('/v1/system/storage/stats', { query: { topOwners } })
}

export function runMaintenance(input: {
  dryRun?: boolean
  tasks?: string[]
  trashRetentionDays?: number
}): Promise<MaintenanceReport> {
  return http.post<MaintenanceReport>('/v1/system/maintenance/run', input)
}

export function listSystemSettings(): Promise<SystemSettingSet> {
  return http.get<SystemSettingSet>('/v1/system/settings/list')
}

export function updateSystemSettings(settings: SystemSetting[]): Promise<SystemSettingSet> {
  return http.put<SystemSettingSet>('/v1/system/settings/update', { settings })
}
