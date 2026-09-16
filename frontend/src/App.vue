<script setup lang="ts">
import { onMounted } from 'vue'
import AppConfirmDialog from '@/components/ui/AppConfirmDialog.vue'
import AppToasts from '@/components/ui/AppToasts.vue'
import { useUiStore } from '@/stores/ui'

const ui = useUiStore()

onMounted(() => {
  ui.applyTheme()
  ui.watchSystemTheme()
})
</script>

<template>
  <RouterView />
  <AppToasts />
  <AppConfirmDialog />
</template>
