/** 路由表与访问守卫。管理后台的路径以 /manage 开头（同时提供 /@manage 别名）。 */

import { createRouter, createWebHistory } from 'vue-router'

import AppShell from '@/components/layout/AppShell.vue'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'

declare module 'vue-router' {
  interface RouteMeta {
    /** 浏览器标题。 */
    title?: string
    /** 需要登录。 */
    requiresAuth?: boolean
    /** 需要至少一项管理类权限。 */
    requiresManage?: boolean
    /** 公开页面（登录页、匿名分享页）。 */
    public?: boolean
    /** 该页面需要的能力，用于菜单与守卫提示。 */
    capability?: 'user_manage' | 'audit_read' | 'storage_manage' | 'system_manage'
  }
}

const router = createRouter({
  history: createWebHistory(import.meta.env.BASE_URL),
  scrollBehavior: (to, from, saved) => saved ?? { top: 0 },
  routes: [
    {
      path: '/login',
      name: 'login',
      component: () => import('@/views/LoginView.vue'),
      meta: { title: '登录', public: true },
    },
    {
      path: '/s/:token',
      name: 'share-public',
      component: () => import('@/views/ShareView.vue'),
      meta: { title: '分享', public: true },
    },
    {
      path: '/',
      component: AppShell,
      meta: { requiresAuth: true },
      children: [
        { path: '', redirect: { name: 'files' } },
        {
          path: 'files/:folderId?',
          name: 'files',
          component: () => import('@/views/FilesView.vue'),
          meta: { title: '我的文件' },
        },
        {
          path: 'search',
          name: 'search',
          component: () => import('@/views/SearchView.vue'),
          meta: { title: '搜索' },
        },
        {
          path: 'shares',
          name: 'shares',
          component: () => import('@/views/SharesView.vue'),
          meta: { title: '我的分享' },
        },
        {
          path: 'trash',
          name: 'trash',
          component: () => import('@/views/TrashView.vue'),
          meta: { title: '回收站' },
        },
        {
          path: 'uploads',
          name: 'uploads',
          component: () => import('@/views/UploadsView.vue'),
          meta: { title: '传输列表' },
        },
        {
          path: 'account',
          name: 'account',
          component: () => import('@/views/AccountView.vue'),
          meta: { title: '账号设置' },
        },
        {
          path: 'manage',
          alias: '/@manage',
          component: () => import('@/views/manage/ManageLayout.vue'),
          meta: { requiresManage: true, title: '管理后台' },
          children: [
            { path: '', redirect: { name: 'manage-overview' } },
            {
              path: 'overview',
              name: 'manage-overview',
              alias: '/@manage/overview',
              component: () => import('@/views/manage/ManageOverview.vue'),
              meta: { title: '系统概览' },
            },
            {
              path: 'users',
              name: 'manage-users',
              alias: '/@manage/users',
              component: () => import('@/views/manage/ManageUsers.vue'),
              meta: { title: '账号管理', capability: 'user_manage' },
            },
            {
              path: 'audit',
              name: 'manage-audit',
              alias: '/@manage/audit',
              component: () => import('@/views/manage/ManageAudit.vue'),
              meta: { title: '审计日志', capability: 'audit_read' },
            },
            {
              path: 'storage',
              name: 'manage-storage',
              alias: '/@manage/storage',
              component: () => import('@/views/manage/ManageStorage.vue'),
              meta: { title: '存储与维护', capability: 'storage_manage' },
            },
            {
              path: 'permissions',
              name: 'manage-permissions',
              alias: '/@manage/permissions',
              component: () => import('@/views/manage/ManagePermissions.vue'),
              meta: { title: '角色与权限', capability: 'user_manage' },
            },
            {
              path: 'settings',
              name: 'manage-settings',
              alias: '/@manage/settings',
              component: () => import('@/views/manage/ManageSettings.vue'),
              meta: { title: '系统设置', capability: 'system_manage' },
            },
          ],
        },
      ],
    },
    {
      path: '/:pathMatch(.*)*',
      name: 'not-found',
      component: () => import('@/views/NotFoundView.vue'),
      meta: { title: '页面不存在', public: true },
    },
  ],
})

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  const ui = useUiStore()

  // 路由守卫是恢复会话的唯一入口，避免刷新后各页面各自判断登录态。
  if (!auth.ready) {
    await auth.bootstrap()
  }

  const isPublic = to.meta.public === true
  if (!isPublic && !auth.isAuthenticated) {
    return { name: 'login', query: to.fullPath === '/' ? {} : { redirect: to.fullPath } }
  }
  // 访客会话仍然允许打开登录页，好让管理员换成正式账号。
  if (to.name === 'login' && auth.isAuthenticated && !auth.isGuest) {
    return { name: 'files' }
  }
  if (to.meta.requiresManage && !auth.canEnterManage) {
    ui.toast.error('无权访问管理后台', '当前账号没有任何管理类权限')
    return { name: 'files' }
  }

  const capability = to.meta.capability
  if (capability) {
    const allowed =
      capability === 'user_manage'
        ? auth.canManageUsers
        : capability === 'audit_read'
          ? auth.canReadAudit
          : capability === 'storage_manage'
            ? auth.canManageStorage
            : auth.canManageSystem
    if (!allowed) {
      ui.toast.error('权限不足', '当前账号缺少该页面对应的权限')
      return { name: 'manage-overview' }
    }
  }
  return true
})

router.afterEach((to) => {
  const title = to.meta.title
  document.title = title ? `${title} · Nagisa 网盘` : 'Nagisa 网盘'
})

export default router
