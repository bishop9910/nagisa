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

// 会话在任意请求中被判定失效时，统一跳回登录页并说明原因。
const auth = useAuthStore(pinia)
const ui = useUiStore(pinia)
auth.setExpiredHandler(() => {
  ui.toast.warning('登录状态已失效', '请重新登录以继续')
  void router.replace({ name: 'login', query: { redirect: router.currentRoute.value.fullPath } })
})

app.mount('#app')
