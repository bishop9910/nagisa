import { fileURLToPath, URL } from 'node:url'

import { defineConfig, loadEnv } from 'vite'
import vue from '@vitejs/plugin-vue'
import vueDevTools from 'vite-plugin-vue-devtools'

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  // 开发服务器把 /v1 与 /docs 代理到后端，浏览器里只存在一个源站，因此完全
  // 不依赖后端的 web.cors_origins 配置。目标地址按下面的顺序解析：
  //   1. 环境变量 VITE_DEV_API_TARGET（shell 或 .env.development.local）；
  //   2. 默认 127.0.0.1:8000（configs/config.yaml 里的 server.http.addr）。
  const env = loadEnv(mode, process.cwd(), '')
  const apiTarget = env.VITE_DEV_API_TARGET || process.env.VITE_DEV_API_TARGET || 'http://127.0.0.1:8000'

  return {
    plugins: [vue(), vueDevTools()],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      port: 5173,
      proxy: {
        '/v1': { target: apiTarget, changeOrigin: true },
        '/docs': { target: apiTarget, changeOrigin: true },
      },
    },
    build: {
      // 后端从 web.root（默认 ./web/dist）托管前端，构建产物直接落到那里；
      // web.enabled=false 时该目录不生效，用 pnpm dev 即可。
      outDir: fileURLToPath(new URL('../web/dist', import.meta.url)),
      emptyOutDir: true,
      chunkSizeWarningLimit: 900,
    },
  }
})
