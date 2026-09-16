<script setup lang="ts">
/** 登录页：左侧品牌区 + 右侧表单；口令在浏览器端加密后再提交。 */
import { computed, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import AppButton from '@/components/ui/AppButton.vue'
import AppIcon from '@/components/ui/AppIcon.vue'
import AppInput from '@/components/ui/AppInput.vue'
import { errorText } from '@/api'
import { useAuthStore } from '@/stores/auth'
import { useSystemStore } from '@/stores/system'
import { useUiStore } from '@/stores/ui'

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const system = useSystemStore()
const ui = useUiStore()

const username = ref('')
const password = ref('')
const remember = ref(true)
const loading = ref(false)
const error = ref('')
/** 部署开启了免登录访客时，登录页同时提供「以访客身份浏览」。 */
const guestAvailable = ref(false)

const redirect = computed(() => {
  const value = route.query.redirect
  return typeof value === 'string' && value.startsWith('/') ? value : '/files'
})

const features = computed(() => {
  const list = system.features
  const labels: Record<string, string> = {
    chunked_upload: '分片续传',
    share_links: '分享链接',
    node_password: '目录加密',
    acl: '细粒度权限',
    versions: '历史版本',
    public_share: '匿名访问',
  }
  const mapped = list.map((item) => labels[item]).filter(Boolean)
  return mapped.length > 0 ? mapped : ['分片续传', '分享链接', '细粒度权限']
})

async function submit(): Promise<void> {
  if (!username.value.trim() || !password.value) {
    error.value = '请输入账号与密码'
    return
  }
  loading.value = true
  error.value = ''
  try {
    await auth.login(username.value.trim(), password.value, remember.value)
    ui.toast.success('登录成功', `欢迎回来，${auth.displayName}`)
    await router.replace(redirect.value)
  } catch (err) {
    error.value = errorText(err)
  } finally {
    loading.value = false
  }
}

/** 不输入口令，直接换取一个只读的访客会话。 */
async function browseAsGuest(): Promise<void> {
  loading.value = true
  error.value = ''
  try {
    if (!(await auth.enterGuestMode())) {
      error.value = '访客模式当前不可用'
      return
    }
    await router.replace(redirect.value)
  } finally {
    loading.value = false
  }
}

onMounted(async () => {
  void system.loadInfo()
  guestAvailable.value = await auth.guestLoginEnabled()
})
</script>

<template>
  <div class="login">
    <button type="button" class="login__theme" :aria-label="ui.isDark ? '切换浅色' : '切换深色'" @click="ui.toggleTheme()">
      <AppIcon :name="ui.isDark ? 'sun' : 'moon'" :size="18" />
    </button>

    <section class="login__brand">
      <div class="login__brand-inner">
        <div class="login__logo">
          <AppIcon name="cloud" :size="26" />
        </div>
        <h1 class="login__brand-title">{{ system.name }}</h1>
        <p class="login__brand-desc">私有化部署的网盘：文件树、分片上传、分享链接与细粒度权限，全部由你自己的服务端管着。</p>
        <ul class="login__features">
          <li v-for="feature in features" :key="feature">
            <AppIcon name="check-circle" :size="16" />
            {{ feature }}
          </li>
        </ul>
        <dl class="login__stats">
          <div>
            <dt>后端版本</dt>
            <dd>v{{ system.version }}</dd>
          </div>
          <div>
            <dt>对象存储</dt>
            <dd>{{ system.storageBackend }}</dd>
          </div>
          <div>
            <dt>数据库</dt>
            <dd>{{ system.databaseBackend }}</dd>
          </div>
        </dl>
      </div>
    </section>

    <section class="login__panel">
      <form class="login__form" @submit.prevent="submit">
        <header class="login__form-head">
          <h2>登录</h2>
          <p class="muted text-sm">使用管理员分配的账号登录</p>
        </header>

        <label class="field">
          <span class="field__label">账号</span>
          <AppInput
            v-model="username"
            size="lg"
            icon="user"
            autocomplete="username"
            placeholder="用户名"
            :invalid="Boolean(error)"
          />
        </label>
        <label class="field">
          <span class="field__label">密码</span>
          <AppInput
            v-model="password"
            type="password"
            size="lg"
            icon="lock"
            autocomplete="current-password"
            placeholder="密码"
            :invalid="Boolean(error)"
            @enter="submit"
          />
        </label>

        <div class="login__row">
          <label class="login__remember">
            <input v-model="remember" type="checkbox" />
            保持登录状态
          </label>
          <a class="text-xs" href="/docs/" target="_blank" rel="noopener">接口文档</a>
        </div>

        <p v-if="error" class="login__error">
          <AppIcon name="alert-circle" :size="15" />
          {{ error }}
        </p>

        <AppButton type="submit" variant="primary" size="lg" block :loading="loading" icon="logout">
          登录
        </AppButton>

        <p class="login__hint">
          口令在浏览器内用服务端的 RSA 公钥加密后提交，明文不会经过网络。
          账号由管理员在「管理后台 → 账号管理」中创建。
        </p>

        <template v-if="guestAvailable">
          <div class="login__divider"><span>或</span></div>
          <AppButton size="lg" block icon="eye" :disabled="loading" @click="browseAsGuest">
            以访客身份浏览（只读）
          </AppButton>
          <p class="login__hint">访客只能查看与下载管理员开放的内容，无法上传或修改。</p>
        </template>
      </form>
    </section>
  </div>
</template>

<style scoped>
.login {
  min-height: 100vh;
  display: grid;
  grid-template-columns: 1.05fr 1fr;
  background: var(--bg-app);
  position: relative;
}

.login__theme {
  position: absolute;
  top: var(--space-4);
  right: var(--space-4);
  width: 36px;
  height: 36px;
  border-radius: var(--radius-md);
  border: 1px solid var(--border);
  background: var(--surface);
  color: var(--text-muted);
  cursor: pointer;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  z-index: 2;
}

.login__brand {
  position: relative;
  overflow: hidden;
  background:
    radial-gradient(1200px 620px at 12% 8%, rgba(99, 102, 241, 0.35), transparent 62%),
    radial-gradient(900px 520px at 88% 92%, rgba(16, 185, 129, 0.22), transparent 60%),
    linear-gradient(150deg, #1b1f4b 0%, #171a35 58%, #0f1226 100%);
  color: #eef1ff;
  display: flex;
  align-items: center;
  padding: var(--space-16) var(--space-12);
}

.login__brand-inner {
  max-width: 30rem;
  display: flex;
  flex-direction: column;
  gap: var(--space-5);
}

.login__logo {
  width: 52px;
  height: 52px;
  border-radius: var(--radius-lg);
  background: rgba(255, 255, 255, 0.12);
  border: 1px solid rgba(255, 255, 255, 0.2);
  display: flex;
  align-items: center;
  justify-content: center;
}

.login__brand-title {
  font-size: var(--text-3xl);
  color: #fff;
  letter-spacing: -0.02em;
}

.login__brand-desc {
  color: rgba(238, 241, 255, 0.76);
  font-size: var(--text-md);
  line-height: 1.75;
}

.login__features {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-2) var(--space-4);
}

.login__features li {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
  color: rgba(238, 241, 255, 0.9);
}

.login__stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: var(--space-3);
  margin: var(--space-4) 0 0;
  padding-top: var(--space-5);
  border-top: 1px solid rgba(255, 255, 255, 0.14);
}

.login__stats dt {
  font-size: var(--text-2xs);
  color: rgba(238, 241, 255, 0.6);
  text-transform: uppercase;
  letter-spacing: 0.06em;
}

.login__stats dd {
  margin: 4px 0 0;
  font-size: var(--text-md);
  font-weight: 620;
  color: #fff;
}

.login__panel {
  display: flex;
  align-items: center;
  justify-content: center;
  padding: var(--space-12) var(--space-8);
}

.login__form {
  width: 100%;
  max-width: 366px;
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.login__form-head {
  margin-bottom: var(--space-2);
}

.login__form-head h2 {
  font-size: var(--text-2xl);
}

.login__form-head p {
  margin-top: 4px;
}

.login__row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
}

.login__remember {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-xs);
  color: var(--text-muted);
  cursor: pointer;
}

.login__error {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  background: var(--danger-50);
  color: var(--danger-600);
  border-radius: var(--radius-md);
  padding: var(--space-3);
  font-size: var(--text-sm);
}

.login__hint {
  font-size: var(--text-2xs);
  color: var(--text-faint);
  line-height: 1.7;
}

.login__divider {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  color: var(--text-faint);
  font-size: var(--text-2xs);
}

.login__divider::before,
.login__divider::after {
  content: '';
  flex: 1 1 auto;
  height: 1px;
  background: var(--border-subtle);
}

@media (max-width: 960px) {
  .login {
    grid-template-columns: 1fr;
  }

  .login__brand {
    display: none;
  }

  .login__panel {
    padding: var(--space-10) var(--space-5);
    align-items: flex-start;
    padding-top: var(--space-16);
  }
}
</style>
