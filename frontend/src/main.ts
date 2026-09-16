import { createApp } from 'vue'
import { createPinia } from 'pinia'

import App from './App.vue'
import router from './router'
import { useAuthStore } from './stores/auth'
import { useUiStore } from './stores/ui'

import './assets/styles/tokens.css'
import './assets/styles/base.css'

const app = createApp(App)
const pinia = createPinia()

app.use(pinia)
app.use(router)

// 会话在任意请求中被判定失效时：能降级成只读访客就留在原地，不能才跳回登录页。
const auth = useAuthStore(pinia)
const ui = useUiStore(pinia)
auth.setExpiredHandler((reason) => {
  if (reason === 'guest-fallback') {
    ui.toast.warning('登录状态已失效', '当前以只读访客身份浏览，可随时重新登录')
    const current = router.currentRoute.value
    if (current.meta.requiresManage || current.meta.capability) {
      void router.replace({ name: 'files' })
    }
    return
  }
  ui.toast.warning('登录状态已失效', '请重新登录以继续')
  void router.replace({ name: 'login', query: { redirect: router.currentRoute.value.fullPath } })
})

app.mount('#app')
