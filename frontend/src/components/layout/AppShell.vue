<script setup lang="ts">
/** 主框架：左侧导航 + 顶部操作栏 + 内容区 + 全局上传面板。 */
import { computed, ref } from 'vue'
import AppSidebar from './AppSidebar.vue'
import AppTopbar from './AppTopbar.vue'
import UploadPanel from '@/components/files/UploadPanel.vue'

const mobileNavOpen = ref(false)
const collapsed = ref(readCollapsed())

function readCollapsed(): boolean {
  try {
    return localStorage.getItem('nagisa.sidebar') === 'collapsed'
  } catch {
    return false
  }
}

function toggleCollapse(): void {
  collapsed.value = !collapsed.value
  try {
    localStorage.setItem('nagisa.sidebar', collapsed.value ? 'collapsed' : 'expanded')
  } catch {
    /* ignore */
  }
}

const shellClass = computed(() => ({ 'is-collapsed': collapsed.value, 'is-mobile-open': mobileNavOpen.value }))
</script>

<template>
  <div class="shell" :class="shellClass">
    <AppSidebar @navigate="mobileNavOpen = false" />
    <div v-if="mobileNavOpen" class="shell__scrim" @click="mobileNavOpen = false" />
    <div class="shell__main">
      <AppTopbar
        :collapsed="collapsed"
        @toggle-collapse="toggleCollapse"
        @toggle-nav="mobileNavOpen = !mobileNavOpen"
      />
      <main class="shell__content">
        <div class="shell__inner">
          <RouterView v-slot="{ Component }">
            <Transition name="route" mode="out-in">
              <component :is="Component" />
            </Transition>
          </RouterView>
        </div>
      </main>
    </div>
    <UploadPanel />
  </div>
</template>

<style scoped>
.shell {
  display: flex;
  min-height: 100vh;
  background: var(--bg-app);
}

.shell__main {
  flex: 1 1 auto;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.shell__content {
  flex: 1 1 auto;
  min-height: 0;
  overflow-y: auto;
}

.shell__inner {
  max-width: var(--layout-content-max);
  margin: 0 auto;
  padding: var(--space-5) var(--space-6) var(--space-10);
}

.shell__scrim {
  position: fixed;
  inset: 0;
  background: var(--overlay);
  z-index: calc(var(--z-drawer) - 1);
  display: none;
}

@media (max-width: 1024px) {
  .shell__inner {
    padding: var(--space-4) var(--space-4) var(--space-8);
  }
}

@media (max-width: 768px) {
  .shell__scrim {
    display: block;
  }
}
</style>
