import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import { fileURLToPath, URL } from 'node:url'

const abslinkPath = fileURLToPath(new URL('./node_modules/abslink/src/abslink.js', import.meta.url))
const abslinkW3CPath = fileURLToPath(new URL('./node_modules/abslink/adapters/w3c.js', import.meta.url))

export default defineConfig({
  plugins: [vue()],
  base: './',
  // jassub 将 libass 渲染器作为 module worker 打包。它的 worker 使用
  // bare import（abslink / abslink/w3c），在部分 Windows npm 安装布局下
  // Vite 的 worker resolver 无法自动读取 package exports。显式定位到已锁定
  // 的 ESM 入口，既保证构建确定性，也让 worker 依赖继续被打入同一构件。
  resolve: {
    alias: [
      { find: 'abslink/w3c', replacement: abslinkW3CPath },
      { find: 'abslink', replacement: abslinkPath },
    ],
  },
  // jassub 的 libass worker 本身是 module worker，iife 打包不支持代码分割
  worker: { format: 'es' },
  build: {
    outDir: 'dist',
    assetsInlineLimit: 0,
  },
  server: {
    port: 5173,
    strictPort: true,
  },
  test: {
    environment: 'jsdom',
    clearMocks: true,
    restoreMocks: true,
  },
})
