<script setup lang="ts">
/**
 * 管理概览：只留最基本的服务标识。
 * 生效的策略与上限、运行状态、服务端运行参数这些详情都在「系统信息」里，
 * 那页需要 system_manage 权限；这里只读公开的 /v1/system/info，谁都能看。
 */
import { onMounted } from 'vue'

import AppButton from '@/components/ui/AppButton.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import { useAuthStore } from '@/stores/auth'
import { useSystemStore } from '@/stores/system'

const auth = useAuthStore()
const system = useSystemStore()

onMounted(() => void system.loadInfo(true))
</script>

<template>
  <div class="overview">
    <section class="card">
      <header class="card__header">
        <div>
          <p class="card__title">服务</p>
          <p class="card__subtitle">来自 GET /v1/system/info（公开接口）</p>
        </div>
        <AppButton size="sm" variant="ghost" icon="refresh" :loading="system.loading" @click="system.loadInfo(true)">
          刷新
        </AppButton>
      </header>
      <div class="card__body">
        <div class="stat-grid">
          <div class="stat">
            <span class="stat__label">服务名称</span>
            <span class="stat__value">{{ system.name }}</span>
          </div>
          <div class="stat">
            <span class="stat__label">构建版本</span>
            <span class="stat__value">{{ system.version }}</span>
            <span class="stat__hint">API {{ system.apiVersion }}</span>
          </div>
        </div>

        <p v-if="system.error" class="field__error overview__error">{{ system.error }}</p>

        <p v-if="auth.canManageSystem" class="overview__more">
          <AppIcon name="info" :size="14" />
          <span>
            运行状态、生效的策略与上限、服务端登记的运行参数见
            <RouterLink :to="{ name: 'manage-settings' }">系统信息</RouterLink>。
          </span>
        </p>
      </div>
    </section>
  </div>
</template>

<style scoped>
.overview {
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.overview__error {
  margin-top: var(--space-3);
}

.overview__more {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  margin-top: var(--space-4);
  padding-top: var(--space-4);
  border-top: 1px solid var(--border-subtle);
  color: var(--text-muted);
  font-size: var(--text-xs);
}
</style>
