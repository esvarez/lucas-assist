import path from 'node:path'
import { fileURLToPath } from 'node:url'
import react from '@vitejs/plugin-react'
import { defineConfig, loadEnv } from 'vite'
import tailwindcss from '@tailwindcss/vite'

const dirname = path.dirname(fileURLToPath(import.meta.url))

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, dirname, '')
  const apiTarget = env.VITE_API_PROXY_TARGET || 'http://localhost:8080'
  const apiStagePath = env.VITE_API_STAGE_PATH ?? ''

  return {
    plugins: [react(), tailwindcss()],
    resolve: {
      alias: {
        '@': dirname,
      },
    },
    server: {
      proxy: {
        '/api': {
          target: apiTarget,
          changeOrigin: true,
          rewrite: (path) => path.replace(/^\/api/, apiStagePath),
        },
      },
    },
  }
})
