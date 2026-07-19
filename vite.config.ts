import { rmSync } from 'fs'
import path from 'path'
import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'
import electron from 'vite-plugin-electron'
import renderer from 'vite-plugin-electron-renderer'
import pkg from './package.json'

const sharedAlias = {
  '@shared': path.resolve(__dirname, 'shared'),
  '@main':   path.resolve(__dirname, 'electron/main')
}
const sourceExtensions = ['.mjs', '.mts', '.ts', '.tsx', '.js', '.jsx', '.json']
const electronMainExternal = Object.keys('dependencies' in pkg ? pkg.dependencies : {})

// https://vitejs.dev/config/
// @ts-ignore
export default defineConfig(({ command }) => {
  if (command === 'build') {
    rmSync('dist', { recursive: true, force: true })
    rmSync('release', { recursive: true, force: true })
  }

  const isBuild = command === 'build'
  const devServerUrl = new URL(pkg.debug.env.VITE_DEV_SERVER_URL)
  return {
    resolve: {
      alias: sharedAlias,
      extensions: sourceExtensions
    },
    build: {
      rollupOptions: {
        output: {
          chunkFileNames: '[name].js',
          entryFileNames: '[name].js',
          assetFileNames: '[name].[ext]'
        }
      }
    },
    esbuild: isBuild ? { drop: ['console', 'debugger'] } : {},
    plugins: [
      vue({
        template: {
          compilerOptions: {
            isCustomElement: (tag) => tag == 'Webview'
          }
        }
      }),
      electron([
        {
          entry: 'electron/main/index.ts',
          onstart({ startup }) {
            if (process.env.VSCODE_DEBUG) {
              console.log(/* For `.vscode/.debug.script.mjs` */ '[startup] Electron App')
            } else {
              startup()
            }
          },
          vite: {
            resolve: { alias: sharedAlias, extensions: sourceExtensions },
            build: {
              minify: isBuild,
              outDir: 'dist/electron/main',
              rollupOptions: {
                input: {
                  index: path.resolve(__dirname, 'electron/main/index.ts')
                },
                output: {
                  entryFileNames: '[name].js',
                  chunkFileNames: '[name].js'
                },
                // @ts-ignore
                external: electronMainExternal
              }
            }
          }
        },
        {
          entry: path.join(__dirname, 'electron/preload/index.ts'),
          onstart({ reload }) {
            reload()
          },
          vite: {
            resolve: { alias: sharedAlias, extensions: sourceExtensions },
            build: {
              minify: isBuild,
              outDir: 'dist/electron/preload',
              rollupOptions: {
                // @ts-ignore
                external: Object.keys('dependencies' in pkg ? pkg.dependencies : {})
              }
            }
          }
        }
      ]),
      renderer()
    ],
    server: {
      host: devServerUrl.hostname,
      port: Number(devServerUrl.port),
      strictPort: true,
      hmr: true
    }
  }
})
