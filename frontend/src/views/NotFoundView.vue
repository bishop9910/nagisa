<script setup lang="ts">
/** 404。 */
import AppButton from '@/components/ui/AppButton.vue'
import AppEmpty from '@/components/ui/AppEmpty.vue'
</script>

<template>
  <div class="notfound">
    <AppEmpty icon="search" title="页面不存在" description="链接可能已失效，或者你没有访问它的权限。">
      <RouterLink to="/files">
        <AppButton variant="primary" icon="home">返回我的文件</AppButton>
      </RouterLink>
    </AppEmpty>
  </div>
</template>

<style scoped>
.notfound {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--bg-app);
}
</style>
