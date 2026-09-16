/** AuditService：审计流水与聚合摘要。 */

import type { AuditLog, AuditLogSet, AuditSummary } from './types'
import { http } from './http'

export function listAuditLogs(
  params: { pageSize?: number; pageToken?: string; filter?: string; orderBy?: string } = {},
): Promise<AuditLogSet> {
  return http.get<AuditLogSet>('/v1/audit/logs/list', { query: params })
}

export function getAuditLog(id: string): Promise<AuditLog> {
  return http.get<AuditLog>(`/v1/audit/logs/${encodeURIComponent(id)}`)
}

export function getAuditSummary(
  params: { from?: string; to?: string; actionPrefix?: string } = {},
): Promise<AuditSummary> {
  return http.get<AuditSummary>('/v1/audit/summary', { query: params })
}
