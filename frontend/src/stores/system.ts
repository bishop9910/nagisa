/** 部署级信息：系统能力、上传限制与健康状态。 */

import { computed, ref } from 'vue'
import { defineStore } from 'pinia'

import { systemApi } from '@/api'
import type { HealthStatus, SystemInfo } from '@/api/types'
import { toInt } from '@/utils/format'

export const useSystemStore = defineStore('system', () => {
  const info = ref<SystemInfo | null>(null)
  const health = ref<HealthStatus | null>(null)
  const loading = ref(false)
  const error = ref('')

  const name = computed(() => info.value?.name || 'Nagisa 网盘')
  const version = computed(() => info.value?.version || '—')
  const apiVersion = computed(() => info.value?.apiVersion || 'v1')
  const features = computed(() => info.value?.features ?? [])
  const uploadModes = computed(() => info.value?.uploadModes ?? [])
  const maxUploadSize = computed(() => toInt(info.value?.maxUploadSize))
  const defaultChunkSize = computed(() => toInt(info.value?.defaultChunkSize) || 8 * 1024 * 1024)
  const minChunkSize = computed(() => toInt(info.value?.minChunkSize) || 5 * 1024 * 1024)
  const maxInlineSize = computed(() => toInt(info.value?.maxInlineSize) || 4 * 1024 * 1024)
  const storageBackend = computed(() => info.value?.storageBackend || 'none')
  const databaseBackend = computed(() => info.value?.databaseBackend || '—')
  const publicBaseUrl = computed(() => info.value?.publicBaseUrl || '')
  const signedUrlTtl = computed(() => info.value?.signedUrlTtlSeconds ?? 1800)
  const uploadSessionTtl = computed(() => info.value?.uploadSessionTtlSeconds ?? 86400)
  /** 口令最小长度由服务端下发（账号 / 文件夹 / 分享共用），拿不到时按默认 8 兜底。 */
  const minPasswordLength = computed(() => toInt(info.value?.auth?.minPasswordLength) || 8)
  const healthStatus = computed(() => health.value?.status || 'unknown')

  function hasFeature(feature: string): boolean {
    return features.value.includes(feature)
  }

  /** 对象存储不可用时，上传与下载相关的入口会给出明确提示。 */
  const storageAvailable = computed(() => storageBackend.value !== '' && storageBackend.value !== 'none')

  async function loadInfo(force = false): Promise<SystemInfo | null> {
    if (info.value && !force) return info.value
    loading.value = true
    error.value = ''
    try {
      info.value = await systemApi.getSystemInfo()
      return info.value
    } catch (err) {
      error.value = err instanceof Error ? err.message : String(err)
      return null
    } finally {
      loading.value = false
    }
  }

  async function loadHealth(deep = false): Promise<HealthStatus | null> {
    try {
      health.value = await systemApi.healthCheck(deep)
      return health.value
    } catch (err) {
      health.value = {
        status: 'down',
        checks: { api: err instanceof Error ? err.message : String(err) },
      }
      return health.value
    }
  }

  return {
    info,
    health,
    loading,
    error,
    name,
    version,
    apiVersion,
    features,
    uploadModes,
    maxUploadSize,
    defaultChunkSize,
    minChunkSize,
    maxInlineSize,
    storageBackend,
    databaseBackend,
    publicBaseUrl,
    signedUrlTtl,
    uploadSessionTtl,
    minPasswordLength,
    healthStatus,
    storageAvailable,
    hasFeature,
    loadInfo,
    loadHealth,
  }
})
